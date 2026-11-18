package stealth

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.zx2c4.com/wireguard/conn"
)

// defaultPostHandshakeDelay is what Proton's own client waits after the TLS
// handshake before sending the first WireGuard packet. Sending immediately
// loses the initiation on some paths (observed on the Windows baseline, see
// docs/PROTOCOL.md section 2).
const defaultPostHandshakeDelay = 100 * time.Millisecond

// DefaultHello is the TLS ClientHello fingerprint used for the Stealth
// handshake. Proton's own client picks at random from a set of Chrome
// fingerprints; a real Chrome fingerprint matters because plain Go TLS is a
// well-known DPI signature.
var DefaultHello = utls.HelloChrome_Auto

// HelloFromName maps a friendly name to a utls ClientHelloID.
func HelloFromName(name string) (utls.ClientHelloID, error) {
	switch name {
	case "", "chrome", "chrome-auto":
		return utls.HelloChrome_Auto, nil
	case "firefox":
		return utls.HelloFirefox_Auto, nil
	case "safari":
		return utls.HelloSafari_Auto, nil
	case "edge":
		return utls.HelloEdge_Auto, nil
	case "golang":
		return utls.HelloGolang, nil
	case "random":
		return utls.HelloRandomizedALPN, nil
	default:
		return utls.ClientHelloID{}, fmt.Errorf("unknown TLS fingerprint %q", name)
	}
}

// Config describes a Stealth (WireGuard over TLS) endpoint.
type Config struct {
	ServerIP   string // Proton entry IP
	Port       int    // TCP port, normally 443
	ServerName string // TLS SNI pinned to this name; empty = rotate the safe pool
	// ServerNames overrides the pool used when ServerName is empty.
	ServerNames []string
	Hello       utls.ClientHelloID
	DialTimeout time.Duration
	HandshakeTimeout time.Duration
	// Mark is applied to the TCP socket (SO_MARK) on platforms that support it.
	// Android has no SO_MARK for unprivileged apps, so this is a no-op there.
	Mark uint32
	// PostHandshakeDelay overrides defaultPostHandshakeDelay (tests use 0).
	PostHandshakeDelay time.Duration
	Logf func(format string, args ...any)
}

// Bind implements wireguard/conn.Bind on top of a single TLS stream. That is
// exactly what Proton calls "Stealth": the WireGuard packets are framed with
// TunSafe framing and carried as TLS application data over TCP/443.
type Bind struct {
	cfg   Config
	addr  string
	snis  []string
	sniAt int
	ep    *Endpoint

	connMu sync.Mutex
	tcp    net.Conn
	tls    *utls.UConn
	closed bool

	sendMu  sync.Mutex
	ts      *tunSafeState
	scratch []byte

	insecureOnce sync.Once

	recvMu sync.Mutex
	hdr    [TunSafeHeaderSize]byte
	body   []byte
	data   []byte
}

