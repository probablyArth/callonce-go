# callonce-go

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

### `WithCache(ctx context.Context) context.Context`

Returns a child context carrying a new `Cache`.

### `FromContext(ctx context.Context) *Cache`

Retrieves the `Cache` from the context, or `nil` if none is present.

### `Get[T any](ctx context.Context, key string, fn func() (T, error)) (T, error)`

Returns the value for `key`. If the value isn't cached, `fn` is called exactly once — concurrent callers for the same key block and receive the same result.

- **Errors are not cached.** A failed call can be retried.
- **`nil` values are cached.** A `(nil, nil)` result is stored.
- **No cache in context?** `fn` is called directly (graceful degradation).
- **Panics propagate** to all waiting callers without poisoning the cache.
- **Same key = same type.** Using the same key with different type parameters is unsupported.

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
