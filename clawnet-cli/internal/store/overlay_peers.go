package store

import "time"

// OverlayPeer mirrors the PeerState from the overlay package for DB storage.
type OverlayPeer struct {
	Address     string    `json:"address"`
	Source      string    `json:"source"`
	Alive       bool      `json:"alive"`
	LastSeen    time.Time `json:"last_seen"`
	LastAttempt time.Time `json:"last_attempt"`
	ConsecFails int       `json:"consec_fails"`
	TotalConns  int       `json:"total_conns"`
}

// SaveOverlayPeers replaces all overlay peer state in the database.
func (s *Store) SaveOverlayPeers(peers map[string]*OverlayPeer) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM overlay_peers`); err != nil {
		return err
	}

	stmt, err := tx.Prepare(`INSERT INTO overlay_peers
		(address, source, alive, last_seen, last_attempt, consec_fails, total_conns, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for addr, p := range peers {
		alive := 0
		if p.Alive {
			alive = 1
		}
		if _, err := stmt.Exec(addr, p.Source, alive,
			p.LastSeen.Format(time.RFC3339),
			p.LastAttempt.Format(time.RFC3339),
			p.ConsecFails, p.TotalConns); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LoadOverlayPeers loads all overlay peer state from the database.
func (s *Store) LoadOverlayPeers() (map[string]*OverlayPeer, error) {
	rows, err := s.DB.Query(`SELECT address, source, alive, last_seen, last_attempt, consec_fails, total_conns FROM overlay_peers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	peers := make(map[string]*OverlayPeer)
	for rows.Next() {
		var p OverlayPeer
		var alive int
		var lastSeen, lastAttempt string
		if err := rows.Scan(&p.Address, &p.Source, &alive, &lastSeen, &lastAttempt, &p.ConsecFails, &p.TotalConns); err != nil {
			return nil, err
		}
		p.Alive = alive != 0
		p.LastSeen, _ = time.Parse(time.RFC3339, lastSeen)
		p.LastAttempt, _ = time.Parse(time.RFC3339, lastAttempt)
		peers[p.Address] = &p
	}
	return peers, rows.Err()
}
