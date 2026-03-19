package cli

import (
	"bufio"
	"encoding/json"
	"errors"
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
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/i18n"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/identity"
	"golang.org/x/term"
)

// ── Random tips shown on usage and status ──
const tipCount = 14

func randomTip() string {
	key := fmt.Sprintf("tip.%d", rand.Intn(tipCount))
	return i18n.T(key)
}

// Verbose controls extra output when -v/--verbose is passed.
var Verbose bool

// JSONOutput controls --json machine-readable output.
var JSONOutput bool

// IsTTY is true when stdout is an interactive terminal.
var IsTTY bool

// noColor returns true when ANSI color should be suppressed (non-TTY or --json).
func noColor() bool { return !IsTTY || JSONOutput }

// c returns the ANSI code if a TTY, empty string otherwise.
// Use: c(dim) + "text" + c(rst)
func c(code string) string {
	if noColor() {
		return ""
	}
	return code
}

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
	// East-Asian ambiguous-width: only include code points widely agreed to be
	// double-width. Geometric shapes (25xx) and card suits (266x) are single-width
	// in most modern terminal emulators; treat them as 1.
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

// safePrefix returns s[:n] if len(s) >= n, otherwise s unchanged.
func safePrefix(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s
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
	// Internal --daemon flag: run daemon in foreground (used by background launcher).
	// This path is invoked by startDaemonBackground() — output goes to log file.
	daemonMode := false
	for _, a := range os.Args[1:] {
		if a == "--daemon" {
			daemonMode = true
		}
		if a == "--no-ui" {
			os.Setenv("CLAWNET_WEBUI_ENABLED", "false")
		}
		if strings.HasPrefix(a, "--webui-dir=") {
			os.Setenv("CLAWNET_WEBUI_DIR", strings.TrimPrefix(a, "--webui-dir="))
		}
		if devBuild && strings.HasPrefix(a, "--dev-layers=") {
			devLayers = strings.Split(strings.TrimPrefix(a, "--dev-layers="), ",")
		}
	}
	if daemonMode {
		return daemon.Start(true, devLayers)
	}

	// Auto-detect language from IP geolocation
	i18n.Init(config.DataDir())

	if len(os.Args) < 2 {
		return printUsageVerbose(false)
	}

	// Global flags: strip -h/--help and -v/--verbose from args
	verbose := false
	jsonOut := false
	filtered := []string{os.Args[0]}
	showHelp := false
	for _, a := range os.Args[1:] {
		switch a {
		case "-v", "--verbose":
			verbose = true
		case "-h", "--help":
			showHelp = true
		case "--json":
			jsonOut = true
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
	JSONOutput = jsonOut
	IsTTY = term.IsTerminal(int(os.Stdout.Fd()))

	// No subcommand after flag stripping
	if len(os.Args) < 2 {
		if showHelp {
			return printUsageVerbose(verbose)
		}
		return printUsageVerbose(false)
	}

	cmd := os.Args[1]

	// Auto-init + auto-start daemon for commands that need it
	if needsDaemon(cmd) {
		if _, err := ensureDaemon(); err != nil {
			return err
		}
	}

	// If -h/--help was passed with a subcommand, show subcommand help
	if showHelp && cmd != "help" {
		return printCmdHelp(cmd)
	}

	// "clawnet <cmd> help" → show that command's help (universal subcommand)
	if len(os.Args) > 2 && os.Args[len(os.Args)-1] == "help" && cmd != "help" {
		return printCmdHelp(cmd)
	}

	switch cmd {
	case "i", "init":
		return cmdInit()
	case "up", "start":
		return cmdStart()
	case "down", "stop":
		return cmdStop()
	case "restart":
		return cmdRestart()
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
	case "t", "task":
		return cmdTask()
	case "b", "board":
		return cmdBoard()
	case "log", "logs":
		return cmdLog()
	case "molt":
		return cmdMolt()
	case "unmolt":
		return cmdUnmolt()
	case "w", "watch":
		return cmdWatch()
	case "role":
		return cmdRole()
	case "swarm":
		return cmdSwarm()
	case "credits", "credit":
		return cmdCredits()
	case "predict", "prediction", "oracle":
		return cmdPredict()
	case "knowledge", "know", "kb":
		return cmdKnowledge()
	case "search":
		return cmdSearch()
	case "get":
		return cmdGet()
	case "annotate":
		return cmdAnnotate()
	case "resume":
		return cmdResume()
	case "skill":
		return cmdSkill()
	case "v", "version":
		fmt.Printf("clawnet v%s\n", daemon.Version)
		return nil
	case "help":
		if len(os.Args) > 2 {
			return printCmdHelp(os.Args[2])
		}
		return printUsageVerbose(verbose)
	default:
		fmt.Fprintln(os.Stderr, i18n.Tf("err.unknown_cmd", cmd))
		return printUsageVerbose(false)
	}
}

func printUsage() error {
	return printUsageVerbose(false)
}

func printUsageVerbose(verbose bool) error {
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
	fmt.Println(coral + "  " + i18n.T("tagline") + rst)
	fmt.Println(dim + "  https://github.com/ChatChatTech/ClawNet  v" + daemon.Version + rst)
	fmt.Println()
	fmt.Println(bold + i18n.T("usage") + rst)
	fmt.Println(tidal + "  clawnet <command> [flags]" + rst)
	fmt.Println()

	// ── Primary commands (always shown) ──
	fmt.Println(bold + i18n.T("commands") + rst)
	fmt.Println(tidal+"  start    "+dim+"(up)     "+rst + i18n.T("cmd.start"))
	fmt.Println(tidal+"  stop     "+dim+"(down)   "+rst + i18n.T("cmd.stop"))
	fmt.Println(tidal+"  restart  "+dim+"         "+rst + i18n.T("cmd.restart"))
	fmt.Println(tidal+"  status   "+dim+"(s)      "+rst + i18n.T("cmd.status"))
	fmt.Println(tidal+"  board    "+dim+"(b)      "+rst + i18n.T("cmd.board"))
	fmt.Println(tidal+"  task     "+dim+"(t)      "+rst + i18n.T("cmd.task"))
	fmt.Println(tidal+"  topo     "+dim+"(map)    "+rst + i18n.T("cmd.topo"))
	fmt.Println(tidal+"  peers    "+dim+"(p)      "+rst + i18n.T("cmd.peers"))
	fmt.Println(tidal+"  chat     "+dim+"         "+rst + i18n.T("cmd.chat"))
	fmt.Println(tidal+"  watch    "+dim+"(w)      "+rst + i18n.T("cmd.watch"))
	fmt.Println(tidal+"  role     "+dim+"         "+rst + i18n.T("cmd.role"))
	fmt.Println(tidal+"  swarm    "+dim+"         "+rst + i18n.T("cmd.swarm"))
	fmt.Println(tidal+"  credits  "+dim+"         "+rst + i18n.T("cmd.credits"))
	fmt.Println(tidal+"  predict  "+dim+"         "+rst + i18n.T("cmd.predict"))
	fmt.Println(tidal+"  knowledge"+dim+"         "+rst + i18n.T("cmd.knowledge"))
	fmt.Println(tidal+"  search   "+dim+"         "+rst + i18n.T("cmd.search"))
	fmt.Println(tidal+"  get      "+dim+"         "+rst + i18n.T("cmd.get"))
	fmt.Println(tidal+"  annotate "+dim+"         "+rst + i18n.T("cmd.annotate"))
	fmt.Println(tidal+"  resume   "+dim+"         "+rst + i18n.T("cmd.resume"))

	if verbose {
		// ── Extended commands ──
		fmt.Println()
		fmt.Println(bold + i18n.T("setup_maintenance") + rst)
		fmt.Println(tidal+"  init     "+dim+"(i)      "+rst + i18n.T("cmd.init"))
		fmt.Println(tidal+"  update   "+dim+"         "+rst + i18n.T("cmd.update"))
		fmt.Println(tidal+"  doctor   "+dim+"(doc)    "+rst + i18n.T("cmd.doctor"))
		fmt.Println(tidal+"  nutshell "+dim+"(nut)    "+rst + i18n.T("cmd.nutshell"))
		fmt.Println(tidal+"  geo-upgrade"+dim+"       "+rst + i18n.T("cmd.geo_upgrade"))
		fmt.Println(tidal+"  log      "+dim+"(logs)   "+rst + i18n.T("cmd.log"))
		fmt.Println(tidal+"  version  "+dim+"(v)      "+rst + i18n.T("cmd.version"))
		fmt.Println()
		fmt.Println(bold + i18n.T("messaging_data") + rst)
		fmt.Println(tidal+"  publish  "+dim+"(pub)    "+rst + i18n.T("cmd.publish"))
		fmt.Println(tidal+"  sub      "+dim+"         "+rst + i18n.T("cmd.sub"))
		fmt.Println()
		fmt.Println(bold + i18n.T("identity_overlay") + rst)
		fmt.Println(tidal+"  export   "+dim+"         "+rst + i18n.T("cmd.export"))
		fmt.Println(tidal+"  import   "+dim+"         "+rst + i18n.T("cmd.import"))
		fmt.Println(tidal+"  molt     "+dim+"         "+rst + i18n.T("cmd.molt"))
		fmt.Println(tidal+"  unmolt   "+dim+"         "+rst + i18n.T("cmd.unmolt"))
		fmt.Println(tidal+"  nuke     "+dim+"         "+rst + i18n.T("cmd.nuke"))
		fmt.Println()
		fmt.Println(bold + i18n.T("ai_integration") + rst)
		fmt.Println(tidal+"  skill    "+dim+"         "+rst + i18n.T("cmd.skill"))
	}

	fmt.Println()
	if devBuild {
		fmt.Println(dim + "  " + i18n.T("flags") + ": -v/--verbose  -h/--help  --json  --dev-layers=layer1,layer2,..." + rst)
		fmt.Println(dim + "  DEV LAYERS: stun, mdns, dht, bt-dht, bootstrap, relay, overlay, k8s" + rst)
	} else {
		fmt.Println(dim + "  " + i18n.T("flags") + ": -v/--verbose  -h/--help  --json" + rst)
	}
	fmt.Println(dim + "  " + i18n.T("hint.cmd_help") + rst)
	if !verbose {
		fmt.Println(dim + "  " + i18n.T("hint.verbose") + rst)
	}
	fmt.Println(dim + "  " + i18n.T("hint.json") + rst)
	fmt.Println(dim + "  " + i18n.T("hint.api") + rst)
	fmt.Println()
	fmt.Println(dim + "  " + i18n.T("hint.skill") + rst)
	fmt.Println()
	fmt.Println(dim + "  " + i18n.T("tip_prefix") + rst + randomTip())
	return nil
}

func getCmdHelps() map[string]string {
	return map[string]string{
		"init":        i18n.T("cmdhelp.init"),
		"start":       i18n.T("cmdhelp.start"),
		"stop":        i18n.T("cmdhelp.stop"),
		"restart":     i18n.T("cmdhelp.restart"),
		"status":      i18n.T("cmdhelp.status"),
		"peers":       i18n.T("cmdhelp.peers"),
		"topo":        i18n.T("cmdhelp.topo"),
		"board":       i18n.T("cmdhelp.board"),
		"chat":        i18n.T("cmdhelp.chat"),
		"watch":       i18n.T("cmdhelp.watch"),
		"role":        i18n.T("cmdhelp.role"),
		"log":         i18n.T("cmdhelp.log"),
		"publish":     i18n.T("cmdhelp.publish"),
		"sub":         i18n.T("cmdhelp.sub"),
		"export":      i18n.T("cmdhelp.export"),
		"import":      i18n.T("cmdhelp.import"),
		"nuke":        i18n.T("cmdhelp.nuke"),
		"doctor":      i18n.T("cmdhelp.doctor"),
		"update":      i18n.T("cmdhelp.update"),
		"nutshell":    i18n.T("cmdhelp.nutshell"),
		"geo-upgrade": i18n.T("cmdhelp.geo-upgrade"),
		"molt":        i18n.T("cmdhelp.molt"),
		"unmolt":      i18n.T("cmdhelp.unmolt"),
		"swarm":       i18n.T("cmdhelp.swarm"),
		"skill":       i18n.T("cmdhelp.skill"),
		"version":     i18n.T("cmdhelp.version"),
		"task":        i18n.T("cmdhelp.task"),
		"credits":     i18n.T("cmdhelp.credits"),
		"predict":     i18n.T("cmdhelp.predict"),
		"knowledge":   i18n.T("cmdhelp.knowledge"),
		"resume":      i18n.T("cmdhelp.resume"),
		"help":        i18n.T("cmdhelp.help"),
	}
}

func printCmdHelp(cmd string) error {
	// Resolve alias to canonical name
	aliases := map[string]string{
		"i": "init", "up": "start", "down": "stop",
		"s": "status", "st": "status", "p": "peers",
		"map": "topo", "pub": "publish", "v": "version",
		"doc": "doctor", "nut": "nutshell", "b": "board",
		"logs": "log", "w": "watch", "t": "task",
		"credit": "credits", "oracle": "predict", "prediction": "predict",
		"know": "knowledge", "kb": "knowledge",
	}
	if canonical, ok := aliases[cmd]; ok {
		cmd = canonical
	}
	// Board has its own rich help
	if cmd == "board" {
		boardHelp()
		return nil
	}
	// New commands with their own verbose help
	switch cmd {
	case "task":
		taskHelp(Verbose)
		return nil
	case "credits":
		creditsHelp(Verbose)
		return nil
	case "predict":
		predictHelp(Verbose)
		return nil
	case "knowledge":
		knowledgeHelp(Verbose)
		return nil
	case "search":
		fmt.Println("clawnet search [query] [options]")
		fmt.Println()
		fmt.Println("  Shortcut for: clawnet knowledge search")
		fmt.Println("  Searches Knowledge Mesh including Context Hub docs.")
		fmt.Println()
		fmt.Println("  Options:")
		fmt.Println("    --tags <tags>      Filter by tags (comma-separated)")
		fmt.Println("    --lang <language>  Filter by language (py, js, ts, go, rb, etc.)")
		fmt.Println("    --limit <n>        Max results (default: 20)")
		fmt.Println()
		fmt.Println("  Examples:")
		fmt.Println("    clawnet search openai")
		fmt.Println("    clawnet search openai --lang py")
		fmt.Println("    clawnet search --tags openai --limit 5")
		fmt.Println("    clawnet search \"stripe payments\"")
		return nil
	case "get":
		return cmdGet() // shows help when called with no args
	case "annotate":
		return cmdAnnotate() // shows help when called with no args
	case "resume":
		resumeHelp(Verbose)
		return nil
	case "swarm":
		swarmHelp(Verbose)
		return nil
	}
	if help, ok := getCmdHelps()[cmd]; ok {
		fmt.Println(help)
		return nil
	}
	fmt.Fprintln(os.Stderr, i18n.Tf("err.unknown_cmd", cmd))
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
		fmt.Println(i18n.Tf("init.created_config", cfgPath))
	} else {
		fmt.Println(i18n.Tf("init.config_exists", cfgPath))
	}

	fmt.Println(i18n.Tf("init.data_dir", dataDir))
	fmt.Println(i18n.Tf("init.peer_id", peerID.String()))
	fmt.Println(i18n.T("init.complete"))
	return nil
}

func cmdStart() error {
	if isDaemonRunning() {
		fmt.Println(i18n.T("start.already"))
		return cmdStatus()
	}
	// Start daemon in background with log file
	_, err := ensureDaemon()
	return err
}

func cmdStop() error {
	dataDir := config.DataDir()
	pidPath := filepath.Join(dataDir, "daemon.pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return errors.New(i18n.T("stop.not_found"))
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
	fmt.Println(i18n.Tf("stop.sent", pid))
	return nil
}

func cmdRestart() error {
	// Read the old PID before stopping so we can wait for the actual
	// process to die — isDaemonRunning() is unreliable here because
	// it returns false as soon as the API stops responding, even though
	// the process may still be alive and holding the port.
	dataDir := config.DataDir()
	pidData, _ := os.ReadFile(filepath.Join(dataDir, "daemon.pid"))
	oldPid, _ := strconv.Atoi(strings.TrimSpace(string(pidData)))

	if oldPid > 0 && isDaemonRunning() {
		if err := cmdStop(); err != nil {
			return err
		}
		// Wait for process to actually terminate (up to 10 seconds)
		for i := 0; i < 100; i++ {
			time.Sleep(100 * time.Millisecond)
			proc, err := os.FindProcess(oldPid)
			if err != nil {
				break
			}
			if proc.Signal(syscall.Signal(0)) != nil {
				break // process is dead
			}
		}
		// Remove stale PID file so new daemon writes a fresh one
		os.Remove(filepath.Join(dataDir, "daemon.pid"))
	}
	return cmdStart()
}

func cmdSkill() error {
	// Look for SKILL.md in several locations
	candidates := []string{
		"SKILL.md",
		filepath.Join(config.DataDir(), "SKILL.md"),
	}
	// Also check next to and above the binary
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append([]string{
			filepath.Join(dir, "SKILL.md"),
			filepath.Join(dir, "..", "SKILL.md"),
		}, candidates...)
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil {
			fmt.Print(string(data))
			return nil
		}
	}
	// Fallback: try to fetch from the daemon API
	cfg, _ := config.Load()
	if cfg != nil {
		base := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)
		if resp, err := http.Get(base + "/api/skill"); err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				if data, err := io.ReadAll(resp.Body); err == nil && len(data) > 0 {
					fmt.Print(string(data))
					return nil
				}
			}
		}
	}
	return fmt.Errorf("SKILL.md not found. Place it next to the clawnet binary or in %s", config.DataDir())
}

