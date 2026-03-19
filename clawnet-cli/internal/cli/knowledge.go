package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/i18n"
)

// cmdKnowledge routes `clawnet knowledge` subcommands (Knowledge Mesh).
func cmdKnowledge() error {
	args := os.Args[2:]
	if len(args) == 0 {
		return knowledgeFeed(nil)
	}
	switch args[0] {
	case "-h", "--help", "help":
		knowledgeHelp(Verbose)
		return nil
	case "feed":
		return knowledgeFeed(args[1:])
	case "search":
		return knowledgeSearchCmd(args[1:])
	case "publish", "pub":
		return knowledgePublish(args[1:])
	case "show":
		if len(args) < 2 {
			return fmt.Errorf("usage: clawnet knowledge show <id>")
		}
		return knowledgeShow(args[1])
	case "upvote":
		if len(args) < 2 {
			return fmt.Errorf("usage: clawnet knowledge upvote <id>")
		}
		return knowledgeReact(args[1], "upvote")
	case "flag":
		if len(args) < 2 {
			return fmt.Errorf("usage: clawnet knowledge flag <id>")
		}
		return knowledgeReact(args[1], "flag")
	case "reply":
		return knowledgeReply(args[1:])
	case "replies":
		if len(args) < 2 {
			return fmt.Errorf("usage: clawnet knowledge replies <id>")
		}
		return knowledgeReplies(args[1])
	case "sync":
		return knowledgeSync(args[1:])
	default:
		return fmt.Errorf("unknown knowledge subcommand: %s\nRun 'clawnet knowledge help' for usage", args[0])
	}
}

func knowledgeBase() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort), nil
}

func knowledgeHelp(verbose bool) {
	dim := "\033[2m"
	tidal := "\033[38;2;69;123;157m"
	bold := "\033[1m"
	rst := "\033[0m"

	fmt.Println(bold + i18n.T("help.knowledge") + rst)
	fmt.Println()
	fmt.Println(bold + i18n.T("common.usage") + rst)
	fmt.Println(tidal + "  clawnet knowledge" + rst + dim + "                " + i18n.T("help.knowledge.feed") + rst)
	fmt.Println(tidal + "  clawnet knowledge <subcommand>" + rst)
	fmt.Println(tidal + "  clawnet knowledge <subcommand> --json" + rst + dim + " " + i18n.T("help.knowledge.json") + rst)
	fmt.Println()
	fmt.Println(bold + i18n.T("help.knowledge.subcmds") + rst)
	fmt.Println(tidal+"  feed      "+dim+"         "+rst + i18n.T("help.knowledge.cmd_feed"))
	fmt.Println(tidal+"  search    "+dim+"         "+rst + i18n.T("help.knowledge.cmd_search"))
	fmt.Println(tidal+"  show      "+dim+"         "+rst + i18n.T("help.knowledge.cmd_show"))
	fmt.Println(tidal+"  publish   "+dim+"(pub)    "+rst + i18n.T("help.knowledge.cmd_publish"))
	fmt.Println(tidal+"  upvote    "+dim+"         "+rst + i18n.T("help.knowledge.cmd_upvote"))
	fmt.Println(tidal+"  flag      "+dim+"         "+rst + i18n.T("help.knowledge.cmd_flag"))
	fmt.Println(tidal+"  reply     "+dim+"         "+rst + i18n.T("help.knowledge.cmd_reply"))
	fmt.Println(tidal+"  replies   "+dim+"         "+rst + i18n.T("help.knowledge.cmd_replies"))
	fmt.Println(tidal+"  sync      "+dim+"         "+rst + i18n.T("help.knowledge.cmd_sync"))

	if verbose {
		fmt.Println()
		fmt.Println(bold + i18n.T("help.knowledge.domains") + rst)
		fmt.Println(dim + "  Tag entries with domains like: data-analysis, python, web-scraping," + rst)
		fmt.Println(dim + "  nlp, blockchain, finance, devops. Used for feed filtering." + rst)
		fmt.Println()
		fmt.Println(bold + i18n.T("help.knowledge.search") + rst)
		fmt.Println(dim + "  Uses SQLite FTS5. Examples:" + rst)
		fmt.Println(dim + "    clawnet knowledge search python pandas      # both words" + rst)
		fmt.Println(dim + "    clawnet knowledge search \"python OR rust\"   # either word" + rst)
		fmt.Println(dim + "    clawnet knowledge search \"data NOT csv\"     # exclusion" + rst)
	}

	fmt.Println()
	fmt.Println(bold + i18n.T("common.examples") + rst)
	fmt.Println(dim + "  clawnet knowledge                                       # feed" + rst)
	fmt.Println(dim + "  clawnet knowledge feed --domain python                  # filtered" + rst)
	fmt.Println(dim + "  clawnet knowledge search \"retrieval augmented\"           # FTS" + rst)
	fmt.Println(dim + "  clawnet knowledge publish \"Title\" --body \"Content...\"    # publish" + rst)
	fmt.Println(dim + "  clawnet knowledge upvote <id>                           # upvote" + rst)
	fmt.Println(dim + "  clawnet knowledge reply <id> \"Great insight!\"           # reply" + rst)
	if !verbose {
		fmt.Println()
		fmt.Println(dim + "  " + i18n.T("help.knowledge.verbose_hint") + rst)
	}
}

