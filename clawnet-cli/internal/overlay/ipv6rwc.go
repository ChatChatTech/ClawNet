package overlay

// IPv6 read-write-close bridge between a TUN device and the Ironwood overlay.
// Before molt: only allows traffic from/to known ClawNet peers.
// After molt: full interoperability with the entire overlay mesh.

import (
	"crypto/ed25519"
	"fmt"
	"sync"

	"github.com/Arceliar/ironwood/types"
)

// keyStore maps overlay IPv6 addresses (200::/7) to Ed25519 public keys.
// Populated from received packets and known peers.
type keyStore struct {
	mu        sync.RWMutex
	addrToKey map[[16]byte]ed25519.PublicKey
}

func newKeyStore() *keyStore {
	return &keyStore{
		addrToKey: make(map[[16]byte]ed25519.PublicKey),
	}
}

func (ks *keyStore) update(key ed25519.PublicKey) {
	addr := OverlayAddress(key)
	dup := make(ed25519.PublicKey, len(key))
	copy(dup, key)
	ks.mu.Lock()
	ks.addrToKey[addr] = dup
	ks.mu.Unlock()
}

func (ks *keyStore) lookup(addr [16]byte) (ed25519.PublicKey, bool) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	key, ok := ks.addrToKey[addr]
	return key, ok
}

// IPv6RWC bridges IPv6 traffic between a TUN device and the Ironwood overlay.
type IPv6RWC struct {
	transport *Transport
	tun       *TUNDevice
	keyStore  *keyStore
}

func newIPv6RWC(t *Transport, tun *TUNDevice) *IPv6RWC {
	rwc := &IPv6RWC{
		transport: t,
		tun:       tun,
		keyStore:  newKeyStore(),
	}
	// Pre-populate with our own key
	rwc.keyStore.update(t.PublicKey())
	return rwc
}

// handleOverlayIPv6 processes an IPv6 packet received from the overlay network.
func (r *IPv6RWC) handleOverlayIPv6(from ed25519.PublicKey, data []byte) {
	if len(data) < 40 || (data[0]>>4) != 6 {
		return
	}

	// Learn source key → address mapping
	r.keyStore.update(from)

	// Not molted → only accept from known ClawNet peers
	if !r.transport.IsMolted() && !r.transport.IsClawPeer(from) {
		return
	}

	if _, err := r.tun.Write(data); err != nil {
		fmt.Printf("[tun] write error: %v\n", err)
	}
}

// tunReadLoop reads IPv6 packets from TUN and sends them via the overlay.
func (r *IPv6RWC) tunReadLoop() {
	buf := make([]byte, 65535)
	for {
		n, err := r.tun.Read(buf)
		if err != nil {
			return
		}
		if n < 40 || (buf[0]>>4) != 6 {
			continue
		}

		// Extract destination IPv6 address (bytes 24-39 in IPv6 header)
		var destAddr [16]byte
		copy(destAddr[:], buf[24:40])

		// Must be in 200::/7 range (first byte & 0xFE == 0x02)
		if (destAddr[0] & 0xFE) != 0x02 {
			continue
		}

		destKey, ok := r.keyStore.lookup(destAddr)
		if !ok {
			continue // unknown destination — need to discover it first
		}

		// Not molted → only send to known ClawNet peers
		if !r.transport.IsMolted() && !r.transport.IsClawPeer(destKey) {
			continue
		}

		packet := make([]byte, n)
		copy(packet, buf[:n])
		_, _ = r.transport.pc.WriteTo(packet, types.Addr(destKey))
	}
}
