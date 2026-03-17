package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
)

// startOfflineSyncLoop periodically retries pending operations that were queued
// while the node was offline or the network was unavailable.
func (d *Daemon) startOfflineSyncLoop(ctx context.Context) {
	// Prune old completed/failed ops on startup
	d.Store.PruneOldPendingOps()

	go func() {
		// Initial delay to let connections establish
		select {
		case <-time.After(15 * time.Second):
		case <-ctx.Done():
			return
		}
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.processPendingOps(ctx)
			}
		}
	}()
}

// processPendingOps drains the pending_ops queue, retrying each operation.
func (d *Daemon) processPendingOps(ctx context.Context) {
	// Only attempt retries when we have peers
	if len(d.Node.ConnectedPeers()) == 0 {
		return
	}

	ops, err := d.Store.ListPendingOps()
	if err != nil || len(ops) == 0 {
		return
	}

	for _, op := range ops {
		if ctx.Err() != nil {
			return
		}
		err := d.replayOp(ctx, op)
		if err != nil {
			d.Store.MarkPendingOpFailed(op.ID, err.Error())
		} else {
			d.Store.MarkPendingOpSent(op.ID)
		}
	}

	// Prune completed
	d.Store.PruneOldPendingOps()
}

// replayOp executes a pending operation based on its type.
func (d *Daemon) replayOp(ctx context.Context, op *store.PendingOp) error {
	switch op.Type {
	case "knowledge":
		var entry store.KnowledgeEntry
		if err := json.Unmarshal([]byte(op.Payload), &entry); err != nil {
			return fmt.Errorf("unmarshal knowledge: %w", err)
		}
		return d.publishKnowledge(ctx, &entry)

	case "dm":
		var msg struct {
			PeerID string `json:"peer_id"`
			Body   string `json:"body"`
		}
		if err := json.Unmarshal([]byte(op.Payload), &msg); err != nil {
			return fmt.Errorf("unmarshal dm: %w", err)
		}
		return d.sendDM(ctx, msg.PeerID, msg.Body)

	case "topic_message":
		var msg struct {
			Topic string `json:"topic"`
			Body  string `json:"body"`
		}
		if err := json.Unmarshal([]byte(op.Payload), &msg); err != nil {
			return fmt.Errorf("unmarshal topic_message: %w", err)
		}
		return d.publishTopicMessage(ctx, msg.Topic, msg.Body)

	default:
		return fmt.Errorf("unknown op type: %s", op.Type)
	}
}