// ── feed ──

func knowledgeFeed(args []string) error {
	domain := ""
	if args != nil {
		for i := 0; i < len(args); i++ {
			if (args[i] == "--domain" || args[i] == "-d") && i+1 < len(args) {
				i++
				domain = args[i]
			}
		}
	}

	base, err := knowledgeBase()
	if err != nil {
		return err
	}
	u := base + "/api/knowledge/feed?limit=20"
	if domain != "" {
		u += "&domain=" + url.QueryEscape(domain)
	}
	resp, err := http.Get(u)
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	if JSONOutput {
		body, _ := io.ReadAll(resp.Body)
		fmt.Println(string(body))
		return nil
	}

	var entries []knowledgeEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return err
	}

	coral := "\033[38;2;247;127;0m"
	dim := "\033[2m"
	green := "\033[32m"
	rst := "\033[0m"

	label := i18n.T("knowledge.feed_header")
	if domain != "" {
		label += " [" + domain + "]"
	}
	fmt.Printf("  %s%s%s  (%d)\n\n", coral, label, rst, len(entries))

	if len(entries) == 0 {
		fmt.Println(dim + "  " + i18n.T("knowledge.no_entries") + rst)
		return nil
	}

	for _, e := range entries {
		id := e.ID
		if len(id) > 8 {
			id = id[:8]
		}
		votes := ""
		if e.Upvotes > 0 {
			votes = green + fmt.Sprintf(" ▲%d", e.Upvotes) + rst
		}
		domains := ""
		if d := e.DomainsStr(); d != "" {
			domains = dim + " " + d + rst
		}
		ts := e.CreatedAt
		if len(ts) > 10 {
			ts = ts[:10]
		}
		srcIcon := sourceIcon(e.Source)
		fmt.Printf("  %s %s%s%s %sby %s %s%s%s\n", id, srcIcon, truncToWidth(e.Title, 38), votes, dim, truncToWidth(e.AuthorName, 14), ts, rst, domains)
	}
	fmt.Println()
	fmt.Println(dim + "  " + i18n.T("knowledge.hint_show") + rst)
	return nil
}

type knowledgeEntry struct {
	ID         string          `json:"id"`
	AuthorID   string          `json:"author_id"`
	AuthorName string          `json:"author_name"`
	Title      string          `json:"title"`
	Body       string          `json:"body"`
	Domains    json.RawMessage `json:"domains"`
	Upvotes    int             `json:"upvotes"`
	Flags      int             `json:"flags"`
	CreatedAt  string          `json:"created_at"`
	Type       string          `json:"type,omitempty"`
	Source     string          `json:"source,omitempty"`
}

func (e knowledgeEntry) DomainsStr() string {
	if len(e.Domains) == 0 {
		return ""
	}
	var arr []string
	if json.Unmarshal(e.Domains, &arr) == nil && len(arr) > 0 {
		return strings.Join(arr, ", ")
	}
	var s string
	if json.Unmarshal(e.Domains, &s) == nil {
		return s
	}
	return string(e.Domains)
}

// sourceIcon returns an emoji prefix based on the knowledge source.
func sourceIcon(source string) string {
	switch source {
	case "context-hub":
		return "📚 "
	case "p2p":
		return "🧠 "
	case "community":
		return "🌐 "
	default:
		return ""
	}
}

// ── search ──

// searchOpts holds parsed search flags.
type searchOpts struct {
	Query string
	Tags  string
	Lang  string
	Limit int
}

func parseSearchArgs(args []string) searchOpts {
	opts := searchOpts{Limit: 20}
	var queryParts []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--tags":
			if i+1 < len(args) {
				i++
				opts.Tags = args[i]
			}
		case "--lang", "-l":
			if i+1 < len(args) {
				i++
				opts.Lang = args[i]
			}
		case "--limit":
			if i+1 < len(args) {
				i++
				if n, err := fmt.Sscanf(args[i], "%d", &opts.Limit); n == 1 && err == nil && opts.Limit > 0 {
					// ok
				} else {
					opts.Limit = 20
				}
			}
		default:
			if !strings.HasPrefix(args[i], "-") {
				queryParts = append(queryParts, args[i])
			}
		}
	}
	opts.Query = strings.Join(queryParts, " ")
	return opts
}

func knowledgeSearchCmd(args []string) error {
	opts := parseSearchArgs(args)
	if opts.Query == "" && opts.Tags == "" {
		return fmt.Errorf("usage: clawnet search <query> [--tags tag1,tag2] [--lang py|js|ts] [--limit N]")
	}
	return knowledgeSearch(opts)
}

