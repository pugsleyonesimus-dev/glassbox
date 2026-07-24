// Package cache provides a content-addressed, size-bounded, TTL-enforced
// cache for immutable RPC responses (transaction envelopes, ledger data,
// footprints).  Mutable simulation responses and credentials must never be
// stored here.
//
// Every entry is keyed by a composite key that includes the network passphrase
// so that responses captured against one network cannot be served to a
// different one.  Stored entries include a SHA-256 checksum that is verified
// on every read; corrupted entries are discarded and a cache-miss is returned.
// Cache hits and misses are recorded in a Stats counter that is surfaced in
// diagnostics.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// DefaultMaxEntries is the number of entries kept before the oldest are
// evicted.  Choose a conservative default; operators can override via Options.
const DefaultMaxEntries = 512

// DefaultTTL is how long an entry remains valid after it was written.
const DefaultTTL = 24 * time.Hour

// ErrCorrupted is returned when a stored entry fails its integrity check.
var ErrCorrupted = errors.New("cache entry corrupted")

// ErrEvicted is returned when an entry has been evicted from the cache.
var ErrEvicted = errors.New("cache entry evicted")

// Kind identifies the type of immutable resource stored in the cache.
type Kind string

const (
	KindEnvelope  Kind = "envelope"
	KindLedger    Kind = "ledger"
	KindFootprint Kind = "footprint"
	// KindSimulation must NOT be used — simulation responses are mutable.
)

// Key is the composite cache key.  Network passphrase is mandatory so that
// entries for different networks are always distinct.
type Key struct {
	// Network is the Stellar network passphrase.
	Network string
	// Kind identifies the resource type.
	Kind Kind
	// ID is a stable, opaque identifier for the specific resource (e.g.
	// transaction hash, ledger sequence serialized as a string).
	ID string
}

// String returns a deterministic string form of the key suitable for use as a
// map key or log line.
func (k Key) String() string {
	return fmt.Sprintf("%s|%s|%s", k.Kind, k.Network, k.ID)
}

// entry is the internal record stored per cache slot.
type entry struct {
	data      []byte
	checksum  string // hex SHA-256 of data
	expiresAt time.Time
	insertedAt time.Time
}

// Stats records observable cache activity for diagnostics.
type Stats struct {
	Hits       uint64
	Misses     uint64
	Corrupted  uint64
	Evictions  uint64
	Insertions uint64
}

// Cache is a thread-safe, content-addressed in-memory store.
type Cache struct {
	mu         sync.Mutex
	entries    map[string]*entry
	insertOrder []string // oldest-first for LRU eviction
	maxEntries int
	ttl        time.Duration
	now        func() time.Time // injectable for testing
	stats      Stats
}

// Options configures a Cache.
type Options struct {
	MaxEntries int
	TTL        time.Duration
	// NowFunc overrides the time source (for testing).
	NowFunc func() time.Time
}

// New creates a Cache with the provided options.  Zero-value fields in opts
// fall back to the package-level defaults.
func New(opts Options) *Cache {
	if opts.MaxEntries <= 0 {
		opts.MaxEntries = DefaultMaxEntries
	}
	if opts.TTL <= 0 {
		opts.TTL = DefaultTTL
	}
	now := opts.NowFunc
	if now == nil {
		now = time.Now
	}
	return &Cache{
		entries:    make(map[string]*entry),
		maxEntries: opts.MaxEntries,
		ttl:        opts.TTL,
		now:        now,
	}
}

// Put stores data under key.  It computes and stores the SHA-256 checksum and
// enforces the size limit by evicting the oldest entry when the cap is
// reached.
func (c *Cache) Put(key Key, data []byte) {
	checksum := computeChecksum(data)
	k := key.String()

	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()

	if _, exists := c.entries[k]; !exists {
		// Enforce capacity: evict oldest entry.
		if len(c.entries) >= c.maxEntries {
			c.evictOldest()
		}
		c.insertOrder = append(c.insertOrder, k)
	}

	c.entries[k] = &entry{
		data:       copyBytes(data),
		checksum:   checksum,
		expiresAt:  now.Add(c.ttl),
		insertedAt: now,
	}
	c.stats.Insertions++
}

