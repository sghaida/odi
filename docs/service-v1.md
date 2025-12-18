# odi/di — tiny generic dependency injection helper (Go)

`di` is a lightweight, explicit wiring helper built around one idea:

- **Build a value** (`Val`)
- **Record what was injected** (`Deps`)
- **Wire dependencies explicitly** using small injector functions (`Injector[T]`)

No container graphs, no reflection-based injection, no runtime magic.

---

## When should you use this?

Use `di` when you want:

- **Explicit wiring** in `main` / bootstrap code (easy to read, easy to diff, easy to review).
- **Small surface area** (no lifecycle hooks, modules, or container graph).
- **Test-friendly construction**: build real services, wire a few deps, and introspect what was injected via `Deps`.
- **Fast success path**: in the common case it’s essentially a map write + a function call.
- **Typed-ish failures**: duplicate key / missing dep / wrong type errors are structured and easy to assert in tests.

Avoid it when you need:

- Automatic graph resolution across many modules/packages
- Lifecycle management (start/stop hooks)
- Advanced scoping (request scopes, per-goroutine scopes, etc.)
- Code generation / compile-time wiring (e.g., Wire)

---

## Install / import

```go
import "github.com/sghaida/odi/di"
```

---

## Key concepts

### Service[T]

A `Service[T]` holds:

- `Val`: your constructed `*T`
- `Deps`: a dependency “bag” storing injected values by key (for introspection/debugging/tests)

```go
type Service[T any] struct {
  Val  *T
  Deps map[DependencyKey]any
}
```

### DependencyKey

A strongly-typed key used to identify stored dependencies.

```go
type DependencyKey string
func Key(name string) DependencyKey
```

Typical usage is package-level constants to avoid typos:

```go
const (
  KeyDB     di.DependencyKey = "db"
  KeyLogger di.DependencyKey = "logger"
)
```

---

## Functionality guide (what it does + when to use it)

### 1) `Key(name string) DependencyKey`

**What it does:** converts a string to `DependencyKey`.

**When to use it:**
- When defining keys inline in examples/tests (`di.Key("db")`)
- Prefer constants for production wiring, but `Key()` is handy for readability and quick setups.

---

### 2) `Init[T](ctor func() *T) *Service[T]`

**What it does:**
- Calls `ctor()` to build your service value (`Val`)
- Initializes `Deps` to an empty map

**When to use it:**
- In `main` / bootstrap code to construct services and dependencies.
- In tests to construct “real-ish” services with explicit injected deps.
- Anytime you want a consistent pattern to create and wire `*T`.

**Example:**
```go
db := di.Init(func() *DB { return &DB{DSN: "postgres://prod"} })
userSvc := di.Init(func() *UserService { return &UserService{} })
```

---

### 3) `(*Service[T]).Value() *T`

**What it does:** returns the constructed value pointer (`Val`).

**When to use it:**
- After wiring, to call the actual service methods.
- When you need to pass the underlying service implementation to something else.

**Example:**
```go
u := userSvc.Value()
_ = u.PlaceOrder("user-123")
```

---

### 4) `Injector[T]`

```go
type Injector[T any] func(*Service[T]) error
```

**What it is:** a small function that mutates the target `Service[T]` in-place (wires deps) and may return an error.

**When to use it:**
- As the standard unit of wiring logic you compose with `With` / `WithAll`.
- To create reusable wiring fragments (e.g., `commonDeps`).

---

### 5) `(*Service[T]).With(inj Injector[T]) (*Service[T], error)`

**What it does:**
- Applies one injector to the service.
- If `inj` is `nil`, it is a no-op and returns `(s, nil)`.

**When to use it:**
- When you’re applying a single dependency.
- When you want to conditionally apply an injector (nil injector becomes a nice “optional” pattern).

**Example:**
```go
_, err := userSvc.With(di.Injecting(KeyDB, db, func(u *UserService, d *DB) { u.DB = d }))
```

---

### 6) `(*Service[T]).WithAll(deps ...Injector[T]) (*Service[T], error)`

**What it does:**
- Applies multiple injectors in order.
- Stops at the first error and returns it.

**When to use it:**
- In `main` wiring code to attach multiple dependencies in one place.
- When wiring order matters or when you want the “fail fast” behavior.

**Example:**
```go
_, err := userSvc.WithAll(
  di.Injecting(KeyDB, db, func(u *UserService, d *DB) { u.DB = d }),
  di.Injecting(KeyLogger, log, func(u *UserService, l *Logger) { u.Logger = l }),
)
```

---

### 7) `Injecting(key, dep, bind) Injector[T]`

```go
func Injecting[T any, D any](
  key DependencyKey,
  dep *Service[D],
  bind func(target *T, dependency *D),
) Injector[T]
```

