package rpc

import (
	"context"
	"time"

	"github.com/drips/glassbox/internal/correlation"
)

// EventKind classifies a telemetry event.
type EventKind string

const (
	EventKindRequestStart    EventKind = "request_start"
	EventKindRequestComplete EventKind = "request_complete"
	EventKindRequestError    EventKind = "request_error"
	EventKindCacheHit        EventKind = "cache_hit"
	EventKindCacheMiss       EventKind = "cache_miss"
	EventKindRetry           EventKind = "retry"
)

// Event is a single telemetry observation.  The CorrelationID is always set
// from the request context so that concurrent operation logs can be separated.
type Event struct {
	Kind          EventKind
	CorrelationID string
	Endpoint      string
	Method        string
	Attempt       int
	Duration      time.Duration
	// Error is the error message string — never the full response body.
	Error string
}

// Hook is a function called for every telemetry event.  Implementations must
// be non-blocking and must not store references to the Event after returning.
type Hook func(Event)

// noopHook discards all events.
func noopHook(Event) {}

// emit fires an event through all registered hooks, attaching the correlation
// ID from ctx if one is present.
func emit(ctx context.Context, hooks []Hook, e Event) {
	if id, ok := correlation.FromContext(ctx); ok {
		e.CorrelationID = id.String()
	}
	for _, h := range hooks {
		h(e)
	}
}
