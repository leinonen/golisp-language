# glisp — CLAUDE.md

Clojure-style language that transpiles to Go. `.glsp` files → Go source → binary via standard `go build`.

## Pipeline

```
source.glsp → lexer → parser → transpiler → Go source → gofmt → go build → binary
```

## Key files

| File | Role |
|---|---|
| `internal/ast/nodes.go` | All AST node types — everything imports this |
| `internal/lexer/lexer.go` | Tokenizer; `^T` → `TokenTypeAnnot` |
| `internal/parser/parser.go` | Tokens → AST nodes |
| `internal/transpiler/transpiler.go` | `Emitter` struct, two-pass `emitFile`, dispatch |
| `internal/transpiler/emit_decl.go` | `def`, `defn`, `defstruct`, `definterface`, `defmethod` |
| `internal/transpiler/emit_expr.go` | `fn`, `let`, `if`, `cond`, `do`, built-ins |
| `internal/transpiler/emit_concurrency.go` | `go`, `go-val`, `par`, `defer`, `chan`, `send!`, `recv!`, `recv-ok!`, `close!`, `select!` (+ `:timeout`), `for-chan`, `with-lock`, `if-err`, method/field/struct interop |
| `internal/transpiler/emit_loop.go` | `loop`/`recur` → `for` |
| `internal/transpiler/emit_types.go` | `identToGo`, `typeExprToGo`, `qualifiedTypeToGo`, `zeroValueFor` |
| `internal/transpiler/emit_runtime.go` | `glispRuntime` (always), `glispSortRuntime`, `glispStrRuntime`, `glispJsonRuntime`, `glispEnvRuntime`, `glispFileRuntime`, `glispReRuntime`, `glispSetRuntime` (conditional) |
| `internal/formatter/formatter.go` | AST → formatted glisp source; `Format(src)` public API |
| `internal/compiler/compiler.go` | Orchestrates pipeline: `Compile`, `CompileAndBuild`, `CompileDir`, `CompileTest`, `TranspileDir`, `GetModule`, `ResolveDeps` |
| `internal/module/modfile.go` | `glisp.mod` parsing/writing: `ReadModFile`, `WriteModFile`, `InitModFile`; `GoRequire` type, `AddGoRequire` |
| `internal/module/resolver.go` | Module download (GitHub tar.gz), go.mod wiring: `Download`, `EnsureGoMod`, `RegisterInGoMod`, `IsCached` |
| `cmd/glisp/main.go` | CLI: `print`, `compile`, `build`, `test`, `fmt`, `get`, `mod` subcommands |
| `web/web.go` | Ring adapter, routing, middleware, request helpers, static files, graceful shutdown — plain Go, not glisp |
| `cmd/glisp-lsp/main.go` | LSP server entry point — JSON-RPC 2.0 over stdio |
| `internal/lsp/server.go` | JSON-RPC dispatch, doc state, handler wiring |
| `internal/lsp/hover.go` | Hover provider + `buildSymbolTable`, `symbolAtPosition` helpers |
| `internal/lsp/definition.go` | Jump-to-definition provider |
| `internal/lsp/completion.go` | Completion provider + `prefixAtPosition` |
| `internal/lsp/diagnostics.go` | Parse error → LSP diagnostic push |
| `internal/lsp/builtins.go` | Doc map for built-in hover + completion detail (includes `web/Request`, `web/Response`, `web/Handler` type entries) |

## Important design decisions

**Two-pass emission**: declarations emitted to a side buffer first so built-in import needs (`fmt`, `errors`, `encoding/json`, …) are discovered before writing the package header. See `emitFile` in `transpiler.go`.

