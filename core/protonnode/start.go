// SPDX-License-Identifier: GPL-3.0-or-later
package protonnode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/leggerezza/android-proton/core/socks5"
	"github.com/leggerezza/android-proton/core/stealth"
	"github.com/leggerezza/android-proton/core/tunnel"
)

// Timing constants. A dropped tunnel is retried immediately with the entries
// that just worked; only repeated failures back off (2s, 5s, 15s, 30s, 60s).
// The health check runs every 15s, so two misses are caught within 30s instead
// of a minute.
const (
	healthCheckInterval = 15 * time.Second
	// gameLockRounds is how many consecutive failures to the SAME entry are
	// retried before switching servers while gameLock is on (PROMPT.md 7:
	// switching servers kills every game session, so it is the last resort).
	gameLockRounds = 3
)

// startConfig is the JSON accepted by Start(). Zero values pick the defaults
// documented on each field.
type startConfig struct {
	// Mode is "normal" or "game". Game mode forbids Secure Core and defaults to
	// the low-latency country order (PROMPT.md 7).
	Mode string `json:"mode,omitempty"`
	// Server pins one logical server by name, e.g. "JP#377".
	Server string `json:"server,omitempty"`
	// Country restricts the exit country (ISO code, e.g. "JP").
	Country string `json:"country,omitempty"`
	// Prefer is the country preference order used when neither Server nor
	// Country is set. Defaults to US,CA,NL (game mode: JP,HK,KR,SG,US).
	Prefer []string `json:"prefer,omitempty"`
	// SecureCore prefers two-hop servers (never allowed in game mode).
	SecureCore bool `json:"secureCore,omitempty"`
	// LockServer keeps the current server across reconnects; nil means the
	// default (true in game mode, false otherwise).
	LockServer *bool `json:"lockServer,omitempty"`
	// SocksPort is the loopback SOCKS5 port (default 1080).
	SocksPort int `json:"socksPort,omitempty"`
	// MTU is the tunnel MTU (default 1320; PROMPT.md 7 says start at 1280).
	MTU int `json:"mtu,omitempty"`
	// SNI overrides the TLS server name of the entry (default: rotated pool).
	SNI string `json:"sni,omitempty"`
	// Fingerprint selects the utls ClientHello: chrome|firefox|safari|edge|golang.
	Fingerprint string `json:"fingerprint,omitempty"`
	// HandshakeTimeoutSec bounds the WireGuard handshake (default 25).
	HandshakeTimeoutSec int `json:"handshakeTimeoutSec,omitempty"`
	// ProbeTimeoutMs bounds each L1 TCP probe (default 4000).
	ProbeTimeoutMs int `json:"probeTimeoutMs,omitempty"`
	// MaxProbe is how many reachable entries to try before giving up on this
	// round (default 3).
	MaxProbe int `json:"maxProbe,omitempty"`
	// Transport selects the outer transport: "stealth" (WireGuard in TLS/TCP,
	// default) or "udp" (plain WireGuard, much faster where UDP is allowed).
	Transport string `json:"transport,omitempty"`
	// IPv6 adds Proton's fixed IPv6 address to the interface, so applications can
	// reach IPv6 destinations (only on servers that advertise the IPv6 feature).
	IPv6 bool `json:"ipv6,omitempty"`
	// ExcludeCountries lists exit countries that must never be connected.
	ExcludeCountries []string `json:"excludeCountries,omitempty"`
	// ScanLevel is the verification depth used for automatic selection:
	// 3 (default; PROMPT.md 5.5: only a real WireGuard handshake counts),
	// 2 or 1 to trade correctness for speed.
	ScanLevel int `json:"scanLevel,omitempty"`
}

