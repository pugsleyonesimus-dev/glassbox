// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/dotandev/glassbox/internal/errors"
)

// validNetworkValues lists all accepted --network flag values across commands.
var validNetworkValues = map[string]bool{
	"testnet":    true,
	"mainnet":    true,
	"futurenet":  true,
	"standalone": true,
	"public":     true,
}

// validateNetwork returns a descriptive error when the network value is not
// one of the accepted values, including the invalid value and a suggestion.
func validateNetwork(network string) error {
	if network == "" {
		return nil // optional in some commands
	}
	if validNetworkValues[strings.ToLower(network)] {
		return nil
	}
	valid := []string{"testnet", "mainnet", "futurenet", "standalone", "public"}
	return errors.WrapValidationError(fmt.Sprintf(
		"invalid network %q — must be one of: %s",
		network, strings.Join(valid, ", "),
	))
}

// validateFilePath returns an error when path is non-empty but the file does
// not exist or is not readable.
func validateFilePath(flag, path string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return errors.WrapValidationError(fmt.Sprintf(
				"--%s: file not found: %q", flag, path,
			))
		}
		return errors.WrapValidationError(fmt.Sprintf(
			"--%s: cannot access %q: %v", flag, path, err,
		))
	}
	return nil
}

// validatePositiveInt returns an error when val is set (non-zero) but not positive.
func validatePositiveInt(flag string, val int) error {
	if val < 0 {
		return errors.WrapValidationError(fmt.Sprintf(
			"--%s must be a positive integer, got %d", flag, val,
		))
	}
	return nil
}

// validateMutuallyExclusive returns an error when more than one of the named
// flags is set, listing all conflicting flags in the message.
func validateMutuallyExclusive(set map[string]bool, flags ...string) error {
	var active []string
	for _, f := range flags {
		if set[f] {
			active = append(active, "--"+f)
		}
	}
	if len(active) > 1 {
		return errors.WrapValidationError(fmt.Sprintf(
			"flags %s are mutually exclusive — provide only one",
			strings.Join(active, " and "),
		))
	}
	return nil
}

// validateGenerateBindingsArgs validates all flags for the generate-bindings
// command at parse time before any business logic runs.
func validateGenerateBindingsArgs(network, wasmPath, outputDir string) error {
	if err := validateNetwork(network); err != nil {
		return err
	}
	if wasmPath != "" {
		if err := validateFilePath("wasm", wasmPath); err != nil {
			return err
		}
	}
	if outputDir != "" {
		if info, err := os.Stat(outputDir); err == nil && !info.IsDir() {
			return errors.WrapValidationError(fmt.Sprintf(
				"--output %q exists but is not a directory", outputDir,
			))
		}
	}
	return nil
}

// validateAuditSignArgs validates all flags for the audit:sign command at
// parse time, catching mutually exclusive inputs and missing required options.
func validateAuditSignArgs(payload, payloadFile, provider string) error {
	if payload != "" && payloadFile != "" {
		return errors.WrapValidationError(
			"--payload and --payload-file are mutually exclusive — provide only one",
		)
	}
	if provider != "" {
		supported := []string{"software", "pkcs11"}
		found := false
		for _, s := range supported {
			if strings.EqualFold(provider, s) {
				found = true
				break
			}
		}
		if !found {
			return errors.WrapValidationError(fmt.Sprintf(
				"--signing-provider %q is not supported — must be one of: %s",
				provider, strings.Join(supported, ", "),
			))
		}
	}
	return nil
}
