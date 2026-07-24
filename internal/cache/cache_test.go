package cache_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/drips/glassbox/internal/cache"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func mainnetKey(id string) cache.Key {
	return cache.Key{
		Network: "Public Global Stellar Network ; September 2015",
		Kind:    cache.KindEnvelope,
		ID:      id,
	}
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{t: start}
}

type fakeClock struct {
	t time.Time
}

func (f *fakeClock) Now() time.Time { return f.t }
func (f *fakeClock) Advance(d time.Duration) { f.t = f.t.Add(d) }

// ---------------------------------------------------------------------------
// hit / miss
// ---------------------------------------------------------------------------

func TestGet_Miss(t *testing.T) {
	c := cache.New(cache.Options{})
	data, err := c.Get(mainnetKey("tx1"))
	if err != nil || data != nil {
		t.Fatalf("expected nil, nil for miss; got %v, %v", data, err)
	}
	if c.Stats().Misses != 1 {
		t.Fatal("miss counter not incremented")
	}
}

func TestPutGet_Hit(t *testing.T) {
	c := cache.New(cache.Options{})
	key := mainnetKey("tx1")
	payload := []byte(`{"result":"ok"}`)

	c.Put(key, payload)
	got, err := c.Get(key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("data mismatch: want %s got %s", payload, got)
	}
	if c.Stats().Hits != 1 {
		t.Fatal("hit counter not incremented")
	}
}

// ---------------------------------------------------------------------------
// corruption
// ---------------------------------------------------------------------------

func TestGet_Corruption(t *testing.T) {
	clk := newFakeClock(time.Now())
	c := cache.New(cache.Options{
		NowFunc: clk.Now,
	})
	key := mainnetKey("tx-corrupt")
	c.Put(key, []byte(`good data`))

	// Tamper: replace entry data via a second Put with a different checksum
	// mismatch by manipulating the internal state — since the cache is opaque,
	// simulate by putting corrupted bytes that have a broken checksum.
	//
	// We use a test-only helper exposed via a CorruptEntry method; since we
	// didn't add that, we test corruption indirectly: put one value, then the
	// cache's checksum for that entry is for "good data".  Put a different key
	// (we cannot corrupt from outside), so instead we use the exported
	// CorruptForTest helper that the test-only file provides.
	//
	// Actually, since cache is opaque, we verify ErrCorrupted by using the
	// exported helper below.
	data, err := c.Get(key)
	if err != nil {
		t.Fatalf("fresh entry should not be corrupted: %v", err)
	}
	_ = data
}

// TestCorrupted uses the test-helper to inject a corrupted entry.
func TestGet_CorruptedEntry(t *testing.T) {
	clk := newFakeClock(time.Now())
	c := cache.New(cache.Options{NowFunc: clk.Now})
	key := mainnetKey("tx-bad")
	// Use CorruptForTest to insert an entry whose stored checksum does not
	// match the data.
	cache.CorruptForTest(c, key, []byte("actual data"), "badhash000")

	_, err := c.Get(key)
	if !errors.Is(err, cache.ErrCorrupted) {
		t.Fatalf("expected ErrCorrupted, got %v", err)
	}
	if c.Stats().Corrupted != 1 {
		t.Fatal("corrupted counter not incremented")
	}
}

// ---------------------------------------------------------------------------
// expiration / eviction
// ---------------------------------------------------------------------------

func TestGet_Expired(t *testing.T) {
	clk := newFakeClock(time.Now())
	c := cache.New(cache.Options{
		TTL:     10 * time.Minute,
		NowFunc: clk.Now,
	})
	key := mainnetKey("tx-exp")
	c.Put(key, []byte("data"))

	clk.Advance(11 * time.Minute)

	_, err := c.Get(key)
	if !errors.Is(err, cache.ErrEvicted) {
		t.Fatalf("expected ErrEvicted after TTL, got %v", err)
	}
	if c.Stats().Evictions != 1 {
		t.Fatal("eviction counter not incremented")
	}
}

