// SPDX-License-Identifier: GPL-3.0-or-later
// Package socks5 implements the SOCKS5 server the node exposes to mihomo /
// FlClash.
//
// P2 scope: TCP CONNECT (RFC 1928 CMD=0x01) and UDP ASSOCIATE (CMD=0x03, see
// udp.go). No authentication; the listener is loopback-only.
//
// The listener is loopback-only by construction: the node must never become a
// public proxy.
package socks5

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	version5 = 5

	methodNoAuth       = 0x00
	methodNoAcceptable = 0xff

	cmdConnect      = 0x01
	cmdBind         = 0x02
	cmdUDPAssociate = 0x03

	atypIPv4   = 0x01
	atypDomain = 0x03
	atypIPv6   = 0x04

	repSuccess            = 0x00
	repGeneralFailure     = 0x01
	repNotAllowed         = 0x02
	repHostUnreachable    = 0x04
	repConnectionRefused  = 0x05
	repCommandUnsupported = 0x07
	repAddrUnsupported    = 0x08
)

// DialFunc opens a connection to host:port. The sidecar passes a dialer that
// goes through the WireGuard tunnel.
type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// Server is a SOCKS5 server.
type Server struct {
	Addr string
	// Listener, when non-nil, is used instead of listening on Addr (useful
	// for tests and for handing in an already bound socket).
	Listener net.Listener
	Dial     DialFunc
	Logf     func(format string, args ...any)
	// DialTimeout bounds how long a single CONNECT may take.
	DialTimeout time.Duration
	// MaxConns caps concurrent client connections (PROMPT.md 5.3: 512).
	// Zero means 512.
	MaxConns int
	// DialUDP opens UDP sockets through the tunnel for UDP ASSOCIATE.
	// Nil disables UDP (the command is then answered with 0x07).
	DialUDP UDPDialFunc
	// UDPIdleTimeout recycles an idle (association, dst) socket (default 120s).
	UDPIdleTimeout time.Duration
	// UDPMaxSessions caps the dst -> socket map per association (default 64).
	UDPMaxSessions int

	// Failure log aggregation: the same failure class is printed once per second
	// and the next line carries the number folded into it.
	failMu         sync.Mutex
	lastFailKey    string
	lastFailAt     time.Time
	failSuppressed int

	lastQuietKey    string
	lastQuietAt     time.Time
	quietSuppressed int

	limiter chan struct{}
}

// loopbackAddr validates that addr is a loopback listener. The node must not
// be reachable from the LAN (PROMPT.md 5.3: listen on 127.0.0.1 only).
func loopbackAddr(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("socks5: invalid listen address %q: %w", addr, err)
	}
	if host == "" {
		return "", fmt.Errorf("socks5: refusing to listen on all interfaces (%q); use 127.0.0.1", addr)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", fmt.Errorf("socks5: refusing to listen on %q; only loopback addresses are allowed", addr)
	}
	return net.JoinHostPort(host, port), nil
}

// ListenAndServe blocks until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	if s.Dial == nil {
		return errors.New("socks5: no dial function")
	}
	if s.Logf == nil {
		s.Logf = func(string, ...any) {}
	}
	if s.DialTimeout == 0 {
		s.DialTimeout = 20 * time.Second
	}
	if s.MaxConns == 0 {
		s.MaxConns = 512
	}
	ln := s.Listener
	if ln == nil {
		addr, err := loopbackAddr(s.Addr)
		if err != nil {
			return err
		}
		ln, err = net.Listen("tcp", addr)
		if err != nil {
			return err
		}
	}
	defer ln.Close()
	s.limiter = make(chan struct{}, s.MaxConns)
	s.Logf("socks5: listening on %s", ln.Addr())

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				continue
			}
			return err
		}
		select {
		case s.limiter <- struct{}{}:
		case <-ctx.Done():
			_ = conn.Close()
			continue
		}
		go func() {
			defer func() { <-s.limiter }()
			s.handle(ctx, conn)
		}()
	}
}

