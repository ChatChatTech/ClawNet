package store

import "database/sql"

// Swarm represents a Swarm Think session.
type Swarm struct {
	ID          string `json:"id"`
	CreatorID   string `json:"creator_id"`
	CreatorName string `json:"creator_name"`
	Title       string `json:"title"`
	Question    string `json:"question"`
	Status      string `json:"status"` // open, synthesizing, closed
	Synthesis   string `json:"synthesis"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// SwarmContribution is a single contribution to a Swarm Think session.
type SwarmContribution struct {
	ID          string `json:"id"`
	SwarmID     string `json:"swarm_id"`
	AuthorID    string `json:"author_id"`
	AuthorName  string `json:"author_name"`
	Body        string `json:"body"`
	CreatedAt   string `json:"created_at"`
}

// InsertSwarm upserts a swarm session.
func (s *Store) InsertSwarm(sw *Swarm) error {
	_, err := s.DB.Exec(
		`INSERT INTO swarms (id, creator_id, creator_name, title, question, status)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   title = excluded.title, question = excluded.question,
		   status = excluded.status, updated_at = datetime('now')`,
		sw.ID, sw.CreatorID, sw.CreatorName, sw.Title, sw.Question, sw.Status,
	)
	return err
}

// GetSwarm returns a swarm by ID.
func (s *Store) GetSwarm(id string) (*Swarm, error) {
	row := s.DB.QueryRow(
		`SELECT id, creator_id, creator_name, title, question, status, synthesis, created_at, updated_at
		 FROM swarms WHERE id = ?`, id,
	)
	sw := &Swarm{}
	err := row.Scan(&sw.ID, &sw.CreatorID, &sw.CreatorName, &sw.Title, &sw.Question,
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
	if status != "" {
		rows, err = s.DB.Query(
			`SELECT id, creator_id, creator_name, title, question, status, synthesis, created_at, updated_at
			 FROM swarms WHERE status = ?
			 ORDER BY created_at DESC LIMIT ? OFFSET ?`,
			status, limit, offset,
		)
	} else {
		rows, err = s.DB.Query(
			`SELECT id, creator_id, creator_name, title, question, status, synthesis, created_at, updated_at
			 FROM swarms ORDER BY created_at DESC LIMIT ? OFFSET ?`,
			limit, offset,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var swarms []*Swarm
	for rows.Next() {
		sw := &Swarm{}
		if err := rows.Scan(&sw.ID, &sw.CreatorID, &sw.CreatorName, &sw.Title, &sw.Question,
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
		`INSERT INTO swarm_contributions (id, swarm_id, author_id, author_name, body)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO NOTHING`,
		c.ID, c.SwarmID, c.AuthorID, c.AuthorName, c.Body,
	)
	return err
}

// ListSwarmContributions returns all contributions for a swarm.
func (s *Store) ListSwarmContributions(swarmID string, limit int) ([]*SwarmContribution, error) {
	rows, err := s.DB.Query(
		`SELECT id, swarm_id, author_id, author_name, body, created_at
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
		if err := rows.Scan(&c.ID, &c.SwarmID, &c.AuthorID, &c.AuthorName, &c.Body, &c.CreatedAt); err != nil {
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