func (c *startConfig) applyDefaults() error {
	c.Mode = strings.ToLower(strings.TrimSpace(c.Mode))
	if c.Mode == "" {
		c.Mode = "normal"
	}
	switch c.Mode {
	case "normal", "game":
	default:
		return fmt.Errorf("protonnode: unknown mode %q (normal|game)", c.Mode)
	}
	if c.Mode == "game" {
		if c.SecureCore {
			return errors.New("protonnode: Secure Core is not available in game mode")
		}
		if len(c.Prefer) == 0 {
			c.Prefer = []string{"JP", "HK", "KR", "SG", "US"}
		}
		if c.LockServer == nil {
			v := true
			c.LockServer = &v
		}
	}
	// No default country order outside game mode: a fixed list pins the user to
	// whatever the list author had in mind (US/CA/NL put an Asia user 150 ms+
	// away from every exit). With no preference the scan covers many countries
	// and the RTT pre-pass picks the nearest entry.
	for i := range c.Prefer {
		c.Prefer[i] = strings.ToUpper(strings.TrimSpace(c.Prefer[i]))
	}
	if c.SocksPort == 0 {
		c.SocksPort = 1080
	}
	if c.SocksPort < 1024 || c.SocksPort > 65535 {
		return fmt.Errorf("protonnode: socksPort %d out of range", c.SocksPort)
	}
	if c.MTU == 0 {
		c.MTU = tunnel.DefaultMTU
	}
	if c.MTU < 576 || c.MTU > 1500 {
		return fmt.Errorf("protonnode: mtu %d out of range", c.MTU)
	}
	if c.Fingerprint == "" {
		c.Fingerprint = "chrome"
	}
	if _, err := stealth.HelloFromName(c.Fingerprint); err != nil {
		return err
	}
	if c.HandshakeTimeoutSec <= 0 {
		c.HandshakeTimeoutSec = 25
	}
	if c.ProbeTimeoutMs <= 0 {
		c.ProbeTimeoutMs = 4000
	}
	if c.MaxProbe <= 0 {
		c.MaxProbe = 3
	}
	c.Transport = strings.ToLower(strings.TrimSpace(c.Transport))
	if c.Transport == "" {
		c.Transport = tunnel.TransportStealth
	}
	if c.Transport != tunnel.TransportStealth && c.Transport != tunnel.TransportUDP {
		return fmt.Errorf("protonnode: unknown transport %q (stealth|udp)", c.Transport)
	}
	for i := range c.ExcludeCountries {
		c.ExcludeCountries[i] = strings.ToUpper(strings.TrimSpace(c.ExcludeCountries[i]))
	}
	if c.ScanLevel == 0 {
		c.ScanLevel = 3
	}
	if c.ScanLevel < 1 || c.ScanLevel > 3 {
		return fmt.Errorf("protonnode: scanLevel %d out of range (1..3)", c.ScanLevel)
	}
	return nil
}

// runState is one Start()..Stop() window.
type runState struct {
	cfg    startConfig
	cancel context.CancelFunc
	done   chan struct{}

	mu          sync.Mutex
	tun         *tunnel.Tunnel
	socks       *socks5.Server
	socksAddr   string
	server      string
	entryIP     string
	sni         string
	handshakeMS int64
	startedAt   time.Time
	reconnects  int
	lastError   string
	exitIP      string
}

func (r *runState) setError(err error) {
	r.mu.Lock()
	if err != nil {
		r.lastError = err.Error()
	} else {
		r.lastError = ""
	}
	r.mu.Unlock()
}

func (r *runState) setConnected(server, entryIP, sni string, handshakeMS int64) {
	r.mu.Lock()
	r.server = server
	r.entryIP = entryIP
	r.sni = sni
	r.handshakeMS = handshakeMS
	r.startedAt = time.Now()
	r.lastError = ""
	r.mu.Unlock()
}

func (r *runState) setSocks(addr string) {
	r.mu.Lock()
	r.socksAddr = addr
	r.mu.Unlock()
}

func (r *runState) setTunnel(t *tunnel.Tunnel, s *socks5.Server) {
	r.mu.Lock()
	r.tun = t
	r.socks = s
	r.mu.Unlock()
}

func (r *runState) bumpReconnects() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reconnects++
	return r.reconnects
}

