// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"

	"github.com/dotandev/glassbox/internal/config"
	"github.com/spf13/cobra"
)

var telemetryCmd = &cobra.Command{
    Use:   "telemetry",
    Short: "Show or control telemetry state",
    Long:  "Shows whether telemetry is enabled and provides helpers to disable it for the current session.",
    RunE: func(cmd *cobra.Command, args []string) error {
        // Check environment first
        env := os.Getenv("GLASSBOX_TELEMETRY")
        cfgEnabled := false
        cfgEndpoint := ""
        if cfg, err := config.Load(); err == nil {
            cfgEnabled = cfg.TelemetryEnabled
            cfgEndpoint = cfg.TelemetryEndpoint
        }

        fmt.Println("Telemetry state:")
        if env != "" {
            fmt.Printf("- Environment override (GLASSBOX_TELEMETRY): %s\n", env)
        } else {
            fmt.Printf("- Env override: none\n")
        }
        fmt.Printf("- Config opt-in: %v\n", cfgEnabled)
        if cfgEndpoint != "" {
            fmt.Printf("- Config endpoint: %s\n", cfgEndpoint)
        }

        fmt.Println("\nTo disable telemetry for the current shell session, run:")
        fmt.Println("  export GLASSBOX_TELEMETRY=false")
        return nil
    },
}

func init() {
    rootCmd.AddCommand(telemetryCmd)
}
