package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/drips/glassbox/internal/cache"
	"github.com/drips/glassbox/internal/correlation"
)

// CorrelationHeader is the HTTP request header used to propagate the
// operation correlation ID to the remote RPC node.
const CorrelationHeader = "X-Glassbox-Correlation-ID"

// DefaultMaxRetries is the default number of retry attempts after a
// transient failure.
const DefaultMaxRetries = 3

// ClientOptions configures the RPC client.
type ClientOptions struct {
	// Endpoint is the base URL of the Soroban RPC node (required).
	Endpoint string

	// HTTPClient is the underlying transport.  If nil, http.DefaultClient is
	// used.
	HTTPClient *http.Client

	// MaxRetries is the maximum number of attempts per request.  Zero falls
	// back to DefaultMaxRetries.
	MaxRetries int

	// RetryDelay is the fixed delay between attempts.  Zero means no delay.
	RetryDelay time.Duration

	// Cache is an optional content-addressed cache.  If non-nil it is
	// consulted before issuing live requests for cacheable methods.
	Cache *cache.Cache

	// NetworkPassphrase is required when Cache is non-nil so that cache keys
	// are network-scoped.
	NetworkPassphrase string

	// Hooks receive telemetry events for every request and retry.
	Hooks []Hook
}

// Client is a JSON-RPC 2.0 client for Stellar Soroban RPC.
type Client struct {
	endpoint          string
	http              *http.Client
	maxRetries        int
	retryDelay        time.Duration
	cache             *cache.Cache
	networkPassphrase string
	hooks             []Hook
}

// NewClient creates a Client from opts.
func NewClient(opts ClientOptions) (*Client, error) {
	if opts.Endpoint == "" {
		return nil, errors.New("rpc.NewClient: endpoint is required")
	}
	h := opts.HTTPClient
	if h == nil {
		h = http.DefaultClient
	}
	maxRetries := opts.MaxRetries
	if maxRetries <= 0 {
		maxRetries = DefaultMaxRetries
	}
	return &Client{
		endpoint:          opts.Endpoint,
		http:              h,
		maxRetries:        maxRetries,
		retryDelay:        opts.RetryDelay,
		cache:             opts.Cache,
		networkPassphrase: opts.NetworkPassphrase,
		hooks:             opts.Hooks,
	}, nil
}

// ---------------------------------------------------------------------------
// Public API — high-level methods
// ---------------------------------------------------------------------------

// GetTransaction fetches and validates a transaction by hash.
func (c *Client) GetTransaction(ctx context.Context, txHash string) (*TransactionResponse, error) {
	ctx, _ = correlation.Ensure(ctx)

	params, err := json.Marshal([]string{txHash})
	if err != nil {
		return nil, fmt.Errorf("GetTransaction: marshal params: %w", err)
	}

	// Try cache first.
	cacheKey := cache.Key{
		Network: c.networkPassphrase,
		Kind:    cache.KindEnvelope,
		ID:      txHash,
	}
	if raw, cacheErr := c.cacheGet(ctx, cacheKey); raw != nil {
		resp, err := validateResponse(raw, c.endpoint, "getTransaction")
		if err != nil {
			return nil, err
		}
		return validateTransactionResponse(resp.Result, c.endpoint, "getTransaction")
	} else if cacheErr != nil && !errors.Is(cacheErr, cache.ErrEvicted) {
		// ErrCorrupted: fall through to live request.
		_ = cacheErr
	}

	raw, err := c.do(ctx, "getTransaction", params)
	if err != nil {
		return nil, err
	}

	resp, err := validateResponse(raw, c.endpoint, "getTransaction")
	if err != nil {
		return nil, err
	}

	txResp, err := validateTransactionResponse(resp.Result, c.endpoint, "getTransaction")
	if err != nil {
		return nil, err
	}

	// Cache the validated raw response.
	c.cachePut(cacheKey, raw)
	return txResp, nil
}

// GetLedger fetches and validates ledger data by sequence number.
func (c *Client) GetLedger(ctx context.Context, sequence uint32) (*LedgerResponse, error) {
	ctx, _ = correlation.Ensure(ctx)

	params, err := json.Marshal(map[string]uint32{"sequence": sequence})
	if err != nil {
		return nil, fmt.Errorf("GetLedger: marshal params: %w", err)
	}

	cacheKey := cache.Key{
		Network: c.networkPassphrase,
		Kind:    cache.KindLedger,
		ID:      fmt.Sprintf("%d", sequence),
	}
	if raw, cacheErr := c.cacheGet(ctx, cacheKey); raw != nil {
		resp, err := validateResponse(raw, c.endpoint, "getLedger")
		if err != nil {
			return nil, err
		}
		return validateLedgerResponse(resp.Result, c.endpoint, "getLedger")
	} else if cacheErr != nil && !errors.Is(cacheErr, cache.ErrEvicted) {
		_ = cacheErr
	}

	raw, err := c.do(ctx, "getLedger", params)
	if err != nil {
		return nil, err
	}

	resp, err := validateResponse(raw, c.endpoint, "getLedger")
	if err != nil {
		return nil, err
	}

	ledger, err := validateLedgerResponse(resp.Result, c.endpoint, "getLedger")
	if err != nil {
		return nil, err
	}

	c.cachePut(cacheKey, raw)
	return ledger, nil
}

