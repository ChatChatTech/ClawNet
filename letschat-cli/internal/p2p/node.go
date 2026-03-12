package p2p

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/multiformats/go-multiaddr"

	"letchat-cli/internal/config"
)

const (
	// mDNS service tag for LAN discovery
	mdnsServiceTag = "letchat.local"
	// DHT protocol prefix
	dhtProtocol = "/letchat"
)

// Node represents a running P2P node.
type Node struct {
	Host       host.Host
	DHT        *dht.IpfsDHT
	PubSub     *pubsub.PubSub
	Topics     map[string]*pubsub.Topic
	Subs       map[string]*pubsub.Subscription
	Config     *config.Config
	cancelFunc context.CancelFunc

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

	opts := []libp2p.Option{
		libp2p.Identity(priv),
		libp2p.ListenAddrs(listenAddrs...),
		libp2p.Security(noise.ID, noise.New),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(libp2pquic.NewTransport),
		libp2p.ConnectionManager(NewConnManager(cfg.MaxConnections)),
		libp2p.NATPortMap(),
		libp2p.EnableNATService(),
		libp2p.EnableHolePunching(),
	}

	if cfg.RelayEnabled {
		opts = append(opts,
			libp2p.EnableRelay(),
			libp2p.EnableRelayService(),
		)
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	node := &Node{
		Host:       h,
		Config:     cfg,
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
	if err := node.setupMDNS(ctx); err != nil {
		// mDNS failure is non-fatal — log and continue
		fmt.Printf("warning: mDNS setup failed: %v\n", err)
	}

	// Connect to bootstrap peers
	node.connectBootstrapPeers(ctx)

	// Start DHT routing discovery in background
	go node.discoverPeers(ctx)

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
	return topic.Publish(ctx, data)
}

// Close shuts down the node gracefully.
func (n *Node) Close() error {
	n.cancelFunc()
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