func cmdStatus() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)

	// Fetch status
	resp, err := http.Get(base + "/api/status")
	if err != nil {
		return fmt.Errorf(i18n.T("err.daemon_connect")+": %w", err)
	}
	defer resp.Body.Close()

	// --json: pass through raw API JSON
	if JSONOutput {
		body, _ := io.ReadAll(resp.Body)
		fmt.Println(string(body))
		return nil
	}

	var status map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return err
	}

	red := "\033[38;2;230;57;70m"
	tidal := "\033[38;2;69;123;157m"
	green := "\033[32m"
	dim := "\033[2m"
	rst := "\033[0m"
	bold := "\033[1m"

	fmt.Println(red + "  " + i18n.T("status.title") + rst)
	fmt.Println()

	// Identity
	if id, ok := status["peer_id"].(string); ok && len(id) >= 16 {
		fmt.Printf(tidal+"  %-14s"+rst+"%s…\n", i18n.T("status.peer_id"), id[:16])
	}
	if v, ok := status["version"].(string); ok {
		fmt.Printf(tidal+"  %-14s"+rst+"%s\n", i18n.T("status.version"), v)
	}
	if peers, ok := status["peers"].(float64); ok {
		fmt.Printf(tidal+"  %-14s"+rst+i18n.Tf("status.peers_fmt", int(peers))+"\n", i18n.T("status.peers"))
	}
	if role, ok := status["role"].(string); ok && role != "" {
		fmt.Printf(tidal+"  %-14s"+rst+"%s\n", i18n.T("status.role"), role)
	}
	fmt.Println()

	// Today's activity digest
	digestResp, _ := http.Get(base + "/api/digest")
	if digestResp != nil {
		var digest struct {
			Summary string `json:"summary"`
		}
		json.NewDecoder(digestResp.Body).Decode(&digest)
		digestResp.Body.Close()
		if digest.Summary != "" {
			fmt.Printf(bold+"  %-14s"+rst+"%s\n", i18n.T("status.today"), digest.Summary)
		}
	}

	// Next action
	if na, ok := status["next_action"].(map[string]any); ok {
		if hint, ok := na["hint"].(string); ok && hint != "" {
			cmd, _ := na["command"].(string)
			if cmd != "" {
				fmt.Printf(green+"  %-14s"+rst+"%s → %s%s%s\n", i18n.T("status.next"), hint, dim, cmd, rst)
			} else {
				fmt.Printf(green+"  %-14s"+rst+"%s\n", i18n.T("status.next"), hint)
			}
		}
	}

	// Pending offline ops
	if pc, ok := status["pending_ops"].(float64); ok && pc > 0 {
		fmt.Printf("\033[33m  %-14s"+rst+"%s\n", i18n.T("status.offline_queue"), i18n.Tf("status.offline_fmt", int(pc)))
	}

	// Zero-balance hint
	if bal, ok := status["balance"].(float64); ok && bal == 0 {
		fmt.Println()
		fmt.Println("\033[33m  " + i18n.T("status.zero_balance") + rst + " " + i18n.T("status.zero_hint"))
		fmt.Println(dim + "     • " + i18n.T("status.zero_opt1"))
		fmt.Println("     • " + i18n.T("status.zero_opt2"))
		fmt.Println("     • " + i18n.T("status.zero_opt3"))
		fmt.Println("     • " + i18n.T("status.zero_opt4") + rst)
	}

	fmt.Println()
	fmt.Println(dim + "  " + i18n.T("tip_prefix") + rst + randomTip())
	return nil
}

// ── Onboarding role choices ──
var onboardRoles = []struct {
	id, icon string
}{
	{"worker", "🔧"},
	{"publisher", "📢"},
	{"thinker", "🧠"},
	{"trader", "🏛️"},
	{"observer", "👀"},
	{"lobster", "🦞"},
}

func roleName(id string) string  { return i18n.T("role." + id) }
func roleDesc(id string) string  { return i18n.T("role." + id + "_desc") }

// 5 futuristic welcome messages for cinematic finish
func welcomeMsg(idx int) string {
	return i18n.T(fmt.Sprintf("welcome.%d", idx))
}

func cmdRole() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)

	// clawnet role set [name] — interactive onboarding or direct set
	if len(os.Args) >= 3 && os.Args[2] == "set" {
		if len(os.Args) >= 4 {
			roleArg := os.Args[3]
			if err := callSetRole(base, roleArg); err != nil {
				return err
			}
			fmt.Print("\033[32m  " + i18n.Tf("role.set_ok", roleArg) + "\n\033[0m")
			return nil
		}
		return roleOnboarding(base)
	}

	// clawnet role — show current + available
	red := "\033[38;2;230;57;70m"
	tidal := "\033[38;2;69;123;157m"
	green := "\033[32m"
	dim := "\033[2m"
	rst := "\033[0m"

	resp, err := http.Get(base + "/api/role")
	if err != nil {
		return fmt.Errorf(i18n.T("err.daemon_connect")+": %w", err)
	}
	defer resp.Body.Close()
	var data struct {
		Role *struct {
			ID   string `json:"id"`
			Icon string `json:"icon"`
			Name string `json:"name"`
			Desc string `json:"description"`
		} `json:"role"`
	}
	json.NewDecoder(resp.Body).Decode(&data)

	fmt.Println(red + "  " + i18n.T("role.title") + rst)
	fmt.Println()
	if data.Role != nil {
		fmt.Printf(tidal+"  %-14s"+rst+"%s %s — %s\n", i18n.T("role.current"), data.Role.Icon, data.Role.Name, data.Role.Desc)
	} else {
		fmt.Println(dim + "  " + i18n.T("role.none") + rst)
	}
	fmt.Println()

	rolesResp, err := http.Get(base + "/api/roles")
	if err == nil {
		var roles []struct {
			ID          string   `json:"id"`
			Icon        string   `json:"icon"`
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Cmds        []string `json:"suggested_commands"`
		}
		json.NewDecoder(rolesResp.Body).Decode(&roles)
		rolesResp.Body.Close()

		fmt.Println("  " + i18n.T("role.available"))
		for _, r := range roles {
			marker := "  "
			if data.Role != nil && data.Role.ID == r.ID {
				marker = green + "► " + rst
			}
			fmt.Printf("  %s%s %-10s %s%s%s\n", marker, r.Icon, r.Name, dim, r.Description, rst)
		}
		fmt.Println()
		fmt.Println(dim + "  " + i18n.T("role.set_hint") + rst)
	}
	return nil
}

