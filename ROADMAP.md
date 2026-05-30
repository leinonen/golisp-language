# glisp Roadmap

**Goal**: a Lisp-syntax language that transpiles to Go, suited for professional server applications, data transformation, and concurrent systems. Clojure-inspired, not Clojure-compatible. See `docs/adr/` for design decisions.

---

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
- [x] `json-response` web helper

### 3b. Routing DSL
- [x] `routes`, `GET`, `POST`, `PUT`, `DELETE`, `PATCH` web functions
- [x] Path params: `/users/:id` extracted into request map as `"params"`

### 3c. Middleware
- [x] `wrap-json` — parse JSON body
- [x] `wrap-cors` — CORS headers
- [x] `wrap-auth` — Bearer token extraction into `"identity"`
- [x] `wrap-timeout` — per-request context deadline
- [x] `compose` / `wrap` — `(web/Wrap handler web/WrapLogging web/WrapCors)`

### 3d. Request helpers
- [x] `query-param`, `path-param`, `body-map`, `header`

### 3e. Static file serving
- [x] `serve-files` — `(web/ServeFiles "/static/" "public/")`

### 3f. Graceful shutdown
- [x] `serve-graceful` — drains in-flight requests on SIGINT/SIGTERM

---

## Phase 4 — Developer Experience

### 4a. Formatter
- [x] `glisp fmt file.glsp` — format in-place
- [x] `glisp fmt --check` — exit non-zero if unformatted
- [x] `make fmt-glsp` Makefile target
- [x] Pretty-print over existing AST: consistent indentation, aligned map pairs
- [x] Comment preservation — `;` and `;;` leading comments survive `glisp fmt`

### 4b. REPL
- [x] `glisp repl` — read form, transpile, run via `go run`, print result
- [ ] Readline history and editing (up-arrow, ctrl-a/e, etc.)
- [ ] Side-effect-free def persistence — snapshot accumulated state as values, not source, so `def` side effects don't re-run on each subsequent eval
- [ ] Multi-value expression support — expressions returning `(values a b)` or Go multi-return should print all values instead of failing to compile
- [ ] String-aware paren balancing — current depth counter is confused by `(` / `)` inside string literals

### 4c. Source maps
- [ ] Emit `// glsp:line:col` comments on generated lines
- [ ] Map Go panic stack traces back to `.glsp` source locations

### 4d. Release infrastructure
Without this, "try glisp" means cloning the repo and running `go build`. Barrier too high for public release.

- [ ] GitHub Actions release workflow — build binaries for linux/mac/windows on tag push
- [ ] Publish binaries to GitHub Releases (amd64 + arm64 for each OS)
- [ ] Install script — `curl -sSL https://get.glisp.dev | sh` (or equivalent)
- [ ] Homebrew tap — `brew install glisp-lang/tap/glisp`
- [ ] `glisp version` reports semver tag

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
- [x] Top-level `defn`/`def` names, built-in forms, web package names

### 5e. Docstrings
- [x] `;;; doc comment` preceding a `defn` stored in AST
- [x] Surfaced in hover text and completion detail

### 5f. Rename symbol
- [x] `textDocument/rename` — rename all occurrences in the current document

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

### 6c. Missing Go interop forms
- [x] `panic` / `recover` — call `panic()` and use `recover()` inside `defer`; essential for middleware and wrapping third-party code
- [x] `switch` / `case` — value switch and type switch; eliminates awkward `cond` workarounds for Go interop
- [ ] `as->` — `(as-> x $ (assoc $ :k v) (dissoc $ :old))` — threading with named binding; useful when thread position varies
- [x] `when-let` / `if-let` — `(when-let [user (find-user id)] ...)` — nil-guarded binding; extremely common pattern. Branch taken when bound value is non-nil (`!= nil`); binding supports destructuring
- [ ] `doto` — `(doto obj (.SetHeader "Content-Type" "application/json") (.Write body))` — fluent/builder-style Go APIs
- [ ] `with-open` — `(with-open [f (os/Open path)] body)` — emits `defer f.Close()`; resource cleanup

---

## Phase 7 — Standard Library

### 7a. HTTP client
- [x] `http/get`, `http/post`, `http/put`, `http/delete`, `http/request`
- [x] Returns `[response error]` for use with `if-err`
- [x] Response map: `{"status" <int> "headers" {...} "body" <string>}`
- [x] Optional headers map on `http/get`, `http/post`, `http/put`
- [x] `http/request` accepts opts map with `"method"`, `"url"`, `"body"`, `"headers"` keys

### 7b. Data transformation
Core ops needed in every real server — transforming API payloads, shaping DB results, building responses.

- [ ] `get-in` — `(get-in m [:a :b :c])` — nested map/slice access
- [ ] `assoc-in` — `(assoc-in m [:a :b] v)` — nested map update
- [ ] `update-in` — `(update-in m [:a :b] f)` — nested map update via function
- [ ] `update` — `(update m :key f)` — update a single key via function
- [ ] `select-keys` — `(select-keys m [:id :name])` — map projection
- [ ] `rename-keys` — `(rename-keys m {:old :new})` — rename map keys
- [ ] `group-by` — `(group-by :status users)` → `{"active" [...] "inactive" [...]}`
- [ ] `frequencies` — `(frequencies [:a :b :a])` → `{:a 2 :b 1}`
- [ ] `into` — `(into {} pairs)` — build map from seq of pairs; `(into [] coll)` — collect to vector
- [ ] `concat` — `(concat coll1 coll2 ...)` — join sequences
- [ ] `mapcat` — `(mapcat f coll)` — map then flatten one level
- [ ] `take-while` / `drop-while` — predicate-based slicing
- [ ] `empty?` / `not-empty` — nil/empty check
- [ ] `second` / `last` — common positional accessors
- [ ] `zipmap` — `(zipmap keys vals)` — build map from two sequences
- [ ] `partition` — `(partition n coll)` — split into chunks of size n
- [ ] `partition-by` — `(partition-by f coll)` — split on predicate changes

