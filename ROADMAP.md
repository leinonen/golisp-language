# glisp Roadmap — The Lisp Era (v2)

**North star**: glisp is a **general-purpose Lisp hosted on Go**. It has its own
language identity, its own core vocabulary, and its own idioms; Go is the *host
platform* — reached through first-class, explicit interop — exactly the way
Clojure is hosted on the JVM and reaches Java. You write glisp, not "Go in
parentheses."

This is a deliberate re-founding. The original identity ("write Clojure-shaped
code, ship a single static Go binary" — ADR-012) made *feeling like Go* the
point. v2 keeps the single static binary and the Go ecosystem, but moves the
**surface language** to glisp's own ground. See:

- **[ADR-016](docs/adr/ADR-016-go-as-host-platform.md)** — Go as host platform, not surface language (supersedes the identity of ADR-012, amends ADR-008)
- **[ADR-017](docs/adr/ADR-017-compile-time-macros.md)** — Compile-time macros (supersedes ADR-005)
- The completed v1 plan is preserved in **[ROADMAP-ARCHIVE.md](ROADMAP-ARCHIVE.md)**.

---

## The three commitments of v2

1. **Its own vocabulary.** Everyday glisp code never types `fmt/`, `os/`,
   `strings/`, `json/`. The language ships a self-contained core library
   (`core`, `string`, `math`, `io`, `sys`, …) with stable glisp-native names.
   Go package names appear only when you deliberately reach for a Go library.
2. **Extensible by its users.** `defmacro` with syntax-quote. New syntax is
   written *in the library*, not patched into the compiler. This is the keystone
   — it's what lets a general-purpose standard library grow at all.
3. **Beyond servers.** CLI/scripting and data processing become as first-class
   as web. The single static binary is a scripting superpower; lean into it.

## What does **not** change

The pivot is about *surface and extensibility*, not about throwing away what
works. These remain load-bearing:

- **Single static binary via Go.** Transpile → `go build` stays the build path.
- **Gradual typing that hardens into real Go structs.** `defstruct` + typed map
  literals + typed keyword access + dot-free method dispatch (ADR-010/013) are a
  genuine strength and stay. `any` is still the always-valid fallback.
- **Errors as values.** `if-err` / `let-or` remain the spine of error handling
  (not exceptions).
- **Go's concurrency, undiluted.** Goroutines, channels, `select!`, and the
  concurrency sugar stay.
- **Toolchain-driven typed interop (Phase 12).** The `go/packages` signature
  loader is the foundation interop is built on — v2 finishes it, it doesn't
  replace it.
- **The tooling bar.** A feature is not done until `glisp fmt` round-trips it and
  the LSP knows it, and the user never debugs generated Go (ADR-011).

---

## Phase 13 — The macro engine (foundation)

The keystone of v2; everything downstream leans on it. This is the single
largest piece of engineering on the roadmap and the highest-leverage. See
ADR-017 for the design rationale.

- [ ] **Reader support** — syntax-quote `` ` ``, unquote `~`, unquote-splice
  `~@`, `'` quote as a real data-producing form, and auto-gensym (`name#`).
- [ ] **Homoiconic compile-time data** — forms (symbols, keywords, lists,
  vectors, maps) are first-class *values* the macro evaluator can construct and
  destructure. `quote` yields data, not a string.
- [ ] **Compile-time evaluator** — a tree-walking interpreter over the macro
  subset of glisp, sufficient to run macro bodies. Runs in a **macroexpansion
  pass inserted between parse and transpile** (same architectural shape as the
  ADR-013 cross-file `DeclSet` pre-pass — an enrichment, not a second product).
- [ ] **`defmacro`** plus `macroexpand` / `macroexpand-1`.
- [ ] **Hygiene model** — non-hygienic with syntax-quote symbol qualification +
  `gensym`, i.e. Clojure's model. Documented explicitly (this was ADR-005's
  central worry; it is now a conscious, bounded tradeoff).
- [ ] **Tooling parity** — formatter and LSP degrade gracefully on user macro
  call-sites (treat unknown heads via the generic call-form path); REPL gains
  `macroexpand`.
