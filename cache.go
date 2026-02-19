package callonce

import (
	"sync"

	"golang.org/x/sync/singleflight"
)

// Cache holds request-scoped memoized results.
// Create one per request via WithCache and retrieve it via FromContext.
type Cache struct {
	group    singleflight.Group
	mu       sync.RWMutex
	store    map[string]any
	observer Observer
}

func (c *Cache) emit(event Event, keyName string, identifier string) {
	if c.observer == nil {
		return
	}
	c.observer.On(EventData{
		Event:      event,
		Key:        keyName,
		Identifier: identifier,
	})
}
