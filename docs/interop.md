# Go interop

glisp is a Lisp **hosted on Go** the way Clojure is hosted on the JVM (ADR-016).
You write glisp against its own standard library (`core` — see
[`builtins.md`](builtins.md) and [`stdlib.md`](stdlib.md)); Go is the host
platform underneath, and **interop** is the deliberate, first-class escape hatch
for reaching into it.

> **The boundary:** reach for interop when you need a Go library that `core`
> doesn't cover — a database driver, a vendor SDK, a niche stdlib corner.
> Otherwise stay in `core`. Interop is *good* and supported (it's how you get the
> whole Go ecosystem), but it is interop, not the everyday surface — just like
> Java interop in Clojure.

Nothing here is needed for the standard library: every stdlib package
auto-imports from the symbols you reference (`math/pi`, `strconv/atoi`, …). This
guide is about **external Go packages** and **Go values**.

## Pulling in a Go package

A Go package dependency is declared in `glisp.mod` under `go-require`, then
imported in the `ns` form with `:import` (glisp modules use `:require`; stdlib
needs neither):

```
; glisp.mod
module github.com/you/app

go-require (
  github.com/jackc/pgx/v5 v5.7.2
)
```

```clojure
(ns db
  (:import [github.com/jackc/pgx/v5]))          ; qualifier = last path segment → pgx
  ; or alias it: (:import [github.com/jackc/pgx/v5 :as pg])
```

`glisp get github.com/jackc/pgx/v5@v5.7.2` fetches it and wires both `glisp.mod`
and `go.mod`. The call-site qualifier is the last path segment (`pgx`), unless
you give an explicit `:as`.

## Calling Go functions

A package-qualified `pkg/fn` symbol calls a Go function. Names are converted
from kebab-case to the Go name (`pgx/connect` → `pgx.Connect`,
`strings/to-upper` → `strings.ToUpper`); a name with any uppercase passes
through as written.

```clojure
(strings/to-upper "hello")                       ; → strings.ToUpper("hello")
(fmt/printf "%s=%d\n" k & vs)                     ; & spreads a slice into Go's variadic tail
```

When the package is loaded (it is whenever you `:import` it), the transpiler
reads its real signatures from the Go toolchain (ADR-015) and uses them:

- **Argument coercion** — an `any`-typed argument at a typed parameter is
  coerced at the call site (`strings.ToUpper(_glispToString(x))`), so values
  pulled from maps/JSON flow into typed Go APIs without manual casts.
- **Arity diagnostics** — a wrong argument count is a position-tagged glisp
  error (`arity error: pgx/query called with 1 arg(s), expected at least 2`)
  instead of an opaque Go compile error. Variadic- and spread-aware.

An *unloaded* package (offline, build error in the dep) degrades silently to
untyped emission — interop is an enhancement, never a build dependency.

## Working with Go values

The big win: a value whose **Go type is statically known** dispatches its
methods and fields *dot-free*, no interop punctuation required. The type is
known three ways:

1. a **type annotation** — `[c *pgx/Conn]`, `(let [c *pgx/Conn …] …)`;
2. a **typed return** from a loaded interop function — `(pkg/new …)`;
3. a **chained result** — the return of a method/field that is itself such a type.

```clojure
(defn run-query [c *pgx/Conn sql string] -> any
  (query c sql))                                 ; → c.Query(sql)   (dot-free dispatch)
```

```clojure
; net/url.URL — a stdlib struct, runnable today
(defn describe [u *url/URL] -> string
  (str (:scheme u) "://" (.-Host u) (:path u)))  ; → u.Scheme, u.Host, u.Path
```

- **Methods** dispatch dot-free: `(query c sql)` → `c.Query(sql)`, with the Go
  parameter types threaded to the arguments. A call naming no method of the type
  is a position-tagged error: `type pgx.Conn has no exported method Quary`.
- **Fields** read dot-free in both spellings — the keyword form `(:scheme u)`
  (uniform with `core` structs) and the interop accessor `(.-Scheme u)` — both
  emit `u.Scheme`. A non-existent field is an error: `type url.URL has no
  exported field Schema`.

### The interop primitives

These stay available for any value the transpiler can't type (a bare `any`, a
result it can't infer) — glisp's analog of Clojure's `(.method)` / `(Foo.)` /
`(cast)`. They are good; they just aren't required once a value is typed.

| Form | Emits | Use |
|---|---|---|
| `(.Method o args)` | `o.Method(args)` | method call on an untyped/`any` value |
| `(.-Field o)` | `o.Field` | field read |
| `(Type. {:f v})` | `Type{F: v}` | struct literal (`(pgx/Conn. {…})`) |
| `(as T v)` | `v.(T)` | type assertion — give an `any` a concrete type |
| `(f a & xs)` | `f(a, xs...)` | spread a slice into a Go variadic parameter |

The common pattern for an opaque handle is to pass it as `any` and assert when
you need its methods:

```clojure
(defn exec [conn any sql string] -> [any error]
  (let [c (as *pgx/Conn conn)]                   ; now c is typed
    (.Exec c (ctx/background) sql)))             ; …or call (exec c …) dot-free
```

## Limitations

A few interop edges are not yet absorbed (tracked in
[`ROADMAP.md`](../ROADMAP.md), Phase 15):

- A multi-return Go **method** used dot-free in single-value position *is* now a
  glisp diagnostic (like multi-return functions) — bind it with `if-err`, or use
  the `(.Method …)` form. A **wrong-kind literal argument** to a loaded `pkg/fn`
  (e.g. a string where a number is wanted) is flagged too.
- An **`if-err`-bound multi-return interop result** is Go-typed but its type
  isn't recorded, so it doesn't yet dispatch dot-free — bind it through a typed
  function parameter (or `(as T …)`) to dispatch on it.
- **Non-literal** wrong-typed args aren't diagnosed at the glisp level (only
  literals are checked, to stay false-positive-free); the Go compiler reports
  them, with `.glsp` line mapping.
- An external type used *only* via a `[c *pkg/T]` annotation (no `pkg/fn` call)
  needs its package `:import`ed to be loaded for dot-free dispatch.

## Wrapping a Go package as a glisp module

Publishing a reusable wrapper is pure glisp — a module with `.glsp` files, a
`glisp.mod` declaring the `go-require`, and PascalCase exported names. See the
design charter (ADR-012) and the module section of the project README/CLAUDE for
the full convention; the rules above (return opaque types as `any`, assert with
`(as T v)` when methods are needed) carry over unchanged.