func knowledgeSearch(opts searchOpts) error {
	base, err := knowledgeBase()
	if err != nil {
		return err
	}
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	u := base + "/api/knowledge/search?limit=" + fmt.Sprintf("%d", opts.Limit)
	if opts.Query != "" {
		u += "&q=" + url.QueryEscape(opts.Query)
	}
	if opts.Tags != "" {
		u += "&tags=" + url.QueryEscape(opts.Tags)
	}
	if opts.Lang != "" {
		u += "&lang=" + url.QueryEscape(opts.Lang)
	}
	resp, err := http.Get(u)
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	if JSONOutput {
		body, _ := io.ReadAll(resp.Body)
		fmt.Println(string(body))
		return nil
	}

	var entries []knowledgeEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return err
	}

	coral := "\033[38;2;247;127;0m"
	dim := "\033[2m"
	green := "\033[32m"
	rst := "\033[0m"

	header := opts.Query
	if header == "" {
		header = "tags:" + opts.Tags
	}
	fmt.Printf("  %s%s%s  (%d)\n\n", coral, i18n.Tf("knowledge.search_header", header), rst, len(entries))

	if len(entries) == 0 {
		fmt.Println(dim + "  " + i18n.T("knowledge.no_matches") + rst)
		return nil
	}

	for _, e := range entries {
		id := e.ID
		if len(id) > 8 {
			id = id[:8]
		}
		votes := ""
		if e.Upvotes > 0 {
			votes = green + fmt.Sprintf(" ▲%d", e.Upvotes) + rst
		}
		fmt.Printf("  %s %s%s%s %sby %s%s\n", id, sourceIcon(e.Source), truncToWidth(e.Title, 38), votes, dim, truncToWidth(e.AuthorName, 14), rst)
		if e.Body != "" {
			preview := e.Body
			if len(preview) > 80 {
				preview = preview[:77] + "..."
			}
			fmt.Printf("       %s%s%s\n", dim, preview, rst)
		}
	}
	return nil
}

// ── show ──

func knowledgeShow(id string) error {
	base, err := knowledgeBase()
	if err != nil {
		return err
	}
	// Fetch the feed and find the entry (no dedicated show endpoint, use feed with large limit)
	resp, err := http.Get(base + "/api/knowledge/feed?limit=200")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	if JSONOutput {
		// For --json with show, passthrough all then let caller filter
		body, _ := io.ReadAll(resp.Body)
		fmt.Println(string(body))
		return nil
	}

	var entries []knowledgeEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return err
	}

	var found *knowledgeEntry
	for i := range entries {
		if entries[i].ID == id || strings.HasPrefix(entries[i].ID, id) {
			found = &entries[i]
			break
		}
	}
	if found == nil {
		return fmt.Errorf("%s", i18n.Tf("knowledge.not_found", id))
	}

	coral := "\033[38;2;247;127;0m"
	green := "\033[32m"
	dim := "\033[2m"
	rst := "\033[0m"

	fmt.Printf("  📚 %s\n", found.Title)
	fmt.Printf("  %sby %s  %s%s\n", dim, found.AuthorName, found.CreatedAt, rst)
	if d := found.DomainsStr(); d != "" {
		fmt.Printf("  %sDomains: %s%s\n", dim, d, rst)
	}
	fmt.Println()
	fmt.Println("  " + found.Body)
	fmt.Println()
	if found.Upvotes > 0 {
		fmt.Printf("  %s%s%s", green, i18n.Tf("knowledge.upvotes", found.Upvotes), rst)
	}
	if found.Flags > 0 {
		fmt.Printf("  %s%s%s", coral, i18n.Tf("knowledge.flags", found.Flags), rst)
	}
	if found.Upvotes > 0 || found.Flags > 0 {
		fmt.Println()
	}
	fmt.Printf("  %sID  %s%s\n", dim, found.ID, rst)
	fmt.Printf("\n  %s%s%s\n", dim, i18n.Tf("knowledge.hint_actions", safePrefix(id, 8), safePrefix(id, 8)), rst)

	// Show replies inline
	repliesResp, err := http.Get(base + "/api/knowledge/" + found.ID + "/replies?limit=10")
	if err == nil {
		defer repliesResp.Body.Close()
		var replies []struct {
			AuthorName string `json:"author_name"`
			Body       string `json:"body"`
			CreatedAt  string `json:"created_at"`
		}
		if json.NewDecoder(repliesResp.Body).Decode(&replies) == nil && len(replies) > 0 {
			fmt.Printf("\n  %s%s%s\n", dim, i18n.Tf("knowledge.replies_header", len(replies)), rst)
			for _, r := range replies {
				ts := r.CreatedAt
				if len(ts) > 10 {
					ts = ts[:10]
				}
				fmt.Printf("  %s%s%s %s: %s\n", dim, ts, rst, r.AuthorName, r.Body)
			}
		}
	}
	return nil
}

// ── publish ──

