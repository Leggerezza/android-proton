// Package proton speaks the Proton VPN API: token handling, server
// (logicals) discovery and WireGuard certificate provisioning.
package proton

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// VPNBase serves /vpn/v2/logicals, /vpn/v1/certificate, ...
	//
	// api.protonmail.ch is used instead of vpn-api.proton.me because mainland
	// China resets TLS connections whose SNI is vpn-api.proton.me or
	// account.proton.me, while api.protonmail.ch is reachable and serves both
	// the core and the VPN API (GET /vpn/v2 answers 401 there, not 404).
	VPNBase = "https://api.protonmail.ch"
	// CoreBase serves authentication (/auth/v4/refresh, /auth/v4, 2FA).
	CoreBase = "https://api.protonmail.ch"
	// AltVPNBase is Proton's canonical host, reachable outside the GFW.
	AltVPNBase = "https://vpn-api.proton.me"

	// DefaultAppVersion is what Proton's client sends as x-pm-appversion.
	// Proton rejects app versions below its per-app floor (API code 5003).
	DefaultAppVersion = "windows-vpn@5.1.9"
	DefaultUserAgent  = "ProtonVPN/5.1.9 (Windows; proton-stealth)"
)

// Tokens is the session material needed to call the API.
type Tokens struct {
	UID     string
	Access  string
	Refresh string
}

// Valid reports whether an access token is present.
func (t Tokens) Valid() bool { return t.Access != "" }

// APIError is a Proton API failure.
type APIError struct {
	Status  int
	Code    int
	Message string
}

func (e *APIError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("proton api: http %d, code %d: %s", e.Status, e.Code, e.Message)
	}
	return fmt.Sprintf("proton api: http %d: %s", e.Status, e.Message)
}

func (e *APIError) IsAuth() bool {
	return e.Status == 401 || e.Code == 10013 || e.Code == 2027
}

// Client is a minimal Proton API client with token refresh.
type Client struct {
	HTTP       *http.Client
	VPNBase    string
	CoreBase   string
	AppVersion string
	UserAgent  string

	mu     sync.Mutex
	tokens Tokens
	// OnTokens is called whenever the token pair is rotated (refresh) so the
	// caller can persist it.
	OnTokens func(Tokens)
	// Logf receives human readable progress messages.
	Logf func(format string, args ...any)

	refreshMu   sync.Mutex
	lastRefresh time.Time
}

// Options configures the API client.
type Options struct {
	// APIIP pins the TCP destination while the URL keeps its host name, so the
	// TLS handshake still validates api.protonmail.ch's certificate. A poisoned
	// or fake-ip DNS therefore cannot redirect the API.
	APIIP string
	// DNSServer ("1.1.1.1:53") replaces the system resolver for API lookups.
	// Android's system resolver is not always usable from a Go process (and a
	// local mihomo fake-ip DNS would hand us an unroutable address), so the
	// node may pin the DNS server that ConnectivityManager reports.
	DNSServer string
	// Timeout bounds one API call (default 30s).
	Timeout time.Duration
}

// NewClient builds a client that talks to Proton directly.
//
// There is no proxy support on purpose: on Android the app is excluded from
// FlClash's VPN and must reach the API itself (PROMPT.md 5.6, role 3).
func NewClient(opts Options) (*Client, error) {
	tr, err := Transport(opts)
	if err != nil {
		return nil, err
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		HTTP:       &http.Client{Transport: tr, Timeout: timeout},
		VPNBase:    VPNBase,
		CoreBase:   CoreBase,
		AppVersion: DefaultAppVersion,
		UserAgent:  DefaultUserAgent,
		Logf:       func(string, ...any) {},
	}, nil
}

// Transport returns the http.Transport used for the Proton API.
//
// Certificate validation stays ON (PROMPT.md 0.4): only the Stealth entry TLS
// is allowed to skip it, and that happens in internal/stealth, not here.
// HTTP/1.1 only: Proton negotiates h2, and a transport with HTTP/2 disabled
// then dies with an EOF on the first read.
func Transport(opts Options) (*http.Transport, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.ForceAttemptHTTP2 = false
	tr.Proxy = nil
	tr.MaxIdleConnsPerHost = 4
	if tr.TLSClientConfig == nil {
		tr.TLSClientConfig = &tls.Config{}
	} else {
		tr.TLSClientConfig = tr.TLSClientConfig.Clone()
	}
	tr.TLSClientConfig.NextProtos = []string{"http/1.1"}

	dialer := &net.Dialer{Timeout: 15 * time.Second}
	if opts.DNSServer != "" {
		dns := opts.DNSServer
		if _, _, err := net.SplitHostPort(dns); err != nil {
			dns = net.JoinHostPort(dns, "53")
		}
		dialer.Resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				d := net.Dialer{Timeout: 5 * time.Second}
				return d.DialContext(ctx, "udp", dns)
			},
		}
	}
	if opts.APIIP != "" || opts.DNSServer != "" {
		pin := opts.APIIP
		tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			if pin != "" && strings.HasSuffix(addr, ":443") {
				addr = net.JoinHostPort(pin, "443")
			}
			return dialer.DialContext(ctx, network, addr)
		}
	}
	return tr, nil
}

