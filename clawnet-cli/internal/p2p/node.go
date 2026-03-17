package p2p

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	record "github.com/libp2p/go-libp2p-record"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	libp2pws "github.com/libp2p/go-libp2p/p2p/transport/websocket"
	"github.com/multiformats/go-multiaddr"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/bootstrap"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/btdht"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
)

const (
	// mDNS service tag for LAN discovery
	mdnsServiceTag = "clawnet.local"
	// DHT protocol prefix
	dhtProtocol = "/clawnet"
)

// Node represents a running P2P node.
type Node struct {
	Host            host.Host
	DHT             *dht.IpfsDHT
	PubSub          *pubsub.PubSub
	Topics          map[string]*pubsub.Topic
	Subs            map[string]*pubsub.Subscription
	Config          *config.Config
	BwCounter       *metrics.BandwidthCounter
	BTDHT           *btdht.Discovery
	cancelFunc      context.CancelFunc

	mu sync.RWMutex
}

// NewNode creates and starts a libp2p node.
func NewNode(ctx context.Context, priv crypto.PrivKey, cfg *config.Config) (*Node, error) {
	ctx, cancel := context.WithCancel(ctx)

	listenAddrs := make([]multiaddr.Multiaddr, 0, len(cfg.ListenAddrs))
	for _, s := range cfg.ListenAddrs {
		ma, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("invalid listen addr %q: %w", s, err)
		}
		listenAddrs = append(listenAddrs, ma)
	}

	// Parse announce addresses for AddrsFactory (Docker/K8s external addrs).
	var announceAddrs []multiaddr.Multiaddr
	for _, s := range cfg.AnnounceAddrs {
		ma, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("invalid announce addr %q: %w", s, err)
		}
		announceAddrs = append(announceAddrs, ma)
	}

	// Parse bootstrap peers into AddrInfo for relay and direct connect.
	var bootstrapInfos []peer.AddrInfo
	for _, addr := range cfg.BootstrapPeers {
		ma, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			continue
		}
		pi, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			continue
		}
		bootstrapInfos = append(bootstrapInfos, *pi)
	}

	bwc := metrics.NewBandwidthCounter()

	opts := []libp2p.Option{
		libp2p.Identity(priv),
		libp2p.ListenAddrs(listenAddrs...),
		libp2p.Security(noise.ID, noise.New),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(libp2pquic.NewTransport),
		libp2p.Transport(libp2pws.New),
		libp2p.ConnectionManager(NewConnManager(cfg.MaxConnections)),
		libp2p.BandwidthReporter(bwc),
		libp2p.NATPortMap(),
		libp2p.EnableNATService(),
		libp2p.EnableHolePunching(),
	}

	// Override advertised addresses when running behind NAT/Docker.
	if len(announceAddrs) > 0 {
		addrs := announceAddrs // capture for closure
		opts = append(opts, libp2p.AddrsFactory(func(_ []multiaddr.Multiaddr) []multiaddr.Multiaddr {
			return addrs
		}))
	}

	// Force private reachability so AutoNAT immediately seeks relay
	// without waiting for (failing) reachability probes.
	if cfg.ForcePrivate {
		opts = append(opts, libp2p.ForceReachabilityPrivate())
	}

	if cfg.RelayEnabled && cfg.LayerEnabled("relay") {
		opts = append(opts,
			libp2p.EnableRelay(),
			libp2p.EnableRelayService(),
		)
		// AutoRelay obtains relay addresses through bootstrap nodes
		// so container/NATed nodes become reachable via circuit relay.
		if len(bootstrapInfos) > 0 {
			opts = append(opts, libp2p.EnableAutoRelayWithStaticRelays(bootstrapInfos))
		}
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	node := &Node{
		Host:       h,
		Config:     cfg,
		BwCounter:  bwc,
		Topics:     make(map[string]*pubsub.Topic),
		Subs:       make(map[string]*pubsub.Subscription),
		cancelFunc: cancel,
	}

	// Initialize Kademlia DHT
	if err := node.setupDHT(ctx); err != nil {
		h.Close()
		cancel()
		return nil, fmt.Errorf("failed to setup DHT: %w", err)
	}

	// Initialize GossipSub
	if err := node.setupPubSub(ctx); err != nil {
		h.Close()
		cancel()
		return nil, fmt.Errorf("failed to setup PubSub: %w", err)
	}

	// Start mDNS discovery for LAN
	if cfg.LayerEnabled("mdns") {
		if err := node.setupMDNS(ctx); err != nil {
			// mDNS failure is non-fatal — log and continue
			fmt.Printf("warning: mDNS setup failed: %v\n", err)
		}
	}

	// Connect to bootstrap peers
	node.connectBootstrapPeers(ctx)

	// Start HTTP bootstrap fetch (GitHub Pages) in background
	if cfg.HTTPBootstrap && cfg.LayerEnabled("bootstrap") {
		go node.httpBootstrap(ctx)
	}

	// Start K8s headless-service DNS discovery in background
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" && cfg.LayerEnabled("k8s") {
		go node.k8sDiscovery(ctx)
	}

	// Start BT Mainline DHT discovery in background
	if cfg.BTDHT.Enabled && cfg.LayerEnabled("bt-dht") {
		go node.startBTDHT(ctx, cfg)
	}

	// Start DHT routing discovery in background
	if cfg.LayerEnabled("dht") {
		go node.discoverPeers(ctx)
	}

	// Auto-join configured topics
	for _, topic := range cfg.TopicsAutoJoin {
		if _, err := node.JoinTopic(topic); err != nil {
			fmt.Printf("warning: failed to join topic %s: %v\n", topic, err)
		}
	}

	return node, nil
}