**What it does (success path):**
1. Ensures the target service is not nil
2. Ensures the dependency service is not nil
3. Ensures `bind` is not nil
4. Ensures `Deps` map exists
5. Ensures the key is not already used
6. Stores `dep.Val` under `Deps[key]`
7. Calls `bind(target.Val, dep.Val)` to attach it

**When to use it:**
- This is the primary way to wire one dependency into another, explicitly.
- Use it for both concrete deps (e.g., `*DB`, `*Logger`) and for interface values (see below).

**Common patterns:**
- **Concrete dependency**
  ```go
  di.Injecting(KeyDB, dbSvc, func(u *UserService, d *DB) { u.DB = d })
  ```
- **Interface dependency** (to break cycles):
  build a `*Service[SomeInterface]` whose `Val` is a pointer to an interface value, then inject it.

---

### 8) `(*Service[T]).Has(key DependencyKey) bool`

**What it does:** returns whether `Deps[key]` exists (regardless of type).

**When to use it:**
- In tests to assert a dependency was wired.
- In debug logging or diagnostics.

**Example:**
```go
if userSvc.Has(KeyDB) { ... }
```

---

### 9) `(*Service[T]).GetAny(key DependencyKey) (any, bool)`

**What it does:** returns the raw stored dependency value without type assertions.

**When to use it:**
- In tests or debug output to inspect what’s stored.
- When you intentionally want “untyped” access.

**Example:**
```go
raw, ok := userSvc.GetAny(KeyDB)
fmt.Printf("stored type=%T ok=%v\n", raw, ok)
```

---

### 10) `GetAs[T, D](s, key) (*D, bool)`

**What it does:**
- Looks up `Deps[key]`
- Returns `(*D, true)` if the stored value is a `*D`, otherwise `(nil, false)`

**When to use it:**
- When missing or wrong type is *not exceptional* and you just want an `ok` boolean.
- Common in tests or optional dependency scenarios.

**Example:**
```go
db, ok := di.GetAs[UserService, DB](userSvc, KeyDB)
```

---

### 11) `TryGetAs[T, D](s, key) (*D, error)`

**What it does:**
- Like `GetAs`, but returns structured errors:
    - `MissingDependencyError` if key missing (or deps not present)
    - `WrongTypeDependencyError` if present but not `*D`

**When to use it:**
- When you want to distinguish “missing” vs “wrong type”.
- When errors are part of control flow (still lightweight; avoids `fmt.Errorf` formatting in the hot-ish path).

**Example:**
```go
db, err := di.TryGetAs[UserService, DB](userSvc, KeyDB)
```

---

### 12) `MustGetAs[T, D](s, key) *D`

**What it does:**
- Returns the dependency as `*D`
- Panics if missing or wrong type

**When to use it:**
- In tests, or when wiring is guaranteed and panic is acceptable.
- Avoid in long-running production paths unless you truly want a hard fail.

**Example:**
```go
db := di.MustGetAs[UserService, DB](userSvc, KeyDB)
```

---

### 13) `(*Service[T]).Clone() *Service[T]`

**What it does:**
- Returns a shallow copy:
    - shares `Val` pointer (same constructed service)
    - copies the `Deps` map into a new map (so later wiring doesn’t mutate the original bag)

**When to use it:**
- When you want to keep the same service instance but experiment with wiring metadata separately.
- In tests when you want to branch wiring scenarios without mutating the original service’s `Deps`.

---

## Errors (what they mean)

- `ErrNilTarget` — injector applied to nil target service or `Val == nil`
- `NilDependencyServiceError{Key}` — dependency service is nil (or has nil Val) for this key
- `NilBindError{Key}` — bind function is nil for this key
- `DuplicateKeyError{Key}` — key already exists in `Deps`
- `MissingDependencyError{Key}` — `TryGetAs` cannot find key
- `WrongTypeDependencyError{Key, GotType}` — `TryGetAs` found key but type is not `*D`

---

## Examples

A complete working showcase is in:

- `examples/v1/models.go`
- `examples/v1/services.go`
- `examples/v1/main.go`

Run it:

```bash
go run ./examples/v1
```

It demonstrates:

- wiring concrete deps (`DB`, `Logger`)
- wiring interface deps (to break mutual dependency cycles)
- calling service methods
- typed retrieval (`GetAs` / `TryGetAs` / `MustGetAs`)
- dependency introspection (`Has` / `GetAny`)
- cloning (`Clone`)
- error cases (duplicate keys, missing deps, wrong type)

---

## Testing / benchmarking

Run tests:

```bash
go test ./...
```

Run benchmarks:

```bash
go test -bench=. -benchmem ./di
```
