// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package simulator

import (
	"encoding/json"
	"strings"
	"testing"
)

// ptr helpers
func strPtr(s string) *string { return &s }
func uint32Ptr(v uint32) *uint32 { return &v }
func uint64Ptr(v uint64) *uint64 { return &v }

// ─── ClassifyFailure nil / success guards ────────────────────────────────────

func TestClassifyFailure_NilResponse(t *testing.T) {
	if got := ClassifyFailure(nil); got != nil {
		t.Errorf("expected nil for nil response, got %v", got)
	}
}

func TestClassifyFailure_SuccessResponse(t *testing.T) {
	resp := &SimulationResponse{Status: "success"}
	if got := ClassifyFailure(resp); got != nil {
		t.Errorf("expected nil for success response, got %v", got)
	}
}

// ─── CPU budget exhaustion ───────────────────────────────────────────────────

func TestClassifyFailure_CPUBudget_ViaUsagePercent(t *testing.T) {
	resp := &SimulationResponse{
		Status: "error",
		Error:  "simulation failed",
		BudgetUsage: &BudgetUsage{
			CPUInstructions: 100_000_000,
			CPULimit:        100_000_000,
			CPUUsagePercent: 100.0,
			MemoryBytes:     10_000_000,
			MemoryLimit:     50_000_000,
			MemoryUsagePercent: 20.0,
		},
	}
	d := ClassifyFailure(resp)
	if d == nil {
		t.Fatal("expected non-nil diagnostic")
	}
	if d.Category != FailureCPUBudget {
		t.Errorf("expected %s, got %s", FailureCPUBudget, d.Category)
	}
	if d.BudgetDetails == nil {
		t.Fatal("expected BudgetDetails to be populated")
	}
	if !d.BudgetDetails.CPUExhausted {
		t.Error("expected CPUExhausted=true")
	}
	if d.BudgetDetails.MemoryExhausted {
		t.Error("expected MemoryExhausted=false")
	}
}

func TestClassifyFailure_CPUBudget_ViaErrorCode(t *testing.T) {
	resp := &SimulationResponse{
		Status:    "error",
		ErrorCode: "ERR_CPU_LIMIT_EXCEEDED",
		Error:     "cpu limit exceeded",
	}
	d := ClassifyFailure(resp)
	if d == nil {
		t.Fatal("expected non-nil diagnostic")
	}
	if d.Category != FailureCPUBudget {
		t.Errorf("expected %s, got %s", FailureCPUBudget, d.Category)
	}
}

func TestClassifyFailure_CPUBudget_ViaMessage(t *testing.T) {
	messages := []string{
		"CpuLimitExceeded: contract used too many instructions",
		"error(budget, cpu): limit reached",
		"budget cpu exhausted",
	}
	for _, msg := range messages {
		resp := &SimulationResponse{Status: "error", Error: msg}
		d := ClassifyFailure(resp)
		if d == nil || d.Category != FailureCPUBudget {
			t.Errorf("message %q: expected %s, got %v", msg, FailureCPUBudget, d)
		}
	}
}

// ─── Memory budget exhaustion ────────────────────────────────────────────────

func TestClassifyFailure_MemoryBudget_ViaUsagePercent(t *testing.T) {
	resp := &SimulationResponse{
		Status: "error",
		Error:  "simulation failed",
		BudgetUsage: &BudgetUsage{
			CPUInstructions:    50_000_000,
			CPULimit:           100_000_000,
			CPUUsagePercent:    50.0,
			MemoryBytes:        50_000_000,
			MemoryLimit:        50_000_000,
			MemoryUsagePercent: 100.0,
		},
	}
	d := ClassifyFailure(resp)
	if d == nil {
		t.Fatal("expected non-nil diagnostic")
	}
	if d.Category != FailureMemoryBudget {
		t.Errorf("expected %s, got %s", FailureMemoryBudget, d.Category)
	}
	if d.BudgetDetails == nil {
		t.Fatal("expected BudgetDetails to be populated")
	}
	if !d.BudgetDetails.MemoryExhausted {
		t.Error("expected MemoryExhausted=true")
	}
}

func TestClassifyFailure_MemoryBudget_ViaErrorCode(t *testing.T) {
	resp := &SimulationResponse{
		Status:    "error",
		ErrorCode: "ERR_MEMORY_LIMIT_EXCEEDED",
		Error:     "memory limit exceeded",
	}
	d := ClassifyFailure(resp)
	if d == nil {
		t.Fatal("expected non-nil diagnostic")
	}
	if d.Category != FailureMemoryBudget {
		t.Errorf("expected %s, got %s", FailureMemoryBudget, d.Category)
	}
}

