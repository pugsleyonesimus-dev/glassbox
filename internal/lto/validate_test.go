// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package lto

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── validateManifest ────────────────────────────────────────────────────────

func TestValidateManifest_CleanManifest(t *testing.T) {
	content := `
[package]
name = "my-contract"

[profile.release]
lto = false
debug = 1
codegen-units = 1
`
	results := validateManifest("Cargo.toml", content)
	assert.Empty(t, results, "clean manifest should produce no results")
}

func TestValidateManifest_FatLTO_Error(t *testing.T) {
	content := `
[profile.release]
lto = true
debug = 1
`
	results := validateManifest("Cargo.toml", content)
	ltoRes := filterByField(results, "lto")
	require.Len(t, ltoRes, 1)
	assert.Equal(t, "error", ltoRes[0].Severity)
	assert.Equal(t, "lto", ltoRes[0].Field)
	assert.Equal(t, "release", ltoRes[0].Profile)
	assert.NotEmpty(t, ltoRes[0].Fix)
}

func TestValidateManifest_FatStringLTO_Error(t *testing.T) {
	content := `
[profile.release]
lto = "fat"
debug = 2
`
	results := validateManifest("Cargo.toml", content)
	ltoRes := filterByField(results, "lto")
	require.Len(t, ltoRes, 1)
	assert.Equal(t, "error", ltoRes[0].Severity)
}

func TestValidateManifest_ThinLTO_Warning(t *testing.T) {
	content := `
[profile.release]
lto = "thin"
debug = 1
`
	results := validateManifest("Cargo.toml", content)
	ltoRes := filterByField(results, "lto")
	require.Len(t, ltoRes, 1)
	assert.Equal(t, "warning", ltoRes[0].Severity)
}

func TestValidateManifest_DebugFalse_Error(t *testing.T) {
	content := `
[profile.release]
lto = false
debug = false
`
	results := validateManifest("Cargo.toml", content)
	dbgRes := filterByField(results, "debug")
	require.Len(t, dbgRes, 1)
	assert.Equal(t, "error", dbgRes[0].Severity)
}

func TestValidateManifest_DebugZero_Error(t *testing.T) {
	content := `
[profile.release]
lto = false
debug = 0
`
	results := validateManifest("Cargo.toml", content)
	dbgRes := filterByField(results, "debug")
	require.Len(t, dbgRes, 1)
	assert.Equal(t, "error", dbgRes[0].Severity)
}

func TestValidateManifest_DebugAbsent_Warning(t *testing.T) {
	content := `
[profile.release]
lto = false
`
	results := validateManifest("Cargo.toml", content)
	dbgRes := filterByField(results, "debug")
	require.Len(t, dbgRes, 1)
	assert.Equal(t, "warning", dbgRes[0].Severity)
}

func TestValidateManifest_DebugValid_NoWarning(t *testing.T) {
	for _, val := range []string{"1", "2", "true"} {
		content := "[profile.release]\nlto = false\ndebug = " + val + "\n"
		results := validateManifest("Cargo.toml", content)
		dbgRes := filterByField(results, "debug")
		assert.Empty(t, dbgRes, "debug = %s should not produce a finding", val)
	}
}

func TestValidateManifest_DebugNotCheckedForDev(t *testing.T) {
	content := `
[profile.dev]
debug = false
`
	results := validateManifest("Cargo.toml", content)
	dbgRes := filterByField(results, "debug")
	assert.Empty(t, dbgRes, "debug is only checked for the release profile")
}

func TestValidateManifest_CodegenUnitsMultiple_Warning(t *testing.T) {
	content := `
[profile.release]
lto = false
debug = 1
codegen-units = 16
`
	results := validateManifest("Cargo.toml", content)
	cuRes := filterByField(results, "codegen-units")
	require.Len(t, cuRes, 1)
	assert.Equal(t, "warning", cuRes[0].Severity)
}

func TestValidateManifest_CodegenUnitsOne_NoWarning(t *testing.T) {
	content := `
[profile.release]
lto = false
debug = 1
codegen-units = 1
`
	results := validateManifest("Cargo.toml", content)
	cuRes := filterByField(results, "codegen-units")
	assert.Empty(t, cuRes)
}

func TestValidateManifest_SplitDebuginfoOff_Warning(t *testing.T) {
	content := `
[profile.release]
lto = false
debug = 1
split-debuginfo = "off"
`
	results := validateManifest("Cargo.toml", content)
	sdRes := filterByField(results, "split-debuginfo")
	require.Len(t, sdRes, 1)
	assert.Equal(t, "warning", sdRes[0].Severity)
}

