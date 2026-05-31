// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package replay

import (
	"path/filepath"
	"testing"

	"github.com/dotandev/glassbox/internal/snapshot"
)

func benchmarkSnapshot(n int) *snapshot.Snapshot {
	entries := make(map[string]string, n)
	for i := 0; i < n; i++ {
		entries[nRepeat("k", 64)+string(rune('0'+i%10))] = nRepeat("v", 128)
	}
	return snapshot.FromMap(entries)
}

func nRepeat(s string, n int) string {
	result := make([]byte, n)
	for i := range result {
		result[i] = s[0]
	}
	return string(result)
}

// BenchmarkRegistryAdd measures snapshot insertion with checksum computation.
func BenchmarkRegistryAdd(b *testing.B) {
	sizes := []struct {
		name    string
		entries int
	}{
		{"Small", 5},
		{"Medium", 50},
		{"Large", 200},
	}

	for _, tt := range sizes {
		b.Run(tt.name, func(b *testing.B) {
			snap := benchmarkSnapshot(tt.entries)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				r := New("v1.0.0", "txhash", "testnet", "env", "meta")
				r.Add(int64(i), snap)
			}
		})
	}
}

// BenchmarkRegistrySaveToFile measures atomic JSON serialisation to disk.
func BenchmarkRegistrySaveToFile(b *testing.B) {
	sizes := []struct {
		name    string
		entries int
	}{
		{"Small", 5},
		{"Medium", 20},
	}

	for _, tt := range sizes {
		b.Run(tt.name, func(b *testing.B) {
			r := New("v1.0.0", "txhash", "testnet", "env", "meta")
			r.Add(1000, benchmarkSnapshot(tt.entries))
			dir := b.TempDir()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				path := filepath.Join(dir, "reg.json")
				_ = r.SaveToFile(path)
			}
		})
	}
}

// BenchmarkRegistryLoadFromFile measures JSON deserialization from disk.
func BenchmarkRegistryLoadFromFile(b *testing.B) {
	r := New("v1.0.0", "txhash", "testnet", "env", "meta")
	for i := 0; i < 10; i++ {
		r.Add(int64(i)*100, benchmarkSnapshot(20))
	}
	dir := b.TempDir()
	path := filepath.Join(dir, "reg.json")
	if err := r.SaveToFile(path); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadFromFile(path)
	}
}

// BenchmarkVerifyIntegrity measures SHA-256 integrity checks.
func BenchmarkVerifyIntegrity(b *testing.B) {
	sizes := []struct {
		name    string
		entries int
	}{
		{"FewEntries", 3},
		{"ManyEntries", 20},
	}

	for _, tt := range sizes {
		b.Run(tt.name, func(b *testing.B) {
			r := New("v1.0.0", "txhash", "testnet", "env", "meta")
			for i := 0; i < tt.entries; i++ {
				r.Add(int64(i)*100, benchmarkSnapshot(10))
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = r.VerifyIntegrity()
			}
		})
	}
}

// BenchmarkValidateLedgerSequence measures mismatch detection.
func BenchmarkValidateLedgerSequence(b *testing.B) {
	b.Run("Match", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = ValidateLedgerSequence(100, 100)
		}
	})

	b.Run("Mismatch", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = ValidateLedgerSequence(200, 100)
		}
	})
}
