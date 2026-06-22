# Phase 14 design — `core`, the standard vocabulary

**Status**: Design (implements [ADR-016](adr/ADR-016-go-as-host-platform.md);
see [ROADMAP.md](../ROADMAP.md) Phase 14)

Phase 13 built the macro engine and a tiny `core` prelude of macros
(`internal/macro/core.glsp`). Phase 14 gives glisp its own **vocabulary**: a
standard library, **written in glisp**, that fronts the Go stdlib with stable
glisp-native names so everyday code stops typing `fmt/`, `strings/`, `os/`. This
is the chosen model (the Clojure↔Java relationship): `core` is real glisp you can
read; Go is the host it wraps.

This doc fixes the *mechanism*. The exact vocabulary will grow over many slices;
what must be right first is how a glisp-authored standard library gets compiled
into every program without growing the compiler.

## 1. Model

- **`core` is glisp.** Functions like `(defn upper [s string] -> string
  (strings/to-upper s))` live in embedded `.glsp` files. The library grows by
  editing glisp, not the transpiler (ADR-016).
- **Two shapes, like Clojure:**
  - *`clojure.core`-style* — bare, auto-referred names (`slurp`, `spit`,
    `even?`). These join the existing bare built-ins (`println`, `str`, `count`).
  - *namespaced libraries* — `(str/upper s)`, `(sys/env "X")`, mirroring
    `clojure.string` / `clojure.java.io`.
- **Go stays reachable.** The raw Go-interop names (`strings/to-upper`,
  `os/args`) keep working unchanged — `core` is *additive*, the migration is
  doc-led, nothing breaks (see §6).

## 2. The central constraint: do NOT compile `core` to Go packages

The tempting design — make `str` a real Go package and emit `str.Upper` — fails:
a glisp namespace named `string`/`math`/`io` would need a Go import aliased to
that name, and `string` is a predeclared Go **type**, `math`/`io` are Go
**packages**. `import string "…"` shadows the type; `math`/`io` collide with the
very stdlib `core` wraps. Real packages also drag user programs into a module
dependency on `golisp/core/...`, which fights the single-binary, no-network
build story.

**Therefore `core` namespaces exist only at the glisp level.** They compile to
**flat, mangled Go functions** injected into the build exactly like the runtime
helpers (`_glispGet`, …) already are — no Go packages, no import aliases, no
collisions. `(str/upper s)` → `_gcore_str_Upper(s)`; `(slurp p)` →
`_gcore_slurp(p)`. The names never appear in Go import space, so `core`'s `math`
and Go's `math` could even coexist.

## 3. Mechanism

### 3.1 Authoring (`internal/core/*.glsp`)

Each namespace is one embedded glisp file with PascalCase `defn`s (the export
convention) wrapping Go via the existing interop and built-ins:

```clojure
(ns str)                                   ; the str/ namespace
(defn Upper [s string] -> string (strings/to-upper s))
(defn Blank? [s string] -> bool (= 0 (count (strings/trim-space s))))
(defn Join  [sep string xs []string] -> string (strings/join xs sep))
```

`core` may call: built-ins, Go interop (`strings/…`), other `core` functions,
and `core` macros. Bare-function files (`clojure.core` style) use the same shape
without a namespacing prefix on the call site.

### 3.2 Mangling + transpile

A dedicated transpile of the `core` sources lowers each `(defn Name …)` in
namespace `ns` to a Go function named `_gcore_<ns>_<Name>` (bare-core files use
`_gcore_<Name>`). Internal references — a `core` function calling `str/lower`, or
a bare `slurp` calling another bare helper — are rewritten to the same mangled
names. This is a small name-resolution pass over `core`'s own AST (it never sees
user code). The result is a block of plain Go functions, cached at toolchain
build time (the `core` glisp is fixed, so this can even be precomputed/tested).

### 3.3 Resolution at the call site

- A new known set `coreNamespaces` (`str`, `sys`, …) and `coreBareNames`
  (`slurp`, `spit`, …), parallel to `builtinNamespaces`.
- In `emitExpr`/`emitCallExpr`, **after** user-`defn` lookup (so a user
  definition shadows `core`, Clojure-style) and the existing built-ins, resolve a
  `ns/fn` whose `ns ∈ coreNamespaces`, or a bare name `∈ coreBareNames`, to its
  mangled `_gcore_…` symbol and mark that namespace/function **needed**.
