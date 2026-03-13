package btdht

import (
	"context"
	"crypto/sha1"
	"fmt"
	"net"
	"sync"
	"time"

	dht "github.com/anacrolix/dht/v2"
)

const (
	// reannounceInterval is how often we re-announce on the BT DHT.
	// BT DHT announce entries expire after ~30 min, so 20 min keeps us fresh.
	reannounceInterval = 20 * time.Minute
	// bootstrapTimeout is the max time to wait for initial BT DHT bootstrap.
	bootstrapTimeout = 30 * time.Second
)

// clawnetInfoHash is the fixed BT DHT infohash that all ClawNet nodes
// announce and search for. It acts as our "torrent" identity in the global
// BitTorrent DHT.
var clawnetInfoHash = sha1.Sum([]byte("clawnet-bootstrap-v1"))

// PeerAddr is an IP:Port pair discovered from the BT DHT.
type PeerAddr struct {
	IP   net.IP
	Port int
}

func (p PeerAddr) String() string {
	return fmt.Sprintf("%s:%d", p.IP, p.Port)
}

// Discovery manages ClawNet node discovery via the BitTorrent Mainline DHT.
type Discovery struct {
	server     *dht.Server
	libp2pPort int

	mu      sync.Mutex
	closed  bool
}

// NewDiscovery creates a BT DHT discovery service.
// listenPort is the UDP port for BT DHT traffic.
// libp2pPort is the local libp2p listen port to announce.
func NewDiscovery(listenPort int, libp2pPort int) (*Discovery, error) {
	conn, err := net.ListenPacket("udp", fmt.Sprintf(":%d", listenPort))
	if err != nil {
		return nil, fmt.Errorf("listen UDP :%d: %w", listenPort, err)
	}

	cfg := dht.NewDefaultServerConfig()
	cfg.Conn = conn
	// anacrolix/dht includes default global bootstrap nodes:
	// router.bittorrent.com:6881, router.utorrent.com:6881, etc.

	server, err := dht.NewServer(cfg)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("create BT DHT server: %w", err)
	}

	go server.TableMaintainer()

	return &Discovery{
		server:     server,
		libp2pPort: libp2pPort,
	}, nil
}

// Bootstrap populates the BT DHT routing table by contacting global
// bootstrap nodes. This should be called once at startup.
func (d *Discovery) Bootstrap() error {
	ctx, cancel := context.WithTimeout(context.Background(), bootstrapTimeout)
	defer cancel()

	stats, err := d.server.BootstrapContext(ctx)
	if err != nil {
		return fmt.Errorf("BT DHT bootstrap: %w", err)
	}

	fmt.Printf("bt-dht: bootstrap complete, tried %d addrs, got %d responses\n",
		stats.NumAddrsTried, stats.NumResponses)
	return nil
}

// FindAndAnnounce performs a single DHT traversal: finds other ClawNet
// peers and announces ourselves. Returns discovered peer addresses.
func (d *Discovery) FindAndAnnounce(ctx context.Context) ([]PeerAddr, error) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil, fmt.Errorf("discovery closed")
	}
	d.mu.Unlock()

	tok, err := d.server.AnnounceTraversal(clawnetInfoHash, dht.AnnouncePeer(dht.AnnouncePeerOpts{
		Port:        d.libp2pPort,
		ImpliedPort: false,
	}))
	if err != nil {
		return nil, fmt.Errorf("announce traversal: %w", err)
	}
	defer tok.Close()

	var peers []PeerAddr
	for {
		select {
		case <-ctx.Done():
			return peers, nil
		case ps, ok := <-tok.Peers:
			if !ok {
				return peers, nil
			}
			for _, p := range ps.Peers {
				peers = append(peers, PeerAddr{
					IP:   p.IP,
					Port: p.Port,
				})
			}
		}
	}
}

// FindOnly performs a get_peers traversal without announcing ourselves.
// Useful for a read-only lookup.
func (d *Discovery) FindOnly(ctx context.Context) ([]PeerAddr, error) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil, fmt.Errorf("discovery closed")
	}
	d.mu.Unlock()

	tok, err := d.server.AnnounceTraversal(clawnetInfoHash)
	if err != nil {
		return nil, fmt.Errorf("announce traversal: %w", err)
	}
	defer tok.Close()

	var peers []PeerAddr
	for {
		select {
		case <-ctx.Done():
			return peers, nil
		case ps, ok := <-tok.Peers:
			if !ok {
				return peers, nil
			}
			for _, p := range ps.Peers {
				peers = append(peers, PeerAddr{
					IP:   p.IP,
					Port: p.Port,
				})
			}
		}
	}
}

// RunLoop starts the periodic announce/find loop. It blocks until ctx is
// cancelled. onPeersFound is called each time new peers are discovered.
func (d *Discovery) RunLoop(ctx context.Context, onPeersFound func([]PeerAddr)) {
	// Do an initial find+announce immediately
	peers, err := d.FindAndAnnounce(ctx)
	if err != nil {
		fmt.Printf("bt-dht: initial announce failed: %v\n", err)
	} else if len(peers) > 0 {
		fmt.Printf("bt-dht: initial discovery found %d peers\n", len(peers))
		onPeersFound(peers)
	}

	ticker := time.NewTicker(reannounceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			peers, err := d.FindAndAnnounce(ctx)
			if err != nil {
				fmt.Printf("bt-dht: announce cycle failed: %v\n", err)
				continue
			}
			if len(peers) > 0 {
				fmt.Printf("bt-dht: discovered %d peers\n", len(peers))
				onPeersFound(peers)
			}
		}
	}
}

// GoodNodes returns the number of good nodes in the BT DHT routing table.
func (d *Discovery) GoodNodes() int {
	return d.server.Stats().GoodNodes
}

// Close shuts down the BT DHT server.
func (d *Discovery) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	d.server.Close()
}
