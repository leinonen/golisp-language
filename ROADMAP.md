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

- [x] **Reader support** (13.0) — syntax-quote `` ` ``, unquote `~`,
  unquote-splice `~@`, `'` quote, and auto-gensym (`name#`).
- [x] **Homoiconic compile-time data** (13.1) — forms (symbols, keywords, lists,
  vectors, maps) are first-class *values* the macro evaluator constructs and
  destructures (`internal/macro`); the AST↔Value bridge un-parses specialized
  forms to generic list data and re-recognizes them on the way out.
- [x] **Compile-time evaluator** (13.1–13.2) — a tree-walking interpreter over
  the macro subset of glisp, plus syntax-quote expansion. Runs in a
  **macroexpansion pass between parse and emit** (`transpileWith`), the same
  shape as the ADR-013 `DeclSet` pre-pass.
- [x] **`defmacro`** (13.3) plus `macroexpand` / `macroexpand-1` (13.5, the
  `glisp macroexpand [--once]` command). Cross-file macro visibility (13.4).
- [x] **Hygiene model** (13.2) — non-hygienic with `gensym` / auto-gensym
  (`name#`), i.e. Clojure's model. (Syntax-quote symbol *qualification* is
  staged to land with `core` in Phase 14.)
- [x] **Tooling parity** (13.6) — `glisp fmt` round-trips `defmacro` and the
  reader forms; the LSP gives `defmacro` hover / completion / outline /
  jump-to-definition; the REPL/CLI gain `macroexpand`.
- [x] **Validation milestone** (13.7) — the auto-loaded `core` prelude
  (`internal/macro/core.glsp`, macros written in glisp, available in every file):
  additive macros `when-not`/`if-not` (13.7a), and the threading macros `->`/`->>`
  **ported from hand-coded emitters** (13.7b) — emitters retired, output proven
  byte-identical against the golden suite. Porting forced the expander's
  container walk to be **complete** (a macro call buried in a `let-or`/`go`/etc.
  body now expands), which the emitter-based forms got for free. The "library
  grows, compiler shrinks" thesis is demonstrated end-to-end. Remaining
  candidates (`as->`, `tap->`, `for`, `assert`, …) can follow the same pattern;
  forms that must touch Go's type system or statement/expression placement stay
  built-in.

> **Open design question tracked here, not yet decided:** whether the
> compile-time evaluator should also back a fast **interpreted execution mode**
> for `glisp run` (skipping `go build` for scripts). Big scripting win; needs its
> own ADR. The evaluator should be built so this stays possible.

---

## Phase 14 — `core`: the standard vocabulary

Give the language its own names; Go becomes the backend. This is where "feels
like a Lisp, not like Go" is actually delivered. `core` is **written in glisp**
(`internal/core/*.glsp`), compiled into each program as mangled `_gcore_<ns>_*`
helpers injected like the runtime (single-file inline, dir builds via
`glisp_core.go`) — namespaces live only at the glisp level, so they never
collide with Go's `string` type / stdlib packages. See
[docs/design/phase-14-core.md](design/phase-14-core.md). **14a (the mechanism +
`str/`) is done.**

- [x] **The `core` mechanism** (14a) — glisp-authored, mangled-and-injected,
  gated on use, with call-site resolution + arg coercion via the existing
  param-hint path; user defns shadow `core`; works in single-file and dir builds.
- [x] **`str` namespace** (14a) — `(str/upper s)`, `lower`, `trim`, `blank?`,
  `starts-with?`, `ends-with?`, `includes?`, `index-of`, `join`, `split`,
  `replace`, `repeat`, over Go's `strings`. (`str/` is distinct from the bare
  `str` concat built-in.) Grows as real code asks.
- [x] **`core` prelude, auto-referred** (14b) — bare clojure.core-style names:
  `slurp`/`spit`/`lines` (over file I/O). Resolution yields to user defns, local
  bindings, def globals, and built-ins, so any of those shadow a bare core name.
  More bare names grow here as needed.
- [x] **`sys` namespace** (14b) — `(sys/args)`, `(sys/env name)`, `(sys/exit code)`
  over Go's `os`. Confirms the mechanism generalizes to a second namespace
  (and that two namespaces inject/dedup together, with transitive closure —
  bare `lines` pulls in `str/split`).
- [x] **LSP for `core`** (14c) — hover and completion for `str/upper`, `sys/env`,
  bare `slurp`, … generated from the parsed `core` signatures so the docs can't
  drift from the implementation.
