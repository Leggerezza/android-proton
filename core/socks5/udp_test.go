// SPDX-License-Identifier: GPL-3.0-or-later
package socks5

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// echoUDP starts a local UDP echo server.
func echoUDP(t *testing.T) *net.UDPConn {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	go func() {
		buf := make([]byte, 65535)
		for {
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = conn.WriteToUDP(buf[:n], from)
		}
	}()
	return conn
}

func udpDial(ctx context.Context, address string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "udp", address)
}

// startSocksWithUDP starts a SOCKS5 server that supports UDP ASSOCIATE.
func startSocksWithUDP(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	srv := &Server{Listener: ln, Dial: dialer, DialUDP: udpDial}
	go func() { _ = srv.ListenAndServe(ctx) }()
	return ln.Addr().String(), func() { cancel(); _ = ln.Close() }
}

// udpAssociate performs the TCP control handshake and returns the relay address.
func udpAssociate(t *testing.T, control net.Conn) *net.UDPAddr {
	t.Helper()
	_ = control.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := control.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	greet := make([]byte, 2)
	if _, err := io.ReadFull(control, greet); err != nil || greet[1] != 0 {
		t.Fatalf("greeting %v, %v", greet, err)
	}
	// CMD=0x03, ATYP=1, 0.0.0.0:0 (the client does not know its port yet).
	if _, err := control.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	head := make([]byte, 10)
	if _, err := io.ReadFull(control, head); err != nil {
		t.Fatal(err)
	}
	if head[1] != 0 {
		t.Fatalf("UDP ASSOCIATE rejected with %#x", head[1])
	}
	return &net.UDPAddr{IP: net.IP(head[4:8]), Port: int(binary.BigEndian.Uint16(head[8:10]))}
}

func TestParseEncodeUDPDatagram(t *testing.T) {
	cases := []struct {
		name string
		pkt  []byte
		dst  string
		body string
	}{
		{"ipv4", append([]byte{0, 0, 0, atypIPv4, 192, 0, 2, 10, 0x1f, 0x90}, []byte("a")...), "192.0.2.10:8080", "a"},
		{"domain", append([]byte{0, 0, 0, atypDomain, 11}, append([]byte("example.com"), 0x00, 0x35)...), "example.com:53", ""},
		{"ipv6", append([]byte{0, 0, 0, atypIPv6}, append(append(make([]byte, 15), 1), 0x00, 0x50)...), "[::1]:80", ""},
	}
	for _, c := range cases {
		frag, dst, payload, err := parseUDPDatagram(c.pkt)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if frag != 0 || dst != c.dst || string(payload) != c.body {
			t.Fatalf("%s: frag=%d dst=%q payload=%q", c.name, frag, dst, payload)
		}
	}
	if _, _, _, err := parseUDPDatagram([]byte{0, 1, 0}); err == nil {
		t.Fatal("short datagram accepted")
	}
	if _, _, _, err := parseUDPDatagram([]byte{1, 0, 0, atypIPv4, 1, 2, 3, 4, 0, 1}); err == nil {
		t.Fatal("non-zero RSV accepted")
	}
}

func TestUDPAssociateEcho(t *testing.T) {
	echo := echoUDP(t)
	addr, stop := startSocksWithUDP(t)
	defer stop()

	control, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	relay := udpAssociate(t, control)

	client, err := net.DialUDP("udp", nil, relay)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))

	payload := []byte("udp through the tunnel")
	pkt := append([]byte{0, 0, 0}, encAddrPort(echo.LocalAddr().(*net.UDPAddr))...)
	pkt = append(pkt, payload...)
	if _, err := client.Write(pkt); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 2048)
	n, err := client.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	_, dst, body, err := parseUDPDatagram(buf[:n])
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != string(payload) {
		t.Fatalf("echo mismatch: %q (from %s)", body, dst)
	}
}

func TestUDPAssociateDropsFragment(t *testing.T) {
	echo := echoUDP(t)
	addr, stop := startSocksWithUDP(t)
	defer stop()

	control, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	relay := udpAssociate(t, control)
	client, err := net.DialUDP("udp", nil, relay)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	pkt := append([]byte{0, 0, 1}, encAddrPort(echo.LocalAddr().(*net.UDPAddr))...)
	pkt = append(pkt, []byte("fragmented")...)
	if _, err := client.Write(pkt); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
	buf := make([]byte, 256)
	if n, err := client.Read(buf); err == nil {
		t.Fatalf("a fragmented datagram was forwarded (%d bytes)", n)
	}
}

func TestUDPAssociateRejectedWithoutDialer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &Server{Listener: ln, Dial: dialer}
	go func() { _ = srv.ListenAndServe(ctx) }()
	defer ln.Close()

	control, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	_ = control.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := control.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(control, make([]byte, 2)); err != nil {
		t.Fatal(err)
	}
	if _, err := control.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	head := make([]byte, 10)
	if _, err := io.ReadFull(control, head); err != nil {
		t.Fatal(err)
	}
	if head[1] != repCommandUnsupported {
		t.Fatalf("expected 0x07, got %#x", head[1])
	}
}

