# ADR-007: `any` as universal runtime type

**Status**: Accepted (amended by ADR-011)

## Context

glisp operates on dynamically-typed values at runtime: map lookups return unknown types, collection elements are untyped, function arguments may be any value. Go requires all values to have concrete types at compile time. The transpiler must choose how to represent untyped runtime values.

## Decision

All dynamically-typed runtime values are `any` (Go's `interface{}`). Runtime helpers (`_glispGet`, `_glispMap`, `_glispReduce`, etc.) accept and return `any`. Type annotations in glisp are pass-through hints for Go's type system, not inference.

## Reasons

- **Simplicity** — no type inference engine needed; the transpiler doesn't need to track types through closures, let bindings, or higher-order functions
- **Correctness** — `any` is always correct; a more specific type could be wrong and cause compile errors
- **Interop** — Go functions return concrete types; `any` accepts them without coercion
- **Precedent** — this is exactly how dynamic languages (Python, Ruby, Clojure on JVM with `Object`) handle mixed-type collections

## Known pain points and mitigations

ADR-011 reclassified this table as a transpiler defect list; absorbed rows are
struck through and kept for history.

| Situation | Problem | Mitigation |
|-----------|---------|------------|
| ~~`(len x)` where x is `any`~~ | ~~`len` needs concrete type~~ | Absorbed (ADR-011): `len` is an alias for `count` → `_glispLen` accepts `any` |
| ~~`(if x ...)` where x is non-bool `any`~~ | ~~Go if requires bool~~ | Absorbed (ADR-011): conditions wrap in `_glispTruthy` — nil/false falsy |
| ~~Known multi-return call as a single value~~ | ~~can't coerce `(T, error)` to `any`~~ | Diagnosed (ADR-011): transpile-time error suggesting `if-err`, for multi-return built-ins and user fns declared `-> [T E]`. Unknown Go interop fns still need `(do (f ...) nil)` (Go error is //line-mapped) |
| `(defn f [] -> int (reduce ...))` | reduce returns `any`, not `int` | Wrap: `(int (reduce ...))` or use `-> any` return |

## Consequences

- Type assertions (`as`) are needed when calling Go APIs that expect concrete types
- Arithmetic on values retrieved from maps may require explicit `(int x)` / `(float64 x)` casts
- The CLAUDE.md documents the common `any`-type constraint patterns for reference
- A future type-inference pass could narrow `any` to concrete types in some positions — but this is not planned
