package callonce_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	callonce "github.com/probablyarth/callonce-go"
)

var testKey = callonce.NewKey[string]("test")

func TestGetWithoutCache(t *testing.T) {
	ctx := context.Background()
	val, err := callonce.Get(ctx, func() (string, error) {
		return "direct", nil
	}, callonce.L(testKey, "1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "direct" {
		t.Fatalf("got %q, want %q", val, "direct")
	}
}

func TestGetCachesResult(t *testing.T) {
	ctx := callonce.WithCache(context.Background())
	var calls atomic.Int32

	fn := func() (string, error) {
		calls.Add(1)
		return "cached", nil
	}

	v1, err := callonce.Get(ctx, fn, callonce.L(testKey, "1"))
	if err != nil {
		t.Fatal(err)
	}
	v2, err := callonce.Get(ctx, fn, callonce.L(testKey, "1"))
	if err != nil {
		t.Fatal(err)
	}

	if v1 != "cached" || v2 != "cached" {
		t.Fatalf("got %q, %q; want %q", v1, v2, "cached")
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("fn called %d times, want 1", n)
	}
}

func TestGetConcurrentDedup(t *testing.T) {
	ctx := callonce.WithCache(context.Background())
	var calls atomic.Int32

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)

	results := make([]string, n)
	errs := make([]error, n)

	for i := range n {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = callonce.Get(ctx, func() (string, error) {
				calls.Add(1)
				return "deduped", nil
			}, callonce.L(testKey, "1"))
		}(i)
	}
	wg.Wait()

	for i := range n {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: unexpected error: %v", i, errs[i])
		}
		if results[i] != "deduped" {
			t.Fatalf("goroutine %d: got %q, want %q", i, results[i], "deduped")
		}
	}
	if c := calls.Load(); c != 1 {
		t.Fatalf("fn called %d times, want 1", c)
	}
}

func TestGetErrorNotCached(t *testing.T) {
	ctx := callonce.WithCache(context.Background())
	var calls atomic.Int32
	errBoom := errors.New("boom")

	// First call: error.
	_, err := callonce.Get(ctx, func() (string, error) {
		calls.Add(1)
		return "", errBoom
	}, callonce.L(testKey, "1"))
	if !errors.Is(err, errBoom) {
		t.Fatalf("got err=%v, want %v", err, errBoom)
	}

	// Second call: success, fn must be invoked again.
	val, err := callonce.Get(ctx, func() (string, error) {
		calls.Add(1)
		return "ok", nil
	}, callonce.L(testKey, "1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "ok" {
		t.Fatalf("got %q, want %q", val, "ok")
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("fn called %d times, want 2", n)
	}
}

func TestGetPanicPropagates(t *testing.T) {
	ctx := callonce.WithCache(context.Background())

	// First call panics.
	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic, got none")
			}
			// singleflight wraps panics; check the string representation.
			if s := fmt.Sprint(r); !strings.Contains(s, "kaboom") {
				t.Fatalf("got panic %v, want it to contain %q", r, "kaboom")
			}
		}()
		callonce.Get(ctx, func() (string, error) {
			panic("kaboom")
		}, callonce.L(testKey, "1"))
	}()

	// Cache should not be poisoned. A subsequent call with the same key succeeds.
	val, err := callonce.Get(ctx, func() (string, error) {
		return "recovered", nil
	}, callonce.L(testKey, "1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "recovered" {
		t.Fatalf("got %q, want %q", val, "recovered")
	}
}

func TestGetNilValueCached(t *testing.T) {
	ctx := callonce.WithCache(context.Background())
	var calls atomic.Int32

	type S struct{ Name string }
	nilKey := callonce.NewKey[*S]("niltest")

	fn := func() (*S, error) {
		calls.Add(1)
		return nil, nil
	}

	v1, err := callonce.Get(ctx, fn, callonce.L(nilKey, "1"))
	if err != nil {
		t.Fatal(err)
	}
	v2, err := callonce.Get(ctx, fn, callonce.L(nilKey, "1"))
	if err != nil {
		t.Fatal(err)
	}

	if v1 != nil || v2 != nil {
		t.Fatalf("got %v, %v; want nil, nil", v1, v2)
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("fn called %d times, want 1", n)
	}
}

func TestGetDifferentKeys(t *testing.T) {
	ctx := callonce.WithCache(context.Background())
	var callsA, callsB atomic.Int32

	key := callonce.NewKey[string]("item")

	va, err := callonce.Get(ctx, func() (string, error) {
		callsA.Add(1)
		return "alpha", nil
	}, callonce.L(key, "a"))
	if err != nil {
		t.Fatal(err)
	}

	vb, err := callonce.Get(ctx, func() (string, error) {
		callsB.Add(1)
		return "beta", nil
	}, callonce.L(key, "b"))
	if err != nil {
		t.Fatal(err)
	}

	if va != "alpha" || vb != "beta" {
		t.Fatalf("got %q, %q; want alpha, beta", va, vb)
	}
	if callsA.Load() != 1 || callsB.Load() != 1 {
		t.Fatal("each key's fn should be called exactly once")
	}
}

