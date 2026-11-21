// SPDX-License-Identifier: GPL-3.0-or-later
package store

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteJSONIsAtomicAndPrivate(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "node"))
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]int{"a": 1}
	if err := s.WriteJSON("prefs.json", payload); err != nil {
		t.Fatal(err)
	}
	var got map[string]int
	if err := s.ReadJSON("prefs.json", &got); err != nil {
		t.Fatal(err)
	}
	if got["a"] != 1 {
		t.Fatalf("round trip lost data: %v", got)
	}
	if _, err := os.Stat(filepath.Join(s.Dir(), "prefs.json.tmp")); !os.IsNotExist(err) {
		t.Fatalf("temp file left behind: %v", err)
	}
	info, err := os.Stat(filepath.Join(s.Dir(), "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Windows does not model Unix permission bits; the 0600 request is free
	// there, so the check only makes sense on the platforms that have it.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 0600", info.Mode().Perm())
	}
}

func TestNewRejectsEmptyDir(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("New(\"\") accepted")
	}
}
