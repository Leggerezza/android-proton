// SPDX-License-Identifier: GPL-3.0-or-later
// Package scan implements the three-level server verification from PROMPT.md 5.5:
//
//	L1  TCP connect to the entry IP (3s)
//	L2  real TLS handshake with a Chrome ClientHello and a rotated safe SNI (5s)
//	L3  complete WireGuard handshake through the Stealth transport
//
// Only L3 counts for automatic server selection: a reachable or TLS-speaking
// entry says nothing about whether the WireGuard peer accepts our key.
package scan

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"

	"github.com/leggerezza/android-proton/core/stealth"
	"github.com/leggerezza/android-proton/core/tunnel"
)

// Level is the verification depth.
type Level int

const (
	LevelTCP    Level = 1
	LevelTLS    Level = 2
	LevelTunnel Level = 3
)

func (l Level) String() string {
	switch l {
	case LevelTCP:
		return "tcp"
	case LevelTLS:
		return "tls"
	case LevelTunnel:
		return "tunnel"
	default:
		return "unknown"
	}
}

// ParseLevel accepts 1/2/3 or "tcp"/"tls"/"tunnel".
func ParseLevel(s string) (Level, error) {
	switch s {
	case "", "3", "tunnel", "l3", "L3":
		return LevelTunnel, nil
	case "1", "tcp", "l1", "L1":
		return LevelTCP, nil
	case "2", "tls", "l2", "L2":
		return LevelTLS, nil
	}
	return 0, fmt.Errorf("scan: unknown level %q (1|2|3)", s)
}

// Target is one entry IP to verify.
type Target struct {
	Server    string  `json:"server"`
	EntryIP   string  `json:"entryIP"`
	Port      int     `json:"port"`
	ServerKey string  `json:"serverKey,omitempty"`
	SNI       string  `json:"sni,omitempty"`
	Country   string  `json:"country,omitempty"`
	Score     float64 `json:"score,omitempty"`
	Load      int     `json:"load,omitempty"`
}

func (t Target) addr() string { return net.JoinHostPort(t.EntryIP, strconv.Itoa(t.Port)) }

// Result is one verified target.
type Result struct {
	Server    string  `json:"server"`
	ServerKey string  `json:"serverKey,omitempty"`
	EntryIP   string  `json:"entryIP"`
	Port      int     `json:"port"`
	Level     int     `json:"level"`
	RTTMs     int64   `json:"rttMs"`
	SNI       string  `json:"sni,omitempty"`
	Country   string  `json:"country,omitempty"`
	Score     float64 `json:"score,omitempty"`
	Load      int     `json:"load,omitempty"`
}

// Options bounds one scan. Zero values pick the documented defaults.
type Options struct {
	Level       Level
	Concurrency int           // default 32
	L1Timeout   time.Duration // default 3s
	L2Timeout   time.Duration // default 5s
	L3Timeout   time.Duration // default 12s
	MaxL2       int           // candidates promoted to L2, default 24
	MaxL3       int           // candidates promoted to L3, default 3
	PrivateKey  []byte        // 32-byte X25519 key, required for L3
	Hello       utls.ClientHelloID
	Logf        func(format string, args ...any)
}

func (o *Options) applyDefaults() {
	if o.Level == 0 {
		o.Level = LevelTunnel
	}
	if o.Concurrency <= 0 {
		o.Concurrency = 32
	}
	if o.L1Timeout <= 0 {
		o.L1Timeout = 3 * time.Second
	}
	if o.L2Timeout <= 0 {
		o.L2Timeout = 5 * time.Second
	}
	if o.L3Timeout <= 0 {
		o.L3Timeout = 12 * time.Second
	}
	if o.MaxL2 <= 0 {
		o.MaxL2 = 24
	}
	if o.MaxL3 <= 0 {
		o.MaxL3 = 3
	}
	if o.Hello.Client == "" {
		o.Hello = stealth.DefaultHello
	}
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
}

// State is the scan lifecycle.
type State string

const (
	StateRunning   State = "running"
	StateDone      State = "done"
	StateCancelled State = "cancelled"
	StateFailed    State = "failed"
)

// Progress is the ScanProgress() payload.
type Progress struct {
	ScanID    string `json:"scanId"`
	State     string `json:"state"`
	Level     int    `json:"level"`
	Stage     string `json:"stage"`
	Total     int    `json:"total"`
	Done      int    `json:"done"`
	Passed    int    `json:"passed"`
	StartedAt string `json:"startedAt"`
	ElapsedMs int64  `json:"elapsedMs"`
	Error     string `json:"error,omitempty"`
}