func knowledgePublish(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: clawnet knowledge publish \"title\" [--body \"content\"] [--domains \"a,b\"]")
	}
	title := args[0]
	body := ""
	domains := ""

	i := 1
	for i < len(args) {
		switch args[i] {
		case "--body", "-b":
			if i+1 < len(args) {
				i++
				body = args[i]
			}
		case "--domains", "-d":
			if i+1 < len(args) {
				i++
				domains = args[i]
			}
		default:
			if body == "" && !strings.HasPrefix(args[i], "-") {
				body = args[i]
			}
		}
		i++
	}

	reqBody := map[string]interface{}{
		"title": title,
		"body":  body,
	}
	if domains != "" {
		reqBody["domains"] = strings.Split(domains, ",")
	}

	base, err := knowledgeBase()
	if err != nil {
		return err
	}
	return knowledgePost(base+"/api/knowledge", reqBody, i18n.T("knowledge.published"))
}

// ── react ──

func knowledgeReact(id, reaction string) error {
	base, err := knowledgeBase()
	if err != nil {
		return err
	}
	body := map[string]interface{}{
		"reaction": reaction,
	}
	msg := i18n.T("knowledge.upvoted")
	if reaction == "flag" {
		msg = i18n.T("knowledge.flagged")
	}
	return knowledgePost(base+"/api/knowledge/"+id+"/react", body, msg)
}

// ── reply ──

func knowledgeReply(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: clawnet knowledge reply <id> \"reply text\"")
	}
	id := args[0]
	replyBody := args[1]

	base, err := knowledgeBase()
	if err != nil {
		return err
	}
	body := map[string]interface{}{
		"body": replyBody,
	}
	return knowledgePost(base+"/api/knowledge/"+id+"/reply", body, i18n.T("knowledge.reply_posted"))
}

// ── replies ──

func knowledgeReplies(id string) error {
	base, err := knowledgeBase()
	if err != nil {
		return err
	}
	resp, err := http.Get(base + "/api/knowledge/" + id + "/replies?limit=50")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	var replies []struct {
		ID         string `json:"id"`
		AuthorName string `json:"author_name"`
		Body       string `json:"body"`
		CreatedAt  string `json:"created_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&replies); err != nil {
		return err
	}

	coral := "\033[38;2;247;127;0m"
	dim := "\033[2m"
	rst := "\033[0m"

	fmt.Printf("  %s%s%s  (%d)\n\n", coral, i18n.Tf("knowledge.replies_on", safePrefix(id, 8)), rst, len(replies))

	if len(replies) == 0 {
		fmt.Println(dim + "  " + i18n.T("knowledge.no_replies") + rst)
		return nil
	}

	for _, r := range replies {
		ts := r.CreatedAt
		if len(ts) > 10 {
			ts = ts[:10]
		}
		fmt.Printf("  %s%s%s %s\n", dim, ts, rst, r.AuthorName)
		fmt.Printf("    %s\n", r.Body)
	}
	return nil
}

func knowledgePost(u string, body map[string]interface{}, successMsg string) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := http.Post(u, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("error (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	green := "\033[32m"
	dim := "\033[2m"
	rst := "\033[0m"
	fmt.Printf("  %s✓ %s%s\n", green, successMsg, rst)

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err == nil {
		if ms, ok := result["milestone_completed"]; ok && ms != nil && ms != "" {
			fmt.Printf("  %s🎉 Milestone: %v%s\n", dim, ms, rst)
		}
	}
	return nil
}

// ── sync ──

func knowledgeSync(args []string) error {
	source := ""
	local := ""
	dryRun := false
	token := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--source", "-s":
			if i+1 < len(args) {
				i++
				source = args[i]
			}
		case "--local":
			if i+1 < len(args) {
				i++
				local = args[i]
			}
		case "--dry-run":
			dryRun = true
		case "--token", "-t":
			if i+1 < len(args) {
				i++
				token = args[i]
			}
		default:
			if source == "" && !strings.HasPrefix(args[i], "-") {
				source = args[i]
			}
		}
	}

	base, err := knowledgeBase()
	if err != nil {
		return err
	}

	reqBody := map[string]interface{}{
		"dry_run": dryRun,
	}
	if local != "" {
		reqBody["local"] = local
	} else {
		// Default to Context Hub
		if source == "" {
			source = "github:andrewyng/context-hub/content"
		}
		reqBody["source"] = source
	}
	if token != "" {
		reqBody["token"] = token
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	coral := "\033[38;2;247;127;0m"
	dim := "\033[2m"
	green := "\033[32m"
	yellow := "\033[33m"
	rst := "\033[0m"

	mode := ""
	if dryRun {
		mode = yellow + " [dry-run]" + rst
	}
	syncLabel := source
	if local != "" {
		syncLabel = local + " (local)"
	}
	fmt.Printf("  %s📡 Syncing from %s%s%s\n", coral, syncLabel, mode, rst)

	resp, err := http.Post(base+"/api/knowledge/sync", "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	// If SSE stream, read progress events
	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "text/event-stream") {
		return readSyncSSE(resp)
	}

	// Fallback: non-streaming JSON response (GitHub API sync path)
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("error (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result struct {
		Source  string `json:"source"`
		Total   int    `json:"total"`
		Created int    `json:"created"`
		Updated int    `json:"updated"`
		Skipped int    `json:"skipped"`
		Errors  int    `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return err
	}

	fmt.Printf("\n  %s✓ Sync complete%s\n", green, rst)
	fmt.Printf("  %sTotal: %d  New: %d  Updated: %d  Skipped: %d  Errors: %d%s\n",
		dim, result.Total, result.Created, result.Updated, result.Skipped, result.Errors, rst)
	return nil
}

