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
	// TxnKeyPrefix is the DHT namespace for signed credit transactions.
	TxnKeyPrefix = "/clawnet-txn/"
)

// TxnRecord is the canonical representation of a credit transaction.
type TxnRecord struct {
	TxnID     string  `json:"txn_id"`
	From      string  `json:"from"`
	To        string  `json:"to"`
	Amount    float64 `json:"amount"`
	Reason    string  `json:"reason"`
	RefID     string  `json:"ref_id,omitempty"`
	Timestamp int64   `json:"timestamp"`
}

// SignedTxn is the wire format stored in DHT: payload + dual signatures.
type SignedTxn struct {
	Payload      []byte `json:"payload"`       // JSON-encoded TxnRecord
	SenderSig    []byte `json:"sender_sig"`    // sender's Ed25519 signature
	ReceiverSig  []byte `json:"receiver_sig"`  // receiver's Ed25519 signature (may be nil initially)
}

// TxnKey returns the DHT key for a transaction ID.
func TxnKey(txnID string) string {
	return TxnKeyPrefix + txnID
}

// SignTxn creates and signs a transaction record as the sender.
func SignTxn(priv crypto.PrivKey, rec TxnRecord) ([]byte, error) {
	if rec.Timestamp == 0 {
		rec.Timestamp = time.Now().Unix()
	}
	payload, err := json.Marshal(rec)
	if err != nil {
		return nil, err
	}
	sig, err := priv.Sign(payload)
	if err != nil {
		return nil, err
	}
	wire := SignedTxn{Payload: payload, SenderSig: sig}
	return json.Marshal(wire)
}

// CounterSignTxn adds the receiver's signature to an existing signed transaction.
func CounterSignTxn(priv crypto.PrivKey, data []byte) ([]byte, error) {
	var wire SignedTxn
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	sig, err := priv.Sign(wire.Payload)
	if err != nil {
		return nil, err
	}
	wire.ReceiverSig = sig
	return json.Marshal(wire)
}

// VerifyTxn verifies the sender's signature (and receiver's if present).
func VerifyTxn(data []byte) (*TxnRecord, bool, error) {
	var wire SignedTxn
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, false, fmt.Errorf("unmarshal wire: %w", err)
	}
	var rec TxnRecord
	if err := json.Unmarshal(wire.Payload, &rec); err != nil {
		return nil, false, fmt.Errorf("unmarshal payload: %w", err)
	}

	// Verify sender signature
	fromPID, err := peer.Decode(rec.From)
	if err != nil {
		return nil, false, fmt.Errorf("invalid sender peer ID: %w", err)
	}
	fromPub, err := fromPID.ExtractPublicKey()
	if err != nil {
		return nil, false, fmt.Errorf("extract sender pubkey: %w", err)
	}
	ok, err := fromPub.Verify(wire.Payload, wire.SenderSig)
	if err != nil || !ok {
		return nil, false, errors.New("invalid sender signature")
	}

	// Verify receiver signature if present
	fullyVerified := false
	if len(wire.ReceiverSig) > 0 {
		toPID, err := peer.Decode(rec.To)
		if err != nil {
			return &rec, false, nil
		}
		toPub, err := toPID.ExtractPublicKey()
		if err != nil {
			return &rec, false, nil
		}
		ok, err := toPub.Verify(wire.Payload, wire.ReceiverSig)
		if err == nil && ok {
			fullyVerified = true
		}
	}

	return &rec, fullyVerified, nil
}

// txnValidator implements record.Validator for the txn DHT namespace.
type txnValidator struct{}

func (v txnValidator) Validate(key string, value []byte) error {
	rec, _, err := VerifyTxn(value)
	if err != nil {
		return err
	}
	expectedKey := TxnKeyPrefix + rec.TxnID
	if key != expectedKey {
		return fmt.Errorf("key mismatch: got %q, expected %q", key, expectedKey)
	}
	return nil
}

func (v txnValidator) Select(key string, vals [][]byte) (int, error) {
	if len(vals) == 0 {
		return 0, errors.New("no values")
	}
	// Prefer fully-signed over sender-only
	for i, val := range vals {
		_, full, err := VerifyTxn(val)
		if err == nil && full {
			return i, nil
		}
	}
	return 0, nil
}

// NewTxnValidator returns a validator for the "clawnet-txn" DHT namespace.
func NewTxnValidator() txnValidator {
	return txnValidator{}
}

// PublishTxn puts a signed transaction into the DHT.
func (n *Node) PublishTxn(ctx context.Context, txnData []byte, txnID string) error {
	key := TxnKey(txnID)
	putCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return n.DHT.PutValue(putCtx, key, txnData)
}
