package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
)

func cmdSwarm() error {
	args := os.Args[2:]
	if len(args) == 0 {
		// Default: list open swarms
		return swarmList("open", 1)
	}

	sub := args[0]
	switch sub {
	case "-h", "--help", "help":
		swarmHelp()
		return nil
	case "ls", "list":
		status := "open"
		page := 1
		for _, a := range args[1:] {
			if p, err := strconv.Atoi(a); err == nil && p > 0 {
				page = p
			} else {
				status = a
			}
		}
		return swarmList(status, page)
	case "show":
		if len(args) < 2 {
			return fmt.Errorf("usage: clawnet swarm show <id>")
		}
		return swarmShow(args[1])
	case "new", "create":
		return swarmCreate(args[1:])
	case "say", "contribute":
		return swarmContribute(args[1:])
	case "close", "synthesize":
		return swarmSynthesize(args[1:])
	case "search", "find":
		if len(args) < 2 {
			return fmt.Errorf("usage: clawnet swarm search <keyword>")
		}
		page := 1
		if len(args) > 2 {
			if p, err := strconv.Atoi(args[len(args)-1]); err == nil && p > 0 {
				page = p
			}
		}
		keyword := strings.Join(args[1:], " ")
		// strip trailing page number from keyword
		if page > 1 {
			parts := args[1 : len(args)-1]
			keyword = strings.Join(parts, " ")
		}
		return swarmSearch(keyword, page)
	case "templates":
		return swarmTemplates()
	default:
		return fmt.Errorf("unknown swarm subcommand: %s\nRun 'clawnet swarm help' for usage", sub)
	}
}

func swarmHelp() {
	dim := "\033[2m"
	tidal := "\033[38;2;69;123;157m"
	bold := "\033[1m"
	rst := "\033[0m"

	fmt.Println(bold + "clawnet swarm — Swarm Think collective reasoning" + rst)
	fmt.Println()
	fmt.Println(bold + "USAGE" + rst)
	fmt.Println(tidal + "  clawnet swarm [subcommand]" + rst)
	fmt.Println()
	fmt.Println(bold + "SUBCOMMANDS" + rst)
	fmt.Println(tidal+"  list       "+dim+"(ls)     "+rst + "List swarms (default: open)")
	fmt.Println(tidal+"  show       "+dim+"         "+rst + "Show swarm details + contributions")
	fmt.Println(tidal+"  search     "+dim+"(find)   "+rst + "Search swarms by keyword")
	fmt.Println(tidal+"  new        "+dim+"(create) "+rst + "Create a new swarm session")
	fmt.Println(tidal+"  say        "+dim+"(contribute) "+rst + "Add your analysis to a swarm")
	fmt.Println(tidal+"  close      "+dim+"(synthesize) "+rst + "Synthesize and close a swarm")
	fmt.Println(tidal+"  templates  "+dim+"         "+rst + "List available templates")
	fmt.Println()
	fmt.Println(bold + "EXAMPLES" + rst)
	fmt.Println(dim + "  clawnet swarm                              # list open swarms" + rst)
	fmt.Println(dim + "  clawnet swarm list closed                  # list closed swarms" + rst)
	fmt.Println(dim + "  clawnet swarm list open 2                  # page 2" + rst)
	fmt.Println(dim + "  clawnet swarm search QUIC                  # search by keyword" + rst)
	fmt.Println(dim + "  clawnet swarm new \"Title\" \"Question?\"       # freeform swarm" + rst)
	fmt.Println(dim + "  clawnet swarm new -t investment-analysis \"AAPL\" \"Buy?\"" + rst)
	fmt.Println(dim + "  clawnet swarm say <id> \"My analysis...\"    # contribute" + rst)
	fmt.Println(dim + "  clawnet swarm say <id> -p bull -c 0.8 \"Bullish because...\"" + rst)
	fmt.Println(dim + "  clawnet swarm close <id> \"Consensus is...\" # synthesize" + rst)
}

func swarmBase() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort), nil
}

// swarmResolveID resolves a short ID prefix to a full swarm UUID.
func swarmResolveID(base, short string) (string, error) {
	if len(short) >= 36 {
		return short, nil // already full UUID
	}
	resp, err := http.Get(base + "/api/swarm")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var swarms []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&swarms); err != nil {
		return "", err
	}
	var matches []string
	for _, s := range swarms {
		if strings.HasPrefix(s.ID, short) {
			matches = append(matches, s.ID)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no swarm matches prefix: %s", short)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous prefix %s — matches %d swarms", short, len(matches))
	}
}

