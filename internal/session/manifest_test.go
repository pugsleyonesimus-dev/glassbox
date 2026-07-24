package session_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/drips/glassbox/internal/network"
	"github.com/drips/glassbox/internal/session"
)

func sampleManifest() session.Manifest {
	return session.Manifest{
		FormatVersion: session.Version,
		CapturedAt:    time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Network: network.Snapshot{
			NetworkName:     "mainnet",
			Passphrase:      "Public Global Stellar Network ; September 2015",
			RPCEndpoint:     "https://soroban-rpc.mainnet.stellar.gateway.fm",
			ProtocolVersion: 21,
		},
		CorrelationID:   "op-0001-abcdef",
		TransactionHash: "deadbeef",
		LedgerSequence:  1234567,
		Entries: []session.ArtifactRef{
			{Kind: "envelope", CacheKey: "sha256:aaa", Checksum: "aaa"},
			{Kind: "ledger", CacheKey: "sha256:bbb", Checksum: "bbb"},
		},
	}
}

func TestWriteRead_Roundtrip(t *testing.T) {
	m := sampleManifest()

	var buf bytes.Buffer
	if err := session.Write(&buf, m); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := session.Read(&buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if got.TransactionHash != m.TransactionHash {
		t.Fatalf("tx hash mismatch")
	}
	if got.Network.Passphrase != m.Network.Passphrase {
		t.Fatalf("passphrase mismatch")
	}
	if len(got.Entries) != len(m.Entries) {
		t.Fatalf("entries len mismatch: want %d got %d", len(m.Entries), len(got.Entries))
	}
}

func TestRead_UnsupportedVersion(t *testing.T) {
	m := sampleManifest()
	m.FormatVersion = session.Version + 99

	var buf bytes.Buffer
	if err := session.Write(&buf, m); err != nil {
		t.Fatalf("Write: %v", err)
	}

	_, err := session.Read(&buf)
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
	if !errors.Is(err, session.ErrUnsupportedVersion) {
		t.Fatalf("expected ErrUnsupportedVersion, got %v", err)
	}
}

func TestValidateForReplay_Compatible(t *testing.T) {
	m := sampleManifest()
	result, err := session.ValidateForReplay(m, m.Network)
	if err != nil {
		t.Fatalf("compatible networks should not error: %v", err)
	}
	if !result.Compatible {
		t.Fatal("result should be compatible")
	}
}

func TestValidateForReplay_IncompatiblePassphrase(t *testing.T) {
	m := sampleManifest()
	replayNet := m.Network
	replayNet.Passphrase = "Test SDF Network ; September 2015"

	_, err := session.ValidateForReplay(m, replayNet)
	if err == nil {
		t.Fatal("expected error for passphrase mismatch")
	}
	if !errors.Is(err, network.ErrIncompatibleNetwork) {
		t.Fatalf("expected ErrIncompatibleNetwork, got %v", err)
	}
}

func TestValidateForReplay_Override(t *testing.T) {
	m := sampleManifest()
	replayNet := m.Network
	replayNet.Passphrase = "Test SDF Network ; September 2015"

	result, err := session.ValidateForReplay(m, replayNet, network.AllowPassphraseMismatch())
	if err != nil {
		t.Fatalf("override should suppress error: %v", err)
	}
	if !result.Compatible {
		t.Fatal("result should be compatible with override")
	}
	desc := network.OverridesDescription(result)
	if desc == "" {
		t.Fatal("override must be described in result")
	}
}
