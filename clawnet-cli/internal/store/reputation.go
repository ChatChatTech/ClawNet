package store

import "database/sql"

// ReputationRecord holds a peer's computed reputation.
type ReputationRecord struct {
	PeerID         string  `json:"peer_id"`
	Score          float64 `json:"score"`
	TasksCompleted int     `json:"tasks_completed"`
	TasksFailed    int     `json:"tasks_failed"`
	Contributions  int     `json:"contributions"`
	KnowledgeCount int     `json:"knowledge_count"`
	UpdatedAt      string  `json:"updated_at"`
}

// UpsertReputation inserts or updates a reputation record.
func (s *Store) UpsertReputation(r *ReputationRecord) error {
	_, err := s.DB.Exec(
		`INSERT INTO reputation (peer_id, score, tasks_completed, tasks_failed, contributions, knowledge_count)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(peer_id) DO UPDATE SET
		   score = excluded.score,
		   tasks_completed = excluded.tasks_completed,
		   tasks_failed = excluded.tasks_failed,
		   contributions = excluded.contributions,
		   knowledge_count = excluded.knowledge_count,
		   updated_at = datetime('now')`,
		r.PeerID, r.Score, r.TasksCompleted, r.TasksFailed, r.Contributions, r.KnowledgeCount,
	)
	return err
}

// GetReputation returns the reputation for a peer.
func (s *Store) GetReputation(peerID string) (*ReputationRecord, error) {
	row := s.DB.QueryRow(
		`SELECT peer_id, score, tasks_completed, tasks_failed, contributions, knowledge_count, updated_at
		 FROM reputation WHERE peer_id = ?`, peerID,
	)
	r := &ReputationRecord{}
	err := row.Scan(&r.PeerID, &r.Score, &r.TasksCompleted, &r.TasksFailed,
		&r.Contributions, &r.KnowledgeCount, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return &ReputationRecord{PeerID: peerID, Score: 50.0}, nil
	}
	return r, err
}

// ListReputation returns all reputation records sorted by score desc.
func (s *Store) ListReputation(limit int) ([]*ReputationRecord, error) {
	rows, err := s.DB.Query(
		`SELECT peer_id, score, tasks_completed, tasks_failed, contributions, knowledge_count, updated_at
		 FROM reputation ORDER BY score DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []*ReputationRecord
	for rows.Next() {
		r := &ReputationRecord{}
		if err := rows.Scan(&r.PeerID, &r.Score, &r.TasksCompleted, &r.TasksFailed,
			&r.Contributions, &r.KnowledgeCount, &r.UpdatedAt); err != nil {
			return nil, err
		}
		recs = append(recs, r)
	}
	return recs, rows.Err()
}

// RecalcReputation recalculates a peer's reputation from their activity.
func (s *Store) RecalcReputation(peerID string) (*ReputationRecord, error) {
	var tasksCompleted, tasksFailed, contributions, knowledgeCount int

	s.DB.QueryRow(`SELECT COUNT(*) FROM tasks WHERE assigned_to = ? AND status = 'approved'`, peerID).Scan(&tasksCompleted)
	s.DB.QueryRow(`SELECT COUNT(*) FROM tasks WHERE assigned_to = ? AND status = 'rejected'`, peerID).Scan(&tasksFailed)
	s.DB.QueryRow(`SELECT COUNT(*) FROM swarm_contributions WHERE author_id = ?`, peerID).Scan(&contributions)
	s.DB.QueryRow(`SELECT COUNT(*) FROM knowledge WHERE author_id = ?`, peerID).Scan(&knowledgeCount)

	// Simple scoring: base 50, +5 per completed task, -3 per failed, +2 per contribution, +1 per knowledge
	score := 50.0 + float64(tasksCompleted)*5.0 - float64(tasksFailed)*3.0 + float64(contributions)*2.0 + float64(knowledgeCount)*1.0
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	rec := &ReputationRecord{
		PeerID:         peerID,
		Score:          score,
		TasksCompleted: tasksCompleted,
		TasksFailed:    tasksFailed,
		Contributions:  contributions,
		KnowledgeCount: knowledgeCount,
	}
	if err := s.UpsertReputation(rec); err != nil {
		return nil, err
	}
	return rec, nil
}
