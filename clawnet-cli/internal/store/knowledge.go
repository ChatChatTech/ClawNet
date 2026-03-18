package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// KnowledgeEntry represents a knowledge item.
type KnowledgeEntry struct {
	ID         string   `json:"id"`
	AuthorID   string   `json:"author_id"`
	AuthorName string   `json:"author_name"`
	Title      string   `json:"title"`
	Body       string   `json:"body"`
	Domains    []string `json:"domains"`
	Upvotes    int      `json:"upvotes"`
	Flags      int      `json:"flags"`
	CreatedAt  string   `json:"created_at"`
}

// KnowledgeReply represents a reply to a knowledge entry.
type KnowledgeReply struct {
	ID          string `json:"id"`
	KnowledgeID string `json:"knowledge_id"`
	AuthorID    string `json:"author_id"`
	AuthorName  string `json:"author_name"`
	Body        string `json:"body"`
	CreatedAt   string `json:"created_at"`
}

// InsertKnowledge upserts a knowledge entry.
func (s *Store) InsertKnowledge(e *KnowledgeEntry) error {
	domains, _ := json.Marshal(e.Domains)
	_, err := s.DB.Exec(
		`INSERT OR IGNORE INTO knowledge (id, author_id, author_name, title, body, domains, upvotes, flags, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.AuthorID, e.AuthorName, e.Title, e.Body, string(domains), e.Upvotes, e.Flags, e.CreatedAt,
	)
	return err
}

// GetKnowledge retrieves a single knowledge entry by ID.
func (s *Store) GetKnowledge(id string) (*KnowledgeEntry, error) {
	row := s.DB.QueryRow(
		`SELECT id, author_id, author_name, title, body, domains, upvotes, flags, created_at
		 FROM knowledge WHERE id = ?`, id,
	)
	return scanKnowledge(row)
}

// ListKnowledge returns knowledge entries, optionally filtered by domain.
func (s *Store) ListKnowledge(domain string, limit, offset int) ([]*KnowledgeEntry, error) {
	var rows *sql.Rows
	var err error
	if domain != "" {
		rows, err = s.DB.Query(
			`SELECT id, author_id, author_name, title, body, domains, upvotes, flags, created_at
			 FROM knowledge WHERE domains LIKE ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
			fmt.Sprintf("%%%s%%", domain), limit, offset,
		)
	} else {
		rows, err = s.DB.Query(
			`SELECT id, author_id, author_name, title, body, domains, upvotes, flags, created_at
			 FROM knowledge ORDER BY created_at DESC LIMIT ? OFFSET ?`,
			limit, offset,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanKnowledgeRows(rows)
}

// SearchKnowledge performs full-text search using FTS5 when available,
// falling back to LIKE-based search on platforms without fts5 support.
func (s *Store) SearchKnowledge(query string, limit int) ([]*KnowledgeEntry, error) {
	if s.HasFTS5 {
		rows, err := s.DB.Query(
			`SELECT k.id, k.author_id, k.author_name, k.title, k.body, k.domains, k.upvotes, k.flags, k.created_at
			 FROM knowledge k
			 JOIN knowledge_fts f ON k.rowid = f.rowid
			 WHERE knowledge_fts MATCH ?
			 ORDER BY rank LIMIT ?`,
			query, limit,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanKnowledgeRows(rows)
	}

	// Fallback: LIKE-based search (no fts5 module)
	pattern := "%" + likeSanitize(query) + "%"
	rows, err := s.DB.Query(
		`SELECT id, author_id, author_name, title, body, domains, upvotes, flags, created_at
		 FROM knowledge
		 WHERE title LIKE ? OR body LIKE ? OR domains LIKE ?
		 ORDER BY created_at DESC LIMIT ?`,
		pattern, pattern, pattern, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanKnowledgeRows(rows)
}

// likeSanitize escapes LIKE wildcards in user input.
func likeSanitize(s string) string {
	r := strings.NewReplacer("%", "\\%", "_", "\\_")
	return r.Replace(s)
}

// ReactKnowledge records a reaction (upvote/flag) and updates counters.
func (s *Store) ReactKnowledge(knowledgeID, peerID, reaction string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Upsert reaction
	_, err = tx.Exec(
		`INSERT INTO knowledge_reactions (knowledge_id, peer_id, reaction)
		 VALUES (?, ?, ?)
		 ON CONFLICT(knowledge_id, peer_id) DO UPDATE SET reaction = excluded.reaction`,
		knowledgeID, peerID, reaction,
	)
	if err != nil {
		return err
	}

	// Recount
	_, err = tx.Exec(
		`UPDATE knowledge SET
			upvotes = (SELECT COUNT(*) FROM knowledge_reactions WHERE knowledge_id = ? AND reaction = 'upvote'),
			flags   = (SELECT COUNT(*) FROM knowledge_reactions WHERE knowledge_id = ? AND reaction = 'flag')
		 WHERE id = ?`,
		knowledgeID, knowledgeID, knowledgeID,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// InsertReply adds a reply to a knowledge entry.
func (s *Store) InsertReply(r *KnowledgeReply) error {
	_, err := s.DB.Exec(
		`INSERT OR IGNORE INTO knowledge_replies (id, knowledge_id, author_id, author_name, body, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		r.ID, r.KnowledgeID, r.AuthorID, r.AuthorName, r.Body, r.CreatedAt,
	)
	return err
}

// ListReplies returns replies for a knowledge entry.
func (s *Store) ListReplies(knowledgeID string, limit int) ([]*KnowledgeReply, error) {
	rows, err := s.DB.Query(
		`SELECT id, knowledge_id, author_id, author_name, body, created_at
		 FROM knowledge_replies WHERE knowledge_id = ? ORDER BY created_at ASC LIMIT ?`,
		knowledgeID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var replies []*KnowledgeReply
	for rows.Next() {
		r := &KnowledgeReply{}
		if err := rows.Scan(&r.ID, &r.KnowledgeID, &r.AuthorID, &r.AuthorName, &r.Body, &r.CreatedAt); err != nil {
			return nil, err
		}
		replies = append(replies, r)
	}
	return replies, rows.Err()
}

// scanner helpers

type scannable interface {
	Scan(dest ...any) error
}

func scanKnowledge(row scannable) (*KnowledgeEntry, error) {
	e := &KnowledgeEntry{}
	var domainsJSON string
	err := row.Scan(&e.ID, &e.AuthorID, &e.AuthorName, &e.Title, &e.Body, &domainsJSON, &e.Upvotes, &e.Flags, &e.CreatedAt)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(domainsJSON), &e.Domains)
	if e.Domains == nil {
		e.Domains = []string{}
	}
	return e, nil
}

func scanKnowledgeRows(rows *sql.Rows) ([]*KnowledgeEntry, error) {
	var entries []*KnowledgeEntry
	for rows.Next() {
		e := &KnowledgeEntry{}
		var domainsJSON string
		if err := rows.Scan(&e.ID, &e.AuthorID, &e.AuthorName, &e.Title, &e.Body, &domainsJSON, &e.Upvotes, &e.Flags, &e.CreatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(domainsJSON), &e.Domains)
		if e.Domains == nil {
			e.Domains = []string{}
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// ListKnowledgeSince returns knowledge entries created after the given RFC3339 timestamp.
func (s *Store) ListKnowledgeSince(since string, limit int) ([]*KnowledgeEntry, error) {
	rows, err := s.DB.Query(
		`SELECT id, author_id, author_name, title, body, domains, upvotes, flags, created_at
		 FROM knowledge WHERE created_at > ? ORDER BY created_at ASC LIMIT ?`,
		since, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanKnowledgeRows(rows)
}

// LatestKnowledgeTime returns the created_at of the most recent knowledge entry.
func (s *Store) LatestKnowledgeTime() string {
	var t sql.NullString
	s.DB.QueryRow(`SELECT MAX(created_at) FROM knowledge`).Scan(&t)
	if t.Valid {
		return t.String
	}
	return ""
}

// EscapeFTS5 escapes a user query for safe FTS5 matching.
func EscapeFTS5(q string) string {
	// Wrap each word in double quotes to avoid FTS5 syntax issues
	words := strings.Fields(q)
	for i, w := range words {
		words[i] = `"` + strings.ReplaceAll(w, `"`, `""`) + `"`
	}
	return strings.Join(words, " ")
}
