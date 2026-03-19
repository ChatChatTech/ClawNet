package daemon

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/discovery"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
	"github.com/google/uuid"
)

// RegisterPhase2Routes adds Phase 2 routes to the mux.
func (d *Daemon) RegisterPhase2Routes(mux *http.ServeMux) {
	// Credits
	mux.HandleFunc("GET /api/credits/balance", d.handleCreditsBalance)
	mux.HandleFunc("GET /api/credits/transactions", d.handleCreditsTransactions)

	// Task Bazaar
	mux.HandleFunc("POST /api/tasks", d.handleCreateTask)
	mux.HandleFunc("GET /api/tasks", d.handleListTasks)
	mux.HandleFunc("GET /api/tasks/board", d.handleTaskBoard)
	mux.HandleFunc("GET /api/tasks/{id}", d.handleGetTask)
	mux.HandleFunc("POST /api/tasks/{id}/bid", d.handleTaskBid)
	mux.HandleFunc("GET /api/tasks/{id}/bids", d.handleTaskBids)
	mux.HandleFunc("POST /api/tasks/{id}/assign", d.handleTaskAssign)
	mux.HandleFunc("POST /api/tasks/{id}/submit", d.handleTaskSubmit)
	mux.HandleFunc("POST /api/tasks/{id}/approve", d.handleTaskApprove)
	mux.HandleFunc("POST /api/tasks/{id}/reject", d.handleTaskReject)
	mux.HandleFunc("POST /api/tasks/{id}/cancel", d.handleTaskCancel)
	// Simple mode: worker claims and submits in one step
	mux.HandleFunc("POST /api/tasks/{id}/claim", d.handleTaskClaim)
	// Auction House: multi-worker submission & settlement
	mux.HandleFunc("POST /api/tasks/{id}/work", d.handleTaskSubmitWork)
	mux.HandleFunc("GET /api/tasks/{id}/submissions", d.handleTaskSubmissions)
	mux.HandleFunc("POST /api/tasks/{id}/pick", d.handleTaskPickWinner)

	// Task Bazaar — Nutshell bundle endpoints
	mux.HandleFunc("POST /api/tasks/{id}/bundle", d.handleUploadTaskBundle)
	mux.HandleFunc("GET /api/tasks/{id}/bundle", d.handleDownloadTaskBundle)

	// Swarm Think
	mux.HandleFunc("GET /api/swarm/templates", d.handleSwarmTemplates)
	mux.HandleFunc("POST /api/swarm", d.handleCreateSwarm)
	mux.HandleFunc("GET /api/swarm", d.handleListSwarms)
	mux.HandleFunc("GET /api/swarm/{id}", d.handleGetSwarm)
	mux.HandleFunc("POST /api/swarm/{id}/contribute", d.handleSwarmContribute)
	mux.HandleFunc("GET /api/swarm/{id}/contributions", d.handleSwarmContributions)
	mux.HandleFunc("POST /api/swarm/{id}/synthesize", d.handleSwarmSynthesize)

	// Reputation
	mux.HandleFunc("GET /api/reputation", d.handleListReputation)
	mux.HandleFunc("GET /api/reputation/{peer_id}", d.handleGetReputation)

	// Credit Audit
	mux.HandleFunc("GET /api/credits/audit", d.handleCreditAudit)

	// Prediction Market (Oracle Arena)
	mux.HandleFunc("POST /api/predictions", d.handleCreatePrediction)
	mux.HandleFunc("GET /api/predictions", d.handleListPredictions)
	mux.HandleFunc("GET /api/predictions/leaderboard", d.handlePredictionLeaderboard)
	mux.HandleFunc("GET /api/predictions/{id}", d.handleGetPrediction)
	mux.HandleFunc("POST /api/predictions/{id}/bet", d.handlePredictionBet)
	mux.HandleFunc("POST /api/predictions/{id}/resolve", d.handlePredictionResolve)
	mux.HandleFunc("POST /api/predictions/{id}/appeal", d.handlePredictionAppeal)
	mux.HandleFunc("GET /api/predictions/{id}/appeals", d.handleListPredictionAppeals)

	// Wealth Leaderboard (Social Energy Model)
	mux.HandleFunc("GET /api/leaderboard", d.handleWealthLeaderboard)

	// Agent Resumes & Matching
	mux.HandleFunc("PUT /api/resume", d.handleUpdateResume)
	mux.HandleFunc("GET /api/resume", d.handleGetOwnResume)
	mux.HandleFunc("GET /api/resumes", d.handleListResumes)
	mux.HandleFunc("GET /api/resume/{peer_id}", d.handleGetPeerResume)
	mux.HandleFunc("GET /api/tasks/{id}/match", d.handleMatchAgentsForTask)
	mux.HandleFunc("GET /api/match/tasks", d.handleMatchTasksForAgent)
	mux.HandleFunc("GET /api/discover", d.handleDiscover)

	// Nutshell E2E integration
	mux.HandleFunc("POST /api/nutshell/publish", d.handleNutshellPublish)
	mux.HandleFunc("POST /api/tasks/{id}/deliver", d.handleNutshellDeliver)

	// Tutorial (built-in onboarding task)
	mux.HandleFunc("POST /api/tutorial/complete", d.handleTutorialComplete)
	mux.HandleFunc("GET /api/tutorial/status", d.handleTutorialStatus)

	// Topo coin system (daemon-internal state machine, no nonces exposed)
	mux.HandleFunc("GET /api/topo/coin-state", d.handleCoinState)
	mux.HandleFunc("POST /api/topo/coin-grab", d.handleCoinGrab)
	mux.HandleFunc("POST /api/topo/coin-redeem", d.handleCoinRedeemV2)
}

// ── Credits handlers ──