func (s *Server) handle(ctx context.Context, c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(30 * time.Second))

	if err := s.handshake(c); err != nil {
		s.logQuietFailure("handshake", err)
		return
	}

	target, cmd, err := readRequest(c)
	if err != nil {
		s.logQuietFailure("bad request", err)
		return
	}
	switch cmd {
	case cmdConnect:
		// handled below
	case cmdUDPAssociate:
		s.handleUDPAssociate(ctx, c)
		return
	default:
		_ = writeReply(c, repCommandUnsupported, nil)
		s.Logf("socks5: unsupported command %#x", cmd)
		return
	}

	dialCtx, cancel := context.WithTimeout(ctx, s.DialTimeout)
	defer cancel()
	upstream, err := s.Dial(dialCtx, "tcp", target)
	if err != nil {
		// The destination is user data: log the SOCKS reply code and the failure
		// class, never the host (PROMPT.md 0.5 / 2: no destination logging by
		// default). The class is what tells an IPv6 target (no route) apart from a
		// DNS failure, which look identical as "rep 0x4".
		rep := replyForError(err)
		s.logConnectFailure(rep, target, err)
		_ = writeReply(c, rep, nil)
		return
	}
	defer upstream.Close()

	if err := writeReply(c, repSuccess, nil); err != nil {
		return
	}
	_ = c.SetDeadline(time.Time{})
	s.Logf("socks5: CONNECT tunnel established")
	relay(c, upstream)
}

func (s *Server) handshake(c net.Conn) error {
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(c, hdr); err != nil {
		return err
	}
	if hdr[0] != version5 {
		return fmt.Errorf("unsupported socks version %d", hdr[0])
	}
	n := int(hdr[1])
	if n == 0 {
		return errors.New("no auth methods offered")
	}
	methods := make([]byte, n)
	if _, err := io.ReadFull(c, methods); err != nil {
		return err
	}
	for _, m := range methods {
		if m == methodNoAuth {
			_, err := c.Write([]byte{version5, methodNoAuth})
			return err
		}
	}
	_, _ = c.Write([]byte{version5, methodNoAcceptable})
	return errors.New("client offers no acceptable auth method")
}

func readRequest(c net.Conn) (target string, cmd byte, err error) {
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(c, hdr); err != nil {
		return "", 0, err
	}
	if hdr[0] != version5 {
		return "", 0, fmt.Errorf("bad version %d", hdr[0])
	}
	cmd = hdr[1]

	var host string
	switch hdr[3] {
	case atypIPv4:
		b := make([]byte, 4)
		if _, err := io.ReadFull(c, b); err != nil {
			return "", 0, err
		}
		host = net.IP(b).String()
	case atypIPv6:
		b := make([]byte, 16)
		if _, err := io.ReadFull(c, b); err != nil {
			return "", 0, err
		}
		host = net.IP(b).String()
	case atypDomain:
		l := make([]byte, 1)
		if _, err := io.ReadFull(c, l); err != nil {
			return "", 0, err
		}
		b := make([]byte, int(l[0]))
		if _, err := io.ReadFull(c, b); err != nil {
			return "", 0, err
		}
		host = string(b)
	default:
		return "", 0, fmt.Errorf("unsupported address type %#x", hdr[3])
	}

	pb := make([]byte, 2)
	if _, err := io.ReadFull(c, pb); err != nil {
		return "", 0, err
	}
	port := binary.BigEndian.Uint16(pb)
	return net.JoinHostPort(host, strconv.Itoa(int(port))), cmd, nil
}

