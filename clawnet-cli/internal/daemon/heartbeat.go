package daemon

import (
	"context"
	"fmt"
	"time"
)

// heartbeatState tracks the last-seen counters across heartbeat ticks.
type heartbeatState struct {
	UnreadDM     int    `json:"unread_dm"`
	KnowledgeTS  string `json:"knowledge_latest"`
	OpenTasks    int    `json:"open_tasks"`
	NewDM        int    `json:"new_dm"`
	NewKnowledge int    `json:"new_knowledge"`
	NewTasks     int    `json:"new_tasks"`
	CheckedAt    string `json:"checked_at"`
}

// startHeartbeat runs periodic checks for new DMs, knowledge, and tasks.
func (d *Daemon) startHeartbeat(ctx context.Context) {
	d.hbState = &heartbeatState{}
	go d.heartbeatLoop(ctx)
}

func (d *Daemon) heartbeatLoop(ctx context.Context) {
	// Initial delay to let the node connect and sync
	select {
	case <-ctx.Done():
		return
	case <-time.After(10 * time.Second):
	}

	// Seed initial state without alerting
	d.doHeartbeat(true)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.doHeartbeat(false)
		}
	}
}

func (d *Daemon) doHeartbeat(silent bool) {
	now := time.Now().UTC().Format(time.RFC3339)

	// Check unread DMs
	unread, _ := d.Store.UnreadDMCount()
	newDM := unread - d.hbState.UnreadDM
	if newDM < 0 {
		newDM = 0
	}

	// Check new knowledge since last check
	var newKnowledge int
	if d.hbState.KnowledgeTS != "" {
		entries, err := d.Store.ListKnowledgeSince(d.hbState.KnowledgeTS, 100)
		if err == nil {
			newKnowledge = len(entries)
		}
	}
	latestKT := d.Store.LatestKnowledgeTime()

	// Check open tasks
	openTasks, _ := d.Store.ListTasks("open", 100, 0)
	newTasks := len(openTasks) - d.hbState.OpenTasks
	if newTasks < 0 {
		newTasks = 0
	}

	// Update state
	d.hbState.UnreadDM = unread
	d.hbState.KnowledgeTS = latestKT
	d.hbState.OpenTasks = len(openTasks)
	d.hbState.NewDM = newDM
	d.hbState.NewKnowledge = newKnowledge
	d.hbState.NewTasks = newTasks
	d.hbState.CheckedAt = now

	// Log only if there's new activity and not in silent (initial) mode
	if !silent && (newDM > 0 || newKnowledge > 0 || newTasks > 0) {
		fmt.Printf("heartbeat: +%d DM, +%d knowledge, +%d tasks\n", newDM, newKnowledge, newTasks)
	}
}
