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
| `internal/lexer/lexer.go` | Tokenizer; no `^` — types are positional |
| `internal/parser/parser.go` | Tokens → AST nodes |
| `internal/transpiler/transpiler.go` | `Emitter` struct, two-pass `emitFile`, dispatch |
| `internal/transpiler/emit_decl.go` | `def`, `defn`, `defstruct`, `definterface`, `defmethod`, `deftype` |
| `internal/transpiler/emit_expr.go` | `fn`, `let`, `if`, `cond`, `do`, built-ins |
| `internal/transpiler/emit_concurrency.go` | `go`, `go-val`, `par`, `defer`, `chan`, `send!`, `recv!`, `recv-ok!`, `close!`, `select!` (+ `:timeout`), `for-chan`, `with-lock`, `if-err`, method/field/struct interop |
| `internal/transpiler/emit_loop.go` | `loop`/`recur` → `for` |
| `internal/transpiler/emit_arity.go` | `builtinArity` table + `checkBuiltinArity` — central arity front-gate for built-in call forms |
| `internal/transpiler/emit_types.go` | `identToGo`, `typeExprToGo`, `qualifiedTypeToGo`, `zeroValueFor` |
| `internal/transpiler/emit_runtime.go` | `glispRuntime` (always), `glispSortRuntime`, `glispStrRuntime`, `glispJsonRuntime`, `glispEnvRuntime`, `glispFileRuntime`, `glispReRuntime`, `glispSetRuntime` (conditional) |
| `internal/formatter/formatter.go` | AST → formatted glisp source; `Format(src)` public API |
| `internal/compiler/compiler.go` | Orchestrates pipeline: `Compile`, `CompileAndBuild`, `CompileDir`, `CompileTest`, `TranspileDir`, `Run`, `GetModule`, `ResolveDeps` |
| `internal/module/modfile.go` | `glisp.mod` parsing/writing: `ReadModFile`, `WriteModFile`, `InitModFile`; `GoRequire` type, `AddGoRequire` |
| `internal/module/resolver.go` | Module download (GitHub tar.gz), go.mod wiring: `Download`, `EnsureGoMod`, `RegisterInGoMod`, `IsCached` |
| `cmd/glisp/main.go` | CLI: `print`, `compile`, `build`, `run`, `test`, `fmt`, `get`, `mod`, `repl`, `doc` subcommands |
| `web/web.go` | Ring adapter, routing, middleware, request helpers, static files, graceful shutdown — plain Go, not glisp |
| `cmd/glisp-lsp/main.go` | LSP server entry point — JSON-RPC 2.0 over stdio |
| `internal/lsp/server.go` | JSON-RPC dispatch, doc state, handler wiring |
| `internal/lsp/hover.go` | Hover provider + `buildSymbolTable`, `symbolAtPosition` helpers |
| `internal/lsp/definition.go` | Jump-to-definition provider |
| `internal/lsp/completion.go` | Completion provider + `prefixAtPosition` |
| `internal/lsp/references.go` | Find-references + shared `findOccurrences` (whole-symbol, skips comment lines; used by rename too) |
| `internal/lsp/rename.go` | Rename provider (`FindRenameEdits`), built on `findOccurrences` |
| `internal/lsp/symbols.go` | Document outline (`DocumentSymbols`) for `textDocument/documentSymbol` |
| `internal/lsp/diagnostics.go` | Parse error → LSP diagnostic push |
| `internal/lsp/builtins.go` | Doc map for built-in hover + completion detail (includes `web/Request`, `web/Response`, `web/Handler` type entries) |

## Important design decisions

**Two-pass emission**: declarations emitted to a side buffer first so built-in import needs (`fmt`, `errors`, `encoding/json`, …) are discovered before writing the package header. See `emitFile` in `transpiler.go`.

**Panic-recovery boundary (robustness)**: `transpileInternal` (`transpiler.go`) wraps the whole lex→parse→emit pipeline in a `defer/recover` that converts any escaping panic (e.g. an unforeseen index-out-of-range deep in an emit helper) into a normal `TranspileError` (`"internal transpiler error: …"`). This guarantees malformed input never crashes the host — the CLI prints a clean error and exits 1; the LSP keeps running. The LSP request dispatcher `Server.Handle` (`internal/lsp/server.go`) has its own `defer/recover` as defense-in-depth for the parser/formatter/hover paths that don't go through the transpiler — on panic it logs and returns a `-32603` internal-error response (or swallows it for notifications). Keep both boundaries: they are the safety net that lets the arity table and other validators be the *primary*, position-tagged defense rather than the only thing standing between bad input and a crash.

