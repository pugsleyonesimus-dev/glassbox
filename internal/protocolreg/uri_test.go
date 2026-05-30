// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package protocolreg

import (
	"strings"
	"testing"
)

// validHash is a 64-character hex string used across tests.
const validHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// baseURI is the minimal valid URI (no optional params).
const baseURI = "glassbox://debug/" + validHash + "?network=testnet"

// ─── Backward-compatible baseline ────────────────────────────────────────────

func TestParseDebugURI(t *testing.T) {
	parsed, err := ParseDebugURI("glassbox://debug/" + validHash + "?network=testnet&operation=2&source=dashboard")
	if err != nil {
		t.Fatalf("ParseDebugURI returned error: %v", err)
	}

	if parsed.TransactionHash != validHash {
		t.Fatalf("unexpected transaction hash: %s", parsed.TransactionHash)
	}
	if parsed.Network != "testnet" {
		t.Fatalf("unexpected network: %s", parsed.Network)
	}
	// Operation (legacy field) must still be populated.
	if parsed.Operation == nil || *parsed.Operation != 2 {
		t.Fatalf("unexpected Operation: %#v", parsed.Operation)
	}
	// Op (new field) must mirror Operation.
	if parsed.Op == nil || *parsed.Op != 2 {
		t.Fatalf("unexpected Op: %#v", parsed.Op)
	}
	if parsed.Source != "dashboard" {
		t.Fatalf("unexpected source: %s", parsed.Source)
	}
}

func TestParseDebugURIRejectsInvalidValues(t *testing.T) {
	tests := []string{
		"",
		"https://example.com",
		"glassbox://decode/" + validHash + "?network=testnet",
		"glassbox://debug/not-a-hash?network=testnet",
		"glassbox://debug/" + validHash,                                    // missing network
		"glassbox://debug/" + validHash + "?network=invalid",               // bad network
		"glassbox://debug/" + validHash + "?network=testnet&operation=-1",  // negative op
	}

	for _, tc := range tests {
		if _, err := ParseDebugURI(tc); err == nil {
			t.Fatalf("expected ParseDebugURI to fail for %q", tc)
		}
	}
}

// ─── Network parameter ───────────────────────────────────────────────────────

func TestParseDebugURI_AllValidNetworks(t *testing.T) {
	networks := []string{"testnet", "mainnet", "futurenet"}
	for _, net := range networks {
		uri := "glassbox://debug/" + validHash + "?network=" + net
		parsed, err := ParseDebugURI(uri)
		if err != nil {
			t.Errorf("network=%q: unexpected error: %v", net, err)
			continue
		}
		if parsed.Network != net {
			t.Errorf("network=%q: got %q", net, parsed.Network)
		}
	}
}

func TestParseDebugURI_InvalidNetworks(t *testing.T) {
	bad := []string{"", "staging", "TESTNET", "Testnet", "local", "devnet"}
	for _, net := range bad {
		uri := "glassbox://debug/" + validHash + "?network=" + net
		_, err := ParseDebugURI(uri)
		if err == nil {
			t.Errorf("network=%q: expected error, got nil", net)
		}
	}
}

func TestParseDebugURI_MissingNetwork_ErrorMentionsParam(t *testing.T) {
	_, err := ParseDebugURI("glassbox://debug/" + validHash)
	if err == nil {
		t.Fatal("expected error for missing network")
	}
	if !strings.Contains(err.Error(), "network") {
		t.Errorf("error should mention 'network', got: %v", err)
	}
}

// ─── op parameter ────────────────────────────────────────────────────────────

func TestParseDebugURI_OpParam(t *testing.T) {
	uri := baseURI + "&op=0"
	parsed, err := ParseDebugURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Op == nil || *parsed.Op != 0 {
		t.Errorf("expected Op=0, got %v", parsed.Op)
	}
	// Operation must mirror Op.
	if parsed.Operation == nil || *parsed.Operation != 0 {
		t.Errorf("expected Operation=0 (mirror of Op), got %v", parsed.Operation)
	}
}

