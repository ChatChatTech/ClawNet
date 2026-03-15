package matrix

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"golang.org/x/crypto/hkdf"
)

// AnnounceMsg is the JSON structure broadcast in the discovery room.
type AnnounceMsg struct {
	Type    string   `json:"type"`
	Version int      `json:"version"`
	PeerID  string   `json:"peer_id"`
	Addrs   []string `json:"addrs"`
	Agent   string   `json:"agent"`
	TS      int64    `json:"ts"`
}

// Discovery manages Matrix-based peer discovery across multiple homeservers.
type Discovery struct {
	priv        crypto.PrivKey
	peerID      peer.ID
	addrs       func() []multiaddr.Multiaddr // function to get current listen addrs
	agent       string
	homeservers []string
	interval    time.Duration
	dataDir     string

	clients map[string]*Client // homeserver → client
	roomIDs map[string]string  // homeserver → room ID
	mu      sync.Mutex
}

// NewDiscovery creates a Matrix discovery instance.
// addrsFunc should return the node's current multiaddrs.
func NewDiscovery(priv crypto.PrivKey, homeservers []string, interval time.Duration, agent, dataDir string, addrsFunc func() []multiaddr.Multiaddr) (*Discovery, error) {
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("derive peer ID: %w", err)
	}
	if len(homeservers) == 0 {
		homeservers = DefaultHomeservers
	}
	if interval <= 0 {
		interval = DefaultAnnounceInterval
	}
	return &Discovery{
		priv:        priv,
		peerID:      pid,
		addrs:       addrsFunc,
		agent:       agent,
		homeservers: homeservers,
		interval:    interval,
		dataDir:     dataDir,
		clients:     make(map[string]*Client),
		roomIDs:     make(map[string]string),
	}, nil
}

// Run starts the discovery loop. It connects to homeservers, joins the
// discovery room, and periodically announces multiaddrs. Discovered peers
// are passed to onPeerFound. Blocks until ctx is cancelled.
func (d *Discovery) Run(ctx context.Context, onPeerFound func(peer.AddrInfo)) {
	// Load cached tokens
	tokens := d.loadTokens()

	// Connect to each homeserver (best effort, 2-3 is enough)
	username := UsernamePrefix + d.peerID.String()[:16]
	password := d.derivePassword()

	var connected int
	for _, hs := range d.homeservers {
		client := NewClient(hs)

		// Try cached token first
		if t, ok := tokens[hs]; ok {
			client.SetToken(t.AccessToken, t.UserID)
			// Validate by doing a quick sync
			_, err := client.Sync(ctx, "", 0)
			if err == nil {
				d.mu.Lock()
				d.clients[hs] = client
				d.mu.Unlock()
				connected++
				fmt.Printf("[matrix] reused cached session on %s\n", hs)
				continue
			}
			// Token expired, fall through to register/login
		}

		// Try register, falls back to login if user exists
		if err := client.Register(ctx, username, password); err != nil {
			fmt.Printf("[matrix] %s: auth failed: %v\n", hs, err)
			continue
		}

		d.mu.Lock()
		d.clients[hs] = client
		d.mu.Unlock()
		connected++

		// Cache token
		token, userID := client.Token()
		tokens[hs] = tokenEntry{AccessToken: token, UserID: userID}

		fmt.Printf("[matrix] authenticated on %s\n", hs)

		if connected >= 3 {
			break // 3 homeservers is sufficient
		}
	}

	if connected == 0 {
		fmt.Println("[matrix] no homeserver reachable, discovery disabled")
		return
	}

	// Save tokens
	d.saveTokens(tokens)

	// Join discovery rooms
	d.mu.Lock()
	for hs, client := range d.clients {
		// Try the canonical room alias on this homeserver
		alias := DiscoveryRoomAlias + ":" + extractDomain(hs)
		roomID, err := client.JoinRoom(ctx, alias)
		if err != nil {
			// Try the matrix.org version (federation)
			alias = DiscoveryRoomAlias + ":matrix.org"
			roomID, err = client.JoinRoom(ctx, alias)
			if err != nil {
				fmt.Printf("[matrix] %s: failed to join discovery room: %v\n", hs, err)
				continue
			}
		}
		d.roomIDs[hs] = roomID
		fmt.Printf("[matrix] joined %s on %s\n", alias, hs)
	}
	d.mu.Unlock()

	// Initial announce
	d.announce(ctx)

	// Start sync loops for each connected client
	var wg sync.WaitGroup
	for hs, client := range d.clients {
		roomID, ok := d.roomIDs[hs]
		if !ok {
			continue
		}
		wg.Add(1)
		go func(hs string, client *Client, roomID string) {
			defer wg.Done()
			d.syncLoop(ctx, hs, client, roomID, onPeerFound)
		}(hs, client, roomID)
	}

	// Periodic announce
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-ticker.C:
			d.announce(ctx)
		}
	}
}

