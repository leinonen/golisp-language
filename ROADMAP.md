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
- [x] Readline history and editing (up-arrow, ctrl-a/e, etc.)
- [ ] Side-effect-free def persistence — snapshot accumulated state as values, not source, so `def` side effects don't re-run on each subsequent eval
- [x] Multi-value expression support — multi-return expressions print all values via variadic `fmt.Println`; errors from assignment mismatch show a `if-err` hint
- [x] String-aware paren balancing — depth counter skips `;` comments and string contents

### 4c. Source maps
- [ ] Emit `// glsp:line:col` comments on generated lines
- [ ] Map Go panic stack traces back to `.glsp` source locations

### 4d. Release infrastructure
Without this, "try glisp" means cloning the repo and running `go build`. Barrier too high for public release.

- [x] GitHub Actions release workflow — build binaries for linux/mac/windows on tag push (`.github/workflows/release.yml`)
- [x] Publish binaries to GitHub Releases (amd64 + arm64 for each OS) — 6-target matrix + `SHA256SUMS`
- [x] Install script — `curl -fsSL …/install.sh | sh` (detects OS/arch, verifies checksum)
- [ ] Homebrew tap — `brew install glisp-lang/tap/glisp`
- [x] `glisp version` reports semver tag — ldflags-stamped via `internal/version`, VCS-info fallback; also `make dist` for local cross-compiles

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
- [x] `textDocument/references` — list all references to a symbol, project-wide (current doc, other open docs, and sibling `.glsp` files on disk); skips full-comment lines

### 5i. Document outline
- [x] `textDocument/documentSymbol` — outline of top-level `ns`/`def`/`defn`/`defstruct`/`definterface`/`defmethod`/`deftype`/`deftest`; selection range targets the name

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

- [x] `get-in` — `(get-in m [:a :b :c])` — nested map/slice access
- [x] `assoc-in` — `(assoc-in m [:a :b] v)` — nested map update
- [x] `update-in` — `(update-in m [:a :b] f)` — nested map update via function
- [x] `update` — `(update m :key f)` — update a single key via function
- [x] `select-keys` — `(select-keys m [:id :name])` — map projection
- [x] `rename-keys` — `(rename-keys m {:old :new})` — rename map keys
- [x] `group-by` — `(group-by :status users)` → `{"active" [...] "inactive" [...]}`
- [x] `frequencies` — `(frequencies [:a :b :a])` → `{:a 2 :b 1}`
- [x] `into` — `(into {} pairs)` — build map from seq of pairs; `(into [] coll)` — collect to vector
- [x] `concat` — `(concat coll1 coll2 ...)` — join sequences
- [x] `mapcat` — `(mapcat f coll)` — map then flatten one level
- [x] `take-while` / `drop-while` — predicate-based slicing
- [x] `empty?` / `not-empty` — nil/empty check
- [x] `second` / `last` — common positional accessors
- [x] `zipmap` — `(zipmap keys vals)` — build map from two sequences
- [x] `partition` — `(partition n coll)` — split into chunks of size n
- [x] `partition-by` — `(partition-by f coll)` — split on predicate changes

### 7c. String & number utilities
- [x] `format` — `(format "Hello, %s! You are %d years old." name age)` — wraps `fmt.Sprintf`
- [x] `parse-int` — `(parse-int s)` → `[int error]` — wraps `strconv.Atoi`
- [x] `parse-float` — `(parse-float s)` → `[float64 error]` — wraps `strconv.ParseFloat`
- [x] `repeat` — `(repeat n val)` → slice of n copies of val
- [x] `interpose` — `(interpose sep coll)` → new seq with sep between each element

### 7d. Set support
The AST node `SetLit` exists; needs transpiler wiring and runtime helpers.

- [x] `#{}` set literals — `#{1 2 3}` → `map[any]struct{}`
- [x] `conj` on sets — add element
- [x] `contains?` on sets — O(1) membership test
- [x] `union` / `intersection` / `difference` — set algebra

---

## Phase 8 — Database (out of language scope — delivered via packages)

**Database access is not a language feature.** Per [ADR-014](docs/adr/ADR-014-database-out-of-language-scope.md)
(superseding ADR-009), the transpiler ships no `db/*` forms, no query builder,
and no `glisp migrate` subcommand. A database is an opt-in package dependency,
consumed with the Go-interop primitives the language already has.

The enabling work is **done** (see `docs/go-interop-exploration.md`): `glisp get`
fetches Go packages, `go-require` wires them into a derived `go.mod`, and
`_glispToSlice` accepts `[]map[string]any` so driver rows flow straight into
`map`/`filter`/`group-by`/`select-keys`.

