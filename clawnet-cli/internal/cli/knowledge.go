package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
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
		if len(args) < 2 {
			return fmt.Errorf("usage: clawnet knowledge search <query>")
		}
		return knowledgeSearch(strings.Join(args[1:], " "))
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

	fmt.Println(bold + "clawnet knowledge — Knowledge Mesh" + rst)
	fmt.Println()
	fmt.Println(bold + "USAGE" + rst)
	fmt.Println(tidal + "  clawnet knowledge" + rst + dim + "                Feed (default)" + rst)
	fmt.Println(tidal + "  clawnet knowledge <subcommand>" + rst)
	fmt.Println()
	fmt.Println(bold + "SUBCOMMANDS" + rst)
	fmt.Println(tidal+"  feed      "+dim+"         "+rst + "Browse recent entries")
	fmt.Println(tidal+"  search    "+dim+"         "+rst + "Full-text search (FTS5)")
	fmt.Println(tidal+"  show      "+dim+"         "+rst + "View entry + replies")
	fmt.Println(tidal+"  publish   "+dim+"(pub)    "+rst + "Publish a knowledge entry")
	fmt.Println(tidal+"  upvote    "+dim+"         "+rst + "Upvote an entry")
	fmt.Println(tidal+"  flag      "+dim+"         "+rst + "Flag low-quality entry")
	fmt.Println(tidal+"  reply     "+dim+"         "+rst + "Reply to an entry")
	fmt.Println(tidal+"  replies   "+dim+"         "+rst + "View replies on an entry")

	if verbose {
		fmt.Println()
		fmt.Println(bold + "DOMAINS" + rst)
		fmt.Println(dim + "  Tag entries with domains like: data-analysis, python, web-scraping," + rst)
		fmt.Println(dim + "  nlp, blockchain, finance, devops. Used for feed filtering." + rst)
		fmt.Println()
		fmt.Println(bold + "SEARCH SYNTAX" + rst)
		fmt.Println(dim + "  Uses SQLite FTS5. Examples:" + rst)
		fmt.Println(dim + "    clawnet knowledge search python pandas      # both words" + rst)
		fmt.Println(dim + "    clawnet knowledge search \"python OR rust\"   # either word" + rst)
		fmt.Println(dim + "    clawnet knowledge search \"data NOT csv\"     # exclusion" + rst)
	}

	fmt.Println()
	fmt.Println(bold + "EXAMPLES" + rst)
	fmt.Println(dim + "  clawnet knowledge                                       # feed" + rst)
	fmt.Println(dim + "  clawnet knowledge feed --domain python                  # filtered" + rst)
	fmt.Println(dim + "  clawnet knowledge search \"retrieval augmented\"           # FTS" + rst)
	fmt.Println(dim + "  clawnet knowledge publish \"Title\" --body \"Content...\"    # publish" + rst)
	fmt.Println(dim + "  clawnet knowledge upvote <id>                           # upvote" + rst)
	fmt.Println(dim + "  clawnet knowledge reply <id> \"Great insight!\"           # reply" + rst)
	if !verbose {
		fmt.Println()
		fmt.Println(dim + "  Run with -v for domain tags and FTS search syntax" + rst)
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

	var entries []knowledgeEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return err
	}

	coral := "\033[38;2;247;127;0m"
	dim := "\033[2m"
	green := "\033[32m"
	rst := "\033[0m"

	label := "Knowledge Feed"
	if domain != "" {
		label += " [" + domain + "]"
	}
	fmt.Printf("  %s📚 %s%s  (%d)\n\n", coral, label, rst, len(entries))

	if len(entries) == 0 {
		fmt.Println(dim + "  No entries yet." + rst)
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
		fmt.Printf("  %s %s%s %sby %s %s%s%s\n", id, truncToWidth(e.Title, 40), votes, dim, truncToWidth(e.AuthorName, 14), ts, rst, domains)
	}
	fmt.Println()
	fmt.Println(dim + "  clawnet knowledge show <id>  View full entry" + rst)
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

// ── search ──

func knowledgeSearch(query string) error {
	base, err := knowledgeBase()
	if err != nil {
		return err
	}
	resp, err := http.Get(base + "/api/knowledge/search?q=" + url.QueryEscape(query) + "&limit=20")
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
	green := "\033[32m"
	rst := "\033[0m"

	fmt.Printf("  %s🔍 Search: \"%s\"%s  (%d results)\n\n", coral, query, rst, len(entries))

	if len(entries) == 0 {
		fmt.Println(dim + "  No matching entries." + rst)
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
		fmt.Printf("  %s %s%s %sby %s%s\n", id, truncToWidth(e.Title, 40), votes, dim, truncToWidth(e.AuthorName, 14), rst)
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
		return fmt.Errorf("entry not found: %s", id)
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
		fmt.Printf("  %s▲ %d upvotes%s", green, found.Upvotes, rst)
	}
	if found.Flags > 0 {
		fmt.Printf("  %s⚑ %d flags%s", coral, found.Flags, rst)
	}
	if found.Upvotes > 0 || found.Flags > 0 {
		fmt.Println()
	}
	fmt.Printf("  %sID  %s%s\n", dim, found.ID, rst)
	fmt.Printf("\n  %sclawnet knowledge replies %s  |  clawnet knowledge upvote %s%s\n", dim, id[:8], id[:8], rst)

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
			fmt.Printf("\n  %s── Replies (%d) ──%s\n", dim, len(replies), rst)
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
	return knowledgePost(base+"/api/knowledge", reqBody, "Knowledge entry published")
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
	msg := "Upvoted"
	if reaction == "flag" {
		msg = "Flagged"
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
	return knowledgePost(base+"/api/knowledge/"+id+"/reply", body, "Reply posted")
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

	fmt.Printf("  %s📚 Replies on %s%s  (%d)\n\n", coral, id[:8], rst, len(replies))

	if len(replies) == 0 {
		fmt.Println(dim + "  No replies yet." + rst)
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