func TestWithCacheFromContext(t *testing.T) {
	// Bare context has no cache.
	if c := callonce.FromContext(context.Background()); c != nil {
		t.Fatalf("expected nil, got %v", c)
	}

	ctx := callonce.WithCache(context.Background())
	c := callonce.FromContext(ctx)
	if c == nil {
		t.Fatal("expected non-nil cache from context")
	}
}

func TestGetDifferentTypes(t *testing.T) {
	ctx := callonce.WithCache(context.Background())

	strKey := callonce.NewKey[string]("val")
	intKey := callonce.NewKey[int]("val")

	vs, err := callonce.Get(ctx, func() (string, error) {
		return "hello", nil
	}, callonce.L(strKey, "1"))
	if err != nil {
		t.Fatal(err)
	}

	vi, err := callonce.Get(ctx, func() (int, error) {
		return 42, nil
	}, callonce.L(intKey, "1"))
	if err != nil {
		t.Fatal(err)
	}

	if vs != "hello" {
		t.Fatalf("got %q, want %q", vs, "hello")
	}
	if vi != 42 {
		t.Fatalf("got %d, want %d", vi, 42)
	}
}

// ---------------------------------------------------------------------------
// OR semantics: multiple lookups per Get call.
// ---------------------------------------------------------------------------

func TestGetORHitOnSecondKey(t *testing.T) {
	ctx := callonce.WithCache(context.Background())
	var calls atomic.Int32

	slugKey := callonce.NewKey[string]("by-slug")
	idKey := callonce.NewKey[string]("by-id")

	// Populate cache via slug only.
	_, err := callonce.Get(ctx, func() (string, error) {
		calls.Add(1)
		return "resource-A", nil
	}, callonce.L(slugKey, "my-slug"))
	if err != nil {
		t.Fatal(err)
	}

	// Now query with both slug and id. Should hit on slug, fn not called again.
	val, err := callonce.Get(ctx, func() (string, error) {
		calls.Add(1)
		return "should-not-run", nil
	}, callonce.L(idKey, "123"), callonce.L(slugKey, "my-slug"))
	if err != nil {
		t.Fatal(err)
	}
	if val != "resource-A" {
		t.Fatalf("got %q, want %q", val, "resource-A")
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("fn called %d times, want 1", n)
	}
}

func TestGetORBackfillsAllKeys(t *testing.T) {
	ctx := callonce.WithCache(context.Background())
	var calls atomic.Int32

	slugKey := callonce.NewKey[string]("by-slug")
	idKey := callonce.NewKey[string]("by-id")

	// Call with both lookups. fn runs once, result cached under both.
	val, err := callonce.Get(ctx, func() (string, error) {
		calls.Add(1)
		return "resource-B", nil
	}, callonce.L(slugKey, "slug-b"), callonce.L(idKey, "456"))
	if err != nil {
		t.Fatal(err)
	}
	if val != "resource-B" {
		t.Fatalf("got %q, want %q", val, "resource-B")
	}

	// Now query with only the id key. Should hit from the backfill.
	val2, err := callonce.Get(ctx, func() (string, error) {
		calls.Add(1)
		return "should-not-run", nil
	}, callonce.L(idKey, "456"))
	if err != nil {
		t.Fatal(err)
	}
	if val2 != "resource-B" {
		t.Fatalf("got %q, want %q", val2, "resource-B")
	}

	// And with only the slug key.
	val3, err := callonce.Get(ctx, func() (string, error) {
		calls.Add(1)
		return "should-not-run", nil
	}, callonce.L(slugKey, "slug-b"))
	if err != nil {
		t.Fatal(err)
	}
	if val3 != "resource-B" {
		t.Fatalf("got %q, want %q", val3, "resource-B")
	}

	if n := calls.Load(); n != 1 {
		t.Fatalf("fn called %d times, want 1", n)
	}
}

func TestGetORBackfillsOnPartialHit(t *testing.T) {
	ctx := callonce.WithCache(context.Background())
	var calls atomic.Int32

	slugKey := callonce.NewKey[string]("by-slug")
	idKey := callonce.NewKey[string]("by-id")

	// Populate via slug only.
	_, err := callonce.Get(ctx, func() (string, error) {
		calls.Add(1)
		return "resource-C", nil
	}, callonce.L(slugKey, "slug-c"))
	if err != nil {
		t.Fatal(err)
	}

	// OR lookup with slug + id. Hits on slug, should backfill id.
	_, err = callonce.Get(ctx, func() (string, error) {
		calls.Add(1)
		return "should-not-run", nil
	}, callonce.L(slugKey, "slug-c"), callonce.L(idKey, "789"))
	if err != nil {
		t.Fatal(err)
	}

	// Now query with only id. Should hit from the backfill.
	val, err := callonce.Get(ctx, func() (string, error) {
		calls.Add(1)
		return "should-not-run", nil
	}, callonce.L(idKey, "789"))
	if err != nil {
		t.Fatal(err)
	}
	if val != "resource-C" {
		t.Fatalf("got %q, want %q", val, "resource-C")
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("fn called %d times, want 1", n)
	}
}
