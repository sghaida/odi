# odi — v4 code-generated facades + graph composition roots (di2) for explicit wiring (Go)

Version **v4** extends v3’s “explicit DI via codegen” approach with two upgrades that make large wiring setups easier while staying **explicit**:

- **Optional dependencies via a Registry** (`di.Registry`)
- **Whole-app wiring via a generated Graph** (composition root)

No container graphs, no reflection injection, no runtime magic, no lifecycle framework — just explicit wiring with better ergonomics.

---

## Why v4 exists

As projects grow, pure “manual wiring” becomes noisy:

- Lots of repetitive constructor + field assignment code in `main`
- Optional integration points (tracing/metrics/logging) get sprinkled everywhere
- Cycles exist (service A references B and B references A), but you still want explicit control

v4 keeps wiring explicit, but:
- generates **builders/facades** for services so required wiring is validated
- supports **optional deps** cleanly via a registry
- generates an **app graph function** so the composition root stays small and readable

---

## When should you use v4?

Use **v4** when you want:

- **Explicit wiring** that scales to 10s of services.
- **Build-time guardrails**:
  - required deps validated by `Build()/MustBuild()`
  - per-method “requires” checks (safe wrappers)
- **Optional deps that don’t leak into constructors**, supplied at build time via a registry:
  - tracer / metrics / logger / feature flags / mocks
- **A clean composition root**:
  - one generated function wires and builds the full graph
  - keeps `main` small and readable
- **Support for cycles** (explicitly wired, intentionally):
  - `UnsafeImpl()` exists specifically for composition-root wiring

---

## When NOT to use v4

Avoid v4 if you need:

- Automatic graph resolution (container decides what to build)
- Lifecycle management (start/stop hooks, modules, scopes)
- Advanced scoping (request scopes, per-goroutine scopes)
- A repo policy that disallows code generation

Consider:
- **Wire** (compile-time whole-graph)
- **fx/dig** (runtime container + lifecycle)

---

## How v4 fits next to v1–v3

| Feature / Style                         | v1 (`di.Service[T]`) | v2 (construct-only) | v3 (`di1` codegen) | v4 (`di2` codegen) |
|-----------------------------------------|----------------------|---------------------|--------------------|--------------------|
| Code generation                         | ❌                    | ❌                   | ✅                  | ✅                  |
| Required dependency validation          | ✅                    | ❌                   | ✅ (`Build`)        | ✅ (`Build`)        |
| Method-level “requires” safe wrappers   | ❌                    | ❌                   | ❌                  | ✅                  |
| Optional deps                           | ❌                    | ❌                   | ❌                  | ✅ (`BuildWith`)    |
| Whole-graph composition root generation | ❌                    | ❌                   | ❌                  | ✅ (`graph.json`)   |
| Cycles support (explicit)               | ⚠️ (manual)          | ⚠️ (manual)         | ✅ (manual)         | ✅ (graph helpers)  |
| Container / auto graph resolution       | ❌                    | ❌                   | ❌                  | ❌                  |

Rule of thumb:

- **Need introspection & typed retrieval → v1**
- **Need maximum simplicity → v2**
- **Need explicit DI + less wiring boilerplate → v3**
- **Need explicit DI + optional deps + app graph → v4**

---

## What v4 generates

v4 produces **two generated outputs**:

1. **Per-service facade/builder** (from `*.inject.json`)
2. **Graph composition root** (from `graph.json`)

Both are driven by JSON specs committed to the repo.

### A) Per-service facade/builder (from `*.inject.json`)

For each service, `di2` generates a facade like `CoreV4` around your concrete `Core` type:

- `NewCoreV4(...)` — constructs the underlying `*Core` via your constructor
- `InjectX(...)` — generated per required dep
- `Build()` / `MustBuild()` — validates required deps
- `BuildWith(reg di.Registry)` — applies optional deps from registry, then validates
- `UnsafeImpl()` — returns the underlying pointer **only for wiring**
- Safe method wrappers:
  - wrapper checks required deps for that method before calling the underlying method

### B) Graph composition root (from `graph.json`)

`di2` generates a function like `BuildAppV4(cfg, reg)`:

- creates builders for each service
- wires the graph explicitly (including cycles)
- calls `Build()` or `BuildWith(reg)` per service
- returns a result struct containing built service pointers

---

## Runtime Registry API (optional deps)

v4 uses a minimal interface:

```go
type Registry interface {
  Resolve(cfg any, key string) (val any, ok bool, err error)
}
```

Generated builders use it like:

- `val, ok, err := reg.Resolve(cfg, "v4.tracer")`
- if `ok`, type-assert and apply
- if missing, either:
  - do nothing, or
  - apply a `defaultExpr` (recommended)

A small in-memory implementation is enough for examples and tests.

---

# Specs

## Service spec (`*.inject.json`)

Each service has a spec file describing:

- how the service is constructed
- required dependencies (validated by `Build`)
- optional dependencies (applied by `BuildWith(reg)`)
- method signatures and per-method required deps (for safe wrappers)

### Minimal service spec structure

