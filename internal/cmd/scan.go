// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"

	"github.com/dotandev/glassbox/internal/abi"
	"github.com/dotandev/glassbox/internal/errors"
	"github.com/dotandev/glassbox/internal/security"
	"github.com/dotandev/glassbox/internal/visualizer"
	"github.com/spf13/cobra"
)

var scanABIPath string

var scanCmd = &cobra.Command{
	Use:     "scan <source-file-or-directory>",
	GroupID: "testing",
	Short:   "Scan contract code and ABI metadata for vulnerability heuristics",
	Long: `Scan Soroban contract source and optional ABI metadata for security patterns
such as privileged functions without visible auth checks, panic-prone code paths,
persistent storage writes without visible auth, and randomness usage.`,
	Example: `  glassbox scan ./contracts/token
  glassbox scan ./contracts/token --abi ./token.abi.json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		spec, err := loadScanABI(scanABIPath)
		if err != nil {
			return errors.WrapValidationError(fmt.Sprintf("failed to load ABI: %v", err))
		}

		detector := security.NewDetector()
		findings, err := detector.ScanSourcePath(args[0], spec)
		if err != nil {
			return errors.WrapValidationError(fmt.Sprintf("failed to scan source: %v", err))
		}

		printSecurityFindings(findings)
		return nil
	},
}

func loadScanABI(path string) (*abi.ContractSpec, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	switch abi.DetectFormat(data) {
	case abi.ImportFormatJSON:
		return abi.ImportFromJSON(data)
	default:
		return abi.ImportFromXDR(data)
	}
}

func printSecurityFindings(findings []security.Finding) {
	if len(findings) == 0 {
		fmt.Printf("%s No security issues detected\n", visualizer.Success())
		return
	}
	fmt.Printf("%s Security findings: %d\n", visualizer.Warning(), len(findings))
	for i, finding := range findings {
		fmt.Printf("%d. [%s] %s - %s\n", i+1, finding.Severity, finding.Title, finding.Description)
		if finding.Evidence != "" {
			fmt.Printf("   Evidence: %s\n", finding.Evidence)
		}
	}
}

func init() {
	scanCmd.Flags().StringVar(&scanABIPath, "abi", "", "Optional contract ABI/spec file (JSON or XDR) for metadata heuristics")
	rootCmd.AddCommand(scanCmd)
}
