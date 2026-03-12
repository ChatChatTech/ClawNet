package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// Store wraps an SQLite database for local persistence.
type Store struct {
	DB *sql.DB
}

// Open opens (or creates) the SQLite database in the given data directory.
func Open(dataDir string) (*Store, error) {
	dbDir := filepath.Join(dataDir, "data")
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(dbDir, "letchat.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Single writer, improve concurrency
	db.SetMaxOpenConns(1)

	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.DB.Close()
}

func (s *Store) migrate() error {
	migrations := []string{
		// Knowledge entries
		`CREATE TABLE IF NOT EXISTS knowledge (
			id          TEXT PRIMARY KEY,
			author_id   TEXT NOT NULL,
			author_name TEXT NOT NULL DEFAULT '',
			title       TEXT NOT NULL,
			body        TEXT NOT NULL,
			domains     TEXT NOT NULL DEFAULT '[]',
			upvotes     INTEGER NOT NULL DEFAULT 0,
			flags       INTEGER NOT NULL DEFAULT 0,
			created_at  TEXT NOT NULL DEFAULT (datetime('now')),
			received_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		// FTS5 index for knowledge search
		`CREATE VIRTUAL TABLE IF NOT EXISTS knowledge_fts USING fts5(
			title, body, domains,
			content='knowledge',
			content_rowid='rowid'
		)`,
		// Triggers to keep FTS index in sync
		`CREATE TRIGGER IF NOT EXISTS knowledge_ai AFTER INSERT ON knowledge BEGIN
			INSERT INTO knowledge_fts(rowid, title, body, domains)
			VALUES (new.rowid, new.title, new.body, new.domains);
		END`,
		`CREATE TRIGGER IF NOT EXISTS knowledge_ad AFTER DELETE ON knowledge BEGIN
			INSERT INTO knowledge_fts(knowledge_fts, rowid, title, body, domains)
			VALUES ('delete', old.rowid, old.title, old.body, old.domains);
		END`,
		`CREATE TRIGGER IF NOT EXISTS knowledge_au AFTER UPDATE ON knowledge BEGIN
			INSERT INTO knowledge_fts(knowledge_fts, rowid, title, body, domains)
			VALUES ('delete', old.rowid, old.title, old.body, old.domains);
			INSERT INTO knowledge_fts(rowid, title, body, domains)
			VALUES (new.rowid, new.title, new.body, new.domains);
		END`,
		// Knowledge reactions
		`CREATE TABLE IF NOT EXISTS knowledge_reactions (
			knowledge_id TEXT NOT NULL,
			peer_id      TEXT NOT NULL,
			reaction     TEXT NOT NULL DEFAULT 'upvote',
			created_at   TEXT NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (knowledge_id, peer_id)
		)`,
		// Knowledge replies
		`CREATE TABLE IF NOT EXISTS knowledge_replies (
			id           TEXT PRIMARY KEY,
			knowledge_id TEXT NOT NULL,
			author_id    TEXT NOT NULL,
			author_name  TEXT NOT NULL DEFAULT '',
			body         TEXT NOT NULL,
			created_at   TEXT NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (knowledge_id) REFERENCES knowledge(id)
		)`,
		// Topic rooms
		`CREATE TABLE IF NOT EXISTS topics (
			name        TEXT PRIMARY KEY,
			description TEXT NOT NULL DEFAULT '',
			creator_id  TEXT NOT NULL,
			created_at  TEXT NOT NULL DEFAULT (datetime('now')),
			joined      INTEGER NOT NULL DEFAULT 1
		)`,
		// Topic messages
		`CREATE TABLE IF NOT EXISTS topic_messages (
			id         TEXT PRIMARY KEY,
			topic_name TEXT NOT NULL,
			author_id  TEXT NOT NULL,
			author_name TEXT NOT NULL DEFAULT '',
			body       TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (topic_name) REFERENCES topics(name)
		)`,
		// Direct messages
		`CREATE TABLE IF NOT EXISTS direct_messages (
			id         TEXT PRIMARY KEY,
			peer_id    TEXT NOT NULL,
			direction  TEXT NOT NULL CHECK(direction IN ('sent','received')),
			body       TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			read       INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_dm_peer ON direct_messages(peer_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_topic_msg ON topic_messages(topic_name, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_knowledge_created ON knowledge(created_at DESC)`,
	}

	for _, m := range migrations {
		if _, err := s.DB.Exec(m); err != nil {
			return fmt.Errorf("exec %q: %w", m[:60], err)
		}
	}
	return nil
}