// announce broadcasts our multiaddrs to all joined discovery rooms.
func (d *Discovery) announce(ctx context.Context) {
	addrs := d.addrs()
	addrStrs := make([]string, len(addrs))
	for i, a := range addrs {
		addrStrs[i] = a.String()
	}
	msg := AnnounceMsg{
		Type:    "clawnet.announce",
		Version: 1,
		PeerID:  d.peerID.String(),
		Addrs:   addrStrs,
		Agent:   d.agent,
		TS:      time.Now().Unix(),
	}
	body, _ := json.Marshal(msg)

	d.mu.Lock()
	defer d.mu.Unlock()
	for hs, client := range d.clients {
		roomID, ok := d.roomIDs[hs]
		if !ok {
			continue
		}
		if err := client.SendMessage(ctx, roomID, string(body)); err != nil {
			fmt.Printf("[matrix] %s: announce failed: %v\n", hs, err)
		}
	}
}

// syncLoop listens for messages in the discovery room and extracts peer addresses.
func (d *Discovery) syncLoop(ctx context.Context, hs string, client *Client, roomID string, onPeerFound func(peer.AddrInfo)) {
	var since string

	// Initial sync to get the since token (skip old messages)
	sr, err := client.Sync(ctx, "", 0)
	if err != nil {
		fmt.Printf("[matrix] %s: initial sync failed: %v\n", hs, err)
		return
	}
	since = sr.NextBatch

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		sr, err := client.Sync(ctx, since, SyncTimeoutMs)
		if err != nil {
			// Retry after brief pause
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
				continue
			}
		}
		since = sr.NextBatch

		// Process events from the discovery room
		room, ok := sr.Rooms.Join[roomID]
		if !ok {
			continue
		}
		for _, event := range room.Timeline.Events {
			if event.Type != "m.room.message" {
				continue
			}
			d.handleRoomMessage(event, onPeerFound)
		}
	}
}

// handleRoomMessage extracts peer address info from a room message.
func (d *Discovery) handleRoomMessage(event RoomEvent, onPeerFound func(peer.AddrInfo)) {
	// Parse the message content to get the body
	var content struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(event.Content, &content); err != nil {
		return
	}

	// Try to parse as announce message
	var msg AnnounceMsg
	if err := json.Unmarshal([]byte(content.Body), &msg); err != nil {
		return
	}
	if msg.Type != "clawnet.announce" || msg.Version != 1 {
		return
	}

	// Skip our own announcements
	if msg.PeerID == d.peerID.String() {
		return
	}

	// Reject announcements older than 1 hour
	if time.Since(time.Unix(msg.TS, 0)) > time.Hour {
		return
	}

	// Parse peer ID and multiaddrs
	pid, err := peer.Decode(msg.PeerID)
	if err != nil {
		return
	}
	var addrs []multiaddr.Multiaddr
	for _, s := range msg.Addrs {
		ma, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			continue
		}
		addrs = append(addrs, ma)
	}
	if len(addrs) == 0 {
		return
	}

	onPeerFound(peer.AddrInfo{ID: pid, Addrs: addrs})
}

// derivePassword deterministically derives a password from the Ed25519 private key.
// Uses HKDF-SHA512 with a fixed salt so the same key always produces the same password.
func (d *Discovery) derivePassword() string {
	raw, err := d.priv.Raw()
	if err != nil {
		// Fallback: use peer ID as password (less secure but functional)
		return d.peerID.String()
	}
	salt := []byte("clawnet-matrix-password-v1")
	hkdfReader := hkdf.New(sha512.New, raw, salt, []byte("matrix-login"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return d.peerID.String()
	}
	return hex.EncodeToString(key)
}

// Token cache
type tokenEntry struct {
	AccessToken string `json:"access_token"`
	UserID      string `json:"user_id"`
}

func (d *Discovery) tokensPath() string {
	return filepath.Join(d.dataDir, "matrix_tokens.json")
}

func (d *Discovery) loadTokens() map[string]tokenEntry {
	tokens := make(map[string]tokenEntry)
	data, err := os.ReadFile(d.tokensPath())
	if err != nil {
		return tokens
	}
	json.Unmarshal(data, &tokens)
	return tokens
}

func (d *Discovery) saveTokens(tokens map[string]tokenEntry) {
	data, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(d.tokensPath()), 0700)
	os.WriteFile(d.tokensPath(), data, 0600)
}

// ConnectedHomeservers returns the number of homeservers currently connected.
func (d *Discovery) ConnectedHomeservers() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.clients)
}

// JoinedRooms returns the number of discovery rooms joined.
func (d *Discovery) JoinedRooms() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.roomIDs)
}

// extractDomain extracts the domain from a homeserver URL.
// e.g. "https://matrix.org" → "matrix.org"
func extractDomain(url string) string {
	s := strings.TrimPrefix(url, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimRight(s, "/")
	return s
}
