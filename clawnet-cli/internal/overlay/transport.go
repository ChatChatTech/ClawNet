package overlay

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/Arceliar/ironwood/encrypted"
	"github.com/Arceliar/ironwood/network"
	"github.com/Arceliar/ironwood/types"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	// MsgTypeDM is the prefix byte for DM messages sent over the overlay.
	MsgTypeDM byte = 0x01
)

// Transport manages an Ironwood overlay network with link-level connection management.
// Wraps encrypted.PacketConn with a links subsystem that handles TCP/TLS connections,
// per-link byte counting, exponential backoff, and URI-based peer addressing.
type Transport struct {
	pc          *encrypted.PacketConn
	privKey     ed25519.PrivateKey
	links       links
	listenPort  int
	staticPeers []string
	ctx         context.Context
	cancel      context.CancelFunc

	// PeerMgr for optional health monitoring and disk persistence
	PeerMgr *PeerManager

	// onMessage callback for received datagrams
	onMessage func(from ed25519.PublicKey, data []byte)

	// TUN device and IPv6 bridge
	tun     *TUNDevice
	ipv6rwc *IPv6RWC

	// Known ClawNet peer keys (populated from handshakes + libp2p sync)
	clawPeers sync.Map // [32]byte → struct{}

	mu      sync.Mutex
	closed  bool
	molted  bool // true = full mesh interop, false = ClawNet-only (default)
}

