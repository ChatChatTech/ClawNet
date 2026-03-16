package overlay

// Link-level connection management for the ClawNet overlay network.
// Link management for ClawNet overlay transport — manages TCP/TLS connections
// with per-link byte counting, exponential backoff, and URI-based addressing.

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// linkType classifies how a link was established.
type linkType int

const (
	linkTypePersistent linkType = iota // Statically configured peer
	linkTypeEphemeral                  // One-time call
	linkTypeIncoming                   // Accepted inbound connection
)

const defaultBackoffLimit = time.Second << 12 // ~1h8m16s

// linkInfo is the map key identifying a unique link (URI without query params).
type linkInfo struct {
	uri   string
	sintf string // source interface, reserved for future use
}

// link tracks the state of a single peering connection.
type link struct {
	ctx       context.Context
	cancel    context.CancelFunc
	kick      chan struct{} // signal to retry immediately
	linkType  linkType
	linkProto string // "TCP", "TLS"
	// Protected by links.mu
	conn    *linkConn
	err     error
	errtime time.Time
}

// linkOptions are per-peer connection parameters parsed from URI query string.
type linkOptions struct {
	priority   uint8
	password   []byte
	maxBackoff time.Duration
}

// linkConn wraps net.Conn with atomic RX/TX byte counters and per-second
// rate tracking. Modeled after the link connection pattern for per-link traffic stats.
type linkConn struct {
	// rx/tx at struct start for 64-bit alignment on 32-bit platforms
	rx     uint64
	tx     uint64
	rxrate uint64
	txrate uint64
	lastrx uint64
	lasttx uint64
	up     time.Time
	net.Conn
}

func (c *linkConn) Read(p []byte) (n int, err error) {
	n, err = c.Conn.Read(p)
	atomic.AddUint64(&c.rx, uint64(n))
	return
}

func (c *linkConn) Write(p []byte) (n int, err error) {
	n, err = c.Conn.Write(p)
	atomic.AddUint64(&c.tx, uint64(n))
	return
}

// links manages all overlay link connections (TCP and TLS).
// Uses sync.Mutex for thread safety (simplified actor model).
type links struct {
	transport  *Transport
	tcp        *linkTCP
	tls        *linkTLS
	mu         sync.Mutex
	_links     map[linkInfo]*link
	_listeners map[*Listener]context.CancelFunc
}

// Listener wraps a net.Listener with cancellation support.
type Listener struct {
	listener net.Listener
	ctx      context.Context
	Cancel   context.CancelFunc
}

func (li *Listener) Addr() net.Addr {
	return li.listener.Addr()
}

// Link error types
type linkError string

func (e linkError) Error() string { return string(e) }

const (
	ErrLinkAlreadyConfigured  = linkError("peer is already configured")
	ErrLinkNotConfigured      = linkError("peer is not configured")
	ErrLinkUnrecognisedSchema = linkError("unsupported link schema")
	ErrLinkNoSuitableIPs      = linkError("no suitable IPs found")
	ErrLinkToSelf             = linkError("cannot connect to self")
)

func (l *links) init(t *Transport) {
	l.transport = t
	l.tcp = newLinkTCP(l)
	l.tls = newLinkTLS(l)
	l._links = make(map[linkInfo]*link)
	l._listeners = make(map[*Listener]context.CancelFunc)
	go l.updateAverages()
}

// updateAverages calculates per-second RX/TX rates for all active links.
func (l *links) updateAverages() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-l.transport.ctx.Done():
			return
		case <-ticker.C:
		}
		l.mu.Lock()
		for _, lnk := range l._links {
			if lnk.conn == nil {
				continue
			}
			rx := atomic.LoadUint64(&lnk.conn.rx)
			tx := atomic.LoadUint64(&lnk.conn.tx)
			lastrx := atomic.LoadUint64(&lnk.conn.lastrx)
			lasttx := atomic.LoadUint64(&lnk.conn.lasttx)
			atomic.StoreUint64(&lnk.conn.rxrate, rx-lastrx)
			atomic.StoreUint64(&lnk.conn.txrate, tx-lasttx)
			atomic.StoreUint64(&lnk.conn.lastrx, rx)
			atomic.StoreUint64(&lnk.conn.lasttx, tx)
		}
		l.mu.Unlock()
	}
}

func (l *links) shutdown() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, cancel := range l._listeners {
		cancel()
	}
	for _, lnk := range l._links {
		lnk.cancel()
		if lnk.conn != nil {
			_ = lnk.conn.Close()
		}
	}
}

