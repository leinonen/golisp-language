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
| `internal/transpiler/emit_decl.go` | `def`, `defn`, `defstruct`, `definterface` |
| `internal/transpiler/emit_expr.go` | `fn`, `let`, `if`, `cond`, `do`, built-ins |
| `internal/transpiler/emit_concurrency.go` | `go`, `defer`, `chan`, `send!`, `recv!`, `select!`, `if-err`, method/field/struct interop |
| `internal/transpiler/emit_loop.go` | `loop`/`recur` → `for` |
| `internal/transpiler/emit_types.go` | `identToGo`, `typeExprToGo`, `zeroValueFor` |
| `internal/transpiler/emit_runtime.go` | `glispRuntime` (always), `glispSortRuntime`, `glispStrRuntime`, `glispJsonRuntime` (conditional) |
| `internal/formatter/formatter.go` | AST → formatted glisp source; `Format(src)` public API |
| `internal/compiler/compiler.go` | Orchestrates pipeline, runs gofmt, runs go build |
| `cmd/glisp/main.go` | CLI: `print`, `compile`, `build`, `test`, `fmt` subcommands |
| `stdlib/web.go` | Ring adapter, routing, middleware, request helpers, static files, graceful shutdown — plain Go, not glisp |

## Important design decisions

**Two-pass emission**: declarations emitted to a side buffer first so built-in import needs (`fmt`, `errors`, `encoding/json`, …) are discovered before writing the package header. See `emitFile` in `transpiler.go`.

**Statement vs expression position**: `let`/`if`/`do`/`when`/`cond` in statement position emit as plain Go blocks. In expression position they wrap in an IIFE `func() any { ... }()`. `emitStmtNode` handles statement position; `emitExpr` handles expression position.

**Return position**: `emitReturnNode` handles tail-position nodes. `loop` in return position emits `return value` directly (no `any` temp var). The `loopInReturn bool` field tracks this.

**`->` in identifiers**: `ring->handler` → `ringToHandler`. Pre-processed with `strings.ReplaceAll(s, "->", "-To-")` before camelCase conversion in `identToGo`.

**Type annotations**: `^(chan int)` needs parens because `chan` followed by space would confuse the lexer. `^[string error]` uses brackets to denote multi-return `(string, error)`.

**Runtime helpers**: `_glispGet`, `_glispAssoc`, etc. are appended to every generated file — no separate runtime package to link. Conditional blocks (`glispSortRuntime`, `glispStrRuntime`, `glispJsonRuntime`) are appended only when the corresponding built-ins are used, gated by `builtinImports` keys (`"sort"`, `"strings"`, `"encoding/json"`).

**`json/encode` / `json/decode`**: built-in forms (no AST node needed — dispatched by symbol name in `emitCallExpr`). Both return multi-value `(value, error)` and are designed for use with `if-err`. `json/decode` returns `any` so it handles both JSON objects and arrays.

**stdlib web API**: all web functionality lives in `stdlib/web.go` as plain Go — no special transpiler forms. Middleware signature is `func(Handler) Handler`. `Wrap(h Handler, mws ...Middleware)` applies middlewares outermost-first. `WrapJson` stores parsed body in `req["json-body"]`; `WrapAuth` stores the Bearer token in `req["identity"]`. `ServeFiles` bridges Ring ↔ `http.FileServer` via `httptest.ResponseRecorder`. `ServeGraceful` traps SIGINT/SIGTERM and shuts down with a 5 s context deadline.

**`any`-type constraints** — the transpiler emits `any` for most values retrieved at runtime (map lookups, collection elements, loop vars). This causes several Go compile errors:

| Situation | What breaks | Fix |
|---|---|---|
| `(len w)` where `w` is `any` | `len` needs a concrete type | `(len (str w))` for strings; for slices use `(int (reduce (fn [n _] (+ (int n) 1)) 0 xs))` |
| `(if x ...)` where `x` is a non-bool `any` | Go `if` requires boolean | `(= x nil)` or `(not= x nil)` for nil checks |
| `(fn [c] ... (go ...) )` — `go` as last expr in a `func(...) any` | missing return | add `nil` as the last expr: `(let [...] (go ...) nil)` |
| `fmt/Println` (or any multi-return Go fn) as the tail of a `func(...) any` closure | `(int, error)` can't coerce to `any` | `(do (fmt/Println ...) nil)` discards the multi-return via IIFE |
| `(defn ^int f [] (reduce ...))` | `_glispReduce` returns `any`, not `int` | either use `^any` return type and cast at call sites, or wrap: `(int (reduce ...))` inline |

## Formatter

`glisp fmt` pretty-prints `.glsp` source from the parsed AST. **Comments are not preserved** (stripped by the lexer). Algorithm: try inline rendering first; use it if `indent*2 + len <= 80`, else multi-line. Map literals with >1 pair are always multi-line with value-column alignment. `defn`/`defstruct`/`definterface`/`deftest`/`cond` are always multi-line.

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

## Module

Module name: `golisp`. Go version in `go.mod`.