func (n *Node) setupDHT(ctx context.Context) error {
	var err error
	n.DHT, err = dht.New(ctx, n.Host,
		dht.Mode(dht.ModeAutoServer),
		dht.ProtocolPrefix(dhtProtocol),
		dht.Validator(record.NamespacedValidator{
			"clawnet-profile": NewProfileValidator(),
			"clawnet-txn":     NewTxnValidator(),
			"clawnet-rep":     NewRepValidator(),
		}),
	)
	if err != nil {
		return err
	}
	return n.DHT.Bootstrap(ctx)
}

func (n *Node) setupPubSub(ctx context.Context) error {
	var err error
	n.PubSub, err = pubsub.NewGossipSub(ctx, n.Host,
		pubsub.WithMessageSignaturePolicy(pubsub.StrictSign),
		pubsub.WithFloodPublish(true),
	)
	return err
}

func (n *Node) setupMDNS(ctx context.Context) error {
	notifee := &mdnsNotifee{host: n.Host, ctx: ctx}
	service := mdns.NewMdnsService(n.Host, mdnsServiceTag, notifee)
	return service.Start()
}

func (n *Node) connectBootstrapPeers(ctx context.Context) {
	for _, peerAddr := range n.Config.BootstrapPeers {
		ma, err := multiaddr.NewMultiaddr(peerAddr)
		if err != nil {
			fmt.Printf("warning: invalid bootstrap addr %s: %v\n", peerAddr, err)
			continue
		}
		pi, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			fmt.Printf("warning: invalid bootstrap peer info %s: %v\n", peerAddr, err)
			continue
		}
		go func(pi peer.AddrInfo) {
			connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			if err := n.Host.Connect(connectCtx, pi); err != nil {
				fmt.Printf("warning: failed to connect to bootstrap %s: %v\n", pi.ID.String()[:16], err)
			} else {
				fmt.Printf("connected to bootstrap peer %s\n", pi.ID.String()[:16])
			}
		}(*pi)
	}
}

func (n *Node) discoverPeers(ctx context.Context) {
	routingDiscovery := drouting.NewRoutingDiscovery(n.DHT)
	// Advertise ourselves
	for {
		_, err := routingDiscovery.Advertise(ctx, mdnsServiceTag)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
				continue
			}
		}
		break
	}

	// Discover peers
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			peerChan, err := routingDiscovery.FindPeers(ctx, mdnsServiceTag)
			if err != nil {
				continue
			}
			for p := range peerChan {
				if p.ID == n.Host.ID() || len(p.Addrs) == 0 {
					continue
				}
				n.Host.Connect(ctx, p)
			}
		}
	}
}

// httpBootstrap fetches bootstrap peers from GitHub Pages and connects.
func (n *Node) httpBootstrap(ctx context.Context) {
	list, err := bootstrap.FetchPeers(ctx)
	if err != nil {
		fmt.Printf("http-bootstrap: failed: %v\n", err)
		return
	}
	fmt.Printf("http-bootstrap: fetched %d peers (v%d)\n", len(list.Nodes), list.Version)
	for _, addr := range list.Nodes {
		ma, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			continue
		}
		pi, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			continue
		}
		go func(pi peer.AddrInfo) {
			cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			if err := n.Host.Connect(cctx, pi); err == nil {
				fmt.Printf("http-bootstrap: connected to %s\n", pi.ID.String()[:16])
			}
		}(*pi)
	}
}

