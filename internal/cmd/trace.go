// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dotandev/glassbox/internal/decoder"
	"github.com/dotandev/glassbox/internal/errors"
	"github.com/dotandev/glassbox/internal/gasmodel"
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
	traceGasModelPath string
	traceComments     []string
	traceMetadata     []string
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
logs or piping to other tools. Add --no-color to disable ANSI colours.`,
	Example: `  # Open the interactive trace viewer
  glassbox trace execution.json

  # Load a trace via the --file flag
  glassbox trace --file debug_trace.json

  # Print a colour-coded ASCII tree and exit (suitable for CI logs)
  glassbox trace --print execution.json

  # Print without ANSI colours (pipe-friendly)
  glassbox trace --print --no-color execution.json | less

  # Force dark-mode colour palette
  glassbox trace --theme dark execution.json

  # Export trace as deterministic JSON for diffing or archiving
  glassbox trace --output-json trace_export.json execution.json

  # Export call graph as SVG
  glassbox trace --export-svg callgraph.svg execution.json`,
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
		if traceGasModelPath != "" {
			model, err := gasmodel.ParseGasModel(traceGasModelPath)
			if err != nil {
				return errors.WrapValidationError(fmt.Sprintf("failed to load gas model: %v", err))
			}
			trace.AnnotateExecutionCosts(executionTrace, nil, model)
		}
		if len(traceComments) > 0 || len(traceMetadata) > 0 {
			opts, err := traceExportOptions()
			if err != nil {
				return err
			}
			executionTrace.Annotations.Comments = append(executionTrace.Annotations.Comments, opts.Comments...)
			if executionTrace.Annotations.SessionMetadata == nil {
				executionTrace.Annotations.SessionMetadata = make(map[string]string)
			}
			for k, v := range opts.SessionMetadata {
				executionTrace.Annotations.SessionMetadata[k] = v
			}
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
			opts, err := traceExportOptions()
			if err != nil {
				return err
			}
			if err := trace.ExportExecutionTraceWithOptions(executionTrace, traceExportFormat, traceExportPath, opts); err != nil {
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
	traceCmd.Flags().StringVar(&traceThemeFlag, "theme", "", "Color theme (default, dark, light, deuteranopia, protanopia, tritanopia, high-contrast)")
	traceCmd.Flags().BoolVar(&tracePrint, "print", false, "Print a rich ASCII tree report and exit (non-interactive)")
	traceCmd.Flags().BoolVar(&traceNoColor, "no-color", false, "Disable ANSI colour output (also honoured via NO_COLOR env var)")
	traceCmd.Flags().StringVar(&traceExportSVG, "export-svg", "", "Export call graph as SVG to specified file")
	traceCmd.Flags().StringVar(&traceOutputJSON, "output-json", "", "Export trace as deterministic JSON to specified file (includes schema_version)")
	traceCmd.Flags().StringVar(&traceExportPath, "export", "", "Export trace report to a file")
	traceCmd.Flags().StringVar(&traceExportFormat, "export-format", "html", "Trace export format (html, markdown)")
	traceCmd.Flags().StringVar(&traceGasModelPath, "gas-model", "", "Gas model JSON used to annotate contract call cost estimates")
	traceCmd.Flags().StringArrayVar(&traceComments, "comment", nil, "Comment to include in exported trace artifacts; repeatable")
	traceCmd.Flags().StringArrayVar(&traceMetadata, "meta", nil, "Session metadata for exported trace artifacts in key=value form; repeatable")

	_ = traceCmd.RegisterFlagCompletionFunc("theme", completeThemeFlag)

	rootCmd.AddCommand(traceCmd)
}

func traceExportOptions() (trace.ExportOptions, error) {
	metadata := make(map[string]string, len(traceMetadata))
	for _, entry := range traceMetadata {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return trace.ExportOptions{}, errors.WrapValidationError("--meta values must use key=value format")
		}
		metadata[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return trace.ExportOptions{
		Comments:        traceComments,
		SessionMetadata: metadata,
	}, nil
}
