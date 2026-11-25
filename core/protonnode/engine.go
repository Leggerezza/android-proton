// SPDX-License-Identifier: GPL-3.0-or-later
package protonnode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/leggerezza/android-proton/core/scan"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/leggerezza/android-proton/core/proton"
	"github.com/leggerezza/android-proton/core/store"
)

// Node states, exactly the names PROMPT.md 5.7 uses.
const (
	stateIdle         = "idle"
	stateScanning     = "scanning"
	stateConnecting   = "connecting"
	stateConnected    = "connected"
	stateReconnecting = "reconnecting"
)

// Errors returned across the gomobile boundary.
var (
	errNotInitialized = errors.New("protonnode: Init() was not called")
	errNoSession      = errors.New("protonnode: no Proton session: call Login() or SetSession() first")
	errTOTPRequired   = errors.New("protonnode: this account requires a TOTP code")
	errAlreadyRunning = errors.New("protonnode: the tunnel is already running")
)

// engine is the single process-wide node. gomobile gives us one Go runtime per
// process and PROMPT.md 5.2 specifies one tunnel per process, so a singleton is
// the honest model (no hidden multi-instance state).
type engine struct {
	latency map[string]int64
	mu      sync.Mutex

	store *store.Store
	log   *ringLog

	client    *proton.Client
	apiIP     string
	dnsServer string

	session      sessionJSON
	sessionDirty bool
	plan         string
	tier         int

	state     string
	lastError string

	run    *runState
	runCfg startConfig

	// scan bookkeeping (P3)
	scans              *scanState
	networkFingerprint string
	lastScanSummary    string

	// Last authentication verdict: an App restart must not pretend a stored
	// session still works (the UI shows "unverified" until we learn more).
	authState string // "", "valid", "expired"
	authAt    time.Time
	authErr   string
}

var eng = &engine{
	state: stateIdle,
	log:   newRingLog(500),
}

func (e *engine) setState(s string) {
	e.mu.Lock()
	e.state = s
	e.mu.Unlock()
}

func (e *engine) stateSnapshot() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state
}

// markAuthValid records a successful authenticated call.
func (e *engine) markAuthValid() {
	e.mu.Lock()
	e.authState = "valid"
	e.authAt = time.Now()
	e.authErr = ""
	e.mu.Unlock()
}

// markAuthExpired records that Proton rejected the session (401/2001).
func (e *engine) markAuthExpired(err error) {
	msg := "session expired"
	if err != nil {
		msg = err.Error()
	}
	e.mu.Lock()
	if e.authState != "expired" {
		e.logfOrNil("auth: session rejected (%s)", msg)
	}
	e.authState = "expired"
	e.authAt = time.Now()
	e.authErr = msg
	e.mu.Unlock()
}

// resetAuthVerdict forgets the verdict (new session, logout, re-init).
func (e *engine) resetAuthVerdict() {
	e.mu.Lock()
	e.authState = ""
	e.authAt = time.Time{}
	e.authErr = ""
	e.mu.Unlock()
}

// isAuthError reports whether err means "these tokens are not accepted".
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *proton.APIError
	if errors.As(err, &apiErr) && apiErr.IsAuth() {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "401") || strings.Contains(msg, "2001")
}

