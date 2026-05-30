// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

// Package simulator provides the simulation failure classifier, which maps raw
// Soroban result codes, XDR error details, budget metrics, and trace events to
// well-defined diagnostic categories.
package simulator

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FailureCategory is a well-defined diagnostic class for a simulation failure.
type FailureCategory string

const (
	// FailureCPUBudget indicates the contract exhausted the Soroban CPU instruction budget.
	FailureCPUBudget FailureCategory = "CPU_BUDGET_EXCEEDED"

	// FailureMemoryBudget indicates the contract exhausted the Soroban memory allocation budget.
	FailureMemoryBudget FailureCategory = "MEMORY_BUDGET_EXCEEDED"

	// FailureAuthFailure indicates a missing or invalid authorization entry or signature.
	FailureAuthFailure FailureCategory = "AUTH_FAILURE"

	// FailureContractTrap indicates a fatal WASM trap (unreachable, out-of-bounds, stack overflow, explicit panic).
	FailureContractTrap FailureCategory = "CONTRACT_TRAP"

	// FailureValidation indicates a ledger-level or protocol-level validation error
	// (e.g. malformed XDR, bad sequence number, Soroban-invalid transaction).
	FailureValidation FailureCategory = "VALIDATION_ERROR"

	// FailureUnknown is used when no specific category can be determined.
	FailureUnknown FailureCategory = "UNKNOWN"
)

// FailureDiagnostic is the structured result produced by ClassifyFailure.
// It is safe to serialize to JSON and is exposed in both CLI and JSON output modes.
type FailureDiagnostic struct {
	// Category is the primary failure classification.
	Category FailureCategory `json:"category"`

	// Summary is a single human-readable sentence describing the failure.
	Summary string `json:"summary"`

	// ErrorCode is the raw error code string from the simulator response, if any.
	ErrorCode string `json:"error_code,omitempty"`

	// ErrorMessage is the raw error message from the simulator response, if any.
	ErrorMessage string `json:"error_message,omitempty"`

	// BudgetDetails is populated for CPU_BUDGET_EXCEEDED and MEMORY_BUDGET_EXCEEDED failures.
	BudgetDetails *BudgetDiagnosticDetails `json:"budget_details,omitempty"`

	// TrapDetails is populated for CONTRACT_TRAP failures.
	TrapDetails *TrapDiagnosticDetails `json:"trap_details,omitempty"`

	// AuthDetails is populated for AUTH_FAILURE failures.
	AuthDetails *AuthDiagnosticDetails `json:"auth_details,omitempty"`

	// ValidationDetails is populated for VALIDATION_ERROR failures.
	ValidationDetails *ValidationDiagnosticDetails `json:"validation_details,omitempty"`
}

// BudgetDiagnosticDetails carries CPU and memory budget metrics for budget failures.
type BudgetDiagnosticDetails struct {
	// CPUInstructions is the number of CPU instructions consumed.
	CPUInstructions uint64 `json:"cpu_instructions"`
	// CPULimit is the maximum allowed CPU instructions.
	CPULimit uint64 `json:"cpu_limit"`
	// CPUUsagePercent is the percentage of the CPU budget consumed (0–100+).
	CPUUsagePercent float64 `json:"cpu_usage_percent"`
	// MemoryBytes is the number of memory bytes consumed.
	MemoryBytes uint64 `json:"memory_bytes"`
	// MemoryLimit is the maximum allowed memory bytes.
	MemoryLimit uint64 `json:"memory_limit"`
	// MemoryUsagePercent is the percentage of the memory budget consumed (0–100+).
	MemoryUsagePercent float64 `json:"memory_usage_percent"`
	// CPUExhausted is true when the CPU budget was fully consumed.
	CPUExhausted bool `json:"cpu_exhausted"`
	// MemoryExhausted is true when the memory budget was fully consumed.
	MemoryExhausted bool `json:"memory_exhausted"`
}

// TrapDiagnosticDetails carries WASM trap information for contract trap failures.
type TrapDiagnosticDetails struct {
	// TrapKind is the raw trap kind value from the stack trace, if available.
	TrapKind string `json:"trap_kind,omitempty"`
	// RawMessage is the raw trap message from the stack trace.
	RawMessage string `json:"raw_message,omitempty"`
	// FrameCount is the number of WASM stack frames captured.
	FrameCount int `json:"frame_count"`
	// TopFrame is the innermost (most recent) stack frame, if available.
	TopFrame *StackFrame `json:"top_frame,omitempty"`
	// SorobanWrapped indicates the trap was wrapped by the Soroban host.
	SorobanWrapped bool `json:"soroban_wrapped"`
}

