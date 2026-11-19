package socks5

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"
)

// SOCKS5 UDP ASSOCIATE (RFC 1928 section 7) as required by PROMPT.md 5.4:
//   - the relay binds 127.0.0.1:0 and returns the real BND in the reply;
//   - its lifetime is the TCP control connection (closing that tears it down);
//   - every datagram is RSV(2)=0 | FRAG | ATYP | DST.ADDR | DST.PORT | DATA,
//     FRAG != 0 is dropped (no fragmentation support), all three ATYP work;
//   - per association the relay keeps at most udpMaxSessions (dst -> socket)
//     and recycles an idle one after udpIdleTimeout (120s), so a game keeps the
//     same source port for the same destination instead of re-creating sockets;
//   - a single datagram is at most 65507 bytes of UDP payload.

const (
	udpDefaultIdleTimeout = 120 * time.Second
	udpDefaultMaxSessions = 64
	// MaxUDPDatagram is the largest UDP payload a SOCKS5 datagram can carry:
	// 65507 (max UDP payload) is the practical cap; the header adds at most 262.
	MaxUDPDatagram = 65507
)

// UDPDialFunc opens a UDP socket to address ("host:port") through the tunnel.
// Domain names are resolved by the callee (inside the tunnel).
type UDPDialFunc func(ctx context.Context, address string) (net.Conn, error)

// handleUDPAssociate serves one association until the control connection closes.
func (s *Server) handleUDPAssociate(ctx context.Context, control net.Conn) {
	if s.DialUDP == nil {
		_ = writeReply(control, repCommandUnsupported, nil)
		s.Logf("socks5: UDP ASSOCIATE refused (no UDP dialer configured)")
		return
	}
	relay, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		_ = writeReply(control, repGeneralFailure, nil)
		s.Logf("socks5: UDP ASSOCIATE cannot bind the relay: %v", err)
		return
	}
	defer relay.Close()

	if err := writeReply(control, repSuccess, relay.LocalAddr()); err != nil {
		return
	}
	assoc := newUDPAssociation(s, relay)
	s.Logf("socks5: UDP ASSOCIATE on %s", relay.LocalAddr())

	assocCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// The association dies with the TCP control connection: a read here returns
	// as soon as the client closes it (mihomo closes it on shutdown).
	go func() {
		buf := make([]byte, 1)
		for {
			if _, err := control.Read(buf); err != nil {
				cancel()
				return
			}
		}
	}()

	// Closing the socket on cancellation unblocks serve() at once; waiting for
	// the per-read deadline would keep answering for up to a second after the
	// control connection is gone.
	go func() {
		<-assocCtx.Done()
		_ = relay.Close()
	}()

	assoc.reapLoop(assocCtx)
	assoc.serve(assocCtx)
	cancel()
	assoc.closeAll()
	s.Logf("socks5: UDP ASSOCIATE closed")
}

// udpAssociation is one (client, relay socket) pair plus its dst -> socket map.
type udpAssociation struct {
	srv  *Server
	conn *net.UDPConn

	mu                sync.Mutex
	loggedFirstPacket bool
	peer              *net.UDPAddr
	sessions          map[string]*udpSession
	order             []string // oldest first, used as the LRU list
	closed            bool
}

type udpSession struct {
	key      string
	upstream net.Conn
	remote   netip.AddrPort
	lastUsed time.Time
}

func newUDPAssociation(srv *Server, conn *net.UDPConn) *udpAssociation {
	return &udpAssociation{
		srv:      srv,
		conn:     conn,
		sessions: make(map[string]*udpSession),
	}
}

func (a *udpAssociation) logf(format string, args ...any) {
	if a.srv != nil && a.srv.Logf != nil {
		a.srv.Logf(format, args...)
	}
}

func (a *udpAssociation) idleTimeout() time.Duration {
	if a.srv.UDPIdleTimeout > 0 {
		return a.srv.UDPIdleTimeout
	}
	return udpDefaultIdleTimeout
}

func (a *udpAssociation) maxSessions() int {
	if a.srv.UDPMaxSessions > 0 {
		return a.srv.UDPMaxSessions
	}
	return udpDefaultMaxSessions
}

