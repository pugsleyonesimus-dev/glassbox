// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadFromFile_XDGStylePath verifies that ~/.glassbox/config.toml is
// loaded when it exists.
func TestLoadFromFile_XDGStylePath(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".glassbox")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfgFile := filepath.Join(cfgDir, "config.toml")
	if err := os.WriteFile(cfgFile, []byte("rpc_url = \"https://xdg.example.com\"\n"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", home)

	cfg := &Config{}
	if err := cfg.loadFromFile(); err != nil {
		t.Fatalf("loadFromFile: %v", err)
	}
	if cfg.RpcUrl != "https://xdg.example.com" {
		t.Errorf("RpcUrl = %q, want https://xdg.example.com", cfg.RpcUrl)
	}
	if ActiveConfigFile() != cfgFile {
		t.Errorf("ActiveConfigFile = %q, want %q", ActiveConfigFile(), cfgFile)
	}
}

// TestLoadFromFile_XDGTakesPrecedenceOverLegacy verifies that
// ~/.glassbox/config.toml takes precedence over ~/.Glassbox.toml.
func TestLoadFromFile_XDGTakesPrecedenceOverLegacy(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".glassbox")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"),
		[]byte("rpc_url = \"https://xdg.example.com\"\n"), 0600); err != nil {
		t.Fatalf("write xdg: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".Glassbox.toml"),
		[]byte("rpc_url = \"https://legacy.example.com\"\n"), 0600); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", home)

	cfg := &Config{}
	if err := cfg.loadFromFile(); err != nil {
		t.Fatalf("loadFromFile: %v", err)
	}
	if cfg.RpcUrl != "https://xdg.example.com" {
		t.Errorf("RpcUrl = %q, want XDG path to win", cfg.RpcUrl)
	}
}

// TestLoadFromFile_LocalOverridesHome verifies that a local .glassbox.toml
// takes precedence over the home-directory config.
func TestLoadFromFile_LocalOverridesHome(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()

	cfgDir := filepath.Join(home, ".glassbox")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"),
		[]byte("rpc_url = \"https://home.example.com\"\n"), 0600); err != nil {
		t.Fatalf("write home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, ".glassbox.toml"),
		[]byte("rpc_url = \"https://local.example.com\"\n"), 0600); err != nil {
		t.Fatalf("write local: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", home)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir) //nolint:errcheck
	if err := os.Chdir(project); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	cfg := &Config{}
	if err := cfg.loadFromFile(); err != nil {
		t.Fatalf("loadFromFile: %v", err)
	}
	if cfg.RpcUrl != "https://local.example.com" {
		t.Errorf("RpcUrl = %q, want local config to win", cfg.RpcUrl)
	}
}

// TestActiveConfigFile_NoFile verifies that ActiveConfigFile returns empty
// when no config file is found.
func TestActiveConfigFile_NoFile(t *testing.T) {
	home := t.TempDir()

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", home)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir) //nolint:errcheck
	if err := os.Chdir(home); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	cfg := &Config{}
	_ = cfg.loadFromFile()
	if ActiveConfigFile() != "" {
		t.Errorf("ActiveConfigFile = %q, want empty when no file found", ActiveConfigFile())
	}
}

// TestLoadFromFile_InvalidTOML verifies that a malformed config value returns
// an error rather than silently using defaults.
func TestLoadFromFile_InvalidTOML(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".glassbox")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"),
		[]byte("crash_reporting = maybe\n"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", home)

	cfg := &Config{}
	err := cfg.loadFromFile()
	if err == nil {
		t.Error("expected error for invalid TOML value, got nil")
	}
}