// AuthDiagnosticDetails carries authorization failure context.
type AuthDiagnosticDetails struct {
	// ContractIDs lists the distinct contract IDs observed in diagnostic events
	// at the time of the auth failure.
	ContractIDs []string `json:"contract_ids,omitempty"`
	// CallerContractID is the contract that initiated the failing call, if detectable.
	CallerContractID string `json:"caller_contract_id,omitempty"`
	// CalleeContractID is the contract that failed the auth check, if detectable.
	CalleeContractID string `json:"callee_contract_id,omitempty"`
}

// ValidationDiagnosticDetails carries ledger-level validation error context.
type ValidationDiagnosticDetails struct {
	// Field is the field or component that failed validation, if known.
	Field string `json:"field,omitempty"`
	// Reason is a short description of why validation failed.
	Reason string `json:"reason,omitempty"`
}

// String returns a human-readable one-line representation of the diagnostic.
func (d *FailureDiagnostic) String() string {
	if d == nil {
		return "no diagnostic available"
	}
	return fmt.Sprintf("[%s] %s", d.Category, d.Summary)
}

// MarshalJSON implements json.Marshaler so callers can embed FailureDiagnostic
// directly in larger JSON structures without extra wrapping.
func (d *FailureDiagnostic) MarshalJSON() ([]byte, error) {
	// Use an alias to avoid infinite recursion.
	type Alias FailureDiagnostic
	return json.Marshal((*Alias)(d))
}

// ─── Classifier ──────────────────────────────────────────────────────────────

// ClassifyFailure inspects a SimulationResponse that represents a failed
// simulation and returns a structured FailureDiagnostic.
//
// Classification priority (highest to lowest):
//  1. CPU budget exhaustion (BudgetUsage.CPUUsagePercent >= 100 or error code/message signals)
//  2. Memory budget exhaustion (BudgetUsage.MemoryUsagePercent >= 100 or error code/message signals)
//  3. Auth failure (error message or diagnostic event signals)
//  4. Contract trap (StackTrace present or error message signals)
//  5. Validation error (error code or message signals)
//  6. Unknown
//
// If resp is nil or resp.Status == "success", ClassifyFailure returns nil.
func ClassifyFailure(resp *SimulationResponse) *FailureDiagnostic {
	if resp == nil || resp.Status == "success" {
		return nil
	}

	errCode := strings.ToUpper(strings.TrimSpace(resp.ErrorCode))
	errMsg := resp.Error

	// 1. CPU budget
	if isCPUBudgetExhausted(errCode, errMsg, resp.BudgetUsage) {
		return buildCPUBudgetDiagnostic(errCode, errMsg, resp.BudgetUsage)
	}

	// 2. Memory budget
	if isMemoryBudgetExhausted(errCode, errMsg, resp.BudgetUsage) {
		return buildMemoryBudgetDiagnostic(errCode, errMsg, resp.BudgetUsage)
	}

	// 3. Auth failure
	if isAuthFailure(errCode, errMsg, resp.DiagnosticEvents) {
		return buildAuthDiagnostic(errCode, errMsg, resp.DiagnosticEvents)
	}

	// 4. Contract trap
	if isContractTrap(errCode, errMsg, resp.StackTrace) {
		return buildTrapDiagnostic(errCode, errMsg, resp.StackTrace)
	}

	// 5. Validation error
	if isValidationError(errCode, errMsg) {
		return buildValidationDiagnostic(errCode, errMsg)
	}

	// 6. Unknown
	return &FailureDiagnostic{
		Category:     FailureUnknown,
		Summary:      summarizeUnknown(errMsg),
		ErrorCode:    resp.ErrorCode,
		ErrorMessage: errMsg,
	}
}

// ─── Category detectors ───────────────────────────────────────────────────────

func isCPUBudgetExhausted(errCode, errMsg string, budget *BudgetUsage) bool {
	if budget != nil && budget.CPUUsagePercent >= 100 {
		return true
	}
	if errCode == "ERR_CPU_LIMIT_EXCEEDED" || errCode == "CPU_LIMIT_EXCEEDED" {
		return true
	}
	lc := strings.ToLower(errMsg)
	return strings.Contains(lc, "cpulimitexceeded") ||
		strings.Contains(lc, "cpu limit exceeded") ||
		strings.Contains(lc, "error(budget, cpu") ||
		(strings.Contains(lc, "budget") && strings.Contains(lc, "cpu"))
}