// serve reads client datagrams and forwards them through the tunnel.
func (a *udpAssociation) serve(ctx context.Context) {
	buf := make([]byte, MaxUDPDatagram+512)
	for {
		if ctx.Err() != nil {
			return
		}
		_ = a.conn.SetReadDeadline(time.Now().Add(time.Second))
		n, from, err := a.conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return
		}
		if !a.acceptPeer(from) {
			a.logf("socks5: UDP packet from a foreign source dropped")
			continue
		}
		if !a.loggedFirstPacket {
			a.loggedFirstPacket = true
			a.logf("socks5: UDP datagram received (%d bytes)", n)
		}
		frag, dst, payload, err := parseUDPDatagram(buf[:n])
		if err != nil {
			a.logf("socks5: malformed UDP datagram dropped: %v", err)
			continue
		}
		if frag != 0 {
			// No fragmentation support: RFC 1928 says drop, never half-assemble.
			continue
		}
		if len(payload) > MaxUDPDatagram {
			continue
		}
		sess, err := a.session(ctx, dst)
		if err != nil {
			a.logf("socks5: UDP dial failed (dropped): %v", err)
			continue
		}
		_ = sess.upstream.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := sess.upstream.Write(payload); err != nil {
			a.logf("socks5: UDP write failed: %v", err)
			a.dropSession(sess.key)
		}
	}
}

// acceptPeer pins the association to the first client address that talks to it.
func (a *udpAssociation) acceptPeer(from *net.UDPAddr) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.peer == nil {
		a.peer = from
		return true
	}
	return a.peer.IP.Equal(from.IP) && a.peer.Port == from.Port
}

// session returns the socket for dst, creating it (and reusing an idle one)
// under the LRU and idle rules.
func (a *udpAssociation) session(ctx context.Context, dst string) (*udpSession, error) {
	a.mu.Lock()
	if sess, ok := a.sessions[dst]; ok {
		sess.lastUsed = time.Now()
		a.touchLocked(dst)
		a.mu.Unlock()
		return sess, nil
	}
	max := a.maxSessions()
	if len(a.sessions) >= max && len(a.order) > 0 {
		oldest := a.order[0]
		a.order = a.order[1:]
		if ev, ok := a.sessions[oldest]; ok {
			delete(a.sessions, oldest)
			_ = ev.upstream.Close()
			a.logf("socks5: UDP session evicted (LRU, cap %d)", max)
		}
	}
	a.mu.Unlock()

	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	upstream, err := a.srv.DialUDP(dctx, dst)
	if err != nil {
		return nil, err
	}
	remote := addrPortOf(upstream.RemoteAddr())

	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		_ = upstream.Close()
		return nil, errors.New("association closed")
	}
	// A concurrent create for the same dst wins; do not leak a second socket.
	if existing, ok := a.sessions[dst]; ok {
		existing.lastUsed = time.Now()
		a.touchLocked(dst)
		a.mu.Unlock()
		_ = upstream.Close()
		return existing, nil
	}
	sess := &udpSession{key: dst, upstream: upstream, remote: remote, lastUsed: time.Now()}
	a.sessions[dst] = sess
	a.order = append(a.order, dst)
	active := len(a.sessions)
	a.mu.Unlock()
	a.logf("socks5: UDP session opened (active %d)", active)

	go a.readUpstream(ctx, sess)
	return sess, nil
}

// readUpstream forwards one socket's replies back to the client.
func (a *udpAssociation) readUpstream(ctx context.Context, sess *udpSession) {
	buf := make([]byte, MaxUDPDatagram)
	for {
		if ctx.Err() != nil {
			return
		}
		_ = sess.upstream.SetReadDeadline(time.Now().Add(time.Second))
		n, err := sess.upstream.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			a.dropSession(sess.key)
			a.logf("socks5: UDP upstream read ended: %v", err)
			return
		}
		a.mu.Lock()
		peer := a.peer
		a.mu.Unlock()
		if peer == nil {
			continue
		}
		pkt := encodeUDPDatagram(sess.remote, buf[:n])
		_ = a.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := a.conn.WriteToUDP(pkt, peer); err != nil {
			return
		}
	}
}

func (a *udpAssociation) touchLocked(key string) {
	for i, k := range a.order {
		if k == key {
			a.order = append(append(a.order[:i], a.order[i+1:]...), key)
			return
		}
	}
	a.order = append(a.order, key)
}

