package daemon

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/multiformats/go-multiaddr"
)

const relayRendezvous = "/clawnet/relay-providers"

// peerWatcher implements network.Notifiee to watch peer connections,
// log events, and feed the hot-peer reconnect list.
type peerWatcher struct {
	d *Daemon
}

func (pw *peerWatcher) Listen(network.Network, multiaddr.Multiaddr)      {}
func (pw *peerWatcher) ListenClose(network.Network, multiaddr.Multiaddr) {}

func (pw *peerWatcher) Connected(_ network.Network, c network.Conn) {
	NotifyTopologyChange()
	pid := c.RemotePeer().String()
	short := pid
	if len(short) > 16 {
		short = short[:16]
	}
	addr := c.RemoteMultiaddr().String()
	fmt.Printf("peer+ %s via %s\n", short, addr)
	// Track as hot peer for reconnect
	pw.d.hotPeers.Store(pid, hotPeer{addr: c.RemoteMultiaddr(), lastSeen: time.Now()})
}

func (pw *peerWatcher) Disconnected(_ network.Network, c network.Conn) {
	NotifyTopologyChange()
	pid := c.RemotePeer().String()
	short := pid
	if len(short) > 16 {
		short = short[:16]
	}
	fmt.Printf("peer- %s\n", short)
	// Update last seen but keep in hot list for reconnect
	if v, ok := pw.d.hotPeers.Load(pid); ok {
		hp := v.(hotPeer)
		hp.lastSeen = time.Now()
		pw.d.hotPeers.Store(pid, hp)
	}
}

// hotPeer tracks recently connected peers for reconnection.
type hotPeer struct {
	addr     multiaddr.Multiaddr
	lastSeen time.Time
}

// watchPeerEvents registers a notifiee that fires topology changes on connect/disconnect.
func (d *Daemon) watchPeerEvents() {
	d.Node.Host.Network().Notify(&peerWatcher{d: d})
	go d.reconnectLoop(d.ctx)
}

// reconnectLoop tries to reconnect to recently lost peers using exponential backoff.
func (d *Daemon) reconnectLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Track per-peer backoff state
	type backoffState struct {
		attempts int
		nextTry  time.Time
	}
	backoff := make(map[string]*backoffState)
	var mu sync.Mutex

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		connectedSet := make(map[string]bool)
		for _, p := range d.Node.ConnectedPeers() {
			connectedSet[p.String()] = true
		}

		now := time.Now()
		d.hotPeers.Range(func(key, value any) bool {
			pid := key.(string)
			hp := value.(hotPeer)

			// Evict peers not seen in 30 minutes
			if now.Sub(hp.lastSeen) > 30*time.Minute {
				d.hotPeers.Delete(pid)
				mu.Lock()
				delete(backoff, pid)
				mu.Unlock()
				return true
			}

			// Skip if already connected
			if connectedSet[pid] {
				mu.Lock()
				delete(backoff, pid)
				mu.Unlock()
				return true
			}

			// Skip self / bootstrap (bootstrap has its own reconnect)
			if pid == d.Node.PeerID().String() {
				return true
			}

			// Check backoff
			mu.Lock()
			bs, ok := backoff[pid]
			if !ok {
				bs = &backoffState{}
				backoff[pid] = bs
			}
			if now.Before(bs.nextTry) {
				mu.Unlock()
				return true
			}
			// Exponential backoff: 10s, 20s, 40s, 80s, 160s max
			delay := time.Duration(math.Min(float64(10*time.Second)*math.Pow(2, float64(bs.attempts)), float64(160*time.Second)))
			bs.attempts++
			bs.nextTry = now.Add(delay)
			mu.Unlock()

			peerID, err := peer.Decode(pid)
			if err != nil {
				return true
			}

			go func() {
				rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()
				if err := d.Node.Host.Connect(rctx, peer.AddrInfo{
					ID:    peerID,
					Addrs: []multiaddr.Multiaddr{hp.addr},
				}); err == nil {
					mu.Lock()
					delete(backoff, pid)
					mu.Unlock()
				}
			}()

			return true
		})
	}
}

// pingLoop periodically pings all connected peers to update latency stats
// stored in the peerstore's LatencyEWMA.
func (d *Daemon) pingLoop(ctx context.Context) {
	// Wait for connections to establish before starting
	time.Sleep(30 * time.Second)

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	ps := ping.NewPingService(d.Node.Host)
	_ = ps // registers handler

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		peers := d.Node.ConnectedPeers()
		for _, p := range peers {
			go func(pid peer.ID) {
				pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()
				ch := ping.Ping(pctx, d.Node.Host, pid)
				select {
				case res := <-ch:
					if res.Error == nil {
						d.Node.Host.Peerstore().RecordLatency(pid, res.RTT)
					}
				case <-pctx.Done():
				}
			}(p)
		}
	}
}

// relayState tracks the health of a known relay peer.
type relayState struct {
	id          peer.ID
	addrs       []multiaddr.Multiaddr
	alive       bool
	failCount   int
	lastPingRTT time.Duration
	lastCheck   time.Time
}

const (
	relayCheckInterval  = 30 * time.Second
	relayPingTimeout    = 8 * time.Second
	relayMaxFail        = 3 // mark down after N consecutive failures
	relayProbeMax       = 5 // max connected peers to probe for relay capability per cycle
)