- [x] **Migrate the taught surface** (14d) — the one-sitting tour, README, and
  the `cli`/`logparser` examples now use `str/`, `sys/`, `slurp`, and bare
  `println`/`format`; the overlapping bare string built-ins (`upper-case`,
  `trim`, `split`, `join`, …) are marked **legacy → `str/…`** in `docs/builtins.md`
  and LSP hover (kept working — non-breaking). `str/join` takes the separator
  first (clojure.string order) and accepts any sequence.
- [x] **Migrate the rest of the example corpus** — `api`, `inventory`, `shapes`,
  `movienight`, and `todos` moved to `str/`/`sys/`/`slurp`/`spit`/`format` too,
  so every example now reads in the canonical surface (only `fmt/printf` and
  `filepath/join`, which have no core form, remain as interop). Surfaced two
  core gaps, now fixed: `sys/env` takes an optional default (`(sys/env "PORT"
  "4000")`), and `str/join` accepts any sequence including sets.
- [ ] **`math` namespace** — native-named over Go `math` (`(math/sqrt x)`,
  `(math/floor x)`, `math/pi`, …). Make it the canonical name, not a raw Go
  passthrough. (Deferred — Go's `math/*` names already read naturally; the
  numeric domain was not chosen for v2.)
- [~] **Grow the vocabulary** — `str/` gained `capitalize`, `trim-start`,
  `trim-end`, `replace-first`, `last-index-of`, `pad-left`, `pad-right` (18a).
  More `str/`, data helpers, and bare `core` functions grow as demand drives.

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
  diagnostics.
  - **Arity diagnostics done** (15a): a wrong-arity call to a loaded Go function
    is a position-tagged `.glsp` error (`arity error: pkg/fn called with N
    arg(s), expected M`) instead of an opaque Go compile error — variadic
    fixed-minimum and spread (`& xs`) aware; an unloaded package degrades to
    untyped emission.
  - **12c method dispatch + return types done** (15c): the loader now reads
    exported named-type **method sets** (`go/packages`, pointer set for structs /
    own set for interfaces). A value whose external Go type is known — from a
    type annotation (`[c *pgx/Conn]`), a typed interop return (`(pkg/new …)`), or
    a chained method result — dispatches that type's methods dot-free
    (`(query c sql)` → `c.Query(sql)`) with arg-type hints, no `(.Method …)`
    needed; the external type also propagates through `let` so results chain. A
    **non-existent method on a known external type is a position-tagged error**
    (`type pkg.T has no exported method M`). Verified end-to-end:
    `(match-string (regexp/must-compile …) s)` → `re.MatchString(s)`.
  - **12e field access done** (15e): the loader also reads exported **struct
    field sets**. A value of a known external struct type reads its fields
    dot-free — `(.-Scheme u)` and `(:scheme u)` both emit `u.Scheme` — and a
    non-existent field is a position-tagged error (`type pkg.T has no exported
    field F`), in both spellings. Field types whose own type is external chain
    through inference. Verified end-to-end with `net/url.URL`.
  - **12e diagnostics complete** (15f): a multi-return external **method** used
    dot-free in single-value position is now gated (`goFuncToSig` renders a 2+
    result method as `(T1, T2)` so `multiReturnCall` catches it), and a
    **wrong-kind literal argument** to a loaded `pkg/fn` is a position-tagged
    error (`checkGoCallArgTypes`, literals only — false-positive-free).
  - Remaining (minor): non-literal wrong-typed args aren't diagnosed (Go reports
    them, `.glsp`-mapped); an `if-err`-bound multi-return interop result isn't
    type-recorded for further dot-free dispatch; `GoImportPaths` doesn't load a
    package referenced only by a type annotation (`:import` it).
- [x] **Document the boundary** (15d) — [`docs/interop.md`](docs/interop.md):
  "reach for interop when you need a Go library; otherwise stay in `core`." A
  single guide covering external-package imports, `pkg/fn` calls with arg
  coercion + arity diagnostics, dot-free method/field dispatch on typed values,
  the interop primitives, and the documented limitations. Linked from the README.

Phase 15 is substantially complete: the interop engine (12a–12e) plus the
boundary doc. The remaining 12e remnants above (wrong-typed-arg diagnostics,
multi-return-method gating, `if-err` result typing) are tracked as a backlog,
not blockers.

---

## Phase 16 — Scripting & CLI

Make glisp a first-class scripting and automation language. Single static
binary + a real Lisp + (optionally) skip-the-build is a strong niche.

- [x] **Shebang support** (16a) — a leading `#!` line is skipped by the lexer
  (only at file start; `#` keeps its meaning elsewhere) and preserved across
  `glisp fmt`, and `glisp <file.glsp> [args]` runs a file directly, so
  `#!/usr/bin/env glisp` scripts are executable (`chmod +x foo.glsp; ./foo.glsp`).
