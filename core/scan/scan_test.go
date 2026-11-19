package scan

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"math/big"
	"net"
	"net/netip"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/device"
	tnetstack "golang.zx2c4.com/wireguard/tun/netstack"

	"github.com/dsh/android-proton/core/stealth"
	"github.com/dsh/android-proton/core/tunnel"
)

func selfSigned(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "scan-test"},
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

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]Level{"": LevelTunnel, "3": LevelTunnel, "tunnel": LevelTunnel, "1": LevelTCP, "tcp": LevelTCP, "2": LevelTLS, "tls": LevelTLS} {
		got, err := ParseLevel(in)
		if err != nil || got != want {
			t.Fatalf("ParseLevel(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseLevel("9"); err == nil {
		t.Fatal("ParseLevel(9) accepted")
	}
}

// TestL1AndL2 checks the first two stages against local listeners.
func TestL1AndL2(t *testing.T) {
	tlsLn, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{selfSigned(t)}})
	if err != nil {
		t.Fatal(err)
	}
	defer tlsLn.Close()
	go func() {
		for {
			c, err := tlsLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				if tc, ok := c.(*tls.Conn); ok {
					_ = tc.Handshake()
				}
				_ = c.Close()
			}(c)
		}
	}()
	port := tlsLn.Addr().(*net.TCPAddr).Port

	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedPort := closed.Addr().(*net.TCPAddr).Port
	_ = closed.Close()

	targets := []Target{
		{Server: "tls-ok", EntryIP: "127.0.0.1", Port: port},
		{Server: "closed", EntryIP: "127.0.0.1", Port: closedPort},
	}
	runner, err := New(targets, Options{Level: LevelTLS, Concurrency: 4, L1Timeout: 2 * time.Second, L2Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	<-runner.Start(ctx)
	if runner.State() != StateDone {
		t.Fatalf("state = %s (%s)", runner.State(), runner.ProgressJSON())
	}
	res := runner.Results()
	if len(res) != 1 || res[0].Server != "tls-ok" || res[0].Level != int(LevelTLS) {
		t.Fatalf("results = %+v", res)
	}
	if res[0].SNI == "" {
		t.Fatal("L2 result has no SNI")
	}
}

// TestL3AgainstLocalTunnel runs the full stage against a local Stealth server:
// only a completed WireGuard handshake may pass.
func TestL3AgainstLocalTunnel(t *testing.T) {
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
	bind := stealth.NewListenBind(ln, selfSigned(t))
	defer bind.Shutdown()

	tunDev, _, err := tnetstack.CreateNetTUN([]netip.Addr{netip.MustParseAddr("10.9.0.1")}, nil, tunnel.DefaultMTU)
	if err != nil {
		t.Fatal(err)
	}
	dev := device.NewDevice(tunDev, bind, device.NewLogger(device.LogLevelError, "srv: "))
	defer dev.Close()
	clientPub, err := clientKP.X25519PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := dev.IpcSet(fmt.Sprintf(
		"private_key=%s\npublic_key=%s\nendpoint=10.9.0.2:51820\nallowed_ip=10.9.0.2/32\n",
		hexOf(serverKP.X25519PrivateKey()), hexOf(clientPub),
	)); err != nil {
		t.Fatal(err)
	}
	if err := dev.Up(); err != nil {
		t.Fatal(err)
	}
	serverPub, err := serverKP.X25519PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	runner, err := New([]Target{{
		Server: "local", EntryIP: "127.0.0.1", Port: port,
		ServerKey: base64.StdEncoding.EncodeToString(serverPub),
	}}, Options{
		Level: LevelTunnel, Concurrency: 2,
		L1Timeout: 2 * time.Second, L2Timeout: 3 * time.Second, L3Timeout: 10 * time.Second,
		PrivateKey: clientKP.X25519PrivateKey(),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	<-runner.Start(ctx)
	res := runner.Results()
	if runner.State() != StateDone || len(res) != 1 {
		t.Fatalf("state=%s results=%+v progress=%s", runner.State(), res, runner.ProgressJSON())
	}
	if res[0].Level != int(LevelTunnel) || res[0].ServerKey == "" {
		t.Fatalf("unexpected L3 result: %+v", res[0])
	}
}

// TestCancelWithinOneSecond is the PROMPT.md 11 acceptance item for scans.
func TestCancelWithinOneSecond(t *testing.T) {
	targets := make([]Target, 0, 8)
	for i := 0; i < 8; i++ {
		targets = append(targets, Target{Server: "blackhole", EntryIP: "10.255.255.1", Port: 65000 + i})
	}
	runner, err := New(targets, Options{Level: LevelTunnel, Concurrency: 8, L1Timeout: 10 * time.Second, PrivateKey: make([]byte, 32)})
	if err != nil {
		t.Fatal(err)
	}
	runner.Start(context.Background())
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	if !runner.Cancel() {
		t.Fatal("Cancel returned false on a running scan")
	}
	runner.Wait(context.Background())
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("cancel took %s, want < 1s", elapsed)
	}
	if state := runner.State(); state != StateCancelled {
		t.Fatalf("state = %s, want cancelled", state)
	}
}

func TestCacheValidity(t *testing.T) {
	now := time.Now()
	c := &Cache{Fingerprint: "wifi-A", Level: 3, At: now.Add(-5 * time.Minute), Results: []Result{{Server: "x"}}}
	if !c.Valid("wifi-A", LevelTunnel, now) {
		t.Fatal("fresh cache for the same network rejected")
	}
	if c.Valid("wifi-B", LevelTunnel, now) {
		t.Fatal("cache reused on another network")
	}
	if c.Valid("wifi-A", LevelTunnel, now.Add(CacheTTL+time.Minute)) {
		t.Fatal("expired cache accepted")
	}
	shallow := &Cache{Fingerprint: "wifi-A", Level: 1, At: now, Results: []Result{{Server: "x"}}}
	if shallow.Valid("wifi-A", LevelTunnel, now) {
		t.Fatal("L1 results accepted for an L3 selection")
	}
}

func hexOf(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, digits[c>>4], digits[c&0x0f])
	}
	return string(out)
}