```json
{
  "package": "v4",
  "wrapperBase": "Core",
  "versionSuffix": "V4",
  "implType": "Core",
  "constructor": "NewCore",

  "required": [],
  "optional": [],
  "methods": []
}
```

### Top-level fields

| Field                      | Meaning                                                                      |
|----------------------------|------------------------------------------------------------------------------|
| `package`                  | Go package for the generated file                                            |
| `wrapperBase`              | Base name for the generated facade (default: `<wrapperBase><versionSuffix>`) |
| `versionSuffix`            | Version suffix appended to the facade                                        |
| `implType`                 | Concrete service type                                                        |
| `constructor`              | Constructor function used to create the service                              |
| `facadeName`               | Optional override for facade name                                            |
| `publicConstructorName`    | Optional override for constructor name                                       |
| `injectPolicy.onOverwrite` | Duplicate required inject handling: `error`, `ignore`, `overwrite`           |
| `cyclic`                   | If true, spec indicates cycle wiring; generator still emits `UnsafeImpl()`   |

### Required dependencies

Required deps are **validated by `Build()`**.

```json
{
  "name": "Alpha",
  "field": "alpha",
  "type": "*Alpha",
  "nilable": true
}
```

| Field     | Meaning                                     |
|-----------|---------------------------------------------|
| `name`    | Logical dep name (used in `Inject<Name>`)   |
| `field`   | Field on the service to assign              |
| `type`    | Go type of the dep                          |
| `nilable` | Must be `true` (generator emits nil checks) |

Each required dep generates:

- `TryInject<Name>(dep)` (returns error)
- `Inject<Name>(dep)` (panics on policy violations)
- build-time validation in `Build()` / `BuildWith()`

### Optional dependencies (via Registry)

Optional deps are applied **only in `BuildWith(reg)`**.

```json
{
  "name": "Tracer",
  "type": "Tracer",
  "registryKey": "v4.tracer",
  "apply": { "kind": "setter", "name": "SetTracer" },
  "defaultExpr": "NoopTracer{}"
}
```

| Field         | Meaning                                            |
|---------------|----------------------------------------------------|
| `registryKey` | Key to use when resolving from the registry        |
| `apply.kind`  | `"setter"` or `"field"`                            |
| `apply.name`  | Setter method name or field name                   |
| `defaultExpr` | Expression applied if key is missing (recommended) |

#### `optional.apply.kind`

- `"setter"`: calls `svc.SetX(dep)`
- `"field"`: assigns `svc.someField = dep` (same-package only)

#### `defaultExpr`

Recommended for optional deps:
- keeps service logic simpler
- makes `BuildWith` deterministic

### Methods (safe wrappers)

v4 can generate wrapper methods that enforce required wiring **per method**.

```json
{
  "name": "Process",
  "params": [
    { "name": "ctx", "type": "context.Context" },
    { "name": "req", "type": "ProcessRequest" }
  ],
  "returns": [
    { "type": "ProcessResponse" },
    { "type": "error" }
  ],
  "requires": ["Alpha", "Beta"]
}
```

- The wrapper checks `requires` deps before calling the underlying method
- If wiring is incomplete, it returns zero values + error

---

### Full example: Core service spec

```json
{
  "package": "v4",
  "wrapperBase": "Core",
  "versionSuffix": "V4",
  "implType": "Core",
  "constructor": "NewCore",
  "facadeName": "CoreV4",
  "publicConstructorName": "NewCoreV4",
  "injectPolicy": { "onOverwrite": "error" },

  "required": [
    { "name": "Alpha", "field": "alpha", "type": "*Alpha", "nilable": true },
    { "name": "Beta",  "field": "beta",  "type": "*Beta",  "nilable": true }
  ],

  "optional": [
    {
      "name": "Tracer",
      "type": "Tracer",
      "registryKey": "v4.tracer",
      "apply": { "kind": "setter", "name": "SetTracer" },
      "defaultExpr": "NoopTracer{}"
    },
    {
      "name": "Metrics",
      "type": "Metrics",
      "registryKey": "v4.metrics",
      "apply": { "kind": "setter", "name": "SetMetrics" },
      "defaultExpr": "NoopMetrics{}"
    }
  ],

  "methods": [
    {
      "name": "Process",
      "params": [
        { "name": "ctx", "type": "context.Context" },
        { "name": "req", "type": "ProcessRequest" }
      ],
      "returns": [
        { "type": "ProcessResponse" },
        { "type": "error" }
      ],
      "requires": ["Alpha", "Beta"]
    }
  ]
}
```

---

## Graph spec (`graph.json`)

The graph spec defines the **composition root** for an application.

It lists:
- which services exist
- which builder constructors to call
- how wiring should connect services
- whether the build step should use a registry (`BuildWith`) or not (`Build`)

### Graph spec structure

```json
{
  "package": "v4",
  "roots": [
    {
      "name": "BuildAppV4",
      "buildWithRegistry": true,
      "services": [],
      "wiring": []
    }
  ]
}
```

### Services section