- [x] Use a Go driver directly — `(:import [github.com/jackc/pgx/v5])` + `(.Method …)` / `(as *pgx/Conn v)`
- [x] Or a glisp wrapper module — `(:require [...])` over a driver (reference: `github.com/leinonen/glispdb`)
- [x] Rows as `[]map[string]any` — a library convention, not a language one
- [ ] ~~`db/connect` / `db/query` / `db/exec` / `db/transaction`~~ — won't do (ADR-014)
- [ ] ~~HoneySQL-style query builder (`db/select`, `db/insert`, …)~~ — won't do (ADR-014)
- [ ] ~~`glisp migrate` subcommand~~ — won't do; use plain SQL + `goose`/`golang-migrate` from a package or script

---

## Phase 8.5 — Concurrency Ergonomics

Higher-level concurrency primitives so common Go patterns don't require verbose interop. All six forms auto-import the packages they need (`sync`, `time`).

- [x] `go-val` — `(go-val body...)` → IIFE returning `chan any`; goroutine sends result. Collect with `(recv! ch)`. Parallel to Clojure's `future`.
- [x] `par` — `(par e1 e2 ...)` → `sync.WaitGroup` block; all bodies run in parallel goroutines; blocks until all finish.
- [x] `for-chan` — `(for-chan [x ch] body...)` → `for x := range ch`; iterate until channel is closed. Distinct from `doseq` which emits `for _, x := range` (index-based).
- [x] `recv-ok!` — `(recv-ok! ch)` → `[]any{val, ok}` via inline IIFE. Use with `[[val ok] (recv-ok! ch)]` destructuring; check `(= ok true)` since `ok` is `any`.
- [x] `with-lock` — `(with-lock mu body...)` → IIFE with `mu.Lock()` / `defer mu.Unlock()`. Unlock guaranteed even on panic.
- [x] `:timeout ms` in `select!` — `(:timeout 5000 body...)` case → `case <-time.After(5000 * time.Millisecond):`.
- [x] `doseq` fix — now uses `_glispToSlice(coll)` instead of `coll.([]any)` assertion; works when collection is already `[]any` (result of `map`, `filter`, literal `let` binding).

---

## Phase 9 — Fun & Power Features

Features that make the language enjoyable to use, not just functional.

- [ ] `time-it` — `(time-it expr)` — prints elapsed time, returns value; great for debugging hot paths
- [ ] `pp` — `(pp val)` — pretty-print any value with indentation; better than `println` for maps/slices
- [ ] `tap->` / `tap->>` — like `->` / `->>` but `pp` each intermediate value; debug pipelines without restructuring code
- [ ] Named `fn` — `(fn self [n] (if (= n 0) 1 (* n (self (- n 1)))))` — self-reference in anonymous functions without `defn`
- [x] `assert` — `(assert cond)` / `(assert cond msg)` — runtime invariant guard; panics if falsy, auto-generating the message from the condition source when none given
- [x] `case` — `(case x :a "alpha" :b "beta" "other")` — Clojure-style value dispatch (trailing default), a surface alias compiled to a Go switch

---

## Phase 10 — Robust Applications

Building blocks that close the gap between a toy language and one you'd stake production on.

### 10a. File I/O ✓
- [x] `read-file` — `(read-file path)` → `[string error]` — wraps `os.ReadFile`
- [x] `write-file` — `(write-file path content)` → `error` — wraps `os.WriteFile`
- [x] `append-file` — `(append-file path content)` → `error` — open with `O_APPEND|O_CREATE|O_WRONLY`
- [x] `file-exists?` — `(file-exists? path)` → `bool` — `os.Stat` + `os.IsNotExist`
- [x] `list-dir` — `(list-dir path)` → `[[]string error]` — wraps `os.ReadDir`
- [x] `mkdir` — `(mkdir path)` → `error` — wraps `os.MkdirAll`

### 10b. Structured logging ✓
- [x] `log/info`, `log/debug`, `log/warn`, `log/error` — variadic key-value pairs after message string; backed by Go 1.21 `log/slog`. Void like `fmt/println` — direct call in statement/return position, IIFE wrapper in expression position. No import needed.

### 10c. Regex ✓
- [x] `re/match` — `(re/match pattern s)` → `bool` — wraps `regexp.MatchString`; panics on invalid pattern
- [x] `re/find` — `(re/find pattern s)` → `any` — leftmost match or nil
- [x] `re/find-all` — `(re/find-all pattern s)` → `[]any` — all non-overlapping matches
- [x] `re/replace` — `(re/replace pattern s repl)` → `string` — `regexp.ReplaceAllString`
- [x] `re/split` — `(re/split pattern s)` → `[]any` — `regexp.Split`