**Central built-in arity gate**: `builtinArity` in `emit_arity.go` maps each built-in call form to a `[min, max]` arg-count range (`max == -1` = variadic). `checkBuiltinArity` is the first thing `emitCallExpr` runs on a `*ast.Symbol` head, *before* any argument is indexed, so a wrong-arity built-in yields a single position-tagged error (`nth expects 2 argument(s), got 1 (at 4:3)`) instead of a Go slice-index panic. Forms absent from the table (or with constraints a range can't express, e.g. `assoc`'s odd-count rule) keep their own downstream check; the table is the canonical front-gate and the place to add a row when introducing a new fixed-arity built-in. Diagnostics (`internal/lsp/diagnostics.go` → `atPosToDiagnostic`) parses both the `… at file:line:col` and `… (at file:line:col)` position suffixes, so these errors surface at the right editor location.

**Auto-import of stdlib packages**: No stdlib package ever needs a user `(:import […])` declaration. The `Emitter` tracks two maps: `builtinImports` (set by explicit `needImport` calls in built-in form handlers — used to gate runtime helper blocks) and `directImports` (set by `emitExpr` → `*ast.Symbol` for any `pkg/fn` symbol the user writes directly, and by `emitStructLitExpr` for struct type names). `emitImports` emits `builtinImports` with `runtimeOnlyPkgs` filtering (multi-file mode), then emits ALL `directImports` unconditionally. `isModuleAlias` prevents emitting a bare `import "web"` when the user has declared `golisp/web` as a module import — only packages whose alias matches no entry in `e.imports` / `e.requires` go into `directImports`. Result: `(:import […])` in `ns` is only needed for external module packages (e.g. `golisp/web`, `github.com/jackc/pgx/v5`).

**No `#(...)` anonymous function shorthand**: deliberately removed. Because it was a reader-level expansion (no AST node), the parser turned `#(+ % 1)` into a plain `(fn [_anonP1] (+ _anonP1 1))` with internal `_anonP1` param names — and since the formatter only ever saw that expanded `FnExpr`, `glisp fmt` silently reshaped `#(...)` source into `(fn [...] ...)` with those ugly generated names (no round-trip). That surprising formatter behavior wasn't worth one redundant way to write a lambda, so `#(` is now a hard lex error (`internal/lexer/lexer.go`) pointing users at `(fn [x] ...)`. Always write lambdas as `(fn [x] ...)`.

**Destructuring**: `LetBinding.Pattern` is `*Symbol`, `*VectorLit` (sequential: `[[a b] coll]` → `_glispGet(tmp, int64(i))`), or `*MapLit` (map: `[{k :key} m]` → `_glispGet(tmp, "key")`). `Param.Pattern` (non-nil) enables the same in `fn`/`defn`/`defmethod` params — a temp name `_pN` is used in the Go signature and bindings are emitted at the top of the function body. Both forms use the existing `_glispGet` runtime helper. A `_` destructure element is skipped (emitting `_ := …` is illegal Go).

