// SPDX-License-Identifier: GPL-3.0-or-later
// Package tunnel wires the Stealth bind into a userspace WireGuard device
// backed by a gVisor netstack. Nothing is installed on the host: no TUN
// adapter, no routes, no admin rights.
package tunnel

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	tnetstack "golang.zx2c4.com/wireguard/tun/netstack"

	"github.com/leggerezza/android-proton/core/stealth"
)

// Defaults match Proton's own client: the tunnel address is 10.2.0.2 and the
// in-tunnel DNS resolver is 10.2.0.1.
const (
	DefaultAddress = "10.2.0.2"
	DefaultDNS     = "10.2.0.1"
	DefaultMTU     = 1320

	// Proton's fixed IPv6 client address and resolver, the same pair its own
	// Android client uses (Constants.VPN_CLIENT_IP_V6 / VPN_SERVER_IP_V6). The
	// server routes that /128 back into the tunnel; nothing is negotiated.
	DefaultIPv6 = "2a07:b944::2:2"
	DefaultDNS6 = "2a07:b944::2:1"
	// TransportStealth is WireGuard framed inside TLS on TCP; TransportUDP is
	// plain WireGuard on UDP (much faster where the network allows it).
	TransportStealth = "stealth"
	TransportUDP     = "udp"
)

// Config describes one tunnel.
type Config struct {
	ServerIP   string
	Port       int
	ServerKey  string // base64 X25519 public key of the entry server
	PrivateKey []byte // 32-byte X25519 private key
	Address    string
	DNS        string
	IPv6       string // client IPv6 address, used when EnableIPv6
	DNS6       string
	EnableIPv6 bool
	MTU        int
	SNI        string
	Hello      utls.ClientHelloID
	Transport  string // "stealth" (default) or "udp"
	Logf       func(format string, args ...any)
}

// Tunnel is a running userspace WireGuard-over-TLS tunnel.
type Tunnel struct {
	cfg  Config
	dev  *device.Device
	net  *tnetstack.Net
	bind conn.Bind

	dnsMu    sync.Mutex
	dnsCache map[string]dnsEntry
	sniUsed  string
}

