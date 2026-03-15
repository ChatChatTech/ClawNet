package crypto

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	libcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/crypto/nacl/box"
)

// Engine manages E2E encryption sessions for DMs.
// Uses X25519 key exchange + NaCl box (XSalsa20-Poly1305).
type Engine struct {
	privScalar [32]byte // our Curve25519 private key
	pubKey     [32]byte // our Curve25519 public key
	db         *sql.DB
	mu         sync.RWMutex
	// cache of peer Curve25519 public keys
	peerKeys map[peer.ID][32]byte
}

// NewEngine creates a new crypto engine from the node's Ed25519 private key.
func NewEngine(priv libcrypto.PrivKey, db *sql.DB) (*Engine, error) {
	scalar, err := Ed25519PrivToCurve25519(priv)
	if err != nil {
		return nil, fmt.Errorf("derive curve25519 privkey: %w", err)
	}

	// Derive our Curve25519 public key from the Ed25519 public key.
	pubRaw, err := priv.GetPublic().Raw()
	if err != nil {
		return nil, fmt.Errorf("extract public key: %w", err)
	}
	pubCurve, err := Ed25519PubToCurve25519(pubRaw)
	if err != nil {
		return nil, fmt.Errorf("derive curve25519 pubkey: %w", err)
	}

	e := &Engine{
		privScalar: scalar,
		pubKey:     pubCurve,
		db:         db,
		peerKeys:   make(map[peer.ID][32]byte),
	}

	if err := e.migrate(); err != nil {
		return nil, fmt.Errorf("crypto migration: %w", err)
	}

	return e, nil
}

// migrate creates the crypto sessions table if needed.
func (e *Engine) migrate() error {
	_, err := e.db.Exec(`CREATE TABLE IF NOT EXISTS crypto_sessions (
		peer_id    TEXT PRIMARY KEY,
		pub_key    TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	return err
}

// EncryptedEnvelope is the wire format for an encrypted DM.
type EncryptedEnvelope struct {
	Version   int    `json:"v"`          // protocol version (1)
	Encrypted bool   `json:"encrypted"`  // always true
	PubKey    string `json:"pub_key"`    // sender's Curve25519 public key (base64)
	Nonce     string `json:"nonce"`      // 24-byte nonce (base64)
	Cipher    string `json:"ciphertext"` // NaCl box ciphertext (base64)
}

// Encrypt encrypts plaintext for the given peer using NaCl box.
// Returns the JSON-encoded EncryptedEnvelope.
func (e *Engine) Encrypt(peerID peer.ID, plaintext []byte) ([]byte, error) {
	peerPub, err := e.getPeerKey(peerID)
	if err != nil {
		return nil, fmt.Errorf("get peer key: %w", err)
	}

	// Generate random nonce
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	// NaCl box: XSalsa20-Poly1305 authenticated encryption
	ciphertext := box.Seal(nil, plaintext, &nonce, &peerPub, &e.privScalar)

	env := EncryptedEnvelope{
		Version:   1,
		Encrypted: true,
		PubKey:    base64.StdEncoding.EncodeToString(e.pubKey[:]),
		Nonce:     base64.StdEncoding.EncodeToString(nonce[:]),
		Cipher:    base64.StdEncoding.EncodeToString(ciphertext),
	}
	return json.Marshal(env)
}

// Decrypt attempts to decrypt an EncryptedEnvelope. Returns the plaintext.
func (e *Engine) Decrypt(peerID peer.ID, envelope []byte) ([]byte, error) {
	var env EncryptedEnvelope
	if err := json.Unmarshal(envelope, &env); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}
	if !env.Encrypted || env.Version != 1 {
		return nil, fmt.Errorf("unsupported envelope version %d", env.Version)
	}

	// Decode sender's public key
	pubBytes, err := base64.StdEncoding.DecodeString(env.PubKey)
	if err != nil || len(pubBytes) != 32 {
		return nil, fmt.Errorf("invalid sender public key")
	}
	var senderPub [32]byte
	copy(senderPub[:], pubBytes)

	// Cache the sender's public key
	e.mu.Lock()
	e.peerKeys[peerID] = senderPub
	e.mu.Unlock()
	e.storePeerKey(peerID, senderPub)

	// Decode nonce
	nonceBytes, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil || len(nonceBytes) != 24 {
		return nil, fmt.Errorf("invalid nonce")
	}
	var nonce [24]byte
	copy(nonce[:], nonceBytes)

	// Decode ciphertext
	ciphertext, err := base64.StdEncoding.DecodeString(env.Cipher)
	if err != nil {
		return nil, fmt.Errorf("invalid ciphertext")
	}

	// NaCl box open
	plaintext, ok := box.Open(nil, ciphertext, &nonce, &senderPub, &e.privScalar)
	if !ok {
		return nil, fmt.Errorf("decryption failed: authentication error")
	}
	return plaintext, nil
}

// IsEncrypted checks if a wire message is an encrypted envelope.
func IsEncrypted(data []byte) bool {
	var probe struct {
		Encrypted bool `json:"encrypted"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}
	return probe.Encrypted
}

// SessionCount returns the number of known peer crypto sessions.
func (e *Engine) SessionCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.peerKeys)
}

// Sessions returns info about all known crypto sessions.
func (e *Engine) Sessions() []SessionInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var result []SessionInfo
	for pid, pub := range e.peerKeys {
		result = append(result, SessionInfo{
			PeerID: pid.String(),
			PubKey: base64.StdEncoding.EncodeToString(pub[:]),
		})
	}
	return result
}

// SessionInfo describes a crypto session with a peer.
type SessionInfo struct {
	PeerID string `json:"peer_id"`
	PubKey string `json:"pub_key"`
}

// getPeerKey retrieves or derives the Curve25519 public key for a peer.
func (e *Engine) getPeerKey(pid peer.ID) ([32]byte, error) {
	e.mu.RLock()
	if k, ok := e.peerKeys[pid]; ok {
		e.mu.RUnlock()
		return k, nil
	}
	e.mu.RUnlock()

	// Try loading from DB
	if k, err := e.loadPeerKey(pid); err == nil {
		e.mu.Lock()
		e.peerKeys[pid] = k
		e.mu.Unlock()
		return k, nil
	}

	// Derive from peer ID (Ed25519 public key embedded in peer ID)
	k, err := PeerIDToCurve25519(pid)
	if err != nil {
		return [32]byte{}, err
	}
	e.mu.Lock()
	e.peerKeys[pid] = k
	e.mu.Unlock()
	e.storePeerKey(pid, k)
	return k, nil
}

func (e *Engine) storePeerKey(pid peer.ID, pub [32]byte) {
	pubB64 := base64.StdEncoding.EncodeToString(pub[:])
	e.db.Exec(
		`INSERT OR REPLACE INTO crypto_sessions (peer_id, pub_key, created_at) VALUES (?, ?, ?)`,
		pid.String(), pubB64, time.Now().UTC().Format(time.RFC3339),
	)
}

func (e *Engine) loadPeerKey(pid peer.ID) ([32]byte, error) {
	var pubB64 string
	err := e.db.QueryRow(`SELECT pub_key FROM crypto_sessions WHERE peer_id = ?`, pid.String()).Scan(&pubB64)
	if err != nil {
		return [32]byte{}, err
	}
	b, err := base64.StdEncoding.DecodeString(pubB64)
	if err != nil || len(b) != 32 {
		return [32]byte{}, fmt.Errorf("invalid stored key")
	}
	var k [32]byte
	copy(k[:], b)
	return k, nil
}