// callSetRole sends PUT /api/role to the daemon.
func callSetRole(base, roleID string) error {
	payload, _ := json.Marshal(map[string]string{"role": roleID})
	req, _ := http.NewRequest("PUT", base+"/api/role", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf(i18n.T("err.daemon_connect")+": %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var result map[string]any
		json.NewDecoder(resp.Body).Decode(&result)
		if e, ok := result["message"].(string); ok {
			return fmt.Errorf("%s", e)
		}
		return fmt.Errorf("failed to set role")
	}
	return nil
}

// roleOnboarding runs the interactive step-by-step identity setup.
// Uses a BIOS-style full-screen TUI with raw mode keyboard input.
func roleOnboarding(base string) error {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("cannot enter raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	outFd := int(os.Stdout.Fd())
	w, h, _ := term.GetSize(outFd)
	if w < 40 {
		w = 80
	}
	if h < 12 {
		h = 24
	}
	innerW := w - 2

	// Enter alt screen, hide cursor
	fmt.Print("\033[?1049h\033[?25l\033[40m")
	defer fmt.Print("\033[?25h\033[?1049l\033[0m")

	// ── Helper: draw BIOS frame ──
	drawFrame := func(title string) {
		fmt.Print("\033[2J\033[1;1H")
		// Top border with title
		tLen := len([]rune(title))
		dashL := 2
		dashR := innerW - dashL - tLen - 2
		if dashR < 0 {
			dashR = 0
		}
		fmt.Printf("%s┌%s %s%s%s %s%s┐%s\r\n",
			cBorder, strings.Repeat("─", dashL), cTitle, title, cReset,
			cBorder, strings.Repeat("─", dashR), cReset)
		for row := 2; row <= h-1; row++ {
			fmt.Printf("\033[%d;1H%s│%s%s%s│%s",
				row, cBorder, cReset, strings.Repeat(" ", innerW), cBorder, cReset)
		}
		fmt.Printf("\033[%d;1H%s└%s┘%s",
			h, cBorder, strings.Repeat("─", innerW), cReset)
	}

	// ── Helper: place text at row,col (content-relative: row 1 = line 2 of screen) ──
	put := func(row, col int, color, text string) {
		fmt.Printf("\033[%d;%dH%s%s%s", row+1, col+1, color, text, cReset)
	}

	// ── Helper: center text horizontally (ANSI + CJK aware) ──
	center := func(row int, color, text string) {
		vw := visibleLen(text)
		col := (w - vw) / 2
		if col < 2 {
			col = 2
		}
		put(row, col, color, text)
	}

	// ── Helper: read one key (returns printable byte or escape seq) ──
	readKey := func() []byte {
		buf := make([]byte, 16)
		n, _ := os.Stdin.Read(buf)
		return buf[:n]
	}

	// ── Step 1: Welcome ──
	drawFrame(i18n.T("onboard.frame_title"))

	cy := h / 2
	center(cy-4, cBanner, i18n.T("onboard.welcome"))
	center(cy-2, "", i18n.T("onboard.tagline_line"))
	center(cy-1, "", i18n.T("onboard.desc1"))
	center(cy, "", i18n.T("onboard.desc2"))
	center(cy+3, cBanner, i18n.T("onboard.need_identity"))

	put(h-2, 2, cDim, i18n.T("onboard.press_enter"))

	for {
		key := readKey()
		if key[0] == '\r' || key[0] == '\n' || key[0] == ' ' {
			break
		}
		if key[0] == 'q' || key[0] == 3 { // q or Ctrl-C
			return fmt.Errorf("aborted")
		}
	}

	// ── Step 2: Role Selection ──
	chosen := len(onboardRoles) - 1 // default: lobster
	for {
		drawFrame(i18n.T("onboard.choose_role"))

		center(3, cTitle, i18n.T("onboard.select_role"))
		center(4, cDim, i18n.T("onboard.nav_hint"))

		for i, r := range onboardRoles {
			row := 7 + i*2
			prefix := "   "
			color := ""
			if i == chosen {
				prefix = " ► "
				color = cHighlight
			}
			line := fmt.Sprintf("%s%s  %-12s — %s", prefix, r.icon, roleName(r.id), roleDesc(r.id))
			// Pad to fill highlight
			for len([]rune(line)) < 50 {
				line += " "
			}
			col := (w - 50) / 2
			if col < 3 {
				col = 3
			}
			if i == chosen {
				put(row, col, color, line)
			} else {
				put(row, col, "", line)
			}
		}

		put(h-2, 2, cDim, i18n.Tf("onboard.current", onboardRoles[chosen].icon, roleName(onboardRoles[chosen].id)))
		put(h-2, w-28, cDim, i18n.T("onboard.nav_keys"))

		key := readKey()
		switch {
		case key[0] == 'q' || key[0] == 3:
			return fmt.Errorf("aborted")
		case key[0] == '\r' || key[0] == '\n':
			goto roleChosen
		case key[0] == 'k' || (len(key) >= 3 && key[0] == 0x1b && key[1] == '[' && key[2] == 'A'):
			chosen--
			if chosen < 0 {
				chosen = len(onboardRoles) - 1
			}
		case key[0] == 'j' || (len(key) >= 3 && key[0] == 0x1b && key[1] == '[' && key[2] == 'B'):
			chosen++
			if chosen >= len(onboardRoles) {
				chosen = 0
			}
		}
	}
roleChosen:
	role := onboardRoles[chosen]

	if err := callSetRole(base, role.id); err != nil {
		// Silently continue — daemon may not be running
		_ = err
	}

	// ── Step 3: Nickname ──
	drawFrame(i18n.T("onboard.nickname_title"))

	center(5, cTitle, i18n.T("onboard.nickname_prompt"))
	center(6, cDim, i18n.T("onboard.nickname_opt"))

	// Line editor with UTF-8 support (CJK, emoji, etc.)
	nickname := ""
	fieldW := 36
	cursorCol := (w - fieldW - 4) / 2
	if cursorCol < 4 {
		cursorCol = 4
	}
	inputRow := 10
	put(inputRow, cursorCol-2, cBorder, "> ")
	fmt.Print("\033[?25h") // show cursor for text input

	// UTF-8 read buffer — accumulates multi-byte sequences
	utf8Buf := make([]byte, 0, 64)

	for {
		// Render input field with visible-width padding
		nw := visibleLen(nickname)
		pad := fieldW - nw
		if pad < 0 {
			pad = 0
		}
		put(inputRow, cursorCol, cTitle, nickname+strings.Repeat(" ", pad))
		// Position cursor at end of visible text
		fmt.Printf("\033[%d;%dH", inputRow+1, cursorCol+nw+1)

		rawBuf := make([]byte, 16)
		n, _ := os.Stdin.Read(rawBuf)
		if n == 0 {
			continue
		}
		for _, b := range rawBuf[:n] {
			switch {
			case b == '\r' || b == '\n':
				goto nickDone
			case b == 3: // Ctrl-C
				return fmt.Errorf("aborted")
			case b == 127 || b == 8: // Backspace
				if len(nickname) > 0 {
					runes := []rune(nickname)
					nickname = string(runes[:len(runes)-1])
				}
				utf8Buf = utf8Buf[:0]
			case b == 0x1b: // Escape sequence start — skip
				utf8Buf = utf8Buf[:0]
			case b >= 0x80 && b <= 0xBF: // UTF-8 continuation byte
				if len(utf8Buf) > 0 {
					utf8Buf = append(utf8Buf, b)
					// Check if we have a complete rune
					r, size := rune(0), 0
					if utf8Buf[0]&0xE0 == 0xC0 && len(utf8Buf) >= 2 {
						r = rune(utf8Buf[0]&0x1F)<<6 | rune(utf8Buf[1]&0x3F)
						size = 2
					} else if utf8Buf[0]&0xF0 == 0xE0 && len(utf8Buf) >= 3 {
						r = rune(utf8Buf[0]&0x0F)<<12 | rune(utf8Buf[1]&0x3F)<<6 | rune(utf8Buf[2]&0x3F)
						size = 3
					} else if utf8Buf[0]&0xF8 == 0xF0 && len(utf8Buf) >= 4 {
						r = rune(utf8Buf[0]&0x07)<<18 | rune(utf8Buf[1]&0x3F)<<12 | rune(utf8Buf[2]&0x3F)<<6 | rune(utf8Buf[3]&0x3F)
						size = 4
					}
					if size > 0 && len(utf8Buf) >= size {
						if visibleLen(nickname)+runeWidth(r) <= fieldW {
							nickname += string(r)
						}
						utf8Buf = utf8Buf[:0]
					}
				}
			case b >= 0xC0: // UTF-8 leading byte
				utf8Buf = append(utf8Buf[:0], b)
			case b >= 32 && b < 127: // Printable ASCII
				if visibleLen(nickname)+1 <= fieldW {
					nickname += string(rune(b))
				}
				utf8Buf = utf8Buf[:0]
			}
		}
	}
nickDone:
	fmt.Print("\033[?25l") // hide cursor again

	if nickname != "" {
		payload, _ := json.Marshal(map[string]string{"agent_name": nickname})
		req, _ := http.NewRequest("PUT", base+"/api/profile",
			strings.NewReader(string(payload)))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}

	// ── Step 4: Scrollable Disclaimer ──
	// Restore terminal before pager (it does its own raw mode)
	term.Restore(fd, oldState)
	fmt.Print("\033[?25h\033[?1049l\033[0m")

	if err := showDisclaimerPager(); err != nil {
		// Re-enter raw mode for cleanup
		oldState, _ = term.MakeRaw(fd)
		return err
	}

	// Re-enter raw mode + alt screen for cinematic finish
	oldState, _ = term.MakeRaw(fd)

	// ── Step 5: Cinematic Finish ──
	cinematicFinish(w, h, role.icon, roleName(role.id), nickname)
	return nil
}

// ── Disclaimer text (EULA) ──

var disclaimerLines = []string{
	"CLAWNET END-USER LICENSE AGREEMENT",
	"",
	"Version 1.0 — Effective Date: 2026-01-01",
	"",
	"IMPORTANT: PLEASE READ THIS AGREEMENT CAREFULLY BEFORE USING CLAWNET.",
	"BY PROCEEDING, YOU ACKNOWLEDGE THAT YOU HAVE READ, UNDERSTOOD, AND",
	"AGREE TO BE BOUND BY THE TERMS AND CONDITIONS OF THIS AGREEMENT.",
	"",
	"=========================================================================",
	"",
	"1. DEFINITIONS",
	"",
	`   "Software" refers to ClawNet, including all binaries, source code,`,
	"   libraries, documentation, configuration files, and associated",
	"   materials distributed under this agreement.",
	"",
	`   "Licensor" refers to Chatchat Technology (HK) Limited, a company`,
	"   incorporated under the laws of Hong Kong Special Administrative",
	"   Region, People's Republic of China.",
	"",
	`   "Community" refers to the ChatChatTech open-source community and`,
	"   its individual contributors.",
	"",
	`   "User" or "You" refers to any individual or entity that downloads,`,
	"   installs, runs, or otherwise uses the Software.",
	"",
	`   "Node" refers to any instance of the Software running on a device`,
	"   that participates in the ClawNet peer-to-peer network.",
	"",
	`   "Shell" refers to the internal credit unit used within the ClawNet`,
	"   network for task settlement. Shell has no monetary value and is not",
	"   a cryptocurrency, token, security, or financial instrument of any",
	"   kind.",
	"",
	"",
	"2. LICENSE GRANT",
	"",
	"   Subject to the terms of this Agreement, the Licensor grants You a",
	"   non-exclusive, worldwide license to use the Software in accordance",
	"   with the GNU Affero General Public License",
	"   version 3 (AGPL-3.0) and the Additional Terms set forth in the",
	"   LICENSE file included with the Software.",
	"",
	"   You may:",
	"   (a) Run the Software on devices You own or control;",
	"   (b) Modify the Software for personal or organizational use;",
	"   (c) Redistribute the Software under the same AGPL-3.0 terms.",
	"",
	"   You may NOT:",
	"   (a) Remove or alter any copyright, trademark, or attribution",
	"       notices included in the Software;",
	"   (b) Use the Software for any purpose that violates applicable law;",
	"   (c) Sublicense the Software under terms inconsistent with AGPL-3.0",
	"       without a separate commercial license from the Licensor.",
	"",
	"",
	"3. PEER-TO-PEER NETWORK PARTICIPATION",
	"",
	"   By running a Node, You voluntarily participate in the ClawNet",
	"   decentralized peer-to-peer network. You acknowledge that:",
	"",
	"   (a) Your Node will communicate with other Nodes operated by third",
	"       parties over the public Internet;",
	"   (b) Data transmitted through the network may traverse jurisdictions",
	"       with varying legal requirements;",
	"   (c) You are solely responsible for ensuring Your use of the network",
	"       complies with all applicable local, national, and international",
	"       laws and regulations;",
	"   (d) The Licensor does not operate, control, or monitor the",
	"       peer-to-peer network and has no ability to censor, intercept,",
	"       or modify traffic between Nodes;",
	"   (e) Other participants in the network are independent third parties",
	"       and the Licensor bears no responsibility for their conduct,",
	"       content, or compliance with any law.",
	"",
	"",
	"4. NO WARRANTY",
	"",
	"   THE SOFTWARE IS PROVIDED \"AS IS\" AND \"AS AVAILABLE\", WITHOUT",
	"   WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED",
	"   TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR",
	"   PURPOSE, TITLE, AND NON-INFRINGEMENT.",
	"",
	"   THE LICENSOR, THE COMMUNITY, AND ALL CONTRIBUTORS MAKE NO",
	"   WARRANTY THAT:",
	"   (a) The Software will meet Your requirements or expectations;",
	"   (b) The Software will be uninterrupted, timely, secure, or",
	"       error-free;",
	"   (c) The results obtained from the Software will be accurate or",
	"       reliable;",
	"   (d) Any errors in the Software will be corrected;",
	"   (e) The network will be available, stable, or performant at any",
	"       given time.",
	"",
	"",
	"5. LIMITATION OF LIABILITY",
	"",
	"   TO THE MAXIMUM EXTENT PERMITTED BY APPLICABLE LAW, IN NO EVENT",
	"   SHALL CHATCHAT TECHNOLOGY (HK) LIMITED, THE CHATCHATTECH COMMUNITY,",
	"   OR ANY CONTRIBUTOR BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,",
	"   SPECIAL, EXEMPLARY, CONSEQUENTIAL, OR PUNITIVE DAMAGES (INCLUDING",
	"   BUT NOT LIMITED TO PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES,",
	"   LOSS OF USE, DATA, PROFITS, BUSINESS INTERRUPTION, OR GOODWILL),",
	"   HOWEVER CAUSED AND UNDER ANY THEORY OF LIABILITY, WHETHER IN",
	"   CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR",
	"   OTHERWISE), ARISING IN ANY WAY OUT OF THE USE OF OR INABILITY TO",
	"   USE THE SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH",
	"   DAMAGE.",
	"",
	"   THIS LIMITATION APPLIES TO:",
	"   (a) Any content, data, or materials transmitted, received, or",
	"       stored through the peer-to-peer network;",
	"   (b) Any actions taken by other participants in the network;",
	"   (c) Any loss or corruption of data, including knowledge entries,",
	"       task records, credit balances, or reputation scores;",
	"   (d) Any unauthorized access to or alteration of Your transmissions",
	"       or data;",
	"   (e) Any security vulnerabilities, exploits, or breaches;",
	"   (f) Any interruption, suspension, or termination of the network or",
	"       any portion thereof.",
	"",
	"",
	"6. INDEMNIFICATION",
	"",
	"   You agree to indemnify, defend, and hold harmless Chatchat",
	"   Technology (HK) Limited, the ChatChatTech community, and their",
	"   respective officers, directors, employees, agents, and",
	"   contributors from and against any and all claims, damages,",
	"   obligations, losses, liabilities, costs, and expenses (including",
	"   reasonable attorneys' fees) arising from:",
	"",
	"   (a) Your use of the Software or participation in the network;",
	"   (b) Your violation of any term of this Agreement;",
	"   (c) Your violation of any applicable law or regulation;",
	"   (d) Any content or data You publish, transmit, or store through",
	"       the network;",
	"   (e) Any claim that Your use of the Software infringes or violates",
	"       the rights of any third party.",
	"",
	"",
	"7. CREDITS AND VIRTUAL UNITS",
	"",
	"   The ClawNet network uses an internal unit called \"Shell\" for task",
	"   settlement between Nodes. You acknowledge and agree that:",
	"",
	"   (a) Shell has NO monetary value, exchange value, or cash",
	"       equivalent;",
	"   (b) Shell is NOT a cryptocurrency, digital currency, token, coin,",
	"       security, commodity, or financial instrument;",
	"   (c) Shell cannot be purchased, sold, traded, or exchanged for any",
	"       fiat currency, cryptocurrency, or other asset;",
	"   (d) The Licensor has no obligation to maintain, honor, or redeem",
	"       any Shell balance;",
	"   (e) Shell balances may be reset, modified, or eliminated at any",
	"       time without notice.",
	"",
	"",
	"8. DATA AND PRIVACY",
	"",
	"   (a) The Software operates on a decentralized peer-to-peer basis.",
	"       The Licensor does not collect, store, or process personal data",
	"       through the operation of the network.",
	"   (b) Your public peer identity (derived from Your cryptographic key",
	"       pair) and any information You choose to publish (such as",
	"       nickname, role, motto, or knowledge entries) will be visible",
	"       to other participants in the network.",
	"   (c) You are solely responsible for any personal data You disclose",
	"       through the network.",
	"   (d) IP addresses may be visible to directly connected peers as an",
	"       inherent property of peer-to-peer networking.",
	"",
	"",
	"9. SECURITY",
	"",
	"   You acknowledge that:",
	"   (a) The Software uses cryptographic protocols, but no system is",
	"       perfectly secure;",
	"   (b) You are responsible for safeguarding Your private keys and",
	"       identity files;",
	"   (c) Loss of Your private key may result in permanent loss of Your",
	"       identity, reputation, and credits;",
	"   (d) The Licensor cannot recover lost keys or restore compromised",
	"       identities;",
	"   (e) You should review the source code and understand the security",
	"       model before running the Software in any sensitive environment.",
	"",
	"",
	"10. OPEN SOURCE COMPONENTS",
	"",
	"    The Software incorporates open-source libraries and components",
	"    licensed under various open-source licenses. A complete list of",
	"    third-party dependencies and their licenses is available in the",
	"    go.mod file and the project repository. The Licensor makes no",
	"    warranty regarding third-party components.",
	"",
	"",
	"11. TERMINATION",
	"",
	"    (a) This Agreement is effective until terminated.",
	"    (b) You may terminate this Agreement at any time by ceasing all",
	"        use of the Software and deleting all copies.",
	"    (c) If You breach any term of this Agreement, Your rights under",
	"        this Agreement terminate automatically. However, if You cure",
	"        the breach within thirty (30) days of becoming aware of such",
	"        breach, Your rights shall be reinstated, consistent with the",
	"        termination and reinstatement provisions of AGPL-3.0 Section 8.",
	"    (d) Upon termination, Sections 4, 5, 6, 7, and 12 shall survive.",
	"",
	"",
	"12. GOVERNING LAW AND DISPUTE RESOLUTION",
	"",
	"    This Agreement shall be governed by and construed in accordance",
	"    with the laws of the Hong Kong Special Administrative Region,",
	"    without regard to its conflict of laws principles.",
	"",
	"    Any dispute arising out of or in connection with this Agreement",
	"    shall be submitted to the exclusive jurisdiction of the courts of",
	"    the Hong Kong Special Administrative Region.",
	"",
	"",
	"13. MISCELLANEOUS",
	"",
	"    (a) This Agreement constitutes the entire agreement between You",
	"        and the Licensor regarding the Software.",
	"    (b) If any provision is held unenforceable, the remaining",
	"        provisions shall continue in full force and effect.",
	"    (c) The Licensor's failure to enforce any right or provision shall",
	"        not constitute a waiver of such right or provision.",
	"    (d) This Agreement may be updated from time to time. Continued",
	"        use of the Software after changes constitutes acceptance of",
	"        the revised terms.",
	"",
	"",
	"=========================================================================",
	"",
	"    Copyright (C) 2023-2026 Chatchat Technology (HK) Limited",
	"    All rights reserved.",
	"",
	"    ClawNet is distributed under the GNU Affero General Public License",
	"    version 3 (AGPL-3.0) with Additional Terms. See the LICENSE file",
	"    for the complete license text.",
	"",
	"    Contact: ink@chatchat.space",
	"    Website: https://chatchat.space",
	"",
	"=========================================================================",
}

// showDisclaimerPager renders a scrollable EULA pager in the terminal.
// The user must scroll to the bottom (Space / ↓ / PageDown) before accepting (Enter).
func showDisclaimerPager() error {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("cannot enter raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	termW, termH, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		termW, termH = 80, 24
	}

	// Reserve 2 lines: title bar + status bar
	viewH := termH - 2
	if viewH < 4 {
		viewH = 4
	}

	totalLines := len(disclaimerLines)
	maxScroll := totalLines - viewH
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := 0
	reachedEnd := maxScroll == 0

	// Enter alt screen, hide cursor
	fmt.Print("\033[?1049h\033[?25l")
	defer fmt.Print("\033[?25h\033[?1049l")

	render := func() {
		fmt.Print("\033[2J\033[1;1H")
		// Title bar (pad to terminal width, erase rest, \r\n for raw mode)
		title := " CLAWNET LICENSE AGREEMENT"
		pad := termW - len(title) - 1
		if pad < 0 {
			pad = 0
		}
		fmt.Printf("%s %s%s\033[K%s\r\n", cBanner, title, strings.Repeat(" ", pad), cReset)

		// Content lines
		end := scroll + viewH
		if end > totalLines {
			end = totalLines
		}
		for i := scroll; i < end; i++ {
			fmt.Printf("  %s\033[K\r\n", disclaimerLines[i])
		}
		// Pad remaining
		for i := end - scroll; i < viewH; i++ {
			fmt.Print("\033[K\r\n")
		}

		// Status bar
		pct := 0
		if maxScroll > 0 {
			pct = scroll * 100 / maxScroll
		} else {
			pct = 100
		}
		if reachedEnd {
			fmt.Printf("%s Press Enter to accept  |  q to quit %s",
				"\033[7m", cReset)
		} else {
			fmt.Printf("%s Space/↓/PgDn to scroll (%d%%)  |  q to quit %s",
				cDim, pct, cReset)
		}
	}
	render()

	buf := make([]byte, 16)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return err
		}
		key := buf[:n]

		switch {
		// q or Ctrl-C — abort
		case key[0] == 'q' || key[0] == 'Q' || key[0] == 3:
			return fmt.Errorf("license not accepted")

		// Enter — accept only if scrolled to end
		case key[0] == '\r' || key[0] == '\n':
			if reachedEnd {
				return nil
			}

		// Space — page down
		case key[0] == ' ':
			scroll += viewH
			if scroll > maxScroll {
				scroll = maxScroll
			}

		// Escape sequences (arrows, pgup/pgdn)
		case n >= 3 && key[0] == 0x1b && key[1] == '[':
			switch key[2] {
			case 'B': // Down arrow
				scroll++
				if scroll > maxScroll {
					scroll = maxScroll
				}
			case 'A': // Up arrow
				scroll--
				if scroll < 0 {
					scroll = 0
				}
			case '5': // Page Up  (ESC [ 5 ~)
				scroll -= viewH
				if scroll < 0 {
					scroll = 0
				}
			case '6': // Page Down (ESC [ 6 ~)
				scroll += viewH
				if scroll > maxScroll {
					scroll = maxScroll
				}
			}

		// j/k vim keys
		case key[0] == 'j':
			scroll++
			if scroll > maxScroll {
				scroll = maxScroll
			}
		case key[0] == 'k':
			scroll--
			if scroll < 0 {
				scroll = 0
			}
		}

		if scroll >= maxScroll {
			reachedEnd = true
		}
		render()
	}
}

