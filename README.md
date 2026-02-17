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

> Apple M4 Pro · Go 1.24 · `go test -bench=. -benchmem`

### Per-call latency

| Scenario | ns/op | B/op | allocs/op |
|----------|------:|-----:|----------:|
| Cache hit | **8.7** | 0 | 0 |
| Cache miss (first call) | 370 | 280 | 3 |
| No cache in context | 2.3 | 0 | 0 |
| Error (not cached) | 63 | 80 | 1 |

Cache hits resolve in **~9 ns** with **zero allocations** — just a read-lock and a map lookup.

### Concurrent throughput (1 000 goroutines)

| Scenario | µs/op | B/op | allocs/op |
|----------|------:|-----:|----------:|
| Same key (max dedup) | **239** | 33 k | 1 010 |
| Mixed keys (100 keys) | 815 | 120 k | 2 370 |
| Unique keys (no dedup) | 1 786 | 471 k | 5 149 |

### callonce vs raw singleflight

Same 1 000-goroutine scenarios — singleflight deduplicates in-flight calls but **does not cache results**, so every iteration goes through `Do()` again.

| Scenario | callonce | singleflight | speedup |
|----------|------:|------:|:------:|
| Same key | 239 µs | 666 µs | **2.8x** |
| Mixed keys | 815 µs | 610 µs | 0.7x |
| Unique keys | 1 786 µs | 604 µs | 0.3x |

callonce shines when keys repeat — the cache eliminates redundant `Do()` calls entirely. With mostly-unique keys the caching overhead (map writes, locks) costs more than it saves; in that scenario raw singleflight is leaner.

Run the benchmarks yourself:

```
go test -bench=. -benchmem ./...
```

## License

Apache-2.0
