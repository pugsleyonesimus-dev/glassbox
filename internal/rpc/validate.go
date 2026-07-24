package rpc

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ValidationError is returned when an RPC response fails field-level
// validation at the boundary.  It always includes the endpoint and method so
// that diagnostics can locate the failure without logging response bodies.
type ValidationError struct {
	Endpoint string
	Method   string
	Field    string
	Reason   string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("rpc validation failed [%s %s] field=%q: %s",
		e.Endpoint, e.Method, e.Field, e.Reason)
}

// ErrValidation is the sentinel used with errors.Is to detect any validation
// failure without inspecting the specific field.
var ErrValidation = errors.New("rpc validation error")

func (e *ValidationError) Is(target error) bool {
	return target == ErrValidation
}

// validateResponse checks that raw is a well-formed JSON-RPC 2.0 response and
// returns a parsed Response.  It checks:
//   - The envelope is a JSON object (not null, array, or scalar).
//   - "jsonrpc" is present and equals "2.0".
//   - Exactly one of "result" or "error" is present.
//   - Error envelopes have integer "code" and non-empty "message".
//
// Unknown additive top-level fields are silently accepted.
// No raw response body is included in any returned error.
func validateResponse(raw []byte, endpoint, method string) (*Response, error) {
	if len(raw) == 0 {
		return nil, &ValidationError{
			Endpoint: endpoint, Method: method,
			Field: "(body)", Reason: "empty response body",
		}
	}

	// First byte check: must be an object.
	for _, b := range raw {
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			continue
		}
		if b != '{' {
			return nil, &ValidationError{
				Endpoint: endpoint, Method: method,
				Field: "(body)", Reason: fmt.Sprintf("expected JSON object, got %q", string(b)),
			}
		}
		break
	}

	// Partial decode to check required fields without capturing everything.
	var partial struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   *RPCError       `json:"error"`
	}
	if err := json.Unmarshal(raw, &partial); err != nil {
		return nil, &ValidationError{
			Endpoint: endpoint, Method: method,
			Field: "(body)", Reason: "malformed JSON",
		}
	}

	if partial.JSONRPC != "2.0" {
		return nil, &ValidationError{
			Endpoint: endpoint, Method: method,
			Field: "jsonrpc", Reason: fmt.Sprintf("expected \"2.0\", got %q", partial.JSONRPC),
		}
	}

	hasResult := len(partial.Result) > 0 && string(partial.Result) != "null"
	hasError := partial.Error != nil

	if !hasResult && !hasError {
		return nil, &ValidationError{
			Endpoint: endpoint, Method: method,
			Field: "result/error", Reason: "response has neither result nor error",
		}
	}

	if hasResult && hasError {
		return nil, &ValidationError{
			Endpoint: endpoint, Method: method,
			Field: "result/error", Reason: "response has both result and error",
		}
	}

	// Validate error envelope when present.
	if hasError {
		if partial.Error.Message == "" {
			return nil, &ValidationError{
				Endpoint: endpoint, Method: method,
				Field: "error.message", Reason: "error message is empty",
			}
		}
	}

	return &Response{
		JSONRPC: partial.JSONRPC,
		ID:      partial.ID,
		Result:  partial.Result,
		Error:   partial.Error,
	}, nil
}

// validateTransactionResponse validates and unmarshals a TransactionResponse
// from a raw JSON result field.
func validateTransactionResponse(raw []byte, endpoint, method string) (*TransactionResponse, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, &ValidationError{
			Endpoint: endpoint, Method: method,
			Field: "result", Reason: "null transaction result",
		}
	}

	var r TransactionResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, &ValidationError{
			Endpoint: endpoint, Method: method,
			Field: "result", Reason: "cannot decode transaction response",
		}
	}

	if r.Status == "" {
		return nil, &ValidationError{
			Endpoint: endpoint, Method: method,
			Field: "result.status", Reason: "missing required field",
		}
	}
	return &r, nil
}

// validateLedgerResponse validates a LedgerResponse.
func validateLedgerResponse(raw []byte, endpoint, method string) (*LedgerResponse, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, &ValidationError{
			Endpoint: endpoint, Method: method,
			Field: "result", Reason: "null ledger result",
		}
	}

	var r LedgerResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, &ValidationError{
			Endpoint: endpoint, Method: method,
			Field: "result", Reason: "cannot decode ledger response",
		}
	}

	if r.Sequence == 0 {
		return nil, &ValidationError{
			Endpoint: endpoint, Method: method,
			Field: "result.sequence", Reason: "missing or zero sequence",
		}
	}
	return &r, nil
}

// validateFootprintResponse validates a FootprintResponse.
func validateFootprintResponse(raw []byte, endpoint, method string) (*FootprintResponse, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, &ValidationError{
			Endpoint: endpoint, Method: method,
			Field: "result", Reason: "null footprint result",
		}
	}

	var r FootprintResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, &ValidationError{
			Endpoint: endpoint, Method: method,
			Field: "result", Reason: "cannot decode footprint response",
		}
	}
	return &r, nil
}

// validateSimulationResponse validates a SimulationResponse.
func validateSimulationResponse(raw []byte, endpoint, method string) (*SimulationResponse, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, &ValidationError{
			Endpoint: endpoint, Method: method,
			Field: "result", Reason: "null simulation result",
		}
	}

	var r SimulationResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, &ValidationError{
			Endpoint: endpoint, Method: method,
			Field: "result", Reason: "cannot decode simulation response",
		}
	}
	return &r, nil
}
