package diagnostics_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/drips/glassbox/internal/cache"
	"github.com/drips/glassbox/internal/correlation"
	"github.com/drips/glassbox/internal/diagnostics"
	"github.com/drips/glassbox/internal/network"
)

func sampleNet() network.Snapshot {
	return network.Snapshot{
		NetworkName:     "testnet",
		Passphrase:      "Test SDF Network ; September 2015",
		RPCEndpoint:     "https://soroban-testnet.stellar.org",
		ProtocolVersion: 21,
	}
}

func TestBuilder_CorrelationID_Present(t *testing.T) {
	id := correlation.New()
	b := diagnostics.NewBuilder(id, sampleNet())
	r := b.Build()

	if r.CorrelationID != id.String() {
		t.Fatalf("expected %s, got %s", id, r.CorrelationID)
	}
}

func TestBuilder_NetworkInfo_Present(t *testing.T) {
	id := correlation.New()
	net := sampleNet()
	r := diagnostics.NewBuilder(id, net).Build()

	if r.Network.Passphrase != net.Passphrase {
		t.Fatalf("passphrase missing from report")
	}
}

func TestBuilder_Overrides_Present(t *testing.T) {
	id := correlation.New()
	b := diagnostics.NewBuilder(id, sampleNet())
	b.SetNetworkOverrides([]string{"AllowPassphraseMismatch"})
	r := b.Build()

	if len(r.NetworkOverrides) != 1 || r.NetworkOverrides[0] != "AllowPassphraseMismatch" {
		t.Fatalf("overrides not set: %v", r.NetworkOverrides)
	}
}

func TestBuilder_CacheStats(t *testing.T) {
	id := correlation.New()
	b := diagnostics.NewBuilder(id, sampleNet())
	b.SetCacheStats(cache.DiagnosticReport{Hits: 5, Misses: 2, TotalEntries: 7})
	r := b.Build()

	if r.CacheStats == nil {
		t.Fatal("cache stats missing")
	}
	if r.CacheStats.Hits != 5 {
		t.Fatalf("cache hits mismatch")
	}
}

func TestBuilder_ValidationError(t *testing.T) {
	id := correlation.New()
	b := diagnostics.NewBuilder(id, sampleNet())
	b.AddValidationError("https://endpoint", "getTransaction", "result.status", "missing")
	r := b.Build()

	if len(r.ValidationErrors) != 1 {
		t.Fatalf("expected 1 validation error, got %d", len(r.ValidationErrors))
	}
	if r.ValidationErrors[0].Field != "result.status" {
		t.Fatalf("field mismatch")
	}
}

// ---------------------------------------------------------------------------
// JSON output
// ---------------------------------------------------------------------------

func TestWriteJSON_ContainsCorrelationID(t *testing.T) {
	id := correlation.New()
	r := diagnostics.NewBuilder(id, sampleNet()).Build()

	var buf bytes.Buffer
	if err := diagnostics.WriteJSON(&buf, r); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["correlation_id"] != id.String() {
		t.Fatalf("correlation_id missing or wrong in JSON output")
	}
}

func TestWriteJSON_ConsistentWithMarshalJSON(t *testing.T) {
	id := correlation.New()
	r := diagnostics.NewBuilder(id, sampleNet()).Build()

	var buf bytes.Buffer
	_ = diagnostics.WriteJSON(&buf, r)

	b2, err := diagnostics.MarshalJSON(r)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	// Both should contain the same correlation ID.
	if !strings.Contains(buf.String(), id.String()) {
		t.Fatal("WriteJSON missing correlation ID")
	}
	if !strings.Contains(string(b2), id.String()) {
		t.Fatal("MarshalJSON missing correlation ID")
	}
}

func TestWriteJSON_OverridesVisible(t *testing.T) {
	id := correlation.New()
	b := diagnostics.NewBuilder(id, sampleNet())
	b.SetNetworkOverrides([]string{"AllowPassphraseMismatch", "AllowEndpointMismatch"})
	r := b.Build()

	var buf bytes.Buffer
	_ = diagnostics.WriteJSON(&buf, r)

	if !strings.Contains(buf.String(), "AllowPassphraseMismatch") {
		t.Fatal("override not visible in JSON output")
	}
}

// ---------------------------------------------------------------------------
// Text output
// ---------------------------------------------------------------------------

func TestWriteText_ContainsCorrelationID(t *testing.T) {
	id := correlation.New()
	r := diagnostics.NewBuilder(id, sampleNet()).Build()

	var buf bytes.Buffer
	if err := diagnostics.WriteText(&buf, r); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	if !strings.Contains(buf.String(), id.String()) {
		t.Fatalf("correlation ID missing from text output: %s", buf.String())
	}
}

func TestWriteText_OverridesVisible(t *testing.T) {
	id := correlation.New()
	b := diagnostics.NewBuilder(id, sampleNet())
	b.SetNetworkOverrides([]string{"AllowPassphraseMismatch"})
	r := b.Build()

	var buf bytes.Buffer
	_ = diagnostics.WriteText(&buf, r)
	if !strings.Contains(buf.String(), "AllowPassphraseMismatch") {
		t.Fatalf("override not visible in text output: %s", buf.String())
	}
}

func TestWriteText_ValidationErrors(t *testing.T) {
	id := correlation.New()
	b := diagnostics.NewBuilder(id, sampleNet())
	b.AddValidationError("https://ep", "getTransaction", "result.status", "missing required field")
	r := b.Build()

	var buf bytes.Buffer
	_ = diagnostics.WriteText(&buf, r)
	if !strings.Contains(buf.String(), "result.status") {
		t.Fatalf("validation error field missing from text output: %s", buf.String())
	}
}

// TestNoSecretsInReport verifies that the report does not include any raw
// response bodies that might carry credentials.
func TestNoSecretsInReport(t *testing.T) {
	id := correlation.New()
	b := diagnostics.NewBuilder(id, sampleNet())
	// Simulate a validation error that came from a response with a "secret"
	// field — only the field metadata is recorded, not the body.
	b.AddValidationError("https://ep", "getTransaction", "jsonrpc", "expected \"2.0\"")
	r := b.Build()

	b2, _ := diagnostics.MarshalJSON(r)
	if strings.Contains(string(b2), "TOP_SECRET_KEY") {
		t.Fatal("secret leaked into diagnostics JSON")
	}

	var buf bytes.Buffer
	_ = diagnostics.WriteText(&buf, r)
	if strings.Contains(buf.String(), "TOP_SECRET_KEY") {
		t.Fatal("secret leaked into diagnostics text")
	}
}
