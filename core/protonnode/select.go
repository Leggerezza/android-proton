// SPDX-License-Identifier: GPL-3.0-or-later
package protonnode

import (
	"context"
	"errors"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/leggerezza/android-proton/core/proton"
)

// serverRow is what ListServers returns (a trimmed LogicalServer: the raw API
// object is far too large for a UI list and carries fields the app never uses).
type serverRow struct {
	Name         string  `json:"name"`
	ExitCountry  string  `json:"exitCountry"`
	EntryCountry string  `json:"entryCountry,omitempty"`
	City         string  `json:"city,omitempty"`
	Load         int     `json:"load"`
	Score        float64 `json:"score"`
	SecureCore   bool    `json:"secureCore"`
	P2P          bool    `json:"p2p,omitempty"`
	Tor          bool    `json:"tor,omitempty"`
	Streaming    bool    `json:"streaming,omitempty"`
	EntryIP      string  `json:"entryIP,omitempty"`
	Tier         int     `json:"tier"`
}

func serverRowOf(s *proton.LogicalServer) serverRow {
	row := serverRow{
		Name:         s.Name,
		ExitCountry:  s.ExitCountry,
		EntryCountry: s.EntryCountry,
		City:         s.City,
		Load:         s.Load,
		Score:        s.Score,
		SecureCore:   s.IsSecureCore(),
		P2P:          s.IsP2P(),
		Tor:          s.IsTor(),
		Streaming:    s.IsStreaming(),
		Tier:         s.EffectiveTier(),
	}
	if ps := s.BestPhysical(); ps != nil {
		row.EntryIP = ps.EntryIP
	}
	return row
}

const (
	serverCacheFile = "servers.json"
	// The logical server list is not secret; caching it lets the node start
	// when Proton's API is unreachable (a routine event behind the GFW). The
	// 30-minute TTL from PROMPT.md 5.5 applies to *scan results*, not here.
	serverCacheMaxAge = 7 * 24 * time.Hour
	// maxCandidates caps the probe list so a fresh start stays bounded.
	maxCandidates = 120
)

type serverCache struct {
	At   time.Time              `json:"at"`
	List []proton.LogicalServer `json:"list"`
}

// servers returns the logical server list: API first, then the cached copy.
func (e *engine) servers(ctx context.Context) ([]proton.LogicalServer, error) {
	e.mu.Lock()
	c := e.client
	st := e.store
	e.mu.Unlock()
	if c == nil {
		return nil, errNotInitialized
	}
	list, err := c.Logicals(ctx)
	if err == nil && len(list) > 0 {
		e.markAuthValid()
		if st != nil {
			if werr := st.WriteJSON(serverCacheFile, &serverCache{At: time.Now(), List: list}); werr != nil {
				e.logfOrNil("servers: cannot cache list: %v", werr)
			}
		}
		e.logfOrNil("servers: %d logical servers from the API", len(list))
		return list, nil
	}
	if err == nil {
		err = errors.New("the API returned an empty server list")
	}
	if isAuthError(err) {
		e.markAuthExpired(err)
	}
	if st != nil {
		var cache serverCache
		if rerr := st.ReadJSON(serverCacheFile, &cache); rerr == nil && len(cache.List) > 0 && time.Since(cache.At) < serverCacheMaxAge {
			e.logfOrNil("servers: API unavailable (%v); using %d cached servers from %s",
				err, len(cache.List), cache.At.Format(time.RFC3339))
			return cache.List, nil
		}
	}
	return nil, err
}

// selectCandidates turns the server list into an ordered candidate list for
// one connection attempt.
func selectCandidates(servers []proton.LogicalServer, cfg *startConfig, tier int) []proton.LogicalServer {
	base := proton.Filter{
		Country:    cfg.Country,
		SecureCore: cfg.SecureCore,
		Name:       cfg.Server,
		TrustTier:  tier,
	}
	if cfg.Server != "" || cfg.Country != "" {
		return proton.Select(servers, base)
	}
	var out []proton.LogicalServer
	seen := map[string]bool{}
	for _, cc := range cfg.Prefer {
		f := base
		f.Country = cc
		part := proton.Select(servers, f)
		for i := range part {
			if seen[part[i].Name] {
				continue
			}
			seen[part[i].Name] = true
			out = append(out, part[i])
			if len(out) >= maxCandidates {
				return out
			}
		}
	}
	if len(out) == 0 {
		// Preferred countries are all unavailable: fall back to the whole list.
		return proton.Select(servers, base)
	}
	return out
}

// candidate pairs a logical server with the physical machine that will be
// dialled.
type candidate struct {
	server *proton.LogicalServer
	phys   *proton.PhysicalServer
}

// probeCandidates runs the L1 check (TCP connect to the entry IP) exactly like
// the Windows baseline: dedupe by entry IP, probe 12 at a time, keep the first
// maxProbe reachable entries. Server-side scan levels L2/L3 are P3.
func (e *engine) probeCandidates(ctx context.Context, servers []proton.LogicalServer, port int, cfg startConfig) []candidate {
	timeout := time.Duration(cfg.ProbeTimeoutMs) * time.Millisecond
	var cands []candidate
	seen := map[string]bool{}
	for i := range servers {
		s := &servers[i]
		for j := range s.Servers {
			ps := &s.Servers[j]
			if ps.Status != 1 || ps.X25519PublicKey == "" || seen[ps.EntryIP] {
				continue
			}
			seen[ps.EntryIP] = true
			cands = append(cands, candidate{server: s, phys: ps})
			if len(cands) >= maxCandidates {
				break
			}
		}
		if len(cands) >= maxCandidates {
			break
		}
	}
	if len(cands) == 0 {
		return nil
	}
	var out []candidate
	const batch = 12
	for start := 0; start < len(cands) && len(out) < cfg.MaxProbe; start += batch {
		end := start + batch
		if end > len(cands) {
			end = len(cands)
		}
		errs := make([]error, end-start)
		var wg sync.WaitGroup
		for i := start; i < end; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				errs[i-start] = probeTCP(ctx, net.JoinHostPort(cands[i].phys.EntryIP, strconv.Itoa(port)), timeout)
			}(i)
		}
		wg.Wait()
		for i := start; i < end; i++ {
			if errs[i-start] != nil {
				continue
			}
			e.logfOrNil("entry %s (%s) reachable", cands[i].phys.EntryIP, cands[i].server.Name)
			out = append(out, cands[i])
			if len(out) >= cfg.MaxProbe {
				break
			}
		}
	}
	if len(out) == 0 {
		e.logfOrNil("entry probe: nothing reachable, trying %s anyway", cands[0].server.Name)
		out = append(out, cands[0])
	}
	return out
}

func probeTCP(ctx context.Context, addr string, timeout time.Duration) error {
	d := net.Dialer{Timeout: timeout}
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	_ = c.Close()
	return nil
}

// sortByScore is kept for the L2/L3 ranking that P3 adds; proton.Select
// already sorts by Score then Load, this helper only documents the intent.
func sortByScore(list []proton.LogicalServer) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Score != list[j].Score {
			return list[i].Score < list[j].Score
		}
		return list[i].Load < list[j].Load
	})
}

var _ = strings.TrimSpace
