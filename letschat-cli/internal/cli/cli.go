package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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

	// Verify daemon is running
	resp, err := http.Get(base + "/api/peers/geo")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	resp.Body.Close()

	// Enter raw terminal mode
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fmt.Errorf("not a terminal, cannot enter full-screen mode")
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("cannot enter raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	fmt.Print("\033[?25l")                      // hide cursor
	defer fmt.Print("\033[?25h\033[2J\033[H")   // show cursor + clear on exit

	// Keypress listener
	quit := make(chan struct{})
	go func() {
		buf := make([]byte, 1)
		for {
			os.Stdin.Read(buf)
			if buf[0] == 'q' || buf[0] == 'Q' || buf[0] == 3 {
				close(quit)
				return
			}
		}
	}()

	// SIGWINCH handler for terminal resize
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	angle := 0.0
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	// Force full clear on first frame
	needClear := true

	for {
		select {
		case <-quit:
			return nil
		case <-sigCh:
			needClear = true // terminal resized, clear artifacts
		case <-ticker.C:
			peers := fetchGeoPeers(base)
			w, h, err := term.GetSize(fd)
			if err != nil {
				w, h = 80, 24
			}
			if needClear {
				fmt.Print("\033[2J") // clear entire screen
				needClear = false
			}
			frame := renderGlobeFrame(peers, w, h, angle)
			fmt.Print("\033[H") // move to top-left
			fmt.Print(frame)
			angle += 0.05
		}
	}
}

// peerGeoData holds geo data for a peer from the API.
type peerGeoData struct {
	PeerID   string   `json:"peer_id"`
	ShortID  string   `json:"short_id"`
	Location string   `json:"location"`
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

// ────────────────────────────────────────────────────────────
// Layout presets (like nload/htop: pick the best fit)
// ────────────────────────────────────────────────────────────

type layoutPreset struct {
	name    string
	minW    int // minimum terminal width
	minH    int // minimum terminal height
	globeW  int // globe render width in chars
	globeH  int // globe render height in chars
	panelW  int // side panel width
}

var presets = []layoutPreset{
	{"xlarge", 160, 48, 80, 36, 30},
	{"large",  120, 36, 60, 26, 24},
	{"medium",  80, 24, 36, 16, 20},
	{"small",   60, 16, 24, 10, 16},
	{"tiny",    40, 10, 16,  7, 10},
}

func pickPreset(termW, termH int) layoutPreset {
	for _, p := range presets {
		if termW >= p.minW && termH >= p.minH {
			return p
		}
	}
	return presets[len(presets)-1] // tiny fallback
}

// lookupWorldMap samples the 180x90 equirectangular bitmap.
// Returns: 0=ocean, 1=land, 2=coastline
func lookupWorldMap(latDeg, lonDeg float64) byte {
	// Clamp
	if latDeg > 89 {
		latDeg = 89
	}
	if latDeg < -89 {
		latDeg = -89
	}
	for lonDeg > 180 {
		lonDeg -= 360
	}
	for lonDeg < -180 {
		lonDeg += 360
	}
	row := int((90.0 - latDeg) / 2.0)
	col := int((lonDeg + 180.0) / 2.0)
	if row < 0 {
		row = 0
	}
	if row >= worldMapH {
		row = worldMapH - 1
	}
	if col < 0 {
		col = 0
	}
	if col >= worldMapW {
		col = worldMapW - 1
	}
	return worldMap[row][col]
}

// renderGlobeFrame renders one frame of the spinning globe with side panels.
func renderGlobeFrame(peers []peerGeoData, termW, termH int, rotation float64) string {
	preset := pickPreset(termW, termH)
	gW := preset.globeW
	gH := preset.globeH
	pW := preset.panelW

	// Actual layout widths — center the globe
	totalNeeded := gW + pW*2
	if totalNeeded > termW {
		// Shrink panels to fit
		pW = (termW - gW) / 2
		if pW < 2 {
			pW = 2
			gW = termW - pW*2
			if gW < 10 {
				gW = 10
			}
		}
	}
	leftPW := (termW - gW) / 2
	rightPW := termW - gW - leftPW
	if leftPW < 2 {
		leftPW = 2
	}
	if rightPW < 2 {
		rightPW = 2
	}

	// Cap globe height to available content rows
	contentH := termH - 2 // status + help line
	if gH > contentH {
		gH = contentH
	}
	if gH < 5 {
		gH = 5
	}

	// ── Render globe ──
	// Orthographic projection with the embedded world map
	rX := float64(gW) / 2.0 * 0.95   // radius in cols
	rY := float64(gH) / 2.0 * 0.95   // radius in rows
	cX := float64(gW) / 2.0
	cY := float64(gH) / 2.0

	globe := make([][]rune, gH)
	for y := range globe {
		globe[y] = make([]rune, gW)
		for x := range globe[y] {
			globe[y][x] = ' '
		}
	}

	for sy := 0; sy < gH; sy++ {
		for sx := 0; sx < gW; sx++ {
			nx := (float64(sx) - cX) / rX
			ny := (float64(sy) - cY) / rY
			r2 := nx*nx + ny*ny
			if r2 > 1.0 {
				continue // outside sphere
			}
			nz := math.Sqrt(1.0 - r2)

			// Rotate around Y axis
			rx := nx*math.Cos(rotation) + nz*math.Sin(rotation)
			rz := -nx*math.Sin(rotation) + nz*math.Cos(rotation)

			// Compute lat/lon on sphere
			lat := math.Asin(-ny) * 180.0 / math.Pi
			lon := math.Atan2(rx, rz) * 180.0 / math.Pi

			terrain := lookupWorldMap(lat, lon)

			// Lighting: simple hemisphere shading
			light := (rz + 1.0) / 2.0 // 0 = dark edge, 1 = bright center

			var ch rune
			switch terrain {
			case 2: // coastline / border
				if light > 0.3 {
					ch = '#'
				} else {
					ch = ':'
				}
			case 1: // land interior
				if light > 0.5 {
					ch = '.'
				} else {
					ch = ','
				}
			default: // ocean
				if light > 0.6 {
					ch = '~'
				} else if light > 0.2 {
					ch = '·'
				} else {
					ch = ' '
				}
			}

			// Globe edge outline
			if r2 > 0.92 {
				ch = '('
				if sx > int(cX) {
					ch = ')'
				}
			}

			globe[sy][sx] = ch
		}
	}

	// ── Project peers onto globe ──
	type peerScreen struct {
		idx     int
		label   string
		visible bool
		gx, gy  int
	}
	pInfos := make([]peerScreen, len(peers))
	for i, p := range peers {
		label := p.Location
		if label == "" || label == "Unknown" {
			label = p.ShortID
		}
		maxLabel := leftPW - 5
		if maxLabel < 4 {
			maxLabel = 4
		}
		if len(label) > maxLabel {
			label = label[:maxLabel]
		}
		pi := peerScreen{idx: i, label: label}

		if p.Geo != nil {
			lat := p.Geo.Latitude * math.Pi / 180.0
			lon := p.Geo.Longitude * math.Pi / 180.0
			px := math.Cos(lat) * math.Sin(lon)
			py := math.Sin(lat)
			pz := math.Cos(lat) * math.Cos(lon)
			rx := px*math.Cos(rotation) - pz*math.Sin(rotation)
			rz := px*math.Sin(rotation) + pz*math.Cos(rotation)

			if rz > 0.05 {
				pi.visible = true
				pi.gx = int(cX + rx*rX)
				pi.gy = int(cY - py*rY)
				if pi.gx < 0 {
					pi.gx = 0
				}
				if pi.gx >= gW {
					pi.gx = gW - 1
				}
				if pi.gy < 0 {
					pi.gy = 0
				}
				if pi.gy >= gH {
					pi.gy = gH - 1
				}
				marker := 'o'
				if i == 0 {
					marker = '*'
				}
				globe[pi.gy][pi.gx] = marker
			}
		}
		pInfos[i] = pi
	}

	// ── Assign peers to side panels ──
	type slot struct {
		row  int
		peer peerScreen
	}
	var leftSlots, rightSlots []slot
	for i, pi := range pInfos {
		row := gH / 2
		if pi.visible {
			row = pi.gy
		}
		if i%2 == 0 {
			leftSlots = append(leftSlots, slot{row: row, peer: pi})
		} else {
			rightSlots = append(rightSlots, slot{row: row, peer: pi})
		}
	}

	// Spread slots evenly within globe height — no overlapping rows
	spreadSlots := func(slots []slot, height int) []slot {
		n := len(slots)
		if n == 0 {
			return slots
		}
		step := float64(height) / float64(n+1)
		for i := range slots {
			slots[i].row = int(step * float64(i+1))
			if slots[i].row >= height {
				slots[i].row = height - 1
			}
		}
		return slots
	}
	leftSlots = spreadSlots(leftSlots, gH)
	rightSlots = spreadSlots(rightSlots, gH)

	// Build row lookup maps
	leftByRow := map[int]slot{}
	for _, s := range leftSlots {
		leftByRow[s.row] = s
	}
	rightByRow := map[int]slot{}
	for _, s := range rightSlots {
		rightByRow[s.row] = s
	}

	// ── Build output ──
	var sb strings.Builder

	for row := 0; row < gH; row++ {
		// Left panel
		if ls, ok := leftByRow[row]; ok {
			marker := "●"
			if ls.peer.idx == 0 {
				marker = "★"
			}
			vis := " "
			if !ls.peer.visible {
				vis = "?"
			}
			text := fmt.Sprintf("%s%s %s", vis, marker, ls.peer.label)
			if len(text) > leftPW-2 {
				text = text[:leftPW-2]
			}
			pad := leftPW - 2 - len([]rune(text))
			if pad < 0 {
				pad = 0
			}
			sb.WriteString("\033[33m")
			sb.WriteString(text)
			sb.WriteString(strings.Repeat("─", pad))
			sb.WriteString("→")
			sb.WriteString("\033[0m")
			sb.WriteByte(' ')
		} else {
			sb.WriteString(strings.Repeat(" ", leftPW))
		}

		// Globe
		for _, ch := range globe[row] {
			switch ch {
			case '#', ':': // coastline
				sb.WriteString("\033[1;37m")  // bright white
				sb.WriteRune(ch)
				sb.WriteString("\033[0m")
			case '.', ',': // land
				sb.WriteString("\033[32m")     // green
				sb.WriteRune(ch)
				sb.WriteString("\033[0m")
			case '*': // self
				sb.WriteString("\033[1;33m")   // bright yellow
				sb.WriteRune(ch)
				sb.WriteString("\033[0m")
			case 'o': // peer
				sb.WriteString("\033[1;36m")   // bright cyan
				sb.WriteRune(ch)
				sb.WriteString("\033[0m")
			case '(', ')': // edge
				sb.WriteString("\033[2;37m")   // dim white
				sb.WriteRune(ch)
				sb.WriteString("\033[0m")
			case '~': // ocean bright
				sb.WriteString("\033[34m")     // blue
				sb.WriteRune(ch)
				sb.WriteString("\033[0m")
			case '·': // ocean dim
				sb.WriteString("\033[2;34m")   // dim blue
				sb.WriteRune(ch)
				sb.WriteString("\033[0m")
			default:
				sb.WriteByte(' ')
			}
		}

		// Right panel
		if rs, ok := rightByRow[row]; ok {
			marker := "●"
			if rs.peer.idx == 0 {
				marker = "★"
			}
			vis := " "
			if !rs.peer.visible {
				vis = "?"
			}
			text := fmt.Sprintf("%s %s%s", rs.peer.label, marker, vis)
			if len(text) > rightPW-2 {
				text = text[:rightPW-2]
			}
			pad := rightPW - 2 - len([]rune(text))
			if pad < 0 {
				pad = 0
			}
			sb.WriteByte(' ')
			sb.WriteString("\033[33m")
			sb.WriteString("←")
			sb.WriteString(strings.Repeat("─", pad))
			sb.WriteString(text)
			sb.WriteString("\033[0m")
		} else {
			sb.WriteString(strings.Repeat(" ", rightPW))
		}

		sb.WriteByte('\n')
	}

	// Pad empty rows to fill content area
	for row := gH; row < contentH; row++ {
		sb.WriteString(strings.Repeat(" ", termW))
		sb.WriteByte('\n')
	}

	// Status bar
	sb.WriteString("\033[7;36m") // reverse cyan
	presetTag := preset.name
	status := fmt.Sprintf(" LetChat Topo │ %d nodes │ %s │ v%s ", len(peers), presetTag, daemon.Version)
	if len(status) > termW {
		status = status[:termW]
	}
	sb.WriteString(status)
	sb.WriteString(strings.Repeat(" ", max(0, termW-len(status))))
	sb.WriteString("\033[0m\n")

	// Help line
	sb.WriteString("\033[2m")
	help := " q:quit │ ★ you │ ● peer │ #:coast │ .:land │ ~:ocean"
	if len(help) > termW {
		help = help[:termW]
	}
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
