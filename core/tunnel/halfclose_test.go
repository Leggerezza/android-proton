// SPDX-License-Identifier: GPL-3.0-or-later
package tunnel_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/device"
	tnetstack "golang.zx2c4.com/wireguard/tun/netstack"

	"github.com/leggerezza/android-proton/core/stealth"
	"github.com/leggerezza/android-proton/core/tunnel"
)

// TestTCPHalfCloseThroughTunnel covers PROMPT.md 5.3: a SOCKS5 TCP relay needs
// CloseWrite() semantics, and the connection it relays is the netstack one.
// The server answers only after it has seen EOF from the client.
func TestTCPHalfCloseThroughTunnel(t *testing.T) {
	const (
		serverVPN = "10.7.0.1"
		clientVPN = "10.7.0.2"
	)

	serverKP, err := stealth.NewKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientKP, err := stealth.NewKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	bind := stealth.NewListenBind(ln, selfSignedCert(t))
	defer bind.Shutdown()

	tunDev, tnet, err := tnetstack.CreateNetTUN([]netip.Addr{netip.MustParseAddr(serverVPN)}, nil, tunnel.DefaultMTU)
	if err != nil {
		t.Fatal(err)
	}
	srvDev := device.NewDevice(tunDev, bind, device.NewLogger(device.LogLevelError, "srv: "))
	defer srvDev.Close()

	clientXPub, err := clientKP.X25519PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := srvDev.IpcSet(fmt.Sprintf(
		"private_key=%s\npublic_key=%s\nendpoint=%s:51820\nallowed_ip=%s/32\n",
		hexOf(serverKP.X25519PrivateKey()), hexOf(clientXPub), clientVPN, clientVPN,
	)); err != nil {
		t.Fatal(err)
	}
	if err := srvDev.Up(); err != nil {
		t.Fatal(err)
	}

	tcpLn, err := tnet.ListenTCPAddrPort(netip.MustParseAddrPort("10.7.0.1:9092"))
	if err != nil {
		t.Fatal(err)
	}
	defer tcpLn.Close()
	go func() {
		conn, err := tcpLn.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		body, err := io.ReadAll(conn) // returns on the client's half-close
		if err != nil {
			return
		}
		_, _ = conn.Write([]byte("received:" + string(body)))
	}()

	serverXPub, err := serverKP.X25519PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	client, err := tunnel.New(tunnel.Config{
		ServerIP:   "127.0.0.1",
		Port:       port,
		ServerKey:  base64.StdEncoding.EncodeToString(serverXPub),
		PrivateKey: clientKP.X25519PrivateKey(),
		Address:    clientVPN,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.Start(ctx, 20*time.Second); err != nil {
		t.Fatalf("tunnel did not come up: %v", err)
	}

	conn, err := client.DialContext(ctx, "tcp", "10.7.0.1:9092")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	cw, ok := conn.(interface{ CloseWrite() error })
	if !ok {
		t.Fatalf("the netstack TCP connection does not implement CloseWrite; the SOCKS5 relay would have to fall back to Close")
	}
	if err := cw.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite: %v", err)
	}
	reply, err := io.ReadAll(conn)
	if err != nil {
		t.Fatal(err)
	}
	if string(reply) != "received:hello" {
		t.Fatalf("half-close reply = %q", reply)
	}
}
