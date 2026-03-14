package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
	"github.com/google/uuid"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

const (
	TaskTopic       = "/clawnet/tasks"
	SwarmTopic      = "/clawnet/swarm"
	CreditAudit     = "/clawnet/credit-audit"
	PredictionTopic = "/clawnet/predictions"
	ResumeTopic     = "/clawnet/resumes"
	DefaultReward   = 1.0  // default credit reward when no explicit reward is set
)

// GossipTaskMsg is the wire format for task messages.
type GossipTaskMsg struct {
	Type string         `json:"type"` // "task", "bid", "update"
	Task *store.Task    `json:"task,omitempty"`
	Bid  *store.TaskBid `json:"bid,omitempty"`
}

// GossipSwarmMsg is the wire format for swarm messages.
type GossipSwarmMsg struct {
	Type         string                   `json:"type"` // "swarm", "contribution", "synthesis"
	Swarm        *store.Swarm             `json:"swarm,omitempty"`
	Contribution *store.SwarmContribution `json:"contribution,omitempty"`
}

// CreditAuditMsg is broadcast so peers can verify credit settlements.
type CreditAuditMsg struct {
	TxnID    string  `json:"txn_id"`
	TaskID   string  `json:"task_id"`
	From     string  `json:"from"`
	To       string  `json:"to"`
	Amount   float64 `json:"amount"`
	Reason   string  `json:"reason"`
	Time     string  `json:"time"`
}

// GossipPredictionMsg is the wire format for prediction market messages.
type GossipPredictionMsg struct {
	Type       string                    `json:"type"` // "prediction", "bet", "resolution"
	Prediction *store.Prediction         `json:"prediction,omitempty"`
	Bet        *store.PredictionBet      `json:"bet,omitempty"`
	Resolution *store.PredictionResolution `json:"resolution,omitempty"`
}

// GossipResumeMsg is the wire format for agent resume broadcasts.
type GossipResumeMsg struct {
	Type   string              `json:"type"` // "resume"
	Resume *store.AgentResume  `json:"resume,omitempty"`
}

// startPhase2Gossip subscribes to task and swarm GossipSub topics.
func (d *Daemon) startPhase2Gossip(ctx context.Context) {
	// Tasks topic
	taskSub, err := d.Node.JoinTopic(TaskTopic)
	if err != nil {
		fmt.Printf("warning: could not join task topic: %v\n", err)
	} else {
		go d.handleTaskSub(ctx, taskSub)
	}

	// Swarm topic
	swarmSub, err := d.Node.JoinTopic(SwarmTopic)
	if err != nil {
		fmt.Printf("warning: could not join swarm topic: %v\n", err)
	} else {
		go d.handleSwarmSub(ctx, swarmSub)
	}

	// Credit audit topic
	auditSub, err := d.Node.JoinTopic(CreditAudit)
	if err != nil {
		fmt.Printf("warning: could not join credit-audit topic: %v\n", err)
	} else {
		go d.handleCreditAuditSub(ctx, auditSub)
	}

	// Prediction market topic
	predSub, err := d.Node.JoinTopic(PredictionTopic)
	if err != nil {
		fmt.Printf("warning: could not join prediction topic: %v\n", err)
	} else {
		go d.handlePredictionSub(ctx, predSub)
	}

	// Agent resume topic
	resumeSub, err := d.Node.JoinTopic(ResumeTopic)
	if err != nil {
		fmt.Printf("warning: could not join resume topic: %v\n", err)
	} else {
		go d.handleResumeSub(ctx, resumeSub)
	}

	// Broadcast own resume periodically
	go d.resumeBroadcastLoop(ctx)

	// Swarm auto-close timer: check every 60s for expired swarms
	go d.swarmExpiryLoop(ctx)

	// Energy regen loop: regenerate energy for all accounts based on prestige
	go d.energyRegenLoop(ctx)

	// Prestige decay loop: apply daily 0.998 decay
	go d.prestigeDecayLoop(ctx)
}

// swarmExpiryLoop periodically closes expired swarms.
func (d *Daemon) swarmExpiryLoop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ids, err := d.Store.CloseExpiredSwarms()
			if err != nil {
				continue
			}
			for _, id := range ids {
				fmt.Printf("swarm: auto-closed expired swarm %s\n", id[:8])
				// Recalc reputation for all contributors
				contribs, _ := d.Store.ListSwarmContributions(id, 1000)
				seen := map[string]bool{}
				for _, c := range contribs {
					if !seen[c.AuthorID] {
						d.Store.RecalcReputation(c.AuthorID)
						seen[c.AuthorID] = true
					}
				}
			}
		}
	}
}

