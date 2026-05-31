// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/dotandev/glassbox/internal/heuristic"
	"github.com/spf13/cobra"
)

// ─── heuristic rules parent command ──────────────────────────────────────────

var heuristicCmd = &cobra.Command{
	Use:     "heuristic",
	GroupID: "development",
	Short:   "Manage and inspect heuristic rules",
	Long: `Commands for working with the heuristic rule engine.

Rules can be loaded from JSON or YAML files, allowing new error patterns to be
added without recompiling the binary.`,
	Example: `  # List built-in heuristic rules
  glassbox heuristic list

  # List rules from a custom rule file
  glassbox heuristic list --rules ./my-rules.json

  # Validate a custom rule file
  glassbox heuristic validate ./my-rules.json

  # Test a rule against a sample error string
  glassbox heuristic test --error "Error(Auth, NotAuthorized)"

  # List rules from a directory and flag conflicts
  glassbox heuristic list --rules-dir ./rules/ --conflicts`,
}

// ─── heuristic list ───────────────────────────────────────────────────────────

var (
	heuristicListRules    []string
	heuristicListRulesDir string
	heuristicListConflict bool
)

var heuristicListCmd = &cobra.Command{
	Use:   "list",
	Short: "Print all active heuristic rules",
	Long: `Print the active heuristic rules in evaluation order.

By default the built-in rules are shown.  Supply --rules or --rules-dir to
include or replace them with external rule files.

Examples:
  glassbox heuristic list
  glassbox heuristic list --rules ./custom.json
  glassbox heuristic list --rules-dir ./rules/ --conflicts`,
	RunE: runHeuristicList,
}

func runHeuristicList(_ *cobra.Command, _ []string) error {
	engine, err := resolveEngine(heuristicListRules, heuristicListRulesDir)
	if err != nil {
		return err
	}

	rs := engine.RuleSet()
	rules := rs.Rules()

	if len(rules) == 0 {
		fmt.Println("No rules loaded.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PRIORITY\tID\tSEVERITY\tMATCH\tSOURCE\tDESCRIPTION")
	fmt.Fprintln(w, "--------\t--\t--------\t-----\t------\t-----------")
	for _, r := range rules {
		src := r.Source
		if src == "" {
			src = "<builtin>"
		}
		mk := string(r.MatchKind)
		if mk == "" {
			mk = "any"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n",
			r.Priority, r.ID, r.Severity, mk, src, r.Description)
	}
	_ = w.Flush()

	if heuristicListConflict {
		fmt.Println()
		printConflicts(rs)
	}

	return nil
}

// ─── heuristic validate ───────────────────────────────────────────────────────

var heuristicValidateCmd = &cobra.Command{
	Use:   "validate <rule-file> [rule-file...]",
	Short: "Validate one or more heuristic rule files",
	Long: `Parse and compile the supplied rule files, reporting any errors.

Exits with code 0 when all files are valid, 1 otherwise.

Examples:
  glassbox heuristic validate ./my-rules.json
  glassbox heuristic validate ./rules/*.yaml`,
	Args: cobra.MinimumNArgs(1),
	RunE: runHeuristicValidate,
}

func runHeuristicValidate(_ *cobra.Command, args []string) error {
	allOK := true
	for _, path := range args {
		if err := heuristic.ValidateRuleFile(path); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", path, err)
			allOK = false
		} else {
			fmt.Printf("OK   %s\n", path)
		}
	}
	if !allOK {
		return fmt.Errorf("one or more rule files failed validation")
	}
	return nil
}

// ─── heuristic test ───────────────────────────────────────────────────────────

var (
	heuristicTestRules    []string
	heuristicTestRulesDir string
	heuristicTestError    string
	heuristicTestNetwork  string
	heuristicTestTxHash   string
)

var heuristicTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Apply heuristic rules to a sample error and print the suggestion",
	Long: `Evaluate the active heuristic rules against a synthetic error string and
print the resulting suggestion.  Useful for verifying that a new rule fires as
expected before deploying it.

Examples:
  glassbox heuristic test --error "Error(Auth, NotAuthorized)"
  glassbox heuristic test --error "Error(Budget, CpuLimitExceeded)" --rules ./custom.json`,
	RunE: runHeuristicTest,
}

func runHeuristicTest(_ *cobra.Command, _ []string) error {
	if heuristicTestError == "" {
		return fmt.Errorf("--error is required")
	}

	engine, err := resolveEngine(heuristicTestRules, heuristicTestRulesDir)
	if err != nil {
		return err
	}

	txHash := heuristicTestTxHash
	if txHash == "" {
		txHash = "aaaaaa000000bbbbbb"
	}
	network := heuristicTestNetwork
	if network == "" {
		network = "testnet"
	}

	in := heuristic.Input{
		TxHash:  txHash,
		Network: network,
		Status:  "error",
		Error:   heuristicTestError,
	}

	fmt.Println(engine.Evaluate(in))
	return nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// resolveEngine builds an Engine from the supplied flags.  When no external
// files are specified the default (built-in) engine is returned.
func resolveEngine(rulePaths []string, rulesDir string) (*heuristic.Engine, error) {
	switch {
	case rulesDir != "":
		return heuristic.LoadEngineFromDir(rulesDir)
	case len(rulePaths) > 0:
		return heuristic.LoadEngineWithBuiltins(rulePaths)
	default:
		return heuristic.DefaultEngine(), nil
	}
}

func printConflicts(rs *heuristic.RuleSet) {
	conflicts := heuristic.DetectConflicts(rs)
	if len(conflicts) == 0 {
		fmt.Println("No conflicting patterns detected.")
		return
	}

	fmt.Printf("Detected %d potential conflict(s):\n\n", len(conflicts))
	for _, c := range conflicts {
		srcA := c.SourceA
		if srcA == "" {
			srcA = "<builtin>"
		}
		srcB := c.SourceB
		if srcB == "" {
			srcB = "<builtin>"
		}
		fmt.Printf("  %s (%s)  ↔  %s (%s)\n", c.RuleA, srcA, c.RuleB, srcB)
		fmt.Printf("    Winner (higher priority): %s\n\n", c.WinnerID)
	}
	fmt.Println("Note: overlapping patterns are not errors; priority ordering resolves which rule fires.")
}

func init() {
	// list flags
	heuristicListCmd.Flags().StringArrayVar(&heuristicListRules, "rules", nil,
		"Path(s) to external rule file(s) to merge with built-in rules")
	heuristicListCmd.Flags().StringVar(&heuristicListRulesDir, "rules-dir", "",
		"Directory containing rule files (replaces built-in rules)")
	heuristicListCmd.Flags().BoolVar(&heuristicListConflict, "conflicts", false,
		"Also print conflicting pattern pairs")

	// validate flags – none beyond positional args

	// test flags
	heuristicTestCmd.Flags().StringArrayVar(&heuristicTestRules, "rules", nil,
		"Path(s) to external rule file(s) to merge with built-in rules")
	heuristicTestCmd.Flags().StringVar(&heuristicTestRulesDir, "rules-dir", "",
		"Directory containing rule files (replaces built-in rules)")
	heuristicTestCmd.Flags().StringVar(&heuristicTestError, "error", "",
		"Error string to test against the rules (required)")
	heuristicTestCmd.Flags().StringVar(&heuristicTestNetwork, "network", "testnet",
		"Network name to include in the rendered suggestion")
	heuristicTestCmd.Flags().StringVar(&heuristicTestTxHash, "tx-hash", "",
		"Transaction hash to include in the rendered suggestion")

	heuristicCmd.AddCommand(heuristicListCmd)
	heuristicCmd.AddCommand(heuristicValidateCmd)
	heuristicCmd.AddCommand(heuristicTestCmd)

	rootCmd.AddCommand(heuristicCmd)
}
