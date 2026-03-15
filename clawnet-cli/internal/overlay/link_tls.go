package overlay

// TLS transport for ClawNet overlay links.
// TLS provides transport encryption; the wire handshake
// handles identity authentication (ed25519 signature verification).

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"time"
)

type linkTLS struct {
	links  *links
	config *tls.Config
}

func newLinkTLS(l *links) *linkTLS {
	return &linkTLS{
		links: l,
		config: &tls.Config{
			InsecureSkipVerify: true, // overlay handshake is the real auth layer
		},
	}
}

func (t *linkTLS) dial(ctx context.Context, u *url.URL) (net.Conn, error) {
	cfg := t.config.Clone()
	return t.links.findSuitableIP(u, func(hostname string, ip net.IP, port int) (net.Conn, error) {
		cfg.ServerName = hostname
		addr := &net.TCPAddr{IP: ip, Port: port}
		dialer := &net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: -1,
		}
		tlsDialer := &tls.Dialer{
			NetDialer: dialer,
			Config:    cfg,
		}
		return tlsDialer.DialContext(ctx, "tcp", addr.String())
	})
}

func (t *linkTLS) listen(ctx context.Context, u *url.URL) (net.Listener, error) {
	if t.config.Certificates == nil && t.config.GetCertificate == nil {
		return nil, fmt.Errorf("TLS listener requires a server certificate")
	}
	lc := &net.ListenConfig{KeepAlive: -1}
	listener, err := lc.Listen(ctx, "tcp", u.Host)
	if err != nil {
		return nil, err
	}
	return tls.NewListener(listener, t.config), nil
}
