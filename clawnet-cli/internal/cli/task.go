package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/i18n"
)

// cmdTask routes `clawnet task` subcommands.
// With no args, shows the board dashboard (same as `clawnet board`).
func cmdTask() error {
	args := os.Args[2:]
	if len(args) == 0 {
		return cmdBoard()
	}

	sub := args[0]
	switch sub {
	case "-h", "--help", "help":
		taskHelp(Verbose)
		return nil
	case "ls", "list":
		return taskList(args[1:])
	case "show":
		if len(args) < 2 {
			return fmt.Errorf("usage: clawnet task show <id>")
		}
		return taskShow(args[1])
	case "create", "new":
		return taskCreate(args[1:])
	case "bid":
		return taskBid(args[1:])
	case "bids":
		if len(args) < 2 {
			return fmt.Errorf("usage: clawnet task bids <id>")
		}
		return taskBids(args[1])
	case "assign":
		return taskAssign(args[1:])
	case "claim":
		return taskClaim(args[1:])
	case "submit":
		return taskSubmit(args[1:])
	case "work":
		return taskWork(args[1:])
	case "submissions", "subs":
		if len(args) < 2 {
			return fmt.Errorf("usage: clawnet task submissions <id>")
		}
		return taskSubmissions(args[1])
	case "pick":
		return taskPick(args[1:])
	case "approve":
		if len(args) < 2 {
			return fmt.Errorf("usage: clawnet task approve <id>")
		}
		return taskApprove(args[1])
	case "reject":
		if len(args) < 2 {
			return fmt.Errorf("usage: clawnet task reject <id>")
		}
		return taskReject(args[1])
	case "cancel":
		if len(args) < 2 {
			return fmt.Errorf("usage: clawnet task cancel <id>")
		}
		return taskCancel(args[1])
	case "download", "dl":
		return taskDownload(args[1:])
	default:
		return fmt.Errorf("unknown task subcommand: %s\nRun 'clawnet task help' for usage", sub)
	}
}

func taskBase() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort), nil
}