// reputationGrantLoop grants +10 credits per week to nodes with reputation > 50.
// Runs every hour, granting 10/168 ≈ 0.06 credits per check to smooth distribution.
// DEPRECATED: replaced by energyRegenLoop, kept for reference.

// energyRegenLoop regenerates energy for all accounts based on prestige.
// Rate: 1 + ln(1 + P/10) E/day per account.
func (d *Daemon) energyRegenLoop(ctx context.Context) {
	// Wait 2 minutes before first check
	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Minute):
	}

	ticker := time.NewTicker(30 * time.Minute) // check every 30 min
	defer ticker.Stop()

	regen := func() {
		n, err := d.Store.RegenAllEnergy()
		if err != nil {
			fmt.Printf("energy-regen: error: %v\n", err)
			return
		}
		if n > 0 {
			fmt.Printf("energy-regen: regenerated energy for %d accounts\n", n)
		}
	}

	regen() // first run
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			regen()
		}
	}
}

// prestigeDecayLoop applies daily prestige decay (0.998×) to all accounts.
func (d *Daemon) prestigeDecayLoop(ctx context.Context) {
	// Wait 3 minutes before first check
	select {
	case <-ctx.Done():
		return
	case <-time.After(3 * time.Minute):
	}

	ticker := time.NewTicker(1 * time.Hour) // check every hour, decay is per-day
	defer ticker.Stop()

	decay := func() {
		n, err := d.Store.DecayAllPrestige()
		if err != nil {
			fmt.Printf("prestige-decay: error: %v\n", err)
			return
		}
		if n > 0 {
			fmt.Printf("prestige-decay: decayed %d accounts\n", n)
		}
	}

	decay() // first run
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			decay()
		}
	}
}

func (d *Daemon) handleTaskSub(ctx context.Context, sub *pubsub.Subscription) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			return
		}
		if msg.ReceivedFrom == d.Node.PeerID() {
			continue
		}

		var gm GossipTaskMsg
		if err := json.Unmarshal(msg.Data, &gm); err != nil {
			continue
		}

		switch gm.Type {
		case "task":
			if gm.Task != nil {
				d.Store.InsertTask(gm.Task)
			}
		case "bid":
			if gm.Bid != nil {
				d.Store.InsertTaskBid(gm.Bid)
			}
		case "update":
			if gm.Task != nil {
				d.Store.InsertTask(gm.Task)
			}
		}
	}
}

func (d *Daemon) handleSwarmSub(ctx context.Context, sub *pubsub.Subscription) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			return
		}
		if msg.ReceivedFrom == d.Node.PeerID() {
			continue
		}

		var gm GossipSwarmMsg
		if err := json.Unmarshal(msg.Data, &gm); err != nil {
			continue
		}

		switch gm.Type {
		case "swarm":
			if gm.Swarm != nil {
				d.Store.InsertSwarm(gm.Swarm)
			}
		case "contribution":
			if gm.Contribution != nil {
				d.Store.InsertSwarmContribution(gm.Contribution)
			}
		case "synthesis":
			if gm.Swarm != nil {
				d.Store.SynthesizeSwarm(gm.Swarm.ID, gm.Swarm.Synthesis)
			}
		}
	}
}

// ── Task publishing helpers ──

func (d *Daemon) publishTask(ctx context.Context, t *store.Task) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	t.AuthorID = d.Node.PeerID().String()
	t.AuthorName = d.Profile.AgentName
	t.Status = "open"
	t.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	t.UpdatedAt = t.CreatedAt

	// Apply default reward if none specified
	if t.Reward <= 0 {
		t.Reward = DefaultReward
	}

	if err := d.Store.InsertTask(t); err != nil {
		return err
	}

	// Freeze reward credits
	if t.Reward > 0 {
		if err := d.Store.FreezeCredits(t.AuthorID, t.Reward); err != nil {
			return fmt.Errorf("freeze credits: %w", err)
		}
	}

	msg := GossipTaskMsg{Type: "task", Task: t}
	data, _ := json.Marshal(msg)
	return d.Node.Publish(ctx, TaskTopic, data)
}

