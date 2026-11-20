package protonnode

import (
	"encoding/json"
	"errors"
	"testing"
)

// The UI relies on this verdict: a stored session must not be advertised as
// usable after Proton rejected it.
func TestAuthVerdictIsReportedInSessionState(t *testing.T) {
	eng.resetAuthVerdict()
	var st map[string]any
	if err := json.Unmarshal([]byte(SessionState()), &st); err != nil {
		t.Fatalf("session state: %v", err)
	}
	if st["authState"] != "unknown" {
		t.Fatalf("fresh verdict = %v, want unknown", st["authState"])
	}

	eng.markAuthExpired(errors.New("proton api: http 401, code 401: Invalid access token"))
	json.Unmarshal([]byte(SessionState()), &st)
	if st["authState"] != "expired" {
		t.Fatalf("after 401 = %v, want expired", st["authState"])
	}

	eng.markAuthValid()
	json.Unmarshal([]byte(SessionState()), &st)
	if st["authState"] != "valid" {
		t.Fatalf("after success = %v, want valid", st["authState"])
	}
	if st["lastVerifiedAt"] == nil || st["lastVerifiedAt"] == "" {
		t.Fatal("valid verdict must carry lastVerifiedAt")
	}
}

func TestIsAuthError(t *testing.T) {
	if !isAuthError(errors.New("proton api: http 401: nope")) {
		t.Fatal("401 not recognised")
	}
	if !isAuthError(errors.New("token refresh failed: code 2001: Invalid input")) {
		t.Fatal("2001 not recognised")
	}
	if isAuthError(errors.New("dial tcp: i/o timeout")) {
		t.Fatal("timeout misclassified as auth error")
	}
	if isAuthError(nil) {
		t.Fatal("nil is not an auth error")
	}
}
