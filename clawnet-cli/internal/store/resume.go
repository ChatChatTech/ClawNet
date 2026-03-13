package store

import (
	"database/sql"
	"encoding/json"
	"strings"
)

// AgentResume represents a peer's self-maintained capability profile.
type AgentResume struct {
	PeerID      string `json:"peer_id"`
	AgentName   string `json:"agent_name"`
	Skills      string `json:"skills"`       // JSON array, e.g. ["data-analysis","python","web-scraping"]
	DataSources string `json:"data_sources"` // JSON array describing accessible data/knowledge
	Description string `json:"description"`  // free-text self-description
	UpdatedAt   string `json:"updated_at"`
}

// UpsertResume inserts or updates an agent resume.
func (s *Store) UpsertResume(r *AgentResume) error {
	_, err := s.DB.Exec(
		`INSERT INTO agent_resumes (peer_id, agent_name, skills, data_sources, description)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(peer_id) DO UPDATE SET
		   agent_name = excluded.agent_name,
		   skills = excluded.skills,
		   data_sources = excluded.data_sources,
		   description = excluded.description,
		   updated_at = datetime('now')`,
		r.PeerID, r.AgentName, r.Skills, r.DataSources, r.Description,
	)
	return err
}

// GetResume returns a single agent's resume by peer ID.
func (s *Store) GetResume(peerID string) (*AgentResume, error) {
	row := s.DB.QueryRow(
		`SELECT peer_id, agent_name, skills, data_sources, description, updated_at
		 FROM agent_resumes WHERE peer_id = ?`, peerID,
	)
	r := &AgentResume{}
	err := row.Scan(&r.PeerID, &r.AgentName, &r.Skills, &r.DataSources, &r.Description, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return r, err
}

// ListResumes returns all known agent resumes ordered by most recently updated.
func (s *Store) ListResumes(limit int) ([]*AgentResume, error) {
	rows, err := s.DB.Query(
		`SELECT peer_id, agent_name, skills, data_sources, description, updated_at
		 FROM agent_resumes ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var resumes []*AgentResume
	for rows.Next() {
		r := &AgentResume{}
		if err := rows.Scan(&r.PeerID, &r.AgentName, &r.Skills, &r.DataSources, &r.Description, &r.UpdatedAt); err != nil {
			return nil, err
		}
		resumes = append(resumes, r)
	}
	return resumes, rows.Err()
}

// parseTags parses a JSON array string into a slice of lowercase trimmed strings.
func parseTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		// Try comma-separated fallback
		for _, t := range strings.Split(raw, ",") {
			t = strings.TrimSpace(strings.ToLower(t))
			if t != "" {
				tags = append(tags, t)
			}
		}
		return tags
	}
	result := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(strings.ToLower(t))
		if t != "" {
			result = append(result, t)
		}
	}
	return result
}