// add adds a peer by URI and starts the connection loop with built-in backoff.
func (l *links) add(rawURI string, lt linkType) error {
	u, err := url.Parse(rawURI)
	if err != nil {
		return fmt.Errorf("invalid peer URI %q: %w", rawURI, err)
	}
	if _, err := l.dialerFor(u); err != nil {
		return err
	}

	options := l.parseOptions(u)
	lu := urlForLinkInfo(*u)
	info := linkInfo{uri: lu.String()}

	l.mu.Lock()
	if state, ok := l._links[info]; ok && state != nil {
		l.mu.Unlock()
		// Kick existing link to retry immediately
		select {
		case state.kick <- struct{}{}:
		default:
		}
		return ErrLinkAlreadyConfigured
	}

	state := &link{
		linkType:  lt,
		linkProto: strings.ToUpper(u.Scheme),
		kick:      make(chan struct{}),
	}
	state.ctx, state.cancel = context.WithCancel(l.transport.ctx)
	l._links[info] = state
	l.mu.Unlock()

	go l.connectLoop(u, info, state, options, lt)
	return nil
}

func (l *links) parseOptions(u *url.URL) linkOptions {
	options := linkOptions{maxBackoff: defaultBackoffLimit}
	if p := u.Query().Get("priority"); p != "" {
		if pi, err := strconv.ParseUint(p, 10, 8); err == nil {
			options.priority = uint8(pi)
		}
	}
	if p := u.Query().Get("password"); p != "" {
		options.password = []byte(p)
	}
	if p := u.Query().Get("maxbackoff"); p != "" {
		if d, err := time.ParseDuration(p); err == nil && d >= 5*time.Second {
			options.maxBackoff = d
		}
	}
	return options
}

// connectLoop is the persistent connection goroutine with exponential backoff.
// Modeled after the link connection loop pattern.
func (l *links) connectLoop(u *url.URL, info linkInfo, state *link, options linkOptions, lt linkType) {
	defer func() {
		l.mu.Lock()
		if l._links[info] == state {
			delete(l._links, info)
		}
		l.mu.Unlock()
	}()

	var backoff int

	backoffWait := func() bool {
		if backoff >= 0 && backoff < 32 {
			backoff++
		}
		if backoff < 0 {
			return false // permanent failure (e.g. self-connection)
		}
		duration := time.Second << backoff
		if duration > options.maxBackoff {
			duration = options.maxBackoff
		}
		select {
		case <-state.kick:
			return true
		case <-state.ctx.Done():
			return false
		case <-time.After(duration):
			return true
		}
	}

	resetBackoff := func() { backoff = 0 }

	for {
		select {
		case <-state.ctx.Done():
			return
		default:
		}

		conn, err := l.connect(state.ctx, u)
		if err != nil {
			if lt == linkTypePersistent {
				l.mu.Lock()
				state.conn = nil
				state.err = err
				state.errtime = time.Now()
				l.mu.Unlock()
				if backoffWait() {
					continue
				}
				return
			}
			return // ephemeral: don't retry
		}

		lc := &linkConn{Conn: conn, up: time.Now()}

		l.mu.Lock()
		if state.conn != nil {
			l.mu.Unlock()
			_ = conn.Close()
			return
		}
		state.conn = lc
		state.err = nil
		state.errtime = time.Time{}
		l.mu.Unlock()

		// handler blocks for the lifetime of the connection.
		// resetBackoff is called inside handler after successful handshake.
		err = l.handler(lt, options, lc, resetBackoff)

		switch {
		case errors.Is(err, ErrLinkToSelf):
			backoff = -1 // permanent, never retry
		}

		_ = lc.Close()
		l.mu.Lock()
		state.conn = nil
		if err == nil {
			err = fmt.Errorf("remote side closed the connection")
		}
		state.err = err
		state.errtime = time.Now()
		l.mu.Unlock()

		if lt == linkTypePersistent {
			if backoffWait() {
				continue
			}
		}
		return
	}
}

// remove closes a peer connection and stops reconnection.
func (l *links) remove(rawURI string) error {
	u, err := url.Parse(rawURI)
	if err != nil {
		return err
	}
	lu := urlForLinkInfo(*u)
	info := linkInfo{uri: lu.String()}

	l.mu.Lock()
	defer l.mu.Unlock()
	state, ok := l._links[info]
	if !ok || state == nil {
		return ErrLinkNotConfigured
	}
	state.cancel()
	if state.conn != nil {
		return state.conn.Close()
	}
	return nil
}