func (d *Daemon) publishTaskBid(ctx context.Context, b *store.TaskBid) error {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	b.BidderID = d.Node.PeerID().String()
	b.BidderName = d.Profile.AgentName
	b.CreatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := d.Store.InsertTaskBid(b); err != nil {
		return err
	}

	msg := GossipTaskMsg{Type: "bid", Bid: b}
	data, _ := json.Marshal(msg)
	return d.Node.Publish(ctx, TaskTopic, data)
}

func (d *Daemon) publishTaskUpdate(ctx context.Context, t *store.Task) error {
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := d.Store.InsertTask(t); err != nil {
		return err
	}

	msg := GossipTaskMsg{Type: "update", Task: t}
	data, _ := json.Marshal(msg)
	return d.Node.Publish(ctx, TaskTopic, data)
}

// ── Swarm publishing helpers ──

func (d *Daemon) publishSwarm(ctx context.Context, sw *store.Swarm) error {
	if sw.ID == "" {
		sw.ID = uuid.New().String()
	}
	sw.CreatorID = d.Node.PeerID().String()
	sw.CreatorName = d.Profile.AgentName
	sw.Status = "open"
	sw.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	sw.UpdatedAt = sw.CreatedAt

	// Compute deadline from duration_minutes
	if sw.DurationMin > 0 && sw.Deadline == "" {
		sw.Deadline = time.Now().UTC().Add(time.Duration(sw.DurationMin) * time.Minute).Format(time.RFC3339)
	}

	if err := d.Store.InsertSwarm(sw); err != nil {
		return err
	}

	msg := GossipSwarmMsg{Type: "swarm", Swarm: sw}
	data, _ := json.Marshal(msg)
	return d.Node.Publish(ctx, SwarmTopic, data)
}

func (d *Daemon) publishSwarmContribution(ctx context.Context, c *store.SwarmContribution) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	c.AuthorID = d.Node.PeerID().String()
	c.AuthorName = d.Profile.AgentName
	c.CreatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := d.Store.InsertSwarmContribution(c); err != nil {
		return err
	}

	msg := GossipSwarmMsg{Type: "contribution", Contribution: c}
	data, _ := json.Marshal(msg)
	return d.Node.Publish(ctx, SwarmTopic, data)
}

func (d *Daemon) publishSwarmSynthesis(ctx context.Context, swarmID, synthesis string) error {
	if err := d.Store.SynthesizeSwarm(swarmID, synthesis); err != nil {
		return err
	}

	sw, err := d.Store.GetSwarm(swarmID)
	if err != nil {
		return err
	}

	msg := GossipSwarmMsg{Type: "synthesis", Swarm: sw}
	data, _ := json.Marshal(msg)
	return d.Node.Publish(ctx, SwarmTopic, data)
}

// ── Credit audit ──

// handleCreditAuditSub processes credit audit messages from peers.
// Peers log these transactions so any node can detect inconsistencies.
func (d *Daemon) handleCreditAuditSub(ctx context.Context, sub *pubsub.Subscription) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			return
		}
		if msg.ReceivedFrom == d.Node.PeerID() {
			continue
		}

		var audit CreditAuditMsg
		if err := json.Unmarshal(msg.Data, &audit); err != nil {
			continue
		}

		// Store the audit record locally for verification
		d.Store.LogCreditAudit(audit.TxnID, audit.TaskID, audit.From, audit.To, audit.Amount, audit.Reason, audit.Time)

		// If we are the recipient, credit our local account
		myID := d.Node.PeerID().String()
		if audit.To == myID && audit.Amount > 0 {
			d.Store.EnsureCreditAccount(myID, 0)
			d.Store.AddCredits(audit.TxnID+"_recv", myID, audit.Amount, "received_"+audit.Reason)
		}
	}
}

// publishCreditAudit broadcasts a credit transaction for peer supervision.
func (d *Daemon) publishCreditAudit(ctx context.Context, txnID, taskID, from, to string, amount float64, reason string) {
	audit := CreditAuditMsg{
		TxnID:  txnID,
		TaskID: taskID,
		From:   from,
		To:     to,
		Amount: amount,
		Reason: reason,
		Time:   time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(audit)
	d.Node.Publish(ctx, CreditAudit, data)
}

// ── Prediction Market gossip ──

func (d *Daemon) handlePredictionSub(ctx context.Context, sub *pubsub.Subscription) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			return
		}
		if msg.ReceivedFrom == d.Node.PeerID() {
			continue
		}

		var gm GossipPredictionMsg
		if err := json.Unmarshal(msg.Data, &gm); err != nil {
			continue
		}

		switch gm.Type {
		case "prediction":
			if gm.Prediction != nil {
				d.Store.InsertPrediction(gm.Prediction)
			}
		case "bet":
			if gm.Bet != nil {
				d.Store.InsertPredictionBet(gm.Bet)
			}
		case "resolution":
			if gm.Resolution != nil {
				d.Store.InsertPredictionResolution(gm.Resolution)
			}
		}
	}
}

