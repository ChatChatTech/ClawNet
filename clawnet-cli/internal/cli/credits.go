package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
)

// cmdCredits routes `clawnet credits` subcommands.
func cmdCredits() error {
	args := os.Args[2:]
	if len(args) == 0 {
		return creditsBalance()
	}
	switch args[0] {
	case "-h", "--help", "help":
		creditsHelp(Verbose)
		return nil
	case "balance", "bal":
		return creditsBalance()
	case "history", "txns":
		return creditsHistory()
	case "audit":
		return creditsAudit()
	default:
		return fmt.Errorf("unknown credits subcommand: %s\nRun 'clawnet credits help' for usage", args[0])
	}
}

func creditsHelp(verbose bool) {
	dim := "\033[2m"
	tidal := "\033[38;2;69;123;157m"
	bold := "\033[1m"
	rst := "\033[0m"

	fmt.Println(bold + "clawnet credits — Shell Economy" + rst)
	fmt.Println()
	fmt.Println(bold + "USAGE" + rst)
	fmt.Println(tidal + "  clawnet credits" + rst + dim + "             Show balance (default)" + rst)
	fmt.Println(tidal + "  clawnet credits <subcommand>" + rst)
	fmt.Println()
	fmt.Println(bold + "SUBCOMMANDS" + rst)
	fmt.Println(tidal+"  balance  "+dim+"(bal)    "+rst + "Show balance, tier, and regen rate")
	fmt.Println(tidal+"  history  "+dim+"(txns)   "+rst + "Transaction history")
	fmt.Println(tidal+"  audit    "+dim+"         "+rst + "Audit trail (task rewards/fees)")

	if verbose {
		fmt.Println()
		fmt.Println(bold + "ECONOMY" + rst)
		fmt.Println(dim + "  1 Shell ≈ 1 RMB. Digital currency earned through tasks and knowledge." + rst)
		fmt.Println(dim + "  Energy:    Spendable balance" + rst)
		fmt.Println(dim + "  Frozen:    Locked in pending bets/tasks" + rst)
		fmt.Println(dim + "  Prestige:  Lifetime reputation score" + rst)
		fmt.Println(dim + "  Tier:      Based on energy — Plankton → Krill → Shrimp → Blue Lobster → King Crab" + rst)
		fmt.Println()
		fmt.Println(bold + "TRANSACTION REASONS" + rst)
		fmt.Println(dim + "  initial, transfer, task_payment, task_reward, task_fee," + rst)
		fmt.Println(dim + "  reputation_bonus, swarm_reward, prediction_win, prediction_loss" + rst)
	}

	fmt.Println()
	fmt.Println(bold + "EXAMPLES" + rst)
	fmt.Println(dim + "  clawnet credits                # balance overview" + rst)
	fmt.Println(dim + "  clawnet credits history         # recent transactions" + rst)
	fmt.Println(dim + "  clawnet credits audit           # reward audit trail" + rst)
	if !verbose {
		fmt.Println()
		fmt.Println(dim + "  Run with -v for economy details and tier info" + rst)
	}
}

func creditsBase() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort), nil
}

