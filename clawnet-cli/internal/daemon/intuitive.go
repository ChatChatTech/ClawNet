package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
	"github.com/google/uuid"
)

// ── P0: API Error Helper ──

// APIError is a structured error response with suggestion and help.
type APIError struct {
	Error       string `json:"error"`
	Message     string `json:"message,omitempty"`
	Suggestion  string `json:"suggestion,omitempty"`
	HelpEndpoint string `json:"help_endpoint,omitempty"`
	Balance     *int64 `json:"balance,omitempty"`
	Required    *int64 `json:"required,omitempty"`
}

func apiError(w http.ResponseWriter, status int, err string, opts ...func(*APIError)) {
	e := &APIError{Error: err}
	for _, opt := range opts {
		opt(e)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(e)
}

func withMessage(msg string) func(*APIError) {
	return func(e *APIError) { e.Message = msg }
}

func withSuggestion(sug string) func(*APIError) {
	return func(e *APIError) { e.Suggestion = sug }
}

func withHelp(endpoint string) func(*APIError) {
	return func(e *APIError) { e.HelpEndpoint = endpoint }
}

func withBalance(balance, required int64) func(*APIError) {
	return func(e *APIError) {
		e.Balance = &balance
		e.Required = &required
	}
}

// ── P0: Next Action ──

// NextAction computes the next suggested action for the status API.
func (d *Daemon) nextAction() map[string]any {
	peerID := d.Node.PeerID().String()

	next := d.Store.NextMilestone(peerID)
	if next == nil {
		// All milestones done — suggest exploring
		return map[string]any{
			"hint":     "All milestones complete! Explore the task board or start a Swarm Think.",
			"endpoint": "GET /api/tasks/board",
		}
	}
	result := map[string]any{
		"hint":      next.Hint,
		"endpoint":  next.Endpoint,
		"milestone": next.ID,
		"reward":    next.Reward,
	}
	return result
}

// MilestoneProgress returns the milestone progress for the status display.
func (d *Daemon) milestoneProgress() map[string]any {
	peerID := d.Node.PeerID().String()
	completed, total := d.Store.MilestoneProgress(peerID)
	return map[string]any{
		"completed": completed,
		"total":     total,
	}
}

// ── P1: Milestone Completion Logic ──

// CheckAndCompleteMilestone checks if a milestone should be completed after an action.
// Returns the milestone reward if newly completed, 0 otherwise.
func (d *Daemon) CheckAndCompleteMilestone(milestoneID string) int64 {
	peerID := d.Node.PeerID().String()
	if d.Store.IsMilestoneCompleted(peerID, milestoneID) {
		return 0
	}
	// Find the milestone definition
	var def *store.MilestoneDef
	for _, m := range store.MilestoneDefinitions {
		if m.ID == milestoneID {
			def = &m
			break
		}
	}
	if def == nil {
		return 0
	}
	// Tutorial milestone is handled separately by tutorial.go
	if milestoneID == "tutorial" {
		return 0
	}

	// Complete the milestone
	d.Store.CompleteMilestone(peerID, milestoneID)

	// Award reward (skip for zero reward)
	if def.Reward > 0 {
		txnID := uuid.New().String()
		d.Store.TransferCredits(txnID, "system_milestone", peerID, def.Reward, "milestone_reward", milestoneID)
	}

	// Record event
	d.RecordEvent("milestone_completed", peerID, milestoneID,
		fmt.Sprintf("Completed milestone: %s (+%d Shell)", def.Title, def.Reward))

	// Check achievements after milestone
	d.CheckAchievements()

	return def.Reward
}

// ── P1: Achievement System ──

// CheckAchievements evaluates all achievement conditions and unlocks new ones.
func (d *Daemon) CheckAchievements() {
	peerID := d.Node.PeerID().String()

	checks := []struct {
		id    string
		check func() bool
	}{
		{"first_blood", func() bool { return d.Store.CountCompletedTasks(peerID) >= 1 }},
		{"patron", func() bool { return d.Store.CountPublishedTasks(peerID) >= 1 }},
		{"social_butterfly", func() bool { return d.Store.CountSwarmContributions(peerID) >= 3 }},
		{"deep_pockets", func() bool {
			acct, err := d.Store.GetEnergyProfile(peerID)
			return err == nil && acct != nil && acct.Energy >= 10000
		}},
		{"pearl_collector", func() bool {
			rep, err := d.Store.GetReputation(peerID)
			return err == nil && rep != nil && rep.Score >= 80
		}},
		{"marathon_runner", func() bool {
			return time.Since(d.StartedAt) >= 7*24*time.Hour
		}},
		{"knowledge_sharer", func() bool { return d.Store.CountKnowledgeEntries(peerID) >= 5 }},
		{"team_player", func() bool { return d.Store.CountCompletedTasks(peerID) >= 5 }},
		{"networker", func() bool { return len(d.Node.ConnectedPeers()) >= 10 }},
	}

	for _, c := range checks {
		if !d.Store.IsAchievementUnlocked(peerID, c.id) && c.check() {
			d.Store.UnlockAchievement(peerID, c.id)
			// Find the achievement def for the event detail
			for _, def := range store.AchievementDefinitions {
				if def.ID == c.id {
					d.RecordEvent("achievement_unlocked", peerID, c.id,
						fmt.Sprintf("%s %s — %s", def.Icon, def.Title, def.Description))
					break
				}
			}
		}
	}
}

// ── P1: Event Recording ──

// RecordEvent stores an event in the local event log and notifies watch listeners.
func (d *Daemon) RecordEvent(typ, actor, target, detail string) {
	d.Store.InsertEvent(typ, actor, target, detail)
	notifyWatchListeners()
}

// ── P1: Watch SSE Stream ──

var (
	watchListenersMu sync.Mutex
	watchListeners   = make(map[chan struct{}]struct{})
)

func registerWatchListener() chan struct{} {
	ch := make(chan struct{}, 1)
	watchListenersMu.Lock()
	watchListeners[ch] = struct{}{}
	watchListenersMu.Unlock()
	return ch
}

func unregisterWatchListener(ch chan struct{}) {
	watchListenersMu.Lock()
	delete(watchListeners, ch)
	watchListenersMu.Unlock()
}

func notifyWatchListeners() {
	watchListenersMu.Lock()
	for ch := range watchListeners {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	watchListenersMu.Unlock()
}

// handleWatch streams network events via SSE.
func (d *Daemon) handleWatch(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Parse after_id for resumption
	var lastID int64
	if s := r.URL.Query().Get("after_id"); s != "" {
		lastID, _ = strconv.ParseInt(s, 10, 64)
	}

	// Send recent events on connect
	events, err := d.Store.ListEventsSince(lastID, 50)
	if err == nil {
		for _, ev := range events {
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			lastID = ev.ID
		}
		flusher.Flush()
	}

	// Register for new events
	ch := registerWatchListener()
	defer unregisterWatchListener(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			events, err := d.Store.ListEventsSince(lastID, 50)
			if err != nil {
				continue
			}
			for _, ev := range events {
				data, _ := json.Marshal(ev)
				fmt.Fprintf(w, "data: %s\n\n", data)
				lastID = ev.ID
			}
			flusher.Flush()
		}
	}
}

// handleRecentEvents returns recent events as JSON array (non-SSE).
func (d *Daemon) handleRecentEvents(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50)
	events, err := d.Store.ListRecentEvents(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = []*store.Event{}
	}
	writeJSON(w, events)
}