### 10d. Error wrapping ✓
- [x] `wrap-error` — `(wrap-error msg err)` → `error` — wraps with `fmt.Errorf("%s: %w", msg, err)` for proper Go error chains
- [x] `errors/is?` — `(errors/is? err target)` → `bool` — wraps `errors.Is` for unwrapping chains

### 10e. atom — shared mutable state ✓
- [x] `(atom init)` — create an atom wrapping init value; backed by `struct { mu sync.Mutex; val any }`
- [x] `(swap! a f)` — atomically update with f; locks, calls f(current), assigns, unlocks
- [x] `(reset! a v)` — unconditional set (locked)
- [x] `(deref a)` — read current value (locked)

### 10f. with-open
- [ ] `(with-open [f (os/Open path)] body...)` — wraps body in `defer f.Close()`; safe resource cleanup for files, HTTP responses, and anything with a `Close()` method

### 10g. Context propagation ✓
- [x] `(ctx/background)` — `context.Background()`
- [x] `(ctx/todo)` — `context.TODO()`
- [x] `(ctx/with-cancel ctx)` → `[]any{ctx, cancel}` — `context.WithCancel`
- [x] `(ctx/with-timeout ctx ms)` → `[]any{ctx, cancel}` — `context.WithTimeout`; ms is milliseconds
- [x] `(ctx/cancel! cancel)` — call the cancel function; type-asserts to `context.CancelFunc`
- [x] `(ctx/value ctx key)` — `ctx.Value(key)` — read a value from context
- [x] `(ctx/with-value ctx key val)` — `context.WithValue` — add key-value to context

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
| 10 | ~~**7d: Set support (`#{}`)** ✓~~ | AST node already exists; wire it up |
| 11 | **6d: `as->` / `doto` / `with-open`** | Ergonomics and Go builder-API interop |
| 12 | **7b: `group-by` / `zipmap` / `partition` / `frequencies` / `rename-keys`** | Fill remaining collection gaps |
| 13 | **4c: Source maps** | Debug Go panics in `.glsp` terms |
| 14 | ~~**8: Database (postgres)**~~ | Out of language scope — opt-in via packages (ADR-014) |
| 15 | ~~**8.5: Concurrency ergonomics**~~ ✓ | `go-val`, `par`, `for-chan`, `recv-ok!`, `with-lock`, `:timeout` in `select!` |
| 16 | **9: Fun features** (`tap->`, `time-it`, `pp`, named `fn`, ~~`assert`~~ ✓, ~~`case`~~ ✓) | Joy and debugging power |
| 17 | ~~**5f: LSP rename**~~ ✓ / **5g–5h: find-refs / code actions** | IDE completeness — nice to have |
| 18 | ~~**10a–d: File I/O, slog, regex, error wrapping**~~ ✓ | Essential for any real-world program |
| 19 | **10e–g: `atom`, `with-open`, context propagation** | Ergonomics for shared state and resource safety |

---

## Phase 11 — Absorbing the `any` seam (ADR-011) and developer feedback loop

Principle: the user never debugs generated Go. Every remaining `any`-constraint
either gets absorbed by emission or becomes a glisp-level diagnostic.

- [x] Truthiness — `_glispTruthy` wrapping for non-bool conditions in `if`/`when`/`cond`/`and`/`or`/`not`/asserts (nil and false falsy)
- [x] `len` as alias for `count` — `_glispLen` accepts `any`
- [x] Statement-only forms (`go`, `select!`, `send!`, `close!`, `par`, `for-chan`, `fan-out`, `defer`) in tail position auto-emit `return nil`
- [x] Multi-return call as a single value — transpile-time diagnostic (suggests `if-err` / `(do ... nil)`) for multi-return built-ins and user `-> [T E]` fns, in fn/loop tails and `let`/`if-let`/`let-or`/`def` bindings; unknown Go interop fns rely on //line-mapped Go errors
- [x] `_glispToSlice` over common concrete slice types (`[]string`, `[]int`, `[]int64`, `[]float64`, `[]bool`, `[]map[string]any`) — `first`/`rest`/`conj`/`contains?`/`get`/`flatten` all route through it; `(rest os/args)` works
- [x] Go-error → `.glsp` mapping audited — build errors and panic user-frames already mapped via //line; fixed: runtime-helper panic frames re-anchored to `glisp_runtime.go` (were misattributed to bogus `.glsp` lines in single-file builds), deftest assertion failures pinned to the exact assert line (were drifting)
- [x] `glisp run file.glsp` — one-shot compile-and-run for a fast edit-run loop (also takes a dir; passes args, propagates exit code, leaves no artifacts)
- [ ] `glisp run --watch` — re-run on save
