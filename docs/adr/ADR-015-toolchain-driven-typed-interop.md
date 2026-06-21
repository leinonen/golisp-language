# ADR-015: Toolchain-driven typed interop (read Go signatures, like jank reads C++)

**Status**: Accepted

## Context

glisp's single biggest force multiplier is the Go ecosystem: every Go package
is *already* a glisp library if the interop path is smooth (ADR-012 rule 4,
`docs/go-interop-exploration.md`). The charter is explicit — "the user never
writes Go", and a capability reachable only by hand-writing a `bridge.go` is an
*incomplete language*, a defect to close.

The interop *forms* are already in place and Clojure-shaped:

| Form | Emits |
|---|---|
| `(uuid/new-string)` | `uuid.NewString()` |
| `(.Year t)` | `t.Year()` |
| `(.-Timeout client)` | `client.Timeout` |
| `(http/Client. {})` | `http.Client{}` |
| `(as *pgx/Conn conn)` | `conn.(*pgx.Conn)` |

What is missing is that **the compiler is type-blind to imported packages.** It
emits the call text but does not know the *types* involved, so:

- A `(pkg/fn …)` call has no known return type. It flows as `any` unless a
  `let` `:=` happens to let Go infer it — so typed positions, dot-free method
  dispatch, and `(:field …)` access on an interop result all require an
  `(as T v)` round-trip (`go-interop-exploration §3.4`, §3.5).
- A variadic Go API (`pgx.Query(ctx, sql, args...)`, `fmt.Errorf`, `append`)
  called with a runtime slice has *no glisp spelling at all* — it needs a
  hand-written `bridge.go` (§3.7, the last reason wrapper modules drop to Go).
- An interop mistake surfaces as a raw `go build` / `go/types` error, not a
  glisp diagnostic (ADR-011 rule 3).

