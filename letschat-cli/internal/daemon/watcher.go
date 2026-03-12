package daemon

import (
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/multiformats/go-multiaddr"
)

// peerWatcher implements network.Notifiee to watch peer connections.
type peerWatcher struct{}

func (pw *peerWatcher) Listen(network.Network, multiaddr.Multiaddr)      {}
func (pw *peerWatcher) ListenClose(network.Network, multiaddr.Multiaddr) {}
func (pw *peerWatcher) Connected(network.Network, network.Conn)          { NotifyTopologyChange() }
func (pw *peerWatcher) Disconnected(network.Network, network.Conn)       { NotifyTopologyChange() }

// watchPeerEvents registers a notifiee that fires topology changes on connect/disconnect.
func (d *Daemon) watchPeerEvents() {
	d.Node.Host.Network().Notify(&peerWatcher{})
}
