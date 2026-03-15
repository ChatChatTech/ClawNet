package p2p

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
)

const (
	// ProfileKeyPrefix is the DHT key namespace for profile records.
	ProfileKeyPrefix = "/clawnet-profile/"
)

// ProfileRecord is the signed envelope stored in the DHT.
type ProfileRecord struct {
	PeerID    string         `json:"peer_id"`
	Profile   config.Profile `json:"profile"`
	Timestamp int64          `json:"timestamp"`
}

// ProfileRecordWire is the wire format: JSON payload + 64-byte Ed25519 signature.
type ProfileRecordWire struct {
	Payload   []byte `json:"payload"`
	Signature []byte `json:"signature"`
}

// ProfileKey returns the DHT key for a given peer ID.
func ProfileKey(pid peer.ID) string {
	return ProfileKeyPrefix + pid.String()
}

// SignProfileRecord serialises and signs a profile record.
func SignProfileRecord(priv crypto.PrivKey, pid peer.ID, profile *config.Profile) ([]byte, error) {
	rec := ProfileRecord{
		PeerID:    pid.String(),
		Profile:   *profile,
		Timestamp: time.Now().Unix(),
	}
	payload, err := json.Marshal(rec)
	if err != nil {
		return nil, err
	}
	sig, err := priv.Sign(payload)
	if err != nil {
		return nil, err
	}
	wire := ProfileRecordWire{Payload: payload, Signature: sig}
	return json.Marshal(wire)
}

// VerifyProfileRecord verifies and unmarshals a profile record.
// It checks that the signature matches the peer ID embedded in the record.
func VerifyProfileRecord(key string, value []byte) (*ProfileRecord, error) {
	var wire ProfileRecordWire
	if err := json.Unmarshal(value, &wire); err != nil {
		return nil, fmt.Errorf("unmarshal wire: %w", err)
	}
	var rec ProfileRecord
	if err := json.Unmarshal(wire.Payload, &rec); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}

	// Verify key matches record's peer ID
	expectedKey := ProfileKeyPrefix + rec.PeerID
	if key != expectedKey {
		return nil, fmt.Errorf("key mismatch: got %q, record says %q", key, expectedKey)
	}

	pid, err := peer.Decode(rec.PeerID)
	if err != nil {
		return nil, fmt.Errorf("invalid peer ID in record: %w", err)
	}

	// Extract public key from peer ID
	pubKey, err := pid.ExtractPublicKey()
	if err != nil {
		return nil, fmt.Errorf("extract pubkey: %w", err)
	}

	ok, err := pubKey.Verify(wire.Payload, wire.Signature)
	if err != nil {
		return nil, fmt.Errorf("verify signature: %w", err)
	}
	if !ok {
		return nil, errors.New("invalid signature")
	}

	return &rec, nil
}

// profileValidator implements record.Validator for the profile namespace.
type profileValidator struct{}

func (v profileValidator) Validate(key string, value []byte) error {
	_, err := VerifyProfileRecord(key, value)
	return err
}

func (v profileValidator) Select(key string, vals [][]byte) (int, error) {
	if len(vals) == 0 {
		return 0, errors.New("no values")
	}
	bestIdx := 0
	var bestTS int64
	for i, val := range vals {
		rec, err := VerifyProfileRecord(key, val)
		if err != nil {
			continue
		}
		if rec.Timestamp > bestTS {
			bestTS = rec.Timestamp
			bestIdx = i
		}
	}
	return bestIdx, nil
}

// NewProfileValidator returns a validator for the "clawnet-profile" DHT namespace.
func NewProfileValidator() profileValidator {
	return profileValidator{}
}

// ── Node methods for publishing / looking up profiles ──

// PublishProfile signs and puts the profile into the DHT.
func (n *Node) PublishProfile(ctx context.Context, profile *config.Profile) error {
	priv := n.Host.Peerstore().PrivKey(n.Host.ID())
	if priv == nil {
		return errors.New("private key not available")
	}
	data, err := SignProfileRecord(priv, n.Host.ID(), profile)
	if err != nil {
		return fmt.Errorf("sign profile: %w", err)
	}
	key := ProfileKey(n.Host.ID())
	putCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return n.DHT.PutValue(putCtx, key, data)
}

// LookupProfile fetches a peer's profile from the DHT.
func (n *Node) LookupProfile(ctx context.Context, pid peer.ID) (*ProfileRecord, error) {
	key := ProfileKey(pid)
	getCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	value, err := n.DHT.GetValue(getCtx, key)
	if err != nil {
		return nil, err
	}
	return VerifyProfileRecord(key, value)
}

// tsFromBytes extracts an int64 from 8 big-endian bytes.
func tsFromBytes(b []byte) int64 {
	return int64(binary.BigEndian.Uint64(b))
}

// tsToBytes converts int64 to 8 big-endian bytes.
func tsToBytes(ts int64) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, ts)
	return buf.Bytes()
}
