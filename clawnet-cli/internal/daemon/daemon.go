package daemon

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
	cryptoe "github.com/ChatChatTech/ClawNet/clawnet-cli/internal/crypto"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/geo"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/identity"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/overlay"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/p2p"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/pow"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"

	"github.com/Arceliar/ironwood/network"
)

const Version = "0.9.8"

// Daemon holds the running node and all services.
type Daemon struct {
	Node       *p2p.Node
	Config     *config.Config
	Profile    *config.Profile
	Store      *store.Store
	Geo        *geo.Locator
	Overlay    *overlay.Transport
	Crypto     *cryptoe.Engine
	DataDir    string
	StartedAt  time.Time
	ctx        context.Context
	PeerMottos      sync.Map // peer_id -> string
	PeerAgentNames  sync.Map // peer_id -> string
	hotPeers        sync.Map // peer_id -> hotPeer (for reconnect)
	rxBytes    atomic.Uint64
	txBytes    atomic.Uint64
	nicName    string
	hbState    *heartbeatState
	geoCache   *overlayGeoCache
}

// getTrafficBytes returns cumulative rx/tx counters from libp2p bandwidth.
func (d *Daemon) getTrafficBytes() (uint64, uint64) {
	return d.rxBytes.Load(), d.txBytes.Load()
}

