package store

// DirectMessage represents a direct message.
type DirectMessage struct {
	ID        string `json:"id"`
	PeerID    string `json:"peer_id"`
	Direction string `json:"direction"` // "sent" or "received"
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	Read      bool   `json:"read"`
}

// InsertDM stores a direct message.
func (s *Store) InsertDM(m *DirectMessage) error {
	read := 0
	if m.Read {
		read = 1
	}
	_, err := s.DB.Exec(
		`INSERT OR IGNORE INTO direct_messages (id, peer_id, direction, body, created_at, read)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		m.ID, m.PeerID, m.Direction, m.Body, m.CreatedAt, read,
	)
	return err
}

// ListDMInbox returns the latest message from each peer.
func (s *Store) ListDMInbox() ([]*DirectMessage, error) {
	rows, err := s.DB.Query(
		`SELECT d.id, d.peer_id, d.direction, d.body, d.created_at, d.read
		 FROM direct_messages d
		 INNER JOIN (
			SELECT peer_id, MAX(created_at) AS max_ts FROM direct_messages GROUP BY peer_id
		 ) g ON d.peer_id = g.peer_id AND d.created_at = g.max_ts
		 ORDER BY d.created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDMRows(rows)
}

// ListDMThread returns messages with a specific peer, newest first.
func (s *Store) ListDMThread(peerID string, limit, offset int) ([]*DirectMessage, error) {
	rows, err := s.DB.Query(
		`SELECT id, peer_id, direction, body, created_at, read
		 FROM direct_messages WHERE peer_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		peerID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDMRows(rows)
}

// MarkDMRead marks all messages from a peer as read.
func (s *Store) MarkDMRead(peerID string) error {
	_, err := s.DB.Exec(
		`UPDATE direct_messages SET read = 1 WHERE peer_id = ? AND direction = 'received'`,
		peerID,
	)
	return err
}

// UnreadDMCount returns the number of unread received messages.
func (s *Store) UnreadDMCount() (int, error) {
	var count int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM direct_messages WHERE direction = 'received' AND read = 0`).Scan(&count)
	return count, err
}

func scanDMRows(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]*DirectMessage, error) {
	type scanner interface {
		Next() bool
		Scan(dest ...any) error
		Err() error
	}
	r := rows.(scanner)
	var msgs []*DirectMessage
	for r.Next() {
		m := &DirectMessage{}
		var read int
		if err := r.Scan(&m.ID, &m.PeerID, &m.Direction, &m.Body, &m.CreatedAt, &read); err != nil {
			return nil, err
		}
		m.Read = read != 0
		msgs = append(msgs, m)
	}
	return msgs, r.Err()
}
