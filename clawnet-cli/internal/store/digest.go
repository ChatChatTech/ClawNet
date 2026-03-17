package store

import (
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// Digest contains a snapshot of network activity for display.
type Digest struct {
	GeneratedAt    string         `json:"generated_at"`
	PeerCount      int            `json:"peer_count"`
	TasksCreated   int            `json:"tasks_created_24h"`
	TasksCompleted int            `json:"tasks_completed_24h"`
	ShellBurned    int64          `json:"shell_burned_24h"`
	KnowledgeCount int            `json:"knowledge_published_24h"`
	TopContributor *DigestActor   `json:"top_contributor,omitempty"`
	RecentEvents   int            `json:"recent_events"`
	Achievements   int            `json:"achievements_unlocked_24h"`
}

// DigestActor is a simplified peer info for digest display.
type DigestActor struct {
	PeerID string `json:"peer_id"`
	Count  int    `json:"actions"`
}

// GenerateDigest produces a 24-hour activity summary.
func (s *Store) GenerateDigest(selfPeerID string, connectedPeers []peer.ID) Digest {
	now := time.Now().UTC()
	since := now.Add(-24 * time.Hour).Format(time.RFC3339)

	d := Digest{
		GeneratedAt: now.Format(time.RFC3339),
		PeerCount:   len(connectedPeers) + 1, // +1 for self
	}

	// Count tasks created in last 24h
	s.DB.QueryRow(`SELECT COUNT(*) FROM tasks WHERE created_at >= ?`, since).Scan(&d.TasksCreated)

	// Count tasks completed (approved) in last 24h
	s.DB.QueryRow(`SELECT COUNT(*) FROM tasks WHERE status IN ('approved','settled') AND updated_at >= ?`, since).Scan(&d.TasksCompleted)

	// Shell burned (fees) in last 24h
	var burned float64
	s.DB.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM credit_transactions WHERE reason = 'fee_burn' AND created_at >= ?`, since).Scan(&burned)
	d.ShellBurned = int64(burned)

	// Knowledge entries published in last 24h
	s.DB.QueryRow(`SELECT COUNT(*) FROM knowledge WHERE created_at >= ?`, since).Scan(&d.KnowledgeCount)

	// Events in last 24h
	s.DB.QueryRow(`SELECT COUNT(*) FROM events WHERE created_at >= ?`, since).Scan(&d.RecentEvents)

	// Achievements unlocked in last 24h
	s.DB.QueryRow(`SELECT COUNT(*) FROM achievements WHERE unlocked_at >= ?`, since).Scan(&d.Achievements)

	// Top contributor (most events as actor in 24h)
	var topPeer string
	var topCount int
	err := s.DB.QueryRow(`SELECT actor, COUNT(*) as cnt FROM events WHERE created_at >= ? AND actor != '' GROUP BY actor ORDER BY cnt DESC LIMIT 1`, since).Scan(&topPeer, &topCount)
	if err == nil && topPeer != "" {
		short := topPeer
		if len(short) > 16 {
			short = short[:16]
		}
		d.TopContributor = &DigestActor{PeerID: short, Count: topCount}
	}

	return d
}

// DigestSummary returns a human-readable one-line summary for CLI display.
func (d Digest) Summary() string {
	s := fmt.Sprintf("%d peers | %d tasks | %d knowledge | %d events",
		d.PeerCount, d.TasksCompleted, d.KnowledgeCount, d.RecentEvents)
	if d.ShellBurned > 0 {
		s += fmt.Sprintf(" | %d Shell burned", d.ShellBurned)
	}
	return s
}
