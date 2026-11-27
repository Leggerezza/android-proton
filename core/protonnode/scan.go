// SPDX-License-Identifier: GPL-3.0-or-later
package protonnode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/leggerezza/android-proton/core/proton"
	"github.com/leggerezza/android-proton/core/scan"
	"github.com/leggerezza/android-proton/core/stealth"
)

// Scan orchestration for the gomobile contract (PROMPT.md 5.1 + 5.5):
//
//	StartScan(scope, concurrency)  non-blocking, returns a scan id
//	ScanProgress(id)               JSON, cheap to poll
//	ScanResults(id)                JSON, the verification results
//	CancelScan(id)                 bool, takes effect well inside 1s
//	LastScanSummary()              JSON, the most recent finished scan
//
// scope accepts "all", a country code ("US"), "game" (JP,HK,KR,SG,US),
// "secure-core", or "name:<logical server>"; append "@1"/"@2" for a shallower
// level (default is the L3 tunnel handshake).
type scanRun struct {
	runner      *scan.Runner
	fingerprint string
	done        <-chan struct{}
}

type scanState struct {
	mu    sync.Mutex
	scans map[string]*scanRun
}

func (e *engine) scanBook() *scanState {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.scans == nil {
		e.scans = &scanState{scans: map[string]*scanRun{}}
	}
	return e.scans
}

// SetNetworkFingerprint tells the core which network it is on so cached scan
// results are only reused on the same network (PROMPT.md 5.5). The app
// computes it from ConnectivityManager and calls this on every change; an
// empty value means "unknown".
func SetNetworkFingerprint(fp string) {
	eng.mu.Lock()
	eng.networkFingerprint = fp
	eng.mu.Unlock()
	eng.logfOrNil("scan: network fingerprint %q", fp)
}

func (e *engine) fingerprint() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.networkFingerprint
}

// closedDone is an already-closed channel, used by cache-hit scans.
func closedDone() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

