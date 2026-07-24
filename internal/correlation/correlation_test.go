package correlation_test

import (
	"context"
	"sync"
	"testing"

	"github.com/drips/glassbox/internal/correlation"
)

func TestNew_Unique(t *testing.T) {
	const n = 1000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := correlation.New()
		if id.IsZero() {
			t.Fatal("New() returned zero ID")
		}
		if _, dup := seen[id.String()]; dup {
			t.Fatalf("duplicate ID generated: %s", id)
		}
		seen[id.String()] = struct{}{}
	}
}

func TestNew_UniqueUnderConcurrency(t *testing.T) {
	const goroutines = 50
	const perGoroutine = 200

	var mu sync.Mutex
	seen := make(map[string]struct{}, goroutines*perGoroutine)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			local := make([]correlation.ID, perGoroutine)
			for i := 0; i < perGoroutine; i++ {
				local[i] = correlation.New()
			}
			mu.Lock()
			defer mu.Unlock()
			for _, id := range local {
				if _, dup := seen[id.String()]; dup {
					t.Errorf("concurrent duplicate ID: %s", id)
				}
				seen[id.String()] = struct{}{}
			}
		}()
	}
	wg.Wait()
}

func TestWithID_RoundTrip(t *testing.T) {
	id := correlation.New()
	ctx := correlation.WithID(context.Background(), id)

	got, ok := correlation.FromContext(ctx)
	if !ok {
		t.Fatal("FromContext returned ok=false")
	}
	if got != id {
		t.Fatalf("want %s, got %s", id, got)
	}
}

func TestFromContext_Missing(t *testing.T) {
	_, ok := correlation.FromContext(context.Background())
	if ok {
		t.Fatal("expected ok=false for empty context")
	}
}

func TestMustFromContext_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for empty context")
		}
	}()
	correlation.MustFromContext(context.Background())
}

func TestEnsure_CreatesWhenAbsent(t *testing.T) {
	ctx, id := correlation.Ensure(context.Background())
	if id.IsZero() {
		t.Fatal("Ensure returned zero ID")
	}
	got, ok := correlation.FromContext(ctx)
	if !ok || got != id {
		t.Fatalf("Ensure ID not stored in context")
	}
}

func TestEnsure_ReusesExisting(t *testing.T) {
	original := correlation.New()
	ctx := correlation.WithID(context.Background(), original)

	ctx2, id2 := correlation.Ensure(ctx)
	if id2 != original {
		t.Fatalf("Ensure should reuse existing ID, want %s got %s", original, id2)
	}
	_ = ctx2
}

// TestStabilityWithinOperation verifies that the same ID is returned from
// context throughout the lifetime of a simulated operation (propagation
// through retries, child calls, etc.).
func TestStabilityWithinOperation(t *testing.T) {
	id := correlation.New()
	root := correlation.WithID(context.Background(), id)

	// Simulate retry loop: each iteration gets a child context derived from root.
	for attempt := 0; attempt < 5; attempt++ {
		child, cancel := context.WithCancel(root)
		got, ok := correlation.FromContext(child)
		cancel()
		if !ok || got != id {
			t.Fatalf("attempt %d: ID changed or missing", attempt)
		}
	}
}

// TestDistinctAcrossOperations verifies that two independent operations get
// different IDs and that they do not leak into each other's contexts.
func TestDistinctAcrossOperations(t *testing.T) {
	idA := correlation.New()
	idB := correlation.New()

	if idA == idB {
		t.Fatal("two New() calls returned same ID")
	}

	ctxA := correlation.WithID(context.Background(), idA)
	ctxB := correlation.WithID(context.Background(), idB)

	gotA, _ := correlation.FromContext(ctxA)
	gotB, _ := correlation.FromContext(ctxB)

	if gotA == gotB {
		t.Fatal("independent contexts share the same ID")
	}
	if gotA != idA || gotB != idB {
		t.Fatal("context returned wrong ID")
	}
}