func taskHelp(verbose bool) {
	dim := "\033[2m"
	tidal := "\033[38;2;69;123;157m"
	coral := "\033[38;2;247;127;0m"
	bold := "\033[1m"
	rst := "\033[0m"

	fmt.Println(bold + i18n.T("help.task") + rst)
	fmt.Println()
	fmt.Println(bold + i18n.T("common.usage") + rst)
	fmt.Println(tidal + "  clawnet task" + rst + dim + "                   " + i18n.T("help.task.dashboard") + rst)
	fmt.Println(tidal + "  clawnet task <subcommand>" + rst + dim + "      " + i18n.T("help.task.execute") + rst)
	fmt.Println(tidal + "  clawnet task <subcommand> --json" + rst + dim + " " + i18n.T("help.task.json") + rst)
	fmt.Println()
	fmt.Println(bold + i18n.T("help.task.subcmds") + rst)
	fmt.Println(tidal+"  list     "+dim+"(ls)     "+rst + i18n.T("help.task.list"))
	fmt.Println(tidal+"  show     "+dim+"         "+rst + i18n.T("help.task.show"))
	fmt.Println(tidal+"  create   "+dim+"(new)    "+rst + i18n.T("help.task.create"))
	fmt.Println(tidal+"  bid      "+dim+"         "+rst + i18n.T("help.task.bid"))
	fmt.Println(tidal+"  bids     "+dim+"         "+rst + i18n.T("help.task.bids"))
	fmt.Println(tidal+"  assign   "+dim+"         "+rst + i18n.T("help.task.assign"))
	fmt.Println(tidal+"  claim    "+dim+"         "+rst + i18n.T("help.task.claim"))
	fmt.Println(tidal+"  submit   "+dim+"         "+rst + i18n.T("help.task.submit"))
	fmt.Println(tidal+"  work     "+dim+"         "+rst + i18n.T("help.task.work"))
	fmt.Println(tidal+"  subs     "+dim+"(submissions)"+rst + " " + i18n.T("help.task.subs"))
	fmt.Println(tidal+"  pick     "+dim+"         "+rst + i18n.T("help.task.pick"))
	fmt.Println(tidal+"  approve  "+dim+"         "+rst + i18n.T("help.task.approve"))
	fmt.Println(tidal+"  reject   "+dim+"         "+rst + i18n.T("help.task.reject"))
	fmt.Println(tidal+"  cancel   "+dim+"         "+rst + i18n.T("help.task.cancel"))
	fmt.Println(tidal+"  download "+dim+"(dl)     "+rst + i18n.T("help.task.download"))

	if verbose {
		fmt.Println()
		fmt.Println(bold + i18n.T("help.task.modes") + rst)
		fmt.Println(dim + "  " + i18n.T("help.task.mode_simple") + rst)
		fmt.Println(dim + "  " + i18n.T("help.task.mode_auction") + rst)
		fmt.Println(dim + "  " + i18n.T("help.task.mode_house") + rst)
		fmt.Println()
		fmt.Println(bold + i18n.T("help.task.lifecycle") + rst)
		fmt.Println(dim + "  " + i18n.T("help.task.lc_simple") + rst)
		fmt.Println(dim + "  " + i18n.T("help.task.lc_auction") + rst)
		fmt.Println(dim + "  " + i18n.T("help.task.lc_house") + rst)
		fmt.Println()
		fmt.Println(bold + i18n.T("help.task.legend") + rst)
		fmt.Println(coral+"  open       "+rst+dim + i18n.T("help.task.st_open") + rst)
		fmt.Println(coral+"  assigned   "+rst+dim + i18n.T("help.task.st_assigned") + rst)
		fmt.Println(coral+"  submitted  "+rst+dim + i18n.T("help.task.st_submitted") + rst)
		fmt.Println(coral+"  approved   "+rst+dim + i18n.T("help.task.st_approved") + rst)
		fmt.Println(coral+"  settled    "+rst+dim + i18n.T("help.task.st_settled") + rst)
		fmt.Println(coral+"  rejected   "+rst+dim + i18n.T("help.task.st_rejected") + rst)
		fmt.Println(coral+"  cancelled  "+rst+dim + i18n.T("help.task.st_cancelled") + rst)
	}

	fmt.Println()
	fmt.Println(bold + i18n.T("common.examples") + rst)
	fmt.Println(dim + "  clawnet task                                    # dashboard" + rst)
	fmt.Println(dim + "  clawnet task list                               # open tasks" + rst)
	fmt.Println(dim + "  clawnet task list assigned                      # assigned tasks" + rst)
	fmt.Println(dim + "  clawnet task create \"Review PR\" --reward 200     # simple task" + rst)
	fmt.Println(dim + "  clawnet task create \"Design\" -r 500 --auction   # auction task" + rst)
	fmt.Println(dim + "  clawnet task create --nut ./my-task               # from nutshell dir" + rst)
	fmt.Println(dim + "  clawnet task bid <id> -a 150 -m \"I can do it\"  # bid 150 shells" + rst)
	fmt.Println(dim + "  clawnet task claim <id> \"result text\" -s 0.85  # claim simple" + rst)
	fmt.Println(dim + "  clawnet task claim <id> --unpack ./workspace     # claim + unpack .nut" + rst)
	fmt.Println(dim + "  clawnet task submit <id> --nut ./workspace        # pack + submit .nut" + rst)
	fmt.Println(dim + "  clawnet task download <id> -o task.nut            # download .nut" + rst)
	fmt.Println(dim + "  clawnet task approve <id>                        # approve & pay" + rst)

	if !verbose {
		fmt.Println()
		fmt.Println(dim + "  " + i18n.T("help.task.verbose_hint") + rst)
	}
}

// ── list ──

