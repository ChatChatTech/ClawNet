package cli

import (
	"bufio"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/daemon"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/identity"
)

// ── Startup animation ──

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type startPhase struct {
	label    string
	duration time.Duration // min display time
}

var phases = []startPhase{
	{"Initializing identity", 400 * time.Millisecond},
	{"Loading configuration", 300 * time.Millisecond},
	{"Starting P2P network", 500 * time.Millisecond},
	{"Connecting to peers", 600 * time.Millisecond},
	{"Opening data store", 300 * time.Millisecond},
	{"Launching daemon", 400 * time.Millisecond},
}

// typewriterLine prints text one character at a time.
func typewriterLine(text string, baseDelay time.Duration) {
	for _, r := range text {
		fmt.Print(string(r))
		jitter := time.Duration(rand.Int63n(int64(baseDelay)))
		time.Sleep(baseDelay/2 + jitter)
	}
	fmt.Println()
}

// spinPhase shows a spinner for a phase, then marks it done.
func spinPhase(label string, minDur time.Duration) {
	dim := "\033[2m"
	green := "\033[32m"
	rst := "\033[0m"
	start := time.Now()
	i := 0
	for time.Since(start) < minDur {
		frame := spinFrames[i%len(spinFrames)]
		fmt.Printf("\r  %s%s%s %s", dim, frame, rst, label)
		time.Sleep(80 * time.Millisecond)
		i++
	}
	fmt.Printf("\r  %s✓%s %s\033[K\n", green, rst, label)
}

// isInitialized checks if clawnet has been initialized (identity.key exists).
func isInitialized() bool {
	dataDir := config.DataDir()
	_, err := os.Stat(filepath.Join(dataDir, "identity.key"))
	return err == nil
}

// LogFile returns the path to the daemon log file.
func LogFile() string {
	return filepath.Join(config.DataDir(), "logs", "daemon.log")
}

// silentInit performs init without verbose output.
func silentInit() error {
	dataDir := config.DataDir()
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
	if _, err := identity.LoadOrGenerate(dataDir); err != nil {
		return fmt.Errorf("identity: %w", err)
	}
	cfgPath := config.ConfigPath()
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		cfg := config.DefaultConfig()
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
	}
	return nil
}

// startDaemonBackground launches the daemon as a detached background process
// with stdout/stderr redirected to the log file.
func startDaemonBackground() error {
	logPath := LogFile()
	os.MkdirAll(filepath.Dir(logPath), 0700)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	self, err := os.Executable()
	if err != nil {
		logFile.Close()
		return fmt.Errorf("resolve executable: %w", err)
	}

	args := []string{"start", "--daemon"}
	if devBuild && len(devLayers) > 0 {
		args = append(args, "--dev-layers="+strings.Join(devLayers, ","))
	}

	cmd := exec.Command(self, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = os.Environ()
	// Detach from parent process group
	setSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("start daemon: %w", err)
	}

	// Don't wait — let it run independently
	go func() {
		cmd.Wait()
		logFile.Close()
	}()

	return nil
}