**Auto-import of stdlib packages**: No stdlib package ever needs a user `(:import […])` declaration. The `Emitter` tracks two maps: `builtinImports` (set by explicit `needImport` calls in built-in form handlers — used to gate runtime helper blocks) and `directImports` (set by `emitExpr` → `*ast.Symbol` for any `pkg/fn` symbol the user writes directly, and by `emitStructLitExpr` for struct type names). `emitImports` emits `builtinImports` with `runtimeOnlyPkgs` filtering (multi-file mode), then emits ALL `directImports` unconditionally. `isModuleAlias` prevents emitting a bare `import "web"` when the user has declared `golisp/web` as a module import — only packages whose alias matches no entry in `e.imports` / `e.requires` go into `directImports`. Result: `(:import […])` in `ns` is only needed for external module packages (e.g. `golisp/web`, `github.com/jackc/pgx/v5`).

**`#(...)` anonymous function shorthand**: `#(+ % 1)` → `(fn [_anonP1] (+ _anonP1 1))`. Implemented as a reader-level expansion in `internal/parser/anon_fn.go` — no new AST node; produces a plain `FnExpr`. `%` and `%1` both map to `_anonP1`; `%2`…`%N` map to `_anonP2`…`_anonPN`; `%&` maps to `_anonPRest` (variadic rest param). The parser walks the body with `collectAnonArgs` to find the max N, then substitutes symbols with `replaceAnonArgs`. Nested `fn`/`#()` forms are **not** recursed into (own scope). Single-expression body: `#(get % "name")`. Multi-form vector body: `#([(get % "name") (get % "stock")])` (one form = the vector literal). `glisp fmt` normalises `#(...)` back to `(fn [...] ...)` — no round-trip.

**Destructuring**: `LetBinding.Pattern` is `*Symbol`, `*VectorLit` (sequential: `[[a b] coll]` → `_glispGet(tmp, int64(i))`), or `*MapLit` (map: `[{k :key} m]` → `_glispGet(tmp, "key")`). `Param.Pattern` (non-nil) enables the same in `fn`/`defn`/`defmethod` params — a temp name `_pN` is used in the Go signature and bindings are emitted at the top of the function body. Both forms use the existing `_glispGet` runtime helper. A `_` destructure element is skipped (emitting `_ := …` is illegal Go).

**`if-let` / `when-let`**: `(if-let [pat expr] then else?)` / `(when-let [pat expr] body...)` (`emit_expr.go`). Binds `pat` (single symbol, or a `_glispGet`-based destructure pattern) from `expr`, then branches on **`!= nil`** (nil-guard, matching `nil?`). Truthy branch gets the bindings in scope; destructured names are emitted *inside* that branch only, so they don't leak into the else/nil branch. `if-let` with no else and `when-let`'s false case return `nil`. Constraint: binding a non-nilable concrete type (e.g. a raw `int`) won't compile against `!= nil` — bind `any`-typed values (map lookups, find-fns).

**`let-or`**: `(let-or [name1 expr1 fallback1 name2 expr2 fallback2 ...] body...)` (`emit_expr.go`). Flat sequential nil-guard bindings — each binding evaluates `expr`, checks `== nil`, and returns `fallback` immediately if nil. Flattens chains of `if-let`/`if (= x nil)` validation guards into a single level. All names are in scope for subsequent bindings and the body. Use for validating request fields before a DB call. AST: `LetOrExpr`/`LetOrBinding` in `ast/nodes.go`.

**Statement vs expression position**: `let`/`if`/`do`/`when`/`cond` in statement position emit as plain Go blocks. In expression position they wrap in an IIFE `func() any { ... }()`. `emitStmtNode` handles statement position; `emitExpr` handles expression position.

**Return position**: `emitReturnNode` handles tail-position nodes. `loop` in return position emits `return value` directly (no `any` temp var). The `loopInReturn bool` field tracks this.

**Loop binding types**: Loop binding vars initialized with a collection literal (`[]`, `{}`, `#{}`) are declared as `var name any = init` so that recur can rebind them with any-returning helpers (e.g. `_glispConj` returns `any`). Scalar inits (`0`, `""`, etc.) keep `:=` so Go infers the concrete type and allows direct arithmetic.

**`->` in identifiers**: `ring->handler` → `ringToHandler`. Pre-processed with `strings.ReplaceAll(s, "->", "-To-")` before camelCase conversion in `identToGo`.

