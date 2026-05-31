// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/dotandev/glassbox/internal/compare"
	"github.com/dotandev/glassbox/internal/errors"
	"github.com/spf13/cobra"
)

var wasmDiffCmd = &cobra.Command{
	Use:     "wasm-diff <local-wasm> <remote-wasm>",
	GroupID: "development",
	Short:   "Compare two WASM binaries for source mapping compatibility",
	Long: `Compare a local WASM build artifact with an on-chain or reference WASM binary
to identify mismatches that can cause source mapping and debug issues.

The tool inspects both binaries for:
  • SHA-256 hash  — whether the content is bit-for-bit identical
  • File size     — detects padding or truncation differences
  • Section count — structural compatibility (same WASM layout)

Exit code 0 is returned when the binaries are identical; non-zero when they differ.`,
	Example: `  # Compare a local build with an on-chain snapshot saved to disk
  glassbox wasm-diff ./target/wasm32-unknown-unknown/release/contract.wasm ./onchain.wasm`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		localPath := args[0]
		remotePath := args[1]

		result, err := compare.DiffWASMFiles(localPath, remotePath)
		if err != nil {
			return errors.WrapValidationError(fmt.Sprintf("wasm-diff failed: %v", err))
		}

		printWASMDiff(result, localPath, remotePath)

		if result.HasDivergence {
			return fmt.Errorf("WASM binaries differ — source mapping may not match the deployed contract")
		}
		return nil
	},
}

func printWASMDiff(result *compare.WASMDiffResult, localPath, remotePath string) {
	fmt.Println()
	fmt.Println("WASM Binary Comparison")
	fmt.Println("──────────────────────────────────────────────────────────────────")
	fmt.Printf("  Local  : %s\n", localPath)
	fmt.Printf("  Remote : %s\n", remotePath)
	fmt.Println()

	printDiffRow("Hash match", result.HashMatch,
		abbreviate(result.Local.Hash, 16), abbreviate(result.Remote.Hash, 16))
	printDiffRow("Size match", result.SizeMatch,
		fmt.Sprintf("%d bytes", result.Local.Size), fmt.Sprintf("%d bytes", result.Remote.Size))
	printDiffRow("Section count", result.SectionMatch,
		fmt.Sprintf("%d section(s)", result.Local.SectionCount),
		fmt.Sprintf("%d section(s)", result.Remote.SectionCount))

	if !result.Local.IsValidWASM {
		fmt.Println()
		fmt.Println("  WARNING: local file does not appear to be a valid WASM binary (missing magic bytes)")
	}
	if !result.Remote.IsValidWASM {
		fmt.Println()
		fmt.Println("  WARNING: remote file does not appear to be a valid WASM binary (missing magic bytes)")
	}

	fmt.Println()
	if result.HasDivergence {
		fmt.Printf("  Result : [DIFF] %s\n", result.Summary)
	} else {
		fmt.Printf("  Result : [OK]   %s\n", result.Summary)
	}
	fmt.Println()
}

func printDiffRow(label string, match bool, localVal, remoteVal string) {
	mark := "[OK]  "
	if !match {
		mark = "[DIFF]"
	}
	fmt.Printf("  %s  %-20s  local=%-24s  remote=%s\n", mark, label, localVal, remoteVal)
}

func abbreviate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func init() {
	rootCmd.AddCommand(wasmDiffCmd)
}