// Results is the ScanResults() payload.
type Results struct {
	ScanID      string   `json:"scanId"`
	State       string   `json:"state"`
	Level       int      `json:"level"`
	Stage       string   `json:"stage"`
	Fingerprint string   `json:"fingerprint"`
	GeneratedAt string   `json:"generatedAt"`
	ElapsedMs   int64    `json:"elapsedMs"`
	Checked     int      `json:"checked"`
	Passed      int      `json:"passed"`
	Results     []Result `json:"results"`
	CacheHit    bool     `json:"cacheHit"`
	Error       string   `json:"error,omitempty"`
}

// Runner owns one scan.
type Runner struct {
	id      string
	opts    Options
	targets []Target

	mu       sync.Mutex
	state    State
	stage    string
	total    int
	done     int
	passed   int
	started  time.Time
	finished time.Time
	err      error
	results  []Result

	cancel    context.CancelFunc
	doneCh    chan struct{}
	fromCache bool
}

// New creates a scan over targets; Start runs it.
func New(targets []Target, opts Options) (*Runner, error) {
	opts.applyDefaults()
	if opts.Level >= LevelTunnel && len(opts.PrivateKey) != 32 {
		return nil, errors.New("scan: level 3 needs a 32-byte WireGuard private key")
	}
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, err
	}
	return &Runner{
		id:      "scan-" + hex.EncodeToString(b[:]),
		opts:    opts,
		targets: targets,
		state:   StateRunning,
		total:   len(targets),
		started: time.Now(),
		doneCh:  make(chan struct{}),
	}, nil
}

// FromCache builds an already-finished scan from cached results, so callers can
// treat "cache hit" and "fresh scan" through the same API.
func FromCache(results []Result, level Level) *Runner {
	now := time.Now()
	r := &Runner{
		id:        "scan-cache-" + now.UTC().Format("20060102T150405Z"),
		opts:      Options{Level: level},
		state:     StateDone,
		stage:     "l3",
		total:     len(results),
		done:      len(results),
		passed:    len(results),
		started:   now,
		finished:  now,
		results:   append([]Result(nil), results...),
		doneCh:    make(chan struct{}),
		fromCache: true,
	}
	close(r.doneCh)
	return r
}

// ID is the scan identifier.
func (r *Runner) ID() string { return r.id }

// Start runs the scan in the background; the channel closes when it is over.
func (r *Runner) Start(parent context.Context) <-chan struct{} {
	ctx, cancel := context.WithCancel(parent)
	r.mu.Lock()
	r.cancel = cancel
	r.mu.Unlock()
	go func() {
		defer close(r.doneCh)
		defer cancel()
		r.run(ctx)
	}()
	return r.doneCh
}

// Cancel asks the scan to stop and reports whether it was still running.
// Workers observe the context between every step, so a scan stops well inside
// the "cancel takes effect within 1s" bar of PROMPT.md 11.
func (r *Runner) Cancel() bool {
	r.mu.Lock()
	running := r.state == StateRunning
	cancel := r.cancel
	r.mu.Unlock()
	if !running || cancel == nil {
		return false
	}
	cancel()
	return true
}

func (r *Runner) setStage(stage string) {
	r.mu.Lock()
	r.stage = stage
	r.mu.Unlock()
}

func (r *Runner) finish(state State, err error) {
	r.mu.Lock()
	if r.state == StateRunning {
		r.state = state
	}
	if err != nil && r.err == nil {
		r.err = err
	}
	r.finished = time.Now()
	r.mu.Unlock()
}

// ProgressJSON renders the ScanProgress payload.
func (r *Runner) ProgressJSON() string {
	r.mu.Lock()
	p := Progress{
		ScanID:    r.id,
		State:     string(r.state),
		Level:     int(r.opts.Level),
		Stage:     r.stage,
		Total:     r.total,
		Done:      r.done,
		Passed:    r.passed,
		StartedAt: r.started.UTC().Format(time.RFC3339),
	}
	if !r.finished.IsZero() {
		p.ElapsedMs = r.finished.Sub(r.started).Milliseconds()
	} else {
		p.ElapsedMs = time.Since(r.started).Milliseconds()
	}
	if r.err != nil {
		p.Error = r.err.Error()
	}
	r.mu.Unlock()
	buf, _ := json.Marshal(p)
	return string(buf)
}