```json
{
  "var": "core",
  "facadeCtor": "NewCoreV4",
  "facadeType": "*CoreV4",
  "implType": "Core"
}
```

| Field        | Meaning                                           |
|--------------|---------------------------------------------------|
| `var`        | Variable name used in generated function          |
| `facadeCtor` | Builder constructor (`NewXv4`)                    |
| `facadeType` | Type of the builder (doc-only; helps readability) |
| `implType`   | Concrete implementation type                      |

### Wiring section

```json
{
  "to": "core",
  "call": "InjectAlpha",
  "argFrom": "alpha"
}
```

This expands to:

```go
coreB.InjectAlpha(alphaB.UnsafeImpl())
```

Wiring always happens **before** `Build()` / `BuildWith()`.

### Full example: Graph spec (Alpha ↔ Beta cycle + Core depends on both)

```json
{
  "package": "v4",
  "roots": [
    {
      "name": "BuildAppV4",
      "buildWithRegistry": true,
      "services": [
        { "var": "alpha", "facadeCtor": "NewAlphaV4", "facadeType": "*AlphaV4", "implType": "Alpha" },
        { "var": "beta",  "facadeCtor": "NewBetaV4",  "facadeType": "*BetaV4",  "implType": "Beta"  },
        { "var": "core",  "facadeCtor": "NewCoreV4",  "facadeType": "*CoreV4",  "implType": "Core"  }
      ],
      "wiring": [
        { "to": "alpha", "call": "InjectBeta",  "argFrom": "beta"  },
        { "to": "beta",  "call": "InjectAlpha", "argFrom": "alpha" },
        { "to": "core",  "call": "InjectAlpha", "argFrom": "alpha" },
        { "to": "core",  "call": "InjectBeta",  "argFrom": "beta"  }
      ]
    }
  ]
}
```

---

## Why `UnsafeImpl()` exists

- Builders must create services early (constructors run up front)
- Wiring requires concrete pointers
- Cycles must be explicit (v4 does not “solve” cycles automatically)
- `UnsafeImpl()` is for composition-root wiring only, never for calling business methods before `Build`

---

# Recommended workflow

## 1) Write services

Each service:
- has a constructor (optionally uses `config.Config`)
- declares required deps as fields (or setters)
- declares optional deps as fields or setters (setters recommended)

## 2) Write specs

- `specs/<name>.inject.json` per service
- `specs/graph.json` per app graph

## 3) Add `go:generate` lines

In each service owner file:

```go
//go:generate go run ../../cmd/di2 -spec specs/core.inject.json -out core_v4.gen.go
```

In a package-level file for the graph:

```go
//go:generate go run ../../cmd/di2 -graph specs/graph.json -out graph_v4.gen.go
```

## 4) Generate

```bash
go generate ./...
```

## 5) Wire in main (two options)

### Option A — Graph wiring (recommended)

```go
cfg, _ := config.LoadFromEnv()

reg := di.NewMapRegistryV2().
  Provide("v4.tracer", v4.NewPrintTracer()).
  Provide("v4.metrics", v4.NewCounterMetrics())

app, err := v4.BuildAppV4(cfg, reg)
if err != nil { panic(err) }

resp, err := app.Core.Process(context.Background(), v4.ProcessRequest{OrderID: "o-1"})
```

### Option B — Manual wiring (tests / small graph)

```go
alphaB := v4.NewAlphaV4(cfg)
betaB  := v4.NewBetaV4(cfg)
coreB  := v4.NewCoreV4(cfg)

alphaB.InjectBeta(betaB.UnsafeImpl())
betaB.InjectAlpha(alphaB.UnsafeImpl())

coreB.InjectAlpha(alphaB.UnsafeImpl())
coreB.InjectBeta(betaB.UnsafeImpl())

alpha := alphaB.MustBuild()
_ = betaB.MustBuild()
core, err := coreB.BuildWith(reg)
```

---

## Import resolution (v4)

v4 is designed to work well when you generate code **inside a consuming project**:

- generated files import **project-local packages** using the project’s module path
- generated files import the DI runtime (`di.Registry`) using the DI library module path

Practically:
- service/config import paths come from the **owner Go file** imports
- DI runtime import path comes from the module that provides the DI runtime package

---

## Examples

A complete working example exists under:

- `odi/examples/v4/`
  - `config/config.go`
  - `specs/*.inject.json`
  - `specs/graph.json`
  - `optional_deps.go`
  - `alpha_service.go`, `beta_service.go`, `core_service.go`
  - `main/main.go`

Run:

```bash
go generate ./...
go run ./odi/examples/v4/main
```

---

## Testing

Run all tests:

```bash
go test ./...
```

To benchmark generator/runtime pieces (if you have benches):

```bash
go test -bench=. -benchmem ./...
```

---

## Summary

v4 gives you:

- explicit DI
- per-service builders with required dep validation
- optional deps via a tiny registry
- graph composition roots for whole-app wiring
- safe cycle wiring tools (`UnsafeImpl()`)

Still no container, no reflection, no magic — just better ergonomics for real apps.