**Package-qualified naming**: glisp source uses lowercase-hyphenated names (`fmt/println`, `web/json-response`). `identToGo` applies `fnToGo` to the part after `/`: if all-lowercase → PascalCase (`println` → `Println`, `json-response` → `JsonResponse`); if any uppercase → pass through as-is (backward compat). Type annotations (`^web/Request`) go through `qualifiedTypeToGo` (slash→dot only) and are unaffected.

**Type annotations**: `^(chan int)` needs parens because `chan` followed by space would confuse the lexer. `^[string error]` uses brackets to denote multi-return `(string, error)`. `^web/Request` uses slash notation for package-qualified types — `typeExprToGo` converts `pkg/Type` → `pkg.Type` via `qualifiedTypeToGo`.

**Runtime helpers**: `_glispGet`, `_glispAssoc`, etc. are appended to every generated file — no separate runtime package to link. Conditional blocks are appended only when the corresponding built-ins are used, gated by `builtinImports` keys:

| Key | Runtime block | Real imports |
|---|---|---|
| `"sort"` | `glispSortRuntime` | `sort` |
| `"strings"` | `glispStrRuntime` | `strings` |
| `"encoding/json"` | `glispJsonRuntime` | `encoding/json`, `fmt` |
| `"net/http"` | `glispHttpRuntime` | `net/http`, `io`, `strings`, `fmt` |
| `"os"` | `glispEnvRuntime` | `os`, `fmt` |
| `"_file"` | `glispFileRuntime` | `os`, `fmt` (pseudo-key; no real `_file` import) |
| `"regexp"` | `glispReRuntime` | `regexp`, `fmt` (runtime-only; not in user-file imports) |
| `"_set"` | `glispSetRuntime` | none (pseudo-key) |
| `"data"` | `glispDataRuntime` | `fmt` |
| `"_atom"` | `glispAtomRuntime` | `sync` (pseudo-key) |
| `"_ctx"` | `glispCtxRuntime` | `context`, `time` (pseudo-key) |

Pseudo-keys (`"_file"`, `"_set"`, `"_atom"`, `"_ctx"`) are never added as real Go imports — they only gate which runtime block is emitted. `"regexp"` is in `runtimeOnlyPkgs` so it appears only in the shared `glisp_runtime.go` in multi-file mode, not in individual user files. For multi-file builds (`glisp build dir/`), helpers are instead written once to `glisp_runtime.go` in the same directory via `transpiler.RuntimeSource`; individual files use `TranspileNoRuntime` which sets `emitRuntime=false`.

**`json/encode` / `json/decode`**: built-in forms (no AST node needed — dispatched by symbol name in `emitCallExpr`). Both return multi-value `(value, error)` and are designed for use with `if-err`. `json/decode` returns `any` so it handles both JSON objects and arrays.

**`os/env`**: built-in form dispatched by symbol name. `(os/env "VAR")` → `os.Getenv`; `(os/env "VAR" "default")` → `os.LookupEnv` with fallback. Returns `string`. Appends `glispEnvRuntime` (gated on `builtinImports["os"]`); also marks `"fmt"` needed in single-file mode since the runtime helper uses `fmt.Sprintf`.

**File I/O built-ins**: `read-file`, `write-file`, `append-file`, `file-exists?`, `list-dir`, `mkdir` — dispatched by symbol name in `emitCallExpr`. All call `e.needImport("_file")` (a pseudo-key). In single-file mode, `emitFile` also calls `needImport("os")` and `needImport("fmt")` before emitting imports; in multi-file mode `RuntimeSource` adds those. `read-file` returns `(string, error)`; `write-file`/`append-file`/`mkdir` return `error` only (not a pair), so use plain `let`/`when` to check them rather than `if-err`.

**`log/slog` built-ins**: `log/info`, `log/debug`, `log/warn`, `log/error` — void in Go (`slog.Info` etc. return nothing). Follow the same pattern as `fmt/println`: `emitSlogCall` emits the raw call; `emitCallExpr` wraps in `func() any { …; return nil }()` for expression position; `emitStmtNode` and `emitReturnExpr` in `transpiler.go` special-case them to emit the direct call (no IIFE, no `return`). Adds `"log/slog"` to imports (not runtime-only — appears in user files directly). No import declaration needed in glisp source.

