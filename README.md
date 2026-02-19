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
3. **Subsequent callers** get the cached result instantly (~26 ns, one allocation).
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

	"github.com/probablyarth/callonce-go"
)

var userKey = callonce.NewKey[string]("user")

func fetchUser() (string, error) {
	fmt.Println("calling downstream")
	return "alice", nil
}

func main() {
	ctx := callonce.WithCache(context.Background())

	// First call executes fetchUser.
	user, _ := callonce.Get(ctx, fetchUser, callonce.L(userKey, "1"))
	fmt.Println(user) // alice

	// Second call returns the cached result. fetchUser is not called again.
	user, _ = callonce.Get(ctx, fetchUser, callonce.L(userKey, "1"))
	fmt.Println(user) // alice
}
```

In a real app, call `WithCache` once in middleware and pass the context down.

## API

```go
// Create a typed cache key (typically a package-level var).
func NewKey[T any](name string) Key[T]

// Create a Lookup pairing a key with an identifier.
func L[T any](key Key[T], identifier string) Lookup[T]

// Attach a new cache to a context (typically once per request).
func WithCache(ctx context.Context, opts ...Option) context.Context

// Retrieve the cache from a context (nil if none).
func FromContext(ctx context.Context) *Cache

// Fetch-or-compute a value. Pass one or more lookups; a cache hit on any
// lookup returns immediately (OR semantics). On a miss, fn runs once and
// the result is cached under every lookup key.
func Get[T any](ctx context.Context, fn func() (T, error), lookups ...Lookup[T]) (T, error)

// Remove lookups from the cache so subsequent Get calls invoke fn again.
func Forget[T any](ctx context.Context, lookups ...Lookup[T])

// Attach an observer to receive hit, miss, and dedup events.
func WithObserver(o Observer) Option
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

### Lookups: key + identifier separation

A `Lookup[T]` pairs a `Key[T]` (*category*, e.g. "user") with an `identifier` string (*instance*, e.g. the user ID). The `L()` helper creates one:

```go
var userKey = callonce.NewKey[*User]("user")

// In a handler:
callonce.Get(ctx, fetchUser, callonce.L(userKey, userID))
```

### Multi-lookup OR semantics

A resource is often addressable by more than one identifier — an ID, a slug, an email, etc. Different code paths may look up the same resource by different identifiers, causing redundant calls even with caching.

Pass multiple lookups to `Get` and it applies **OR semantics**: a cache hit on *any* lookup returns immediately, and on a miss the result is stored under *every* lookup key. This means a fetch-by-slug automatically seeds the by-ID cache entry and vice versa.

```go
var byID   = callonce.NewKey[*User]("user-by-id")
var bySlug = callonce.NewKey[*User]("user-by-slug")

// One call, two cache entries. Future lookups by either identifier hit cache.
user, err := callonce.Get(ctx, fetchUser,
    callonce.L(byID, "42"),
    callonce.L(bySlug, "alice"),
)
```

### Manual invalidation with `Forget`

Sometimes you need to invalidate a cached entry mid-request — for example, after a mutation. `Forget` removes specific lookups from the cache so the next `Get` call triggers a fresh fetch.

```go
// Update the user, then invalidate so the next read sees the change.
updateUser(ctx, userID, newData)
callonce.Forget(ctx, callonce.L(userKey, userID))
```

Like `Get`, `Forget` is a no-op if the context has no cache.

### Observability with `Observer`

Attach an `Observer` to receive structured events on every cache interaction:

```go
type Observer interface {
    On(eventData EventData)
}
```

Three event types are emitted:
- `EventHit` — a cached value was returned
- `EventMiss` — no cache entry existed, `fn` was called
- `EventDedup` — a concurrent caller shared an in-flight singleflight result

Each event carries the key name and identifier, so you can log, count, or push metrics however you like:

```go
type metricsObserver struct{}

func (m *metricsObserver) On(e callonce.EventData) {
    switch e.Event {
    case callonce.EventHit:
        hitCounter.WithLabelValues(e.Key).Inc()
    case callonce.EventMiss:
        missCounter.WithLabelValues(e.Key).Inc()
    case callonce.EventDedup:
        dedupCounter.WithLabelValues(e.Key).Inc()
    }
}

ctx := callonce.WithCache(r.Context(), callonce.WithObserver(&metricsObserver{}))
```

The observer is optional. When nil, no events are dispatched and there is zero overhead.

**Important:** `On` is called synchronously on the hot path — it blocks `Get` until it returns. Keep your observer fast (atomic increments, channel sends, etc.). Avoid blocking I/O like HTTP calls or disk writes inside `On`; push to a background worker instead.

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
| Multiple lookups | OR semantics; hit on any key, result stored under all |
| `Forget` | Removes specific lookups; next `Get` re-invokes `fn` |
| `Observer` | Optional; receives `EventHit`, `EventMiss`, `EventDedup` with key + identifier |

## Benchmarks

> Apple M4 Pro · Go 1.25 · `go test -bench=. -benchmem -count=10`

### Per-call latency

| Scenario | ns/op | B/op | allocs/op |
|----------|------:|-----:|----------:|
| Cache hit | **19.55** ± 2% | 0 | 0 |
| Cache miss (first call) | 389.4 ± 5% | 268 | 5 |
| No cache in context | 2.54 ± 1% | 0 | 0 |
| Error (not cached) | 105.6 ± 4% | 96 | 2 |

Cache hits resolve in **~20 ns** with zero allocations. The no-cache fallback path adds only ~2.5 ns.

### Concurrent throughput (1,000 goroutines)

| Scenario | µs/op | B/op | allocs/op |
|----------|------:|-----:|----------:|
| Same key (max dedup) | **284** ± 28% | 33 k | 1,013 |
| Mixed keys (100 keys) | 930 ± 2% | 136 k | 3,369 |
| Unique keys (no dedup) | 2,109 ± 2% | 509 k | 7,182 |

### callonce vs raw singleflight

Same 1,000-goroutine scenarios. Singleflight deduplicates in-flight calls but **does not cache results**, so every iteration goes through `Do()` again.

| Scenario | callonce | singleflight | speedup |
|----------|------:|------:|:------:|
| Same key | 284 µs | 658 µs | **2.3x** |
| Mixed keys | 930 µs | 643 µs | 0.7x |
| Unique keys | 2,109 µs | 630 µs | 0.3x |

callonce shines when keys repeat. The cache eliminates redundant `Do()` calls entirely. With mostly-unique keys the caching overhead (map writes, locks) costs more than it saves; in that scenario raw singleflight is leaner.

```
go test -bench=. -benchmem -count=10 ./...
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
