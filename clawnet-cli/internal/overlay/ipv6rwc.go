package overlay

// IPv6 read-write-close bridge between a TUN device and the Ironwood overlay.
// Before molt: only allows traffic from/to known ClawNet peers.
// After molt: full interoperability with the entire overlay mesh.

import (
	"crypto/ed25519"
	"fmt"
	"sync"
	"time"

	"github.com/Arceliar/ironwood/types"
)

const keyStoreTimeout = 2 * time.Minute

// keyStore maps overlay IPv6 addresses (200::/7) to Ed25519 public keys.
// On cache miss, triggers a SendLookup and buffers one packet per destination.
type keyStore struct {
	mu        sync.Mutex
	addrToKey map[[16]byte]*keyEntry
	addrBuf   map[[16]byte]*pendingBuf // buffered packet awaiting key discovery
}

type keyEntry struct {
	key     ed25519.PublicKey
	timeout *time.Timer
}

type pendingBuf struct {
	packet  []byte
	timeout *time.Timer
}

func newKeyStore() *keyStore {
	return &keyStore{
		addrToKey: make(map[[16]byte]*keyEntry),
		addrBuf:   make(map[[16]byte]*pendingBuf),
	}
}

// update registers a key→address mapping (called on PathNotify and received packets).
// If a buffered packet exists for this address, returns it for sending.
func (ks *keyStore) update(key ed25519.PublicKey) (buffered []byte, addr [16]byte) {
	addr = OverlayAddress(key)
	dup := make(ed25519.PublicKey, len(key))
	copy(dup, key)

	ks.mu.Lock()
	defer ks.mu.Unlock()

	if entry, exists := ks.addrToKey[addr]; exists {
		entry.timeout.Stop()
		entry.key = dup
		entry.timeout = time.AfterFunc(keyStoreTimeout, func() {
			ks.mu.Lock()
			delete(ks.addrToKey, addr)
			ks.mu.Unlock()
		})
	} else {
		ks.addrToKey[addr] = &keyEntry{
			key: dup,
			timeout: time.AfterFunc(keyStoreTimeout, func() {
				ks.mu.Lock()
				delete(ks.addrToKey, addr)
				ks.mu.Unlock()
			}),
		}
	}

	// Flush buffered packet if any
	if buf := ks.addrBuf[addr]; buf != nil {
		buffered = buf.packet
		buf.timeout.Stop()
		delete(ks.addrBuf, addr)
	}
	return
}

func (ks *keyStore) lookup(addr [16]byte) (ed25519.PublicKey, bool) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	if entry, ok := ks.addrToKey[addr]; ok {
		entry.timeout.Reset(keyStoreTimeout)
		return entry.key, true
	}
	return nil, false
}

// bufferPacket stores one packet per destination while awaiting key discovery.
func (ks *keyStore) bufferPacket(addr [16]byte, packet []byte) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	if buf := ks.addrBuf[addr]; buf != nil {
		buf.packet = packet
		buf.timeout.Reset(keyStoreTimeout)
	} else {
		ks.addrBuf[addr] = &pendingBuf{
			packet: packet,
			timeout: time.AfterFunc(keyStoreTimeout, func() {
				ks.mu.Lock()
				delete(ks.addrBuf, addr)
				ks.mu.Unlock()
			}),
		}
	}
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

// onPathNotify is called when Ironwood discovers a path to a new key.
// Updates the keyStore and flushes any buffered packet for that key's address.
func (r *IPv6RWC) onPathNotify(key ed25519.PublicKey) {
	buffered, _ := r.keyStore.update(key)
	if buffered != nil {
		_, _ = r.transport.pc.WriteTo(buffered, types.Addr(key))
	}
}

// tunReadLoop reads IPv6 packets from TUN and sends them via the overlay.
func (r *IPv6RWC) tunReadLoop() {
	fmt.Println("[tun] tunReadLoop started")
	buf := make([]byte, 65535)
	for {
		n, err := r.tun.Read(buf)
		if err != nil {
			fmt.Printf("[tun] Read error: %v\n", err)
			return
		}
		if n < 40 || (buf[0]>>4) != 6 {
			fmt.Printf("[tun] non-IPv6: n=%d\n", n)
			continue
		}

		fmt.Printf("[tun] read %d bytes from TUN\n", n)

		// Extract destination IPv6 address (bytes 24-39 in IPv6 header)
		var destAddr [16]byte
		copy(destAddr[:], buf[24:40])

		// Must be in 200::/7 range (first byte & 0xFE == 0x02)
		if (destAddr[0] & 0xFE) != 0x02 {
			fmt.Printf("[tun] skip: dest[0]=%02x not in 200::/7\n", destAddr[0])
			continue
		}

		destKey, ok := r.keyStore.lookup(destAddr)
		if !ok {
			// Key unknown — trigger bloom-filter lookup and buffer packet.
			// PartialKeyForAddr reverse-derives enough of the key from the
			// address for the SubnetKeyTransform bloom match to succeed.
			partial := PartialKeyForAddr(destAddr)
			fmt.Printf("[tun] SendLookup dest=%x partial=%x\n", destAddr[:4], partial[:8])
			r.transport.pc.PacketConn.SendLookup(partial)
			packet := make([]byte, n)
			copy(packet, buf[:n])
			r.keyStore.bufferPacket(destAddr, packet)
			continue
		}
		fmt.Printf("[tun] send %d bytes to key=%x\n", n, destKey[:8])

		// Not molted → only send to known ClawNet peers
		if !r.transport.IsMolted() && !r.transport.IsClawPeer(destKey) {
			fmt.Printf("[tun] blocked: not ClawPeer\n")
			continue
		}

		packet := make([]byte, n)
		copy(packet, buf[:n])
		_, _ = r.transport.pc.WriteTo(packet, types.Addr(destKey))
	}
}
