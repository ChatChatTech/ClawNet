package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
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

// ── Topo slogans (picked randomly on each launch) ──
var topoSlogans = []string{
	"Enjoy Surfing the Decentralized Web",
	"Where Agents Meet, Ideas Spark",
	"Intelligence is Better When Shared",
	"Your Network, Your Rules",
	"Building Trust, One Node at a Time",
	"Decentralize Everything",
	"Think Together, Build Together",
	"The Hive Mind Awaits",
	"Connect. Collaborate. Create.",
	"Powered by Curiosity",
	"Agents Without Borders",
	"The Future is Distributed",
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

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	slogan := topoSlogans[rng.Intn(len(topoSlogans))]

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

	// Pre-render static header (only changes when stats refresh or resize)
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
			headerCache = "" // force header rebuild
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
			// Rebuild header only when stats change or terminal width changes
			if statsChanged || headerCache == "" || w != lastTermW {
				headerCache = renderHeader(w, slogan, netStats)
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

type networkStats struct {
	Peers       int      `json:"peers"`
	Version     string   `json:"version"`
	Topics      []string `json:"topics"`
	Balance     float64  `json:"-"`
	Frozen      float64  `json:"-"`
	TotalEarned float64  `json:"-"`
	TotalSpent  float64  `json:"-"`
	Location    string   `json:"location"`
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
	var stats networkStats
	if resp, err := http.Get(base + "/api/status"); err == nil {
		json.NewDecoder(resp.Body).Decode(&stats)
		resp.Body.Close()
	}
	if resp, err := http.Get(base + "/api/credits/balance"); err == nil {
		var ci creditInfo
		json.NewDecoder(resp.Body).Decode(&ci)
		resp.Body.Close()
		stats.Balance = ci.Balance
		stats.Frozen = ci.Frozen
		stats.TotalEarned = ci.TotalEarned
		stats.TotalSpent = ci.TotalSpent
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

// padLine pads a visible-text line to exactly termW, adds clear-to-EOL + \r\n.
func padLine(sb *strings.Builder, visibleLen, termW int) {
	if visibleLen < termW {
		sb.WriteString(strings.Repeat(" ", termW-visibleLen))
	}
	sb.WriteString("\033[K\r\n")
}

// renderHeader builds the static top 3 lines (title + subtitle + separator).
// It is cached and only rebuilt when stats or terminal width changes.
func renderHeader(termW int, slogan string, stats networkStats) string {
	innerW := termW - 2
	if innerW < 10 {
		innerW = 10
	}
	var sb strings.Builder

	// === ROW 1: TOP BORDER + TITLE ===
	titleText := " LetChat Agent Network "
	brandText := " " + slogan + " "
	titleLen := len([]rune(titleText))
	brandLen := len([]rune(brandText))
	fillTotal := innerW - titleLen - brandLen
	if fillTotal < 2 {
		fillTotal = 2
	}
	dashL := fillTotal / 2
	dashR := fillTotal - dashL

	sb.WriteString("\033[1;36m┌")
	sb.WriteString(strings.Repeat("─", dashL))
	sb.WriteString("\033[1;33m")
	sb.WriteString(titleText)
	sb.WriteString("\033[0;36m")
	sb.WriteString(brandText)
	sb.WriteString("\033[1;36m")
	sb.WriteString(strings.Repeat("─", dashR))
	sb.WriteString("┐\033[0m")
	sb.WriteString("\033[K\r\n")

	// === ROW 2: SUBTITLE + STATS ===
	powered := " Powered by Chatchat Technology Limited"
	statsText := fmt.Sprintf("Nodes:%d  Credits:%.0f  Topics:%d  v%s ",
		stats.Peers+1, stats.Balance, len(stats.Topics), daemon.Version)
	poweredLen := len([]rune(powered))
	statsLen := len(statsText)
	midGap := innerW - poweredLen - statsLen
	if midGap < 1 {
		midGap = 1
	}

	sb.WriteString("\033[1;36m│\033[0;35m")
	sb.WriteString(powered)
	sb.WriteString(strings.Repeat(" ", midGap))
	sb.WriteString("\033[1;33m")
	sb.WriteString(statsText)
	sb.WriteString("\033[1;36m│\033[0m")
	sb.WriteString("\033[K\r\n")

	// === ROW 3: SEPARATOR ===
	sb.WriteString("\033[1;36m├")
	sb.WriteString(strings.Repeat("─", innerW))
	sb.WriteString("┤\033[0m")
	sb.WriteString("\033[K\r\n")

	return sb.String()
}

// renderTopoFrame builds one complete frame: uses cached header + dynamic globe + node cards at bottom.
func renderTopoFrame(peers []peerGeoData, termW, termH int, rotation float64, header string, stats networkStats) string {
	innerW := termW - 2
	if innerW < 10 {
		innerW = 10
	}

	// Layout: 3 header rows + globeH rows + 1 separator + cardRows + 1 separator + 1 help + 1 bottom border
	// Card panel: each card ~4 rows high, laid out horizontally
	nPeers := len(peers)
	if nPeers == 0 {
		nPeers = 1
	}
	cardH := 4
	if termH < 30 {
		cardH = 3
	}
	if termH < 20 {
		cardH = 2
	}
	// Bottom section: 1(sep) + cardH + 1(sep) + 1(help) + 1(bottom border) = cardH + 4
	bottomH := cardH + 4
	globeH := termH - 3 - bottomH // 3 header rows
	if globeH < 5 {
		globeH = 5
	}

	gW := innerW
	gH := globeH

	// ── Render globe ──
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
			light := (rz + 1.0) / 2.0

			var ch rune
			switch terrain {
			case 2:
				if light > 0.3 {
					ch = '#'
				} else {
					ch = ':'
				}
			case 1:
				if light > 0.5 {
					ch = '.'
				} else {
					ch = ','
				}
			default:
				if light > 0.6 {
					ch = '~'
				} else if light > 0.2 {
					ch = '·'
				} else {
					ch = ' '
				}
			}
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
	type peerInfo struct {
		shortID  string
		location string
		country  string
		lat, lon float64
		isSelf   bool
		visible  bool
	}
	pInfos := make([]peerInfo, len(peers))
	for i, p := range peers {
		pi := peerInfo{
			shortID:  p.ShortID,
			location: p.Location,
			isSelf:   i == 0,
		}
		if p.Geo != nil {
			pi.country = p.Geo.Country
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
					globe[sy][sx] = '@'
				}
			}
		}
		pInfos[i] = pi
	}

	// ── Build frame ──
	var sb strings.Builder
	sb.Grow(termW * termH * 4)

	// Header (cached, already has \033[K\r\n on each line)
	sb.WriteString(header)

	// Globe rows
	for row := 0; row < gH; row++ {
		sb.WriteString("\033[1;36m│\033[0m")
		for _, ch := range globe[row] {
			switch ch {
			case '@':
				sb.WriteString("\033[1;31m@\033[0m")
			case '#', ':':
				sb.WriteString("\033[1;37m")
				sb.WriteRune(ch)
				sb.WriteString("\033[0m")
			case '.', ',':
				sb.WriteString("\033[32m")
				sb.WriteRune(ch)
				sb.WriteString("\033[0m")
			case '(', ')':
				sb.WriteString("\033[2;37m")
				sb.WriteRune(ch)
				sb.WriteString("\033[0m")
			case '~':
				sb.WriteString("\033[34m")
				sb.WriteRune(ch)
				sb.WriteString("\033[0m")
			case '·':
				sb.WriteString("\033[2;34m")
				sb.WriteRune(ch)
				sb.WriteString("\033[0m")
			default:
				sb.WriteByte(' ')
			}
		}
		sb.WriteString("\033[1;36m│\033[0m\033[K\r\n")
	}

	// ── Separator above cards ──
	sb.WriteString("\033[1;36m├")
	sb.WriteString(strings.Repeat("─", innerW))
	sb.WriteString("┤\033[0m\033[K\r\n")

	// ── Node cards row ──
	// Calculate card width per peer
	cardW := innerW / max(nPeers, 1)
	if cardW > 30 {
		cardW = 30
	}
	if cardW < 12 {
		cardW = 12
	}
	// Total cards row width
	totalCardsW := cardW * nPeers
	// Left padding to center cards
	cardPadL := (innerW - totalCardsW) / 2
	if cardPadL < 0 {
		cardPadL = 0
	}
	cardPadR := innerW - cardPadL - totalCardsW
	if cardPadR < 0 {
		cardPadR = 0
	}

	// Build card content for each peer (each card has cardH lines)
	type cardContent struct {
		lines []string // each line is exactly cardW visible chars
	}
	cards := make([]cardContent, nPeers)
	for i := range pInfos {
		p := pInfos[i]
		cc := cardContent{lines: make([]string, cardH)}
		insideW := cardW - 2 // minus left+right border

		marker := "●"
		borderH := "─"
		borderTL := "┌"
		borderTR := "┐"
		borderBL := "└"
		borderBR := "┘"
		borderV := "│"
		if p.isSelf {
			marker = "★"
			borderH = "═"
			borderTL = "╔"
			borderTR = "╗"
			borderBL = "╚"
			borderBR = "╝"
			borderV = "║"
		}

		// Line 0: top border
		cc.lines[0] = borderTL + strings.Repeat(borderH, insideW) + borderTR

		if cardH >= 3 {
			// Line 1: marker + short ID
			idText := fmt.Sprintf(" %s %s", marker, p.shortID)
			if len([]rune(idText)) > insideW {
				idText = string([]rune(idText)[:insideW])
			}
			pad := insideW - len([]rune(idText))
			if pad < 0 {
				pad = 0
			}
			cc.lines[1] = borderV + idText + strings.Repeat(" ", pad) + borderV

			if cardH >= 4 {
				// Line 2: location info
				loc := p.location
				if loc == "" || loc == "Unknown" {
					loc = p.country
				}
				if loc == "" {
					loc = "?"
				}
				locText := " " + loc
				if p.lat != 0 || p.lon != 0 {
					locText = fmt.Sprintf(" %s %.1f,%.1f", loc, p.lat, p.lon)
				}
				if len([]rune(locText)) > insideW {
					locText = string([]rune(locText)[:insideW])
				}
				pad = insideW - len([]rune(locText))
				if pad < 0 {
					pad = 0
				}
				cc.lines[2] = borderV + locText + strings.Repeat(" ", pad) + borderV

				// Line 3: bottom border
				cc.lines[3] = borderBL + strings.Repeat(borderH, insideW) + borderBR
			} else {
				// Line 2 (if cardH==3): bottom border
				cc.lines[2] = borderBL + strings.Repeat(borderH, insideW) + borderBR
			}
		} else {
			// cardH==2: compact
			idText := fmt.Sprintf(" %s %s", marker, p.shortID)
			if len([]rune(idText)) > insideW {
				idText = string([]rune(idText)[:insideW])
			}
			pad := insideW - len([]rune(idText))
			if pad < 0 {
				pad = 0
			}
			cc.lines[1] = borderBL + idText + strings.Repeat(" ", pad) + borderBR
		}
		cards[i] = cc
	}

	// Emit card rows
	for li := 0; li < cardH; li++ {
		sb.WriteString("\033[1;36m│\033[0m")
		sb.WriteString(strings.Repeat(" ", cardPadL))
		for ci, cc := range cards {
			color := "\033[33m"
			if pInfos[ci].isSelf {
				color = "\033[1;33m"
			}
			sb.WriteString(color)
			if li < len(cc.lines) {
				sb.WriteString(cc.lines[li])
			} else {
				sb.WriteString(strings.Repeat(" ", cardW))
			}
			sb.WriteString("\033[0m")
		}
		sb.WriteString(strings.Repeat(" ", cardPadR))
		sb.WriteString("\033[1;36m│\033[0m\033[K\r\n")
	}

	// ── Separator below cards ──
	sb.WriteString("\033[1;36m├")
	sb.WriteString(strings.Repeat("─", innerW))
	sb.WriteString("┤\033[0m\033[K\r\n")

	// ── Help line ──
	help := " q:Quit │ ★:You │ @:Peer │ #:Coast │ .:Land │ ~:Ocean"
	earnedSpent := fmt.Sprintf("Earned:%.0f Spent:%.0f ", stats.TotalEarned, stats.TotalSpent)
	helpLen := len([]rune(help))
	earnLen := len(earnedSpent)
	helpGap := innerW - helpLen - earnLen
	if helpGap < 1 {
		helpGap = 1
	}
	sb.WriteString("\033[1;36m│\033[0;2m")
	sb.WriteString(help)
	sb.WriteString(strings.Repeat(" ", helpGap))
	sb.WriteString("\033[0;33m")
	sb.WriteString(earnedSpent)
	sb.WriteString("\033[1;36m│\033[0m\033[K\r\n")

	// ── Bottom frame ──
	sb.WriteString("\033[1;36m└")
	sb.WriteString(strings.Repeat("─", innerW))
	sb.WriteString("┘\033[0m\033[K")

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