- [ ] **Validation milestone** — reimplement a batch of existing special forms
  (`when`, `when-not`, `->`, `->>`, `as->`, `cond`, `if-let`, `when-let`,
  `doto`, `assert`, `for`) as `core` **macros**, retiring their bespoke
  transpiler emitters where it simplifies the compiler. If the language can
  define its own control flow, the macro system is real. (Forms that must touch
  Go's type system or statement/expression placement may stay built-in — the
  goal is to *prove and shrink*, not to dogmatically move everything.)

> **Open design question tracked here, not yet decided:** whether the
> compile-time evaluator should also back a fast **interpreted execution mode**
> for `glisp run` (skipping `go build` for scripts). Big scripting win; needs its
> own ADR. The evaluator should be built so this stays possible.

---

## Phase 14 — `core`: the standard vocabulary

Give the language its own names; Go becomes the backend. This is where "feels
like a Lisp, not like Go" is actually delivered. Wherever practical, `core` is
**written in glisp** (dogfooding the Phase 13 macros and fns); only the leaves
that must touch Go are transpiler-known.

- [ ] **`core` prelude, auto-referred** — `println`/`print`/`pr`/`prn`/`str`/
  `format`/`assert`/… available unqualified (`println` and `print` already are).
- [ ] **`string` namespace** (shaped like `clojure.string`) — `(string/upper s)`,
  `(string/lower s)`, `(string/split s re)`, `(string/join sep coll)`,
  `(string/replace s a b)`, `(string/trim s)`, `(string/blank? s)`, … over Go's
  `strings`, but `string/*` is *the* surface.
- [ ] **`math` namespace** — native-named over Go `math` (`(math/sqrt x)`,
  `(math/floor x)`, `math/pi`, …). Already partly present; make it the canonical
  name, not a raw Go passthrough.
- [ ] **`io` namespace** — `(slurp path)`, `(spit path content)`, `read-line`,
  `lines`, alongside the existing file built-ins (which become `io/*`).
- [ ] **`sys` namespace** — `(sys/args)`, `(sys/env "X")`, `(sys/exit code)`,
  `(sys/getenv …)` replacing the surface use of `os/args` / `os/env` / `os/exit`.
- [ ] **Migrate the surface** — examples, the tour, and docs move from raw
  `fmt/`/`os/`/`strings/`/`json/`/`re/` to `core` namespaces. The raw Go names
  keep working **as interop** (Phase 15) — deprecated in docs, not removed, so no
  code breaks.
- [ ] **`core` is documented as the language**, not as "wrappers." `glisp doc`
  and the LSP lead with `core`; Go names are surfaced under interop.

---

## Phase 15 — Interop, the Clojure-to-Java way

Make Go interop a deliberate, ergonomic, *visible* facility — first-class but no
longer the default surface. Built directly on the Phase 12 typed-interop loader.

- [ ] **One clear interop import** — `(import [github.com/user/pkg :as alias])`
  as the blessed way to pull in a Go package, mirroring Clojure's `(:import …)`.
  Keep `pkg/fn` call syntax for power users.
- [ ] **Keep the interop primitives** — `(.Method o args)`, `(.-Field o)`,
  `(Type. {…})`, `(as T v)`, and `& spread`. These are golisp's `(.method)` /
  `(Foo.)` / `(cast)`, and they are *good*. They stay visible, like Java interop
  in Clojure.
- [ ] **Finish typed interop (Phase 12 remnants)** — 12c typed returns into
  dot-free dispatch and typed positions; 12e full glisp-level interop
  diagnostics (wrong arity / wrong-typed arg / missing field-or-method as
  position-tagged `.glsp` errors).
- [ ] **Document the boundary** — "reach for interop when you need a Go library;
  otherwise stay in `core`." Wrapping a Go package is pure glisp (ADR-012 rule 4
  carries forward), but it's now clearly *interop*, not the everyday surface.

---

## Phase 16 — Scripting & CLI

Make glisp a first-class scripting and automation language. Single static
binary + a real Lisp + (optionally) skip-the-build is a strong niche.

- [ ] **Shebang support** — `#!/usr/bin/env glisp` lines tolerated by the lexer
  so `.glsp` files are directly executable.
- [ ] **Fast `glisp run`** — and **`glisp run --watch`** (re-run on save,
  carried over from Phase 11).