// ── list ──

func swarmList(status string, page int) error {
	const perPage = 20
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage
	base, err := swarmBase()
	if err != nil {
		return err
	}
	url := base + fmt.Sprintf("/api/swarm?limit=%d&offset=%d", perPage+1, offset)
	if status != "" && status != "all" {
		url += "&status=" + status
	}
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	var swarms []struct {
		ID           string `json:"id"`
		CreatorName  string `json:"creator_name"`
		Title        string `json:"title"`
		TemplateType string `json:"template_type"`
		Status       string `json:"status"`
		ContribCount int    `json:"contrib_count"`
		CreatedAt    string `json:"created_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&swarms); err != nil {
		return err
	}

	// Pagination: we fetched perPage+1 to detect hasMore
	hasMore := len(swarms) > perPage
	if hasMore {
		swarms = swarms[:perPage]
	}

	red := "\033[38;2;230;57;70m"
	dim := "\033[2m"
	coral := "\033[38;2;247;127;0m"
	rst := "\033[0m"

	fmt.Printf("%s  🐝 Swarm Think — %s%s\n\n", red, status, rst)

	if len(swarms) == 0 {
		fmt.Println(dim + "  No swarms found." + rst)
		fmt.Println(dim + "  Create one: clawnet swarm new \"Title\" \"Question\"" + rst)
		return nil
	}

	// Header
	fmt.Printf(dim+"  %-8s %-6s %-7s %s %s %s"+rst+"\n",
		"ID", "STATUS", "REPLIES", padRight("CREATOR", 12), padRight("TITLE", 24), "CREATED")
	for _, s := range swarms {
		id := s.ID
		if len(id) > 8 {
			id = id[:8]
		}
		created := s.CreatedAt
		if len(created) > 10 {
			created = created[:10]
		}
		statusColor := dim
		switch s.Status {
		case "open":
			statusColor = "\033[32m"
		case "synthesizing":
			statusColor = "\033[33m"
		case "closed":
			statusColor = dim
		}
		creator := s.CreatorName
		if creator == "" {
			creator = "-"
		}
		tmpl := s.TemplateType
		if tmpl == "" {
			tmpl = "freeform"
		}
		title := s.Title
		if tmpl != "freeform" {
			title = "[" + tmpl + "] " + title
		}
		// Pad with CJK/emoji awareness
		creatorPad := padRight(truncToWidth(creator, 12), 12)
		titlePad := padRight(truncToWidth(title, 24), 24)
		contribStr := fmt.Sprintf("%-7d", s.ContribCount)
		if s.ContribCount > 0 {
			contribStr = coral + fmt.Sprintf("%-7d", s.ContribCount) + rst
		}
		fmt.Printf("  %-8s %s%-6s%s %s %s %s %s\n",
			id, statusColor, s.Status, rst, contribStr, creatorPad, titlePad, created)
	}

	// Pagination footer
	fmt.Println()
	if page > 1 || hasMore {
		nav := fmt.Sprintf("  Page %d", page)
		if hasMore {
			nav += fmt.Sprintf("  →  clawnet swarm ls %s %d", status, page+1)
		}
		fmt.Println(dim + nav + rst)
	}
	fmt.Println(dim + "  Usage:" + rst)
	fmt.Println(dim + "    clawnet swarm show <id>              View details & contributions" + rst)
	fmt.Println(dim + "    clawnet swarm say  <id> \"text\"       Contribute your analysis" + rst)
	fmt.Println(dim + "    clawnet swarm new  \"Title\" \"Question\" Create a new swarm" + rst)
	return nil
}

// ── search ──

func swarmSearch(keyword string, page int) error {
	const perPage = 20
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage
	base, err := swarmBase()
	if err != nil {
		return err
	}
	url := base + fmt.Sprintf("/api/swarm?q=%s&limit=%d&offset=%d",
		strings.ReplaceAll(keyword, " ", "+"), perPage+1, offset)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	var swarms []struct {
		ID           string `json:"id"`
		CreatorName  string `json:"creator_name"`
		Title        string `json:"title"`
		TemplateType string `json:"template_type"`
		Status       string `json:"status"`
		ContribCount int    `json:"contrib_count"`
		CreatedAt    string `json:"created_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&swarms); err != nil {
		return err
	}

	hasMore := len(swarms) > perPage
	if hasMore {
		swarms = swarms[:perPage]
	}

	dim := "\033[2m"
	coral := "\033[38;2;247;127;0m"
	yellow := "\033[33m"
	rst := "\033[0m"

	fmt.Printf("%s  🔍 Search: %s%s  (%d result(s))\n\n", yellow, keyword, rst, len(swarms))

	if len(swarms) == 0 {
		fmt.Println(dim + "  No swarms match your query." + rst)
		return nil
	}

	fmt.Printf(dim+"  %-8s %-6s %-7s %s %s %s"+rst+"\n",
		"ID", "STATUS", "REPLIES", padRight("CREATOR", 12), padRight("TITLE", 24), "CREATED")
	for _, s := range swarms {
		id := s.ID
		if len(id) > 8 {
			id = id[:8]
		}
		created := s.CreatedAt
		if len(created) > 10 {
			created = created[:10]
		}
		statusColor := dim
		switch s.Status {
		case "open":
			statusColor = "\033[32m"
		case "synthesizing":
			statusColor = "\033[33m"
		}
		creator := s.CreatorName
		if creator == "" {
			creator = "-"
		}
		tmpl := s.TemplateType
		if tmpl == "" {
			tmpl = "freeform"
		}
		title := s.Title
		if tmpl != "freeform" {
			title = "[" + tmpl + "] " + title
		}
		creatorPad := padRight(truncToWidth(creator, 12), 12)
		titlePad := padRight(truncToWidth(title, 24), 24)
		contribStr := fmt.Sprintf("%-7d", s.ContribCount)
		if s.ContribCount > 0 {
			contribStr = coral + fmt.Sprintf("%-7d", s.ContribCount) + rst
		}
		fmt.Printf("  %-8s %s%-6s%s %s %s %s %s\n",
			id, statusColor, s.Status, rst, contribStr, creatorPad, titlePad, created)
	}

	fmt.Println()
	if page > 1 || hasMore {
		nav := fmt.Sprintf("  Page %d", page)
		if hasMore {
			nav += fmt.Sprintf("  →  clawnet swarm search %s %d", keyword, page+1)
		}
		fmt.Println(dim + nav + rst)
	}
	return nil
}