func taskList(args []string) error {
	status := "open"
	page := 1
	for _, a := range args {
		if p, err := strconv.Atoi(a); err == nil && p > 0 {
			page = p
		} else {
			status = a
		}
	}

	const perPage = 20
	offset := (page - 1) * perPage
	base, err := taskBase()
	if err != nil {
		return err
	}

	url := base + fmt.Sprintf("/api/tasks?limit=%d&offset=%d", perPage+1, offset)
	if status != "" && status != "all" {
		url += "&status=" + status
	}
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	if JSONOutput {
		body, _ := io.ReadAll(resp.Body)
		fmt.Println(string(body))
		return nil
	}

	var tasks []struct {
		ID          string `json:"id"`
		AuthorName  string `json:"author_name"`
		Title       string `json:"title"`
		Reward      int64  `json:"reward"`
		Status      string `json:"status"`
		Mode        string `json:"mode"`
		AssignedTo  string `json:"assigned_to"`
		CreatedAt   string `json:"created_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return err
	}

	hasMore := len(tasks) > perPage
	if hasMore {
		tasks = tasks[:perPage]
	}

	coral := "\033[38;2;247;127;0m"
	dim := "\033[2m"
	green := "\033[32m"
	yellow := "\033[33m"
	rst := "\033[0m"

	fmt.Printf("%s  %s%s\n\n", coral, i18n.Tf("task.list_header", status), rst)

	if len(tasks) == 0 {
		fmt.Println(dim + "  " + i18n.T("task.none") + rst)
		return nil
	}

	fmt.Printf(dim+"  %-8s %-10s %6s %-7s %s %s %s"+rst+"\n",
		i18n.T("task.col.id"), i18n.T("task.col.status"), i18n.T("task.col.reward"), i18n.T("task.col.mode"), padRight(i18n.T("task.col.author"), 14), padRight(i18n.T("task.col.title"), 22), i18n.T("task.col.created"))
	for _, t := range tasks {
		id := t.ID
		if len(id) > 8 {
			id = id[:8]
		}
		created := t.CreatedAt
		if len(created) > 10 {
			created = created[:10]
		}
		statusColor := dim
		switch t.Status {
		case "open":
			statusColor = green
		case "assigned":
			statusColor = yellow
		case "submitted":
			statusColor = coral
		case "approved", "settled":
			statusColor = green
		}
		mode := t.Mode
		if mode == "" {
			mode = "simple"
		}
		author := t.AuthorName
		if author == "" {
			author = "-"
		}
		authorPad := padRight(truncToWidth(author, 14), 14)
		titlePad := padRight(truncToWidth(t.Title, 22), 22)
		fmt.Printf("  %-8s %s%-10s%s %6d %-7s %s %s %s\n",
			id, statusColor, t.Status, rst, t.Reward, mode, authorPad, titlePad, created)
	}

	fmt.Println()
	if page > 1 || hasMore {
		nav := fmt.Sprintf("  Page %d", page)
		if hasMore {
			nav += fmt.Sprintf("  →  clawnet task list %s %d", status, page+1)
		}
		fmt.Println(dim + nav + rst)
	}
	fmt.Println(dim + "  " + i18n.T("task.hint_show") + rst)
	return nil
}

// ── show ──

func taskShow(idArg string) error {
	base, err := taskBase()
	if err != nil {
		return err
	}
	fullID, err := resolveTaskID(base, idArg)
	if err != nil {
		return err
	}
	resp, err := http.Get(base + "/api/tasks/" + fullID)
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error: %s", strings.TrimSpace(string(body)))
	}

	if JSONOutput {
		body, _ := io.ReadAll(resp.Body)
		fmt.Println(string(body))
		return nil
	}

	var t struct {
		ID            string  `json:"id"`
		AuthorName    string  `json:"author_name"`
		Title         string  `json:"title"`
		Description   string  `json:"description"`
		Tags          string  `json:"tags"`
		Reward        int64   `json:"reward"`
		Status        string  `json:"status"`
		Mode          string  `json:"mode"`
		AssignedTo    string  `json:"assigned_to"`
		Result        string  `json:"result"`
		TargetPeer    string  `json:"target_peer"`
		Deadline      string  `json:"deadline"`
		BidCloseAt    string  `json:"bid_close_at"`
		WorkDeadline  string  `json:"work_deadline"`
		SelfEvalScore float64 `json:"self_eval_score"`
		CreatedAt     string  `json:"created_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return err
	}

	coral := "\033[38;2;247;127;0m"
	dim := "\033[2m"
	green := "\033[32m"
	rst := "\033[0m"

	fmt.Printf("  🦞 %s\n", t.Title)
	if t.Description != "" {
		fmt.Printf("  %s%s%s\n", dim, t.Description, rst)
	}
	fmt.Println()

	mode := t.Mode
	if mode == "" {
		mode = "simple"
	}
	fmt.Printf("  %-10s  %s\n", i18n.T("task.field.status"), t.Status)
	fmt.Printf("  %-10s  %s\n", i18n.T("task.field.mode"), mode)
	fmt.Printf("  %-10s  %s%s%s\n", "🐚", coral, i18n.Tf("task.field.reward", t.Reward), rst)
	fmt.Printf("  %-10s  %s\n", i18n.T("task.field.author"), t.AuthorName)
	fmt.Printf("  %-10s  %s\n", i18n.T("task.field.created"), t.CreatedAt)
	if t.Tags != "" && t.Tags != "[]" {
		fmt.Printf("  %-10s  %s\n", i18n.T("task.field.tags"), t.Tags)
	}
	if t.Deadline != "" {
		fmt.Printf("  %-10s  %s\n", i18n.T("task.field.deadline"), t.Deadline)
	}
	if t.BidCloseAt != "" {
		fmt.Printf("  %-10s  %s\n", i18n.T("task.field.bid_close"), t.BidCloseAt)
	}
	if t.WorkDeadline != "" {
		fmt.Printf("  %-10s  %s\n", i18n.T("task.field.work_due"), t.WorkDeadline)
	}
	if t.AssignedTo != "" {
		fmt.Printf("  %-10s  %s\n", i18n.T("task.field.assigned"), t.AssignedTo)
	}
	if t.TargetPeer != "" {
		fmt.Printf("  %-10s  %s%s%s\n", i18n.T("task.field.target"), dim, t.TargetPeer, rst)
	}
	if t.Result != "" {
		fmt.Printf("\n  %s%s%s\n", green, i18n.T("task.field.result"), rst)
		fmt.Printf("  %s\n", t.Result)
	}
	fmt.Printf("  %sID          %s%s\n", dim, t.ID, rst)

	// Show available actions
	fmt.Println()
	switch t.Status {
	case "open":
		if mode == "simple" {
			fmt.Printf("  %sActions: clawnet task claim %s \"result\" -s 0.85%s\n", dim, safePrefix(t.ID, 8), rst)
		} else {
			fmt.Printf("  %sActions: clawnet task bid %s -a <amount>%s\n", dim, safePrefix(t.ID, 8), rst)
		}
	case "assigned":
		fmt.Printf("  %sActions: clawnet task submit %s \"result\"%s\n", dim, safePrefix(t.ID, 8), rst)
	case "submitted":
		fmt.Printf("  %sActions: clawnet task approve %s  |  clawnet task reject %s%s\n", dim, safePrefix(t.ID, 8), safePrefix(t.ID, 8), rst)
	}
	return nil
}

