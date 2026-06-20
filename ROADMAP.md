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
- [x] `map-indexed` — `(map-indexed f coll)` → calls `(f index item)` for each element (index is an int64 starting at 0); `_glispMapIndexed` runtime helper asserts `func(any, any) any`. Eliminates the `(map (fn [i] (nth coll i)) (range (count coll)))` workaround

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
- [x] **Cross-file struct field access** — `(:field b)` on a variable typed as a struct defined in another file now emits `b.Field`. A directory build (`compileDir`) collects every file's top-level declarations into a `transpiler.DeclSet` (`CollectDecls`) *before* any file is emitted, and each file is transpiled with `TranspileNoRuntimeFileExt`, which folds the sibling declarations into the pre-pass type tables (`e.structs`/`e.ifaces`/`e.methods`/`e.symbols`/`defGlobals`). Only the current file's nodes are emitted; the externals seed type resolution only.
- [x] **Cross-file interface method dispatch** — `(method-name repo arg)` where `repo` is typed as an interface/struct defined in another file now emits `repo.MethodName(arg)` (same first-pass `DeclSet` collection as above populates `e.ifaces`/`e.methods` package-wide).
- [x] **Inline comments inside `let` bindings** — a trailing `;` comment after a binding (`(let [x 1 ; why\n y 2] …)`) is preserved in place by the formatter instead of being relocated to the end of the file. `formatLet` (covering `let`/`loop`/`with-open`) consumes the comment via `takeTrailingComment`: non-last bindings keep it before the newline, the last binding after the closing `]`. The inline gate now starts at the header line so a trailing comment on the first binding forces the form multi-line. Own-line comments between bindings already worked. (Trailing comments after body statements — where the enclosing `)` shares the line — remain a separate, open case.)

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

### 3g. Hypermedia & streaming (see `docs/web-enhancements-exploration.md`)
Hiccup (`web/html.go`), SSE (`web/sse.go`), and websockets (`web/ws.go`) are promoted. The §5 transpiler bug cluster (Phase 11 below) is fixed — natural-style SSE/websocket producer code compiles.

- [x] Hiccup rendering — `(web/html [:div {:class "x"} ...])`, `web/html-page`, `web/render-response`, `web/raw`; escaped by default, `#id.class` tag shorthand, `map`-output splicing. Promoted; reference app `examples/todos` (the full stack: hiccup + htmx + SSE + websocket chat)
- [x] SSE — `(web/sse-response ch)` streams a `chan any` as `text/event-stream` with idle keepalive comments; `(web/done req)` (lazy, cached) closes on client disconnect for `select!`-based producers; `(web/go-recover (fn [] …))` contains producer panics
- [x] Websockets — `(web/websocket (fn [req in out] ...))`; dependency-free RFC 6455 (text + binary, ping/pong + idle server ping, fragmentation, close-code negotiation, UTF-8/protocol validation, message cap, write deadlines); in/out are `chan any`, reads via `for-chan`. Validated against `coder/websocket`
- [x] htmx helpers — `(web/hx-request? req)`, `web/hx-trigger`/`hx-redirect`/`hx-refresh` header setters, and `web/htmx-js` serving the embedded `htmx.min.js` (offline single binary; `examples/todos` uses it instead of a CDN)

---

## Phase 4 — Developer Experience

