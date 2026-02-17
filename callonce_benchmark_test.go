package callonce_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/probablyarth/callonce-go"
	"golang.org/x/sync/singleflight"
)

func BenchmarkGet_SameKey_1000(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ctx := callonce.WithCache(context.Background())
		var wg sync.WaitGroup
		wg.Add(1000)
		for j := 0; j < 1000; j++ {
			go func() {
				defer wg.Done()
				callonce.Get(ctx, "k", func() (string, error) {
					return "v", nil
				})
			}()
		}
		wg.Wait()
	}
}

func BenchmarkGet_UniqueKeys_1000(b *testing.B) {
	b.ReportAllocs()
	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%d", i)
	}
	for i := 0; i < b.N; i++ {
		ctx := callonce.WithCache(context.Background())
		var wg sync.WaitGroup
		wg.Add(1000)
		for j := 0; j < 1000; j++ {
			go func(j int) {
				defer wg.Done()
				callonce.Get(ctx, keys[j], func() (string, error) {
					return "v", nil
				})
			}(j)
		}
		wg.Wait()
	}
}

func BenchmarkGet_MixedWorkload(b *testing.B) {
	b.ReportAllocs()
	keys := make([]string, 100)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%d", i)
	}
	for i := 0; i < b.N; i++ {
		ctx := callonce.WithCache(context.Background())
		var wg sync.WaitGroup
		wg.Add(1000)
		for j := 0; j < 1000; j++ {
			go func(j int) {
				defer wg.Done()
				callonce.Get(ctx, keys[j%100], func() (string, error) {
					return "v", nil
				})
			}(j)
		}
		wg.Wait()
	}
}

func BenchmarkGet_CacheHit(b *testing.B) {
	b.ReportAllocs()
	ctx := callonce.WithCache(context.Background())
	// Pre-populate.
	callonce.Get(ctx, "k", func() (string, error) {
		return "v", nil
	})
	for i := 0; i < b.N; i++ {
		callonce.Get(ctx, "k", func() (string, error) {
			return "v", nil
		})
	}
}

func BenchmarkSingleflight_Baseline(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var g singleflight.Group
		var wg sync.WaitGroup
		wg.Add(1000)
		for j := 0; j < 1000; j++ {
			go func() {
				defer wg.Done()
				g.Do("k", func() (any, error) {
					return "v", nil
				})
			}()
		}
		wg.Wait()
	}
}

func BenchmarkGet_NoCache(b *testing.B) {
	b.ReportAllocs()
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		callonce.Get(ctx, "k", func() (string, error) {
			return "v", nil
		})
	}
}
