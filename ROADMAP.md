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
- [ ] Distinguish parse errors from transpile errors

### 2d. Test framework
- [x] `deftest` special form → `func TestXxx(t *testing.T)`
- [x] `assert=`, `assert-true`, `assert-false`, `assert-nil`, `assert-err`
- [x] `glisp test file.glsp` CLI command

### 2e. Multi-file / namespace support
- [ ] `glisp build dir/` — compile all `.glsp` files in a directory
- [ ] Files sharing the same `ns` compile into the same Go package

---

## Phase 3 — Web Server

### 3a. JSON support
- [ ] `json/encode` — `map[string]any` → JSON string
- [ ] `json/decode` — JSON string → `map[string]any, error`
- [ ] `json-response` stdlib helper

### 3b. Routing DSL
- [ ] `routes`, `GET`, `POST`, `PUT`, `DELETE`, `PATCH` stdlib functions
- [ ] Path params: `/users/:id` extracted into request map as `"params"`

### 3c. Middleware
- [ ] `wrap-json` — parse JSON body
- [ ] `wrap-cors` — CORS headers
- [ ] `wrap-auth` — Bearer token extraction into `"identity"`
- [ ] `wrap-timeout` — per-request context deadline
- [ ] `compose` — `(compose wrap-logging wrap-cors handler)`

### 3d. Request helpers
- [ ] `query-param`, `path-param`, `body-map`, `header`

### 3e. Static file serving
- [ ] `serve-files` — `(serve-files "public/")`

### 3f. Graceful shutdown
- [ ] `serve-graceful` — drains in-flight requests on SIGINT/SIGTERM

---

## Phase 4 — Developer Experience

### 4a. Formatter
- [ ] `glisp fmt file.glsp` — format in-place
- [ ] `glisp fmt --check` — exit non-zero if unformatted
- [ ] `make fmt-glsp` Makefile target
- [ ] Pretty-print over existing AST: consistent indentation, aligned map pairs

### 4b. REPL
- [ ] `glisp repl` — read form, transpile, run via `go run`, print result

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
- [ ] Run lexer+parser on `textDocument/didChange`
- [ ] Push errors as `textDocument/publishDiagnostics`

### 5b. Hover
- [ ] Return type annotation or `defn` signature for symbol under cursor

### 5c. Jump-to-definition
- [ ] Resolve symbol → `defn`/`def` location

### 5d. Completions
- [ ] Top-level `defn`/`def` names, built-in forms, stdlib names

### 5e. VS Code extension
- [ ] `editors/vscode/` — TextMate grammar for syntax highlighting
- [ ] `.glsp` file association
- [ ] Launch `glisp-lsp` as language server

---

## Order of attack

| # | Item | Why |
|---|---|---|
| 1 | 2a collection ops | Unblocks real programs immediately |
| 2 | 2b string ops | Unblocks web handler logic |
| 3 | 3a JSON | Unblocks API servers |
| 4 | 3b routing DSL | Makes web code readable |
| 5 | 3c–3f web features | Production-grade HTTP |
| 6 | 2c better errors | Quality of life |
| 7 | 2d test framework | Confidence |
| 8 | 4a formatter | Consistency |
| 9 | 5a–5b LSP diagnostics + hover | Editor integration |
| 10 | 5c–5e LSP completions + jump + VS Code | Full IDE support |
| 11 | 4b REPL | Interactive development |
| 12 | 2e multi-file | Larger programs |
