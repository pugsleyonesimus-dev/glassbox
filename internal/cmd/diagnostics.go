// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/dotandev/glassbox/internal/cache"
	"github.com/dotandev/glassbox/internal/config"
	"github.com/dotandev/glassbox/internal/plugin"
	"github.com/dotandev/glassbox/internal/version"
	"github.com/spf13/cobra"
)

var diagnosticsCmd = &cobra.Command{
	Use:     "diagnostics",
	Aliases: []string{"diag", "dashboard"},
	GroupID: "utility",
	Short:   "Comprehensive diagnostics dashboard for Glassbox",
	Long: `Display a unified dashboard with health and status for Glassbox.

This command aggregates data from:
  - System and version information
  - RPC endpoint health
  - Cache status
  - Configuration
  - Plugins

Use --json for machine-readable output.`,
	Args: cobra.NoArgs,
	RunE: runDiagnostics,
}

type DiagnosticsOutput struct {
	Version       string              `json:"version"`
	Timestamp     time.Time           `json:"timestamp"`
	System        SystemInfo          `json:"system"`
	RPC           []RPCStatus         `json:"rpc"`
	Cache         CacheStatus         `json:"cache"`
	Config        ConfigSummary       `json:"config"`
	Plugins       []PluginSummary     `json:"plugins"`
	OverallHealth string              `json:"overall_health"`
}

type SystemInfo struct {
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	HomeDir string `json:"home_dir"`
}

type RPCStatus struct {
	URL      string        `json:"url"`
	Status   string        `json:"status"`
	Latency  time.Duration `json:"latency_ms"`
	Error    string        `json:"error,omitempty"`
	Healthy  bool          `json:"healthy"`
}

type CacheStatus struct {
	Directory string `json:"directory"`
	Size      string `json:"size"`
	FileCount int    `json:"file_count"`
	MaxSize   string `json:"max_size"`
	Healthy   bool   `json:"healthy"`
}

type ConfigSummary struct {
	Source  string `json:"source,omitempty"`
	Network string `json:"network"`
	RPCURL  string `json:"rpc_url"`
	Healthy bool   `json:"healthy"`
}

type PluginSummary struct {
	Name         string             `json:"name"`
	Version      string             `json:"version"`
	Capabilities []plugin.Capability `json:"capabilities"`
	Healthy      bool               `json:"healthy"`
}

func runDiagnostics(cmd *cobra.Command, args []string) error {
	w := cmd.OutOrStdout()
	output := DiagnosticsOutput{
		Version:   version.Version,
		Timestamp: time.Now(),
	}

	// System Info
	output.System = SystemInfo{
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
	}
	home, err := os.UserHomeDir()
	if err == nil {
		output.System.HomeDir = home
	}

	// Load config
	cfg, err := config.Load()
	if err == nil {
		configSource := config.ActiveConfigFile()
		output.Config = ConfigSummary{
			Source:  configSource,
			Network: string(cfg.Network),
			RPCURL:  cfg.RpcUrl,
			Healthy: true,
		}
	}

	// RPC Health Check
	var rpcURLs []string
	if err == nil {
		if len(cfg.SorobanRpcUrls) > 0 {
			rpcURLs = cfg.SorobanRpcUrls
		} else if len(cfg.RpcUrls) > 0 {
			rpcURLs = cfg.RpcUrls
		} else if cfg.RpcUrl != "" {
			rpcURLs = []string{cfg.RpcUrl}
		}
	}
	timeout := 15 * time.Second
	if cfg != nil && cfg.RequestTimeout > 0 {
		timeout = time.Duration(cfg.RequestTimeout) * time.Second
	}
	client := &http.Client{Timeout: timeout}
	output.RPC = make([]RPCStatus, 0, len(rpcURLs))
	for _, url := range rpcURLs {
		url = strings.TrimSpace(url)
		if url == "" {
			continue
		}
		rpcStatus := RPCStatus{URL: url, Status: "[OK]", Healthy: true}
		start := time.Now()
		resp, err := client.Get(url)
		if err != nil {
			rpcStatus.Status = "[FAIL]"
			rpcStatus.Error = err.Error()
			rpcStatus.Healthy = false
		} else {
			resp.Body.Close()
			if resp.StatusCode >= 400 {
				rpcStatus.Status = "[FAIL]"
				rpcStatus.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
				rpcStatus.Healthy = false
			}
		}
		rpcStatus.Latency = time.Since(start).Round(time.Millisecond)
		output.RPC = append(output.RPC, rpcStatus)
	}

	// Cache Status
	cacheDir := getCacheDir()
	manager := cache.NewManager(cacheDir, cache.DefaultConfig())
	output.Cache.Directory = cacheDir
	size, err := manager.GetCacheSize()
	if err == nil {
		output.Cache.Size = formatBytes(size)
		output.Cache.MaxSize = formatBytes(cache.DefaultConfig().MaxSizeBytes)
		output.Cache.Healthy = size <= cache.DefaultConfig().MaxSizeBytes
	}
	files, err := manager.ListCachedFiles()
	if err == nil {
		output.Cache.FileCount = len(files)
	}

	// Plugins
	pluginDir := resolvePluginDir("")
	manifests, _ := plugin.DiscoverManifests(pluginDir)
	output.Plugins = make([]PluginSummary, 0, len(manifests))
	for _, m := range manifests {
		output.Plugins = append(output.Plugins, PluginSummary{
			Name:         m.Name,
			Version:      m.Version,
			Capabilities: m.Capabilities,
			Healthy:      true,
		})
	}

	// Overall Health
	output.OverallHealth = "Healthy"
	for _, r := range output.RPC {
		if !r.Healthy {
			output.OverallHealth = "Degraded"
			break
		}
	}
	if !output.Cache.Healthy {
		output.OverallHealth = "Degraded"
	}

	// Print Dashboard
	printDiagnosticsDashboard(w, output)

	return nil
}