func (d *Daemon) publishPrediction(ctx context.Context, p *store.Prediction) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	p.CreatorID = d.Node.PeerID().String()
	p.Status = "open"
	p.CreatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := d.Store.InsertPrediction(p); err != nil {
		return err
	}

	msg := GossipPredictionMsg{Type: "prediction", Prediction: p}
	data, _ := json.Marshal(msg)
	return d.Node.Publish(ctx, PredictionTopic, data)
}

func (d *Daemon) publishPredictionBet(ctx context.Context, b *store.PredictionBet) error {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	b.BettorID = d.Node.PeerID().String()
	b.CreatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := d.Store.InsertPredictionBet(b); err != nil {
		return err
	}

	msg := GossipPredictionMsg{Type: "bet", Bet: b}
	data, _ := json.Marshal(msg)
	return d.Node.Publish(ctx, PredictionTopic, data)
}

func (d *Daemon) publishPredictionResolution(ctx context.Context, r *store.PredictionResolution) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	r.ResolverID = d.Node.PeerID().String()
	r.CreatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := d.Store.InsertPredictionResolution(r); err != nil {
		return err
	}

	msg := GossipPredictionMsg{Type: "resolution", Resolution: r}
	data, _ := json.Marshal(msg)
	return d.Node.Publish(ctx, PredictionTopic, data)
}

// ── Agent Resume gossip ──

func (d *Daemon) handleResumeSub(ctx context.Context, sub *pubsub.Subscription) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			return
		}
		if msg.ReceivedFrom == d.Node.PeerID() {
			continue
		}

		var gm GossipResumeMsg
		if err := json.Unmarshal(msg.Data, &gm); err != nil {
			continue
		}
		if gm.Type == "resume" && gm.Resume != nil {
			d.Store.UpsertResume(gm.Resume)
		}
	}
}

// publishResume broadcasts the local agent's resume to the network.
func (d *Daemon) publishResume(ctx context.Context, r *store.AgentResume) error {
	r.PeerID = d.Node.PeerID().String()
	if r.AgentName == "" && d.Profile != nil {
		r.AgentName = d.Profile.AgentName
	}

	if err := d.Store.UpsertResume(r); err != nil {
		return err
	}

	msg := GossipResumeMsg{Type: "resume", Resume: r}
	data, _ := json.Marshal(msg)
	return d.Node.Publish(ctx, ResumeTopic, data)
}

// resumeBroadcastLoop periodically publishes own resume to the network.
func (d *Daemon) resumeBroadcastLoop(ctx context.Context) {
	// Initial delay to let peer connections establish
	select {
	case <-ctx.Done():
		return
	case <-time.After(10 * time.Second):
	}

	broadcast := func() {
		peerID := d.Node.PeerID().String()
		resume, _ := d.Store.GetResume(peerID)
		if resume == nil {
			// Auto-build resume from profile
			resume = d.buildResumeFromProfile()
		}
		if resume != nil {
			msg := GossipResumeMsg{Type: "resume", Resume: resume}
			data, _ := json.Marshal(msg)
			d.Node.Publish(ctx, ResumeTopic, data)
		}
	}

	broadcast()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			broadcast()
		}
	}
}

// buildResumeFromProfile creates a resume from the node's Profile capabilities and domains.
func (d *Daemon) buildResumeFromProfile() *store.AgentResume {
	if d.Profile == nil {
		return nil
	}
	peerID := d.Node.PeerID().String()

	// Merge domains + capabilities into skills
	skillSet := make(map[string]bool)
	for _, s := range d.Profile.Domains {
		skillSet[s] = true
	}
	for _, s := range d.Profile.Capabilities {
		skillSet[s] = true
	}
	skills := make([]string, 0, len(skillSet))
	for s := range skillSet {
		skills = append(skills, s)
	}

	skillsJSON, _ := json.Marshal(skills)
	resume := &store.AgentResume{
		PeerID:      peerID,
		AgentName:   d.Profile.AgentName,
		Skills:      string(skillsJSON),
		DataSources: "[]",
		Description: d.Profile.Bio,
	}

	// Store locally
	d.Store.UpsertResume(resume)
	return resume
}
