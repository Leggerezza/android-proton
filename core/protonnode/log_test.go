// SPDX-License-Identifier: GPL-3.0-or-later
package protonnode

import (
	"strings"
	"testing"
)

func TestRedactMasksEmailAndToken(t *testing.T) {
	line := "login: alice.smith@example.com token AQAAANCMnd8BFdERjHoAwE/Cl+sBAAAA"
	got := redact(line)
	if strings.Contains(got, "alice.smith") {
		t.Fatalf("email local part leaked: %q", got)
	}
	if strings.Contains(got, "AQAAANCMnd8BFdERjHoAwE") {
		t.Fatalf("token leaked: %q", got)
	}
	if !strings.Contains(got, "al***@example.com") {
		t.Fatalf("email not partially masked as expected: %q", got)
	}
}

func TestRingLogKeepsNewest(t *testing.T) {
	r := newRingLog(3)
	for _, s := range []string{"a", "b", "c", "d"} {
		r.add(s)
	}
	got := strings.Join(r.tail(10), ",")
	if got != "b,c,d" {
		t.Fatalf("tail = %q, want b,c,d", got)
	}
	if strings.Join(r.tail(2), ",") != "c,d" {
		t.Fatalf("tail(2) = %q", strings.Join(r.tail(2), ","))
	}
}
