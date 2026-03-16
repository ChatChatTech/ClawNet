package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/pow"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
)

// migrateJSONFiles checks for legacy JSON files in the data directory
// and imports them into the SQLite database, then renames them to .migrated.
func migrateJSONFiles(dataDir string, db *store.Store) {
	migrateProfile(dataDir, db)
	migratePeers(dataDir, db)
	migratePoW(dataDir, db)
	migrateMatrixTokens(dataDir, db)
}

func migrateProfile(dataDir string, db *store.Store) {
	path := filepath.Join(dataDir, "profile.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var p config.Profile
	if json.Unmarshal(data, &p) != nil {
		return
	}
	if err := db.SaveProfile(&store.ProfileEntry{
		AgentName:    p.AgentName,
		Visibility:   p.Visibility,
		Domains:      p.Domains,
		Capabilities: p.Capabilities,
		Bio:          p.Bio,
		Motto:        p.Motto,
		GeoCity:      p.GeoCity,
		GeoLatFuzzy:  p.GeoLatFuzzy,
		GeoLonFuzzy:  p.GeoLonFuzzy,
		Version:      p.Version,
	}); err != nil {
		fmt.Printf("[migrate] profile.json → DB failed: %v\n", err)
		return
	}
	os.Rename(path, path+".migrated")
	fmt.Println("[migrate] profile.json → DB ✓")
}

func migratePeers(dataDir string, db *store.Store) {
	path := filepath.Join(dataDir, "peers.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var pf struct {
		Version int                        `json:"version"`
		Peers   map[string]json.RawMessage `json:"peers"`
	}
	if json.Unmarshal(data, &pf) != nil || pf.Peers == nil {
		return
	}
	peers := make(map[string]*store.OverlayPeer, len(pf.Peers))
	for addr, raw := range pf.Peers {
		var p store.OverlayPeer
		if json.Unmarshal(raw, &p) == nil {
			p.Address = addr
			peers[addr] = &p
		}
	}
	if err := db.SaveOverlayPeers(peers); err != nil {
		fmt.Printf("[migrate] peers.json → DB failed: %v\n", err)
		return
	}
	os.Rename(path, path+".migrated")
	fmt.Printf("[migrate] peers.json → DB ✓ (%d peers)\n", len(peers))
}

func migratePoW(dataDir string, db *store.Store) {
	path := filepath.Join(dataDir, "pow_proof.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var p pow.Proof
	if json.Unmarshal(data, &p) != nil {
		return
	}
	if err := db.SavePoWProof(&store.PoWProof{
		PeerID:     p.PeerID,
		Nonce:      p.Nonce,
		Difficulty: p.Difficulty,
	}); err != nil {
		fmt.Printf("[migrate] pow_proof.json → DB failed: %v\n", err)
		return
	}
	os.Rename(path, path+".migrated")
	fmt.Println("[migrate] pow_proof.json → DB ✓")
}

func migrateMatrixTokens(dataDir string, db *store.Store) {
	path := filepath.Join(dataDir, "matrix_tokens.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var raw map[string]struct {
		AccessToken string `json:"access_token"`
		UserID      string `json:"user_id"`
	}
	if json.Unmarshal(data, &raw) != nil {
		return
	}
	tokens := make(map[string]store.MatrixToken, len(raw))
	for hs, entry := range raw {
		tokens[hs] = store.MatrixToken{
			Homeserver:  hs,
			AccessToken: entry.AccessToken,
			UserID:      entry.UserID,
		}
	}
	if err := db.SaveMatrixTokens(tokens); err != nil {
		fmt.Printf("[migrate] matrix_tokens.json → DB failed: %v\n", err)
		return
	}
	os.Rename(path, path+".migrated")
	fmt.Printf("[migrate] matrix_tokens.json → DB ✓ (%d tokens)\n", len(tokens))
}