// TestAssociationLRUAndIdle exercises the per-association session map directly.
func TestAssociationLRUAndIdle(t *testing.T) {
	echo := echoUDP(t)
	relay, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()
	srv := &Server{DialUDP: udpDial, UDPMaxSessions: 2, UDPIdleTimeout: 150 * time.Millisecond}
	assoc := newUDPAssociation(srv, relay)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assoc.reapLoop(ctx)

	target := echo.LocalAddr().String()
	for _, dst := range []string{target, "127.0.0.1:9", "127.0.0.1:10"} {
		if _, err := assoc.session(ctx, dst); err != nil {
			t.Fatalf("session(%s): %v", dst, err)
		}
	}
	assoc.mu.Lock()
	n := len(assoc.sessions)
	_, kept1 := assoc.sessions["127.0.0.1:9"]
	_, kept2 := assoc.sessions["127.0.0.1:10"]
	assoc.mu.Unlock()
	if n != 2 || !kept1 || !kept2 {
		t.Fatalf("LRU did not evict the oldest: n=%d kept9=%v kept10=%v", n, kept1, kept2)
	}

	time.Sleep(400 * time.Millisecond)
	assoc.mu.Lock()
	n = len(assoc.sessions)
	assoc.mu.Unlock()
	if n != 0 {
		t.Fatalf("idle sessions were not recycled: %d left", n)
	}
}

// TestUDPAssociateConcurrentTargets is the PROMPT.md 7.4 acceptance item:
// at least 16 different destinations active at the same time through one
// association, each with its own upstream socket.
func TestUDPAssociateConcurrentTargets(t *testing.T) {
	const targets = 16
	echoes := make([]*net.UDPConn, targets)
	for i := range echoes {
		echoes[i] = echoUDP(t)
	}
	addr, stop := startSocksWithUDP(t)
	defer stop()

	control, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	relay := udpAssociate(t, control)

	client, err := net.DialUDP("udp", nil, relay)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(10 * time.Second))

	for i, echo := range echoes {
		pkt := append([]byte{0, 0, 0}, encAddrPort(echo.LocalAddr().(*net.UDPAddr))...)
		pkt = append(pkt, []byte(fmt.Sprintf("target-%02d", i))...)
		if _, err := client.Write(pkt); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	seen := map[string]bool{}
	buf := make([]byte, 2048)
	for len(seen) < targets {
		n, err := client.Read(buf)
		if err != nil {
			t.Fatalf("read after %d replies: %v", len(seen), err)
		}
		_, _, body, err := parseUDPDatagram(buf[:n])
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(body), "target-") {
			t.Fatalf("unexpected reply %q", body)
		}
		seen[string(body)] = true
	}
	if len(seen) != targets {
		t.Fatalf("only %d of %d targets answered", len(seen), targets)
	}
}

// TestUDPAssociateDiesWithControlConnection is the PROMPT.md 5.4.2 rule: the
// association's lifetime is the TCP control connection.
func TestUDPAssociateDiesWithControlConnection(t *testing.T) {
	echo := echoUDP(t)
	addr, stop := startSocksWithUDP(t)
	defer stop()

	control, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	relay := udpAssociate(t, control)

	client, err := net.DialUDP("udp", nil, relay)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))

	pkt := append([]byte{0, 0, 0}, encAddrPort(echo.LocalAddr().(*net.UDPAddr))...)
	pkt = append(pkt, []byte("alive")...)
	if _, err := client.Write(pkt); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 512)
	if _, err := client.Read(buf); err != nil {
		t.Fatalf("first exchange failed: %v", err)
	}

	// Closing the control connection must tear the relay down. Allow up to 2s
	// for the teardown to reach the socket (scheduler dependent on a phone).
	_ = control.Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := client.Write(pkt); err != nil {
			return // already gone, which is the expected outcome
		}
		_ = client.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		if _, err := client.Read(buf); err != nil {
			return // no answer: the relay is closed, as required
		}
		if time.Now().After(deadline) {
			t.Fatal("relay still answered a datagram 2s after the control connection closed")
		}
	}
}

// TestServerClosesListenerOnCancel backs the "Stop() leaves no socket behind"
// acceptance item (PROMPT.md 5.2).
func TestServerClosesListenerOnCancel(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	srv := &Server{Listener: ln, Dial: dialer, DialUDP: udpDial}
	done := make(chan struct{})
	go func() { _ = srv.ListenAndServe(ctx); close(done) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("ListenAndServe did not return after cancel")
	}
	// The port must be free again: no residual listener.
	again, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listener still held after Stop: %v", err)
	}
	_ = again.Close()
}

func encAddrPort(a *net.UDPAddr) []byte {
	ip := a.IP.To4()
	out := []byte{atypIPv4}
	out = append(out, ip...)
	return binary.BigEndian.AppendUint16(out, uint16(a.Port))
}