**Annotated destructuring (map only)**: a map destructure binding may carry a `:- Type` annotation that types the local instead of leaving it `any` — `[{name :name :- string age :age :- int} req]`. Parsed with no grammar change (the `:-` lexes as `KeywordLit("-")`, becoming an annotation pair that `mapDestructureEntries` folds into the preceding binding in `emit_expr.go`). Emission: `string`/`int`/`float64` route through the always-present `_glispToString`/`_glispToInt`/`_glispToFloat64` helpers (smart conversion — `_glispToInt`/`_glispToFloat64` handle JSON's float64 numbers *and* parse numeric strings like query/path params via `strconv`); any other type uses a checked assertion `name, _ := _glispGet(src, "key").(T)` (zero value if absent/wrong type). A struct-typed annotation registers the local in `e.localTypes`, so `(:field name)` keyword access works on it. Types must be simple names (`string`, `Product`, `*Product`, `web/Request`); bracketed types like `[]string` are not supported here. Works in both `let` and param patterns; sequential (`[a b]`) destructuring is unannotated.

**`if-let` / `when-let`**: `(if-let [pat expr] then else?)` / `(when-let [pat expr] body...)` (`emit_expr.go`). Binds `pat` (single symbol, or a `_glispGet`-based destructure pattern) from `expr`, then branches on **`!= nil`** (nil-guard, matching `nil?`). Truthy branch gets the bindings in scope; destructured names are emitted *inside* that branch only, so they don't leak into the else/nil branch. `if-let` with no else and `when-let`'s false case return `nil`. Constraint: binding a non-nilable concrete type (e.g. a raw `int`) won't compile against `!= nil` — bind `any`-typed values (map lookups, find-fns).

**`let-or`**: `(let-or [name1 expr1 fallback1 name2 expr2 fallback2 ...] body...)` (`emit_expr.go`). Flat sequential nil-guard bindings — each binding evaluates `expr`, checks `== nil`, and returns `fallback` immediately if nil. Flattens chains of `if-let`/`if (= x nil)` validation guards into a single level. All names are in scope for subsequent bindings and the body. Use for validating request fields before a DB call. AST: `LetOrExpr`/`LetOrBinding` in `ast/nodes.go`.

**Statement vs expression position**: `let`/`if`/`do`/`when`/`cond` in statement position emit as plain Go blocks. In expression position they wrap in an IIFE `func() any { ... }()`. `emitStmtNode` handles statement position; `emitExpr` handles expression position.

**Return position**: `emitReturnNode` handles tail-position nodes. `loop` in return position emits `return value` directly (no `any` temp var). The `loopInReturn bool` field tracks this.

**Loop binding types**: An explicit `TypeAnnot` on a loop binding always wins — `(loop [xs []string []] ...)` emits `var xs []string = []string{}`. Without an annotation, the old rules apply: collection-init vars use `var name any = init` (so `recur` can rebind with `any`-returning helpers like `_glispConj`); scalar inits use `:=` for concrete Go inference. See `isBindingTypeStart()` in `parser.go` for the disambiguation heuristic (excludes `(` to avoid false positives with value expressions like `(fn ...)`).

**Struct type inference (gradual typing)**: two related features let map-shaped, Clojure-style code compile to typed Go structs when the surrounding type is known — `any` stays the fallback, never a requirement. A pre-pass in `emitFile` populates `e.structs` (glisp struct name → field table) and richer `e.symbols` entries (`fnSig.paramTypes`/`retType`). Machinery lives in `emit_typeinfo.go`.

- **Typed map literals → structs**: a plain `{:field v}` in a *struct-typed position* emits `Struct{Field: v}` instead of `map[string]any`. Positions that supply the hint: user-function call args (matched to `fnSig.paramTypes`), typed `let`/`def` bindings, and tail/return position (via `e.currentRetType`). Routed in `emitExprWithHint` → `structHint` (recognise the type) → `emitStructLitFromMap`. Keys may be keywords, symbols, or strings; an **unknown field is a compile-time glisp error** (catches typos before Go). An explicit map `TypeAnnot` on the literal always wins. Pointer hints (`*T`) emit `&Struct{...}`.
- **Typed keyword access**: `(:field x)` emits `x.Field` (not `_glispGet`) when `x` is a variable statically known to hold a declared struct. The local type environment `e.localTypes` (glisp var → struct name) is managed with `pushTypeScope`/`popTypeScope` around `defn`/`fn`/`defmethod` and `let` bodies; populated from struct-typed params, the method receiver, typed `let` bindings, and inference (`inferValueStructType`: struct literals and calls to fns with a struct return type). A keyword naming no field of the known struct is a compile-time error. Untyped values (bare maps, `any` params) fall back to `_glispGet` unchanged. Only the 1-arg form upgrades; `(:k x default)` stays a runtime lookup.

**`->` in identifiers**: `ring->handler` → `ringToHandler`. Pre-processed with `strings.ReplaceAll(s, "->", "-To-")` before camelCase conversion in `identToGo`.

**Package-qualified naming**: glisp source uses lowercase-hyphenated names (`fmt/println`, `web/json-response`). `identToGo` applies `fnToGo` to the part after `/`: if all-lowercase → PascalCase (`println` → `Println`, `json-response` → `JsonResponse`); if any uppercase → pass through as-is (backward compat). Type expressions like `web/Request` go through `qualifiedTypeToGo` (slash→dot only).

**Type syntax** — positional, no `^` prefix:

| Form | Example |
|---|---|
| `defn` single return | `(defn grade [score int] -> string ...)` |
| `defn` multi-return | `(defn f [x int] -> [string error] ...)` |
| `defn` void (no return) | `(defn say [s string] -> void ...)` → `func say(s string)` |
| `fn` | `(fn [x int] -> string body)` |
| `defmethod` | `(defmethod *Circle Area [c] -> float64 body)` |
| `defstruct` | `(defstruct Circle radius float64)` |
| `definterface` | `(definterface S (Area [] -> float64))` |
| `def` (typed) | `(def x int 42)` |
| `def` with typed collection | `(def xs []string ["a" "b"])` → `var xs []string = []string{"a", "b"}` |
| `let` typed binding | `(let [x string "hello"] ...)` → `var x string = "hello"` |
| `loop` typed binding | `(loop [xs []string [] n int 0] ...)` → `var xs []string = []string{}; var n int = 0` |
| `deftype` named type | `(deftype UserId int)` → `type UserId int` |
| `go-val` typed channel | `(go-val string body)` → returns `chan string` instead of `chan any` |
| type assertion | `(as *Circle val)` |
| channel type | `(chan any n)` or `(chan map[string]any n)` |
| complex channel type | `(chan (chan any) n)` — parens around `chan T` types |
| slice type | `[]any`, `[]string` — in params/return positions |
| multi-return type | `[string error]` in `-> [string error]` |
| package-qualified | `web/Request`, `*pgx/Conn` — slash→dot via `qualifiedTypeToGo` |

`->` for return types: a bare return-type symbol after params would be ambiguous (indistinguishable from first body expr). `->` makes return type unambiguous. `parseTypeExpr()` in `parser.go` handles complex types by reading individual tokens without needing `^`.

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

**web API**: all web functionality lives in `web/web.go` as plain Go — no special transpiler forms. `Request` and `Response` are type aliases for `map[string]any` (use `web/Request` / `web/Response` in glisp type positions); the request map carries `"method"`, `"path"`, `"query"`, `"headers"`, `"body"`, `"remote-addr"`, `"host"`, `"scheme"`. `Handler` is `func(Request) Response`. Middleware signature is `func(Handler) Handler`. `wrap(h Handler, mws ...Middleware)` applies middlewares outermost-first. `wrap-json` stores parsed body in `req["json-body"]`; `wrap-auth` stores the Bearer token in `req["identity"]`; `wrap-auth-func` takes a `(fn [token string] -> bool)` validator; `wrap-cors` short-circuits `OPTIONS` preflight requests with 204 + CORS headers; `wrap-recover` logs the panic + stack via `slog` and returns a generic 500. `serve-files` bridges Ring ↔ `http.FileServer` via `httptest.ResponseRecorder`, forwarding request headers so conditional/Range requests work. `serve-graceful` traps SIGINT/SIGTERM and shuts down with a 5 s context deadline. HTTP route helpers: `(web/get path handler)`, `(web/post path handler)`, etc. Route patterns support `:name` segment params (URL-decoded) and a trailing `*name` wildcard capturing the rest of the path — so `(web/get "/static/*path" (web/serve-files "/static/" dir))` composes with `routes`. `routes` accepts `Route` values and `[]Route` groups from `(web/context prefix & routes)` (nestable prefix grouping); it returns 405 + `Allow` header when the path matches but the method doesn't, 404 otherwise. Response `"status"` may be `int`/`int64`/`float64` (`statusOf`). Request helpers: `query-param`, `path-param`, `form-param` (urlencoded body), `header` (case-insensitive), `cookie`, `body-map`; `set-cookie` adds a `Set-Cookie` header to a response. JSON error-response helpers `web/bad-request` (400), `web/unauthorized` (401), `web/not-found` (404), `web/server-error` (500) take a message string and return `{"error": msg}`; `web/no-content` (204) takes no args. New public `web` functions need a matching doc entry in `internal/lsp/builtins.go`.

**Concurrency primitives** — six forms beyond the basic `go`/`chan`/`send!`/`recv!`/`close!`/`select!`:

| Form | Emits | Notes |
|---|---|---|
| `(go-val body...)` | IIFE → `chan any` with buffered 1-slot channel + goroutine | Returns immediately; caller `(recv! ch)` to collect the result. Body runs in a goroutine that sends via `_ch <- func() any { return ... }()`. |
| `(par body1 body2 ...)` | `{ var _wg sync.WaitGroup; _wg.Add(n); go func()...}` | N bodies run concurrently; `_wg.Wait()` blocks until all finish. Auto-imports `"sync"`. |
| `(for-chan [x ch] body...)` | `for x := range ch { body }` | Iterates until channel is closed. Separate from `doseq` — `for x := range slice` gives index, not value. |
| `(recv-ok! ch)` | `func() []any { _v, _ok := <-ch; return []any{_v, _ok} }()` | Use with `[[val ok] (recv-ok! ch)]` destructuring. `(if ok ...)` works directly — conditions are truthy-wrapped (ADR-011). |
| `(with-lock mu body...)` | `func() any { mu.Lock(); defer mu.Unlock(); body }()` | IIFE ensures unlock even on panic. `mu` is evaluated twice — use a symbol, not a complex expr. Auto-imports `"sync"`. |
| `:timeout ms` in `select!` | `case <-time.After(ms * time.Millisecond):` | Add as a case in any `select!`; fires after `ms` milliseconds. Auto-imports `"time"`. |

**Collection helpers accept concrete slices**: `_glispToSlice` is the single conversion point for all sequence helpers (`map`, `filter`, `reduce`, `first`, `rest`, `doseq`, `sort`, `contains?`, `get`, `conj`, `flatten`, …) and handles `[]any` plus common concrete slices: `[]string` (e.g. `os/args`), `[]int`, `[]int64`, `[]float64`, `[]bool`, `[]map[string]any`. `_glispGet` on a slice is bounds-checked — out-of-range returns nil, matching map-lookup semantics. Unknown slice types convert to nil (helpers see an empty sequence).

**`defmethod` — receiver methods**: `(defmethod *ReceiverType name [self params...] -> RetType body)` emits `func (self *ReceiverType) Name(params) RetType { body }`. The receiver type before the method name uses `*T` for pointer receivers, `T` for value receivers. The first element of the params vector is the receiver variable name; remaining params are regular params. Together with `definterface` and `defstruct`, this is the full Go interface/struct/method triad.

**Truthiness (ADR-011)**: `nil` and `false` are falsy; every other value is truthy. Conditions in `if`/`when`/`cond` (all positions, incl. loop tails), `and`/`or`/`not` operands, and `assert-true`/`assert-false` are emitted via `emitCondition` (`emit_expr.go`): expressions statically known to be Go `bool` — comparisons, logic ops, the `boolBuiltins` set, user fns declared `-> bool` (looked up in `e.symbols`) — emit as-is; everything else wraps in the always-present runtime helper `_glispTruthy(v)`. So `(if (get m "k") ...)`, `(when user ...)`, `(if ok ...)` after `recv-ok!` all work directly on `any` values. `and`/`or` still *return* Go `bool` (not the last value). `if-let`/`when-let`/`let-or` deliberately keep nil-guard (`!= nil`) semantics — a bound `false` is a present value. When adding a bool-returning built-in, add it to `boolBuiltins` so conditions skip the wrapper.

**Statement-only tails auto-return (ADR-011)**: `go`, `select!`, `par`, `for-chan`, `fan-out`, `defer`, `send!`, `close!` in the tail position of a value-returning function emit the statement followed by `return nil` (`emitReturnNode` in `transpiler.go`). No trailing `nil` needed. `-> void` fns are unaffected (their bodies never hit return position).

**`len` / `count`**: aliases; both emit `_glispLen`, which accepts `any` (strings, `[]any`, common concrete slices/maps, sets). Unknown types count as 0.

**`any`-type constraints** — the transpiler emits `any` for most values retrieved at runtime (map lookups, collection elements, loop vars). Remaining cases that cause Go compile errors (per ADR-011, each should eventually be absorbed or turned into a glisp-level diagnostic):

| Situation | What breaks | Fix |
|---|---|---|
| any multi-return Go fn (e.g. `os/create`) as the tail of a `func(...) any` closure | `(T, error)` can't coerce to `any` | `(do (os/create ...) nil)` — note: `fmt/println` and `fmt/print` are handled automatically |
| `(defn f [] -> int (reduce ...))` | `_glispReduce` returns `any`, not `int` | either use `-> any` return type and cast at call sites, or wrap: `(int (reduce ...))` inline |
| `(defn f [] ...)` with no `-> RetType`, void-ish body | Go generates `func f()` (void); using it in return position fails | add `-> void` for true void functions; add `-> any` only if the fn is used in expression/return position |

## Formatter

`glisp fmt` pretty-prints `.glsp` source from the parsed AST. **`;` and `;;` comments are preserved in place** — both as leading comments of a form and interleaved within form bodies (between body statements, `let` bindings, etc.); blank lines between a comment and the following form are normalized away. The `cfmt` struct threads the `CommentMap` + a `used` set through the recursive formatters, consuming comments depth-first; a form that contains a comment is forced multi-line (via `inlineOK`/`nodeMaxLine`) so an inline rendering never drops it. Trailing inline comments (after code on the same line) are not preserved. `;;;` docstrings are preserved via `DefnDecl.Doc`/`MethodDecl.Doc`; consecutive `;;;` lines accumulate into one multi-line docstring (`Doc` holds the lines `\n`-joined; the parser appends in `parseAll`, the formatter re-emits one `;;;` line per line via `formatDoc`). An **orphan `;;;` block** (not attached to a `defn`/`defmethod`, e.g. a file-level docstring before `ns`) is recorded by the parser in `orphanDocs` and surfaced to the formatter through the `CommentMap`, so it is preserved as a leading comment rather than dropped. **Channel types** (`(chan T)`) lose their parens at parse time (`TypeExpr.Text` is `"chan T"`); the formatter's `tt`/`wrapChanType` helper re-adds them at every type-emission site so the output round-trips. The full **concurrency form family** round-trips: `select!` (cases emit `[binding chan]` / `[(send! ch v)]` / `:default` / `:timeout ms` via `selectCaseHead`), `for-chan`, `par`, `with-lock`, `recv-ok!`, `go-val`, `pipeline`, `fan-out`, `fan-in`, and `let-or` each have `inline`+`format`+`nodeMaxLine` cases. Algorithm: try inline rendering first; use it if `indent*2 + len <= 80`, else multi-line. Map literals with >1 pair are always multi-line with value-column alignment. `defn`/`defstruct`/`definterface`/`defmethod`/`deftest`/`cond` are always multi-line.

```
glisp fmt file.glsp          # format in-place
glisp fmt --check file.glsp  # exit 1 if not already formatted
make fmt-glsp                # format all examples/*.glsp
```

Always format example `.glsp` files with a freshly built binary (`make fmt-glsp` rebuilds `glisp` first, then formats) — never a stale/cached one.

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
(defn Add [a int b int] -> int (+ a b))

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

(defn Connect [url string] -> [any error]
  (pgx/connect context/background url))

(defn Exec [conn any sql string] -> [any error]
  (let [typed (as *pgx/Conn conn)]
    (.Exec typed context/background sql)))
```

Key rules for Go-wrapping modules:
- Use `(:import [...])` for external/vendor Go packages (stdlib is auto-imported; no declaration needed); `(:require [...])` only for other glisp modules
- Return opaque Go types as `any`; callers type-assert with `(as *pkg/Type val)` when methods are needed
- Type syntax for pointer-to-external-type: `*pgx/Conn` → `*pgx.Conn` (slash→dot via `qualifiedTypeToGo`)
- The package qualifier comes from the last path segment: `(:import [github.com/jackc/pgx/v5])` → qualifier is `pgx`
- Go interop primitives available: `(.Method obj args)` for method calls, `(.-Field obj)` for field access, `(Type. {:field val})` for struct literals, `(as T val)` for type assertions
- **Bridge pattern for variadic Go APIs**: write a hand-written `bridge.go` (same package) with unexported Go helpers that handle variadic spreading and type assertions; call them from glisp as unqualified names. Unqualified calls use `identToGo` (camelCase, lowercase first letter), so `(bridge-query ...)` → `bridgeQuery`. The bridge functions must be unexported (lowercase) — they're only accessible within the package.
- **Collections accept common concrete slices**: `_glispToSlice` converts `[]string`, `[]int`, `[]int64`, `[]float64`, `[]bool`, and `[]map[string]any` (in addition to `[]any`), so bridge functions returning those work directly with `map`/`filter`/`reduce`/`first`/`rest`/`get`/`contains?`. A bridge returning some *other* concrete slice type should still convert to `[]any` before returning.

## Go module

Go module name: `golisp`. Go version in `go.mod`.