// ResultsJSON renders the ScanResults payload.
func (r *Runner) ResultsJSON(fingerprint string, cacheHit bool) string {
	r.mu.Lock()
	out := Results{
		ScanID:      r.id,
		State:       string(r.state),
		Level:       int(r.opts.Level),
		Stage:       r.stage,
		Fingerprint: fingerprint,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Checked:     len(r.targets),
		Passed:      r.passed,
		Results:     append([]Result{}, r.results...),
		CacheHit:    cacheHit || r.fromCache,
	}
	if !r.finished.IsZero() {
		out.ElapsedMs = r.finished.Sub(r.started).Milliseconds()
	} else {
		out.ElapsedMs = time.Since(r.started).Milliseconds()
	}
	if r.err != nil {
		out.Error = r.err.Error()
	}
	if out.Results == nil {
		out.Results = []Result{}
	}
	r.mu.Unlock()
	buf, _ := json.Marshal(out)
	return string(buf)
}

// Results returns a copy of the verified results.
func (r *Runner) Results() []Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Result(nil), r.results...)
}

// State returns the lifecycle state.
func (r *Runner) State() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

// Checked returns how many targets the scan looked at.
func (r *Runner) Checked() int { return len(r.targets) }

// run executes L1 -> L2 -> L3.
func (r *Runner) run(ctx context.Context) {
	r.setStage("l1")
	l1, l1err := r.sweep(ctx, r.targets, func(ctx context.Context, t Target) (Result, error) {
		return probeTCP(ctx, t, r.opts.L1Timeout)
	})
	if ctx.Err() != nil {
		r.finish(StateCancelled, nil)
		return
	}
	r.opts.Logf("scan %s: l1 %d/%d reachable", r.id, len(l1), len(r.targets))
	if r.opts.Level <= LevelTCP {
		r.complete(l1, l1err)
		return
	}

	r.setStage("l2")
	sortByQualityR(l1)
	l2Input := targetsOf(l1)
	if len(l2Input) > r.opts.MaxL2 {
		l2Input = l2Input[:r.opts.MaxL2]
	}
	r.resetCounters(len(l2Input))
	l2, l2err := r.sweep(ctx, l2Input, func(ctx context.Context, t Target) (Result, error) {
		return probeTLS(ctx, t, r.opts.L2Timeout, r.opts.Hello, r.opts.Logf)
	})
	if ctx.Err() != nil {
		r.finish(StateCancelled, nil)
		return
	}
	r.opts.Logf("scan %s: l2 %d/%d tls ok", r.id, len(l2), len(l2Input))
	if r.opts.Level <= LevelTLS {
		r.complete(l2, l2err, l1err)
		return
	}

	r.setStage("l3")
	sortByQualityR(l2)
	l3Input := targetsOf(l2)
	if len(l3Input) > r.opts.MaxL3 {
		l3Input = l3Input[:r.opts.MaxL3]
	}
	r.resetCounters(len(l3Input))
	l3, l3err := r.sweepTunnel(ctx, l3Input)
	if ctx.Err() != nil {
		r.finish(StateCancelled, nil)
		return
	}
	r.opts.Logf("scan %s: l3 %d/%d wireguard ok", r.id, len(l3), len(l3Input))
	r.complete(l3, l3err, l2err, l1err)
}

func (r *Runner) resetCounters(total int) {
	r.mu.Lock()
	r.total = total
	r.done = 0
	r.passed = 0
	r.mu.Unlock()
}

func (r *Runner) complete(results []Result, errs ...error) {
	sortByQualityR(results)
	r.mu.Lock()
	r.results = results
	r.passed = len(results)
	r.mu.Unlock()
	var first error
	for _, e := range errs {
		if e != nil && first == nil {
			first = e
		}
	}
	if len(results) == 0 {
		if first == nil {
			first = errors.New("scan: no target passed")
		}
		r.finish(StateFailed, first)
		return
	}
	r.finish(StateDone, nil)
}