func TestParseDebugURI_OpParam_LargeIndex(t *testing.T) {
	uri := baseURI + "&op=99"
	parsed, err := ParseDebugURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Op == nil || *parsed.Op != 99 {
		t.Errorf("expected Op=99, got %v", parsed.Op)
	}
}

// "op" takes precedence over "operation" when both are present.
func TestParseDebugURI_OpTakesPrecedenceOverOperation(t *testing.T) {
	uri := baseURI + "&op=3&operation=7"
	parsed, err := ParseDebugURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Op == nil || *parsed.Op != 3 {
		t.Errorf("expected Op=3 (op wins), got %v", parsed.Op)
	}
}

func TestParseDebugURI_OperationParam_LegacyAlias(t *testing.T) {
	uri := baseURI + "&operation=5"
	parsed, err := ParseDebugURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Op == nil || *parsed.Op != 5 {
		t.Errorf("expected Op=5 via legacy 'operation' param, got %v", parsed.Op)
	}
	if parsed.Operation == nil || *parsed.Operation != 5 {
		t.Errorf("expected Operation=5, got %v", parsed.Operation)
	}
}

func TestParseDebugURI_InvalidOpValues(t *testing.T) {
	bad := []string{"-1", "-100", "abc", "1.5", " ", "2147483648000"}
	for _, v := range bad {
		uri := baseURI + "&op=" + v
		_, err := ParseDebugURI(uri)
		if err == nil {
			t.Errorf("op=%q: expected error, got nil", v)
		}
	}
}

