package di

// v2 provides a minimal dependency construction helper.
//
// v2 removes all dependency tracking and injectors from v1 and keeps only the core idea:
//
//   - Construct values explicitly and wire them manually.
//
// There are no keys, no injectors, no dependency bag, and no runtime validation.
//
// Use v2 when you want maximum simplicity (constructors + structs), zero DI concepts inside
// services, and minimal runtime cost. If you want guardrails (validation, introspection,
// typed-ish errors), use v1 (Service[T]).


// ServiceV2 Generic service instance container (no dep-state here anymore; state is in wrappers)
// It intentionally contains no dependency state.
// Wiring is done manually by assigning fields directly in constructors or in main.
type ServiceV2[T any] struct {
	Val *T
}

// New constructs a value via ctor and returns it wrapped in ServiceV2.
//
// The return is a value (not a pointer) to keep the API minimal.
func New[T any](ctor func() *T) ServiceV2[T] {
	return ServiceV2[T]{ Val: ctor()}
}
