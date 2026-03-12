package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/ChatChatTech/letschat/letschat-cli/internal/config"
	"github.com/ChatChatTech/letschat/letschat-cli/internal/geo"
	"github.com/ChatChatTech/letschat/letschat-cli/internal/identity"
	"github.com/ChatChatTech/letschat/letschat-cli/internal/p2p"
	"github.com/ChatChatTech/letschat/letschat-cli/internal/store"
)

const Version = "0.4.0"

// Daemon holds the running node and all services.
type Daemon struct {
	Node    *p2p.Node
	Config  *config.Config
	Profile *config.Profile
	Store   *store.Store
	Geo     *geo.Locator
	DataDir string
	ctx     context.Context
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

	fmt.Printf("LetChat Daemon v%s\n", Version)
	fmt.Printf("Peer ID: %s\n", peerID.String())
	fmt.Printf("Data dir: %s\n", dataDir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	node, err := p2p.NewNode(ctx, priv, cfg)
	if err != nil {
		return fmt.Errorf("start p2p node: %w", err)
	}
	defer node.Close()

	// Print listen addresses
	for _, addr := range node.Addrs() {
		fmt.Printf("Listening on: %s/p2p/%s\n", addr, peerID.String())
	}

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
		Node:    node,
		Config:  cfg,
		Profile: profile,
		Store:   db,
		Geo:     geoLoc,
		DataDir: dataDir,
		ctx:     ctx,
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

	// Ensure local credit account exists with initial balance
	d.Store.EnsureCreditAccount(node.PeerID().String(), 50.0)

	// Register libp2p stream handler for direct messages
	d.registerDMHandler()

	// Watch for peer connect/disconnect to push topology updates
	d.watchPeerEvents()

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
			AgentName:  "LetChat Node",
			Visibility: config.DefaultVisibility,
			Domains:    []string{},
			Capabilities: []string{},
		}
	}
	var p config.Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return &config.Profile{
			AgentName:  "LetChat Node",
			Visibility: config.DefaultVisibility,
			Domains:    []string{},
			Capabilities: []string{},
		}
	}
	return &p
}
