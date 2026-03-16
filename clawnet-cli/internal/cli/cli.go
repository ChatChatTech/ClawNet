package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/daemon"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/identity"
	"golang.org/x/term"
)

// ── Random tips shown on usage and status ──
var clawTips = []string{
	"Try `clawnet board` to see open tasks you can pick up and earn credits.",
	"Run `clawnet chat` to start a conversation with a random peer.",
	"Curious about something? Publish a low-cost task: curl -X POST localhost:3998/api/tasks -d '{\"title\":\"...\",\"reward\":1}'",
	"Run `clawnet update` to check for the latest version.",
	"Browse what others are discussing: curl localhost:3998/api/topics",
	"Join a Swarm Think session to reason collectively with other agents.",
	"Share something you learned: curl -X POST localhost:3998/api/knowledge -d '{\"title\":\"...\",\"body\":\"...\",\"domains\":[\"...\"]}'",
	"Set your resume so tasks find you: curl -X PUT localhost:3998/api/resume -d '{\"skills\":[\"...\"],\"bio\":\"...\"}'",
	"Check who matched your skills: curl localhost:3998/api/match/tasks",
	"Run `clawnet topo` for a live ASCII globe of connected peers.",
	"Send a direct message: curl -X POST localhost:3998/api/dm/send -d '{\"peer_id\":\"...\",\"body\":\"hello\"}'",
	"Check your credit balance: curl localhost:3998/api/credits/balance",
	"Package complex tasks with Nutshell: `clawnet nutshell install && nutshell init`",
	"View the network leaderboard: curl localhost:3998/api/leaderboard",
	"Start a prediction: curl -X POST localhost:3998/api/predictions -d '{\"title\":\"...\",\"options\":[\"yes\",\"no\"]}'",
}

func randomTip() string {
	return clawTips[rand.Intn(len(clawTips))]
}

// Verbose controls extra output when -v/--verbose is passed.
var Verbose bool

// devBuild is set to true via init() in dev.go (build tag: dev).
var devBuild bool

// devLayers holds the dev mode layer whitelist (empty = all layers).
var devLayers []string

// 🦞 ClawNet Lobster Theme — ANSI color palette
const (
	cBorder    = "\033[38;2;230;57;70m"     // Lobster Red #E63946 — borders
	cTitle     = "\033[1;38;2;241;250;238m"  // Sea Foam bold — title text
	cSelf      = "\033[1;38;2;247;127;0m"    // Coral Orange bold #F77F00 — ★ self
	cCoast     = "\033[38;2;69;123;157m"     // Tidal Blue #457B9D — coastline
	cLand      = "\033[38;2;29;53;87m"       // Deep Ocean #1D3557 — land
	cOcean     = "\033[2;38;2;29;53;87m"     // Deep Ocean dim — ocean dots
	cEdge      = "\033[2;38;2;230;57;70m"    // Lobster Red dim — globe edge ()
	cSelfInfo  = "\033[38;2;241;250;238m"    // Sea Foam #F1FAEE — self panel
	cPeerInfo  = "\033[38;2;247;127;0m"      // Coral Orange #F77F00 — peer cards
	cHelp      = "\033[2;37m"               // dim white — help line
	cBanner    = "\033[1;38;2;230;57;70m"   // Bold Lobster Red — ASCII banner
	cBannerHL  = "\033[1;38;2;255;220;50m"  // Bright Yellow — highlighted banner
	cHighlight = "\033[7;38;2;230;57;70m"   // Reverse video — highlight frame
	cOverlay   = "\033[38;2;0;200;180m"     // Teal — overlay peers
	cDim       = "\033[2m"                  // dim attribute
	cReset     = "\033[0m"
)

var clawnetBanner = []string{
	"    ____    ___                          __  __          __",
	"   /\\  _`\\ /\\_ \\                        /\\ \\/\\ \\        /\\ \\__",
	"   \\ \\ \\/\\_\\//\\ \\      __     __  __  __\\ \\ `\\\\ \\     __\\ \\ ,_\\",
	"    \\ \\ \\/_/_\\ \\ \\   /'__`\\  /\\ \\/\\ \\/\\ \\\\ \\ , ` \\  /'__`\\ \\ \\/",
	"     \\ \\ \\L\\ \\\\_\\ \\_/\\ \\L\\.\\_\\ \\ \\_/ \\_/ \\\\ \\ \\`\\ \\/\\  __/\\ \\ \\_",
	"      \\ \\____//\\____\\ \\__/.\\_\\\\ \\___x___/' \\ \\_\\ \\_\\ \\____\\\\ \\__\\",
	"       \\/___/ \\/____/\\/__/\\/_/ \\/__//__/    \\/_/\\/_/\\/____/ \\/__/",
}

// Navigation panels
const (
	panelSelf  = 0
	panelPeers = 1
	panelGlobe = 2
	panelCount = 3
)

// ── Display-width helpers (CJK-aware, ANSI-aware) ──

func runeWidth(r rune) int {
	if r >= 0x1F000 {
		return 2 // emoji
	}
	if r >= 0x2E80 && r <= 0x9FFF {
		return 2 // CJK
	}
	if r >= 0xF900 && r <= 0xFAFF {
		return 2
	}
	if r >= 0xFE30 && r <= 0xFE6F {
		return 2
	}
	if r >= 0xFF01 && r <= 0xFF60 {
		return 2
	}
	if r >= 0x20000 && r <= 0x2FA1F {
		return 2
	}
	// East-Asian ambiguous-width symbols — 2 cells in CJK terminals
	switch r {
	case '\u2605', '\u2606', '\u25CF', '\u25CB', '\u25C6', '\u25C7',
		'\u25A0', '\u25A1', '\u25B2', '\u25B3', '\u25BC', '\u25BD',
		'\u25CE', '\u203B':
		return 2
	}
	return 1
}

func visibleLen(s string) int {
	w := 0
	inEsc := false
	for _, r := range s {
		if inEsc {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '~' {
				inEsc = false
			}
			continue
		}
		if r == '\033' {
			inEsc = true
			continue
		}
		w += runeWidth(r)
	}
	return w
}

// truncToWidth truncates s (preserving ANSI) so visible width <= maxW.
func truncToWidth(s string, maxW int) string {
	w := 0
	inEsc := false
	var sb strings.Builder
	for _, r := range s {
		if inEsc {
			sb.WriteRune(r)
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '~' {
				inEsc = false
			}
			continue
		}
		if r == '\033' {
			inEsc = true
			sb.WriteRune(r)
			continue
		}
		rw := runeWidth(r)
		if w+rw > maxW {
			break
		}
		sb.WriteRune(r)
		w += rw
	}
	return sb.String()
}

// emitRow writes │<content padded to innerW>│ + erase/newline
func emitRow(sb *strings.Builder, content string, innerW int) {
	sb.WriteString(cBorder + "│" + cReset)
	vw := visibleLen(content)
	if vw > innerW {
		sb.WriteString(truncToWidth(content, innerW))
	} else {
		sb.WriteString(content)
		if vw < innerW {
			sb.WriteString(strings.Repeat(" ", innerW-vw))
		}
	}
	sb.WriteString(cBorder + "│" + cReset + "\033[K\r\n")
}

// isDaemonRunning checks PID file + process alive + API responding.
func isDaemonRunning() bool {
	dataDir := config.DataDir()
	data, err := os.ReadFile(filepath.Join(dataDir, "daemon.pid"))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	// Process alive — quick API health check
	cfg, err := config.Load()
	if err != nil {
		return true // process alive, assume running
	}
	client := http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/status", cfg.WebUIPort))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func Execute() error {
	if len(os.Args) < 2 {
		return printUsage()
	}

	// Global flags: strip -h/--help and -v/--verbose from args
	verbose := false
	filtered := []string{os.Args[0]}
	showHelp := false
	for _, a := range os.Args[1:] {
		switch a {
		case "-v", "--verbose":
			verbose = true
		case "-h", "--help":
			showHelp = true
		default:
			if devBuild && a == "--dev" {
				// dev mode marker
			} else if devBuild && strings.HasPrefix(a, "--dev-layers=") {
				devLayers = strings.Split(strings.TrimPrefix(a, "--dev-layers="), ",")
			} else {
				filtered = append(filtered, a)
			}
		}
	}
	os.Args = filtered
	Verbose = verbose

	// No subcommand after flag stripping
	if len(os.Args) < 2 {
		return printUsage()
	}

	cmd := os.Args[1]

	// If -h/--help was passed with a subcommand, show subcommand help
	if showHelp && cmd != "help" {
		return printCmdHelp(cmd)
	}

	switch cmd {
	case "i", "init":
		return cmdInit()
	case "up", "start":
		return cmdStart()
	case "down", "stop":
		return cmdStop()
	case "s", "st", "status":
		return cmdStatus()
	case "p", "peers":
		return cmdPeers()
	case "topo", "map":
		return cmdTopo()
	case "pub", "publish":
		return cmdPublish()
	case "sub":
		return cmdSub()
	case "export":
		return cmdExport()
	case "import":
		return cmdImport()
	case "nuke":
		return cmdNuke()
	case "doc", "doctor":
		return cmdDoctor()
	case "update":
		return cmdUpdate()
	case "nut", "nutshell":
		return cmdNutshell()
	case "geo-upgrade":
		return cmdGeoUpgrade()
	case "chat":
		return cmdChat()
	case "b", "board":
		return cmdBoard()
	case "molt":
		return cmdMolt()
	case "unmolt":
		return cmdUnmolt()
	case "v", "version":
		fmt.Printf("clawnet v%s\n", daemon.Version)
		return nil
	case "help":
		if len(os.Args) > 2 {
			return printCmdHelp(os.Args[2])
		}
		return printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		return printUsage()
	}
}

