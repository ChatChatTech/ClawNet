package overlay

import (
	"crypto/ed25519"
	"encoding/binary"
)

// ReputationBucket represents discrete reputation tiers for bloom filter encoding.
type ReputationBucket uint8

const (
	BucketLow    ReputationBucket = 0 // score < 25
	BucketNormal ReputationBucket = 1 // 25 <= score < 50
	BucketGood   ReputationBucket = 2 // 50 <= score < 75
	BucketElite  ReputationBucket = 3 // score >= 75
)

// ScoreToBucket converts a float64 reputation score to a discrete bucket.
func ScoreToBucket(score float64) ReputationBucket {
	switch {
	case score >= 75:
		return BucketElite
	case score >= 50:
		return BucketGood
	case score >= 25:
		return BucketNormal
	default:
		return BucketLow
	}
}

// ReputationBloomTransform creates a bloom transform function that encodes
// reputation information into the last 4 bytes of the public key for
// Ironwood bloom filter lookups. This causes higher-reputation nodes to
// have distinct bloom signatures per tier, enabling reputation-aware routing.
//
// Format: pubkey[:28] + bucket(1B) + reserved(3B)
//
// getScore is called to fetch the local node's current reputation score.
func ReputationBloomTransform(getScore func() float64) func(ed25519.PublicKey) ed25519.PublicKey {
	return func(key ed25519.PublicKey) ed25519.PublicKey {
		if len(key) != ed25519.PublicKeySize {
			return key
		}
		score := getScore()
		bucket := ScoreToBucket(score)

		out := make(ed25519.PublicKey, ed25519.PublicKeySize)
		copy(out, key[:28])
		out[28] = byte(bucket)
		// bytes 29-31 reserved (zero)
		binary.BigEndian.PutUint16(out[30:32], 0)
		return out
	}
}
