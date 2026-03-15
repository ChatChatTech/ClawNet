package p2p

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	// RepKeyPrefix is the DHT namespace for reputation snapshots.
	RepKeyPrefix = "/clawnet-rep/"
)

// RepSnapshot is the signed reputation record stored in DHT.
type RepSnapshot struct {
	PeerID         string  `json:"peer_id"`
	Score          float64 `json:"score"`
	TasksCompleted int     `json:"tasks_completed"`
	TasksFailed    int     `json:"tasks_failed"`
	Contributions  int     `json:"contributions"`
	KnowledgeCount int     `json:"knowledge_count"`
	Timestamp      int64   `json:"timestamp"`
}

// SignedRep is the wire format: payload + signature of the self-attested reputation.
type SignedRep struct {
	Payload   []byte `json:"payload"`
	Signature []byte `json:"signature"`
}

// RepKey returns the DHT key for a peer's reputation.
func RepKey(pid peer.ID) string {
	return RepKeyPrefix + pid.String()
}

// SignRepSnapshot creates and signs a reputation snapshot.
func SignRepSnapshot(priv crypto.PrivKey, snap RepSnapshot) ([]byte, error) {
	if snap.Timestamp == 0 {
		snap.Timestamp = time.Now().Unix()
	}
	payload, err := json.Marshal(snap)
	if err != nil {
		return nil, err
	}
	sig, err := priv.Sign(payload)
	if err != nil {
		return nil, err
	}
	wire := SignedRep{Payload: payload, Signature: sig}
	return json.Marshal(wire)
}

// VerifyRepSnapshot verifies a signed reputation snapshot.
func VerifyRepSnapshot(key string, value []byte) (*RepSnapshot, error) {
	var wire SignedRep
	if err := json.Unmarshal(value, &wire); err != nil {
		return nil, fmt.Errorf("unmarshal wire: %w", err)
	}
	var snap RepSnapshot
	if err := json.Unmarshal(wire.Payload, &snap); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}
	expectedKey := RepKeyPrefix + snap.PeerID
	if key != expectedKey {
		return nil, fmt.Errorf("key mismatch: got %q, expected %q", key, expectedKey)
	}
	pid, err := peer.Decode(snap.PeerID)
	if err != nil {
		return nil, fmt.Errorf("invalid peer ID: %w", err)
	}
	pubKey, err := pid.ExtractPublicKey()
	if err != nil {
		return nil, fmt.Errorf("extract pubkey: %w", err)
	}
	ok, err := pubKey.Verify(wire.Payload, wire.Signature)
	if err != nil || !ok {
		return nil, errors.New("invalid signature")
	}
	return &snap, nil
}

// repValidator implements record.Validator for the reputation DHT namespace.
type repValidator struct{}

func (v repValidator) Validate(key string, value []byte) error {
	_, err := VerifyRepSnapshot(key, value)
	return err
}

func (v repValidator) Select(key string, vals [][]byte) (int, error) {
	if len(vals) == 0 {
		return 0, errors.New("no values")
	}
	bestIdx := 0
	var bestTS int64
	for i, val := range vals {
		snap, err := VerifyRepSnapshot(key, val)
		if err != nil {
			continue
		}
		if snap.Timestamp > bestTS {
			bestTS = snap.Timestamp
			bestIdx = i
		}
	}
	return bestIdx, nil
}

// NewRepValidator returns a validator for the "clawnet-rep" DHT namespace.
func NewRepValidator() repValidator {
	return repValidator{}
}

// PublishReputation signs and puts a reputation snapshot into the DHT.
func (n *Node) PublishReputation(ctx context.Context, snap RepSnapshot) error {
	priv := n.Host.Peerstore().PrivKey(n.Host.ID())
	if priv == nil {
		return errors.New("private key not available")
	}
	data, err := SignRepSnapshot(priv, snap)
	if err != nil {
		return fmt.Errorf("sign reputation: %w", err)
	}
	key := RepKey(n.Host.ID())
	putCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return n.DHT.PutValue(putCtx, key, data)
}

// LookupReputation fetches a peer's reputation snapshot from the DHT.
func (n *Node) LookupReputation(ctx context.Context, pid peer.ID) (*RepSnapshot, error) {
	key := RepKey(pid)
	getCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	value, err := n.DHT.GetValue(getCtx, key)
	if err != nil {
		return nil, err
	}
	return VerifyRepSnapshot(key, value)
}
