// SPDX-License-Identifier: GPL-3.0-or-later
package tunnel_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/device"
	tnetstack "golang.zx2c4.com/wireguard/tun/netstack"

	"github.com/leggerezza/android-proton/core/socks5"
	"github.com/leggerezza/android-proton/core/stealth"
	"github.com/leggerezza/android-proton/core/tunnel"
)

// TestStealthTunnelEndToEnd runs the complete sidecar stack against a local
// counterpart:
//
//	TLS (same framing as Proton's Stealth) -> WireGuard handshake ->
//	transport data -> gVisor netstack -> SOCKS5 -> HTTP
//
// It verifies the parts we own. The only thing it cannot verify is Proton's
// own server and API behaviour (that needs a Proton account, see verify.ps1).
func TestStealthTunnelEndToEnd(t *testing.T) {
	const (
		serverVPN = "10.7.0.1"
		clientVPN = "10.7.0.2"
		httpAddr  = "10.7.0.1:8080"
		bodyLen   = 6000 // several WireGuard data packets, so framing matters
	)

	serverKP, err := stealth.NewKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientKP, err := stealth.NewKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	// --- server: TLS listener + WireGuard device + HTTP served over netstack
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	bind := stealth.NewListenBind(ln, selfSignedCert(t))
	defer bind.Shutdown()

	tunDev, tnet, err := tnetstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr(serverVPN)}, nil, tunnel.DefaultMTU)
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
		hexOf(serverKP.X25519PrivateKey()),
		hexOf(clientXPub),
		clientVPN, clientVPN,
	)); err != nil {
		t.Fatal(err)
	}
	if err := srvDev.Up(); err != nil {
		t.Fatal(err)
	}

	payload := strings.Repeat("A", bodyLen)
	httpLn, err := tnet.ListenTCPAddrPort(netip.MustParseAddrPort(httpAddr))
	if err != nil {
		t.Fatal(err)
	}
	httpSrv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = io.WriteString(w, payload)
	})}
	go func() {
		if err := httpSrv.Serve(httpLn); err != nil && err != http.ErrServerClosed {
			t.Logf("http serve ended: %v", err)
		}
	}()
	defer httpSrv.Close()

	// --- client: Stealth transport + userspace WireGuard
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

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	start := time.Now()
	if err := client.Start(ctx, 20*time.Second); err != nil {
		t.Fatalf("tunnel did not come up: %v", err)
	}
	t.Logf("handshake ok in %s (SNI %s)", time.Since(start).Round(time.Millisecond), client.ServerName())

	// --- SOCKS5 in front of the tunnel
	sln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &socks5.Server{Listener: sln, Dial: client.DialContext}
	go func() { _ = srv.ListenAndServe(ctx) }()

	body := socksHTTPGet(t, sln.Addr().String(), httpAddr, "/")
	if !strings.HasPrefix(body, "HTTP/1.1 200") {
		t.Fatalf("unexpected response head: %q", firstN(body, 120))
	}
	idx := strings.Index(body, "\r\n\r\n")
	if idx < 0 {
		t.Fatalf("no header/body separator in response")
	}
	got := body[idx+4:]
	if len(got) != bodyLen {
		t.Fatalf("payload truncated through the tunnel: got %d bytes, want %d", len(got), bodyLen)
	}
	if got != payload {
		t.Fatalf("payload corrupted through the tunnel")
	}
	if rx, tx, _, _ := client.Stats(); rx == 0 || tx == 0 {
		t.Fatalf("wireguard counters look wrong: rx=%d tx=%d", rx, tx)
	}
}

func hexOf(b []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, hexdigits[c>>4], hexdigits[c&0x0f])
	}
	return string(out)
}

func firstN(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// socksHTTPGet performs a SOCKS5 CONNECT and a plain HTTP/1.1 GET through it.
func socksHTTPGet(t *testing.T, socksAddr, target, path string) string {
	t.Helper()
	c, err := net.DialTimeout("tcp", socksAddr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial socks: %v", err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(30 * time.Second))

	if _, err := c.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	greet := make([]byte, 2)
	if _, err := io.ReadFull(c, greet); err != nil {
		t.Fatal(err)
	}
	if greet[0] != 5 || greet[1] != 0 {
		t.Fatalf("bad greeting reply %v", greet)
	}

	host, portStr, _ := net.SplitHostPort(target)
	ip := net.ParseIP(host).To4()
	if ip == nil {
		t.Fatalf("test target must be an IPv4 literal")
	}
	var port uint16
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		t.Fatal(err)
	}
	req := append([]byte{5, 1, 0, 1}, ip...)
	req = binary.BigEndian.AppendUint16(req, port)
	if _, err := c.Write(req); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(c, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] != 0 {
		t.Fatalf("SOCKS5 CONNECT failed with reply code %#x", reply[1])
	}

	if _, err := fmt.Fprintf(c, "GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", path, host); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(c)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return string(out)
}

// selfSignedCert builds a throwaway certificate for the local TLS server.
func selfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "proton-stealth-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}