// cinematicFinish renders a full-screen welcome ceremony with a red wave animation.
// A tide surges up from the bottom twice, revealing the ClawNet banner as it recedes.
func cinematicFinish(w, h int, roleIcon, roleName, nickname string) {
	innerW := w - 2

	fmt.Print("\033[?1049h\033[?25l\033[40m\033[2J")
	defer fmt.Print("\033[?1049l\033[0m")

	// ── Border ──
	fmt.Print("\033[1;1H")
	fmt.Print(cBorder + "┌" + strings.Repeat("─", innerW) + "┐" + cReset)
	for row := 2; row <= h-1; row++ {
		fmt.Printf("\033[%d;1H%s│%s%s%s│%s",
			row, cBorder, cReset, strings.Repeat(" ", innerW), cBorder, cReset)
	}
	fmt.Printf("\033[%d;1H%s└%s┘%s", h, cBorder, strings.Repeat("─", innerW), cReset)

	// ── Banner layout ──
	maxBW := 0
	for _, line := range clawnetBanner {
		if len(line) > maxBW {
			maxBW = len(line)
		}
	}
	topPad := (h - 12) / 2
	if topPad < 3 {
		topPad = 3
	}
	bCol := (w - maxBW) / 2
	if bCol < 2 {
		bCol = 2
	}

	// Pre-compute banner character map
	type xy struct{ r, c int }
	bmap := make(map[xy]byte)
	for i, line := range clawnetBanner {
		for j := 0; j < len(line); j++ {
			if line[j] != ' ' {
				bmap[xy{topPad + i, bCol + j}] = line[j]
			}
		}
	}

	// ── Easing ──
	easeIO := func(t float64) float64 {
		if t < 0.5 {
			return 2 * t * t
		}
		return -1 + (4-2*t)*t
	}

	// ── Wave keyframes: (time_fraction, screen_row) ──
	bH := len(clawnetBanner)
	type kf struct{ t, y float64 }
	keys := []kf{
		{0.00, float64(h + 3)},                 // off-screen bottom
		{0.30, float64(topPad + bH/2)},          // surge 1: covers bottom half
		{0.50, float64(topPad + bH + 4)},        // recede 1: below banner
		{0.72, float64(topPad - 2)},              // surge 2: above banner entirely
		{1.00, float64(h + 4)},                   // full recede
	}
	nFrames := 120

	baseAt := func(f int) float64 {
		t := float64(f) / float64(nFrames)
		for i := 1; i < len(keys); i++ {
			if t <= keys[i].t {
				p := easeIO((t - keys[i-1].t) / (keys[i].t - keys[i-1].t))
				return keys[i-1].y + (keys[i].y-keys[i-1].y)*p
			}
		}
		return keys[len(keys)-1].y
	}

	// ── Wave surface: 5 sine harmonics + pseudo-noise for irregular foam ──
	waveAt := func(col, f int, by float64) float64 {
		x, ft := float64(col), float64(f)
		// Large slow swell
		v := 1.8 * math.Sin(x*0.06+ft*0.13)
		// Mid-frequency chop
		v += 1.0 * math.Sin(x*0.17+ft*0.09+2.7)
		// Short ripples
		v += 0.6 * math.Sin(x*0.31+ft*0.21+1.1)
		// Cross-wave interference
		v += 0.4 * math.Sin(x*0.07-ft*0.15+4.2)
		// Micro-turbulence (col-dependent pseudo-noise)
		noise := math.Sin(float64(col*col)*0.013+ft*0.37) * math.Sin(float64(col)*0.41+ft*0.07)
		v += 0.5 * noise
		return by + v
	}

	// ── Pre-built color strings ──
	wBg := [5]string{
		"\033[48;2;190;42;55m",  // shallow
		"\033[48;2;145;30;40m",
		"\033[48;2;100;22;30m",
		"\033[48;2;60;14;20m",
		"\033[48;2;32;8;12m", // deep
	}
	foamC := "\033[1;38;2;250;130;140m\033[48;2;170;40;50m"
	sprayC := "\033[38;2;140;50;55m\033[49m"
	sandC := "\033[38;2;55;28;22m\033[49m"
	banC := cBanner + "\033[49m"
	clrC := "\033[0m"

	hw := float64(h + 10) // high-water mark (minimum baseY)

	// ── Wave animation loop ──
	for f := 0; f <= nFrames; f++ {
		by := baseAt(f)
		if by < hw {
			hw = by
		}

		var sb strings.Builder
		sb.Grow(w * h * 8)

		for row := 2; row <= h-1; row++ {
			sb.WriteString(fmt.Sprintf("\033[%d;2H", row))
			last := ""

			for col := 2; col <= w-1; col++ {
				sy := waveAt(col, f, by)
				fr := float64(row)

				if fr > sy+0.5 {
					// ── Water (background color by depth) ──
					depth := fr - sy
					idx := 4
					switch {
					case depth < 2:
						idx = 0
					case depth < 4:
						idx = 1
					case depth < 7:
						idx = 2
					case depth < 11:
						idx = 3
					}
					c := wBg[idx]
					if c != last {
						sb.WriteString(c)
						last = c
					}
					sb.WriteByte(' ')

				} else if fr > sy-0.5 {
					// ── Foam crest ──
					if foamC != last {
						sb.WriteString(foamC)
						last = foamC
					}
// Varied foam characters for irregular look
				hash := col*31 + f*7
				switch hash % 6 {
				case 0:
					sb.WriteByte('=')
				case 1:
					sb.WriteByte('~')
				case 2:
					sb.WriteByte('-')
				case 3:
					sb.WriteString("~")
				case 4:
					sb.WriteByte('^')
				default:
						sb.WriteByte('~')
					}

				} else {
					// ── Above water ──
					revealed := hw-1.5 < fr

					if bch, ok := bmap[xy{row, col}]; ok && revealed {
						// Banner character (revealed by receding wave)
						if banC != last {
							sb.WriteString(banC)
							last = banC
						}
						sb.WriteByte(bch)
					} else if fr > sy-2.0 && (col*17+f*3)%7 < 2 {
						// Spray droplets near the crest
						if sprayC != last {
							sb.WriteString(sprayC)
							last = sprayC
						}
						sb.WriteByte('.')
					} else if (row*7+col*13+17)%11 < 2 {
						// Sand dots (beach texture)
						if sandC != last {
							sb.WriteString(sandC)
							last = sandC
						}
						sb.WriteByte('.')
					} else {
						if clrC != last {
							sb.WriteString(clrC)
							last = clrC
						}
						sb.WriteByte(' ')
					}
				}
			}
			sb.WriteString(cReset)
		}
		os.Stdout.WriteString(sb.String())
		time.Sleep(33 * time.Millisecond)
	}

	// ── Redraw banner on top of remaining beach scene (no clear) ──
	for i, line := range clawnetBanner {
		fmt.Printf("\033[%d;%dH%s%s%s", topPad+i, bCol, cBanner, line, cReset)
	}

	// ── Typewriter welcome message ──
	msg := welcomeMsg(rand.Intn(5))
	msgRow := topPad + bH + 2
	msgCol := (w - len(msg)) / 2
	if msgCol < 2 {
		msgCol = 2
	}
	fmt.Printf("\033[%d;%dH", msgRow, msgCol)
	for _, ch := range msg {
		fmt.Printf("%s%c%s", cTitle, ch, cReset)
		time.Sleep(30 * time.Millisecond)
	}

	// ── Role + nickname ──
	var info string
	if nickname != "" {
		info = fmt.Sprintf("%s %s — %s", roleIcon, roleName, nickname)
	} else {
		info = fmt.Sprintf("%s %s", roleIcon, roleName)
	}
	infoRow := msgRow + 2
	infoCol := (w - len([]rune(info))) / 2
	if infoCol < 2 {
		infoCol = 2
	}
	fmt.Printf("\033[%d;%dH%s%s%s", infoRow, infoCol, cBannerHL, info, cReset)

	// ── Exit with countdown ──
	time.Sleep(400 * time.Millisecond)

	// Non-blocking key read via goroutine
	keyPressed := make(chan struct{}, 1)
	go func() {
		b := make([]byte, 1)
		os.Stdin.Read(b)
		keyPressed <- struct{}{}
	}()

	for sec := 3; sec >= 0; sec-- {
		var exitMsg string
		if sec > 0 {
			exitMsg = fmt.Sprintf("Press any key to exit... (%d)", sec)
		} else {
			exitMsg = ""
		}
		// Clear previous line then draw
		clearLine := strings.Repeat(" ", 40)
		fmt.Printf("\033[%d;%dH%s", h-2, (w-40)/2, clearLine)
		if exitMsg != "" {
			exitCol := (w - len(exitMsg)) / 2
			fmt.Printf("\033[%d;%dH%s%s%s", h-2, exitCol, cDim, exitMsg, cReset)
		}

		if sec == 0 {
			break
		}

		select {
		case <-keyPressed:
			return
		case <-time.After(1 * time.Second):
		}
	}
}

