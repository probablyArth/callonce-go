package callonce

// Observer receives cache lifecycle events. Implementations must be safe
// for concurrent use when the cache is accessed from multiple goroutines.
type Observer interface {
	On(eventData EventData)
}

// Event represents a cache event type.
type Event int

const (
	// EventHit is emitted when a Get call finds a cached value.
	EventHit Event = iota
	// EventMiss is emitted when a Get call invokes fn.
	EventMiss
	// EventDedup is emitted when a concurrent caller shares an in-flight
	// singleflight result instead of triggering a new call.
	EventDedup
)

// EventData carries the details of a cache event.
type EventData struct {
	Event      Event
	Key        string
	Identifier string
}
