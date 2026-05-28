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
| `internal/transpiler/emit_runtime.go` | `glispRuntime` constant appended to every output |
| `internal/compiler/compiler.go` | Orchestrates pipeline, runs gofmt, runs go build |
| `cmd/glisp/main.go` | CLI: `print`, `compile`, `build` subcommands |
| `stdlib/web.go` | Ring adapter — plain Go, not glisp |

## Important design decisions

**Two-pass emission**: declarations emitted to a side buffer first so built-in import needs (`fmt`, `errors`) are discovered before writing the package header. See `emitFile` in `transpiler.go`.

**Statement vs expression position**: `let`/`if`/`do`/`when`/`cond` in statement position emit as plain Go blocks. In expression position they wrap in an IIFE `func() any { ... }()`. `emitStmtNode` handles statement position; `emitExpr` handles expression position.

**Return position**: `emitReturnNode` handles tail-position nodes. `loop` in return position emits `return value` directly (no `any` temp var). The `loopInReturn bool` field tracks this.

**`->` in identifiers**: `ring->handler` → `ringToHandler`. Pre-processed with `strings.ReplaceAll(s, "->", "-To-")` before camelCase conversion in `identToGo`.

**Type annotations**: `^(chan int)` needs parens because `chan` followed by space would confuse the lexer. `^[string error]` uses brackets to denote multi-return `(string, error)`.

**Runtime helpers**: `_glispGet`, `_glispAssoc`, etc. are appended to every generated file — no separate runtime package to link.

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
