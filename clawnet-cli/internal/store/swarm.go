package store

import "database/sql"

// Swarm represents a Swarm Think session.
type Swarm struct {
	ID              string `json:"id"`
	CreatorID       string `json:"creator_id"`
	CreatorName     string `json:"creator_name"`
	Title           string `json:"title"`
	Question        string `json:"question"`
	Domains         string `json:"domains"`          // JSON array of domain tags
	MaxParticipants int    `json:"max_participants"` // 0 = unlimited
	DurationMin     int    `json:"duration_minutes"` // 0 = no time limit
	Deadline        string `json:"deadline"`         // RFC3339, set from duration at creation
	Status          string `json:"status"`           // open, synthesizing, closed
	Synthesis       string `json:"synthesis"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// SwarmContribution is a single contribution to a Swarm Think session.
type SwarmContribution struct {
	ID          string `json:"id"`
	SwarmID     string `json:"swarm_id"`
	AuthorID    string `json:"author_id"`
	AuthorName  string `json:"author_name"`
	Perspective string `json:"perspective"` // bull, bear, neutral, devil-advocate
	Body        string `json:"body"`
	Confidence  float64 `json:"confidence"` // 0.0 - 1.0
	CreatedAt   string `json:"created_at"`
}

// InsertSwarm upserts a swarm session.
func (s *Store) InsertSwarm(sw *Swarm) error {
	_, err := s.DB.Exec(
		`INSERT INTO swarms (id, creator_id, creator_name, title, question, domains, max_participants, duration_min, deadline, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   title = excluded.title, question = excluded.question,
		   status = excluded.status, deadline = excluded.deadline,
		   updated_at = datetime('now')`,
		sw.ID, sw.CreatorID, sw.CreatorName, sw.Title, sw.Question,
		sw.Domains, sw.MaxParticipants, sw.DurationMin, sw.Deadline, sw.Status,
	)
	return err
}

// GetSwarm returns a swarm by ID.
func (s *Store) GetSwarm(id string) (*Swarm, error) {
	row := s.DB.QueryRow(
		`SELECT id, creator_id, creator_name, title, question,
		        COALESCE(domains,'[]'), COALESCE(max_participants,0),
		        COALESCE(duration_min,0), COALESCE(deadline,''),
		        status, synthesis, created_at, updated_at
		 FROM swarms WHERE id = ?`, id,
	)
	sw := &Swarm{}
	err := row.Scan(&sw.ID, &sw.CreatorID, &sw.CreatorName, &sw.Title, &sw.Question,
		&sw.Domains, &sw.MaxParticipants, &sw.DurationMin, &sw.Deadline,
		&sw.Status, &sw.Synthesis, &sw.CreatedAt, &sw.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return sw, err
}

// ListSwarms returns swarm sessions with optional status filter.
func (s *Store) ListSwarms(status string, limit, offset int) ([]*Swarm, error) {
	var rows *sql.Rows
	var err error
	q := `SELECT id, creator_id, creator_name, title, question,
	             COALESCE(domains,'[]'), COALESCE(max_participants,0),
	             COALESCE(duration_min,0), COALESCE(deadline,''),
	             status, synthesis, created_at, updated_at
	      FROM swarms`
	if status != "" {
		rows, err = s.DB.Query(q+" WHERE status = ? ORDER BY created_at DESC LIMIT ? OFFSET ?",
			status, limit, offset)
	} else {
		rows, err = s.DB.Query(q+" ORDER BY created_at DESC LIMIT ? OFFSET ?",
			limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var swarms []*Swarm
	for rows.Next() {
		sw := &Swarm{}
		if err := rows.Scan(&sw.ID, &sw.CreatorID, &sw.CreatorName, &sw.Title, &sw.Question,
			&sw.Domains, &sw.MaxParticipants, &sw.DurationMin, &sw.Deadline,
			&sw.Status, &sw.Synthesis, &sw.CreatedAt, &sw.UpdatedAt); err != nil {
			return nil, err
		}
		swarms = append(swarms, sw)
	}
	return swarms, rows.Err()
}

// InsertSwarmContribution inserts a contribution.
func (s *Store) InsertSwarmContribution(c *SwarmContribution) error {
	_, err := s.DB.Exec(
		`INSERT INTO swarm_contributions (id, swarm_id, author_id, author_name, perspective, body, confidence)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO NOTHING`,
		c.ID, c.SwarmID, c.AuthorID, c.AuthorName, c.Perspective, c.Body, c.Confidence,
	)
	return err
}

// ListSwarmContributions returns all contributions for a swarm.
func (s *Store) ListSwarmContributions(swarmID string, limit int) ([]*SwarmContribution, error) {
	rows, err := s.DB.Query(
		`SELECT id, swarm_id, author_id, author_name,
		        COALESCE(perspective,''), body, COALESCE(confidence,0), created_at
		 FROM swarm_contributions WHERE swarm_id = ?
		 ORDER BY created_at ASC LIMIT ?`,
		swarmID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contribs []*SwarmContribution
	for rows.Next() {
		c := &SwarmContribution{}
		if err := rows.Scan(&c.ID, &c.SwarmID, &c.AuthorID, &c.AuthorName,
			&c.Perspective, &c.Body, &c.Confidence, &c.CreatedAt); err != nil {
			return nil, err
		}
		contribs = append(contribs, c)
	}
	return contribs, rows.Err()
}

// SynthesizeSwarm stores the synthesis result and closes the swarm.
func (s *Store) SynthesizeSwarm(swarmID, synthesis string) error {
	_, err := s.DB.Exec(
		`UPDATE swarms SET status = 'closed', synthesis = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		synthesis, swarmID,
	)
	return err
}

// CloseExpiredSwarms closes swarms whose deadline has passed.
// Returns the IDs of swarms that were closed.
func (s *Store) CloseExpiredSwarms() ([]string, error) {
	rows, err := s.DB.Query(
		`SELECT id FROM swarms
		 WHERE status = 'open' AND deadline != '' AND deadline < datetime('now')`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, id := range ids {
		s.DB.Exec(`UPDATE swarms SET status = 'closed', updated_at = datetime('now') WHERE id = ?`, id)
	}
	return ids, nil
}