// GetFootprint fetches and validates ledger entry data (footprint).
func (c *Client) GetFootprint(ctx context.Context, keys []string) (*FootprintResponse, error) {
	ctx, _ = correlation.Ensure(ctx)

	params, err := json.Marshal(map[string][]string{"keys": keys})
	if err != nil {
		return nil, fmt.Errorf("GetFootprint: marshal params: %w", err)
	}

	// Footprint key is derived from the sorted keys hash — not cached here for
	// simplicity; caching is handled at the GetTransaction layer.
	raw, err := c.do(ctx, "getLedgerEntries", params)
	if err != nil {
		return nil, err
	}

	resp, err := validateResponse(raw, c.endpoint, "getLedgerEntries")
	if err != nil {
		return nil, err
	}

	return validateFootprintResponse(resp.Result, c.endpoint, "getLedgerEntries")
}

// SimulateTransaction sends a simulation request.  Results are explicitly NOT
// cached because simulation is mutable and credential-bearing.
func (c *Client) SimulateTransaction(ctx context.Context, envelopeXDR string) (*SimulationResponse, error) {
	ctx, _ = correlation.Ensure(ctx)

	params, err := json.Marshal(map[string]string{"transaction": envelopeXDR})
	if err != nil {
		return nil, fmt.Errorf("SimulateTransaction: marshal params: %w", err)
	}

	// Never cache simulation responses.
	raw, err := c.do(ctx, "simulateTransaction", params)
	if err != nil {
		return nil, err
	}

	resp, err := validateResponse(raw, c.endpoint, "simulateTransaction")
	if err != nil {
		return nil, err
	}

	return validateSimulationResponse(resp.Result, c.endpoint, "simulateTransaction")
}

// ---------------------------------------------------------------------------
// Core HTTP dispatch
// ---------------------------------------------------------------------------

// do sends a JSON-RPC request and returns the raw response body.  It retries
// on network errors up to maxRetries times, attaching the correlation ID to
// every attempt.  The response body is validated at the envelope level before
// being returned.
func (c *Client) do(ctx context.Context, method string, params json.RawMessage) ([]byte, error) {
	corrID, _ := correlation.FromContext(ctx)

	var lastErr error
	for attempt := 1; attempt <= c.maxRetries; attempt++ {
		if attempt > 1 {
			emit(ctx, c.hooks, Event{
				Kind:     EventKindRetry,
				Endpoint: c.endpoint,
				Method:   method,
				Attempt:  attempt,
			})
			if c.retryDelay > 0 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(c.retryDelay):
				}
			}
		}

		start := time.Now()
		raw, err := c.sendOnce(ctx, method, params, corrID.String())
		elapsed := time.Since(start)

		if err != nil {
			emit(ctx, c.hooks, Event{
				Kind:     EventKindRequestError,
				Endpoint: c.endpoint,
				Method:   method,
				Attempt:  attempt,
				Duration: elapsed,
				Error:    err.Error(),
			})
			lastErr = err
			// Only retry on network/transport errors; validation errors are
			// terminal.
			var ve *ValidationError
			if errors.As(err, &ve) {
				return nil, err
			}
			continue
		}

		emit(ctx, c.hooks, Event{
			Kind:     EventKindRequestComplete,
			Endpoint: c.endpoint,
			Method:   method,
			Attempt:  attempt,
			Duration: elapsed,
		})
		return raw, nil
	}

	return nil, fmt.Errorf("rpc %s after %d attempts: %w", method, c.maxRetries, lastErr)
}

// sendOnce performs a single HTTP POST and returns the raw response body
// without any validation beyond HTTP status.
func (c *Client) sendOnce(ctx context.Context, method string, params json.RawMessage, corrID string) ([]byte, error) {
	req := Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	emit(ctx, c.hooks, Event{
		Kind:          EventKindRequestStart,
		Endpoint:      c.endpoint,
		Method:        method,
		CorrelationID: corrID,
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if corrID != "" {
		httpReq.Header.Set(CorrelationHeader, corrID)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http status %d for method %s", resp.StatusCode, method)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return raw, nil
}

// ---------------------------------------------------------------------------
// Cache helpers
// ---------------------------------------------------------------------------

func (c *Client) cacheGet(ctx context.Context, key cache.Key) ([]byte, error) {
	if c.cache == nil {
		return nil, nil
	}
	data, err := c.cache.Get(key)
	if err != nil {
		emit(ctx, c.hooks, Event{Kind: EventKindCacheMiss, Endpoint: c.endpoint})
		return nil, err
	}
	if data == nil {
		emit(ctx, c.hooks, Event{Kind: EventKindCacheMiss, Endpoint: c.endpoint})
		return nil, nil
	}
	emit(ctx, c.hooks, Event{Kind: EventKindCacheHit, Endpoint: c.endpoint})
	return data, nil
}

func (c *Client) cachePut(key cache.Key, data []byte) {
	if c.cache != nil {
		c.cache.Put(key, data)
	}
}
