// Package di provides small, explicit dependency wiring helpers for Go.
//
// This package intentionally supports two approaches:
//
//   - v1: Service[T] + Injector[T] — explicit wiring with a dependency bag (Deps)
//     for introspection and structured/typed-ish errors (duplicate keys, missing deps,
//     wrong types). Best when you want guardrails and test assertions around wiring.
//
//   - v2: ServiceV2[T] — minimal construction-only wrapper with no dependency tracking,
//     no injectors, and no runtime validation. Best when you want maximum simplicity
//     and fully manual wiring via normal constructors and field assignment.
//
// Both versions avoid reflection-based injection and do not provide an automatic container
// or graph resolution. Wiring remains explicit in your composition root (main/bootstrap).
//
// Quick guidance
//
// Use v1 when you want:
//   - Explicit wiring plus guardrails (Build/With-style validation patterns)
//   - Dependency introspection (what was injected?) via Deps
//   - Structured errors you can assert in tests
//
// Use v2 when you want:
//   - Just constructors and structs (manual wiring)
//   - Zero DI concepts inside services
//   - Minimal runtime overhead and simplest API
//
// examples can be found under examples/v1 and examples/v2
// Import
//
//	 "github.com/sghaida/odi/di"
package di