// Start connects in the background and returns immediately. Progress is
// observable through Status() and RecentLogs(); a failure to reach any server
// is retried every reconnectDelay seconds, it does not make Start() fail.
func Start(cfgJSON string) (string, error) {
	var cfg startConfig
	if cfgJSON != "" {
		if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
			return "", fmt.Errorf("protonnode: parse start config: %w", err)
		}
	}
	if err := cfg.applyDefaults(); err != nil {
		return "", err
	}

	eng.mu.Lock()
	if eng.client == nil {
		eng.mu.Unlock()
		return "", errNotInitialized
	}
	if eng.run != nil {
		eng.mu.Unlock()
		return "", errAlreadyRunning
	}
	if eng.session.Access == "" && eng.session.Refresh == "" {
		eng.mu.Unlock()
		return "", errNoSession
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &runState{cfg: cfg, cancel: cancel, done: make(chan struct{})}
	eng.run = run
	eng.runCfg = cfg
	eng.lastError = ""
	eng.state = stateScanning
	eng.mu.Unlock()

	eng.logfOrNil("start: mode %s, socks 127.0.0.1:%d, mtu %d, fingerprint %s",
		cfg.Mode, cfg.SocksPort, cfg.MTU, cfg.Fingerprint)
	go eng.supervise(ctx, run)
	return "", nil
}

// Stop tears the tunnel down. It is idempotent and bounded by 5s (PROMPT.md
// 5.2: stop SOCKS -> close active connections -> close UDP associations ->
// close WireGuard -> wait for goroutines).
func Stop() error {
	eng.mu.Lock()
	run := eng.run
	eng.mu.Unlock()
	if run == nil {
		return nil
	}
	eng.logfOrNil("stop: requested")
	run.cancel()
	select {
	case <-run.done:
	case <-time.After(5 * time.Second):
		eng.logfOrNil("stop: goroutines did not finish within 5s")
	}
	run.mu.Lock()
	socks, tun := run.socks, run.tun
	run.mu.Unlock()
	if socks != nil && socks.Listener != nil {
		_ = socks.Listener.Close()
	}
	if tun != nil {
		_ = tun.Close()
	}
	eng.mu.Lock()
	if eng.run == run {
		eng.run = nil
	}
	eng.state = stateIdle
	eng.mu.Unlock()
	eng.logfOrNil("stop: done")
	return nil
}

// supervise is the reconnect loop: connect, serve, and on any failure retry
// after reconnectDelay. Every attempt re-runs server selection unless the
// game-lock policy says the current entry must be kept.
func (e *engine) supervise(ctx context.Context, run *runState) {
	defer close(run.done)
	attempt := 0
	sameServerFailures := 0
	// reconnectBackoff is indexed by consecutive failures; index 0 means "retry
	// right away", which is what happens after a tunnel that had been up drops.
	reconnectBackoff := []time.Duration{0, 2 * time.Second, 5 * time.Second, 15 * time.Second, 30 * time.Second, 60 * time.Second}
	lockedServer := run.cfg.Server
	for {
		if ctx.Err() != nil {
			e.setState(stateIdle)
			return
		}
		if attempt > 0 {
			e.setState(stateReconnecting)
			idx := attempt
			if idx >= len(reconnectBackoff) {
				idx = len(reconnectBackoff) - 1
			}
			wait := reconnectBackoff[idx]
			if wait > 0 {
				e.logfOrNil("reconnect: attempt %d in %s", attempt+1, wait)
				if !sleepCtx(ctx, wait) {
					e.setState(stateIdle)
					return
				}
			} else {
				e.logfOrNil("reconnect: attempt %d immediately", attempt+1)
			}
		}
		attempt++

		// While game-lock is on and the previous server still has retry budget,
		// keep hammering the same logical server instead of picking a new exit
		// (PROMPT.md 7: a server switch drops every game session).
		lock := run.cfg.LockServer != nil && *run.cfg.LockServer
		if lock && lockedServer != "" && sameServerFailures < gameLockRounds {
			run.cfg.Server = lockedServer
		} else if sameServerFailures >= gameLockRounds {
			e.logfOrNil("reconnect: %d failed rounds on %s, allowing a server switch", sameServerFailures, lockedServer)
			run.cfg.Server = ""
			lockedServer = ""
		}

		if err := e.connectOnce(ctx, run); err != nil {
			if ctx.Err() != nil {
				e.setState(stateIdle)
				return
			}
			run.setError(err)
			e.mu.Lock()
			e.lastError = err.Error()
			e.mu.Unlock()
			e.logfOrNil("connect failed: %v", err)
			run.bumpReconnects()
			sameServerFailures++
			continue
		}
		sameServerFailures = 0
		e.setState(stateConnected)
		if err := e.serve(ctx, run); err != nil {
			run.setError(err)
			e.logfOrNil("tunnel lost: %v", err)
		}
		e.teardown(run)
		if ctx.Err() != nil {
			e.setState(stateIdle)
			return
		}
		// The tunnel had been up, so the next attempt reconnects right away with
		// the last verified entries instead of waiting out a backoff.
		attempt = 0
	}
}

