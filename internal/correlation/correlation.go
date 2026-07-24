// Package correlation provides per-operation correlation IDs that are
// propagated through context values, request logs, progress events, metrics,
// and returned diagnostics.  Concurrent operations each carry an independent
// ID so that log lines from simultaneous RPC calls are never mixed.
package correlation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
)

// ctxKey is the unexported context key type so that no external package can
// accidentally overwrite or read the value with an untyped key.
type ctxKey struct{}

// ID is an opaque, comparable correlation identifier.  The zero value is
// invalid; use New() or FromContext().
type ID struct {
	value string
}

// String returns the canonical string representation used in logs and reports.
func (id ID) String() string { return id.value }

// IsZero reports whether id is the invalid zero value.
func (id ID) IsZero() bool { return id.value == "" }

// global counter used to make IDs human-readable in short form (e.g. op-0001)
// while still being globally unique via the UUID component.
var counter atomic.Uint64

// New creates a fresh correlation ID.  Each call is guaranteed to return a
// value distinct from all previously generated IDs within the process lifetime.
func New() ID {
	seq := counter.Add(1)
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is fatal — the OS entropy pool is broken.
		panic(fmt.Sprintf("correlation: crypto/rand failed: %v", err))
	}
	// Format as UUID v4 (RFC 4122 variant bits set).
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	uid := fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:]),
	)
	return ID{value: fmt.Sprintf("op-%04d-%s", seq, uid)}
}

// WithID returns a child context that carries id.  Panics if id.IsZero().
func WithID(ctx context.Context, id ID) context.Context {
	if id.IsZero() {
		panic("correlation: cannot store zero ID in context")
	}
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext extracts the correlation ID stored in ctx.  If no ID has been
// set, it returns the zero ID and ok=false.
func FromContext(ctx context.Context) (ID, bool) {
	id, ok := ctx.Value(ctxKey{}).(ID)
	return id, ok
}

// MustFromContext extracts the correlation ID or panics.  Use in code paths
// where a missing ID is always a programming error.
func MustFromContext(ctx context.Context) ID {
	id, ok := FromContext(ctx)
	if !ok {
		panic("correlation: no ID in context")
	}
	return id
}

// Ensure returns a context that is guaranteed to carry a correlation ID.
// If ctx already has one it is returned unchanged; otherwise a new ID is
// generated, stored, and the new context is returned along with the new ID.
func Ensure(ctx context.Context) (context.Context, ID) {
	if id, ok := FromContext(ctx); ok {
		return ctx, id
	}
	id := New()
	return WithID(ctx, id), id
}