func cmdDoctor() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)
	resp, err := http.Get(base + "/api/diagnostics")
	if err != nil {
		return fmt.Errorf(i18n.T("err.daemon_connect")+": %w", err)
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

	fmt.Println(red + "  " + i18n.T("doctor.title") + rst)
	fmt.Println()

	// Identity
	if id, ok := diag["peer_id"].(string); ok {
		fmt.Printf(tidal+"  %-12s"+rst+"%s\n", i18n.T("doctor.peer_id"), safePrefix(id, 16)+"…")
	}
	if v, ok := diag["version"].(string); ok {
		fmt.Printf(tidal+"  %-12s"+rst+"%s\n", i18n.T("doctor.version"), v)
	}
	if up, ok := diag["uptime"].(string); ok {
		fmt.Printf(tidal+"  %-12s"+rst+"%s\n", i18n.T("doctor.uptime"), up)
	}
	fmt.Println()

	// Addresses
	fmt.Println(coral + "  " + i18n.T("doctor.addresses") + rst)
	if addrs, ok := diag["listen_addrs"].([]any); ok {
		for _, a := range addrs {
			fmt.Printf("    %-9s%s\n", i18n.T("doctor.listen"), a)
		}
	}
	if addrs, ok := diag["announce_addrs"].([]any); ok && len(addrs) > 0 {
		for _, a := range addrs {
			fmt.Printf("    %-9s%s\n", i18n.T("doctor.announce"), a)
		}
	} else {
		fmt.Printf("    %-9s%s%s%s\n", i18n.T("doctor.announce"), dim, i18n.T("doctor.announce_none"), rst)
	}
	fmt.Println()

	// NAT & Relay
	fmt.Println(coral + "  " + i18n.T("doctor.nat_relay") + rst)
	if mode, ok := diag["nat_mode"].(string); ok {
		fmt.Printf("    %-13s%s\n", i18n.T("doctor.nat_mode"), mode)
	}
	if relay, ok := diag["relay_enabled"].(bool); ok {
		if relay {
			fmt.Printf("    %-13s%s%s%s\n", i18n.T("doctor.relay"), green, i18n.T("doctor.relay_enabled"), rst)
		} else {
			fmt.Printf("    %-13s%s%s%s\n", i18n.T("doctor.relay"), red, i18n.T("doctor.relay_disabled"), rst)
		}
	}
	direct := int(getFloat(diag, "connections_direct"))
	relayC := int(getFloat(diag, "connections_relay"))
	fmt.Printf("    %-13s%d\n", i18n.T("doctor.direct_conn"), direct)
	fmt.Printf("    %-13s%d\n", i18n.T("doctor.relay_conn"), relayC)
	fmt.Println()

	// Discovery
	fmt.Println(coral + "  " + i18n.T("doctor.discovery") + rst)
	dhtSize := int(getFloat(diag, "dht_routing_table"))
	fmt.Printf("    %-13s%s\n", i18n.T("doctor.dht_table"), i18n.Tf("doctor.dht_peers", dhtSize))
	if bt, ok := diag["btdht_status"].(string); ok {
		sym := green + "✓" + rst
		if bt == "disabled" {
			sym = red + "✗" + rst
		}
		fmt.Printf("    %-13s%s %s\n", i18n.T("doctor.btdht"), sym, bt)
	}
	overlayPeers := int(getFloat(diag, "overlay_peers"))
	if overlayPeers > 0 {
		fmt.Printf("    %-13s%s%s%s\n", i18n.T("doctor.overlay"), green, i18n.Tf("doctor.overlay_peers", overlayPeers), rst)
	} else if _, ok := diag["overlay_peers"]; ok {
		fmt.Printf("    %-13s%s%s%s\n", i18n.T("doctor.overlay"), dim, i18n.T("doctor.overlay_none"), rst)
	}
	cryptoSessions := int(getFloat(diag, "crypto_sessions"))
	if cryptoSessions > 0 {
		fmt.Printf("    %-13s%s%s%s\n", i18n.T("doctor.e2e_crypto"), green, i18n.Tf("doctor.crypto_sessions", cryptoSessions), rst)
	} else {
		fmt.Printf("    %-13s%s%s%s\n", i18n.T("doctor.e2e_crypto"), green, i18n.T("doctor.crypto_enabled"), rst)
	}
	fmt.Println()

	// Bootstrap
	fmt.Println(coral + "  " + i18n.T("doctor.bootstrap") + rst)
	if bs, ok := diag["bootstrap_peers"].(map[string]any); ok {
		for id, v := range bs {
			reachable, _ := v.(bool)
			sym := red + i18n.T("doctor.bs_unreachable") + rst
			if reachable {
				sym = green + i18n.T("doctor.bs_connected") + rst
			}
			fmt.Printf("    %s  %s\n", id, sym)
		}
	}
	fmt.Println()

	// Summary
	total := int(getFloat(diag, "peers_total"))
	if total == 0 {
		fmt.Printf("%s  %s\n%s", red, i18n.T("doctor.no_peers"), rst)
	} else {
		fmt.Printf("%s  %s%s\n", green, i18n.Tf("doctor.peers_ok", total), rst)
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
		return fmt.Errorf(i18n.T("err.daemon_connect")+": %w", err)
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
		fmt.Printf("    %s  %s\n", tidal+i18n.T("peers.self")+rst, strings.Join(parts, "  "))
	}

	fmt.Print("\n" + coral + "  " + i18n.Tf("peers.connected_fmt", len(remote)) + rst + "\n\n")

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
	if len(os.Args) > 2 {
		switch os.Args[2] {
		case "-h", "--help", "help":
			boardHelp()
			return nil
		}
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)

	// Fetch board data from API
	resp, err := http.Get(base + "/api/tasks/board")
	if err != nil {
		return fmt.Errorf(i18n.T("err.daemon_connect")+": %w", err)
	}
	defer resp.Body.Close()

	// --json: pass through raw API JSON
	if JSONOutput {
		body, _ := io.ReadAll(resp.Body)
		fmt.Println(string(body))
		return nil
	}

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
	yellow := "\033[33m"
	dim := "\033[2m"
	rst := "\033[0m"

	fmt.Println(red + "  " + i18n.T("board.title") + rst)
	fmt.Println()

	// My Published Tasks
	fmt.Println(coral + "  " + i18n.T("board.my_published") + rst)
	if len(board.MyPublished) == 0 {
		fmt.Println(dim + "    " + i18n.T("board.none") + rst)
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
		case "approved", "settled":
			statusColor = green
		}
		bidInfo := ""
		if t.BidCount > 0 {
			bidInfo = "  " + i18n.Tf("board.bid_count", t.BidCount)
		}
		subInfo := ""
		if t.SubCount > 0 {
			subInfo = "  " + i18n.Tf("board.sub_count", t.SubCount)
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
			target = dim + " " + i18n.T("board.targeted") + rst
		}
		countdown := formatCountdown(t.BidCloseAt, t.WorkDeadline, t.Status)
		fmt.Printf("    %s[%s]%s %s%d%s %s%s%s%s%s%s\n",
			statusColor, t.Status, rst, coral, t.Reward, rst, t.Title, target, bidInfo, subInfo, assignee, countdown)
		fmt.Printf("    %s%s  %s%s\n", dim, safePrefix(t.ID, 8)+"...", safePrefix(t.CreatedAt, 10), rst)
	}
	fmt.Println()

	// My Assignments / Active Work
	fmt.Println(coral + "  " + i18n.T("board.my_active") + rst)
	if len(board.MyAssigned) == 0 {
		fmt.Println(dim + "    " + i18n.T("board.none_active") + rst)
	}
	for _, t := range board.MyAssigned {
		countdown := formatCountdown(t.BidCloseAt, t.WorkDeadline, t.Status)
		expectPay := ""
		if t.ExpectedPay > 0 {
			expectPay = fmt.Sprintf("  %sE[pay]=%d%s", yellow, t.ExpectedPay, rst)
		}
		fmt.Printf("    %s[%s]%s %s%d%s %s  by %s%s%s\n",
			tidal, t.Status, rst, coral, t.Reward, rst, t.Title, truncName(t.AuthorName, 16), countdown, expectPay)
		fmt.Printf("    %s%s  %s  %d bid(s) %d sub(s)%s\n",
			dim, safePrefix(t.ID, 8)+"...", safePrefix(t.CreatedAt, 10), t.BidCount, t.SubCount, rst)
	}
	fmt.Println()

	// Open Tasks (available to claim)
	fmt.Println(coral + "  " + i18n.T("board.open_tasks") + rst)
	if len(board.OpenTasks) == 0 {
		fmt.Println(dim + "    " + i18n.T("board.none") + rst)
	}
	for _, t := range board.OpenTasks {
		target := ""
		if t.TargetPeer != "" {
			target = dim + " " + i18n.T("board.targeted") + rst
		}
		countdown := formatCountdown(t.BidCloseAt, t.WorkDeadline, t.Status)
		expectPay := ""
		if t.ExpectedPay > 0 {
			expectPay = fmt.Sprintf("  %sE[pay]=%d%s", yellow, t.ExpectedPay, rst)
		}
		fmt.Printf("    %s%d%s %s%s  %sby %s%s%s%s\n",
			coral, t.Reward, rst, t.Title, target, dim, truncName(t.AuthorName, 16), rst, countdown, expectPay)
		fmt.Printf("    %s%s  %s  %s%s\n",
			dim, safePrefix(t.ID, 8)+"...", safePrefix(t.CreatedAt, 10), i18n.Tf("board.bid_count", t.BidCount), rst)
	}

	fmt.Println()
	fmt.Println(dim + "  " + i18n.T("tip_prefix") + rst + randomTip())
	return nil
}

func boardHelp() {
	dim := "\033[2m"
	tidal := "\033[38;2;69;123;157m"
	coral := "\033[38;2;247;127;0m"
	bold := "\033[1m"
	rst := "\033[0m"

	fmt.Println(bold + "clawnet board — Task Auction House Dashboard" + rst)
	fmt.Println()
	fmt.Println(bold + "USAGE" + rst)
	fmt.Println(tidal + "  clawnet board" + rst + dim + "        " + i18n.T("board.help.board_desc") + rst)
	fmt.Println(tidal + "  " + i18n.T("board.help.operations") + rst + dim + " " + i18n.T("board.help.operations_desc") + rst)
	fmt.Println(tidal + "  " + i18n.T("board.help.help") + rst + dim + "   " + i18n.T("board.help.help_desc") + rst)
	fmt.Println()
	fmt.Println(bold + i18n.T("board.help.sections") + rst)
	fmt.Println(coral + "  " + i18n.T("board.help.s_published") + rst + "   " + i18n.T("board.help.s_published_desc"))
	fmt.Println(coral + "  " + i18n.T("board.help.s_active") + rst + "       " + i18n.T("board.help.s_active_desc"))
	fmt.Println(coral + "  " + i18n.T("board.help.s_open") + rst + "           " + i18n.T("board.help.s_open_desc"))
	fmt.Println()
	fmt.Println(bold + i18n.T("board.help.lifecycle") + rst)
	fmt.Println(dim + "  " + i18n.T("board.help.lc1") + rst)
	fmt.Println(dim + "  " + i18n.T("board.help.lc2") + rst)
	fmt.Println(dim + "  " + i18n.T("board.help.lc3") + rst)
	fmt.Println(dim + "  " + i18n.T("board.help.lc4") + rst)
	fmt.Println(dim + "  " + i18n.T("board.help.lc5") + rst)
	fmt.Println()
	fmt.Println(bold + i18n.T("board.help.operations_title") + rst)
	fmt.Println()
	fmt.Println(tidal + "  " + i18n.T("board.help.op_create") + rst)
	fmt.Println(dim + `    clawnet task create "Review PR" -r 200 -d "..."` + rst)
	fmt.Println()
	fmt.Println(tidal + "  " + i18n.T("board.help.op_bid") + rst)
	fmt.Println(dim + `    clawnet task bid <id> -a 150 -m "I can do this"` + rst)
	fmt.Println()
	fmt.Println(tidal + "  " + i18n.T("board.help.op_claim") + rst)
	fmt.Println(dim + `    clawnet task claim <id> "result text" -s 0.85` + rst)
	fmt.Println()
	fmt.Println(tidal + "  " + i18n.T("board.help.op_assign") + rst)
	fmt.Println(dim + `    clawnet task assign <id> --to <peer_id>` + rst)
	fmt.Println()
	fmt.Println(tidal + "  " + i18n.T("board.help.op_submit") + rst)
	fmt.Println(dim + `    clawnet task submit <id>` + rst)
	fmt.Println(dim + `    clawnet task approve <id>` + rst)
	fmt.Println(dim + `    clawnet task reject <id>` + rst)
	fmt.Println()
	fmt.Println(tidal + "  " + i18n.T("board.help.op_cancel") + rst)
	fmt.Println(dim + `    clawnet task cancel <id>` + rst)
	fmt.Println()
	fmt.Println(bold + i18n.T("board.help.reading") + rst)
	fmt.Println(dim + "  " + i18n.T("board.help.r_open") + rst)
	fmt.Println(dim + "  " + i18n.T("board.help.r_assigned") + rst)
	fmt.Println(dim + "  " + i18n.T("board.help.r_submitted") + rst)
	fmt.Println(dim + "  " + i18n.T("board.help.r_settled") + rst)
	fmt.Println(dim + "  " + i18n.T("board.help.r_reward") + rst)
	fmt.Println(dim + "  " + i18n.T("board.help.r_expected") + rst)
	fmt.Println(dim + "  " + i18n.T("board.help.r_bids") + rst)
	fmt.Println(dim + "  " + i18n.T("board.help.r_targeted") + rst)
	fmt.Println()
	fmt.Println(bold + i18n.T("board.help.ai_title") + rst)
	fmt.Println(dim + "  " + i18n.T("board.help.ai_desc") + rst)
	fmt.Println(dim + "    clawnet task list --json        # structured task list" + rst)
	fmt.Println(dim + "    clawnet task show <id> --json   # task details" + rst)
	fmt.Println(dim + "    clawnet credits --json          # balance check" + rst)
	fmt.Println(dim + "    clawnet status --json            # node overview" + rst)
	fmt.Println()
	fmt.Println(bold + i18n.T("board.help.examples") + rst)
	fmt.Println(dim + "  clawnet board                  # view dashboard" + rst)
	fmt.Println(dim + "  clawnet task list               # non-interactive task list" + rst)
	fmt.Println(dim + "  clawnet status                 # check your balance & port" + rst)
	fmt.Println(dim + "  clawnet watch                  # live feed of task events" + rst)
}

// formatCountdown shows time remaining until bid close or work deadline.
func formatCountdown(bidCloseAt, workDeadline, status string) string {
	if status != "open" {
		return ""
	}
	dim := "\033[2m"
	yellow := "\033[33m"
	rst := "\033[0m"
	now := time.Now().UTC()

	if bidCloseAt != "" {
		if bc, err := time.Parse(time.RFC3339, bidCloseAt); err == nil {
			remaining := bc.Sub(now)
			if remaining > 0 {
				return fmt.Sprintf("  %s⏱ bid closes %s%s", yellow, fmtCountdownDur(remaining), rst)
			}
		}
	}
	if workDeadline != "" {
		if wd, err := time.Parse(time.RFC3339, workDeadline); err == nil {
			remaining := wd.Sub(now)
			if remaining > 0 {
				return fmt.Sprintf("  %s⏱ submit by %s%s", dim, fmtCountdownDur(remaining), rst)
			}
			return fmt.Sprintf("  %s⏱ settling...%s", dim, rst)
		}
	}
	return ""
}

func fmtCountdownDur(d time.Duration) string {
	if d > time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	return fmt.Sprintf("%.0fm", d.Minutes())
}

type boardTask struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	Reward       int64  `json:"reward"`
	AuthorName   string `json:"author_name"`
	AssignedTo   string `json:"assigned_to"`
	TargetPeer   string `json:"target_peer"`
	BidCount     int    `json:"bid_count"`
	SubCount     int    `json:"sub_count"`
	BidCloseAt   string `json:"bid_close_at"`
	WorkDeadline string `json:"work_deadline"`
	ExpectedPay  int64  `json:"expected_pay"`
	CreatedAt    string `json:"created_at"`
}

