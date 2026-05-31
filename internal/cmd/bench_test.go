// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func resetBenchFlags() {
	benchMode = "all"
	benchCount = 5
	benchJSON = false
}

func TestBenchCmd_AllModes(t *testing.T) {
	resetBenchFlags()
	var out bytes.Buffer
	benchCmd.SetOut(&out)

	if err := benchCmd.RunE(benchCmd, nil); err != nil {
		t.Fatalf("RunE failed: %v", err)
	}

	output := out.String()
	for _, stage := range []string{"rpc/marshal", "replay/marshal", "sourcemap/marshal"} {
		if !strings.Contains(output, stage) {
			t.Errorf("expected stage %q in output", stage)
		}
	}
}

func TestBenchCmd_RPCMode(t *testing.T) {
	resetBenchFlags()
	benchMode = "rpc"
	var out bytes.Buffer
	benchCmd.SetOut(&out)

	if err := benchCmd.RunE(benchCmd, nil); err != nil {
		t.Fatalf("RunE failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "rpc/") {
		t.Errorf("expected rpc stage in output, got: %s", output)
	}
	if strings.Contains(output, "replay/") {
		t.Error("expected no replay stage in rpc-only mode")
	}
}

func TestBenchCmd_ReplayMode(t *testing.T) {
	resetBenchFlags()
	benchMode = "replay"
	var out bytes.Buffer
	benchCmd.SetOut(&out)

	if err := benchCmd.RunE(benchCmd, nil); err != nil {
		t.Fatalf("RunE failed: %v", err)
	}

	if !strings.Contains(out.String(), "replay/") {
		t.Error("expected replay stage in output")
	}
}

func TestBenchCmd_SourcemapMode(t *testing.T) {
	resetBenchFlags()
	benchMode = "sourcemap"
	var out bytes.Buffer
	benchCmd.SetOut(&out)

	if err := benchCmd.RunE(benchCmd, nil); err != nil {
		t.Fatalf("RunE failed: %v", err)
	}

	if !strings.Contains(out.String(), "sourcemap/") {
		t.Error("expected sourcemap stage in output")
	}
}

func TestBenchCmd_InvalidMode(t *testing.T) {
	resetBenchFlags()
	benchMode = "invalid"

	err := benchCmd.RunE(benchCmd, nil)
	if err == nil {
		t.Error("expected error for unknown mode")
	}
	if !strings.Contains(err.Error(), "unknown benchmark mode") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBenchCmd_JSONOutput(t *testing.T) {
	resetBenchFlags()
	benchJSON = true
	benchCount = 2
	var out bytes.Buffer
	benchCmd.SetOut(&out)

	if err := benchCmd.RunE(benchCmd, nil); err != nil {
		t.Fatalf("RunE failed: %v", err)
	}

	var results []BenchResult
	if err := json.Unmarshal(out.Bytes(), &results); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nraw: %s", err, out.String())
	}
	if len(results) == 0 {
		t.Error("expected at least one result in JSON output")
	}
	for _, r := range results {
		if r.Stage == "" {
			t.Error("expected non-empty stage name")
		}
		if r.Iterations != 2 {
			t.Errorf("expected 2 iterations, got %d for stage %s", r.Iterations, r.Stage)
		}
	}
}

func TestMeasureStage_CapturesMetrics(t *testing.T) {
	callCount := 0
	result := measureStage("test/stage", 10, func() {
		callCount++
	})

	if result.Stage != "test/stage" {
		t.Errorf("expected stage 'test/stage', got %q", result.Stage)
	}
	if result.Iterations != 10 {
		t.Errorf("expected 10 iterations, got %d", result.Iterations)
	}
	if result.TotalTime <= 0 {
		t.Error("expected positive total time")
	}
	if result.AvgTime <= 0 {
		t.Error("expected positive avg time")
	}
}

func TestGenerateKeys(t *testing.T) {
	keys := generateKeys(5)
	if len(keys) != 5 {
		t.Errorf("expected 5 keys, got %d", len(keys))
	}
	for _, k := range keys {
		if len(k) != 64 {
			t.Errorf("expected key length 64, got %d", len(k))
		}
	}
}

func TestGenerateLedgerEntries(t *testing.T) {
	entries := generateLedgerEntries(10)
	if len(entries) != 10 {
		t.Errorf("expected 10 entries, got %d", len(entries))
	}
}