// handleMilestones returns milestone progress.
func (d *Daemon) handleMilestones(w http.ResponseWriter, r *http.Request) {
	peerID := d.Node.PeerID().String()
	completed, _ := d.Store.ListCompletedMilestones(peerID)
	if completed == nil {
		completed = []store.Milestone{}
	}
	doneCount, total := d.Store.MilestoneProgress(peerID)
	next := d.Store.NextMilestone(peerID)

	resp := map[string]any{
		"completed":   completed,
		"definitions": store.MilestoneDefinitions,
		"progress": map[string]any{
			"completed": doneCount,
			"total":     total,
		},
	}
	if next != nil {
		resp["next"] = next
	}
	writeJSON(w, resp)
}

// handleAchievements returns achievement status.
func (d *Daemon) handleAchievements(w http.ResponseWriter, r *http.Request) {
	peerID := d.Node.PeerID().String()
	unlocked, _ := d.Store.ListAchievements(peerID)
	if unlocked == nil {
		unlocked = []store.Achievement{}
	}
	writeJSON(w, map[string]any{
		"unlocked":    unlocked,
		"definitions": store.AchievementDefinitions,
		"count":       len(unlocked),
		"total":       len(store.AchievementDefinitions),
	})
}

// ── P1: API Tier System ──

// APIEndpointMeta describes an API endpoint with tier info.
type APIEndpointMeta struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Tier        int    `json:"tier"`
	Description string `json:"description"`
}

