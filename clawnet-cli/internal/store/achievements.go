package store

import (
	"database/sql"
	"time"
)

// Achievement represents an unlocked achievement.
type Achievement struct {
	ID         string `json:"id"`
	PeerID     string `json:"peer_id"`
	UnlockedAt string `json:"unlocked_at"`
}

// AchievementDef defines an achievement's metadata.
type AchievementDef struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

// AchievementDefinitions lists all possible achievements.
var AchievementDefinitions = []AchievementDef{
	{ID: "first_blood", Title: "First Blood", Description: "Complete your first task", Icon: "🏅"},
	{ID: "patron", Title: "Patron", Description: "Publish your first task", Icon: "🏅"},
	{ID: "social_butterfly", Title: "Social Butterfly", Description: "Contribute to 3 Swarm Thinks", Icon: "🏅"},
	{ID: "deep_pockets", Title: "Deep Pockets", Description: "Hold 10,000 Shell", Icon: "🏅"},
	{ID: "pearl_collector", Title: "Pearl Collector", Description: "Reputation score reaches 80+", Icon: "🏅"},
	{ID: "marathon_runner", Title: "Marathon Runner", Description: "Node online for 7+ days", Icon: "🏅"},
	{ID: "wise_crab", Title: "Wise Crab", Description: "Prediction accuracy exceeds 80%", Icon: "🏅"},
	{ID: "knowledge_sharer", Title: "Knowledge Sharer", Description: "Share 5 knowledge entries", Icon: "🏅"},
	{ID: "team_player", Title: "Team Player", Description: "Complete 5 tasks for others", Icon: "🏅"},
	{ID: "networker", Title: "Networker", Description: "Connect with 10+ peers", Icon: "🏅"},
}

// UnlockAchievement marks an achievement as unlocked.
func (s *Store) UnlockAchievement(peerID, achievementID string) error {
	_, err := s.DB.Exec(`INSERT OR IGNORE INTO achievements (id, peer_id, unlocked_at) VALUES (?, ?, ?)`,
		achievementID, peerID, time.Now().UTC().Format(time.RFC3339))
	return err
}

// IsAchievementUnlocked checks if an achievement is already unlocked.
func (s *Store) IsAchievementUnlocked(peerID, achievementID string) bool {
	var count int
	s.DB.QueryRow(`SELECT COUNT(*) FROM achievements WHERE peer_id = ? AND id = ?`, peerID, achievementID).Scan(&count)
	return count > 0
}

// ListAchievements returns all unlocked achievements for a peer.
func (s *Store) ListAchievements(peerID string) ([]Achievement, error) {
	rows, err := s.DB.Query(`SELECT id, peer_id, unlocked_at FROM achievements WHERE peer_id = ? ORDER BY unlocked_at`, peerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var achievements []Achievement
	for rows.Next() {
		var a Achievement
		if err := rows.Scan(&a.ID, &a.PeerID, &a.UnlockedAt); err != nil {
			return nil, err
		}
		achievements = append(achievements, a)
	}
	return achievements, nil
}

// AchievementCount returns number of unlocked achievements.
func (s *Store) AchievementCount(peerID string) int {
	var count int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM achievements WHERE peer_id = ?`, peerID).Scan(&count)
	if err != nil && err != sql.ErrNoRows {
		return 0
	}
	return count
}

// ── Achievement check queries ──

// CountCompletedTasks returns tasks where this peer was the worker and status = approved/settled.
func (s *Store) CountCompletedTasks(peerID string) int {
	var count int
	s.DB.QueryRow(`SELECT COUNT(*) FROM tasks WHERE assigned_to = ? AND status IN ('approved', 'settled')`, peerID).Scan(&count)
	return count
}

// CountPublishedTasks returns tasks authored by this peer.
func (s *Store) CountPublishedTasks(peerID string) int {
	var count int
	s.DB.QueryRow(`SELECT COUNT(*) FROM tasks WHERE author_id = ?`, peerID).Scan(&count)
	return count
}

// CountSwarmContributions returns swarm contributions by this peer.
func (s *Store) CountSwarmContributions(peerID string) int {
	var count int
	s.DB.QueryRow(`SELECT COUNT(*) FROM swarm_contributions WHERE author_id = ?`, peerID).Scan(&count)
	return count
}

// CountKnowledgeEntries returns knowledge entries authored by this peer.
func (s *Store) CountKnowledgeEntries(peerID string) int {
	var count int
	s.DB.QueryRow(`SELECT COUNT(*) FROM knowledge WHERE author_id = ?`, peerID).Scan(&count)
	return count
}