// connectOnce performs one full selection + handshake cycle.
func (e *engine) connectOnce(ctx context.Context, run *runState) error {
	e.setState(stateScanning)
	e.mu.Lock()
	c := e.client
	tier := e.tier
	eng.mu.Unlock()
	if c == nil {
		return errNotInitialized
	}
	servers, err := e.servers(ctx)
	if err != nil {
		return err
	}
	candidates := selectCandidates(servers, &run.cfg, tier)
	if len(candidates) == 0 {
		return errors.New("no server matches the configured filter")
	}
	port := stealthPort()
	if run.cfg.Transport == tunnel.TransportUDP {
		port = wireGuardUDPPort()
	}
	picks, err := e.pickCandidates(ctx, servers, candidates, run.cfg, port)
	if err != nil {
		return err
	}
	if len(picks) == 0 {
		return errors.New("no reachable Proton entry")
	}
	if err := e.ensureCertificate(ctx); err != nil {
		return err
	}
	e.mu.Lock()
	kp, kperr := e.session.keyPair()
	e.mu.Unlock()
	if kperr != nil {
		return kperr
	}
	hello, err := stealth.HelloFromName(run.cfg.Fingerprint)
	if err != nil {
		return err
	}
	var lastErr error
	for i, cand := range picks {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if cand.phys == nil || cand.phys.X25519PublicKey == "" {
			continue
		}
		e.setState(stateConnecting)
		e.logfOrNil("server %d/%d: %s (%s, %s) entry %s load %d%% secure-core=%v",
			i+1, len(picks), cand.server.Name, cand.server.ExitCountry, cand.server.City,
			cand.phys.EntryIP, cand.server.Load, cand.server.IsSecureCore())
		// Proton only routes the fixed IPv6 /128 on servers that advertise the
		// feature; on the others adding the address would make IPv6 dials hang
		// instead of failing fast, so it is decided per server.
		enableIPv6 := run.cfg.IPv6 && cand.server.IsIPv6Supported()
		tun, terr := tunnel.New(tunnel.Config{
			ServerIP:   cand.phys.EntryIP,
			Port:       port,
			ServerKey:  cand.phys.X25519PublicKey,
			PrivateKey: kp.X25519PrivateKey(),
			MTU:        run.cfg.MTU,
			SNI:        run.cfg.SNI,
			Hello:      hello,
			Transport:  run.cfg.Transport,
			EnableIPv6: enableIPv6,
			Logf:       e.logfOrNil,
		})
		if terr != nil {
			lastErr = terr
			e.logfOrNil("attempt failed: %v", terr)
			continue
		}
		start := time.Now()
		hctx, hcancel := context.WithTimeout(ctx, time.Duration(run.cfg.HandshakeTimeoutSec)*time.Second)
		herr := tun.Start(hctx, time.Duration(run.cfg.HandshakeTimeoutSec)*time.Second)
		hcancel()
		if herr != nil {
			_ = tun.Close()
			lastErr = herr
			e.logfOrNil("attempt failed: %v", herr)
			continue
		}
		handshake := time.Since(start).Milliseconds()
		run.setTunnel(tun, nil)
		run.setConnected(cand.server.Name, cand.phys.EntryIP, tun.ServerName(), handshake)
		e.mu.Lock()
		e.session.LastServer = cand.server.Name
		e.mu.Unlock()
		e.savePrefs()
		e.logfOrNil("tunnel up in %dms (sni %s)", handshake, tun.ServerName())
		return nil
	}
	if lastErr == nil {
		lastErr = errors.New("no candidate had a usable WireGuard key")
	}
	return lastErr
}

