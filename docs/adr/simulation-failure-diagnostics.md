# Simulation Failure Diagnostics

## Overview

When a Soroban transaction simulation fails, Glassbox classifies the failure into
one of five well-defined diagnostic categories and exposes structured details in
both CLI text output and JSON export mode.

---

## Failure Categories

| Category | Constant | Meaning |
|---|---|---|
| `CPU_BUDGET_EXCEEDED` | `FailureCPUBudget` | The contract consumed all available Soroban CPU instruction budget before completing. |
| `MEMORY_BUDGET_EXCEEDED` | `FailureMemoryBudget` | The contract consumed all available Soroban memory allocation budget before completing. |
| `AUTH_FAILURE` | `FailureAuthFailure` | A required authorization entry or signature was absent or invalid. Includes cross-contract auth failures. |
| `CONTRACT_TRAP` | `FailureContractTrap` | A fatal WASM trap occurred inside the contract: explicit panic, unreachable instruction, stack overflow, out-of-bounds memory access, or integer fault. |
| `VALIDATION_ERROR` | `FailureValidation` | A ledger-level or protocol-level validation error: malformed XDR, bad sequence number, Soroban-invalid transaction, or schema validation failure. |
| `UNKNOWN` | `FailureUnknown` | No specific category could be determined. Inspect the raw error message and XDR. |

---

## Classification Priority

When multiple signals are present, the classifier applies them in this order
(highest priority first):

1. **CPU budget exhaustion** — `BudgetUsage.CPUUsagePercent >= 100`, or error code
   `ERR_CPU_LIMIT_EXCEEDED`, or message contains `cpulimitexceeded` /
   `error(budget, cpu`.
2. **Memory budget exhaustion** — `BudgetUsage.MemoryUsagePercent >= 100`, or error
   code `ERR_MEMORY_LIMIT_EXCEEDED`, or message contains `memlimitexceeded` /
   `error(budget, mem`.
3. **Auth failure** — message contains `error(auth,`, `require_auth`,
   `not authorized`, `auth failed`, `missing authorization`, `unauthorized`.
4. **Contract trap** — `StackTrace` is non-nil, or error code `SIM_PROCESS_CRASHED` /
   `WASM_TRAP` / `CONTRACT_TRAP`, or message contains `wasm trap`, `unreachable`,
   `panic:`, `stack overflow`, `out of bounds`, `integer divide by zero`.
5. **Validation error** — error code `VALIDATION_FAILED` / `TX_SOROBAN_INVALID` /
   `TX_MALFORMED` / `INVALID_INPUT`, or message contains `decode envelope`,
   `tx_soroban_invalid`, `bad sequence`, `insufficient fee`.
6. **Unknown** — none of the above matched.

---

## Structured Output

### Go API

```go
import "github.com/dotandev/glassbox/internal/simulator"

resp, err := runner.Run(ctx, req)
// resp.Status == "error" after a failed simulation
diag := simulator.ClassifyFailure(resp)
if diag != nil {
    fmt.Println(diag)                  // "[CPU_BUDGET_EXCEEDED] Contract execution exhausted..."
    fmt.Println(diag.Category)         // "CPU_BUDGET_EXCEEDED"
    fmt.Println(diag.Summary)          // human-readable sentence
    if diag.BudgetDetails != nil {
        fmt.Println(diag.BudgetDetails.CPUUsagePercent) // e.g. 100.0
    }
}
```

### JSON structure

`ClassifyFailure` returns a `*FailureDiagnostic` which serialises to:

```json
{
  "category": "CPU_BUDGET_EXCEEDED",
  "summary": "Contract execution exhausted the Soroban CPU instruction budget: 100000000/100000000 instructions used (100.0%).",
  "error_code": "ERR_CPU_LIMIT_EXCEEDED",
  "error_message": "cpu limit exceeded",
  "budget_details": {
    "cpu_instructions": 100000000,
    "cpu_limit": 100000000,
    "cpu_usage_percent": 100.0,
    "memory_bytes": 12000000,
    "memory_limit": 50000000,
    "memory_usage_percent": 24.0,
    "cpu_exhausted": true,
    "memory_exhausted": false
  }
}
```

Category-specific detail objects:

| Category | Detail field | Type |
|---|---|---|
| `CPU_BUDGET_EXCEEDED` / `MEMORY_BUDGET_EXCEEDED` | `budget_details` | `BudgetDiagnosticDetails` |
| `CONTRACT_TRAP` | `trap_details` | `TrapDiagnosticDetails` |
| `AUTH_FAILURE` | `auth_details` | `AuthDiagnosticDetails` |
| `VALIDATION_ERROR` | `validation_details` | `ValidationDiagnosticDetails` |

#### `BudgetDiagnosticDetails`

```json
{
  "cpu_instructions": 100000000,
  "cpu_limit": 100000000,
  "cpu_usage_percent": 100.0,
  "memory_bytes": 12000000,
  "memory_limit": 50000000,
  "memory_usage_percent": 24.0,
  "cpu_exhausted": true,
  "memory_exhausted": false
}
```

#### `TrapDiagnosticDetails`

```json
{
  "trap_kind": "Unreachable",
  "raw_message": "wasm trap: unreachable",
  "frame_count": 3,
  "top_frame": {
    "index": 0,
    "func_index": 42,
    "func_name": "my_contract_fn",
    "wasm_offset": 1234
  },
  "soroban_wrapped": true
}
```

#### `AuthDiagnosticDetails`

```json
{
  "contract_ids": [
    "CAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABSC4",
    "CBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBSC4X"
  ],
  "caller_contract_id": "CAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABSC4",
  "callee_contract_id": "CBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBSC4X"
}
```

#### `ValidationDiagnosticDetails`

```json
{
  "field": "envelope_xdr",
  "reason": "failed to decode transaction envelope XDR"
}
```

---

## CLI Text Output

The `FailureDiagnostic.String()` method returns a compact one-liner suitable for
terminal output:

```
[CPU_BUDGET_EXCEEDED] Contract execution exhausted the Soroban CPU instruction budget: 100000000/100000000 instructions used (100.0%).
```

---

## Source Location

- Classifier: `internal/simulator/failure_classifier.go`
- Tests: `internal/simulator/failure_classifier_test.go`
