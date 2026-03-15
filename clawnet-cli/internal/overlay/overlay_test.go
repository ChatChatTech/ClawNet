package overlay

import (
	"crypto/ed25519"
	"testing"
	"time"
)

func TestScoreToBucket(t *testing.T) {
	tests := []struct {
		score float64
		want  ReputationBucket
	}{
		{0, BucketLow},
		{10, BucketLow},
		{24.9, BucketLow},
		{25, BucketNormal},
		{49.9, BucketNormal},
		{50, BucketGood},
		{74.9, BucketGood},
		{75, BucketElite},
		{100, BucketElite},
		{200, BucketElite},
	}
	for _, tt := range tests {
		got := ScoreToBucket(tt.score)
		if got != tt.want {
			t.Errorf("ScoreToBucket(%v) = %v, want %v", tt.score, got, tt.want)
		}
	}
}

func TestReputationBloomTransform(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)

	// Test with different scores
	for _, score := range []float64{0, 25, 50, 75, 100} {
		xform := ReputationBloomTransform(func() float64 { return score })
		out := xform(pub)

		if len(out) != ed25519.PublicKeySize {
			t.Fatalf("output size %d, want %d", len(out), ed25519.PublicKeySize)
		}

		// First 28 bytes should match original key
		for i := 0; i < 28; i++ {
			if out[i] != pub[i] {
				t.Errorf("byte %d: got %x, want %x", i, out[i], pub[i])
			}
		}

		// Byte 28 should be the bucket
		expected := ScoreToBucket(score)
		if out[28] != byte(expected) {
			t.Errorf("score=%.0f: bucket byte = %d, want %d", score, out[28], expected)
		}

		// Bytes 30-31 should be zero (reserved)
		if out[30] != 0 || out[31] != 0 {
			t.Errorf("reserved bytes not zero: %x %x", out[30], out[31])
		}
	}
}

func TestReputationBloomTransformDifferentBuckets(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)

	// Different scores should produce different transforms
	lowXform := ReputationBloomTransform(func() float64 { return 10 })
	highXform := ReputationBloomTransform(func() float64 { return 80 })

	low := lowXform(pub)
	high := highXform(pub)

	// They should differ at byte 28
	if low[28] == high[28] {
		t.Error("low and high score should produce different bloom keys")
	}
}

func TestPathBridgeThrottle(t *testing.T) {
	bridge := NewPathBridge(nil, nil, 100*time.Millisecond)

	pub, _, _ := ed25519.GenerateKey(nil)

	// With nil host, OnPathNotify returns early (no crash)
	bridge.OnPathNotify(pub)

	// Since host is nil, the key is NOT added to recent (early return)
	bridge.mu.Lock()
	if len(bridge.recent) != 0 {
		t.Errorf("recent map size = %d, want 0 (nil host → early return)", len(bridge.recent))
	}
	bridge.mu.Unlock()

	// Verify the throttle duration was set correctly
	if bridge.throttle != 100*time.Millisecond {
		t.Errorf("throttle = %v, want 100ms", bridge.throttle)
	}
}

func TestPathBridgeCleanup(t *testing.T) {
	bridge := NewPathBridge(nil, nil, time.Millisecond)

	pub, _, _ := ed25519.GenerateKey(nil)

	// Manually set an old timestamp
	keyHex := "test_key"
	bridge.mu.Lock()
	bridge.recent[keyHex] = time.Now().Add(-10 * time.Minute)
	bridge.recent["fresh_key"] = time.Now()
	bridge.mu.Unlock()

	// Only test that the key was set
	_ = pub

	bridge.Cleanup()

	bridge.mu.Lock()
	if _, ok := bridge.recent[keyHex]; ok {
		t.Error("stale entry should have been cleaned up")
	}
	if _, ok := bridge.recent["fresh_key"]; !ok {
		t.Error("fresh entry should remain")
	}
	bridge.mu.Unlock()
}

func TestMsgTypeDMConstant(t *testing.T) {
	if MsgTypeDM != 0x01 {
		t.Errorf("MsgTypeDM = %x, want 0x01", MsgTypeDM)
	}
}