// ── create ──

func taskCreate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: clawnet task create \"title\" [--reward N] [--desc \"...\"] [--auction] [--tags \"a,b\"] [--deadline \"RFC3339\"] [--target <peer_id>] [--nut <dir>]")
	}

	title := ""
	reward := int64(0)
	desc := ""
	mode := "simple"
	tags := ""
	deadline := ""
	targetPeer := ""
	nutDir := ""

	i := 0
	for i < len(args) {
		switch args[i] {
		case "-r", "--reward":
			if i+1 < len(args) {
				i++
				r, err := strconv.ParseInt(args[i], 10, 64)
				if err != nil {
					return fmt.Errorf("invalid reward: %s", args[i])
				}
				reward = r
			}
		case "--desc", "-d":
			if i+1 < len(args) {
				i++
				desc = args[i]
			}
		case "--auction":
			mode = "auction"
		case "--tags":
			if i+1 < len(args) {
				i++
				tags = args[i]
			}
		case "--deadline":
			if i+1 < len(args) {
				i++
				deadline = args[i]
			}
		case "--target":
			if i+1 < len(args) {
				i++
				targetPeer = args[i]
			}
		case "--nut":
			if i+1 < len(args) {
				i++
				nutDir = args[i]
			}
		default:
			if title == "" {
				title = args[i]
			}
		}
		i++
	}

	// --nut: read nutshell.json manifest for task metadata
	if nutDir != "" {
		manifest, err := readNutshellManifest(nutDir)
		if err != nil {
			return err
		}
		if title == "" {
			title = manifest.Title
		}
		if desc == "" {
			desc = manifest.Summary
		}
		if tags == "" && manifest.Skills != "" {
			tags = manifest.Skills
		}
		if deadline == "" && manifest.Deadline != "" {
			deadline = manifest.Deadline
		}
		if reward == 0 && manifest.Reward > 0 {
			reward = int64(manifest.Reward)
		}
	}

	if title == "" {
		return fmt.Errorf("title is required")
	}

	body := map[string]interface{}{
		"title":  title,
		"reward": reward,
		"mode":   mode,
	}
	if desc != "" {
		body["description"] = desc
	}
	if tags != "" {
		body["tags"] = tags
	}
	if deadline != "" {
		body["deadline"] = deadline
	}
	if targetPeer != "" {
		body["target_peer"] = targetPeer
	}

	base, err := taskBase()
	if err != nil {
		return err
	}

	result, err := taskPostReturn(base+"/api/tasks", body, i18n.T("task.created"))
	if err != nil {
		return err
	}

	// If --nut was specified and task created, upload the bundle via nutshell CLI
	if nutDir != "" {
		taskID, _ := result["task_id"].(string)
		if taskID == "" {
			taskID, _ = result["id"].(string)
		}
		if taskID != "" {
			return taskUploadNut(nutDir, taskID, base)
		}
	}
	return nil
}