// NewBind creates a Stealth bind for one server. The TCP+TLS connection is
// established lazily on first use, then reused for the life of the tunnel.
func NewBind(cfg Config) (*Bind, error) {
	ip := net.ParseIP(cfg.ServerIP)
	if ip == nil {
		return nil, fmt.Errorf("invalid server IP %q", cfg.ServerIP)
	}
	if cfg.Port == 0 {
		cfg.Port = 443
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 10 * time.Second
	}
	if cfg.HandshakeTimeout == 0 {
		cfg.HandshakeTimeout = 10 * time.Second
	}
	if cfg.Hello.Client == "" {
		cfg.Hello = DefaultHello
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	var snis []string
	switch {
	case cfg.ServerName != "":
		snis = []string{cfg.ServerName}
	case len(cfg.ServerNames) > 0:
		snis = cfg.ServerNames
	default:
		snis = SafeServerNamesForIP(cfg.ServerIP)
	}
	ap := netip.AddrPortFrom(netip.MustParseAddr(cfg.ServerIP), uint16(cfg.Port))
	return &Bind{
		cfg:     cfg,
		addr:    ap.String(),
		snis:    snis,
		ep:      &Endpoint{addr: ap},
		ts:      newTunSafeState(),
		scratch: make([]byte, 65535),
		body:    make([]byte, MaxFramePayload),
		data:    make([]byte, MaxFramePayload+wgDataHeaderSize+1),
	}, nil
}

// ServerName returns the SNI of the next connection attempt.
func (b *Bind) ServerName() string {
	b.connMu.Lock()
	defer b.connMu.Unlock()
	return b.currentSNI()
}

func (b *Bind) currentSNI() string {
	if len(b.snis) == 0 {
		return ServerNameForIP(b.cfg.ServerIP)
	}
	return b.snis[b.sniAt%len(b.snis)]
}

// rotateSNI moves to the next name in the pool; called after a failed TLS
// handshake, which in mainland China usually means the SNI is blacklisted.
func (b *Bind) rotateSNI() {
	if len(b.snis) > 1 {
		b.sniAt = (b.sniAt + 1) % len(b.snis)
	}
}

// Endpoint returns the (single) endpoint this bind talks to.
func (b *Bind) Endpoint() conn.Endpoint { return b.ep }

func (b *Bind) Open(port uint16) ([]conn.ReceiveFunc, uint16, error) {
	b.connMu.Lock()
	b.closed = false
	b.connMu.Unlock()
	return []conn.ReceiveFunc{b.receive}, port, nil
}

func (b *Bind) receive(bufs [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
	b.recvMu.Lock()
	defer b.recvMu.Unlock()

	c, err := b.getConn()
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			return 0, net.ErrClosed
		}
		return 0, temporary(err)
	}

	pkt, err := b.ts.readFrame(c, b.hdr[:], b.body, b.data)
	if err != nil {
		b.resetConn()
		// A closed/EOF stream is final: the device tears down and the node
		// supervisor reconnects with a fresh TLS session. Only transient
		// errors keep the wireguard receive loop alive.
		if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return 0, net.ErrClosed
		}
		return 0, temporary(fmt.Errorf("stealth: read: %w", err))
	}

	if len(pkt) > len(bufs[0]) {
		return 0, fmt.Errorf("stealth: packet of %d bytes does not fit the %d-byte buffer", len(pkt), len(bufs[0]))
	}
	n := copy(bufs[0], pkt)
	sizes[0] = n
	eps[0] = b.ep
	return 1, nil
}

func (b *Bind) Send(bufs [][]byte, ep conn.Endpoint) error {
	c, err := b.getConn()
	if err != nil {
		return err
	}

	b.sendMu.Lock()
	defer b.sendMu.Unlock()
	for _, buf := range bufs {
		if err := b.ts.writeFrame(c, buf, b.scratch); err != nil {
			b.resetConn()
			return fmt.Errorf("stealth: write: %w", err)
		}
	}
	return nil
}

func (b *Bind) Close() error {
	b.connMu.Lock()
	defer b.connMu.Unlock()
	b.closed = true
	return b.closeLocked()
}

func (b *Bind) SetMark(mark uint32) error {
	b.connMu.Lock()
	b.cfg.Mark = mark
	b.connMu.Unlock()
	return nil
}

func (b *Bind) ParseEndpoint(s string) (conn.Endpoint, error) {
	ap, err := netip.ParseAddrPort(s)
	if err != nil {
		return nil, err
	}
	b.connMu.Lock()
	b.ep = &Endpoint{addr: ap}
	ep := b.ep
	b.connMu.Unlock()
	return ep, nil
}

func (b *Bind) BatchSize() int { return 1 }

