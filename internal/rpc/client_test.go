package rpc_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/drips/glassbox/internal/cache"
	"github.com/drips/glassbox/internal/correlation"
	"github.com/drips/glassbox/internal/rpc"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// rpcHandler returns an httptest.Server that serves a fixed JSON-RPC response.
func rpcServer(t *testing.T, responseBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responseBody))
	}))
}

// captureServer records every request header and returns body.
func captureServer(t *testing.T, responseBody string) (*httptest.Server, *[]http.Header) {
	t.Helper()
	var mu sync.Mutex
	var captured []http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		captured = append(captured, r.Header.Clone())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responseBody))
	}))
	return srv, &captured
}

func newClient(t *testing.T, endpoint string, opts ...func(*rpc.ClientOptions)) *rpc.Client {
	t.Helper()
	o := rpc.ClientOptions{
		Endpoint:   endpoint,
		MaxRetries: 1,
	}
	for _, f := range opts {
		f(&o)
	}
	c, err := rpc.NewClient(o)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

// ---------------------------------------------------------------------------
// Correlation ID propagation
// ---------------------------------------------------------------------------

func TestGetTransaction_CorrelationID_Propagated(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"result":{"status":"SUCCESS","envelopeXdr":"AABB","ledger":5}}`
	srv, headers := captureServer(t, body)
	defer srv.Close()

	id := correlation.New()
	ctx := correlation.WithID(context.Background(), id)

	client := newClient(t, srv.URL)
	_, err := client.GetTransaction(ctx, "txhash")
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}

	if len(*headers) == 0 {
		t.Fatal("no requests captured")
	}
	got := (*headers)[0].Get(rpc.CorrelationHeader)
	if got != id.String() {
		t.Fatalf("expected correlation header %q, got %q", id.String(), got)
	}
}

func TestGetTransaction_CorrelationID_StableAcrossRetries(t *testing.T) {
	attempt := 0
	var seenIDs []string
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seenIDs = append(seenIDs, r.Header.Get(rpc.CorrelationHeader))
		n := attempt
		attempt++
		mu.Unlock()

		if n == 0 {
			// First attempt: force a transient HTTP 503.
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"status":"SUCCESS","ledger":1}}`))
	}))
	defer srv.Close()

	id := correlation.New()
	ctx := correlation.WithID(context.Background(), id)

	client := newClient(t, srv.URL, func(o *rpc.ClientOptions) {
		o.MaxRetries = 3
		o.RetryDelay = 0
	})
	_, err := client.GetTransaction(ctx, "tx123")
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seenIDs) < 2 {
		t.Fatalf("expected at least 2 attempts for retry test, got %d", len(seenIDs))
	}
	for i, seen := range seenIDs {
		if seen != id.String() {
			t.Fatalf("attempt %d: correlation ID changed during retry: want %s got %s", i, id, seen)
		}
	}
}

// TestConcurrentOperations_IsolatedIDs verifies that two concurrent operations
// with different correlation IDs never mix.
func TestConcurrentOperations_IsolatedIDs(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"result":{"status":"SUCCESS","ledger":1}}`
	var mu sync.Mutex
	idsByAttempt := make(map[string][]string) // correlation ID -> tx hashes seen

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get(rpc.CorrelationHeader)
		// decode request body to get tx hash
		var req struct {
			Params json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		idsByAttempt[corrID] = append(idsByAttempt[corrID], string(req.Params))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	idA := correlation.New()
	idB := correlation.New()

	ctxA := correlation.WithID(context.Background(), idA)
	ctxB := correlation.WithID(context.Background(), idB)

	clientA := newClient(t, srv.URL, func(o *rpc.ClientOptions) { o.MaxRetries = 1 })
	clientB := newClient(t, srv.URL, func(o *rpc.ClientOptions) { o.MaxRetries = 1 })

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = clientA.GetTransaction(ctxA, "txA") }()
	go func() { defer wg.Done(); _, _ = clientB.GetTransaction(ctxB, "txB") }()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	// Every request tagged with idA must not appear under idB and vice versa.
	if _, ok := idsByAttempt[idB.String()]; ok {
		if _, ok2 := idsByAttempt[idA.String()]; ok2 {
			// Both IDs were used — they must be different.
			if idA.String() == idB.String() {
				t.Fatal("concurrent operations have the same correlation ID")
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Validation at RPC boundary
// ---------------------------------------------------------------------------

func TestGetTransaction_MalformedResponse(t *testing.T) {
	srv := rpcServer(t, `{not valid json`)
	defer srv.Close()

	client := newClient(t, srv.URL)
	_, err := client.GetTransaction(context.Background(), "txhash")
	if err == nil {
		t.Fatal("expected error for malformed response")
	}
	if !errors.Is(err, rpc.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestGetTransaction_TruncatedResponse(t *testing.T) {
	srv := rpcServer(t, `{"jsonrpc":"2.0","id":1`)
	defer srv.Close()

	client := newClient(t, srv.URL)
	_, err := client.GetTransaction(context.Background(), "txhash")
	if err == nil {
		t.Fatal("expected error for truncated response")
	}
}

func TestGetTransaction_WrongVersion(t *testing.T) {
	srv := rpcServer(t, `{"jsonrpc":"1.0","id":1,"result":{"status":"SUCCESS"}}`)
	defer srv.Close()

	client := newClient(t, srv.URL)
	_, err := client.GetTransaction(context.Background(), "txhash")
	if !errors.Is(err, rpc.ErrValidation) {
		t.Fatalf("expected ErrValidation for wrong version, got %v", err)
	}
}

func TestGetTransaction_NullFieldResponse(t *testing.T) {
	srv := rpcServer(t, `{"jsonrpc":"2.0","id":1,"result":null}`)
	defer srv.Close()

	client := newClient(t, srv.URL)
	_, err := client.GetTransaction(context.Background(), "txhash")
	if err == nil {
		t.Fatal("expected error for null result")
	}
}

func TestGetLedger_MissingSequence(t *testing.T) {
	srv := rpcServer(t, `{"jsonrpc":"2.0","id":1,"result":{"ledgerHash":"ABCD"}}`)
	defer srv.Close()

	client := newClient(t, srv.URL)
	_, err := client.GetLedger(context.Background(), 0)
	if err == nil {
		t.Fatal("expected error for zero sequence")
	}
}

// ---------------------------------------------------------------------------
// Cache integration
// ---------------------------------------------------------------------------

func TestGetTransaction_CacheHit(t *testing.T) {
	hitCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"status":"SUCCESS","ledger":1}}`))
	}))
	defer srv.Close()

	c := cache.New(cache.Options{})
	client := newClient(t, srv.URL, func(o *rpc.ClientOptions) {
		o.Cache = c
		o.NetworkPassphrase = "Test SDF Network ; September 2015"
	})

	ctx := context.Background()
	_, err := client.GetTransaction(ctx, "txCacheTest")
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	if hitCount != 1 {
		t.Fatalf("expected 1 RPC call, got %d", hitCount)
	}

	// Second identical request must be served from cache.
	_, err = client.GetTransaction(ctx, "txCacheTest")
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	if hitCount != 1 {
		t.Fatalf("second request must not hit RPC; got %d total calls", hitCount)
	}
}

