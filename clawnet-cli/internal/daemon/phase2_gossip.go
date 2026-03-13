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
	TaskTopic      = "/clawnet/tasks"
	SwarmTopic     = "/clawnet/swarm"
	CreditAudit    = "/clawnet/credit-audit"
	DefaultReward  = 10.0 // default credit reward when no explicit reward is set
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
