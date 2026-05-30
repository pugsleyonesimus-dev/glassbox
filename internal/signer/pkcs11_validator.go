// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package signer

import (
	"context"
	"fmt"
	"os"
	"time"
)

// ValidationResult holds the outcome of a single preflight check step.
type ValidationResult struct {
	// Step is the name of the validation step (e.g. "module_load").
	Step string
	// OK is true when the step passed.
	OK bool
	// Message is a human-readable description of the outcome.
	Message string
	// Remediation is an actionable hint when OK is false.
	Remediation string
}

// PreflightReport is the aggregated result of all PKCS#11 preflight checks.
type PreflightReport struct {
	// Results contains one entry per validation step, in execution order.
	Results []ValidationResult
	// Ready is true only when every step passed.
	Ready bool
}

// pass appends a successful step to the report.
func (r *PreflightReport) pass(step, message string) {
	r.Results = append(r.Results, ValidationResult{
		Step:    step,
		OK:      true,
		Message: message,
	})
}

// fail appends a failed step to the report and marks the report as not ready.
func (r *PreflightReport) fail(step, message, remediation string) {
	r.Results = append(r.Results, ValidationResult{
		Step:        step,
		OK:          false,
		Message:     message,
		Remediation: remediation,
	})
	r.Ready = false
}

// Pkcs11Provider is the interface that the validator uses to interact with a
// PKCS#11 module. It is satisfied by a real module binding and by test mocks.
type Pkcs11Provider interface {
	// LoadModule attempts to load the PKCS#11 shared library at the given path.
	// Returns an error if the file is missing, not executable, or cannot be
	// opened as a PKCS#11 module.
	LoadModule(path string) error

	// Initialize calls C_Initialize on the loaded module.
	Initialize() error

	// GetSlotList returns the list of available slot IDs.
	// If tokenPresent is true, only slots with a token inserted are returned.
	GetSlotList(tokenPresent bool) ([]uint64, error)

	// GetTokenInfo returns a human-readable label for the token in the given slot.
	GetTokenInfo(slotID uint64) (label string, err error)

	// OpenSession opens a read-only serial session on the given slot.
	OpenSession(slotID uint64) (sessionHandle uint64, err error)

	// Login authenticates the user with the given PIN on the open session.
	Login(session uint64, pin string) error

	// FindKey searches for a private key by label or hex ID.
	// Returns the key handle, or an error if no matching key is found.
	FindKey(session uint64, keyLabel, keyIDHex string) (keyHandle uint64, err error)

	// SignTest performs a minimal test signing operation to verify the key is
	// usable. The data slice is a small fixed test vector.
	SignTest(session uint64, keyHandle uint64, data []byte) error

	// CloseSession closes the open session.
	CloseSession(session uint64) error

	// Finalize calls C_Finalize on the module.
	Finalize() error
}

// ValidatorConfig controls the behaviour of the preflight validator.
type ValidatorConfig struct {
	// ModuleTimeout is the maximum time allowed for module initialization.
	// Defaults to 10 seconds.
	ModuleTimeout time.Duration

	// MaxRetries is the number of times to retry a failed module initialization
	// before giving up. Defaults to 2.
	MaxRetries int

	// RetryDelay is the wait between retries. Defaults to 500ms.
	RetryDelay time.Duration
}

// DefaultValidatorConfig returns a ValidatorConfig with sensible defaults.
func DefaultValidatorConfig() ValidatorConfig {
	return ValidatorConfig{
		ModuleTimeout: 10 * time.Second,
		MaxRetries:    2,
		RetryDelay:    500 * time.Millisecond,
	}
}

// Pkcs11Validator performs a series of preflight checks against a PKCS#11
// module before any signing operation is attempted. It surfaces actionable
// errors for each failure mode rather than raw vendor error codes.
type Pkcs11Validator struct {
	cfg      Pkcs11Config
	vcfg     ValidatorConfig
	provider Pkcs11Provider
}

// NewPkcs11Validator creates a validator for the given PKCS#11 configuration.
// provider is the PKCS#11 backend; pass nil to use the default OS-level loader
// (not yet implemented — callers must supply a provider in the current build).
func NewPkcs11Validator(cfg Pkcs11Config, vcfg ValidatorConfig, provider Pkcs11Provider) *Pkcs11Validator {
	return &Pkcs11Validator{
		cfg:      cfg,
		vcfg:     vcfg,
		provider: provider,
	}
}