func writeReply(c net.Conn, rep byte, bound net.Addr) error {
	buf := []byte{version5, rep, 0x00}
	var ip net.IP
	var port int
	switch a := bound.(type) {
	case *net.TCPAddr:
		if a != nil {
			ip, port = a.IP, a.Port
		}
	case *net.UDPAddr:
		if a != nil {
			ip, port = a.IP, a.Port
		}
	}
	if ip4 := ip.To4(); ip4 != nil {
		buf = append(buf, atypIPv4)
		buf = append(buf, ip4...)
	} else if ip16 := ip.To16(); ip16 != nil {
		buf = append(buf, atypIPv6)
		buf = append(buf, ip16...)
	} else {
		buf = append(buf, atypIPv4, 0, 0, 0, 0)
	}
	buf = binary.BigEndian.AppendUint16(buf, uint16(port))
	_, err := c.Write(buf)
	return err
}

func replyForError(err error) byte {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return repHostUnreachable
	}
	msg := err.Error()
	switch {
	case containsAny(msg, "refused"):
		return repConnectionRefused
	case containsAny(msg, "no such host", "no route to host", "network is unreachable"):
		return repHostUnreachable
	default:
		return repGeneralFailure
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) <= len(s) && indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// relay copies bytes in both directions until either side closes.
func relay(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	cp := func(dst, src net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		} else {
			_ = dst.Close()
		}
	}
	go cp(a, b)
	go cp(b, a)
	wg.Wait()
}

// addressFamily reports the target's family without logging the address itself.
func addressFamily(target string) string {
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		return "family=?"
	}
	host = strings.Trim(host, "[]")
	if strings.Contains(host, ":") {
		return "family=ipv6"
	}
	if ip := net.ParseIP(host); ip == nil {
		return "family=name"
	}
	return "family=ipv4"
}

// errorClass reduces a dial error to a short, non-identifying class.
func errorClass(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "network is unreachable"), strings.Contains(msg, "no route"):
		return "unreachable"
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "deadline"):
		return "timeout"
	case strings.Contains(msg, "refused"):
		return "refused"
	case strings.Contains(msg, "no such host"), strings.Contains(msg, "not found"), strings.Contains(msg, "lookup"):
		return "dns"
	case strings.Contains(msg, "reset"):
		return "reset"
	default:
		return "error"
	}
}

// logConnectFailure keeps the per-connection failure line useful without letting
// a client that opens hundreds of doomed connections flood the ring log: the
// same (reply, family, class) triple is printed at most once per second, and the
// next line reports how many were folded into it.
func (s *Server) logConnectFailure(rep byte, target string, err error) {
	key := fmt.Sprintf("%#x/%s/%s", rep, addressFamily(target), errorClass(err))
	s.failMu.Lock()
	now := time.Now()
	if key == s.lastFailKey && now.Sub(s.lastFailAt) < time.Second {
		s.failSuppressed++
		s.failMu.Unlock()
		return
	}
	suppressed := s.failSuppressed
	s.lastFailKey = key
	s.lastFailAt = now
	s.failSuppressed = 0
	s.failMu.Unlock()
	if suppressed > 0 {
		s.Logf("socks5: CONNECT failed (rep %#x, %s, %s) x%d", rep, addressFamily(target), errorClass(err), suppressed+1)
		return
	}
	s.Logf("socks5: CONNECT failed (rep %#x, %s, %s)", rep, addressFamily(target), errorClass(err))
}

// logQuietFailure aggregates client-side protocol noise (health probes that
// connect and close, half-open reads): mihomo opens dozens of these per minute,
// and a single line per second per kind is enough to keep the ring log readable.
func (s *Server) logQuietFailure(kind string, err error) {
	key := kind + "/" + errorClass(err)
	s.failMu.Lock()
	now := time.Now()
	if key == s.lastQuietKey && now.Sub(s.lastQuietAt) < time.Second {
		s.quietSuppressed++
		s.failMu.Unlock()
		return
	}
	suppressed := s.quietSuppressed
	s.lastQuietKey = key
	s.lastQuietAt = now
	s.quietSuppressed = 0
	s.failMu.Unlock()
	if suppressed > 0 {
		s.Logf("socks5: %s (%s) x%d", kind, errorClass(err), suppressed+1)
		return
	}
	s.Logf("socks5: %s (%s)", kind, errorClass(err))
}
