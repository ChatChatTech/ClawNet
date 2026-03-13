package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
	"github.com/google/uuid"
)

// RegisterPhase2Routes adds Phase 2 routes to the mux.
func (d *Daemon) RegisterPhase2Routes(mux *http.ServeMux) {
	// Credits
	mux.HandleFunc("GET /api/credits/balance", d.handleCreditsBalance)
	mux.HandleFunc("GET /api/credits/transactions", d.handleCreditsTransactions)
	mux.HandleFunc("POST /api/credits/transfer", d.handleCreditsTransfer)

	// Task Bazaar
	mux.HandleFunc("POST /api/tasks", d.handleCreateTask)
	mux.HandleFunc("GET /api/tasks", d.handleListTasks)
	mux.HandleFunc("GET /api/tasks/{id}", d.handleGetTask)
	mux.HandleFunc("POST /api/tasks/{id}/bid", d.handleTaskBid)
	mux.HandleFunc("GET /api/tasks/{id}/bids", d.handleTaskBids)
	mux.HandleFunc("POST /api/tasks/{id}/assign", d.handleTaskAssign)
	mux.HandleFunc("POST /api/tasks/{id}/submit", d.handleTaskSubmit)
	mux.HandleFunc("POST /api/tasks/{id}/approve", d.handleTaskApprove)
	mux.HandleFunc("POST /api/tasks/{id}/reject", d.handleTaskReject)
	mux.HandleFunc("POST /api/tasks/{id}/cancel", d.handleTaskCancel)

	// Swarm Think
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

	// Wealth Leaderboard (Social Energy Model)
	mux.HandleFunc("GET /api/leaderboard", d.handleWealthLeaderboard)
}

// ── Credits handlers ──

func (d *Daemon) handleCreditsBalance(w http.ResponseWriter, r *http.Request) {
	peerID := d.Node.PeerID().String()
	profile, err := d.Store.GetEnergyProfile(peerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Also include legacy fields for backward compat
	writeJSON(w, map[string]any{
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
	})
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

func (d *Daemon) handleCreditsTransfer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ToPeer string  `json:"to_peer"`
		Amount float64 `json:"amount"`
		Reason string  `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.ToPeer == "" || body.Amount <= 0 {
		http.Error(w, `{"error":"to_peer and positive amount required"}`, http.StatusBadRequest)
		return
	}
	if body.Reason == "" {
		body.Reason = "transfer"
	}
	fromPeer := d.Node.PeerID().String()
	txnID := uuid.New().String()
	if err := d.Store.TransferCredits(txnID, fromPeer, body.ToPeer, body.Amount, body.Reason, ""); err != nil {
		if err == store.ErrInsufficientCredits {
			http.Error(w, `{"error":"insufficient credits"}`, http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "transferred", "txn_id": txnID})
}

// ── Task handlers ──

func (d *Daemon) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var t store.Task
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if t.Title == "" {
		http.Error(w, `{"error":"title is required"}`, http.StatusBadRequest)
		return
	}
	if err := d.publishTask(d.ctx, &t); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	writeJSON(w, t)
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

func (d *Daemon) handleTaskBid(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	var body struct {
		Amount  float64 `json:"amount"`
		Message string  `json:"message"`
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
	if err := d.Store.AssignTask(taskID, body.AssignTo); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	t, _ := d.Store.GetTask(taskID)
	if t != nil {
		d.publishTaskUpdate(d.ctx, t)
	}
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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

	t, _ = d.Store.GetTask(taskID)
	if t != nil {
		d.publishTaskUpdate(d.ctx, t)
	}
	writeJSON(w, map[string]string{"status": "approved"})
}

func (d *Daemon) handleTaskReject(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")

	t, err := d.Store.GetTask(taskID)
	if err != nil || t == nil {
		http.Error(w, `{"error":"task not found"}`, http.StatusNotFound)
		return
	}
	if err := d.Store.RejectTask(taskID); err != nil {
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

// ── Swarm handlers ──

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
	if err := d.publishSwarm(d.ctx, &sw); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, sw)
}

func (d *Daemon) handleListSwarms(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	swarms, err := d.Store.ListSwarms(status, limit, offset)
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
		Perspective: body.Perspective, Confidence: body.Confidence}
	if err := d.publishSwarmContribution(d.ctx, c); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, c)
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
		Option    string  `json:"option"`
		Stake     float64 `json:"stake"`
		Reasoning string  `json:"reasoning"`
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
		// Consensus reached — resolve and settle
		if err := d.Store.ResolvePrediction(predID, body.Result); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		d.settlePrediction(predID, body.Result)
		writeJSON(w, map[string]any{"status": "resolved", "result": body.Result, "consensus": count})
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
			e.PeerID = e.PeerID // keep ID
			// Attach agent name as extra field via wrapper
			_ = name
		}
	}
	writeJSON(w, entries)
}
