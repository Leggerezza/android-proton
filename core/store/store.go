// Package store persists the node's non-secret state (server cache, last used
// server, preferences) inside a caller-provided directory.
//
// It deliberately never touches credentials: tokens, the Ed25519 seed and the
// WireGuard certificate travel through protonnode.SetSession/SessionJSON, and
// the Android app encrypts them with an Android Keystore key (AES-GCM) before
// writing them anywhere (PROMPT.md 0.5, SECURITY.md). A Go side file store for
// secrets would be a second, weaker copy of the same data.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrNoDir means the store has no directory (Init was not called).
var ErrNoDir = errors.New("store: no directory configured")

// Store is a directory holding JSON documents.
type Store struct {
	dir string
}

// New creates a store rooted at dir, creating the directory when needed.
func New(dir string) (*Store, error) {
	if dir == "" {
		return nil, ErrNoDir
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("store: create %s: %w", dir, err)
	}
	return &Store{dir: dir}, nil
}

// Dir returns the storage directory.
func (s *Store) Dir() string { return s.dir }

// ReadJSON decodes name into v. A missing file returns ErrNotExist unchanged.
func (s *Store) ReadJSON(name string, v any) error {
	buf, err := os.ReadFile(filepath.Join(s.dir, name))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(buf, v); err != nil {
		return fmt.Errorf("store: parse %s: %w", name, err)
	}
	return nil
}

// WriteJSON writes name atomically with 0600 permissions.
func (s *Store) WriteJSON(name string, v any) error {
	buf, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.WriteBytes(name, buf)
}

// WriteBytes writes name atomically with 0600 permissions.
func (s *Store) WriteBytes(name string, buf []byte) error {
	if s.dir == "" {
		return ErrNoDir
	}
	tmp := filepath.Join(s.dir, name+".tmp")
	if err := os.WriteFile(tmp, buf, 0o600); err != nil {
		return fmt.Errorf("store: write %s: %w", name, err)
	}
	if err := os.Rename(tmp, filepath.Join(s.dir, name)); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("store: rename %s: %w", name, err)
	}
	return nil
}

// Remove deletes name; a missing file is not an error.
func (s *Store) Remove(name string) error {
	err := os.Remove(filepath.Join(s.dir, name))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