// Start initializes and runs the daemon until interrupted.
// devLayers optionally restricts which discovery/transport layers start (empty = all).
func Start(foreground bool, devLayers []string) error {
	dataDir := config.DataDir()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg.DevLayers = devLayers

	priv, err := identity.LoadOrGenerate(dataDir)
	if err != nil {
		return fmt.Errorf("load identity: %w", err)
	}

	peerID, err := identity.PeerIDFromKey(priv)
	if err != nil {
		return fmt.Errorf("derive peer ID: %w", err)
	}

	fmt.Printf("ClawNet Daemon v%s\n", Version)
	if len(devLayers) > 0 {
		fmt.Printf(">>> DEV MODE — active layers: %v\n", devLayers)
	}
	fmt.Printf("Peer ID: %s\n", peerID.String())
	fmt.Printf("Data dir: %s\n", dataDir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// STUN: auto-detect external IP if no AnnounceAddrs configured.
	if len(cfg.AnnounceAddrs) == 0 && cfg.LayerEnabled("stun") {
		if extIP := p2p.DetectExternalIP(); extIP != "" {
			fmt.Printf("STUN detected external IP: %s\n", extIP)
			// Determine ip4 vs ip6 protocol based on address format
			proto := "ip4"
			if strings.Contains(extIP, ":") {
				proto = "ip6"
			}
			cfg.AnnounceAddrs = []string{
				fmt.Sprintf("/%s/%s/tcp/4001", proto, extIP),
				fmt.Sprintf("/%s/%s/udp/4001/quic-v1", proto, extIP),
				fmt.Sprintf("/%s/%s/tcp/4002/ws", proto, extIP),
			}
		}
	}

	// Overlay mesh: detect 200::/7 IPv6 addresses from system interfaces.
	// If the machine has overlay mesh connectivity, any peer on the mesh can
	// directly dial us via these addresses — zero configuration needed.
	if overlayAddrs := detectOverlayAddrs(); len(overlayAddrs) > 0 {
		for _, addr := range overlayAddrs {
			fmt.Printf("Overlay IPv6 detected: %s\n", addr)
			cfg.AnnounceAddrs = append(cfg.AnnounceAddrs,
				fmt.Sprintf("/ip6/%s/tcp/4001", addr),
				fmt.Sprintf("/ip6/%s/udp/4001/quic-v1", addr),
			)
		}
	}

	// Open local SQLite store (needed before node creation for Matrix token persistence)
	db, err := store.Open(dataDir)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	// Load or create profile (from DB now)
	profile := loadProfile(db)
	profile.Version = Version

	// One-time migration: import legacy JSON files into DB
	migrateJSONFiles(dataDir, db)

	node, err := p2p.NewNode(ctx, priv, cfg)
	if err != nil {
		return fmt.Errorf("start p2p node: %w", err)
	}
	// Inject DB-backed token store so matrix discovery persists tokens in SQLite
	node.MatrixTokenStore = &matrixTokenAdapter{db: db}
	node.StartMatrixDiscovery(ctx, priv)
	defer node.Close()

	// Print listen addresses
	for _, addr := range node.Addrs() {
		fmt.Printf("Listening on: %s/p2p/%s\n", addr, peerID.String())
	}

	// Print discovery layer status
	layerStatus := func(name, label string, enabled bool) {
		if !enabled {
			return
		}
		if cfg.LayerEnabled(name) {
			fmt.Printf("  %-15s enabled\n", label+":")
		} else {
			fmt.Printf("  %-15s SKIPPED (dev mode)\n", label+":")
		}
	}
	fmt.Println("Discovery layers:")
	layerStatus("mdns", "mDNS", true)
	layerStatus("bootstrap", "HTTP Bootstrap", cfg.HTTPBootstrap)
	layerStatus("bt-dht", "BT DHT", cfg.BTDHT.Enabled)
	layerStatus("dht", "Kademlia DHT", true)
	layerStatus("relay", "Relay", cfg.RelayEnabled)
	// Check for WebSocket listener
	for _, a := range cfg.ListenAddrs {
		if strings.HasSuffix(a, "/ws") {
			fmt.Println("  WebSocket:       enabled")
			break
		}
	}
	if cfg.ForcePrivate {
		fmt.Println("  NAT mode:        force_private")
	}
	if len(cfg.AnnounceAddrs) > 0 {
		fmt.Printf("  Announce:        %v\n", cfg.AnnounceAddrs)
	}
	fmt.Printf("  Bootstrap:       %d peers configured\n", len(cfg.BootstrapPeers))
	layerStatus("matrix", "Matrix", cfg.MatrixDiscovery.Enabled)
	layerStatus("overlay", "Overlay", cfg.Overlay.Enabled)
	layerStatus("stun", "STUN", true)

	// Initialize geo locator
	geoLoc, err := geo.NewLocator(dataDir)
	if err != nil {
		fmt.Printf("warning: geo locator unavailable: %v\n", err)
	} else {
		defer geoLoc.Close()
		fmt.Printf("Geo database: %s\n", geoLoc.DBType())
	}

	// Initialize E2E crypto engine
	cryptoEngine, err := cryptoe.NewEngine(priv, db.DB)
	if err != nil {
		fmt.Printf("[crypto] E2E engine init failed: %v (DMs will be unencrypted)\n", err)
	} else {
		fmt.Println("[crypto] E2E encryption: enabled (NaCl box)")
	}

	d := &Daemon{
		Node:      node,
		Config:    cfg,
		Profile:   profile,
		Store:     db,
		Geo:       geoLoc,
		Crypto:    cryptoEngine,
		DataDir:   dataDir,
		StartedAt: time.Now(),
		ctx:       ctx,
	}

	// Start overlay transport (if enabled)
	if cfg.Overlay.Enabled && cfg.LayerEnabled("overlay") {
		// Build overlay network options for deep fusion
		var overlayOpts []network.Option

		// PathNotify → libp2p bridge: overlay path discovery triggers libp2p connect
		bridge := overlay.NewPathBridge(node.Host, nil, time.Second)
		overlayOpts = append(overlayOpts, network.WithPathNotify(bridge.OnPathNotify))

		// Reputation-weighted bloom transform: higher rep nodes get distinct bloom signatures
		overlayOpts = append(overlayOpts, network.WithBloomTransform(
			overlay.ReputationBloomTransform(func() float64 {
				rec, err := db.GetReputation(node.PeerID().String())
				if err != nil {
					return 50.0 // default
				}
				return rec.Score
			}),
		))

		ot, err := overlay.NewTransport(priv, cfg.Overlay.ListenPort, cfg.Overlay.StaticPeers, cfg.Overlay.BootstrapPeers, overlayOpts...)
		if err != nil {
			fmt.Printf("[overlay] init failed: %v (non-fatal)\n", err)
		} else {
			d.Overlay = ot
			bridge.SetTransport(ot)

			// Setup TUN device for overlay IPv6
			if err := ot.SetupTUN(); err != nil {
				fmt.Printf("[tun] setup failed: %v (non-fatal)\n", err)
			}

			// Set initial molt state from config
			if cfg.Overlay.Molted {
				ot.Molt()
			}

			go ot.Run(ctx)
			fmt.Println("[overlay] transport started")

			// Start async geo cache for overlay peers
			d.geoCache = newOverlayGeoCache(ot, geoLoc)
			go d.geoCache.run(ctx.Done())

			// Periodically sync libp2p peers → clawPeers for TUN filtering
			go d.syncClawPeers(ctx)
		}
	}

	// Write PID file
	pidPath := filepath.Join(dataDir, "daemon.pid")
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0600); err != nil {
		fmt.Printf("warning: could not write PID file: %v\n", err)
	}
	defer os.Remove(pidPath)

	// Start API server
	apiServer := d.StartAPI(ctx)
	defer apiServer.Close()

	// Start GossipSub message handlers for knowledge and topic rooms
	d.startGossipHandlers(ctx)

	// Start Phase 2 gossip handlers (tasks, swarm)
	d.startPhase2Gossip(ctx)

	// Start prediction settlement loop (auto-settle expired pending predictions)
	go d.predictionSettlementLoop(ctx)

	// Start Auction House auto-settlement loop
	d.startTaskSettler(ctx)

	// Start silent auto-updater
	d.startAutoUpdater(ctx)

	// Anti-Sybil: require one-time PoW before granting initial credits
	peerIDStr := node.PeerID().String()

	var proof *pow.Proof
	if sp, _ := db.LoadPoWProof(peerIDStr); sp != nil {
		proof = &pow.Proof{PeerID: sp.PeerID, Nonce: sp.Nonce, Difficulty: sp.Difficulty}
	}
	if proof == nil || proof.PeerID != peerIDStr || !pow.Verify(proof.PeerID, proof.Nonce, pow.DefaultDifficulty) {
		fmt.Printf("[PoW] Solving proof-of-work (one-time, ~45s)...\n")
		nonce := pow.Solve(peerIDStr, pow.DefaultDifficulty)
		proof = &pow.Proof{PeerID: peerIDStr, Nonce: nonce, Difficulty: pow.DefaultDifficulty}
		db.SavePoWProof(&store.PoWProof{PeerID: peerIDStr, Nonce: nonce, Difficulty: pow.DefaultDifficulty})
		fmt.Printf("[PoW] Solved! nonce=%d\n", nonce)
	}
	// Initial grant: 4200 Shell (PoW existence proof).
	// Complete the tutorial (+4200 Shell) for a total of 8400 Shell starting balance.
	d.Store.EnsureCreditAccount(peerIDStr, 4200)

	// Seed built-in tutorial task (one-time onboarding)
	d.seedTutorialTask()

	// Register libp2p stream handler for direct messages
	d.registerDMHandler()
	d.registerOverlayDMHandler()

	// Register P2P bundle transfer stream handler
	d.registerBundleHandler()

	// Register history sync stream handler and start sync
	d.registerSyncHandler()
	d.startHistorySync(ctx)

	// Watch for peer connect/disconnect to push topology updates
	d.watchPeerEvents()

	// Start periodic peer latency measurement
	go d.pingLoop(ctx)

	// Start relay health monitoring (ping relays, discover backups)
	go d.relayHealthLoop(ctx)

	// Publish profile to DHT and start periodic refresh
	d.startProfilePublisher(ctx)

	// Start heartbeat (periodic inbox/feed/tasks check)
	d.startHeartbeat(ctx)

	fmt.Printf("API server: http://localhost:%d\n", cfg.WebUIPort)
	fmt.Printf("Node is running. Press Ctrl+C to stop.\n")

	// Wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nShutting down...")
	return nil
}