**Regex built-ins**: `re/match`, `re/find`, `re/find-all`, `re/replace`, `re/split` — dispatched by symbol name, calling `e.needImport("regexp")`. `"regexp"` is in `runtimeOnlyPkgs` so it only appears in `glisp_runtime.go` in multi-file mode. All helpers use `regexp.MustCompile` — invalid patterns panic at runtime. `re/find` returns `any` (nil on no match). Go uses RE2 syntax — no lookaheads/lookbehinds.

**Error wrapping**: `wrap-error` emits `fmt.Errorf("%s: %w", msg, err)` inline — needs `"fmt"` import. `errors/is?` emits `errors.Is(err, target)` via `emitRuntimeCall("errors.Is", ...)` — `"errors"` is already in the tracked import list.

**Context built-ins**: `ctx/background` and `ctx/todo` emit inline (`context.Background()` / `context.TODO()`) and call `e.needImport("context")` for the real import. `ctx/with-cancel`, `ctx/with-timeout`, `ctx/cancel!`, `ctx/value`, `ctx/with-value` call `e.needImport("_ctx")` — a pseudo-key that gates `glispCtxRuntime` and auto-imports `context` + `time`. Multi-return forms (`ctx/with-cancel`, `ctx/with-timeout`) return `[]any{ctx, cancel}` for use with vector destructuring. `ctx/cancel!` type-asserts to `context.CancelFunc` internally so callers don't need to; it returns `any` (nil) so it works in both statement and expression position. No import declaration ever needed in glisp source.

**web API**: all web functionality lives in `web/web.go` as plain Go — no special transpiler forms. `Request` and `Response` are type aliases for `map[string]any` (use `^web/Request` / `^web/Response` in glisp annotations). `Handler` is `func(Request) Response`. Middleware signature is `func(Handler) Handler`. `wrap(h Handler, mws ...Middleware)` applies middlewares outermost-first. `wrap-json` stores parsed body in `req["json-body"]`; `wrap-auth` stores the Bearer token in `req["identity"]`. `serve-files` bridges Ring ↔ `http.FileServer` via `httptest.ResponseRecorder`. `serve-graceful` traps SIGINT/SIGTERM and shuts down with a 5 s context deadline. HTTP route helpers: `(web/get path handler)`, `(web/post path handler)`, etc.

**Concurrency primitives** — six forms beyond the basic `go`/`chan`/`send!`/`recv!`/`close!`/`select!`:

| Form | Emits | Notes |
|---|---|---|
| `(go-val body...)` | IIFE → `chan any` with buffered 1-slot channel + goroutine | Returns immediately; caller `(recv! ch)` to collect the result. Body runs in a goroutine that sends via `_ch <- func() any { return ... }()`. |
| `(par body1 body2 ...)` | `{ var _wg sync.WaitGroup; _wg.Add(n); go func()...}` | N bodies run concurrently; `_wg.Wait()` blocks until all finish. Auto-imports `"sync"`. |
| `(for-chan [x ch] body...)` | `for x := range ch { body }` | Iterates until channel is closed. Separate from `doseq` — `for x := range slice` gives index, not value. |
| `(recv-ok! ch)` | `func() []any { _v, _ok := <-ch; return []any{_v, _ok} }()` | Use with `[[val ok] (recv-ok! ch)]` destructuring. Check with `(= ok true)` — `ok` is `any`, not `bool`. |
| `(with-lock mu body...)` | `func() any { mu.Lock(); defer mu.Unlock(); body }()` | IIFE ensures unlock even on panic. `mu` is evaluated twice — use a symbol, not a complex expr. Auto-imports `"sync"`. |
| `:timeout ms` in `select!` | `case <-time.After(ms * time.Millisecond):` | Add as a case in any `select!`; fires after `ms` milliseconds. Auto-imports `"time"`. |

