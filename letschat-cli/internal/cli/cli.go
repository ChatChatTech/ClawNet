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

	// Verify daemon
	resp, err := http.Get(base + "/api/peers/geo")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	resp.Body.Close()

	// Pick a slogan for this session
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	slogan := topoSlogans[rng.Intn(len(topoSlogans))]

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

	// Cache network stats (refresh every 3 seconds)
	var netStats networkStats
	var lastStatsFetch time.Time

	for {
		select {
		case <-quit:
			return nil
		case <-sigCh:
			needClear = true
		case <-ticker.C:
			peers := fetchGeoPeers(base)
			w, h, err := term.GetSize(fd)
			if err != nil {
				w, h = 80, 24
			}
			// Refresh stats every 3 sec
			if time.Since(lastStatsFetch) > 3*time.Second {
				netStats = fetchNetworkStats(base)
				lastStatsFetch = time.Now()
			}
			if needClear {
				fmt.Print("\033[2J")
				needClear = false
			}
			frame := renderGlobeFrame(peers, w, h, angle, slogan, netStats)
			fmt.Print("\033[H")
			fmt.Print(frame)
			angle += 0.05
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
	// Status
	if resp, err := http.Get(base + "/api/status"); err == nil {
		json.NewDecoder(resp.Body).Decode(&stats)
		resp.Body.Close()
	}
	// Credits
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

// ── Layout presets ──

type layoutPreset struct {
	name    string
	minW    int
	minH    int
	globeW  int
	globeH  int
	panelW  int
	cardH   int // height of each peer card
}

var presets = []layoutPreset{
	{"xlarge", 160, 48, 80, 38, 32, 5},
	{"large",  120, 36, 56, 28, 28, 4},
	{"medium",  80, 24, 36, 16, 20, 3},
	{"small",   60, 16, 24, 10, 16, 2},
	{"tiny",    40, 10, 16,  7, 10, 2},
}

func pickPreset(termW, termH int) layoutPreset {
	for _, p := range presets {
		if termW >= p.minW && termH >= p.minH {
			return p
		}
	}
	return presets[len(presets)-1]
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

// ── Helper: write a string truncated / padded to exactly `w` visible chars ──
func fitStr(s string, w int) string {
	runes := []rune(s)
	if len(runes) >= w {
		return string(runes[:w])
	}
	return s + strings.Repeat(" ", w-len(runes))
}

// ── renderGlobeFrame ──
func renderGlobeFrame(peers []peerGeoData, termW, termH int, rotation float64, slogan string, stats networkStats) string {
	preset := pickPreset(termW, termH)
	gW := preset.globeW
	gH := preset.globeH

	// Layout: border(1) + [left panel] + [globe] + [right panel] + border(1)
	// Top: border(1) + title(1) + border(1) = 3 rows
	// Bottom: border(1) + help(1) = 2 rows
	// Content area = termH - 5

	contentH := termH - 5
	if gH > contentH {
		gH = contentH
	}
	if gH < 5 {
		gH = 5
	}

	innerW := termW - 2 // minus left+right border columns
	leftPW := (innerW - gW) / 2
	rightPW := innerW - gW - leftPW
	if leftPW < 2 {
		leftPW = 2
	}
	if rightPW < 2 {
		rightPW = 2
	}
	// Rebalance globe to use actual available space
	gW = innerW - leftPW - rightPW
	if gW < 10 {
		gW = 10
	}

	cardH := preset.cardH

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

	// ── Build peer info + project onto globe ──
	type peerScreen struct {
		idx      int
		shortID  string
		location string
		country  string
		lat, lon float64
		hasGeo   bool
		visible  bool
		screenX  int // column on globe buffer (0-based)
		screenY  int // row on globe buffer (0-based)
		isSelf   bool
	}
	pInfos := make([]peerScreen, len(peers))
	for i, p := range peers {
		pi := peerScreen{
			idx:      i,
			shortID:  p.ShortID,
			location: p.Location,
			isSelf:   i == 0,
		}
		if p.Geo != nil {
			pi.country = p.Geo.Country
			pi.lat = p.Geo.Latitude
			pi.lon = p.Geo.Longitude
			pi.hasGeo = true

			latR := p.Geo.Latitude * math.Pi / 180.0
			lonR := p.Geo.Longitude * math.Pi / 180.0
			px := math.Cos(latR) * math.Sin(lonR)
			py := math.Sin(latR)
			pz := math.Cos(latR) * math.Cos(lonR)
			rx := px*math.Cos(rotation) - pz*math.Sin(rotation)
			rz := px*math.Sin(rotation) + pz*math.Cos(rotation)
			if rz > 0.05 {
				pi.visible = true
				pi.screenX = int(cX + rx*rX)
				pi.screenY = int(cY - py*rY)
				if pi.screenX < 0 {
					pi.screenX = 0
				}
				if pi.screenX >= gW {
					pi.screenX = gW - 1
				}
				if pi.screenY < 0 {
					pi.screenY = 0
				}
				if pi.screenY >= gH {
					pi.screenY = gH - 1
				}
				globe[pi.screenY][pi.screenX] = '@'
			}
		}
		pInfos[i] = pi
	}

	// ── Assign peers to left/right card slots ──
	// Each card takes `cardH` rows. Cards are evenly spread in contentH.
	type cardSlot struct {
		startRow int // first row of this card in globe-coord space
		peer     peerScreen
	}
	var leftCards, rightCards []cardSlot
	for i, pi := range pInfos {
		if i%2 == 0 {
			leftCards = append(leftCards, cardSlot{peer: pi})
		} else {
			rightCards = append(rightCards, cardSlot{peer: pi})
		}
	}
	// Distribute cards evenly within gH rows
	distributeCards := func(cards []cardSlot, height, ch int) []cardSlot {
		n := len(cards)
		if n == 0 {
			return cards
		}
		totalCardH := n * ch
		if totalCardH > height {
			ch = height / n
			if ch < 1 {
				ch = 1
			}
		}
		gap := 0
		if n > 1 {
			gap = (height - totalCardH) / (n + 1)
			if gap < 0 {
				gap = 0
			}
		} else {
			gap = (height - ch) / 2
		}
		y := gap
		for i := range cards {
			cards[i].startRow = y
			y += ch + gap
			if y > height-ch {
				y = height - ch
			}
		}
		return cards
	}
	leftCards = distributeCards(leftCards, gH, cardH)
	rightCards = distributeCards(rightCards, gH, cardH)

	// ── Build screen buffer ──
	var sb strings.Builder

	// === TOP BORDER + TITLE ===
	// ┌──── LetChat Agent Network ─── Powered by Chatchat Technology Limited ────┐
	titleText := " LetChat Agent Network "
	brandText := fmt.Sprintf(" %s ", slogan)
	topInner := innerW
	titleLen := len([]rune(titleText))
	brandLen := len([]rune(brandText))
	dashLeft := (topInner - titleLen - brandLen) / 2
	if dashLeft < 1 {
		dashLeft = 1
	}
	dashRight := topInner - titleLen - brandLen - dashLeft
	if dashRight < 1 {
		dashRight = 1
	}

	sb.WriteString("\033[1;36m┌")
	sb.WriteString(strings.Repeat("─", dashLeft))
	sb.WriteString("\033[1;33m")
	sb.WriteString(titleText)
	sb.WriteString("\033[2;37m")
	sb.WriteString(strings.Repeat("─", max(1, dashRight-brandLen)))
	sb.WriteString("\033[0;36m")
	sb.WriteString(brandText)
	sb.WriteString("\033[1;36m")
	remaining := innerW - dashLeft - titleLen - max(1, dashRight-brandLen) - brandLen
	if remaining > 0 {
		sb.WriteString(strings.Repeat("─", remaining))
	}
	sb.WriteString("┐\033[0m\r\n")

	// === SUBTITLE (Powered by ...) + stats on right ===
	powered := " Powered by Chatchat Technology Limited"
	statsText := fmt.Sprintf("Nodes:%d  Credits:%.0f  Topics:%d  v%s ",
		stats.Peers+1, stats.Balance, len(stats.Topics), daemon.Version)
	subtitleInner := innerW
	statsLen := len(statsText)
	poweredLen := len(powered)
	midGap := subtitleInner - poweredLen - statsLen
	if midGap < 1 {
		midGap = 1
	}
	sb.WriteString("\033[1;36m│\033[0;35m")
	sb.WriteString(fitStr(powered, poweredLen))
	sb.WriteString(strings.Repeat(" ", midGap))
	sb.WriteString("\033[1;33m")
	sb.WriteString(fitStr(statsText, statsLen))
	sb.WriteString("\033[1;36m│\033[0m\r\n")

	// === SEPARATOR ===
	sb.WriteString("\033[1;36m├")
	sb.WriteString(strings.Repeat("─", innerW))
	sb.WriteString("┤\033[0m\r\n")

	// === CONTENT ROWS: [border][left panel][globe][right panel][border] ===
	// Pre-build left/right card row lookups
	type cardLine struct {
		text    string // rendered text for this line of the card
		lineIdx int    // which line within the card (0=top border, 1=id, 2=info, etc.)
		peer    peerScreen
	}
	leftLines := map[int]cardLine{}
	rightLines := map[int]cardLine{}

	buildCardLines := func(cards []cardSlot, pw, ch int, isLeft bool) map[int]cardLine {
		result := map[int]cardLine{}
		for _, card := range cards {
			p := card.peer
			for li := 0; li < ch; li++ {
				row := card.startRow + li
				if row >= gH {
					break
				}
				var text string
				selfMark := ""
				if p.isSelf {
					selfMark = "★"
				} else {
					selfMark = "●"
				}
				switch {
				case li == 0 && ch >= 3:
					// Top line: marker + short ID
					text = fmt.Sprintf(" %s %s", selfMark, p.shortID)
				case li == 1 && ch >= 3:
					// Info line: location / country
					loc := p.location
					if p.country != "" && p.country != loc {
						loc = p.country
					}
					if loc == "" {
						loc = "Unknown"
					}
					if p.hasGeo && (p.lat != 0 || p.lon != 0) {
						text = fmt.Sprintf("   %s %.1f,%.1f", loc, p.lat, p.lon)
					} else {
						text = fmt.Sprintf("   %s", loc)
					}
				case li == 0 && ch < 3:
					// Compact: everything on one line
					loc := p.location
					if loc == "" {
						loc = p.shortID
					}
					text = fmt.Sprintf(" %s %s", selfMark, loc)
				default:
					// Extra lines for larger cards
					if li == 2 && ch >= 4 {
						if p.isSelf {
							text = fmt.Sprintf("   Cr:%.0f Fr:%.0f", stats.Balance, stats.Frozen)
						} else {
							text = "   ──────"
						}
					} else if li == 3 && ch >= 5 {
						text = fmt.Sprintf("   PeerID:%s..", p.shortID[:8])
					} else {
						text = ""
					}
				}

				maxW := pw - 3 // leave room for connector
				if len([]rune(text)) > maxW {
					text = string([]rune(text)[:maxW])
				}
				result[row] = cardLine{text: text, lineIdx: li, peer: p}
			}
		}
		return result
	}
	leftLines = buildCardLines(leftCards, leftPW, cardH, true)
	rightLines = buildCardLines(rightCards, rightPW, cardH, false)

	for row := 0; row < gH; row++ {
		sb.WriteString("\033[1;36m│\033[0m") // left border

		// ── Left panel ──
		if cl, ok := leftLines[row]; ok {
			text := cl.text
			p := cl.peer

			// Determine connector target row on globe
			connChar := " "
			if p.visible {
				// Dynamic connector: point toward the @ on the globe
				if p.screenY == row {
					connChar = "→"
				} else if p.screenY > row {
					connChar = "↘"
				} else {
					connChar = "↗"
				}
			} else if cl.lineIdx == 0 {
				connChar = "·"
			}

			padLen := leftPW - len([]rune(text)) - 2
			if padLen < 0 {
				padLen = 0
			}
			color := "\033[33m" // yellow
			if p.isSelf {
				color = "\033[1;33m" // bright yellow
			}
			sb.WriteString(color)
			sb.WriteString(text)
			sb.WriteString(strings.Repeat(" ", padLen))
			sb.WriteString(connChar)
			sb.WriteString("\033[0m")
			sb.WriteByte(' ')
		} else {
			sb.WriteString(strings.Repeat(" ", leftPW))
		}

		// ── Globe ──
		for _, ch := range globe[row] {
			switch ch {
			case '@':
				sb.WriteString("\033[1;31m@\033[0m") // bright red
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

		// ── Right panel ──
		if cl, ok := rightLines[row]; ok {
			text := cl.text
			p := cl.peer

			connChar := " "
			if p.visible {
				if p.screenY == row {
					connChar = "←"
				} else if p.screenY > row {
					connChar = "↙"
				} else {
					connChar = "↖"
				}
			} else if cl.lineIdx == 0 {
				connChar = "·"
			}

			color := "\033[33m"
			if p.isSelf {
				color = "\033[1;33m"
			}
			sb.WriteByte(' ')
			sb.WriteString(color)
			sb.WriteString(connChar)
			// Pad text to fill right panel
			padLen := rightPW - len([]rune(text)) - 2
			if padLen < 0 {
				padLen = 0
			}
			sb.WriteString(text)
			sb.WriteString(strings.Repeat(" ", padLen))
			sb.WriteString("\033[0m")
		} else {
			sb.WriteString(strings.Repeat(" ", rightPW))
		}

		sb.WriteString("\033[1;36m│\033[0m") // right border
		sb.WriteString("\033[K\r\n")
	}

	// Pad remaining content rows
	for row := gH; row < contentH; row++ {
		sb.WriteString("\033[1;36m│\033[0m")
		sb.WriteString(strings.Repeat(" ", innerW))
		sb.WriteString("\033[1;36m│\033[0m")
		sb.WriteString("\033[K\r\n")
	}

	// === BOTTOM BORDER ===
	sb.WriteString("\033[1;36m├")
	sb.WriteString(strings.Repeat("─", innerW))
	sb.WriteString("┤\033[0m\r\n")

	// === HELP LINE ===
	help := " q:Quit │ ★:You │ @:Peer │ #:Coast │ .:Land │ ~:Ocean"
	earnedSpent := fmt.Sprintf("Earned:%.0f Spent:%.0f ", stats.TotalEarned, stats.TotalSpent)
	helpLen := len([]rune(help))
	earnLen := len([]rune(earnedSpent))
	helpGap := innerW - helpLen - earnLen
	if helpGap < 1 {
		helpGap = 1
	}
	sb.WriteString("\033[1;36m│\033[0;2m")
	sb.WriteString(help)
	sb.WriteString(strings.Repeat(" ", helpGap))
	sb.WriteString("\033[0;33m")
	sb.WriteString(earnedSpent)
	sb.WriteString("\033[1;36m│\033[0m\r\n")

	// === BOTTOM FRAME ===
	sb.WriteString("\033[1;36m└")
	sb.WriteString(strings.Repeat("─", innerW))
	sb.WriteString("┘\033[0m")

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
