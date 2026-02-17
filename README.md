# callonce-go

[![Go Reference](https://pkg.go.dev/badge/github.com/probablyarth/callonce-go.svg)](https://pkg.go.dev/github.com/probablyarth/callonce-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/probablyarth/callonce-go)](https://goreportcard.com/report/github.com/probablyarth/callonce-go)

Request-scoped call coalescing and memoization for Go.

When a single HTTP request fans out into multiple goroutines that fetch the same downstream resource, `callonce` ensures the function is called **once** and the result is shared. Think `singleflight` + caching, scoped to a request lifetime via `context.Context`.

## Install

```
go get github.com/probablyarth/callonce-go
```

## Usage

```go
package main

import (
	"context"
	"fmt"

	callonce "github.com/probablyarth/callonce-go"
)

func fetchUser() (string, error) {
	fmt.Println("calling downstream")
	return "alice", nil
}

func main() {
	// Attach a cache to the request context.
	ctx := callonce.WithCache(context.Background())

	// First call executes fetchUser.
	user, _ := callonce.Get(ctx, "user:1", fetchUser)
	fmt.Println(user) // alice

	// Second call returns the cached result — fetchUser is not called again.
	user, _ = callonce.Get(ctx, "user:1", fetchUser)
	fmt.Println(user) // alice
}
```

In a real app you'd call `WithCache` once at the top of your HTTP handler (or middleware) and pass the context down.

## API

```go
// Attach a new cache to a context (typically once per request).
func WithCache(ctx context.Context) context.Context

// Retrieve the cache from a context (nil if none).
func FromContext(ctx context.Context) *Cache

// Fetch-or-compute a value. Concurrent callers for the same key
// share a single in-flight call and its cached result.
func Get[T any](ctx context.Context, key string, fn func() (T, error)) (T, error)
```

| Behaviour | Detail |
|-----------|--------|
| Errors | Not cached — a failed call can be retried |
| `nil` values | Cached — a `(nil, nil)` result is stored |
| No cache in context | `fn` is called directly (graceful degradation) |
| Panics | Propagate to all waiters without poisoning the cache |
| Type safety | Same key must always use the same type `T` |

## Benchmarks

```
BenchmarkGet_CacheHit-14             138195949       8.620 ns/op      0 B/op    0 allocs/op
BenchmarkGet_NoCache-14              539415440       2.268 ns/op      0 B/op    0 allocs/op
BenchmarkGet_SameKey_1000-14             5181      231213 ns/op   33027 B/op  1010 allocs/op
BenchmarkGet_MixedWorkload-14            1531      786255 ns/op  120408 B/op  2377 allocs/op
BenchmarkGet_UniqueKeys_1000-14           685     1784373 ns/op  468797 B/op  5143 allocs/op
BenchmarkSingleflight_Baseline-14        1954      627942 ns/op   42605 B/op  1231 allocs/op
```

Cache hits are ~9ns with zero allocations. The no-cache fallback path adds ~2ns of overhead.

## License

Apache-2.0
