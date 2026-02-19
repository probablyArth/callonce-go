package callonce

import (
	"context"
)

type contextKey struct{}

// WithCache returns a child context that carries a new Cache.
func WithCache(ctx context.Context, opts ...Option) context.Context {
	cache := &Cache{
		store: make(map[string]any),
	}
	for _, opt := range opts {
		opt(cache)
	}
	return context.WithValue(ctx, contextKey{}, cache)
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
		delete(c.store, l.getFullKey())
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
	if len(lookups) == 0 {
		return fn()
	}

	c := FromContext(ctx)
	if c == nil {
		return fn()
	}

	// Fast path: check if any key is already cached.
	c.mu.RLock()
	for _, lookup := range lookups {
		if v, ok := c.store[lookup.getFullKey()]; ok {
			c.mu.RUnlock()
			c.emit(EventHit, lookup.Key.name, lookup.Identifier)
			if len(lookups) > 1 {
				c.mu.Lock()
				for _, l2 := range lookups {
					c.store[l2.getFullKey()] = v
				}
				c.mu.Unlock()
			}
			return v.(T), nil
		}
	}
	c.mu.RUnlock()

	// Slow path: singleflight dedup on the first key.
	val, err, shared := c.group.Do(lookups[0].Key.name+delimiter+lookups[0].Identifier, func() (any, error) {
		// Double-check: another goroutine may have cached while we waited.
		c.mu.RLock()
		for _, l := range lookups {
			if v, ok := c.store[l.getFullKey()]; ok {
				c.mu.RUnlock()
				c.emit(EventHit, l.Key.name, l.Identifier)
				return v, nil
			}
		}
		c.mu.RUnlock()

		c.emit(EventMiss, lookups[0].Key.name, lookups[0].Identifier)
		result, err := fn()
		if err != nil {
			return result, err
		}

		// Store under ALL keys.
		c.mu.Lock()
		for _, l := range lookups {
			c.store[l.getFullKey()] = result
		}
		c.mu.Unlock()

		return result, nil
	})

	// Shared callers piggyback on the in-flight result.
	if shared {
		c.emit(EventDedup, lookups[0].Key.name, lookups[0].Identifier)
	}

	if err != nil {
		var zero T
		return zero, err
	}

	return val.(T), nil
}
