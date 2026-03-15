package overlay

// TCP transport for overlay links.
// Inspired by Yggdrasil's src/core/link_tcp.go.

import (
	"context"
	"net"
	"net/url"
	"time"
)

type linkTCP struct {
	links *links
}

func newLinkTCP(l *links) *linkTCP {
	return &linkTCP{links: l}
}

func (t *linkTCP) dial(ctx context.Context, u *url.URL) (net.Conn, error) {
	return t.links.findSuitableIP(u, func(_ string, ip net.IP, port int) (net.Conn, error) {
		addr := &net.TCPAddr{IP: ip, Port: port}
		dialer := &net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: -1,
		}
		return dialer.DialContext(ctx, "tcp", addr.String())
	})
}

func (t *linkTCP) listen(ctx context.Context, u *url.URL) (net.Listener, error) {
	lc := &net.ListenConfig{KeepAlive: -1}
	return lc.Listen(ctx, "tcp", u.Host)
}