// Validate runs all preflight checks and returns a PreflightReport. The
// context is used to enforce the overall timeout; individual steps also
// respect ValidatorConfig.ModuleTimeout.
//
// Steps performed:
//  1. module_path   — verify the .so/.dylib/.dll file exists and is readable
//  2. module_load   — load the module with timeout + retry
//  3. slot_enum     — enumerate slots and match by label or index
//  4. token_info    — retrieve and display token metadata
//  5. session_open  — open a PKCS#11 session
//  6. pin_auth      — authenticate with the user PIN
//  7. key_lookup    — locate the signing key by label or ID
//  8. sign_test     — perform a test signing operation
func (v *Pkcs11Validator) Validate(ctx context.Context) *PreflightReport {
	report := &PreflightReport{Ready: true}

	// Step 1: module path
	if !v.checkModulePath(report) {
		return report
	}

	// Step 2: module load (with timeout + retry)
	if !v.checkModuleLoad(ctx, report) {
		return report
	}
	defer func() { _ = v.provider.Finalize() }()

	// Step 3: slot enumeration
	slotID, ok := v.checkSlotEnum(report)
	if !ok {
		return report
	}

	// Step 4: token info
	v.checkTokenInfo(report, slotID)

	// Step 5: session open
	session, ok := v.checkSessionOpen(report, slotID)
	if !ok {
		return report
	}
	defer func() { _ = v.provider.CloseSession(session) }()

	// Step 6: PIN authentication
	if !v.checkPINAuth(report, session) {
		return report
	}

	// Step 7: key lookup
	keyHandle, ok := v.checkKeyLookup(report, session)
	if !ok {
		return report
	}

	// Step 8: test sign
	v.checkSignTest(report, session, keyHandle)

	return report
}

// checkModulePath verifies the module file exists and is readable.
func (v *Pkcs11Validator) checkModulePath(report *PreflightReport) bool {
	path := v.cfg.ModulePath
	if path == "" {
		report.fail("module_path",
			"GLASSBOX_PKCS11_MODULE is not set",
			"set GLASSBOX_PKCS11_MODULE to the path of your PKCS#11 shared library (e.g. /usr/lib/softhsm/libsofthsm2.so)")
		return false
	}

	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		report.fail("module_path",
			fmt.Sprintf("module file not found: %s", path),
			platformModuleHint(path))
		return false
	}
	if err != nil {
		report.fail("module_path",
			fmt.Sprintf("cannot access module file %s: %v", path, err),
			"check file permissions; the process user must have read access to the module")
		return false
	}
	if info.IsDir() {
		report.fail("module_path",
			fmt.Sprintf("%s is a directory, not a shared library", path),
			"GLASSBOX_PKCS11_MODULE must point to a .so/.dylib/.dll file, not a directory")
		return false
	}

	report.pass("module_path", fmt.Sprintf("module file found: %s (%d bytes)", path, info.Size()))
	return true
}

// checkModuleLoad loads the PKCS#11 module with timeout and retry.
func (v *Pkcs11Validator) checkModuleLoad(ctx context.Context, report *PreflightReport) bool {
	timeout := v.vcfg.ModuleTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	maxRetries := v.vcfg.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	retryDelay := v.vcfg.RetryDelay
	if retryDelay == 0 {
		retryDelay = 500 * time.Millisecond
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				report.fail("module_load",
					"context cancelled while waiting to retry module load",
					"check for HSM device connectivity issues")
				return false
			case <-time.After(retryDelay):
			}
		}

		loadCtx, cancel := context.WithTimeout(ctx, timeout)
		err := v.loadWithTimeout(loadCtx)
		cancel()

		if err == nil {
			msg := fmt.Sprintf("module loaded successfully: %s", v.cfg.ModulePath)
			if attempt > 0 {
				msg += fmt.Sprintf(" (after %d retries)", attempt)
			}
			report.pass("module_load", msg)
			return true
		}
		lastErr = err
	}

	report.fail("module_load",
		fmt.Sprintf("failed to load PKCS#11 module after %d attempt(s): %v", maxRetries+1, lastErr),
		"verify the module is a valid PKCS#11 shared library; check OS architecture (amd64/arm64); ensure all module dependencies are installed")
	return false
}

