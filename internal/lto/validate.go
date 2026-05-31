// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package lto

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidationResult holds a single finding from Cargo.toml validation.
type ValidationResult struct {
	// File is the Cargo.toml path where the issue was found.
	File string
	// Profile is the build profile where the issue was found.
	// May be empty for workspace-level issues.
	Profile string
	// Field is the Cargo.toml key that triggered the finding.
	Field string
	// Severity is "error" or "warning".
	Severity string
	// Message is a human-readable description of the issue.
	Message string
	// Fix is a suggested Cargo.toml snippet to resolve the issue.
	Fix string
}

// profileSettings tracks the keys parsed from a single [profile.*] section.
type profileSettings struct {
	hasLTO              bool
	ltoValue            string
	hasDebug            bool
	debugValue          string
	hasCodegenUnits     bool
	codegenUnitsValue   string
	hasSplitDebuginfo   bool
	splitDebuginfoValue string
}

// ValidateCargoProject inspects the Cargo project at dir and returns a list of
// validation results covering LTO, debug info, codegen units, split-debuginfo,
// and workspace layout issues that affect source mapping accuracy.
func ValidateCargoProject(dir string) ([]ValidationResult, error) {
	var results []ValidationResult

	rootToml := filepath.Join(dir, "Cargo.toml")
	data, err := os.ReadFile(rootToml)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", rootToml, err)
	}
	content := string(data)

	results = append(results, validateManifest(rootToml, content)...)

	// Discover workspace members and validate each one.
	members := parseWorkspaceMembers(content, dir)
	coveredByWorkspace := make(map[string]bool, len(members))
	for _, memberDir := range members {
		coveredByWorkspace[memberDir] = true
		memberToml := filepath.Join(memberDir, "Cargo.toml")
		memberData, readErr := os.ReadFile(memberToml)
		if readErr != nil {
			continue
		}
		results = append(results, validateManifest(memberToml, string(memberData))...)
	}

	// Also scan direct subdirectories for Cargo.toml (common Soroban layout
	// where the workspace root and contract directories are siblings).
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subDir := filepath.Join(dir, entry.Name())
		if coveredByWorkspace[subDir] {
			continue
		}
		subToml := filepath.Join(subDir, "Cargo.toml")
		if _, statErr := os.Stat(subToml); statErr != nil {
			continue
		}
		subData, readErr := os.ReadFile(subToml)
		if readErr != nil {
			continue
		}
		results = append(results, validateManifest(subToml, string(subData))...)
	}

	return results, nil
}

// validateManifest checks a single Cargo.toml for source-mapping issues.
func validateManifest(path, content string) []ValidationResult {
	var results []ValidationResult

	lines := strings.Split(content, "\n")
	currentProfile := ""
	inProfile := false

	profiles := make(map[string]*profileSettings)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.HasPrefix(trimmed, "[") {
			inProfile = false
			currentProfile = ""

			if strings.HasPrefix(trimmed, "[profile.") {
				name := strings.TrimPrefix(trimmed, "[profile.")
				name = strings.TrimSuffix(name, "]")
				name = strings.TrimSpace(name)
				currentProfile = name
				inProfile = true
				if profiles[currentProfile] == nil {
					profiles[currentProfile] = &profileSettings{}
				}
			}
			continue
		}

		if !inProfile || !strings.Contains(trimmed, "=") {
			continue
		}

		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		ps := profiles[currentProfile]

		switch key {
		case "lto":
			ps.hasLTO = true
			ps.ltoValue = value
		case "debug":
			ps.hasDebug = true
			ps.debugValue = value
		case "codegen-units":
			ps.hasCodegenUnits = true
			ps.codegenUnitsValue = value
		case "split-debuginfo":
			ps.hasSplitDebuginfo = true
			ps.splitDebuginfoValue = value
		}
	}

	for profName, ps := range profiles {
		results = append(results, checkLTO(path, profName, ps)...)
		results = append(results, checkDebug(path, profName, ps)...)
		results = append(results, checkCodegenUnits(path, profName, ps)...)
		results = append(results, checkSplitDebuginfo(path, profName, ps)...)
	}

	return results
}

func checkLTO(path, profName string, ps *profileSettings) []ValidationResult {
	if !ps.hasLTO {
		return nil
	}
	kind := ParseLTOValue(ps.ltoValue)
	switch kind {
	case LTOFat:
		return []ValidationResult{{
			File:     path,
			Profile:  profName,
			Field:    "lto",
			Severity: "error",
			Message: fmt.Sprintf(
				"[%s] lto = %s (fat LTO) — produces incorrect DWARF source mappings; "+
					"WASM instruction offsets in stack traces will point to wrong source lines",
				profName, ps.ltoValue,
			),
			Fix: "Set lto = false or remove the lto key in [profile." + profName + "]",
		}}
	case LTOThin:
		return []ValidationResult{{
			File:     path,
			Profile:  profName,
			Field:    "lto",
			Severity: "warning",
			Message: fmt.Sprintf(
				"[%s] lto = %s (thin LTO) — may produce inaccurate DWARF source mappings",
				profName, ps.ltoValue,
			),
			Fix: "Consider setting lto = false in [profile." + profName + "] for reliable source-level debugging",
		}}
	}
	return nil
}

