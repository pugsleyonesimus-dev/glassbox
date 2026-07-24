package network_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/drips/glassbox/internal/network"
)

func mainnet() network.Snapshot {
	return network.Snapshot{
		NetworkName:     "mainnet",
		Passphrase:      "Public Global Stellar Network ; September 2015",
		RPCEndpoint:     "https://soroban-rpc.mainnet.stellar.gateway.fm",
		ProtocolVersion: 21,
		NodeID:          "node-abc",
		ExtraMetadata:   map[string]string{"region": "us-east-1"},
	}
}

func testnet() network.Snapshot {
	return network.Snapshot{
		NetworkName:     "testnet",
		Passphrase:      "Test SDF Network ; September 2015",
		RPCEndpoint:     "https://soroban-testnet.stellar.org",
		ProtocolVersion: 21,
	}
}

// ---------------------------------------------------------------------------
// Checksum tests
// ---------------------------------------------------------------------------

func TestChecksum_Deterministic(t *testing.T) {
	s := mainnet()
	c1, err := s.Checksum()
	if err != nil {
		t.Fatal(err)
	}
	c2, err := s.Checksum()
	if err != nil {
		t.Fatal(err)
	}
	if c1 != c2 {
		t.Fatalf("checksum is not deterministic: %s vs %s", c1, c2)
	}
}

func TestChecksum_ChangesOnFieldChange(t *testing.T) {
	s1 := mainnet()
	s2 := mainnet()
	s2.Passphrase = "Different Passphrase"

	c1, _ := s1.Checksum()
	c2, _ := s2.Checksum()
	if c1 == c2 {
		t.Fatal("checksum must differ when passphrase changes")
	}
}

func TestChecksum_DeterministicExtraMetadataOrder(t *testing.T) {
	s1 := mainnet()
	s1.ExtraMetadata = map[string]string{"b": "2", "a": "1"}

	s2 := mainnet()
	s2.ExtraMetadata = map[string]string{"a": "1", "b": "2"}

	c1, _ := s1.Checksum()
	c2, _ := s2.Checksum()
	if c1 != c2 {
		t.Fatal("checksum must be same regardless of map insertion order")
	}
}

// ---------------------------------------------------------------------------
// JSON serialization tests
// ---------------------------------------------------------------------------

func TestMarshalJSON_Roundtrip(t *testing.T) {
	original := mainnet()
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded network.Snapshot
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Passphrase != original.Passphrase {
		t.Fatalf("passphrase mismatch after round-trip")
	}
	if decoded.ProtocolVersion != original.ProtocolVersion {
		t.Fatalf("protocol version mismatch after round-trip")
	}
}

// ---------------------------------------------------------------------------
// Compare tests
// ---------------------------------------------------------------------------

func TestCompare_Compatible(t *testing.T) {
	s := mainnet()
	result := network.Compare(s, s)
	if !result.Compatible {
		t.Fatalf("identical snapshots must be compatible: %v", result.Errors)
	}
	if len(result.ActiveOverrides) != 0 {
		t.Fatal("no overrides expected for identical snapshots")
	}
}

func TestCompare_PassphraseMismatch_Rejected(t *testing.T) {
	capture := mainnet()
	replay := testnet() // different passphrase
	replay.RPCEndpoint = capture.RPCEndpoint
	replay.ProtocolVersion = capture.ProtocolVersion

	result := network.Compare(capture, replay)
	if result.Compatible {
		t.Fatal("passphrase mismatch must be rejected by default")
	}

	var ce *network.CompatibilityError
	if !errors.As(result.Errors[0], &ce) {
		t.Fatalf("expected CompatibilityError, got %T", result.Errors[0])
	}
	if ce.Field != "passphrase" {
		t.Fatalf("expected field=passphrase, got %q", ce.Field)
	}
}

func TestCompare_PassphraseMismatch_Override(t *testing.T) {
	capture := mainnet()
	replay := testnet()
	replay.RPCEndpoint = capture.RPCEndpoint
	replay.ProtocolVersion = capture.ProtocolVersion

	result := network.Compare(capture, replay, network.AllowPassphraseMismatch())
	if !result.Compatible {
		t.Fatalf("override should make result compatible: %v", result.Errors)
	}
	if len(result.ActiveOverrides) == 0 {
		t.Fatal("active overrides must be recorded")
	}
}

func TestCompare_OverridesDescription_NonEmpty(t *testing.T) {
	capture := mainnet()
	replay := testnet()
	replay.RPCEndpoint = capture.RPCEndpoint
	replay.ProtocolVersion = capture.ProtocolVersion

	result := network.Compare(capture, replay, network.AllowPassphraseMismatch())
	desc := network.OverridesDescription(result)
	if desc == "" {
		t.Fatal("expected non-empty description when overrides are active")
	}
}

func TestCompare_OverridesDescription_Empty(t *testing.T) {
	s := mainnet()
	result := network.Compare(s, s)
	if desc := network.OverridesDescription(result); desc != "" {
		t.Fatalf("expected empty description, got %q", desc)
	}
}

func TestCompare_FirstError_Sentinel(t *testing.T) {
	capture := mainnet()
	replay := testnet()
	replay.RPCEndpoint = capture.RPCEndpoint
	replay.ProtocolVersion = capture.ProtocolVersion

	result := network.Compare(capture, replay)
	err := result.FirstError()
	if err == nil {
		t.Fatal("expected error from incompatible snapshots")
	}
	if !errors.Is(err, network.ErrIncompatibleNetwork) {
		t.Fatalf("error must wrap ErrIncompatibleNetwork, got %v", err)
	}
}

func TestCompare_CustomNetwork_PassphraseMismatch(t *testing.T) {
	custom := network.Snapshot{
		NetworkName:     "custom",
		Passphrase:      "My Custom Network ; 2024",
		RPCEndpoint:     "https://rpc.custom.example.com",
		ProtocolVersion: 20,
	}
	replay := custom
	replay.Passphrase = "Different Custom Network ; 2024"

	result := network.Compare(custom, replay)
	if result.Compatible {
		t.Fatal("custom network passphrase change must be rejected")
	}
}

func TestCompare_MultipleFieldMismatches(t *testing.T) {
	capture := mainnet()
	replay := testnet() // passphrase and endpoint differ

	result := network.Compare(capture, replay)
	if result.Compatible {
		t.Fatal("multiple mismatches should be incompatible")
	}
	// Should report at least passphrase and endpoint errors.
	if len(result.Errors) < 2 {
		t.Fatalf("expected at least 2 errors, got %d", len(result.Errors))
	}
}

func TestCompare_AllOverrides(t *testing.T) {
	capture := mainnet()
	replay := testnet()
	replay.ProtocolVersion = 19

	result := network.Compare(capture, replay,
		network.AllowPassphraseMismatch(),
		network.AllowEndpointMismatch(),
		network.AllowProtocolMismatch(),
	)
	if !result.Compatible {
		t.Fatalf("all-override should be compatible: %v", result.Errors)
	}
	if len(result.ActiveOverrides) < 3 {
		t.Fatalf("expected 3 active overrides, got %d", len(result.ActiveOverrides))
	}
}