// loadWithTimeout calls LoadModule and Initialize within the given context deadline.
func (v *Pkcs11Validator) loadWithTimeout(ctx context.Context) error {
	type result struct{ err error }
	ch := make(chan result, 1)

	go func() {
		if err := v.provider.LoadModule(v.cfg.ModulePath); err != nil {
			ch <- result{err}
			return
		}
		ch <- result{v.provider.Initialize()}
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("module initialization timed out: %w", ctx.Err())
	case r := <-ch:
		return r.err
	}
}

// checkSlotEnum enumerates slots and selects one by token label or slot index.
func (v *Pkcs11Validator) checkSlotEnum(report *PreflightReport) (uint64, bool) {
	slots, err := v.provider.GetSlotList(true)
	if err != nil {
		report.fail("slot_enum",
			fmt.Sprintf("failed to enumerate slots: %v", err),
			"verify the module is initialized and a token is inserted; run 'pkcs11-tool --list-slots' to check")
		return 0, false
	}

	if len(slots) == 0 {
		report.fail("slot_enum",
			"no slots with tokens found",
			"insert the HSM token; for SoftHSM2 run 'softhsm2-util --init-token --slot 0 --label MyToken --pin 1234 --so-pin 0000'")
		return 0, false
	}

	// Match by token label if configured.
	if v.cfg.TokenLabel != "" {
		for _, slotID := range slots {
			label, infoErr := v.provider.GetTokenInfo(slotID)
			if infoErr != nil {
				continue
			}
			if label == v.cfg.TokenLabel {
				report.pass("slot_enum",
					fmt.Sprintf("found token %q in slot %d (%d slot(s) total)", v.cfg.TokenLabel, slotID, len(slots)))
				return slotID, true
			}
		}
		report.fail("slot_enum",
			fmt.Sprintf("no token with label %q found (checked %d slot(s))", v.cfg.TokenLabel, len(slots)),
			"verify GLASSBOX_PKCS11_TOKEN_LABEL matches the token label exactly; run 'pkcs11-tool --list-slots' to list token labels")
		return 0, false
	}

	// Fall back to slot index.
	idx := v.cfg.SlotIndex
	if idx < 0 || idx >= len(slots) {
		report.fail("slot_enum",
			fmt.Sprintf("slot index %d is out of range (found %d slot(s))", idx, len(slots)),
			fmt.Sprintf("set GLASSBOX_PKCS11_SLOT to a value between 0 and %d", len(slots)-1))
		return 0, false
	}

	report.pass("slot_enum",
		fmt.Sprintf("using slot index %d (slot ID %d, %d slot(s) total)", idx, slots[idx], len(slots)))
	return slots[idx], true
}

// checkTokenInfo retrieves and records token metadata (non-fatal).
func (v *Pkcs11Validator) checkTokenInfo(report *PreflightReport, slotID uint64) {
	label, err := v.provider.GetTokenInfo(slotID)
	if err != nil {
		// Non-fatal: token info is informational only.
		report.pass("token_info", fmt.Sprintf("token info unavailable for slot %d (non-fatal): %v", slotID, err))
		return
	}
	report.pass("token_info", fmt.Sprintf("token label: %q (slot %d)", label, slotID))
}

// checkSessionOpen opens a PKCS#11 session.
func (v *Pkcs11Validator) checkSessionOpen(report *PreflightReport, slotID uint64) (uint64, bool) {
	session, err := v.provider.OpenSession(slotID)
	if err != nil {
		report.fail("session_open",
			fmt.Sprintf("failed to open session on slot %d: %v", slotID, err),
			"check that no other process holds an exclusive session on this slot; verify the token is not write-protected")
		return 0, false
	}
	report.pass("session_open", fmt.Sprintf("session opened on slot %d (handle %d)", slotID, session))
	return session, true
}