// StartScan starts a background scan and returns its id immediately.
func StartScan(scope string, concurrency int) (string, error) {
	eng.mu.Lock()
	c := eng.client
	session := eng.session
	tier := eng.tier
	eng.mu.Unlock()
	if c == nil {
		return "", errNotInitialized
	}
	kp, err := session.keyPair()
	if err != nil {
		return "", err
	}
	level, err := scanLevelOf(scope)
	if err != nil {
		return "", err
	}
	// A fresh cache for this network short-circuits the scan (PROMPT.md 5.5).
	if level >= scan.LevelTunnel {
		if cached := eng.cachedScanCandidates(64); len(cached) > 0 {
			fp := eng.fingerprint()
			runner := scan.FromCache(cached, scan.LevelTunnel)
			book := eng.scanBook()
			book.mu.Lock()
			book.scans[runner.ID()] = &scanRun{runner: runner, fingerprint: fp, done: closedDone()}
			book.mu.Unlock()
			eng.mu.Lock()
			eng.lastScanSummary = runner.ResultsJSON(fp, true)
			eng.mu.Unlock()
			eng.logfOrNil("scan %s: cache hit (%d entries, ttl %s, network %q)", runner.ID(), len(cached), scan.CacheTTL, fp)
			return runner.ID(), nil
		}
	}
	base := strings.TrimSpace(scope)
	if i := strings.LastIndex(base, "@"); i >= 0 {
		base = base[:i]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	servers, serr := eng.servers(ctx)
	cancel()
	if serr != nil {
		return "", serr
	}
	targets, err := buildTargets(servers, base, tier)
	if err != nil {
		return "", err
	}
	if len(targets) == 0 {
		return "", fmt.Errorf("scan: scope %q matched no server", scope)
	}
	hello, err := stealth.HelloFromName("chrome")
	if err != nil {
		return "", err
	}
	runner, err := scan.New(targets, scan.Options{
		Level:       level,
		Concurrency: concurrency,
		PrivateKey:  kp.X25519PrivateKey(),
		Hello:       hello,
		Logf:        eng.logfOrNil,
	})
	if err != nil {
		return "", err
	}
	fp := eng.fingerprint()
	done := runner.Start(context.Background())
	book := eng.scanBook()
	book.mu.Lock()
	book.scans[runner.ID()] = &scanRun{runner: runner, fingerprint: fp, done: done}
	book.mu.Unlock()

	go func() {
		<-done
		eng.persistScan(runner.ID())
	}()
	eng.logfOrNil("scan %s: started (scope %q, level %d, %d targets, concurrency %d)",
		runner.ID(), scope, level, len(targets), concurrency)
	return runner.ID(), nil
}

// persistScan caches a finished scan and remembers it as the last one.
func (e *engine) persistScan(id string) {
	book := eng.scanBook()
	book.mu.Lock()
	run, ok := book.scans[id]
	book.mu.Unlock()
	if !ok || run.runner.State() != scan.StateDone {
		return
	}
	results := run.runner.Results()
	if len(results) == 0 {
		return
	}
	e.mu.Lock()
	st := e.store
	e.mu.Unlock()
	cache := &scan.Cache{
		Fingerprint: run.fingerprint,
		Level:       int(scan.LevelTunnel),
		At:          time.Now(),
		Results:     results,
	}
	if err := scan.SaveCache(effectiveStore(st), cache); err != nil {
		e.logfOrNil("scan %s: cannot cache results: %v", id, err)
	}
	summary := run.runner.ResultsJSON(run.fingerprint, false)
	e.mu.Lock()
	e.lastScanSummary = summary
	e.mu.Unlock()
	e.logfOrNil("scan %s: done, %d verified entries cached for %s", id, len(results), scan.CacheTTL)
}

// ScanProgress returns the progress JSON of one scan.
func ScanProgress(scanID string) string {
	book := eng.scanBook()
	book.mu.Lock()
	run, ok := book.scans[scanID]
	book.mu.Unlock()
	if !ok {
		return fmt.Sprintf("{\"scanId\":%q,\"state\":\"unknown\",\"level\":3,\"stage\":\"\",\"total\":0,\"done\":0,\"passed\":0,\"startedAt\":%q,\"elapsedMs\":0}",
			scanID, time.Now().UTC().Format(time.RFC3339))
	}
	return run.runner.ProgressJSON()
}

// ScanResults returns the result JSON of one scan.
func ScanResults(scanID string) string {
	book := eng.scanBook()
	book.mu.Lock()
	run, ok := book.scans[scanID]
	book.mu.Unlock()
	if !ok {
		return fmt.Sprintf("{\"scanId\":%q,\"state\":\"unknown\",\"level\":3,\"stage\":\"\",\"fingerprint\":\"\",\"generatedAt\":%q,\"elapsedMs\":0,\"checked\":0,\"passed\":0,\"results\":[],\"cacheHit\":false}",
			scanID, time.Now().UTC().Format(time.RFC3339))
	}
	return run.runner.ResultsJSON(run.fingerprint, false)
}

// CancelScan stops a running scan. It waits up to 1s so a true return really
// means "the scan stopped" (PROMPT.md 11).
func CancelScan(scanID string) bool {
	book := eng.scanBook()
	book.mu.Lock()
	run, ok := book.scans[scanID]
	book.mu.Unlock()
	if !ok {
		return false
	}
	if !run.runner.Cancel() {
		return false
	}
	select {
	case <-run.done:
	case <-time.After(time.Second):
	}
	return true
}

// LastScanSummary returns the most recently finished scan, or "{}".
func LastScanSummary() string {
	eng.mu.Lock()
	defer eng.mu.Unlock()
	if eng.lastScanSummary == "" {
		return "{}"
	}
	return eng.lastScanSummary
}

// buildTargets turns servers into scan targets for a scope string.
func buildTargets(servers []proton.LogicalServer, scope string, tier int) ([]scan.Target, error) {
	scope = strings.TrimSpace(strings.ToLower(scope))
	f := proton.Filter{TrustTier: tier}
	var prefer []string
	switch {
	case scope == "", scope == "all":
	case scope == "game":
		prefer = []string{"JP", "HK", "KR", "SG", "US"}
	case scope == "secure-core":
		f.SecureCore = true
	case strings.HasPrefix(scope, "name:"):
		f.Name = strings.TrimSpace(scope[len("name:"):])
	case len(scope) == 2:
		f.Country = strings.ToUpper(scope)
	default:
		return nil, fmt.Errorf("scan: unknown scope %q", scope)
	}
	var list []proton.LogicalServer
	if len(prefer) > 0 {
		for _, cc := range prefer {
			fc := f
			fc.Country = cc
			list = append(list, proton.Select(servers, fc)...)
		}
	} else {
		list = proton.Select(servers, f)
	}
	var targets []scan.Target
	seen := map[string]bool{}
	for i := range list {
		s := &list[i]
		ps := s.BestPhysical()
		if ps == nil || ps.X25519PublicKey == "" || seen[ps.EntryIP] {
			continue
		}
		seen[ps.EntryIP] = true
		targets = append(targets, scan.Target{
			Server:    s.Name,
			EntryIP:   ps.EntryIP,
			Port:      443,
			ServerKey: ps.X25519PublicKey,
			Country:   s.ExitCountry,
			Score:     s.Score,
			Load:      s.Load,
		})
	}
	return targets, nil
}

// scanLevelOf reads an optional "@<level>" suffix; the default is L3.
func scanLevelOf(scope string) (scan.Level, error) {
	if i := strings.LastIndex(scope, "@"); i >= 0 {
		return scan.ParseLevel(scope[i+1:])
	}
	return scan.LevelTunnel, nil
}

// scanStore adapts *store.Store for the scan cache.
type jsonStore interface {
	ReadJSON(name string, v any) error
	WriteJSON(name string, v any) error
}

type scanStore struct{ s jsonStore }

func (a scanStore) ReadJSON(name string, v any) error  { return a.s.ReadJSON(name, v) }
func (a scanStore) WriteJSON(name string, v any) error { return a.s.WriteJSON(name, v) }

func effectiveStore(s jsonStore) scan.Store {
	if s == nil {
		return nil
	}
	return scanStore{s: s}
}

// pickCandidates chooses the entries to dial. With scanLevel 3 (the default)
// only a real WireGuard handshake counts: cached L3 results are used when they
// are fresh for this network, otherwise a scan runs inline. scanLevel < 3 keeps
// the fast L1 TCP probe.
func (e *engine) pickCandidates(ctx context.Context, servers []proton.LogicalServer, candidates []proton.LogicalServer, cfg startConfig, port int) ([]candidate, error) {
	// A manual server/country choice must never be silently replaced by the
	// cache: the user asked for that exact server, so verify it fresh.
	pinned := cfg.Server != "" || cfg.Country != ""
	if cfg.ScanLevel >= 3 {
		if !pinned {
			if cached := e.cachedScanCandidates(cfg.MaxProbe); len(cached) > 0 {
				if picks := candidatesFromResults(servers, cached); len(picks) > 0 {
					e.logfOrNil("scan: %d L3-verified entries reused from the cache", len(picks))
					return picks, nil
				}
			}
		}
		// Speed beats optimality for the first connection: pick the quickest
		// entry that answers on TCP below the "good enough" threshold and try a
		// real handshake with it, instead of waiting for the full L2/L3 ladder.
		if picks, ferr := e.fastStart(ctx, candidates, cfg); ferr == nil && len(picks) > 0 {
			return picks, nil
		} else if ferr != nil {
			e.logfOrNil("fast start: %v (falling back to the full scan)", ferr)
		}
		picks, err := e.inlineScan(ctx, candidates, cfg)
		if err == nil && len(picks) > 0 {
			return picks, nil
		}
		// No L1 fallback at scan level 3: dialling an entry that failed TLS or
		// the WireGuard handshake only produces an endless retry loop.
		e.logfOrNil("scan: L3 verification found nothing usable (%v)", err)
		if cfg.Server != "" {
			return nil, fmt.Errorf("pinned server %q did not pass L3 verification: %w", cfg.Server, err)
		}
		return nil, fmt.Errorf("no entry passed L3 verification: %w", err)
	}
	return e.probeCandidates(ctx, candidates, port, cfg), nil
}

// inlineScan runs a blocking L3 scan over the candidates and returns the
// verified ones (bounded, so a cold connect stays reasonable).
func (e *engine) inlineScan(ctx context.Context, candidates []proton.LogicalServer, cfg startConfig) ([]candidate, error) {
	// A cheap TCP pre-pass orders the pool by measured RTT before the expensive
	// TLS + WireGuard ladder runs. Proton's Score is a load figure, not a
	// latency one, so without this a user in Asia can end up on a US entry
	// (1.2 s+ per request) simply because those servers scored better.
	const (
		maxPool    = 200
		maxTargets = 24
	)
	pool := make([]scan.Target, 0, maxPool)
	byEntry := map[string]proton.LogicalServer{}
	// In automatic mode the candidate list is ordered by Proton's load score, so
	// taking the head of it concentrates the pool in a few (often distant)
	// countries. Cap the number of entries per exit country instead, so the RTT
	// pre-pass gets to see the nearby ones too.
	pinned := cfg.Country != "" || cfg.Server != ""
	const perCountry = 4
	perCountryCount := map[string]int{}
	for i := range candidates {
		s := &candidates[i]
		ps := s.BestPhysical()
		if ps == nil || ps.X25519PublicKey == "" {
			continue
		}
		if _, dup := byEntry[ps.EntryIP]; dup {
			continue
		}
		if !pinned {
			if perCountryCount[s.ExitCountry] >= perCountry {
				continue
			}
			perCountryCount[s.ExitCountry]++
		}
		byEntry[ps.EntryIP] = *s
		pool = append(pool, scan.Target{
			Server: s.Name, EntryIP: ps.EntryIP, Port: 443, ServerKey: ps.X25519PublicKey,
			Country: s.ExitCountry, Score: s.Score, Load: s.Load,
		})
		if len(pool) >= maxPool {
			break
		}
	}
	if len(pool) == 0 {
		return nil, errors.New("scan: no target with a WireGuard key")
	}

	targets := pool
	if len(pool) > maxTargets {
		fastest, err := e.fastestByRTT(ctx, pool, maxTargets)
		if err != nil {
			e.logfOrNil("scan: RTT pre-pass failed (%v), keeping the pool order", err)
		} else {
			targets = fastest
			e.logfOrNil("scan: RTT pre-pass kept %d of %d entries (%s)", len(targets), len(pool), rttSummary(targets))
		}
	}
	e.mu.Lock()
	kp, kerr := e.session.keyPair()
	fp := e.networkFingerprint
	st := e.store
	e.mu.Unlock()
	if kerr != nil {
		return nil, kerr
	}
	hello, err := stealth.HelloFromName("chrome")
	if err != nil {
		return nil, err
	}
	runner, err := scan.New(targets, scan.Options{
		Level:       scan.LevelTunnel,
		Concurrency: 32,
		PrivateKey:  kp.X25519PrivateKey(),
		Hello:       hello,
		Logf:        e.logfOrNil,
	})
	if err != nil {
		return nil, err
	}
	done := runner.Start(ctx)
	select {
	case <-done:
	case <-ctx.Done():
		runner.Cancel()
		<-done
		return nil, ctx.Err()
	}
	results := runner.Results()
	if len(results) == 0 {
		return nil, errors.New("scan: no entry completed a WireGuard handshake")
	}
	if err := scan.SaveCache(effectiveStore(st), &scan.Cache{
		Fingerprint: fp,
		Level:       int(scan.LevelTunnel),
		At:          time.Now(),
		Results:     results,
	}); err != nil {
		e.logfOrNil("scan: cannot cache results: %v", err)
	}
	e.mu.Lock()
	e.lastScanSummary = runner.ResultsJSON(fp, false)
	e.mu.Unlock()
	summary, _ := json.Marshal(results)
	e.logfOrNil("scan: L3 verified %s", string(summary))
	return candidatesFromResults(nil, results), nil
}

// fastestByRTT probes the pool with plain TCP connects and returns the
// quickest entries, keeping the scan's own ordering when timings tie.
func (e *engine) fastestByRTT(ctx context.Context, pool []scan.Target, limit int) ([]scan.Target, error) {
	runner, err := scan.New(pool, scan.Options{
		Level:       scan.LevelTCP,
		Concurrency: 48,
		L1Timeout:   3 * time.Second,
		Logf:        e.logfOrNil,
	})
	if err != nil {
		return nil, err
	}
	done := runner.Start(ctx)
	select {
	case <-done:
	case <-ctx.Done():
		runner.Cancel()
		<-done
		return nil, ctx.Err()
	}
	results := runner.Results()
	if len(results) == 0 {
		return nil, errors.New("scan: no entry answered on TCP")
	}
	byEntry := map[string]scan.Target{}
	for _, t := range pool {
		byEntry[t.EntryIP] = t
	}
	out := make([]scan.Target, 0, limit)
	for _, res := range results {
		if t, ok := byEntry[res.EntryIP]; ok {
			out = append(out, t)
			if len(out) >= limit {
				break
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("scan: RTT results did not map back to targets")
	}
	return out, nil
}

// rttSummary renders the first few entries with their measured RTT for the log.
func rttSummary(targets []scan.Target) string {
	parts := make([]string, 0, len(targets))
	for _, t := range targets {
		parts = append(parts, t.Server+" "+t.EntryIP)
	}
	const max = 5
	if len(parts) > max {
		parts = append(parts[:max], "...")
	}
	return strings.Join(parts, ", ")
}

// candidatesFromResults maps verified entries back onto the server list (when
// it is available) or synthesises candidates from the result itself.
func candidatesFromResults(servers []proton.LogicalServer, results []scan.Result) []candidate {
	out := make([]candidate, 0, len(results))
	for _, res := range results {
		entryIP := res.EntryIP
		port := res.Port
		if port == 0 {
			port = 443
		}
		phys := &proton.PhysicalServer{EntryIP: entryIP, Status: 1, X25519PublicKey: res.ServerKey}
		var srv *proton.LogicalServer
		if len(servers) > 0 {
			for i := range servers {
				if servers[i].Name == res.Server {
					srv = &servers[i]
					break
				}
			}
		}
		if srv == nil {
			srv = &proton.LogicalServer{Name: res.Server, ExitCountry: res.Country, Status: 1, Score: res.Score, Load: res.Load}
		}
		if len(srv.Servers) == 0 {
			srv.Servers = []proton.PhysicalServer{*phys}
		}
		out = append(out, candidate{server: srv, phys: phys})
	}
	return out
}

// cachedScanCandidates returns up to n cached L3-verified targets for this
// network, or nil when the cache is missing, stale or from another network.
func (e *engine) cachedScanCandidates(n int) []scan.Result {
	e.mu.Lock()
	st := e.store
	fp := e.networkFingerprint
	e.mu.Unlock()
	cache := scan.LoadCache(effectiveStore(st))
	if !cache.Valid(fp, scan.LevelTunnel, time.Now()) {
		return nil
	}
	if len(cache.Results) > n {
		return cache.Results[:n]
	}
	return cache.Results
}

var _ = errors.New

// fastStartThreshold is "acceptable for now": a TCP round trip below this is
// worth connecting to immediately while the full scan runs afterwards.
const fastStartThreshold = 700 * time.Millisecond

// fastStart probes the pool with plain TCP and returns the quickest entries that
// answer within fastStartThreshold. It never runs L2/L3 itself: the caller's
// handshake is the verification.
func (e *engine) fastStart(ctx context.Context, candidates []proton.LogicalServer, cfg startConfig) ([]candidate, error) {
	const maxPool = 200
	pool := make([]scan.Target, 0, maxPool)
	byEntry := map[string]proton.LogicalServer{}
	for i := range candidates {
		s := &candidates[i]
		ps := s.BestPhysical()
		if ps == nil || ps.X25519PublicKey == "" {
			continue
		}
		if _, dup := byEntry[ps.EntryIP]; dup {
			continue
		}
		byEntry[ps.EntryIP] = *s
		pool = append(pool, scan.Target{
			Server: s.Name, EntryIP: ps.EntryIP, Port: 443, ServerKey: ps.X25519PublicKey,
			Country: s.ExitCountry, Score: s.Score, Load: s.Load,
		})
		if len(pool) >= maxPool {
			break
		}
	}
	if len(pool) == 0 {
		return nil, errors.New("fast start: no target with a WireGuard key")
	}
	runner, err := scan.New(pool, scan.Options{
		Level:       scan.LevelTCP,
		Concurrency: 32,
		L1Timeout:   3 * time.Second,
		Logf:        e.logfOrNil,
	})
	if err != nil {
		return nil, err
	}
	done := runner.Start(ctx)
	select {
	case <-done:
	case <-ctx.Done():
		runner.Cancel()
		<-done
		return nil, ctx.Err()
	}
	results := runner.Results()
	quick := make([]scan.Result, 0, 3)
	for _, res := range results {
		if res.RTTMs > 0 && time.Duration(res.RTTMs)*time.Millisecond <= fastStartThreshold {
			quick = append(quick, res)
			if len(quick) >= 3 {
				break
			}
		}
	}
	if len(quick) == 0 {
		return nil, fmt.Errorf("fast start: no entry answered under %s", fastStartThreshold)
	}
	e.logfOrNil("fast start: %d candidates under %s, first %s (%d ms)",
		len(quick), fastStartThreshold, quick[0].Server, quick[0].RTTMs)
	return candidatesFromResults(nil, quick), nil
}
