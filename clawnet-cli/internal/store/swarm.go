package store

import "database/sql"

// Swarm represents a Swarm Think session.
type Swarm struct {
	ID              string `json:"id"`
	CreatorID       string `json:"creator_id"`
	CreatorName     string `json:"creator_name"`
	Title           string `json:"title"`
	Question        string `json:"question"`
	TemplateType    string `json:"template_type"`    // freeform, investment-analysis, tech-selection
	Domains         string `json:"domains"`          // JSON array of domain tags
	MaxParticipants int    `json:"max_participants"` // 0 = unlimited
	DurationMin     int    `json:"duration_minutes"` // 0 = no time limit
	Deadline        string `json:"deadline"`         // RFC3339, set from duration at creation
	Status          string `json:"status"`           // open, synthesizing, closed
	Synthesis       string `json:"synthesis"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
	ContribCount    int    `json:"contrib_count"`    // number of contributions
	LastActivity    string `json:"last_activity"`    // most recent activity timestamp
}

// SwarmContribution is a single contribution to a Swarm Think session.
type SwarmContribution struct {
	ID          string  `json:"id"`
	SwarmID     string  `json:"swarm_id"`
	AuthorID    string  `json:"author_id"`
	AuthorName  string  `json:"author_name"`
	Section     string  `json:"section"`     // template section key (e.g. "fundamentals")
	Perspective string  `json:"perspective"` // bull, bear, neutral, devil-advocate
	Body        string  `json:"body"`
	Confidence  float64 `json:"confidence"` // 0.0 - 1.0
	CreatedAt   string  `json:"created_at"`
}

// ── Swarm Templates ──

// SwarmTemplateSection defines a structured section within a template.
type SwarmTemplateSection struct {
	Key         string `json:"key"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// SwarmTemplate is a predefined structure for Swarm Think sessions.
type SwarmTemplate struct {
	Type                string                 `json:"type"`
	Name                string                 `json:"name"`
	Description         string                 `json:"description"`
	DefaultDomains      []string               `json:"default_domains"`
	DefaultDuration     int                    `json:"default_duration_minutes"`
	Sections            []SwarmTemplateSection  `json:"sections"`
	Perspectives        []string               `json:"perspectives"`
}

// SwarmTemplates is the built-in template registry.
var SwarmTemplates = []SwarmTemplate{
	{
		Type:            "investment-analysis",
		Name:            "Investment Analysis",
		Description:     "Multi-angle due diligence for investment decisions",
		DefaultDomains:  []string{"finance", "investment"},
		DefaultDuration: 30,
		Sections: []SwarmTemplateSection{
			{Key: "fundamentals", Title: "Fundamentals", Description: "Revenue, margins, cash flow, balance sheet analysis"},
			{Key: "technicals", Title: "Technical Analysis", Description: "Price action, volume, momentum indicators, chart patterns"},
			{Key: "market", Title: "Market & Competition", Description: "TAM/SAM, competitive landscape, moat assessment"},
			{Key: "risks", Title: "Risk Factors", Description: "Regulatory, macro, execution, concentration risks"},
			{Key: "catalysts", Title: "Catalysts & Thesis", Description: "Upcoming events, growth drivers, bull/bear thesis"},
		},
		Perspectives: []string{"bull", "bear", "neutral"},
	},
	{
		Type:            "tech-selection",
		Name:            "Technology Selection",
		Description:     "Structured evaluation for choosing between technology options",
		DefaultDomains:  []string{"technology", "engineering"},
		DefaultDuration: 45,
		Sections: []SwarmTemplateSection{
			{Key: "requirements", Title: "Requirements Fit", Description: "How well each option meets functional and non-functional requirements"},
			{Key: "scalability", Title: "Scalability & Performance", Description: "Benchmarks, load handling, horizontal/vertical scaling"},
			{Key: "ecosystem", Title: "Ecosystem & Community", Description: "Documentation quality, community size, package availability, hiring pool"},
			{Key: "cost", Title: "Cost Analysis", Description: "Licensing, infrastructure, development time, maintenance burden"},
			{Key: "risks", Title: "Risks & Migration", Description: "Vendor lock-in, deprecation risk, migration complexity, learning curve"},
		},
		Perspectives: []string{"neutral", "devil-advocate"},
	},
}

// GetSwarmTemplate returns a template by type, or nil if not found.
func GetSwarmTemplate(typ string) *SwarmTemplate {
	for i := range SwarmTemplates {
		if SwarmTemplates[i].Type == typ {
			return &SwarmTemplates[i]
		}
	}
	return nil
}

// InsertSwarm upserts a swarm session.
func (s *Store) InsertSwarm(sw *Swarm) error {
	_, err := s.DB.Exec(
		`INSERT INTO swarms (id, creator_id, creator_name, title, question, template_type, domains, max_participants, duration_min, deadline, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   title = excluded.title, question = excluded.question,
		   template_type = excluded.template_type,
		   status = excluded.status, deadline = excluded.deadline,
		   updated_at = datetime('now')`,
		sw.ID, sw.CreatorID, sw.CreatorName, sw.Title, sw.Question,
		sw.TemplateType, sw.Domains, sw.MaxParticipants, sw.DurationMin, sw.Deadline, sw.Status,
	)
	return err
}

// GetSwarm returns a swarm by ID.
func (s *Store) GetSwarm(id string) (*Swarm, error) {
	row := s.DB.QueryRow(
		`SELECT id, creator_id, creator_name, title, question,
		        COALESCE(template_type,'freeform'),
		        COALESCE(domains,'[]'), COALESCE(max_participants,0),
		        COALESCE(duration_min,0), COALESCE(deadline,''),
		        status, synthesis, created_at, updated_at
		 FROM swarms WHERE id = ?`, id,
	)
	sw := &Swarm{}
	err := row.Scan(&sw.ID, &sw.CreatorID, &sw.CreatorName, &sw.Title, &sw.Question,
		&sw.TemplateType, &sw.Domains, &sw.MaxParticipants, &sw.DurationMin, &sw.Deadline,
		&sw.Status, &sw.Synthesis, &sw.CreatedAt, &sw.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return sw, err
}

// ListSwarms returns swarm sessions with optional status filter.
// Sorted by most recent activity (latest contribution or update) first,
// then by contribution count descending.
func (s *Store) ListSwarms(status string, limit, offset int) ([]*Swarm, error) {
	var rows *sql.Rows
	var err error
	q := `SELECT s.id, s.creator_id, s.creator_name, s.title, s.question,
	             COALESCE(s.template_type,'freeform'),
	             COALESCE(s.domains,'[]'), COALESCE(s.max_participants,0),
	             COALESCE(s.duration_min,0), COALESCE(s.deadline,''),
	             s.status, s.synthesis, s.created_at, s.updated_at,
	             COUNT(c.id) AS contrib_count,
	             COALESCE(MAX(c.created_at), s.updated_at) AS last_activity
	      FROM swarms s
	      LEFT JOIN swarm_contributions c ON c.swarm_id = s.id`
	if status != "" {
		rows, err = s.DB.Query(q+" WHERE s.status = ? GROUP BY s.id ORDER BY last_activity DESC, contrib_count DESC LIMIT ? OFFSET ?",
			status, limit, offset)
	} else {
		rows, err = s.DB.Query(q+" GROUP BY s.id ORDER BY last_activity DESC, contrib_count DESC LIMIT ? OFFSET ?",
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
			&sw.TemplateType, &sw.Domains, &sw.MaxParticipants, &sw.DurationMin, &sw.Deadline,
			&sw.Status, &sw.Synthesis, &sw.CreatedAt, &sw.UpdatedAt,
			&sw.ContribCount, &sw.LastActivity); err != nil {
			return nil, err
		}
		swarms = append(swarms, sw)
	}
	return swarms, rows.Err()
}

// SearchSwarms searches swarms by keyword in title and contribution body.
func (s *Store) SearchSwarms(query string, limit, offset int) ([]*Swarm, error) {
	like := "%" + likeSanitize(query) + "%"
	rows, err := s.DB.Query(
		`SELECT s.id, s.creator_id, s.creator_name, s.title, s.question,
		        COALESCE(s.template_type,'freeform'),
		        COALESCE(s.domains,'[]'), COALESCE(s.max_participants,0),
		        COALESCE(s.duration_min,0), COALESCE(s.deadline,''),
		        s.status, s.synthesis, s.created_at, s.updated_at,
		        COUNT(c.id) AS contrib_count,
		        COALESCE(MAX(c.created_at), s.updated_at) AS last_activity
		 FROM swarms s
		 LEFT JOIN swarm_contributions c ON c.swarm_id = s.id
		 WHERE s.id IN (
		     SELECT DISTINCT s2.id FROM swarms s2
		     LEFT JOIN swarm_contributions c2 ON c2.swarm_id = s2.id
		     WHERE s2.title LIKE ? OR s2.question LIKE ? OR c2.body LIKE ?
		 )
		 GROUP BY s.id
		 ORDER BY last_activity DESC, contrib_count DESC
		 LIMIT ? OFFSET ?`,
		like, like, like, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var swarms []*Swarm
	for rows.Next() {
		sw := &Swarm{}
		if err := rows.Scan(&sw.ID, &sw.CreatorID, &sw.CreatorName, &sw.Title, &sw.Question,
			&sw.TemplateType, &sw.Domains, &sw.MaxParticipants, &sw.DurationMin, &sw.Deadline,
			&sw.Status, &sw.Synthesis, &sw.CreatedAt, &sw.UpdatedAt,
			&sw.ContribCount, &sw.LastActivity); err != nil {
			return nil, err
		}
		swarms = append(swarms, sw)
	}
	return swarms, rows.Err()
}

// InsertSwarmContribution inserts a contribution.
func (s *Store) InsertSwarmContribution(c *SwarmContribution) error {
	_, err := s.DB.Exec(
		`INSERT INTO swarm_contributions (id, swarm_id, author_id, author_name, section, perspective, body, confidence)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO NOTHING`,
		c.ID, c.SwarmID, c.AuthorID, c.AuthorName, c.Section, c.Perspective, c.Body, c.Confidence,
	)
	return err
}

// ListSwarmContributions returns all contributions for a swarm.
func (s *Store) ListSwarmContributions(swarmID string, limit int) ([]*SwarmContribution, error) {
	rows, err := s.DB.Query(
		`SELECT id, swarm_id, author_id, author_name,
		        COALESCE(section,''), COALESCE(perspective,''), body, COALESCE(confidence,0), created_at
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
			&c.Section, &c.Perspective, &c.Body, &c.Confidence, &c.CreatedAt); err != nil {
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
