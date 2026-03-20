package daemon

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
	"github.com/google/uuid"
)

// GenerateTaskInsight creates a knowledge entry from a completed task.
// Called automatically after task approval to capture experience knowledge.
func (d *Daemon) GenerateTaskInsight(t *store.Task) {
	if t == nil || t.Status != "approved" {
		return
	}

	// Build structured insight body
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Task Completed: %s\n\n", t.Title))

	if t.Description != "" {
		b.WriteString(fmt.Sprintf("**Description**: %s\n\n", t.Description))
	}

	b.WriteString(fmt.Sprintf("**Reward**: %d Shell\n", t.Reward))
	b.WriteString(fmt.Sprintf("**Mode**: %s\n", t.Mode))

	// Parse tags for domains
	var tags []string
	if t.Tags != "" {
		_ = json.Unmarshal([]byte(t.Tags), &tags)
	}
	if len(tags) > 0 {
		b.WriteString(fmt.Sprintf("**Skills Used**: %s\n", strings.Join(tags, ", ")))
	}

	// Duration if we have timestamps
	if t.CreatedAt != "" && t.UpdatedAt != "" {
		if created, err1 := time.Parse(time.RFC3339, t.CreatedAt); err1 == nil {
			if updated, err2 := time.Parse(time.RFC3339, t.UpdatedAt); err2 == nil {
				dur := updated.Sub(created)
				if dur > 0 {
					b.WriteString(fmt.Sprintf("**Duration**: %s\n", formatDuration(dur)))
				}
			}
		}
	}

	if t.Result != "" {
		summary := t.Result
		if len(summary) > 500 {
			summary = summary[:500] + "…"
		}
		b.WriteString(fmt.Sprintf("\n**Result Summary**: %s\n", summary))
	}

	// Determine insight title
	title := fmt.Sprintf("Experience: %s", t.Title)
	if len(title) > 200 {
		title = title[:200]
	}

	// Determine domains from tags
	domains := tags
	if len(domains) == 0 {
		domains = []string{"general"}
	}

	entry := &store.KnowledgeEntry{
		ID:         uuid.New().String(),
		AuthorID:   d.Node.PeerID().String(),
		AuthorName: d.Profile.AgentName,
		Title:      title,
		Body:       b.String(),
		Domains:    domains,
		Type:       "task-insight",
		Source:     "local",
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	// Publish to local store + P2P network
	if err := d.publishKnowledge(d.ctx, entry); err != nil {
		fmt.Printf("task-insight: failed to publish for task %s: %v\n", t.ID, err)
		return
	}

	d.RecordEvent("knowledge_published", d.Node.PeerID().String(), entry.ID,
		fmt.Sprintf("Auto-generated task insight: %s", title))
}

// formatDuration returns a human-readable duration string.
func formatDuration(dur time.Duration) string {
	if dur < time.Minute {
		return fmt.Sprintf("%ds", int(dur.Seconds()))
	}
	if dur < time.Hour {
		return fmt.Sprintf("%dm", int(dur.Minutes()))
	}
	h := int(dur.Hours())
	m := int(dur.Minutes()) % 60
	if h < 24 {
		if m > 0 {
			return fmt.Sprintf("%dh%dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	}
	d := h / 24
	return fmt.Sprintf("%dd%dh", d, h%24)
}
