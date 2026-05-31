// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package sourcemap

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// BenchmarkSourceCacheGet measures source cache lookup latency.
func BenchmarkSourceCacheGet(b *testing.B) {
	dir := b.TempDir()
	sc, err := NewSourceCache(dir)
	if err != nil {
		b.Fatal(err)
	}

	src := &SourceCode{
		ContractID: strings.Repeat("c", 56),
		WasmHash:   strings.Repeat("w", 64),
		Files:      map[string]string{"lib.rs": strings.Repeat("s", 2048)},
	}
	if err := sc.Put(src); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sc.Get(src.ContractID)
	}
}

// BenchmarkSourceCachePut measures source cache write latency.
func BenchmarkSourceCachePut(b *testing.B) {
	sizes := []struct {
		name       string
		sourceSize int
	}{
		{"Small_1KB", 1024},
		{"Medium_16KB", 16 * 1024},
		{"Large_64KB", 64 * 1024},
	}

	for _, tt := range sizes {
		b.Run(tt.name, func(b *testing.B) {
			dir := b.TempDir()
			sc, err := NewSourceCache(dir)
			if err != nil {
				b.Fatal(err)
			}

			src := &SourceCode{
				ContractID: strings.Repeat("c", 56),
				WasmHash:   strings.Repeat("w", 64),
				Files:      map[string]string{"lib.rs": strings.Repeat("s", tt.sourceSize)},
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = sc.Put(src)
			}
		})
	}
}

// BenchmarkCacheEntryMarshal measures JSON serialization of a cache entry.
func BenchmarkCacheEntryMarshal(b *testing.B) {
	entry := CacheEntry{
		Source: &SourceCode{
			ContractID: strings.Repeat("c", 56),
			WasmHash:   strings.Repeat("w", 64),
			Files:      map[string]string{"lib.rs": strings.Repeat("s", 4096)},
		},
		CachedAt: time.Now(),
		TTL:      DefaultCacheTTL.String(),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(entry)
	}
}

// BenchmarkCacheEntryUnmarshal measures JSON deserialization of a cache entry.
func BenchmarkCacheEntryUnmarshal(b *testing.B) {
	entry := CacheEntry{
		Source: &SourceCode{
			ContractID: strings.Repeat("c", 56),
			WasmHash:   strings.Repeat("w", 64),
			Files:      map[string]string{"lib.rs": strings.Repeat("s", 4096)},
		},
		CachedAt: time.Now(),
		TTL:      DefaultCacheTTL.String(),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out CacheEntry
		_ = json.Unmarshal(data, &out)
	}
}