### 7c. String & number utilities
- [x] `format` — `(format "Hello, %s! You are %d years old." name age)` — wraps `fmt.Sprintf`
- [x] `parse-int` — `(parse-int s)` → `[int error]` — wraps `strconv.Atoi`
- [x] `parse-float` — `(parse-float s)` → `[float64 error]` — wraps `strconv.ParseFloat`
- [x] `repeat` — `(repeat n val)` → slice of n copies of val
- [x] `interpose` — `(interpose sep coll)` → new seq with sep between each element

### 7d. Set support
The AST node `SetLit` exists; needs transpiler wiring and runtime helpers.

- [ ] `#{}` set literals — `#{1 2 3}` → `map[any]struct{}`
- [ ] `conj` on sets — add element
- [ ] `contains?` on sets — O(1) membership test
- [ ] `union` / `intersection` / `difference` — set algebra

---

## Phase 8 — Database

Postgres-first. Rows returned as `[]map[string]any`, natural fit for glisp's map-centric data model. See ADR-009.

### 8a. Connection & basic ops
- [ ] `db/connect` — `(db/connect url)` → connection pool via `pgx`; returns `[conn error]`
- [ ] `db/query` — `(db/query conn sql args)` → `[rows error]` where rows is `[]map[string]any`
- [ ] `db/query-one` — `(db/query-one conn sql args)` → `[row error]` — single row or error
- [ ] `db/exec` — `(db/exec conn sql args)` → `[result error]`
- [ ] `db/transaction` — `(db/transaction conn (fn [tx] ...))` — auto-rollback on error/panic

### 8b. Query builder (HoneySQL-inspired)
Map-based query construction; compiles to parameterized SQL.

- [ ] `(db/select [:id :name] :from :users :where [:= :id id])` — SELECT
- [ ] `(db/insert :users {:name "Alice" :email "a@b.com"})` — INSERT
- [ ] `(db/update :users {:name "Bob"} [:= :id id])` — UPDATE
- [ ] `(db/delete :users [:= :id id])` — DELETE

### 8c. Migrations
- [ ] `glisp migrate up` / `glisp migrate down` — wraps `goose` or `golang-migrate`
- [ ] Migration files as plain SQL in `migrations/`

---

## Phase 9 — Fun & Power Features

Features that make the language enjoyable to use, not just functional.

- [ ] `time-it` — `(time-it expr)` — prints elapsed time, returns value; great for debugging hot paths
- [ ] `pp` — `(pp val)` — pretty-print any value with indentation; better than `println` for maps/slices
- [ ] `tap->` / `tap->>` — like `->` / `->>` but `pp` each intermediate value; debug pipelines without restructuring code
- [ ] Named `fn` — `(fn self [n] (if (= n 0) 1 (* n (self (- n 1)))))` — self-reference in anonymous functions without `defn`
- [ ] `assert` — `(assert condition "message")` — runtime guard; panics with message if condition is false
- [ ] `case` — `(case x :a "alpha" :b "beta" "other")` — value-equality switch; simpler than `cond` for dispatch on known values

---

## Order of Attack

Items 1–9 are v1 blockers: a stranger can't write a real program or install glisp without them. Items 10+ are post-v1.

| # | Item | Why |
|---|------|-----|
| 1 | ~~**6d: `panic` / `recover`**~~ ✓ | Blocks writing safe middleware; can't recover from third-party panics |
| 2 | **4d: Release infrastructure** | Can't publish without binaries and an install story |
| 3 | ~~**6d: `switch` / `case`**~~ ✓ | Essential Go form; eliminates awkward `cond` chains in interop code |
| 4 | ~~**6d: `when-let` / `if-let`**~~ ✓ | Extremely common nil-guard pattern; small effort, high payoff |
| 5 | **7c: `format` / `parse-int` / `parse-float`** | Every real program needs string formatting and input parsing |
| 6 | **7b: `empty?` / `not-empty` / `second` / `last`** | Embarrassing gaps; trivial to add |
| 7 | **7b: `get-in` / `assoc-in` / `update-in` / `update` / `select-keys`** | Needed in every REST handler — shaping request/response maps |
| 8 | **7b: `concat` / `into` / `mapcat` / `take-while` / `drop-while`** | Data pipeline ops for transforms and aggregations |
| 9 | **4b: REPL readline / history** | First thing new users try; bare REPL with no editing is painful |
| 10 | **7d: Set support (`#{}`)** | AST node already exists; wire it up |
| 11 | **6d: `as->` / `doto` / `with-open`** | Ergonomics and Go builder-API interop |
| 12 | **7b: `group-by` / `zipmap` / `partition` / `frequencies` / `rename-keys`** | Fill remaining collection gaps |
| 13 | **4c: Source maps** | Debug Go panics in `.glsp` terms |
| 14 | **8: Database (postgres)** | Next major capability unlock for real applications |
| 15 | **9: Fun features** (`tap->`, `time-it`, `pp`, named `fn`, `assert`, `case`) | Joy and debugging power |
| 16 | ~~**5f: LSP rename**~~ ✓ / **5g–5h: find-refs / code actions** | IDE completeness — nice to have |