- Argument coercion reuses the existing param-hint path: `core` signatures are
  known (they're glisp `defn`s with types), so `any` args coerce at `core` call
  sites just like user-fn calls — `(str/upper (get m "k"))` works.

### 3.4 Injection (gated, like runtime helpers)

Mirror the runtime-helper machinery (`emit_runtime.go`, `RuntimeSource`):

- Track needed `core` namespaces/functions in a set alongside `builtinImports`.
- **Single-file build**: append the needed `core` functions' Go to the file,
  after the user code, before/with the runtime helpers.
- **Multi-file dir build**: write them once into the shared `glisp_runtime.go`
  (or a sibling `glisp_core.go`) so the package has them once.
- Gating granularity: per-namespace to start (pull in a whole namespace's funcs
  when any member is used); `go build` dead-code-eliminates the unused, so output
  stays clean. Per-function gating is a later refinement if needed.

Transitive needs (a used `core` fn calls another `core` fn / a `core` macro)
must mark their callees needed too — computed from `core`'s own
dependency graph at mangling time (each function records which `core` symbols it
references; pulling one pulls its closure).

### 3.5 Single source of truth, tested

Because `core` is fixed glisp, its transpiled-and-mangled Go is deterministic. A
golden test transpiles every `core` namespace and `go vet`s the result, so a
broken `core` is caught at glisp's own `go test`, never in a user build (the same
guarantee `CoreMacros()` already gives for the macro prelude).

## 4. Namespace naming (additive, non-colliding)

To keep Phase 14 non-breaking, the first `core` names avoid the Go-interop
qualifiers already in use (`strings`, `math`, `os`, `io`, `fmt`):

| core | wraps | notes |
|---|---|---|
| bare `slurp`/`spit`/`read-line`/`lines` | `os`/`bufio` | `clojure.core`/`clojure.java.io` style; file built-ins (`read-file`…) stay as the low-level form |
| `str/*` (`upper` `lower` `trim` `split` `join` `blank?` `replace` `starts-with?` …) | `strings` | `str/` is distinct from the bare `str` concat built-in (qualified vs bare) |
| `sys/*` (`args` `env` `exit` `getenv`) | `os` | distinct from `os/` interop |

`math` and `io` are **deferred**: Go's `math/*` names already read naturally and
the `io` qualifier collides with Go's `io` package; bare `slurp`/`spit` cover the
common file needs. They can get `core` namespaces later if a glisp-native math
surface earns its keep (the numeric domain was not chosen for v2).

Priority order at a call site: **user defn → built-in form → `core` → Go
interop**, so users can shadow `core`, and `core` shadows nothing that exists
today.

## 5. Tooling parity (ADR-012)

- **fmt**: `core` calls are ordinary `ns/fn` / bare calls — the formatter already
  round-trips them via the generic call path. No change.
- **LSP**: add `core` namespaces/functions to `BuiltinDocs` (hover) and to
  completion (offer `str/up…` → `str/upper` with the glisp signature). Generated
  from the `core` `defn` signatures so docs can't drift.
- **`glisp doc`**: list `core` alongside the built-ins.

## 6. Migration (doc-led, non-breaking)

- Raw Go-interop names keep compiling. `core` adds glisp-native fronts.
- The README, the one-sitting tour, and `examples/*` move to `core` names so the
  *taught* surface is glisp-native; the Go names are demoted to the interop docs.
- No deprecation warnings in the compiler (that would be noise); the steering is
  in documentation, per ADR-016.

## 7. Delivery order

1. **14a — mechanism + `str/`.** The whole pipeline end-to-end for one namespace:
   embed + mangle + dependency-closure + gated injection (single-file *and* dir)
   + call-site resolution + arg coercion + the `core` golden/vet test. Ship `str/`
   with a useful first set. This is the de-risking slice; everything else is
   vocabulary on top.
2. **14b — bare `slurp`/`spit`/`lines` + `sys/`.** Exercises bare-core
   resolution and a second namespace.
3. **14c — migrate the tour + README + examples** to `core` names; add LSP
   docs/completion for `core`.
4. **14d+ — grow the vocabulary** as real programs ask for it (the data/CLI
   domains drive this), and revisit `math`/`io` namespaces if warranted.

## 8. Risks & open questions

- **Per-namespace vs per-function gating.** Start per-namespace (simple; relies
  on Go DCE). Revisit only if generated-file size becomes a concern.
- **Mangling collisions.** `_gcore_` prefix is reserved; document it as off-limits
  for user identifiers (identToGo already produces no `_gcore_`-prefixed names
  from ordinary glisp).
- **`core` calling macros.** A `core` function body may use `core` macros
  (`when-not`, `->`); expansion already runs on the `core` transpile, so this
  works — but the `core` transpile must run the macro pass too (it does, via the
  same `transpileWith` path).
- **Overhead.** One Go call per `core` op wrapping a stdlib call; Go inlines
  trivial wrappers, so it is effectively free. Measured if ever in doubt.
- **`str` bare vs `str/` namespace.** Coexist (bare concat built-in vs qualified
  string ops); document the distinction. If it confuses users, the namespace can
  be renamed (`string/`) — still collision-free since it never becomes a Go
  import.
- **Self-hosting horizon.** Once `core` is substantial glisp, the "interpreted
  execution mode" open question (run `core`-heavy scripts without `go build`)
  becomes more attractive — tracked separately.
