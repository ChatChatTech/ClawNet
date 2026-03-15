package overlay

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	libcrypto "github.com/libp2p/go-libp2p/core/crypto"
)

// PathBridge connects Ironwood path discovery to libp2p peer connections.
// When Ironwood discovers a new path to a public key, PathBridge attempts
// to establish a libp2p connection to that peer.
type PathBridge struct {
	host      host.Host
	transport *Transport

	mu       sync.Mutex
	recent   map[string]time.Time // hex(pubkey) -> last attempt time
	throttle time.Duration
}

// NewPathBridge creates a bridge that triggers libp2p connections
// when Ironwood discovers paths to new peers.
func NewPathBridge(h host.Host, t *Transport, throttle time.Duration) *PathBridge {
	if throttle <= 0 {
		throttle = time.Second
	}
	return &PathBridge{
		host:      h,
		transport: t,
		recent:    make(map[string]time.Time),
		throttle:  throttle,
	}
}

// OnPathNotify is the callback for Ironwood's WithPathNotify option.
// It converts the discovered Ed25519 public key to a libp2p peer ID
// and attempts a connection if not already connected.
func (b *PathBridge) OnPathNotify(key ed25519.PublicKey) {
	if b.host == nil {
		return
	}

	keyHex := hex.EncodeToString(key[:8])

	// Throttle: skip if we attempted this peer recently
	b.mu.Lock()
	if last, ok := b.recent[keyHex]; ok && time.Since(last) < b.throttle {
		b.mu.Unlock()
		return
	}
	b.recent[keyHex] = time.Now()
	b.mu.Unlock()

	// Convert Ed25519 pubkey to libp2p PeerID
	libPub, err := libcrypto.UnmarshalEd25519PublicKey(key)
	if err != nil {
		return
	}
	pid, err := peer.IDFromPublicKey(libPub)
	if err != nil {
		return
	}

	// Skip if already connected via libp2p
	if b.host.Network().Connectedness(pid) == 1 { // Connected
		return
	}

	// Attempt connection in background with short timeout
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// We don't have the peer's multiaddr, but libp2p may find it
		// via DHT or peerstore if we've seen this peer before.
		addrInfo := b.host.Peerstore().PeerInfo(pid)
		if len(addrInfo.Addrs) == 0 {
			return // No known addresses, can't connect
		}

		if err := b.host.Connect(ctx, addrInfo); err != nil {
			fmt.Printf("[overlay/bridge] libp2p connect to %s failed: %v\n", keyHex, err)
		} else {
			fmt.Printf("[overlay/bridge] libp2p connected to %s via path discovery\n", keyHex)
		}
	}()
}

// Cleanup removes stale entries from the throttle map.
// Should be called periodically (e.g., every 5 minutes).
func (b *PathBridge) Cleanup() {
	b.mu.Lock()
	defer b.mu.Unlock()
	cutoff := time.Now().Add(-5 * time.Minute)
	for k, t := range b.recent {
		if t.Before(cutoff) {
			delete(b.recent, k)
		}
	}
}

// SetTransport sets the overlay transport reference (allows deferred init).
func (b *PathBridge) SetTransport(t *Transport) {
	b.transport = t
}
