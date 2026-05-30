// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package protocolreg

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var txHashPattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

// allowedNetworks is the set of valid network identifiers for the deep link.
var allowedNetworks = map[string]bool{
	"testnet":   true,
	"mainnet":   true,
	"futurenet": true,
}

// allowedViews is the set of valid view mode identifiers for the deep link.
var allowedViews = map[string]bool{
	"trace":      true,
	"flamegraph": true,
	"events":     true,
	"auth":       true,
	"budget":     true,
	"storage":    true,
}

// ParsedDebugURI holds the validated fields extracted from a glassbox:// debug URI.
//
// Supported URI format:
//
//	glassbox://debug/<txhash>?network=<n>[&op=<i>][&operation=<i>][&view=<v>][&source=<s>][&signature=<s>]
//
// Query parameters:
//   - network  (required) — one of: testnet, mainnet, futurenet
//   - op       (optional) — zero-based operation index (alias for "operation")
//   - operation (optional) — zero-based operation index (legacy; "op" takes precedence)
//   - view     (optional) — initial view mode: trace, flamegraph, events, auth, budget, storage
//   - source   (optional) — free-form source identifier (e.g. "dashboard")
//   - signature (optional) — free-form signature hint
type ParsedDebugURI struct {
	// Raw is the original unmodified URI string.
	Raw string
	// TransactionHash is the 64-character lowercase hex transaction hash.
	TransactionHash string
	// Network is the validated network identifier (testnet, mainnet, futurenet).
	Network string
	// Op is the zero-based operation index, populated from the "op" or "operation" query parameter.
	// nil means no operation was specified.
	Op *int
	// Operation is an alias for Op retained for backward compatibility.
	// It always mirrors Op.
	Operation *int
	// View is the requested initial view mode (trace, flamegraph, events, auth, budget, storage).
	// Empty string means no view was specified and the default view should be used.
	View string
	// Source is an optional free-form source identifier.
	Source string
	// Signature is an optional free-form signature hint.
	Signature string
}

// ParseDebugURI parses and validates a glassbox:// debug URI.
//
// Returns a descriptive error for each class of invalid input:
//   - empty URI
//   - wrong scheme
//   - wrong host (not "debug")
//   - missing or malformed transaction hash
//   - missing or invalid network
//   - invalid op/operation index (non-numeric or negative)
//   - unrecognised view mode
func ParseDebugURI(raw string) (*ParsedDebugURI, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("protocol URI must not be empty")
	}
	if !strings.HasPrefix(raw, Scheme+"://") {
		return nil, fmt.Errorf("invalid protocol URI: expected %s://", Scheme)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse protocol URI: %w", err)
	}

	if parsed.Host != "debug" {
		return nil, fmt.Errorf("invalid protocol host %q: expected \"debug\"", parsed.Host)
	}

	transactionHash := strings.TrimPrefix(parsed.EscapedPath(), "/")
	transactionHash, err = url.PathUnescape(transactionHash)
	if err != nil {
		return nil, fmt.Errorf("decode transaction hash: %w", err)
	}
	if !txHashPattern.MatchString(transactionHash) {
		return nil, fmt.Errorf("invalid transaction hash %q: must be a 64-character hex string", transactionHash)
	}

	q := parsed.Query()

	// --- network (required) ---
	network := q.Get("network")
	if network == "" {
		return nil, fmt.Errorf("missing required query parameter: network")
	}
	if !allowedNetworks[network] {
		return nil, fmt.Errorf("invalid network %q: must be one of testnet, mainnet, futurenet", network)
	}

	result := &ParsedDebugURI{
		Raw:             raw,
		TransactionHash: transactionHash,
		Network:         network,
		Source:          q.Get("source"),
		Signature:       q.Get("signature"),
	}

	// --- op / operation (optional, "op" takes precedence) ---
	opStr := q.Get("op")
	if opStr == "" {
		opStr = q.Get("operation")
	}
	if opStr != "" {
		parsedOp, err := strconv.Atoi(opStr)
		if err != nil || parsedOp < 0 {
			return nil, fmt.Errorf("invalid operation index %q: must be a non-negative integer", opStr)
		}
		result.Op = &parsedOp
		result.Operation = &parsedOp
	}

	// --- view (optional) ---
	if view := q.Get("view"); view != "" {
		if !allowedViews[view] {
			return nil, fmt.Errorf("invalid view %q: must be one of trace, flamegraph, events, auth, budget, storage", view)
		}
		result.View = view
	}

	return result, nil
}
