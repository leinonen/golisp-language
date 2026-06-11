# Exploration: frictionless interop with the Go package ecosystem

**Status**: Exploration (input for a future ADR-014)
**Date**: 2026-06-10

glisp's pitch is "write Clojure-shaped code, ship a single static Go binary"
(ADR-012). The single biggest force multiplier available to the language is the
existing Go ecosystem: every Go package is *already* a glisp library if the
interop path is smooth. This document records a hands-on exploration of that
path — what works today, what breaks (every defect below was reproduced against
the current `main`), and a proposal for making "use a Go package" a
one-command, one-declaration experience.

## 1. What works today

The interop surface is already substantial:

| Primitive | Example | Emits |
|---|---|---|
| Qualified call | `(uuid/new-string)` | `uuid.NewString()` |
| Qualified value | `time/March` | `time.March` |
| Method call | `(.Year t)` | `t.Year()` |
| Field access | `(.-Timeout client)` | `client.Timeout` |
| Struct literal | `(http/Client. {})` | `http.Client{}` |
| Type assertion | `(as *pgx/Conn conn)` | `conn.(*pgx.Conn)` |
| Qualified type | `*pgx/Conn` in type position | `*pgx.Conn` |
| External import | `(:import [github.com/google/uuid])` | `import "github.com/google/uuid"` |
| Multi-return | `(if-err [f err] (os/create p) … …)` | `f, err := os.Create(p); if err != nil …` |

Things that pleasantly exceed expectations:

- **Name conversion is genuinely nice.** `(uuid/new-string)` →
  `uuid.NewString()`, `(strings/has-prefix s p)` → `strings.HasPrefix(s, p)`.
  Kebab-case feels native and the PascalCase rule is predictable.
- **Concrete types flow further than ADR-007 suggests.** `let` scalar bindings
  use `:=`, so `(let [t (time/now)] (.Year t))` keeps `t` as `time.Time` and
  the method call compiles with no annotation.
- **Homogeneous vector literals get concrete element types.**
  `(strings/join ["a" "b"] ", ")` emits `strings.Join([]string{"a", "b"}, ", ")`
  — a literal string vector becomes `[]string`, so typed Go APIs accept it.
- **Auto-import for single-segment stdlib** (`fmt`, `strings`, `time`, `os`,
  …) means most programs need no `(:import …)` at all.

End-to-end, this works today (after manual `go.mod` setup, see §3.1):

```clojure
(ns main
  (:import [github.com/google/uuid]))

(defn main [] -> void
  (fmt/println "id:" (uuid/new-string)))
```

## 2. Verified defects

> **Status update**: §2.1–2.3 and §2.5 are fixed on this branch (qualifier
> resolution incl. `/vN` and `:as`, Clojure-shaped import clauses, the
> `context` import, and the CLAUDE.md example). §2.4 (`go.mod` self-healing)
> is part of the P2 proposal and still open.

Each of these was reproduced with a freshly built `glisp` binary. They are
ordinary bugs — fixable independently of any design change — but every one of
them lands the user in raw Go errors or invalid generated Go, violating
ADR-012 rule 3 ("the user never debugs generated Go").

### 2.1 Version-suffixed import paths emit a spurious bare import

```clojure
(ns db
  (:import [github.com/jackc/pgx/v5]))

(defn connect-db [url string] -> [any error]
  (pgx/connect (ctx/background) url))
```

emits **invalid Go**:

```go
import (
    "pgx"                          // ← spurious; package "pgx" does not exist
    "github.com/jackc/pgx/v5"
)
```

Cause: `isModuleAlias` (`transpiler.go`) matches a qualifier only against the
*literal* last path segment. For `github.com/jackc/pgx/v5` that segment is
`v5`, so the qualifier `pgx` falls through to `directImports` and is emitted
as a bare import. Go's own convention — the import qualifier of a `/vN`
module path is the *second-to-last* segment — is not implemented. The
CLAUDE.md "Wrapping a Go package" section documents this exact pattern as
working.

### 2.2 `:as` aliases don't suppress the spurious import either

The natural workaround, an explicit alias, also fails:

```clojure
(:import [[github.com/jackc/pgx/v5 :as pgx]])
```

emits both the alias *and* the bare import — still invalid Go:

```go
import (
    "pgx"
    pgx "github.com/jackc/pgx/v5"
)
```

Cause: `isModuleAlias` checks only `ImportSpec.Path`, never `ImportSpec.Alias`.

Related paper cut: the alias syntax requires double brackets
(`(:import [[path :as alias]])`). The Clojure-idiomatic
`(:import [path :as alias])` and the multi-vector clause
`(:import [a] [b])` are both parse errors; the parser wants one outer vector
per clause with bare paths or nested `[path :as alias]` vectors inside.

### 2.3 `(ctx/background)` alone emits Go with a missing import

```clojure
(defn main [] -> void
  (let [c (ctx/background)]
    (fmt/println c)))
```