// NewTransport creates an Ironwood overlay transport with link management.
// priv is the libp2p Ed25519 private key (shared identity).
// staticPeers and bootstrapPeers are peer URIs ("tcp://host:port", "tls://host:port")
// or legacy "host:port" format (auto-normalized to tcp://).
// opts are Ironwood network.Option values (WithPathNotify, WithBloomTransform, etc.).
func NewTransport(priv crypto.PrivKey, listenPort int, staticPeers, bootstrapPeers []string, opts ...network.Option) (*Transport, error) {
	rawKey, err := priv.Raw()
	if err != nil {
		return nil, fmt.Errorf("extract private key: %w", err)
	}
	// libp2p Ed25519 Raw() returns 64 bytes: seed(32) + pubkey(32)
	edPrivKey := ed25519.PrivateKey(rawKey)

	pc, err := encrypted.NewPacketConn(edPrivKey, opts...)
	if err != nil {
		return nil, fmt.Errorf("create overlay packetconn: %w", err)
	}

	// Merge static, bootstrap, and default overlay public peers, deduplicating
	allPeers := mergeUnique(staticPeers, bootstrapPeers)
	allPeers = mergeUnique(allPeers, DefaultOverlayPeers)

	// Normalize: ensure URI scheme present (backward compat with "host:port")
	for i, p := range allPeers {
		if !strings.Contains(p, "://") {
			allPeers[i] = "tcp://" + p
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	t := &Transport{
		pc:          pc,
		privKey:     edPrivKey,
		listenPort:  listenPort,
		staticPeers: allPeers,
		ctx:         ctx,
		cancel:      cancel,
	}

	// Initialize the links subsystem (starts rate-tracking goroutine)
	t.links.init(t)

	return t, nil
}

// mergeUnique combines two string slices, removing duplicates.
func mergeUnique(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	var out []string
	for _, s := range a {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	for _, s := range b {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// Run starts the overlay transport. It opens a TCP listener, adds all static peers
// as persistent links (with built-in exponential backoff), and runs the receive loop.
// Blocks until ctx is cancelled.
func (t *Transport) Run(ctx context.Context) {
	pubKey := t.privKey.Public().(ed25519.PublicKey)
	fmt.Printf("[overlay] public key: %s\n", hex.EncodeToString(pubKey[:8]))

	// Start TCP listener via links subsystem
	if t.listenPort > 0 {
		listenURI := fmt.Sprintf("tcp://:%d", t.listenPort)
		if _, err := t.links.listen(listenURI); err != nil {
			fmt.Printf("[overlay] listen on :%d failed: %v\n", t.listenPort, err)
		} else {
			fmt.Printf("[overlay] listening on :%d\n", t.listenPort)
		}
	}

	// Add all peers as persistent links (links subsystem handles backoff)
	for _, uri := range t.staticPeers {
		_ = t.links.add(uri, linkTypePersistent) // errors non-fatal (duplicate, bad URI)
	}

	// Start receive loop
	go t.receiveLoop()

	select {
	case <-ctx.Done():
	case <-t.ctx.Done():
	}
	t.Close()
}

// receiveLoop reads datagrams from the Ironwood overlay network.
// Routes IPv6 packets (first nibble = 6) to TUN if active,
// all other messages to the application handler.
func (t *Transport) receiveLoop() {
	buf := make([]byte, 65535)
	for {
		select {
		case <-t.ctx.Done():
			return
		default:
		}

		n, addr, err := t.pc.ReadFrom(buf)
		if err != nil || n == 0 {
			continue
		}

		a, ok := addr.(types.Addr)
		if !ok {
			continue
		}
		from := ed25519.PublicKey(a)

		// IPv6 packets → TUN device
		if n > 0 && (buf[0]>>4) == 6 && t.ipv6rwc != nil {
			data := make([]byte, n)
			copy(data, buf[:n])
			t.ipv6rwc.handleOverlayIPv6(from, data)
			continue
		}

		// Application messages → handler
		if t.onMessage != nil {
			data := make([]byte, n)
			copy(data, buf[:n])
			t.onMessage(from, data)
		}
	}
}

// SetMessageHandler sets the callback for received messages.
func (t *Transport) SetMessageHandler(fn func(from ed25519.PublicKey, data []byte)) {
	t.onMessage = fn
}

// Send sends a datagram to a peer via the Ironwood overlay.
func (t *Transport) Send(ctx context.Context, pid peer.ID, data []byte) error {
	pub, err := pid.ExtractPublicKey()
	if err != nil {
		return fmt.Errorf("extract public key from peer ID: %w", err)
	}
	rawPub, err := pub.Raw()
	if err != nil {
		return fmt.Errorf("extract raw public key: %w", err)
	}
	if len(rawPub) != ed25519.PublicKeySize {
		return fmt.Errorf("unexpected public key size: %d", len(rawPub))
	}
	destAddr := types.Addr(rawPub)
	_, err = t.pc.WriteTo(data, destAddr)
	return err
}

// AddPeer adds a persistent peer by URI. The link auto-reconnects with backoff.
func (t *Transport) AddPeer(uri string) error {
	return t.links.add(uri, linkTypePersistent)
}

// RemovePeer removes a peer by URI and stops reconnection.
func (t *Transport) RemovePeer(uri string) error {
	return t.links.remove(uri)
}

// RetryPeersNow kicks all links to attempt reconnection immediately.
func (t *Transport) RetryPeersNow() {
	t.links.RetryPeersNow()
}

// Molt enables full mesh interoperability — any overlay peer (including
// non-ClawNet clients) can communicate via IPv6 through TUN.
func (t *Transport) Molt() {
	t.mu.Lock()
	if t.molted {
		t.mu.Unlock()
		return
	}
	t.molted = true
	t.mu.Unlock()

	fmt.Println("[overlay] molted — full mesh interoperability enabled")
}

// Unmolt returns to ClawNet-only mode — only known ClawNet peers can
// communicate via IPv6 through TUN.
func (t *Transport) Unmolt() {
	t.mu.Lock()
	if !t.molted {
		t.mu.Unlock()
		return
	}
	t.molted = false
	t.mu.Unlock()

	fmt.Println("[overlay] unmolted — ClawNet-only mode")
}

// IsMolted returns whether the transport allows full mesh interop.
func (t *Transport) IsMolted() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.molted
}

// OverlayAddress returns the ClawNet 200::/7 IPv6 address derived from
// this node's overlay Ed25519 public key.
func (t *Transport) OverlayAddress() string {
	return FormatOverlayAddress(t.PublicKey())
}

// OverlaySubnet returns the overlay /64 subnet prefix.
func (t *Transport) OverlaySubnet() string {
	return FormatOverlaySubnet(t.PublicKey())
}

// PeerCount returns the number of currently connected overlay peers.
func (t *Transport) PeerCount() int {
	peers := t.links.GetPeers()
	count := 0
	for _, p := range peers {
		if p.Up {
			count++
		}
	}
	return count
}

// ConnectedPeer holds info about a directly connected overlay peer.
type ConnectedPeer struct {
	KeyHex     string // first 8 bytes of ed25519 pubkey, hex-encoded
	RemoteAddr string // TCP "ip:port"
}

// GetConnectedPeers returns all directly connected overlay peers with TCP addresses.
func (t *Transport) GetConnectedPeers() []ConnectedPeer {
	peers := t.links.GetPeers()
	out := make([]ConnectedPeer, 0)
	for _, p := range peers {
		if p.Up && p.Key != "" {
			keyHex := p.Key
			if len(keyHex) > 16 {
				keyHex = keyHex[:16]
			}
			out = append(out, ConnectedPeer{
				KeyHex:     keyHex,
				RemoteAddr: p.RemoteAddr,
			})
		}
	}
	return out
}

// GetPeers returns rich peer info merging link-layer and ironwood stats.
func (t *Transport) GetPeers() []PeerInfo {
	return t.links.GetPeers()
}

// PublicKey returns the node's Ed25519 public key in the overlay network.
func (t *Transport) PublicKey() ed25519.PublicKey {
	return t.privKey.Public().(ed25519.PublicKey)
}

// Close shuts down the overlay transport and TUN device.
func (t *Transport) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	t.closed = true
	t.cancel()
	t.links.shutdown()
	t.pc.Close()
	if t.tun != nil {
		t.tun.Close()
	}
}

// ── TUN + Molt ──

// SetupTUN creates a TUN device (claw0) with the overlay IPv6 address
// and starts the TUN→overlay send loop. Requires root.
func (t *Transport) SetupTUN() error {
	addr := OverlayAddress(t.PublicKey())
	ip := net.IP(addr[:])

	tun, err := NewTUNDevice("claw0", ip, 65535)
	if err != nil {
		return fmt.Errorf("create TUN: %w", err)
	}

	t.tun = tun
	t.ipv6rwc = newIPv6RWC(t, tun)

	// Register own key as ClawNet peer
	t.RegisterClawPeer(t.PublicKey())

	go t.ipv6rwc.tunReadLoop()

	fmt.Printf("[tun] %s up with %s/7\n", tun.Name(), ip)
	return nil
}

// TUNName returns the TUN interface name, or empty if TUN is not active.
func (t *Transport) TUNName() string {
	if t.tun != nil {
		return t.tun.Name()
	}
	return ""
}

// RegisterClawPeer adds a key to the known ClawNet peer set.
// Called from handshake completion and libp2p peer sync.
func (t *Transport) RegisterClawPeer(key ed25519.PublicKey) {
	var k [32]byte
	copy(k[:], key)
	t.clawPeers.Store(k, struct{}{})
	// Also populate keyStore for TUN address resolution
	if t.ipv6rwc != nil {
		t.ipv6rwc.keyStore.update(key)
	}
}

// IsClawPeer checks if a key belongs to a known ClawNet peer.
func (t *Transport) IsClawPeer(key ed25519.PublicKey) bool {
	var k [32]byte
	copy(k[:], key)
	_, ok := t.clawPeers.Load(k)
	return ok
}

// ── Debug/diagnostics ──

// DebugInfo returns detailed overlay network introspection data.
type DebugInfo struct {
	Self     DebugSelf      `json:"self"`
	Peers    []DebugPeer    `json:"peers"`
	Tree     []DebugTree    `json:"tree"`
	Paths    []DebugPath    `json:"paths"`
	Sessions []DebugSession `json:"sessions"`
}

// DebugSelf contains this node's overlay identity.
type DebugSelf struct {
	Key            string `json:"key"`
	RoutingEntries int    `json:"routing_entries"`
}

// DebugPeer contains info about a directly connected overlay peer.
// Now includes link-layer stats (RX/TX bytes, rate) from linkConn.
type DebugPeer struct {
	Key     string        `json:"key"`
	Root    string        `json:"root"`
	Port    uint64        `json:"port"`
	Latency time.Duration `json:"latency_ms"`
	Prio    uint8         `json:"priority"`
	// Link-layer fields (new, from linkConn byte counting)
	URI     string `json:"uri,omitempty"`
	Up      bool   `json:"up"`
	RXBytes uint64 `json:"rx_bytes"`
	TXBytes uint64 `json:"tx_bytes"`
	RXRate  uint64 `json:"rx_rate"`
	TXRate  uint64 `json:"tx_rate"`
}

// DebugTree contains a spanning tree entry.
type DebugTree struct {
	Key    string `json:"key"`
	Parent string `json:"parent"`
	Seq    uint64 `json:"sequence"`
}

// DebugPath contains a cached source-routed path.
type DebugPath struct {
	Key  string   `json:"key"`
	Path []uint64 `json:"path"`
	Seq  uint64   `json:"sequence"`
}

// DebugSession contains an encrypted session.
type DebugSession struct {
	Key string `json:"key"`
}

// GetDebugInfo returns comprehensive overlay network state for diagnostics.
func (t *Transport) GetDebugInfo() *DebugInfo {
	if t == nil || t.pc == nil {
		return nil
	}
	info := &DebugInfo{}

	// Self info from network-level debug
	selfInfo := t.pc.PacketConn.Debug.GetSelf()
	info.Self = DebugSelf{
		Key:            hex.EncodeToString(selfInfo.Key),
		RoutingEntries: int(selfInfo.RoutingEntries),
	}

	// Use rich peer info from links (merges link-layer + ironwood stats)
	for _, p := range t.links.GetPeers() {
		info.Peers = append(info.Peers, DebugPeer{
			Key:     p.Key,
			Root:    p.Root,
			Port:    p.Port,
			Latency: p.Latency,
			Prio:    p.Priority,
			URI:     p.URI,
			Up:      p.Up,
			RXBytes: p.RXBytes,
			TXBytes: p.TXBytes,
			RXRate:  p.RXRate,
			TXRate:  p.TXRate,
		})
	}

	// Spanning tree
	for _, tr := range t.pc.PacketConn.Debug.GetTree() {
		info.Tree = append(info.Tree, DebugTree{
			Key:    hex.EncodeToString(tr.Key),
			Parent: hex.EncodeToString(tr.Parent),
			Seq:    tr.Sequence,
		})
	}

	// Source-routed paths
	for _, pa := range t.pc.PacketConn.Debug.GetPaths() {
		info.Paths = append(info.Paths, DebugPath{
			Key:  hex.EncodeToString(pa.Key),
			Path: pa.Path,
			Seq:  pa.Sequence,
		})
	}

	// Encrypted sessions
	for _, s := range t.pc.Debug.GetSessions() {
		info.Sessions = append(info.Sessions, DebugSession{
			Key: hex.EncodeToString(s.Key),
		})
	}

	return info
}
