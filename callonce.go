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
func Get[T any](ctx context.Context, key Key[T], identifier string, fn func() (T, error)) (T, error) {
	c := FromContext(ctx)
	if c == nil {
		return fn()
	}
	k := key.name + ":" + identifier

	// Fast path: already cached.
	c.mu.RLock()
	if v, ok := c.store[k]; ok {
		c.mu.RUnlock()
		return v.(T), nil
	}
	c.mu.RUnlock()

	// Slow path: singleflight dedup.
	val, err, _ := c.group.Do(k, func() (any, error) {
		// Double-check: another goroutine may have cached while we waited.
		c.mu.RLock()
		if v, ok := c.store[k]; ok {
			c.mu.RUnlock()
			return v, nil
		}
		c.mu.RUnlock()

		result, err := fn()
		if err != nil {
			return result, err
		}

		c.mu.Lock()
		c.store[k] = result
		c.mu.Unlock()

		return result, nil
	})

	if err != nil {
		var zero T
		return zero, err
	}

	return val.(T), nil
}
