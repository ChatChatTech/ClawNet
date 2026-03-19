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
	ActiveTasks int    `json:"active_tasks"` // current task load
	UpdatedAt   string `json:"updated_at"`
}

// UpsertResume inserts or updates an agent resume.
func (s *Store) UpsertResume(r *AgentResume) error {
	_, err := s.DB.Exec(
		`INSERT INTO agent_resumes (peer_id, agent_name, skills, data_sources, description, active_tasks)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(peer_id) DO UPDATE SET
		   agent_name = excluded.agent_name,
		   skills = excluded.skills,
		   data_sources = excluded.data_sources,
		   description = excluded.description,
		   active_tasks = excluded.active_tasks,
		   updated_at = datetime('now')`,
		r.PeerID, r.AgentName, r.Skills, r.DataSources, r.Description, r.ActiveTasks,
	)
	return err
}

// GetResume returns a single agent's resume by peer ID.
func (s *Store) GetResume(peerID string) (*AgentResume, error) {
	row := s.DB.QueryRow(
		`SELECT peer_id, agent_name, skills, data_sources, description, active_tasks, updated_at
		 FROM agent_resumes WHERE peer_id = ?`, peerID,
	)
	r := &AgentResume{}
	err := row.Scan(&r.PeerID, &r.AgentName, &r.Skills, &r.DataSources, &r.Description, &r.ActiveTasks, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return r, err
}

// ListResumes returns all known agent resumes ordered by most recently updated.
func (s *Store) ListResumes(limit int) ([]*AgentResume, error) {
	rows, err := s.DB.Query(
		`SELECT peer_id, agent_name, skills, data_sources, description, active_tasks, updated_at
		 FROM agent_resumes ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var resumes []*AgentResume
	for rows.Next() {
		r := &AgentResume{}
		if err := rows.Scan(&r.PeerID, &r.AgentName, &r.Skills, &r.DataSources, &r.Description, &r.ActiveTasks, &r.UpdatedAt); err != nil {
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

// RecalcActiveTasks counts in-progress tasks (assigned or submitted) for a peer
// and updates the active_tasks field on their resume.
func (s *Store) RecalcActiveTasks(peerID string) (int, error) {
	var count int
	err := s.DB.QueryRow(
		`SELECT COUNT(*) FROM tasks WHERE assigned_to = ? AND status IN ('assigned','submitted')`,
		peerID,
	).Scan(&count)
	if err != nil {
		return 0, err
	}
	s.DB.Exec(`UPDATE agent_resumes SET active_tasks = ? WHERE peer_id = ?`, count, peerID)
	return count, nil
}

// AutoUpdateResumeSkills merges new tags from a completed task into the agent's resume skills.
// Returns true if skills were actually updated.
func (s *Store) AutoUpdateResumeSkills(peerID, taskTags string) (bool, error) {
	resume, err := s.GetResume(peerID)
	if err != nil || resume == nil {
		return false, err
	}
	existing := parseTags(resume.Skills)
	newTags := parseTags(taskTags)
	if len(newTags) == 0 {
		return false, nil
	}

	existSet := make(map[string]bool, len(existing))
	for _, t := range existing {
		existSet[t] = true
	}

	added := false
	for _, t := range newTags {
		if !existSet[t] {
			existing = append(existing, t)
			existSet[t] = true
			added = true
		}
	}
	if !added {
		return false, nil
	}

	skillsJSON, _ := json.Marshal(existing)
	resume.Skills = string(skillsJSON)
	return true, s.UpsertResume(resume)
}

// SearchResumes returns resumes whose skills match any of the given tags.
func (s *Store) SearchResumes(tags []string, limit int) ([]*AgentResume, error) {
	if len(tags) == 0 {
		return s.ListResumes(limit)
	}
	// Fetch all resumes and filter in Go (tags are JSON arrays, not individually indexed)
	all, err := s.ListResumes(500)
	if err != nil {
		return nil, err
	}
	tagSet := make(map[string]bool, len(tags))
	for _, t := range tags {
		tagSet[strings.TrimSpace(strings.ToLower(t))] = true
	}

	var result []*AgentResume
	for _, r := range all {
		skills := parseTags(r.Skills)
		for _, sk := range skills {
			if tagSet[sk] {
				result = append(result, r)
				break
			}
		}
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}
