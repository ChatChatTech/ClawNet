package store

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// PendingOp represents an operation queued while the daemon was offline or network was unavailable.
type PendingOp struct {
	ID        string `json:"id"`
	Type      string `json:"type"`      // "knowledge", "task_deliver", "dm", "topic_message"
	Payload   string `json:"payload"`   // JSON-encoded operation data
	Status    string `json:"status"`    // "pending", "sent", "failed"
	Retries   int    `json:"retries"`
	CreatedAt string `json:"created_at"`
	Error     string `json:"error,omitempty"`
}

// QueuePendingOp adds an operation to the offline queue.
func (s *Store) QueuePendingOp(opType string, payload any) (string, error) {
	id := uuid.New().String()
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	_, err = s.DB.Exec(
		`INSERT INTO pending_ops (id, type, payload, status) VALUES (?, ?, ?, 'pending')`,
		id, opType, string(data),
	)
	return id, err
}

// ListPendingOps returns all pending operations for retry.
func (s *Store) ListPendingOps() ([]*PendingOp, error) {
	rows, err := s.DB.Query(
		`SELECT id, type, payload, status, retries, created_at, COALESCE(error,'')
		 FROM pending_ops WHERE status = 'pending' ORDER BY created_at ASC LIMIT 100`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ops []*PendingOp
	for rows.Next() {
		var op PendingOp
		if err := rows.Scan(&op.ID, &op.Type, &op.Payload, &op.Status, &op.Retries, &op.CreatedAt, &op.Error); err != nil {
			return nil, err
		}
		ops = append(ops, &op)
	}
	return ops, nil
}

// MarkPendingOpSent marks an operation as successfully sent.
func (s *Store) MarkPendingOpSent(id string) error {
	_, err := s.DB.Exec(`UPDATE pending_ops SET status = 'sent' WHERE id = ?`, id)
	return err
}

// MarkPendingOpFailed records a failure and increments retry count.
func (s *Store) MarkPendingOpFailed(id, errMsg string) error {
	_, err := s.DB.Exec(
		`UPDATE pending_ops SET retries = retries + 1, error = ? WHERE id = ?`,
		errMsg, id,
	)
	return err
}

// PendingOpCount returns the number of pending operations.
func (s *Store) PendingOpCount() int {
	var count int
	s.DB.QueryRow(`SELECT COUNT(*) FROM pending_ops WHERE status = 'pending'`).Scan(&count)
	return count
}

// PruneOldPendingOps removes sent or old failed operations.
func (s *Store) PruneOldPendingOps() {
	cutoff := time.Now().Add(-72 * time.Hour).Format(time.RFC3339)
	s.DB.Exec(`DELETE FROM pending_ops WHERE status = 'sent'`)
	s.DB.Exec(`DELETE FROM pending_ops WHERE status = 'failed' AND retries >= 10`)
	s.DB.Exec(`DELETE FROM pending_ops WHERE created_at < ?`, cutoff)
}
