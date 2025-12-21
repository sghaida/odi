# odi — v3 code-generated facades (di1) for explicit injection (Go)

Version **v3** introduces **code generation** (`cmd/di1`) to keep wiring **explicit** while adding
**compile-time ergonomics**:

- You write a tiny `*.inject.json` spec next to your service.
- You add a `//go:generate ...` directive in the **owner** Go file.
- `di1` generates a **facade/builder** with:
    - `Inject<Name>(dep)` methods for **required** dependencies
    - a generic `Inject(fn)` hook for **custom / optional** wiring
    - `Build()` validation and `MustBuild()` convenience

There is **no container**, **no reflection wiring**, **no module graphs**.

---

## When should you use v3?

Use **v3** when you want:

- **Explicit wiring** in `main`/bootstrap, but with **less boilerplate** than v1/v2.
- **Build-time guardrails**: “required deps must be wired” is enforced by `Build()`.
- **Ergonomic injection API**: `InjectDB(...)`, `InjectCache(...)`, etc.
- A clear separation between:
    - **construction** (constructor)
    - **wiring** (injectors/setters)
    - **validation** (Build/MustBuild)
- A repeatable pattern across many services/packages.

Typical use cases:

- Medium/large codebases where manual wiring becomes noisy
- Teams that want “explicit DI” without adopting a full container
- Services that have a stable constructor but evolving dependency sets

---

## When NOT to use v3

Avoid v3 if you need:

- Automatic graph resolution across many modules/packages
- Lifecycle management (start/stop hooks, modules, scopes)
- Advanced scoping (request scopes, per-goroutine scopes)
- Pure compile-time graph generation like Wire (end-to-end graph)
- No codegen in the repo/tooling policy

In those cases consider:
- **Wire** (compile-time whole-graph)
- **fx/dig** (runtime container + lifecycle)

---

## How v3 fits next to v1 and v2

| Feature / Style                | v1 (`di.Service[T]`) | v2 (construct-only) | v3 (`di1` codegen) |
|--------------------------------|----------------------|---------------------|--------------------|
| Code generation                | ❌                    | ❌                   | ✅                  |
| Injectors helpers              | ✅                    | ❌                   | ✅ (generated)      |
| Required dependency validation | ✅                    | ❌                   | ✅ (`Build`)        |
| Dependency bag / introspection | ✅                    | ❌                   | ❌                  |
| Manual wiring                  | ✅                    | ✅                   | ✅                  |
| Boilerplate in `main`          | Medium               | Low                 | Low                |
| Container / graph resolution   | ❌                    | ❌                   | ❌                  |

Rule of thumb:

> **Need introspection & typed retrieval → v1**  
> **Need maximum simplicity → v2**  
> **Need explicit DI + less wiring boilerplate → v3**

---

## Core idea

v3 generates a **builder** (facade) around a concrete implementation.

Generated facade responsibilities:

- Construct the service (`New<Facade>(cfg)` calls your constructor)
- Track which **required** deps were injected (`hasX` booleans)
- Provide explicit `InjectX(...)` methods
- Validate wiring at `Build()` time

Your service responsibilities:

- Provide a constructor (optionally taking `config.Config`)
- Expose setters for required/optional deps **or** use fields directly if in same package
- Implement interfaces that other services depend on (to break cycles)

---

## Generated API (what you get)

Assuming a facade named `UserSvcV3` for `ImplType: UserSvc`:

### `NewUserSvcV3(cfg config.Config) *UserSvcV3`

**What it does**
- Creates an internal `*UserSvc` via your constructor
- Returns the facade (builder)

**When to use**
- In `main` / bootstrap where you wire dependencies

---

### `Inject<Name>(dep <Type>) *UserSvcV3`

Generated for each **required** dependency in the spec.

**What it does**
- Assigns the dependency into the service (usually to a field)
- Marks that dependency as present (`has<Name> = true`)
- Returns the builder for chaining

**When to use**
- For required deps that must be present before `Build()`

**Example**
```go
builder := v3.NewUserSvcV3(cfg).
  InjectDB(db).
  InjectLogger(logger)
```

---

### `Inject(fn func(*UserSvc)) *UserSvcV3`

**What it does**
- Runs an arbitrary function against the underlying service pointer
- Designed for:
    - optional deps (logger, metrics)
    - calling exported setters
    - one-off wiring tweaks
    - capturing the underlying pointer for cycle wiring patterns

**When to use**
- Optional dependencies
- Non-field wiring patterns (setters, closures, derived deps)

**Example**
```go
builder.Inject(func(s *v3.UserSvc) {
  s.SetLogger(log)     // optional
  s.SetTimeout(2500)   // derived config
})
```

---

### `Build() (*UserSvc, error)`

**What it does**
- Verifies all required deps were injected
- Returns the underlying service pointer if valid

**When to use**
- Prefer in production wiring when you want **error propagation**

**Example**
```go
svc, err := builder.Build()
if err != nil {
  return err
}
```

---

### `MustBuild() *UserSvc`

**What it does**
- Calls `Build()`
- Panics on error

**When to use**
- Simple examples, CLIs, tests, or cases where wiring failure should hard-fail.

---

## The spec file (`*.inject.json`)

A minimal spec looks like:

