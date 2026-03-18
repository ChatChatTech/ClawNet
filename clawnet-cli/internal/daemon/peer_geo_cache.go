package daemon

import (
	"sync"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/geo"
	"github.com/libp2p/go-libp2p/core/peer"
)

// peerGeoEntry is a cached geo lookup result for one libp2p peer.
type peerGeoEntry struct {
	PeerID         string       `json:"peer_id"`
	ShortID        string       `json:"short_id"`
	AgentName      string       `json:"agent_name,omitempty"`
	Role           string       `json:"role,omitempty"`
	Location       string       `json:"location"`
	Geo            *geo.GeoInfo `json:"geo,omitempty"`
	IsSelf         bool         `json:"is_self"`
	LatencyMs      int64        `json:"latency_ms"`
	ConnectedSince int64        `json:"connected_since"`
	Motto          string       `json:"motto,omitempty"`
	BwIn           int64        `json:"bw_in"`
	BwOut          int64        `json:"bw_out"`
	Reputation     float64      `json:"reputation"`
	resolved       bool         // internal: geo has been looked up
}

// peerGeoCache maintains an asynchronously-refreshed geo cache for libp2p peers.
// The background goroutine resolves a few IPs per tick so that the API handler
// never blocks on geo lookups, eliminating the flickering caused by per-request resolution.
type peerGeoCache struct {
	mu      sync.RWMutex
	entries map[peer.ID]*peerGeoEntry

	daemon *Daemon
}

func newPeerGeoCache(d *Daemon) *peerGeoCache {
	return &peerGeoCache{
		entries: make(map[peer.ID]*peerGeoEntry),
		daemon:  d,
	}
}

func (c *peerGeoCache) run(done <-chan struct{}) {
	// Resolve immediately on startup, then every 3 seconds.
	c.refresh()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			c.refresh()
		}
	}
}

const peerGeoBatchSize = 10

func (c *peerGeoCache) refresh() {
	d := c.daemon

	connected := d.Node.ConnectedPeers()
	connSet := make(map[peer.ID]struct{}, len(connected))
	for _, p := range connected {
		connSet[p] = struct{}{}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict disconnected peers
	for pid := range c.entries {
		if _, ok := connSet[pid]; !ok {
			delete(c.entries, pid)
		}
	}

	// Update existing entries' dynamic fields and track unresolved
	var needResolve []peer.ID
	for _, p := range connected {
		pid := p.String()
		e, exists := c.entries[p]
		if !exists {
			e = &peerGeoEntry{
				PeerID:   pid,
				ShortID:  shortID(pid),
				Location: "Resolving...",
			}
			c.entries[p] = e
			needResolve = append(needResolve, p)
		} else if !e.resolved {
			needResolve = append(needResolve, p)
		}

		// Always refresh volatile fields
		lat := d.Node.Host.Peerstore().LatencyEWMA(p)
		if lat > 0 {
			e.LatencyMs = lat.Milliseconds()
		}
		conns := d.Node.Host.Network().ConnsToPeer(p)
		if len(conns) > 0 {
			e.ConnectedSince = conns[0].Stat().Opened.Unix()
		}
		if m, ok := d.PeerMottos.Load(pid); ok {
			e.Motto = m.(string)
		}
		if n, ok := d.PeerAgentNames.Load(pid); ok {
			e.AgentName = n.(string)
		}
		if rl, ok := d.PeerRoles.Load(pid); ok {
			e.Role = rl.(string)
		}
		if bw := d.Node.BwCounter; bw != nil {
			st := bw.GetBandwidthForPeer(p)
			e.BwIn = int64(st.TotalIn)
			e.BwOut = int64(st.TotalOut)
		}
		if rep, err := d.Store.GetReputation(pid); err == nil {
			e.Reputation = rep.Score
		}
	}

	// Batch-resolve geo for unresolved peers
	if d.Geo == nil || len(needResolve) == 0 {
		return
	}
	if len(needResolve) > peerGeoBatchSize {
		needResolve = needResolve[:peerGeoBatchSize]
	}
	for _, p := range needResolve {
		addrs := d.Node.Host.Peerstore().Addrs(p)
		e := c.entries[p]
		e.resolved = true
		for _, a := range addrs {
			ip := geo.ExtractIP(a.String())
			if ip != "" && geo.IsPublicIP(ip) {
				if gi := d.Geo.Lookup(ip); gi != nil {
					e.Location = gi.Label()
					e.Geo = gi
				} else {
					e.Location = "Unknown"
				}
				break
			}
		}
		if !e.resolved || e.Location == "Resolving..." {
			e.Location = "Private"
			e.resolved = true
		}
	}
}

// snapshot returns a copy of all cached entries plus self, ready for JSON serialization.
func (c *peerGeoCache) snapshot() []peerGeoEntry {
	d := c.daemon

	// Build self entry
	selfID := d.Node.PeerID().String()
	self := peerGeoEntry{
		PeerID:  selfID,
		ShortID: shortID(selfID),
		IsSelf:  true,
	}
	if d.Profile != nil {
		self.Motto = d.Profile.Motto
		self.AgentName = d.Profile.AgentName
		self.Role = d.Profile.Role
	}
	if rep, err := d.Store.GetReputation(selfID); err == nil {
		self.Reputation = rep.Score
	}
	if d.Geo != nil {
		for _, a := range d.Node.Addrs() {
			ip := geo.ExtractIP(a.String())
			if ip != "" && geo.IsPublicIP(ip) {
				if gi := d.Geo.Lookup(ip); gi != nil {
					self.Location = gi.Label()
					self.Geo = gi
				}
				break
			}
		}
	}
	if self.Location == "" {
		self.Location = "Unknown"
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]peerGeoEntry, 0, len(c.entries)+1)
	out = append(out, self)
	for _, e := range c.entries {
		out = append(out, *e)
	}
	return out
}
