package pow

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
)

// DefaultDifficulty is the number of leading zero bits required.
// 20 bits ≈ ~1M SHA-256 hashes ≈ 0.1-0.5s on modern hardware.
const DefaultDifficulty = 20

// Proof stores a solved PoW challenge.
type Proof struct {
	PeerID     string `json:"peer_id"`
	Nonce      uint64 `json:"nonce"`
	Difficulty int    `json:"difficulty"`
}

// Solve finds a nonce such that SHA-256(peerID || nonce) has at least
// `difficulty` leading zero bits. Returns the winning nonce.
func Solve(peerID string, difficulty int) uint64 {
	prefix := []byte(peerID)
	buf := make([]byte, len(prefix)+8)
	copy(buf, prefix)
	var nonce uint64
	for {
		binary.BigEndian.PutUint64(buf[len(prefix):], nonce)
		hash := sha256.Sum256(buf)
		if hasLeadingZeros(hash[:], difficulty) {
			return nonce
		}
		nonce++
	}
}

// Verify checks that SHA-256(peerID || nonce) has at least difficulty leading zero bits.
func Verify(peerID string, nonce uint64, difficulty int) bool {
	prefix := []byte(peerID)
	buf := make([]byte, len(prefix)+8)
	copy(buf, prefix)
	binary.BigEndian.PutUint64(buf[len(prefix):], nonce)
	hash := sha256.Sum256(buf)
	return hasLeadingZeros(hash[:], difficulty)
}

func hasLeadingZeros(hash []byte, bits int) bool {
	fullBytes := bits / 8
	for i := 0; i < fullBytes; i++ {
		if hash[i] != 0 {
			return false
		}
	}
	rem := bits % 8
	if rem > 0 {
		mask := byte(0xFF << (8 - rem))
		if hash[fullBytes]&mask != 0 {
			return false
		}
	}
	return true
}

// proofPath returns the path to the PoW proof file.
func proofPath(dataDir string) string {
	return filepath.Join(dataDir, "pow_proof.json")
}

// LoadProof reads a previously saved PoW proof from disk.
// Returns nil if no proof exists.
func LoadProof(dataDir string) *Proof {
	data, err := os.ReadFile(proofPath(dataDir))
	if err != nil {
		return nil
	}
	var p Proof
	if json.Unmarshal(data, &p) != nil {
		return nil
	}
	return &p
}

// SaveProof persists a PoW proof to disk.
func SaveProof(dataDir string, p *Proof) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(proofPath(dataDir), data, 0600)
}
