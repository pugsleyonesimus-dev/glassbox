// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/dotandev/glassbox/internal/errors"
	"github.com/spf13/cobra"
)

var (
	benchMode  string
	benchCount int
	benchJSON  bool
)

// BenchResult holds timing and allocation metrics for a single benchmark stage.
type BenchResult struct {
	Stage       string        `json:"stage"`
	Iterations  int           `json:"iterations"`
	TotalTime   time.Duration `json:"total_time_ns"`
	AvgTime     time.Duration `json:"avg_time_ns"`
	AllocsPerOp uint64        `json:"allocs_per_op"`
	BytesPerOp  uint64        `json:"bytes_per_op"`
}

var benchCmd = &cobra.Command{
	Use:     "bench",
	GroupID: "development",
	Short:   "Run performance benchmarks for RPC, replay, and source mapping",
	Long: `Measure timing and memory usage for key pipeline stages.

MODES
  rpc        Benchmark JSON-RPC request marshaling and response parsing
  replay     Benchmark snapshot registry serialization and integrity checks
  sourcemap  Benchmark source cache lookups and entry serialization
  all        Run all benchmarks (default)

EXAMPLES
  # Run all benchmarks
  glassbox bench

  # Benchmark RPC only with 10 iterations
  glassbox bench --mode rpc --count 10

  # Output results as JSON
  glassbox bench --mode all --json`,
	Args:  cobra.NoArgs,
	RunE:  runBench,
}

func init() {
	benchCmd.Flags().StringVar(&benchMode, "mode", "all",
		"Pipeline stage to benchmark: rpc, replay, sourcemap, or all")
	benchCmd.Flags().IntVar(&benchCount, "count", 5,
		"Number of benchmark iterations per stage")
	benchCmd.Flags().BoolVar(&benchJSON, "json", false,
		"Output benchmark results as JSON")

	rootCmd.AddCommand(benchCmd)
}

func runBench(cmd *cobra.Command, _ []string) error {
	mode := strings.ToLower(strings.TrimSpace(benchMode))
	switch mode {
	case "rpc", "replay", "sourcemap", "all":
	default:
		return errors.WrapValidationError(
			fmt.Sprintf("unknown benchmark mode %q; choose rpc, replay, sourcemap, or all", mode),
		)
	}

	var results []BenchResult

	if mode == "rpc" || mode == "all" {
		results = append(results, runRPCBenchmarks(benchCount)...)
	}
	if mode == "replay" || mode == "all" {
		results = append(results, runReplayBenchmarks(benchCount)...)
	}
	if mode == "sourcemap" || mode == "all" {
		results = append(results, runSourcemapBenchmarks(benchCount)...)
	}

	out := cmd.OutOrStdout()

	if benchJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	for _, r := range results {
		fmt.Fprintf(out, "\nStage: %s\n", r.Stage)
		fmt.Fprintf(out, "  Iterations:       %d\n", r.Iterations)
		fmt.Fprintf(out, "  Duration (avg):   %s\n", r.AvgTime)
		fmt.Fprintf(out, "  Allocs/op:        %d\n", r.AllocsPerOp)
		fmt.Fprintf(out, "  Bytes/op:         %d\n", r.BytesPerOp)
	}
	return nil
}

// runRPCBenchmarks measures JSON marshal/unmarshal latency for RPC payloads.
func runRPCBenchmarks(n int) []BenchResult {
	type rpcReq struct {
		Jsonrpc string        `json:"jsonrpc"`
		ID      int           `json:"id"`
		Method  string        `json:"method"`
		Params  []interface{} `json:"params"`
	}

	req := rpcReq{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  "getLedgerEntries",
		Params:  []interface{}{generateKeys(50)},
	}

	return []BenchResult{
		measureStage("rpc/marshal", n, func() {
			_, _ = json.Marshal(req)
		}),
		measureStage("rpc/unmarshal", n, func() {
			data, _ := json.Marshal(req)
			var out rpcReq
			_ = json.Unmarshal(data, &out)
		}),
	}
}

// runReplayBenchmarks measures snapshot registry serialization latency.
func runReplayBenchmarks(n int) []BenchResult {
	type snapshotEntry struct {
		Timestamp int64             `json:"timestamp"`
		Entries   map[string]string `json:"entries"`
		Checksum  string            `json:"checksum"`
	}

	snap := snapshotEntry{
		Timestamp: time.Now().Unix(),
		Entries:   generateLedgerEntries(20),
		Checksum:  strings.Repeat("a", 64),
	}

	return []BenchResult{
		measureStage("replay/marshal", n, func() {
			_, _ = json.Marshal(snap)
		}),
		measureStage("replay/unmarshal", n, func() {
			data, _ := json.Marshal(snap)
			var out snapshotEntry
			_ = json.Unmarshal(data, &out)
		}),
	}
}

// runSourcemapBenchmarks measures source cache entry serialization.
func runSourcemapBenchmarks(n int) []BenchResult {
	type cacheEntry struct {
		ContractID string    `json:"contract_id"`
		Source     string    `json:"source"`
		FetchedAt  time.Time `json:"fetched_at"`
		ExpiresAt  time.Time `json:"expires_at"`
	}

	entry := cacheEntry{
		ContractID: strings.Repeat("c", 56),
		Source:     strings.Repeat("s", 4096),
		FetchedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(24 * time.Hour),
	}

	return []BenchResult{
		measureStage("sourcemap/marshal", n, func() {
			_, _ = json.Marshal(entry)
		}),
		measureStage("sourcemap/unmarshal", n, func() {
			data, _ := json.Marshal(entry)
			var out cacheEntry
			_ = json.Unmarshal(data, &out)
		}),
	}
}

// measureStage runs fn n times, collects timing and allocation metrics, and
// returns a BenchResult.
func measureStage(name string, n int, fn func()) BenchResult {
	// Warm-up
	for i := 0; i < 3; i++ {
		fn()
	}

	var memBefore, memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)

	start := time.Now()
	for i := 0; i < n; i++ {
		fn()
	}
	total := time.Since(start)

	runtime.ReadMemStats(&memAfter)

	allocsDelta := memAfter.Mallocs - memBefore.Mallocs
	bytesDelta := memAfter.TotalAlloc - memBefore.TotalAlloc

	allocsPerOp := uint64(0)
	bytesPerOp := uint64(0)
	if n > 0 {
		allocsPerOp = allocsDelta / uint64(n)
		bytesPerOp = bytesDelta / uint64(n)
	}

	return BenchResult{
		Stage:       name,
		Iterations:  n,
		TotalTime:   total,
		AvgTime:     total / time.Duration(n),
		AllocsPerOp: allocsPerOp,
		BytesPerOp:  bytesPerOp,
	}
}

func generateKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = strings.Repeat("k", 64)
	}
	return keys
}

func generateLedgerEntries(n int) map[string]string {
	m := make(map[string]string, n)
	for i := 0; i < n; i++ {
		m[fmt.Sprintf("key-%d", i)] = strings.Repeat("v", 128)
	}
	return m
}
