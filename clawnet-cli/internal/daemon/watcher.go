package daemon

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
	"github.com/multiformats/go-multiaddr"
)

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