// waitForDaemon polls the API until the daemon is responsive or timeout.
func waitForDaemon(timeout time.Duration) bool {
	cfg, _ := config.Load()
	port := 3998
	if cfg != nil {
		port = cfg.WebUIPort
	}
	deadline := time.Now().Add(timeout)
	client := http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/status", port))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return true
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

// ensureDaemon makes sure init has happened and the daemon is running.
// If not, it auto-initializes and starts the daemon in background with a progress animation.
// Returns true if the daemon was just started (caller may want to wait a bit).
func ensureDaemon() (justStarted bool, err error) {
	if isDaemonRunning() {
		return false, nil
	}

	red := "\033[38;2;230;57;70m"
	rst := "\033[0m"
	dim := "\033[2m"
	green := "\033[32m"

	needsInit := !isInitialized()

	if needsInit {
		fmt.Println()
		typewriterLine(red+"  🦞 Welcome to ClawNet"+rst, 30*time.Millisecond)
		fmt.Println()
		spinPhase(phases[0].label, phases[0].duration)
		if err := silentInit(); err != nil {
			return false, fmt.Errorf("auto-init failed: %w", err)
		}
		spinPhase(phases[1].label, phases[1].duration)
	} else {
		fmt.Println()
		fmt.Printf("  %s🦞 Starting ClawNet daemon...%s\n", dim, rst)
		fmt.Println()
	}

	// Start daemon in background
	if needsInit {
		spinPhase(phases[2].label, phases[2].duration)
	}
	if err := startDaemonBackground(); err != nil {
		return false, err
	}

	if needsInit {
		spinPhase(phases[3].label, phases[3].duration)
		spinPhase(phases[4].label, phases[4].duration)
	}

	// Wait for daemon to become responsive
	label := "Waiting for daemon"
	if needsInit {
		label = phases[5].label
	}

	i := 0
	deadline := time.Now().Add(90 * time.Second) // PoW can take ~45s
	cfg, _ := config.Load()
	port := 3998
	if cfg != nil {
		port = cfg.WebUIPort
	}
	client := http.Client{Timeout: 500 * time.Millisecond}
	ready := false
	for time.Now().Before(deadline) {
		frame := spinFrames[i%len(spinFrames)]
		fmt.Printf("\r  %s%s%s %s", dim, frame, rst, label)
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/status", port))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(300 * time.Millisecond)
		i++
	}

	if ready {
		fmt.Printf("\r  %s✓%s %s\033[K\n", green, rst, label)
	} else {
		fmt.Printf("\r  %s…%s %s %s(still starting, check: clawnet log)%s\033[K\n",
			dim, rst, label, dim, rst)
	}

	fmt.Println()
	if needsInit {
		fmt.Printf("  %sDaemon running in background. Logs: clawnet log%s\n", dim, rst)
	} else {
		fmt.Printf("  %sDaemon started. Logs: clawnet log%s\n", dim, rst)
	}
	fmt.Println()

	return true, nil
}

// needsDaemon returns true if the command requires a running daemon.
func needsDaemon(cmd string) bool {
	switch cmd {
	case "i", "init", "v", "version", "help", "update",
		"export", "import", "nuke", "nut", "nutshell",
		"geo-upgrade", "up", "start", "log", "logs":
		return false
	default:
		return true
	}
}

// ── Log command ──

// cmdLog shows daemon log output. Supports levels:
//   - clawnet log          → summary (milestones only)
//   - clawnet log -v       → verbose (full log)
//   - clawnet log -f       → follow (tail -f)
//   - clawnet log --clear  → clear log file
func cmdLog() error {
	logPath := LogFile()

	// Handle subcommands
	follow := false
	verbose := false
	clear := false
	for _, arg := range os.Args[2:] {
		switch arg {
		case "-f", "--follow":
			follow = true
		case "-v", "--verbose":
			verbose = true
		case "--clear":
			clear = true
		}
	}

	if clear {
		if err := os.Truncate(logPath, 0); err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No log file found.")
				return nil
			}
			return err
		}
		fmt.Println("Log cleared.")
		return nil
	}

	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No log file yet. Start the daemon first.")
			return nil
		}
		return err
	}
	defer f.Close()

	if follow {
		return tailFollow(f, verbose)
	}

	// Read and filter
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if verbose || isMilestoneLine(line) {
			fmt.Println(line)
		}
	}
	return scanner.Err()
}

// isMilestoneLine returns true for important log lines (summary level).
// Filters out noisy connection attempts, discovery chatter, etc.
func isMilestoneLine(line string) bool {
	// Always show milestone markers
	milestones := []string{
		"ClawNet Daemon",
		"Peer ID:",
		"API server:",
		"Node is running",
		"Shutting down",
		"[PoW]",
		"[overlay] transport started",
		"[overlay] public key:",
		"[overlay] listening on",
		"[overlay] TCP listener started",
		"[crypto]",
		"Geo database:",
		"STUN detected",
		"Discovery layers:",
		"warning:",
		"error:",
		"fatal:",
		"Overlay IPv6",
		"version",
		daemon.Version,
	}
	for _, m := range milestones {
		if strings.Contains(line, m) {
			return true
		}
	}
	// Skip noisy lines
	noisy := []string{
		"Listening on:",
		"enabled",
		"SKIPPED",
		"Bootstrap:",
		"Announce:",
		"connected outbound",
		"connected inbound",
	}
	for _, n := range noisy {
		if strings.Contains(line, n) {
			return false
		}
	}
	return false
}

// tailFollow implements tail -f behavior on the log file.
func tailFollow(f *os.File, verbose bool) error {
	// Seek to end
	f.Seek(0, io.SeekEnd)

	dim := "\033[2m"
	rst := "\033[0m"
	fmt.Printf("%s[following %s — Ctrl+C to stop]%s\n", dim, LogFile(), rst)

	scanner := bufio.NewScanner(f)
	for {
		for scanner.Scan() {
			line := scanner.Text()
			if verbose || isMilestoneLine(line) {
				fmt.Println(line)
			}
		}
		time.Sleep(200 * time.Millisecond)
		// Reset scanner to try reading more
		scanner = bufio.NewScanner(f)
	}
}
