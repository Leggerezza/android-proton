package proton

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Feature bits used by Proton's logical servers.
const (
	FeatureSecureCore = 1
	FeatureTor        = 2
	FeatureP2P        = 4
	FeatureStreaming  = 8
	FeatureIPv6       = 16
)

// Tiers.
const (
	TierFree      = 0
	TierBasic     = 1
	TierPlus      = 2
	TierVisionary = 3
)

// PhysicalServer is one machine behind a logical server.
type PhysicalServer struct {
	ID              string `json:"ID"`
	EntryIP         string `json:"EntryIP"`
	ExitIP          string `json:"ExitIP"`
	Domain          string `json:"Domain"`
	Status          int    `json:"Status"`
	Generation      int    `json:"Generation"`
	Label           string `json:"Label"`
	X25519PublicKey string `json:"X25519PublicKey"`
}

// LogicalServer is what the user selects ("CH#10", "US-GA#3", ...).
type LogicalServer struct {
	ID           string           `json:"ID"`
	Name         string           `json:"Name"`
	EntryCountry string           `json:"EntryCountry"`
	ExitCountry  string           `json:"ExitCountry"`
	Domain       string           `json:"Domain"`
	Tier         int              `json:"Tier"`
	Features     int              `json:"Features"`
	City         string           `json:"City"`
	Score        float64          `json:"Score"`
	Load         int              `json:"Load"`
	Status       int              `json:"Status"`
	Servers      []PhysicalServer `json:"Servers"`
}

// HasFeature reports whether a feature bit is set.
func (s LogicalServer) HasFeature(f int) bool { return s.Features&f != 0 }

// IsOnline reports whether the logical server is usable.
func (s LogicalServer) IsOnline() bool { return s.Status == 1 }

// IsSecureCore reports whether traffic is routed through a Secure Core hop.
func (s LogicalServer) IsSecureCore() bool { return s.HasFeature(FeatureSecureCore) }

// IsP2P reports whether the server allows P2P.
func (s LogicalServer) IsP2P() bool { return s.HasFeature(FeatureP2P) }

// IsTor reports whether the server is a Tor exit.
func (s LogicalServer) IsTor() bool { return s.HasFeature(FeatureTor) }

// IsStreaming reports whether the server is a streaming server.
func (s LogicalServer) IsStreaming() bool { return s.HasFeature(FeatureStreaming) }

// EffectiveTier treats the legacy "basic" tier as Plus.
func (s LogicalServer) EffectiveTier() int {
	if s.Tier == TierBasic {
		return TierPlus
	}
	return s.Tier
}

// BestPhysical returns the first online machine, preferring one that has a
// WireGuard public key (needed for the Stealth tunnel).
func (s LogicalServer) BestPhysical() *PhysicalServer {
	for i := range s.Servers {
		if s.Servers[i].Status == 1 && s.Servers[i].X25519PublicKey != "" {
			return &s.Servers[i]
		}
	}
	for i := range s.Servers {
		if s.Servers[i].Status == 1 {
			return &s.Servers[i]
		}
	}
	return nil
}

type logicalsResponse struct {
	Code           int             `json:"Code"`
	LogicalServers []LogicalServer `json:"LogicalServers"`
	Total          int             `json:"Total"`
}

// Logicals fetches the full server list, including Secure Core.
//
// v1 is the primary source on purpose: it carries Load and Score, which is
// what Proton's own client ranks servers by. v2 dropped both (it only has
// StatusReference), so ranking by score/load is impossible there.
func (c *Client) Logicals(ctx context.Context) ([]LogicalServer, error) {
	var resp logicalsResponse
	err := c.Get(ctx, "/vpn/v1/logicals?SecureCoreFilter=all&WithState=true", &resp)
	if err != nil || len(resp.LogicalServers) == 0 {
		var v2 logicalsResponse
		if err2 := c.Get(ctx, "/vpn/v2/logicals?SecureCoreFilter=all", &v2); err2 != nil {
			if err != nil {
				return nil, err
			}
			return nil, err2
		}
		resp = v2
	}
	if len(resp.LogicalServers) == 0 {
		return nil, fmt.Errorf("proton api returned an empty server list")
	}
	return resp.LogicalServers, nil
}