// ── bid ──

func taskBid(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: clawnet task bid <id> [-a amount] [-m message]")
	}
	id := args[0]
	amount := int64(0)
	message := ""

	i := 1
	for i < len(args) {
		switch args[i] {
		case "-a", "--amount":
			if i+1 < len(args) {
				i++
				a, err := strconv.ParseInt(args[i], 10, 64)
				if err != nil {
					return fmt.Errorf("invalid amount: %s", args[i])
				}
				amount = a
			}
		case "-m", "--message":
			if i+1 < len(args) {
				i++
				message = args[i]
			}
		default:
			if message == "" && !strings.HasPrefix(args[i], "-") {
				message = args[i]
			}
		}
		i++
	}

	body := map[string]interface{}{
		"amount":  amount,
		"message": message,
	}
	base, err := taskBase()
	if err != nil {
		return err
	}
	return taskPost(base+"/api/tasks/"+id+"/bid", body, i18n.T("task.bid_placed"))
}

// ── bids ──

func taskBids(id string) error {
	base, err := taskBase()
	if err != nil {
		return err
	}
	resp, err := http.Get(base + "/api/tasks/" + id + "/bids")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	var bids []struct {
		ID         string `json:"id"`
		BidderName string `json:"bidder_name"`
		BidderID   string `json:"bidder_id"`
		Amount     int64  `json:"amount"`
		Message    string `json:"message"`
		CreatedAt  string `json:"created_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&bids); err != nil {
		return err
	}

	dim := "\033[2m"
	coral := "\033[38;2;247;127;0m"
	rst := "\033[0m"

	fmt.Printf("  %s  (%s)\n\n", i18n.Tf("task.bids_header", safePrefix(id, 8)), i18n.Tf("task.bids_count", len(bids)))

	if len(bids) == 0 {
		fmt.Println(dim + "  " + i18n.T("task.no_bids") + rst)
		return nil
	}

	for i, b := range bids {
		name := b.BidderName
		if name == "" {
			name = safePrefix(b.BidderID, 12)
		}
		fmt.Printf("  #%d %s  %s%d shells%s", i+1, name, coral, b.Amount, rst)
		if b.Message != "" {
			fmt.Printf("  %s\"%s\"%s", dim, b.Message, rst)
		}
		fmt.Println()
	}
	fmt.Println()
	fmt.Printf("  %s%s%s\n", dim, i18n.Tf("task.hint_assign", safePrefix(id, 8)), rst)
	return nil
}

// ── assign ──

func taskAssign(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: clawnet task assign <id> --to <peer_id>")
	}
	id := args[0]
	peerID := ""
	for i := 1; i < len(args); i++ {
		if (args[i] == "--to" || args[i] == "-t") && i+1 < len(args) {
			i++
			peerID = args[i]
		}
	}
	if peerID == "" {
		return fmt.Errorf("--to <peer_id> is required")
	}
	body := map[string]interface{}{
		"assign_to": peerID,
	}
	base, err := taskBase()
	if err != nil {
		return err
	}
	return taskPost(base+"/api/tasks/"+id+"/assign", body, i18n.T("task.assigned"))
}

// ── claim (simple mode) ──

func taskClaim(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: clawnet task claim <id> [\"result\" [-s score]] [--unpack <dir>]")
	}
	id := args[0]
	result := ""
	score := 0.8
	unpackDir := ""

	i := 1
	for i < len(args) {
		switch args[i] {
		case "-s", "--score":
			if i+1 < len(args) {
				i++
				s, err := strconv.ParseFloat(args[i], 64)
				if err != nil {
					return fmt.Errorf("invalid score: %s", args[i])
				}
				score = s
			}
		case "--unpack":
			if i+1 < len(args) {
				i++
				unpackDir = args[i]
			}
		default:
			if result == "" && !strings.HasPrefix(args[i], "-") {
				result = args[i]
			}
		}
		i++
	}

	// --unpack mode: claim task and download + unpack .nut bundle
	if unpackDir != "" {
		base, err := taskBase()
		if err != nil {
			return err
		}
		// Use nutshell claim if available
		nutPath, nutErr := exec.LookPath("nutshell")
		if nutErr != nil {
			return fmt.Errorf("%s", i18n.T("task.nutshell_required"))
		}
		fullID, err := resolveTaskID(base, id)
		if err != nil {
			return err
		}
		cmd := exec.Command(nutPath, "claim", fullID, "-o", unpackDir)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	if result == "" {
		return fmt.Errorf("result text is required (or use --unpack <dir> to claim + download .nut)")
	}

	body := map[string]interface{}{
		"result":          result,
		"self_eval_score": score,
	}
	base, err := taskBase()
	if err != nil {
		return err
	}
	return taskPost(base+"/api/tasks/"+id+"/claim", body, i18n.T("task.claimed"))
}

// ── submit (assigned task) ──

func taskSubmit(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: clawnet task submit <id> \"result\" [--nut <dir>]")
	}
	id := args[0]
	result := ""
	nutDir := ""

	i := 1
	for i < len(args) {
		switch args[i] {
		case "--nut":
			if i+1 < len(args) {
				i++
				nutDir = args[i]
			}
		default:
			if result == "" && !strings.HasPrefix(args[i], "-") {
				result = args[i]
			}
		}
		i++
	}

	// --nut mode: pack + submit via nutshell deliver
	if nutDir != "" {
		nutPath, nutErr := exec.LookPath("nutshell")
		if nutErr != nil {
			return fmt.Errorf("%s", i18n.T("task.nutshell_required"))
		}
		cmd := exec.Command(nutPath, "deliver", "--dir", nutDir)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	if result == "" {
		return fmt.Errorf("result text is required (or use --nut <dir> to pack + submit delivery)")
	}

	body := map[string]interface{}{
		"result": result,
	}
	base, err := taskBase()
	if err != nil {
		return err
	}
	return taskPost(base+"/api/tasks/"+id+"/submit", body, i18n.T("task.submitted"))
}

// ── work (auction house parallel submission) ──

func taskWork(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: clawnet task work <id> \"result\"")
	}
	id := args[0]
	result := args[1]

	body := map[string]interface{}{
		"result": result,
	}
	base, err := taskBase()
	if err != nil {
		return err
	}
	return taskPost(base+"/api/tasks/"+id+"/work", body, i18n.T("task.work_submitted"))
}

// ── submissions ──

func taskSubmissions(id string) error {
	base, err := taskBase()
	if err != nil {
		return err
	}
	resp, err := http.Get(base + "/api/tasks/" + id + "/submissions")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	var subs []struct {
		ID          string `json:"id"`
		WorkerName  string `json:"worker_name"`
		WorkerID    string `json:"worker_id"`
		Result      string `json:"result"`
		IsWinner    bool   `json:"is_winner"`
		SubmittedAt string `json:"submitted_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&subs); err != nil {
		return err
	}

	dim := "\033[2m"
	coral := "\033[38;2;247;127;0m"
	green := "\033[32m"
	rst := "\033[0m"

	fmt.Printf("  %s  (%d)\n\n", i18n.Tf("task.submissions_header", safePrefix(id, 8)), len(subs))

	if len(subs) == 0 {
		fmt.Println(dim + "  " + i18n.T("task.no_submissions") + rst)
		return nil
	}

	for i, s := range subs {
		winner := ""
		if s.IsWinner {
			winner = green + " " + i18n.T("task.winner") + rst
		}
		name := s.WorkerName
		if name == "" {
			name = safePrefix(s.WorkerID, 12)
		}
		fmt.Printf("  #%d %s%s%s  %s%s%s\n", i+1, coral, name, rst, dim, safePrefix(s.SubmittedAt, 10), rst+winner)
		preview := s.Result
		if len(preview) > 80 {
			preview = preview[:77] + "..."
		}
		fmt.Printf("     %s%s%s\n", dim, preview, rst)
	}
	fmt.Println()
	fmt.Printf("  %s%s%s\n", dim, i18n.Tf("task.hint_pick", safePrefix(id, 8)), rst)
	return nil
}

