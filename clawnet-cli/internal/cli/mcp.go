package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/i18n"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/mcp"
)

func cmdMCP() error {
	args := os.Args[2:]

	if len(args) == 0 {
		mcpHelp()
		return nil
	}

	for _, a := range args {
		if a == "-h" || a == "--help" || a == "help" {
			mcpHelp()
			return nil
		}
	}

	sub := args[0]
	switch sub {
	case "start":
		return cmdMCPStart()
	case "install":
		return cmdMCPInstall(args[1:])
	case "config":
		return cmdMCPConfig(args[1:])
	default:
		fmt.Fprintf(os.Stderr, i18n.Tf("mcp.unknown_sub", sub))
		mcpHelp()
		return nil
	}
}

// cmdMCPStart runs the MCP server on stdio (for IDE integration).
func cmdMCPStart() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)

	server := mcp.NewServer(baseURL)
	return server.Run()
}

// cmdMCPInstall writes MCP configuration for the specified editor.
func cmdMCPInstall(args []string) error {
	editor := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--editor", "-e":
			if i+1 < len(args) {
				i++
				editor = args[i]
			}
		default:
			if !strings.HasPrefix(args[i], "-") && editor == "" {
				editor = args[i]
			}
		}
	}

	if editor == "" {
		fmt.Println(i18n.T("mcp.install_pick"))
		fmt.Println("  clawnet mcp install cursor")
		fmt.Println("  clawnet mcp install vscode")
		fmt.Println("  clawnet mcp install claude")
		fmt.Println("  clawnet mcp install windsurf")
		return nil
	}

	clawnetBin := clawnetBinaryPath()

	switch strings.ToLower(editor) {
	case "cursor":
		return installMCPCursor(clawnetBin)
	case "vscode", "code":
		return installMCPVSCode(clawnetBin)
	case "claude", "claude-code":
		return installMCPClaude(clawnetBin)
	case "windsurf":
		return installMCPWindsurf(clawnetBin)
	default:
		fmt.Fprintf(os.Stderr, i18n.Tf("mcp.unknown_editor", editor))
		return nil
	}
}

// cmdMCPConfig prints the MCP configuration JSON for manual setup.
func cmdMCPConfig(args []string) error {
	clawnetBin := clawnetBinaryPath()

	mcpConfig := map[string]any{
		"mcpServers": map[string]any{
			"clawnet": map[string]any{
				"command": clawnetBin,
				"args":    []string{"mcp", "start"},
			},
		},
	}

	if JSONOutput {
		data, _ := json.MarshalIndent(mcpConfig, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	bold := c("1")
	dim := c("2")
	rst := c("0")

	fmt.Println(bold + "🦞 MCP Configuration" + rst)
	fmt.Println()
	fmt.Println(dim + "Add this to your editor's MCP config:" + rst)
	fmt.Println()
	data, _ := json.MarshalIndent(mcpConfig, "", "  ")
	fmt.Println(string(data))
	return nil
}

func clawnetBinaryPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "clawnet"
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe
	}
	return resolved
}

func installMCPCursor(binPath string) error {
	// Cursor uses .cursor/mcp.json in workspace or ~/.cursor/mcp.json globally
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".cursor")
	return installMCPGeneric(binPath, configDir, "mcp.json", "Cursor")
}

func installMCPVSCode(binPath string) error {
	home, _ := os.UserHomeDir()
	var configDir string
	switch runtime.GOOS {
	case "darwin":
		configDir = filepath.Join(home, "Library", "Application Support", "Code", "User")
	case "linux":
		configDir = filepath.Join(home, ".config", "Code", "User")
	default:
		configDir = filepath.Join(home, ".config", "Code", "User")
	}
	return installMCPGeneric(binPath, configDir, "mcp.json", "VS Code")
}

func installMCPClaude(binPath string) error {
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".claude")
	return installMCPGeneric(binPath, configDir, "mcp_servers.json", "Claude Code")
}