fails with `undefined: context`. Cause: `ctx/background` / `ctx/todo` call
`e.needImport("context")`, but `"context"` is missing from the fixed package
list in `emitImports` (`transpiler.go`), so the flag is silently dropped. The
other `ctx/*` forms mask the bug because the `_ctx` pseudo-key path adds
`context` separately. The golden file
`internal/transpiler/testdata/context_propagation.go.golden` is itself invalid
Go (uses `context.Background()` with no import) — golden tests compare text
and never compile, so this was invisible.

### 2.4 The "no go.mod" error message recommends a fix that doesn't fix it

`glisp build` without a `go.mod` says:

```
build failed: no go.mod found — glisp builds with the Go toolchain, which needs a module.
  fix: run `glisp mod init <module-path>` in this directory (or a parent), then build again
```

But `glisp mod init` writes only `glisp.mod`, never `go.mod` — following the
suggested fix reproduces the identical error. The actual fix (`go mod init` +
`go get` per dependency) is undocumented.

### 2.5 Doc bug: `context/background` as a bare argument

CLAUDE.md's module-wrapping example passes `context/background` (a symbol) as
an argument: `(pgx/connect context/background url)`. That emits
`pgx.Connect(context.Background, url)` — the *function value*, not a call —
which cannot compile. It must be the built-in call form `(ctx/background)`.

## 3. Friction inventory (works as designed, but costs adoption)

### 3.1 Adding a Go dependency is a four-step manual ritual

To use `github.com/google/uuid` today:

```
glisp mod init myapp        # glisp.mod (does not help the Go toolchain)
go mod init myapp           # the file the toolchain actually needs
go get github.com/google/uuid
glisp build main.glsp
```

Three places where this should have been one step:

- **`glisp get` rejects Go packages.** `glisp get github.com/google/uuid@v1.6.0`
  downloads the tarball, then dies with *"no .glsp files found"*. The tool is
  inches from doing the right thing — it has the package and the version in
  hand and gives up.
- **App-level `go-require` is dead weight.** `glisp.mod` accepts a
  `go-require (…)` block, but `ResolveDeps` only processes `require` (glisp
  modules). Go requirements declared by the *app itself* are never wired into
  `go.mod`; the propagation only happens for downloaded wrapper modules via
  `GetModule`. Declaring your Go deps in `glisp.mod` does nothing.
- **Nothing creates `go.mod`.** Not `mod init`, not `build`, despite `build`
  knowing exactly what's missing (§2.4).

### 3.2 Module downloads bypass the Go module ecosystem

`module.Download` fetches GitHub release tarballs over plain HTTPS. That means:
github.com only (hard error for any other host), no GOPROXY, no checksum
database, no private-module auth (`GOPRIVATE`/`.netrc`), no semver resolution
(`@latest`), no pseudo-versions. The Go toolchain solves all of this; for the
*Go-package* half of dependencies we can simply delegate to `go get` and
inherit the entire ecosystem.

### 3.3 Multi-segment stdlib packages break the "no imports needed" promise

`(filepath/join "a" "b")` emits `import "filepath"` → *"package filepath is
not in std"*. The auto-import mechanism uses the qualifier verbatim as the
import path, which only works for single-segment packages plus the hardcoded
built-in list. The workaround — `(:import [path/filepath])` — works but is
undiscoverable, and the failure is a raw Go error.

### 3.4 `any` values into typed Go parameters

`(strings/to-upper (first xs))` → *"cannot use s (variable of interface type
any) as string value"*. Expected under ADR-007; the fix is `(as string s)` or
a converter (`str`, `int`). Similarly a runtime-built `[]any` cannot be passed
where Go wants `[]string` (`strings.Join` — though the glisp-native `str/join`
covers that case). The gradual-typing machinery already absorbs much of this;
the remainder is the documented, accepted cost. **No change proposed** — but
the diagnostics could name the glisp-level fix (§4.4).

### 3.5 Unknown multi-return functions in value position

Known multi-return built-ins get a friendly transpile-time diagnostic, but an
unknown interop fn — `(let [f (os/create "x")] …)` — yields Go's *"assignment
mismatch: 1 variable but os.Create returns 2 values"*. Line-mapped, but
Go-worded. (Same class as ADR-011's absorbed rows; the unknown-fn case is
acknowledged there as unresolved.)

### 3.6 Method vs field guessing

`(.Timeout client)` on a field → *"invalid operation: cannot call
non-function"*. The `.X` / `.-X` distinction is inherited from ClojureScript
and is fine once known, but the failure mode is a raw Go error. Cheap
improvement: pattern-match this Go error in `buildError` and suggest `(.-X …)`.

### 3.7 Variadic spreading needs a hand-written Go bridge

Calling `append`-style or `(fmt.Errorf args...)`-style variadic APIs with a
runtime collection requires the documented `bridge.go` pattern. `apply` exists
but routes through `_glispApply` (glisp fns only). This is real friction for
wrapper-module authors, though not for app code. Candidate future form:
a spread marker in call position (e.g. `(pgx/query conn sql & args)`), emitting
`args...` when the value is a slice. Needs design care; out of scope here.

## 4. Proposal: the frictionless path

Target experience — two steps, total:

```
$ glisp get github.com/google/uuid          # one command
```