func checkDebug(path, profName string, ps *profileSettings) []ValidationResult {
	if profName != "release" {
		return nil
	}
	if !ps.hasDebug {
		return []ValidationResult{{
			File:     path,
			Profile:  profName,
			Field:    "debug",
			Severity: "warning",
			Message:  fmt.Sprintf("[%s] debug setting absent — source mapping may be limited without debug info", profName),
			Fix:      "Add debug = 1 (line tables) or debug = 2 (full DWARF) to [profile.release]",
		}}
	}
	dv := strings.Trim(strings.TrimSpace(ps.debugValue), "\"'")
	if dv == "false" || dv == "0" {
		return []ValidationResult{{
			File:     path,
			Profile:  profName,
			Field:    "debug",
			Severity: "error",
			Message: fmt.Sprintf(
				"[%s] debug = %s — no debug info will be emitted; source mapping is impossible",
				profName, ps.debugValue,
			),
			Fix: "Set debug = 1 (line tables) or debug = 2 (full DWARF) in [profile.release]",
		}}
	}
	return nil
}

func checkCodegenUnits(path, profName string, ps *profileSettings) []ValidationResult {
	if profName != "release" || !ps.hasCodegenUnits {
		return nil
	}
	cu := strings.TrimSpace(ps.codegenUnitsValue)
	if cu != "1" {
		return []ValidationResult{{
			File:     path,
			Profile:  profName,
			Field:    "codegen-units",
			Severity: "warning",
			Message: fmt.Sprintf(
				"[%s] codegen-units = %s — multiple codegen units fragment DWARF info across compilation units",
				profName, ps.codegenUnitsValue,
			),
			Fix: "Set codegen-units = 1 in [profile.release] for deterministic DWARF mappings",
		}}
	}
	return nil
}

func checkSplitDebuginfo(path, profName string, ps *profileSettings) []ValidationResult {
	if !ps.hasSplitDebuginfo {
		return nil
	}
	sdv := strings.Trim(strings.TrimSpace(ps.splitDebuginfoValue), "\"'")
	if sdv == "off" || sdv == "packed" {
		return []ValidationResult{{
			File:     path,
			Profile:  profName,
			Field:    "split-debuginfo",
			Severity: "warning",
			Message: fmt.Sprintf(
				"[%s] split-debuginfo = %s — debuginfo may not be available for source mapping",
				profName, ps.splitDebuginfoValue,
			),
			Fix: `Set split-debuginfo = "unpacked" in [profile.` + profName + `] to keep DWARF sections accessible`,
		}}
	}
	return nil
}

// parseWorkspaceMembers extracts workspace member paths from a root Cargo.toml.
func parseWorkspaceMembers(content, rootDir string) []string {
	var members []string
	lines := strings.Split(content, "\n")
	inWorkspace := false
	inMembers := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.HasPrefix(trimmed, "[") {
			// Leaving the workspace section when we hit any new section header.
			if inWorkspace && trimmed != "[workspace]" {
				inWorkspace = false
				inMembers = false
			}
			if trimmed == "[workspace]" {
				inWorkspace = true
			}
			continue
		}

		if !inWorkspace {
			continue
		}

		if strings.HasPrefix(trimmed, "members") && strings.Contains(trimmed, "=") {
			inMembers = true
			// Handle inline arrays: members = ["a", "b"]
			rest := strings.SplitN(trimmed, "=", 2)[1]
			rest = strings.TrimSpace(rest)
			if strings.Contains(rest, "]") {
				// Single-line array
				rest = strings.Trim(rest, "[]")
				for _, part := range strings.Split(rest, ",") {
					p := strings.Trim(strings.TrimSpace(part), "\"'")
					if p != "" {
						members = append(members, filepath.Join(rootDir, p))
					}
				}
				inMembers = false
			}
			continue
		}

		if inMembers {
			if strings.Contains(trimmed, "]") {
				inMembers = false
				continue
			}
			p := strings.Trim(trimmed, " \t,\"'[]")
			if p != "" && !strings.HasPrefix(p, "#") {
				members = append(members, filepath.Join(rootDir, p))
			}
		}
	}
	return members
}
