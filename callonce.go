package callonce

import (
	"context"
	"sync"

	"golang.org/x/sync/singleflight"
)

type contextKey struct{}

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

// Get returns the value for key, calling fn at most once per cache
// for a given key. Concurrent callers for the same key block and
// receive the same result. Errors are not cached.
//
// If ctx has no Cache (WithCache was not called), fn is called directly.
//
// The same key must always be used with the same type T.
func Get[T any](ctx context.Context, key string, fn func() (T, error)) (T, error) {
	c := FromContext(ctx)
	if c == nil {
		return fn()
	}

	// Fast path: already cached.
	c.mu.RLock()
	if v, ok := c.store[key]; ok {
		c.mu.RUnlock()
		return v.(T), nil
	}
	c.mu.RUnlock()

	// Slow path: singleflight dedup.
	val, err, _ := c.group.Do(key, func() (any, error) {
		// Double-check: another goroutine may have cached while we waited.
		c.mu.RLock()
		if v, ok := c.store[key]; ok {
			c.mu.RUnlock()
			return v, nil
		}
		c.mu.RUnlock()

		result, err := fn()
		if err != nil {
			return result, err
		}

		c.mu.Lock()
		c.store[key] = result
		c.mu.Unlock()

		return result, nil
	})

	if err != nil {
		var zero T
		return zero, err
	}
	return val.(T), nil
}
