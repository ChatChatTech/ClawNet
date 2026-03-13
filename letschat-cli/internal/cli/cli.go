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

// 🦞 ClawNet Lobster Theme — ANSI color palette
const (
	cBorder    = "\033[38;2;230;57;70m"  // Lobster Red #E63946 — borders
	cTitle     = "\033[1;38;2;241;250;238m" // Sea Foam bold — title text
	cSelf      = "\033[1;38;2;247;127;0m"  // Coral Orange bold #F77F00 — ★ self
	cPeer      = "\033[1;38;2;230;57;70m"  // Lobster Red bold #E63946 — @ peers
	cCoast     = "\033[38;2;69;123;157m"   // Tidal Blue #457B9D — coastline
	cLand      = "\033[38;2;29;53;87m"     // Deep Ocean #1D3557 — land
	cOcean     = "\033[2;38;2;29;53;87m"   // Deep Ocean dim — ocean dots
	cEdge      = "\033[2;38;2;230;57;70m"  // Lobster Red dim — globe edge ()
	cSelfInfo  = "\033[38;2;241;250;238m"  // Sea Foam #F1FAEE — self panel
	cPeerInfo  = "\033[38;2;247;127;0m"    // Coral Orange #F77F00 — peer cards
	cHelp      = "\033[2;37m"              // dim white — help line
	cReset     = "\033[0m"
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
	case "version":
		fmt.Printf("clawnet v%s\n", daemon.Version)
		return nil
	case "help", "--help", "-h":
		return printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		return printUsage()
	}
}

