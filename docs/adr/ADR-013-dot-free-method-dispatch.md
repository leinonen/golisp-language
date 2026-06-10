# ADR-013: Dot-free method dispatch on typed values

**Status**: Accepted

## Context

The `defstruct`/`definterface`/`defmethod` triad produced clean Go, but
*calling* a method required the Go-interop escape hatch:

```clojure
(defn print-shape [s Shape] -> void
  (fmt/printf "%s area=%.4f\n" (.Describe s) (.Area s)))
```

The leading-dot form `(.Method obj)` reads as foreign syntax inside otherwise
Clojure-shaped code, and it is asymmetric with the rest of the gradual-typing
story: `(:radius c)` already upgrades to `c.Radius` when `c` is statically
known to hold a declared struct (ADR-010, `emit_typeinfo.go`), yet the
equivalent call position had no such upgrade. Per the design charter
(ADR-012), typed code should harden *without rewriting its shape* — and the
natural Lisp shape for "area of s" is `(area s)`.

## Decision

`(area s)` emits `s.Area()` when:

1. the head is an unqualified symbol that names no built-in form, no
   user-defined `defn`, and no in-scope value binding (param, receiver,
   `let`/`loop`/`if-let`/`let-or`/destructure binding, or `def` global), and
2. the first argument is statically known — via the local type environment or
   value inference (struct literals, calls with declared return types) — to
   hold a locally-declared struct or interface type, and
3. that type declares a matching method (`defmethod` receivers first, then
   `definterface` signatures).

The method name converts exactly like exported module calls (`fnToGo`):
all-lowercase kebab becomes PascalCase (`describe` → `Describe`,
`to-string` → `ToString`); a name containing any uppercase passes through
as-is; `identToGo` is the fallback so unexported predicate methods resolve
too (`drained?` → `isDrained`).

Resolution order is fixed and shadowing is total: **built-ins > user `defn`s
> in-scope bindings > method dispatch > plain call**. A local closure named
`area` always wins over the method; `(empty? c)` stays the built-in even if
the type declares `Empty?`. Dispatch is a fallback that only fires where the
program would otherwise fail to compile, so no existing program changes
meaning.

Dispatched calls get the same static guarantees as user function calls:
position-tagged arity errors, the multi-return single-value gate
(`-> [T E]` methods must go through `if-err`), `-> bool` methods skip the
`_glispTruthy` wrapper in conditions, and parameter types thread to argument
hints (typed map literals work as method arguments).

## Implementation

`internal/transpiler/emit_methods.go`. The `emitFile` pre-pass collects
`e.ifaces` and `e.methods` tables; `resolveMethodCall` runs in the
`emitCallExpr` fallthrough after built-ins and user functions.
`e.localVars` (every in-scope value binding, scoped with `pushTypeScope`)
provides the shadowing set; `e.localTypes` was widened to record interface
types alongside structs.

## Alternatives considered

- **Keep `(.Method obj)` as the only form.** Rejected: it leaks Go syntax
  into the language's signature idiom and contradicts the keyword-access
  precedent.
- **A reader macro or special form (e.g. `(! area s)`).** Rejected: ADR-012
  demands one obvious way, and the obvious way is the plain call.
- **Dynamic dispatch on `any` values.** Rejected: would need runtime
  reflection, breaking "Go's runtime model, undiluted". `any` values keep
  using `(as T v)` + interop.

## Consequences

- The shapes example and similar typed code contain no interop forms at all.
- `(.Method obj)` remains, and remains necessary, for values the transpiler
  cannot type: `any`, external Go types, and methods declared in *other
  files* of a multi-file build (the pre-pass is per-file).
- Adding a method to a type can, in principle, change a previously-failing
  plain call into a dispatch — never the reverse, and never a call that
  compiled before.