### 4a. Formatter
- [x] `glisp fmt file.glsp` — format in-place
- [x] `glisp fmt --check` — exit non-zero if unformatted
- [x] `make fmt-glsp` Makefile target
- [x] Pretty-print over existing AST: consistent indentation, aligned map pairs
- [x] Comment preservation — `;` and `;;` leading comments survive `glisp fmt`
- [x] Preserve float literals — whole-number floats keep a `.0` suffix on reformat (previously `8.0` → `8`, silently changing the literal from `float64` to `int64`)

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
- [x] `as->` — `(as-> x $ (assoc $ :k v) (dissoc $ :old))` — threading with named binding; useful when thread position varies. Emits an IIFE that rebinds the named placeholder (`var $ any = x; $ = ...`) step by step; `$` is now a valid symbol char (`identToGo` maps it to `_dollar`). AST-level form in `emitAsThread` (`emit_expr.go`)
- [x] `when-let` / `if-let` — `(when-let [user (find-user id)] ...)` — nil-guarded binding; extremely common pattern. Branch taken when bound value is non-nil (`!= nil`); binding supports destructuring
- [x] `doto` — `(doto obj (.SetHeader "Content-Type" "application/json") (.Write body))` — fluent/builder-style Go APIs. Evaluates `obj` once, threads it into each step (as the `.method` receiver, else as the first arg — dot-free method dispatch resolves too), and returns it. AST-level form (`*ast.DotoExpr`); a `(.method …)` step is parsed as a `.`-headed CallExpr so the receiver-less and zero-arg (`(.flush)`) forms parse, and `dotoThread` inserts the once-evaluated temp at emit. Returns `any` (object pointer-ness isn't reliably inferable for a typed return).
- [ ] `with-open` — `(with-open [f (os/Open path)] body)` — emits `defer f.Close()`; resource cleanup
- [x] Keywords as functions — `(map :title coll)` / `(filter :active users)` — a bare keyword in the function position of any HOF built-in lowers to a `_glispGet` closure (central lowering in `emitRuntimeArg`, `emit_expr.go`)
- [x] `fnil` — `(update tally k (fnil (fn [n] (inc n)) 0))` — wrap a fn so a nil argument becomes a default. Needed because `or` deliberately returns Go `bool` (ADR-011), so the Clojure `(or n 0)` default idiom is unavailable
- [x] `for` comprehension — `(for [x coll y coll2 :when pred] expr)` — Clojure-style sequence comprehension with optional `:when` guard. Multiple `[name coll]` bindings nest as a cartesian product; emits an IIFE building a `[]any`. Replaces the nested `map`+`flatten` workaround (`emitFor` in `emit_expr.go`)

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
- [x] `max` / `min` — plain numeric variadic forms (mixed int/float compared via float64 coercion)
- [x] `max-by` / `min-by` — `(max-by f coll)` — collection variants of `max-key`/`min-key`; f may be a keyword

### 7d. Set support
The AST node `SetLit` exists; needs transpiler wiring and runtime helpers.

- [x] `#{}` set literals — `#{1 2 3}` → `map[any]struct{}`
- [x] `conj` on sets — add element
- [x] `contains?` on sets — O(1) membership test
- [x] `union` / `intersection` / `difference` — set algebra
- [x] `(set coll)` constructor — build a set from any sequence; `into #{}` also works now (`_glispInto` gained a set target)
- [x] Sets as sequences — `_glispToSlice` enumerates `map[any]struct{}` in sorted order (deterministic), so `map`/`filter`/`doseq`/`sort`/`join`/`into` work on sets

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

- [x] `time-it` — `(time-it expr)` — evaluates expr, prints elapsed time tagged with the expression source, returns the value. Inline timer IIFE (`time.Now()`/`time.Since`); `emitTimeIt` in `emit_expr.go`
- [x] `pp` — `(pp val)` — pretty-print any value with indentation (maps with sorted keys, nested slices) and return it unchanged; better than `println` for maps/slices. Runtime helper `_glispPP`/`_glispPPString` gated on the `_pp` pseudo-key (imports `fmt`; `strconv` is always present)
- [x] `tap->` / `tap->>` — like `->` / `->>` but `pp` each intermediate value (incl. the initial one); debug pipelines without restructuring code. AST rewrite wrapping every thread stage in `(pp …)` (`emitTapFirst`/`emitTapLast`)
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
- [x] **Typed atoms** — `(atom T init)` (e.g. `(atom int 0)`, `(atom map[string]Book {})`) records the element type so a typed `(deref a)` coerces to a concrete *scalar* (int/float/string) without an `(as …)` cast; the init is built under the element-type hint. Tracked via `e.atomTypes` (let/def/param bindings), `e.globalAtomTypes` (top-level `def`), and `structInfo.atomElems` (struct fields). **Any-seam limit:** map/slice/struct element atoms keep `any` deref (a helper write like `assoc` would drift the stored shape, so a concrete assertion could panic) — use bare `any`-element atoms with the collection helpers there.
- [x] **Atoms as struct fields** — the type spelling `Atom` (bare, `any` element) or `(Atom T)` is valid in `defstruct` fields and params, emitting Go `*_glispAtom`; the element type drives field/param deref coercion. So `(defstruct Repo store (Atom map[string]any) hits (Atom int))` works, and stateful structs no longer need a module-level `def` singleton.

### 10f. with-open ✓
- [x] `(with-open [name resource ...] body...)` — binds each resource and `defer`s `Close()` on it inside an IIFE (function-scoped, so cleanup runs at the form's exit, LIFO, even on panic). `_glispClose` asserts `interface{ Close() error }`, so the resource needn't be statically typed; bindings accept an optional type annotation (`[f *os/File (expr)]`). The return type propagates into the IIFE (`hintPropagatable`), so a `with-open` works as a typed function tail. A multi-return resource (`os/open` → `(*os.File, error)`) must be unpacked with `if-err` first.

### 10g. Context propagation ✓
- [x] `(ctx/background)` — `context.Background()`
- [x] `(ctx/todo)` — `context.TODO()`
- [x] `(ctx/with-cancel ctx)` → `[]any{ctx, cancel}` — `context.WithCancel`
- [x] `(ctx/with-timeout ctx ms)` → `[]any{ctx, cancel}` — `context.WithTimeout`; ms is milliseconds
- [x] `(ctx/cancel! cancel)` — call the cancel function; type-asserts to `context.CancelFunc`
- [x] `(ctx/value ctx key)` — `ctx.Value(key)` — read a value from context
- [x] `(ctx/with-value ctx key val)` — `context.WithValue` — add key-value to context
- [x] `(ctx/done? ctx)` → `bool` / `(ctx/err ctx)` → `error` — complete the family; no more `(as context/Context c)` + `.Err` interop. `ctx/done?` is in `boolBuiltins`, so conditions skip the truthy wrapper
- [ ] `ctx` in `select!` — a `(:done ctx body...)` case emitting `case <-ctx.Done():`, so workers can race a context against their channels without interop

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
| 11 | ~~**6d: `as->`**~~ ✓ / **`doto` / `with-open`** | Ergonomics and Go builder-API interop |
| 12 | **7b: `group-by` / `zipmap` / `partition` / `frequencies` / `rename-keys`** | Fill remaining collection gaps |
| 13 | **4c: Source maps** | Debug Go panics in `.glsp` terms |
| 14 | ~~**8: Database (postgres)**~~ | Out of language scope — opt-in via packages (ADR-014) |
| 15 | ~~**8.5: Concurrency ergonomics**~~ ✓ | `go-val`, `par`, `for-chan`, `recv-ok!`, `with-lock`, `:timeout` in `select!` |
| 16 | **9: Fun features** (~~`tap->`~~ ✓, ~~`time-it`~~ ✓, ~~`pp`~~ ✓, named `fn`, ~~`assert`~~ ✓, ~~`case`~~ ✓) | Joy and debugging power |
| 17 | ~~**5f: LSP rename**~~ ✓ / **5g–5h: find-refs / code actions** | IDE completeness — nice to have |
| 18 | ~~**10a–d: File I/O, slog, regex, error wrapping**~~ ✓ | Essential for any real-world program |
| 19 | **10e–g: `atom`, `with-open`, context propagation** | Ergonomics for shared state and resource safety |
| 20 | **2a: `map-indexed`** / **6c: `for` comprehension** | Eliminated workarounds forced by their absence in FPS game dev |
| 21 | **11: Numeric auto-coercion + IIFE type propagation + void tail** | Eliminates need for Go bridge code in any project using arithmetic or conditionals with typed results |

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
- [x] Typed fn as HOF argument — passing a `defn` with concrete param/return types where a runtime helper asserts `func(any) any` is now a position-tagged transpile error naming the fix (wrap in a lambda or declare `any` types), instead of a runtime interface-conversion panic. Local bindings shadowing a defn name are not flagged; variadic fns are left to the runtime (`apply` handles `func(...any) any`)
- [x] `(string x)` on `any` — now routes through `_glispToString` (was a raw Go conversion: compile error on interface values, int→rune footgun on numbers). `_glispToString` smart-converts: strings pass through, `[]byte`/numbers/bools stringify, anything else → `""`
- [x] `dotimes` with `_` binding — substitutes a synthetic loop counter (was illegal Go `for _ := 0; _ < 3`)
- [x] `select!` in `loop` tail position — statement-only forms in a loop tail now emit the statement plus `break`/`return nil` (was `_loopN = select { … }`, invalid Go). Same ADR-011 rule fn tails already followed; surfaced by SSE/websocket producer code (`docs/web-enhancements-exploration.md` §5)
- [x] `_` binding in a `select!` recv case — `([_ ch] body)` now emits `case <-ch:` (was `case _ := <-ch:`, "no new variables on left side of :=")
- [x] bare `nil` as a `select!` case body — bare scalar literals in statement position are skipped (a `nil` expression statement is illegal Go)
- [x] statement-only forms (`close!`, `send!`, …) as `if` branches in a loop tail — handled by the same loop-tail statement-only rule (was `close(ch)` in value position, "used as value")
- [x] `panic` in tail position of a value-returning fn (incl. `do`/loop tails) — emits a bare `panic(...)` statement (was `return panic(...)`, invalid Go; a bare panic satisfies Go's termination analysis)
- [x] **Numeric auto-coercion in arithmetic context** — arithmetic (`+ - * / mod`) and ordering comparisons (`< > <= >=`) on statically-`any` operands (map/slice lookups, untyped params, range loop vars, destructure binds) now route through coercion helpers (`_glispAdd`/`_glispMul`/`_glispGt`/…, runtime block gated on the `_num` pseudo-key) that coerce each operand to int64/float64 — preserving integer-ness when no float is present. Typed numeric code stays native (`(a + b)`). An `any`-arith result in a concrete numeric position (typed `let`/`-> int`/`-> float64`) is smart-converted via `_glispToInt`/`_glispToFloat64` (`emitExprWithHint`). Provably-`any` detection is conservative (`exprIsAny`/`localAny`) so existing typed code is never re-typed. `=`/`not=` stay native interface comparisons
- [x] **IIFE type propagation for `if`/`when`/`do`/`cond`/`switch`** — in expression (non-tail) position with a concrete type hint (typed `let`/`def` binding, typed param, return position), the block IIFE is now emitted as `func() T { … }()` instead of `func() any`, threading the hint through `e.currentRetType` so each branch return is emitted and coerced for `T` (struct map literals, typed slice literals, numeric coercion). `emitExprWithHint` routes these via `emitTypedIIFE`; `hintPropagatable` gates safety — constructs with an implicit `return nil` tail (`when`, no-default `cond`/`switch`) only propagate a nilable hint (slice/map/pointer/interface/`error`), an `if` propagates only when it has an `else`, and `do`/defaulted `cond`/`switch` propagate any hint
- [x] **`(map f coll)` returns `[]any`, not a typed slice** — a `(map (fn [v] …) coll)` in a `[]T` position (typed binding/param/return) now emits a typed loop (`func() []T { r := []T{}; for _, x := range _glispToSlice(coll) { r = append(r, fn(x)) }; return r }()`) instead of `_glispMap`, so it satisfies a `[]Book` return type. `tryEmitTypedMap` (`emit_expr.go`) fires for a single untyped-param lambda; when the lambda's Go return type is `any` (the common `(fn [v] (as Book v))` case) the appended value is asserted to the element type. Keyword fns, `[]any` hints, and typed-param lambdas fall back to `_glispMap`. Complementary runtime fix: `_glispToSlice`/`_glispLen` gained a reflection fallback so the resulting `[]Book` (and any other user-typed slice) still works with the collection helpers (`len`/`first`/`map`/…) — the base runtime now imports `reflect` (already linked via `fmt`)
- [x] **Void-returning Go calls in `when`/`do` tail position** — a void-returning call (`os/exit`, a user `-> void` fn/method) in return position (when/do/if tail, IIFE-wrapped or direct) now emits `<call>; return nil` instead of an invalid `return os.Exit(0)`. `voidReturnBuiltins` + `isVoidCall` (`emit_expr.go`) gate it; wired into `emitReturnNode`'s `CallExpr` case, mirroring the statement-only-form rule

### 11a. Ergonomics — next up (ordered by friction removed)

Prioritized by paper-cuts-per-session, not difficulty. Each surfaces in any
`any`-seam-heavy program (maps-as-values, per-frame arithmetic, `loop`/`recur`,
hand-written Go interop). Every item reuses existing Phase 11 machinery
(`emitExprWithHint` / `hintPropagatable` / numeric coercion) and serves the same
principle: the user never debugs generated Go.

- [ ] **Typed return propagation for `conj`/`reduce`/`filter`/`into`/`assoc`** — these still return `any`, so in a `-> []T` / `-> bool` position the user must hand-write `(as []any (conj …))` / `(as bool (reduce …))`. Common enough to appear several times in a single function-heavy file. Generalize the typed-`map` work (`tryEmitTypedMap`): when the hint is `[]T`/`bool`/`map[...]...` and the seed/element type is compatible, emit a typed form or auto-insert the `.([]T)` assertion. *Highest ROI — most frequent friction, machinery already exists.*
- [ ] **Coerce `any` args at stdlib numeric call sites (or diagnose)** — user-fn params coerce via `_glispToFloat64`, but `(math/abs any-expr)` does not → raw Go error; the only fix today is wrapping in a `(defn fabs [x float64] (math/abs x))` shim. The stdlib signature is already known at emit time (qualifier resolution → `math.Abs`); coerce the arg for known `func(float64…)`/`func(int…)` targets. MVP: a position-tagged glisp diagnostic naming the wrap-fn fix instead of the Go error.
- [ ] **Auto-promote across concrete numeric types in arithmetic** — `(/ concrete-int 2.0)` mixes concrete `int` and `float64` → Go compile error (coercion only fires across the `any` seam). Promote `int`→`float64` when an arithmetic form mixes concrete numerics, or emit a cast-suggesting diagnostic. (Clojure promotes silently.)
- [ ] **Mark `loop` scalar bindings as `any`** — the known remaining gap in the numeric-coercion notes; closing it means arithmetic on a `loop`-bound scalar coerces like every other `any` value, removing surprise type annotations on `loop`.
- [ ] **Parser: trailing `;` comment after a `let`-binding value** — `(let [x (f) ; why\n y 2] …)` → `parse error: expected symbol, got comment`, even though the formatter (4a/2e) claims to preserve binding comments. Pure friction. (The formatter half landed; the parser half rejects the same position.)
- [ ] **Reflect struct fields in `_glispGet`** — `(:field x)` / `(get x "field")` on an `any` that holds a struct returns nil (noted in the `emit_expr.go` comment). Add a reflect fallback like the one already added to `_glispToSlice`, so keyword/`get` access is uniform across maps and structs.
- [ ] **Declare external Go signatures** — `(declare-go fn-name [int] -> []any)` (or read sibling hand-written `.go` func signatures during a dir build) so calls into hand-written Go get typed returns instead of `any`. Makes the documented "bridge pattern for variadic/interop Go APIs" type-flow cleanly for any program that mixes `.glsp` with hand-written Go.

Lower priority (correctness / large bet, tracked but not ergonomics-first):
- [ ] **Value-equality `=`/`not=`** — currently Go `==`, so `(= (int64 1) (int 1))` is `false` and collections compare by identity (documented footgun). A `_glispEquals` (numeric unification + deep collection compare) matches Clojure but is a behavior change → wants an ADR.
- [ ] **User macros (`defmacro`)** — currently parses to `"(macros not yet supported)"`. The largest gap vs Clojure and the reason every new surface form requires patching the transpiler. Even a restricted compile-time (non-hygienic) AST→AST expansion would let the *library* grow instead of the *compiler*. Large enough to warrant an explicit in-scope/out-of-scope ADR (cf. ADR-014 for databases).