// New creates the device and configures the single peer (0.0.0.0/0).
func New(cfg Config) (*Tunnel, error) {
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.Address == "" {
		cfg.Address = DefaultAddress
	}
	if cfg.DNS == "" {
		cfg.DNS = DefaultDNS
	}
	if cfg.MTU == 0 {
		cfg.MTU = DefaultMTU
	}
	if cfg.Port == 0 {
		if cfg.Transport == TransportUDP {
			cfg.Port = 51820
		} else {
			cfg.Port = 443
		}
	}
	if cfg.Transport == "" {
		cfg.Transport = TransportStealth
	}
	if cfg.Transport != TransportStealth && cfg.Transport != TransportUDP {
		return nil, fmt.Errorf("unknown transport %q (stealth|udp)", cfg.Transport)
	}
	if cfg.IPv6 == "" {
		cfg.IPv6 = DefaultIPv6
	}
	if cfg.DNS6 == "" {
		cfg.DNS6 = DefaultDNS6
	}

	serverKey, err := base64.StdEncoding.DecodeString(cfg.ServerKey)
	if err != nil || len(serverKey) != 32 {
		return nil, fmt.Errorf("server public key is not a 32-byte base64 value")
	}
	if len(cfg.PrivateKey) != 32 {
		return nil, fmt.Errorf("client private key must be 32 bytes")
	}

	addr, err := netip.ParseAddr(cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("tunnel address %q: %w", cfg.Address, err)
	}
	dns, err := netip.ParseAddr(cfg.DNS)
	if err != nil {
		return nil, fmt.Errorf("tunnel dns %q: %w", cfg.DNS, err)
	}
	// The interface carries both families when IPv6 is on, exactly like Proton's
	// client: the IPv6 address is a fixed /128 the server routes back.
	addrs := []netip.Addr{addr}
	dnsAddrs := []netip.Addr{dns}
	if cfg.EnableIPv6 {
		addr6, err := netip.ParseAddr(cfg.IPv6)
		if err != nil {
			return nil, fmt.Errorf("tunnel ipv6 address %q: %w", cfg.IPv6, err)
		}
		dns6, err := netip.ParseAddr(cfg.DNS6)
		if err != nil {
			return nil, fmt.Errorf("tunnel ipv6 dns %q: %w", cfg.DNS6, err)
		}
		addrs = append(addrs, addr6)
		dnsAddrs = append(dnsAddrs, dns6)
	}

	tunDev, tnet, err := tnetstack.CreateNetTUN(addrs, dnsAddrs, cfg.MTU)
	if err != nil {
		return nil, fmt.Errorf("create netstack: %w", err)
	}

	var bind conn.Bind
	if cfg.Transport == TransportUDP {
		bind = conn.NewDefaultBind()
		cfg.Logf("tunnel: plain WireGuard over UDP to %s:%d", cfg.ServerIP, cfg.Port)
	} else {
		stealthBind, err := stealth.NewBind(stealth.Config{
			ServerIP:   cfg.ServerIP,
			Port:       cfg.Port,
			ServerName: cfg.SNI,
			Hello:      cfg.Hello,
			Logf:       cfg.Logf,
		})
		if err != nil {
			_ = tunDev.Close()
			return nil, err
		}
		bind = stealthBind
		cfg.Logf("tunnel: stealth bind, sni rotates per connection")
	}

	logger := device.NewLogger(device.LogLevelError, "wg: ")
	dev := device.NewDevice(tunDev, bind, logger)

	allowed := "allowed_ip=0.0.0.0/0\n"
	if cfg.EnableIPv6 {
		allowed += "allowed_ip=::/0\n"
	}
	ipc := fmt.Sprintf(
		"private_key=%s\npublic_key=%s\nendpoint=%s:%d\n%s\npersistent_keepalive_interval=25\n",
		hex.EncodeToString(cfg.PrivateKey),
		hex.EncodeToString(serverKey),
		cfg.ServerIP, cfg.Port, strings.TrimSuffix(allowed, "\n"),
	)
	if err := dev.IpcSet(ipc); err != nil {
		dev.Close()
		return nil, fmt.Errorf("configure wireguard device: %w", err)
	}

	tun := &Tunnel{cfg: cfg, dev: dev, net: tnet, bind: bind}
	if b, ok := bind.(*stealth.Bind); ok {
		tun.sniUsed = b.ServerName()
	} else {
		tun.sniUsed = "udp"
	}
	return tun, nil
}

// ServerName returns the SNI used for the stealth handshake ("udp" for the
// plain WireGuard transport), for the status line.
func (t *Tunnel) ServerName() string {
	if t.sniUsed != "" {
		return t.sniUsed
	}
	return t.cfg.SNI
}

// Start brings the device up and waits for the WireGuard handshake.
func (t *Tunnel) Start(ctx context.Context, timeout time.Duration) error {
	if err := t.dev.Up(); err != nil {
		return fmt.Errorf("bring up wireguard device: %w", err)
	}
	return t.WaitHandshake(ctx, timeout)
}

// WaitHandshake blocks until the peer handshake completes.
func (t *Tunnel) WaitHandshake(ctx context.Context, timeout time.Duration) error {
	if timeout == 0 {
		timeout = 20 * time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()

	for {
		if t.HasHandshake() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("WireGuard handshake did not complete within %s", timeout)
		case <-tick.C:
		}
	}
}

// HasHandshake reports whether a handshake has ever completed.
func (t *Tunnel) HasHandshake() bool {
	_, _, last, err := t.Stats()
	return err == nil && !last.IsZero()
}

// Stats reads the device counters from the UAPI.
func (t *Tunnel) Stats() (rx, tx int64, last time.Time, err error) {
	var sb strings.Builder
	if err := t.dev.IpcGetOperation(&sb); err != nil {
		return 0, 0, time.Time{}, err
	}
	for _, line := range strings.Split(sb.String(), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "rx_bytes":
			fmt.Sscanf(value, "%d", &rx)
		case "tx_bytes":
			fmt.Sscanf(value, "%d", &tx)
		case "last_handshake_time_sec":
			var sec int64
			fmt.Sscanf(value, "%d", &sec)
			if sec > 0 {
				last = time.Unix(sec, 0)
			}
		}
	}
	return rx, tx, last, nil
}