// sweep runs probe over targets with the configured concurrency.
func (r *Runner) sweep(ctx context.Context, targets []Target, probe func(context.Context, Target) (Result, error)) ([]Result, error) {
	var (
		mu       sync.Mutex
		results  []Result
		firstErr error
		wg       sync.WaitGroup
	)
	sem := make(chan struct{}, r.opts.Concurrency)
	for _, t := range targets {
		if ctx.Err() != nil {
			break
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(t Target) {
			defer wg.Done()
			defer func() { <-sem }()
			res, err := probe(ctx, t)
			mu.Lock()
			r.mu.Lock()
			r.done++
			if err == nil {
				results = append(results, res)
				r.passed++
			} else if firstErr == nil && !errors.Is(err, context.Canceled) {
				firstErr = err
			}
			r.mu.Unlock()
			mu.Unlock()
		}(t)
	}
	wg.Wait()
	return results, firstErr
}

// probeTCP is L1: a plain TCP connect.
func probeTCP(ctx context.Context, t Target, timeout time.Duration) (Result, error) {
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	c, err := d.DialContext(ctx, "tcp", t.addr())
	if err != nil {
		return Result{}, err
	}
	rtt := time.Since(start).Milliseconds()
	_ = c.Close()
	return Result{
		Server: t.Server, ServerKey: t.ServerKey, EntryIP: t.EntryIP, Port: t.Port, Level: int(LevelTCP),
		RTTMs: rtt, Country: t.Country, Score: t.Score, Load: t.Load,
	}, nil
}

// probeTLS is L2: a real TLS handshake. The certificate is not validated (the
// Stealth terminator presents a rotating certificate), the one documented
// exception in PROMPT.md 0.4; the bypass is logged once per process.
func probeTLS(ctx context.Context, t Target, timeout time.Duration, hello utls.ClientHelloID, logf func(string, ...any)) (Result, error) {
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	raw, err := d.DialContext(ctx, "tcp", t.addr())
	if err != nil {
		return Result{}, err
	}
	defer raw.Close()
	sni := t.SNI
	if sni == "" {
		snis := stealth.SafeServerNamesForIP(t.EntryIP)
		if len(snis) > 0 {
			sni = snis[0]
		}
	}
	cfg := &utls.Config{InsecureSkipVerify: true, ServerName: sni}
	uc := utls.UClient(raw, cfg, hello)
	_ = uc.SetDeadline(time.Now().Add(timeout))
	if err := uc.HandshakeContext(ctx); err != nil {
		return Result{}, err
	}
	rtt := time.Since(start).Milliseconds()
	_ = uc.Close()
	bypassOnce.Do(func() {
		logf("scan: certificate verification is skipped for Proton entries (rotating cert; PROMPT.md 0.4)")
	})
	return Result{
		Server: t.Server, ServerKey: t.ServerKey, EntryIP: t.EntryIP, Port: t.Port, Level: int(LevelTLS),
		RTTMs: rtt, SNI: sni, Country: t.Country, Score: t.Score, Load: t.Load,
	}, nil
}

// sweepTunnel runs the L3 stage: a full WireGuard handshake per target.
func (r *Runner) sweepTunnel(ctx context.Context, targets []Target) ([]Result, error) {
	var (
		results  []Result
		firstErr error
	)
	for _, t := range targets {
		if ctx.Err() != nil {
			break
		}
		res, err := r.probeTunnel(ctx, t)
		r.mu.Lock()
		r.done++
		if err == nil {
			results = append(results, res)
			r.passed++
		} else if firstErr == nil && !errors.Is(err, context.Canceled) {
			firstErr = err
		}
		r.mu.Unlock()
	}
	return results, firstErr
}

func (r *Runner) probeTunnel(ctx context.Context, t Target) (Result, error) {
	if t.ServerKey == "" {
		return Result{}, errors.New("scan: target has no WireGuard public key")
	}
	start := time.Now()
	tun, err := tunnel.New(tunnel.Config{
		ServerIP:   t.EntryIP,
		Port:       t.Port,
		ServerKey:  t.ServerKey,
		PrivateKey: r.opts.PrivateKey,
		SNI:        t.SNI,
		Hello:      r.opts.Hello,
		Logf:       r.opts.Logf,
	})
	if err != nil {
		return Result{}, err
	}
	defer tun.Close()
	hctx, cancel := context.WithTimeout(ctx, r.opts.L3Timeout)
	defer cancel()
	if err := tun.Start(hctx, r.opts.L3Timeout); err != nil {
		return Result{}, err
	}
	rtt := time.Since(start).Milliseconds()
	return Result{
		Server: t.Server, ServerKey: t.ServerKey, EntryIP: t.EntryIP, Port: t.Port, Level: int(LevelTunnel),
		RTTMs: rtt, SNI: tun.ServerName(), Country: t.Country, Score: t.Score, Load: t.Load,
	}, nil
}

// Wait blocks until the scan finished or ctx is done.
func (r *Runner) Wait(ctx context.Context) {
	select {
	case <-r.doneCh:
	case <-ctx.Done():
	}
}

var bypassOnce sync.Once

// targetsOf promotes L1/L2 results back into targets for the next stage.
func targetsOf(results []Result) []Target {
	out := make([]Target, 0, len(results))
	for _, res := range results {
		out = append(out, Target{
			Server: res.Server, ServerKey: res.ServerKey, EntryIP: res.EntryIP, Port: res.Port,
			Country: res.Country, Score: res.Score, Load: res.Load,
		})
	}
	return out
}

func sortByQualityR(list []Result) {
	sort.SliceStable(list, func(i, j int) bool { return list[i].RTTMs < list[j].RTTMs })
}