// ── pick ──

func taskPick(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: clawnet task pick <task_id> --sub <submission_id>")
	}
	id := args[0]
	subID := ""
	for i := 1; i < len(args); i++ {
		if (args[i] == "--sub" || args[i] == "-s") && i+1 < len(args) {
			i++
			subID = args[i]
		}
	}
	if subID == "" {
		return fmt.Errorf("--sub <submission_id> is required")
	}
	body := map[string]interface{}{
		"submission_id": subID,
	}
	base, err := taskBase()
	if err != nil {
		return err
	}
	return taskPost(base+"/api/tasks/"+id+"/pick", body, i18n.T("task.settled"))
}

// ── approve / reject / cancel ──

func taskApprove(id string) error {
	base, err := taskBase()
	if err != nil {
		return err
	}
	return taskPost(base+"/api/tasks/"+id+"/approve", nil, i18n.T("task.approved"))
}

func taskReject(id string) error {
	base, err := taskBase()
	if err != nil {
		return err
	}
	return taskPost(base+"/api/tasks/"+id+"/reject", nil, i18n.T("task.rejected"))
}

func taskCancel(id string) error {
	base, err := taskBase()
	if err != nil {
		return err
	}
	return taskPost(base+"/api/tasks/"+id+"/cancel", nil, i18n.T("task.cancelled"))
}

