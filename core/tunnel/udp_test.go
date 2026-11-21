// SPDX-License-Identifier: GPL-3.0-or-later
package tunnel_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/device"
	tnetstack "golang.zx2c4.com/wireguard/tun/netstack"

	"github.com/leggerezza/android-proton/core/stealth"
	"github.com/leggerezza/android-proton/core/tunnel"
)

// TestUDPThroughTunnel proves the part the Windows baseline never did
// (PROMPT.md 4: "netstack UDP - unverified"): a UDP datagram survives the
// TunSafe framing, the WireGuard data path and the gVisor netstack, in both
// directions.
func TestUDPThroughTunnel(t *testing.T) {
	const (
		serverVPN = "10.7.0.1"
		clientVPN = "10.7.0.2"
		echoAddr  = "10.7.0.1:9091"
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

	// UDP echo server inside the server-side netstack.
	echoConn, err := tnet.ListenUDPAddrPort(netip.MustParseAddrPort(echoAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer echoConn.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := echoConn.ReadFrom(buf)
			if err != nil {
				return
			}
			if _, err := echoConn.WriteTo(buf[:n], from); err != nil {
				return
			}
		}
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

	conn, err := client.DialUDP(ctx, echoAddr)
	if err != nil {
		t.Fatalf("dial UDP inside the tunnel: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	for i, payload := range [][]byte{[]byte("ping"), []byte("a longer datagram through the wireguard data path")} {
		if _, err := conn.Write(payload); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		buf := make([]byte, 2048)
		n, err := conn.Read(buf)
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if string(buf[:n]) != string(payload) {
			t.Fatalf("echo %d mismatch: %q", i, buf[:n])
		}
	}

	// The tunnel must survive a burst: the framing compresses consecutive data
	// packets, and a bug there shows up as reordered or lost datagrams.
	for i := 0; i < 64; i++ {
		payload := []byte(fmt.Sprintf("burst-%02d", i))
		if _, err := conn.Write(payload); err != nil {
			t.Fatalf("burst write %d: %v", i, err)
		}
		buf := make([]byte, 2048)
		n, err := conn.Read(buf)
		if err != nil {
			t.Fatalf("burst read %d: %v", i, err)
		}
		if string(buf[:n]) != string(payload) {
			t.Fatalf("burst %d mismatch: %q", i, buf[:n])
		}
	}
}