// LogicalForProtocol discovers which TCP port Proton wants for Stealth.
func (c *Client) StealthPort(ctx context.Context) int {
	var cfg struct {
		DefaultPorts struct {
			WireGuard struct {
				TCP []int `json:"TCP"`
			} `json:"WireGuard"`
		} `json:"DefaultPorts"`
	}
	if err := c.Get(ctx, "/vpn/v2/clientconfig", &cfg); err != nil {
		return 443
	}
	if len(cfg.DefaultPorts.WireGuard.TCP) > 0 && cfg.DefaultPorts.WireGuard.TCP[0] > 0 {
		return cfg.DefaultPorts.WireGuard.TCP[0]
	}
	return 443
}

// Filter narrows a server list. The zero value matches every server.
type Filter struct {
	Country          string // ISO country code of the exit country
	EntryCountry     string
	Name             string // exact logical server name, e.g. "CH#10"
	SecureCore       bool
	RequireP2P       bool
	RequireTor       bool
	RequireStreaming bool
	TrustTier        int // highest tier the account can use (0 = no limit)
	OnlineOnly       bool
	Search           string
}

func (f Filter) Match(s LogicalServer) bool {
	if f.TrustTier > 0 && s.EffectiveTier() > f.TrustTier {
		return false
	}
	if f.OnlineOnly && !s.IsOnline() {
		return false
	}
	if f.Name != "" && !strings.EqualFold(s.Name, f.Name) {
		return false
	}
	if f.Country != "" && !strings.EqualFold(s.ExitCountry, f.Country) {
		return false
	}
	if f.EntryCountry != "" && !strings.EqualFold(s.EntryCountry, f.EntryCountry) {
		return false
	}
	if f.SecureCore && !s.IsSecureCore() {
		return false
	}
	if f.RequireP2P && !s.IsP2P() {
		return false
	}
	if f.RequireTor && !s.IsTor() {
		return false
	}
	if f.RequireStreaming && !s.IsStreaming() {
		return false
	}
	if f.Search != "" {
		q := strings.ToLower(f.Search)
		if !strings.Contains(strings.ToLower(s.Name), q) &&
			!strings.Contains(strings.ToLower(s.City), q) &&
			!strings.Contains(strings.ToLower(s.ExitCountry), q) &&
			!strings.Contains(strings.ToLower(s.Domain), q) {
			return false
		}
	}
	return true
}

// Select returns the matching servers, best first (Proton's own score, then
// load).
func Select(servers []LogicalServer, f Filter) []LogicalServer {
	out := make([]LogicalServer, 0, len(servers))
	for _, s := range servers {
		if f.Match(s) {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score < out[j].Score
		}
		return out[i].Load < out[j].Load
	})
	return out
}

// PickServer resolves a filter to exactly one server, trying in order:
// exact name, then best by score/load.
func PickServer(servers []LogicalServer, f Filter) (*LogicalServer, error) {
	f.OnlineOnly = true
	matches := Select(servers, f)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no server matches %s", describeFilter(f))
	}
	best := matches[0]
	return &best, nil
}

func describeFilter(f Filter) string {
	var parts []string
	if f.Name != "" {
		parts = append(parts, "name="+f.Name)
	}
	if f.Country != "" {
		parts = append(parts, "country="+f.Country)
	}
	if f.EntryCountry != "" {
		parts = append(parts, "entry="+f.EntryCountry)
	}
	if f.SecureCore {
		parts = append(parts, "secure-core")
	}
	if f.RequireP2P {
		parts = append(parts, "p2p")
	}
	if f.Search != "" {
		parts = append(parts, "search="+f.Search)
	}
	if len(parts) == 0 {
		return "the given filter (fastest server)"
	}
	return strings.Join(parts, ", ")
}

// CountryCodes lists exit countries present in the server list.
func CountryCodes(servers []LogicalServer) []string {
	seen := map[string]struct{}{}
	for _, s := range servers {
		seen[s.ExitCountry] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
