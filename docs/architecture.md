# Transpiler architecture

```
source.glsp → lexer → parser → transpiler → Go source → gofmt → go build → binary
```

The transpiler lives in `internal/transpiler/` and is split across several files by concern:

| File | Role |
|---|---|
| `transpiler.go` | `Emitter` struct, two-pass `emitFile`, import resolution |
| `emit_decl.go` | Top-level declarations: `def`, `defn`, `defstruct`, `definterface`, `defmethod`, `deftype` |
| `emit_expr.go` | Expressions: `fn`, `let`, `if`, `cond`, `do`, `loop`/`recur`, built-ins |
| `emit_concurrency.go` | Concurrency forms: `go`, `chan`, `send!`, `recv!`, `select!`, `par`, `with-lock`, … |
| `emit_types.go` | Type annotation conversion: `identToGo`, `typeExprToGo`, `qualifiedTypeToGo` |
| `emit_runtime.go` | Inline Go runtime helpers appended to generated files |

## Two-pass emission

`emitFile` uses two passes to solve a chicken-and-egg problem: Go requires the `import` block at the top of the file, but which packages are needed is only known after emitting all declarations.

1. **Pass 1** — emit all declarations into a scratch `Emitter`. This discovers which packages are needed by setting flags in `builtinImports` and `directImports`.
2. **Pass 2** — write `package …` and `import (…)` into the real buffer using the discovered sets, then append the pass-1 output.

## Import tracking

The emitter maintains two import sets:

- **`builtinImports`** — set by `needImport("pkg")` inside built-in form handlers (e.g. `needImport("sort")` when `sort-by` is used). Used to gate which runtime helper blocks are appended.
- **`directImports`** — set when the user writes a qualified symbol directly (`fmt/println` → adds `"fmt"`). Always emitted unconditionally.

Stdlib packages never require a user `(:import […])` declaration — the emitter adds them automatically. `(:import […])` in an `ns` form is only needed for external Go module packages (`golisp/web`, `github.com/jackc/pgx/v5`, etc.).

## Statement vs expression position

Most forms (`let`, `if`, `cond`, `do`, `when`) can appear in both positions:

- **Statement position** — emitted as plain Go blocks via `emitStmtNode`.
- **Expression position** — wrapped in an immediately-invoked function literal `func() any { … }()` so the form produces a value.

The emitter tracks position through the call stack: top-level body statements go through `emitStmtNode`; anything used as an argument or binding RHS goes through `emitExpr`.

## Runtime helpers

Every generated file ends with inline Go helper functions (`_glispGet`, `_glispAssoc`, `_glispConj`, `_glispReduce`, etc.) that implement glisp's dynamic collection semantics. Conditional blocks (`glispSortRuntime`, `glispStrRuntime`, etc.) are appended only when the corresponding built-ins are actually used, keeping output lean.

For directory builds (`glisp build dir/`) the helpers are emitted once into a shared `glisp_runtime.go` file instead of duplicated in every file. Individual files are transpiled with `TranspileNoRuntime` which omits the helpers.

## Design decisions

Architecture decision records live in [`adr/`](adr/) — Go compilation target, transpiler over interpreter, positional type syntax, truthiness, dot-free method dispatch, and more.
