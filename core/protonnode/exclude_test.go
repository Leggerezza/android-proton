// SPDX-License-Identifier: GPL-3.0-or-later
package protonnode

import (
	"testing"

	"github.com/leggerezza/android-proton/core/proton"
)

func srv(name, country string) proton.LogicalServer {
	return proton.LogicalServer{
		Name:         name,
		ExitCountry:  country,
		Status:       1,
		Features:     proton.FeatureIPv6,
		EntryCountry: country,
		Servers: []proton.PhysicalServer{{
			EntryIP:         "203.0.113.1",
			X25519PublicKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		}},
	}
}

func TestExcludedCountriesAreNeverCandidates(t *testing.T) {
	servers := []proton.LogicalServer{srv("JP#1", "JP"), srv("RU#1", "RU"), srv("US#1", "US")}
	cfg := startConfig{ExcludeCountries: []string{"ru"}}
	if err := cfg.applyDefaults(); err != nil {
		t.Fatal(err)
	}
	got := selectCandidates(servers, &cfg, 0)
	for _, s := range got {
		if s.ExitCountry == "RU" {
			t.Fatalf("excluded country RU was offered as a candidate: %+v", s)
		}
	}
	if len(got) != 2 {
		t.Fatalf("want 2 candidates, got %d", len(got))
	}
}

func TestExcludedCountryBeatsAPinnedCountry(t *testing.T) {
	servers := []proton.LogicalServer{srv("RU#1", "RU"), srv("JP#1", "JP")}
	cfg := startConfig{Country: "RU", ExcludeCountries: []string{"RU"}}
	if err := cfg.applyDefaults(); err != nil {
		t.Fatal(err)
	}
	if got := selectCandidates(servers, &cfg, 0); len(got) != 0 {
		t.Fatalf("pinning an excluded country must yield nothing, got %+v", got)
	}
}

func TestTransportDefaultsAndValidation(t *testing.T) {
	cfg := startConfig{}
	if err := cfg.applyDefaults(); err != nil {
		t.Fatal(err)
	}
	if cfg.Transport != "stealth" {
		t.Fatalf("default transport = %q, want stealth", cfg.Transport)
	}
	udp := startConfig{Transport: "UDP"}
	if err := udp.applyDefaults(); err != nil {
		t.Fatal(err)
	}
	if udp.Transport != "udp" {
		t.Fatalf("transport = %q, want udp", udp.Transport)
	}
	bad := startConfig{Transport: "quic"}
	if err := bad.applyDefaults(); err == nil {
		t.Fatal("unknown transport accepted")
	}
}