- [ ] **Interpreted fast-path (stretch)** — reuse the Phase 13 evaluator to run
  scripts without `go build`, for sub-second startup. Gated by its own ADR.
- [ ] **`cli` library** — declarative arg/flag/subcommand parsing
  (`clojure.tools.cli`-shaped).
- [ ] **`proc` / `sh`** — subprocess execution with captured stdout/stderr/exit.
- [ ] **Filesystem & paths** — glob, walk, path join/split, temp files, building
  on the existing file built-ins folded into `io`.

---

## Phase 17 — Data processing

Lean on the strong collection library; add the pieces an ETL/transform job
needs. Eager throughout (no lazy seqs — see open questions).

- [ ] **Eager transducers** — `(comp (map f) (filter g) (take n))` with
  `transduce` / `into` / `eduction`. Transducers compose pipelines **without**
  laziness, so they fit ADR-003 cleanly and unify the existing HOFs.
- [ ] **CSV** — read/write, header-mapped rows as `[]map[string]any` (the row
  convention already used for DB results).
- [ ] **Streaming JSON / line-oriented IO** — process large inputs without
  loading them whole; `lines` + transduce.
- [ ] **Pipeline ergonomics** — make the `read → transform → write` shape a
  first-class, documented idiom (the data-domain analog of the web
  validation-handler idiom).

---

## Phase 18 — Web, continued

Keep the v1 strength; evolve it under the new model.

- [ ] **Web as a `core`-style library** — the web stack (routing, middleware,
  hiccup, SSE, websockets) presented as a first-class glisp library rather than
  a special Go package the user imports by URL.
- [ ] **Richer routing / templating** ergonomics, now that macros exist (a
  routing DSL is the canonical "macros earn their keep" example).
- [ ] **Observability** — structured request logging, metrics hooks, tracing
  context already half-present via `ctx/*` and `log/*`.

---

## Tooling & polish (continuous, not a phase)

- **fmt + LSP parity** for macros, `core` namespaces, and interop — the ADR-012
  tooling-parity invariant carries forward unchanged.
- **Error messages stay glisp-level** (ADR-011). Macro-expansion errors must
  point at `.glsp` source, including inside expanded forms — a new front for the
  "never debug generated code" principle.
- **Docs** — a real docs site / book; the README leads with the v2 identity. The
  **one-sitting tour** survives, now demonstrating macros and `core`.
- **Homebrew tap** (carried over from v1).

---

## Open questions / candidate revisits

Each needs its own ADR before adoption — listed so they're tracked, not assumed.

- **Lazy sequences (ADR-003).** Banned in v1. A general-purpose data language
  feels the absence (infinite/unbounded `range`, pull-based streaming).
  *Decision: try eager transducers + generators first; revisit only if they
  prove insufficient.*
- **Value-equality `=` / `not=`.** Still Go `==` (documented footgun:
  `(= (int64 1) (int 1))` is `false`, collections compare by identity). More
  pressing now that the audience is broader. Wants `_glispEquals` + an ADR.
- **Interpreted runtime / self-hosting.** The macro evaluator opens the door;
  how far to walk through it (scripting fast-path, a true REPL, eventual
  self-hosting of `core`) is undecided.
- **Reader macros / tagged literals** (`#inst`, `#uuid`, user-defined). Deferred;
  `defmacro` ships first.
- **Hygiene.** Start gensym-based (Clojure's model); move toward fuller hygiene
  only if real bugs warrant it.
- **Protocols vs `definterface`/`defmethod` (ADR-008).** Revisit only if
  polymorphism needs outgrow the current triad.
- **Numeric story.** Still `int64`/`float64`. The numeric/scientific domain was
  *not* chosen for v2, so the numeric-tower ban (ADR-012) stands; revisit only if
  that domain is later added.

## Anti-roadmap (still out, unless a future ADR supersedes)

- **A second *compilation* target.** Go stays the build backend. (An *interpreted
  execution mode* over the same source is a different thing and is an open
  question above, not a second target.)
- **Numeric tower** — deferred; not in the chosen domains.
- **Exceptions as the primary error idiom** — errors-as-values stays the spine;
  `panic`/`recover` remain for crashes and Go interop.