func truncName(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// ── Topo command with keyboard navigation ──

// beachEmoji tracks an emoji riding the wave surface.
type beachEmoji struct {
	emoji rune
	col   int // column position within the box
	row   int // fixed row where this emoji was deposited by the wave
}

// coinMessages displayed when a coin is grabbed.
var coinMessages = []string{
	"A golden treasure from the depths of ClawNet!",
	"The sea rewards the patient observer.",
	"Fortune favors the connected. Lucky catch!",
	"A coin forged in the fires of distributed consensus!",
	"The ClawNet ocean gives back to those who watch.",
	"Rare find! The network smiles upon you today.",
	"Between the waves, fortune awaits the curious.",
	"A gift from the deep — the lobsters send their regards.",
	"Patience on the shore pays in golden dividends.",
	"The tide brings wealth to the faithful node keeper.",
}

type topoState struct {
	activePanel   int  // panelSelf, panelPeers, panelGlobe
	detailMode    bool // true = detail view, false = globe view
	showOverlay   bool // true = show overlay/infra peers on globe and panels
	peerScrollOff int  // scroll offset in peer detail list
	selectedPeer  int  // 1-based peer index for number key selection (0 = none)
	agentCount    int  // last known count of agent peers (for left/right nav)
	feedScrollOff int  // scroll offset in activity feed
	daemonBase    string // daemon HTTP base URL (e.g. http://127.0.0.1:2024)

	// Pulse animation: recently active peers flash on the globe
	pulseActors    map[string]int64 // short peer ID prefix → expire unix timestamp
	lastEchoSeenTS int64            // timestamp of the latest processed echo

	// Wave animation state for the ClawNet stats banner
	waveFrame    int
	waveStrong   bool           // whether current wave cycle has strong banner-blowing waves
	displacedMap map[[2]int]int // banner (row,col) → vertical displacement (negative = pushed up)
	displaceExp  int            // frame when displaced chars settle back

	// Beach emoji — ride the wave surface
	beachEmojis []beachEmoji // active emojis on the wave

	// Gold coin system (daemon-internal, TUI reads visual state)
	beachCoins   []beachCoin // 1-3 coins currently on beach (from daemon)
	coinPollCh   chan *coinVisualResult // async coin state results

	// Treasure chest (count tracked by daemon, TUI mirrors for rendering)
	chestCoins  int      // coins collected in the chest (0-10)
	chestMsg    string   // current message to display next to chest
	chestMsgExp int      // frame when message expires

	// Grab animation: coin flies from beach to chest via parabolic arc
	grabActive bool // animation in progress
	grabCount  int  // how many coins are being grabbed this animation
	grabStartR int  // start row (in screen space — relative to box top)
	grabStartC int  // start column
	grabFrame  int  // current frame of animation
	grabTotal  int  // total frames for the arc
}

// beachCoin is a single gold coin sitting on the beach.
type beachCoin struct {
	row int
	col int
}

// coinVisualResult mirrors daemon's CoinVisualState.
type coinVisualResult struct {
	BeachCoins    int      `json:"beach_coins"`
	ChestCoins    int      `json:"chest_coins"`
	CoinPositions [][2]int `json:"coin_positions,omitempty"`
	Message       string   `json:"message,omitempty"`
}

func cmdTopo() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)

	resp, err := http.Get(base + "/api/peers/geo")
	if err != nil {
		return fmt.Errorf(i18n.T("err.daemon_connect")+": %w", err)
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

	state := &topoState{activePanel: panelGlobe, showOverlay: true, pulseActors: make(map[string]int64), coinPollCh: make(chan *coinVisualResult, 1), daemonBase: base}
	var overlayPeersCache []peerGeoData

	for {
		// Process all pending input
		drainInput:
		for {
			select {
			case key := <-keyCh:
				switch key {
				case 27: // Esc — exit immediately
					return nil
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
				case 13: // Enter — open detail or grab coin
					if state.detailMode && state.activePanel == panelGlobe &&
						len(state.beachCoins) > 0 && !state.grabActive && state.chestCoins+len(state.beachCoins) <= 10 {
						// Grab all coins — start parabolic animation from first coin
						state.grabActive = true
						state.grabCount = len(state.beachCoins)
						state.grabStartR = state.beachCoins[0].row
						state.grabStartC = state.beachCoins[0].col
						state.grabFrame = 0
						state.grabTotal = 15
						state.beachCoins = nil
						// Tell daemon to move beach coins to chest
						go func() {
							resp, err := http.Post(base+"/api/topo/coin-grab", "application/json", nil)
							if err == nil {
								resp.Body.Close()
							}
						}()
					} else if !state.detailMode {
						state.detailMode = true
						state.peerScrollOff = 0
						state.feedScrollOff = 0
						needClear = true
					}
				case 'c', 'C': // Convert coins to Shell
					if state.detailMode && state.activePanel == panelGlobe && state.chestCoins > 0 {
						go func() {
							resp, err := http.Post(base+"/api/topo/coin-redeem", "application/json", nil)
							if err == nil {
								resp.Body.Close()
							}
						}()
						amount := int64(state.chestCoins) * 100
						state.chestCoins = 0
						state.chestMsg = fmt.Sprintf("+%d Shell redeemed!", amount)
						state.chestMsgExp = state.waveFrame + 40
					}
				case 'o', 'O': // Toggle overlay/infra peers visibility
					state.showOverlay = !state.showOverlay
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
				case "C": // Right — next panel / next peer
					if state.detailMode && state.activePanel == panelPeers && state.selectedPeer > 0 {
						if state.selectedPeer < state.agentCount {
							state.selectedPeer++
						} else {
							state.selectedPeer = 1
						}
						state.peerScrollOff = 0
					} else if !state.detailMode {
						state.activePanel = (state.activePanel + 1) % panelCount
					}
				case "D": // Left — prev panel / prev peer
					if state.detailMode && state.activePanel == panelPeers && state.selectedPeer > 0 {
						if state.selectedPeer > 1 {
							state.selectedPeer--
						} else {
							state.selectedPeer = state.agentCount
						}
						state.peerScrollOff = 0
					} else if !state.detailMode {
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
			// Fetch overlay peers (less frequently) — always cache for count display
			if time.Since(lastStatsFetch) > 2*time.Second || len(overlayPeersCache) == 0 {
				overlayPeersCache = fetchOverlayGeo(base)
			}
			// Only merge overlay peers into render list when showOverlay is on
			if state.showOverlay {
				peers = append(peers, overlayPeersCache...)
			}
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
			// Pulse: fetch recent echoes and mark active peers
			updatePulseActors(base, state)
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
				frame = renderTopoFrame(peers, w, h, angle, headerCache, netStats, state, overlayPeersCache)
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
	Role           string   `json:"role,omitempty"`
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
	Balance   int64    `json:"-"`
	Frozen    int64    `json:"-"`
	LocalValue string  `json:"-"`
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
	Balance     int64  `json:"balance"`
	Frozen      int64  `json:"frozen"`
	TotalEarned int64  `json:"total_earned"`
	TotalSpent  int64  `json:"total_spent"`
	LocalValue  string `json:"local_value"`
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
			ShortID:   safePrefix(p.KeyHex, 8),
			AgentName: "claw:" + safePrefix(p.KeyHex, 8),
			Location:  p.Location,
			Geo:       p.Geo,
			IsOverlay: true,
			LatencyMs: p.LatencyMs,
		}
		if d, ok := rateMap[safePrefix(p.KeyHex, 8)]; ok {
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
		stats.LocalValue = ci.LocalValue
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
			Amount    int64  `json:"amount"`
			Reason    string `json:"reason"`
			CreatedAt string `json:"created_at"`
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
				Detail: fmt.Sprintf("%s%d  %s", sign, t.Amount, t.Reason),
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

// echoEntry matches the JSON from /api/echoes.
type echoEntry struct {
	Actor     string `json:"actor"`
	Timestamp int64  `json:"ts"`
}

// updatePulseActors fetches recent echoes and marks actors as pulsing for 2 seconds.
func updatePulseActors(base string, state *topoState) {
	now := time.Now().Unix()

	// Expire old pulses
	for k, exp := range state.pulseActors {
		if now > exp {
			delete(state.pulseActors, k)
		}
	}

	resp, err := http.Get(base + "/api/echoes?limit=10")
	if err != nil {
		return
	}
	var echoes []echoEntry
	json.NewDecoder(resp.Body).Decode(&echoes)
	resp.Body.Close()

	for _, e := range echoes {
		if e.Timestamp > state.lastEchoSeenTS {
			state.pulseActors[e.Actor] = now + 2 // pulse for 2 seconds
		}
	}
	// Update high-water mark
	for _, e := range echoes {
		if e.Timestamp > state.lastEchoSeenTS {
			state.lastEchoSeenTS = e.Timestamp
		}
	}
}

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

// roleIconFor returns a compact emoji for a role ID, or empty string.
func roleIconFor(role string) string {
	switch role {
	case "worker":
		return "🔧"
	case "publisher":
		return "📢"
	case "thinker":
		return "🧠"
	case "trader":
		return "🏛️"
	case "observer":
		return "👀"
	case "lobster":
		return "🦞"
	}
	return ""
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

// peerSymbol returns a distinct character for each reputation tier (ocean ecology metaphor).
func peerSymbol(score float64) string {
	switch {
	case score >= 100:
		return "♦" // Lobster King
	case score >= 70:
		return "◆" // Lobster
	case score >= 50:
		return "●" // Shrimp
	case score >= 30:
		return "○" // Krill
	default:
		return "·" // Plankton
	}
}

// peerTierName returns the display name for a reputation tier.
func peerTierName(score float64) string {
	switch {
	case score >= 100:
		return "Lobster King"
	case score >= 70:
		return "Lobster"
	case score >= 50:
		return "Shrimp"
	case score >= 30:
		return "Krill"
	default:
		return "Plankton"
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
	statsText := fmt.Sprintf("Nodes:%d  Shell:%d  Topics:%d  v%s",
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
	role           string
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
			role:           p.Role,
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

func renderTopoFrame(peers []peerGeoData, termW, termH int, rotation float64, header string, stats networkStats, state *topoState, overlayPeersCache []peerGeoData) string {
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
		peerID     string
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
					markers = append(markers, markerPos{sx: sx, sy: sy, idx: i, isSelf: p.IsSelf, isOverlay: p.IsOverlay, reputation: p.Reputation, peerID: p.PeerID})
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
		pulsing    bool
	}
	globeMarkers := make(map[[2]int]markerCell)
	for mi, m := range markers {
		isPulsing := false
		if len(m.peerID) >= 16 {
			_, isPulsing = state.pulseActors[m.peerID[:16]]
		}
		globeMarkers[[2]int{m.sx, m.sy}] = markerCell{isSelf: m.isSelf, isOverlay: m.isOverlay, density: markerDensity[mi], reputation: m.reputation, pulsing: isPulsing}
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
						sb.WriteString(cSelf + "@" + cReset)
					} else if mc.isOverlay {
						sb.WriteString(cOverlay + "+" + cReset)
					} else if mc.pulsing {
						// Pulse: bright white-on-yellow flash for recently active peers
						sb.WriteString("\033[1;97;43m" + peerSymbol(mc.reputation) + cReset)
					} else {
						sb.WriteString(repColor(mc.reputation) + peerSymbol(mc.reputation) + cReset)
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
	agentCount := 0
	overlayCount := len(overlayPeersCache)
	for _, p := range peers {
		if !p.IsSelf && !p.IsOverlay {
			agentCount++
		}
	}
	panelNames := []string{"Self", "Peers", "Globe"}
	infraLabel := "o:Infra"
	if state.showOverlay {
		infraLabel = "o:Infra*"
	}
	help := fmt.Sprintf(" <>/Tab:[%s]  Enter:Detail  1-9:Peer  %s  q:Quit  @:You  Agents:%d  Infra:%d",
		panelNames[state.activePanel], infraLabel, agentCount, overlayCount)
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

	var sb strings.Builder
	sb.Grow(termW * termH * 4)
	sb.WriteString(header)

	pInfos := buildPeerInfos(peers)
	now := time.Now().Unix()

	// Count overlay peers for infra panel
	var overlayCount int
	for _, pi := range pInfos {
		if pi.isOverlay {
			overlayCount++
		}
	}

	// Calculate layout: when on Peers panel and there are overlay peers,
	// reserve space for a fixed infra panel at the bottom
	infraH := 0 // total lines consumed by infra box (including borders)
	infraContentH := 0
	if state.activePanel == panelPeers && overlayCount > 0 {
		// infra panel: 1 top border + content rows + 1 bottom border
		// target ~30% of terminal height, min 5 content rows, max 14
		infraContentH = termH * 30 / 100
		if infraContentH < 5 {
			infraContentH = 5
		}
		// need at most ceil(overlayCount/2) rows for two-column
		maxNeeded := (overlayCount + 1) / 2
		if infraContentH > maxNeeded {
			infraContentH = maxNeeded
		}
		if infraContentH > 14 {
			infraContentH = 14
		}
		infraH = infraContentH + 2 // +2 for top/bottom border lines
	}

	contentH := termH - 4 - infraH // header + sep + help + bottom - infra
	if contentH < 4 {
		contentH = 4
	}

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

	// ── Fixed infra panel at bottom (Peers panel only) ──
	if infraH > 0 {
		// Top border with title
		title := fmt.Sprintf(" Infrastructure ── %d relay nodes ", overlayCount)
		padLen := innerW - len([]rune(title))
		if padLen < 0 {
			padLen = 0
		}
		sb.WriteString(cBorder + "├" + cReset + cTitle + title + cBorder + strings.Repeat("─", padLen) + "┤" + cReset + "\033[K\r\n")

		// Infra content rows (two-column, detailed location, no IDs)
		infraLines := buildInfraLines(pInfos, innerW, infraContentH)
		for i := 0; i < infraContentH; i++ {
			if i < len(infraLines) {
				emitRow(&sb, infraLines[i], innerW)
			} else {
				emitRow(&sb, "", innerW)
			}
		}

		// Bottom border of infra box doubles as the separator before help
		sb.WriteString(cBorder + "├")
		sb.WriteString(strings.Repeat("─", innerW))
		sb.WriteString("┤" + cReset + "\033[K\r\n")
	} else {
		// ── Separator ──
		sb.WriteString(cBorder + "├")
		sb.WriteString(strings.Repeat("─", innerW))
		sb.WriteString("┤" + cReset + "\033[K\r\n")
	}

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
	shellLine := fmt.Sprintf("%d (frozen: %d)", stats.Balance, stats.Frozen)
	if stats.LocalValue != "" {
		shellLine += "  ≈ " + stats.LocalValue
	}
	lines = append(lines, cSelfInfo+" Shell:      "+cReset+shellLine)
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
	// Separate agents from overlay/infra peers
	var agentEntries []peerInfo
	for _, pi := range pInfos {
		if pi.isSelf || pi.isOverlay {
			continue
		}
		agentEntries = append(agentEntries, pi)
	}
	state.agentCount = len(agentEntries)

	var lines []string

	// If a specific peer was selected via number key, show that peer's detail
	if state.selectedPeer > 0 && state.selectedPeer <= len(agentEntries) {
		p := agentEntries[state.selectedPeer-1]
		// Title with reputation tier badge
		tierSym := peerSymbol(p.reputation)
		tierName := peerTierName(p.reputation)
		tierColor := repColor(p.reputation)
		displayName := "@" + p.shortID
		if p.agentName != "" {
			displayName = p.agentName
		}
		lines = append(lines, cTitle+fmt.Sprintf(" Peer #%d", state.selectedPeer)+cReset+"  "+tierColor+tierSym+" "+tierName+cReset+"  "+cPeerInfo+displayName+cReset)
		lines = append(lines, "")
		lines = append(lines, cSelf+" Peer ID:    "+cReset+p.peerID)
		lines = append(lines, cSelf+" Short ID:   "+cReset+p.shortID)
		if p.agentName != "" {
			lines = append(lines, cSelf+" Agent:      "+cReset+p.agentName)
		}
		if p.role != "" {
			roleIcon := roleIconFor(p.role)
			lines = append(lines, cSelf+" Role:       "+cReset+roleIcon+" "+p.role)
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
		lines = append(lines, cSelf+" Reputation: "+cReset+tierColor+fmt.Sprintf("%.1f  %s %s", p.reputation, tierSym, tierName)+cReset)
		if p.motto != "" {
			lines = append(lines, cSelf+" Motto:      "+cReset+p.motto)
		}
		return lines
	}

	lines = append(lines, cTitle+fmt.Sprintf(" Agents -- %d connected", len(agentEntries))+cReset)
	lines = append(lines, "")

	if len(agentEntries) == 0 {
		lines = append(lines, cDim+"  No peers connected"+cReset)
		return lines
	}

	// Build agent entry lines
	var allEntries []string
	for i, p := range agentEntries {
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

		// Peer header: number + tier badge + name
		tierSym := peerSymbol(p.reputation)
		tierColor := repColor(p.reputation)
		displayName := "@" + p.shortID
		if p.agentName != "" {
			displayName = p.agentName
		}
		allEntries = append(allEntries,
			fmt.Sprintf(cPeerInfo+"  %d. "+cReset+tierColor+"%s "+cReset+cPeerInfo+"%s"+cReset, i+1, tierSym, displayName))
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

// buildInfraLines builds two-column infra panel content (no IDs, detailed location + latency)
func buildInfraLines(pInfos []peerInfo, innerW, infraContentH int) []string {
	var overlayEntries []peerInfo
	for _, pi := range pInfos {
		if pi.isOverlay {
			overlayEntries = append(overlayEntries, pi)
		}
	}

	colW := (innerW - 3) / 2 // two columns with a gap
	if colW < 15 {
		colW = innerW - 2
	}
	twoCol := colW < innerW-5

	// Fixed-width latency field for alignment
	fmtLat := func(ms int64) string {
		if ms > 0 {
			s := fmt.Sprintf("%dms", ms)
			// Right-align in 6 chars
			if len(s) < 6 {
				return strings.Repeat(" ", 6-len(s)) + s
			}
			return s
		}
		return "   --"
	}

	formatEntry := func(p peerInfo) string {
		// Build detailed location: City, Region, Country
		var parts []string
		if p.city != "" {
			parts = append(parts, p.city)
		}
		if p.region != "" {
			parts = append(parts, p.region)
		}
		if p.country != "" {
			parts = append(parts, p.country)
		}
		loc := strings.Join(parts, ", ")
		if loc == "" {
			loc = p.location
		}
		if loc == "" || loc == "Unknown" {
			loc = "?"
		}
		locW := colW - 9 // reserve space for dot + latency
		if locW < 4 {
			locW = 4
		}
		locStr := truncStr(loc, locW)
		// Pad location to fixed width
		pad := locW - len([]rune(locStr))
		if pad < 0 {
			pad = 0
		}
		return fmt.Sprintf("· %s%s %s", locStr, strings.Repeat(" ", pad), fmtLat(p.latencyMs))
	}

	var lines []string
	if twoCol {
		for i := 0; i < len(overlayEntries); i += 2 {
			leftEntry := formatEntry(overlayEntries[i])
			left := cOverlay + " " + leftEntry + cReset
			leftVis := 1 + len([]rune(leftEntry))
			padL := colW - leftVis + 1
			if padL < 1 {
				padL = 1
			}
			if i+1 < len(overlayEntries) {
				rightEntry := formatEntry(overlayEntries[i+1])
				right := cOverlay + rightEntry + cReset
				lines = append(lines, left+strings.Repeat(" ", padL)+right)
			} else {
				lines = append(lines, left)
			}
			if len(lines) >= infraContentH {
				break
			}
		}
	} else {
		for i, p := range overlayEntries {
			lines = append(lines, cOverlay+" "+formatEntry(p)+cReset)
			if i+1 >= infraContentH {
				break
			}
		}
	}
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
	shellDisplay := fmt.Sprintf("%d (frozen: %d)", stats.Balance, stats.Frozen)
	if stats.LocalValue != "" {
		shellDisplay += "  ≈ " + stats.LocalValue
	}
	statsLines := []kv{
		{"", cTitle + "ClawNet" + cReset + " " + cDim + "v" + daemon.Version + cReset},
		{"", strings.Repeat("-", 30)},
		{"Nodes", fmt.Sprintf("%d total (%d peers + self)", totalPeers+1, totalPeers)},
		{"Shell", shellDisplay},
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

	// ── Animated wave box on left, stats on right ──
	bannerW := 0
	for _, l := range clawnetBanner {
		if len([]rune(l)) > bannerW {
			bannerW = len([]rune(l))
		}
	}
	gap := 4
	bH := len(clawnetBanner)
	boxInnerW := bannerW
	showBanner := w >= boxInnerW+2+gap+20

	// Advance wave animation state
	state.waveFrame++
	f := state.waveFrame

	if showBanner {
		// ── Box dimensions ──
		boxInnerH := len(statsLines)
		if boxInnerH < bH+6 {
			boxInnerH = bH + 6
		}

		// Center banner within the box
		bannerLeft := (boxInnerW - bannerW) / 2
		bannerTop := (boxInnerH - bH) / 2

		// ── Wave cycle: ~250 frames per full cycle ──
		cycleLen := 250
		if f%cycleLen == 0 {
			state.waveStrong = rand.Float64() < 0.35
		}
		cycleFrac := float64(f%cycleLen) / float64(cycleLen)

		// Easing
		easeIO := func(t float64) float64 {
			if t < 0.5 {
				return 2 * t * t
			}
			return -1 + (4-2*t)*t
		}

		// ── Wave keyframes ──
		calm := float64(boxInnerH) + 2.0
		gentle := float64(boxInnerH) - 3.0
		mid := float64(bannerTop) + float64(bH) - 1.0
		peak := float64(bannerTop) - 1.0

		type kf struct{ t, y float64 }
		var keys []kf
		if state.waveStrong {
			keys = []kf{
				{0.00, calm},
				{0.20, calm},
				{0.38, gentle},
				{0.50, mid},
				{0.62, gentle + 1},
				{0.76, peak},
				{0.92, calm},
				{1.00, calm},
			}
		} else {
			keys = []kf{
				{0.00, calm},
				{0.30, calm},
				{0.48, gentle},
				{0.62, mid + 2},
				{0.80, calm},
				{1.00, calm},
			}
		}

		baseY := calm
		for i := 1; i < len(keys); i++ {
			if cycleFrac <= keys[i].t {
				p := easeIO((cycleFrac - keys[i-1].t) / (keys[i].t - keys[i-1].t))
				baseY = keys[i-1].y + (keys[i].y-keys[i-1].y)*p
				break
			}
		}

		waveAt := func(col int) float64 {
			x := float64(col)
			ft := float64(f)
			return baseY +
				0.6*math.Sin(x*0.12+ft*0.08) +
				0.3*math.Sin(x*0.25+ft*0.05)
		}

		// ── Beach emojis: deposited by wave, revealed when water recedes ──
		// Emojis wash up with the wave. While water covers them they are invisible.
		// As the water recedes, emojis are revealed at the foam edge.
		// The *next* wave surge wipes the old set and may deposit new ones.
		beachItems := []rune{'🦐', '🐚', '🍂', '🍃', '🦀', '🐡'}

		// Deposit phase: place emojis while water is at/past peak (still covering them).
		// Strong wave peaks at ~0.50 and ~0.76; normal wave peaks at ~0.62.
		// We deposit right after the highest point so emojis are hidden under water.
		depositOK := false
		if state.waveStrong {
			depositOK = cycleFrac > 0.74 && cycleFrac < 0.80
		} else {
			depositOK = cycleFrac > 0.58 && cycleFrac < 0.66
		}
		if depositOK && len(state.beachEmojis) == 0 {
			n := 1 + rand.Intn(3)
			for i := 0; i < n; i++ {
				// Place emojis BELOW the current water surface so they start hidden.
				// They'll be revealed as the water recedes past their row.
				surfInt := int(baseY) + 1 // one row below surface = definitely submerged
				if surfInt < bannerTop {
					surfInt = bannerTop
				}
				// Don't go deeper than a few rows below surface (or they'd never show)
				maxRow := surfInt + 4
				if maxRow > boxInnerH-2 {
					maxRow = boxInnerH - 2
				}
				if surfInt > maxRow {
					continue
				}
				dr := surfInt + rand.Intn(maxRow-surfInt+1)
				state.beachEmojis = append(state.beachEmojis, beachEmoji{
					emoji: beachItems[rand.Intn(len(beachItems))],
					col:   rand.Intn(boxInnerW-4) + 2,
					row:   dr,
				})
			}
		}

		// Wipe phase: clear old emojis only when the *next* wave surge
		// actually rises past them (water covers their rows again).
		if len(state.beachEmojis) > 0 && cycleFrac > 0.30 && cycleFrac < 0.55 {
			allCovered := true
			for _, e := range state.beachEmojis {
				if float64(e.row) < waveAt(e.col)-0.5 {
					// This emoji is still above water — not yet covered
					allCovered = false
					break
				}
			}
			if allCovered && baseY < float64(bannerTop+bH) {
				state.beachEmojis = nil
			}
		}

		// ── Gold coin: poll daemon for visual coin state ──
		// Non-blocking check for results from previous poll
		select {
		case result := <-state.coinPollCh:
			if result != nil {
				// Update beach coins from daemon state
				if result.BeachCoins > 0 && len(result.CoinPositions) > 0 && !state.grabActive {
					state.beachCoins = nil
					for _, pos := range result.CoinPositions {
						state.beachCoins = append(state.beachCoins, beachCoin{
							row: pos[0],
							col: pos[1],
						})
					}
				} else if result.BeachCoins == 0 && !state.grabActive {
					state.beachCoins = nil
				}
				// Sync chest count from daemon (authoritative)
				if !state.grabActive {
					state.chestCoins = result.ChestCoins
				}
				// Daemon message (e.g. after redeem)
				if result.Message != "" {
					state.chestMsg = result.Message
					state.chestMsgExp = f + 40
				}
			}
		default:
		}

		// At cycle start, poll daemon for coin visual state
		if f%cycleLen == 1 && !state.grabActive {
			pollBase := state.daemonBase
			pollCh := state.coinPollCh
			go func() {
				resp, err := http.Get(pollBase + "/api/topo/coin-state")
				if err != nil {
					return
				}
				defer resp.Body.Close()
				var result coinVisualResult
				if json.NewDecoder(resp.Body).Decode(&result) == nil {
					select {
					case pollCh <- &result:
					default:
					}
				}
			}()
		}

		// ── Grab animation progress ──
		if state.grabActive {
			state.grabFrame++
			if state.grabFrame >= state.grabTotal {
				state.grabActive = false
				state.chestCoins += state.grabCount
				state.chestMsg = coinMessages[rand.Intn(len(coinMessages))]
				state.chestMsgExp = f + 40
			}
		}

		// Clear expired chest message
		if state.chestMsg != "" && f >= state.chestMsgExp {
			state.chestMsg = ""
		}

		// ── Banner character map ──
		type bxy struct{ r, c int }
		bmap := make(map[bxy]rune)
		for i, line := range clawnetBanner {
			for j, ch := range line {
				if ch != ' ' {
					bmap[bxy{i, j}] = ch
				}
			}
		}

		// ── Char displacement ──
		if state.displacedMap == nil {
			state.displacedMap = make(map[[2]int]int)
		}
		bannerBot := float64(bannerTop + bH - 1)
		if baseY < bannerBot && cycleFrac > 0.3 && cycleFrac < 0.8 {
			if f%7 == 0 && len(state.displacedMap) < 12 {
				waterRow := int(baseY) - bannerTop
				if waterRow < 0 {
					waterRow = 0
				}
				if waterRow >= bH {
					waterRow = bH - 1
				}
				for attempt := 0; attempt < 5; attempt++ {
					tr := waterRow + rand.Intn(3) - 1
					if tr < 0 || tr >= bH {
						continue
					}
					tc := rand.Intn(bannerW)
					key := [2]int{tr, tc}
					if _, ok := bmap[bxy{tr, tc}]; ok {
						if _, already := state.displacedMap[key]; !already {
							state.displacedMap[key] = -(1 + rand.Intn(2))
							state.displaceExp = f + 40 + rand.Intn(60)
							break
						}
					}
				}
			}
		}
		if baseY >= calm-1 && len(state.displacedMap) > 0 {
			if f >= state.displaceExp {
				for k := range state.displacedMap {
					delete(state.displacedMap, k)
					break
				}
				state.displaceExp = f + 3
			}
		}

		dispScreen := make(map[bxy]rune)
		dispOrigSkip := make(map[bxy]bool)
		for key, dy := range state.displacedMap {
			origR, origC := key[0], key[1]
			if ch, ok := bmap[bxy{origR, origC}]; ok {
				newR := bannerTop + origR + dy
				if newR >= 0 && newR < boxInnerH {
					dispScreen[bxy{newR, bannerLeft + origC}] = ch
				}
				dispOrigSkip[bxy{origR, origC}] = true
			}
		}

		// ── Build emoji position set for rendering ──
		// Beach emojis are only visible if their fixed row is ABOVE the current
		// water surface (i.e. the water has receded past them).
		type emojiPos struct {
			emoji rune
			row   int
			col   int
		}
		var emojiRender []emojiPos
		for _, e := range state.beachEmojis {
			// Only render if the water surface at this column is below the emoji's row
			if float64(e.row) < waveAt(e.col)-0.5 {
				emojiRender = append(emojiRender, emojiPos{e.emoji, e.row, e.col})
			}
		}

		// Gold coins on beach — rendered as gold-colored $ (safe ASCII, no layout risk)
		coinCells := make(map[int][]int) // row → list of cols with coins
		for _, bc := range state.beachCoins {
			coinCells[bc.row] = append(coinCells[bc.row], bc.col)
		}

		// Grab animation: compute coin position on parabolic arc
		var grabPos *emojiPos
		if state.grabActive {
			t := float64(state.grabFrame) / float64(state.grabTotal)
			endR := boxInnerH - 2
			endC := boxInnerW - 4
			ctrlR := float64(state.grabStartR) - 5
			ctrlC := float64(state.grabStartC+endC) / 2.0
			r := (1-t)*(1-t)*float64(state.grabStartR) + 2*(1-t)*t*ctrlR + t*t*float64(endR)
			c := (1-t)*(1-t)*float64(state.grabStartC) + 2*(1-t)*t*ctrlC + t*t*float64(endC)
			grabPos = &emojiPos{'$', int(r), int(c)}
		}

		// ── Color palette ──
		wBg := [5]string{
			"\033[48;2;190;42;55m",
			"\033[48;2;145;30;40m",
			"\033[48;2;100;22;30m",
			"\033[48;2;60;14;20m",
			"\033[48;2;32;8;12m",
		}
		foamC := "\033[1;38;2;250;130;140m\033[48;2;170;40;50m"
		sprayC := "\033[38;2;140;50;55m\033[49m"
		sandC := "\033[38;2;194;154;108m\033[49m"
		sandDotC := "\033[38;2;160;125;85m\033[49m"
		banC := cBanner + "\033[49m"
		banDimC := "\033[38;2;180;80;90m\033[49m"
		clrC := "\033[0m"

		gapStr := strings.Repeat(" ", gap)

		// ── Right side: stats + treasure chest ──
		// Build right-side lines: stats first, then chest if coins > 0 or coin on beach
		var rightLines []string
		for _, s := range statsLines {
			if s.k != "" {
				rightLines = append(rightLines, gapStr+cSelf+s.k+cReset+": "+s.v)
			} else if s.v != "" {
				rightLines = append(rightLines, gapStr+s.v)
			} else {
				rightLines = append(rightLines, "")
			}
		}

		// Treasure chest (show when coins > 0 or coins on beach or grab active)
		goldFg := "\033[1;38;2;255;215;0m"
		showChest := state.chestCoins > 0 || len(state.beachCoins) > 0 || state.grabActive
		if showChest {
			var chestLines []string
			chestW := 22
			chestBdr := cBorder
			chestGap := gapStr + "      " // extra indent to push chest right
			chestLines = append(chestLines, chestGap+chestBdr+"\u250c"+strings.Repeat("\u2500", chestW)+"\u2510"+cReset)
			// Coin display: single row with $ symbols
			coinStr := ""
			for i := 0; i < 10; i++ {
				if i < state.chestCoins {
					coinStr += goldFg + "$" + cReset
				} else {
					coinStr += cDim + "." + cReset
				}
			}
			coinPad := chestW - 2 - 10 // 10 visible coin/dot chars, 2 for side spaces
			chestLines = append(chestLines, chestGap+chestBdr+"\u2502"+cReset+" "+coinStr+strings.Repeat(" ", coinPad)+" "+chestBdr+"\u2502"+cReset)
			// Status line
			statusStr := fmt.Sprintf("%d/10 coins", state.chestCoins)
			statPad := chestW - 2 - len(statusStr)
			if statPad < 0 {
				statPad = 0
			}
			chestLines = append(chestLines, chestGap+chestBdr+"\u2502"+cReset+" "+cTitle+statusStr+cReset+strings.Repeat(" ", statPad)+" "+chestBdr+"\u2502"+cReset)
			// Action line
			var actionStr string
			if len(state.beachCoins) > 0 && state.chestCoins < 10 {
				actionStr = cSelf + "Enter" + cReset + ": Grab!"
			} else if state.chestCoins > 0 {
				actionStr = cSelf + "c" + cReset + ": \u2192 Shell"
			}
			if actionStr != "" {
				pureLen := 0
				inEsc := false
				for _, ch := range actionStr {
					if ch == '\033' {
						inEsc = true
					} else if inEsc && ch == 'm' {
						inEsc = false
					} else if !inEsc {
						pureLen++
					}
				}
				actPad := chestW - 2 - pureLen
				if actPad < 0 {
					actPad = 0
				}
				chestLines = append(chestLines, chestGap+chestBdr+"\u2502"+cReset+" "+actionStr+strings.Repeat(" ", actPad)+" "+chestBdr+"\u2502"+cReset)
			} else {
				chestLines = append(chestLines, chestGap+chestBdr+"\u2502"+cReset+strings.Repeat(" ", chestW)+chestBdr+"\u2502"+cReset)
			}
			chestLines = append(chestLines, chestGap+chestBdr+"\u2514"+strings.Repeat("\u2500", chestW)+"\u2518"+cReset)

			// Message below chest
			var msgLines []string
			if state.chestMsg != "" {
				msgLines = append(msgLines, chestGap+"  "+cDim+state.chestMsg+cReset)
				if state.chestCoins > 0 {
					msgLines = append(msgLines, chestGap+"  "+cDim+"Press "+cReset+cSelf+"c"+cReset+cDim+" to convert \u2192 Shell"+cReset)
				}
			}

			// Position chest at bottom-right of the box — push 2 rows
			// below the box bottom border so it sits further down
			chestStart := boxInnerH + 4 - len(chestLines)
			if chestStart < len(rightLines)+2 {
				chestStart = len(rightLines) + 2
			}
			for len(rightLines) < chestStart {
				rightLines = append(rightLines, "")
			}
			rightLines = append(rightLines, chestLines...)
			rightLines = append(rightLines, msgLines...)
		}

		fmtRight := func(idx int) string {
			if idx < 0 || idx >= len(rightLines) {
				return ""
			}
			return rightLines[idx]
		}

		lines = append(lines, "")

		// ── Top border ──
		lines = append(lines, " "+cBorder+"┌"+strings.Repeat("─", boxInnerW)+"┐"+cReset+fmtRight(0))

		// ── Inner rows ──
		for row := 0; row < boxInnerH; row++ {
			var cellSB strings.Builder
			cellSB.Grow(boxInnerW * 16)
			last := ""

			// Check if any emoji should render on this row
			emojiCols := make(map[int]rune)
			for _, ep := range emojiRender {
				if ep.row == row {
					emojiCols[ep.col] = ep.emoji
				}
			}
			// Coin and grab use special gold rendering
			coinColSet := make(map[int]bool)
			for _, cc := range coinCells[row] {
				coinColSet[cc] = true
			}
			grabCol := -1
			if grabPos != nil && grabPos.row == row {
				grabCol = grabPos.col
			}

			goldC := "\033[1;38;2;255;215;0m\033[49m" // bright gold on default bg

			for col := 0; col < boxInnerW; col++ {
				sy := waveAt(col)
				fr := float64(row)

				// Gold coin: render as single $ in gold color
				if coinColSet[col] || col == grabCol {
					if goldC != last {
						cellSB.WriteString(goldC)
						last = goldC
					}
					cellSB.WriteByte('$')
					continue
				}

				// Check for emoji at this position (always renders on top)
				if em, ok := emojiCols[col]; ok {
					if clrC != last {
						cellSB.WriteString(clrC)
						last = clrC
					}
					cellSB.WriteString(string(em))
					col++ // emoji is 2 cells wide
					continue
				}

				if fr > sy+0.5 {
					// ── Underwater ──
					depth := fr - sy
					idx := 4
					switch {
					case depth < 2:
						idx = 0
					case depth < 4:
						idx = 1
					case depth < 7:
						idx = 2
					case depth < 11:
						idx = 3
					}
					c := wBg[idx]
					if c != last {
						cellSB.WriteString(c)
						last = c
					}
					cellSB.WriteByte(' ')

				} else if fr > sy-0.5 {
					// ── Foam crest ──
					if foamC != last {
						cellSB.WriteString(foamC)
						last = foamC
					}
					if (col+f)%4 == 0 {
						cellSB.WriteByte('=')
					} else {
						cellSB.WriteByte('~')
					}

				} else {
					// ── Above water ──
					bannerR := row - bannerTop
					bannerC := col - bannerLeft

					if dch, ok := dispScreen[bxy{row, col}]; ok {
						if banDimC != last {
							cellSB.WriteString(banDimC)
							last = banDimC
						}
						cellSB.WriteRune(dch)
						continue
					}

					if bannerR >= 0 && bannerR < bH && bannerC >= 0 && bannerC < bannerW {
						if dispOrigSkip[bxy{bannerR, bannerC}] {
							if clrC != last {
								cellSB.WriteString(clrC)
								last = clrC
							}
							cellSB.WriteByte(' ')
							continue
						}
						if ch, ok := bmap[bxy{bannerR, bannerC}]; ok {
							if banC != last {
								cellSB.WriteString(banC)
								last = banC
							}
							cellSB.WriteRune(ch)
							continue
						}
					}

					if fr > sy-2.0 && (col*17+f*3)%7 < 2 {
						if sprayC != last {
							cellSB.WriteString(sprayC)
							last = sprayC
						}
						cellSB.WriteByte('.')
					} else if fr < float64(bannerTop) && (col*7+row*13+17)%11 < 2 {
						if sandDotC != last {
							cellSB.WriteString(sandDotC)
							last = sandDotC
						}
						cellSB.WriteByte('.')
					} else if fr < float64(bannerTop) && (col*11+row*3+7)%15 < 1 {
						if sandC != last {
							cellSB.WriteString(sandC)
							last = sandC
						}
						cellSB.WriteByte(',')
					} else {
						if clrC != last {
							cellSB.WriteString(clrC)
							last = clrC
						}
						cellSB.WriteByte(' ')
					}
				}
			}
			cellSB.WriteString(cReset)

			lines = append(lines, " "+cBorder+"│"+cReset+cellSB.String()+cBorder+"│"+cReset+fmtRight(row+1))
		}

		// ── Bottom border ──
		lines = append(lines, " "+cBorder+"└"+strings.Repeat("─", boxInnerW)+"┘"+cReset+fmtRight(boxInnerH+1))

		// Remaining right-side lines below the box
		padLeft := strings.Repeat(" ", boxInnerW+3)
		for i := boxInnerH + 2; i < len(rightLines); i++ {
			lines = append(lines, padLeft+rightLines[i])
		}
	} else {
		lines = append(lines, "")
		for i := 0; i < len(statsLines); i++ {
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

	// Reputation legend — fixed-width entries for alignment
	fmtTier := func(rep float64, name string, width int) string {
		s := repColor(rep) + peerSymbol(rep) + cReset + " " + name
		vis := visibleLen(s)
		if pad := width - vis; pad > 0 {
			s += strings.Repeat(" ", pad)
		}
		return s
	}
	lines = append(lines, "")
	lines = append(lines, " "+cTitle+"Reputation"+cReset)
	lines = append(lines, "   "+fmtTier(20, "Plankton", 12)+fmtTier(40, "Krill", 12)+fmtTier(60, "Shrimp", 12)+fmtTier(80, "Lobster", 12)+fmtTier(120, "King", 12))

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
		shellInfo := fmt.Sprintf("  Shell: %d (frozen: %d)", stats.Balance, stats.Frozen)
		if stats.LocalValue != "" {
			shellInfo += "  ≈ " + stats.LocalValue
		}
		lines = append(lines, shellInfo)
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

	// Separate agents from overlay/infra peers
	var agentEntries, overlayEntries []peerInfo
	for _, pi := range pInfos {
		if pi.isSelf {
			continue
		}
		if pi.isOverlay {
			overlayEntries = append(overlayEntries, pi)
		} else {
			agentEntries = append(agentEntries, pi)
		}
	}

	peerLines := make([]string, bottomH)
	for i := range peerLines {
		peerLines[i] = strings.Repeat(" ", peerW)
	}

	usedLines := 0

	// Agent cards (full 4-line cards)
	if len(agentEntries) > 0 {
		maxCards := cols * cardRows
		if maxCards > len(agentEntries) {
			maxCards = len(agentEntries)
		}

		type cardData struct {
			lines [4]string
		}
		allCards := make([]cardData, maxCards)
		for ci := 0; ci < maxCards; ci++ {
			p := agentEntries[ci]
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

			// Show nickname (or @shortID) with tier symbol + role icon prefix
			displayName := "@" + p.shortID
			if p.agentName != "" {
				displayName = p.agentName
			}
			roleIcon := roleIconFor(p.role)
			tierSym := peerSymbol(p.reputation) + " "
			prefix := tierSym + roleIcon
			idLine := " " + prefix + truncStr(displayName, insW-2-len([]rune(prefix)))
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
				usedLines = lineIdx + 1
			}
		}
	}

	// Overlay/infra peers — two-column abbreviated entries below agent cards (no IDs)
	if len(overlayEntries) > 0 && usedLines < bottomH {
		// Separator line
		if usedLines > 0 && usedLines < bottomH {
			sep := fmt.Sprintf(" -- Infra (%d nodes) ", len(overlayEntries))
			padLen := peerW - len(sep)
			if padLen < 0 {
				padLen = 0
			}
			peerLines[usedLines] = sep + strings.Repeat("-", padLen)
			usedLines++
		}
		colW := (peerW - 2) / 2
		if colW < 12 {
			colW = peerW - 1
		}
		twoCol := colW < peerW-4
		for i := 0; i < len(overlayEntries) && usedLines < bottomH; i++ {
			fmtInfra := func(p peerInfo) string {
				var parts []string
				if p.city != "" {
					parts = append(parts, p.city)
				}
				if p.region != "" {
					parts = append(parts, p.region)
				}
				if p.country != "" {
					parts = append(parts, p.country)
				}
				loc := strings.Join(parts, ", ")
				if loc == "" {
					loc = p.location
				}
				if loc == "" || loc == "Unknown" {
					loc = "?"
				}
				latStr := "   --"
				if p.latencyMs > 0 {
					s := fmt.Sprintf("%dms", p.latencyMs)
					if len(s) < 6 {
						latStr = strings.Repeat(" ", 6-len(s)) + s
					} else {
						latStr = s
					}
				}
				locW := colW - 10
				if locW < 4 {
					locW = 4
				}
				locStr := truncStr(loc, locW)
				pad := locW - len([]rune(locStr))
				if pad < 0 {
					pad = 0
				}
				return fmt.Sprintf(" · %s%s %s", locStr, strings.Repeat(" ", pad), latStr)
			}
			if twoCol && i+1 < len(overlayEntries) {
				left := fmtInfra(overlayEntries[i])
				right := fmtInfra(overlayEntries[i+1])
				combined := left + right
				pad := peerW - len([]rune(combined))
				if pad < 0 {
					pad = 0
				}
				peerLines[usedLines] = combined + strings.Repeat(" ", pad)
				i++ // skip next
			} else {
				line := fmtInfra(overlayEntries[i])
				pad := peerW - len([]rune(line))
				if pad < 0 {
					pad = 0
				}
				peerLines[usedLines] = line + strings.Repeat(" ", pad)
			}
			usedLines++
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
		return fmt.Errorf(i18n.T("err.daemon_connect")+": %w", err)
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
		return fmt.Errorf(i18n.T("err.daemon_connect")+": %w", err)
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

// ── Watch command (live event stream) ──

func cmdWatch() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)

	red := "\033[38;2;230;57;70m"
	coral := "\033[38;2;247;127;0m"
	tidal := "\033[38;2;69;123;157m"
	green := "\033[32m"
	dim := "\033[2m"
	rst := "\033[0m"

	fmt.Println(red + "  🦞 ClawNet Watch" + rst)
	fmt.Println(dim + "  Live event stream — Ctrl+C to stop" + rst)
	fmt.Println()

	resp, err := http.Get(base + "/api/watch")
	if err != nil {
		return fmt.Errorf(i18n.T("err.daemon_connect")+": %w", err)
	}
	defer resp.Body.Close()

	// Handle Ctrl+C gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		resp.Body.Close()
	}()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := line[6:]
		var ev struct {
			ID        int64  `json:"id"`
			Type      string `json:"type"`
			Actor     string `json:"actor"`
			Target    string `json:"target"`
			Detail    string `json:"detail"`
			CreatedAt string `json:"created_at"`
		}
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue
		}

		// Format timestamp
		ts := ev.CreatedAt
		if t, err := time.Parse(time.RFC3339, ev.CreatedAt); err == nil {
			ts = t.Local().Format("15:04:05")
		}

		// Pick icon + color by event type
		icon := "  "
		color := dim
		switch ev.Type {
		case "milestone_completed":
			icon = "🏆"
			color = coral
		case "achievement_unlocked":
			icon = "⭐"
			color = green
		case "task_created":
			icon = "📋"
			color = tidal
		case "task_claimed", "task_completed":
			icon = "✅"
			color = green
		case "task_approved":
			icon = "💰"
			color = coral
		case "knowledge_published":
			icon = "📖"
			color = tidal
		case "topic_message":
			icon = "💬"
			color = tidal
		case "swarm_contribution":
			icon = "🐝"
			color = coral
		case "tutorial_completed":
			icon = "🎉"
			color = green
		default:
			icon = "·"
			color = dim
		}

		// Actor short ID
		actor := ev.Actor
		if len(actor) > 12 {
			actor = actor[:12] + "…"
		}

		fmt.Printf("  %s%s%s %s %s%s%s  %s%s%s\n",
			dim, ts, rst, icon, color, ev.Detail, rst, dim, actor, rst)
	}

	fmt.Println()
	fmt.Println(dim + "  Stream ended." + rst)
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
		return fmt.Errorf(i18n.T("err.daemon_connect")+": %w", err)
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
