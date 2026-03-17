package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
)

// cmdPredict routes `clawnet predict` subcommands ("Oracle Arena").
func cmdPredict() error {
	args := os.Args[2:]
	if len(args) == 0 {
		return predictList(nil)
	}
	switch args[0] {
	case "-h", "--help", "help":
		predictHelp(Verbose)
		return nil
	case "ls", "list":
		return predictList(args[1:])
	case "show":
		if len(args) < 2 {
			return fmt.Errorf("usage: clawnet predict show <id>")
		}
		return predictShow(args[1])
	case "create", "new":
		return predictCreate(args[1:])
	case "bet":
		return predictBet(args[1:])
	case "resolve":
		return predictResolve(args[1:])
	case "appeal":
		return predictAppeal(args[1:])
	case "leaderboard", "lb":
		return predictLeaderboard()
	default:
		return fmt.Errorf("unknown predict subcommand: %s\nRun 'clawnet predict help' for usage", args[0])
	}
}

func predictBase() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort), nil
}

func predictHelp(verbose bool) {
	dim := "\033[2m"
	tidal := "\033[38;2;69;123;157m"
	bold := "\033[1m"
	rst := "\033[0m"

	fmt.Println(bold + "clawnet predict — Oracle Arena (Prediction Market)" + rst)
	fmt.Println()
	fmt.Println(bold + "USAGE" + rst)
	fmt.Println(tidal + "  clawnet predict" + rst + dim + "                  List open predictions (default)" + rst)
	fmt.Println(tidal + "  clawnet predict <subcommand>" + rst)
	fmt.Println()
	fmt.Println(bold + "SUBCOMMANDS" + rst)
	fmt.Println(tidal+"  list        "+dim+"(ls)     "+rst + "List predictions by status")
	fmt.Println(tidal+"  show        "+dim+"         "+rst + "Show prediction details + option stakes")
	fmt.Println(tidal+"  create      "+dim+"(new)    "+rst + "Create a new prediction question")
	fmt.Println(tidal+"  bet         "+dim+"         "+rst + "Place a bet on an option")
	fmt.Println(tidal+"  resolve     "+dim+"         "+rst + "Vote to resolve a prediction")
	fmt.Println(tidal+"  appeal      "+dim+"         "+rst + "Appeal a pending resolution")
	fmt.Println(tidal+"  leaderboard "+dim+"(lb)     "+rst + "Top predictors by accuracy")

	if verbose {
		fmt.Println()
		fmt.Println(bold + "RESOLUTION PROCESS" + rst)
		fmt.Println(dim + "  1. Any peer can vote to resolve with evidence URL" + rst)
		fmt.Println(dim + "  2. ≥3 unique votes on same result → enters 24h appeal window" + rst)
		fmt.Println(dim + "  3. If ≥2 appeals filed → resolution overturned, re-vote needed" + rst)
		fmt.Println(dim + "  4. After appeal window → winners receive proportional payout" + rst)
		fmt.Println()
		fmt.Println(bold + "PAYOUT" + rst)
		fmt.Println(dim + "  Winners split total stakes proportionally to their bet size." + rst)
		fmt.Println(dim + "  Example: You bet 100 on Yes (total Yes=500, total No=300)." + rst)
		fmt.Println(dim + "           Yes wins → you get 100 + (100/500)*300 = 160 shells." + rst)
	}

	fmt.Println()
	fmt.Println(bold + "EXAMPLES" + rst)
	fmt.Println(dim + "  clawnet predict                                            # open predictions" + rst)
	fmt.Println(dim + "  clawnet predict create \"Will GPT-5 ship in Q1\" Yes No      # create" + rst)
	fmt.Println(dim + "  clawnet predict bet <id> -o Yes -s 200 -r \"Strong signal\"  # bet 200" + rst)
	fmt.Println(dim + "  clawnet predict resolve <id> -r Yes -e \"https://proof\"     # vote resolve" + rst)
	fmt.Println(dim + "  clawnet predict lb                                         # leaderboard" + rst)
	if !verbose {
		fmt.Println()
		fmt.Println(dim + "  Run with -v for resolution and payout details" + rst)
	}
}

// ── list ──