// taskPost sends a POST request with JSON body and prints a success message.
func taskPost(url string, body map[string]interface{}, successMsg string) error {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(data)
	} else {
		reqBody = bytes.NewReader([]byte("{}"))
	}

	resp, err := http.Post(url, "application/json", reqBody)
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
	fmt.Printf("  %s✓ %s%s\n", green, successMsg, rst)

	// Print response details if available
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err == nil {
		dim := "\033[2m"
		for k, v := range result {
			if k == "status" || k == "task_id" || k == "id" || k == "reward_paid" || k == "milestone_completed" {
				fmt.Printf("  %s%s: %v%s\n", dim, k, v, rst)
			}
		}
	}
	return nil
}

// resolveTaskID resolves a short (prefix) task ID to the full UUID.
func resolveTaskID(base, short string) (string, error) {
	if len(short) >= 32 {
		return short, nil // already full
	}
	resp, err := http.Get(base + "/api/tasks?limit=200")
	if err != nil {
		return "", fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()
	var tasks []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return "", err
	}
	for _, t := range tasks {
		if strings.HasPrefix(t.ID, short) {
			return t.ID, nil
		}
	}
	return short, nil // fall through, let API return 404
}

// ── download ──

// taskDownload downloads the .nut bundle attached to a task.
func taskDownload(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: clawnet task download <id> [-o <path>]")
	}
	id := args[0]
	outPath := ""

	for i := 1; i < len(args); i++ {
		if (args[i] == "-o" || args[i] == "--output") && i+1 < len(args) {
			i++
			outPath = args[i]
		}
	}

	base, err := taskBase()
	if err != nil {
		return err
	}
	fullID, err := resolveTaskID(base, id)
	if err != nil {
		return err
	}

	if outPath == "" {
		outPath = safePrefix(fullID, 8) + ".nut"
	}

	resp, err := http.Get(base + "/api/tasks/" + fullID + "/bundle")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return fmt.Errorf("no .nut bundle attached to task %s", id)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(f, resp.Body)
	f.Close()
	if copyErr != nil {
		os.Remove(outPath)
		return copyErr
	}

	green := "\033[32m"
	dim := "\033[2m"
	rst := "\033[0m"
	fmt.Printf("  %s%s%s\n", green, i18n.T("task.bundle_downloaded"), rst)
	fmt.Printf("  %s%s%s\n", dim, i18n.Tf("task.bundle_file", outPath, n), rst)
	return nil
}

