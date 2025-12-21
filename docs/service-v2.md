# odi — v2 minimal dependency construction (Go)

Version **v2** of `odi/di` is an intentionally **minimal** approach to dependency
construction in Go.

It removes *all* dependency tracking and injectors from v1 and keeps only the
absolute core idea:

> **Construct values explicitly and wire them manually.**

No keys, no injectors, no dependency bag, no runtime validation.

---

## When should you use v2?

Use **v2** when you want:

- **Maximum simplicity** — just constructors and structs
- **Zero magic** — no hidden wiring or container state
- **Explicit order** — you control construction order in `main`
- **Library-friendly code** — no dependency on DI concepts inside services
- **Easy examples & tutorials**
- **Fast startup** — nothing happens except allocations

Typical use cases:

- Small services
- CLI tools
- Examples / demos
- Libraries
- When Go’s normal constructor pattern is enough

---

## When NOT to use v2

Avoid v2 if you need:

- Dependency introspection (what was injected?)
- Duplicate dependency detection
- Optional / conditional wiring helpers
- Typed errors for missing dependencies
- Runtime validation of wiring
- Test assertions around injected deps

In those cases, use **v1**.

---

## Core API

### ServiceV2[T]

```go
type ServiceV2[T any] struct {
    Val *T
}
```

This is just a **thin wrapper** around a constructed value.

There is **no dependency state**.

---

### New[T](ctor func() *T) ServiceV2[T]

```go
func New[T any](ctor func() *T) ServiceV2[T]
```

**What it does:**

- Calls `ctor`
- Stores the result in `Val`
- Returns a value (not a pointer)

**Example:**

```go
db := di.New(func() *DB {
    return &DB{DSN: "postgres://prod"}
})
```

---

## How wiring works in v2

There is **no automatic wiring**.

You wire dependencies by **assigning fields directly**.

This is intentional.

---

## Example project structure

```text
examples/v2/
  models.go
  services.go
  main.go
```

---

## Example: Models

```go
package main

type DB struct {
    DSN string
}

type Logger struct {
    Level string
}

type BasketItem struct {
    SKU   string
    Qty   int
    Price int
}

type Basket struct {
    UserID string
    Items  []BasketItem
}
```

---

## Example: Services

```go
package main

import "fmt"

type BasketService struct {
    DB     *DB
    Logger *Logger
}

func (b *BasketService) GetBasket(userID string) (*Basket, error) {
    if b.DB == nil || b.Logger == nil {
        return nil, fmt.Errorf("basket: missing deps")
    }
    return &Basket{
        UserID: userID,
        Items: []BasketItem{
            {SKU: "apple", Qty: 2, Price: 3},
            {SKU: "banana", Qty: 5, Price: 1},
        },
    }, nil
}

type UserService struct {
    DB     *DB
    Logger *Logger
    Basket *BasketService
}

func (u *UserService) GetUserBasket(userID string) (*Basket, error) {
    if u.Basket == nil {
        return nil, fmt.Errorf("user: missing basket service")
    }
    return u.Basket.GetBasket(userID)
}
```

---

## Example: main.go (explicit wiring)

```go
package main

import (
    "log"

    "github.com/sghaida/odi/di"
)

func main() {
    db := di.New(func() *DB {
        return &DB{DSN: "postgres://prod"}
    })

    logger := di.New(func() *Logger {
        return &Logger{Level: "info"}
    })

    basketSvc := di.New(func() *BasketService {
        return &BasketService{
            DB:     db.Val,
            Logger: logger.Val,
        }
    })

    userSvc := di.New(func() *UserService {
        return &UserService{
            DB:     db.Val,
            Logger: logger.Val,
            Basket: basketSvc.Val,
        }
    })

    basket, err := userSvc.Val.GetUserBasket("user-123")
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("basket: %+v", basket)
}
```

---

## v1 vs v2 comparison

| Feature                  | v1  | v2      |
|--------------------------|-----|---------|
| Injectors                | ✅   | ❌       |
| Dependency keys          | ✅   | ❌       |
| Dependency introspection | ✅   | ❌       |
| Typed DI errors          | ✅   | ❌       |
| Clone / Has / GetAs      | ✅   | ❌       |
| Manual wiring            | ⚠️  | ✅       |
| Simplicity               | ⚠️  | ✅       |
| Runtime cost             | Low | Minimal |

---

## Rule of thumb

> **If you want guardrails → v1**  
> **If you want simplicity → v2**

Both versions are intentionally supported.
