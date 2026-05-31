// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"strings"
	"testing"
)

// TestCommandExamples verifies that key subcommands expose non-empty Example
// fields so that `--help` output includes representative usage guidance.
func TestCommandExamples(t *testing.T) {
	cases := []struct {
		name    string
		example string
	}{
		{"trace", traceCmd.Example},
		{"compare", compareCmd.Example},
		{"heuristic", heuristicCmd.Example},
		{"debug", debugCmd.Example},
		{"session", sessionCmd.Example},
		{"cache", cacheCmd.Example},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if strings.TrimSpace(tc.example) == "" {
				t.Errorf("command %q has no Example field; add usage examples to help reduce onboarding friction", tc.name)
			}
		})
	}
}

// TestDryRunCommandExample verifies the dry-run subcommand has examples.
func TestDryRunCommandExample(t *testing.T) {
	if strings.TrimSpace(dryRunCmd.Example) == "" {
		t.Error("dry-run command has no Example field")
	}
}

// TestCommandExampleContent verifies that examples reference real flag names.
func TestCommandExampleContent(t *testing.T) {
	cases := []struct {
		name        string
		example     string
		mustContain []string
	}{
		{
			name:        "trace",
			example:     traceCmd.Example,
			mustContain: []string{"glassbox trace", "--print", "--theme"},
		},
		{
			name:        "compare",
			example:     compareCmd.Example,
			mustContain: []string{"glassbox compare", "--wasm", "--network"},
		},
		{
			name:        "dry-run",
			example:     dryRunCmd.Example,
			mustContain: []string{"glassbox dry-run", "--network"},
		},
		{
			name:        "heuristic",
			example:     heuristicCmd.Example,
			mustContain: []string{"glassbox heuristic list", "glassbox heuristic validate"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, substr := range tc.mustContain {
				if !strings.Contains(tc.example, substr) {
					t.Errorf("command %q Example missing expected content %q", tc.name, substr)
				}
			}
		})
	}
}