func loadProfile(db *store.Store) *config.Profile {
	pe, err := db.LoadProfile()
	if err != nil || pe == nil {
		return &config.Profile{
			AgentName:  "ClawNet Node",
			Visibility: config.DefaultVisibility,
			Domains:    []string{},
			Capabilities: []string{},
		}
	}
	return &config.Profile{
		AgentName:    pe.AgentName,
		Visibility:   pe.Visibility,
		Domains:      pe.Domains,
		Capabilities: pe.Capabilities,
		Bio:          pe.Bio,
		Motto:        pe.Motto,
		GeoCity:      pe.GeoCity,
		GeoLatFuzzy:  pe.GeoLatFuzzy,
		GeoLonFuzzy:  pe.GeoLonFuzzy,
		Version:      pe.Version,
	}
}

// saveProfile persists the current profile to the database.
func (d *Daemon) saveProfile() error {
	p := d.Profile
	return d.Store.SaveProfile(&store.ProfileEntry{
		AgentName:    p.AgentName,
		Visibility:   p.Visibility,
		Domains:      p.Domains,
		Capabilities: p.Capabilities,
		Bio:          p.Bio,
		Motto:        p.Motto,
		GeoCity:      p.GeoCity,
		GeoLatFuzzy:  p.GeoLatFuzzy,
		GeoLonFuzzy:  p.GeoLonFuzzy,
		Version:      p.Version,
	})
}

