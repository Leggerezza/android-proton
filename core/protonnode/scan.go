package protonnode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dsh/android-proton/core/proton"
	"github.com/dsh/android-proton/core/scan"
	"github.com/dsh/android-proton/core/stealth"
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
	const maxTargets = 48
	targets := make([]scan.Target, 0, maxTargets)
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
		targets = append(targets, scan.Target{
			Server: s.Name, EntryIP: ps.EntryIP, Port: 443, ServerKey: ps.X25519PublicKey,
			Country: s.ExitCountry, Score: s.Score, Load: s.Load,
		})
		if len(targets) >= maxTargets {
			break
		}
	}
	if len(targets) == 0 {
		return nil, errors.New("scan: no target with a WireGuard key")
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
