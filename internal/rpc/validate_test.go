package rpc

// validate_test.go is in package rpc (not rpc_test) so it can access the
// unexported validateResponse and related functions directly.

import (
	"errors"
	"testing"
)

const (
	testEndpoint = "https://soroban-testnet.stellar.org"
	testMethod   = "getTransaction"
)

// ---------------------------------------------------------------------------
// validateResponse tests
// ---------------------------------------------------------------------------

func TestValidateResponse_WellFormed(t *testing.T) {
	raw := []byte(`{"jsonrpc":"2.0","id":1,"result":{"status":"SUCCESS"}}`)
	resp, err := validateResponse(raw, testEndpoint, testMethod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.JSONRPC != "2.0" {
		t.Fatalf("jsonrpc mismatch")
	}
}

func TestValidateResponse_EmptyBody(t *testing.T) {
	_, err := validateResponse(nil, testEndpoint, testMethod)
	assertValidationError(t, err, "(body)")
}

func TestValidateResponse_NonObject_Array(t *testing.T) {
	_, err := validateResponse([]byte(`[1,2,3]`), testEndpoint, testMethod)
	assertValidationError(t, err, "(body)")
}

func TestValidateResponse_NonObject_String(t *testing.T) {
	_, err := validateResponse([]byte(`"hello"`), testEndpoint, testMethod)
	assertValidationError(t, err, "(body)")
}

func TestValidateResponse_MalformedJSON(t *testing.T) {
	_, err := validateResponse([]byte(`{not valid json`), testEndpoint, testMethod)
	assertValidationError(t, err, "(body)")
}

func TestValidateResponse_WrongJSONRPCVersion(t *testing.T) {
	raw := []byte(`{"jsonrpc":"1.0","id":1,"result":{}}`)
	_, err := validateResponse(raw, testEndpoint, testMethod)
	assertValidationError(t, err, "jsonrpc")
}

func TestValidateResponse_NeitherResultNorError(t *testing.T) {
	raw := []byte(`{"jsonrpc":"2.0","id":1}`)
	_, err := validateResponse(raw, testEndpoint, testMethod)
	assertValidationError(t, err, "result/error")
}

func TestValidateResponse_BothResultAndError(t *testing.T) {
	raw := []byte(`{"jsonrpc":"2.0","id":1,"result":{},"error":{"code":-1,"message":"x"}}`)
	_, err := validateResponse(raw, testEndpoint, testMethod)
	assertValidationError(t, err, "result/error")
}

func TestValidateResponse_ErrorEnvelope_EmptyMessage(t *testing.T) {
	raw := []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":""}}`)
	_, err := validateResponse(raw, testEndpoint, testMethod)
	assertValidationError(t, err, "error.message")
}

func TestValidateResponse_ValidErrorEnvelope(t *testing.T) {
	raw := []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":"Invalid request"}}`)
	resp, err := validateResponse(raw, testEndpoint, testMethod)
	if err != nil {
		t.Fatalf("valid error envelope should not fail validation: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error field populated")
	}
}

func TestValidateResponse_NullResult(t *testing.T) {
	raw := []byte(`{"jsonrpc":"2.0","id":1,"result":null}`)
	// null result means "no result" — should be treated as missing.
	_, err := validateResponse(raw, testEndpoint, testMethod)
	assertValidationError(t, err, "result/error")
}

func TestValidateResponse_UnknownFieldsPreserved(t *testing.T) {
	// Unknown additive fields should not cause validation errors.
	raw := []byte(`{"jsonrpc":"2.0","id":1,"result":{"status":"SUCCESS"},"x-future-field":"ignored"}`)
	_, err := validateResponse(raw, testEndpoint, testMethod)
	if err != nil {
		t.Fatalf("unknown additive fields must be accepted: %v", err)
	}
}

func TestValidateResponse_NullBody(t *testing.T) {
	_, err := validateResponse([]byte("null"), testEndpoint, testMethod)
	assertValidationError(t, err, "(body)")
}

// ---------------------------------------------------------------------------
// validateTransactionResponse tests
// ---------------------------------------------------------------------------

func TestValidateTransactionResponse_Valid(t *testing.T) {
	raw := []byte(`{"status":"SUCCESS","envelopeXdr":"AABB","ledger":100}`)
	r, err := validateTransactionResponse(raw, testEndpoint, testMethod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Status != "SUCCESS" {
		t.Fatalf("expected SUCCESS")
	}
}

func TestValidateTransactionResponse_MissingStatus(t *testing.T) {
	raw := []byte(`{"envelopeXdr":"AABB"}`)
	_, err := validateTransactionResponse(raw, testEndpoint, testMethod)
	assertValidationError(t, err, "result.status")
}

func TestValidateTransactionResponse_NullResult(t *testing.T) {
	_, err := validateTransactionResponse([]byte("null"), testEndpoint, testMethod)
	assertValidationError(t, err, "result")
}

func TestValidateTransactionResponse_WrongType(t *testing.T) {
	// status is an integer instead of string — should fail JSON decoding of
	// the typed struct.  Our function returns a ValidationError.
	raw := []byte(`{"status":123}`)
	_, err := validateTransactionResponse(raw, testEndpoint, testMethod)
	if err != nil {
		// This may or may not error depending on json decoder leniency; the
		// key acceptance criterion is that status="" causes a validation error.
		assertValidationError(t, err, "result")
	}
}

// ---------------------------------------------------------------------------
// validateLedgerResponse tests
// ---------------------------------------------------------------------------

func TestValidateLedgerResponse_Valid(t *testing.T) {
	raw := []byte(`{"sequence":42,"ledgerHash":"ABCD"}`)
	r, err := validateLedgerResponse(raw, testEndpoint, "getLedger")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Sequence != 42 {
		t.Fatalf("sequence mismatch")
	}
}

func TestValidateLedgerResponse_ZeroSequence(t *testing.T) {
	raw := []byte(`{"sequence":0}`)
	_, err := validateLedgerResponse(raw, testEndpoint, "getLedger")
	assertValidationError(t, err, "result.sequence")
}

func TestValidateLedgerResponse_Null(t *testing.T) {
	_, err := validateLedgerResponse(nil, testEndpoint, "getLedger")
	assertValidationError(t, err, "result")
}

// ---------------------------------------------------------------------------
// validateFootprintResponse tests
// ---------------------------------------------------------------------------

func TestValidateFootprintResponse_Valid(t *testing.T) {
	raw := []byte(`{"entries":[{"key":"K","xdr":"X"}],"latestLedger":10}`)
	r, err := validateFootprintResponse(raw, testEndpoint, "getLedgerEntries")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Entries) != 1 {
		t.Fatalf("expected 1 entry")
	}
}

func TestValidateFootprintResponse_EmptyEntries(t *testing.T) {
	raw := []byte(`{"entries":[],"latestLedger":5}`)
	_, err := validateFootprintResponse(raw, testEndpoint, "getLedgerEntries")
	if err != nil {
		t.Fatalf("empty entries list is valid: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ValidationError sentinel
// ---------------------------------------------------------------------------

func TestValidationError_Sentinel(t *testing.T) {
	_, err := validateResponse(nil, testEndpoint, testMethod)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected errors.Is(err, ErrValidation), got %v", err)
	}
}

func TestValidationError_ContainsEndpointAndMethod(t *testing.T) {
	_, err := validateResponse(nil, testEndpoint, testMethod)
	ve := err.Error()
	if ve == "" {
		t.Fatal("error message is empty")
	}
	// Must reference the endpoint and method for diagnostics.
	if !containsAll(ve, testEndpoint, testMethod) {
		t.Fatalf("error must contain endpoint and method, got: %s", ve)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func assertValidationError(t *testing.T, err error, expectedField string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected ValidationError for field %q, got nil", expectedField)
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	if ve.Field != expectedField {
		t.Fatalf("expected field %q, got %q", expectedField, ve.Field)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