func printDiagnosticsDashboard(w io.Writer, out DiagnosticsOutput) {
	fmt.Fprintln(w("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Fprintln(w("║                    GLASSBOX DIAGNOSTICS                       ║")
	fmt.Fprintln(w("╚═══════════════════════════════════════════════════════════════╝")
	fmt.Fprintln(w()
	fmt.Fprintf(w("Version:     %s\n", out.Version)
	fmt.Fprintf(w("Timestamp:   %s\n", out.Timestamp.Format(time.RFC1123))
	fmt.Fprintf(w("Overall:     ")
	if out.OverallHealth == "Healthy" {
		fmt.Fprintf(w("\033[32m%s\033[0m\n", out.OverallHealth)
	} else {
		fmt.Fprintf(w("\033[33m%s\033[0m\n", out.OverallHealth)
	}
	fmt.Fprintln(w()

	// Section: System
	fmt.Fprintln(w("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w("SYSTEM INFO")
	fmt.Fprintln(w("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintf(w("OS:          %s\n", out.System.OS)
	fmt.Fprintf(w("Arch:        %s\n", out.System.Arch)
	if out.System.HomeDir != "" {
		fmt.Fprintf(w("Home:        %s\n", out.System.HomeDir)
	}
	fmt.Fprintln(w()

	// Section: RPC
	fmt.Fprintln(w("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w("RPC ENDPOINTS")
	fmt.Fprintln(w("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if len(out.RPC) == 0 {
		fmt.Fprintln(w("No RPC endpoints configured")
	} else {
		for _, r := range out.RPC {
			if r.Healthy {
				fmt.Fprintf(w("  \033[32m[OK]\033[0m %s\n", r.URL)
				fmt.Fprintf(w("      Latency: %v\n", r.Latency)
			} else {
				fmt.Fprintf(w("  \033[31m[FAIL]\033[0m %s\n", r.URL)
				fmt.Fprintf(w("      Error: %s\n", r.Error)
			}
			fmt.Fprintln(w()
		}
	}

	// Section: Cache
	fmt.Fprintln(w("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w("CACHE STATUS")
	fmt.Fprintln(w("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintf(w("Directory:   %s\n", out.Cache.Directory)
	fmt.Fprintf(w("Size:        %s / %s\n", out.Cache.Size, out.Cache.MaxSize)
	fmt.Fprintf(w("Files:       %d\n", out.Cache.FileCount)
	if out.Cache.Healthy {
		fmt.Fprintf(w("Status:      \033[32mHealthy\033[0m\n")
	} else {
		fmt.Fprintf(w("Status:      \033[33mOver limit\033[0m\n")
	}
	fmt.Fprintln(w()

	// Section: Config
	fmt.Fprintln(w("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w("CONFIGURATION")
	fmt.Fprintln(w("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if out.Config.Source != "" {
		fmt.Fprintf(w("Source:      %s\n", out.Config.Source)
	}
	if out.Config.Network != "" {
		fmt.Fprintf(w("Network:     %s\n", out.Config.Network)
	}
	if out.Config.RPCURL != "" {
		fmt.Fprintf(w("RPC URL:     %s\n", out.Config.RPCURL)
	}
	fmt.Fprintln(w()

	// Section: Plugins
	fmt.Fprintln(w("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w("PLUGINS")
	fmt.Fprintln(w("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if len(out.Plugins) == 0 {
		fmt.Fprintln(w("No plugins discovered")
	} else {
		for _, p := range out.Plugins {
			fmt.Fprintf(w("  %s (v%s)\n", p.Name, p.Version)
			if len(p.Capabilities) > 0 {
				caps := make([]string, len(p.Capabilities))
				for i, c := range p.Capabilities {
					caps[i] = string(c)
				}
				fmt.Fprintf(w("    Capabilities: %s\n", strings.Join(caps, ", "))
			}
			fmt.Fprintln(w()
		}
	}
	fmt.Fprintln(w()
	fmt.Fprintln(w("Tip: Run 'glassbox help' to see all commands")
}

func init() {
	rootCmd.AddCommand(diagnosticsCmd)
}
