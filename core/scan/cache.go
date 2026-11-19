package scan

import (
	"time"
)

// CacheTTL is the lifetime of a scan result (PROMPT.md 5.5: 30 minutes).
const CacheTTL = 30 * time.Minute

// cacheFile is where the cache lives inside the node's config directory.
const cacheFile = "scan-cache.json"

// Store is the subset of store.Store the cache needs (kept as an interface so
// the scan package stays free of file-system policy).
type Store interface {
	ReadJSON(name string, v any) error
	WriteJSON(name string, v any) error
}

// Cache is the persisted result of one scan.
type Cache struct {
	Fingerprint string    `json:"fingerprint"`
	Level       int       `json:"level"`
	At          time.Time `json:"at"`
	Results     []Result  `json:"results"`
}

// Valid reports whether the cache may be reused for this fingerprint/level.
func (c *Cache) Valid(fingerprint string, level Level, now time.Time) bool {
	if c == nil || len(c.Results) == 0 {
		return false
	}
	if c.Fingerprint != fingerprint {
		return false
	}
	if c.Level < int(level) {
		return false
	}
	return now.Sub(c.At) < CacheTTL
}

// LoadCache reads the cache; a missing or corrupt file yields nil.
func LoadCache(st Store) *Cache {
	if st == nil {
		return nil
	}
	var c Cache
	if err := st.ReadJSON(cacheFile, &c); err != nil {
		return nil
	}
	return &c
}

// SaveCache persists the cache.
func SaveCache(st Store, c *Cache) error {
	if st == nil || c == nil {
		return nil
	}
	return st.WriteJSON(cacheFile, c)
}