func TestValidateManifest_SplitDebuginfoPacked_Warning(t *testing.T) {
	content := `
[profile.release]
lto = false
debug = 2
split-debuginfo = "packed"
`
	results := validateManifest("Cargo.toml", content)
	sdRes := filterByField(results, "split-debuginfo")
	require.Len(t, sdRes, 1)
	assert.Equal(t, "warning", sdRes[0].Severity)
}

func TestValidateManifest_SplitDebuginfoUnpacked_NoWarning(t *testing.T) {
	content := `
[profile.release]
lto = false
debug = 1
split-debuginfo = "unpacked"
`
	results := validateManifest("Cargo.toml", content)
	sdRes := filterByField(results, "split-debuginfo")
	assert.Empty(t, sdRes)
}

func TestValidateManifest_MultipleProfiles(t *testing.T) {
	content := `
[profile.release]
lto = true
debug = 1

[profile.bench]
lto = "fat"
debug = 0
`
	results := validateManifest("Cargo.toml", content)
	ltoRes := filterByField(results, "lto")
	assert.Len(t, ltoRes, 2, "both profiles with fat LTO should be reported")
}

// ─── parseWorkspaceMembers ────────────────────────────────────────────────────

func TestParseWorkspaceMembers_MultiLine(t *testing.T) {
	content := `
[workspace]
members = [
    "contracts/token",
    "contracts/nft",
]
`
	members := parseWorkspaceMembers(content, "/project")
	assert.Equal(t, []string{"/project/contracts/token", "/project/contracts/nft"}, members)
}

func TestParseWorkspaceMembers_InlineArray(t *testing.T) {
	content := `[workspace]
members = ["contracts/a", "contracts/b"]
`
	members := parseWorkspaceMembers(content, "/root")
	assert.Equal(t, []string{"/root/contracts/a", "/root/contracts/b"}, members)
}

func TestParseWorkspaceMembers_NoWorkspace(t *testing.T) {
	content := `[package]
name = "my-contract"
`
	members := parseWorkspaceMembers(content, "/root")
	assert.Empty(t, members)
}

// ─── ValidateCargoProject ─────────────────────────────────────────────────────

func TestValidateCargoProject_SingleManifest(t *testing.T) {
	dir := t.TempDir()
	content := "[profile.release]\nlto = false\ndebug = 1\ncodegen-units = 1\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(content), 0644))

	results, err := ValidateCargoProject(dir)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestValidateCargoProject_MissingRoot(t *testing.T) {
	_, err := ValidateCargoProject(t.TempDir())
	assert.Error(t, err, "should fail when root Cargo.toml is absent")
}

func TestValidateCargoProject_WorkspaceMemberIssue(t *testing.T) {
	dir := t.TempDir()
	rootToml := "[workspace]\nmembers = [\n    \"contracts/token\",\n]\n"
	memberToml := "[profile.release]\nlto = true\ndebug = 1\n"

	require.NoError(t, os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(rootToml), 0644))
	contractDir := filepath.Join(dir, "contracts", "token")
	require.NoError(t, os.MkdirAll(contractDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(contractDir, "Cargo.toml"), []byte(memberToml), 0644))

	results, err := ValidateCargoProject(dir)
	require.NoError(t, err)

	ltoRes := filterByField(results, "lto")
	require.NotEmpty(t, ltoRes)
	assert.Equal(t, "error", ltoRes[0].Severity)
}

func TestValidateCargoProject_SubdirNotInWorkspace(t *testing.T) {
	dir := t.TempDir()
	rootToml := "[package]\nname = \"root\"\n"
	subToml := "[profile.release]\nlto = \"fat\"\ndebug = 0\n"

	require.NoError(t, os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(rootToml), 0644))
	subDir := filepath.Join(dir, "sibling-contract")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "Cargo.toml"), []byte(subToml), 0644))

	results, err := ValidateCargoProject(dir)
	require.NoError(t, err)
	// Both fat LTO (error) and debug=0 (error) should be reported for the subdir
	assert.NotEmpty(t, results)
}

// ─── helper ──────────────────────────────────────────────────────────────────

func filterByField(results []ValidationResult, field string) []ValidationResult {
	var out []ValidationResult
	for _, r := range results {
		if r.Field == field {
			out = append(out, r)
		}
	}
	return out
}