func isMemoryBudgetExhausted(errCode, errMsg string, budget *BudgetUsage) bool {
	if budget != nil && budget.MemoryUsagePercent >= 100 {
		return true
	}
	if errCode == "ERR_MEMORY_LIMIT_EXCEEDED" || errCode == "MEMORY_LIMIT_EXCEEDED" {
		return true
	}
	lc := strings.ToLower(errMsg)
	return strings.Contains(lc, "memlimitexceeded") ||
		strings.Contains(lc, "memory limit exceeded") ||
		strings.Contains(lc, "err_memory_limit_exceeded") ||
		strings.Contains(lc, "error(budget, mem") ||
		(strings.Contains(lc, "budget") && strings.Contains(lc, "mem"))
}

func isAuthFailure(errCode, errMsg string, events []DiagnosticEvent) bool {
	if errCode == "AUTH_FAILED" || errCode == "SOROBAN_AUTH_FAILED" {
		return true
	}
	lc := strings.ToLower(errMsg)
	if strings.Contains(lc, "error(auth,") ||
		strings.Contains(lc, "not authorized") ||
		strings.Contains(lc, "require_auth") ||
		strings.Contains(lc, "auth failed") ||
		strings.Contains(lc, "missing authorization") ||
		strings.Contains(lc, "authorization failed") ||
		strings.Contains(lc, "unauthorized") ||
		(strings.Contains(lc, "auth") && strings.Contains(lc, "fail")) {
		return true
	}
	// Check diagnostic events for auth-related topics
	for _, e := range events {
		for _, topic := range e.Topics {
			tl := strings.ToLower(topic)
			if strings.Contains(tl, "auth") || strings.Contains(tl, "require_auth") {
				return true
			}
		}
	}
	return false
}

func isContractTrap(errCode, errMsg string, stackTrace *WasmStackTrace) bool {
	if stackTrace != nil {
		return true
	}
	if errCode == "SIM_PROCESS_CRASHED" || errCode == "WASM_TRAP" || errCode == "CONTRACT_TRAP" {
		return true
	}
	lc := strings.ToLower(errMsg)
	return strings.Contains(lc, "wasm trap") ||
		strings.Contains(lc, "contract trapped") ||
		strings.Contains(lc, "unreachable") ||
		strings.Contains(lc, "stack overflow") ||
		strings.Contains(lc, "out of bounds memory access") ||
		strings.Contains(lc, "integer overflow") ||
		strings.Contains(lc, "integer divide by zero") ||
		strings.Contains(lc, "panic:") ||
		strings.Contains(lc, "contract_invocation_failed")
}

func isValidationError(errCode, errMsg string) bool {
	if errCode == "VALIDATION_FAILED" || errCode == "VALIDATION_ERROR" ||
		errCode == "TX_SOROBAN_INVALID" || errCode == "TX_MALFORMED" ||
		errCode == "INVALID_INPUT" {
		return true
	}
	lc := strings.ToLower(errMsg)
	return strings.Contains(lc, "tx_soroban_invalid") ||
		strings.Contains(lc, "tx_malformed") ||
		strings.Contains(lc, "invalid input") ||
		strings.Contains(lc, "validation error") ||
		strings.Contains(lc, "decode envelope") ||
		strings.Contains(lc, "decode ledgerkey") ||
		strings.Contains(lc, "decode ledgerentry") ||
		strings.Contains(lc, "decode wasm") ||
		strings.Contains(lc, "bad sequence") ||
		strings.Contains(lc, "insufficient fee") ||
		strings.Contains(lc, "tx_bad_seq") ||
		strings.Contains(lc, "tx_insufficient_fee")
}

// ─── Diagnostic builders ─────────────────────────────────────────────────────

