// Package di provides a set of explicit dependency injection approaches for Go.
//
// This repository explores a progression of small, opinionated patterns:
//
//   - v1: runtime injectors + dependency introspection (dep bag / typed-ish errors)
//   - v2: construction only (plain constructors + manual wiring)
//   - v3: code-generated facades/builders (di1)
//   - v4: code-generated facades + graph composition roots + optional-deps registry (di2)
//
// The goal is to keep wiring explicit (usually in your composition root / main),
// avoid reflection-based containers, and keep the surface area intentionally small.
//
// Start with the README and examples in the repo for end-to-end wiring style.
//
// Package di See subpackages:
//   - di: library package(s) used by the examples / generators
//   - cmd/di1, cmd/di2: code generators for v3/v4 style wiring
//   - examples/*: runnable examples for each version
package di