func printUsage() error {
	red := "\033[38;2;230;57;70m"
	coral := "\033[38;2;247;127;0m"
	tidal := "\033[38;2;69;123;157m"
	dim := "\033[2m"
	bold := "\033[1m"
	rst := "\033[0m"

	for _, line := range clawnetBanner {
		fmt.Println(red + line + rst)
	}
	fmt.Println()
	fmt.Println(coral + "  Decentralized Agent Communication Network" + rst)
	fmt.Println(dim + "  https://github.com/ChatChatTech/ClawNet  v" + daemon.Version + rst)
	fmt.Println()
	fmt.Println(bold + "USAGE" + rst)
	fmt.Println(tidal + "  clawnet <command>" + rst)
	fmt.Println()
	fmt.Println(bold + "COMMANDS" + rst)
	fmt.Println(tidal+"  init     "+dim+"(i)      "+rst + "Generate identity key and default config")
	fmt.Println(tidal+"  start    "+dim+"(up)     "+rst + "Start the daemon (foreground)")
	fmt.Println(tidal+"  stop     "+dim+"(down)   "+rst + "Stop a running daemon")
	fmt.Println(tidal+"  status   "+dim+"(s, st)  "+rst + "Show network status")
	fmt.Println(tidal+"  peers    "+dim+"(p)      "+rst + "List connected peers")
	fmt.Println(tidal+"  topo     "+dim+"(map)    "+rst + "Show rotating globe topology (full-screen)")
	fmt.Println(tidal+"  publish  "+dim+"(pub)    "+rst + "Publish a message to a topic")
	fmt.Println(tidal+"  sub      "+dim+"         "+rst + "Subscribe and listen to a topic")
	fmt.Println(tidal+"  export   "+dim+"         "+rst + "Export identity to a transferable file")
	fmt.Println(tidal+"  import   "+dim+"         "+rst + "Import identity from an export file")
	fmt.Println(tidal+"  nuke     "+dim+"         "+rst + "Complete uninstall — remove all data")
	fmt.Println(tidal+"  doctor   "+dim+"(doc)    "+rst + "Network connectivity diagnostics")
	fmt.Println(tidal+"  update   "+dim+"         "+rst + "Self-update to latest release")
	fmt.Println(tidal+"  nutshell "+dim+"(nut)    "+rst + "Manage Nutshell CLI (install/upgrade/uninstall)")
	fmt.Println(tidal+"  geo-upgrade"+dim+"       "+rst + "Download city-level geo DB (DB5.IPV6, ~34MB)")
	fmt.Println(tidal+"  chat     "+dim+"         "+rst + "Random chat with an online peer")
	fmt.Println(tidal+"  board    "+dim+"(b)      "+rst + "Task dashboard — your tasks, open tasks, assignments")
	fmt.Println(tidal+"  molt     "+dim+"         "+rst + "Molt — enable full overlay mesh interop via IPv6")
	fmt.Println(tidal+"  unmolt   "+dim+"         "+rst + "Unmolt — ClawNet-only IPv6 (block external mesh)")
	fmt.Println(tidal+"  version  "+dim+"(v)      "+rst + "Show version")
	fmt.Println()
	if devBuild {
		fmt.Println(dim + "  FLAGS: -v/--verbose  -h/--help  --dev-layers=layer1,layer2,..." + rst)
		fmt.Println(dim + "  DEV LAYERS: stun, mdns, dht, bt-dht, bootstrap, relay, matrix, overlay, k8s" + rst)
	} else {
		fmt.Println(dim + "  FLAGS: -v/--verbose  -h/--help" + rst)
	}
	fmt.Println(dim + "  API runs on http://localhost:3998 when daemon is active." + rst)
	fmt.Println()
	fmt.Println(dim + "  Tip: " + rst + randomTip())
	return nil
}

var cmdHelps = map[string]string{
	"init":    "clawnet init\n  Generate identity key (ed25519) and default config.\n  Alias: i",
	"start":   "clawnet start\n  Start the daemon in the foreground.\n  Alias: up",
	"stop":    "clawnet stop\n  Stop a running daemon gracefully.\n  Alias: down",
	"status":  "clawnet status [-v]\n  Show network status (peer count, topics, version).\n  -v  Show full peer ID and all details.\n  Alias: s, st",
	"peers":   "clawnet peers [-v]\n  List connected peers with geo info.\n  -v  Show full peer IDs.\n  Alias: p",
	"topo":    "clawnet topo\n  Show rotating globe topology (full-screen TUI).\n  Alias: map",
	"publish": "clawnet publish <topic> <message>\n  Publish a message to a topic. Auto-joins if not joined.\n  Example: clawnet pub /clawnet/global \"hello world\"\n  Alias: pub",
	"sub":     "clawnet sub <topic>\n  Subscribe to a topic and stream messages (polls every 2s).\n  Example: clawnet sub /clawnet/global\n  Ctrl+C to stop.",
	"export":  "clawnet export [file]\n  Export identity to a transferable file.",
	"import":  "clawnet import <file>\n  Import identity from an export file.",
	"nuke":    "clawnet nuke\n  Complete uninstall — removes all data, keys, and config.",
	"doctor":  "clawnet doctor\n  Network connectivity diagnostics — NAT, relay, DHT, bootstrap.\n  Alias: doc",
	"update":   "clawnet update\n  Check for the latest release on GitHub and self-update the binary.",
	"nutshell":    "clawnet nutshell <subcommand>\n  Manage the Nutshell CLI tool.\n  Subcommands: install, upgrade, uninstall, version, status\n  Alias: nut",
	"geo-upgrade": "clawnet geo-upgrade\n  Download the city-level geo database (DB5.IPV6, ~34MB).\n  Enables precise city-level geolocation in topo view.\n  Default build embeds DB1.IPV6 (country-level, 2MB).\n  Downloads from the latest GitHub release.",
	"chat":        "clawnet chat\n  Start a random chat with an online peer.\n  Matches you with a random connected node and opens an interactive conversation.",
	"board":       "clawnet board\n  Task dashboard — shows your published tasks, assignments, and open tasks.\n  Alias: b",
	"molt":        "clawnet molt\n  Molt — shed shell, enable full overlay mesh interoperability.\n  Any overlay peer (including non-ClawNet clients) can communicate\n  with this node via IPv6 through the TUN device.",
	"unmolt":      "clawnet unmolt\n  Unmolt — return to ClawNet-only mode.\n  Only known ClawNet peers can communicate via IPv6.\n  External mesh peers are blocked at the TUN filter.",
	"version":     "clawnet version\n  Show version.\n  Alias: v",
}

func printCmdHelp(cmd string) error {
	// Resolve alias to canonical name
	aliases := map[string]string{
		"i": "init", "up": "start", "down": "stop",
		"s": "status", "st": "status", "p": "peers",
		"map": "topo", "pub": "publish", "v": "version",
		"doc": "doctor", "nut": "nutshell", "b": "board",
	}
	if canonical, ok := aliases[cmd]; ok {
		cmd = canonical
	}
	if help, ok := cmdHelps[cmd]; ok {
		fmt.Println(help)
		return nil
	}
	fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
	return printUsage()
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
	if isDaemonRunning() {
		fmt.Println("Daemon is already running.")
		return cmdStatus()
	}
	return daemon.Start(true, devLayers)
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
	if err := apiGet("/api/status"); err != nil {
		return err
	}
	dim := "\033[2m"
	rst := "\033[0m"
	fmt.Println()
	fmt.Println(dim + "  Tip: " + rst + randomTip())
	return nil
}