// serve runs SOCKS5 on the loopback interface and watches the tunnel. It
// returns when the tunnel is judged dead or ctx is cancelled.
func (e *engine) serve(ctx context.Context, run *runState) error {
	run.mu.Lock()
	tun := run.tun
	port := run.cfg.SocksPort
	run.mu.Unlock()
	if tun == nil {
		return errors.New("serve: no tunnel")
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	srv := &socks5.Server{
		Addr: addr,
		Dial: func(dctx context.Context, network, address string) (net.Conn, error) {
			return tun.DialContext(dctx, network, address)
		},
		// UDP ASSOCIATE is served through the same tunnel: the dst domain is
		// resolved by 10.2.0.1, never on the host (PROMPT.md 5.4 / 5.6).
		DialUDP: func(dctx context.Context, address string) (net.Conn, error) {
			return tun.DialUDP(dctx, address)
		},
		Logf: e.logfOrNil,
	}
	run.mu.Lock()
	run.socks = srv
	run.socksAddr = addr
	run.mu.Unlock()
	e.logfOrNil("SOCKS5 listening on %s (TCP CONNECT + UDP ASSOCIATE)", addr)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go e.healthLoop(runCtx, tun, cancel)

	err := srv.ListenAndServe(runCtx)
	if ctx.Err() != nil || runCtx.Err() != nil {
		if err == nil {
			return nil
		}
		return errors.New("health check failed twice in a row")
	}
	return err
}

// healthLoop probes through the tunnel every healthCheckInterval and cancels
// the run after two consecutive failures (PROMPT.md 2: 30s probe x2).
func (e *engine) healthLoop(ctx context.Context, tun *tunnel.Tunnel, cancel context.CancelFunc) {
	tick := time.NewTicker(healthCheckInterval)
	defer tick.Stop()
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if probeThroughTunnel(ctx, tun) {
				if failures > 0 {
					e.logfOrNil("health: recovered after %d failure(s)", failures)
				}
				failures = 0
				continue
			}
			failures++
			e.logfOrNil("health: check failed (%d/2)", failures)
			if failures >= 2 {
				e.logfOrNil("health: tunnel judged dead")
				cancel()
				return
			}
		}
	}
}

// probeThroughTunnel proves that TCP really flows through the tunnel: a plain
// HTTP/1.1 request to a well-known host, no TLS (cheaper and it also works
// when the exit blocks 443).
func probeThroughTunnel(ctx context.Context, tun *tunnel.Tunnel) bool {
	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := tun.DialContext(dctx, "tcp", "www.gstatic.com:80")
	if err != nil {
		return false
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
	req := "HEAD /generate_204 HTTP/1.1\r\nHost: www.gstatic.com\r\nConnection: close\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		return false
	}
	buf := make([]byte, 1)
	n, _ := conn.Read(buf)
	return n > 0
}

// teardown closes the SOCKS listener and the tunnel of the current attempt.
func (e *engine) teardown(run *runState) {
	run.mu.Lock()
	socks, tun := run.socks, run.tun
	run.socks, run.tun = nil, nil
	run.mu.Unlock()
	if socks != nil && socks.Listener != nil {
		_ = socks.Listener.Close()
	}
	if tun != nil {
		_ = tun.Close()
	}
}

// stealthPort is the TCP port Proton serves Stealth on. 443 is the default and
// the only one the baseline ever observed; /vpn/v2/clientconfig can override
// it and is consulted lazily in P3 (scanning) to keep P1's startup offline-safe.
func stealthPort() int { return 443 }

// wireGuardUDPPort is Proton's plain-WireGuard UDP port. 51820 is the standard
// value and the fallback when /vpn/v2/clientconfig cannot be read.
func wireGuardUDPPort() int {
	eng.mu.Lock()
	c := eng.client
	eng.mu.Unlock()
	if c == nil {
		return 51820
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if p := c.WireGuardUDPPort(ctx); p > 0 {
		return p
	}
	return 51820
}
