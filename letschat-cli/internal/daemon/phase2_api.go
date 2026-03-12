package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ChatChatTech/letschat/letschat-cli/internal/store"
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
}

// ── Credits handlers ──

func (d *Daemon) handleCreditsBalance(w http.ResponseWriter, r *http.Request) {
	peerID := d.Node.PeerID().String()
	acc, err := d.Store.GetCreditBalance(peerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, acc)
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

	// Recalc reputation for assignee
	if t.AssignedTo != "" {
		d.Store.RecalcReputation(t.AssignedTo)
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
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Body == "" {
		http.Error(w, `{"error":"body is required"}`, http.StatusBadRequest)
		return
	}
	c := &store.SwarmContribution{SwarmID: swarmID, Body: body.Body}
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

	// Recalc reputation for all contributors
	contribs, _ := d.Store.ListSwarmContributions(swarmID, 1000)
	seen := map[string]bool{}
	for _, c := range contribs {
		if !seen[c.AuthorID] {
			d.Store.RecalcReputation(c.AuthorID)
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