// relayHealthLoop periodically checks relay nodes and discovers backups.
func (d *Daemon) relayHealthLoop(ctx context.Context) {
	// Give the node time to connect before first check.
	time.Sleep(15 * time.Second)

	// Seed known relays from bootstrap peers.
	relays := d.initRelayList()
	if len(relays) == 0 && !d.Config.RelayEnabled {
		fmt.Println("[relay-health] no relay peers configured, skipping health loop")
		return
	}

	if d.Node.DHT == nil {
		fmt.Println("[relay-health] DHT unavailable, skipping relay discovery")
		return
	}

	rd := drouting.NewRoutingDiscovery(d.Node.DHT)

	// If this node offers relay (and is not ForcePrivate), advertise.
	if d.Config.RelayEnabled && !d.Config.ForcePrivate {
		go func() {
			for {
				if _, err := rd.Advertise(ctx, relayRendezvous); err == nil {
					fmt.Println("[relay-health] advertised as relay provider")
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(10 * time.Minute):
				}
			}
		}()
	}

	ticker := time.NewTicker(relayCheckInterval)
	defer ticker.Stop()

	discoverTick := 0 // discover from DHT every 6th cycle (~3 min)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		aliveCount := 0
		for i := range relays {
			r := &relays[i]
			alive, rtt := d.pingRelay(ctx, r.id)
			r.lastCheck = time.Now()
			r.lastPingRTT = rtt

			if alive {
				if !r.alive {
					fmt.Printf("[relay-health] relay UP: %s (rtt=%s)\n", r.id.String()[:16], rtt)
				}
				r.alive = true
				r.failCount = 0
				aliveCount++
			} else {
				r.failCount++
				if r.alive && r.failCount >= relayMaxFail {
					r.alive = false
					fmt.Printf("[relay-health] relay DOWN: %s (fail=%d)\n", r.id.String()[:16], r.failCount)
				}
			}
		}

		// Periodic DHT relay discovery (every ~3 min) or when all relays down.
		discoverTick++
		if aliveCount == 0 || discoverTick >= 6 {
			discoverTick = 0
			discovered := d.discoverRelaysDHT(ctx, rd)
			discovered = append(discovered, d.probeRelayPeers(ctx)...)

			for _, di := range discovered {
				exists := false
				for _, r := range relays {
					if r.id == di.id {
						exists = true
						break
					}
				}
				if !exists {
					relays = append(relays, di)
					fmt.Printf("[relay-health] discovered relay: %s\n", di.id.String()[:16])
				}
			}
		}

		// Ensure we're connected to at least one alive relay.
		connSet := make(map[peer.ID]bool)
		for _, p := range d.Node.ConnectedPeers() {
			connSet[p] = true
		}
		for _, r := range relays {
			if r.alive && !connSet[r.id] {
				go func(ri relayState) {
					cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
					defer cancel()
					if err := d.Node.Host.Connect(cctx, peer.AddrInfo{
						ID:    ri.id,
						Addrs: ri.addrs,
					}); err == nil {
						fmt.Printf("[relay-health] reconnected to relay %s\n", ri.id.String()[:16])
					}
				}(r)
			}
		}
	}
}

// initRelayList seeds the relay list from bootstrap peers.
func (d *Daemon) initRelayList() []relayState {
	var relays []relayState
	for _, addr := range d.Config.BootstrapPeers {
		ma, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			continue
		}
		pi, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			continue
		}
		relays = append(relays, relayState{
			id:    pi.ID,
			addrs: pi.Addrs,
			alive: true, // assume alive at start
		})
	}
	return relays
}

// pingRelay pings a relay peer and returns (alive, rtt).
func (d *Daemon) pingRelay(ctx context.Context, pid peer.ID) (bool, time.Duration) {
	pctx, cancel := context.WithTimeout(ctx, relayPingTimeout)
	defer cancel()
	ch := ping.Ping(pctx, d.Node.Host, pid)
	select {
	case res := <-ch:
		if res.Error == nil {
			return true, res.RTT
		}
		return false, 0
	case <-pctx.Done():
		return false, 0
	}
}

// discoverRelaysDHT finds relay providers registered via DHT rendezvous.
func (d *Daemon) discoverRelaysDHT(ctx context.Context, rd *drouting.RoutingDiscovery) []relayState {
	var result []relayState
	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	peerChan, err := rd.FindPeers(dctx, relayRendezvous)
	if err != nil {
		return nil
	}
	selfID := d.Node.PeerID()
	for p := range peerChan {
		if p.ID == selfID || len(p.Addrs) == 0 {
			continue
		}
		result = append(result, relayState{
			id:    p.ID,
			addrs: p.Addrs,
			alive: true,
		})
		if len(result) >= 10 {
			break
		}
	}
	return result
}

// probeRelayPeers checks connected peers for circuit relay v2 support
// by looking for "circuit" or "relay" in their protocol list.
func (d *Daemon) probeRelayPeers(ctx context.Context) []relayState {
	var result []relayState
	peers := d.Node.ConnectedPeers()
	probed := 0
	for _, pid := range peers {
		if probed >= relayProbeMax {
			break
		}
		protos, err := d.Node.Host.Peerstore().GetProtocols(pid)
		if err != nil {
			continue
		}
		probed++
		for _, p := range protos {
			if strings.Contains(string(p), "circuit") || strings.Contains(string(p), "relay") {
				addrs := d.Node.Host.Peerstore().Addrs(pid)
				result = append(result, relayState{
					id:    pid,
					addrs: addrs,
					alive: true,
				})
				break
			}
		}
	}
	return result
}