func TestClassifyFailure_MemoryBudget_ViaMessage(t *testing.T) {
	messages := []string{
		"MemLimitExceeded: contract used too much memory",
		"err_memory_limit_exceeded",
		"error(budget, mem): limit reached",
		"budget mem exhausted",
	}
	for _, msg := range messages {
		resp := &SimulationResponse{Status: "error", Error: msg}
		d := ClassifyFailure(resp)
		if d == nil || d.Category != FailureMemoryBudget {
			t.Errorf("message %q: expected %s, got %v", msg, FailureMemoryBudget, d)
		}
	}
}

// ─── Auth failure ────────────────────────────────────────────────────────────

func TestClassifyFailure_AuthFailure_ViaMessage(t *testing.T) {
	messages := []string{
		"error(auth, not authorized): require_auth failed",
		"not authorized to call this function",
		"require_auth: missing entry",
		"auth failed: no signature",
		"missing authorization for contract",
		"authorization failed for address",
		"unauthorized access",
	}
	for _, msg := range messages {
		resp := &SimulationResponse{Status: "error", Error: msg}
		d := ClassifyFailure(resp)
		if d == nil || d.Category != FailureAuthFailure {
			t.Errorf("message %q: expected %s, got %v", msg, FailureAuthFailure, d)
		}
	}
}

func TestClassifyFailure_AuthFailure_WithDiagnosticEvents(t *testing.T) {
	callerID := "CAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABSC4"
	calleeID := "CBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBSC4X"
	resp := &SimulationResponse{
		Status: "error",
		Error:  "error(auth, not authorized)",
		DiagnosticEvents: []DiagnosticEvent{
			{EventType: "contract", ContractID: strPtr(callerID), Topics: []string{"transfer"}},
			{EventType: "contract", ContractID: strPtr(calleeID), Topics: []string{"require_auth"}},
		},
	}
	d := ClassifyFailure(resp)
	if d == nil {
		t.Fatal("expected non-nil diagnostic")
	}
	if d.Category != FailureAuthFailure {
		t.Errorf("expected %s, got %s", FailureAuthFailure, d.Category)
	}
	if d.AuthDetails == nil {
		t.Fatal("expected AuthDetails to be populated")
	}
	if d.AuthDetails.CallerContractID != callerID {
		t.Errorf("expected caller %s, got %s", callerID, d.AuthDetails.CallerContractID)
	}
	if d.AuthDetails.CalleeContractID != calleeID {
		t.Errorf("expected callee %s, got %s", calleeID, d.AuthDetails.CalleeContractID)
	}
	if len(d.AuthDetails.ContractIDs) != 2 {
		t.Errorf("expected 2 contract IDs, got %d", len(d.AuthDetails.ContractIDs))
	}
}

func TestClassifyFailure_AuthFailure_SingleContract(t *testing.T) {
	contractID := "CAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABSC4"
	resp := &SimulationResponse{
		Status: "error",
		Error:  "not authorized",
		DiagnosticEvents: []DiagnosticEvent{
			{EventType: "contract", ContractID: strPtr(contractID)},
		},
	}
	d := ClassifyFailure(resp)
	if d == nil || d.Category != FailureAuthFailure {
		t.Fatalf("expected %s, got %v", FailureAuthFailure, d)
	}
	if d.AuthDetails.CallerContractID != "" {
		t.Errorf("expected empty caller, got %s", d.AuthDetails.CallerContractID)
	}
	if d.AuthDetails.CalleeContractID != contractID {
		t.Errorf("expected callee %s, got %s", contractID, d.AuthDetails.CalleeContractID)
	}
}

// ─── Contract trap ───────────────────────────────────────────────────────────

func TestClassifyFailure_ContractTrap_ViaStackTrace(t *testing.T) {
	funcIdx := uint32(42)
	funcName := "my_contract_fn"
	resp := &SimulationResponse{
		Status: "error",
		Error:  "wasm trap: unreachable",
		StackTrace: &WasmStackTrace{
			TrapKind:       "Unreachable",
			RawMessage:     "wasm trap: unreachable",
			SorobanWrapped: true,
			Frames: []StackFrame{
				{Index: 0, FuncIndex: &funcIdx, FuncName: &funcName},
			},
		},
	}
	d := ClassifyFailure(resp)
	if d == nil {
		t.Fatal("expected non-nil diagnostic")
	}
	if d.Category != FailureContractTrap {
		t.Errorf("expected %s, got %s", FailureContractTrap, d.Category)
	}
	if d.TrapDetails == nil {
		t.Fatal("expected TrapDetails to be populated")
	}
	if d.TrapDetails.FrameCount != 1 {
		t.Errorf("expected 1 frame, got %d", d.TrapDetails.FrameCount)
	}
	if !d.TrapDetails.SorobanWrapped {
		t.Error("expected SorobanWrapped=true")
	}
	if d.TrapDetails.TopFrame == nil {
		t.Fatal("expected TopFrame to be set")
	}
	if d.TrapDetails.TopFrame.FuncName == nil || *d.TrapDetails.TopFrame.FuncName != funcName {
		t.Errorf("unexpected top frame func name: %v", d.TrapDetails.TopFrame.FuncName)
	}
}