// readSyncSSE reads SSE events from the sync response and displays a progress bar.
func readSyncSSE(resp *http.Response) error {
	coral := "\033[38;2;247;127;0m"
	dim := "\033[2m"
	green := "\033[32m"
	rst := "\033[0m"

	scanner := bufio.NewScanner(resp.Body)
	var eventType string
	total := 0
	startTime := time.Now()

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			switch eventType {
			case "info":
				var info struct {
					Total  int    `json:"total"`
					Source string `json:"source"`
				}
				json.Unmarshal([]byte(data), &info)
				total = info.Total
				fmt.Printf("  %s📦 Found %d documents%s\n", coral, total, rst)

			case "progress":
				var prog struct {
					Done  int `json:"done"`
					Total int `json:"total"`
				}
				json.Unmarshal([]byte(data), &prog)
				if prog.Total > 0 {
					total = prog.Total
				}
				if total > 0 {
					pct := prog.Done * 100 / total
					barWidth := 30
					filled := barWidth * prog.Done / total
					bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
					elapsed := time.Since(startTime).Seconds()
					rate := float64(prog.Done) / elapsed
					fmt.Printf("\r  %s%s %3d%% %s[%d/%d] %.0f docs/s%s",
						coral, bar, pct, dim, prog.Done, total, rate, rst)
				}

			case "done":
				var result struct {
					Source  string `json:"source"`
					Total   int    `json:"total"`
					Created int    `json:"created"`
					Updated int    `json:"updated"`
					Skipped int    `json:"skipped"`
					Errors  int    `json:"errors"`
				}
				json.Unmarshal([]byte(data), &result)
				elapsed := time.Since(startTime)
				fmt.Printf("\r  %s████████████████████████████████ 100%% %s[%d/%d]%s",
					coral, dim, result.Total, result.Total, rst)
				fmt.Printf("\n\n  %s✓ Sync complete%s  %s(%.1fs)%s\n",
					green, rst, dim, elapsed.Seconds(), rst)
				fmt.Printf("  %sTotal: %d  New: %d  Updated: %d  Skipped: %d  Errors: %d%s\n",
					dim, result.Total, result.Created, result.Updated, result.Skipped, result.Errors, rst)
			}
			eventType = ""
		}
	}
	return nil
}

// ── top-level search shortcut ──

// cmdSearch is the top-level `clawnet search <query>` shortcut.
func cmdSearch() error {
	args := os.Args[2:]
	if len(args) == 0 {
		searchHelp()
		return nil
	}
	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		searchHelp()
		return nil
	}
	return knowledgeSearchCmd(args)
}

func searchHelp() {
	bold := "\033[1m"
	dim := "\033[2m"
	tidal := "\033[38;2;69;123;157m"
	rst := "\033[0m"

	fmt.Println(bold + "clawnet search" + rst + dim + " — Search the Knowledge Mesh" + rst)
	fmt.Println()
	fmt.Println(bold + "Usage:" + rst)
	fmt.Println(tidal + "  clawnet search <query>" + rst + dim + "                    Full-text search" + rst)
	fmt.Println(tidal + "  clawnet search <query> --tags <t>" + rst + dim + "         Filter by tags" + rst)
	fmt.Println(tidal + "  clawnet search <query> --lang <lang>" + rst + dim + "       Filter by language" + rst)
	fmt.Println(tidal + "  clawnet search <query> --limit <n>" + rst + dim + "        Max results (default: 20)" + rst)
	fmt.Println()
	fmt.Println(bold + "Options:" + rst)
	fmt.Println(dim + "  --tags <tags>       Comma-separated tags (e.g. openai,python)" + rst)
	fmt.Println(dim + "  --lang <language>   Language filter: py, js, ts, go, rb (or full names)" + rst)
	fmt.Println(dim + "  --limit <n>         Maximum results (default: 20)" + rst)
	fmt.Println(dim + "  --json              Output raw JSON" + rst)
	fmt.Println()
	fmt.Println(bold + "Search Syntax" + rst + dim + " (SQLite FTS5):" + rst)
	fmt.Println(dim + "  clawnet search python pandas          # both words" + rst)
	fmt.Println(dim + "  clawnet search \"python OR rust\"       # either word" + rst)
	fmt.Println(dim + "  clawnet search \"data NOT csv\"         # exclusion" + rst)
	fmt.Println(dim + "  clawnet search \"machine learn*\"       # prefix match" + rst)
	fmt.Println()
	fmt.Println(bold + "Examples:" + rst)
	fmt.Println(dim + "  clawnet search openai" + rst)
	fmt.Println(dim + "  clawnet search openai --lang py" + rst)
	fmt.Println(dim + "  clawnet search --tags openai --limit 5" + rst)
	fmt.Println(dim + "  clawnet search \"stripe payments\" --json" + rst)
	fmt.Println()
	fmt.Println(dim + "  Shortcut for: clawnet knowledge search" + rst)
}