**`doseq` collection handling**: uses `_glispToSlice(coll)` (which accepts `any`) rather than `coll.([]any)` type assertion, so it works when the collection is already statically typed as `[]any` (e.g. result of `map`, `filter`, or a literal binding via `let`).

**`defmethod` — receiver methods**: `(defmethod ^*ReceiverType name [self params...] ^RetType body)` emits `func (self *ReceiverType) Name(params) RetType { body }`. The `^` annotation before the method name is the receiver type (`^T` value receiver, `^*T` pointer receiver). The first element of the params vector is the receiver variable name; remaining params are regular params. Together with `definterface` and `defstruct`, this is the full Go interface/struct/method triad.

**`any`-type constraints** — the transpiler emits `any` for most values retrieved at runtime (map lookups, collection elements, loop vars). This causes several Go compile errors:

| Situation | What breaks | Fix |
|---|---|---|
| `(len w)` where `w` is `any` | `len` needs a concrete type | `(len (str w))` for strings; for slices use `(int (reduce (fn [n _] (+ (int n) 1)) 0 xs))` |
| `(if x ...)` where `x` is a non-bool `any` | Go `if` requires boolean | `(= x nil)` or `(not= x nil)` for nil checks |
| `(fn [c] ... (go ...) )` — `go` as last expr in a `func(...) any` | missing return | add `nil` as the last expr: `(let [...] (go ...) nil)` |
| `(let [...] (select! ...) )` — `select!` as last expr in `let` body | missing return — `select` is statement-only | add `nil` after: `(let [...] (select! ...) nil)` |
| `(if ok ...)` where `ok` from `[[val ok] (recv-ok! ch)]` | `ok` is `any` not `bool`; Go `if` requires bool | `(if (= ok true) ...)` |
| any multi-return Go fn (e.g. `os/create`) as the tail of a `func(...) any` closure | `(T, error)` can't coerce to `any` | `(do (os/create ...) nil)` — note: `fmt/println` and `fmt/print` are handled automatically |
| `(defn ^int f [] (reduce ...))` | `_glispReduce` returns `any`, not `int` | either use `^any` return type and cast at call sites, or wrap: `(int (reduce ...))` inline |
| passing `[]T` (concrete slice) to `reduce`/`map`/`filter` | `_glispToSlice` only handles `[]any`; concrete slices iterate as nil | in Go bridge code, convert: `result := make([]any, len(s)); for i,v := range s { result[i]=v }` |
| `(defn f [] ...)` with no `^RetType`, void-ish body | Go generates `func f()` (void); using it in return position fails | add explicit `^any` annotation if the fn is called in expression/return position |

## Formatter

`glisp fmt` pretty-prints `.glsp` source from the parsed AST. **Leading `;` and `;;` comments are preserved** (attached to the form they precede; blank lines between the comment and the form are normalized away). Trailing inline comments (after code on the same line) are not preserved. `;;;` docstrings are preserved via `DefnDecl.Doc`. Algorithm: try inline rendering first; use it if `indent*2 + len <= 80`, else multi-line. Map literals with >1 pair are always multi-line with value-column alignment. `defn`/`defstruct`/`definterface`/`defmethod`/`deftest`/`cond` are always multi-line.

```
glisp fmt file.glsp          # format in-place
glisp fmt --check file.glsp  # exit 1 if not already formatted
make fmt-glsp                # format all examples/*.glsp
```

## Testing

```
go test ./...                              # all tests
go test ./internal/transpiler/... -update  # regenerate golden files
make examples                              # compile + build examples
```

Golden files live in `internal/transpiler/testdata/`. Each `.glsp` has a matching `.go.golden`. Run with `-update` to regenerate after intentional output changes.

## Adding a new special form

1. Add AST node(s) to `internal/ast/nodes.go`
2. Parse it in `internal/parser/parser.go` (dispatch by head symbol)
3. Add emit method to the appropriate `emit_*.go` file
4. Wire into `emitExpr` switch in `transpiler.go`
5. If it can appear in statement position, also wire into `emitStmtNode`
6. Add a snippet test in `transpiler_test.go` and/or a golden file