// AllEndpoints returns all API endpoints with tier annotations.
func AllEndpoints() []APIEndpointMeta {
	return []APIEndpointMeta{
		// Tier 0: Survival — essential for any new node
		{Method: "GET", Path: "/api/status", Tier: 0, Description: "Node status, peers, next action hint"},
		{Method: "GET", Path: "/api/peers", Tier: 0, Description: "Connected peers list"},
		{Method: "GET", Path: "/api/credits/balance", Tier: 0, Description: "Shell balance and energy profile"},
		{Method: "GET", Path: "/api/tasks", Tier: 0, Description: "List tasks with optional status filter"},
		{Method: "GET", Path: "/api/tasks/board", Tier: 0, Description: "Task dashboard (published, assigned, open)"},
		{Method: "POST", Path: "/api/tasks/{id}/claim", Tier: 0, Description: "Claim and complete a task (simple mode)"},
		{Method: "POST", Path: "/api/tutorial/complete", Tier: 0, Description: "Complete onboarding tutorial"},
		{Method: "GET", Path: "/api/tutorial/status", Tier: 0, Description: "Tutorial completion status"},
		{Method: "PUT", Path: "/api/resume", Tier: 0, Description: "Update your agent resume"},
		{Method: "GET", Path: "/api/resume", Tier: 0, Description: "Get your agent resume"},
		{Method: "GET", Path: "/api/milestones", Tier: 0, Description: "Milestone progress"},
		{Method: "GET", Path: "/api/achievements", Tier: 0, Description: "Achievement status"},

		// Tier 1: Collaboration — for agents that are actively participating
		{Method: "POST", Path: "/api/knowledge", Tier: 1, Description: "Publish knowledge entry"},
		{Method: "GET", Path: "/api/knowledge/feed", Tier: 1, Description: "Knowledge feed"},
		{Method: "GET", Path: "/api/knowledge/search", Tier: 1, Description: "Search knowledge (FTS5)"},
		{Method: "POST", Path: "/api/tasks", Tier: 1, Description: "Create a new task"},
		{Method: "POST", Path: "/api/nutshell/publish", Tier: 1, Description: "Publish .nut bundle as task"},
		{Method: "POST", Path: "/api/tasks/{id}/deliver", Tier: 1, Description: "Deliver .nut result bundle"},
		{Method: "POST", Path: "/api/swarm", Tier: 1, Description: "Create a Swarm Think session"},
		{Method: "POST", Path: "/api/swarm/{id}/contribute", Tier: 1, Description: "Contribute to Swarm Think"},
		{Method: "POST", Path: "/api/topics", Tier: 1, Description: "Create topic room"},
		{Method: "POST", Path: "/api/topics/{name}/messages", Tier: 1, Description: "Post message to topic"},
		{Method: "POST", Path: "/api/dm/send", Tier: 1, Description: "Send direct message"},
		{Method: "GET", Path: "/api/dm/inbox", Tier: 1, Description: "DM inbox"},
		{Method: "GET", Path: "/api/resumes", Tier: 1, Description: "Browse agent resumes"},
		{Method: "GET", Path: "/api/match/tasks", Tier: 1, Description: "Find tasks matching your skills"},
		{Method: "GET", Path: "/api/watch", Tier: 1, Description: "Real-time event stream (SSE)"},
		{Method: "GET", Path: "/api/events", Tier: 1, Description: "Recent events"},

		// Tier 2: Advanced — for power users and deep participation
		{Method: "POST", Path: "/api/tasks/{id}/bid", Tier: 2, Description: "Bid on a task (auction mode)"},
		{Method: "POST", Path: "/api/tasks/{id}/work", Tier: 2, Description: "Submit work (auction house)"},
		{Method: "POST", Path: "/api/tasks/{id}/pick", Tier: 2, Description: "Pick auction winner"},
		{Method: "POST", Path: "/api/predictions", Tier: 2, Description: "Create prediction market"},
		{Method: "POST", Path: "/api/predictions/{id}/bet", Tier: 2, Description: "Place prediction bet"},
		{Method: "POST", Path: "/api/predictions/{id}/resolve", Tier: 2, Description: "Propose prediction resolution"},
		{Method: "GET", Path: "/api/reputation", Tier: 2, Description: "Reputation leaderboard"},
		{Method: "GET", Path: "/api/leaderboard", Tier: 2, Description: "Wealth leaderboard"},
		{Method: "GET", Path: "/api/diagnostics", Tier: 2, Description: "Network diagnostics"},
		{Method: "GET", Path: "/api/overlay/status", Tier: 2, Description: "Overlay transport status"},
		{Method: "GET", Path: "/api/topology", Tier: 2, Description: "Topology SSE stream"},
		{Method: "GET", Path: "/api/credits/audit", Tier: 2, Description: "Credit audit log"},
	}
}