func installMCPWindsurf(binPath string) error {
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".windsurf")
	return installMCPGeneric(binPath, configDir, "mcp.json", "Windsurf")
}

func installMCPGeneric(binPath, configDir, filename, editorName string) error {
	os.MkdirAll(configDir, 0755)
	configPath := filepath.Join(configDir, filename)

	// Read existing config or start fresh
	existing := make(map[string]any)
	if data, err := os.ReadFile(configPath); err == nil {
		json.Unmarshal(data, &existing)
	}

	// Get or create mcpServers
	servers, ok := existing["mcpServers"].(map[string]any)
	if !ok {
		servers = make(map[string]any)
	}

	// Add/update clawnet entry
	servers["clawnet"] = map[string]any{
		"command": binPath,
		"args":    []string{"mcp", "start"},
	}
	existing["mcpServers"] = servers

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return err
	}

	bold := c("1")
	green := "\033[32m"
	rst := c("0")
	dim := c("2")

	fmt.Printf(green+"✓"+rst+" "+bold+i18n.Tf("mcp.installed", editorName)+rst+"\n")
	fmt.Printf(dim+"  Config: %s"+rst+"\n", configPath)
	fmt.Printf(dim+"  Binary: %s"+rst+"\n", binPath)
	fmt.Println()
	fmt.Println(dim + i18n.T("mcp.restart_hint") + rst)
	return nil
}

func mcpHelp() {
	bold := c("1")
	dim := c("2")
	rst := c("0")

	fmt.Println(bold + "🦞 ClawNet MCP Server" + rst)
	fmt.Println(dim + i18n.T("mcp.desc") + rst)
	fmt.Println()
	fmt.Println(bold + i18n.T("usage") + rst)
	fmt.Println("  clawnet mcp start                      " + dim + i18n.T("mcp.help_start") + rst)
	fmt.Println("  clawnet mcp install <editor>            " + dim + i18n.T("mcp.help_install") + rst)
	fmt.Println("  clawnet mcp config                      " + dim + i18n.T("mcp.help_config") + rst)
	fmt.Println()
	fmt.Println(bold + i18n.T("mcp.editors") + ":" + rst)
	fmt.Println("  cursor    " + dim + "Cursor IDE" + rst)
	fmt.Println("  vscode    " + dim + "Visual Studio Code" + rst)
	fmt.Println("  claude    " + dim + "Claude Code (CLI)" + rst)
	fmt.Println("  windsurf  " + dim + "Windsurf IDE" + rst)
	fmt.Println()
	fmt.Println(bold + i18n.T("mcp.tools") + ":" + rst)
	fmt.Println("  knowledge_search    " + dim + i18n.T("mcp.tool.knowledge_search") + rst)
	fmt.Println("  knowledge_publish   " + dim + i18n.T("mcp.tool.knowledge_publish") + rst)
	fmt.Println("  task_create         " + dim + i18n.T("mcp.tool.task_create") + rst)
	fmt.Println("  task_list           " + dim + i18n.T("mcp.tool.task_list") + rst)
	fmt.Println("  task_show           " + dim + i18n.T("mcp.tool.task_show") + rst)
	fmt.Println("  task_claim          " + dim + i18n.T("mcp.tool.task_claim") + rst)
	fmt.Println("  reputation_query    " + dim + i18n.T("mcp.tool.reputation_query") + rst)
	fmt.Println("  agent_discover      " + dim + i18n.T("mcp.tool.agent_discover") + rst)
	fmt.Println("  network_status      " + dim + i18n.T("mcp.tool.network_status") + rst)
	fmt.Println("  credits_balance     " + dim + i18n.T("mcp.tool.credits_balance") + rst)
	fmt.Println("  chat_send           " + dim + i18n.T("mcp.tool.chat_send") + rst)
	fmt.Println("  chat_inbox          " + dim + i18n.T("mcp.tool.chat_inbox") + rst)
}
