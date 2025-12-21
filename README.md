# odi
[![CI](https://github.com/sghaida/odi/actions/workflows/ci.yml/badge.svg)](https://github.com/sghaida/odi/actions/workflows/ci.yml)
[![Codacy Badge](https://app.codacy.com/project/badge/Grade/b210465d4ee74204a513e84aff012bdd)](https://app.codacy.com/gh/sghaida/odi/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_grade)
[![Codacy Badge](https://app.codacy.com/project/badge/Coverage/b210465d4ee74204a513e84aff012bdd)](https://app.codacy.com/gh/sghaida/odi/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_coverage)

Opinionated DI micro framework

# odi — Dependency Injection approaches (v1 → v4)

This repo explores a progression of **explicit** dependency injection patterns in Go — from a tiny runtime helper (v1), to pure manual wiring (v2), to code-generated builders (v3), and finally to code-generated builders **plus** composition roots and optional-deps registry (v4).

Each version is intentionally small and opinionated. Pick the one that matches your constraints.

---

## Quick navigation

### Versions (docs)

- **v1 — tiny generic DI helper (runtime injectors + dep bag)**
    - [Docs](docs/service-v1.md)
    - [Examples](examples/v1)

- **v2 — minimal construction only (no injectors, no dep tracking)**
    - [Docs](docs/service-v2.md)
    - [Examples](examples/v2)

- **v3 — code-generated facades/builders (di1)**
    - [Docs](docs/service-v3.md)
    - [Examples](examples/v3)

- **v4 — code-generated facades + graph composition roots (di2)**
    - [Docs](docs/service-v4.md)
    - [Examples](examples/v4)

> Tip: start with the examples first — they show the wiring style end-to-end.

---

## Which version should I use?

### v1 — Runtime injectors + dependency introspection
Use v1 when you want **explicit wiring** but also want guardrails and test-friendly introspection.

Best for:
- explicit wiring in `main`
- typed-ish errors for missing/wrong-type deps
- validating wiring in tests (`Has`, `GetAny`, `TryGetAs`, etc.)
- seeing “what was injected” via a dep bag

Docs & details:
- [Docs](docs/service-v1.md)
- Example walkthrough: [Examples](examples/v1)

---

### v2 — Construction only (manual wiring)
Use v2 when you want the simplest possible approach: constructors + plain struct wiring.

Best for:
- small services / CLIs / demo code
- codebases where Go constructors are enough
- zero DI concepts leaking into your service design

Docs & details:
- [Docs](docs/service-v2.md)
- Example walkthrough: [Examples](examples/v2)

---

### v3 — Codegen builders (di1)
Use v3 when manual wiring is getting noisy, but you still want wiring to stay **explicit** (no containers).

Best for:
- “explicit DI” with less boilerplate in `main`
- build-time validation that required deps were injected (`Build`)
- ergonomic, generated `InjectX(...)` methods
- teams that want a repeatable pattern across services

Docs & details:
- [Docs](docs/service-v3.md)
- Example walkthrough: [Examples](examples/v3)

---

### v4 — Codegen builders + registry + generated composition root (di2)
Use v4 when your app has many services and you want:
- clean composition root (generated graph function)
- optional dependencies via a small registry (`BuildWith(reg)`)
- per-method “requires” wrappers for safer calling
- explicit cycle wiring tools (`UnsafeImpl()`), without containers

Docs & details:
- [Docs](docs/service-v4.md)
- Example walkthrough: [Examples](examples/v4)

---


## Suggested reading order

If you’re new to the repo:

- Start with **v2** examples to get the baseline “Go constructors only”.
- Move to **v1** if you want runtime guardrails and dependency introspection.
- Move to **v3** if you want fewer wiring lines and can accept code generation.
- Move to **v4** if you want a scalable composition root + optional deps via a registry.

---

