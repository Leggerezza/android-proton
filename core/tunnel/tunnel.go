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
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/device"
	tnetstack "golang.zx2c4.com/wireguard/tun/netstack"
	utls "github.com/refraction-networking/utls"

	"github.com/dsh/android-proton/core/stealth"
)

// Defaults match Proton's own client: the tunnel address is 10.2.0.2 and the
// in-tunnel DNS resolver is 10.2.0.1.
const (
	DefaultAddress = "10.2.0.2"
	DefaultDNS     = "10.2.0.1"
	DefaultMTU     = 1320
)

// Config describes one Stealth tunnel.
type Config struct {
	ServerIP   string
	Port       int
	ServerKey  string // base64 X25519 public key of the entry server
	PrivateKey []byte // 32-byte X25519 private key
	Address    string
	DNS        string
	MTU        int
	SNI        string
	Hello      utls.ClientHelloID
	Logf       func(format string, args ...any)
}

// Tunnel is a running userspace WireGuard-over-TLS tunnel.
type Tunnel struct {
	cfg  Config
	dev  *device.Device
	net  *tnetstack.Net
	bind *stealth.Bind
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
		cfg.Port = 443
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

	tunDev, tnet, err := tnetstack.CreateNetTUN([]netip.Addr{addr}, []netip.Addr{dns}, cfg.MTU)
	if err != nil {
		return nil, fmt.Errorf("create netstack: %w", err)
	}

	bind, err := stealth.NewBind(stealth.Config{
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

	logger := device.NewLogger(device.LogLevelError, "wg: ")
	dev := device.NewDevice(tunDev, bind, logger)

	ipc := fmt.Sprintf(
		"private_key=%s\npublic_key=%s\nendpoint=%s:%d\nallowed_ip=0.0.0.0/0\npersistent_keepalive_interval=25\n",
		hex.EncodeToString(cfg.PrivateKey),
		hex.EncodeToString(serverKey),
		cfg.ServerIP, cfg.Port,
	)
	if err := dev.IpcSet(ipc); err != nil {
		dev.Close()
		return nil, fmt.Errorf("configure wireguard device: %w", err)
	}

	return &Tunnel{cfg: cfg, dev: dev, net: tnet, bind: bind}, nil
}

// ServerName returns the SNI used for the handshake.
func (t *Tunnel) ServerName() string { return t.bind.ServerName() }

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

// Close tears everything down.
func (t *Tunnel) Close() error {
	_ = t.bind.Close()
	if t.dev != nil {
		t.dev.Close()
	}
	return nil
}
