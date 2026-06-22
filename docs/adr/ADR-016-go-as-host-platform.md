# ADR-016: Go as host platform, not surface language

**Status**: Accepted (re-founds the project; supersedes the *identity* of
ADR-012, amends ADR-008, sets up ADR-017)

## Context

ADR-012 fixed glisp's identity as **"write Clojure-shaped code, ship a single
static Go binary"** with the rule "Go's runtime model, undiluted." That charter
was honest about what the language was: a very pleasant way to *write Go in
parentheses*. It served the build-out of Phases 2–12 well.

But it produced a language that, in daily use, still reads as Go: everyday code
types `fmt/println`, `os/args`, `strings/upper-case`, `json/encode`,
`re/match`. The Go package vocabulary *is* the surface vocabulary. That keeps
glisp a niche — "Lisp syntax over the Go stdlib" — rather than a language a
broad audience would reach for as *a Lisp*. Two consequences of the v1 charter
became limiting:

1. **No surface identity of its own.** There is no stable glisp vocabulary; the
   names are whatever Go happens to call things, and they shift with Go.
2. **The compiler is the only place the language can grow** (ADR-005 deferred
   macros), so every new surface form is a transpiler patch. That does not scale
   to a general-purpose standard library.

The precedent for the resolution is **Clojure's relationship to Java**: Clojure
is unmistakably its own language with its own core library and idioms; the JVM
is the *host platform* and Java is reached through clean, first-class, *visible*
interop (`(.method o)`, `(Foo.)`, `(:import …)`) when you want a Java library —
but you write Clojure, not "Java in parentheses," essentially all the time.

## Decision

**Go is glisp's host platform, not its surface language** — the way the JVM is
Clojure's host, not its surface.

1. **glisp has its own core vocabulary.** A self-contained standard library
   (`core`, `string`, `math`, `io`, `sys`, …) provides stable, glisp-native
   names for everyday work. Day-to-day code does not type Go package names.
2. **Go interop is first-class, ergonomic, and explicit — but not the default.**
   `(import [pkg :as alias])`, `(.Method o)`, `(.-Field o)`, `(Type. {…})`,
   `(as T v)`, and `& spread` remain and are *good*. They are glisp's Java
   interop. You reach for them to use a Go library; you don't live in them.
3. **The single static Go binary stays.** Transpile → `go build` is still the
   build path. "Hosted on Go" means Go is the platform we target and the
   ecosystem we borrow — not that we hide it or replace it.

This **supersedes the identity statement of ADR-012** ("Go's runtime model,
undiluted" as the *point*). It **does not** discard ADR-012's engineering
discipline — the charter's rules (one obvious way; the user never reads or
writes generated Go; tooling parity; the one-sitting tour) carry forward and
still govern feature proposals. What changes is the *identity those rules serve*.

## What carries forward unchanged

- **Gradual typing → real Go structs** (ADR-010/013). A genuine strength;
  `any` stays the always-valid fallback.
- **Errors as values** — `if-err` / `let-or`, not exceptions.
- **Go concurrency, undiluted** — goroutines, channels, `select!`. (The one
  place "undiluted Go" remains the literal goal, because it is excellent.)
- **Toolchain-driven typed interop** (ADR-015 / Phase 12) — the `go/packages`
  signature loader is the *foundation* interop is built on.
- **The discipline of ADR-012** — minimalism, tooling parity, glisp-level
  diagnostics, the tour.

## Amendment to ADR-008

ADR-008 ("Clojure inspiration, not compatibility") is amended in one respect:
the relationship to the host is now explicitly modeled on Clojure↔Java. We still
do not aim for Clojure *source* compatibility, and Go idioms we embrace
(errors-as-values, goroutines, multi-return, type assertions) are unchanged. But
"have its own core vocabulary distinct from the host's" moves from a non-goal to
a goal.

## Consequences

- A large new body of work: the `core` standard library (Phase 14) and the macro
  system that makes it tractable to write (ADR-017 / Phase 13).
- Raw Go names (`fmt/`, `os/`, `strings/`) are **not removed** — they remain
  valid interop, demoted in docs to the interop section. No existing code breaks.
- The README and tour must be rewritten to lead with the v2 identity.
- Feature evaluation gains a question alongside ADR-012's: *does this belong in
  `core` (the language) or in interop (the host)?*
- The "second compilation target" ban (ADR-012 anti-roadmap) stands. An eventual
  *interpreted execution mode* over the same source (open question in the
  roadmap) is a separate matter requiring its own ADR.