// DialContext opens a TCP connection through the tunnel. Domain names are
// resolved by the in-tunnel DNS server, so no plaintext lookup leaks.
func (t *Tunnel) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("tunnel: only tcp is supported, got %q", network)
	}
	return t.net.DialContext(ctx, "tcp", address)
}

// DialTCPAddrPort is the netip-typed variant.
func (t *Tunnel) DialTCPAddrPort(ctx context.Context, addr netip.AddrPort) (net.Conn, error) {
	return t.net.DialContext(ctx, "tcp", addr.String())
}

// ResolveHost resolves host inside the tunnel, using the tunnel's DNS server
// (10.2.0.1). Nothing is resolved on the host, so no plaintext lookup leaks.
// dnsTTL bounds how long a resolved name is reused. The tunnel is rebuilt on
// every connect and on a network change, so stale entries cannot outlive it.
const dnsTTL = 60 * time.Second

type dnsEntry struct {
	addrs   []netip.Addr
	expires time.Time
}

func (t *Tunnel) ResolveHost(ctx context.Context, host string) ([]netip.Addr, error) {
	// A SOCKS client such as mihomo sends hostnames, not addresses, so a music
	// player that opens dozens of image/API connections pays a DNS round trip
	// inside the tunnel for each new one. Cache the answer briefly instead.
	t.dnsMu.Lock()
	if e, ok := t.dnsCache[host]; ok && time.Now().Before(e.expires) {
		addrs := e.addrs
		t.dnsMu.Unlock()
		return addrs, nil
	}
	t.dnsMu.Unlock()

	addrs, err := t.net.LookupContextHost(ctx, host)
	if err != nil {
		// A single dropped query inside the tunnel shows up as a failed CONNECT
		// for every app that was waiting on it. Retry once unless the answer was
		// a real "no such host".
		if !strings.Contains(strings.ToLower(err.Error()), "no such host") {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(150 * time.Millisecond):
			}
			addrs, err = t.net.LookupContextHost(ctx, host)
		}
		if err != nil {
			return nil, fmt.Errorf("tunnel: resolve %q: %w", host, err)
		}
	}
	out := make([]netip.Addr, 0, len(addrs))
	for _, a := range addrs {
		if ip, err := netip.ParseAddr(a); err == nil {
			out = append(out, ip)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("tunnel: resolve %q: no usable address", host)
	}
	t.dnsMu.Lock()
	if t.dnsCache == nil {
		t.dnsCache = make(map[string]dnsEntry)
	}
	if len(t.dnsCache) >= 256 {
		for k := range t.dnsCache {
			delete(t.dnsCache, k)
			break
		}
	}
	t.dnsCache[host] = dnsEntry{addrs: out, expires: time.Now().Add(dnsTTL)}
	t.dnsMu.Unlock()
	return out, nil
}

// DialUDP opens a UDP socket inside the tunnel, connected to address
// ("host:port" or "ip:port"). Domain names are resolved inside the tunnel with
// a 5s budget (PROMPT.md 5.4.5), so the SOCKS5 UDP relay never leaks a DNS
// query to the local network.
func (t *Tunnel) DialUDP(ctx context.Context, address string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("tunnel: bad UDP address %q: %w", address, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("tunnel: bad UDP port %q", portStr)
	}
	var ip netip.Addr
	if parsed, err := netip.ParseAddr(host); err == nil {
		ip = parsed
	} else {
		rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		addrs, err := t.ResolveHost(rctx, host)
		if err != nil {
			return nil, err
		}
		ip = addrs[0]
	}
	return t.net.DialUDPAddrPort(netip.AddrPort{}, netip.AddrPortFrom(ip, uint16(port)))
}

// Close tears everything down.
func (t *Tunnel) Close() error {
	_ = t.bind.Close()
	if t.dev != nil {
		t.dev.Close()
	}
	return nil
}
