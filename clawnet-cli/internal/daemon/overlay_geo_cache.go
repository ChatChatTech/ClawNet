package daemon

import (
	"net"
	"sync"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/geo"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/overlay"
)

// overlayGeoEntry is a cached geo lookup result for one overlay peer.
type overlayGeoEntry struct {
	KeyHex    string       `json:"key"`
	Location  string       `json:"location"`
	Geo       *geo.GeoInfo `json:"geo,omitempty"`
	LatencyMs int64        `json:"latency_ms"`
}

// overlayGeoCache maintains an asynchronously-refreshed geo cache for overlay peers.
// The background goroutine resolves a few peers per tick so that the API handler
// never blocks on geo lookups.
type overlayGeoCache struct {
	mu      sync.RWMutex
	entries map[string]*overlayGeoEntry // keyHex → entry

	overlay *overlay.Transport
	geo     *geo.Locator
}

func newOverlayGeoCache(ot *overlay.Transport, g *geo.Locator) *overlayGeoCache {
	return &overlayGeoCache{
		entries: make(map[string]*overlayGeoEntry),
		overlay: ot,
		geo:     g,
	}
}

// run is the background refresh loop. It merges current connected peers
// into the cache, resolves unresolved entries incrementally (batch per tick),
// and evicts stale entries that are no longer connected.
func (c *overlayGeoCache) run(done <-chan struct{}) {
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

// batchSize controls how many new peer IPs are geo-resolved per tick.
const geoBatchSize = 10

func (c *overlayGeoCache) refresh() {
	if c.overlay == nil {
		return
	}

	// Collect connected peers and latency
	cpeers := c.overlay.GetConnectedPeers()
	latMap := make(map[string]int64)
	if dbg := c.overlay.GetDebugInfo(); dbg != nil {
		for _, p := range dbg.Peers {
			if len(p.Key) >= 16 {
				latMap[p.Key[:16]] = p.Latency.Milliseconds()
			}
		}
	}

	connectedSet := make(map[string]overlay.ConnectedPeer, len(cpeers))
	for _, cp := range cpeers {
		connectedSet[cp.KeyHex] = cp
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict peers no longer connected
	for k := range c.entries {
		if _, ok := connectedSet[k]; !ok {
			delete(c.entries, k)
		}
	}

	// Update latency for existing entries and mark unresolved ones for lookup
	var needResolve []string
	for key := range connectedSet {
		if e, ok := c.entries[key]; ok {
			// Refresh latency
			if lat, ok := latMap[key]; ok {
				e.LatencyMs = lat
			}
			// Re-queue if still unresolved
			if e.Location == "Resolving..." {
				needResolve = append(needResolve, key)
			}
		} else {
			// New peer: add placeholder, queue for resolve
			entry := &overlayGeoEntry{
				KeyHex:   key,
				Location: "Resolving...",
			}
			if lat, ok := latMap[key]; ok {
				entry.LatencyMs = lat
			}
			c.entries[key] = entry
			needResolve = append(needResolve, key)
		}
	}

	// Resolve a batch of peers (incremental, never blocks the full list)
	if c.geo == nil || len(needResolve) == 0 {
		return
	}
	if len(needResolve) > geoBatchSize {
		needResolve = needResolve[:geoBatchSize]
	}
	for _, key := range needResolve {
		cp, ok := connectedSet[key]
		if !ok {
			continue
		}
		host, _, err := net.SplitHostPort(cp.RemoteAddr)
		if err != nil || host == "" {
			c.entries[key].Location = "Private"
			continue
		}
		if !geo.IsPublicIP(host) {
			c.entries[key].Location = "Private"
			continue
		}
		if gi := c.geo.Lookup(host); gi != nil {
			c.entries[key].Location = gi.Label()
			c.entries[key].Geo = gi
		} else {
			c.entries[key].Location = "Unknown"
		}
	}
}

// snapshot returns a copy of the current cache for JSON serialization.
func (c *overlayGeoCache) snapshot() []overlayGeoEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]overlayGeoEntry, 0, len(c.entries))
	for _, e := range c.entries {
		out = append(out, *e)
	}
	return out
}
