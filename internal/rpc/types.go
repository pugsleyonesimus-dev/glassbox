// Package rpc provides a JSON-RPC 2.0 HTTP client for Stellar Soroban RPC
// endpoints.  It integrates:
//   - Per-request correlation IDs drawn from context (feature a)
//   - Response validation at the RPC boundary (feature d)
//   - Telemetry hooks for metrics and progress events (feature a)
//   - Cache integration for immutable responses (feature c)
//
// Unknown additive fields in responses are preserved for forward
// compatibility.  No response body containing credentials is ever logged.
package rpc

import (
	"encoding/json"
	"fmt"
)

// ---------------------------------------------------------------------------
// JSON-RPC 2.0 wire types
// ---------------------------------------------------------------------------

// Request is an outbound JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is the raw inbound JSON-RPC 2.0 response.  Unknown top-level
// fields are preserved in Extra for forward compatibility.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is the standard JSON-RPC error object.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

// ---------------------------------------------------------------------------
// Stellar-specific response shapes
// ---------------------------------------------------------------------------

// TransactionResponse is the validated shape of a getTransaction result.
type TransactionResponse struct {
	Status          string          `json:"status"`
	EnvelopeXDR     string          `json:"envelopeXdr,omitempty"`
	ResultXDR       string          `json:"resultXdr,omitempty"`
	ResultMetaXDR   string          `json:"resultMetaXdr,omitempty"`
	Ledger          uint32          `json:"ledger,omitempty"`
	LedgerCloseTime int64           `json:"ledgerCloseTime,omitempty"`
	// Extra captures any additional fields for forward compatibility.
	Extra map[string]json.RawMessage `json:"-"`
}

// LedgerResponse is the validated shape of a getLedger result.
type LedgerResponse struct {
	Sequence    uint32 `json:"sequence"`
	LedgerXDR   string `json:"ledgerXdr,omitempty"`
	LedgerHash  string `json:"ledgerHash,omitempty"`
}

// SimulationResponse is the validated shape of a simulateTransaction result.
// This type is defined for validation purposes only; simulation results must
// never be cached.
type SimulationResponse struct {
	// Error is set if simulation failed.
	Error string `json:"error,omitempty"`
	// Results holds per-invocation results.
	Results []json.RawMessage `json:"results,omitempty"`
	// LatestLedger is the ledger sequence at simulation time.
	LatestLedger uint32 `json:"latestLedger"`
	// Cost holds resource cost information.
	Cost json.RawMessage `json:"cost,omitempty"`
}

// FootprintResponse is the validated shape of a getLedgerEntries result.
type FootprintResponse struct {
	Entries     []LedgerEntry   `json:"entries,omitempty"`
	LatestLedger uint32         `json:"latestLedger"`
}

// LedgerEntry is a single entry returned by getLedgerEntries.
type LedgerEntry struct {
	Key        string `json:"key"`
	XDR        string `json:"xdr"`
	LastModifiedLedgerSeq uint32 `json:"lastModifiedLedgerSeq,omitempty"`
}

// PaginatedResponse wraps any paginated RPC result.
type PaginatedResponse struct {
	Records json.RawMessage `json:"records"`
	Cursor  string          `json:"cursor,omitempty"`
}
