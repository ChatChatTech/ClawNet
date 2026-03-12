package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ChatChatTech/letschat/letschat-cli/internal/config"
	"github.com/ChatChatTech/letschat/letschat-cli/internal/daemon"
	"github.com/ChatChatTech/letschat/letschat-cli/internal/identity"
	"golang.org/x/term"
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
	case "topo":
		return cmdTopo()
	case "geo-update":
		return cmdGeoUpdate()
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
  letchat topo              Show rotating globe topology (full-screen)
  letchat geo-update        Download city-level geo database (DB11)
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

func cmdTopo() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)

	// Fetch initial data to verify daemon is running
	resp, err := http.Get(base + "/api/peers/geo")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	resp.Body.Close()

	// Enter raw terminal mode (like htop)
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fmt.Errorf("not a terminal, cannot enter full-screen mode")
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("cannot enter raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	// Hide cursor
	fmt.Print("\033[?25l")
	defer fmt.Print("\033[?25h\033[2J\033[H") // show cursor, clear screen on exit

	// Channel to detect 'q' keypress
	quit := make(chan struct{})
	go func() {
		buf := make([]byte, 1)
		for {
			os.Stdin.Read(buf)
			if buf[0] == 'q' || buf[0] == 'Q' || buf[0] == 3 { // q or Ctrl-C
				close(quit)
				return
			}
		}
	}()

	angle := 0.0
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-quit:
			return nil
		case <-ticker.C:
			// Fetch geo peers
			peers := fetchGeoPeers(base)
			// Get terminal size
			w, h, err := term.GetSize(fd)
			if err != nil {
				w, h = 80, 24
			}
			// Render frame
			frame := renderGlobeFrame(peers, w, h, angle)
			// Output: move to top-left, print frame
			fmt.Print("\033[H")
			fmt.Print(frame)
			angle += 0.06 // ~2° per frame, full rotation in ~5 seconds at 150ms
		}
	}
}

// peerGeoData holds geo data for a peer from the API.
type peerGeoData struct {
	PeerID   string  `json:"peer_id"`
	ShortID  string  `json:"short_id"`
	Location string  `json:"location"`
	Geo      *geoInfo `json:"geo,omitempty"`
}

