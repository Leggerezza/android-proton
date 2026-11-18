package protonnode

import "testing"

func TestGameModeDefaults(t *testing.T) {
	cfg := startConfig{Mode: "game"}
	if err := cfg.applyDefaults(); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Prefer) != 5 || cfg.Prefer[0] != "JP" {
		t.Fatalf("game mode prefer = %v, want JP,HK,KR,SG,US", cfg.Prefer)
	}
	if cfg.LockServer == nil || !*cfg.LockServer {
		t.Fatal("game mode must lock the server by default")
	}
	if cfg.SocksPort != 1080 || cfg.MTU != 1320 || cfg.Fingerprint != "chrome" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestGameModeRejectsSecureCore(t *testing.T) {
	cfg := startConfig{Mode: "game", SecureCore: true}
	if err := cfg.applyDefaults(); err == nil {
		t.Fatal("game mode accepted Secure Core")
	}
}

func TestNormalModeDefaultsAndValidation(t *testing.T) {
	cfg := startConfig{}
	if err := cfg.applyDefaults(); err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != "normal" || len(cfg.Prefer) != 3 || cfg.Prefer[0] != "US" {
		t.Fatalf("normal defaults: %+v", cfg)
	}
	if cfg.LockServer != nil {
		t.Fatal("normal mode should not force the server lock")
	}
	bad := startConfig{SocksPort: 80}
	if err := bad.applyDefaults(); err == nil {
		t.Fatal("port 80 accepted")
	}
	bad = startConfig{Fingerprint: "nonsense"}
	if err := bad.applyDefaults(); err == nil {
		t.Fatal("unknown fingerprint accepted")
	}
}

func TestSelectCandidatesPrefersOrderAndSkipsUnavailable(t *testing.T) {
	servers := []protoServerFixture{
		{name: "JP#1", cc: "JP"},
		{name: "HK#1", cc: "HK"},
		{name: "US#1", cc: "US"},
		{name: "DE#1", cc: "DE"},
	}
	list := make([]logicalServer, 0, len(servers))
	for _, s := range servers {
		list = append(list, s.toLogical())
	}
	cfg := startConfig{Prefer: []string{"JP", "US"}}
	out := selectCandidates(list, &cfg, 0)
	if len(out) != 2 || out[0].Name != "JP#1" || out[1].Name != "US#1" {
		t.Fatalf("selectCandidates = %v", namesOf(out))
	}
}