// listen starts a listener for incoming connections on the given URI.
func (l *links) listen(rawURI string) (*Listener, error) {
	u, err := url.Parse(rawURI)
	if err != nil {
		return nil, err
	}
	var proto interface {
		listen(ctx context.Context, u *url.URL) (net.Listener, error)
	}
	switch strings.ToLower(u.Scheme) {
	case "tcp":
		proto = l.tcp
	case "tls":
		proto = l.tls
	default:
		return nil, ErrLinkUnrecognisedSchema
	}

	ctx, ctxcancel := context.WithCancel(l.transport.ctx)
	listener, err := proto.listen(ctx, u)
	if err != nil {
		ctxcancel()
		return nil, err
	}

	addr := listener.Addr()
	cancel := func() {
		ctxcancel()
		_ = listener.Close()
	}
	li := &Listener{listener: listener, ctx: ctx, Cancel: cancel}

	options := l.parseOptions(u)

	l.mu.Lock()
	l._listeners[li] = cancel
	l.mu.Unlock()

	go func() {
		fmt.Printf("[overlay] %s listener started on %s\n", strings.ToUpper(u.Scheme), addr)
		defer func() {
			cancel()
			l.mu.Lock()
			delete(l._listeners, li)
			l.mu.Unlock()
			fmt.Printf("[overlay] %s listener stopped on %s\n", strings.ToUpper(u.Scheme), addr)
		}()
		for {
			conn, err := li.listener.Accept()
			if err != nil {
				return
			}
			go l.handleIncoming(u, conn, options)
		}
	}()
	return li, nil
}

func (l *links) handleIncoming(u *url.URL, conn net.Conn, options linkOptions) {
	defer conn.Close()

	pu := *u
	pu.Host = conn.RemoteAddr().String()
	pu.RawQuery = ""
	info := linkInfo{uri: pu.String()}

	lc := &linkConn{Conn: conn, up: time.Now()}
	state := &link{
		linkType:  linkTypeIncoming,
		linkProto: strings.ToUpper(u.Scheme),
		kick:      make(chan struct{}),
		conn:      lc,
	}
	state.ctx, state.cancel = context.WithCancel(l.transport.ctx)

	l.mu.Lock()
	if existing, ok := l._links[info]; ok && existing != nil && existing.conn != nil {
		l.mu.Unlock()
		return // already connected
	}
	l._links[info] = state
	l.mu.Unlock()

	defer func() {
		l.mu.Lock()
		if l._links[info] == state {
			delete(l._links, info)
		}
		l.mu.Unlock()
	}()

	switch err := l.handler(linkTypeIncoming, options, lc, nil); {
	case err == nil, errors.Is(err, io.EOF), errors.Is(err, net.ErrClosed):
	default:
		fmt.Printf("[overlay] incoming link %s error: %v\n", conn.RemoteAddr(), err)
	}
}

// handler performs the wire handshake then hands the
// connection to ironwood. onSuccess is called after handshake succeeds
// (resets backoff). Blocks until the link disconnects.
func (l *links) handler(lt linkType, options linkOptions, conn *linkConn, onSuccess func()) error {
	remoteKey, prio, err := overlayHandshake(conn, l.transport.privKey, options.priority, options.password)
	if err != nil {
		return fmt.Errorf("handshake: %w", err)
	}

	localPub := l.transport.privKey.Public().(ed25519.PublicKey)
	if remoteKey.Equal(localPub) {
		return ErrLinkToSelf
	}

	dir := "outbound"
	if lt == linkTypeIncoming {
		dir = "inbound"
	}
	keyHex := hex.EncodeToString(remoteKey[:8])
	fmt.Printf("[overlay] connected %s: %s@%s\n", dir, keyHex, conn.RemoteAddr())

	// Register as known ClawNet peer (completed our wire handshake)
	l.transport.RegisterClawPeer(remoteKey)

	if onSuccess != nil {
		onSuccess()
	}

	err = l.transport.pc.HandleConn(remoteKey, conn, prio)
	switch {
	case err == nil, errors.Is(err, io.EOF), errors.Is(err, net.ErrClosed):
		fmt.Printf("[overlay] disconnected %s: %s@%s\n", dir, keyHex, conn.RemoteAddr())
	default:
		fmt.Printf("[overlay] disconnected %s: %s@%s; error: %v\n", dir, keyHex, conn.RemoteAddr(), err)
	}
	return err
}

// connect dispatches to the appropriate transport dialer.
func (l *links) connect(ctx context.Context, u *url.URL) (net.Conn, error) {
	dialer, err := l.dialerFor(u)
	if err != nil {
		return nil, err
	}
	return dialer.dial(ctx, u)
}