// ── nutshell helpers ──

// nutshellManifest holds the fields we extract from a nutshell.json.
type nutshellManifest struct {
	Title    string
	Summary  string
	Skills   string
	Deadline string
	Reward   float64
}

// readNutshellManifest reads a nutshell.json from a directory and extracts task fields.
func readNutshellManifest(dir string) (*nutshellManifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "nutshell.json"))
	if err != nil {
		return nil, fmt.Errorf("cannot read nutshell.json in %s: %w", dir, err)
	}
	var raw struct {
		Task struct {
			Title   string `json:"title"`
			Summary string `json:"summary"`
		} `json:"task"`
		ExpiresAt string `json:"expires_at"`
		Tags      struct {
			SkillsRequired []string `json:"skills_required"`
		} `json:"tags"`
		Extensions map[string]json.RawMessage `json:"extensions"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid nutshell.json: %w", err)
	}
	m := &nutshellManifest{
		Title:    raw.Task.Title,
		Summary:  raw.Task.Summary,
		Deadline: raw.ExpiresAt,
	}
	if len(raw.Tags.SkillsRequired) > 0 {
		m.Skills = strings.Join(raw.Tags.SkillsRequired, ",")
	}
	// Extract reward from extensions.clawnet.reward.amount
	if ext, ok := raw.Extensions["clawnet"]; ok {
		var clawExt map[string]interface{}
		json.Unmarshal(ext, &clawExt)
		if rw, ok := clawExt["reward"].(map[string]interface{}); ok {
			if amt, ok := rw["amount"].(float64); ok && amt > 0 {
				m.Reward = amt
			}
		}
	}
	return m, nil
}

// taskPostReturn sends a POST request and returns the parsed JSON response.
func taskPostReturn(url string, body map[string]interface{}, successMsg string) (map[string]interface{}, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(data)
	} else {
		reqBody = bytes.NewReader([]byte("{}"))
	}

	resp, err := http.Post(url, "application/json", reqBody)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("error (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	green := "\033[32m"
	dim := "\033[2m"
	rst := "\033[0m"
	fmt.Printf("  %s✓ %s%s\n", green, successMsg, rst)

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err == nil {
		for k, v := range result {
			if k == "status" || k == "task_id" || k == "id" || k == "reward_paid" || k == "milestone_completed" {
				fmt.Printf("  %s%s: %v%s\n", dim, k, v, rst)
			}
		}
	}
	return result, nil
}

// taskUploadNut packs and uploads a .nut bundle to a task.
func taskUploadNut(nutDir, taskID, base string) error {
	nutPath, err := exec.LookPath("nutshell")
	if err != nil {
		return fmt.Errorf("%s", i18n.T("task.nutshell_required"))
	}

	// Pack the bundle
	nutFile := filepath.Join(os.TempDir(), safePrefix(taskID, 8)+".nut")
	cmd := exec.Command(nutPath, "pack", "--dir", nutDir, "-o", nutFile)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nutshell pack failed: %w", err)
	}
	defer os.Remove(nutFile)

	// Read packed bundle
	bundleData, err := os.ReadFile(nutFile)
	if err != nil {
		return err
	}

	// Upload via multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("bundle", filepath.Base(nutFile))
	if err != nil {
		return err
	}
	part.Write(bundleData)
	writer.Close()

	req, err := http.NewRequest("POST", base+"/api/tasks/"+taskID+"/bundle", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload bundle: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	dim := "\033[2m"
	rst := "\033[0m"
	fmt.Printf("  %s%s%s\n", dim, i18n.Tf("task.bundle_uploaded", safePrefix(taskID, 8)), rst)
	return nil
}