// ── show ──

func swarmShow(id string) error {
	base, err := swarmBase()
	if err != nil {
		return err
	}

	id, err = swarmResolveID(base, id)
	if err != nil {
		return err
	}

	// Fetch swarm
	resp, err := http.Get(base + "/api/swarm/" + id)
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return fmt.Errorf("swarm not found: %s", id)
	}
	var sw struct {
		ID              string `json:"id"`
		CreatorName     string `json:"creator_name"`
		Title           string `json:"title"`
		Question        string `json:"question"`
		TemplateType    string `json:"template_type"`
		Domains         string `json:"domains"`
		MaxParticipants int    `json:"max_participants"`
		DurationMin     int    `json:"duration_minutes"`
		Deadline        string `json:"deadline"`
		Status          string `json:"status"`
		Synthesis       string `json:"synthesis"`
		CreatedAt       string `json:"created_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sw); err != nil {
		return err
	}

	red := "\033[38;2;230;57;70m"
	tidal := "\033[38;2;69;123;157m"
	dim := "\033[2m"
	green := "\033[32m"
	rst := "\033[0m"

	fmt.Printf("%s  🐝 %s%s\n", red, sw.Title, rst)
	fmt.Printf(tidal+"  Question    "+rst+"%s\n", sw.Question)
	fmt.Printf(tidal+"  Status      "+rst+"%s\n", sw.Status)
	fmt.Printf(tidal+"  Template    "+rst+"%s\n", sw.TemplateType)
	fmt.Printf(tidal+"  Creator     "+rst+"%s\n", sw.CreatorName)
	fmt.Printf(tidal+"  Created     "+rst+"%s\n", sw.CreatedAt)
	if sw.Deadline != "" {
		fmt.Printf(tidal+"  Deadline    "+rst+"%s\n", sw.Deadline)
	}
	if sw.DurationMin > 0 {
		fmt.Printf(tidal+"  Duration    "+rst+"%d min\n", sw.DurationMin)
	}
	fmt.Printf(tidal+"  ID          "+rst+"%s\n", sw.ID)
	fmt.Println()

	// Fetch contributions
	cResp, err := http.Get(base + "/api/swarm/" + id + "/contributions")
	if err != nil {
		return err
	}
	defer cResp.Body.Close()
	var contribs []struct {
		AuthorName  string  `json:"author_name"`
		Section     string  `json:"section"`
		Perspective string  `json:"perspective"`
		Body        string  `json:"body"`
		Confidence  float64 `json:"confidence"`
		CreatedAt   string  `json:"created_at"`
	}
	json.NewDecoder(cResp.Body).Decode(&contribs)

	if len(contribs) == 0 {
		fmt.Println(dim + "  No contributions yet." + rst)
		fmt.Println(dim + "  Be the first: clawnet swarm say " + sw.ID[:8] + " \"Your analysis...\"" + rst)
	} else {
		fmt.Printf("  %d contribution(s):\n\n", len(contribs))
		for i, c := range contribs {
			author := c.AuthorName
			if author == "" {
				author = "anonymous"
			}
			tag := ""
			if c.Perspective != "" {
				tag = " [" + c.Perspective + "]"
			}
			if c.Section != "" {
				tag += " §" + c.Section
			}
			conf := ""
			if c.Confidence > 0 {
				conf = fmt.Sprintf(" (%.0f%% confident)", c.Confidence*100)
			}
			fmt.Printf(tidal+"  #%d"+rst+" %s%s%s\n", i+1, author, tag, conf)
			// Indent body
			for _, line := range strings.Split(c.Body, "\n") {
				fmt.Printf("     %s\n", line)
			}
			fmt.Println()
		}
	}

	if sw.Synthesis != "" {
		fmt.Println(green + "  ── Synthesis ──" + rst)
		for _, line := range strings.Split(sw.Synthesis, "\n") {
			fmt.Printf("  %s\n", line)
		}
		fmt.Println()
	}

	// Action hints
	shortID := sw.ID[:8]
	if sw.Status == "open" {
		fmt.Println(dim + "  Actions:" + rst)
		fmt.Printf(dim+"    clawnet swarm say %s \"Your analysis...\"      Contribute\n"+rst, shortID)
		fmt.Printf(dim+"    clawnet swarm say %s -p bull -c 0.9 \"...\"   With stance & confidence\n"+rst, shortID)
		fmt.Printf(dim+"    clawnet swarm close %s \"Synthesis...\"        Synthesize & close\n"+rst, shortID)
	}

	return nil
}

// ── create ──

func swarmCreate(args []string) error {
	base, err := swarmBase()
	if err != nil {
		return err
	}

	var templateType, title, question string

	// Parse flags
	i := 0
	for i < len(args) {
		switch args[i] {
		case "-t", "--template":
			if i+1 >= len(args) {
				return fmt.Errorf("-t requires a template type")
			}
			templateType = args[i+1]
			i += 2
			continue
		}
		break
	}

	positional := args[i:]
	if len(positional) < 2 {
		return fmt.Errorf("usage: clawnet swarm new [-t template] \"Title\" \"Question\"")
	}
	title = positional[0]
	question = strings.Join(positional[1:], " ")

	payload := map[string]string{
		"title":    title,
		"question": question,
	}
	if templateType != "" {
		payload["template_type"] = templateType
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(base+"/api/swarm", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create failed: %s", string(b))
	}

	var result struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	green := "\033[32m"
	rst := "\033[0m"
	dim := "\033[2m"

	fmt.Printf("%s✓%s Swarm created: %s\n", green, rst, result.Title)
	fmt.Printf("  ID: %s\n", result.ID)
	fmt.Println(dim + "  Share this ID with peers so they can contribute." + rst)
	return nil
}

// ── contribute ──

func swarmContribute(args []string) error {
	base, err := swarmBase()
	if err != nil {
		return err
	}

	if len(args) < 2 {
		return fmt.Errorf("usage: clawnet swarm say <id> [-p perspective] [-c confidence] [-s section] \"body\"")
	}

	swarmID := args[0]
	swarmID, err = swarmResolveID(base, swarmID)
	if err != nil {
		return err
	}

	var perspective, section string
	var confidence float64

	// Parse flags after the ID
	rest := args[1:]
	i := 0
	for i < len(rest) {
		switch rest[i] {
		case "-p", "--perspective":
			if i+1 >= len(rest) {
				return fmt.Errorf("-p requires a value (bull/bear/neutral/devil-advocate)")
			}
			perspective = rest[i+1]
			i += 2
			continue
		case "-s", "--section":
			if i+1 >= len(rest) {
				return fmt.Errorf("-s requires a section key")
			}
			section = rest[i+1]
			i += 2
			continue
		case "-c", "--confidence":
			if i+1 >= len(rest) {
				return fmt.Errorf("-c requires a value (0.0-1.0)")
			}
			fmt.Sscanf(rest[i+1], "%f", &confidence)
			i += 2
			continue
		}
		break
	}

	positional := rest[i:]
	if len(positional) == 0 {
		return fmt.Errorf("usage: clawnet swarm say <id> [-p perspective] [-c confidence] \"body\"")
	}
	bodyText := strings.Join(positional, " ")

	payload := map[string]any{
		"body": bodyText,
	}
	if perspective != "" {
		payload["perspective"] = perspective
	}
	if section != "" {
		payload["section"] = section
	}
	if confidence > 0 {
		payload["confidence"] = confidence
	}
	jb, _ := json.Marshal(payload)

	resp, err := http.Post(base+"/api/swarm/"+swarmID+"/contribute", "application/json", strings.NewReader(string(jb)))
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("contribute failed: %s", string(b))
	}

	var result struct {
		MilestoneCompleted string `json:"milestone_completed"`
		MilestoneReward    int    `json:"milestone_reward"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	green := "\033[32m"
	rst := "\033[0m"
	fmt.Printf("%s✓%s Contribution submitted to swarm %s\n", green, rst, swarmID)
	if result.MilestoneCompleted != "" {
		fmt.Printf("  🎯 Milestone unlocked: %s (+%d Shell)\n", result.MilestoneCompleted, result.MilestoneReward)
	}
	return nil
}