```json
{
  "package": "v3",
  "wrapperBase": "FraudSvc",
  "versionSuffix": "V3",
  "implType": "FraudSvc",
  "constructor": "NewFraudSvc",
  "imports": {
    "config": "github.com/sghaida/odi/examples/v3/config"
  },
  "required": [
    { "name": "TransactionGetter", "field": "txGetter", "type": "TransactionGetter" },
    { "name": "DecisionWriter", "field": "writer", "type": "DecisionWriter" }
  ],
  "optional": [
    { "name": "Logger", "field": "logger", "type": "Logger" }
  ]
}
```

### Fields explained

- `package`: Go package of the generated file
- `wrapperBase` + `versionSuffix`: default facade name (`FraudSvc` + `V3` => `FraudSvcV3`)
- `facadeName` (optional): override the generated facade name
- `implType`: concrete service type you’re wrapping (e.g., `FraudSvc`)
- `constructor`: the function used to create the concrete service
- `imports.config` (optional/fallback): config package import path when constructor requires `config.Config`
- `required`: list of required deps; each generates an `Inject<Name>` method
- `optional`: list of optional deps; validated for uniqueness but not required by `Build()`

> **Important:** v3 uses `name` for method generation (`Inject<Name>`) and tracks presence via `has<Name>`.

---

## How to wire: step-by-step

### Step 1 — Write the service

Your service should:
- define interfaces it depends on (required deps)
- optionally define setters for required/optional deps
- expose the constructor mentioned in the spec

Example (simplified):

```go
type Logger interface { Infof(string, ...any) }

type TransactionGetter interface { GetTransaction(id string) (*Transaction, error) }
type DecisionWriter interface { WriteDecision(txID string, decision Decision) error }

type FraudSvc struct {
  txGetter TransactionGetter
  writer   DecisionWriter
  logger   Logger // optional
}

func NewFraudSvc(cfg config.Config) *FraudSvc {
  _ = cfg
  return &FraudSvc{}
}

func (f *FraudSvc) SetTransactionGetter(g TransactionGetter) { f.txGetter = g }
func (f *FraudSvc) SetDecisionWriter(w DecisionWriter)       { f.writer = w }
func (f *FraudSvc) SetLogger(l Logger)                       { f.logger = l }
```

---

### Step 2 — Add `go:generate`

Put this in the owner Go file (same package directory as the spec):

```go
//go:generate go run ../../cmd/di1 -spec ./specs/fraud.inject.json -out ./fraud_di.gen.go
```

---

### Step 3 — Generate

```bash
go generate ./...
```

---

### Step 4 — Wire in `main`

```go
cfg := config.Config{Env: "dev"}

fraudB := v3.NewFraudSvcV3(cfg)

fraudB.
  InjectTransactionGetter(txRepo).
  Inject(func(s *v3.FraudSvc) { s.SetLogger(log) }).
  InjectDecisionWriter(decisionSvc)

fraudSvc, err := fraudB.Build()
if err != nil { panic(err) }
```

---

## Handling cycles safely (FraudSvc ↔ DecisionSvc)

Cycles are common when two services depend on each other through **interfaces**.

v3 does not solve cycles automatically; it gives you explicit tools to wire them:

- constructors create concrete objects
- `Inject(fn)` lets you capture pointers and call setters
- required deps still validated at build time

A safe pattern:

1. Create both builders (each creates its underlying service pointer)
2. Capture both underlying pointers using `Inject(fn)`
3. Wire the cycle using setters via `Inject(fn)`
4. Then `Build()` / `MustBuild()`

```go
fraudB := v3.NewFraudSvcV3(cfg)
decisionB := v3.NewDecisionSvcV3(cfg)

var fraudSvc *v3.FraudSvc
var decisionSvc *v3.DecisionSvc

fraudB.Inject(func(s *v3.FraudSvc) { fraudSvc = s })
decisionB.Inject(func(s *v3.DecisionSvc) { decisionSvc = s })

fraudB.Inject(func(s *v3.FraudSvc) { s.SetDecisionWriter(decisionSvc) })
decisionB.Inject(func(s *v3.DecisionSvc) { s.SetFraudChecker(fraudSvc) })

fraudB.InjectTransactionGetter(txRepo)
decisionB.InjectDecisionStore(store)

fraudB.Inject(func(s *v3.FraudSvc) { s.SetLogger(log) })
decisionB.Inject(func(s *v3.DecisionSvc) { s.SetLogger(log) })

fraudSvcFinal := fraudB.MustBuild()
decisionSvcFinal := decisionB.MustBuild()

_ = fraudSvcFinal
_ = decisionSvcFinal
```

**Why this works**
- Builders already hold pointers created by constructors.
- `Inject(fn)` lets you set cross-references before `Build()` validation.
- You avoid recursion by designing one side’s method to be “pure” (as you did with `CheckRisk`).

---

## Imports and `config.Config`

If your constructor takes `config.Config`, generated code must import the config package under alias `config`.

v3 supports:
- **Reading imports from the owner file** (preferred)
- Falling back to `spec.imports.config` if not found

Best practice in the owner file:

```go
import (
  config "github.com/sghaida/odi/examples/v3/config"
)
```

If you import it without alias:

```go
import (
  "github.com/sghaida/odi/examples/v3/config"
)
```

it usually still works if the package name is `config`, but aliasing is safest and avoids ambiguity.

---

## Examples

A complete working example is in:

- `examples/v3/models/`
- `examples/v3/config/`
- `examples/v3/services/`
- `examples/v3/main/`

Run:

```bash
go generate ./...
go run ./examples/v3/main
```

---

## Testing

The generator (`cmd/di1`) is fully unit-testable:
- spec validation
- owner-file detection
- import parsing & resolution
- atomic file writing

Run:

```bash
go test ./...
```
