package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ChatChatTech/letschat/letschat-cli/internal/config"
	"github.com/ChatChatTech/letschat/letschat-cli/internal/daemon"
	"github.com/ChatChatTech/letschat/letschat-cli/internal/identity"
)

func Execute() error {
	if len(os.Args) < 2 {
		return printUsage()
	}

	switch os.Args[1] {
	case "init":
		return cmdInit()
	case "start":
		return cmdStart()
	case "stop":
		return cmdStop()
	case "status":
		return cmdStatus()
	case "peers":
		return cmdPeers()
	case "version":
		fmt.Printf("letchat v%s\n", daemon.Version)
		return nil
	case "help", "--help", "-h":
		return printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		return printUsage()
	}
}

func printUsage() error {
	fmt.Println(`letchat — decentralized agent communication network

Usage:
  letchat init              Generate identity key and default config
  letchat start             Start the daemon (foreground)
  letchat stop              Stop a running daemon
  letchat status            Show network status
  letchat peers             List connected peers
  letchat version           Show version

API runs on http://localhost:3847 when daemon is active.`)
	return nil
}

func cmdInit() error {
	dataDir := config.DataDir()

	// Create directory structure
	dirs := []string{
		dataDir,
		filepath.Join(dataDir, "wireguard", "peers"),
		filepath.Join(dataDir, "data", "knowledge"),
		filepath.Join(dataDir, "data", "tasks"),
		filepath.Join(dataDir, "data", "predictions"),
		filepath.Join(dataDir, "data", "topics"),
		filepath.Join(dataDir, "data", "credits"),
		filepath.Join(dataDir, "data", "reputation"),
		filepath.Join(dataDir, "logs"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0700); err != nil {
			return fmt.Errorf("create dir %s: %w", d, err)
		}
	}

	// Generate or load identity key
	priv, err := identity.LoadOrGenerate(dataDir)
	if err != nil {
		return fmt.Errorf("identity: %w", err)
	}
	peerID, err := identity.PeerIDFromKey(priv)
	if err != nil {
		return fmt.Errorf("peer ID: %w", err)
	}

	// Write default config if doesn't exist
	cfgPath := config.ConfigPath()
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		cfg := config.DefaultConfig()
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		fmt.Printf("Created config: %s\n", cfgPath)
	} else {
		fmt.Printf("Config exists: %s\n", cfgPath)
	}

	fmt.Printf("Data directory: %s\n", dataDir)
	fmt.Printf("Peer ID: %s\n", peerID.String())
	fmt.Println("Initialization complete.")
	return nil
}

func cmdStart() error {
	return daemon.Start(true)
}

func cmdStop() error {
	dataDir := config.DataDir()
	pidPath := filepath.Join(dataDir, "daemon.pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return fmt.Errorf("no running daemon found (no PID file)")
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("invalid PID file: %w", err)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process %d not found: %w", pid, err)
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("failed to stop daemon (pid %d): %w", pid, err)
	}
	fmt.Printf("Sent stop signal to daemon (pid %d)\n", pid)
	return nil
}

func cmdStatus() error {
	return apiGet("/api/status")
}

func cmdPeers() error {
	return apiGet("/api/peers")
}

func apiGet(path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("http://127.0.0.1:%d%s", cfg.WebUIPort, path)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("cannot connect to daemon (is it running?): %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	// Pretty print JSON
	var out any
	if err := json.Unmarshal(body, &out); err != nil {
		fmt.Println(string(body))
		return nil
	}
	pretty, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(pretty))
	return nil
}