func TestClassifyFailure_ContractTrap_ViaMessage(t *testing.T) {
	tests := []struct {
		msg             string
		summaryContains string
	}{
		{"wasm trap: unreachable instruction executed", "unreachable"},
		{"contract trapped: memory access out of bounds", "out-of-bounds"},
		{"stack overflow in wasm execution", "stack overflow"},
		{"panic: assertion failed at line 42", "assertion failed at line 42"},
		{"integer divide by zero", "divide-by-zero"},
		{"integer overflow in arithmetic", "overflow"},
		{"contract_invocation_failed", "fatal WASM trap"},
	}
	for _, tt := range tests {
		resp := &SimulationResponse{Status: "error", Error: tt.msg}
		d := ClassifyFailure(resp)
		if d == nil || d.Category != FailureContractTrap {
			t.Errorf("message %q: expected %s, got %v", tt.msg, FailureContractTrap, d)
			continue
		}
		if tt.summaryContains != "" && !containsCI(d.Summary, tt.summaryContains) {
			t.Errorf("message %q: summary %q does not contain %q", tt.msg, d.Summary, tt.summaryContains)
		}
	}
}

func TestClassifyFailure_ContractTrap_ViaErrorCode(t *testing.T) {
	codes := []string{"SIM_PROCESS_CRASHED", "WASM_TRAP", "CONTRACT_TRAP"}
	for _, code := range codes {
		resp := &SimulationResponse{Status: "error", ErrorCode: code, Error: "trap"}
		d := ClassifyFailure(resp)
		if d == nil || d.Category != FailureContractTrap {
			t.Errorf("code %q: expected %s, got %v", code, FailureContractTrap, d)
		}
	}
}

// ─── Validation error ────────────────────────────────────────────────────────

func TestClassifyFailure_Validation_ViaErrorCode(t *testing.T) {
	codes := []string{"VALIDATION_FAILED", "VALIDATION_ERROR", "TX_SOROBAN_INVALID", "TX_MALFORMED", "INVALID_INPUT"}
	for _, code := range codes {
		resp := &SimulationResponse{Status: "error", ErrorCode: code, Error: "validation failed"}
		d := ClassifyFailure(resp)
		if d == nil || d.Category != FailureValidation {
			t.Errorf("code %q: expected %s, got %v", code, FailureValidation, d)
		}
	}
}

func TestClassifyFailure_Validation_ViaMessage(t *testing.T) {
	tests := []struct {
		msg   string
		field string
	}{
		{"failed to decode Envelope XDR", "envelope_xdr"},
		{"failed to decode LedgerKey", "ledger_key"},
		{"failed to decode LedgerEntry", "ledger_entry"},
		{"failed to decode WASM module", "wasm"},
		{"tx_soroban_invalid: resource limits exceeded", "transaction"},
		{"tx_malformed: bad parameters", "transaction"},
		{"bad sequence number", "sequence_number"},
		{"insufficient fee for transaction", "fee"},
	}
	for _, tt := range tests {
		resp := &SimulationResponse{Status: "error", Error: tt.msg}
		d := ClassifyFailure(resp)
		if d == nil || d.Category != FailureValidation {
			t.Errorf("message %q: expected %s, got %v", tt.msg, FailureValidation, d)
			continue
		}
		if d.ValidationDetails == nil {
			t.Errorf("message %q: expected ValidationDetails to be populated", tt.msg)
			continue
		}
		if tt.field != "" && d.ValidationDetails.Field != tt.field {
			t.Errorf("message %q: expected field=%q, got %q", tt.msg, tt.field, d.ValidationDetails.Field)
		}
	}
}

// ─── Unknown ─────────────────────────────────────────────────────────────────

func TestClassifyFailure_Unknown(t *testing.T) {
	resp := &SimulationResponse{
		Status: "error",
		Error:  "some completely unrecognized error",
	}
	d := ClassifyFailure(resp)
	if d == nil {
		t.Fatal("expected non-nil diagnostic")
	}
	if d.Category != FailureUnknown {
		t.Errorf("expected %s, got %s", FailureUnknown, d.Category)
	}
	if d.ErrorMessage != resp.Error {
		t.Errorf("expected ErrorMessage=%q, got %q", resp.Error, d.ErrorMessage)
	}
}