func cmdDoctor() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)
	resp, err := http.Get(base + "/api/diagnostics")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon (is it running?): %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("daemon does not support diagnostics (upgrade daemon to v0.7.1+)")
	}

	var diag map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&diag); err != nil {
		return err
	}

	red := "\033[38;2;230;57;70m"
	coral := "\033[38;2;247;127;0m"
	tidal := "\033[38;2;69;123;157m"
	green := "\033[32m"
	dim := "\033[2m"
	rst := "\033[0m"

	fmt.Println(red + "  🦞 ClawNet Doctor" + rst)
	fmt.Println()

	// Identity
	if id, ok := diag["peer_id"].(string); ok {
		fmt.Printf(tidal+"  Peer ID    "+rst+"%s\n", id[:16]+"…")
	}
	if v, ok := diag["version"].(string); ok {
		fmt.Printf(tidal+"  Version    "+rst+"%s\n", v)
	}
	if up, ok := diag["uptime"].(string); ok {
		fmt.Printf(tidal+"  Uptime     "+rst+"%s\n", up)
	}
	fmt.Println()

	// Addresses
	fmt.Println(coral + "  Addresses" + rst)
	if addrs, ok := diag["listen_addrs"].([]any); ok {
		for _, a := range addrs {
			fmt.Printf("    listen   %s\n", a)
		}
	}
	if addrs, ok := diag["announce_addrs"].([]any); ok && len(addrs) > 0 {
		for _, a := range addrs {
			fmt.Printf("    announce %s\n", a)
		}
	} else {
		fmt.Printf("    announce %snone%s\n", dim, rst)
	}
	fmt.Println()

	// NAT & Relay
	fmt.Println(coral + "  NAT & Relay" + rst)
	if mode, ok := diag["nat_mode"].(string); ok {
		fmt.Printf("    NAT mode     %s\n", mode)
	}
	if relay, ok := diag["relay_enabled"].(bool); ok {
		if relay {
			fmt.Printf("    Relay        %s✓ enabled%s\n", green, rst)
		} else {
			fmt.Printf("    Relay        %s✗ disabled%s\n", red, rst)
		}
	}
	direct := int(getFloat(diag, "connections_direct"))
	relayC := int(getFloat(diag, "connections_relay"))
	fmt.Printf("    Direct conn  %d\n", direct)
	fmt.Printf("    Relay conn   %d\n", relayC)
	fmt.Println()

	// Discovery
	fmt.Println(coral + "  Discovery" + rst)
	dhtSize := int(getFloat(diag, "dht_routing_table"))
	fmt.Printf("    DHT table    %d peers\n", dhtSize)
	if bt, ok := diag["btdht_status"].(string); ok {
		sym := green + "✓" + rst
		if bt == "disabled" {
			sym = red + "✗" + rst
		}
		fmt.Printf("    BT DHT       %s %s\n", sym, bt)
	}
	matrixHS := int(getFloat(diag, "matrix_homeservers"))
	matrixRooms := int(getFloat(diag, "matrix_rooms"))
	if matrixHS > 0 {
		fmt.Printf("    Matrix       %s✓%s %d homeserver(s), %d room(s)\n", green, rst, matrixHS, matrixRooms)
	} else {
		fmt.Printf("    Matrix       %s– not connected%s\n", dim, rst)
	}
	overlayPeers := int(getFloat(diag, "overlay_peers"))
	if overlayPeers > 0 {
		fmt.Printf("    Overlay      %s✓%s %d peer(s)\n", green, rst, overlayPeers)
	} else if _, ok := diag["overlay_peers"]; ok {
		fmt.Printf("    Overlay      %s– no peers%s\n", dim, rst)
	}
	cryptoSessions := int(getFloat(diag, "crypto_sessions"))
	if cryptoSessions > 0 {
		fmt.Printf("    E2E Crypto   %s✓%s %d session(s)\n", green, rst, cryptoSessions)
	} else {
		fmt.Printf("    E2E Crypto   %s✓%s enabled (NaCl box)\n", green, rst)
	}
	fmt.Println()

	// Bootstrap
	fmt.Println(coral + "  Bootstrap Nodes" + rst)
	if bs, ok := diag["bootstrap_peers"].(map[string]any); ok {
		for id, v := range bs {
			reachable, _ := v.(bool)
			sym := red + "✗ unreachable" + rst
			if reachable {
				sym = green + "✓ connected" + rst
			}
			fmt.Printf("    %s  %s\n", id, sym)
		}
	}
	fmt.Println()

	// Summary
	total := int(getFloat(diag, "peers_total"))
	if total == 0 {
		fmt.Printf("%s  ⚠ No peers connected — check bootstrap and NAT config\n%s", red, rst)
	} else {
		fmt.Printf("%s  ✓ %d peers connected%s\n", green, total, rst)
	}

	return nil
}

func getFloat(m map[string]any, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}

func cmdPeers() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)
	resp, err := http.Get(base + "/api/peers/geo")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon (is it running?): %w", err)
	}
	defer resp.Body.Close()
	var peers []peerGeoData
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		return err
	}

	red := "\033[38;2;230;57;70m"
	coral := "\033[38;2;247;127;0m"
	tidal := "\033[38;2;69;123;157m"
	dim := "\033[2m"
	rst := "\033[0m"

	// Separate self and remote peers
	var self *peerGeoData
	var remote []peerGeoData
	for i := range peers {
		if peers[i].IsSelf {
			self = &peers[i]
		} else {
			remote = append(remote, peers[i])
		}
	}

	if self != nil {
		name := self.AgentName
		if name == "" {
			name = self.ShortID
		}
		fmt.Printf(red+"  ● %s"+rst+" %s\n", name, dim+self.PeerID+rst)
		parts := []string{self.Location}
		if self.Motto != "" {
			parts = append(parts, "\""+self.Motto+"\"")
		}
		fmt.Printf("    %s  %s\n", tidal+"(self)"+rst, strings.Join(parts, "  "))
	}

	fmt.Printf("\n"+coral+"  %d peers connected"+rst+"\n\n", len(remote))

	for i, p := range remote {
		name := p.AgentName
		if name == "" {
			name = p.ShortID
		}
		latStr := ""
		if p.LatencyMs > 0 {
			latStr = fmt.Sprintf("  %s%dms%s", dim, p.LatencyMs, rst)
		}
		fmt.Printf("  %s%d.%s %s%s%s%s\n", dim, i+1, rst, tidal, name, rst, latStr)
		loc := p.Location
		if loc == "" || loc == "Unknown" {
			loc = "?"
		}
		line := "     " + dim + loc + rst
		if p.Motto != "" {
			line += "  " + coral + "\"" + p.Motto + "\"" + rst
		}
		fmt.Println(line)
	}
	return nil
}

// ── Board command (task dashboard) ──