// SetTokens installs a session.
func (c *Client) SetTokens(t Tokens) {
	c.mu.Lock()
	c.tokens = t
	c.mu.Unlock()
}

// Tokens returns the current session.
func (c *Client) Tokens() Tokens {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tokens
}

func (c *Client) setTokens(t Tokens) {
	c.mu.Lock()
	c.tokens = t
	cb := c.OnTokens
	c.mu.Unlock()
	if cb != nil {
		cb(t)
	}
}

// Get performs an authenticated GET against the VPN API.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, c.VPNBase+path, nil, out, true)
}

// Post performs an authenticated POST against the VPN API.
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, c.VPNBase+path, body, out, true)
}

// CorePost performs a POST against the account API (auth endpoints).
func (c *Client) CorePost(ctx context.Context, path string, body, out any, auth bool) error {
	return c.do(ctx, http.MethodPost, c.CoreBase+path, body, out, auth)
}

// CoreGet performs a GET against the account API.
func (c *Client) CoreGet(ctx context.Context, path string, out any, auth bool) error {
	return c.do(ctx, http.MethodGet, c.CoreBase+path, nil, out, auth)
}

func (c *Client) do(ctx context.Context, method, url string, body, out any, auth bool) error {
	// The GFW resets Proton's front door in bursts (a request that worked a
	// minute ago can fail with EOF), so transport errors and 5xx are retried
	// with backoff before giving up.
	var err error
	backoff := 2 * time.Second
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}
		err = c.doOnce(ctx, method, url, body, out, auth)
		if err == nil {
			return nil
		}
		apiErr, ok := err.(*APIError)
		if !ok {
			// transport error (EOF, reset, timeout): retry
			c.Logf("retry %d after transport error: %v", attempt+1, err)
			continue
		}
		if apiErr.Status == 401 && auth {
			if rerr := c.Refresh(ctx); rerr != nil {
				return fmt.Errorf("%w (token refresh failed: %v)", err, rerr)
			}
			continue
		}
		if apiErr.Status >= 500 {
			c.Logf("retry %d after http %d", attempt+1, apiErr.Status)
			continue
		}
		return err
	}
	return err
}

func (c *Client) doOnce(ctx context.Context, method, url string, body, out any, auth bool) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-pm-appversion", c.AppVersion)
	req.Header.Set("User-Agent", c.UserAgent)
	if auth {
		t := c.Tokens()
		if t.Access != "" {
			req.Header.Set("Authorization", "Bearer "+t.Access)
		}
		if t.UID != "" {
			req.Header.Set("x-pm-uid", t.UID)
		}
	}

	c.Logf("%s %s", method, url)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var apiErr APIError
		_ = json.Unmarshal(payload, &apiErr)
		apiErr.Status = resp.StatusCode
		if apiErr.Message == "" {
			apiErr.Message = strings.TrimSpace(string(payload))
			if len(apiErr.Message) > 300 {
				apiErr.Message = apiErr.Message[:300]
			}
		}
		return &apiErr
	}
	if out != nil {
		if err := json.Unmarshal(payload, out); err != nil {
			return fmt.Errorf("decode %s: %w", url, err)
		}
	}
	return nil
}

// Refresh rotates the access token. Proton refresh tokens are single-use, so
// this is serialised and rate limited per client.
func (c *Client) Refresh(ctx context.Context) error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	if !c.lastRefresh.IsZero() && time.Since(c.lastRefresh) < 10*time.Second {
		return fmt.Errorf("refusing to refresh twice within 10s")
	}
	t := c.Tokens()
	if t.Refresh == "" {
		return fmt.Errorf("no refresh token available")
	}

	reqBody := map[string]string{
		"ResponseType": "token",
		"GrantType":    "refresh_token",
		"RefreshToken": t.Refresh,
		"RedirectURI":  "https://protonmail.com",
	}
	var out struct {
		Code         int    `json:"Code"`
		AccessToken  string `json:"AccessToken"`
		RefreshToken string `json:"RefreshToken"`
		UID          string `json:"UID"`
		Scope        string `json:"Scope"`
	}

	err := c.doOnce(ctx, http.MethodPost, c.CoreBase+"/auth/v4/refresh", reqBody, &out, false)
	if err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.Status == 404 {
			// Older deployments expose refresh on the VPN host instead.
			err = c.doOnce(ctx, http.MethodPost, c.VPNBase+"/auth/refresh", reqBody, &out, false)
		}
	}
	if err != nil {
		return err
	}
	if out.AccessToken == "" {
		return fmt.Errorf("refresh returned no access token")
	}
	c.lastRefresh = time.Now()
	nt := Tokens{UID: out.UID, Access: out.AccessToken, Refresh: out.RefreshToken}
	if nt.UID == "" {
		nt.UID = t.UID
	}
	if nt.Refresh == "" {
		nt.Refresh = t.Refresh
	}
	c.setTokens(nt)
	return nil
}