func buildCPUBudgetDiagnostic(errCode, errMsg string, budget *BudgetUsage) *FailureDiagnostic {
	d := &FailureDiagnostic{
		Category:     FailureCPUBudget,
		ErrorCode:    errCode,
		ErrorMessage: errMsg,
	}
	if budget != nil {
		d.BudgetDetails = &BudgetDiagnosticDetails{
			CPUInstructions:    budget.CPUInstructions,
			CPULimit:           budget.CPULimit,
			CPUUsagePercent:    budget.CPUUsagePercent,
			MemoryBytes:        budget.MemoryBytes,
			MemoryLimit:        budget.MemoryLimit,
			MemoryUsagePercent: budget.MemoryUsagePercent,
			CPUExhausted:       budget.CPUUsagePercent >= 100,
			MemoryExhausted:    budget.MemoryUsagePercent >= 100,
		}
		d.Summary = fmt.Sprintf(
			"Contract execution exhausted the Soroban CPU instruction budget: %d/%d instructions used (%.1f%%).",
			budget.CPUInstructions, budget.CPULimit, budget.CPUUsagePercent,
		)
	} else {
		d.Summary = "Contract execution exhausted the Soroban CPU instruction budget."
	}
	return d
}

func buildMemoryBudgetDiagnostic(errCode, errMsg string, budget *BudgetUsage) *FailureDiagnostic {
	d := &FailureDiagnostic{
		Category:     FailureMemoryBudget,
		ErrorCode:    errCode,
		ErrorMessage: errMsg,
	}
	if budget != nil {
		d.BudgetDetails = &BudgetDiagnosticDetails{
			CPUInstructions:    budget.CPUInstructions,
			CPULimit:           budget.CPULimit,
			CPUUsagePercent:    budget.CPUUsagePercent,
			MemoryBytes:        budget.MemoryBytes,
			MemoryLimit:        budget.MemoryLimit,
			MemoryUsagePercent: budget.MemoryUsagePercent,
			CPUExhausted:       budget.CPUUsagePercent >= 100,
			MemoryExhausted:    budget.MemoryUsagePercent >= 100,
		}
		d.Summary = fmt.Sprintf(
			"Contract execution exhausted the Soroban memory allocation budget: %d/%d bytes used (%.1f%%).",
			budget.MemoryBytes, budget.MemoryLimit, budget.MemoryUsagePercent,
		)
	} else {
		d.Summary = "Contract execution exhausted the Soroban memory allocation budget."
	}
	return d
}

func buildAuthDiagnostic(errCode, errMsg string, events []DiagnosticEvent) *FailureDiagnostic {
	caller, callee := extractAuthContractIDs(events)
	allIDs := collectDistinctContractIDs(events)

	details := &AuthDiagnosticDetails{
		ContractIDs:      allIDs,
		CallerContractID: caller,
		CalleeContractID: callee,
	}

	var summary string
	switch {
	case caller != "" && callee != "":
		summary = fmt.Sprintf(
			"Authorization failure: contract %s invoked contract %s which could not satisfy the required auth check.",
			caller, callee,
		)
	case callee != "":
		summary = fmt.Sprintf(
			"Authorization failure: contract %s could not satisfy a required auth check.",
			callee,
		)
	default:
		summary = "Authorization failure: a required signature or auth entry was absent or invalid."
	}

	return &FailureDiagnostic{
		Category:     FailureAuthFailure,
		Summary:      summary,
		ErrorCode:    errCode,
		ErrorMessage: errMsg,
		AuthDetails:  details,
	}
}

func buildTrapDiagnostic(errCode, errMsg string, stackTrace *WasmStackTrace) *FailureDiagnostic {
	details := &TrapDiagnosticDetails{}
	if stackTrace != nil {
		details.RawMessage = stackTrace.RawMessage
		details.FrameCount = len(stackTrace.Frames)
		details.SorobanWrapped = stackTrace.SorobanWrapped
		if stackTrace.TrapKind != nil {
			details.TrapKind = fmt.Sprintf("%v", stackTrace.TrapKind)
		}
		if len(stackTrace.Frames) > 0 {
			top := stackTrace.Frames[0]
			details.TopFrame = &top
		}
	}

	summary := buildTrapSummary(errMsg, details)

	return &FailureDiagnostic{
		Category:     FailureContractTrap,
		Summary:      summary,
		ErrorCode:    errCode,
		ErrorMessage: errMsg,
		TrapDetails:  details,
	}
}