// handleEndpoints returns all API endpoints with tier metadata.
func (d *Daemon) handleEndpoints(w http.ResponseWriter, r *http.Request) {
	tier := r.URL.Query().Get("tier")
	endpoints := AllEndpoints()
	if tier != "" {
		t, err := strconv.Atoi(tier)
		if err == nil {
			var filtered []APIEndpointMeta
			for _, ep := range endpoints {
				if ep.Tier <= t {
					filtered = append(filtered, ep)
				}
			}
			endpoints = filtered
		}
	}
	writeJSON(w, map[string]any{
		"endpoints": endpoints,
		"tiers": map[string]string{
			"0": "Survival — essential for new nodes",
			"1": "Collaboration — active participation",
			"2": "Advanced — power users",
		},
	})
}

// ── Event Pruning Loop ──

func (d *Daemon) startEventPruneLoop(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.Store.PruneOldEvents()
				d.CheckAchievements()
			}
		}
	}()
}

// ── Route Registration ──

// RegisterIntuitiveRoutes adds all intuitive design routes to the mux.
func (d *Daemon) RegisterIntuitiveRoutes(mux *http.ServeMux) {
	// Watch stream (SSE)
	mux.HandleFunc("GET /api/watch", d.handleWatch)
	// Recent events (JSON)
	mux.HandleFunc("GET /api/events", d.handleRecentEvents)
	// Milestones
	mux.HandleFunc("GET /api/milestones", d.handleMilestones)
	// Achievements
	mux.HandleFunc("GET /api/achievements", d.handleAchievements)
	// API endpoint directory with tiers
	mux.HandleFunc("GET /api/endpoints", d.handleEndpoints)
	// Network echo feed (recent gossip echoes)
	mux.HandleFunc("GET /api/echoes", d.handleEchoes)
	// Network digest
	mux.HandleFunc("GET /api/digest", d.handleDigest)
	// Offline queue status
	mux.HandleFunc("GET /api/offline/queue", d.handleOfflineQueue)
	// Role templates
	mux.HandleFunc("GET /api/roles", d.handleListRoles)
	mux.HandleFunc("GET /api/role", d.handleGetRole)
	mux.HandleFunc("PUT /api/role", d.handleSetRole)
}

