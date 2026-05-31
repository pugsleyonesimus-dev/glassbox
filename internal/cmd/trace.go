// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/dotandev/glassbox/internal/decoder"
	"github.com/dotandev/glassbox/internal/errors"
	"github.com/dotandev/glassbox/internal/trace"
	"github.com/dotandev/glassbox/internal/visualizer"
	"github.com/spf13/cobra"
)

var (
	traceFile        string
	traceThemeFlag   string
	tracePrint       bool
	traceNoColor     bool
	traceExportSVG   string
	traceOutputJSON  string
	traceExportPath   string
	traceExportFormat string
)

var traceCmd = &cobra.Command{
	Use:     "trace <trace-file>",
	GroupID: "core",
	Short:   "Interactive trace navigation and debugging",
	Long: `Launch an interactive trace viewer for bi-directional navigation through execution traces.

The trace viewer allows you to:
- Step forward and backward through execution
- Jump to specific steps
- Reconstruct state at any point
- View memory and host state changes

Use --print for a one-shot, colour-coded ASCII tree report suitable for CI
logs or piping to other tools. Add --no-color to disable ANSI colours.

Example:
  Glassbox trace execution.json
  Glassbox trace --file debug_trace.json
  Glassbox trace --print execution.json
  Glassbox trace --print --no-color execution.json | less`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Apply theme if specified, otherwise auto-detect
		if traceThemeFlag != "" {
			visualizer.SetTheme(visualizer.Theme(traceThemeFlag))
		} else {
			visualizer.SetTheme(visualizer.DetectTheme())
		}

		var filename string
		if len(args) > 0 {
			filename = args[0]
		} else if traceFile != "" {
			filename = traceFile
		} else {
			return errors.WrapCliArgumentRequired("file")
		}

		// Check if file exists
		if _, err := os.Stat(filename); os.IsNotExist(err) {
			return errors.WrapValidationError(fmt.Sprintf("trace file not found: %s", filename))
		}

		// Load trace from file
		data, err := os.ReadFile(filename)
		if err != nil {
			return errors.WrapValidationError(fmt.Sprintf("failed to read trace file: %v", err))
		}

		executionTrace, err := trace.FromJSON(data)
		if err != nil {
			return errors.WrapUnmarshalFailed(err, "trace")
		}

		// --print: render a rich ASCII tree report then exit (non-interactive)
		if tracePrint {
			opts := trace.PrintOptions{
				NoColor: traceNoColor,
			}
			trace.PrintExecutionTrace(executionTrace, opts)
			return nil
		}

		// --export-svg: generate a call graph SVG and exit
		if traceExportSVG != "" {
			if len(executionTrace.DiagnosticEvents) == 0 {
				return errors.WrapValidationError("no diagnostic events found in trace; call graph with gas cannot be generated")
			}

			// --output-json: write deterministic schema'd JSON export and exit
			if traceOutputJSON != "" {
				data, err := executionTrace.ExportJSON("1.0", time.Now())
				if err != nil {
					return errors.WrapValidationError(fmt.Sprintf("failed to export trace json: %v", err))
				}
				if err := os.WriteFile(traceOutputJSON, data, 0644); err != nil {
					return errors.WrapValidationError(fmt.Sprintf("failed to save JSON: %v", err))
				}
				fmt.Printf("%s Trace exported to: %s\n", visualizer.Symbol("success"), traceOutputJSON)
				return nil
			}
			// Load config to get MaxTraceDepth
			maxDepth := 50

			callTree, err := decoder.DecodeDiagnosticEvents(executionTrace.DiagnosticEvents, maxDepth)
			if err != nil {
				return errors.WrapValidationError(fmt.Sprintf("failed to decode call tree: %v", err))
			}
			svg := visualizer.GenerateCallGraphSVG(callTree, maxDepth)
			if err := os.WriteFile(traceExportSVG, []byte(svg), 0644); err != nil {
				return errors.WrapValidationError(fmt.Sprintf("failed to save SVG: %v", err))
			}
			fmt.Printf("%s Call graph exported to: %s\n", visualizer.Symbol("success"), traceExportSVG)
			return nil
		}

		if traceExportPath != "" {
			if tracePrint {
				return errors.WrapValidationError("cannot specify both --export and --print")
			}
			if err := trace.ExportExecutionTrace(executionTrace, traceExportFormat, traceExportPath); err != nil {
				return errors.WrapValidationError(fmt.Sprintf("failed to export trace: %v", err))
			}
			fmt.Printf("%s Trace exported to: %s\n", visualizer.Symbol("success"), traceExportPath)
			return nil
		}

		// Start interactive viewer
		viewer := trace.NewInteractiveViewer(executionTrace)
		return viewer.Start()
	},
}

func init() {
	traceCmd.Flags().StringVarP(&traceFile, "file", "f", "", "Trace file to load")
	traceCmd.Flags().StringVar(&traceThemeFlag, "theme", "", "Color theme (default, deuteranopia, protanopia, tritanopia, high-contrast)")
	traceCmd.Flags().BoolVar(&tracePrint, "print", false, "Print a rich ASCII tree report and exit (non-interactive)")
	traceCmd.Flags().BoolVar(&traceNoColor, "no-color", false, "Disable ANSI colour output (also honoured via NO_COLOR env var)")
	traceCmd.Flags().StringVar(&traceExportSVG, "export-svg", "", "Export call graph as SVG to specified file")
	traceCmd.Flags().StringVar(&traceOutputJSON, "output-json", "", "Export trace as deterministic JSON to specified file (includes schema_version)")

	_ = traceCmd.RegisterFlagCompletionFunc("theme", completeThemeFlag)

	rootCmd.AddCommand(traceCmd)
}