## Module system

glisp modules are GitHub repos (or local directories) containing `.glsp` files + a `glisp.mod`. The module path is also the Go import path.

**`glisp.mod`** — plain-text dependency file at project root:
```
module github.com/myuser/myapp

require (
  github.com/user/mathlib v1.0.0
)
```

**ns syntax** — `:require` for glisp modules; `:import` only for external module packages (stdlib is auto-imported):
```clojure
(ns main
  (:import [golisp/web])
  (:require [github.com/user/mathlib]))
```
The qualifier in glisp code is the last path segment: `(mathlib/add 3 4)`.

**Export convention** — library modules must use PascalCase names for exported functions:
```clojure
; in the library (github.com/user/mathlib):
(defn ^int Add [^int a ^int b] (+ a b))

; consumer writes lowercase — fnToGo converts add → Add:
(mathlib/add 3 4)  ; → mathlib.Add(3, 4)
```
Private (unexported) helpers use kebab-case as normal: `(defn helper [...] ...)`.

**CLI commands**:
```
glisp mod init [module-path]         create glisp.mod for a new module/app
glisp get <module>[@version]         download + transpile + register a dependency
glisp build <dir/>                   auto-fetches missing deps from glisp.mod before building
```

**How deps are wired**: `glisp get` downloads the GitHub archive to `~/.glisp/pkg/mod/<path>@<version>/`, transpiles `.glsp` → `.go` there, writes a `go.mod` for the module, then runs `go mod edit -require` and `-replace` in the project's `go.mod` so standard `go build` can find it.

**Writing a module**:
1. Create a repo with `.glsp` files and `glisp.mod` (`(module github.com/you/lib)`)
2. Use PascalCase for all exported function/type names
3. Tag a release (`v1.0.0`) on GitHub
4. Consumers run `glisp get github.com/you/lib@v1.0.0`

**Wrapping a Go package** — modules can declare Go package dependencies with `go-require`. `glisp get` propagates them into both the module's own `go.mod` and the project's `go.mod`:

```
module github.com/leinonen/pgxdb

go-require (
  github.com/jackc/pgx/v5 v5.7.2
)
```

In the module's `.glsp` files, import the Go package via `:import` (not `:require` — that's for glisp modules):
```clojure
(ns db
  (:import [context]
           [github.com/jackc/pgx/v5]))

(defn ^[any error] Connect [^string url]
  (pgx/connect context/background url))

(defn ^[any error] Exec [^any conn ^string sql]
  (let [typed (as ^*pgx/Conn conn)]
    (.Exec typed context/background sql)))
```

Key rules for Go-wrapping modules:
- Use `(:import [...])` for external/vendor Go packages (stdlib is auto-imported; no declaration needed); `(:require [...])` only for other glisp modules
- Return opaque Go types as `any`; callers type-assert with `(as ^*pkg/Type val)` when methods are needed
- Type annotation syntax for pointer-to-external-type: `^*pgx/Conn` → `*pgx.Conn` (slash→dot via `qualifiedTypeToGo`)
- The package qualifier comes from the last path segment: `(:import [github.com/jackc/pgx/v5])` → qualifier is `pgx`
- Go interop primitives available: `(.Method obj args)` for method calls, `(.-Field obj)` for field access, `(Type. {:field val})` for struct literals, `(as ^T val)` for type assertions
- **Bridge pattern for variadic Go APIs**: write a hand-written `bridge.go` (same package) with unexported Go helpers that handle variadic spreading and type assertions; call them from glisp as unqualified names. Unqualified calls use `identToGo` (camelCase, lowercase first letter), so `(bridge-query ...)` → `bridgeQuery`. The bridge functions must be unexported (lowercase) — they're only accessible within the package.
- **`[]any` for collections**: glisp's `_glispToSlice` only handles `[]any`. If a Go bridge function returns a concrete slice type (e.g. `[]map[string]any`), convert it to `[]any` before returning so `reduce`/`map`/`filter` work correctly.

## Go module

Go module name: `golisp`. Go version in `go.mod`.
