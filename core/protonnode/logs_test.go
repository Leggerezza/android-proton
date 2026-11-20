package protonnode

import (
	"strings"
	"testing"
)

// Regression: the gomobile entry point never created the ring log, so
// RecentLogs() returned nothing (or threw) in the app.
func TestRecentLogsAfterInit(t *testing.T) {
	if err := Init(t.TempDir()); err != nil {
		t.Fatalf("init: %v", err)
	}
	out := RecentLogs(10)
	if out == "" {
		t.Fatal("RecentLogs is empty after Init")
	}
	if !strings.Contains(out, "core: initialized") {
		t.Fatalf("expected the init line, got %q", out)
	}
}
