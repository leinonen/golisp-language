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
| `internal/transpiler/emit_concurrency.go` | `go`, `defer`, `chan`, `send!`, `recv!`, `select!`, `if-err`, method/field/struct interop |
| `internal/transpiler/emit_loop.go` | `loop`/`recur` → `for` |
| `internal/transpiler/emit_types.go` | `identToGo`, `typeExprToGo`, `qualifiedTypeToGo`, `zeroValueFor` |
| `internal/transpiler/emit_runtime.go` | `glispRuntime` (always), `glispSortRuntime`, `glispStrRuntime`, `glispJsonRuntime`, `glispEnvRuntime` (conditional) |
| `internal/formatter/formatter.go` | AST → formatted glisp source; `Format(src)` public API |
| `internal/compiler/compiler.go` | Orchestrates pipeline: `Compile`, `CompileAndBuild`, `CompileDir`, `CompileTest`, `TranspileDir`, `GetModule`, `ResolveDeps` |
| `internal/module/modfile.go` | `glisp.mod` parsing/writing: `ReadModFile`, `WriteModFile`, `InitModFile` |
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

**Destructuring**: `LetBinding.Pattern` is `*Symbol`, `*VectorLit` (sequential: `[[a b] coll]` → `_glispGet(tmp, int64(i))`), or `*MapLit` (map: `[{k :key} m]` → `_glispGet(tmp, "key")`). `Param.Pattern` (non-nil) enables the same in `fn`/`defn`/`defmethod` params — a temp name `_pN` is used in the Go signature and bindings are emitted at the top of the function body. Both forms use the existing `_glispGet` runtime helper. A `_` destructure element is skipped (emitting `_ := …` is illegal Go).

**`if-let` / `when-let`**: `(if-let [pat expr] then else?)` / `(when-let [pat expr] body...)` (`emit_expr.go`). Binds `pat` (single symbol, or a `_glispGet`-based destructure pattern) from `expr`, then branches on **`!= nil`** (nil-guard, matching `nil?`). Truthy branch gets the bindings in scope; destructured names are emitted *inside* that branch only, so they don't leak into the else/nil branch. `if-let` with no else and `when-let`'s false case return `nil`. Constraint: binding a non-nilable concrete type (e.g. a raw `int`) won't compile against `!= nil` — bind `any`-typed values (map lookups, find-fns).

**Statement vs expression position**: `let`/`if`/`do`/`when`/`cond` in statement position emit as plain Go blocks. In expression position they wrap in an IIFE `func() any { ... }()`. `emitStmtNode` handles statement position; `emitExpr` handles expression position.

**Return position**: `emitReturnNode` handles tail-position nodes. `loop` in return position emits `return value` directly (no `any` temp var). The `loopInReturn bool` field tracks this.

**`->` in identifiers**: `ring->handler` → `ringToHandler`. Pre-processed with `strings.ReplaceAll(s, "->", "-To-")` before camelCase conversion in `identToGo`.

**Package-qualified naming**: glisp source uses lowercase-hyphenated names (`fmt/println`, `web/json-response`). `identToGo` applies `fnToGo` to the part after `/`: if all-lowercase → PascalCase (`println` → `Println`, `json-response` → `JsonResponse`); if any uppercase → pass through as-is (backward compat). Type annotations (`^web/Request`) go through `qualifiedTypeToGo` (slash→dot only) and are unaffected.

**Type annotations**: `^(chan int)` needs parens because `chan` followed by space would confuse the lexer. `^[string error]` uses brackets to denote multi-return `(string, error)`. `^web/Request` uses slash notation for package-qualified types — `typeExprToGo` converts `pkg/Type` → `pkg.Type` via `qualifiedTypeToGo`.

**Runtime helpers**: `_glispGet`, `_glispAssoc`, etc. are appended to every generated file — no separate runtime package to link. Conditional blocks (`glispSortRuntime`, `glispStrRuntime`, `glispJsonRuntime`, `glispEnvRuntime`) are appended only when the corresponding built-ins are used, gated by `builtinImports` keys (`"sort"`, `"strings"`, `"encoding/json"`, `"os"`). For multi-file builds (`glisp build dir/`), helpers are instead written once to `glisp_runtime.go` in the same directory via `transpiler.RuntimeSource`; individual files use `TranspileNoRuntime` which sets `emitRuntime=false`.

**`json/encode` / `json/decode`**: built-in forms (no AST node needed — dispatched by symbol name in `emitCallExpr`). Both return multi-value `(value, error)` and are designed for use with `if-err`. `json/decode` returns `any` so it handles both JSON objects and arrays.

**`os/env`**: built-in form dispatched by symbol name. `(os/env "VAR")` → `os.Getenv`; `(os/env "VAR" "default")` → `os.LookupEnv` with fallback. Returns `string`. Appends `glispEnvRuntime` (gated on `builtinImports["os"]`); also marks `"fmt"` needed in single-file mode since the runtime helper uses `fmt.Sprintf`.

**web API**: all web functionality lives in `web/web.go` as plain Go — no special transpiler forms. `Request` and `Response` are type aliases for `map[string]any` (use `^web/Request` / `^web/Response` in glisp annotations). `Handler` is `func(Request) Response`. Middleware signature is `func(Handler) Handler`. `wrap(h Handler, mws ...Middleware)` applies middlewares outermost-first. `wrap-json` stores parsed body in `req["json-body"]`; `wrap-auth` stores the Bearer token in `req["identity"]`. `serve-files` bridges Ring ↔ `http.FileServer` via `httptest.ResponseRecorder`. `serve-graceful` traps SIGINT/SIGTERM and shuts down with a 5 s context deadline. HTTP route helpers: `(web/get path handler)`, `(web/post path handler)`, etc.

**`defmethod` — receiver methods**: `(defmethod ^*ReceiverType name [self params...] ^RetType body)` emits `func (self *ReceiverType) Name(params) RetType { body }`. The `^` annotation before the method name is the receiver type (`^T` value receiver, `^*T` pointer receiver). The first element of the params vector is the receiver variable name; remaining params are regular params. Together with `definterface` and `defstruct`, this is the full Go interface/struct/method triad.

**`any`-type constraints** — the transpiler emits `any` for most values retrieved at runtime (map lookups, collection elements, loop vars). This causes several Go compile errors:

| Situation | What breaks | Fix |
|---|---|---|
| `(len w)` where `w` is `any` | `len` needs a concrete type | `(len (str w))` for strings; for slices use `(int (reduce (fn [n _] (+ (int n) 1)) 0 xs))` |
| `(if x ...)` where `x` is a non-bool `any` | Go `if` requires boolean | `(= x nil)` or `(not= x nil)` for nil checks |
| `(fn [c] ... (go ...) )` — `go` as last expr in a `func(...) any` | missing return | add `nil` as the last expr: `(let [...] (go ...) nil)` |
| `fmt/println` (or any multi-return Go fn) as the tail of a `func(...) any` closure | `(int, error)` can't coerce to `any` | `(do (fmt/println ...) nil)` discards the multi-return via IIFE |
| `(defn ^int f [] (reduce ...))` | `_glispReduce` returns `any`, not `int` | either use `^any` return type and cast at call sites, or wrap: `(int (reduce ...))` inline |

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

**ns syntax** — `:require` for glisp modules (alongside existing `:import` for Go stdlib):
```clojure
(ns main
  (:import [fmt])
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

## Go module

Go module name: `golisp`. Go version in `go.mod`.
