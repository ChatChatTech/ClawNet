package overlay

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// probeInterval is how often we check peer health.
	probeInterval = 5 * time.Minute

	// saveInterval is how often we persist peer state to disk.
	saveInterval = 30 * time.Minute

	// maxConsecFailsHardcoded caps backoff for hardcoded peers (never removed).
	maxConsecFailsHardcoded = 100

	// maxConsecFailsDiscovered removes discovered peers after this many failures.
	maxConsecFailsDiscovered = 50
)

// PeerState tracks the health and metadata of a single overlay peer.
type PeerState struct {
	Address     string    `json:"address"`
	Source      string    `json:"source"`       // "hardcoded", "discovered", "user"
	Alive       bool      `json:"alive"`
	LastSeen    time.Time `json:"last_seen"`
	LastAttempt time.Time `json:"last_attempt"`
	ConsecFails int       `json:"consec_fails"`
	TotalConns  int       `json:"total_conns"`
}

// peerFile is the on-disk format for persisted peer state.
type peerFile struct {
	Version   int                   `json:"version"`
	UpdatedAt time.Time             `json:"updated_at"`
	Peers     map[string]*PeerState `json:"peers"`
}

// PeerManager tracks overlay peer health, applies exponential backoff
// for unreachable peers, and persists state across restarts.
type PeerManager struct {
	mu    sync.RWMutex
	peers map[string]*PeerState // addr → state

	transport *Transport
	dataDir   string

	ctx    context.Context
	cancel context.CancelFunc
}

// NewPeerManager creates a PeerManager that monitors the given transport.
// dataDir is the ClawNet data directory (e.g. ~/.openclaw/clawnet).
func NewPeerManager(transport *Transport, dataDir string) *PeerManager {
	ctx, cancel := context.WithCancel(context.Background())
	pm := &PeerManager{
		peers:     make(map[string]*PeerState),
		transport: transport,
		dataDir:   dataDir,
		ctx:       ctx,
		cancel:    cancel,
	}

	// Load persisted state, then merge hardcoded defaults
	pm.load()
	pm.mergeDefaults()

	return pm
}

// Run starts the background probe and save loops. Blocks until ctx is cancelled.
func (pm *PeerManager) Run(ctx context.Context) {
	probeTicker := time.NewTicker(probeInterval)
	saveTicker := time.NewTicker(saveInterval)
	defer probeTicker.Stop()
	defer saveTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			pm.save()
			return
		case <-pm.ctx.Done():
			pm.save()
			return
		case <-probeTicker.C:
			pm.probe()
		case <-saveTicker.C:
			pm.save()
		}
	}
}

// Stop cancels the PeerManager's background loops and saves state.
func (pm *PeerManager) Stop() {
	pm.cancel()
}

