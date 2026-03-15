package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/geo"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/identity"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/p2p"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/pow"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
)

const Version = "0.8.5"

// Daemon holds the running node and all services.
type Daemon struct {
	Node       *p2p.Node
	Config     *config.Config
	Profile    *config.Profile
	Store      *store.Store
	Geo        *geo.Locator
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
}

// getTrafficBytes returns cumulative rx/tx counters from libp2p bandwidth.
func (d *Daemon) getTrafficBytes() (uint64, uint64) {
	return d.rxBytes.Load(), d.txBytes.Load()
}

// Start initializes and runs the daemon until interrupted.
func Start(foreground bool) error {
	dataDir := config.DataDir()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	priv, err := identity.LoadOrGenerate(dataDir)
	if err != nil {
		return fmt.Errorf("load identity: %w", err)
	}

	peerID, err := identity.PeerIDFromKey(priv)
	if err != nil {
		return fmt.Errorf("derive peer ID: %w", err)
	}

	fmt.Printf("ClawNet Daemon v%s\n", Version)
	fmt.Printf("Peer ID: %s\n", peerID.String())
	fmt.Printf("Data dir: %s\n", dataDir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// STUN: auto-detect external IP if no AnnounceAddrs configured.
	if len(cfg.AnnounceAddrs) == 0 {
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

	node, err := p2p.NewNode(ctx, priv, cfg)
	if err != nil {
		return fmt.Errorf("start p2p node: %w", err)
	}
	defer node.Close()

	// Print listen addresses
	for _, addr := range node.Addrs() {
		fmt.Printf("Listening on: %s/p2p/%s\n", addr, peerID.String())
	}

	// Print discovery layer status
	fmt.Println("Discovery layers:")
	fmt.Println("  mDNS:          enabled (LAN)")
	if cfg.HTTPBootstrap {
		fmt.Println("  HTTP Bootstrap: enabled")
	}
	if cfg.BTDHT.Enabled {
		fmt.Printf("  BT DHT:        enabled (UDP :%d)\n", cfg.BTDHT.ListenPort)
	}
	fmt.Println("  Kademlia DHT:  enabled (30s poll)")
	if cfg.RelayEnabled {
		fmt.Println("  Relay:         enabled")
	}
	// Check for WebSocket listener
	for _, a := range cfg.ListenAddrs {
		if strings.HasSuffix(a, "/ws") {
			fmt.Println("  WebSocket:     enabled")
			break
		}
	}
	if cfg.ForcePrivate {
		fmt.Println("  NAT mode:      force_private")
	}
	if len(cfg.AnnounceAddrs) > 0 {
		fmt.Printf("  Announce:      %v\n", cfg.AnnounceAddrs)
	}
	fmt.Printf("  Bootstrap:     %d peers configured\n", len(cfg.BootstrapPeers))

	// Load or create profile
	profile := loadProfile(dataDir)
	profile.Version = Version

	// Open local SQLite store
	db, err := store.Open(dataDir)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	// Initialize geo locator
	geoLoc, err := geo.NewLocator(dataDir)
	if err != nil {
		fmt.Printf("warning: geo locator unavailable: %v\n", err)
	} else {
		defer geoLoc.Close()
		fmt.Printf("Geo database: %s\n", geoLoc.DBType())
	}

	d := &Daemon{
		Node:      node,
		Config:    cfg,
		Profile:   profile,
		Store:     db,
		Geo:       geoLoc,
		DataDir:   dataDir,
		StartedAt: time.Now(),
		ctx:       ctx,
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

	// Anti-Sybil: require one-time PoW before granting initial credits
	peerIDStr := node.PeerID().String()
	proof := pow.LoadProof(dataDir)
	if proof == nil || proof.PeerID != peerIDStr || !pow.Verify(proof.PeerID, proof.Nonce, pow.DefaultDifficulty) {
		fmt.Printf("[PoW] Solving proof-of-work (one-time, ~3s)...\n")
		nonce := pow.Solve(peerIDStr, pow.DefaultDifficulty)
		proof = &pow.Proof{PeerID: peerIDStr, Nonce: nonce, Difficulty: pow.DefaultDifficulty}
		pow.SaveProof(dataDir, proof)
		fmt.Printf("[PoW] Solved! nonce=%d\n", nonce)
	}
	// Initial grant: 10 credits (just enough to explore).
	// Complete the tutorial (+50) for a productive starting balance of 60.
	d.Store.EnsureCreditAccount(peerIDStr, 10.0)

	// Seed built-in tutorial task (one-time onboarding)
	d.seedTutorialTask()

	// Register libp2p stream handler for direct messages
	d.registerDMHandler()

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

func loadProfile(dataDir string) *config.Profile {
	profilePath := filepath.Join(dataDir, "profile.json")
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return &config.Profile{
			AgentName:  "ClawNet Node",
			Visibility: config.DefaultVisibility,
			Domains:    []string{},
			Capabilities: []string{},
		}
	}
	var p config.Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return &config.Profile{
			AgentName:  "ClawNet Node",
			Visibility: config.DefaultVisibility,
			Domains:    []string{},
			Capabilities: []string{},
		}
	}
	return &p
}

// saveProfile persists the current profile to disk.
func (d *Daemon) saveProfile() error {
	data, err := json.MarshalIndent(d.Profile, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d.DataDir, "profile.json"), data, 0600)
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
