// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"

	"github.com/dotandev/glassbox/internal/errors"
	"github.com/dotandev/glassbox/internal/lto"
	"github.com/spf13/cobra"
)

var validateCargoCmd = &cobra.Command{
	Use:     "validate-cargo [path]",
	GroupID: "testing",
	Short:   "Validate Cargo project configuration for source mapping compatibility",
	Long: `Inspect Cargo.toml files in a Soroban contract project and warn about settings
that may compromise source mapping accuracy and DWARF extraction.

Checks performed:
  • lto            — fat LTO breaks DWARF offsets; thin LTO degrades accuracy
  • debug          — must be enabled (1 or 2) in the release profile
  • codegen-units  — single unit (= 1) required for deterministic DWARF
  • split-debuginfo — "off" or "packed" may limit debuginfo availability
  • workspace      — discovers workspace members and validates each manifest

The command exits with a non-zero status when errors are found.`,
	Example: `  # Validate the current directory
  glassbox validate-cargo

  # Validate a specific project path
  glassbox validate-cargo ./contracts/my-contract`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) == 1 {
			dir = args[0]
		}

		if _, err := os.Stat(dir); err != nil {
			return errors.WrapValidationError(fmt.Sprintf("path not found: %s", dir))
		}

		results, err := lto.ValidateCargoProject(dir)
		if err != nil {
			return errors.WrapValidationError(fmt.Sprintf("validation failed: %v", err))
		}

		if len(results) == 0 {
			fmt.Printf("OK  No issues found in Cargo project at %s\n", dir)
			fmt.Println("    Source mapping and DWARF extraction settings look correct.")
			return nil
		}

		errorCount, warningCount := countSeverities(results)

		fmt.Printf("Cargo project validation: %s\n", dir)
		fmt.Printf("Found %d error(s), %d warning(s)\n\n", errorCount, warningCount)

		for _, r := range results {
			label := "WARNING"
			if r.Severity == "error" {
				label = "ERROR  "
			}
			fmt.Printf("[%s] %s\n", label, r.Message)
			if r.Fix != "" {
				fmt.Printf("           Fix: %s\n", r.Fix)
			}
			fmt.Println()
		}

		if errorCount > 0 {
			return fmt.Errorf("validation found %d error(s) — source mapping may not work correctly", errorCount)
		}
		return nil
	},
}

func countSeverities(results []lto.ValidationResult) (errors, warnings int) {
	for _, r := range results {
		switch r.Severity {
		case "error":
			errors++
		case "warning":
			warnings++
		}
	}
	return
}

func init() {
	rootCmd.AddCommand(validateCargoCmd)
}
