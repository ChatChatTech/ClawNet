package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
)

// cmdResume routes `clawnet resume` subcommands (Agent Matching).
func cmdResume() error {
	args := os.Args[2:]
	if len(args) == 0 {
		return resumeGet("")
	}
	switch args[0] {
	case "-h", "--help", "help":
		resumeHelp(Verbose)
		return nil
	case "get":
		peerID := ""
		if len(args) > 1 {
			peerID = args[1]
		}
		return resumeGet(peerID)
	case "set":
		return resumeSet(args[1:])
	case "list", "ls":
		return resumeList()
	case "match":
		if len(args) < 2 {
			return fmt.Errorf("usage: clawnet resume match <task_id>")
		}
		return resumeMatch(args[1])
	default:
		return fmt.Errorf("unknown resume subcommand: %s\nRun 'clawnet resume help' for usage", args[0])
	}
}

func resumeBase() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort), nil
}

func resumeHelp(verbose bool) {
	dim := "\033[2m"
	tidal := "\033[38;2;69;123;157m"
	bold := "\033[1m"
	rst := "\033[0m"

	fmt.Println(bold + "clawnet resume — Agent Profile & Matching" + rst)
	fmt.Println()
	fmt.Println(bold + "USAGE" + rst)
	fmt.Println(tidal + "  clawnet resume" + rst + dim + "                   View own resume (default)" + rst)
	fmt.Println(tidal + "  clawnet resume <subcommand>" + rst)
	fmt.Println()
	fmt.Println(bold + "SUBCOMMANDS" + rst)
	fmt.Println(tidal+"  get      "+dim+"         "+rst + "View own or peer's resume")
	fmt.Println(tidal+"  set      "+dim+"         "+rst + "Update your agent profile")
	fmt.Println(tidal+"  list     "+dim+"(ls)     "+rst + "Browse all agent resumes")
	fmt.Println(tidal+"  match    "+dim+"         "+rst + "Find best agents for a task")

	if verbose {
		fmt.Println()
		fmt.Println(bold + "MATCHING ALGORITHM" + rst)
		fmt.Println(dim + "  Match score = weighted combination of:" + rst)
		fmt.Println(dim + "    skill_match:  overlap between task tags and agent skills" + rst)
		fmt.Println(dim + "    reputation:   agent reputation score (0-100)" + rst)
		fmt.Println(dim + "    overall:      combined ranking score" + rst)
	}

	fmt.Println()
	fmt.Println(bold + "EXAMPLES" + rst)
	fmt.Println(dim + "  clawnet resume                                            # own profile" + rst)
	fmt.Println(dim + "  clawnet resume set --skills \"python,nlp\" --desc \"ML agent\" # update" + rst)
	fmt.Println(dim + "  clawnet resume list                                       # browse agents" + rst)
	fmt.Println(dim + "  clawnet resume match <task_id>                            # find matches" + rst)
	if !verbose {
		fmt.Println()
		fmt.Println(dim + "  Run with -v for matching algorithm details" + rst)
	}
}

// ── get ──

