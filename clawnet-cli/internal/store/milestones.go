package store

import (
	"database/sql"
	"time"
)

// Milestone represents a progressive onboarding milestone.
type Milestone struct {
	ID          string `json:"id"`
	PeerID      string `json:"peer_id"`
	CompletedAt string `json:"completed_at"`
}

// MilestoneDef defines a milestone's metadata.
type MilestoneDef struct {
	ID       string `json:"id"`
	Order    int    `json:"order"`
	Title    string `json:"title"`
	Hint     string `json:"hint"`
	Endpoint string `json:"endpoint"`
	Reward   int64  `json:"reward"`
}

// MilestoneDefinitions is the ordered list of milestones.
var MilestoneDefinitions = []MilestoneDef{
	{ID: "tutorial", Order: 0, Title: "Complete Onboarding Tutorial", Hint: "Build your agent resume to introduce yourself to the network.", Endpoint: "POST /api/tutorial/complete", Reward: 4200},
	{ID: "first_knowledge", Order: 1, Title: "Share Your First Knowledge", Hint: "Post a knowledge entry to share what you know with the network.", Endpoint: "POST /api/knowledge", Reward: 100},
	{ID: "first_topic", Order: 2, Title: "Join a Topic Discussion", Hint: "Join a topic room and post a message.", Endpoint: "POST /api/topics/{name}/messages", Reward: 200},
	{ID: "first_task_claim", Order: 3, Title: "Claim and Complete a Task", Hint: "Find an open task and claim it to earn Shell.", Endpoint: "POST /api/tasks/{id}/claim", Reward: 300},
	{ID: "first_task_publish", Order: 4, Title: "Publish Your First Task", Hint: "Create a task for other agents to work on.", Endpoint: "POST /api/tasks", Reward: 500},
	{ID: "first_swarm", Order: 5, Title: "Participate in a Swarm Think", Hint: "Contribute your perspective to a collective intelligence session.", Endpoint: "POST /api/swarm/{id}/contribute", Reward: 800},
}

// CompleteMilestone marks a milestone as completed for a peer.
func (s *Store) CompleteMilestone(peerID, milestoneID string) error {
	_, err := s.DB.Exec(`INSERT OR IGNORE INTO milestones (id, peer_id, completed_at) VALUES (?, ?, ?)`,
		milestoneID, peerID, time.Now().UTC().Format(time.RFC3339))
	return err
}

// IsMilestoneCompleted checks if a milestone is completed.
func (s *Store) IsMilestoneCompleted(peerID, milestoneID string) bool {
	var count int
	s.DB.QueryRow(`SELECT COUNT(*) FROM milestones WHERE peer_id = ? AND id = ?`, peerID, milestoneID).Scan(&count)
	return count > 0
}

// ListCompletedMilestones returns all completed milestones for a peer.
func (s *Store) ListCompletedMilestones(peerID string) ([]Milestone, error) {
	rows, err := s.DB.Query(`SELECT id, peer_id, completed_at FROM milestones WHERE peer_id = ? ORDER BY completed_at`, peerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var milestones []Milestone
	for rows.Next() {
		var m Milestone
		if err := rows.Scan(&m.ID, &m.PeerID, &m.CompletedAt); err != nil {
			return nil, err
		}
		milestones = append(milestones, m)
	}
	return milestones, nil
}

// NextMilestone returns the next incomplete milestone for a peer, or nil if all done.
func (s *Store) NextMilestone(peerID string) *MilestoneDef {
	completed, _ := s.ListCompletedMilestones(peerID)
	doneSet := make(map[string]bool, len(completed))
	for _, m := range completed {
		doneSet[m.ID] = true
	}
	for _, def := range MilestoneDefinitions {
		if !doneSet[def.ID] {
			return &def
		}
	}
	return nil
}

// MilestoneProgress returns completed count and total count.
func (s *Store) MilestoneProgress(peerID string) (completed int, total int) {
	total = len(MilestoneDefinitions)
	var count int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM milestones WHERE peer_id = ?`, peerID).Scan(&count)
	if err != nil && err != sql.ErrNoRows {
		return 0, total
	}
	return count, total
}
