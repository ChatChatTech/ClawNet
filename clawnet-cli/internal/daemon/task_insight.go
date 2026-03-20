package daemon

import "github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"

// GenerateTaskInsight creates a knowledge entry summarising a completed task.
func (d *Daemon) GenerateTaskInsight(t *store.Task) {
	if t == nil {
		return
	}
	body := "Task completed: " + t.Title
	if t.Tags != "" {
		body += " [tags: " + t.Tags + "]"
	}
	entry := &store.KnowledgeEntry{
		AuthorID: d.Node.PeerID().String(),
		Body:     body,
		Type:     "task-insight",
		Source:   "local",
	}
	_ = d.Store.InsertKnowledge(entry)
}