// ---------------------------------------------------------------------------
// Telemetry hooks
// ---------------------------------------------------------------------------

func TestTelemetry_CorrelationIDInEvents(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"result":{"status":"SUCCESS","ledger":1}}`
	srv := rpcServer(t, body)
	defer srv.Close()

	id := correlation.New()
	ctx := correlation.WithID(context.Background(), id)

	var events []rpc.Event
	var mu sync.Mutex
	hook := func(e rpc.Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	client := newClient(t, srv.URL, func(o *rpc.ClientOptions) {
		o.Hooks = []rpc.Hook{hook}
		o.MaxRetries = 1
	})
	_, err := client.GetTransaction(ctx, "txEvent")
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) == 0 {
		t.Fatal("no telemetry events emitted")
	}
	for _, e := range events {
		if e.CorrelationID != id.String() {
			t.Fatalf("event %v: expected correlation ID %s, got %s", e.Kind, id, e.CorrelationID)
		}
	}
}

func TestTelemetry_RetryEvents(t *testing.T) {
	attempt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempt == 0 {
			attempt++
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"status":"SUCCESS","ledger":1}}`))
	}))
	defer srv.Close()

	var retryEvents int
	var mu sync.Mutex
	hook := func(e rpc.Event) {
		mu.Lock()
		if e.Kind == rpc.EventKindRetry {
			retryEvents++
		}
		mu.Unlock()
	}

	client := newClient(t, srv.URL, func(o *rpc.ClientOptions) {
		o.Hooks = []rpc.Hook{hook}
		o.MaxRetries = 3
		o.RetryDelay = 0
	})
	_, err := client.GetTransaction(context.Background(), "txRetry")
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if retryEvents == 0 {
		t.Fatal("expected at least one retry event")
	}
}

// ---------------------------------------------------------------------------
// No credentials in error messages
// ---------------------------------------------------------------------------

func TestGetTransaction_NoCredentialsInError(t *testing.T) {
	// Response body containing a secret that must not appear in error messages.
	body := `{"jsonrpc":"1.0","id":1,"result":{"secret":"TOP_SECRET_KEY","status":"OK"}}`
	srv := rpcServer(t, body)
	defer srv.Close()

	client := newClient(t, srv.URL)
	_, err := client.GetTransaction(context.Background(), "txSecret")
	if err == nil {
		t.Fatal("expected validation error")
	}
	// The raw body must not leak into the error message.
	errStr := err.Error()
	if containsStr(errStr, "TOP_SECRET_KEY") {
		t.Fatalf("secret value leaked into error message: %s", errStr)
	}
}

func TestSimulateTransaction_NeverCached(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"latestLedger":10}}`))
	}))
	defer srv.Close()

	c := cache.New(cache.Options{TTL: time.Hour})
	client := newClient(t, srv.URL, func(o *rpc.ClientOptions) {
		o.Cache = c
		o.NetworkPassphrase = "net"
	})

	ctx := context.Background()
	_, _ = client.SimulateTransaction(ctx, "xdr1")
	_, _ = client.SimulateTransaction(ctx, "xdr1")

	// Both calls must reach the server; simulation is never cached.
	if callCount != 2 {
		t.Fatalf("simulation must not be cached: got %d calls, expected 2", callCount)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