// checkPINAuth authenticates with the user PIN.
func (v *Pkcs11Validator) checkPINAuth(report *PreflightReport, session uint64) bool {
	if v.cfg.PIN == "" {
		report.fail("pin_auth",
			"GLASSBOX_PKCS11_PIN is not set",
			"set GLASSBOX_PKCS11_PIN to the user PIN for the token")
		return false
	}

	if err := v.provider.Login(session, v.cfg.PIN); err != nil {
		report.fail("pin_auth",
			fmt.Sprintf("PIN authentication failed: %v", err),
			"verify GLASSBOX_PKCS11_PIN is correct; note that repeated failures may lock the token (use SO PIN to unlock)")
		return false
	}

	report.pass("pin_auth", "PIN authentication successful")
	return true
}

// checkKeyLookup searches for the signing key by label or ID.
func (v *Pkcs11Validator) checkKeyLookup(report *PreflightReport, session uint64) (uint64, bool) {
	if v.cfg.KeyLabel == "" && v.cfg.KeyIDHex == "" {
		report.fail("key_lookup",
			"neither GLASSBOX_PKCS11_KEY_LABEL nor GLASSBOX_PKCS11_KEY_ID is set",
			"set GLASSBOX_PKCS11_KEY_LABEL to the CKA_LABEL of the signing key, or GLASSBOX_PKCS11_KEY_ID to its hex CKA_ID")
		return 0, false
	}

	keyHandle, err := v.provider.FindKey(session, v.cfg.KeyLabel, v.cfg.KeyIDHex)
	if err != nil {
		selector := v.cfg.KeyLabel
		if selector == "" {
			selector = "id:" + v.cfg.KeyIDHex
		}
		report.fail("key_lookup",
			fmt.Sprintf("signing key %q not found: %v", selector, err),
			"verify the key label/ID with 'pkcs11-tool --list-objects --type privkey'; ensure the key has CKA_SIGN=true and is an Ed25519 key")
		return 0, false
	}

	selector := v.cfg.KeyLabel
	if selector == "" {
		selector = "id:" + v.cfg.KeyIDHex
	}
	report.pass("key_lookup", fmt.Sprintf("signing key %q found (handle %d)", selector, keyHandle))
	return keyHandle, true
}

// checkSignTest performs a minimal test signing operation.
func (v *Pkcs11Validator) checkSignTest(report *PreflightReport, session, keyHandle uint64) {
	testData := []byte("glassbox-pkcs11-preflight-test-vector")
	if err := v.provider.SignTest(session, keyHandle, testData); err != nil {
		report.fail("sign_test",
			fmt.Sprintf("test signing operation failed: %v", err),
			"verify the key supports CKM_EDDSA; check that the key has the sign permission (CKA_SIGN=true)")
		return
	}
	report.pass("sign_test", "test signing operation succeeded")
}

// platformModuleHint returns a platform-specific hint for a missing module file.
func platformModuleHint(path string) string {
	hints := map[string]string{
		"/usr/lib/softhsm/libsofthsm2.so":                    "install SoftHSM2: 'apt install softhsm2' (Debian/Ubuntu) or 'brew install softhsm' (macOS)",
		"/usr/lib/x86_64-linux-gnu/softhsm/libsofthsm2.so":   "install SoftHSM2: 'apt install softhsm2' (Debian/Ubuntu)",
		"/usr/local/lib/softhsm/libsofthsm2.so":               "install SoftHSM2: 'brew install softhsm' (macOS) or build from source",
		"/usr/lib/opensc-pkcs11.so":                           "install OpenSC: 'apt install opensc' (Debian/Ubuntu) or 'brew install opensc' (macOS)",
		"/usr/local/lib/opensc-pkcs11.so":                     "install OpenSC: 'brew install opensc' (macOS)",
		"/usr/lib/x86_64-linux-gnu/opensc-pkcs11.so":          "install OpenSC: 'apt install opensc' (Debian/Ubuntu)",
		"/usr/lib/libykcs11.so":                               "install YubiKey PKCS#11: 'apt install ykcs11' (Debian/Ubuntu) or 'brew install yubico-piv-tool' (macOS)",
		"/usr/local/lib/libykcs11.dylib":                      "install YubiKey PKCS#11: 'brew install yubico-piv-tool' (macOS)",
		"/usr/lib/x86_64-linux-gnu/libykcs11.so":              "install YubiKey PKCS#11: 'apt install ykcs11' (Debian/Ubuntu)",
	}

	if hint, ok := hints[path]; ok {
		return hint
	}
	return fmt.Sprintf("verify the path %q is correct for your platform and HSM vendor; check vendor documentation for the module location", path)
}
