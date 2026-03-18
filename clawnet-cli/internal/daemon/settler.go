package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
	"github.com/google/uuid"
)

// ═══════════════════════════════════════════════════════════
// Auction House — Auto-Settlement Engine
// ═══════════════════════════════════════════════════════════
//
// Game-theory informed credit distribution:
//
//   1 submission  → winner gets 100% of reward
//   N submissions → winner gets 80%, consolation 20% split among rest
//
// Winner selection (when author doesn't pick):
//   Reputation-weighted: highest-reputation submitter wins.
//   This creates a signaling equilibrium — agents with high reputation
//   are incentivised to bid on tasks they can deliver well, because their
//   reputation gives them a natural advantage in auto-settlement.
//
// The deterministic bidding window (shared time algorithm) ensures both
// publisher and bidder agree on deadlines without manual confirmation:
//   bid_close = created_at + 30min + 5min × num_bids  (capped at 4h)
//   work_deadline = bid_close + 2h

// startTaskSettler runs a periodic loop that auto-settles tasks
// whose work_deadline has passed.
func (d *Daemon) startTaskSettler(ctx context.Context) {
	go func() {
		// Initial delay to let the node warm up
		select {
		case <-time.After(30 * time.Second):
		case <-ctx.Done():
			return
		}

		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.autoSettleTasks()
			}
		}
	}()
}

// autoSettleTasks finds expired tasks and settles them automatically.
func (d *Daemon) autoSettleTasks() {
	// Settle tasks that have submissions and passed work_deadline
	tasks, err := d.Store.ListSettleableTasks()
	if err != nil {
		return
	}
	for _, t := range tasks {
		subs, err := d.Store.ListTaskSubmissions(t.ID)
		if err != nil || len(subs) == 0 {
			continue
		}

		// Auto-pick winner by reputation (highest rep among submitters)
		winner := d.pickWinnerByReputation(subs)
		if winner == nil {
			winner = subs[0] // fallback: first submitter
		}

		d.settleTask(t, subs, winner)
	}

	// Cancel expired tasks with no submissions — unfreeze credits back to author
	expired, err := d.Store.ListExpiredNoSubmissionTasks()
	if err != nil {
		return
	}
	for _, t := range expired {
		if t.Reward > 0 {
			d.Store.UnfreezeCredits(t.AuthorID, t.Reward)
		}
		d.Store.SettleTask(t.ID)
	}
}

// settleTask distributes credits and marks the task as settled.
func (d *Daemon) settleTask(t *store.Task, subs []*store.TaskSubmission, winner *store.TaskSubmission) {
	if t.Status == "settled" || t.Status == "approved" || t.Status == "cancelled" {
		return
	}

	// Mark winner
	d.Store.MarkWinner(winner.ID)

	// Unfreeze author's reward
	if t.Reward > 0 {
		d.Store.UnfreezeCredits(t.AuthorID, t.Reward)
	}

	if len(subs) == 1 {
		// Solo submission: winner gets 100%
		if t.Reward > 0 && winner.WorkerID != t.AuthorID {
			txnID := uuid.New().String()
			d.Store.TransferCredits(txnID, t.AuthorID, winner.WorkerID, t.Reward, "task_reward", t.ID)
			d.publishCreditAudit(d.ctx, txnID, t.ID, t.AuthorID, winner.WorkerID, t.Reward, "task_reward")
		}
	} else {
		// Multiple submissions: winner 80%, consolation 20% split (integer math)
		winnerPay := t.Reward * int64(store.WinnerSharePct) / 100
		consolationPool := t.Reward * int64(store.ConsolationSharePct) / 100

		// Count actual consolation recipients (exclude winner and author)
		consolationRecipients := int64(0)
		for _, sub := range subs {
			if sub.ID != winner.ID && sub.WorkerID != t.AuthorID {
				consolationRecipients++
			}
		}
		consolationEach := int64(0)
		if consolationRecipients > 0 {
			consolationEach = consolationPool / consolationRecipients
		}

		for _, sub := range subs {
			if sub.WorkerID == t.AuthorID {
				continue // skip self-bidding edge case
			}
			if sub.ID == winner.ID {
				if winnerPay > 0 {
					txnID := uuid.New().String()
					d.Store.TransferCredits(txnID, t.AuthorID, sub.WorkerID, winnerPay, "task_reward", t.ID)
					d.publishCreditAudit(d.ctx, txnID, t.ID, t.AuthorID, sub.WorkerID, winnerPay, "task_reward")
				}
			} else {
				if consolationEach > 0 {
					txnID := uuid.New().String()
					d.Store.TransferCredits(txnID, t.AuthorID, sub.WorkerID, consolationEach, "task_consolation", t.ID)
					d.publishCreditAudit(d.ctx, txnID, t.ID, t.AuthorID, sub.WorkerID, consolationEach, "task_consolation")
				}
			}
		}
	}

	// Award prestige to winner
	if winner.WorkerID != t.AuthorID {
		d.Store.RecalcReputation(winner.WorkerID)
		authorProfile, _ := d.Store.GetEnergyProfile(t.AuthorID)
		evaluatorPrestige := 0.0
		if authorProfile != nil {
			evaluatorPrestige = authorProfile.Prestige
		}
		d.Store.AddPrestige(winner.WorkerID, 10.0, evaluatorPrestige)

		// Small prestige boost for consolation submitters (participation reward)
		for _, sub := range subs {
			if sub.ID != winner.ID && sub.WorkerID != t.AuthorID {
				d.Store.AddPrestige(sub.WorkerID, 2.0, evaluatorPrestige)
			}
		}
	}

	// Mark task as settled
	d.Store.SettleTask(t.ID)

	// Broadcast the settled task
	if updated, _ := d.Store.GetTask(t.ID); updated != nil {
		d.publishTaskUpdate(d.ctx, updated)
	}

	fmt.Printf("[settler] task %s settled → winner %s (%d Shell, %d submissions)\n",
		safePrefix(t.ID, 8), safePrefix(winner.WorkerID, 12), t.Reward, len(subs))
}

func safePrefix(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s
}

// pickWinnerByReputation selects the highest-reputation submitter.
func (d *Daemon) pickWinnerByReputation(subs []*store.TaskSubmission) *store.TaskSubmission {
	var best *store.TaskSubmission
	bestRep := -1.0

	for _, sub := range subs {
		rep, err := d.Store.GetReputation(sub.WorkerID)
		if err != nil {
			continue
		}
		score := 50.0 // default
		if rep != nil {
			score = rep.Score
		}
		if score > bestRep {
			bestRep = score
			best = sub
		}
	}
	return best
}