type geoInfo struct {
	Country   string  `json:"country"`
	Region    string  `json:"region"`
	City      string  `json:"city"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timezone  string  `json:"timezone"`
}

func fetchGeoPeers(base string) []peerGeoData {
	resp, err := http.Get(base + "/api/peers/geo")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var data []peerGeoData
	json.NewDecoder(resp.Body).Decode(&data)
	return data
}

// renderGlobeFrame renders an ASCII globe with peer markers.
func renderGlobeFrame(peers []peerGeoData, termW, termH int, rotation float64) string {
	// Globe dimensions - leave room for info panel
	globeW := termW - 2
	if globeW > 80 {
		globeW = 80
	}
	globeH := termH - 6
	if globeH < 10 {
		globeH = 10
	}
	if globeH > 36 {
		globeH = 36
	}

	// Radius
	rX := float64(globeW) / 2.0 * 0.9
	rY := float64(globeH) / 2.0 * 0.9
	cX := float64(globeW) / 2.0
	cY := float64(globeH) / 2.0

	// Build the screen buffer
	buf := make([][]rune, globeH)
	for y := range buf {
		buf[y] = make([]rune, globeW)
		for x := range buf[y] {
			buf[y][x] = ' '
		}
	}

	// Shade characters from dark to light
	shading := []rune(".:-=+*#%@")

	// Draw the globe with simple latitude/longitude lines
	for sy := 0; sy < globeH; sy++ {
		for sx := 0; sx < globeW; sx++ {
			// Normalize to [-1, 1]
			nx := (float64(sx) - cX) / rX
			ny := (float64(sy) - cY) / rY
			r2 := nx*nx + ny*ny
			if r2 > 1.0 {
				continue
			}
			// Z on sphere
			nz := math.Sqrt(1.0 - r2)

			// Rotate around Y axis
			rx := nx*math.Cos(rotation) + nz*math.Sin(rotation)
			rz := -nx*math.Sin(rotation) + nz*math.Cos(rotation)

			// Convert to lat/lon
			lat := math.Asin(ny) * 180.0 / math.Pi // -90 to 90 (inverted Y)
			lon := math.Atan2(rx, rz) * 180.0 / math.Pi

			// Simple shading based on Z depth (facing us = brighter)
			shade := int((rz + 1.0) / 2.0 * float64(len(shading)-1))
			if shade < 0 {
				shade = 0
			}
			if shade >= len(shading) {
				shade = len(shading) - 1
			}

			ch := shading[shade]

			// Draw latitude lines every 30° and longitude lines every 30°
			latMod := math.Mod(math.Abs(lat), 30.0)
			lonMod := math.Mod(math.Abs(lon), 30.0)
			if latMod < 2.5 || lonMod < 2.5 {
				// Grid line - slightly different char
				ch = '·'
				if latMod < 1.5 || lonMod < 1.5 {
					ch = '+'
				}
			}

			// Equator and prime meridian
			if math.Abs(lat) < 2.0 {
				ch = '─'
			}
			if math.Abs(lon) < 2.0 {
				ch = '│'
			}
			if math.Abs(lat) < 2.0 && math.Abs(lon) < 2.0 {
				ch = '┼'
			}

			buf[sy][sx] = ch
		}
	}

	// Plot peer markers on the globe
	for i, p := range peers {
		if p.Geo == nil {
			continue
		}
		lat := p.Geo.Latitude * math.Pi / 180.0
		lon := p.Geo.Longitude * math.Pi / 180.0

		// 3D coordinates from lat/lon
		px := math.Cos(lat) * math.Sin(lon)
		py := math.Sin(lat) // Note: screen Y is inverted
		pz := math.Cos(lat) * math.Cos(lon)

		// Rotate around Y axis (same as globe rotation)
		rx := px*math.Cos(rotation) - pz*math.Sin(rotation)
		rz := px*math.Sin(rotation) + pz*math.Cos(rotation)

		// Only show if on the visible hemisphere
		if rz < 0.05 {
			continue
		}

		// Project to screen
		sx := int(cX + rx*rX)
		sy := int(cY - py*rY) // invert Y

		if sx >= 0 && sx < globeW && sy >= 0 && sy < globeH {
			marker := '●'
			if i == 0 {
				marker = '★' // self
			}
			buf[sy][sx] = marker
		}
	}

	// Build output string
	var sb strings.Builder

	// Title bar
	title := " 🌐 LetChat Network Topology "
	pad := (termW - len(title)) / 2
	if pad < 0 {
		pad = 0
	}
	sb.WriteString("\033[1;36m") // bold cyan
	sb.WriteString(strings.Repeat(" ", pad))
	sb.WriteString(title)
	sb.WriteString(strings.Repeat(" ", termW-pad-len(title)))
	sb.WriteString("\033[0m\n")

	// Globe
	for _, row := range buf {
		line := string(row)
		if len(line) < termW {
			line += strings.Repeat(" ", termW-len(line))
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Info panel at the bottom
	sb.WriteString("\033[1;33m") // bold yellow
	info := fmt.Sprintf(" Nodes: %d", len(peers))
	// List peer labels
	labels := []string{}
	for i, p := range peers {
		marker := "●"
		if i == 0 {
			marker = "★"
		}
		loc := p.Location
		if loc == "" || loc == "Unknown" {
			loc = p.ShortID
		}
		labels = append(labels, fmt.Sprintf("%s %s", marker, loc))
	}
	info += "  │  " + strings.Join(labels, "  ")
	if len(info) > termW {
		info = info[:termW]
	}
	sb.WriteString(info)
	sb.WriteString(strings.Repeat(" ", max(0, termW-len(info))))
	sb.WriteString("\033[0m\n")

	// Help line
	sb.WriteString("\033[2m") // dim
	help := " Press 'q' to quit"
	sb.WriteString(help)
	sb.WriteString(strings.Repeat(" ", max(0, termW-len(help))))
	sb.WriteString("\033[0m")

	return sb.String()
}

func cmdGeoUpdate() error {
	dataDir := config.DataDir()
	targetDir := filepath.Join(dataDir, "data")
	os.MkdirAll(targetDir, 0700)

	targetPath := filepath.Join(targetDir, "IP2LOCATION-LITE-DB11.BIN")

	// Check for .env file with token
	token := os.Getenv("IP2LOCATION_TOKEN")
	if token == "" {
		// Try reading from .env in working directory
		envData, err := os.ReadFile(".env")
		if err == nil {
			for _, line := range strings.Split(string(envData), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "IP2LOCATION_TOKEN=") {
					token = strings.TrimPrefix(line, "IP2LOCATION_TOKEN=")
					break
				}
			}
		}
	}

	if token == "" {
		return fmt.Errorf("IP2LOCATION_TOKEN not set. Set it in environment or .env file")
	}

	url := fmt.Sprintf("https://www.ip2location.com/download/?token=%s&file=DB11LITEBINIPV6", token)

	fmt.Printf("Downloading city-level database (DB11) to %s ...\n", targetPath)
	fmt.Println("This may take a while (~100MB)...")

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	// The response is a ZIP file, save it first
	zipPath := targetPath + ".zip"
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("create zip: %w", err)
	}
	n, err := io.Copy(zipFile, resp.Body)
	zipFile.Close()
	if err != nil {
		os.Remove(zipPath)
		return fmt.Errorf("download error: %w", err)
	}
	fmt.Printf("Downloaded %d bytes\n", n)

	// We'll need to unzip - for now just tell user
	fmt.Printf("ZIP saved to: %s\n", zipPath)
	fmt.Printf("Please extract IP2LOCATION-LITE-DB11.BIN from the ZIP to:\n  %s\n", targetPath)
	fmt.Println("Then restart the daemon to use city-level geo resolution.")
	return nil
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