// startProfilePublisher publishes the profile to DHT on startup
// and refreshes every 10 minutes.
func (d *Daemon) startProfilePublisher(ctx context.Context) {
	publish := func() {
		if err := d.Node.PublishProfile(ctx, d.Profile); err != nil {
			fmt.Printf("dht-profile: publish failed: %v\n", err)
		} else {
			fmt.Println("dht-profile: published to DHT")
		}
	}

	// Initial publish after a short delay (let DHT warm up)
	go func() {
		select {
		case <-time.After(15 * time.Second):
		case <-ctx.Done():
			return
		}
		publish()

		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				publish()
			}
		}
	}()
}

// detectOverlayAddrs scans network interfaces for overlay 200::/7 IPv6 addresses.
// If the machine has overlay mesh connectivity, these addresses are reachable by any
// peer on the mesh network, enabling direct connectivity without NAT traversal.
func detectOverlayAddrs() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var addrs []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		ifAddrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range ifAddrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP
			// 200::/7 range (0x02xx or 0x03xx first byte) — overlay addresses
			if len(ip) == net.IPv6len && (ip[0]&0xFE) == 0x02 {
				addrs = append(addrs, ip.String())
			}
		}
	}
	return addrs
}

// syncClawPeers periodically copies libp2p peer keys into the overlay's
// ClawNet peer set for TUN IPv6 filtering.
func (d *Daemon) syncClawPeers(ctx context.Context) {
	if d.Overlay == nil {
		return
	}
	// Initial sync after a short delay
	time.Sleep(5 * time.Second)
	d.doSyncClawPeers()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.doSyncClawPeers()
		}
	}
}

func (d *Daemon) doSyncClawPeers() {
	for _, pid := range d.Node.Host.Peerstore().Peers() {
		pub, err := pid.ExtractPublicKey()
		if err != nil {
			continue
		}
		raw, err := pub.Raw()
		if err != nil || len(raw) != ed25519.PublicKeySize {
			continue
		}
		d.Overlay.RegisterClawPeer(ed25519.PublicKey(raw))
	}
}