func (e *engine) markSessionDirty() {
	e.mu.Lock()
	e.sessionDirty = true
	e.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// Init prepares the node: cfgDir receives only non-secret state (server cache,
// preferences). Credentials are never written here; the app holds SessionJSON
// encrypted with an Android Keystore key.
func Init(cfgDir string) error {
	st, err := store.New(cfgDir)
	if err != nil {
		return err
	}
	eng.mu.Lock()
	defer eng.mu.Unlock()
	if eng.store != nil {
		return nil // idempotent
	}
	if eng.log == nil {
		// The ring log must exist before anything logs: RecentLogs() reads it
		// and the CLI used to be the only place that created it.
		eng.log = newRingLog(500)
	}
	eng.store = st
	eng.loadPrefsLocked()
	c, err := proton.NewClient(proton.Options{
		APIIP:     eng.apiIP,
		DNSServer: eng.dnsServer,
		Timeout:   30 * time.Second,
	})
	if err != nil {
		return err
	}
	c.Logf = eng.logfOrNil
	eng.client = c
	if eng.session.Access != "" || eng.session.Refresh != "" {
		c.SetTokens(eng.session.tokens())
	}
	eng.logfOrNil("core: initialized (dir %s)", cfgDir)
	return nil
}

// prefsJSON is the non-secret configuration persisted between runs.
type prefsJSON struct {
	APIIP      string `json:"apiIP,omitempty"`
	DNSServer  string `json:"dnsServer,omitempty"`
	LastServer string `json:"lastServer,omitempty"`
}

const prefsFile = "prefs.json"

func (e *engine) loadPrefsLocked() {
	var p prefsJSON
	if err := e.store.ReadJSON(prefsFile, &p); err == nil {
		e.apiIP = p.APIIP
		e.dnsServer = p.DNSServer
		e.session.LastServer = p.LastServer
	}
}

func (e *engine) savePrefs() {
	e.mu.Lock()
	p := prefsJSON{APIIP: e.apiIP, DNSServer: e.dnsServer, LastServer: e.session.LastServer}
	st := e.store
	e.mu.Unlock()
	if st == nil {
		return
	}
	if err := st.WriteJSON(prefsFile, p); err != nil {
		e.logfOrNil("prefs: %v", err)
	}
}

// Shutdown stops the tunnel and releases everything. The process may keep
// running afterwards; Init can be called again.
func Shutdown() {
	_ = Stop()
	eng.mu.Lock()
	eng.client = nil
	eng.store = nil
	eng.mu.Unlock()
	eng.logfOrNil("core: shutdown")
}

// ---------------------------------------------------------------------------
// Session
// ---------------------------------------------------------------------------

// SetSession installs a session previously exported with SessionJSON (the app
// decrypts it with the Keystore key before calling this).
func SetSession(jsonStr string) error {
	if jsonStr == "" {
		return errors.New("protonnode: empty session")
	}
	var s sessionJSON
	if err := json.Unmarshal([]byte(jsonStr), &s); err != nil {
		return fmt.Errorf("protonnode: parse session: %w", err)
	}
	if s.Access == "" && s.Refresh == "" {
		return errors.New("protonnode: session carries no token")
	}
	eng.mu.Lock()
	defer eng.mu.Unlock()
	eng.session = s
	eng.sessionDirty = false
	// A restored session is unverified until an authenticated call says so.
	eng.authState = ""
	eng.authAt = time.Time{}
	eng.authErr = ""
	if eng.client != nil {
		eng.client.SetTokens(s.tokens())
	}
	eng.logfOrNil("session: loaded for device %s (uid %s)", s.DeviceName, maskUID(s.UID))
	return nil
}

// SessionJSON exports the current session so the caller can persist it
// encrypted. It contains the access/refresh tokens and the Ed25519 seed: never
// log it and never write it unencrypted.
func SessionJSON() (string, error) {
	eng.mu.Lock()
	jsonStr, err := eng.marshalSession()
	eng.mu.Unlock()
	if err != nil {
		return "", err
	}
	return jsonStr, nil
}

// SessionState is the log-safe summary (no tokens).
func SessionState() string {
	eng.mu.Lock()
	st := eng.sessionState()
	eng.mu.Unlock()
	buf, _ := json.Marshal(st)
	return string(buf)
}

// SetAPIDNS pins the resolver used for Proton API lookups ("172.26.26.3" or
// "172.26.26.3:53"). On Android the system resolver is not always reachable
// from a Go process, and with a local mihomo fake-ip DNS the API host would
// resolve to an unroutable 198.18.x address. An empty value restores the
// system resolver.
func SetAPIDNS(server string) error {
	eng.mu.Lock()
	eng.dnsServer = server
	eng.mu.Unlock()
	return rebuildClient()
}

// rebuildClient recreates the API client after a transport setting changed,
// carrying the current session over.
func rebuildClient() error {
	eng.mu.Lock()
	c, err := proton.NewClient(proton.Options{
		APIIP:     eng.apiIP,
		DNSServer: eng.dnsServer,
		Timeout:   30 * time.Second,
	})
	if err != nil {
		eng.mu.Unlock()
		return err
	}
	c.Logf = eng.logfOrNil
	c.SetTokens(eng.session.tokens())
	eng.client = c
	dns := eng.dnsServer
	eng.mu.Unlock()
	eng.savePrefs()
	eng.logfOrNil("api: client rebuilt (dns %q)", dns)
	return nil
}

// Login performs Proton's SRP login, optionally completes TOTP, then refreshes
// the plan information. 30s budget.
func Login(user, pass, totp string) (string, error) {
	eng.mu.Lock()
	c := eng.client
	eng.mu.Unlock()
	if c == nil {
		return "", errNotInitialized
	}
	if user == "" || pass == "" {
		return "", errors.New("protonnode: user and password are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := c.Login(ctx, user, pass)
	if err != nil {
		return "", err
	}
	eng.markAuthValid()
	eng.logfOrNil("login: SRP ok (uid %s)", maskUID(res.UID))
	if res.Needs2FA {
		if totp == "" {
			return "", errTOTPRequired
		}
		if err := c.Submit2FA(ctx, totp); err != nil {
			return "", err
		}
		eng.logfOrNil("login: TOTP accepted")
	}
	eng.mu.Lock()
	eng.session.setTokens(c.Tokens())
	eng.session.Email = user
	if eng.session.DeviceName == "" {
		eng.session.DeviceName = randomDeviceName()
	}
	eng.sessionDirty = true
	eng.mu.Unlock()

	eng.refreshAccount(15 * time.Second)
	eng.savePrefs()
	return SessionState(), nil
}

// Logout invalidates the session on Proton's side and forgets it locally.
func Logout() error {
	eng.mu.Lock()
	c := eng.client
	eng.mu.Unlock()
	if c == nil {
		return errNotInitialized
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := c.Logout(ctx)
	eng.mu.Lock()
	eng.session = sessionJSON{}
	eng.sessionDirty = true
	eng.plan, eng.tier = "", 0
	eng.authState = ""
	eng.authAt = time.Time{}
	eng.authErr = ""
	eng.mu.Unlock()
	if err != nil {
		// A network failure must not keep the user logged in locally.
		eng.logfOrNil("logout: server call failed (%v); local session cleared anyway", err)
		return nil
	}
	eng.logfOrNil("logout: done")
	return nil
}

// refreshAccount reads the plan/tier (also the cheapest way to prove the token
// still works). Failures are logged, never fatal.
func (e *engine) refreshAccount(timeout time.Duration) {
	e.mu.Lock()
	c := e.client
	e.mu.Unlock()
	if c == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	acc, err := c.Account(ctx)
	if err != nil {
		e.logfOrNil("account: %v", err)
		if isAuthError(err) {
			e.markAuthExpired(err)
		}
		return
	}
	e.markAuthValid()
	e.mu.Lock()
	e.plan = acc.VPN.PlanTitle
	e.tier = acc.VPN.MaxTier
	e.mu.Unlock()
	e.logfOrNil("account: plan %q tier %d", acc.VPN.PlanTitle, acc.VPN.MaxTier)
}

// VerifySession checks the stored session with one authenticated call and
// returns the resulting SessionState. The UI uses it only when the verdict is
// unknown — never on every "Sign in" press.
func VerifySession() (string, error) {
	eng.mu.Lock()
	c := eng.client
	eng.mu.Unlock()
	if c == nil {
		return "", errNotInitialized
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := c.Account(ctx); err != nil {
		if isAuthError(err) {
			eng.markAuthExpired(err)
			return SessionState(), nil
		}
		return SessionState(), err
	}
	eng.markAuthValid()
	eng.refreshAccount(10 * time.Second)
	return SessionState(), nil
}

// ListServers returns the selectable servers as JSON. country is an ISO code
// ("US"); secureCore filters to the two-hop servers.
func ListServers(country string, secureCore bool) (string, error) {
	eng.mu.Lock()
	c := eng.client
	eng.mu.Unlock()
	if c == nil {
		return "", errNotInitialized
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	servers, err := eng.servers(ctx)
	if err != nil {
		return "", err
	}
	f := proton.Filter{
		Country:    strings.ToUpper(strings.TrimSpace(country)),
		SecureCore: secureCore,
	}
	matches := proton.Select(servers, f)
	// Measured RTTs come from the last L3 scan: the UI shows them so the user can
	// see why an entry is slow before connecting to it.
	rtts := map[string]int64{}
	eng.mu.Lock()
	st := eng.store
	eng.mu.Unlock()
	if st != nil {
		if c := scan.LoadCache(effectiveStore(st)); c != nil {
			for _, res := range c.Results {
				if res.RTTMs <= 0 {
					continue
				}
				if prev, ok := rtts[res.EntryIP]; !ok || res.RTTMs < prev {
					rtts[res.EntryIP] = res.RTTMs
				}
			}
		}
	}
	eng.mu.Lock()
	latency := make(map[string]int64, len(eng.latency))
	for k, v := range eng.latency {
		latency[k] = v
	}
	eng.mu.Unlock()
	out := make([]serverRow, 0, len(matches))
	for i := range matches {
		row := serverRowOf(&matches[i])
		if ms, ok := rtts[row.EntryIP]; ok {
			// A verified L3 result is the better number.
			row.RTTMs = ms
			row.RTTSource = "l3"
		} else if ms, ok := latency[row.Name]; ok {
			row.RTTMs = ms
			row.RTTSource = "tcp"
		}
		out = append(out, row)
	}
	eng.logfOrNil("servers: %d of %d match (country %q secure-core %v)", len(out), len(servers), country, secureCore)
	buf, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

// MeasureLatencies runs the cheap L1 TCP probe over the server list (bounded) and
// remembers the round trip per logical server, so the list can show a number for
// entries that were never part of an L3 verification. It does not touch the
// running tunnel.
func MeasureLatencies(scope string) (int, error) {
	eng.mu.Lock()
	c := eng.client
	tier := eng.tier
	eng.mu.Unlock()
	if c == nil {
		return 0, errNotInitialized
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	servers, err := eng.servers(ctx)
	if err != nil {
		return 0, err
	}
	targets, err := buildTargets(servers, scope, tier)
	if err != nil {
		return 0, err
	}
	const maxTargets = 160
	if len(targets) > maxTargets {
		targets = targets[:maxTargets]
	}
	if len(targets) == 0 {
		return 0, errors.New("no servers to measure")
	}
	runner, err := scan.New(targets, scan.Options{
		Level:       scan.LevelTCP,
		Concurrency: 32,
		L1Timeout:   3 * time.Second,
		Logf:        eng.logfOrNil,
	})
	if err != nil {
		return 0, err
	}
	done := runner.Start(ctx)
	select {
	case <-done:
	case <-ctx.Done():
		runner.Cancel()
		<-done
	}
	results := runner.Results()
	eng.mu.Lock()
	if eng.latency == nil {
		eng.latency = map[string]int64{}
	}
	for i := range results {
		if results[i].RTTMs > 0 {
			eng.latency[results[i].Server] = results[i].RTTMs
		}
	}
	n := len(eng.latency)
	eng.mu.Unlock()
	eng.logfOrNil("latency: measured %d of %d targets (total known %d)", len(results), len(targets), n)
	return len(results), nil
}

// RecentLogs returns the newest n redacted protocol lines, one per line.
func RecentLogs(n int) string {
	eng.mu.Lock()
	log := eng.log
	eng.mu.Unlock()
	if log == nil {
		return ""
	}
	return strings.Join(log.tail(n), "\n")
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func (e *engine) logfOrNil(format string, args ...any) {
	if e.log != nil {
		e.log.logf(format, args...)
	}
}

// sleepCtx waits for d or until ctx is done; it reports whether it slept.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func pid() int { return os.Getpid() }
