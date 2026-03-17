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
	Role         string   `json:"role"`
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
