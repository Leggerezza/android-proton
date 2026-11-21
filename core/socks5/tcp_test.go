// SPDX-License-Identifier: GPL-3.0-or-later
package socks5

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func TestLoopbackAddrRejectsNonLoopback(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:1080", ":1080", "10.0.0.5:1080", "192.168.1.2:9000"} {
		if _, err := loopbackAddr(addr); err == nil {
			t.Fatalf("loopbackAddr(%q) accepted a non-loopback listener", addr)
		}
	}
	got, err := loopbackAddr("127.0.0.1:1080")
	if err != nil || got != "127.0.0.1:1080" {
		t.Fatalf("loopbackAddr(127.0.0.1:1080) = %q, %v", got, err)
	}
}

// TestConnectRelay exercises the whole SOCKS5 TCP path against a local echo
// server: greeting, CONNECT, the success reply and the bidirectional relay.
func TestConnectRelay(t *testing.T) {
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echoLn.Close()
	go func() {
		for {
			c, err := echoLn.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}()
		}
	}()

	socksLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &Server{Listener: socksLn, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}, MaxConns: 4}
	go func() { _ = srv.ListenAndServe(ctx) }()
	defer socksLn.Close()

	c, err := net.DialTimeout("tcp", socksLn.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))

	if _, err := c.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	greet := make([]byte, 2)
	if _, err := io.ReadFull(c, greet); err != nil || greet[0] != 5 || greet[1] != 0 {
		t.Fatalf("greeting reply %v, %v", greet, err)
	}

	target := echoLn.Addr().(*net.TCPAddr)
	req := []byte{5, 1, 0, 1, 127, 0, 0, 1, byte(target.Port >> 8), byte(target.Port)}
	if _, err := c.Write(req); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(c, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] != 0 {
		t.Fatalf("CONNECT failed with reply %#x", reply[1])
	}

	payload := []byte("hello through socks5")
	if _, err := c.Write(payload); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != string(payload) {
		t.Fatalf("echo mismatch: %q", buf)
	}
}

func TestUDPAssociateIsRejected(t *testing.T) {
	// A UDP ASSOCIATE request must be answered with REP=0x07, never silently
	// accepted: mihomo would otherwise assume UDP works (P1 is TCP-only).
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echoLn.Close()
	socksLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &Server{Listener: socksLn, Dial: dialer}
	go func() { _ = srv.ListenAndServe(ctx) }()
	defer socksLn.Close()

	c, err := net.DialTimeout("tcp", socksLn.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(c, make([]byte, 2)); err != nil {
		t.Fatal(err)
	}
	// CMD=0x03 (UDP ASSOCIATE), ATYP=1, 127.0.0.1:1080
	if _, err := c.Write([]byte{5, 3, 0, 1, 127, 0, 0, 1, 4, 56}); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(c, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] != 0x07 {
		t.Fatalf("UDP ASSOCIATE reply = %#x, want 0x07", reply[1])
	}
}

// dialer adapts net.Dial to the context-aware DialFunc the server takes.
func dialer(ctx context.Context, network, address string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, address)
}
