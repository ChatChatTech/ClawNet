package store

import "database/sql"

// CreditAccount represents a peer's credit account.
type CreditAccount struct {
	PeerID       string  `json:"peer_id"`
	Balance      float64 `json:"balance"`
	Frozen       float64 `json:"frozen"`
	TotalEarned  float64 `json:"total_earned"`
	TotalSpent   float64 `json:"total_spent"`
	UpdatedAt    string  `json:"updated_at"`
}

// CreditTransaction represents a credit transfer record.
type CreditTransaction struct {
	ID        string  `json:"id"`
	FromPeer  string  `json:"from_peer"`
	ToPeer    string  `json:"to_peer"`
	Amount    float64 `json:"amount"`
	Reason    string  `json:"reason"` // "transfer", "task_payment", "task_reward", "initial", "reputation_bonus", "swarm_reward"
	RefID     string  `json:"ref_id"` // optional reference (task_id, etc.)
	CreatedAt string  `json:"created_at"`
}

// EnsureCreditAccount creates an account with initial balance if it doesn't exist.
func (s *Store) EnsureCreditAccount(peerID string, initialBalance float64) error {
	_, err := s.DB.Exec(
		`INSERT OR IGNORE INTO credit_accounts (peer_id, balance, total_earned)
		 VALUES (?, ?, ?)`,
		peerID, initialBalance, initialBalance,
	)
	return err
}

// GetCreditBalance returns the credit account for a peer.
func (s *Store) GetCreditBalance(peerID string) (*CreditAccount, error) {
	row := s.DB.QueryRow(
		`SELECT peer_id, balance, frozen, total_earned, total_spent, updated_at
		 FROM credit_accounts WHERE peer_id = ?`, peerID,
	)
	a := &CreditAccount{}
	err := row.Scan(&a.PeerID, &a.Balance, &a.Frozen, &a.TotalEarned, &a.TotalSpent, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return &CreditAccount{PeerID: peerID}, nil
	}
	return a, err
}

// TransferCredits moves credits from one peer to another within a transaction.
func (s *Store) TransferCredits(txnID, fromPeer, toPeer string, amount float64, reason, refID string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Check sender balance
	var balance float64
	err = tx.QueryRow(`SELECT balance FROM credit_accounts WHERE peer_id = ?`, fromPeer).Scan(&balance)
	if err != nil {
		return err
	}
	if balance < amount {
		return ErrInsufficientCredits
	}

	// Debit sender
	_, err = tx.Exec(
		`UPDATE credit_accounts SET balance = balance - ?, total_spent = total_spent + ?, updated_at = datetime('now')
		 WHERE peer_id = ?`, amount, amount, fromPeer,
	)
	if err != nil {
		return err
	}

	// Credit receiver (ensure account exists)
	_, err = tx.Exec(
		`INSERT INTO credit_accounts (peer_id, balance, total_earned)
		 VALUES (?, ?, ?)
		 ON CONFLICT(peer_id) DO UPDATE SET
		   balance = balance + ?, total_earned = total_earned + ?, updated_at = datetime('now')`,
		toPeer, amount, amount, amount, amount,
	)
	if err != nil {
		return err
	}

	// Record transaction
	_, err = tx.Exec(
		`INSERT INTO credit_transactions (id, from_peer, to_peer, amount, reason, ref_id)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		txnID, fromPeer, toPeer, amount, reason, refID,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// FreezeCredits freezes an amount from available balance.
func (s *Store) FreezeCredits(peerID string, amount float64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var balance float64
	err = tx.QueryRow(`SELECT balance FROM credit_accounts WHERE peer_id = ?`, peerID).Scan(&balance)
	if err != nil {
		return err
	}
	if balance < amount {
		return ErrInsufficientCredits
	}

	_, err = tx.Exec(
		`UPDATE credit_accounts SET balance = balance - ?, frozen = frozen + ?, updated_at = datetime('now')
		 WHERE peer_id = ?`, amount, amount, peerID,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// UnfreezeCredits returns frozen credits back to available balance.
func (s *Store) UnfreezeCredits(peerID string, amount float64) error {
	_, err := s.DB.Exec(
		`UPDATE credit_accounts SET balance = balance + ?, frozen = frozen - ?, updated_at = datetime('now')
		 WHERE peer_id = ? AND frozen >= ?`,
		amount, amount, peerID, amount,
	)
	return err
}

// ListCreditTransactions returns recent transactions for a peer.
func (s *Store) ListCreditTransactions(peerID string, limit, offset int) ([]*CreditTransaction, error) {
	rows, err := s.DB.Query(
		`SELECT id, from_peer, to_peer, amount, reason, ref_id, created_at
		 FROM credit_transactions
		 WHERE from_peer = ? OR to_peer = ?
		 ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		peerID, peerID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txns []*CreditTransaction
	for rows.Next() {
		t := &CreditTransaction{}
		if err := rows.Scan(&t.ID, &t.FromPeer, &t.ToPeer, &t.Amount, &t.Reason, &t.RefID, &t.CreatedAt); err != nil {
			return nil, err
		}
		txns = append(txns, t)
	}
	return txns, rows.Err()
}

// AddCredits adds credits to a peer (for initial grant, reputation bonus, etc.)
func (s *Store) AddCredits(txnID, peerID string, amount float64, reason string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO credit_accounts (peer_id, balance, total_earned)
		 VALUES (?, ?, ?)
		 ON CONFLICT(peer_id) DO UPDATE SET
		   balance = balance + ?, total_earned = total_earned + ?, updated_at = datetime('now')`,
		peerID, amount, amount, amount, amount,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		`INSERT INTO credit_transactions (id, from_peer, to_peer, amount, reason, ref_id)
		 VALUES (?, 'system', ?, ?, ?, '')`,
		txnID, peerID, amount, reason,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// LogCreditAudit stores a credit audit record received from peers for supervision.
func (s *Store) LogCreditAudit(txnID, taskID, from, to string, amount float64, reason, eventTime string) error {
	_, err := s.DB.Exec(
		`INSERT OR IGNORE INTO credit_audit_log (txn_id, task_id, from_peer, to_peer, amount, reason, event_time)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		txnID, taskID, from, to, amount, reason, eventTime,
	)
	return err
}

// CreditAuditRecord represents a peer-broadcast credit audit entry.
type CreditAuditRecord struct {
	TxnID      string  `json:"txn_id"`
	TaskID     string  `json:"task_id"`
	FromPeer   string  `json:"from_peer"`
	ToPeer     string  `json:"to_peer"`
	Amount     float64 `json:"amount"`
	Reason     string  `json:"reason"`
	EventTime  string  `json:"event_time"`
	ReceivedAt string  `json:"received_at"`
}

// ListCreditAudit returns recent credit audit records.
func (s *Store) ListCreditAudit(limit, offset int) ([]*CreditAuditRecord, error) {
	rows, err := s.DB.Query(
		`SELECT txn_id, task_id, from_peer, to_peer, amount, reason, event_time, received_at
		 FROM credit_audit_log ORDER BY received_at DESC LIMIT ? OFFSET ?`, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*CreditAuditRecord
	for rows.Next() {
		r := &CreditAuditRecord{}
		if err := rows.Scan(&r.TxnID, &r.TaskID, &r.FromPeer, &r.ToPeer, &r.Amount, &r.Reason, &r.EventTime, &r.ReceivedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	if result == nil {
		result = []*CreditAuditRecord{}
	}
	return result, nil
}