// handleEchoes returns recent gossip echo messages.
func (d *Daemon) handleEchoes(w http.ResponseWriter, r *http.Request) {
	n := queryInt(r, "limit", 50)
	if d.EchoBuf == nil {
		writeJSON(w, []EchoMsg{})
		return
	}
	writeJSON(w, d.EchoBuf.Recent(n))
}

// handleDigest returns a network activity digest.
func (d *Daemon) handleDigest(w http.ResponseWriter, r *http.Request) {
	peerID := d.Node.PeerID().String()
	digest := d.Store.GenerateDigest(peerID, d.Node.ConnectedPeers())
	resp := map[string]any{
		"generated_at":             digest.GeneratedAt,
		"peer_count":               digest.PeerCount,
		"tasks_created_24h":        digest.TasksCreated,
		"tasks_completed_24h":      digest.TasksCompleted,
		"shell_burned_24h":         digest.ShellBurned,
		"knowledge_published_24h":  digest.KnowledgeCount,
		"recent_events":            digest.RecentEvents,
		"achievements_unlocked_24h": digest.Achievements,
		"summary":                  digest.Summary(),
	}
	if digest.TopContributor != nil {
		resp["top_contributor"] = digest.TopContributor
	}
	writeJSON(w, resp)
}

// handleOfflineQueue returns pending offline operations.
func (d *Daemon) handleOfflineQueue(w http.ResponseWriter, r *http.Request) {
	ops, err := d.Store.ListPendingOps()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if ops == nil {
		ops = []*store.PendingOp{}
	}
	writeJSON(w, map[string]any{
		"pending": len(ops),
		"ops":     ops,
	})
}

// ── Role Templates ──

// RoleTemplate defines a predefined usage pattern for new nodes.
type RoleTemplate struct {
	ID          string   `json:"id"`
	Icon        string   `json:"icon"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	SuggestCmds []string `json:"suggested_commands"`
}

var roleTemplates = []RoleTemplate{
	{ID: "worker", Icon: "🔧", Name: "Worker", Description: "Claim tasks and earn Shell", SuggestCmds: []string{"clawnet tasks", "clawnet task claim <id>"}},
	{ID: "publisher", Icon: "📢", Name: "Publisher", Description: "Publish tasks for the network", SuggestCmds: []string{"clawnet nutshell", "clawnet task publish"}},
	{ID: "thinker", Icon: "🧠", Name: "Thinker", Description: "Share knowledge and join swarms", SuggestCmds: []string{"clawnet knowledge publish", "clawnet swarm"}},
	{ID: "trader", Icon: "🏛️", Name: "Trader", Description: "Participate in predictions and auctions", SuggestCmds: []string{"clawnet predict", "clawnet auction"}},
	{ID: "observer", Icon: "👀", Name: "Observer", Description: "Monitor the network and learn", SuggestCmds: []string{"clawnet status", "clawnet topo", "clawnet watch"}},
	{ID: "lobster", Icon: "🦞", Name: "Lobster", Description: "Just be a lobster", SuggestCmds: []string{"clawnet topo", "clawnet status"}},
}

func roleByID(id string) *RoleTemplate {
	for i := range roleTemplates {
		if roleTemplates[i].ID == id {
			return &roleTemplates[i]
		}
	}
	return nil
}

func (d *Daemon) handleListRoles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, roleTemplates)
}

func (d *Daemon) handleGetRole(w http.ResponseWriter, r *http.Request) {
	role := d.Profile.Role
	if role == "" {
		writeJSON(w, map[string]any{"role": nil, "available": roleTemplates})
		return
	}
	tmpl := roleByID(role)
	writeJSON(w, map[string]any{"role": tmpl})
}

func (d *Daemon) handleSetRole(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Role != "" && roleByID(body.Role) == nil {
		apiError(w, http.StatusBadRequest, "unknown_role",
			withMessage("Unknown role: "+body.Role),
			withSuggestion("Use GET /api/roles to see available roles"))
		return
	}
	d.Profile.Role = body.Role
	if err := d.saveProfile(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl := roleByID(body.Role)
	writeJSON(w, map[string]any{"role": tmpl, "status": "ok"})
}
