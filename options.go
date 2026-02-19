package callonce

// Option configures a Cache created by WithCache.
type Option func(*Cache)

// WithObserver attaches an Observer that receives hit, miss, and dedup
// events for the lifetime of the cache.
func WithObserver(o Observer) Option {
	return func(cache *Cache) {
		cache.observer = o
	}
}