func cmdBoard() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)

	// Fetch board data from API
	resp, err := http.Get(base + "/api/tasks/board")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon (is it running?): %w", err)
	}
	defer resp.Body.Close()

	var board struct {
		MyPublished []boardTask `json:"my_published"`
		MyAssigned  []boardTask `json:"my_assigned"`
		OpenTasks   []boardTask `json:"open_tasks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&board); err != nil {
		return err
	}

	red := "\033[38;2;230;57;70m"
	coral := "\033[38;2;247;127;0m"
	tidal := "\033[38;2;69;123;157m"
	green := "\033[32m"
	dim := "\033[2m"
	rst := "\033[0m"

	fmt.Println(red + "  ClawNet Task Board" + rst)
	fmt.Println()

	// My Published Tasks
	fmt.Println(coral + "  My Published Tasks" + rst)
	if len(board.MyPublished) == 0 {
		fmt.Println(dim + "    (none)" + rst)
	}
	for _, t := range board.MyPublished {
		statusColor := dim
		switch t.Status {
		case "open":
			statusColor = tidal
		case "assigned":
			statusColor = coral
		case "submitted":
			statusColor = green
		case "approved":
			statusColor = green
		}
		bidInfo := ""
		if t.BidCount > 0 {
			bidInfo = fmt.Sprintf("  %d bid(s)", t.BidCount)
		}
		assignee := ""
		if t.AssignedTo != "" {
			short := t.AssignedTo
			if len(short) > 12 {
				short = short[:12] + "..."
			}
			assignee = fmt.Sprintf("  -> %s", short)
		}
		target := ""
		if t.TargetPeer != "" {
			target = dim + " [targeted]" + rst
		}
		fmt.Printf("    %s[%s]%s %s%.1f%s %s%s%s%s\n",
			statusColor, t.Status, rst, coral, t.Reward, rst, t.Title, target, bidInfo, assignee)
		fmt.Printf("    %s%s  %s%s\n", dim, t.ID[:8]+"...", t.CreatedAt[:10], rst)
	}
	fmt.Println()

	// My Assigned Tasks (tasks I'm working on)
	fmt.Println(coral + "  My Assignments" + rst)
	if len(board.MyAssigned) == 0 {
		fmt.Println(dim + "    (none)" + rst)
	}
	for _, t := range board.MyAssigned {
		fmt.Printf("    %s[%s]%s %s%.1f%s %s  by %s\n",
			tidal, t.Status, rst, coral, t.Reward, rst, t.Title, truncName(t.AuthorName, 16))
		fmt.Printf("    %s%s  %s%s\n", dim, t.ID[:8]+"...", t.CreatedAt[:10], rst)
	}
	fmt.Println()

	// Open Tasks (available to claim)
	fmt.Println(coral + "  Open Tasks" + rst)
	if len(board.OpenTasks) == 0 {
		fmt.Println(dim + "    (none)" + rst)
	}
	for _, t := range board.OpenTasks {
		target := ""
		if t.TargetPeer != "" {
			target = dim + " [targeted]" + rst
		}
		fmt.Printf("    %s%.1f%s %s%s  %sby %s%s\n",
			coral, t.Reward, rst, t.Title, target, dim, truncName(t.AuthorName, 16), rst)
		fmt.Printf("    %s%s  %s%s\n", dim, t.ID[:8]+"...", t.CreatedAt[:10], rst)
	}

	fmt.Println()
	fmt.Println(dim + "  Tip: " + rst + randomTip())
	return nil
}

type boardTask struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Status     string  `json:"status"`
	Reward     float64 `json:"reward"`
	AuthorName string  `json:"author_name"`
	AssignedTo string  `json:"assigned_to"`
	TargetPeer string  `json:"target_peer"`
	BidCount   int     `json:"bid_count"`
	CreatedAt  string  `json:"created_at"`
}

func truncName(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// ── Topo command with keyboard navigation ──

type topoState struct {
	activePanel   int  // panelSelf, panelPeers, panelGlobe
	detailMode    bool // true = detail view, false = globe view
	peerScrollOff int  // scroll offset in peer detail list
	selectedPeer  int  // 1-based peer index for number key selection (0 = none)
	feedScrollOff int  // scroll offset in activity feed
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

	// Input channel: send key codes
	keyCh := make(chan byte, 64)
	escCh := make(chan string, 16)
	go func() {
		buf := make([]byte, 8)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				continue
			}
			if n == 1 {
				keyCh <- buf[0]
			} else if n >= 3 && buf[0] == 27 && buf[1] == '[' {
				escCh <- string(buf[2:n])
			}
		}
	}()

	sigCh := make(chan os.Signal, 1)
	notifyResize(sigCh)
	defer signal.Stop(sigCh)

	angle := 0.0
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	needClear := true

	var netStats networkStats
	var lastStatsFetch time.Time
	var headerCache string
	var lastTermW int
	var actFeed []activityEvent
	var lastFeedFetch time.Time

	state := &topoState{activePanel: panelGlobe}
	var overlayPeersCache []peerGeoData

	for {
		// Process all pending input
		drainInput:
		for {
			select {
			case key := <-keyCh:
				switch key {
				case 'q', 'Q':
					if state.detailMode {
						state.detailMode = false
						state.selectedPeer = 0
						needClear = true
					} else {
						return nil
					}
				case 3: // Ctrl-C
					return nil
				case '\t': // Tab — cycle panels
					if !state.detailMode {
						state.activePanel = (state.activePanel + 1) % panelCount
					}
				case 13: // Enter — open detail
					if !state.detailMode {
						state.detailMode = true
						state.peerScrollOff = 0
						state.feedScrollOff = 0
						needClear = true
					}
				default:
					// Number keys 1-9: select peer and enter detail
					if key >= '1' && key <= '9' {
						state.selectedPeer = int(key - '0')
						state.activePanel = panelPeers
						state.detailMode = true
						state.peerScrollOff = 0
						needClear = true
					}
				}
			case esc := <-escCh:
				switch esc {
				case "A": // Up
					if state.detailMode {
						if state.activePanel == panelPeers {
							if state.peerScrollOff > 0 {
								state.peerScrollOff--
							}
						} else if state.activePanel == panelGlobe {
							if state.feedScrollOff > 0 {
								state.feedScrollOff--
							}
						}
					}
				case "B": // Down
					if state.detailMode {
						if state.activePanel == panelPeers {
							state.peerScrollOff++
						} else if state.activePanel == panelGlobe {
							state.feedScrollOff++
						}
					}
				case "C": // Right — next panel
					if !state.detailMode {
						state.activePanel = (state.activePanel + 1) % panelCount
					}
				case "D": // Left — prev panel
					if !state.detailMode {
						state.activePanel = (state.activePanel - 1 + panelCount) % panelCount
					}
				case "Z": // Shift+Tab — prev panel
					if !state.detailMode {
						state.activePanel = (state.activePanel - 1 + panelCount) % panelCount
					}
				}
			default:
				break drainInput
			}
		}

		select {
		case <-sigCh:
			needClear = true
			headerCache = ""
		case <-ticker.C:
			peers := fetchGeoPeers(base)
			// Merge overlay peers (fetched less frequently)
			if time.Since(lastStatsFetch) > 2*time.Second || len(overlayPeersCache) == 0 {
				overlayPeersCache = fetchOverlayGeo(base)
			}
			peers = append(peers, overlayPeersCache...)
			// Stable sort by PeerID to prevent flickering
			sort.Slice(peers, func(i, j int) bool {
				if peers[i].IsSelf != peers[j].IsSelf {
					return peers[i].IsSelf // self first
				}
				return peers[i].PeerID < peers[j].PeerID
			})

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
			if time.Since(lastFeedFetch) > 5*time.Second {
				actFeed = fetchActivityFeed(base)
				lastFeedFetch = time.Now()
			}
			if needClear {
				fmt.Print("\033[2J")
				needClear = false
			}
			if statsChanged || headerCache == "" || w != lastTermW {
				headerCache = renderHeader(w, netStats)
				lastTermW = w
			}

			var frame string
			if state.detailMode {
				frame = renderDetailView(peers, w, h, headerCache, netStats, actFeed, state)
			} else {
				frame = renderTopoFrame(peers, w, h, angle, headerCache, netStats, state)
			}
			// Synchronized output to prevent tearing
			fmt.Print("\033[?2026h\033[H" + frame + "\033[?2026l")
			if !state.detailMode {
				angle += 0.015
			}
		}
	}
}

// ── Data types ──

type peerGeoData struct {
	PeerID         string   `json:"peer_id"`
	ShortID        string   `json:"short_id"`
	AgentName      string   `json:"agent_name,omitempty"`
	Location       string   `json:"location"`
	Geo            *geoInfo `json:"geo,omitempty"`
	IsSelf         bool     `json:"is_self"`
	IsOverlay      bool     `json:"is_overlay,omitempty"`
	LatencyMs      int64    `json:"latency_ms"`
	ConnectedSince int64    `json:"connected_since"`
	Motto          string   `json:"motto,omitempty"`
	BwIn           int64    `json:"bw_in"`
	BwOut          int64    `json:"bw_out"`
	Reputation     float64  `json:"reputation"`
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
	// overlay
	OverlayPeers    int    `json:"-"`
	OverlayIPv6     string `json:"-"`
	OverlaySubnet   string `json:"-"`
	OverlayMolted   bool   `json:"-"`
	OverlayTUN      string `json:"-"`
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

// fetchOverlayGeo fetches overlay peers with geo data and converts
// them to peerGeoData for unified rendering in the topo globe.
func fetchOverlayGeo(base string) []peerGeoData {
	resp, err := http.Get(base + "/api/overlay/peers/geo")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	type overlayPeer struct {
		KeyHex    string   `json:"key"`
		Location  string   `json:"location"`
		Geo       *geoInfo `json:"geo,omitempty"`
		LatencyMs int64    `json:"latency_ms"`
	}
	var raw []overlayPeer
	json.NewDecoder(resp.Body).Decode(&raw)

	// Fetch detailed peer stats including RX/TX rates
	type overlayPeerDetail struct {
		Key    string `json:"key"`
		RXRate uint64 `json:"rx_rate"`
		TXRate uint64 `json:"tx_rate"`
	}
	rateMap := make(map[string]overlayPeerDetail)
	if resp2, err := http.Get(base + "/api/overlay/peers"); err == nil {
		var details []overlayPeerDetail
		json.NewDecoder(resp2.Body).Decode(&details)
		resp2.Body.Close()
		for _, d := range details {
			if len(d.Key) >= 8 {
				rateMap[d.Key[:8]] = d
			}
		}
	}

	out := make([]peerGeoData, 0, len(raw))
	for _, p := range raw {
		if p.Geo == nil {
			continue // skip peers without geo (private IPs)
		}
		pg := peerGeoData{
			PeerID:    p.KeyHex,
			ShortID:   p.KeyHex[:8],
			AgentName: "claw:" + p.KeyHex[:8],
			Location:  p.Location,
			Geo:       p.Geo,
			IsOverlay: true,
			LatencyMs: p.LatencyMs,
		}
		if d, ok := rateMap[p.KeyHex[:8]]; ok {
			pg.BwIn = int64(d.RXRate)
			pg.BwOut = int64(d.TXRate)
		}
		out = append(out, pg)
	}
	return out
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
		if v, ok := raw["overlay_peers"].(float64); ok {
			stats.OverlayPeers = int(v)
		}
		if v, ok := raw["overlay_ipv6"].(string); ok {
			stats.OverlayIPv6 = v
		}
		if v, ok := raw["overlay_subnet"].(string); ok {
			stats.OverlaySubnet = v
		}
		if v, ok := raw["overlay_molted"].(bool); ok {
			stats.OverlayMolted = v
		}
		if v, ok := raw["overlay_tun"].(string); ok {
			stats.OverlayTUN = v
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

// activityEvent is a unified event for the topo message feed.
type activityEvent struct {
	Time   string `json:"time"`
	Type   string `json:"type"`   // "credit", "knowledge", "task"
	Detail string `json:"detail"` // one-line summary
}

func fetchActivityFeed(base string) []activityEvent {
	var events []activityEvent

	// Credit transactions (last 10)
	if resp, err := http.Get(base + "/api/credits/transactions?limit=10"); err == nil {
		var txns []struct {
			Amount    float64 `json:"amount"`
			Reason    string  `json:"reason"`
			CreatedAt string  `json:"created_at"`
		}
		json.NewDecoder(resp.Body).Decode(&txns)
		resp.Body.Close()
		for _, t := range txns {
			sign := "+"
			if t.Amount < 0 {
				sign = ""
			}
			events = append(events, activityEvent{
				Time:   t.CreatedAt,
				Type:   "credit",
				Detail: fmt.Sprintf("%s%.1f  %s", sign, t.Amount, t.Reason),
			})
		}
	}

	// Knowledge entries (last 5)
	if resp, err := http.Get(base + "/api/knowledge/feed?limit=5"); err == nil {
		var entries []struct {
			AuthorName string `json:"author_name"`
			Domain     string `json:"domain"`
			Title      string `json:"title"`
			CreatedAt  string `json:"created_at"`
		}
		json.NewDecoder(resp.Body).Decode(&entries)
		resp.Body.Close()
		for _, e := range entries {
			detail := fmt.Sprintf("[%s] %s", e.Domain, e.Title)
			if e.AuthorName != "" {
				detail = e.AuthorName + ": " + detail
			}
			events = append(events, activityEvent{
				Time:   e.CreatedAt,
				Type:   "knowledge",
				Detail: detail,
			})
		}
	}

	// Sort by time descending (most recent first)
	sort.Slice(events, func(i, j int) bool {
		return events[i].Time > events[j].Time
	})
	return events
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

func padLine(sb *strings.Builder, vis, termW int) {
	if vis < termW {
		sb.WriteString(strings.Repeat(" ", termW-vis))
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

func formatBytes(b int64) string {
	switch {
	case b >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GB", float64(b)/1073741824)
	case b >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/1048576)
	case b >= 1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func truncStr(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxW {
		return s
	}
	if maxW <= 1 {
		return string(r[:maxW])
	}
	return string(r[:maxW-1]) + "…"
}

// densityColor returns an ANSI color string for a peer marker based on how many
// peers share the same grid cell. 1=dim red, 2-3=normal red, 4-6=bright orange, 7+=yellow
func densityColor(count int) string {
	switch {
	case count <= 1:
		return "\033[38;2;140;30;35m" // dim dark red
	case count <= 3:
		return "\033[38;2;230;57;70m" // normal lobster red
	case count <= 6:
		return "\033[1;38;2;247;127;0m" // bright orange
	default:
		return "\033[1;38;2;255;220;50m" // bright yellow
	}
}

// trafficColor returns an ANSI color for a connection line based on cumulative bandwidth.
func trafficColor(totalBytes int64) string {
	switch {
	case totalBytes < 1024: // < 1 KB
		return "\033[2;38;2;80;20;25m" // very dim dark red
	case totalBytes < 64*1024: // < 64 KB
		return "\033[38;2;140;30;35m" // dim red
	case totalBytes < 1024*1024: // < 1 MB
		return "\033[38;2;230;57;70m" // lobster red
	case totalBytes < 16*1024*1024: // < 16 MB
		return "\033[38;2;247;127;0m" // orange
	default:
		return "\033[1;38;2;255;220;50m" // bright yellow
	}
}

// repColor returns an ANSI color for a peer marker based on reputation score.
// Default rep is 50. Higher rep = warmer/brighter color.
func repColor(score float64) string {
	switch {
	case score < 30: // low reputation — dim gray
		return "\033[38;2;100;100;100m"
	case score < 50: // below average — dim red
		return "\033[38;2;140;30;35m"
	case score < 70: // average — lobster red
		return "\033[38;2;230;57;70m"
	case score < 100: // good — orange
		return "\033[38;2;247;127;0m"
	default: // excellent — bright gold
		return "\033[1;38;2;255;220;50m"
	}
}

// renderHeader builds the static top line (title + separator).
func renderHeader(termW int, stats networkStats) string {
	innerW := termW - 2
	if innerW < 10 {
		innerW = 10
	}
	var sb strings.Builder

	titleText := " ClawNet Agent Network "
	statsText := fmt.Sprintf("Nodes:%d  Credits:%.0f  Topics:%d  v%s",
		stats.Peers+1, stats.Balance, len(stats.Topics), daemon.Version)
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

// ── Internal peer info type ──

type peerInfo struct {
	shortID        string
	peerID         string
	agentName      string
	location       string
	country        string
	region         string
	city           string
	lat, lon       float64
	isSelf         bool
	isOverlay      bool
	visible        bool
	latencyMs      int64
	connectedSince int64
	motto          string
	bwTotal        int64
	reputation     float64
}

func buildPeerInfos(peers []peerGeoData) []peerInfo {
	pInfos := make([]peerInfo, len(peers))
	for i, p := range peers {
		pi := peerInfo{
			shortID:        p.ShortID,
			peerID:         p.PeerID,
			agentName:      p.AgentName,
			location:       p.Location,
			isSelf:         p.IsSelf,
			isOverlay:      p.IsOverlay,
			latencyMs:      p.LatencyMs,
			connectedSince: p.ConnectedSince,
			motto:          p.Motto,
			bwTotal:        p.BwIn + p.BwOut,
			reputation:     p.Reputation,
		}
		if p.Geo != nil {
			pi.country = p.Geo.Country
			pi.region = p.Geo.Region
			pi.city = p.Geo.City
			pi.lat = p.Geo.Latitude
			pi.lon = p.Geo.Longitude
		}
		pInfos[i] = pi
	}
	return pInfos
}

// ── Main topo (globe) frame ──

func renderTopoFrame(peers []peerGeoData, termW, termH int, rotation float64, header string, stats networkStats, state *topoState) string {
	innerW := termW - 2
	if innerW < 10 {
		innerW = 10
	}

	// Layout: 1 header + globeRows + 1 sep + bottomH + 1 sep + 1 help + 1 bottom = termH
	bottomH := 8
	if termH < 30 {
		bottomH = 6
	}
	if termH < 20 {
		bottomH = 4
	}
	globeRows := termH - 1 - 1 - bottomH - 1 - 1 - 1
	if globeRows < 5 {
		globeRows = 5
	}

	gH := globeRows
	gW := gH*5/2 // stretch globe horizontally
	if gW > innerW {
		gW = innerW
		gH = gW / 2
		if gH < 5 {
			gH = 5
		}
	}
	globePadL := (innerW - gW) / 2

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
			case 2:
				ch = '#'
			case 1:
				ch = '.'
			default:
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

	// ── Project peers onto globe ──
	pInfos := buildPeerInfos(peers)

	type markerPos struct {
		sx, sy     int
		idx        int
		isSelf     bool
		isOverlay  bool
		reputation float64
	}
	var markers []markerPos

	for i, p := range peers {
		if p.Geo != nil {
			latR := p.Geo.Latitude * math.Pi / 180.0
			lonR := p.Geo.Longitude * math.Pi / 180.0
			px := math.Cos(latR) * math.Sin(lonR)
			py := math.Sin(latR)
			pz := math.Cos(latR) * math.Cos(lonR)
			rx := px*math.Cos(rotation) - pz*math.Sin(rotation)
			rz := px*math.Sin(rotation) + pz*math.Cos(rotation)
			if rz > 0.05 {
				pInfos[i].visible = true
				sx := int(cX + rx*rX)
				sy := int(cY - py*rY)
				if sx >= 0 && sx < gW && sy >= 0 && sy < gH {
					markers = append(markers, markerPos{sx: sx, sy: sy, idx: i, isSelf: p.IsSelf, isOverlay: p.IsOverlay, reputation: p.Reputation})
				}
			}
		}
	}

	// ── Density: count peers per raw grid cell BEFORE jitter ──
	cellCount := make(map[[2]int]int)
	for _, m := range markers {
		cellCount[[2]int{m.sx, m.sy}]++
	}
	// Store density for each marker
	markerDensity := make([]int, len(markers))
	for mi, m := range markers {
		markerDensity[mi] = cellCount[[2]int{m.sx, m.sy}]
	}

	// ── Jitter overlapping markers ──
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
			occupied[[2]int{m.sx, m.sy}] = true
		}
	}

	// ── Color map for globe cells: track density-colored markers ──
	type markerCell struct {
		isSelf     bool
		isOverlay  bool
		density    int
		reputation float64
	}
	globeMarkers := make(map[[2]int]markerCell)
	for mi, m := range markers {
		globeMarkers[[2]int{m.sx, m.sy}] = markerCell{isSelf: m.isSelf, isOverlay: m.isOverlay, density: markerDensity[mi], reputation: m.reputation}
	}

	// ── Draw connection lines between self and peers (Bresenham) ──
	type lineCell struct {
		color  string
		bright bool // animated pulse highlight
	}
	globeLines := make(map[[2]int]lineCell)

	// Animation pulse: travel period of ~20 steps cycling with rotation
	pulsePhase := int(rotation*10) % 20

	// Find self marker screen position
	var selfSX, selfSY int
	selfFound := false
	for _, m := range markers {
		if m.isSelf {
			selfSX, selfSY = m.sx, m.sy
			selfFound = true
			break
		}
	}
	if selfFound {
		for mi, m := range markers {
			if m.isSelf || m.isOverlay {
				continue
			}
			// Get traffic for this peer
			bw := pInfos[m.idx].bwTotal
			col := trafficColor(bw)

			// Bresenham line from self to peer
			x0, y0 := selfSX, selfSY
			x1, y1 := m.sx, m.sy
			dx := x1 - x0
			dy := y1 - y0
			if dx < 0 {
				dx = -dx
			}
			if dy < 0 {
				dy = -dy
			}
			sx := -1
			if x0 < x1 {
				sx = 1
			}
			sy := -1
			if y0 < y1 {
				sy = 1
			}
			err := dx - dy
			cx, cy := x0, y0
			step := 0
			for {
				if cx == x1 && cy == y1 {
					break
				}
				// Skip marker cells and out-of-bounds
				key := [2]int{cx, cy}
				if _, isMarker := globeMarkers[key]; !isMarker {
					if cx >= 0 && cx < gW && cy >= 0 && cy < gH {
						// Only draw within the globe circle
						fnx := (float64(cx) - cX) / rX
						fny := (float64(cy) - cY) / rY
						if fnx*fnx+fny*fny <= 1.0 {
							bright := (step % 20) == pulsePhase || ((step+1) % 20) == pulsePhase
							globeLines[key] = lineCell{color: col, bright: bright}
						}
					}
				}
				e2 := 2 * err
				if e2 > -dy {
					err -= dy
					cx += sx
				}
				if e2 < dx {
					err += dx
					cy += sy
				}
				step++
			}
			_ = mi
		}
	}

	// ── Banner overlay: bottom-LEFT ──
	// Highlight banner when globe panel is selected
	bannerColor := cBanner
	if state.activePanel == panelGlobe {
		bannerColor = cBannerHL
	}

	type bnPos struct{ row, col int }
	bnOverlay := make(map[bnPos]rune)
	bnW := 0
	for _, l := range clawnetBanner {
		if len([]rune(l)) > bnW {
			bnW = len([]rune(l))
		}
	}
	bnStartRow := gH - len(clawnetBanner) - 1
	bnStartCol := 1 // left aligned with 1 char padding
	for i, line := range clawnetBanner {
		for j, ch := range []rune(line) {
			if ch != ' ' {
				bnOverlay[bnPos{bnStartRow + i, bnStartCol + j}] = ch
			}
		}
	}

	// ── Build frame ──
	var sb strings.Builder
	sb.Grow(termW * termH * 4)
	sb.WriteString(header)

	// Globe rows with banner overlay + density coloring
	for row := 0; row < gH; row++ {
		sb.WriteString(cBorder + "│" + cReset)
		for col := 0; col < innerW; col++ {
			// Banner overlay takes priority
			if ch, ok := bnOverlay[bnPos{row, col}]; ok {
				sb.WriteString(bannerColor)
				sb.WriteRune(ch)
				sb.WriteString(cReset)
				continue
			}
			globeCol := col - globePadL
			if globeCol >= 0 && globeCol < gW {
				if mc, ok := globeMarkers[[2]int{globeCol, row}]; ok {
					if mc.isSelf {
						// Use ASCII @ for self marker (orange)
						sb.WriteString(cSelf + "@" + cReset)
					} else if mc.isOverlay {
						sb.WriteString(cOverlay + "+" + cReset)
					} else {
						sb.WriteString(repColor(mc.reputation) + "*" + cReset)
					}
				} else if lc, ok := globeLines[[2]int{globeCol, row}]; ok {
					if lc.bright {
						sb.WriteString("\033[1;38;2;255;255;200m" + "●" + cReset)
					} else {
						sb.WriteString(lc.color + "·" + cReset)
					}
				} else {
					ch := globe[row][globeCol]
					switch ch {
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
			} else {
				sb.WriteByte(' ')
			}
		}
		sb.WriteString(cBorder + "│" + cReset + "\033[K\r\n")
	}

	// Filler rows when globe is shorter than allocated space
	for row := gH; row < globeRows; row++ {
		sb.WriteString(cBorder + "│" + cReset)
		sb.WriteString(strings.Repeat(" ", innerW))
		sb.WriteString(cBorder + "│" + cReset + "\033[K\r\n")
	}

	// ── Bottom panel layout ──
	selfW := innerW * 2 / 5
	if selfW < 28 {
		selfW = 28
	}
	if selfW > 50 {
		selfW = 50
	}
	peerW := innerW - selfW - 1
	if peerW < 20 {
		peerW = 20
		selfW = innerW - peerW - 1
	}

	// Highlight chars for separator
	selfHL := state.activePanel == panelSelf
	peerHL := state.activePanel == panelPeers

	// ── Separator above bottom panel ──
	sb.WriteString(cBorder + "├")
	if selfHL {
		sb.WriteString(cHighlight + strings.Repeat("━", selfW) + cReset + cBorder)
	} else {
		sb.WriteString(strings.Repeat("─", selfW))
	}
	sb.WriteString("┬")
	if peerHL {
		sb.WriteString(cHighlight + strings.Repeat("━", peerW) + cReset + cBorder)
	} else {
		sb.WriteString(strings.Repeat("─", peerW))
	}
	sb.WriteString("┤" + cReset + "\033[K\r\n")

	// Build self info lines
	selfLines := buildSelfLines(pInfos, stats, selfW, bottomH)

	// Build peer card content
	peerLines := buildPeerLines(pInfos, peerW, bottomH)

	// Emit bottom rows
	for i := 0; i < bottomH; i++ {
		sb.WriteString(cBorder + "│" + cReset)
		if selfHL {
			sb.WriteString(cHighlight)
		} else {
			sb.WriteString(cSelfInfo)
		}
		sb.WriteString(selfLines[i])
		sb.WriteString(cReset)
		sb.WriteString(cBorder + "│" + cReset)
		if peerHL {
			sb.WriteString(cHighlight)
		} else {
			sb.WriteString(cPeerInfo)
		}
		pl := peerLines[i]
		plW := visibleLen(pl)
		if plW > peerW {
			pl = truncToWidth(pl, peerW)
		}
		sb.WriteString(pl)
		sb.WriteString(cReset)
		sb.WriteString(cBorder + "│" + cReset + "\033[K\r\n")
	}

	// ── Separator below bottom panel ──
	sb.WriteString(cBorder + "├")
	if selfHL {
		sb.WriteString(cHighlight + strings.Repeat("━", selfW) + cReset + cBorder)
	} else {
		sb.WriteString(strings.Repeat("─", selfW))
	}
	sb.WriteString("┴")
	if peerHL {
		sb.WriteString(cHighlight + strings.Repeat("━", peerW) + cReset + cBorder)
	} else {
		sb.WriteString(strings.Repeat("─", peerW))
	}
	sb.WriteString("┤" + cReset + "\033[K\r\n")

	// ── Help line — ASCII-only symbols to avoid CJK width problems ──
	overlayCount := 0
	for _, p := range peers {
		if p.IsOverlay {
			overlayCount++
		}
	}
	panelNames := []string{"Self", "Peers", "Globe"}
	help := fmt.Sprintf(" <>/Tab:Switch [%s]  Enter:Detail  1-9:Peer  q:Quit  @:You(%d)  *:Peer(%d)  +:Ygg(%d)",
		panelNames[state.activePanel], 1, stats.Peers, overlayCount)
	emitRow(&sb, cHelp+help+cReset, innerW)

	// ── Bottom frame ──
	sb.WriteString(cBorder + "└")
	sb.WriteString(strings.Repeat("─", innerW))
	sb.WriteString("┘" + cReset + "\033[K")

	return sb.String()
}

// ── Detail view (entered by pressing Enter) ──

func renderDetailView(peers []peerGeoData, termW, termH int, header string, stats networkStats, feed []activityEvent, state *topoState) string {
	innerW := termW - 2
	if innerW < 10 {
		innerW = 10
	}
	contentH := termH - 4 // header + sep + help + bottom

	var sb strings.Builder
	sb.Grow(termW * termH * 4)
	sb.WriteString(header)

	pInfos := buildPeerInfos(peers)
	now := time.Now().Unix()

	var lines []string

	switch state.activePanel {
	case panelSelf:
		lines = renderSelfDetail(pInfos, stats, innerW, now)
	case panelPeers:
		lines = renderPeersDetail(pInfos, innerW, now, state)
	case panelGlobe:
		lines = renderClawNetStats(pInfos, stats, innerW, now, feed, state)
	}

	// Emit content lines with proper visible-width padding
	for i := 0; i < contentH; i++ {
		if i < len(lines) {
			emitRow(&sb, lines[i], innerW)
		} else {
			emitRow(&sb, "", innerW)
		}
	}

	// ── Separator ──
	sb.WriteString(cBorder + "├")
	sb.WriteString(strings.Repeat("─", innerW))
	sb.WriteString("┤" + cReset + "\033[K\r\n")

	// ── Help line ──
	help := " q:Back"
	if state.activePanel == panelPeers {
		help = " Up/Down:Scroll  q:Back"
	} else if state.activePanel == panelGlobe {
		help = " Up/Down:Scroll  q:Back"
	}
	emitRow(&sb, cHelp+help+cReset, innerW)

	// ── Bottom border ──
	sb.WriteString(cBorder + "└")
	sb.WriteString(strings.Repeat("─", innerW))
	sb.WriteString("┘" + cReset + "\033[K")

	return sb.String()
}

// renderSelfDetail shows extended self node information
func renderSelfDetail(pInfos []peerInfo, stats networkStats, w int, now int64) []string {
	var lines []string
	lines = append(lines, cTitle+" @ My Node -- Detailed View"+cReset)
	lines = append(lines, "")

	var self *peerInfo
	for i := range pInfos {
		if pInfos[i].isSelf {
			self = &pInfos[i]
			break
		}
	}
	if self == nil && len(pInfos) > 0 {
		self = &pInfos[0]
	}

	if self != nil {
		lines = append(lines, cSelf+" Peer ID:    "+cReset+self.peerID)
		lines = append(lines, cSelf+" Short ID:   "+cReset+self.shortID)
		if self.agentName != "" {
			lines = append(lines, cSelf+" Agent:      "+cReset+self.agentName)
		}
		loc := self.location
		if loc == "" || loc == "Unknown" {
			loc = self.country
		}
		if loc == "" {
			loc = "Unknown (run: clawnet geo-upgrade)"
		}
		lines = append(lines, cSelf+" Location:   "+cReset+loc)
		if self.city != "" {
			lines = append(lines, cSelf+" City:       "+cReset+self.city)
		}
		if self.region != "" {
			lines = append(lines, cSelf+" Region:     "+cReset+self.region)
		}
		if self.country != "" {
			lines = append(lines, cSelf+" Country:    "+cReset+self.country)
		}
		lines = append(lines, cSelf+" Coords:     "+cReset+fmt.Sprintf("%.4f, %.4f", self.lat, self.lon))
		if self.motto != "" {
			lines = append(lines, cSelf+" Motto:      "+cReset+self.motto)
		}
	}

	lines = append(lines, "")
	lines = append(lines, cTitle+" Network"+cReset)
	lines = append(lines, cSelfInfo+" Credits:    "+cReset+fmt.Sprintf("%.1f (frozen: %.1f)", stats.Balance, stats.Frozen))
	lines = append(lines, cSelfInfo+" Peers:      "+cReset+fmt.Sprintf("%d", stats.Peers))
	lines = append(lines, cSelfInfo+" Topics:     "+cReset+fmt.Sprintf("%d", len(stats.Topics)))
	if stats.StartedAt > 0 {
		lines = append(lines, cSelfInfo+" Uptime:     "+cReset+formatDuration(now-stats.StartedAt))
	}
	lines = append(lines, cSelfInfo+" Version:    "+cReset+daemon.Version)
	if stats.OverlayIPv6 != "" {
		lines = append(lines, cSelfInfo+" Claw IPv6:  "+cReset+stats.OverlayIPv6)
	}
	if stats.OverlayTUN != "" {
		moltTag := "\033[38;2;42;157;143m(ClawNet-only)\033[0m"
		if stats.OverlayMolted {
			moltTag = "\033[38;2;230;57;70m(molted)\033[0m"
		}
		lines = append(lines, cSelfInfo+" TUN:        "+cReset+stats.OverlayTUN+" "+moltTag)
	}
	if stats.OverlayPeers > 0 {
		lines = append(lines, cSelfInfo+" Overlay:    "+cReset+fmt.Sprintf("%d peers", stats.OverlayPeers))
	}

	if len(stats.Topics) > 0 {
		lines = append(lines, "")
		lines = append(lines, cTitle+" Subscribed Topics"+cReset)
		// Sort topics for stable display — prevents flickering
		sorted := make([]string, len(stats.Topics))
		copy(sorted, stats.Topics)
		sort.Strings(sorted)
		for _, t := range sorted {
			lines = append(lines, cSelfInfo+"  . "+cReset+t)
		}
	}

	return lines
}

// renderPeersDetail shows a scrollable list of all peers
func renderPeersDetail(pInfos []peerInfo, w int, now int64, state *topoState) []string {
	var peerEntries []peerInfo
	for _, pi := range pInfos {
		if !pi.isSelf {
			peerEntries = append(peerEntries, pi)
		}
	}

	var lines []string

	// If a specific peer was selected via number key, show that peer's detail
	if state.selectedPeer > 0 && state.selectedPeer <= len(peerEntries) {
		p := peerEntries[state.selectedPeer-1]
		lines = append(lines, cTitle+fmt.Sprintf(" Peer #%d Detail", state.selectedPeer)+cReset)
		lines = append(lines, "")
		lines = append(lines, cSelf+" Peer ID:    "+cReset+p.peerID)
		lines = append(lines, cSelf+" Short ID:   "+cReset+p.shortID)
		if p.agentName != "" {
			lines = append(lines, cSelf+" Agent:      "+cReset+p.agentName)
		}
		loc := p.city
		if loc == "" {
			loc = p.region
		}
		if loc == "" {
			loc = p.country
		}
		if loc == "" {
			loc = "Unknown"
		}
		lines = append(lines, cSelf+" Location:   "+cReset+loc)
		if p.city != "" {
			lines = append(lines, cSelf+" City:       "+cReset+p.city)
		}
		if p.region != "" {
			lines = append(lines, cSelf+" Region:     "+cReset+p.region)
		}
		if p.country != "" {
			lines = append(lines, cSelf+" Country:    "+cReset+p.country)
		}
		if p.lat != 0 || p.lon != 0 {
			lines = append(lines, cSelf+" Coords:     "+cReset+fmt.Sprintf("%.4f, %.4f", p.lat, p.lon))
		}
		if p.latencyMs > 0 {
			lines = append(lines, cSelf+" Latency:    "+cReset+fmt.Sprintf("%dms", p.latencyMs))
		}
		if p.connectedSince > 0 {
			lines = append(lines, cSelf+" Connected:  "+cReset+formatDuration(now-p.connectedSince))
		}
		if p.bwTotal > 0 {
			lines = append(lines, cSelf+" Traffic:    "+cReset+formatBytes(p.bwTotal))
		}
		lines = append(lines, cSelf+" Reputation: "+cReset+fmt.Sprintf("%.1f", p.reputation))
		if p.motto != "" {
			lines = append(lines, cSelf+" Motto:      "+cReset+p.motto)
		}
		return lines
	}

	lines = append(lines, cTitle+fmt.Sprintf(" Peers -- %d connected", len(peerEntries))+cReset)
	lines = append(lines, "")

	if len(peerEntries) == 0 {
		lines = append(lines, cDim+"  No peers connected"+cReset)
		return lines
	}

	// Build all peer entry lines
	var allEntries []string
	for i, p := range peerEntries {
		loc := p.city
		if loc == "" {
			loc = p.region
		}
		if loc == "" {
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

		allEntries = append(allEntries,
			fmt.Sprintf(cPeerInfo+"  %d. @%s"+cReset, i+1, p.shortID))
		if p.agentName != "" {
			allEntries = append(allEntries,
				fmt.Sprintf("     Name: "+cPeerInfo+"%s"+cReset, p.agentName))
		}
		allEntries = append(allEntries,
			fmt.Sprintf("     ID:  %s", truncStr(p.peerID, w-10)))
		allEntries = append(allEntries,
			fmt.Sprintf("     Loc: %s  Lat: %s  Up: %s", loc, latStr, upStr))
		if p.lat != 0 || p.lon != 0 {
			allEntries = append(allEntries,
				fmt.Sprintf("     Coord: %.4f, %.4f", p.lat, p.lon))
		}
		if p.motto != "" {
			allEntries = append(allEntries,
				fmt.Sprintf("     Motto: "+cDim+"%s"+cReset, truncStr(p.motto, w-14)))
		}
		allEntries = append(allEntries, "")
	}

	// Apply scroll offset
	if state.peerScrollOff > len(allEntries)-1 {
		state.peerScrollOff = len(allEntries) - 1
	}
	if state.peerScrollOff < 0 {
		state.peerScrollOff = 0
	}
	visible := allEntries[state.peerScrollOff:]
	lines = append(lines, visible...)

	return lines
}

// renderClawNetStats shows a neofetch-style page with ASCII banner + stats + sysinfo + activity feed
func renderClawNetStats(pInfos []peerInfo, stats networkStats, w int, now int64, feed []activityEvent, state *topoState) []string {
	var lines []string

	// Count peers by location
	cityMap := make(map[string]int)
	countryMap := make(map[string]int)
	totalPeers := 0
	for _, p := range pInfos {
		if !p.isSelf {
			totalPeers++
		}
		if p.city != "" {
			cityMap[p.city]++
		}
		if p.country != "" {
			countryMap[p.country]++
		}
	}

	// Build stat labels + values (right side of banner)
	type kv struct{ k, v string }
	statsLines := []kv{
		{"", cTitle + "ClawNet" + cReset + " " + cDim + "v" + daemon.Version + cReset},
		{"", strings.Repeat("-", 30)},
		{"Nodes", fmt.Sprintf("%d total (%d peers + self)", totalPeers+1, totalPeers)},
		{"Credits", fmt.Sprintf("%.1f (frozen: %.1f)", stats.Balance, stats.Frozen)},
		{"Topics", fmt.Sprintf("%d subscribed", len(stats.Topics))},
	}
	if stats.StartedAt > 0 {
		statsLines = append(statsLines, kv{"Uptime", formatDuration(now - stats.StartedAt)})
	}
	if stats.PeerID != "" {
		statsLines = append(statsLines, kv{"Peer ID", truncStr(stats.PeerID, w-75)})
	}
	statsLines = append(statsLines, kv{"Protocol", "libp2p + GossipSub + Kademlia DHT"})
	statsLines = append(statsLines, kv{"Transport", "TCP + QUIC-v1, Noise encryption"})
	statsLines = append(statsLines, kv{"Storage", "SQLite + FTS5 (BM25)"})

	// System info
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	statsLines = append(statsLines, kv{"", ""})
	statsLines = append(statsLines, kv{"Goroutines", fmt.Sprintf("%d", runtime.NumGoroutine())})
	statsLines = append(statsLines, kv{"Mem Alloc", fmt.Sprintf("%.1f MB", float64(ms.Alloc)/1048576)})
	statsLines = append(statsLines, kv{"Mem Sys", fmt.Sprintf("%.1f MB", float64(ms.Sys)/1048576)})
	statsLines = append(statsLines, kv{"GC Cycles", fmt.Sprintf("%d", ms.NumGC)})

	if len(countryMap) > 0 {
		statsLines = append(statsLines, kv{"", ""})
		// Sort countries for deterministic output
		var countries []string
		for c := range countryMap {
			countries = append(countries, c)
		}
		sort.Strings(countries)
		countryStr := ""
		for i, c := range countries {
			if i > 0 {
				countryStr += ", "
			}
			countryStr += fmt.Sprintf("%s(%d)", c, countryMap[c])
		}
		statsLines = append(statsLines, kv{"Countries", countryStr})
	}
	if len(cityMap) > 0 {
		var cities []string
		for c := range cityMap {
			cities = append(cities, c)
		}
		sort.Strings(cities)
		cityStr := ""
		for i, c := range cities {
			if i > 0 {
				cityStr += ", "
			}
			cityStr += fmt.Sprintf("%s(%d)", c, cityMap[c])
		}
		statsLines = append(statsLines, kv{"Cities", truncStr(cityStr, w-75)})
	}

	// Render neofetch-style: banner on left, stats on right
	bannerW := 0
	for _, l := range clawnetBanner {
		if len([]rune(l)) > bannerW {
			bannerW = len([]rune(l))
		}
	}
	gap := 4
	showBanner := w >= bannerW+gap+20 // hide banner if too narrow

	maxRows := len(statsLines)
	if showBanner && len(clawnetBanner) > maxRows {
		maxRows = len(clawnetBanner)
	}

	lines = append(lines, "")
	for i := 0; i < maxRows; i++ {
		if showBanner {
			var left string
			if i < len(clawnetBanner) {
				left = clawnetBanner[i]
			}
			leftR := []rune(left)
			padN := bannerW - len(leftR)
			if padN < 0 {
				padN = 0
			}
			leftPadded := cBanner + string(leftR) + cReset + strings.Repeat(" ", padN)
			gapStr := strings.Repeat(" ", gap)

			var right string
			if i < len(statsLines) {
				s := statsLines[i]
				if s.k != "" {
					right = cSelf + s.k + cReset + ": " + s.v
				} else {
					right = s.v
				}
			}
			lines = append(lines, " "+leftPadded+gapStr+right)
		} else {
			if i < len(statsLines) {
				s := statsLines[i]
				if s.k != "" {
					lines = append(lines, "  "+cSelf+s.k+cReset+": "+s.v)
				} else if s.v != "" {
					lines = append(lines, "  "+s.v)
				} else {
					lines = append(lines, "")
				}
			}
		}
	}

	// Reputation legend
	lines = append(lines, "")
	lines = append(lines, " "+cTitle+"Reputation"+cReset)
	lines = append(lines,
		"   "+repColor(20)+"*"+cReset+" <30  "+
			repColor(40)+"*"+cReset+" 30-49  "+
			repColor(60)+"*"+cReset+" 50-69  "+
			repColor(80)+"*"+cReset+" 70-99  "+
			repColor(120)+"*"+cReset+" 100+")

	// ── Activity Feed ──
	if len(feed) > 0 {
		lines = append(lines, "")
		lines = append(lines, " "+cTitle+"Activity Feed"+cReset+" "+cDim+"(Up/Down to scroll)"+cReset)
		lines = append(lines, "")
		var feedLines []string
		for _, ev := range feed {
			icon := "·"
			switch ev.Type {
			case "credit":
				icon = "$"
			case "knowledge":
				icon = "K"
			case "task":
				icon = "T"
			}
			ts := ev.Time
			if len(ts) > 16 {
				ts = ts[5:16] // trim to "MM-DD HH:MM"
			}
			feedLines = append(feedLines,
				fmt.Sprintf("  %s %s %s", cDim+ts+cReset, cSelf+icon+cReset, truncStr(ev.Detail, w-22)))
		}
		if state.feedScrollOff > len(feedLines)-1 {
			state.feedScrollOff = len(feedLines) - 1
		}
		if state.feedScrollOff < 0 {
			state.feedScrollOff = 0
		}
		lines = append(lines, feedLines[state.feedScrollOff:]...)
	}

	return lines
}

// ── Helper: build self info lines for bottom panel ──

func buildSelfLines(pInfos []peerInfo, stats networkStats, selfW, bottomH int) []string {
	selfLines := make([]string, bottomH)
	var selfPeer *peerInfo
	for i := range pInfos {
		if pInfos[i].isSelf {
			selfPeer = &pInfos[i]
			break
		}
	}
	if selfPeer == nil && len(pInfos) > 0 {
		selfPeer = &pInfos[0]
	}

	now := time.Now().Unix()
	if selfPeer != nil {
		insW := selfW - 2
		lines := []string{
			fmt.Sprintf("@ %s", truncStr(selfPeer.shortID, insW-2)),
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
				vw := visibleLen(txt)
				if vw > selfW {
					txt = truncToWidth(txt, selfW)
					vw = visibleLen(txt)
				}
				selfLines[i] = txt + strings.Repeat(" ", selfW-vw)
			} else {
				selfLines[i] = strings.Repeat(" ", selfW)
			}
		}
	} else {
		for i := range selfLines {
			selfLines[i] = strings.Repeat(" ", selfW)
		}
	}
	return selfLines
}

// ── Helper: build peer card lines for bottom panel ──

func buildPeerLines(pInfos []peerInfo, peerW, bottomH int) []string {
	now := time.Now().Unix()

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

	var peerEntries []peerInfo
	for _, pi := range pInfos {
		if !pi.isSelf {
			peerEntries = append(peerEntries, pi)
		}
	}

	peerLines := make([]string, bottomH)
	for i := range peerLines {
		peerLines[i] = strings.Repeat(" ", peerW)
	}

	if len(peerEntries) > 0 {
		maxCards := cols * cardRows
		if maxCards > len(peerEntries) {
			maxCards = len(peerEntries)
		}

		type cardData struct {
			lines [4]string
		}
		allCards := make([]cardData, maxCards)
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

			// Use ASCII box chars to avoid CJK width issues
			allCards[ci].lines[0] = "+" + strings.Repeat("-", insW) + "+"

			idLine := " @" + truncStr(p.shortID, insW-2)
			pad := insW - len([]rune(idLine))
			if pad < 0 {
				pad = 0
			}
			allCards[ci].lines[1] = "|" + idLine + strings.Repeat(" ", pad) + "|"

			infoLine := " " + truncStr(loc, insW-1)
			pad = insW - len([]rune(infoLine))
			if pad < 0 {
				pad = 0
			}
			allCards[ci].lines[2] = "|" + infoLine + strings.Repeat(" ", pad) + "|"

			statLine := fmt.Sprintf(" %s %s", latStr, upStr)
			statLine = truncStr(statLine, insW)
			pad = insW - len([]rune(statLine))
			if pad < 0 {
				pad = 0
			}
			allCards[ci].lines[3] = "+" + statLine + strings.Repeat("-", pad) + "+"
		}

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

	return peerLines
}

func cmdPublish() error {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "usage: clawnet publish <topic> <message>\n")
		fmt.Fprintf(os.Stderr, "  e.g. clawnet publish /clawnet/global \"hello world\"\n")
		return nil
	}
	topic := url.PathEscape(os.Args[2])
	msg := strings.Join(os.Args[3:], " ")
	// Auto-join the topic first (ignore errors — may already be joined)
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)
	http.Post(base+"/api/topics/"+topic+"/join", "application/json", nil)
	return apiPost("/api/topics/"+topic+"/messages", map[string]string{"body": msg})
}

func cmdSub() error {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: clawnet sub <topic>\n")
		fmt.Fprintf(os.Stderr, "  e.g. clawnet sub /clawnet/global\n")
		return nil
	}
	topic := url.PathEscape(os.Args[2])
	displayTopic := os.Args[2]
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)

	// First, join the topic
	http.Post(base+"/api/topics/"+topic+"/join", "application/json", nil)

	// Print last 10 messages
	resp, err := http.Get(base + "/api/topics/" + topic + "/messages?limit=10")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	var msgs []struct {
		AuthorID   string `json:"author_id"`
		AuthorName string `json:"author_name"`
		Body       string `json:"body"`
		CreatedAt  string `json:"created_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&msgs); err != nil {
		return err
	}

	coral := "\033[38;2;247;127;0m"
	dim := "\033[2m"
	rst := "\033[0m"

	fmt.Printf("%ssubscribed to %s%s (last %d messages)\n\n", coral, displayTopic, rst, len(msgs))
	for _, m := range msgs {
		ts := m.CreatedAt
		if len(ts) > 19 {
			ts = ts[11:19]
		}
		from := m.AuthorName
		if from == "" {
			from = m.AuthorID
		}
		if len(from) > 16 {
			from = from[:16]
		}
		fmt.Printf("%s%s%s %s%s%s: %s\n", dim, ts, rst, coral, from, rst, m.Body)
	}

	// Poll for new messages
	fmt.Printf("\n%slistening... (Ctrl+C to stop)%s\n\n", dim, rst)
	seen := len(msgs)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-sigCh:
			fmt.Println()
			return nil
		case <-ticker.C:
			resp, err := http.Get(base + "/api/topics/" + topic + "/messages?limit=50")
			if err != nil {
				continue
			}
			var allMsgs []struct {
				AuthorID   string `json:"author_id"`
				AuthorName string `json:"author_name"`
				Body       string `json:"body"`
				CreatedAt  string `json:"created_at"`
			}
			json.NewDecoder(resp.Body).Decode(&allMsgs)
			resp.Body.Close()
			if len(allMsgs) > seen {
				for _, m := range allMsgs[seen:] {
					ts := m.CreatedAt
					if len(ts) > 19 {
						ts = ts[11:19]
					}
					from := m.AuthorName
					if from == "" {
						from = m.AuthorID
					}
					if len(from) > 16 {
						from = from[:16]
					}
					fmt.Printf("%s%s%s %s%s%s: %s\n", dim, ts, rst, coral, from, rst, m.Body)
				}
				seen = len(allMsgs)
			}
		}
	}
}

func apiPost(path string, body any) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("http://127.0.0.1:%d%s", cfg.WebUIPort, path)
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json", strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("cannot connect to daemon (is it running?): %w", err)
	}
	defer resp.Body.Close()
	resBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(resBody))
	}
	var out any
	if err := json.Unmarshal(resBody, &out); err != nil {
		fmt.Println(string(resBody))
		return nil
	}
	pretty, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(pretty))
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
