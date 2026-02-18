package callonce_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	callonce "github.com/probablyarth/callonce-go"
	"golang.org/x/sync/singleflight"
)

var benchKey = callonce.NewKey[string]("bench")

// ---------------------------------------------------------------------------
// Single-goroutine benchmarks: measure per-call latency.
// ---------------------------------------------------------------------------

// How fast is a cache hit (RLock + map lookup)?
func BenchmarkCacheHit(b *testing.B) {
	ctx := callonce.WithCache(context.Background())
	callonce.Get(ctx, func() (string, error) { return "v", nil }, callonce.L(benchKey, "1"))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		callonce.Get(ctx, func() (string, error) { return "v", nil }, callonce.L(benchKey, "1"))
	}
}

// How fast is a cache miss (singleflight + write)?
func BenchmarkCacheMiss(b *testing.B) {
	ids := make([]string, b.N)
	for i := range ids {
		ids[i] = fmt.Sprintf("%d", i)
	}

	ctx := callonce.WithCache(context.Background())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		callonce.Get(ctx, func() (string, error) { return "v", nil }, callonce.L(benchKey, ids[i]))
	}
}

// Overhead when no cache is attached to the context (graceful degradation).
func BenchmarkNoCache(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		callonce.Get(ctx, func() (string, error) { return "v", nil }, callonce.L(benchKey, "1"))
	}
}

// Errors are not cached. Measure the retry path.
func BenchmarkErrorNotCached(b *testing.B) {
	ctx := callonce.WithCache(context.Background())
	fail := errors.New("fail")

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		callonce.Get(ctx, func() (string, error) { return "", fail }, callonce.L(benchKey, "1"))
	}
}

// ---------------------------------------------------------------------------
// Concurrent benchmarks: measure throughput under contention.
// ---------------------------------------------------------------------------

// 1000 goroutines all requesting the same key.
// Only one call executes; the rest wait and share the result.
func BenchmarkConcurrent_SameKey(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ctx := callonce.WithCache(context.Background())
		var wg sync.WaitGroup
		wg.Add(1000)
		for j := 0; j < 1000; j++ {
			go func() {
				defer wg.Done()
				callonce.Get(ctx, func() (string, error) { return "v", nil }, callonce.L(benchKey, "1"))
			}()
		}
		wg.Wait()
	}
}

// 1000 goroutines each requesting a unique key. No dedup, pure write contention.
func BenchmarkConcurrent_UniqueKeys(b *testing.B) {
	ids := make([]string, 1000)
	for i := range ids {
		ids[i] = fmt.Sprintf("%d", i)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ctx := callonce.WithCache(context.Background())
		var wg sync.WaitGroup
		wg.Add(1000)
		for j := 0; j < 1000; j++ {
			go func(j int) {
				defer wg.Done()
				callonce.Get(ctx, func() (string, error) { return "v", nil }, callonce.L(benchKey, ids[j]))
			}(j)
		}
		wg.Wait()
	}
}

// 1000 goroutines sharing 100 keys. Realistic mix of hits and dedup.
func BenchmarkConcurrent_MixedKeys(b *testing.B) {
	ids := make([]string, 100)
	for i := range ids {
		ids[i] = fmt.Sprintf("%d", i)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ctx := callonce.WithCache(context.Background())
		var wg sync.WaitGroup
		wg.Add(1000)
		for j := 0; j < 1000; j++ {
			go func(j int) {
				defer wg.Done()
				callonce.Get(ctx, func() (string, error) { return "v", nil }, callonce.L(benchKey, ids[j%100]))
			}(j)
		}
		wg.Wait()
	}
}

// b.RunParallel: cache hit under true parallel reader contention.
func BenchmarkParallel_CacheHit(b *testing.B) {
	ctx := callonce.WithCache(context.Background())
	callonce.Get(ctx, func() (string, error) { return "v", nil }, callonce.L(benchKey, "1"))

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			callonce.Get(ctx, func() (string, error) { return "v", nil }, callonce.L(benchKey, "1"))
		}
	})
}

// ---------------------------------------------------------------------------
// Singleflight comparison: same scenarios, raw singleflight (no caching).
// ---------------------------------------------------------------------------

// singleflight alone: 1000 goroutines, same key.
// Result is NOT cached, so every iteration goes through Do() again.
func BenchmarkSingleflight_SameKey(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var g singleflight.Group
		var wg sync.WaitGroup
		wg.Add(1000)
		for j := 0; j < 1000; j++ {
			go func() {
				defer wg.Done()
				g.Do("k", func() (any, error) { return "v", nil })
			}()
		}
		wg.Wait()
	}
}

// singleflight alone: 1000 goroutines, unique keys. No dedup benefit.
func BenchmarkSingleflight_UniqueKeys(b *testing.B) {
	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%d", i)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var g singleflight.Group
		var wg sync.WaitGroup
		wg.Add(1000)
		for j := 0; j < 1000; j++ {
			go func(j int) {
				defer wg.Done()
				g.Do(keys[j], func() (any, error) { return "v", nil })
			}(j)
		}
		wg.Wait()
	}
}

// singleflight alone: 1000 goroutines, 100 keys. Partial dedup.
func BenchmarkSingleflight_MixedKeys(b *testing.B) {
	keys := make([]string, 100)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%d", i)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var g singleflight.Group
		var wg sync.WaitGroup
		wg.Add(1000)
		for j := 0; j < 1000; j++ {
			go func(j int) {
				defer wg.Done()
				g.Do(keys[j%100], func() (any, error) { return "v", nil })
			}(j)
		}
		wg.Wait()
	}
}