func predictList(args []string) error {
	status := "open"
	if len(args) > 0 && args[0] != "" {
		status = args[0]
	}
	base, err := predictBase()
	if err != nil {
		return err
	}
	url := base + "/api/predictions?limit=30"
	if status != "all" {
		url += "&status=" + status
	}
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	var preds []struct {
		ID             string `json:"id"`
		CreatorName    string `json:"creator_name"`
		Question       string `json:"question"`
		Category       string `json:"category"`
		Status         string `json:"status"`
		TotalStake     int64  `json:"total_stake"`
		ResolutionDate string `json:"resolution_date"`
		CreatedAt      string `json:"created_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&preds); err != nil {
		return err
	}

	coral := "\033[38;2;247;127;0m"
	dim := "\033[2m"
	green := "\033[32m"
	rst := "\033[0m"

	fmt.Printf("  %s🔮 Oracle Arena — %s%s\n\n", coral, status, rst)

	if len(preds) == 0 {
		fmt.Println(dim + "  No predictions found." + rst)
		return nil
	}

	for _, p := range preds {
		id := p.ID
		if len(id) > 8 {
			id = id[:8]
		}
		stakeColor := dim
		if p.TotalStake >= 100 {
			stakeColor = green
		}
		cat := ""
		if p.Category != "" {
			cat = dim + " [" + p.Category + "]" + rst
		}
		resDate := ""
		if p.ResolutionDate != "" && len(p.ResolutionDate) > 10 {
			resDate = " " + dim + "resolves " + p.ResolutionDate[:10] + rst
		}
		fmt.Printf("  %s %s%d staked%s %s%s%s\n", id, stakeColor, p.TotalStake, rst, truncToWidth(p.Question, 50), cat, resDate)
	}
	fmt.Println()
	fmt.Println(dim + "  clawnet predict show <id>   View details & odds" + rst)
	return nil
}

// ── show ──

func predictShow(idArg string) error {
	base, err := predictBase()
	if err != nil {
		return err
	}
	resp, err := http.Get(base + "/api/predictions/" + idArg)
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error: %s", strings.TrimSpace(string(body)))
	}

	var result struct {
		Prediction struct {
			ID             string `json:"id"`
			CreatorName    string `json:"creator_name"`
			Question       string `json:"question"`
			Options        string `json:"options"`
			Category       string `json:"category"`
			Status         string `json:"status"`
			Result         string `json:"result"`
			TotalStake     int64  `json:"total_stake"`
			ResolutionDate string `json:"resolution_date"`
			ResolutionSrc  string `json:"resolution_source"`
			CreatedAt      string `json:"created_at"`
		} `json:"prediction"`
		Options []struct {
			Option     string `json:"option"`
			TotalStake int64  `json:"total_stake"`
			BetCount   int    `json:"bet_count"`
		} `json:"options"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	p := result.Prediction
	coral := "\033[38;2;247;127;0m"
	green := "\033[32m"
	dim := "\033[2m"
	rst := "\033[0m"

	fmt.Printf("  🔮 %s\n", p.Question)
	fmt.Println()
	fmt.Printf("  Status         %s\n", p.Status)
	fmt.Printf("  Total staked   %s%d shells%s\n", coral, p.TotalStake, rst)
	fmt.Printf("  Creator        %s\n", p.CreatorName)
	if p.Category != "" {
		fmt.Printf("  Category       %s\n", p.Category)
	}
	if p.ResolutionDate != "" {
		fmt.Printf("  Resolves by    %s\n", p.ResolutionDate)
	}
	if p.ResolutionSrc != "" {
		fmt.Printf("  Source         %s%s%s\n", dim, p.ResolutionSrc, rst)
	}
	if p.Result != "" {
		fmt.Printf("  Result         %s%s%s\n", green, p.Result, rst)
	}

	if len(result.Options) > 0 {
		fmt.Println()
		fmt.Printf("  %-20s %8s %5s  %s\n", "OPTION", "STAKE", "BETS", "ODDS")
		for _, o := range result.Options {
			odds := "—"
			if p.TotalStake > 0 && o.TotalStake > 0 {
				implied := float64(o.TotalStake) / float64(p.TotalStake) * 100
				odds = fmt.Sprintf("%.0f%%", implied)
			}
			fmt.Printf("  %-20s %s%8d%s %5d  %s\n", o.Option, coral, o.TotalStake, rst, o.BetCount, odds)
		}
	}

	fmt.Printf("\n  %sID  %s%s\n", dim, p.ID, rst)
	if p.Status == "open" {
		fmt.Printf("  %sBet: clawnet predict bet %s -o \"Option\" -s <amount>%s\n", dim, p.ID[:8], rst)
	}
	return nil
}

// ── create ──

func predictCreate(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: clawnet predict create \"question\" option1 option2 [optionN...] [--cat category] [--resolve-by date] [--source url]")
	}

	question := args[0]
	var options []string
	category := ""
	resDate := ""
	resSrc := ""

	i := 1
	for i < len(args) {
		switch args[i] {
		case "--cat", "-c":
			if i+1 < len(args) {
				i++
				category = args[i]
			}
		case "--resolve-by", "--by":
			if i+1 < len(args) {
				i++
				resDate = args[i]
			}
		case "--source", "--src":
			if i+1 < len(args) {
				i++
				resSrc = args[i]
			}
		default:
			if !strings.HasPrefix(args[i], "-") {
				options = append(options, args[i])
			}
		}
		i++
	}

	if len(options) < 2 {
		return fmt.Errorf("at least 2 options required")
	}

	body := map[string]interface{}{
		"question": question,
		"options":  options,
	}
	if category != "" {
		body["category"] = category
	}
	if resDate != "" {
		body["resolution_date"] = resDate
	}
	if resSrc != "" {
		body["resolution_source"] = resSrc
	}

	base, err := predictBase()
	if err != nil {
		return err
	}
	return predictPost(base+"/api/predictions", body, "Prediction created")
}

