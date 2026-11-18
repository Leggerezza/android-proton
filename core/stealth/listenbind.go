package stealth

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/conn"
)

// ListenBind is the server-side counterpart of Bind: it terminates the TLS
// stream and speaks the same TunSafe framing. It exists so the transport can
// be exercised end to end (internal/tunnel/integration_test.go) without a
// Proton account.
//
// It accepts one TLS client, which is all a single WireGuard peer needs.
// Note: wireguard's device calls Bind.Close() and then Bind.Open() again on
// every BindUpdate, so Close() must not destroy the listening socket; use
// Shutdown() for that.
type ListenBind struct {
	ln               net.Listener
	cert             tls.Certificate
	handshakeTimeout time.Duration

	ready     chan struct{}
	acceptOne sync.Once

	mu       sync.Mutex
	conn     *tls.Conn
	closed   bool
	closedCh chan struct{}

	sendMu  sync.Mutex
	ts      *tunSafeState
	scratch []byte

	recvMu sync.Mutex
	hdr    [TunSafeHeaderSize]byte
	body   []byte
	data   []byte

	ep *Endpoint
}

// NewListenBind wraps an already listening TCP socket.
func NewListenBind(ln net.Listener, cert tls.Certificate) *ListenBind {
	return &ListenBind{
		ln:               ln,
		cert:             cert,
		handshakeTimeout: 10 * time.Second,
		ready:            make(chan struct{}),
		closedCh:         make(chan struct{}),
		ts:               newTunSafeState(),
		scratch:          make([]byte, 65535),
		body:             make([]byte, MaxFramePayload),
		data:             make([]byte, MaxFramePayload+wgDataHeaderSize+1),
		ep:               &Endpoint{},
	}
}

// Open re-arms the bind (wireguard calls Close+Open on every update).
func (b *ListenBind) Open(uint16) ([]conn.ReceiveFunc, uint16, error) {
	b.mu.Lock()
	b.closed = false
	select {
	case <-b.closedCh:
		b.closedCh = make(chan struct{})
	default:
	}
	b.mu.Unlock()

	port := 0
	if ta, ok := b.ln.Addr().(*net.TCPAddr); ok {
		port = ta.Port
	}
	b.acceptOne.Do(func() { go b.acceptLoop() })
	return []conn.ReceiveFunc{b.receive}, uint16(port), nil
}

func (b *ListenBind) acceptLoop() {
	raw, err := b.ln.Accept()
	if err != nil {
		b.acceptOne.Do(func() {})
		return
	}
	tc := tls.Server(raw, &tls.Config{Certificates: []tls.Certificate{b.cert}})
	_ = tc.SetDeadline(time.Now().Add(b.handshakeTimeout))
	if err := tc.Handshake(); err != nil {
		_ = raw.Close()
		return
	}
	_ = tc.SetDeadline(time.Time{})

	b.mu.Lock()
	b.conn = tc
	if ra, ok := raw.RemoteAddr().(*net.TCPAddr); ok {
		if ip, err := netip.ParseAddr(ra.IP.String()); err == nil {
			b.ep.addr = netip.AddrPortFrom(ip, uint16(ra.Port))
		}
	}
	b.mu.Unlock()
	close(b.ready)
}

func (b *ListenBind) waitConn() (*tls.Conn, error) {
	b.mu.Lock()
	ch := b.closedCh
	b.mu.Unlock()

	select {
	case <-b.ready:
	case <-ch:
		return nil, net.ErrClosed
	case <-time.After(30 * time.Second):
		return nil, errors.New("stealth: no TLS client connected")
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, net.ErrClosed
	}
	if b.conn == nil {
		return nil, errors.New("stealth: TLS client failed to handshake")
	}
	return b.conn, nil
}

func (b *ListenBind) receive(bufs [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
	c, err := b.waitConn()
	if err != nil {
		return 0, err
	}
	b.recvMu.Lock()
	defer b.recvMu.Unlock()

	pkt, err := b.ts.readFrame(c, b.hdr[:], b.body, b.data)
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			return 0, net.ErrClosed
		}
		return 0, temporary(fmt.Errorf("stealth(listen): read: %w", err))
	}
	if len(pkt) > len(bufs[0]) {
		return 0, fmt.Errorf("stealth(listen): packet of %d bytes does not fit", len(pkt))
	}
	sizes[0] = copy(bufs[0], pkt)
	eps[0] = b.ep
	return 1, nil
}

func (b *ListenBind) Send(bufs [][]byte, ep conn.Endpoint) error {
	c, err := b.waitConn()
	if err != nil {
		return err
	}
	b.sendMu.Lock()
	defer b.sendMu.Unlock()
	for _, buf := range bufs {
		if err := b.ts.writeFrame(c, buf, b.scratch); err != nil {
			return fmt.Errorf("stealth(listen): write: %w", err)
		}
	}
	return nil
}

// Close drops the current TLS connection but keeps the listener, as
// wireguard's BindUpdate expects to be able to Open() again.
func (b *ListenBind) Close() error {
	b.mu.Lock()
	b.closed = true
	select {
	case <-b.closedCh:
	default:
		close(b.closedCh)
	}
	c := b.conn
	b.conn = nil
	b.mu.Unlock()
	if c != nil {
		return c.Close()
	}
	return nil
}

// Shutdown closes the listening socket for good.
func (b *ListenBind) Shutdown() error {
	_ = b.Close()
	return b.ln.Close()
}

func (b *ListenBind) SetMark(uint32) error { return nil }

func (b *ListenBind) ParseEndpoint(s string) (conn.Endpoint, error) {
	if ap, err := netip.ParseAddrPort(s); err == nil {
		b.mu.Lock()
		b.ep.addr = ap
		b.mu.Unlock()
	}
	return b.ep, nil
}

func (b *ListenBind) BatchSize() int { return 1 }