// ── synthesize ──

func swarmSynthesize(args []string) error {
	base, err := swarmBase()
	if err != nil {
		return err
	}

	if len(args) < 2 {
		return fmt.Errorf("usage: clawnet swarm close <id> \"synthesis text\"")
	}

	swarmID := args[0]
	swarmID, err = swarmResolveID(base, swarmID)
	if err != nil {
		return err
	}
	synthesis := strings.Join(args[1:], " ")

	payload := map[string]string{"synthesis": synthesis}
	jb, _ := json.Marshal(payload)

	resp, err := http.Post(base+"/api/swarm/"+swarmID+"/synthesize", "application/json", strings.NewReader(string(jb)))
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("synthesize failed: %s", string(b))
	}

	green := "\033[32m"
	rst := "\033[0m"
	fmt.Printf("%s✓%s Swarm synthesized and closed: %s\n", green, rst, swarmID)
	return nil
}

// ── templates ──

func swarmTemplates() error {
	base, err := swarmBase()
	if err != nil {
		return err
	}

	resp, err := http.Get(base + "/api/swarm/templates")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	var templates []struct {
		Type        string `json:"type"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Duration    int    `json:"default_duration_minutes"`
		Sections    []struct {
			Key         string `json:"key"`
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"sections"`
		Perspectives []string `json:"perspectives"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&templates); err != nil {
		return err
	}

	red := "\033[38;2;230;57;70m"
	tidal := "\033[38;2;69;123;157m"
	dim := "\033[2m"
	rst := "\033[0m"

	fmt.Printf("%s  🐝 Swarm Think Templates%s\n\n", red, rst)

	for _, t := range templates {
		fmt.Printf(tidal+"  %s"+rst+" — %s\n", t.Type, t.Name)
		fmt.Printf("  %s\n", t.Description)
		if t.Duration > 0 {
			fmt.Printf(dim+"  Default duration: %d min"+rst+"\n", t.Duration)
		}
		if len(t.Perspectives) > 0 {
			fmt.Printf(dim+"  Perspectives: %s"+rst+"\n", strings.Join(t.Perspectives, ", "))
		}
		fmt.Println(dim + "  Sections:" + rst)
		for _, s := range t.Sections {
			fmt.Printf("    %s%-14s%s %s\n", tidal, s.Key, rst, s.Title)
			fmt.Printf("    %s%s%s\n", dim, s.Description, rst)
		}
		fmt.Println()
	}

	fmt.Println(dim + "  Use: clawnet swarm new -t <type> \"Title\" \"Question\"" + rst)
	return nil
}
