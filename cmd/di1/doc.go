// Command di1 — v3 code-generated facades (di1) for explicit injection (Go)
//
// Version v3 introduces code generation (cmd/di1) to keep wiring explicit while adding
// compile-time ergonomics:
//
//   - You write a tiny *.inject.json spec next to your service.
//   - You add a //go:generate ... directive in the owner Go file.
//   - di1 generates a facade/builder with:
//       - Inject<Name>(dep) methods for required dependencies
//       - a generic Inject(fn) hook for custom / optional wiring
//       - Build() validation and MustBuild() convenience
//
// There is no container, no reflection wiring, no module graphs.
//
// When to use v3
//
// Use v3 when you want:
//
//   - Explicit wiring in main/bootstrap, but with less boilerplate than v1/v2.
//   - Build-time guardrails: required deps must be wired, enforced by Build().
//   - An ergonomic injection API: InjectDB(...), InjectCache(...), etc.
//   - A clear separation between construction, wiring, and validation.
//   - A repeatable pattern across many services/packages.
//
// When NOT to use v3
//
// Avoid v3 if you need automatic graph resolution across many packages, lifecycle management,
// advanced scoping, whole-graph compile-time generation (like Wire), or you cannot use codegen
// per repo/tooling policy. Consider Wire or fx/dig in those cases.
//
// Core idea
//
// v3 generates a builder (facade) around a concrete implementation:
//
//   - Construct the service (New<Facade>(cfg) calls your constructor)
//   - Track which required deps were injected (hasX booleans)
//   - Provide explicit InjectX(...) methods
//   - Validate wiring at Build() time
//
// Spec format (*.inject.json)
//
// Minimal example:
//
//	{
//	  "package": "v3",
//	  "wrapperBase": "FraudSvc",
//	  "versionSuffix": "V3",
//	  "implType": "FraudSvc",
//	  "constructor": "NewFraudSvc",
//	  "imports": {
//	    "config": "github.com/sghaida/odi/examples/v3/config"
//	  },
//	  "required": [
//	    { "name": "TransactionGetter", "field": "txGetter", "type": "TransactionGetter" },
//	    { "name": "DecisionWriter",     "field": "writer",   "type": "DecisionWriter" }
//	  ],
//	  "optional": [
//	    { "name": "Logger", "field": "logger", "type": "Logger" }
//	  ]
//	}
//
// Typical go:generate usage
//
// Put this in the owner Go file (same package directory as the spec):
//
//	//go:generate go run ../../cmd/di1 -spec ./specs/fraud.inject.json -out ./fraud_di.gen.go
//
// Then:
//
//	go generate ./...
//
// Generated API (summary)
//
// The generated facade/builder typically includes:
//
//   - New<Facade>(cfg) *<Facade>
//   - Inject<Name>(dep <Type>) *<Facade>    // for each required dep
//   - Inject(fn func(*<ImplType>)) *<Facade> // custom/optional wiring
//   - Build() (*<ImplType>, error)           // validates required deps
//   - MustBuild() *<ImplType>                // panics on invalid wiring
//
// Example wiring
//
//	builder := v3.NewFraudSvcV3(cfg).
//		InjectTransactionGetter(txRepo).
//		Inject(func(s *v3.FraudSvc) { s.SetLogger(log) }).
//		InjectDecisionWriter(decisionSvc)
//
//	svc, err := builder.Build()
//	if err != nil {
//		// handle invalid wiring
//	}
//
// Handling cycles
//
// di1 does not resolve cycles automatically. A safe explicit pattern is:
//
//   - Create both builders (each constructs its underlying pointer).
//   - Capture pointers via Inject(fn).
//   - Wire cross-references via setters inside Inject(fn).
//   - Call Build()/MustBuild() after required deps are satisfied.
//
// See the repository docs/service-v3.md and examples/v3 for end-to-end usage.
package main