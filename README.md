# callonce-go

[![Go Reference](https://pkg.go.dev/badge/github.com/probablyarth/callonce-go.svg)](https://pkg.go.dev/github.com/probablyarth/callonce-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/probablyarth/callonce-go)](https://goreportcard.com/report/github.com/probablyarth/callonce-go)

Request-scoped deduplication and memoization for Go.

## The problem

A single HTTP request often fans out into multiple goroutines (middleware, service layers, template rendering) that independently call the same downstream resource. Without coordination:

- **Redundant calls.** The same database query or API call runs 3-5x per request.
- **Wasted resources.** Each duplicate call consumes a connection, adds latency, and increases load on downstream services.
- **singleflight alone isn't enough.** It deduplicates *in-flight* calls, but once a call completes, the next caller triggers it all over again. There's no caching.

## The solution

`callonce` combines **singleflight deduplication** with a **per-request cache**, scoped to a `context.Context` lifetime:

1. **First caller** for a key triggers the function and caches the result.
2. **Concurrent callers** for the same key share the in-flight call (singleflight).
3. **Subsequent callers** get the cached result instantly (~9 ns, zero allocations).
4. **When the request ends**, the context (and cache) is garbage collected. No TTLs, no eviction, no stale data.

## Install

```
go get github.com/probablyarth/callonce-go
```

## Quick start

```go
package main

import (
	"context"
	"fmt"

	callonce "github.com/probablyarth/callonce-go"
)

var userKey = callonce.NewKey[string]("user")

func fetchUser() (string, error) {
	fmt.Println("calling downstream")
	return "alice", nil
}

func main() {
	ctx := callonce.WithCache(context.Background())

	// First call executes fetchUser.
	user, _ := callonce.Get(ctx, userKey, "1", fetchUser)
	fmt.Println(user) // alice

	// Second call returns the cached result. fetchUser is not called again.
	user, _ = callonce.Get(ctx, userKey, "1", fetchUser)
	fmt.Println(user) // alice
}
```

In a real app, call `WithCache` once in middleware and pass the context down.

## API

```go
// Create a typed cache key (typically a package-level var).
func NewKey[T any](name string) Key[T]

// Attach a new cache to a context (typically once per request).
func WithCache(ctx context.Context) context.Context

// Retrieve the cache from a context (nil if none).
func FromContext(ctx context.Context) *Cache

// Fetch-or-compute a value. Concurrent callers for the same key + identifier
// share a single in-flight call and its cached result.
func Get[T any](ctx context.Context, key Key[T], identifier string, fn func() (T, error)) (T, error)
```

## Design decisions

### Typed keys with `Key[T]`

Cache keys are created with `NewKey[T]`, which encodes the Go type into the underlying key string. This means `NewKey[string]("user")` and `NewKey[int]("user")` produce different cache slots, so **type collisions are impossible**. The compiler enforces that the function passed to `Get` returns the type matching the key.

```go
var userKey  = callonce.NewKey[*User]("user")   // Key[*User]
var countKey = callonce.NewKey[int]("count")     // Key[int]

// Compiler error: can't pass Key[int] where Key[*User] is expected.
```

### Declare keys once, not in hot paths

`NewKey[T]` uses `fmt.Sprintf` and reflection internally to encode the type name into the key string. This is what prevents type collisions, but it means each call allocates. Declare keys as package-level variables so the cost is paid once at init, not on every request:

```go
// Good: created once at startup.
var userKey = callonce.NewKey[*User]("user")

// Bad: allocates on every call to the handler.
func handler(w http.ResponseWriter, r *http.Request) {
    key := callonce.NewKey[*User]("user") // unnecessary allocation
    ...
}
```

### Key + identifier separation

The `Key[T]` represents the *category* (e.g. "user"), while the `identifier` string represents the *instance* (e.g. the user ID). This keeps key declarations static and reusable:

```go
var userKey = callonce.NewKey[*User]("user")

// In a handler:
callonce.Get(ctx, userKey, userID, fetchUser)
```

### Errors are not cached

A failed call doesn't poison the cache. The next caller retries the function, which is the right default for transient errors like network timeouts or database blips.

### Graceful degradation

If `WithCache` was never called (no cache in context), `Get` calls the function directly and returns the result. No panic, no error. Your code works with or without the cache.

### No TTLs or eviction

The cache is tied to the request context. When the request ends, the context is canceled, the cache becomes unreachable, and the GC cleans it up. This eliminates an entire class of bugs around stale data, cache invalidation, and memory leaks.

### Panic safety

If the function panics, the panic propagates to all waiting goroutines (via singleflight), but the cache is **not poisoned**. A subsequent call with the same key will retry.

## Behaviour summary

| Behaviour | Detail |
|-----------|--------|
| Errors | Not cached; a failed call can be retried |
| `nil` values | Cached; a `(nil, nil)` result is stored |
| No cache in context | `fn` is called directly (graceful degradation) |
| Panics | Propagate to all waiters without poisoning the cache |
| Type safety | Enforced at compile time via `Key[T]` |

## Benchmarks

> Apple M4 Pro · Go 1.24 · `go test -bench=. -benchmem`

### Per-call latency

| Scenario | ns/op | B/op | allocs/op |
|----------|------:|-----:|----------:|
| Cache hit | **24.4** ± 3% | 16 | 1 |
| Cache miss (first call) | 365 ± 5% | 303 | 4 |
| No cache in context | 2.5 ± 1% | 0 | 0 |
| Error (not cached) | 83.5 ± 1% | 96 | 2 |

Cache hits resolve in **~24 ns** with a single allocation (the key concatenation). The no-cache fallback path adds only ~2.5 ns.

### Concurrent throughput (1 000 goroutines)

| Scenario | µs/op | B/op | allocs/op |
|----------|------:|-----:|----------:|
| Same key (max dedup) | **235** ± 1% | 49 k | 2 010 |
| Mixed keys (100 keys) | 836 ± 1% | 132 k | 3 365 |
| Unique keys (no dedup) | 1 904 ± 1% | 481 k | 6 190 |

### callonce vs raw singleflight

Same 1 000-goroutine scenarios. Singleflight deduplicates in-flight calls but **does not cache results**, so every iteration goes through `Do()` again.

| Scenario | callonce | singleflight | speedup |
|----------|------:|------:|:------:|
| Same key | 235 µs | 682 µs | **2.9x** |
| Mixed keys | 836 µs | 636 µs | 0.8x |
| Unique keys | 1 904 µs | 611 µs | 0.3x |

callonce shines when keys repeat. The cache eliminates redundant `Do()` calls entirely. With mostly-unique keys the caching overhead (map writes, locks) costs more than it saves; in that scenario raw singleflight is leaner.

```
go test -bench=. -benchmem ./...
```

## Please Consider Giving the Repo a Star ⭐

<a href="https://github.com/probablyarth/callonce-go">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=probablyarth/callonce-go&type=Timeline&theme=dark" />
    <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=probablyarth/callonce-go&type=Timeline" />
    <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=probablyarth/callonce-go&type=Timeline" />
  </picture>
</a>

## License

Apache-2.0
