package di

import (
	"errors"
	"fmt"
)

// Registry provides optional dependencies at build time.
//
// It is intentionally:
// - read-only
// - side effect free
// - build-time only
//
// Expected usage:
//
//	val, ok, err := reg.Resolve(cfg, "some.key")
type Registry interface {
	Resolve(cfg any, key string) (val any, ok bool, err error)
}

// ErrRegistryPanic is returned if a registry implementation panics internally.
var ErrRegistryPanic = errors.New("registry: panic during Resolve")

// MapRegistry is a simple in-memory registry.
// It ignores cfg (but keeps it in the signature so future registries can use it).
type MapRegistry struct {
	items map[string]any
}

func NewMapRegistry() *MapRegistry {
	return &MapRegistry{items: map[string]any{}}
}

// Provide stores a value under a key and returns the registry for chaining.
func (r *MapRegistry) Provide(key string, val any) *MapRegistry {
	r.items[key] = val
	return r
}

// Resolve implements Registry and defensively converts panics into errors.
func (r *MapRegistry) Resolve(_ any, key string) (val any, ok bool, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			val = nil
			ok = false
			err = fmt.Errorf("%w: %v", ErrRegistryPanic, rec)
		}
	}()

	v, ok := r.items[key]
	return v, ok, nil
}

// Get returns the value if present (no panic).
func (r *MapRegistry) Get(key string) (any, bool) {
	v, ok := r.items[key]
	return v, ok
}

// MustGet returns the value or panics with a helpful message.
// Useful in examples/tests where missing registry keys should fail fast.
func (r *MapRegistry) MustGet(key string) any {
	v, ok := r.items[key]
	if !ok {
		panic(fmt.Errorf("di: registry missing key %q", key))
	}
	return v
}
