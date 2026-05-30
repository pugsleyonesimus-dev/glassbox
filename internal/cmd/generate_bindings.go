// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dotandev/glassbox/internal/bindings"
	"github.com/spf13/cobra"
)

var (
	bindingsOutput       string
	bindingsContractID   string
	bindingsNetwork      string
	bindingsPackage      string
	bindingsDebugMeta    bool
	bindingsWasmSrcPath  string
)

var generateBindingsCmd = &cobra.Command{
	Use:   "generate-bindings <wasm-file>",
	Short: "Generate TypeScript bindings for a Soroban smart contract",
	Long: `Generate strongly-typed TypeScript client bindings from a Soroban smart contract.

This command extracts the contract specification from the WASM file and generates
a TypeScript client that provides:

  • Type-safe method calls with full Soroban type mapping
  • ABI metadata objects for each contract function (name, inputs, outputs, source hints)
  • Optional withDebugMetadata() wrappers that attach metadata to call results
  • Glassbox integration for simulation and debugging

Generated files:
  types.ts               – TypeScript interfaces, enums, and error classes
  metadata.ts            – Per-function ABI descriptors with source-location hints
  client.ts              – Strongly-typed async client class
  Glassbox-integration.ts – ErstSimulator wrapper for local simulation
  index.ts               – Barrel export
  package.json           – npm package manifest
  README.md              – Usage documentation

Examples:
  glassbox generate-bindings contract.wasm
  glassbox generate-bindings --output ./src/bindings --package my-contract contract.wasm
  glassbox generate-bindings --debug-metadata --wasm-source ./contract.wasm contract.wasm`,
	Args: cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return validateGenerateBindingsArgs(bindingsNetwork, args[0], bindingsOutput)
	},
	RunE: runGenerateBindings,
}

func runGenerateBindings(_ *cobra.Command, args []string) error {
	wasmPath := args[0]

	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return fmt.Errorf("failed to read WASM file: %w", err)
	}

	if bindingsOutput == "" {
		bindingsOutput = "."
	}

	if bindingsPackage == "" {
		base := filepath.Base(wasmPath)
		bindingsPackage = base[:len(base)-len(filepath.Ext(base))]
	}

	wasmSrc := bindingsWasmSrcPath
	if wasmSrc == "" && bindingsDebugMeta {
		wasmSrc = wasmPath
	}

	config := bindings.GeneratorConfig{
		WasmBytes:        wasmBytes,
		OutputDir:        bindingsOutput,
		PackageName:      bindingsPackage,
		ContractID:       bindingsContractID,
		Network:          bindingsNetwork,
		IncludeDebugMeta: bindingsDebugMeta,
		WasmSourcePath:   wasmSrc,
	}

	generator := bindings.NewGenerator(config)
	files, err := generator.Generate()
	if err != nil {
		return fmt.Errorf("failed to generate bindings: %w", err)
	}

	for _, file := range files {
		fullPath := filepath.Join(bindingsOutput, file.Path)

		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		if err := os.WriteFile(fullPath, []byte(file.Content), 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", fullPath, err)
		}

		fmt.Printf("Generated: %s\n", fullPath)
	}

	fmt.Printf("\n[OK] TypeScript bindings generated successfully\n")
	fmt.Printf("Package: %s\n", bindingsPackage)
	fmt.Printf("Output:  %s\n", bindingsOutput)
	if bindingsDebugMeta {
		fmt.Println("Debug metadata: enabled (metadata.ts + withDebugMetadata option)")
	}

	return nil
}

func init() {
	generateBindingsCmd.Flags().StringVarP(&bindingsOutput, "output", "o", "",
		"Output directory (defaults to current directory)")
	generateBindingsCmd.Flags().StringVarP(&bindingsPackage, "package", "p", "",
		"Package name (defaults to WASM filename without extension)")
	generateBindingsCmd.Flags().StringVar(&bindingsContractID, "contract-id", "",
		"Contract ID for network calls")
	generateBindingsCmd.Flags().StringVarP(&bindingsNetwork, "network", "n", "testnet",
		"Stellar network (testnet, mainnet, futurenet)")
	generateBindingsCmd.Flags().BoolVar(&bindingsDebugMeta, "debug-metadata", false,
		"Generate ABI metadata objects and withDebugMetadata() wrappers")
	generateBindingsCmd.Flags().StringVar(&bindingsWasmSrcPath, "wasm-source", "",
		"Source path hint embedded in debug metadata (defaults to the WASM file path when --debug-metadata is set)")

	rootCmd.AddCommand(generateBindingsCmd)
}
