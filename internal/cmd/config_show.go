// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dotandev/glassbox/internal/config"
	"github.com/spf13/cobra"
)

var configShowCmd = &cobra.Command{
	Use:     "config show",
	Short:   "Print the active Glassbox configuration and its source",
	GroupID: "utility",
	Long: `Print the active Glassbox configuration resolved from all sources.

Configuration is loaded in the following precedence order (highest wins):
  1. CLI flags
  2. Environment variables (GLASSBOX_*)
  3. Repository-local config  (.glassbox.toml  or  .Glassbox.toml)
  4. Home directory config    (~/.glassbox/config.toml  or  ~/.Glassbox.toml)
  5. Built-in defaults

Config file locations searched (first match wins):
  .glassbox.toml                  (current directory)
  .Glassbox.toml                  (current directory)
  ~/.glassbox/config.toml         (home directory, XDG-style)
  ~/.Glassbox.toml                (home directory, legacy)
  /etc/Glassbox/config.toml       (system-wide)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Active configuration:")
		fmt.Fprintln(cmd.OutOrStdout(), cfg.String())

		// Report which config file was loaded, if any.
		if src := config.ActiveConfigFile(); src != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "\nLoaded from: %s\n", src)
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "\nNo config file found — using defaults and environment variables.")
		}

		return nil
	},
}

var configCmd = &cobra.Command{
	Use:     "config",
	Short:   "Manage Glassbox configuration",
	GroupID: "utility",
}

func init() {
	configCmd.AddCommand(configShowCmd)
	rootCmd.AddCommand(configCmd)
}

// defaultConfigFilePath returns the canonical ~/.glassbox/config.toml path.
func defaultConfigFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".glassbox", "config.toml"), nil
}