func printUsage() error {
	fmt.Println(`clawnet — decentralized agent communication network

Usage:
  clawnet init              Generate identity key and default config
  clawnet start             Start the daemon (foreground)
  clawnet stop              Stop a running daemon
  clawnet status            Show network status
  clawnet peers             List connected peers
  clawnet topo              Show rotating globe topology (full-screen)
  clawnet version           Show version

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

	resp, err := http.Get(base + "/api/peers/geo")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	resp.Body.Close()

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fmt.Errorf("not a terminal, cannot enter full-screen mode")
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("cannot enter raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	fmt.Print("\033[?1049h")
	fmt.Print("\033[?25l")
	defer fmt.Print("\033[?25l\033[?1049l")

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

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	angle := 0.0
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	needClear := true

	var netStats networkStats
	var lastStatsFetch time.Time
	var headerCache string
	var lastTermW int

	for {
		select {
		case <-quit:
			return nil
		case <-sigCh:
			needClear = true
			headerCache = ""
		case <-ticker.C:
			peers := fetchGeoPeers(base)
			w, h, err := term.GetSize(fd)
			if err != nil {
				w, h = 80, 24
			}
			statsChanged := false
			if time.Since(lastStatsFetch) > 3*time.Second {
				netStats = fetchNetworkStats(base)
				lastStatsFetch = time.Now()
				statsChanged = true
			}
			if needClear {
				fmt.Print("\033[2J")
				needClear = false
			}
			if statsChanged || headerCache == "" || w != lastTermW {
				headerCache = renderHeader(w, netStats)
				lastTermW = w
			}
			frame := renderTopoFrame(peers, w, h, angle, headerCache, netStats)
			fmt.Print("\033[H")
			fmt.Print(frame)
			angle += 0.015
		}
	}
}

// ── Data types ──

type peerGeoData struct {
	PeerID         string   `json:"peer_id"`
	ShortID        string   `json:"short_id"`
	Location       string   `json:"location"`
	Geo            *geoInfo `json:"geo,omitempty"`
	IsSelf         bool     `json:"is_self"`
	LatencyMs      int64    `json:"latency_ms"`
	ConnectedSince int64    `json:"connected_since"`
}

type geoInfo struct {
	Country   string  `json:"country"`
	Region    string  `json:"region"`
	City      string  `json:"city"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timezone  string  `json:"timezone"`
}

type networkStats struct {
	Peers     int      `json:"peers"`
	Version   string   `json:"version"`
	Topics    []string `json:"topics"`
	Balance   float64  `json:"-"`
	Frozen    float64  `json:"-"`
	Location  string   `json:"location"`
	StartedAt int64    `json:"started_at"`
	PeerID    string   `json:"peer_id"`
}

type creditInfo struct {
	Balance     float64 `json:"balance"`
	Frozen      float64 `json:"frozen"`
	TotalEarned float64 `json:"total_earned"`
	TotalSpent  float64 `json:"total_spent"`
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

func fetchNetworkStats(base string) networkStats {
	var raw map[string]any
	var stats networkStats
	if resp, err := http.Get(base + "/api/status"); err == nil {
		json.NewDecoder(resp.Body).Decode(&raw)
		resp.Body.Close()
		if v, ok := raw["peers"].(float64); ok {
			stats.Peers = int(v)
		}
		if v, ok := raw["version"].(string); ok {
			stats.Version = v
		}
		if v, ok := raw["location"].(string); ok {
			stats.Location = v
		}
		if v, ok := raw["peer_id"].(string); ok {
			stats.PeerID = v
		}
		if v, ok := raw["started_at"].(float64); ok {
			stats.StartedAt = int64(v)
		}
		if arr, ok := raw["topics"].([]any); ok {
			for _, t := range arr {
				if s, ok := t.(string); ok {
					stats.Topics = append(stats.Topics, s)
				}
			}
		}
	}
	if resp, err := http.Get(base + "/api/credits/balance"); err == nil {
		var ci creditInfo
		json.NewDecoder(resp.Body).Decode(&ci)
		resp.Body.Close()
		stats.Balance = ci.Balance
		stats.Frozen = ci.Frozen
	}
	return stats
}

// lookupWorldMap samples the 180x90 equirectangular bitmap.
func lookupWorldMap(latDeg, lonDeg float64) byte {
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

func padLine(sb *strings.Builder, visibleLen, termW int) {
	if visibleLen < termW {
		sb.WriteString(strings.Repeat(" ", termW-visibleLen))
	}
	sb.WriteString("\033[K\r\n")
}

func formatDuration(seconds int64) string {
	if seconds <= 0 {
		return "-"
	}
	d := seconds / 86400
	h := (seconds % 86400) / 3600
	m := (seconds % 3600) / 60
	if d > 0 {
		return fmt.Sprintf("%dd%dh", d, h)
	}
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func truncStr(s string, maxW int) string {
	r := []rune(s)
	if len(r) <= maxW {
		return s
	}
	if maxW <= 1 {
		return string(r[:maxW])
	}
	return string(r[:maxW-1]) + "…"
}

// renderHeader builds the static top 2 lines (title + separator).
func renderHeader(termW int, stats networkStats) string {
	innerW := termW - 2
	if innerW < 10 {
		innerW = 10
	}
	var sb strings.Builder

	// === ROW 1: TOP BORDER + TITLE ===
	titleText := " ClawNet Agent Network "
	statsText := fmt.Sprintf("Nodes:%d  Credits:%.0f  Topics:%d  v%s",
		stats.Peers+1, stats.Balance, len(stats.Topics), daemon.Version)
	// Layout: ┌──[title]──────────┐  with stats embedded in title area
	// Combine into one display string
	headerDisplay := titleText + "  " + statsText + " "
	headerLen := len([]rune(headerDisplay))
	fillTotal := innerW - headerLen
	if fillTotal < 2 {
		fillTotal = 2
	}
	dashL := fillTotal / 2
	dashR := fillTotal - dashL

	sb.WriteString(cBorder + "┌")
	sb.WriteString(strings.Repeat("─", dashL))
	sb.WriteString(cTitle)
	sb.WriteString(headerDisplay)
	sb.WriteString(cReset + cBorder)
	sb.WriteString(strings.Repeat("─", dashR))
	sb.WriteString("┐" + cReset)
	sb.WriteString("\033[K\r\n")

	return sb.String()
}

// renderTopoFrame builds one complete frame.
func renderTopoFrame(peers []peerGeoData, termW, termH int, rotation float64, header string, stats networkStats) string {
	innerW := termW - 2
	if innerW < 10 {
		innerW = 10
	}

	// Layout: 1 header + globeH + 1 sep + bottomH + 1 help + 1 bottom border
	// Bottom: self info on left, peer cards on right
	bottomH := 8
	if termH < 30 {
		bottomH = 6
	}
	if termH < 20 {
		bottomH = 4
	}
	globeH := termH - 1 - 1 - bottomH - 1 - 1 - 1 // header, sep_above, bottom, sep_below, help, frame
	if globeH < 5 {
		globeH = 5
	}

	// Strictly circular globe: terminal chars are ~2:1 (height:width)
	// For a circle: gW (cols) = gH (rows) * 2
	gH := globeH
	gW := gH * 2
	if gW > innerW {
		gW = innerW
		gH = gW / 2
		if gH < 5 {
			gH = 5
		}
	}
	// Center the globe horizontally
	globePadL := (innerW - gW) / 2
	globePadR := innerW - gW - globePadL

	rX := float64(gW) / 2.0 * 0.95
	rY := float64(gH) / 2.0 * 0.95
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
				continue
			}
			nz := math.Sqrt(1.0 - r2)
			rx := nx*math.Cos(rotation) + nz*math.Sin(rotation)
			rz := -nx*math.Sin(rotation) + nz*math.Cos(rotation)
			lat := math.Asin(-ny) * 180.0 / math.Pi
			lon := math.Atan2(rx, rz) * 180.0 / math.Pi
			terrain := lookupWorldMap(lat, lon)

			var ch rune
			switch terrain {
			case 2: // coastline
				ch = '#'
			case 1: // land
				ch = '.'
			default: // ocean
				if r2 > 0.92 {
					if sx > int(cX) {
						ch = ')'
					} else {
						ch = '('
					}
				} else {
					ch = '·'
				}
			}
			globe[sy][sx] = ch
		}
	}

	// ── Project all peers onto globe with jitter to avoid overlap ──
	type peerInfo struct {
		shortID        string
		peerID         string
		location       string
		country        string
		region         string
		city           string
		lat, lon       float64
		isSelf         bool
		visible        bool
		latencyMs      int64
		connectedSince int64
	}
	pInfos := make([]peerInfo, len(peers))

	// First pass: compute raw screen positions for visible peers
	type markerPos struct {
		sx, sy int
		idx    int
		isSelf bool
	}
	var markers []markerPos

	for i, p := range peers {
		pi := peerInfo{
			shortID:        p.ShortID,
			peerID:         p.PeerID,
			location:       p.Location,
			isSelf:         p.IsSelf,
			latencyMs:      p.LatencyMs,
			connectedSince: p.ConnectedSince,
		}
		if p.Geo != nil {
			pi.country = p.Geo.Country
			pi.region = p.Geo.Region
			pi.city = p.Geo.City
			pi.lat = p.Geo.Latitude
			pi.lon = p.Geo.Longitude

			latR := p.Geo.Latitude * math.Pi / 180.0
			lonR := p.Geo.Longitude * math.Pi / 180.0
			px := math.Cos(latR) * math.Sin(lonR)
			py := math.Sin(latR)
			pz := math.Cos(latR) * math.Cos(lonR)
			rx := px*math.Cos(rotation) - pz*math.Sin(rotation)
			rz := px*math.Sin(rotation) + pz*math.Cos(rotation)
			if rz > 0.05 {
				pi.visible = true
				sx := int(cX + rx*rX)
				sy := int(cY - py*rY)
				if sx >= 0 && sx < gW && sy >= 0 && sy < gH {
					markers = append(markers, markerPos{sx: sx, sy: sy, idx: i, isSelf: p.IsSelf})
				}
			}
		}
		pInfos[i] = pi
	}

	// Second pass: jitter overlapping markers so each gets a unique cell
	// Use a spiral pattern around the original position
	occupied := make(map[[2]int]bool)
	spiralDx := []int{0, 1, 0, -1, 1, -1, 1, -1, 2, -2, 0, 0, 2, -2, 2, -2, 1, -1, 1, -1, 3, -3, 0, 0, 2, -2, 3, -3}
	spiralDy := []int{0, 0, -1, 0, -1, -1, 1, 1, 0, 0, -2, 2, -1, -1, 1, 1, -2, -2, 2, 2, 0, 0, -3, 3, -2, -2, -1, -1}

	for mi := range markers {
		m := &markers[mi]
		placed := false
		for si := 0; si < len(spiralDx); si++ {
			nx := m.sx + spiralDx[si]
			ny := m.sy + spiralDy[si]
			key := [2]int{nx, ny}
			if nx >= 0 && nx < gW && ny >= 0 && ny < gH && !occupied[key] {
				// Check it's within the globe circle
				fnx := (float64(nx) - cX) / rX
				fny := (float64(ny) - cY) / rY
				if fnx*fnx+fny*fny <= 1.0 {
					m.sx = nx
					m.sy = ny
					occupied[key] = true
					placed = true
					break
				}
			}
		}
		if !placed {
			// Last resort: place at original even if overlapping
			occupied[[2]int{m.sx, m.sy}] = true
		}
	}

	// Place markers on globe: ★ for self, @ for peers
	for _, m := range markers {
		if m.isSelf {
			globe[m.sy][m.sx] = '★'
		} else {
			globe[m.sy][m.sx] = '@'
		}
	}

	// ── Build frame ──
	var sb strings.Builder
	sb.Grow(termW * termH * 4)

	// Header (cached)
	sb.WriteString(header)

	// Globe rows
	for row := 0; row < gH; row++ {
		sb.WriteString(cBorder + "│" + cReset)
		sb.WriteString(strings.Repeat(" ", globePadL))
		for _, ch := range globe[row] {
			switch ch {
			case '★':
				sb.WriteString(cSelf + "★" + cReset)
			case '@':
				sb.WriteString(cPeer + "@" + cReset)
			case '#':
				sb.WriteString(cCoast + "#" + cReset)
			case '.':
				sb.WriteString(cLand + "." + cReset)
			case '(', ')':
				sb.WriteString(cEdge)
				sb.WriteRune(ch)
				sb.WriteString(cReset)
			case '·':
				sb.WriteString(cOcean + "·" + cReset)
			default:
				sb.WriteByte(' ')
			}
		}
		sb.WriteString(strings.Repeat(" ", globePadR))
		sb.WriteString(cBorder + "│" + cReset + "\033[K\r\n")
	}

	// ── Separator above bottom panel (with ┬ for vertical divider) ──
	// ── Bottom panel layout ──
	selfW := innerW * 2 / 5
	if selfW < 28 {
		selfW = 28
	}
	if selfW > 50 {
		selfW = 50
	}
	peerW := innerW - selfW - 1 // 1 for vertical separator
	if peerW < 20 {
		peerW = 20
		selfW = innerW - peerW - 1
	}

	// Emit separator above bottom panel: ├──...──┬──...──┤
	sb.WriteString(cBorder + "├")
	sb.WriteString(strings.Repeat("─", selfW))
	sb.WriteString("┬")
	sb.WriteString(strings.Repeat("─", peerW))
	sb.WriteString("┤" + cReset + "\033[K\r\n")

	// Build self info lines
	selfLines := make([]string, bottomH)
	var selfPeer *peerInfo
	for i := range pInfos {
		if pInfos[i].isSelf {
			selfPeer = &pInfos[i]
			break
		}
	}
	// Fallback: first entry is self
	if selfPeer == nil && len(pInfos) > 0 {
		selfPeer = &pInfos[0]
	}

	now := time.Now().Unix()
	if selfPeer != nil {
		insW := selfW - 2 // padding
		lines := []string{
			fmt.Sprintf("★ %s", truncStr(selfPeer.shortID, insW-2)),
		}
		if stats.PeerID != "" {
			lines = append(lines, fmt.Sprintf("  ID: %s", truncStr(stats.PeerID, insW-6)))
		}
		loc := selfPeer.location
		if loc == "" || loc == "Unknown" {
			loc = selfPeer.country
		}
		if loc != "" && loc != "Unknown" {
			lines = append(lines, fmt.Sprintf("  Loc: %s", truncStr(loc, insW-7)))
		}
		if selfPeer.lat != 0 || selfPeer.lon != 0 {
			lines = append(lines, fmt.Sprintf("  Coord: %.2f, %.2f", selfPeer.lat, selfPeer.lon))
		}
		lines = append(lines, fmt.Sprintf("  Credits: %.1f (frozen: %.1f)", stats.Balance, stats.Frozen))
		if stats.StartedAt > 0 {
			upSec := now - stats.StartedAt
			lines = append(lines, fmt.Sprintf("  Uptime: %s", formatDuration(upSec)))
		}
		lines = append(lines, fmt.Sprintf("  Topics: %d  Peers: %d", len(stats.Topics), stats.Peers))
		lines = append(lines, fmt.Sprintf("  Version: %s", daemon.Version))

		for i := 0; i < bottomH; i++ {
			if i < len(lines) {
				txt := lines[i]
				if len([]rune(txt)) > selfW {
					txt = string([]rune(txt)[:selfW])
				}
				pad := selfW - len([]rune(txt))
				selfLines[i] = txt + strings.Repeat(" ", pad)
			} else {
				selfLines[i] = strings.Repeat(" ", selfW)
			}
		}
	} else {
		for i := range selfLines {
			selfLines[i] = strings.Repeat(" ", selfW)
		}
	}

	// Build peer card content (right side)
	// Cards in a 4-wide x 2-tall grid (or fewer if less space)
	cardW := peerW / 4
	if cardW < 18 {
		cardW = peerW / 3
	}
	if cardW < 18 {
		cardW = peerW / 2
	}
	if cardW < 18 {
		cardW = peerW
	}
	cols := peerW / cardW
	if cols < 1 {
		cols = 1
	}
	cardRows := bottomH / 4
	if cardRows < 1 {
		cardRows = 1
	}
	// Collect peer entries (non-self)
	var peerEntries []peerInfo
	for _, pi := range pInfos {
		if !pi.isSelf {
			peerEntries = append(peerEntries, pi)
		}
	}

	// Each card is 4 lines: top border, id+loc, latency+uptime, bottom border
	// If bottomH < 4, compress
	peerLines := make([]string, bottomH)
	for i := range peerLines {
		peerLines[i] = strings.Repeat(" ", peerW)
	}

	if len(peerEntries) > 0 {
		maxCards := cols * cardRows
		if maxCards > len(peerEntries) {
			maxCards = len(peerEntries)
		}

		type cardLines struct {
			lines [4]string
		}
		allCards := make([]cardLines, maxCards)
		for ci := 0; ci < maxCards; ci++ {
			p := peerEntries[ci]
			insW := cardW - 2
			if insW < 1 {
				insW = 1
			}

			loc := ""
			if p.city != "" {
				loc = p.city
			} else if p.region != "" {
				loc = p.region
			} else if p.country != "" {
				loc = p.country
			}
			if loc == "" {
				loc = "?"
			}

			latStr := "-"
			if p.latencyMs > 0 {
				latStr = fmt.Sprintf("%dms", p.latencyMs)
			}
			upStr := "-"
			if p.connectedSince > 0 {
				upStr = formatDuration(now - p.connectedSince)
			}

			allCards[ci].lines[0] = "╭" + strings.Repeat("─", insW) + "╮"

			idLine := " @" + truncStr(p.shortID, insW-2)
			pad := insW - len([]rune(idLine))
			if pad < 0 {
				pad = 0
			}
			allCards[ci].lines[1] = "│" + idLine + strings.Repeat(" ", pad) + "│"

			infoLine := " " + truncStr(loc, insW-1)
			pad = insW - len([]rune(infoLine))
			if pad < 0 {
				pad = 0
			}
			allCards[ci].lines[2] = "│" + infoLine + strings.Repeat(" ", pad) + "│"

			statLine := fmt.Sprintf(" %s %s", latStr, upStr)
			statLine = truncStr(statLine, insW)
			pad = insW - len([]rune(statLine))
			if pad < 0 {
				pad = 0
			}
			allCards[ci].lines[3] = "╰" + statLine + strings.Repeat("─", pad) + "╯"
		}

		// Lay out cards into peerLines
		for row := 0; row < cardRows; row++ {
			for lineOff := 0; lineOff < 4; lineOff++ {
				lineIdx := row*4 + lineOff
				if lineIdx >= bottomH {
					break
				}
				var rowSB strings.Builder
				vis := 0
				for col := 0; col < cols; col++ {
					ci := row*cols + col
					if ci < maxCards {
						rowSB.WriteString(allCards[ci].lines[lineOff])
						vis += cardW
					}
				}
				pad := peerW - vis
				if pad > 0 {
					rowSB.WriteString(strings.Repeat(" ", pad))
				}
				peerLines[lineIdx] = rowSB.String()
			}
		}
	}

	// Emit bottom rows
	for i := 0; i < bottomH; i++ {
		sb.WriteString(cBorder + "│" + cReset)
		sb.WriteString(cSelfInfo)
		sb.WriteString(selfLines[i])
		sb.WriteString(cReset)
		sb.WriteString(cBorder + "│" + cReset)
		sb.WriteString(cPeerInfo)
		// Make sure peerLines[i] is exactly peerW visible chars
		pl := peerLines[i]
		if len([]rune(pl)) > peerW {
			pl = string([]rune(pl)[:peerW])
		}
		sb.WriteString(pl)
		sb.WriteString(cReset)
		sb.WriteString(cBorder + "│" + cReset + "\033[K\r\n")
	}

	// ── Separator below bottom panel: ├──...──┴──...──┤ ──
	sb.WriteString(cBorder + "├")
	sb.WriteString(strings.Repeat("─", selfW))
	sb.WriteString("┴")
	sb.WriteString(strings.Repeat("─", peerW))
	sb.WriteString("┤" + cReset + "\033[K\r\n")

	// ── Help line ──
	help := " q:Quit  ★:You  @:Peer"
	helpLen := len([]rune(help))
	helpPad := innerW - helpLen
	if helpPad < 0 {
		helpPad = 0
	}
	sb.WriteString(cBorder + "│" + cHelp)
	sb.WriteString(help)
	sb.WriteString(strings.Repeat(" ", helpPad))
	sb.WriteString(cBorder + "│" + cReset + "\033[K\r\n")

	// ── Bottom frame ──
	sb.WriteString(cBorder + "└")
	sb.WriteString(strings.Repeat("─", innerW))
	sb.WriteString("┘" + cReset + "\033[K")

	return sb.String()
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