func TestClassifyFailure_Unknown_EmptyError(t *testing.T) {
	resp := &SimulationResponse{Status: "error"}
	d := ClassifyFailure(resp)
	if d == nil || d.Category != FailureUnknown {
		t.Fatalf("expected %s, got %v", FailureUnknown, d)
	}
}

// ─── Priority ordering ───────────────────────────────────────────────────────

// CPU budget takes priority over auth failure when both signals are present.
func TestClassifyFailure_Priority_CPUOverAuth(t *testing.T) {
	resp := &SimulationResponse{
		Status: "error",
		Error:  "error(auth, not authorized): cpu limit exceeded",
		BudgetUsage: &BudgetUsage{
			CPUUsagePercent:    100.0,
			MemoryUsagePercent: 20.0,
		},
	}
	d := ClassifyFailure(resp)
	if d == nil || d.Category != FailureCPUBudget {
		t.Errorf("expected %s (CPU takes priority), got %v", FailureCPUBudget, d)
	}
}

// Memory budget takes priority over contract trap when both signals are present.
func TestClassifyFailure_Priority_MemoryOverTrap(t *testing.T) {
	resp := &SimulationResponse{
		Status: "error",
		Error:  "wasm trap: memory limit exceeded",
		BudgetUsage: &BudgetUsage{
			CPUUsagePercent:    50.0,
			MemoryUsagePercent: 100.0,
		},
	}
	d := ClassifyFailure(resp)
	if d == nil || d.Category != FailureMemoryBudget {
		t.Errorf("expected %s (memory takes priority), got %v", FailureMemoryBudget, d)
	}
}

// ─── JSON serialization ──────────────────────────────────────────────────────

func TestFailureDiagnostic_JSONRoundTrip(t *testing.T) {
	resp := &SimulationResponse{
		Status:    "error",
		ErrorCode: "ERR_MEMORY_LIMIT_EXCEEDED",
		Error:     "memory limit exceeded",
		BudgetUsage: &BudgetUsage{
			CPUInstructions:    80_000_000,
			CPULimit:           100_000_000,
			CPUUsagePercent:    80.0,
			MemoryBytes:        50_000_000,
			MemoryLimit:        50_000_000,
			MemoryUsagePercent: 100.0,
		},
	}
	d := ClassifyFailure(resp)
	if d == nil {
		t.Fatal("expected non-nil diagnostic")
	}

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded FailureDiagnostic
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.Category != d.Category {
		t.Errorf("Category mismatch after round-trip: %s vs %s", decoded.Category, d.Category)
	}
	if decoded.BudgetDetails == nil {
		t.Fatal("BudgetDetails missing after round-trip")
	}
	if decoded.BudgetDetails.MemoryExhausted != true {
		t.Error("MemoryExhausted should be true after round-trip")
	}
}

func TestFailureDiagnostic_JSONContainsExpectedFields(t *testing.T) {
	resp := &SimulationResponse{
		Status:    "error",
		ErrorCode: "WASM_TRAP",
		Error:     "panic: assertion failed",
		StackTrace: &WasmStackTrace{
			TrapKind:   "Unreachable",
			RawMessage: "panic: assertion failed",
			Frames:     []StackFrame{{Index: 0}},
		},
	}
	d := ClassifyFailure(resp)
	data, _ := json.Marshal(d)
	jsonStr := string(data)

	for _, field := range []string{"category", "summary", "error_code", "error_message", "trap_details"} {
		if !containsCI(jsonStr, field) {
			t.Errorf("JSON output missing field %q: %s", field, jsonStr)
		}
	}
}

// ─── String() method ─────────────────────────────────────────────────────────

func TestFailureDiagnostic_String(t *testing.T) {
	d := &FailureDiagnostic{
		Category: FailureCPUBudget,
		Summary:  "CPU budget exhausted.",
	}
	s := d.String()
	if !containsCI(s, string(FailureCPUBudget)) {
		t.Errorf("String() missing category: %s", s)
	}
	if !containsCI(s, "CPU budget exhausted") {
		t.Errorf("String() missing summary: %s", s)
	}
}

func TestFailureDiagnostic_String_Nil(t *testing.T) {
	var d *FailureDiagnostic
	s := d.String()
	if s == "" {
		t.Error("String() on nil should return non-empty string")
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func containsCI(s, sub string) bool {
	return len(sub) > 0 &&
		len(s) >= len(sub) &&
		func() bool {
			sl := strings.ToLower(s)
			subl := strings.ToLower(sub)
			for i := 0; i <= len(sl)-len(subl); i++ {
				if sl[i:i+len(subl)] == subl {
					return true
				}
			}
			return false
		}()
}