// Get retrieves data for key.  It returns (nil, ErrCorrupted) when the stored
// checksum does not match, (nil, ErrEvicted) when the entry has expired, and
// (nil, nil) on a plain miss.  A non-nil error is always a cache miss; the
// caller must fall back to a live RPC request.
func (c *Cache) Get(key Key) ([]byte, error) {
	k := key.String()

	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.entries[k]
	if !ok {
		c.stats.Misses++
		return nil, nil
	}

	if c.now().After(e.expiresAt) {
		c.deleteEntry(k)
		c.stats.Misses++
		c.stats.Evictions++
		return nil, ErrEvicted
	}

	// Integrity check.
	if computeChecksum(e.data) != e.checksum {
		c.deleteEntry(k)
		c.stats.Corrupted++
		c.stats.Misses++
		return nil, ErrCorrupted
	}

	c.stats.Hits++
	return copyBytes(e.data), nil
}

// Invalidate removes a single entry from the cache.
func (c *Cache) Invalidate(key Key) {
	k := key.String()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleteEntry(k)
}

// InvalidateAll removes all entries, resetting the cache to empty.
func (c *Cache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*entry)
	c.insertOrder = c.insertOrder[:0]
}

// Stats returns a snapshot of cache activity counters.
func (c *Cache) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
}

// SnapshotKeys returns the string keys of all non-expired entries sorted
// lexicographically.  Intended for diagnostics only.
func (c *Cache) SnapshotKeys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	keys := make([]string, 0, len(c.entries))
	for k, e := range c.entries {
		if !now.After(e.expiresAt) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

func (c *Cache) evictOldest() {
	if len(c.insertOrder) == 0 {
		return
	}
	oldest := c.insertOrder[0]
	c.insertOrder = c.insertOrder[1:]
	delete(c.entries, oldest)
	c.stats.Evictions++
}

func (c *Cache) deleteEntry(k string) {
	delete(c.entries, k)
	// Remove from insertOrder (linear scan — acceptable for small caches).
	for i, key := range c.insertOrder {
		if key == k {
			c.insertOrder = append(c.insertOrder[:i], c.insertOrder[i+1:]...)
			break
		}
	}
}

func computeChecksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func copyBytes(b []byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

// ---------------------------------------------------------------------------
// Diagnostic helpers
// ---------------------------------------------------------------------------

// DiagnosticReport is a JSON-serializable summary of current cache state for
// inclusion in debug reports.  It does NOT include entry contents (which may
// contain sensitive data).
type DiagnosticReport struct {
	TotalEntries int    `json:"total_entries"`
	Hits         uint64 `json:"hits"`
	Misses       uint64 `json:"misses"`
	Corrupted    uint64 `json:"corrupted"`
	Evictions    uint64 `json:"evictions"`
	Insertions   uint64 `json:"insertions"`
}

// Diagnostics returns a report of current cache activity.
func (c *Cache) Diagnostics() DiagnosticReport {
	c.mu.Lock()
	defer c.mu.Unlock()
	return DiagnosticReport{
		TotalEntries: len(c.entries),
		Hits:         c.stats.Hits,
		Misses:       c.stats.Misses,
		Corrupted:    c.stats.Corrupted,
		Evictions:    c.stats.Evictions,
		Insertions:   c.stats.Insertions,
	}
}

// MarshalDiagnosticsJSON serializes the diagnostic report to JSON.
func (c *Cache) MarshalDiagnosticsJSON() ([]byte, error) {
	report := c.Diagnostics()
	b, err := json.Marshal(report)
	if err != nil {
		return nil, fmt.Errorf("cache diagnostics marshal: %w", err)
	}
	return b, nil
}
