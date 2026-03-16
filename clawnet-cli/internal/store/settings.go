package store

import "encoding/json"

// ProfileEntry is a key-value pair for the node profile.
type ProfileEntry struct {
	AgentName    string   `json:"agent_name"`
	Visibility   string   `json:"visibility"`
	Domains      []string `json:"domains"`
	Capabilities []string `json:"capabilities"`
	Bio          string   `json:"bio"`
	Motto        string   `json:"motto"`
	GeoCity      string   `json:"geo_city"`
	GeoLatFuzzy  float64  `json:"geo_lat_fuzzy"`
	GeoLonFuzzy  float64  `json:"geo_lon_fuzzy"`
	Version      string   `json:"version"`
}

// SaveProfile persists the node profile as JSON in the node_profile table.
func (s *Store) SaveProfile(p *ProfileEntry) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`INSERT INTO node_profile (key, value) VALUES ('profile', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, string(data))
	return err
}

// LoadProfile reads the node profile from the database.
// Returns nil if no profile is stored.
func (s *Store) LoadProfile() (*ProfileEntry, error) {
	var value string
	err := s.DB.QueryRow(`SELECT value FROM node_profile WHERE key = 'profile'`).Scan(&value)
	if err != nil {
		return nil, err
	}
	var p ProfileEntry
	if err := json.Unmarshal([]byte(value), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// PoWProof stores a PoW challenge result.
type PoWProof struct {
	PeerID     string `json:"peer_id"`
	Nonce      uint64 `json:"nonce"`
	Difficulty int    `json:"difficulty"`
}

// SavePoWProof persists a PoW proof in the database.
func (s *Store) SavePoWProof(p *PoWProof) error {
	_, err := s.DB.Exec(`INSERT INTO pow_proof (peer_id, nonce, difficulty) VALUES (?, ?, ?)
		ON CONFLICT(peer_id) DO UPDATE SET nonce = excluded.nonce, difficulty = excluded.difficulty`,
		p.PeerID, int64(p.Nonce), p.Difficulty)
	return err
}

// LoadPoWProof reads the PoW proof for a given peer ID.
// Returns nil, nil if no proof exists.
func (s *Store) LoadPoWProof(peerID string) (*PoWProof, error) {
	var nonce int64
	var difficulty int
	err := s.DB.QueryRow(`SELECT nonce, difficulty FROM pow_proof WHERE peer_id = ?`, peerID).Scan(&nonce, &difficulty)
	if err != nil {
		return nil, nil // no proof
	}
	return &PoWProof{PeerID: peerID, Nonce: uint64(nonce), Difficulty: difficulty}, nil
}

// MatrixToken stores a Matrix homeserver session.
type MatrixToken struct {
	Homeserver  string `json:"homeserver"`
	AccessToken string `json:"access_token"`
	UserID      string `json:"user_id"`
}

// SaveMatrixTokens replaces all Matrix tokens in the database.
func (s *Store) SaveMatrixTokens(tokens map[string]MatrixToken) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM matrix_tokens`); err != nil {
		return err
	}

	stmt, err := tx.Prepare(`INSERT INTO matrix_tokens (homeserver, access_token, user_id, updated_at)
		VALUES (?, ?, ?, datetime('now'))`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for hs, t := range tokens {
		if _, err := stmt.Exec(hs, t.AccessToken, t.UserID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LoadMatrixTokens reads all Matrix tokens from the database.
func (s *Store) LoadMatrixTokens() (map[string]MatrixToken, error) {
	rows, err := s.DB.Query(`SELECT homeserver, access_token, user_id FROM matrix_tokens`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make(map[string]MatrixToken)
	for rows.Next() {
		var t MatrixToken
		if err := rows.Scan(&t.Homeserver, &t.AccessToken, &t.UserID); err != nil {
			return nil, err
		}
		tokens[t.Homeserver] = t
	}
	return tokens, rows.Err()
}
