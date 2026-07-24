// Package session defines the session and replay artifact types.  A Manifest
// is serialized to disk (or any io.Writer) when a debug capture is saved and
// read back before replay.  It embeds a network.Snapshot so that the replay
// engine can detect network mismatches before touching any RPC data.
package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/drips/glassbox/internal/network"
)

// Version is the manifest format version.  Increment this when the shape
// changes in a backwards-incompatible way.
const Version = 1

// Manifest is the root artifact written during capture and read during replay.
type Manifest struct {
	// FormatVersion allows future readers to detect format incompatibilities.
	FormatVersion int `json:"format_version"`

	// CapturedAt is the UTC time at which the session was captured.
	CapturedAt time.Time `json:"captured_at"`

	// Network is the canonical network snapshot taken at capture time.
	Network network.Snapshot `json:"network"`

	// CorrelationID is the operation correlation ID from the capture run.  It
	// is preserved so that replay logs can cross-reference the original run.
	CorrelationID string `json:"correlation_id"`

	// TransactionHash is the hex-encoded hash of the inspected transaction.
	TransactionHash string `json:"transaction_hash"`

	// LedgerSequence is the ledger at which the transaction was applied.
	LedgerSequence uint32 `json:"ledger_sequence"`

	// Entries holds the individual cached artifact references (content
	// addresses of envelope, footprint, ledger, and simulation blobs).
	Entries []ArtifactRef `json:"entries,omitempty"`
}

// ArtifactRef is a reference to an individual cached artifact.
type ArtifactRef struct {
	// Kind identifies what the blob contains (e.g. "envelope", "footprint",
	// "ledger", "simulation").
	Kind string `json:"kind"`

	// CacheKey is the content-addressed key used to retrieve the blob from
	// the cache store.
	CacheKey string `json:"cache_key"`

	// Checksum is the hex-encoded SHA-256 of the raw blob for integrity
	// verification on load.
	Checksum string `json:"checksum"`
}

// ErrUnsupportedVersion is returned when a manifest has a format version that
// this build does not understand.
var ErrUnsupportedVersion = errors.New("unsupported manifest format version")

// Write serializes m to w using canonical JSON encoding.
func Write(w io.Writer, m Manifest) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return fmt.Errorf("session.Write: %w", err)
	}
	return nil
}

// Read deserializes a Manifest from r.  It returns ErrUnsupportedVersion if
// the stored FormatVersion is greater than Version.
func Read(r io.Reader) (Manifest, error) {
	var m Manifest
	dec := json.NewDecoder(r)
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("session.Read: %w", err)
	}
	if m.FormatVersion > Version {
		return Manifest{}, fmt.Errorf("%w: stored=%d current=%d",
			ErrUnsupportedVersion, m.FormatVersion, Version)
	}
	return m, nil
}

// ValidateForReplay checks the replay-time network snapshot against the one
// stored in the manifest.  It returns a non-nil error (wrapping
// network.ErrIncompatibleNetwork) if the networks are incompatible and no
// override option permits the difference.
func ValidateForReplay(m Manifest, replayNetwork network.Snapshot, opts ...network.CompareOption) (network.CompareResult, error) {
	result := network.Compare(m.Network, replayNetwork, opts...)
	if !result.Compatible {
		return result, result.FirstError()
	}
	return result, nil
}
