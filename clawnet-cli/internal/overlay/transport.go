package overlay

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/Arceliar/ironwood/encrypted"
	"github.com/Arceliar/ironwood/network"
	"github.com/Arceliar/ironwood/types"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	// reconnectInterval is how often we retry static peers.
	reconnectInterval = 2 * time.Minute
	// keyExchangeTimeout limits the handshake phase per connection.
	keyExchangeTimeout = 5 * time.Second

	// MsgTypeDM is the prefix byte for DM messages sent over the overlay.
	MsgTypeDM byte = 0x01
)

// Transport manages an Ironwood overlay network as a backup transport.
// Uses encrypted.PacketConn which provides E2E encrypted datagram
// messaging over a spanning-tree + DHT + bloom-filter overlay.
type Transport struct {
	pc          *encrypted.PacketConn
	privKey     ed25519.PrivateKey
	listener    net.Listener
	listenPort  int
	staticPeers []string

	// onMessage callback for received datagrams
	onMessage func(from ed25519.PublicKey, data []byte)

	// Track active connections for PeerCount
	connsMu sync.Mutex
	conns   map[string]struct{} // hex(pubkey) -> struct{}

	mu     sync.Mutex
	closed bool
}

// NewTransport creates an Ironwood overlay transport.
// priv is the libp2p Ed25519 private key (shared identity).
// staticPeers are "host:port" TCP addresses of known overlay nodes.
// opts are optional Ironwood network.Option values (WithPathNotify, WithBloomTransform, etc.).
func NewTransport(priv crypto.PrivKey, listenPort int, staticPeers []string, opts ...network.Option) (*Transport, error) {
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

	t := &Transport{
		pc:          pc,
		privKey:     edPrivKey,
		listenPort:  listenPort,
		staticPeers: staticPeers,
		conns:       make(map[string]struct{}),
	}

	return t, nil
}

// Run starts the overlay transport. It listens for incoming connections,
// maintains connections to static peers, and runs the receive loop.
// Blocks until ctx is cancelled.
func (t *Transport) Run(ctx context.Context) {
	pubKey := t.privKey.Public().(ed25519.PublicKey)
	fmt.Printf("[overlay] public key: %s\n", hex.EncodeToString(pubKey[:8]))

	// Start TCP listener
	if t.listenPort > 0 {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", t.listenPort))
		if err != nil {
			fmt.Printf("[overlay] listen on :%d failed: %v\n", t.listenPort, err)
		} else {
			t.listener = ln
			fmt.Printf("[overlay] listening on :%d\n", t.listenPort)
			go t.acceptLoop(ctx, ln)
		}
	}

	// Connect to static peers
	for _, addr := range t.staticPeers {
		addr := addr
		go t.connectLoop(ctx, addr)
	}

	// Start receive loop
	go t.receiveLoop(ctx)

	<-ctx.Done()
	t.Close()
}

// acceptLoop accepts incoming TCP connections and hands them to ironwood.
func (t *Transport) acceptLoop(ctx context.Context, ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				fmt.Printf("[overlay] accept error: %v\n", err)
				time.Sleep(time.Second)
				continue
			}
		}
		go t.handleConn(conn)
	}
}

// connectLoop periodically tries to connect to a static peer.
func (t *Transport) connectLoop(ctx context.Context, addr string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			time.Sleep(reconnectInterval)
			continue
		}
		t.handleConn(conn)
		// handleConn blocks until disconnection; retry after interval
		time.Sleep(reconnectInterval)
	}
}

// handleConn performs a key exchange and hands the connection to ironwood.
// The protocol: each side sends its ed25519 public key (32 bytes),
// then reads the remote's key, then calls pc.HandleConn which blocks
// until the connection closes.
func (t *Transport) handleConn(conn net.Conn) {
	defer conn.Close()

	localPub := t.privKey.Public().(ed25519.PublicKey)

	// Send our public key
	_ = conn.SetDeadline(time.Now().Add(keyExchangeTimeout))
	if _, err := conn.Write(localPub); err != nil {
		return
	}

	// Read remote public key
	remotePub := make([]byte, ed25519.PublicKeySize)
	if _, err := io.ReadFull(conn, remotePub); err != nil {
		return
	}
	_ = conn.SetDeadline(time.Time{}) // clear deadline

	remoteKey := ed25519.PublicKey(remotePub)
	keyHex := hex.EncodeToString(remoteKey[:8])

	t.connsMu.Lock()
	t.conns[keyHex] = struct{}{}
	t.connsMu.Unlock()

	// HandleConn blocks until the connection is closed
	if err := t.pc.HandleConn(remoteKey, conn, 0); err != nil {
		fmt.Printf("[overlay] peer %s disconnected: %v\n", keyHex, err)
	}

	t.connsMu.Lock()
	delete(t.conns, keyHex)
	t.connsMu.Unlock()
}

// receiveLoop reads datagrams from the Ironwood overlay network.
func (t *Transport) receiveLoop(ctx context.Context) {
	buf := make([]byte, 65535)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, addr, err := t.pc.ReadFrom(buf)
		if err != nil || n == 0 {
			continue
		}

		if t.onMessage != nil {
			if a, ok := addr.(types.Addr); ok {
				data := make([]byte, n)
				copy(data, buf[:n])
				t.onMessage(ed25519.PublicKey(a), data)
			}
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

// PeerCount returns the number of directly connected overlay peers.
func (t *Transport) PeerCount() int {
	t.connsMu.Lock()
	defer t.connsMu.Unlock()
	return len(t.conns)
}

// PublicKey returns the node's Ed25519 public key in the overlay network.
func (t *Transport) PublicKey() ed25519.PublicKey {
	return t.privKey.Public().(ed25519.PublicKey)
}

// Close shuts down the overlay transport.
func (t *Transport) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	t.closed = true
	if t.listener != nil {
		t.listener.Close()
	}
	t.pc.Close()
}

// DebugInfo returns detailed overlay network introspection data.
type DebugInfo struct {
	Self     DebugSelf       `json:"self"`
	Peers    []DebugPeer     `json:"peers"`
	Tree     []DebugTree     `json:"tree"`
	Paths    []DebugPath     `json:"paths"`
	Sessions []DebugSession  `json:"sessions"`
}

// DebugSelf contains this node's overlay identity.
type DebugSelf struct {
	Key            string `json:"key"`
	RoutingEntries int    `json:"routing_entries"`
}

// DebugPeer contains info about a directly connected overlay peer.
type DebugPeer struct {
	Key     string        `json:"key"`
	Root    string        `json:"root"`
	Port    uint64        `json:"port"`
	Latency time.Duration `json:"latency_ms"`
	Prio    uint8         `json:"priority"`
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

	// Connected peers
	for _, p := range t.pc.PacketConn.Debug.GetPeers() {
		info.Peers = append(info.Peers, DebugPeer{
			Key:     hex.EncodeToString(p.Key),
			Root:    hex.EncodeToString(p.Root),
			Port:    p.Port,
			Latency: p.Latency,
			Prio:    p.Priority,
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
