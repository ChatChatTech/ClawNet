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

	dbPath := filepath.Join(dbDir, "clawnet.db")
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

		// Phase 2 — Credit accounts
		`CREATE TABLE IF NOT EXISTS credit_accounts (
			peer_id      TEXT PRIMARY KEY,
			balance      REAL NOT NULL DEFAULT 0,
			frozen       REAL NOT NULL DEFAULT 0,
			total_earned REAL NOT NULL DEFAULT 0,
			total_spent  REAL NOT NULL DEFAULT 0,
			updated_at   TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS credit_transactions (
			id         TEXT PRIMARY KEY,
			from_peer  TEXT NOT NULL,
			to_peer    TEXT NOT NULL,
			amount     REAL NOT NULL,
			reason     TEXT NOT NULL DEFAULT 'transfer',
			ref_id     TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_credit_txn_peer ON credit_transactions(from_peer, created_at)`,

		// Phase 2 — Task Bazaar
		`CREATE TABLE IF NOT EXISTS tasks (
			id          TEXT PRIMARY KEY,
			author_id   TEXT NOT NULL,
			author_name TEXT NOT NULL DEFAULT '',
			title       TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			reward      REAL NOT NULL DEFAULT 0,
			status      TEXT NOT NULL DEFAULT 'open'
			            CHECK(status IN ('open','assigned','submitted','approved','rejected','cancelled')),
			assigned_to TEXT NOT NULL DEFAULT '',
			result      TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS task_bids (
			id          TEXT PRIMARY KEY,
			task_id     TEXT NOT NULL,
			bidder_id   TEXT NOT NULL,
			bidder_name TEXT NOT NULL DEFAULT '',
			amount      REAL NOT NULL DEFAULT 0,
			message     TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (task_id) REFERENCES tasks(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_task_bids ON task_bids(task_id, created_at)`,

		// Phase 2 — Swarm Think
		`CREATE TABLE IF NOT EXISTS swarms (
			id           TEXT PRIMARY KEY,
			creator_id   TEXT NOT NULL,
			creator_name TEXT NOT NULL DEFAULT '',
			title        TEXT NOT NULL,
			question     TEXT NOT NULL DEFAULT '',
			status       TEXT NOT NULL DEFAULT 'open'
			             CHECK(status IN ('open','synthesizing','closed')),
			synthesis    TEXT NOT NULL DEFAULT '',
			created_at   TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at   TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS swarm_contributions (
			id          TEXT PRIMARY KEY,
			swarm_id    TEXT NOT NULL,
			author_id   TEXT NOT NULL,
			author_name TEXT NOT NULL DEFAULT '',
			body        TEXT NOT NULL,
			created_at  TEXT NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (swarm_id) REFERENCES swarms(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_swarm_contrib ON swarm_contributions(swarm_id, created_at)`,

		// Phase 2 — Reputation
		`CREATE TABLE IF NOT EXISTS reputation (
			peer_id          TEXT PRIMARY KEY,
			score            REAL NOT NULL DEFAULT 50,
			tasks_completed  INTEGER NOT NULL DEFAULT 0,
			tasks_failed     INTEGER NOT NULL DEFAULT 0,
			contributions    INTEGER NOT NULL DEFAULT 0,
			knowledge_count  INTEGER NOT NULL DEFAULT 0,
			updated_at       TEXT NOT NULL DEFAULT (datetime('now'))
		)`,

		// Phase 2.1 — Peer credit audit log
		`CREATE TABLE IF NOT EXISTS credit_audit_log (
			txn_id      TEXT PRIMARY KEY,
			task_id     TEXT NOT NULL DEFAULT '',
			from_peer   TEXT NOT NULL,
			to_peer     TEXT NOT NULL,
			amount      REAL NOT NULL,
			reason      TEXT NOT NULL DEFAULT '',
			event_time  TEXT NOT NULL,
			received_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,

		// Phase 2.2 — Swarm Think enhancements: stance labels + time limits
		`ALTER TABLE swarms ADD COLUMN domains TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE swarms ADD COLUMN max_participants INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE swarms ADD COLUMN duration_min INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE swarms ADD COLUMN deadline TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE swarm_contributions ADD COLUMN perspective TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE swarm_contributions ADD COLUMN confidence REAL NOT NULL DEFAULT 0`,

		// Phase 4 — Swarm Think deep templates
		`ALTER TABLE swarms ADD COLUMN template_type TEXT NOT NULL DEFAULT 'freeform'`,
		`ALTER TABLE swarm_contributions ADD COLUMN section TEXT NOT NULL DEFAULT ''`,

		// Phase 3 — Prediction Market (Oracle Arena)
		`CREATE TABLE IF NOT EXISTS predictions (
			id                TEXT PRIMARY KEY,
			creator_id        TEXT NOT NULL,
			creator_name      TEXT NOT NULL DEFAULT '',
			question          TEXT NOT NULL,
			options           TEXT NOT NULL DEFAULT '[]',
			category          TEXT NOT NULL DEFAULT 'custom',
			resolution_date   TEXT NOT NULL,
			resolution_source TEXT NOT NULL DEFAULT '',
			status            TEXT NOT NULL DEFAULT 'open'
			                  CHECK(status IN ('open', 'pending', 'resolved', 'cancelled')),
			result            TEXT NOT NULL DEFAULT '',
			appeal_deadline   TEXT NOT NULL DEFAULT '',
			total_stake       REAL NOT NULL DEFAULT 0,
			created_at        TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS prediction_bets (
			id            TEXT PRIMARY KEY,
			prediction_id TEXT NOT NULL,
			bettor_id     TEXT NOT NULL,
			bettor_name   TEXT NOT NULL DEFAULT '',
			option        TEXT NOT NULL,
			stake         REAL NOT NULL DEFAULT 0,
			reasoning     TEXT NOT NULL DEFAULT '',
			created_at    TEXT NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (prediction_id) REFERENCES predictions(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pred_bets ON prediction_bets(prediction_id, bettor_id)`,
		`CREATE TABLE IF NOT EXISTS prediction_resolutions (
			id            TEXT PRIMARY KEY,
			prediction_id TEXT NOT NULL,
			resolver_id   TEXT NOT NULL,
			result        TEXT NOT NULL,
			evidence_url  TEXT NOT NULL DEFAULT '',
			created_at    TEXT NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (prediction_id) REFERENCES predictions(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pred_res ON prediction_resolutions(prediction_id, result)`,

		// Phase 3.1 — Prediction appeal mechanism
		`ALTER TABLE predictions ADD COLUMN appeal_deadline TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE IF NOT EXISTS prediction_appeals (
			id            TEXT PRIMARY KEY,
			prediction_id TEXT NOT NULL,
			appellant_id  TEXT NOT NULL,
			reason        TEXT NOT NULL DEFAULT '',
			evidence_url  TEXT NOT NULL DEFAULT '',
			created_at    TEXT NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (prediction_id) REFERENCES predictions(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pred_appeals ON prediction_appeals(prediction_id, appellant_id)`,

		// Phase 4 — Energy & Prestige system (Social Energy Model)
		`ALTER TABLE credit_accounts ADD COLUMN prestige REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE credit_accounts ADD COLUMN last_regen TEXT NOT NULL DEFAULT (datetime('now'))`,

		// Phase 4 — Task template structured fields
		`ALTER TABLE tasks ADD COLUMN tags TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE tasks ADD COLUMN deadline TEXT NOT NULL DEFAULT ''`,

		// Phase 4 — Agent Resumes (supply-demand matching)
		`CREATE TABLE IF NOT EXISTS agent_resumes (
			peer_id      TEXT PRIMARY KEY,
			agent_name   TEXT NOT NULL DEFAULT '',
			skills       TEXT NOT NULL DEFAULT '[]',
			data_sources TEXT NOT NULL DEFAULT '[]',
			description  TEXT NOT NULL DEFAULT '',
			updated_at   TEXT NOT NULL DEFAULT (datetime('now'))
		)`,

		// Phase 5 — Nutshell integration (native .nut bundle support)
		`ALTER TABLE tasks ADD COLUMN nutshell_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE tasks ADD COLUMN nutshell_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE tasks ADD COLUMN bundle_type TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE IF NOT EXISTS task_bundles (
			task_id     TEXT PRIMARY KEY,
			bundle      BLOB NOT NULL,
			hash        TEXT NOT NULL DEFAULT '',
			size        INTEGER NOT NULL DEFAULT 0,
			uploaded_at TEXT NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (task_id) REFERENCES tasks(id)
		)`,
	}

	for _, m := range migrations {
		if _, err := s.DB.Exec(m); err != nil {
			// ALTER TABLE ADD COLUMN fails if column already exists — that's OK
			if len(m) > 12 && m[:12] == "ALTER TABLE " {
				continue
			}
			return fmt.Errorf("exec %q: %w", m[:60], err)
		}
	}

	// Migrate predictions CHECK constraint to allow 'pending' status.
	// Only needed for DBs created before appeal mechanism was added.
	if err := s.migratePredictionsCheck(); err != nil {
		return fmt.Errorf("predictions check migration: %w", err)
	}

	return nil
}

// migratePredictionsCheck recreates the predictions table if its CHECK constraint
// doesn't include 'pending'. This is a one-time migration for existing databases.
func (s *Store) migratePredictionsCheck() error {
	// Try a no-op update with status='pending' on a non-existent row to probe the CHECK.
	_, err := s.DB.Exec(`UPDATE predictions SET status = 'pending' WHERE id = '__check_probe__'`)
	if err == nil {
		return nil // CHECK already allows 'pending' — nothing to do
	}
	// CHECK violation means old schema — recreate
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS predictions_v2 (
			id                TEXT PRIMARY KEY,
			creator_id        TEXT NOT NULL,
			creator_name      TEXT NOT NULL DEFAULT '',
			question          TEXT NOT NULL,
			options           TEXT NOT NULL DEFAULT '[]',
			category          TEXT NOT NULL DEFAULT 'custom',
			resolution_date   TEXT NOT NULL,
			resolution_source TEXT NOT NULL DEFAULT '',
			status            TEXT NOT NULL DEFAULT 'open'
			                  CHECK(status IN ('open', 'pending', 'resolved', 'cancelled')),
			result            TEXT NOT NULL DEFAULT '',
			appeal_deadline   TEXT NOT NULL DEFAULT '',
			total_stake       REAL NOT NULL DEFAULT 0,
			created_at        TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`INSERT OR IGNORE INTO predictions_v2
		   SELECT id, creator_id, creator_name, question, options, category,
		          resolution_date, resolution_source, status, result,
		          COALESCE(appeal_deadline, ''), total_stake, created_at, updated_at
		   FROM predictions`,
		`DROP TABLE predictions`,
		`ALTER TABLE predictions_v2 RENAME TO predictions`,
	}
	for _, stmt := range stmts {
		if _, err := s.DB.Exec(stmt); err != nil {
			return fmt.Errorf("exec migration: %w", err)
		}
	}
	return nil
}