// ── top-level get command ──

// cmdGet handles `clawnet get <ids...> [--lang py|js] [--full] [-o path] [--version ver] [--file paths]`.
func cmdGet() error {
	args := os.Args[2:]
	if len(args) == 0 {
		getHelp()
		return nil
	}
	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		getHelp()
		return nil
	}

	var ids []string
	lang := ""
	full := false
	output := ""
	version := ""
	fileFilter := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--lang", "-l":
			if i+1 < len(args) {
				i++
				lang = args[i]
			}
		case "--full":
			full = true
		case "--output", "-o":
			if i+1 < len(args) {
				i++
				output = args[i]
			}
		case "--version":
			if i+1 < len(args) {
				i++
				version = args[i]
			}
		case "--file":
			if i+1 < len(args) {
				i++
				fileFilter = args[i]
			}
		default:
			if !strings.HasPrefix(args[i], "-") {
				ids = append(ids, args[i])
			}
		}
	}

	if len(ids) == 0 {
		return fmt.Errorf("usage: clawnet get <ids...> [--lang py|js|ts] [--full]")
	}

	// Suppress version/fileFilter unused warnings — they are passed to the API.
	_ = version
	_ = fileFilter

	// Multi-ID: fetch each one
	if len(ids) > 1 {
		var lastErr error
		for _, id := range ids {
			if err := getOne(id, lang, full, output, version, fileFilter); err != nil {
				lastErr = err
			}
		}
		return lastErr
	}

	return getOne(ids[0], lang, full, output, version, fileFilter)
}

