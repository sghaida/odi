// Package di provides a small, generic dependency injection helper.
//
// It models a constructed value (Val) plus a bag of recorded dependencies (Deps).
// Wiring is done via Injector functions that mutate a Service in-place and return
// typed errors on invalid wiring (e.g., duplicate keys or missing dependencies).
//
// Design goals:
//   - Lightweight: small API surface, no container graph, no reflection for injection.
//   - Explicit wiring: dependencies are injected intentionally via injectors.
//   - Safe defaults: detects duplicates and nil wiring mistakes early.
//   - Test-friendly: works well in unit tests and supports introspection via Deps.
//
// Notes on performance:
//   - The success path is dominated by a map write and a function call.
//   - Error paths avoid fmt.Errorf to keep failure handling inexpensive when used
//     in benchmarks or for control flow (e.g., TryGetAs missing checks).
package di

import (
	"errors"
	"reflect"
	"strconv"
)

var (
	// ErrNilTarget is returned when an injector is applied to a nil service
	// or a service with a nil Val.
	ErrNilTarget = errors.New("di: nil target service")

	// ErrNilDep is returned when an injector is created/applied with a nil dependency
	// service or a dependency service with a nil Val. Some helpers return a more
	// specific typed error with key context (see NilDependencyServiceError).
	ErrNilDep = errors.New("di: nil dependency service")

	// ErrNilBind is returned when an injector is created with a nil bind function.
	// Some helpers return a more specific typed error with key context (see NilBindError).
	ErrNilBind = errors.New("di: nil bind function")
)

// DependencyKey identifies a dependency stored in a Service's Deps bag.
//
// Keys are typically defined as package-level constants to avoid typos.
//
// Example:
//
//	const (
//	  KeyDB     di.DependencyKey = "db"
//	  KeyLogger di.DependencyKey = "logger"
//	)
type DependencyKey string

// Key converts a string into a DependencyKey.
//
// This is a small convenience for defining keys (often as constants).
func Key(name string) DependencyKey { return DependencyKey(name) }

// DuplicateKeyError is returned when an injector attempts to register a dependency
// under a key that already exists in the target Service.
type DuplicateKeyError struct{ Key DependencyKey }

// Error implements the error interface.
func (e DuplicateKeyError) Error() string {
	// Example: di: duplicate dependency key "db"
	return "di: duplicate dependency key " + strconv.Quote(string(e.Key))
}

// MissingDependencyError is returned when a dependency key is not present.
//
// It is used by TryGetAs to distinguish "missing" from "wrong type".
type MissingDependencyError struct{ Key DependencyKey }

// Error implements the error interface.
func (e MissingDependencyError) Error() string {
	// Example: di: dependency "db" missing
	return "di: dependency " + strconv.Quote(string(e.Key)) + " missing"
}

// WrongTypeDependencyError is returned when a dependency exists but is of a different type.
//
// It is used by TryGetAs when a key is present but the stored value is not *D.
type WrongTypeDependencyError struct {
	// Key is the dependency key requested.
	Key DependencyKey

	// GotType is reflect.TypeOf(raw).String() for the stored value.
	GotType string
}

// Error implements the error interface.
func (e WrongTypeDependencyError) Error() string {
	// Example: di: dependency "db" has wrong type (*mypkg.Logger)
	return "di: dependency " + strconv.Quote(string(e.Key)) + " has wrong type (" + e.GotType + ")"
}

// NilDependencyServiceError indicates a nil dependency service for a specific key.
//
// This provides key context without using fmt.Errorf.
type NilDependencyServiceError struct{ Key DependencyKey }

// Error implements the error interface.
func (e NilDependencyServiceError) Error() string {
	// Example: di: nil dependency service for key "db"
	return "di: nil dependency service for key " + strconv.Quote(string(e.Key))
}

// NilBindError indicates a nil bind function for a specific key.
//
// This provides key context without using fmt.Errorf.
type NilBindError struct{ Key DependencyKey }

// Error implements the error interface.
func (e NilBindError) Error() string {
	// Example: di: nil bind function for key "db"
	return "di: nil bind function for key " + strconv.Quote(string(e.Key))
}

// Service is a small DI container around a concrete instance plus recorded deps.
//
// Val is the constructed value.
// Deps stores dependency pointers keyed by DependencyKey for introspection/debugging.
//
// The dependency bag is intentionally loose (map[DependencyKey]any) so you can attach
// any pointer type without restricting user code.
//
// Typed retrieval is available via GetAs / TryGetAs / MustGetAs.
type Service[T any] struct {
	Val  *T
	Deps map[DependencyKey]any
}

// Init constructs a Service by calling ctor and initializing the dependency bag.
func Init[T any](ctor func() *T) *Service[T] {
	return &Service[T]{Val: ctor(), Deps: make(map[DependencyKey]any)}
}

