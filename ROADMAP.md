# glisp Roadmap

## Phase 2 — Language Completeness

### 2a. Core collection operations
- [x] `map` — `(map f coll)`
- [x] `filter` — `(filter pred coll)`
- [x] `reduce` — `(reduce f init coll)`
- [x] `take` / `drop` — slice prefix/suffix
- [x] `reverse`
- [x] `contains?` — key in map, value in slice, or substring
- [x] `some` / `every?` — predicate over collection
- [x] `sort-by` — sort slice by key fn
- [x] `flatten`
- [x] `range` — `(range n)` or `(range start end)`

### 2b. String operations
- [x] `subs` — `(subs s start end?)`
- [x] `upper-case` / `lower-case`
- [x] `trim`
- [x] `split` / `join`
- [x] `starts-with?` / `ends-with?`
- [x] `replace`
- [x] `contains?` on string (handled by unified `contains?`)

### 2c. Better error messages
- [x] Show offending source line with `^` pointer
- [x] "Did you mean?" hints for common typos (`defun` → `defn`, `lambda` → `fn`)
- [x] Distinguish parse errors from transpile errors

### 2d. Test framework
- [x] `deftest` special form → `func TestXxx(t *testing.T)`
- [x] `assert=`, `assert-true`, `assert-false`, `assert-nil`, `assert-err`
- [x] `glisp test file.glsp` CLI command

### 2e. Multi-file / namespace support
- [x] `glisp build dir/` — compile all `.glsp` files in a directory
- [x] Files sharing the same `ns` compile into the same Go package

---

## Phase 3 — Web Server

### 3a. JSON support
- [x] `json/encode` — `any` → JSON string (returns `[string error]`)
- [x] `json/decode` — JSON string → `any, error` (handles objects and arrays)
- [x] `json-response` stdlib helper

### 3b. Routing DSL
- [x] `routes`, `GET`, `POST`, `PUT`, `DELETE`, `PATCH` stdlib functions
- [x] Path params: `/users/:id` extracted into request map as `"params"`

### 3c. Middleware
- [x] `wrap-json` — parse JSON body
- [x] `wrap-cors` — CORS headers
- [x] `wrap-auth` — Bearer token extraction into `"identity"`
- [x] `wrap-timeout` — per-request context deadline
- [x] `compose` / `wrap` — `(stdlib/Wrap handler stdlib/WrapLogging stdlib/WrapCors)`

### 3d. Request helpers
- [x] `query-param`, `path-param`, `body-map`, `header`

### 3e. Static file serving
- [x] `serve-files` — `(stdlib/ServeFiles "/static/" "public/")`

### 3f. Graceful shutdown
- [x] `serve-graceful` — drains in-flight requests on SIGINT/SIGTERM

---

## Phase 4 — Developer Experience

### 4a. Formatter
- [x] `glisp fmt file.glsp` — format in-place
- [x] `glisp fmt --check` — exit non-zero if unformatted
- [x] `make fmt-glsp` Makefile target
- [x] Pretty-print over existing AST: consistent indentation, aligned map pairs

### 4b. REPL
- [x] `glisp repl` — read form, transpile, run via `go run`, print result
- [ ] Readline history and editing (up-arrow, ctrl-a/e, etc.)
- [ ] Side-effect-free def persistence — snapshot accumulated state as values, not source, so `def` side effects don't re-run on each subsequent eval
- [ ] Multi-value expression support — expressions returning `(values a b)` or Go multi-return should print all values instead of failing to compile
- [ ] String-aware paren balancing — current depth counter is confused by `(` / `)` inside string literals

### 4c. Source maps
- [ ] Emit `// glsp:line:col` comments on generated lines
- [ ] Map Go panic stack traces back to `.glsp` source locations

---

## Phase 5 — LSP

Written in Go. Speaks JSON-RPC over stdio per the LSP spec.

```
cmd/glisp-lsp/main.go
internal/lsp/
  server.go       — JSON-RPC dispatch
  analysis.go     — symbol table, scope resolution
  diagnostics.go  — parse error push
  hover.go        — hover provider
  definition.go   — jump-to-definition
  completion.go   — completion provider
```

### 5a. Diagnostics
- [x] Run lexer+parser on `textDocument/didChange`
- [x] Push errors as `textDocument/publishDiagnostics`

### 5b. Hover
- [x] Return type annotation or `defn` signature for symbol under cursor

### 5c. Jump-to-definition
- [x] Resolve symbol → `defn`/`def` location

### 5d. Completions
- [x] Top-level `defn`/`def` names, built-in forms, stdlib names

### 5e. Docstrings
- [x] `;;; doc comment` preceding a `defn` stored in AST
- [x] Surfaced in hover text and completion detail

### 5f. Rename symbol
- [ ] `textDocument/rename` — rename across all files in workspace

### 5g. Find references
- [ ] `textDocument/references` — list all call sites of a symbol

### 5h. Code actions
- [ ] `textDocument/codeAction` — quick-fixes (e.g. add missing type annotation)

---

## Phase 6 — Language Power

### 6a. Threading macros
- [x] `->` — thread-first: `(-> x (f a) (g b))` → `(g (f x a) b)`
- [x] `->>` — thread-last: `(->> x (map f) (filter g))`
- [x] Implemented as AST rewrite in `emitCallExpr` (`emit_expr.go`)

### 6b. Destructuring
- [x] Sequential: `(let [[a b c] coll] ...)` — bind by position (`_glispGet` index)
- [x] Map: `(let [{k :key} m] ...)` — bind by key (`_glispGet` string key)
- [x] In `fn`/`defn`/`defmethod` param vectors: `(fn [[x y]] ...)`, `(fn [{k :key}] ...)`

### 6c. Macro system
- [ ] `defmacro name [args] body` — define a compile-time transformation
- [ ] `macroexpand` — expand a macro call for debugging
- [ ] Requires an evaluation pass before transpilation

---

## Phase 7 — Standard Library

### 7a. HTTP client
- [ ] `http/get`, `http/post`, `http/put`, `http/delete`, `http/request`
- [ ] Returns `[response error]` for use with `if-err`
- [ ] Plain Go in `stdlib/http.go`, mirroring the `web.go` pattern

---

## Order of attack

| # | Item | Why |
|---|---|---|
| 1 | ~~6a threading macros~~ | ✓ done |
| 2 | ~~6b destructuring~~ | ✓ done |
| 3 | ~~5e docstrings~~ | ✓ done |
| 4 | 7a HTTP client | Enables full client-server programs in glisp |
| 5 | 5f–5h LSP rename/refs/actions | Full IDE experience |
| 6 | ~~4b REPL~~ | ✓ done |
| 7 | 4c source maps | Debug Go panics in .glsp terms |
| 8 | 6c macro system | Most complex; enables DSL authoring |
