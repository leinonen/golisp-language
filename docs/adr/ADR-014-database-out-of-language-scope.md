# ADR-014: Database access is out of language scope — opt-in via packages

**Status**: Accepted. **Supersedes ADR-009.**

## Context

ADR-009 planned a postgres-first database story *inside the language*: built-in
`db/connect` / `db/query` / `db/exec` / `db/transaction` forms, a HoneySQL-style
query builder (`db/select`, `db/insert`, …), and a `glisp migrate` subcommand.
At the time, using a Go package meant a four-step manual `go.mod` ritual, so a
blessed in-core database wrapper looked like the only ergonomic option.

That premise no longer holds. The Go-interop work (ADR-012's identity property
2; `docs/go-interop-exploration.md`, implemented) made an external Go package a
one-command, one-declaration dependency:

- `glisp get github.com/jackc/pgx/v5` fetches a Go package and records it under
  `go-require` (single source of truth); `go.mod` is a derived artifact.
- `(:import [github.com/jackc/pgx/v5])` + kebab-case calls, `(.Method obj …)`,
  `(as *pgx/Conn v)`, and the `bridge.go` escape hatch cover the full surface.
- `_glispToSlice` already accepts `[]map[string]any`, so rows returned by a
  library flow straight into `map` / `filter` / `group-by` / `select-keys`.

A wrapper module (`github.com/leinonen/glispdb` over `pgx`) is the proof: it
delivers everything ADR-009 wanted with **zero transpiler involvement**.

Meanwhile the charter (ADR-012) sets the test: a new form must enable something
currently impossible or painful, the core stays small, and the language must not
grow just because a capability is useful. Built-in `db/*` forms fail that test —
they are a second way to do what package interop already does, and they couple
the language to one driver (pgx) and one database (postgres).

## Decision

Database access is **out of the language's scope**. The transpiler ships no
`db/*` special forms, no query builder, and no `glisp migrate` subcommand.

A database is an **opt-in package dependency**, consumed like any other Go or
glisp library:

- a glisp wrapper module (e.g. `glispdb`) via `(:require [...])`, or
- a Go driver (e.g. `pgx`) directly via `(:import [...])` + `go-require`,

using the existing interop primitives. Rows as `[]map[string]any` remains the
recommended convention — but it is a *library* convention, enforced by the
package, not by the language.

## Reasons

- **Thin static core (ADR-012).** "Write Clojure-shaped code, ship a single
  static Go binary." The binary stays small and SQL-free unless a program asks
  for it; nothing pays for a database it does not use.
- **The ecosystem is the library.** The interop multiplier means every Go
  database driver — pgx, `database/sql`, sqlite, a Mongo client — is *already* a
  glisp library. Blessing one in-core would freeze a choice the ecosystem should
  keep making.
- **One obvious way (charter rule 2).** `(:import [pgx])` + interop is the
  obvious way to use a Go package; `db/*` forms would be a redundant second
  spelling, the same reasoning that removed `#(...)`.
- **No new `any`-seam debt.** Driver results are `any`/`[]map[string]any` at the
  boundary and already absorbed by the collection helpers; a wrapper module
  hardens types where it wants (`(as *pgx/Conn …)`), exactly as designed.
- **Migrations are a tool concern, not a syntax concern.** Plain SQL +
  `goose`/`golang-migrate` (or a glisp script invoking them) needs no language
  feature.

## Consequences

- ADR-009 is superseded; its `db/*` forms, query builder, and `glisp migrate`
  are not implemented and will not be. ROADMAP "Phase 8 — Database" is reframed
  as package-delivered, with the enabling work marked done.
- The recommended path is documented via the wrapper-module pattern already in
  CLAUDE.md (`go-require` + `:import` + bridge functions); `glispdb` can serve as
  the reference implementation and be linked from the README.
- The anti-roadmap (ADR-012) gains an entry by reference: *in-core database
  forms* are out of scope, binding unless a future ADR supersedes this one.
- If a database genuinely needs a language-level affordance later (it has not so
  far), that affordance must clear the charter on its own merits — not arrive
  bundled as "database support."