```clojure
(ns main
  (:import [github.com/google/uuid]))        ; one declaration

(defn main [] -> void
  (fmt/println (uuid/new-string)))           ; kebab-case, no wrapper needed
```

`glisp build` / `glisp run` just work from there, including on a fresh clone
(deps re-resolved from `glisp.mod`).

### 4.1 `glisp get` learns Go packages (P1 — the headline)

When the target of `glisp get` is not a glisp module, treat it as a Go
package and delegate to the Go toolchain:

1. Ensure `go.mod` exists (create it from `glisp.mod`'s module path; create
   `glisp.mod` too if absent).
2. Run `go get <pkg>[@version]` in the project dir — inheriting GOPROXY,
   sumdb, GOPRIVATE, semver/`@latest` resolution, and non-GitHub hosts for free.
3. Record the resolved version under `go-require (…)` in `glisp.mod`, so the
   glisp-level manifest stays the single source of truth.

Detection order: try `glisp.mod`-style resolution first (cheap HEAD or cached
check); on "not a glisp module", fall back to `go get`. An explicit flag
(`glisp get -go <pkg>`) can force the Go path, but the default should guess
right.

### 4.2 `glisp build`/`run` self-heal the Go module (P2)

At the top of every build path (alongside the existing `ResolveDeps` call):

1. If `glisp.mod` exists but `go.mod` doesn't → generate `go.mod` from it
   (module path, `go` directive).
2. Sync every `go-require` entry into `go.mod` (`go mod edit -require` — same
   code path `GetModule` already uses for wrapper-module propagation).

This makes `glisp.mod` + `*.glsp` a *sufficient* checkout: `go.mod` becomes a
derived artifact, fresh clones build with one command, and the §2.4 error
message loop disappears (with `glisp mod init` also writing `go.mod`, the
message becomes true).

### 4.3 Fix qualifier resolution (P3 — the §2 bug cluster)

In `isModuleAlias` / import emission:

- Treat a trailing `/vN` (N ≥ 2, all digits) segment per the Go convention:
  the default qualifier of `github.com/jackc/pgx/v5` is `pgx`.
- Match `ImportSpec.Alias` in addition to the path-derived qualifier.
- Add `"context"` to the `emitImports` package list (§2.3).
- Accept the Clojure-shaped clause forms: `(:import [path :as alias])` and
  multiple vectors per clause.
- Compile the golden-file corpus once in CI (`go vet` or `go build` over
  `testdata/*.golden` as a slow test) so invalid generated Go can't hide
  behind text-only comparison again.

### 4.4 Glisp-level diagnostic for unknown qualifiers (P4)

When `directImports` would emit a bare import for a qualifier that is not a
known stdlib package, fail at *transpile time* with a position-tagged error:

```
unknown package 'filepath' (at 5:16) — declare it in ns:
  (:import [path/filepath])
```

Implementation: embed the stdlib package list (a generated map of last-segment
→ full path; unique entries like `filepath` → `path/filepath` can auto-import
directly, ambiguous ones like `rand` → {`math/rand`, `crypto/rand`} produce a
"did you mean" error). This turns the worst remaining raw-Go failure into a
guided fix, consistent with the central-arity-gate philosophy: validate at the
glisp layer, keep `//line` mapping as the safety net, not the UX.

### 4.5 Small ergonomics (P5)

- `buildError`: pattern-match *"cannot call non-function"* → suggest `(.-Field obj)`;
  *"assignment mismatch: 1 variable but … returns 2 values"* → suggest `if-err`.
- Fix the CLAUDE.md example: `context/background` → `(ctx/background)` (§2.5).
- Document the `(:import [path/filepath])` requirement for multi-segment
  stdlib packages until 4.4 lands.

## 5. What deliberately stays the same

Per the ADR-012 charter, these were considered and *not* proposed:

- **`(as T v)` for typed APIs** — gradual typing's explicit boundary. Auto
  insertion would require type inference (anti-roadmap).
- **`.Method` / `.-Field` / `Type.` syntax** — proven ClojureScript precedent,
  one obvious way each. Only the *error message* improves (4.5).
- **The bridge pattern for variadic/complex APIs** — a hand-written `bridge.go`
  remains the escape hatch for wrapper modules. A spread marker (§3.7) is
  noted as a possible future form but needs its own proposal.
- **Wrapper glisp modules (`go-require` + `:import`)** — still the right shape
  for shared, ergonomic APIs (e.g. `pgxdb`); P1/P2 make the underlying
  plumbing they rely on work for plain apps too.

## 6. Suggested sequencing

| Step | Scope | Risk |
|---|---|---|
| P3 bug cluster (§2.1–2.3, alias parsing, golden-compile check) | small, isolated | low |
| P2 self-healing `go.mod` (+ `mod init` writes both files) | `compiler.go`, `modfile.go` | low |
| P1 `glisp get` for Go packages | `resolver.go`, `main.go` | medium |
| P4 unknown-qualifier diagnostic + stdlib map | transpiler | medium |
| P5 error-message and doc fixes | trivial | none |

P3+P2+P5 alone would make every pattern currently documented in CLAUDE.md
actually work end-to-end; P1 makes the ecosystem feel native; P4 closes the
last common raw-Go failure mode.
