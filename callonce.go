package callonce

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/singleflight"
)

type contextKey struct{}

// Key represents a strongly-typed cache key.
// The type parameter T is encoded into the underlying key string,
// so different types with the same name will not collide.
type Key[T any] struct {
	name string
}

// NewKey creates a new typed cache key.
func NewKey[T any](name string) Key[T] {
	var zero T
	return Key[T]{name: fmt.Sprintf("%T:%s", zero, name)}
}

// Lookup pairs a Key with an identifier for cache lookups.
type Lookup[T any] struct {
	Key        Key[T]
	Identifier string
}

// L creates a Lookup pairing a key with an identifier.
func L[T any](key Key[T], identifier string) Lookup[T] {
	return Lookup[T]{Key: key, Identifier: identifier}
}

// Cache holds request-scoped memoized results.
// Create one per request via WithCache and retrieve it via FromContext.
type Cache struct {
	group singleflight.Group
	mu    sync.RWMutex
	store map[string]any
}

// WithCache returns a child context that carries a new Cache.
func WithCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKey{}, &Cache{
		store: make(map[string]any),
	})
}

// FromContext retrieves the Cache from ctx, or nil if none is present.
func FromContext(ctx context.Context) *Cache {
	c, _ := ctx.Value(contextKey{}).(*Cache)
	return c
}

// Forget removes the given lookups from the cache so that subsequent
// calls to Get will invoke fn again. It is a no-op if ctx has no Cache.
func Forget[T any](ctx context.Context, lookups ...Lookup[T]) {
	c := FromContext(ctx)
	if c == nil {
		return
	}

	c.mu.Lock()
	for _, l := range lookups {
		delete(c.store, l.Key.name+":"+l.Identifier)
	}
	c.mu.Unlock()
}

// Get returns the value for the given lookups, calling fn at most once per
// cache. When multiple lookups are provided, a cache hit on any one of them
// returns immediately (OR semantics). On a cache miss fn is called once and
// the result is stored under every lookup key, so future callers using any
// of those identifiers will get a cache hit.
//
// If ctx has no Cache (WithCache was not called), fn is called directly.
func Get[T any](ctx context.Context, fn func() (T, error), lookups ...Lookup[T]) (T, error) {
	c := FromContext(ctx)
	if c == nil {
		return fn()
	}

	// Build cache key strings.
	cacheKeys := make([]string, len(lookups))
	for i, l := range lookups {
		cacheKeys[i] = l.Key.name + ":" + l.Identifier
	}

	// Fast path: check if any key is already cached.
	c.mu.RLock()
	for _, k := range cacheKeys {
		if v, ok := c.store[k]; ok {
			c.mu.RUnlock()
			// Backfill all other keys so future lookups by any
			// identifier also hit cache.
			if len(cacheKeys) > 1 {
				c.mu.Lock()
				for _, k2 := range cacheKeys {
					c.store[k2] = v
				}
				c.mu.Unlock()
			}
			return v.(T), nil
		}
	}
	c.mu.RUnlock()

	// Slow path: singleflight dedup on the first key.
	val, err, _ := c.group.Do(cacheKeys[0], func() (any, error) {
		// Double-check: another goroutine may have cached while we waited.
		c.mu.RLock()
		for _, k := range cacheKeys {
			if v, ok := c.store[k]; ok {
				c.mu.RUnlock()
				return v, nil
			}
		}
		c.mu.RUnlock()

		result, err := fn()
		if err != nil {
			return result, err
		}

		// Store under ALL keys.
		c.mu.Lock()
		for _, k := range cacheKeys {
			c.store[k] = result
		}
		c.mu.Unlock()

		return result, nil
	})

	if err != nil {
		var zero T
		return zero, err
	}

	return val.(T), nil
}
