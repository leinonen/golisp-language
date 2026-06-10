# ADR-011: Truthiness and absorbing `any`-type constraints

**Status**: Accepted (amends ADR-007)

## Context

ADR-007 chose `any` as the universal runtime type and documented a table of
"known pain points": situations where natural glisp code emits Go that does
not compile, because the `any` seam leaks through (`if` on a non-bool, `len`
on `any`, a `go`/`select!` in tail position leaving a missing return). Each
row had a documented workaround — `(not= x nil)`, `(len (str w))`, a trailing
`nil` — that the *user* had to know and apply.

For a language whose promise is "get things done easily, and programs are
robust", every such workaround is a broken promise: the user writes
reasonable glisp and receives a **Go** compile error about code they never
wrote. The pain table was treated as documentation; this ADR re-classifies it
as a transpiler defect list.

## Decision

**The user should never have to debug generated Go.** Every entry in the
`any`-constraint table must be resolved in one of two ways, in order of
preference:

1. **Absorbed** — the transpiler emits code that just works (a runtime helper
   or smarter emission).
2. **Diagnosed** — when absorption is impossible, the transpiler reports a
   *glisp-level* error at the `.glsp` source position, with the fix in the
   message.

A documented workaround is not an acceptable steady state.

### Mechanisms (implemented with this ADR)

**Truthiness.** glisp adopts Clojure's truthiness: `nil` and `false` are
falsy, every other value is truthy. Conditions in `if`, `when`, `cond`,
loop-tail `if`/`cond`, `and`, `or`, `not`, `assert-true`, and `assert-false`
are emitted through `emitCondition`: expressions statically known to be Go
`bool` (comparisons, logic ops, bool-returning built-ins, user fns declared
`-> bool`) emit unchanged; everything else is wrapped in the always-present
runtime helper `_glispTruthy(v)` (`v != nil && v != false`). Consequences:

- `(if (get m "k") ...)`, `(when user ...)` work directly on map lookups and
  other `any` values — no more `(not= x nil)` boilerplate.
- `(and a b)` / `(or a b)` accept `any` operands (they still *return* Go
  `bool`, not the last value — Clojure's value-returning `and`/`or` is
  explicitly not adopted).
- `ok` from `(recv-ok! ch)` is usable directly: `(if ok ...)` — the
  `(= ok true)` workaround is obsolete.
- `false` stored in data is correctly falsy, matching Clojure intuition.

**Universal `len`/`count`.** `(len x)` is now an alias for `(count x)`; both
emit `_glispLen(x)`, which accepts `any` and covers strings, `[]any`,
concrete slices (`[]string`, `[]int`, `[]float64`, `[]map[string]any`), maps,
and sets. `(len (str w))` and count-via-reduce workarounds are obsolete.

**Statement-only tails auto-return.** Statement-only forms (`go`, `select!`,
`par`, `for-chan`, `fan-out`, `defer`, `send!`, `close!`) in the tail
position of a value-returning function now emit the statement followed by
`return nil`. The trailing-`nil` workaround is obsolete. (Functions declared
`-> void` are unaffected — their bodies never emit in return position.)

### Remaining rows (future work, same principle)

- Multi-return Go call (`(T, error)`) as the tail of a `func(...) any`
  closure — absorb by detecting multi-return built-ins in tail position and
  emitting the call + `return nil`, or diagnose with the `(do ... nil)` fix.
- Concrete slices (`[]T` from Go interop) passed to `reduce`/`map`/`filter` —
  extend `_glispToSlice` over common concrete element types.
- When a Go build error does leak through, map it back to the `.glsp`
  file/line wherever possible.

## Reasons

- **Fun** — the feedback loop survives. Each absorbed row deletes a class of
  "why doesn't this compile" moments that disproportionately hit newcomers.
- **Robust** — truthiness is *defined semantics* instead of an emergent
  property of what Go happens to accept; `_glispTruthy` behaves identically
  everywhere a condition appears.
- **Small** — the fix is one tiny runtime helper and emission changes; no
  type inference engine (ADR-007's core bet is unchanged — `any` stays the
  universal runtime type, helpers stay `any → any`).

## Consequences

- `(= x nil)` / `(not= x nil)` still work and remain idiomatic when nil-ness
  (not truthiness) is specifically meant, e.g. distinguishing `false` from
  "absent".
- `if-let` / `when-let` / `let-or` keep their **nil-guard** (`!= nil`)
  semantics: they bind-and-test presence, so a bound `false` is still a
  value. This is a deliberate difference from bare `if` truthiness.
- A statically-bool condition emits identical Go as before; only `any`-typed
  conditions gain the `_glispTruthy` wrapper (negligible runtime cost).
- Generated code for non-bool conditions is less "Go-natural" to read — an
  acceptable cost, since the whole point is that users stop reading it.
- The pain-point table in CLAUDE.md shrinks to the genuinely unresolved rows.
