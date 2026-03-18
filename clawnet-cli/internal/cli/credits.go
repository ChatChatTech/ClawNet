package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/i18n"
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

	fmt.Println(bold + i18n.T("help.credits") + rst)
	fmt.Println()
	fmt.Println(bold + i18n.T("common.usage") + rst)
	fmt.Println(tidal + "  clawnet credits" + rst + dim + "             " + i18n.T("help.credits.balance") + rst)
	fmt.Println(tidal + "  clawnet credits <subcommand>" + rst)
	fmt.Println(tidal + "  clawnet credits --json" + rst + dim + "         " + i18n.T("help.credits.json") + rst)
	fmt.Println()
	fmt.Println(bold + i18n.T("help.credits.subcmds") + rst)
	fmt.Println(tidal+"  balance  "+dim+"(bal)    "+rst + i18n.T("help.credits.cmd_balance"))
	fmt.Println(tidal+"  history  "+dim+"(txns)   "+rst + i18n.T("help.credits.cmd_history"))
	fmt.Println(tidal+"  audit    "+dim+"         "+rst + i18n.T("help.credits.cmd_audit"))

	if verbose {
		fmt.Println()
		fmt.Println(bold + i18n.T("help.credits.economy") + rst)
		fmt.Println(dim + "  " + i18n.T("help.credits.economy_desc") + rst)
		fmt.Println(dim + "  " + i18n.T("help.credits.energy") + rst)
		fmt.Println(dim + "  " + i18n.T("help.credits.frozen") + rst)
		fmt.Println(dim + "  " + i18n.T("help.credits.prestige") + rst)
		fmt.Println(dim + "  " + i18n.T("help.credits.tier") + rst)
		fmt.Println()
		fmt.Println(bold + i18n.T("help.credits.txn_reasons") + rst)
		fmt.Println(dim + "  initial, transfer, task_payment, task_reward, task_fee," + rst)
		fmt.Println(dim + "  reputation_bonus, swarm_reward, prediction_win, prediction_loss" + rst)
	}

	fmt.Println()
	fmt.Println(bold + i18n.T("common.examples") + rst)
	fmt.Println(dim + "  clawnet credits                # balance overview" + rst)
	fmt.Println(dim + "  clawnet credits history         # recent transactions" + rst)
	fmt.Println(dim + "  clawnet credits audit           # reward audit trail" + rst)
	if !verbose {
		fmt.Println()
		fmt.Println(dim + "  " + i18n.T("help.credits.verbose_hint") + rst)
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

	if JSONOutput {
		body, _ := io.ReadAll(resp.Body)
		fmt.Println(string(body))
		return nil
	}

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
	fmt.Printf("  %-10s  %s%d shells%s\n", i18n.T("credits.field.energy"), green, bal.Energy, rst)
	if bal.Frozen > 0 {
		fmt.Printf("  %-10s  %d shells %s%s%s\n", i18n.T("credits.field.frozen"), bal.Frozen, dim, i18n.T("credits.field.frozen_note"), rst)
	}
	fmt.Printf("  %-10s  %.1f\n", i18n.T("credits.field.prestige"), bal.Prestige)
	fmt.Printf("  %-10s  Lv.%d %s %s\n", i18n.T("credits.field.tier"), bal.Tier.Level, bal.Tier.Name, bal.Tier.Emoji)
	if bal.RegenRate > 0 {
		fmt.Printf("  %-10s  %.2f/hr\n", i18n.T("credits.field.regen"), bal.RegenRate)
	}
	fmt.Println()
	fmt.Printf("  %s%s%s\n", dim, i18n.Tf("credits.earned_spent", bal.TotalEarned, bal.TotalSpent, bal.TotalEarned-bal.TotalSpent), rst)
	if bal.LocalValue != "" {
		fmt.Printf("  %s%s%s\n", dim, i18n.Tf("credits.local_value", bal.LocalValue), rst)
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

	if JSONOutput {
		body, _ := io.ReadAll(resp.Body)
		fmt.Println(string(body))
		return nil
	}

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

	fmt.Printf("  %s%s%s  (%d)\n\n", coral, i18n.T("credits.history_header"), rst, len(txns))

	if len(txns) == 0 {
		fmt.Println(dim + "  " + i18n.T("credits.no_transactions") + rst)
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

	if JSONOutput {
		body, _ := io.ReadAll(resp.Body)
		fmt.Println(string(body))
		return nil
	}

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

	fmt.Printf("  %s%s%s  (%d)\n\n", coral, i18n.T("credits.audit_header"), rst, len(records))

	if len(records) == 0 {
		fmt.Println(dim + "  " + i18n.T("credits.no_audit") + rst)
		return nil
	}

	fmt.Printf(dim+"  %-10s %7s %-18s %-10s %s"+rst+"\n", i18n.T("credits.audit_col.task"), i18n.T("credits.audit_col.amount"), i18n.T("credits.audit_col.reason"), i18n.T("credits.audit_col.fromto"), i18n.T("credits.audit_col.time"))
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