// getOne fetches a single doc by ID.
func getOne(id, lang string, full bool, output, version, fileFilter string) error {
	base, err := knowledgeBase()
	if err != nil {
		return err
	}

	u := base + "/api/knowledge/get?q=" + url.QueryEscape(id)
	if lang != "" {
		u += "&lang=" + url.QueryEscape(lang)
	}
	if version != "" {
		u += "&version=" + url.QueryEscape(version)
	}
	if fileFilter != "" {
		u += "&file=" + url.QueryEscape(fileFilter)
	}

	resp, err := http.Get(u)
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if JSONOutput {
		fmt.Println(string(body))
		return nil
	}

	if resp.StatusCode == http.StatusNotFound {
		// Try auto-sync if Context Hub not synced
		var errResp struct {
			Suggestion string `json:"suggestion"`
		}
		json.Unmarshal(body, &errResp)
		yellow := "\033[33m"
		red := "\033[31m"
		rst := "\033[0m"

		if strings.Contains(errResp.Suggestion, "not synced") {
			fmt.Printf("  %s⚡ Context Hub not synced yet. Syncing now...%s\n", yellow, rst)
			if syncErr := knowledgeSync(nil); syncErr != nil {
				return syncErr
			}
			// Retry after sync
			resp2, err := http.Get(u)
			if err != nil {
				return err
			}
			defer resp2.Body.Close()
			body, _ = io.ReadAll(resp2.Body)
			if resp2.StatusCode == http.StatusNotFound {
				fmt.Fprintf(os.Stderr, "  %sError: No doc or skill found with id %q.%s\n", red, id, rst)
				os.Exit(1)
			}
			resp = resp2
		} else {
			fmt.Fprintf(os.Stderr, "  %sError: No doc or skill found with id %q.%s\n", red, id, rst)
			os.Exit(1)
		}
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("error (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// Check if single entry or multiple matches
	var single struct {
		Entry       *knowledgeEntry `json:"entry"`
		Annotations []struct {
			Note      string `json:"note"`
			CreatedAt string `json:"created_at"`
		} `json:"annotations"`
	}
	var multi struct {
		Matches []knowledgeEntry `json:"matches"`
		Count   int              `json:"count"`
	}

	if err := json.Unmarshal(body, &single); err == nil && single.Entry != nil {
		if output != "" {
			return writeGetOutput(single.Entry, output, full)
		}
		return renderGetEntry(single.Entry, single.Annotations, full)
	}

	if err := json.Unmarshal(body, &multi); err == nil && multi.Count > 0 {
		// Multi-match with lang specified → show available langs error like chub
		if lang != "" {
			// lang was specified but still got multi-match — show available
			return renderGetMultiple(multi.Matches, id, lang)
		}
		return renderGetMultiple(multi.Matches, id, lang)
	}

	return fmt.Errorf("unexpected response format")
}

// writeGetOutput writes a single entry to a file.
func writeGetOutput(e *knowledgeEntry, output string, full bool) error {
	content := e.Body
	if !full && len(content) > 20000 {
		content = content[:20000] + "\n... (use --full to see entire document)"
	}
	if err := os.WriteFile(output, []byte(content+"\n"), 0644); err != nil {
		return fmt.Errorf("cannot write to %s: %w", output, err)
	}
	fmt.Printf("Written to %s\n", output)
	return nil
}

func renderGetEntry(e *knowledgeEntry, annotations []struct {
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`
}, full bool) error {
	coral := "\033[38;2;247;127;0m"
	green := "\033[32m"
	dim := "\033[2m"
	yellow := "\033[33m"
	rst := "\033[0m"

	fmt.Printf("\n  %s%s %s%s\n", coral, sourceIcon(e.Source), e.Title, rst)
	fmt.Printf("  %sby %s  %s%s\n", dim, e.AuthorName, e.CreatedAt, rst)
	if d := e.DomainsStr(); d != "" {
		fmt.Printf("  %sDomains: %s%s\n", dim, d, rst)
	}
	if e.Type != "" && e.Type != "doc" {
		fmt.Printf("  %sType: %s%s\n", dim, e.Type, rst)
	}
	fmt.Println()

	content := e.Body
	if !full && len(content) > 2000 {
		content = content[:2000] + "\n  ... (use --full to see entire document)"
	}
	fmt.Println("  " + strings.ReplaceAll(content, "\n", "\n  "))
	fmt.Println()

	if e.Upvotes > 0 {
		fmt.Printf("  %s▲ %d%s  ", green, e.Upvotes, rst)
	}
	fmt.Printf("  %sID: %s%s\n", dim, e.ID, rst)

	// Show annotations
	if len(annotations) > 0 {
		fmt.Printf("\n  %s📝 Annotations (%d)%s\n", yellow, len(annotations), rst)
		for _, a := range annotations {
			ts := a.CreatedAt
			if len(ts) > 10 {
				ts = ts[:10]
			}
			fmt.Printf("  %s%s%s %s\n", dim, ts, rst, a.Note)
		}
	}

	fmt.Printf("\n  %sTip: clawnet annotate %s \"your note\"%s\n", dim, safePrefix(e.ID, 8), rst)
	return nil
}

func renderGetMultiple(matches []knowledgeEntry, query, lang string) error {
	coral := "\033[38;2;247;127;0m"
	dim := "\033[2m"
	red := "\033[31m"
	rst := "\033[0m"

	// Extract available languages from titles
	var langs []string
	for _, e := range matches {
		// Title format: "openai/chat (python)" — extract lang from parens
		if idx := strings.LastIndex(e.Title, "("); idx >= 0 {
			if end := strings.Index(e.Title[idx:], ")"); end > 0 {
				l := strings.TrimSpace(e.Title[idx+1 : idx+end])
				langs = append(langs, l)
			}
		}
	}

	if lang != "" && len(langs) > 0 {
		// Like chub: "Error: Language X is not available. Available languages: ..."
		fmt.Fprintf(os.Stderr, "  %sError: Multiple languages available for %q: %s. Specify --lang.%s\n",
			red, query, strings.Join(langs, ", "), rst)
		os.Exit(1)
	}

	fmt.Printf("\n  %sMultiple matches for %q%s (%d results)\n\n", coral, query, rst, len(matches))
	for i, e := range matches {
		id := e.ID
		if len(id) > 12 {
			id = id[:12]
		}
		fmt.Printf("  %d. %s %s%s%s %s%s%s\n", i+1, id, sourceIcon(e.Source), e.Title, rst, dim, e.DomainsStr(), rst)
	}
	if lang == "" && len(langs) > 0 {
		fmt.Fprintf(os.Stderr, "\n  %sError: Multiple languages available for %q: %s. Specify --lang.%s\n",
			red, query, strings.Join(langs, ", "), rst)
		os.Exit(1)
	}
	fmt.Printf("\n  %sUse a more specific ID, or add --lang to filter by language%s\n", dim, rst)
	fmt.Printf("  %sExample: clawnet get %s --lang py%s\n", dim, query, rst)
	return nil
}

// ── top-level annotate command ──

// cmdAnnotate handles `clawnet annotate <id> <note>` / `--clear` / `--list`.
func cmdAnnotate() error {
	args := os.Args[2:]
	if len(args) == 0 {
		annotateHelp()
		return nil
	}
	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		annotateHelp()
		return nil
	}

	base, err := knowledgeBase()
	if err != nil {
		return err
	}

	// --list: show all annotations
	if args[0] == "--list" {
		return annotateList(base)
	}

	id := args[0]

	// --clear: remove annotations for this entry
	for _, a := range args[1:] {
		if a == "--clear" {
			return annotateClear(base, id)
		}
	}

	// Add annotation
	if len(args) < 2 {
		return fmt.Errorf("usage: clawnet annotate <id> \"note text\"")
	}
	note := strings.Join(args[1:], " ")
	return annotateAdd(base, id, note)
}

func getHelp() {
	bold := "\033[1m"
	dim := "\033[2m"
	tidal := "\033[38;2;69;123;157m"
	rst := "\033[0m"

	fmt.Println(bold + "clawnet get" + rst + dim + " — Fetch docs by ID" + rst)
	fmt.Println()
	fmt.Println(bold + "Usage:" + rst)
	fmt.Println(tidal + "  clawnet get <ids...> [options]" + rst)
	fmt.Println()
	fmt.Println(bold + "Options:" + rst)
	fmt.Println(dim + "  --lang <language>   Language variant: py, js, ts, go, rb (or full names)" + rst)
	fmt.Println(dim + "  --version <ver>     Specific version" + rst)
	fmt.Println(dim + "  -o, --output <path> Write to file or directory" + rst)
	fmt.Println(dim + "  --full              Fetch all files (not just entry point)" + rst)
	fmt.Println(dim + "  --file <paths>      Fetch specific file(s) by path (comma-separated)" + rst)
	fmt.Println(dim + "  --json              Output raw JSON" + rst)
	fmt.Println()
	fmt.Println(bold + "Examples:" + rst)
	fmt.Println(dim + "  clawnet get openai/chat --lang py" + rst)
	fmt.Println(dim + "  clawnet get stripe/api --lang js" + rst)
	fmt.Println(dim + "  clawnet get fastapi --full" + rst)
	fmt.Println(dim + "  clawnet get openai/chat stripe/api --lang py" + rst)
	fmt.Println(dim + "  clawnet get openai/chat --lang py -o docs/openai.md" + rst)
	fmt.Println()
	fmt.Println(dim + "  IDs are matched against source_path (Context Hub compatible)." + rst)
	fmt.Println(dim + "  Auto-syncs from Context Hub on first use if no results found." + rst)
}

func annotateHelp() {
	bold := "\033[1m"
	dim := "\033[2m"
	tidal := "\033[38;2;69;123;157m"
	rst := "\033[0m"

	fmt.Println(bold + "clawnet annotate" + rst + dim + " — Attach notes to knowledge entries" + rst)
	fmt.Println()
	fmt.Println(bold + "Usage:" + rst)
	fmt.Println(tidal + "  clawnet annotate <id> \"note text\"" + rst + dim + "   Add annotation" + rst)
	fmt.Println(tidal + "  clawnet annotate <id> --clear" + rst + dim + "          Clear annotations" + rst)
	fmt.Println(tidal + "  clawnet annotate --list" + rst + dim + "                 List all annotations" + rst)
	fmt.Println()
	fmt.Println(bold + "Examples:" + rst)
	fmt.Println(dim + "  clawnet annotate abc123 \"Great for RAG pipelines\"" + rst)
	fmt.Println(dim + "  clawnet annotate abc123 --clear" + rst)
	fmt.Println(dim + "  clawnet annotate --list" + rst)
	fmt.Println()
	fmt.Println(dim + "  Annotations are local-only (not P2P synced)." + rst)
	fmt.Println(dim + "  Shown inline when using 'clawnet get'." + rst)
}

func annotateAdd(base, id, note string) error {
	data, _ := json.Marshal(map[string]string{"note": note})
	resp, err := http.Post(base+"/api/knowledge/"+id+"/annotate", "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	green := "\033[32m"
	rst := "\033[0m"
	fmt.Printf("  %s✓ Annotation added%s\n", green, rst)
	return nil
}

func annotateClear(base, id string) error {
	req, _ := http.NewRequest("DELETE", base+"/api/knowledge/"+id+"/annotations", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	green := "\033[32m"
	rst := "\033[0m"
	fmt.Printf("  %s✓ Annotations cleared%s\n", green, rst)
	return nil
}

func annotateList(base string) error {
	// Get all knowledge with annotations — use a simple approach:
	// fetch recent knowledge feed, then check each for annotations
	resp, err := http.Get(base + "/api/knowledge/feed?limit=200")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	var entries []knowledgeEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return err
	}

	coral := "\033[38;2;247;127;0m"
	dim := "\033[2m"
	yellow := "\033[33m"
	rst := "\033[0m"

	found := 0
	for _, e := range entries {
		aResp, err := http.Get(base + "/api/knowledge/" + e.ID + "/annotations")
		if err != nil {
			continue
		}
		var annotations []struct {
			Note      string `json:"note"`
			CreatedAt string `json:"created_at"`
		}
		json.NewDecoder(aResp.Body).Decode(&annotations)
		aResp.Body.Close()

		if len(annotations) == 0 {
			continue
		}
		if found == 0 {
			fmt.Printf("  %s📝 All Annotations%s\n\n", coral, rst)
		}
		found++

		id := e.ID
		if len(id) > 8 {
			id = id[:8]
		}
		fmt.Printf("  %s%s%s %s%s%s\n", yellow, id, rst, e.Title, dim, rst)
		for _, a := range annotations {
			ts := a.CreatedAt
			if len(ts) > 10 {
				ts = ts[:10]
			}
			fmt.Printf("    %s%s%s %s\n", dim, ts, rst, a.Note)
		}
		fmt.Println()
	}

	if found == 0 {
		fmt.Printf("  %sNo annotations yet. Use 'clawnet annotate <id> \"note\"' to add one.%s\n", dim, rst)
	}
	return nil
}