func (d *Daemon) handleCreditsBalance(w http.ResponseWriter, r *http.Request) {
	peerID := d.Node.PeerID().String()
	profile, err := d.Store.GetEnergyProfile(peerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := map[string]any{
		"peer_id":      profile.PeerID,
		"balance":      profile.Energy,
		"energy":       profile.Energy,
		"frozen":       profile.Frozen,
		"prestige":     profile.Prestige,
		"tier":         profile.Tier,
		"regen_rate":   profile.RegenRate,
		"total_earned": profile.TotalEarned,
		"total_spent":  profile.TotalSpent,
		"updated_at":   profile.UpdatedAt,
	}
	// Add local currency exchange info based on node geo location
	if c := d.selfCurrency(); c != nil {
		resp["currency"] = c.Code
		resp["currency_symbol"] = c.Symbol
		resp["exchange_rate"] = c.Rate
		resp["local_value"] = c.Format(profile.Energy)
	}
	writeJSON(w, resp)
}

func (d *Daemon) handleCreditsTransactions(w http.ResponseWriter, r *http.Request) {
	peerID := d.Node.PeerID().String()
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	txns, err := d.Store.ListCreditTransactions(peerID, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if txns == nil {
		txns = []*store.CreditTransaction{}
	}
	writeJSON(w, txns)
}

// ── Task handlers ──

func (d *Daemon) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	// Accept tags as either a JSON array or a pre-encoded string
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var t store.Task
	if v, ok := raw["title"]; ok {
		json.Unmarshal(v, &t.Title)
	}
	if v, ok := raw["description"]; ok {
		json.Unmarshal(v, &t.Description)
	}
	if v, ok := raw["reward"]; ok {
		json.Unmarshal(v, &t.Reward)
	}
	if v, ok := raw["deadline"]; ok {
		json.Unmarshal(v, &t.Deadline)
	}
	// Tags: accept ["a","b"] array or "a,b" string
	if v, ok := raw["tags"]; ok {
		var arr []string
		if json.Unmarshal(v, &arr) == nil {
			encoded, _ := json.Marshal(arr)
			t.Tags = string(encoded)
		} else {
			var s string
			json.Unmarshal(v, &s)
			t.Tags = s
		}
	}
	// Nutshell integration fields (optional)
	if v, ok := raw["nutshell_hash"]; ok {
		json.Unmarshal(v, &t.NutshellHash)
	}
	if v, ok := raw["nutshell_id"]; ok {
		json.Unmarshal(v, &t.NutshellID)
	}
	if v, ok := raw["bundle_type"]; ok {
		json.Unmarshal(v, &t.BundleType)
	}
	// Targeted task (optional — only specific peer can accept)
	if v, ok := raw["target_peer"]; ok {
		json.Unmarshal(v, &t.TargetPeer)
	}
	// Task mode: "simple" (default, first-come-first-served) or "auction" (bid/assign flow)
	if v, ok := raw["mode"]; ok {
		json.Unmarshal(v, &t.Mode)
	}
	if t.Mode != "auction" {
		t.Mode = "simple"
	}

	if t.Title == "" {
		apiError(w, http.StatusBadRequest, "title_required",
			withMessage("Task title is required."),
			withSuggestion("Provide a clear title describing what the agent should do."))
		return
	}
	if t.Reward < 0 {
		apiError(w, http.StatusBadRequest, "invalid_reward",
			withMessage("Reward cannot be negative."))
		return
	}

	fromPeer := d.Node.PeerID().String()

	// Zero-reward tasks: no fee, published as "help wanted"
	if t.Reward > 0 {
		if t.Reward < 100 {
			apiError(w, http.StatusBadRequest, "reward_too_low",
				withMessage("Minimum reward is 100 Shell (or 0 for help-wanted tasks)."),
				withSuggestion("Set reward >= 100 to attract workers, or 0 for free collaboration."),
				withHelp("GET /api/credits/balance"))
			return
		}

		// Deduct 5% task fee (deflationary burn) before publishing
		fee := t.Reward * 5 / 100
		feeTxnID := uuid.New().String()
		if err := d.Store.TransferCredits(feeTxnID, fromPeer, "system_burn", fee, "task_fee", ""); err != nil {
			if err == store.ErrInsufficientCredits {
				profile, _ := d.Store.GetEnergyProfile(fromPeer)
				bal := int64(0)
				if profile != nil {
					bal = profile.Energy
				}
				apiError(w, http.StatusBadRequest, "insufficient_balance",
					withMessage(fmt.Sprintf("Need %d Shell (reward %d + 5%% fee %d) but balance is %d.", t.Reward+fee, t.Reward, fee, bal)),
					withBalance(bal, t.Reward+fee),
					withSuggestion("Claim and complete a task to earn Shell, or publish a 0-reward help-wanted task."),
					withHelp("GET /api/tasks?status=open"))
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err := d.publishTask(d.ctx, &t); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// P1: Check milestone + record event
	milestoneReward := d.CheckAndCompleteMilestone("first_task_publish")
	d.RecordEvent("task_published", fromPeer, t.ID, fmt.Sprintf("%s published '%s' (%d Shell)", d.Profile.AgentName, t.Title, t.Reward))
	d.BroadcastEcho(r.Context(), "task_published", t.Title, fmt.Sprintf("%s published '%s' (%d Shell)", d.Profile.AgentName, t.Title, t.Reward))

	resp := map[string]any{"task": t}
	if milestoneReward > 0 {
		resp["milestone_completed"] = "first_task_publish"
		resp["milestone_reward"] = milestoneReward
	}
	writeJSON(w, resp)
}

func (d *Daemon) handleListTasks(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	tasks, err := d.Store.ListTasks(status, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if tasks == nil {
		tasks = []*store.Task{}
	}
	writeJSON(w, tasks)
}

func (d *Daemon) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := d.Store.GetTask(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if t == nil {
		http.Error(w, `{"error":"task not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, t)
}

func (d *Daemon) handleTaskBoard(w http.ResponseWriter, r *http.Request) {
	peerID := d.Node.PeerID().String()
	published, assigned, open, err := d.Store.TaskBoard(peerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if published == nil {
		published = []*store.BoardTask{}
	}
	if assigned == nil {
		assigned = []*store.BoardTask{}
	}
	if open == nil {
		open = []*store.BoardTask{}
	}
	writeJSON(w, map[string]any{
		"my_published": published,
		"my_assigned":  assigned,
		"open_tasks":   open,
	})
}

func (d *Daemon) handleTaskBid(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")

	// Prevent self-bidding: cannot bid on your own task
	if task, _ := d.Store.GetTask(taskID); task != nil && task.AuthorID == d.Node.PeerID().String() {
		http.Error(w, `{"error":"cannot bid on your own task"}`, http.StatusForbidden)
		return
	}

	// Enforce targeting: if task has a target_peer, only that peer can bid
	if task, _ := d.Store.GetTask(taskID); task != nil && task.TargetPeer != "" && task.TargetPeer != d.Node.PeerID().String() {
		http.Error(w, `{"error":"this task is targeted to another peer"}`, http.StatusForbidden)
		return
	}

	var body struct {
		Amount  int64  `json:"amount"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	bid := &store.TaskBid{
		TaskID:  taskID,
		Amount:  body.Amount,
		Message: body.Message,
	}
	if err := d.publishTaskBid(d.ctx, bid); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, bid)
}

func (d *Daemon) handleTaskBids(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	bids, err := d.Store.ListTaskBids(taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if bids == nil {
		bids = []*store.TaskBid{}
	}
	writeJSON(w, bids)
}

func (d *Daemon) handleTaskAssign(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	var body struct {
		AssignTo string `json:"assign_to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.AssignTo == "" {
		http.Error(w, `{"error":"assign_to is required"}`, http.StatusBadRequest)
		return
	}

	// Prevent self-assignment: cannot assign your own task to yourself
	if task, _ := d.Store.GetTask(taskID); task != nil && task.AuthorID == body.AssignTo {
		http.Error(w, `{"error":"cannot assign task to its author"}`, http.StatusForbidden)
		return
	}

	if err := d.Store.AssignTask(taskID, body.AssignTo); err != nil {
		if errors.Is(err, store.ErrTaskStateConflict) {
			http.Error(w, `{"error":"task is not open — cannot assign"}`, http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	t, _ := d.Store.GetTask(taskID)
	if t != nil {
		d.publishTaskUpdate(d.ctx, t)
	}
	// I-6: Update active task count for newly assigned worker
	d.Store.RecalcActiveTasks(body.AssignTo)
	writeJSON(w, map[string]string{"status": "assigned"})
}

func (d *Daemon) handleTaskSubmit(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	var body struct {
		Result string `json:"result"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := d.Store.SubmitTask(taskID, body.Result); err != nil {
		if errors.Is(err, store.ErrTaskStateConflict) {
			http.Error(w, `{"error":"task is not assigned — cannot submit"}`, http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	t, _ := d.Store.GetTask(taskID)
	if t != nil {
		d.publishTaskUpdate(d.ctx, t)
	}
	writeJSON(w, map[string]string{"status": "submitted"})
}

func (d *Daemon) handleTaskApprove(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")

	// Get task to know reward info
	t, err := d.Store.GetTask(taskID)
	if err != nil || t == nil {
		http.Error(w, `{"error":"task not found"}`, http.StatusNotFound)
		return
	}
	if err := d.Store.ApproveTask(taskID); err != nil {
		if errors.Is(err, store.ErrTaskStateConflict) {
			http.Error(w, `{"error":"task is not submitted — cannot approve"}`, http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Prevent self-approve: if somehow author == assignee, just unfreeze credits (no transfer/prestige)
	var oldPct int
	if t.AssignedTo != "" && t.AssignedTo != t.AuthorID {
		oldPct, _ = d.Store.ReputationPercentile(t.AssignedTo)
	}
	if t.AssignedTo == t.AuthorID {
		if t.Reward > 0 {
			d.Store.UnfreezeCredits(t.AuthorID, t.Reward)
		}
	} else {
		// Pay the assignee: unfreeze and transfer
		if t.Reward > 0 && t.AssignedTo != "" {
			d.Store.UnfreezeCredits(t.AuthorID, t.Reward)
			txnID := uuid.New().String()
			d.Store.TransferCredits(txnID, t.AuthorID, t.AssignedTo, t.Reward, "task_reward", taskID)
			// Broadcast credit audit for peer supervision
			d.publishCreditAudit(d.ctx, txnID, taskID, t.AuthorID, t.AssignedTo, t.Reward, "task_reward")
		}

		// Recalc reputation for assignee + award prestige
		if t.AssignedTo != "" {
			d.Store.RecalcReputation(t.AssignedTo)
			// Award prestige: task completion gives +10 prestige, weighted by author's prestige
			authorProfile, _ := d.Store.GetEnergyProfile(t.AuthorID)
			evaluatorPrestige := 0.0
			if authorProfile != nil {
				evaluatorPrestige = authorProfile.Prestige
			}
			d.Store.AddPrestige(t.AssignedTo, 10.0, evaluatorPrestige)
		}
	}

	t, _ = d.Store.GetTask(taskID)
	if t != nil {
		d.publishTaskUpdate(d.ctx, t)
	}

	// P0: Enhanced settlement receipt
	receipt := map[string]any{
		"status":      "approved",
		"task_id":     taskID,
		"reward_paid": t.Reward,
	}
	if t != nil && t.Reward > 0 {
		fee := t.Reward * 5 / 100
		receipt["fee_burned"] = fee
		// Get updated balances
		if workerProfile, err := d.Store.GetEnergyProfile(t.AssignedTo); err == nil && workerProfile != nil {
			receipt["worker_new_balance"] = workerProfile.Energy
			receipt["total_earned"] = workerProfile.TotalEarned
		}
		if authorProfile, err := d.Store.GetEnergyProfile(t.AuthorID); err == nil && authorProfile != nil {
			receipt["publisher_new_balance"] = authorProfile.Energy
		}
		// Reputation percentile + rank change for worker
		if t.AssignedTo != "" {
			if rep, err := d.Store.GetReputation(t.AssignedTo); err == nil && rep != nil {
				receipt["worker_reputation"] = rep.Score
				if pct, err := d.Store.ReputationPercentile(t.AssignedTo); err == nil {
					receipt["percentile"] = pct
					receipt["rank_change"] = pct - oldPct
				}
			}
		}
	}

	// Record event
	if t != nil {
		d.RecordEvent("task_approved", d.Node.PeerID().String(), taskID,
			fmt.Sprintf("Task '%s' approved — %d Shell paid to worker", t.Title, t.Reward))
		d.BroadcastEcho(r.Context(), "task_approved", t.Title, fmt.Sprintf("Task '%s' approved — %d Shell paid", t.Title, t.Reward))

		// I-6: Auto-update worker resume — merge task tags + recalc active load
		if t.AssignedTo != "" {
			d.Store.AutoUpdateResumeSkills(t.AssignedTo, t.Tags)
			d.Store.RecalcActiveTasks(t.AssignedTo)
		}
	}

	writeJSON(w, receipt)
}

func (d *Daemon) handleTaskReject(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")

	t, err := d.Store.GetTask(taskID)
	if err != nil || t == nil {
		http.Error(w, `{"error":"task not found"}`, http.StatusNotFound)
		return
	}
	if err := d.Store.RejectTask(taskID); err != nil {
		if errors.Is(err, store.ErrTaskStateConflict) {
			http.Error(w, `{"error":"task is not submitted — cannot reject"}`, http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Unfreeze credits back to author
	if t.Reward > 0 {
		d.Store.UnfreezeCredits(t.AuthorID, t.Reward)
	}

	// Recalc reputation for assignee
	if t.AssignedTo != "" {
		d.Store.RecalcReputation(t.AssignedTo)
		d.Store.RecalcActiveTasks(t.AssignedTo)
	}

	t, _ = d.Store.GetTask(taskID)
	if t != nil {
		d.publishTaskUpdate(d.ctx, t)
	}
	writeJSON(w, map[string]string{"status": "rejected"})
}

func (d *Daemon) handleTaskCancel(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")

	t, err := d.Store.GetTask(taskID)
	if err != nil || t == nil {
		http.Error(w, `{"error":"task not found"}`, http.StatusNotFound)
		return
	}
	// Only the task author can cancel
	if t.AuthorID != d.Node.PeerID().String() {
		http.Error(w, `{"error":"only the task author can cancel"}`, http.StatusForbidden)
		return
	}
	if err := d.Store.CancelTask(taskID); err != nil {
		if errors.Is(err, store.ErrTaskStateConflict) {
			http.Error(w, `{"error":"task is not open/assigned — cannot cancel"}`, http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Unfreeze reward back to author
	if t.Reward > 0 {
		d.Store.UnfreezeCredits(t.AuthorID, t.Reward)
	}

	t, _ = d.Store.GetTask(taskID)
	if t != nil {
		d.publishTaskUpdate(d.ctx, t)
	}
	writeJSON(w, map[string]string{"status": "cancelled"})
}

// handleTaskClaim lets a worker claim a simple-mode task in one step:
// sets assigned_to + result + self_eval_score, transitions open → submitted.
// The author's node will auto-approve via gossip when it sees the submission.
func (d *Daemon) handleTaskClaim(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	t, err := d.Store.GetTask(taskID)
	if err != nil || t == nil {
		http.Error(w, `{"error":"task not found"}`, http.StatusNotFound)
		return
	}
	if t.Mode != "simple" {
		http.Error(w, `{"error":"task is not in simple mode — use bid/assign flow"}`, http.StatusBadRequest)
		return
	}
	if t.Status != "open" {
		http.Error(w, `{"error":"task is not open"}`, http.StatusConflict)
		return
	}
	peerID := d.Node.PeerID().String()
	if t.AuthorID == peerID {
		http.Error(w, `{"error":"cannot claim your own task"}`, http.StatusForbidden)
		return
	}
	if t.TargetPeer != "" && t.TargetPeer != peerID {
		http.Error(w, `{"error":"this task is targeted to another peer"}`, http.StatusForbidden)
		return
	}

	var body struct {
		Result        string  `json:"result"`
		SelfEvalScore float64 `json:"self_eval_score"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Result == "" {
		http.Error(w, `{"error":"result is required"}`, http.StatusBadRequest)
		return
	}
	if body.SelfEvalScore < 0.6 {
		http.Error(w, `{"error":"self_eval_score must be >= 0.6"}`, http.StatusBadRequest)
		return
	}

	if err := d.Store.ClaimTask(taskID, peerID, body.Result, body.SelfEvalScore); err != nil {
		if errors.Is(err, store.ErrTaskStateConflict) {
			http.Error(w, `{"error":"task already claimed or not in simple mode"}`, http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Publish the updated task via gossip so the author's node can auto-approve
	t, _ = d.Store.GetTask(taskID)
	if t != nil {
		d.publishTaskUpdate(d.ctx, t)
	}

	// P1: Check milestone + record event
	milestoneReward := d.CheckAndCompleteMilestone("first_task_claim")
	d.RecordEvent("task_claimed", peerID, taskID, fmt.Sprintf("%s claimed '%s'", d.Profile.AgentName, t.Title))
	d.BroadcastEcho(r.Context(), "task_claimed", t.Title, fmt.Sprintf("%s claimed '%s'", d.Profile.AgentName, t.Title))

	// I-6: Update active task count for claimer
	d.Store.RecalcActiveTasks(peerID)

	resp := map[string]any{"status": "submitted", "task_id": taskID}
	if milestoneReward > 0 {
		resp["milestone_completed"] = "first_task_claim"
		resp["milestone_reward"] = milestoneReward
	}
	writeJSON(w, resp)
}

// ── Auction House handlers (multi-worker submit / pick winner) ──

// handleTaskSubmitWork allows any bidder to submit work for a task (multi-worker parallel execution).
// In the Auction House model, bid = start working. Workers submit results here.
func (d *Daemon) handleTaskSubmitWork(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")

	t, err := d.Store.GetTask(taskID)
	if err != nil || t == nil {
		http.Error(w, `{"error":"task not found"}`, http.StatusNotFound)
		return
	}
	if t.Status != "open" && t.Status != "assigned" {
		http.Error(w, `{"error":"task is not accepting submissions"}`, http.StatusConflict)
		return
	}

	peerID := d.Node.PeerID().String()

	// Must have bid on this task to submit work
	bids, _ := d.Store.ListTaskBids(taskID)
	hasBid := false
	for _, b := range bids {
		if b.BidderID == peerID {
			hasBid = true
			break
		}
	}
	if !hasBid {
		http.Error(w, `{"error":"must bid on task before submitting work"}`, http.StatusForbidden)
		return
	}

	// Prevent duplicate submissions from same worker
	subs, _ := d.Store.ListTaskSubmissions(taskID)
	for _, s := range subs {
		if s.WorkerID == peerID {
			http.Error(w, `{"error":"already submitted work for this task"}`, http.StatusConflict)
			return
		}
	}

	var body struct {
		Result string `json:"result"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Result == "" {
		http.Error(w, `{"error":"result is required"}`, http.StatusBadRequest)
		return
	}

	sub := &store.TaskSubmission{
		TaskID: taskID,
		Result: body.Result,
	}
	if err := d.publishTaskSubmission(d.ctx, sub); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, sub)
}

// handleTaskSubmissions returns all submissions for a task.
func (d *Daemon) handleTaskSubmissions(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	subs, err := d.Store.ListTaskSubmissions(taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if subs == nil {
		subs = []*store.TaskSubmission{}
	}
	writeJSON(w, subs)
}

// handleTaskPickWinner lets the task author manually pick a winner from submissions.
// If the author doesn't pick within the grace period, auto-settlement picks by reputation.
func (d *Daemon) handleTaskPickWinner(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")

	t, err := d.Store.GetTask(taskID)
	if err != nil || t == nil {
		http.Error(w, `{"error":"task not found"}`, http.StatusNotFound)
		return
	}

	// Only task author can pick
	if t.AuthorID != d.Node.PeerID().String() {
		http.Error(w, `{"error":"only the task author can pick a winner"}`, http.StatusForbidden)
		return
	}
	if t.Status != "open" && t.Status != "assigned" && t.Status != "submitted" {
		http.Error(w, `{"error":"task already settled or cancelled"}`, http.StatusConflict)
		return
	}

	var body struct {
		SubmissionID string `json:"submission_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.SubmissionID == "" {
		http.Error(w, `{"error":"submission_id is required"}`, http.StatusBadRequest)
		return
	}

	subs, err := d.Store.ListTaskSubmissions(taskID)
	if err != nil || len(subs) == 0 {
		http.Error(w, `{"error":"no submissions to pick from"}`, http.StatusBadRequest)
		return
	}

	// Verify submission exists and belongs to this task
	var winnerSub *store.TaskSubmission
	for _, s := range subs {
		if s.ID == body.SubmissionID {
			winnerSub = s
			break
		}
	}
	if winnerSub == nil {
		http.Error(w, `{"error":"submission not found for this task"}`, http.StatusNotFound)
		return
	}

	// Settle: mark winner, distribute credits
	d.settleTask(t, subs, winnerSub)

	writeJSON(w, map[string]string{"status": "settled", "winner": winnerSub.WorkerID})
}

// ── Swarm handlers ──

func (d *Daemon) handleSwarmTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, store.SwarmTemplates)
}

func (d *Daemon) handleCreateSwarm(w http.ResponseWriter, r *http.Request) {
	var sw store.Swarm
	if err := json.NewDecoder(r.Body).Decode(&sw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if sw.Title == "" || sw.Question == "" {
		http.Error(w, `{"error":"title and question are required"}`, http.StatusBadRequest)
		return
	}
	// Apply template defaults when template_type is set
	if sw.TemplateType != "" && sw.TemplateType != "freeform" {
		tmpl := store.GetSwarmTemplate(sw.TemplateType)
		if tmpl == nil {
			http.Error(w, `{"error":"unknown template_type"}`, http.StatusBadRequest)
			return
		}
		if sw.Domains == "" || sw.Domains == "[]" {
			b, _ := json.Marshal(tmpl.DefaultDomains)
			sw.Domains = string(b)
		}
		if sw.DurationMin == 0 && tmpl.DefaultDuration > 0 {
			sw.DurationMin = tmpl.DefaultDuration
		}
	}
	if sw.TemplateType == "" {
		sw.TemplateType = "freeform"
	}
	if err := d.publishSwarm(d.ctx, &sw); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, sw)
}

func (d *Daemon) handleListSwarms(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	q := r.URL.Query().Get("q")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	var swarms []*store.Swarm
	var err error
	if q != "" {
		swarms, err = d.Store.SearchSwarms(q, limit, offset)
	} else {
		swarms, err = d.Store.ListSwarms(status, limit, offset)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if swarms == nil {
		swarms = []*store.Swarm{}
	}
	writeJSON(w, swarms)
}

func (d *Daemon) handleGetSwarm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sw, err := d.Store.GetSwarm(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if sw == nil {
		http.Error(w, `{"error":"swarm not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, sw)
}

func (d *Daemon) handleSwarmContribute(w http.ResponseWriter, r *http.Request) {
	swarmID := r.PathValue("id")
	var body struct {
		Body        string  `json:"body"`
		Section     string  `json:"section"`     // template section key
		Perspective string  `json:"perspective"` // bull, bear, neutral, devil-advocate
		Confidence  float64 `json:"confidence"`  // 0.0 - 1.0
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Body == "" {
		http.Error(w, `{"error":"body is required"}`, http.StatusBadRequest)
		return
	}
	c := &store.SwarmContribution{SwarmID: swarmID, Body: body.Body,
		Section: body.Section, Perspective: body.Perspective, Confidence: body.Confidence}
	if err := d.publishSwarmContribution(d.ctx, c); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Milestone: first swarm contribution
	reward := d.CheckAndCompleteMilestone("first_swarm")
	d.RecordEvent("swarm_contribution", d.Node.PeerID().String(), swarmID, body.Body)

	resp := map[string]any{"contribution": c}
	if reward > 0 {
		resp["milestone_completed"] = "first_swarm"
		resp["milestone_reward"] = reward
	}
	writeJSON(w, resp)
}

func (d *Daemon) handleSwarmContributions(w http.ResponseWriter, r *http.Request) {
	swarmID := r.PathValue("id")
	limit := queryInt(r, "limit", 100)
	contribs, err := d.Store.ListSwarmContributions(swarmID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if contribs == nil {
		contribs = []*store.SwarmContribution{}
	}
	writeJSON(w, contribs)
}

func (d *Daemon) handleSwarmSynthesize(w http.ResponseWriter, r *http.Request) {
	swarmID := r.PathValue("id")
	var body struct {
		Synthesis string `json:"synthesis"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Synthesis == "" {
		http.Error(w, `{"error":"synthesis is required"}`, http.StatusBadRequest)
		return
	}
	if err := d.publishSwarmSynthesis(d.ctx, swarmID, body.Synthesis); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Recalc reputation for all contributors + award prestige
	contribs, _ := d.Store.ListSwarmContributions(swarmID, 1000)
	seen := map[string]bool{}
	synthesizerProfile, _ := d.Store.GetEnergyProfile(d.Node.PeerID().String())
	synthPrestige := 0.0
	if synthesizerProfile != nil {
		synthPrestige = synthesizerProfile.Prestige
	}
	for _, c := range contribs {
		if !seen[c.AuthorID] {
			d.Store.RecalcReputation(c.AuthorID)
			// +5 prestige per swarm participation, weighted by synthesizer's prestige
			d.Store.AddPrestige(c.AuthorID, 5.0, synthPrestige)
			seen[c.AuthorID] = true
		}
	}

	writeJSON(w, map[string]string{"status": "synthesized"})
}

// ── Reputation handlers ──

func (d *Daemon) handleListReputation(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50)
	recs, err := d.Store.ListReputation(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if recs == nil {
		recs = []*store.ReputationRecord{}
	}
	writeJSON(w, recs)
}

func (d *Daemon) handleGetReputation(w http.ResponseWriter, r *http.Request) {
	peerID := r.PathValue("peer_id")
	rec, err := d.Store.RecalcReputation(peerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, rec)
}

func (d *Daemon) handleCreditAudit(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	records, err := d.Store.ListCreditAudit(limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, records)
}

// ── Prediction Market handlers ──

func (d *Daemon) handleCreatePrediction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Question         string   `json:"question"`
		Options          []string `json:"options"`
		Category         string   `json:"category"`
		ResolutionDate   string   `json:"resolution_date"`
		ResolutionSource string   `json:"resolution_source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Question == "" || len(body.Options) < 2 {
		http.Error(w, `{"error":"question and at least 2 options required"}`, http.StatusBadRequest)
		return
	}
	if body.ResolutionDate == "" {
		http.Error(w, `{"error":"resolution_date required"}`, http.StatusBadRequest)
		return
	}
	if body.Category == "" {
		body.Category = "custom"
	}

	optJSON, _ := json.Marshal(body.Options)
	p := &store.Prediction{
		Question:         body.Question,
		Options:          string(optJSON),
		Category:         body.Category,
		ResolutionDate:   body.ResolutionDate,
		ResolutionSource: body.ResolutionSource,
		Status:           "open",
	}
	if err := d.publishPrediction(d.ctx, p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, p)
}

func (d *Daemon) handleListPredictions(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	category := r.URL.Query().Get("category")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	preds, err := d.Store.ListPredictions(status, category, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if preds == nil {
		preds = []*store.Prediction{}
	}
	writeJSON(w, preds)
}

func (d *Daemon) handleGetPrediction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, details, err := d.Store.GetPredictionDetails(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if p == nil {
		http.Error(w, `{"error":"prediction not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{
		"prediction": p,
		"options":    details,
	})
}

func (d *Daemon) handlePredictionBet(w http.ResponseWriter, r *http.Request) {
	predID := r.PathValue("id")
	var body struct {
		Option    string `json:"option"`
		Stake     int64  `json:"stake"`
		Reasoning string `json:"reasoning"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Option == "" || body.Stake <= 0 {
		http.Error(w, `{"error":"option and positive stake required"}`, http.StatusBadRequest)
		return
	}

	// Validate prediction exists and is open
	p, err := d.Store.GetPrediction(predID)
	if err != nil || p == nil {
		http.Error(w, `{"error":"prediction not found"}`, http.StatusNotFound)
		return
	}
	if p.Status != "open" {
		http.Error(w, `{"error":"prediction is not open for betting"}`, http.StatusBadRequest)
		return
	}

	// Validate option
	if err := store.ValidatePredictionOption(p.Options, body.Option); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	// Freeze bettor's credits
	peerID := d.Node.PeerID().String()
	if err := d.Store.FreezeCredits(peerID, body.Stake); err != nil {
		if err == store.ErrInsufficientCredits {
			http.Error(w, `{"error":"insufficient credits"}`, http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	bet := &store.PredictionBet{
		PredictionID: predID,
		Option:       body.Option,
		Stake:        body.Stake,
		Reasoning:    body.Reasoning,
	}
	if err := d.publishPredictionBet(d.ctx, bet); err != nil {
		// Unfreeze on failure
		d.Store.UnfreezeCredits(peerID, body.Stake)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, bet)
}

func (d *Daemon) handlePredictionResolve(w http.ResponseWriter, r *http.Request) {
	predID := r.PathValue("id")
	var body struct {
		Result      string `json:"result"`
		EvidenceURL string `json:"evidence_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Result == "" {
		http.Error(w, `{"error":"result required"}`, http.StatusBadRequest)
		return
	}

	p, err := d.Store.GetPrediction(predID)
	if err != nil || p == nil {
		http.Error(w, `{"error":"prediction not found"}`, http.StatusNotFound)
		return
	}
	if p.Status != "open" {
		http.Error(w, `{"error":"prediction already resolved"}`, http.StatusBadRequest)
		return
	}
	if err := store.ValidatePredictionOption(p.Options, body.Result); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	// Record this resolution proposal
	res := &store.PredictionResolution{
		PredictionID: predID,
		Result:       body.Result,
		EvidenceURL:  body.EvidenceURL,
	}
	if err := d.publishPredictionResolution(d.ctx, res); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if ≥3 unique resolvers agree on this result
	count, _ := d.Store.CountResolutions(predID, body.Result)
	if count >= 3 {
		// Consensus reached — enter appeal period (24h) before final settlement
		deadline := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
		if err := d.Store.SetPendingWithAppeal(predID, body.Result, deadline); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"status": "pending", "result": body.Result, "consensus": count, "appeal_deadline": deadline})
		return
	}

	writeJSON(w, map[string]any{"status": "pending", "result": body.Result, "votes": count, "needed": 3})
}

func (d *Daemon) handlePredictionLeaderboard(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50)
	entries, err := d.Store.GetPredictionLeaderboard(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []*store.PredictionLeaderEntry{}
	}
	writeJSON(w, entries)
}

// settlePrediction distributes winnings after a prediction is resolved.
func (d *Daemon) settlePrediction(predID, result string) {
	settlements, err := d.Store.SettlePrediction(predID, result)
	if err != nil {
		return
	}

	bets, _ := d.Store.ListPredictionBets(predID)

	// Unfreeze all bets and deduct from losers, pay winners
	for _, b := range bets {
		d.Store.UnfreezeCredits(b.BettorID, b.Stake)
	}

	// Deduct from losers
	for _, b := range bets {
		if b.Option != result {
			txnID := uuid.New().String()
			d.Store.TransferCredits(txnID, b.BettorID, "prediction_pool", b.Stake, "prediction_loss", predID)
			d.publishCreditAudit(d.ctx, txnID, predID, b.BettorID, "prediction_pool", b.Stake, "prediction_loss")
		}
	}

	// Pay winners their proportional share
	for _, s := range settlements {
		if s.Profit > 0 {
			txnID := uuid.New().String()
			d.Store.AddCredits(txnID, s.PeerID, s.Profit, "prediction_win")
			d.publishCreditAudit(d.ctx, txnID, predID, "prediction_pool", s.PeerID, s.Profit, "prediction_win")
		}
	}
}

// handlePredictionAppeal allows bettors to challenge a pending prediction result.
func (d *Daemon) handlePredictionAppeal(w http.ResponseWriter, r *http.Request) {
	predID := r.PathValue("id")
	var body struct {
		Reason      string `json:"reason"`
		EvidenceURL string `json:"evidence_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Reason == "" {
		http.Error(w, `{"error":"reason required"}`, http.StatusBadRequest)
		return
	}

	p, err := d.Store.GetPrediction(predID)
	if err != nil || p == nil {
		http.Error(w, `{"error":"prediction not found"}`, http.StatusNotFound)
		return
	}
	if p.Status != "pending" {
		http.Error(w, `{"error":"prediction is not in appeal period"}`, http.StatusBadRequest)
		return
	}

	appeal := &store.PredictionAppeal{
		PredictionID: predID,
		AppellantID:  d.Node.PeerID().String(),
		Reason:       body.Reason,
		EvidenceURL:  body.EvidenceURL,
	}
	if err := d.publishPredictionAppeal(d.ctx, appeal); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if enough appeals to overturn
	count, _ := d.Store.CountAppeals(predID)
	if count >= 2 {
		// Overturn: revert to open, clear resolutions and appeals
		if err := d.Store.RevertPredictionToOpen(predID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"status": "overturned", "appeals": count})
		return
	}

	writeJSON(w, map[string]any{"status": "appeal_recorded", "appeals": count, "needed": 2})
}

// handleListPredictionAppeals returns all appeals for a prediction.
func (d *Daemon) handleListPredictionAppeals(w http.ResponseWriter, r *http.Request) {
	predID := r.PathValue("id")
	appeals, err := d.Store.ListPredictionAppeals(predID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if appeals == nil {
		appeals = []*store.PredictionAppeal{}
	}
	writeJSON(w, appeals)
}

// ── Wealth Leaderboard (Social Energy Model) ──

func (d *Daemon) handleWealthLeaderboard(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50)
	entries, err := d.Store.GetWealthLeaderboard(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Enrich with agent names from gossip cache
	for _, e := range entries {
		if name, ok := d.PeerAgentNames.Load(e.PeerID); ok {
			_ = name // TODO: attach agent name as extra field
		}
	}
	writeJSON(w, entries)
}

// ── Agent Resume & Matching ──

func (d *Daemon) handleUpdateResume(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Skills      []string `json:"skills"`
		DataSources []string `json:"data_sources"`
		Description string   `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	skillsJSON, _ := json.Marshal(body.Skills)
	dsJSON, _ := json.Marshal(body.DataSources)
	if body.DataSources == nil {
		dsJSON = []byte("[]")
	}

	resume := &store.AgentResume{
		PeerID:      d.Node.PeerID().String(),
		AgentName:   d.Profile.AgentName,
		Skills:      string(skillsJSON),
		DataSources: string(dsJSON),
		Description: body.Description,
	}

	// Save locally first, then respond immediately. Gossip asynchronously.
	if err := d.Store.UpsertResume(resume); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	go d.publishResume(d.ctx, resume)
	writeJSON(w, resume)
}

func (d *Daemon) handleGetOwnResume(w http.ResponseWriter, r *http.Request) {
	peerID := d.Node.PeerID().String()
	resume, err := d.Store.GetResume(peerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if resume == nil {
		// Auto-build from profile
		resume = d.buildResumeFromProfile()
	}
	if resume == nil {
		resume = &store.AgentResume{PeerID: peerID, Skills: "[]", DataSources: "[]"}
	}
	writeJSON(w, resume)
}

func (d *Daemon) handleListResumes(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50)
	resumes, err := d.Store.ListResumes(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if resumes == nil {
		resumes = []*store.AgentResume{}
	}
	writeJSON(w, resumes)
}

func (d *Daemon) handleGetPeerResume(w http.ResponseWriter, r *http.Request) {
	peerID := r.PathValue("peer_id")
	resume, err := d.Store.GetResume(peerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if resume == nil {
		http.Error(w, `{"error":"resume not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, resume)
}

func (d *Daemon) handleMatchAgentsForTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	matches, err := d.Store.MatchAgentsForTask(taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if matches == nil {
		matches = []*store.MatchResult{}
	}
	writeJSON(w, matches)
}

func (d *Daemon) handleMatchTasksForAgent(w http.ResponseWriter, r *http.Request) {
	peerID := d.Node.PeerID().String()
	tasks, err := d.Store.MatchTasksForAgent(peerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if tasks == nil {
		tasks = []*store.Task{}
	}
	writeJSON(w, tasks)
}

// handleDiscover searches for agents by skill tags with reputation-weighted ranking.
// Query params: skill (comma-separated), min_reputation (int), limit (int)
func (d *Daemon) handleDiscover(w http.ResponseWriter, r *http.Request) {
	skillParam := r.URL.Query().Get("skill")
	minRep := queryFloat(r, "min_reputation", 0)
	limit := queryInt(r, "limit", 20)

	var tags []string
	if skillParam != "" {
		for _, s := range strings.Split(skillParam, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				tags = append(tags, s)
			}
		}
	}

	// Get all resumes
	resumes, err := d.Store.ListResumes(500)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build candidate list
	var candidates []discovery.AgentCandidate
	for _, res := range resumes {
		rep, _ := d.Store.GetReputation(res.PeerID)
		repScore := 50.0
		var completed, failed int
		if rep != nil {
			repScore = rep.Score
			completed = rep.TasksCompleted
			failed = rep.TasksFailed
		}
		if repScore < minRep {
			continue
		}
		skills := discovery.ParseTagsJSON(res.Skills)
		candidates = append(candidates, discovery.AgentCandidate{
			PeerID:         res.PeerID,
			AgentName:      res.AgentName,
			Skills:         skills,
			Reputation:     repScore,
			TasksCompleted: completed,
			TasksFailed:    failed,
			ActiveTasks:    res.ActiveTasks,
		})
	}

	ranked := discovery.RankCandidates(candidates, tags, discovery.DefaultWeights())
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	writeJSON(w, ranked)
}

// ── Nutshell bundle handlers ──

func (d *Daemon) handleUploadTaskBundle(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")

	// Verify task exists
	t, err := d.Store.GetTask(taskID)
	if err != nil || t == nil {
		http.Error(w, `{"error":"task not found"}`, http.StatusNotFound)
		return
	}

	// Limit bundle size to 50 MB
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)

	var data []byte
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/") {
		// Handle multipart/form-data (nutshell CLI sends this)
		if err := r.ParseMultipartForm(50 << 20); err != nil {
			http.Error(w, `{"error":"invalid multipart form"}`, http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("bundle")
		if err != nil {
			http.Error(w, `{"error":"missing bundle field in form"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()
		data, err = io.ReadAll(file)
		if err != nil {
			http.Error(w, `{"error":"read error"}`, http.StatusBadRequest)
			return
		}
	} else {
		// Handle raw binary body
		data, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, `{"error":"bundle too large or read error"}`, http.StatusBadRequest)
			return
		}
	}

	// Validate NUT magic header
	if len(data) < 4 || string(data[:3]) != "NUT" {
		http.Error(w, `{"error":"invalid .nut bundle (bad magic)"}`, http.StatusBadRequest)
		return
	}

	// Compute SHA-256 hash
	hash := sha256hex(data)

	if err := d.Store.InsertTaskBundle(taskID, data, hash); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"task_id": taskID,
		"hash":    hash,
		"size":    len(data),
	})
}

func (d *Daemon) handleDownloadTaskBundle(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")

	bundle, hash, err := d.Store.GetTaskBundle(taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If not cached locally, try fetching from the P2P network.
	if bundle == nil {
		bundle, hash, err = d.fetchBundleFromNetwork(r.Context(), taskID)
		if err != nil || bundle == nil {
			http.Error(w, `{"error":"no bundle for this task"}`, http.StatusNotFound)
			return
		}
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.nut"`, taskID))
	w.Header().Set("X-Nutshell-Hash", hash)
	w.Write(bundle)
}

func sha256hex(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:])
}

// ── Nutshell E2E handlers ──

// handleNutshellPublish accepts a .nut file, validates it, extracts metadata,
// creates a Task, stores the bundle, and broadcasts via GossipSub.
//
// Accepts multipart/form-data with field "bundle" or raw binary body.
// Optional form fields / JSON: reward, tags, deadline.
func (d *Daemon) handleNutshellPublish(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)

	var bundleData []byte
	var reward int64
	var tags, deadline string

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/") {
		if err := r.ParseMultipartForm(50 << 20); err != nil {
			http.Error(w, `{"error":"invalid multipart form"}`, http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("bundle")
		if err != nil {
			http.Error(w, `{"error":"missing bundle field"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()
		bundleData, err = io.ReadAll(file)
		if err != nil {
			http.Error(w, `{"error":"read error"}`, http.StatusBadRequest)
			return
		}
		// Optional form fields
		if v := r.FormValue("reward"); v != "" {
			fmt.Sscanf(v, "%d", &reward)
		}
		tags = r.FormValue("tags")
		deadline = r.FormValue("deadline")
	} else {
		var err error
		bundleData, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, `{"error":"bundle too large or read error"}`, http.StatusBadRequest)
			return
		}
	}

	// Validate NUT magic header
	if len(bundleData) < 4 || string(bundleData[:3]) != "NUT" {
		http.Error(w, `{"error":"invalid .nut bundle (bad magic header)"}`, http.StatusBadRequest)
		return
	}

	// Extract nutshell manifest from bundle.
	// Format: NUT<version_byte><4-byte manifest length><JSON manifest><payload...>
	manifest, err := parseNutManifest(bundleData)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid manifest: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	// Compute content-addressed hash
	hash := sha256hex(bundleData)

	// Build task from manifest
	title := manifest.Name
	if title == "" {
		title = "Nutshell Task"
	}
	desc := manifest.Description
	if manifest.AcceptanceCriteria != "" {
		desc += "\n\nAcceptance Criteria: " + manifest.AcceptanceCriteria
	}

	t := store.Task{
		Title:        title,
		Description:  desc,
		NutshellHash: hash,
		NutshellID:   manifest.ID,
		BundleType:   "request",
	}
	if reward > 0 {
		t.Reward = reward
	}
	if tags != "" {
		t.Tags = tags
	}
	if deadline != "" {
		t.Deadline = deadline
	}

	// Publish task (assigns ID, broadcasts via GossipSub)
	if err := d.publishTask(d.ctx, &t); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Store the bundle blob
	if err := d.Store.InsertTaskBundle(t.ID, bundleData, hash); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"store bundle: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"task_id":       t.ID,
		"nutshell_hash": hash,
		"nutshell_id":   manifest.ID,
		"title":         t.Title,
		"bundle_size":   len(bundleData),
	})
}

// handleNutshellDeliver accepts a completed .nut result bundle for a task,
// validates it, stores it, and submits the task.
func (d *Daemon) handleNutshellDeliver(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")

	t, err := d.Store.GetTask(taskID)
	if err != nil || t == nil {
		http.Error(w, `{"error":"task not found"}`, http.StatusNotFound)
		return
	}
	if t.Status != "assigned" {
		http.Error(w, `{"error":"task must be in assigned status to deliver"}`, http.StatusBadRequest)
		return
	}
	// Only the assignee can deliver
	selfID := d.Node.PeerID().String()
	if t.AssignedTo != selfID {
		http.Error(w, `{"error":"only the assigned agent can deliver"}`, http.StatusForbidden)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)

	var bundleData []byte
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/") {
		if err := r.ParseMultipartForm(50 << 20); err != nil {
			http.Error(w, `{"error":"invalid multipart form"}`, http.StatusBadRequest)
			return
		}
		file, _, fErr := r.FormFile("bundle")
		if fErr != nil {
			http.Error(w, `{"error":"missing bundle field"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()
		bundleData, err = io.ReadAll(file)
		if err != nil {
			http.Error(w, `{"error":"read error"}`, http.StatusBadRequest)
			return
		}
	} else {
		bundleData, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, `{"error":"bundle too large or read error"}`, http.StatusBadRequest)
			return
		}
	}

	// Validate NUT magic
	if len(bundleData) < 4 || string(bundleData[:3]) != "NUT" {
		http.Error(w, `{"error":"invalid .nut bundle (bad magic header)"}`, http.StatusBadRequest)
		return
	}

	hash := sha256hex(bundleData)

	// Store the delivery bundle (overwrite if exists)
	if err := d.Store.InsertTaskBundle(taskID, bundleData, hash); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"store bundle: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Transition task to submitted status
	t.Status = "submitted"
	t.Result = fmt.Sprintf("delivery bundle: sha256:%s (%d bytes)", hash, len(bundleData))
	d.publishTaskUpdate(d.ctx, t)

	writeJSON(w, map[string]any{
		"task_id": taskID,
		"status":  "submitted",
		"hash":    hash,
		"size":    len(bundleData),
	})
}

// nutManifest represents the parsed metadata from a .nut bundle.
type nutManifest struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	AcceptanceCriteria string `json:"acceptance_criteria"`
}

// parseNutManifest extracts the JSON manifest from a .nut bundle.
// Real nutshell format: NUT<version_byte><gzip(tar(nutshell.json + files...))>
// The nutshell.json inside the tar contains the manifest. Task fields are
// nested: {"id", "task": {"title", "description", "acceptance_criteria"}}.
func parseNutManifest(data []byte) (*nutManifest, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("bundle too small")
	}

	// Skip 4-byte header: NUT + version byte
	gzReader, err := gzip.NewReader(bytes.NewReader(data[4:]))
	if err != nil {
		return nil, fmt.Errorf("gzip decompress: %w", err)
	}
	defer gzReader.Close()

	tr := tar.NewReader(gzReader)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar read: %w", err)
		}
		if hdr.Name == "nutshell.json" {
			// Limit manifest size to 1 MB
			mData, err := io.ReadAll(io.LimitReader(tr, 1<<20))
			if err != nil {
				return nil, fmt.Errorf("read manifest: %w", err)
			}
			return parseNutManifestJSON(mData)
		}
	}
	return nil, fmt.Errorf("nutshell.json not found in bundle")
}

// parseNutManifestJSON parses the nutshell.json content into a nutManifest.
func parseNutManifestJSON(data []byte) (*nutManifest, error) {
	var raw struct {
		ID   string `json:"id"`
		Task struct {
			Title              string `json:"title"`
			Description        string `json:"description"`
			AcceptanceCriteria string `json:"acceptance_criteria"`
		} `json:"task"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("manifest JSON: %w", err)
	}
	m := &nutManifest{
		ID:                 raw.ID,
		Name:               raw.Task.Title,
		Description:        raw.Task.Description,
		AcceptanceCriteria: raw.Task.AcceptanceCriteria,
	}
	if m.Name == "" {
		// Fallback: try top-level name field
		var fallback struct {
			Name string `json:"name"`
		}
		json.Unmarshal(data, &fallback)
		m.Name = fallback.Name
	}
	if m.Name == "" {
		return nil, fmt.Errorf("manifest missing required field: task.title or name")
	}
	return m, nil
}
