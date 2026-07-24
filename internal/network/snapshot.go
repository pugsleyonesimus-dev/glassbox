// Package network provides canonical network configuration snapshots that are
// captured at debug time and compared before replay.  Mismatches between the
// capture-time and replay-time network abort the replay by default; an
// explicit override is required for intentional cross-network analysis, and
// that override is always visible in reports.
package network

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Snapshot is a canonical, deterministically serializable record of the
// network configuration that was active when a debug session was captured.
// All fields that can change between networks are included so that a replay
// against a different network is detected immediately.
type Snapshot struct {
	// NetworkName is the human-readable network identifier (e.g. "mainnet",
	// "testnet", "custom").
	NetworkName string `json:"network_name"`

	// Passphrase is the Stellar network passphrase.  It is stored because it
	// is the authoritative discriminator between networks.
	Passphrase string `json:"passphrase"`

	// RPCEndpoint is the base URL of the RPC node used during capture.
	RPCEndpoint string `json:"rpc_endpoint"`

	// ProtocolVersion is the Stellar protocol version reported by the node.
	ProtocolVersion uint32 `json:"protocol_version"`

	// NodeID is the self-reported identity of the RPC node (may be empty for
	// older nodes that do not expose it).
	NodeID string `json:"node_id,omitempty"`

	// ExtraMetadata holds any additional key/value pairs that should be
	// preserved across sessions for forward compatibility.  Keys are sorted on
	// serialization so the output is deterministic.
	ExtraMetadata map[string]string `json:"extra_metadata,omitempty"`
}

// Checksum returns a stable hex-encoded SHA-256 digest of the snapshot.  The
// digest is derived from the canonical JSON representation so it is identical
// across processes and architectures.
func (s Snapshot) Checksum() (string, error) {
	b, err := s.marshalCanonical()
	if err != nil {
		return "", fmt.Errorf("network snapshot checksum: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// marshalCanonical produces deterministic JSON: map keys are sorted, numeric
// fields are rendered without trailing zeros, and no extra whitespace is
// added.
func (s Snapshot) marshalCanonical() ([]byte, error) {
	// Build a stable intermediary with sorted extra metadata keys.
	type wire struct {
		NetworkName     string            `json:"network_name"`
		Passphrase      string            `json:"passphrase"`
		RPCEndpoint     string            `json:"rpc_endpoint"`
		ProtocolVersion uint32            `json:"protocol_version"`
		NodeID          string            `json:"node_id,omitempty"`
		ExtraMetadata   map[string]string `json:"extra_metadata,omitempty"`
	}

	// Deterministic extra metadata: copy into a sorted-key map proxy.
	var sortedExtra map[string]string
	if len(s.ExtraMetadata) > 0 {
		keys := make([]string, 0, len(s.ExtraMetadata))
		for k := range s.ExtraMetadata {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		sortedExtra = make(map[string]string, len(keys))
		for _, k := range keys {
			sortedExtra[k] = s.ExtraMetadata[k]
		}
	}

	w := wire{
		NetworkName:     s.NetworkName,
		Passphrase:      s.Passphrase,
		RPCEndpoint:     s.RPCEndpoint,
		ProtocolVersion: s.ProtocolVersion,
		NodeID:          s.NodeID,
		ExtraMetadata:   sortedExtra,
	}
	return json.Marshal(w)
}

// MarshalJSON implements json.Marshaler using canonical serialization so that
// any JSON encoder produces deterministic output.
func (s Snapshot) MarshalJSON() ([]byte, error) {
	return s.marshalCanonical()
}

// CompatibilityError is returned by Compare when the replay-time snapshot is
// incompatible with the capture-time snapshot.
type CompatibilityError struct {
	Field    string
	Captured string
	Replayed string
}

func (e *CompatibilityError) Error() string {
	return fmt.Sprintf(
		"network snapshot mismatch on field %q: captured=%q replay=%q",
		e.Field, e.Captured, e.Replayed,
	)
}

// CompareOption configures the behaviour of Compare.
type CompareOption func(*compareConfig)

type compareConfig struct {
	allowPassphraseMismatch bool
	allowEndpointMismatch   bool
	allowProtocolMismatch   bool
}

// AllowPassphraseMismatch permits replay against a network with a different
// passphrase.  Using this option is recorded in diagnostics reports so the
// deliberate choice is always visible.
func AllowPassphraseMismatch() CompareOption {
	return func(c *compareConfig) { c.allowPassphraseMismatch = true }
}

// AllowEndpointMismatch permits replay against a different RPC endpoint.
func AllowEndpointMismatch() CompareOption {
	return func(c *compareConfig) { c.allowEndpointMismatch = true }
}

// AllowProtocolMismatch permits replay when the protocol version differs.
func AllowProtocolMismatch() CompareOption {
	return func(c *compareConfig) { c.allowProtocolMismatch = true }
}

// CompareResult contains the outcome of a snapshot comparison including any
// active overrides so that reports can surface intentional cross-network
// analysis.
type CompareResult struct {
	// Compatible is true when all checked fields match (or mismatches were
	// explicitly overridden).
	Compatible bool

	// Errors lists every field mismatch that was NOT overridden.
	Errors []error

	// ActiveOverrides names every CompareOption that was applied and changed
	// the outcome.
	ActiveOverrides []string
}

// Compare checks whether replay is compatible with capture.  By default any
// field mismatch returns an error; supply CompareOption values to allow
// specific deviations.
func Compare(capture, replay Snapshot, opts ...CompareOption) CompareResult {
	cfg := &compareConfig{}
	for _, o := range opts {
		o(cfg)
	}

	var errs []error
	var overrides []string

	check := func(field, cap, rep string, allowed bool, overrideName string) {
		if cap == rep {
			return
		}
		if allowed {
			overrides = append(overrides, overrideName)
			return
		}
		errs = append(errs, &CompatibilityError{Field: field, Captured: cap, Replayed: rep})
	}

	check("passphrase", capture.Passphrase, replay.Passphrase,
		cfg.allowPassphraseMismatch, "AllowPassphraseMismatch")
	check("rpc_endpoint", capture.RPCEndpoint, replay.RPCEndpoint,
		cfg.allowEndpointMismatch, "AllowEndpointMismatch")

	capProto := fmt.Sprintf("%d", capture.ProtocolVersion)
	repProto := fmt.Sprintf("%d", replay.ProtocolVersion)
	check("protocol_version", capProto, repProto,
		cfg.allowProtocolMismatch, "AllowProtocolMismatch")

	return CompareResult{
		Compatible:      len(errs) == 0,
		Errors:          errs,
		ActiveOverrides: overrides,
	}
}

// OverridesDescription returns a human-readable summary of active overrides
// for inclusion in diagnostic reports.  Returns an empty string when no
// overrides are active.
func OverridesDescription(result CompareResult) string {
	if len(result.ActiveOverrides) == 0 {
		return ""
	}
	return "intentional cross-network overrides: " + strings.Join(result.ActiveOverrides, ", ")
}

// ErrIncompatibleNetwork is a sentinel that callers can use with errors.Is to
// detect any network compatibility failure without inspecting field details.
var ErrIncompatibleNetwork = errors.New("incompatible network snapshot")

// FirstError returns the first compatibility error wrapped under
// ErrIncompatibleNetwork, or nil if result.Compatible is true.
func (r CompareResult) FirstError() error {
	if r.Compatible {
		return nil
	}
	if len(r.Errors) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrIncompatibleNetwork, r.Errors[0])
}
