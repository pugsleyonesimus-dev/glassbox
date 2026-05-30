// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package signer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

// ─── Mock provider ────────────────────────────────────────────────────────────

// mockPkcs11Provider is a configurable test double for Pkcs11Provider.
// Each field controls the behaviour of the corresponding method.
type mockPkcs11Provider struct {
	loadModuleErr  error
	initializeErr  error
	slots          []uint64
	getSlotListErr error
	tokenLabels    map[uint64]string
	getTokenInfoErr error
	sessionHandle  uint64
	openSessionErr error
	loginErr       error
	keyHandle      uint64
	findKeyErr     error
	signTestErr    error
	finalizeErr    error
	closeSessionErr error
}

func (m *mockPkcs11Provider) LoadModule(_ string) error        { return m.loadModuleErr }
func (m *mockPkcs11Provider) Initialize() error               { return m.initializeErr }
func (m *mockPkcs11Provider) Finalize() error                 { return m.finalizeErr }
func (m *mockPkcs11Provider) CloseSession(_ uint64) error     { return m.closeSessionErr }

func (m *mockPkcs11Provider) GetSlotList(_ bool) ([]uint64, error) {
	return m.slots, m.getSlotListErr
}

func (m *mockPkcs11Provider) GetTokenInfo(slotID uint64) (string, error) {
	if m.getTokenInfoErr != nil {
		return "", m.getTokenInfoErr
	}
	if m.tokenLabels != nil {
		if label, ok := m.tokenLabels[slotID]; ok {
			return label, nil
		}
	}
	return fmt.Sprintf("token-%d", slotID), nil
}

func (m *mockPkcs11Provider) OpenSession(_ uint64) (uint64, error) {
	return m.sessionHandle, m.openSessionErr
}

func (m *mockPkcs11Provider) Login(_ uint64, _ string) error {
	return m.loginErr
}

func (m *mockPkcs11Provider) FindKey(_ uint64, _, _ string) (uint64, error) {
	return m.keyHandle, m.findKeyErr
}