func TestEviction_SizeLimit(t *testing.T) {
	const max = 3
	c := cache.New(cache.Options{MaxEntries: max})

	for i := 0; i < max+2; i++ {
		key := cache.Key{Network: "net", Kind: cache.KindLedger, ID: fmt.Sprintf("%d", i)}
		c.Put(key, []byte("x"))
	}
	// After inserting max+2 entries only max should remain.
	keys := c.SnapshotKeys()
	if len(keys) != max {
		t.Fatalf("expected %d entries after eviction, got %d", max, len(keys))
	}
	if c.Stats().Evictions != 2 {
		t.Fatalf("expected 2 evictions, got %d", c.Stats().Evictions)
	}
}

// ---------------------------------------------------------------------------
// network identity in key
// ---------------------------------------------------------------------------

func TestNetworkIsolation(t *testing.T) {
	c := cache.New(cache.Options{})
	mainnet := cache.Key{Network: "Public Global Stellar Network ; September 2015", Kind: cache.KindEnvelope, ID: "tx1"}
	testnet := cache.Key{Network: "Test SDF Network ; September 2015", Kind: cache.KindEnvelope, ID: "tx1"}

	c.Put(mainnet, []byte("mainnet-data"))

	data, err := c.Get(testnet)
	if err != nil || data != nil {
		t.Fatal("testnet key must not hit mainnet entry")
	}
}

// ---------------------------------------------------------------------------
// invalidation
// ---------------------------------------------------------------------------

func TestInvalidate(t *testing.T) {
	c := cache.New(cache.Options{})
	key := mainnetKey("tx-inv")
	c.Put(key, []byte("data"))

	c.Invalidate(key)
	data, err := c.Get(key)
	if err != nil || data != nil {
		t.Fatal("invalidated entry must not be returned")
	}
}

func TestInvalidateAll(t *testing.T) {
	c := cache.New(cache.Options{})
	for i := 0; i < 5; i++ {
		c.Put(mainnetKey(fmt.Sprintf("tx%d", i)), []byte("d"))
	}
	c.InvalidateAll()
	if keys := c.SnapshotKeys(); len(keys) != 0 {
		t.Fatalf("expected 0 entries after InvalidateAll, got %d", len(keys))
	}
}

// ---------------------------------------------------------------------------
// second identical request completes from cache
// ---------------------------------------------------------------------------

func TestOfflineReplay_SecondRequest(t *testing.T) {
	c := cache.New(cache.Options{})
	key := mainnetKey("txABC")
	blob := []byte(`{"id":1,"result":{"status":"success"}}`)

	// First request: populate cache.
	c.Put(key, blob)

	// Second request: should succeed without RPC.
	got, err := c.Get(key)
	if err != nil || got == nil {
		t.Fatalf("second identical request must succeed from cache: err=%v", err)
	}
	if string(got) != string(blob) {
		t.Fatalf("cache returned wrong data")
	}
}

// ---------------------------------------------------------------------------
// diagnostics
// ---------------------------------------------------------------------------

func TestDiagnostics_Counts(t *testing.T) {
	c := cache.New(cache.Options{})
	key := mainnetKey("dx1")
	c.Put(key, []byte("data"))
	_, _ = c.Get(key)              // hit
	_, _ = c.Get(mainnetKey("x2")) // miss

	d := c.Diagnostics()
	if d.Hits != 1 {
		t.Fatalf("expected 1 hit, got %d", d.Hits)
	}
	if d.Misses != 1 {
		t.Fatalf("expected 1 miss, got %d", d.Misses)
	}
	if d.Insertions != 1 {
		t.Fatalf("expected 1 insertion, got %d", d.Insertions)
	}
}

func TestDiagnosticsJSON_Valid(t *testing.T) {
	c := cache.New(cache.Options{})
	b, err := c.MarshalDiagnosticsJSON()
	if err != nil {
		t.Fatalf("MarshalDiagnosticsJSON: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("expected non-empty JSON")
	}
}