- [x] **`glisp run --watch`** (16b) — rebuilds and re-runs the target on every
  source change, killing and restarting the previous process so long-running
  programs (e.g. web servers) restart cleanly. Dependency-free mtime polling; a
  build failure is reported and watching continues. Ctrl-C stops the child and
  exits with no orphan. (Fast-startup `glisp run` interpreted path remains a
  separate stretch item below.)
- [ ] **Interpreted fast-path (stretch)** — reuse the Phase 13 evaluator to run
  scripts without `go build`, for sub-second startup. Gated by its own ADR.
- [x] **`cli` library** (16c) — `cli/parse-opts` (clojure.tools.cli-shaped),
  authored in glisp as a `core` namespace (`internal/core/cli.glsp`). Specs are
  maps (`{:long "--port" :short "-p" :default 8080 :int true}` / `:flag`);
  returns `{:options :arguments :errors :summary}`. Handles `--name value`,
  `--name=value`, `-n value`, flags, defaults, `:int` coercion, `--`, and
  collects unknown-option / missing-value / bad-int errors. Dogfooded in
  `examples/cli` (`--help`/`--json`). Subcommand parsing is a possible follow-up.
- [x] **`proc` / `sh`** (16d) — `(proc/run cmd & args)` (no shell) and
  `(proc/sh command)` (via `sh -c`) run external commands, returning
  `{:out :err :exit :ok}` (captured stdout/stderr, exit code, success flag).
  Built-in namespace with a Go runtime helper (`_proc` pseudo-key → `os/exec` +
  `bytes`), keeping the unsafe `*exec.ExitError` assertion in Go. As a side
  benefit, `str/` calls now coerce these `any` map values (`(str/trim (:out r))`).
- [x] **Filesystem & paths** (16e) — a `path/` namespace (`join`, `dir`, `base`,
  `ext`, `clean`, filepath-backed) plus bare `(glob pattern)` and `(walk dir)`.
  Built-in forms with runtime helpers (`_path` → `path/filepath`; `_walk` →
  `path/filepath` + `io/fs`), alongside the existing file built-ins
  (`read-file`/`write-file`/`list-dir`/`mkdir`/`file-exists?`, `slurp`/`spit`).
  (`path/split` and temp-file/dir helpers — which need multi-return or `os` —
  remain a possible follow-up.)

---

## Phase 17 — Data processing

Lean on the strong collection library; add the pieces an ETL/transform job
needs. Eager throughout (no lazy seqs — see open questions).

- [x] **Eager transducers** (17b) — `map`/`filter`/`remove`/`keep`/`take`/`drop`/
  `take-while`/`drop-while` called with a single arg return a transducer (a unary
  `rf→rf`), so the existing `comp` composes them; `transduce`/`sequence`/`into`
  (3-arg) apply them. Eager (ADR-003) with a `Reduced` sentinel for early
  termination (`take`/`take-while`). Runtime block `glispXfRuntime` (`_xf` key,
  no real imports). The 2-arg eager forms are unchanged.
- [x] **CSV** (17a) — `(csv/parse text)` → header-mapped rows (`[]any` of
  `map[string]any`, first record = header) and `(csv/write rows)` → CSV string
  (header = first row's keys, sorted). Built-in forms over `encoding/csv` (the
  `json/` pattern); both return `(value, error)` for `if-err`. Composes with
  `slurp`/`spit`/`walk` for read→transform→write.
- [x] **Line-oriented IO** (17c) — `(read-lines path)` reads a file's lines into
  a vector; `(transduce-lines xform rf init path)` streams the lines through a
  transducer pipeline in **constant memory** (`bufio.Scanner`), honoring the
  `Reduced` sentinel so `take`/`take-while` stop reading early. Both return
  `(value, error)` for `if-err`.
- [x] **Streaming JSON** (17d) — `(transduce-json xform rf init path)` streams a
  top-level JSON array's elements one at a time through a transducer pipeline via
  `json.Decoder` (constant memory), honoring the `Reduced` sentinel so
  `take`/`take-while` stop decoding early. The JSON analog of `transduce-lines`;
  returns `(value, error)` for `if-err`.
- [x] **Pipeline ergonomics** (17c) — the `read → transform → write` idiom is now
  first-class and documented (`docs/builtins.md`): `transduce-lines` (or
  `slurp`/`csv/parse`) → transducer pipeline → `spit`/`csv/write`, the
  data-domain analog of the web validation-handler idiom.

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