func (m *mockPkcs11Provider) SignTest(_ uint64, _ uint64, _ []byte) error {
	return m.signTestErr
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func happyProvider() *mockPkcs11Provider {
	return &mockPkcs11Provider{
		slots:         []uint64{0},
		tokenLabels:   map[uint64]string{0: "TestToken"},
		sessionHandle: 1,
		keyHandle:     42,
	}
}

func defaultCfg() Pkcs11Config {
	return Pkcs11Config{
		ModulePath: "/fake/libpkcs11.so",
		PIN:        "1234",
		KeyLabel:   "signing-key",
	}
}

func fastVcfg() ValidatorConfig {
	return ValidatorConfig{
		ModuleTimeout: 2 * time.Second,
		MaxRetries:    0,
		RetryDelay:    0,
	}
}

// writeTempModule creates a temporary file to simulate a module on disk.
func writeTempModule(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "libpkcs11-*.so")
	if err != nil {
		t.Fatalf("failed to create temp module: %v", err)
	}
	_, _ = f.WriteString("fake pkcs11 module")
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestPkcs11Validator_HappyPath(t *testing.T) {
	modPath := writeTempModule(t)
	cfg := defaultCfg()
	cfg.ModulePath = modPath

	v := NewPkcs11Validator(cfg, fastVcfg(), happyProvider())
	report := v.Validate(context.Background())

	if !report.Ready {
		for _, r := range report.Results {
			if !r.OK {
				t.Errorf("step %q failed: %s — %s", r.Step, r.Message, r.Remediation)
			}
		}
		t.Fatal("expected all preflight checks to pass")
	}

	steps := make(map[string]bool)
	for _, r := range report.Results {
		steps[r.Step] = r.OK
	}
	for _, expected := range []string{"module_path", "module_load", "slot_enum", "token_info", "session_open", "pin_auth", "key_lookup", "sign_test"} {
		if !steps[expected] {
			t.Errorf("expected step %q to be present and passing", expected)
		}
	}
}

func TestPkcs11Validator_ModulePathMissing(t *testing.T) {
	cfg := defaultCfg()
	cfg.ModulePath = "/nonexistent/libpkcs11.so"

	v := NewPkcs11Validator(cfg, fastVcfg(), happyProvider())
	report := v.Validate(context.Background())

	if report.Ready {
		t.Fatal("expected validation to fail for missing module path")
	}
	assertStepFailed(t, report, "module_path")
}

func TestPkcs11Validator_ModulePathEmpty(t *testing.T) {
	cfg := defaultCfg()
	cfg.ModulePath = ""

	v := NewPkcs11Validator(cfg, fastVcfg(), happyProvider())
	report := v.Validate(context.Background())

	if report.Ready {
		t.Fatal("expected validation to fail for empty module path")
	}
	assertStepFailed(t, report, "module_path")
	assertRemediationContains(t, report, "module_path", "GLASSBOX_PKCS11_MODULE")
}

func TestPkcs11Validator_ModuleLoadFails(t *testing.T) {
	modPath := writeTempModule(t)
	cfg := defaultCfg()
	cfg.ModulePath = modPath

	provider := happyProvider()
	provider.loadModuleErr = errors.New("dlopen: invalid ELF header")

	v := NewPkcs11Validator(cfg, fastVcfg(), provider)
	report := v.Validate(context.Background())

	if report.Ready {
		t.Fatal("expected validation to fail when module load fails")
	}
	assertStepFailed(t, report, "module_load")
}

func TestPkcs11Validator_ModuleLoadRetries(t *testing.T) {
	modPath := writeTempModule(t)
	cfg := defaultCfg()
	cfg.ModulePath = modPath

	callCount := 0
	provider := &retryMockProvider{
		inner:       happyProvider(),
		failUntil:   2, // fail first 2 attempts, succeed on 3rd
		callCounter: &callCount,
	}

	vcfg := ValidatorConfig{
		ModuleTimeout: 2 * time.Second,
		MaxRetries:    2,
		RetryDelay:    1 * time.Millisecond,
	}

	v := NewPkcs11Validator(cfg, vcfg, provider)
	report := v.Validate(context.Background())

	if !report.Ready {
		for _, r := range report.Results {
			if !r.OK {
				t.Errorf("step %q failed: %s", r.Step, r.Message)
			}
		}
		t.Fatal("expected validation to succeed after retries")
	}

	// Verify the module_load pass message mentions retries.
	for _, r := range report.Results {
		if r.Step == "module_load" && r.OK {
			if r.Message == "" {
				t.Error("expected non-empty message for module_load step")
			}
			return
		}
	}
	t.Error("module_load step not found in report")
}

func TestPkcs11Validator_NoSlotsFound(t *testing.T) {
	modPath := writeTempModule(t)
	cfg := defaultCfg()
	cfg.ModulePath = modPath

	provider := happyProvider()
	provider.slots = []uint64{}

	v := NewPkcs11Validator(cfg, fastVcfg(), provider)
	report := v.Validate(context.Background())

	if report.Ready {
		t.Fatal("expected validation to fail when no slots found")
	}
	assertStepFailed(t, report, "slot_enum")
	assertRemediationContains(t, report, "slot_enum", "softhsm2-util")
}

func TestPkcs11Validator_TokenLabelNotFound(t *testing.T) {
	modPath := writeTempModule(t)
	cfg := defaultCfg()
	cfg.ModulePath = modPath
	cfg.TokenLabel = "NonExistentToken"

	provider := happyProvider()
	provider.tokenLabels = map[uint64]string{0: "OtherToken"}

	v := NewPkcs11Validator(cfg, fastVcfg(), provider)
	report := v.Validate(context.Background())

	if report.Ready {
		t.Fatal("expected validation to fail when token label not found")
	}
	assertStepFailed(t, report, "slot_enum")
	assertRemediationContains(t, report, "slot_enum", "GLASSBOX_PKCS11_TOKEN_LABEL")
}

func TestPkcs11Validator_SlotIndexOutOfRange(t *testing.T) {
	modPath := writeTempModule(t)
	cfg := defaultCfg()
	cfg.ModulePath = modPath
	cfg.SlotIndex = 5 // only 1 slot available

	v := NewPkcs11Validator(cfg, fastVcfg(), happyProvider())
	report := v.Validate(context.Background())

	if report.Ready {
		t.Fatal("expected validation to fail for out-of-range slot index")
	}
	assertStepFailed(t, report, "slot_enum")
}

func TestPkcs11Validator_SessionOpenFails(t *testing.T) {
	modPath := writeTempModule(t)
	cfg := defaultCfg()
	cfg.ModulePath = modPath

	provider := happyProvider()
	provider.openSessionErr = errors.New("CKR_TOKEN_NOT_PRESENT")

	v := NewPkcs11Validator(cfg, fastVcfg(), provider)
	report := v.Validate(context.Background())

	if report.Ready {
		t.Fatal("expected validation to fail when session open fails")
	}
	assertStepFailed(t, report, "session_open")
}

func TestPkcs11Validator_BadPIN(t *testing.T) {
	modPath := writeTempModule(t)
	cfg := defaultCfg()
	cfg.ModulePath = modPath

	provider := happyProvider()
	provider.loginErr = &Pkcs11Error{
		Op:          "C_Login",
		RV:          CKR_PIN_INCORRECT,
		Message:     "the PIN is incorrect",
		Remediation: "verify GLASSBOX_PKCS11_PIN is correct",
	}

	v := NewPkcs11Validator(cfg, fastVcfg(), provider)
	report := v.Validate(context.Background())

	if report.Ready {
		t.Fatal("expected validation to fail for bad PIN")
	}
	assertStepFailed(t, report, "pin_auth")
	assertRemediationContains(t, report, "pin_auth", "GLASSBOX_PKCS11_PIN")
}

func TestPkcs11Validator_PINEmpty(t *testing.T) {
	modPath := writeTempModule(t)
	cfg := defaultCfg()
	cfg.ModulePath = modPath
	cfg.PIN = ""

	v := NewPkcs11Validator(cfg, fastVcfg(), happyProvider())
	report := v.Validate(context.Background())

	if report.Ready {
		t.Fatal("expected validation to fail for empty PIN")
	}
	assertStepFailed(t, report, "pin_auth")
	assertRemediationContains(t, report, "pin_auth", "GLASSBOX_PKCS11_PIN")
}

func TestPkcs11Validator_KeyLabelMissing(t *testing.T) {
	modPath := writeTempModule(t)
	cfg := defaultCfg()
	cfg.ModulePath = modPath
	cfg.KeyLabel = ""
	cfg.KeyIDHex = ""

	v := NewPkcs11Validator(cfg, fastVcfg(), happyProvider())
	report := v.Validate(context.Background())

	if report.Ready {
		t.Fatal("expected validation to fail when no key selector is set")
	}
	assertStepFailed(t, report, "key_lookup")
	assertRemediationContains(t, report, "key_lookup", "GLASSBOX_PKCS11_KEY_LABEL")
}

func TestPkcs11Validator_KeyNotFound(t *testing.T) {
	modPath := writeTempModule(t)
	cfg := defaultCfg()
	cfg.ModulePath = modPath
	cfg.KeyLabel = "nonexistent-key"

	provider := happyProvider()
	provider.findKeyErr = errors.New("no matching key found")

	v := NewPkcs11Validator(cfg, fastVcfg(), provider)
	report := v.Validate(context.Background())

	if report.Ready {
		t.Fatal("expected validation to fail when key not found")
	}
	assertStepFailed(t, report, "key_lookup")
	assertRemediationContains(t, report, "key_lookup", "pkcs11-tool")
}

func TestPkcs11Validator_UnsupportedModule(t *testing.T) {
	modPath := writeTempModule(t)
	cfg := defaultCfg()
	cfg.ModulePath = modPath

	provider := happyProvider()
	provider.signTestErr = &Pkcs11Error{
		Op:          "C_SignInit",
		RV:          CKR_MECHANISM_INVALID,
		Message:     "the CKM_EDDSA mechanism is not supported by this module",
		Remediation: "verify the module supports Ed25519 signing",
	}

	v := NewPkcs11Validator(cfg, fastVcfg(), provider)
	report := v.Validate(context.Background())

	if report.Ready {
		t.Fatal("expected validation to fail for unsupported mechanism")
	}
	assertStepFailed(t, report, "sign_test")
}

func TestPkcs11Validator_ContextCancelled(t *testing.T) {
	modPath := writeTempModule(t)
	cfg := defaultCfg()
	cfg.ModulePath = modPath

	// Provider that blocks on LoadModule until context is cancelled.
	provider := &blockingProvider{inner: happyProvider()}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	vcfg := ValidatorConfig{
		ModuleTimeout: 200 * time.Millisecond,
		MaxRetries:    0,
	}

	v := NewPkcs11Validator(cfg, vcfg, provider)
	report := v.Validate(ctx)

	if report.Ready {
		t.Fatal("expected validation to fail when context is cancelled")
	}
	assertStepFailed(t, report, "module_load")
}

func TestPkcs11Validator_TokenInfoUnavailable(t *testing.T) {
	// token_info failure is non-fatal; validation should still succeed.
	modPath := writeTempModule(t)
	cfg := defaultCfg()
	cfg.ModulePath = modPath

	provider := happyProvider()
	provider.getTokenInfoErr = errors.New("CKR_FUNCTION_NOT_SUPPORTED")

	v := NewPkcs11Validator(cfg, fastVcfg(), provider)
	report := v.Validate(context.Background())

	if !report.Ready {
		for _, r := range report.Results {
			if !r.OK {
				t.Errorf("unexpected failure at step %q: %s", r.Step, r.Message)
			}
		}
		t.Fatal("expected validation to succeed even when token_info is unavailable")
	}
}

// ─── Error mapping tests ──────────────────────────────────────────────────────

func TestMapPkcs11Error_OK(t *testing.T) {
	if err := MapPkcs11Error("C_Login", CKR_OK); err != nil {
		t.Fatalf("expected nil for CKR_OK, got %v", err)
	}
}

func TestMapPkcs11Error_KnownCodes(t *testing.T) {
	cases := []struct {
		rv          uint64
		wantInMsg   string
		wantInHint  string
	}{
		{CKR_PIN_INCORRECT, "PIN is incorrect", "GLASSBOX_PKCS11_PIN"},
		{CKR_PIN_LOCKED, "PIN is locked", "SO (Security Officer) PIN"},
		{CKR_TOKEN_NOT_PRESENT, "no token is present", "softhsm2-util"},
		{CKR_MECHANISM_INVALID, "CKM_EDDSA mechanism is not supported", "SoftHSM2"},
		{CKR_KEY_FUNCTION_NOT_PERMITTED, "sign permission", "CKA_SIGN=true"},
		{CKR_USER_PIN_NOT_INITIALIZED, "PIN has not been initialized", "pkcs11-tool"},
		{CKR_DEVICE_REMOVED, "device was removed", "reinsert"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("0x%08X", tc.rv), func(t *testing.T) {
			err := MapPkcs11Error("C_Test", tc.rv)
			if err == nil {
				t.Fatal("expected non-nil error")
			}
			if !containsSubstring(err.Message, tc.wantInMsg) {
				t.Errorf("message %q does not contain %q", err.Message, tc.wantInMsg)
			}
			if !containsSubstring(err.Remediation, tc.wantInHint) {
				t.Errorf("remediation %q does not contain %q", err.Remediation, tc.wantInHint)
			}
		})
	}
}

func TestMapPkcs11Error_VendorDefined(t *testing.T) {
	err := MapPkcs11Error("C_Sign", CKR_VENDOR_DEFINED+1)
	if err == nil {
		t.Fatal("expected non-nil error for vendor-defined code")
	}
	if !containsSubstring(err.Message, "vendor-defined") {
		t.Errorf("expected 'vendor-defined' in message, got %q", err.Message)
	}
}

func TestMapPkcs11Error_UnknownCode(t *testing.T) {
	err := MapPkcs11Error("C_Sign", 0x00001234)
	if err == nil {
		t.Fatal("expected non-nil error for unknown code")
	}
	if !containsSubstring(err.Message, "unknown") {
		t.Errorf("expected 'unknown' in message, got %q", err.Message)
	}
}

func TestPkcs11Error_ErrorString(t *testing.T) {
	err := &Pkcs11Error{
		Op:          "C_Login",
		RV:          CKR_PIN_INCORRECT,
		Message:     "the PIN is incorrect",
		Remediation: "verify GLASSBOX_PKCS11_PIN",
	}
	s := err.Error()
	if !containsSubstring(s, "C_Login") {
		t.Errorf("error string missing op: %q", s)
	}
	if !containsSubstring(s, "PIN is incorrect") {
		t.Errorf("error string missing message: %q", s)
	}
	if !containsSubstring(s, "verify GLASSBOX_PKCS11_PIN") {
		t.Errorf("error string missing remediation: %q", s)
	}
}

// ─── Assertion helpers ────────────────────────────────────────────────────────

func assertStepFailed(t *testing.T, report *PreflightReport, step string) {
	t.Helper()
	for _, r := range report.Results {
		if r.Step == step {
			if r.OK {
				t.Errorf("expected step %q to fail, but it passed", step)
			}
			return
		}
	}
	t.Errorf("step %q not found in report", step)
}

func assertRemediationContains(t *testing.T, report *PreflightReport, step, substr string) {
	t.Helper()
	for _, r := range report.Results {
		if r.Step == step {
			if !containsSubstring(r.Remediation, substr) {
				t.Errorf("step %q remediation %q does not contain %q", step, r.Remediation, substr)
			}
			return
		}
	}
	t.Errorf("step %q not found in report", step)
}

func containsSubstring(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}

// ─── Additional mock providers ────────────────────────────────────────────────

// retryMockProvider fails LoadModule for the first `failUntil` calls.
type retryMockProvider struct {
	inner       *mockPkcs11Provider
	failUntil   int
	callCounter *int
}

func (r *retryMockProvider) LoadModule(path string) error {
	*r.callCounter++
	if *r.callCounter <= r.failUntil {
		return fmt.Errorf("transient load error (attempt %d)", *r.callCounter)
	}
	return r.inner.LoadModule(path)
}
func (r *retryMockProvider) Initialize() error                              { return r.inner.Initialize() }
func (r *retryMockProvider) GetSlotList(tp bool) ([]uint64, error)          { return r.inner.GetSlotList(tp) }
func (r *retryMockProvider) GetTokenInfo(id uint64) (string, error)         { return r.inner.GetTokenInfo(id) }
func (r *retryMockProvider) OpenSession(id uint64) (uint64, error)          { return r.inner.OpenSession(id) }
func (r *retryMockProvider) Login(s uint64, pin string) error               { return r.inner.Login(s, pin) }
func (r *retryMockProvider) FindKey(s uint64, l, id string) (uint64, error) { return r.inner.FindKey(s, l, id) }
func (r *retryMockProvider) SignTest(s, k uint64, d []byte) error           { return r.inner.SignTest(s, k, d) }
func (r *retryMockProvider) CloseSession(s uint64) error                    { return r.inner.CloseSession(s) }
func (r *retryMockProvider) Finalize() error                                { return r.inner.Finalize() }

// blockingProvider blocks LoadModule until the context is cancelled.
type blockingProvider struct {
	inner *mockPkcs11Provider
}

func (b *blockingProvider) LoadModule(_ string) error {
	// Block for a long time to simulate a hung module.
	time.Sleep(10 * time.Second)
	return nil
}
func (b *blockingProvider) Initialize() error                              { return b.inner.Initialize() }
func (b *blockingProvider) GetSlotList(tp bool) ([]uint64, error)          { return b.inner.GetSlotList(tp) }
func (b *blockingProvider) GetTokenInfo(id uint64) (string, error)         { return b.inner.GetTokenInfo(id) }
func (b *blockingProvider) OpenSession(id uint64) (uint64, error)          { return b.inner.OpenSession(id) }
func (b *blockingProvider) Login(s uint64, pin string) error               { return b.inner.Login(s, pin) }
func (b *blockingProvider) FindKey(s uint64, l, id string) (uint64, error) { return b.inner.FindKey(s, l, id) }
func (b *blockingProvider) SignTest(s, k uint64, d []byte) error           { return b.inner.SignTest(s, k, d) }
func (b *blockingProvider) CloseSession(s uint64) error                    { return b.inner.CloseSession(s) }
func (b *blockingProvider) Finalize() error                                { return b.inner.Finalize() }
