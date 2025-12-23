// Command di2 — v4 code-generated facades + graph composition roots for explicit wiring (Go)
//
// Version v4 extends v3’s “explicit DI via codegen” approach with two upgrades that
// make large wiring setups easier while staying explicit:
//
//   - Optional dependencies via a Registry (di.Registry)
//   - Whole-app wiring via a generated Graph (composition root)
//
// No container graphs, no reflection injection, no runtime magic, no lifecycle framework —
// just explicit wiring with better ergonomics.
//
// Why v4 exists
//
// As projects grow, manual wiring becomes noisy:
//
//   - Repetitive constructor + field assignment code in main
//   - Optional integration points (tracing/metrics/logging) sprinkled everywhere
//   - Cycles exist, but you still want explicit control
//
// v4 keeps wiring explicit, but:
//   - generates builders/facades for services so required wiring is validated
//   - supports optional deps cleanly via a registry
//   - generates an app graph function so the composition root stays small and readable
//
// When to use v4
//
// Use v4 when you want:
//
//   - Explicit wiring that scales to many services
//   - Build-time guardrails:
//       - required deps validated by Build()/MustBuild()
//       - per-method "requires" checks (safe wrappers)
//   - Optional deps that don’t leak into constructors, supplied at build time via a registry
//   - A clean composition root (generated graph function wires/builds the full app)
//   - Explicit, intentional cycle wiring (UnsafeImpl() for composition-root wiring)
//
// When NOT to use v4
//
// Avoid v4 if you need automatic graph resolution, lifecycle management, advanced scoping,
// or if repo/tooling policy disallows code generation. Consider Wire (compile-time whole-graph)
// or fx/dig (runtime container + lifecycle) in those cases.
//
// What di2 generates
//
// di2 produces two generated outputs:
//
//   1) Per-service facade/builder (from *.inject.json)
//   2) Graph composition root (from graph.json)
//
// A) Per-service facade/builder (from *.inject.json)
//
// For each service, di2 generates a facade around your concrete implementation:
//
//   - New<Facade>(...) constructs the underlying *Impl via your constructor
//   - InjectX(...) for required deps
//   - Build()/MustBuild() validates required deps
//   - BuildWith(reg di.Registry) applies optional deps from the registry, then validates
//   - UnsafeImpl() returns the underlying pointer for wiring only (composition root)
//   - Optional safe method wrappers that enforce per-method "requires" deps
//
// B) Graph composition root (from graph.json)
//
// di2 can generate a function like BuildAppV4(cfg, reg) that:
//
//   - creates builders for each service
//   - wires the graph explicitly (including cycles)
//   - calls Build() or BuildWith(reg) per service
//   - returns a result struct containing built service pointers
//
// Optional deps via Registry
//
// v4 uses a minimal interface:
//
//	type Registry interface {
//		Resolve(cfg any, key string) (val any, ok bool, err error)
//	}
//
// Generated builders use registry keys (e.g. "v4.tracer") to resolve optional deps,
// apply them (setter or field assignment), and can fall back to a default expression
// when the key is missing.
//
// Spec overview
//
// Service specs (*.inject.json) describe construction, required deps, optional deps,
// and method-level "requires" for safe wrappers.
//
// Graph specs (graph.json) describe the composition root: which services exist,
// how builders are constructed, how wiring connects services, and whether builds
// use BuildWith(reg) or Build().
//
// Typical go:generate usage
//
// Per service:
//
//	//go:generate go run ../../cmd/di2 -spec specs/core.inject.json -out core_v4.gen.go
//
// For a graph:
//
//	//go:generate go run ../../cmd/di2 -graph specs/graph.json -out graph_v4.gen.go
//
// Then:
//
//	go generate ./...
//
// Cycle wiring note
//
// di2 does not solve cycles automatically. Cycles remain explicit. UnsafeImpl() exists
// to enable composition-root wiring before Build()/BuildWith() validation completes.
// Do not call business methods on the underlying implementation before Build().
//
// See the repository docs/service-v4.md and examples/v4 for end-to-end usage.
package main