// probe checks which peers are currently connected and updates health state.
func (pm *PeerManager) probe() {
	connected := pm.transport.GetConnectedPeers()

	// Build a set of connected remote addresses
	connAddrs := make(map[string]struct{}, len(connected))
	for _, cp := range connected {
		connAddrs[cp.RemoteAddr] = struct{}{}
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	now := time.Now()
	for addr, state := range pm.peers {
		if _, ok := connAddrs[addr]; ok {
			// Peer is connected
			state.Alive = true
			state.LastSeen = now
			state.ConsecFails = 0
			state.TotalConns++
		} else {
			// Peer is not connected right now
			if state.Alive {
				// Was alive, now appears disconnected
				state.Alive = false
				state.ConsecFails++
			} else {
				state.ConsecFails++
			}
		}

		// Remove discovered peers that fail too many times
		if state.Source == "discovered" && state.ConsecFails > maxConsecFailsDiscovered {
			delete(pm.peers, addr)
		}
	}
}

// BackoffDuration returns the retry backoff for a peer based on consecutive failures.
func BackoffDuration(consecFails int) time.Duration {
	switch {
	case consecFails <= 2:
		return 2 * time.Minute
	case consecFails <= 5:
		return 5 * time.Minute
	case consecFails <= 10:
		return 30 * time.Minute
	case consecFails <= 20:
		return 2 * time.Hour
	default:
		return 24 * time.Hour
	}
}

// ShouldRetry returns true if enough time has passed since the last attempt
// given the peer's current backoff level.
func (pm *PeerManager) ShouldRetry(addr string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	state, ok := pm.peers[addr]
	if !ok {
		return true // unknown peer, allow
	}
	if state.ConsecFails == 0 {
		return true
	}
	backoff := BackoffDuration(state.ConsecFails)
	return time.Since(state.LastAttempt) >= backoff
}

// RecordAttempt records that we attempted to connect to a peer.
func (pm *PeerManager) RecordAttempt(addr string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if state, ok := pm.peers[addr]; ok {
		state.LastAttempt = time.Now()
	}
}

// RecordSuccess records a successful connection to a peer.
func (pm *PeerManager) RecordSuccess(addr string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if state, ok := pm.peers[addr]; ok {
		state.Alive = true
		state.LastSeen = time.Now()
		state.ConsecFails = 0
		state.TotalConns++
	}
}

// RecordFailure records a failed connection attempt.
func (pm *PeerManager) RecordFailure(addr string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if state, ok := pm.peers[addr]; ok {
		state.Alive = false
		state.ConsecFails++
	}
}

// AddDiscoveredPeer adds a dynamically discovered peer to the table.
func (pm *PeerManager) AddDiscoveredPeer(addr string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.peers[addr]; exists {
		return
	}
	pm.peers[addr] = &PeerState{
		Address: addr,
		Source:  "discovered",
	}
}

// GetStats returns a snapshot of all peer states for diagnostics.
func (pm *PeerManager) GetStats() []PeerState {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	out := make([]PeerState, 0, len(pm.peers))
	for _, s := range pm.peers {
		out = append(out, *s)
	}
	return out
}

// AliveCount returns the number of currently alive peers.
func (pm *PeerManager) AliveCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	count := 0
	for _, s := range pm.peers {
		if s.Alive {
			count++
		}
	}
	return count
}

// mergeDefaults ensures all hardcoded peers are in the table.
func (pm *PeerManager) mergeDefaults() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, addr := range DefaultYggdrasilPeers {
		if _, exists := pm.peers[addr]; !exists {
			pm.peers[addr] = &PeerState{
				Address: addr,
				Source:  "hardcoded",
			}
		}
	}
}

// peersFilePath returns the path to peers.json in the data directory.
func (pm *PeerManager) peersFilePath() string {
	return filepath.Join(pm.dataDir, "peers.json")
}

// load reads persisted peer state from disk.
func (pm *PeerManager) load() {
	data, err := os.ReadFile(pm.peersFilePath())
	if err != nil {
		return // no persisted state, that's fine
	}

	var pf peerFile
	if err := json.Unmarshal(data, &pf); err != nil {
		fmt.Printf("[peermgr] failed to parse peers.json: %v\n", err)
		return
	}

	if pf.Version != 1 || pf.Peers == nil {
		return
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	for addr, state := range pf.Peers {
		state.Address = addr
		// Reset alive status — will be re-evaluated by probe
		state.Alive = false
		pm.peers[addr] = state
	}
	fmt.Printf("[peermgr] loaded %d peers from disk\n", len(pf.Peers))
}

// save writes current peer state to disk.
func (pm *PeerManager) save() {
	pm.mu.RLock()
	pf := peerFile{
		Version:   1,
		UpdatedAt: time.Now(),
		Peers:     make(map[string]*PeerState, len(pm.peers)),
	}
	for addr, state := range pm.peers {
		pf.Peers[addr] = state
	}
	pm.mu.RUnlock()

	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		fmt.Printf("[peermgr] failed to marshal peers: %v\n", err)
		return
	}

	path := pm.peersFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		fmt.Printf("[peermgr] failed to create dir: %v\n", err)
		return
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		fmt.Printf("[peermgr] failed to write peers.json: %v\n", err)
	}
}