// startBTDHT initializes and runs the BitTorrent Mainline DHT discovery.
func (n *Node) startBTDHT(ctx context.Context, cfg *config.Config) {
	// Determine port to announce: prefer announce addr, fall back to host addr.
	libp2pPort := config.DefaultP2PPort
	if len(cfg.AnnounceAddrs) > 0 {
		if ma, err := multiaddr.NewMultiaddr(cfg.AnnounceAddrs[0]); err == nil {
			if p, err := ma.ValueForProtocol(multiaddr.P_TCP); err == nil {
				fmt.Sscanf(p, "%d", &libp2pPort)
			}
		}
	} else {
		for _, addr := range n.Host.Addrs() {
			if p, err := addr.ValueForProtocol(multiaddr.P_TCP); err == nil {
				fmt.Sscanf(p, "%d", &libp2pPort)
				break
			}
		}
	}

	disc, err := btdht.NewDiscovery(cfg.BTDHT.ListenPort, libp2pPort)
	if err != nil {
		fmt.Printf("bt-dht: setup failed: %v\n", err)
		return
	}
	n.BTDHT = disc

	if err := disc.Bootstrap(); err != nil {
		fmt.Printf("bt-dht: bootstrap failed: %v\n", err)
		// Continue anyway — partial bootstrap can still work
	}

	fmt.Printf("bt-dht: running on UDP :%d, announcing libp2p port %d\n",
		cfg.BTDHT.ListenPort, libp2pPort)

	disc.RunLoop(ctx, func(peers []btdht.PeerAddr) {
		for _, p := range peers {
			// Try connecting via both TCP and QUIC on the announced port
			addrs := []string{
				fmt.Sprintf("/ip4/%s/tcp/%d", p.IP, p.Port),
				fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", p.IP, p.Port),
			}
			for _, addrStr := range addrs {
				ma, err := multiaddr.NewMultiaddr(addrStr)
				if err != nil {
					continue
				}
				go func(ma multiaddr.Multiaddr) {
					cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
					defer cancel()
					// We don't have the peer ID — libp2p will discover it
					// during the Noise handshake. Non-libp2p endpoints will
					// fail quickly at protocol negotiation.
					n.Host.Connect(cctx, peer.AddrInfo{
						Addrs: []multiaddr.Multiaddr{ma},
					})
				}(ma)
			}
		}
	})
}

// JoinTopic joins a GossipSub topic and subscribes to it.
func (n *Node) JoinTopic(topicName string) (*pubsub.Subscription, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if sub, ok := n.Subs[topicName]; ok {
		return sub, nil
	}

	topic, err := n.PubSub.Join(topicName)
	if err != nil {
		return nil, err
	}

	sub, err := topic.Subscribe()
	if err != nil {
		return nil, err
	}

	n.Topics[topicName] = topic
	n.Subs[topicName] = sub
	return sub, nil
}

// Publish sends a message to a GossipSub topic.
func (n *Node) Publish(ctx context.Context, topicName string, data []byte) error {
	n.mu.RLock()
	topic, ok := n.Topics[topicName]
	n.mu.RUnlock()

	if !ok {
		return fmt.Errorf("not subscribed to topic %s", topicName)
	}
	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return topic.Publish(pubCtx, data)
}

// Close shuts down the node gracefully.
func (n *Node) Close() error {
	n.cancelFunc()
	if n.BTDHT != nil {
		n.BTDHT.Close()
	}
	if n.DHT != nil {
		n.DHT.Close()
	}
	return n.Host.Close()
}

// PeerID returns this node's peer ID.
func (n *Node) PeerID() peer.ID {
	return n.Host.ID()
}

// ConnectedPeers returns the list of currently connected peer IDs.
func (n *Node) ConnectedPeers() []peer.ID {
	return n.Host.Network().Peers()
}

// Addrs returns the node's listen addresses.
func (n *Node) Addrs() []multiaddr.Multiaddr {
	return n.Host.Addrs()
}

// MU returns the node's read-write mutex for external access to Topics/Subs maps.
func (n *Node) MU() *sync.RWMutex {
	return &n.mu
}

// k8sDiscovery resolves a Kubernetes headless service to pod IPs and connects.
// Set CLAWNET_K8S_SERVICE to the headless service DNS name, e.g.
// "clawnet-headless.default.svc.cluster.local". Runs every 30s.
func (n *Node) k8sDiscovery(ctx context.Context) {
	svcName := os.Getenv("CLAWNET_K8S_SERVICE")
	if svcName == "" {
		svcName = "clawnet-headless"
	}
	fmt.Printf("k8s-discovery: watching service %s\n", svcName)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	resolve := func() {
		ips, err := net.DefaultResolver.LookupHost(ctx, svcName)
		if err != nil {
			return
		}
		for _, ip := range ips {
			addr := fmt.Sprintf("/ip4/%s/tcp/4001", ip)
			ma, err := multiaddr.NewMultiaddr(addr)
			if err != nil {
				continue
			}
			// We don't know the peer ID yet, connect by addr only.
			pi, err := peer.AddrInfosFromP2pAddrs(ma)
			if err != nil || len(pi) == 0 {
				// No peer ID in addr — wrap in AddrInfo with empty ID and try direct connect
				cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
				_ = n.Host.Connect(cctx, peer.AddrInfo{Addrs: []multiaddr.Multiaddr{ma}})
				cancel()
			}
		}
	}

	resolve() // initial probe
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			resolve()
		}
	}
}