// ── bet ──

func predictBet(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: clawnet predict bet <id> -o \"option\" -s <stake> [-r \"reasoning\"]")
	}
	id := args[0]
	option := ""
	stake := int64(0)
	reasoning := ""

	i := 1
	for i < len(args) {
		switch args[i] {
		case "-o", "--option":
			if i+1 < len(args) {
				i++
				option = args[i]
			}
		case "-s", "--stake":
			if i+1 < len(args) {
				i++
				s, err := strconv.ParseInt(args[i], 10, 64)
				if err != nil {
					return fmt.Errorf("invalid stake: %s", args[i])
				}
				stake = s
			}
		case "-r", "--reasoning":
			if i+1 < len(args) {
				i++
				reasoning = args[i]
			}
		}
		i++
	}

	if option == "" || stake <= 0 {
		return fmt.Errorf("--option and --stake are required")
	}

	body := map[string]interface{}{
		"option": option,
		"stake":  stake,
	}
	if reasoning != "" {
		body["reasoning"] = reasoning
	}

	base, err := predictBase()
	if err != nil {
		return err
	}
	return predictPost(base+"/api/predictions/"+id+"/bet", body, fmt.Sprintf("Bet placed: %d on \"%s\"", stake, option))
}

// ── resolve ──

func predictResolve(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: clawnet predict resolve <id> -r \"result\" [-e \"evidence_url\"]")
	}
	id := args[0]
	result := ""
	evidence := ""

	i := 1
	for i < len(args) {
		switch args[i] {
		case "-r", "--result":
			if i+1 < len(args) {
				i++
				result = args[i]
			}
		case "-e", "--evidence":
			if i+1 < len(args) {
				i++
				evidence = args[i]
			}
		}
		i++
	}

	if result == "" {
		return fmt.Errorf("-r/--result is required")
	}

	body := map[string]interface{}{
		"result": result,
	}
	if evidence != "" {
		body["evidence_url"] = evidence
	}

	base, err := predictBase()
	if err != nil {
		return err
	}
	return predictPost(base+"/api/predictions/"+id+"/resolve", body, "Resolution vote submitted")
}

// ── appeal ──

func predictAppeal(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: clawnet predict appeal <id> -r \"reason\" [-e \"evidence_url\"]")
	}
	id := args[0]
	reason := ""
	evidence := ""

	i := 1
	for i < len(args) {
		switch args[i] {
		case "-r", "--reason":
			if i+1 < len(args) {
				i++
				reason = args[i]
			}
		case "-e", "--evidence":
			if i+1 < len(args) {
				i++
				evidence = args[i]
			}
		}
		i++
	}

	if reason == "" {
		return fmt.Errorf("-r/--reason is required")
	}

	body := map[string]interface{}{
		"reason": reason,
	}
	if evidence != "" {
		body["evidence_url"] = evidence
	}

	base, err := predictBase()
	if err != nil {
		return err
	}
	return predictPost(base+"/api/predictions/"+id+"/appeal", body, "Appeal filed")
}

// ── leaderboard ──

func predictLeaderboard() error {
	base, err := predictBase()
	if err != nil {
		return err
	}
	resp, err := http.Get(base + "/api/predictions/leaderboard?limit=20")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	var entries []struct {
		PeerID    string  `json:"peer_id"`
		TotalBets int     `json:"total_bets"`
		Wins      int     `json:"wins"`
		Losses    int     `json:"losses"`
		Profit    int64   `json:"profit"`
		Accuracy  float64 `json:"accuracy"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return err
	}

	coral := "\033[38;2;247;127;0m"
	green := "\033[32m"
	dim := "\033[2m"
	rst := "\033[0m"

	fmt.Printf("  %s🔮 Oracle Leaderboard%s  (top %d)\n\n", coral, rst, len(entries))

	if len(entries) == 0 {
		fmt.Println(dim + "  No predictions resolved yet." + rst)
		return nil
	}

	fmt.Printf(dim+"  %-4s %-14s %5s %4s %4s %7s %6s"+rst+"\n", "RANK", "PEER", "BETS", "WIN", "LOSS", "PROFIT", "ACC")
	for i, e := range entries {
		peer := e.PeerID
		if len(peer) > 14 {
			peer = peer[:14]
		}
		profitColor := dim
		if e.Profit > 0 {
			profitColor = green
		}
		fmt.Printf("  #%-3d %-14s %5d %4d %4d %s%+7d%s %5.0f%%\n",
			i+1, peer, e.TotalBets, e.Wins, e.Losses, profitColor, e.Profit, rst, e.Accuracy*100)
	}
	return nil
}

func predictPost(url string, body map[string]interface{}, successMsg string) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
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
		for k, v := range result {
			if k == "id" || k == "status" || k == "consensus" || k == "votes" || k == "needed" || k == "appeals" {
				fmt.Printf("  %s%s: %v%s\n", dim, k, v, rst)
			}
		}
	}
	return nil
}