func buildTrapSummary(errMsg string, details *TrapDiagnosticDetails) string {
	lc := strings.ToLower(errMsg)
	switch {
	case strings.Contains(lc, "panic:"):
		// Extract the panic message after "panic:"
		idx := strings.Index(strings.ToLower(errMsg), "panic:")
		panicMsg := strings.TrimSpace(errMsg[idx+6:])
		if len(panicMsg) > 120 {
			panicMsg = panicMsg[:120] + "..."
		}
		return fmt.Sprintf("Contract trap: explicit panic — %s", panicMsg)
	case strings.Contains(lc, "unreachable"):
		return "Contract trap: WASM unreachable instruction executed (likely an explicit panic or assertion failure in the contract)."
	case strings.Contains(lc, "stack overflow"):
		return "Contract trap: WASM stack overflow (possible infinite recursion in the contract)."
	case strings.Contains(lc, "out of bounds"):
		return "Contract trap: WASM out-of-bounds memory access."
	case strings.Contains(lc, "integer divide by zero"):
		return "Contract trap: integer divide-by-zero."
	case strings.Contains(lc, "integer overflow"):
		return "Contract trap: integer overflow."
	case details != nil && details.TrapKind != "":
		return fmt.Sprintf("Contract trap: %s.", details.TrapKind)
	default:
		return "Contract trap: a fatal WASM trap occurred inside the contract."
	}
}

func buildValidationDiagnostic(errCode, errMsg string) *FailureDiagnostic {
	field, reason := extractValidationDetails(errCode, errMsg)
	details := &ValidationDiagnosticDetails{
		Field:  field,
		Reason: reason,
	}

	summary := "Ledger-level validation error"
	if reason != "" {
		summary = fmt.Sprintf("Ledger-level validation error: %s.", reason)
	} else if errMsg != "" {
		msg := errMsg
		if len(msg) > 120 {
			msg = msg[:120] + "..."
		}
		summary = fmt.Sprintf("Ledger-level validation error: %s.", msg)
	}

	return &FailureDiagnostic{
		Category:          FailureValidation,
		Summary:           summary,
		ErrorCode:         errCode,
		ErrorMessage:      errMsg,
		ValidationDetails: details,
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// extractAuthContractIDs returns the last two distinct contract IDs from
// diagnostic events, treating the earlier as caller and the later as callee.
func extractAuthContractIDs(events []DiagnosticEvent) (caller, callee string) {
	ids := collectDistinctContractIDs(events)
	switch len(ids) {
	case 0:
		return "", ""
	case 1:
		return "", ids[0]
	default:
		return ids[len(ids)-2], ids[len(ids)-1]
	}
}

// collectDistinctContractIDs returns all distinct contract IDs from events,
// preserving first-seen order.
func collectDistinctContractIDs(events []DiagnosticEvent) []string {
	seen := make(map[string]struct{})
	var ids []string
	for _, e := range events {
		if e.ContractID == nil {
			continue
		}
		id := *e.ContractID
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids
}

// extractValidationDetails maps known error codes and messages to a field/reason pair.
func extractValidationDetails(errCode, errMsg string) (field, reason string) {
	lc := strings.ToLower(errMsg)
	switch {
	case strings.Contains(lc, "decode envelope") || strings.Contains(lc, "envelope_xdr"):
		return "envelope_xdr", "failed to decode transaction envelope XDR"
	case strings.Contains(lc, "decode ledgerkey") || strings.Contains(lc, "ledger_key"):
		return "ledger_key", "failed to decode ledger key XDR"
	case strings.Contains(lc, "decode ledgerentry") || strings.Contains(lc, "ledger_entry"):
		return "ledger_entry", "failed to decode ledger entry XDR"
	case strings.Contains(lc, "decode wasm"):
		return "wasm", "failed to decode WASM module"
	case strings.Contains(lc, "tx_soroban_invalid"):
		return "transaction", "Soroban-specific transaction validation failed"
	case strings.Contains(lc, "tx_malformed") || strings.Contains(lc, "malformed"):
		return "transaction", "transaction is malformed or has invalid parameters"
	case strings.Contains(lc, "bad sequence") || strings.Contains(lc, "tx_bad_seq"):
		return "sequence_number", "sequence number does not match source account"
	case strings.Contains(lc, "insufficient fee") || strings.Contains(lc, "tx_insufficient_fee"):
		return "fee", "fee is too small for the transaction"
	case errCode == "VALIDATION_FAILED" || errCode == "VALIDATION_ERROR":
		return "", "request failed schema validation"
	default:
		return "", ""
	}
}

func summarizeUnknown(errMsg string) string {
	if errMsg == "" {
		return "Simulation failed with no diagnostic information; inspect the raw XDR for details."
	}
	msg := errMsg
	if len(msg) > 200 {
		msg = msg[:200] + "..."
	}
	return fmt.Sprintf("Simulation failed: %s", msg)
}