func TestParseDebugURI_NoOp_FieldIsNil(t *testing.T) {
	parsed, err := ParseDebugURI(baseURI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Op != nil {
		t.Errorf("expected Op=nil when not specified, got %v", *parsed.Op)
	}
	if parsed.Operation != nil {
		t.Errorf("expected Operation=nil when not specified, got %v", *parsed.Operation)
	}
}

// ─── view parameter ──────────────────────────────────────────────────────────

func TestParseDebugURI_AllValidViews(t *testing.T) {
	views := []string{"trace", "flamegraph", "events", "auth", "budget", "storage"}
	for _, v := range views {
		uri := baseURI + "&view=" + v
		parsed, err := ParseDebugURI(uri)
		if err != nil {
			t.Errorf("view=%q: unexpected error: %v", v, err)
			continue
		}
		if parsed.View != v {
			t.Errorf("view=%q: got %q", v, parsed.View)
		}
	}
}

func TestParseDebugURI_InvalidViews(t *testing.T) {
	bad := []string{"unknown", "TRACE", "Flamegraph", "raw", "json", "hex"}
	for _, v := range bad {
		uri := baseURI + "&view=" + v
		_, err := ParseDebugURI(uri)
		if err == nil {
			t.Errorf("view=%q: expected error, got nil", v)
		}
	}
}

func TestParseDebugURI_InvalidView_ErrorMentionsAllowed(t *testing.T) {
	_, err := ParseDebugURI(baseURI + "&view=unknown")
	if err == nil {
		t.Fatal("expected error for invalid view")
	}
	// Error should list the allowed values so the user knows what to use.
	for _, allowed := range []string{"trace", "flamegraph", "events"} {
		if !strings.Contains(err.Error(), allowed) {
			t.Errorf("error should mention allowed view %q, got: %v", allowed, err)
		}
	}
}

func TestParseDebugURI_NoView_FieldIsEmpty(t *testing.T) {
	parsed, err := ParseDebugURI(baseURI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.View != "" {
		t.Errorf("expected View=\"\" when not specified, got %q", parsed.View)
	}
}

// ─── Combined parameters ─────────────────────────────────────────────────────

// Acceptance-criteria URI from the spec: glassbox://debug/?network=testnet&op=0
// Note: the spec shows no tx hash in the example, but our parser requires one.
// We test the closest valid form: glassbox://debug/<hash>?network=testnet&op=0
func TestParseDebugURI_AcceptanceCriteria_NetworkAndOp(t *testing.T) {
	uri := "glassbox://debug/" + validHash + "?network=testnet&op=0"
	parsed, err := ParseDebugURI(uri)
	if err != nil {
		t.Fatalf("acceptance-criteria URI failed: %v", err)
	}
	if parsed.Network != "testnet" {
		t.Errorf("expected network=testnet, got %q", parsed.Network)
	}
	if parsed.Op == nil || *parsed.Op != 0 {
		t.Errorf("expected op=0, got %v", parsed.Op)
	}
}

func TestParseDebugURI_AllParams(t *testing.T) {
	uri := "glassbox://debug/" + validHash + "?network=futurenet&op=2&view=flamegraph&source=ci&signature=abc"
	parsed, err := ParseDebugURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.TransactionHash != validHash {
		t.Errorf("hash mismatch")
	}
	if parsed.Network != "futurenet" {
		t.Errorf("expected futurenet, got %q", parsed.Network)
	}
	if parsed.Op == nil || *parsed.Op != 2 {
		t.Errorf("expected op=2, got %v", parsed.Op)
	}
	if parsed.View != "flamegraph" {
		t.Errorf("expected view=flamegraph, got %q", parsed.View)
	}
	if parsed.Source != "ci" {
		t.Errorf("expected source=ci, got %q", parsed.Source)
	}
	if parsed.Signature != "abc" {
		t.Errorf("expected signature=abc, got %q", parsed.Signature)
	}
	if parsed.Raw != uri {
		t.Errorf("Raw field should preserve original URI")
	}
}

// ─── Transaction hash validation ─────────────────────────────────────────────

func TestParseDebugURI_HashCaseInsensitive(t *testing.T) {
	// Uppercase hex should be accepted.
	upperHash := "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF"
	uri := "glassbox://debug/" + upperHash + "?network=testnet"
	parsed, err := ParseDebugURI(uri)
	if err != nil {
		t.Fatalf("uppercase hash should be accepted: %v", err)
	}
	if parsed.TransactionHash != upperHash {
		t.Errorf("hash should be preserved as-is: got %q", parsed.TransactionHash)
	}
}

func TestParseDebugURI_ShortHash_Rejected(t *testing.T) {
	shortHash := "0123456789abcdef"
	uri := "glassbox://debug/" + shortHash + "?network=testnet"
	_, err := ParseDebugURI(uri)
	if err == nil {
		t.Error("short hash should be rejected")
	}
}

func TestParseDebugURI_EmptyHash_Rejected(t *testing.T) {
	uri := "glassbox://debug/?network=testnet"
	_, err := ParseDebugURI(uri)
	if err == nil {
		t.Error("empty hash should be rejected")
	}
}

// ─── Malformed URI diagnostics ───────────────────────────────────────────────

func TestParseDebugURI_WrongScheme_ErrorMentionsScheme(t *testing.T) {
	_, err := ParseDebugURI("https://debug/" + validHash + "?network=testnet")
	if err == nil {
		t.Fatal("expected error for wrong scheme")
	}
	if !strings.Contains(err.Error(), "glassbox") {
		t.Errorf("error should mention expected scheme, got: %v", err)
	}
}

func TestParseDebugURI_WrongHost_ErrorMentionsDebug(t *testing.T) {
	_, err := ParseDebugURI("glassbox://inspect/" + validHash + "?network=testnet")
	if err == nil {
		t.Fatal("expected error for wrong host")
	}
	if !strings.Contains(err.Error(), "debug") {
		t.Errorf("error should mention expected host 'debug', got: %v", err)
	}
}

func TestParseDebugURI_InvalidOpNonNumeric_ErrorMentionsParam(t *testing.T) {
	_, err := ParseDebugURI(baseURI + "&op=notanumber")
	if err == nil {
		t.Fatal("expected error for non-numeric op")
	}
	if !strings.Contains(err.Error(), "operation index") {
		t.Errorf("error should mention 'operation index', got: %v", err)
	}
}

func TestParseDebugURI_InvalidNetwork_ErrorMentionsAllowed(t *testing.T) {
	_, err := ParseDebugURI("glassbox://debug/" + validHash + "?network=badnet")
	if err == nil {
		t.Fatal("expected error for invalid network")
	}
	for _, allowed := range []string{"testnet", "mainnet", "futurenet"} {
		if !strings.Contains(err.Error(), allowed) {
			t.Errorf("error should mention allowed network %q, got: %v", allowed, err)
		}
	}
}
