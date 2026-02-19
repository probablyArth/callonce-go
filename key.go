package callonce

import "fmt"

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

func (l Lookup[T]) getFullKey() string {
	return l.Key.name + delimiter + l.Identifier
}