// Value returns the constructed value pointer.
func (s *Service[T]) Value() *T { return s.Val }

// Injector mutates a Service in-place and returns an error if wiring fails.
//
// Injectors are applied via (*Service[T]).With or WithAll.
type Injector[T any] func(*Service[T]) error

// With applies a single injector to the Service.
//
// If inj is nil, With is a no-op and returns (s, nil).
func (s *Service[T]) With(inj Injector[T]) (*Service[T], error) {
	if inj == nil {
		return s, nil
	}
	if err := inj(s); err != nil {
		return s, err
	}
	return s, nil
}

// WithAll applies multiple injectors in order.
//
// It stops at the first error and returns that error.
func (s *Service[T]) WithAll(deps ...Injector[T]) (*Service[T], error) {
	for _, inj := range deps {
		if _, err := s.With(inj); err != nil {
			return s, err
		}
	}
	return s, nil
}

// Injecting builds an Injector that binds a dependency into a target.
//
// It records the dependency pointer in s.Deps[key], then calls bind to attach
// the dependency to the target service implementation.
//
// The returned injector fails if:
//   - the target service (or its Val) is nil (ErrNilTarget)
//   - the dependency service (or its Val) is nil (NilDependencyServiceError)
//   - bind is nil (NilBindError)
//   - key already exists in the target's Deps (DuplicateKeyError)
func Injecting[T any, D any](
	key DependencyKey,
	dep *Service[D],
	bind func(target *T, dependency *D),
) Injector[T] {
	return func(s *Service[T]) error {
		if s == nil || s.Val == nil {
			return ErrNilTarget
		}
		if dep == nil || dep.Val == nil {
			return NilDependencyServiceError{Key: key}
		}
		if bind == nil {
			return NilBindError{Key: key}
		}
		if s.Deps == nil {
			s.Deps = make(map[DependencyKey]any)
		}
		if _, exists := s.Deps[key]; exists {
			return DuplicateKeyError{Key: key}
		}

		d := dep.Val
		s.Deps[key] = d
		bind(s.Val, d)
		return nil
	}
}

// Has reports whether a dependency exists for the key (regardless of type).
func (s *Service[T]) Has(key DependencyKey) bool {
	if s == nil || s.Deps == nil {
		return false
	}
	_, ok := s.Deps[key]
	return ok
}

// GetAny returns the raw stored dependency value without type assertions.
func (s *Service[T]) GetAny(key DependencyKey) (any, bool) {
	if s == nil || s.Deps == nil {
		return nil, false
	}
	v, ok := s.Deps[key]
	return v, ok
}

// GetAs returns the dependency typed as *D.
//
// ok is false if the key is missing or the stored value is not a *D.
func GetAs[T any, D any](s *Service[T], key DependencyKey) (*D, bool) {
	if s == nil || s.Deps == nil {
		return nil, false
	}
	raw, ok := s.Deps[key]
	if !ok || raw == nil {
		return nil, false
	}
	d, ok := raw.(*D)
	return d, ok
}

// TryGetAs returns the dependency typed as *D.
//
// It returns:
//   - MissingDependencyError if the key is not present
//   - WrongTypeDependencyError if the key exists but is not a *D
//
// It avoids fmt.Errorf so failure paths can be used in hot-ish code without
// paying formatting costs per call.
func TryGetAs[T any, D any](s *Service[T], key DependencyKey) (*D, error) {
	if s == nil || s.Deps == nil {
		return nil, MissingDependencyError{Key: key}
	}
	raw, ok := s.Deps[key]
	if !ok || raw == nil {
		return nil, MissingDependencyError{Key: key}
	}
	d, ok := raw.(*D)
	if !ok {
		return nil, WrongTypeDependencyError{
			Key:     key,
			GotType: reflect.TypeOf(raw).String(),
		}
	}
	return d, nil
}

// MustGetAs returns the dependency typed as *D or panics.
//
// It panics if the key is missing or the stored value is not a *D.
func MustGetAs[T any, D any](s *Service[T], key DependencyKey) *D {
	d, ok := GetAs[T, D](s, key)
	if !ok {
		panic(MissingDependencyError{Key: key})
	}
	return d
}

// Clone returns a shallow copy of the Service.
//
// The constructed value pointer (Val) is shared.
// The dependency bag (Deps) is copied into a new map so further wiring does not
// mutate the original Service's Deps.
func (s *Service[T]) Clone() *Service[T] {
	if s == nil {
		return nil
	}
	cp := &Service[T]{Val: s.Val}
	if len(s.Deps) > 0 {
		cp.Deps = make(map[DependencyKey]any, len(s.Deps))
		for k, v := range s.Deps {
			cp.Deps[k] = v
		}
	} else {
		cp.Deps = make(map[DependencyKey]any)
	}
	return cp
}