func (a *udpAssociation) dropSession(key string) {
	a.mu.Lock()
	sess, ok := a.sessions[key]
	if ok {
		delete(a.sessions, key)
		for i, k := range a.order {
			if k == key {
				a.order = append(a.order[:i], a.order[i+1:]...)
				break
			}
		}
	}
	a.mu.Unlock()
	if ok {
		_ = sess.upstream.Close()
	}
}

// reapLoop closes sessions that stayed idle longer than the configured timeout.
func (a *udpAssociation) reapLoop(ctx context.Context) {
	idle := a.idleTimeout()
	interval := idle / 4
	if interval < 50*time.Millisecond {
		interval = 50 * time.Millisecond
	}
	if interval > 30*time.Second {
		interval = 30 * time.Second
	}
	tick := time.NewTicker(interval)
	go func() {
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				deadline := time.Now().Add(-idle)
				var stale []string
				a.mu.Lock()
				for k, s := range a.sessions {
					if s.lastUsed.Before(deadline) {
						stale = append(stale, k)
					}
				}
				a.mu.Unlock()
				for _, k := range stale {
					a.dropSession(k)
					a.logf("socks5: UDP session recycled after %s idle", idle)
				}
			}
		}
	}()
}

func (a *udpAssociation) closeAll() {
	a.mu.Lock()
	a.closed = true
	list := make([]*udpSession, 0, len(a.sessions))
	for _, s := range a.sessions {
		list = append(list, s)
	}
	a.sessions = map[string]*udpSession{}
	a.order = nil
	a.mu.Unlock()
	for _, s := range list {
		_ = s.upstream.Close()
	}
}

// parseUDPDatagram decodes RSV|FRAG|ATYP|DST.ADDR|DST.PORT|DATA.
func parseUDPDatagram(b []byte) (frag byte, dst string, payload []byte, err error) {
	if len(b) < 4 {
		return 0, "", nil, errors.New("short datagram")
	}
	if b[0] != 0 || b[1] != 0 {
		return 0, "", nil, errors.New("RSV is not zero")
	}
	frag = b[2]
	atyp := b[3]
	p := 4
	var host string
	switch atyp {
	case atypIPv4:
		if len(b) < p+4 {
			return 0, "", nil, errors.New("short IPv4 address")
		}
		host = net.IP(b[p : p+4]).String()
		p += 4
	case atypIPv6:
		if len(b) < p+16 {
			return 0, "", nil, errors.New("short IPv6 address")
		}
		host = net.IP(b[p : p+16]).String()
		p += 16
	case atypDomain:
		if len(b) < p+1 {
			return 0, "", nil, errors.New("short domain length")
		}
		l := int(b[p])
		p++
		if l == 0 || len(b) < p+l {
			return 0, "", nil, errors.New("bad domain")
		}
		host = string(b[p : p+l])
		p += l
	default:
		return 0, "", nil, fmt.Errorf("unsupported ATYP %#x", atyp)
	}
	if len(b) < p+2 {
		return 0, "", nil, errors.New("short port")
	}
	port := int(binary.BigEndian.Uint16(b[p : p+2]))
	p += 2
	return frag, net.JoinHostPort(host, strconv.Itoa(port)), b[p:], nil
}

// encodeUDPDatagram wraps a reply payload for the client.
func encodeUDPDatagram(remote netip.AddrPort, payload []byte) []byte {
	addr := remote.Addr().Unmap()
	out := make([]byte, 0, 4+16+2+len(payload))
	out = append(out, 0, 0, 0)
	if addr.Is4() {
		out = append(out, atypIPv4)
		v4 := addr.As4()
		out = append(out, v4[:]...)
	} else {
		out = append(out, atypIPv6)
		v16 := addr.As16()
		out = append(out, v16[:]...)
	}
	out = binary.BigEndian.AppendUint16(out, remote.Port())
	out = append(out, payload...)
	return out
}

func addrPortOf(a net.Addr) netip.AddrPort {
	if ap, err := netip.ParseAddrPort(a.String()); err == nil {
		return ap
	}
	if ua, ok := a.(*net.UDPAddr); ok {
		ip, _ := netip.AddrFromSlice(ua.IP)
		return netip.AddrPortFrom(ip, uint16(ua.Port))
	}
	return netip.AddrPort{}
}