**jank** — the native Clojure dialect on LLVM — solves the analogous problem
for C++ by embedding Clang and **reading the host's real types.** Its interop
forms are the same ones Clojure already uses for Java (`(.size s)`,
`(cpp/.-a b)`, `(cpp/string. …)`); they become *seamless* not through new
syntax but because the compiler knows the types. Its only meaningful addition
is `cpp/cast` for genuine C++ overload ambiguity. ([jank-lang.org](https://jank-lang.org/),
[jank's C++ interop](https://jank-lang.org/blog/2025-06-06-next-phase-of-interop/))

glisp already has the precedent for the equivalent move: `stdlibgen`
(`internal/transpiler/internal/stdlibgen`) shells to `go list std` and bakes
the result into `stdlib.go`. Reading a package's *exported signatures* is the
same class of toolchain-derived fact, one step richer.

## Decision

**The compiler becomes type-aware of imported Go packages by reading their
exported signatures from the Go toolchain (`go/packages`, `NeedTypes`), and
folds them into the same pre-pass symbol tables that user `defn`s populate**
(`e.symbols`, `e.structs`, `e.ifaces`). This is jank's model with Clang
replaced by `go/types`. The existing forms gain, with no new surface syntax
except a spread marker:

1. **Typed returns.** A `(pkg/fn …)` call carries its real Go return type, so
   it flows into typed `let`/`def`/param/return positions and dot-free method
   dispatch (ADR-013) without `(as T …)`. `(:field (pkg/fn …))` and
   `(method (pkg/fn …))` resolve directly.
2. **Coerced/checked arguments.** `any` arguments at typed Go parameter
   positions coerce through the *existing* hint machinery — `numericCoercion`
   for numbers, the assertion/`emitExprWithHint` path otherwise. This
   generalizes the hardcoded `stdlibNumericParams` table (Phase 11a, `math/*`
   only) to *every* loaded package.
3. **Variadic auto-spread.** `(pkg/fn a b & xs)` emits `pkg.Fn(a, b, xs...)`
   when the loaded signature marks the final parameter variadic. `& sym`
   reuses the `& rest` spelling that already exists in `fn` params. This closes
   `go-interop-exploration §3.7` — the last documented reason a wrapper module
   needs Go.
4. **Glisp-level interop diagnostics.** Wrong arity, a wrong-typed argument, or
   a field/method that does not exist on the package's type becomes a
   position-tagged `.glsp` error at transpile time (ADR-011 rule 3), driven by
   the loaded signatures, instead of a raw `go/types` error.

This is an *enrichment of the existing pre-pass*, the same shape as ADR-013's
cross-file `DeclSet` collection — **not** a new evaluator, a second compile
phase, or a runtime change. The transpiler's structure is unchanged.

### Scope and non-goals

- **This is not type inference of glisp code.** It reads *host-declared*
  signatures, exactly as `stdlibgen` reads `go list std` and as jank reads
  Clang. The anti-roadmap's ban (ADR-012) is on "a type-inference engine beyond
  local, syntactic struct hinting" — i.e. inferring *glisp* types — which this
  does not do. No glisp expression's type is inferred that wasn't before.
- **`(as T v)` stays the explicit `any`-seam boundary.** Where a value's
  dynamic type genuinely isn't known to the compiler (a `map` lookup, an
  `any`-returning helper), `(as …)` remains required. Auto-inserting it there
  would need real inference. Go has no function overloading, so jank's dominant
  driver for explicit casts (`cpp/cast`) is largely absent here — `(as …)`
  covers the residue.
- **No inline Go, no `declare-go` form.** The lesson from jank is the *opposite*
  of inlining: make qualified-symbol interop good enough that dropping to Go is
  never necessary (charter rule 4). This supersedes the earlier "declare-go"
  sketch, which only made hand-written Go *pleasant* rather than *unnecessary*.
- **Purely additive / offline-degrading.** When `go/packages` cannot load a
  package (no network, unresolved dep, build error in the dep), that package's
  calls emit exactly as they do today (untyped). Typed interop is an
  enhancement layered on top, never a hard dependency — mirroring the
  panic-recovery boundary philosophy (`transpiler.go`).
- **Not the macro bet.** This is the *use existing libraries* half of
  extensibility; authoring new syntax (`defmacro`) is the *other* half, still
  governed by ADR-005 and the anti-roadmap. Interop needs only signature
  *reading*, not compile-time *evaluation*, which is why it is the smaller,
  higher-confidence move and is taken first.

## Implementation sketch

1. **Loader** — a `go/packages` load (`NeedTypes | NeedTypesInfo`) keyed by
   import path, run in the `emitFile` pre-pass *after* `ResolveDeps` has fetched
   the module and wired `go.mod`. Results cached per build (and ideally per
   `go.sum` hash across builds).
2. **Mapping** — walk each package's exported objects (`types.Func`,
   `types.Named` methods, struct fields) into the existing `fnSig` /
   `structInfo` shapes, using `identToGo`'s kebab↔Pascal rules in reverse for
   lookup (`new-string` ↔ `NewString`).
3. **Coercion** — thread loaded param types through `emitExprWithHint` at the
   general call path, exactly as user-fn param types and `stdlibNumericParams`
   already are.
4. **Spread** — detect a trailing `& sym` (or slice) call argument against a
   variadic signature; emit `...`.
5. **Diagnostics** — validate arity/types/field-existence against the loaded
   tables in `emitCallExpr`, producing `… (at L:C)` errors that
   `internal/lsp/diagnostics.go` already surfaces.
6. **Degradation** — a load failure for a package marks it "untyped"; all its
   calls fall through to current emission. No build ever fails *because* typed
   interop was attempted.

## Alternatives considered

- **Hand-maintained signature tables (extend `stdlibgen`).** Rejected: does not
  scale to arbitrary third-party packages and goes stale across versions.
  `go/packages` gives correct, version-matched signatures for free.
- **A `declare-go` / inline-Go form.** Rejected and superseded: it optimizes
  *writing* Go, but charter rule 4 wants Go-writing *unnecessary*.
- **A full embedded glisp evaluator (which would also unlock macros).**
  Out of scope: interop needs signature *reading* only. Coupling it to the much
  larger evaluator bet (ADR-005) would delay a high-confidence win behind a
  speculative one.
- **jank's `cpp/cast` type-DSL.** Not needed: Go has no overloading, so the
  ambiguity that motivates it does not arise; `(as T v)` is sufficient.

## Consequences

- The dominant interop friction — the `(as …)` sprinkle and `bridge.go` for
  variadics — largely disappears. Wrapping a Go package (e.g. `pgxdb`) becomes
  pure glisp, finally satisfying charter rule 4 for library authors as well as
  app authors.
- Builds gain a `go/packages` type-load step for imported packages: a real cost,
  mitigated by caching and by the fact that stdlib auto-imports (the common
  case) already resolve without it. The load is skipped entirely for programs
  with no external `(:import …)`.
- The transpiler takes a build-time dependency on `golang.org/x/tools/go/packages`
  (the module is `golisp`; this is a tooling dependency, not shipped in
  generated binaries).
- A new diagnostic surface to honor ADR-011 rule 3: interop mismatches must read
  as glisp errors at `.glsp` positions, which the loaded signatures now make
  possible at transpile time rather than via `//line`-mapped Go errors.
- ADR-005 (macros) is untouched. This ADR deliberately delivers the *use the
  ecosystem* half of extensibility while the *extend the syntax* half remains
  deferred.
