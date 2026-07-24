// export_test.go exposes internal helpers for white-box testing.
// This file is only compiled during `go test`.
package cache

import "time"

// CorruptForTest inserts an entry whose stored checksum is intentionally wrong
// so that the next Get returns ErrCorrupted.  This is only available in tests.
func CorruptForTest(c *Cache, key Key, data []byte, badChecksum string) {
	k := key.String()
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[k]; !exists {
		c.insertOrder = append(c.insertOrder, k)
	}
	c.entries[k] = &entry{
		data:       copyBytes(data),
		checksum:   badChecksum, // intentionally wrong
		expiresAt:  c.now().Add(c.ttl),
		insertedAt: c.now(),
	}
}

// SetNow replaces the time source mid-test without recreating the cache.
func (c *Cache) SetNow(fn func() time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = fn
}