type linkDialer interface {
	dial(ctx context.Context, u *url.URL) (net.Conn, error)
}

func (l *links) dialerFor(u *url.URL) (linkDialer, error) {
	switch strings.ToLower(u.Scheme) {
	case "tcp":
		return l.tcp, nil
	case "tls":
		return l.tls, nil
	default:
		return nil, ErrLinkUnrecognisedSchema
	}
}

// findSuitableIP resolves the URL host and tries each IP until one connects.
func (l *links) findSuitableIP(u *url.URL, fn func(hostname string, ip net.IP, port int) (net.Conn, error)) (net.Conn, error) {
	host, p, err := net.SplitHostPort(u.Host)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return nil, err
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	var filtered []net.IP
	for _, ip := range ips {
		if ip.IsUnspecified() || ip.IsMulticast() {
			continue
		}
		filtered = append(filtered, ip)
	}
	if len(filtered) == 0 {
		return nil, ErrLinkNoSuitableIPs
	}
	for _, ip := range filtered {
		conn, dialErr := fn(host, ip, port)
		if dialErr != nil {
			err = dialErr
			continue
		}
		return conn, nil
	}
	return nil, err
}

// RetryPeersNow kicks all links to attempt reconnection immediately.
func (l *links) RetryPeersNow() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, lnk := range l._links {
		select {
		case lnk.kick <- struct{}{}:
		default:
		}
	}
}

// PeerInfo contains rich information about a link peer, merging
// link-layer stats (RX/TX bytes, rate, uptime) with ironwood routing info.
// PeerInfo holds status for a single overlay link.
// Returned by GetPeers() for API consumers.
type PeerInfo struct {
	URI           string        `json:"uri"`
	Up            bool          `json:"up"`
	Inbound       bool          `json:"inbound"`
	LastError     string        `json:"last_error,omitempty"`
	LastErrorTime time.Time     `json:"last_error_time,omitempty"`
	Key           string        `json:"key"`
	Root          string        `json:"root"`
	Port          uint64        `json:"port"`
	Priority      uint8         `json:"priority"`
	RXBytes       uint64        `json:"rx_bytes"`
	TXBytes       uint64        `json:"tx_bytes"`
	RXRate        uint64        `json:"rx_rate"`
	TXRate        uint64        `json:"tx_rate"`
	Uptime        time.Duration `json:"uptime"`
	Latency       time.Duration `json:"latency"`
	RemoteAddr    string        `json:"remote_addr"`
}

// GetPeers returns rich peer info by merging link-layer and ironwood stats.
// GetPeers returns status info for all overlay links.
func (l *links) GetPeers() []PeerInfo {
	iwPeers := l.transport.pc.PacketConn.Debug.GetPeers()
	type iwInfo struct {
		Key, Root []byte
		Port      uint64
		Priority  uint8
		Latency   time.Duration
	}
	connMap := make(map[net.Conn]iwInfo, len(iwPeers))
	for _, p := range iwPeers {
		connMap[p.Conn] = iwInfo{p.Key, p.Root, p.Port, p.Priority, p.Latency}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	peers := make([]PeerInfo, 0, len(l._links))
	for info, state := range l._links {
		pi := PeerInfo{URI: info.uri}
		if state.err != nil {
			pi.LastError = state.err.Error()
			pi.LastErrorTime = state.errtime
		}
		var conn net.Conn
		if c := state.conn; c != nil {
			conn = c
			pi.Up = true
			pi.Inbound = state.linkType == linkTypeIncoming
			pi.RXBytes = atomic.LoadUint64(&c.rx)
			pi.TXBytes = atomic.LoadUint64(&c.tx)
			pi.RXRate = atomic.LoadUint64(&c.rxrate)
			pi.TXRate = atomic.LoadUint64(&c.txrate)
			pi.Uptime = time.Since(c.up)
			pi.RemoteAddr = c.RemoteAddr().String()
		}
		if iw, ok := connMap[conn]; ok {
			pi.Key = hex.EncodeToString(iw.Key)
			pi.Root = hex.EncodeToString(iw.Root)
			pi.Port = iw.Port
			pi.Priority = iw.Priority
			pi.Latency = iw.Latency
		}
		peers = append(peers, pi)
	}
	return peers
}

// urlForLinkInfo strips query parameters from a URL for use as a dedup key.
func urlForLinkInfo(u url.URL) url.URL {
	u.RawQuery = ""
	return u
}
