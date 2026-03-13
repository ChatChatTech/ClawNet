package p2p

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

// mdnsNotifee handles mDNS peer discovery events.
type mdnsNotifee struct {
	host host.Host
	ctx  context.Context
}

func (n *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	if pi.ID == n.host.ID() {
		return
	}
	fmt.Printf("mDNS: discovered peer %s\n", pi.ID.String()[:16])
	if err := n.host.Connect(n.ctx, pi); err != nil {
		fmt.Printf("mDNS: failed to connect to %s: %v\n", pi.ID.String()[:16], err)
	}
}