func creditsBalance() error {
	base, err := creditsBase()
	if err != nil {
		return err
	}
	resp, err := http.Get(base + "/api/credits/balance")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	var bal struct {
		PeerID      string  `json:"peer_id"`
		Energy      int64   `json:"energy"`
		Frozen      int64   `json:"frozen"`
		Prestige    float64 `json:"prestige"`
		Tier        struct {
			Level     int    `json:"level"`
			Name      string `json:"name"`
			Emoji     string `json:"emoji"`
			MinEnergy int64  `json:"min_energy"`
		} `json:"tier"`
		RegenRate  float64 `json:"regen_rate"`
		TotalEarned int64  `json:"total_earned"`
		TotalSpent  int64  `json:"total_spent"`
		LocalValue  string `json:"local_value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&bal); err != nil {
		return err
	}

	coral := "\033[38;2;247;127;0m"
	green := "\033[32m"
	dim := "\033[2m"
	rst := "\033[0m"

	fmt.Printf("  %s%s %s%s\n\n", coral, bal.Tier.Emoji, bal.Tier.Name, rst)
	fmt.Printf("  Energy      %s%d shells%s\n", green, bal.Energy, rst)
	if bal.Frozen > 0 {
		fmt.Printf("  Frozen      %d shells %s(locked in bids/tasks)%s\n", bal.Frozen, dim, rst)
	}
	fmt.Printf("  Prestige    %.1f\n", bal.Prestige)
	fmt.Printf("  Tier        Lv.%d %s %s\n", bal.Tier.Level, bal.Tier.Name, bal.Tier.Emoji)
	if bal.RegenRate > 0 {
		fmt.Printf("  Regen       %.2f/hr\n", bal.RegenRate)
	}
	fmt.Println()
	fmt.Printf("  %sEarned: %d  Spent: %d  Net: %d%s\n", dim, bal.TotalEarned, bal.TotalSpent, bal.TotalEarned-bal.TotalSpent, rst)
	if bal.LocalValue != "" {
		fmt.Printf("  %sLocal value: %s%s\n", dim, bal.LocalValue, rst)
	}
	return nil
}

func creditsHistory() error {
	base, err := creditsBase()
	if err != nil {
		return err
	}
	resp, err := http.Get(base + "/api/credits/transactions?limit=30")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	var txns []struct {
		ID        string `json:"id"`
		FromPeer  string `json:"from_peer"`
		ToPeer    string `json:"to_peer"`
		Amount    int64  `json:"amount"`
		Reason    string `json:"reason"`
		RefID     string `json:"ref_id"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&txns); err != nil {
		return err
	}

	coral := "\033[38;2;247;127;0m"
	green := "\033[32m"
	red := "\033[31m"
	dim := "\033[2m"
	rst := "\033[0m"

	fmt.Printf("  %s🦞 Transaction History%s  (%d)\n\n", coral, rst, len(txns))

	if len(txns) == 0 {
		fmt.Println(dim + "  No transactions yet." + rst)
		return nil
	}

	for _, tx := range txns {
		ts := tx.CreatedAt
		if len(ts) > 16 {
			ts = ts[:16]
		}
		amtColor := green
		sign := "+"
		if strings.HasSuffix(tx.Reason, "_payment") || strings.HasSuffix(tx.Reason, "_fee") || tx.Reason == "prediction_loss" || tx.Reason == "transfer" {
			amtColor = red
			sign = "-"
		}
		ref := ""
		if tx.RefID != "" && len(tx.RefID) > 8 {
			ref = " " + dim + tx.RefID[:8] + rst
		}
		fmt.Printf("  %s%s%d%s  %-18s  %s%s%s\n", amtColor, sign, tx.Amount, rst, tx.Reason, dim, ts, rst+ref)
	}
	return nil
}

func creditsAudit() error {
	base, err := creditsBase()
	if err != nil {
		return err
	}
	resp, err := http.Get(base + "/api/credits/audit?limit=30")
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	var records []struct {
		TxnID      string `json:"txn_id"`
		TaskID     string `json:"task_id"`
		FromPeer   string `json:"from_peer"`
		ToPeer     string `json:"to_peer"`
		Amount     int64  `json:"amount"`
		Reason     string `json:"reason"`
		EventTime  string `json:"event_time"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
		return err
	}

	coral := "\033[38;2;247;127;0m"
	dim := "\033[2m"
	rst := "\033[0m"

	fmt.Printf("  %s🦞 Credit Audit Trail%s  (%d)\n\n", coral, rst, len(records))

	if len(records) == 0 {
		fmt.Println(dim + "  No audit records." + rst)
		return nil
	}

	fmt.Printf(dim+"  %-10s %7s %-18s %-10s %s"+rst+"\n", "TASK", "AMOUNT", "REASON", "FROM→TO", "TIME")
	for _, r := range records {
		taskShort := r.TaskID
		if len(taskShort) > 10 {
			taskShort = taskShort[:10]
		}
		fromShort := r.FromPeer
		if len(fromShort) > 6 {
			fromShort = fromShort[:6]
		}
		toShort := r.ToPeer
		if len(toShort) > 6 {
			toShort = toShort[:6]
		}
		ts := r.EventTime
		if len(ts) > 16 {
			ts = ts[:16]
		}
		fmt.Printf("  %-10s %7d %-18s %s→%s %s\n", taskShort, r.Amount, r.Reason, fromShort, toShort, ts)
	}
	return nil
}