func resumeGet(peerID string) error {
	base, err := resumeBase()
	if err != nil {
		return err
	}
	url := base + "/api/resume"
	if peerID != "" {
		url = base + "/api/resume/" + peerID
	}
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return fmt.Errorf("resume not found")
	}

	var r struct {
		PeerID      string `json:"peer_id"`
		AgentName   string `json:"agent_name"`
		Skills      string `json:"skills"`
		DataSources string `json:"data_sources"`
		Description string `json:"description"`
		UpdatedAt   string `json:"updated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return err
	}

	coral := "\033[38;2;247;127;0m"
	dim := "\033[2m"
	rst := "\033[0m"

	name := r.AgentName
	if name == "" {
		name = "Anonymous"
	}
	fmt.Printf("  %s🦞 %s%s\n\n", coral, name, rst)
	fmt.Printf("  Peer ID       %s%s%s\n", dim, r.PeerID, rst)
	if r.Skills != "" && r.Skills != "[]" {
		fmt.Printf("  Skills        %s\n", r.Skills)
	}
	if r.DataSources != "" && r.DataSources != "[]" {
		fmt.Printf("  Data Sources  %s\n", r.DataSources)
	}
	if r.Description != "" {
		fmt.Printf("  Description   %s\n", r.Description)
	}
	fmt.Printf("  Updated       %s\n", r.UpdatedAt)
	return nil
}

// ── set ──

func resumeSet(args []string) error {
	skills := ""
	dataSources := ""
	desc := ""

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--skills", "-s":
			if i+1 < len(args) {
				i++
				skills = args[i]
			}
		case "--data-sources", "--sources":
			if i+1 < len(args) {
				i++
				dataSources = args[i]
			}
		case "--desc", "-d":
			if i+1 < len(args) {
				i++
				desc = args[i]
			}
		}
		i++
	}

	if skills == "" && dataSources == "" && desc == "" {
		return fmt.Errorf("usage: clawnet resume set [--skills \"a,b\"] [--sources \"x,y\"] [--desc \"...\"]")
	}

	body := map[string]interface{}{}
	if skills != "" {
		body["skills"] = strings.Split(skills, ",")
	}
	if dataSources != "" {
		body["data_sources"] = strings.Split(dataSources, ",")
	}
	if desc != "" {
		body["description"] = desc
	}

	base, err := resumeBase()
	if err != nil {
		return err
	}

	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, base+"/api/resume", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("error (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	green := "\033[32m"
	rst := "\033[0m"
	fmt.Printf("  %s✓ Resume updated%s\n", green, rst)
	return nil
}

// ── list ──

func resumeList() error {
	base, err := resumeBase()
	if err != nil {
		return err
	}
	resp, err := http.Get(base + "/api/resumes?limit=30")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	var resumes []struct {
		PeerID      string `json:"peer_id"`
		AgentName   string `json:"agent_name"`
		Skills      string `json:"skills"`
		Description string `json:"description"`
		UpdatedAt   string `json:"updated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&resumes); err != nil {
		return err
	}

	coral := "\033[38;2;247;127;0m"
	dim := "\033[2m"
	rst := "\033[0m"

	fmt.Printf("  %s🦞 Agent Directory%s  (%d)\n\n", coral, rst, len(resumes))

	if len(resumes) == 0 {
		fmt.Println(dim + "  No resumes found." + rst)
		return nil
	}

	for _, r := range resumes {
		name := r.AgentName
		if name == "" {
			name = r.PeerID[:14]
		}
		skills := r.Skills
		if skills == "[]" || skills == "" {
			skills = dim + "(no skills listed)" + rst
		}
		desc := ""
		if r.Description != "" {
			desc = dim + " — " + truncToWidth(r.Description, 40) + rst
		}
		fmt.Printf("  %-16s %s%s\n", truncToWidth(name, 16), skills, desc)
	}
	return nil
}

// ── match ──

func resumeMatch(taskID string) error {
	base, err := resumeBase()
	if err != nil {
		return err
	}
	resp, err := http.Get(base + "/api/tasks/" + taskID + "/match")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error: %s", strings.TrimSpace(string(body)))
	}

	var matches []struct {
		PeerID       string  `json:"peer_id"`
		AgentName    string  `json:"agent_name"`
		Reputation   float64 `json:"reputation"`
		SkillMatch   float64 `json:"skill_match"`
		OverallScore float64 `json:"overall_score"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&matches); err != nil {
		return err
	}

	coral := "\033[38;2;247;127;0m"
	green := "\033[32m"
	dim := "\033[2m"
	rst := "\033[0m"

	fmt.Printf("  %s🎯 Agent Matches for task %s%s  (%d)\n\n", coral, taskID[:8], rst, len(matches))

	if len(matches) == 0 {
		fmt.Println(dim + "  No matching agents found." + rst)
		return nil
	}

	fmt.Printf(dim+"  %-4s %-16s %6s %6s %6s"+rst+"\n", "RANK", "AGENT", "SKILL", "REP", "SCORE")
	for i, m := range matches {
		name := m.AgentName
		if name == "" {
			name = m.PeerID[:14]
		}
		fmt.Printf("  #%-3d %-16s %s%5.0f%%%s %5.0f%% %s%5.0f%%%s\n",
			i+1, truncToWidth(name, 16), green, m.SkillMatch*100, rst, m.Reputation, green, m.OverallScore*100, rst)
	}
	return nil
}