// getConn returns a live TLS connection, dialling and handshaking if needed.
func (b *Bind) getConn() (net.Conn, error) {
	b.connMu.Lock()
	defer b.connMu.Unlock()

	if b.closed {
		return nil, net.ErrClosed
	}
	if b.tls != nil {
		return b.tls, nil
	}
	if b.tcp == nil {
		d := net.Dialer{Timeout: b.cfg.DialTimeout}
		c, err := d.Dial("tcp", b.addr)
		if err != nil {
			return nil, fmt.Errorf("stealth: tcp dial %s: %w", b.addr, err)
		}
		if tc, ok := c.(*net.TCPConn); ok {
			_ = tc.SetNoDelay(true)
			_ = tc.SetKeepAlive(true)
			_ = tc.SetKeepAlivePeriod(25 * time.Second)
		}
		b.cfg.Logf("stealth: tcp connected to %s", b.addr)
		b.tcp = c
	}

	sni := b.currentSNI()
	b.warnInsecureOnce()
	tlsConf := &utls.Config{
		InsecureSkipVerify: true, // Proton's Stealth terminates TLS with a rotating cert
		ServerName:         sni,
	}
	uc := utls.UClient(b.tcp, tlsConf, b.cfg.Hello)
	_ = uc.SetDeadline(time.Now().Add(b.cfg.HandshakeTimeout))
	if err := uc.Handshake(); err != nil {
		_ = uc.Close()
		b.tcp = nil
		b.tls = nil
		b.rotateSNI()
		b.cfg.Logf("stealth: TLS handshake with %s failed using sni %q (%v); next attempt uses %q",
			b.addr, sni, err, b.currentSNI())
		return nil, fmt.Errorf("stealth: TLS handshake with %s (sni %s): %w", b.addr, sni, err)
	}
	_ = uc.SetDeadline(time.Time{})
	b.ts.reset()
	b.tls = uc
	b.cfg.Logf("stealth: TLS established with %s (sni %s)", b.addr, sni)

	// Proton sleeps briefly after the handshake: on some networks the first
	// WireGuard initiation sent immediately after the handshake is lost.
	if d := b.cfg.PostHandshakeDelay; d != 0 {
		time.Sleep(d)
	} else {
		time.Sleep(defaultPostHandshakeDelay)
	}
	return uc, nil
}

// warnInsecureOnce records the one intentional certificate bypass: Proton's
// Stealth terminator presents a rotating certificate, so validating it is
// impossible (PROMPT.md 0.4 allows exactly this endpoint, but it must be
// logged once so the bypass is never silent).
func (b *Bind) warnInsecureOnce() {
	b.insecureOnce.Do(func() {
		b.cfg.Logf("stealth: certificate verification is skipped for the Proton entry (rotating cert; PROMPT.md 0.4)")
	})
}

func (b *Bind) resetConn() {
	b.connMu.Lock()
	defer b.connMu.Unlock()
	_ = b.closeLocked()
}

func (b *Bind) closeLocked() error {
	var err error
	if b.tls != nil {
		err = b.tls.Close()
		b.tls = nil
	}
	if b.tcp != nil {
		_ = b.tcp.Close()
		b.tcp = nil
	}
	b.ts.reset()
	return err
}

// Endpoint is the single remote endpoint of a Stealth tunnel.
type Endpoint struct {
	addr netip.AddrPort
}

func (e *Endpoint) ClearSrc()           {}
func (e *Endpoint) SrcToString() string { return "" }
func (e *Endpoint) DstToString() string { return e.addr.String() }
func (e *Endpoint) DstIP() netip.Addr   { return e.addr.Addr() }
func (e *Endpoint) SrcIP() netip.Addr   { return netip.Addr{} }
func (e *Endpoint) DstToBytes() []byte  { return e.addr.Addr().AsSlice() }

// temporary marks an error as retryable so wireguard-go keeps its receive
// loop alive across a dropped TLS connection instead of tearing the device
// down.
type temporaryError struct{ err error }

func (e *temporaryError) Error() string   { return e.err.Error() }
func (e *temporaryError) Unwrap() error   { return e.err }
func (e *temporaryError) Timeout() bool   { return false }
func (e *temporaryError) Temporary() bool { return true }

func temporary(err error) error { return &temporaryError{err: err} }
