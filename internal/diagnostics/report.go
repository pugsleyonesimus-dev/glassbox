// Package diagnostics assembles human-readable and JSON debug reports from
// the collected state of a glassbox operation.  Reports include the
// correlation ID, network snapshot, cache statistics, and any validation
// errors, but never expose query secrets or raw response bodies.
package diagnostics

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/drips/glassbox/internal/cache"
	"github.com/drips/glassbox/internal/correlation"
	"github.com/drips/glassbox/internal/network"
)

// Report aggregates all diagnostic information for one debug operation.
type Report struct {
	// CorrelationID is the operation-scoped ID that links all RPC requests in
	// this operation.
	CorrelationID string `json:"correlation_id"`

	// Timestamp is when the report was generated.
	Timestamp time.Time `json:"timestamp"`

	// Network is the snapshot of the network configuration in use.
	Network network.Snapshot `json:"network"`

	// NetworkOverrides lists any active cross-network override options that
	// were applied, making intentional cross-network analysis visible.
	NetworkOverrides []string `json:"network_overrides,omitempty"`

	// CacheStats is a snapshot of cache activity during this operation.
	CacheStats *cache.DiagnosticReport `json:"cache_stats,omitempty"`

	// ValidationErrors holds any RPC validation failures encountered.
	ValidationErrors []ValidationEntry `json:"validation_errors,omitempty"`

	// Notes holds any free-form annotated observations added during the
	// operation.
	Notes []string `json:"notes,omitempty"`
}

// ValidationEntry records a single RPC validation failure without including
// the response body.
type ValidationEntry struct {
	Endpoint string `json:"endpoint"`
	Method   string `json:"method"`
	Field    string `json:"field"`
	Reason   string `json:"reason"`
}

// Builder constructs a Report incrementally during an operation.
type Builder struct {
	report Report
}

// NewBuilder creates a Builder for the operation identified by id.
func NewBuilder(id correlation.ID, net network.Snapshot) *Builder {
	return &Builder{
		report: Report{
			CorrelationID: id.String(),
			Timestamp:     time.Now().UTC(),
			Network:       net,
		},
	}
}

// SetNetworkOverrides records the active cross-network override options for
// inclusion in the report.
func (b *Builder) SetNetworkOverrides(overrides []string) {
	b.report.NetworkOverrides = overrides
}

// SetCacheStats attaches a cache diagnostic report.
func (b *Builder) SetCacheStats(stats cache.DiagnosticReport) {
	b.report.CacheStats = &stats
}

// AddValidationError records a validation failure.  The raw response body
// must never be passed here; only field metadata and reason strings are
// accepted.
func (b *Builder) AddValidationError(endpoint, method, field, reason string) {
	b.report.ValidationErrors = append(b.report.ValidationErrors, ValidationEntry{
		Endpoint: endpoint,
		Method:   method,
		Field:    field,
		Reason:   reason,
	})
}

// AddNote appends a free-form diagnostic note.
func (b *Builder) AddNote(note string) {
	b.report.Notes = append(b.report.Notes, note)
}

// Build returns the assembled Report.
func (b *Builder) Build() Report {
	r := b.report
	return r
}

// ---------------------------------------------------------------------------
// Serialization
// ---------------------------------------------------------------------------

// WriteJSON serializes r to w as indented JSON.
func WriteJSON(w io.Writer, r Report) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return fmt.Errorf("diagnostics.WriteJSON: %w", err)
	}
	return nil
}

// WriteText writes a human-readable summary of r to w.
func WriteText(w io.Writer, r Report) error {
	lines := []string{
		fmt.Sprintf("=== Glassbox Diagnostic Report ==="),
		fmt.Sprintf("Correlation ID : %s", r.CorrelationID),
		fmt.Sprintf("Timestamp      : %s", r.Timestamp.Format(time.RFC3339)),
		fmt.Sprintf("Network        : %s (passphrase: %s)", r.Network.NetworkName, r.Network.Passphrase),
		fmt.Sprintf("RPC Endpoint   : %s", r.Network.RPCEndpoint),
		fmt.Sprintf("Protocol       : %d", r.Network.ProtocolVersion),
	}

	if len(r.NetworkOverrides) > 0 {
		lines = append(lines, fmt.Sprintf("⚠  Network Overrides: %s",
			strings.Join(r.NetworkOverrides, ", ")))
	}

	if r.CacheStats != nil {
		cs := r.CacheStats
		lines = append(lines,
			fmt.Sprintf("Cache          : entries=%d hits=%d misses=%d corrupted=%d evictions=%d",
				cs.TotalEntries, cs.Hits, cs.Misses, cs.Corrupted, cs.Evictions))
	}

	if len(r.ValidationErrors) > 0 {
		lines = append(lines, "Validation Errors:")
		for _, ve := range r.ValidationErrors {
			lines = append(lines, fmt.Sprintf("  [%s %s] field=%q reason=%s",
				ve.Endpoint, ve.Method, ve.Field, ve.Reason))
		}
	}

	if len(r.Notes) > 0 {
		lines = append(lines, "Notes:")
		for _, n := range r.Notes {
			lines = append(lines, "  "+n)
		}
	}

	_, err := fmt.Fprintln(w, strings.Join(lines, "\n"))
	return err
}

// MarshalJSON returns the JSON encoding of r.
func MarshalJSON(r Report) ([]byte, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("diagnostics.MarshalJSON: %w", err)
	}
	return b, nil
}
