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
| `internal/transpiler/emit_arity.go` | `builtinArity` table + `checkBuiltinArity` — central arity front-gate for built-in call forms; `multiReturnBuiltins` + `checkMultiReturnValue` — single-value-position gate for multi-return calls |
| `internal/transpiler/emit_types.go` | `identToGo`, `typeExprToGo`, `qualifiedTypeToGo`, `zeroValueFor` |
| `internal/transpiler/emit_methods.go` | Dot-free method dispatch: `collectMethodTables`, `resolveMethodCall`, `emitMethodCall`, `namedTypeHint` |
| `internal/transpiler/emit_runtime.go` | `glispRuntime` (always), `glispSortRuntime`, `glispStrRuntime`, `glispJsonRuntime`, `glispEnvRuntime`, `glispFileRuntime`, `glispReRuntime`, `glispSetRuntime` (conditional) |
| `internal/formatter/formatter.go` | AST → formatted glisp source; `Format(src)` public API |
| `internal/compiler/compiler.go` | Orchestrates pipeline: `Compile`, `CompileAndBuild`, `CompileDir`, `CompileTest`, `TranspileDir`, `Run`, `GetModule`, `ResolveDeps` |
| `internal/module/modfile.go` | `glisp.mod` parsing/writing: `ReadModFile`, `WriteModFile`, `InitModFile`; `GoRequire` type, `AddGoRequire` |
| `internal/module/resolver.go` | Module download (GitHub tar.gz), go.mod wiring: `Download`, `EnsureGoMod`, `EnsureProjectGoMod`, `RegisterInGoMod`, `RequireVersion`, `IsCached` |
| `internal/version/version.go` | Build version: ldflags-injected `Version`, VCS-info fallback; `String()`, `Full()` (used by `glisp version`) |
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

**Stdlib qualifier resolution & unknown-package diagnostic**: when `emitExpr` sees a `pkg/fn` symbol whose `pkg` is not a module alias, `resolveDirectImport` (`transpiler.go`) resolves it through `stdlibByQualifier` (`internal/transpiler/stdlib.go`, generated by `internal/transpiler/internal/stdlibgen` from `go list std`). A **unique** stdlib qualifier — single-segment (`os`) or multi-segment (`filepath` → `path/filepath`, emitting `filepath.Join`) — is added to `directImports` as its *full path*. An **ambiguous** qualifier (`rand` → `crypto/rand`|`math/rand`) or an **unknown** one raises a position-tagged glisp error (`… (at L:C)`, surfaced by the LSP) naming the `(:import …)` fix, instead of a raw "package X is not in std" Go error (go-interop-exploration P4). Safe because this branch is reached only for *undeclared* qualifiers and a bare import resolves only for stdlib. Regenerate `stdlib.go` after a Go toolchain upgrade (`go run ./internal/transpiler/internal/stdlibgen > internal/transpiler/stdlib.go`).

**No `#(...)` anonymous function shorthand**: deliberately removed. Because it was a reader-level expansion (no AST node), the parser turned `#(+ % 1)` into a plain `(fn [_anonP1] (+ _anonP1 1))` with internal `_anonP1` param names — and since the formatter only ever saw that expanded `FnExpr`, `glisp fmt` silently reshaped `#(...)` source into `(fn [...] ...)` with those ugly generated names (no round-trip). That surprising formatter behavior wasn't worth one redundant way to write a lambda, so `#(` is now a hard lex error (`internal/lexer/lexer.go`) pointing users at `(fn [x] ...)`. Always write lambdas as `(fn [x] ...)`.

**Destructuring**: `LetBinding.Pattern` is `*Symbol`, `*VectorLit` (sequential: `[[a b] coll]` → `_glispGet(tmp, int64(i))`), or `*MapLit` (map: `[{k :key} m]` → `_glispGet(tmp, "key")`). `Param.Pattern` (non-nil) enables the same in `fn`/`defn`/`defmethod` params — a temp name `_pN` is used in the Go signature and bindings are emitted at the top of the function body. Both forms use the existing `_glispGet` runtime helper. A `_` destructure element is skipped (emitting `_ := …` is illegal Go).

**Annotated destructuring (map only)**: a map destructure binding may carry a `:- Type` annotation that types the local instead of leaving it `any` — `[{name :name :- string age :age :- int} req]`. Parsed with no grammar change (the `:-` lexes as `KeywordLit("-")`, becoming an annotation pair that `mapDestructureEntries` folds into the preceding binding in `emit_expr.go`). Emission: `string`/`int`/`float64` route through the always-present `_glispToString`/`_glispToInt`/`_glispToFloat64` helpers (smart conversion — `_glispToInt`/`_glispToFloat64` handle JSON's float64 numbers *and* parse numeric strings like query/path params via `strconv`; `_glispToString` passes strings through and stringifies `[]byte`/numbers/bools, `""` otherwise — the `(string x)` cast form emits it too, never a raw Go `string(x)` conversion); any other type uses a checked assertion `name, _ := _glispGet(src, "key").(T)` (zero value if absent/wrong type). A struct-typed annotation registers the local in `e.localTypes`, so `(:field name)` keyword access works on it. Types must be simple names (`string`, `Product`, `*Product`, `web/Request`); bracketed types like `[]string` are not supported here. Works in both `let` and param patterns; sequential (`[a b]`) destructuring is unannotated.

**`if-let` / `when-let`**: `(if-let [pat expr] then else?)` / `(when-let [pat expr] body...)` (`emit_expr.go`). Binds `pat` (single symbol, or a `_glispGet`-based destructure pattern) from `expr`, then branches on **`!= nil`** (nil-guard, matching `nil?`). Truthy branch gets the bindings in scope; destructured names are emitted *inside* that branch only, so they don't leak into the else/nil branch. `if-let` with no else and `when-let`'s false case return `nil`. Constraint: binding a non-nilable concrete type (e.g. a raw `int`) won't compile against `!= nil` — bind `any`-typed values (map lookups, find-fns).

**`let-or`**: `(let-or [name1 expr1 fallback1 name2 expr2 fallback2 ...] body...)` (`emit_expr.go`). Flat sequential nil-guard bindings — each binding evaluates `expr`, checks `== nil`, and returns `fallback` immediately if nil. Flattens chains of `if-let`/`if (= x nil)` validation guards into a single level. All names are in scope for subsequent bindings and the body. Use for validating request fields before a DB call. AST: `LetOrExpr`/`LetOrBinding` in `ast/nodes.go`.

**`case` (alias for `switch`)**: `(case expr v1 b1 ... default?)` is the Clojure-style spelling of `switch` — the fallthrough is a trailing unpaired form instead of `:default`. It is **the same AST node** as `switch`: `parseCase` produces a `SwitchExpr` with `Head == "case"` (vs `"switch"`), so it reuses all of switch's emission unchanged (value/statement/typed-return via the existing `*ast.SwitchExpr` dispatch — no `any` IIFE in return position) and its formatter. `formatSwitch`/`inline` branch on `Head` only to pick the keyword and render the fallback (`:default body` for switch, trailing `body` for case); both use the same 2-space hanging-indent layout as `cond`/`switch` (NOT a Style-A pair-form — aligning clauses way out under the dispatch value read badly). A degenerate `(case x)` with no clauses or default is a parse error.

**`assert` (runtime invariant guard)**: `(assert cond)` / `(assert cond msg)` — a built-in call form (no AST node) that panics when `cond` is falsy. With no message the panic text is auto-generated from the condition's source via `formatter.FormatNode` (`assertion failed: (> n 0)`), which is what lifts it above `(when (not c) (panic msg))`. `emitAssertGuard` writes `if !(<emitCondition>) { panic(<msg>) }`; statement and return positions emit the bare guard (return position adds `return nil`), expression position wraps it in an IIFE yielding nil. Truthiness goes through `emitCondition` (ADR-011). Arity `{1,2}` via `builtinArity`; `panic`/`recover` exist for crashes and interop, `assert` for fail-fast invariants.

**Statement vs expression position**: `let`/`if`/`do`/`when`/`cond` in statement position emit as plain Go blocks. In expression position they wrap in an IIFE `func() any { ... }()`. `emitStmtNode` handles statement position; `emitExpr` handles expression position.

**Return position**: `emitReturnNode` handles tail-position nodes. `loop` in return position emits `return value` directly (no `any` temp var). The `loopInReturn bool` field tracks this.

**Loop binding types**: An explicit `TypeAnnot` on a loop binding always wins — `(loop [xs []string []] ...)` emits `var xs []string = []string{}`. Without an annotation, the old rules apply: collection-init vars use `var name any = init` (so `recur` can rebind with `any`-returning helpers like `_glispConj`); scalar inits use `:=` for concrete Go inference. See `isBindingTypeStart()` in `parser.go` for the disambiguation heuristic (excludes `(` to avoid false positives with value expressions like `(fn ...)`).

**Struct type inference (gradual typing)**: two related features let map-shaped, Clojure-style code compile to typed Go structs when the surrounding type is known — `any` stays the fallback, never a requirement. A pre-pass in `emitFile` populates `e.structs` (glisp struct name → field table) and richer `e.symbols` entries (`fnSig.paramTypes`/`retType`). Machinery lives in `emit_typeinfo.go`.

- **Typed map literals → structs**: a plain `{:field v}` in a *struct-typed position* emits `Struct{Field: v}` instead of `map[string]any`. Positions that supply the hint: user-function call args (matched to `fnSig.paramTypes`), typed `let`/`def` bindings, and tail/return position (via `e.currentRetType`). Routed in `emitExprWithHint` → `structHint` (recognise the type) → `emitStructLitFromMap`. Keys may be keywords, symbols, or strings; an **unknown field is a compile-time glisp error** (catches typos before Go). An explicit map `TypeAnnot` on the literal always wins. Pointer hints (`*T`) emit `&Struct{...}`.
- **Typed keyword access**: `(:field x)` emits `x.Field` (not `_glispGet`) when `x` is a variable statically known to hold a declared struct. The local type environment `e.localTypes` (glisp var → struct/interface name) is managed with `pushTypeScope`/`popTypeScope` around `defn`/`fn`/`defmethod` and `let` bodies; populated from struct/interface-typed params, the method receiver, typed `let` bindings, and inference (`inferValueStructType`: struct literals, calls to fns with a declared return type, dot-free method calls). A keyword naming no field of the known struct is a compile-time error. Untyped values (bare maps, `any` params) fall back to `_glispGet` unchanged. Only the 1-arg form upgrades; `(:k x default)` stays a runtime lookup.
- **Dot-free method dispatch**: `(area s)` emits `s.Area()` when `s` is statically known (via `e.localTypes` or value inference) to hold a locally-declared struct or interface type with a matching method — no `(.Area s)` interop needed. Machinery in `emit_methods.go`: the `emitFile` pre-pass builds `e.ifaces` (definterface method tables) and `e.methods` (defmethod tables, keyed by bare receiver name); `resolveMethodCall` runs in the `emitCallExpr` fallthrough, *after* built-ins and user functions. Resolution order: built-in forms > user `defn`s > in-scope value bindings (`e.localVars`: params, receiver, `let`/`loop`/`if-let`/`let-or`/destructure bindings, `def` globals — scoped with `pushTypeScope`) > method dispatch > plain call. Method names convert like exported module calls (`fnToGo`: `describe` → `Describe`, `to-string` → `ToString`; any-uppercase passes through), falling back to `identToGo` for unexported defmethods (`drained?` → `isDrained`). Wrong arg count is a position-tagged arity error; multi-return methods (`-> [T E]`) are gated by `checkMultiReturnValue` (use `if-err`); `-> bool` methods skip the `_glispTruthy` wrapper in conditions. Receiver expressions need an inferable type — `any` values still require `(as T v)` + `(.Method ...)` interop, which remains for external/cross-file types (per-file pre-pass: methods in other files of a multi-file build are invisible).

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

**Go-error line mapping (`//line`)**: file-mode transpiles (`TranspileFile` — every CLI build/run/test path) emit `//line file.glsp:N` directives, so Go compile errors, panic stack frames, and `t.Errorf` test failures report `.glsp` positions. Three details: each deftest assertion emits a directive both before its `if` and before the `t.Errorf` call, so a failure reports the assertion's exact line; the appended runtime-helper block is re-anchored with `//line glisp_runtime.go:1` (gated on `sawLineDir`, propagated from the two-pass `declEmitter`) so helper panic frames never point at bogus `.glsp` lines; filename-less `Transpile` (print/golden paths) emits no directives at all.

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

**Bare `println` / `print`**: aliases for `fmt/println` / `fmt/print` — written unqualified, no `fmt/` prefix. The bare spellings are added to the same three `emitFmtPrint`/`emitFmtPrintCall` dispatch sites (expression position in `emit_expr.go`, statement + return position in `transpiler.go`); `emitFmtPrintCall` treats `"print"` like `"fmt/print"` (Println otherwise). Only `println`/`print` are aliased this way — the rest of `fmt` (`fmt/printf`, `fmt/sprintf`, `fmt/errorf`, …) keeps the prefix, as do other built-in namespaces (`os/env`, `json/*`, `re/*`, `ctx/*`, `log/*`), which are language keywords with synthetic expansions and cannot be referenced unqualified or aliased.

**`log/slog` built-ins**: `log/info`, `log/debug`, `log/warn`, `log/error` — void in Go (`slog.Info` etc. return nothing). Follow the same pattern as `fmt/println`: `emitSlogCall` emits the raw call; `emitCallExpr` wraps in `func() any { …; return nil }()` for expression position; `emitStmtNode` and `emitReturnExpr` in `transpiler.go` special-case them to emit the direct call (no IIFE, no `return`). Adds `"log/slog"` to imports (not runtime-only — appears in user files directly). No import declaration needed in glisp source.

**Regex built-ins**: `re/match`, `re/find`, `re/find-all`, `re/replace`, `re/split` — dispatched by symbol name, calling `e.needImport("regexp")`. `"regexp"` is in `runtimeOnlyPkgs` so it only appears in `glisp_runtime.go` in multi-file mode. All helpers use `regexp.MustCompile` — invalid patterns panic at runtime. `re/find` returns `any` (nil on no match). Go uses RE2 syntax — no lookaheads/lookbehinds.

**Error wrapping**: `wrap-error` emits `fmt.Errorf("%s: %w", msg, err)` inline — needs `"fmt"` import. `errors/is?` emits `errors.Is(err, target)` via `emitRuntimeCall("errors.Is", ...)` — `"errors"` is already in the tracked import list.

**Context built-ins**: `ctx/background` and `ctx/todo` emit inline (`context.Background()` / `context.TODO()`) and call `e.needImport("context")` for the real import. `ctx/with-cancel`, `ctx/with-timeout`, `ctx/cancel!`, `ctx/value`, `ctx/with-value` call `e.needImport("_ctx")` — a pseudo-key that gates `glispCtxRuntime` and auto-imports `context` + `time`. Multi-return forms (`ctx/with-cancel`, `ctx/with-timeout`) return `[]any{ctx, cancel}` for use with vector destructuring. `ctx/cancel!` type-asserts to `context.CancelFunc` internally so callers don't need to; it returns `any` (nil) so it works in both statement and expression position. `ctx/done?` (→ `bool`, in `boolBuiltins`) and `ctx/err` (→ `error`) report cancellation/deadline state without `(as context/Context …)` interop. No import declaration ever needed in glisp source.

**web API**: all web functionality lives in `web/web.go` as plain Go — no special transpiler forms. `Request` and `Response` are type aliases for `map[string]any` (use `web/Request` / `web/Response` in glisp type positions); the request map carries `"method"`, `"path"`, `"query"`, `"headers"`, `"body"`, `"remote-addr"`, `"host"`, `"scheme"`. `Handler` is `func(Request) Response`. Middleware signature is `func(Handler) Handler`. `wrap(h Handler, mws ...Middleware)` applies middlewares outermost-first. `wrap-json` stores parsed body in `req["json-body"]`; `wrap-auth` stores the Bearer token in `req["identity"]`; `wrap-auth-func` takes a `(fn [token string] -> bool)` validator; `wrap-cors` short-circuits `OPTIONS` preflight requests with 204 + CORS headers; `wrap-recover` logs the panic + stack via `slog` and returns a generic 500. `serve-files` bridges Ring ↔ `http.FileServer` via `httptest.ResponseRecorder`, forwarding request headers so conditional/Range requests work. `serve-graceful` traps SIGINT/SIGTERM and shuts down with a 5 s context deadline. HTTP route helpers: `(web/get path handler)`, `(web/post path handler)`, etc. Route patterns support `:name` segment params (URL-decoded) and a trailing `*name` wildcard capturing the rest of the path — so `(web/get "/static/*path" (web/serve-files "/static/" dir))` composes with `routes`. `routes` accepts `Route` values and `[]Route` groups from `(web/context prefix & routes)` (nestable prefix grouping); it returns 405 + `Allow` header when the path matches but the method doesn't, 404 otherwise. Response `"status"` may be `int`/`int64`/`float64` (`statusOf`). Request helpers: `query-param`, `path-param`, `form-param` (urlencoded body), `header` (case-insensitive), `cookie`, `body-map`; `set-cookie` adds a `Set-Cookie` header to a response. JSON error-response helpers `web/bad-request` (400), `web/unauthorized` (401), `web/not-found` (404), `web/server-error` (500) take a message string and return `{"error": msg}`; `web/no-content` (204) takes no args. New public `web` functions need a matching doc entry in `internal/lsp/builtins.go`. **Hiccup HTML rendering** (`web/html.go`, stable): `(web/html node)` renders `[:tag {:attrs …} children…]` vectors to HTML — keywords-as-strings make hiccup trees ordinary glisp data. Text/attribute values are escaped by default (`web/raw` is the explicit opt-out for trusted markup); `#id.class` tag shorthand; a `[]any` child whose first element isn't a string splices (so `map` output drops in); boolean attrs render bare/omitted; void elements get no closing tag; attrs render sorted (deterministic). `web/html-page` prefixes `<!DOCTYPE html>`; `(web/render-response status node)` wraps in a text/html response. Reference app: `examples/todos` (hiccup + htmx). **SSE** (`web/sse.go`, stable): `(web/sse-response ch)` streams a `chan any` as `text/event-stream` (string → data line; map may carry `"event"`/`"data"`/`"id"`/`"retry"`; idle streams emit a keepalive comment every 15 s — `"keepalive"` seconds key overrides, 0 disables). The producer runs in its own goroutine: start it with `(web/go-recover (fn [] …))` (panics are logged, not fatal — the goroutine analog of `wrap-recover`), end the stream with `(defer (close! ch))`, and race `(web/done req)` in `select!` to stop when the client disconnects. `web/done` is lazy — the disconnect-bridge goroutine is created on first call and cached in `req["done"]`; the raw request context is stashed under the private `"__context"` key by `RingToHTTP`. **Websockets** (`web/ws.go`, stable): dependency-free RFC 6455 — `(web/websocket (fn [req in out] …))` wraps the handler into a route; text messages arrive on `in` as strings, binary as `[]byte` (closed on disconnect); values sent on `out` go out as text frames (`[]byte` → binary). Returning from the handler closes the connection (queued `out` messages are flushed, then close 1000). Hardening: client masking enforced, fragmented messages reassembled, UTF-8 validation (close 1007), protocol violations (RSV bits, oversized/fragmented control frames, orphan continuations) close 1002, 1 MiB message cap closes 1009 (`"max-message"` bytes key on a raw `{"websocket" h}` response overrides), per-frame write deadlines, idle server pings every 30 s, client close codes echoed. No permessage-deflate/subprotocols. Validated against `coder/websocket`. SSE/websocket/hiccup responses are escape-hatch response values interpreted by `RingToHTTP` — the Ring handler model is unchanged.

**Concurrency primitives** — six forms beyond the basic `go`/`chan`/`send!`/`recv!`/`close!`/`select!`:

| Form | Emits | Notes |
|---|---|---|
| `(go-val body...)` | IIFE → `chan any` with buffered 1-slot channel + goroutine | Returns immediately; caller `(recv! ch)` to collect the result. Body runs in a goroutine that sends via `_ch <- func() any { return ... }()`. |
| `(par body1 body2 ...)` | `{ var _wg sync.WaitGroup; _wg.Add(n); go func()...}` | N bodies run concurrently; `_wg.Wait()` blocks until all finish. Auto-imports `"sync"`. |
| `(for-chan [x ch] body...)` | `for x := range ch { body }` | Iterates until channel is closed. Separate from `doseq` — `for x := range slice` gives index, not value. |
| `(recv-ok! ch)` | `func() []any { _v, _ok := <-ch; return []any{_v, _ok} }()` | Use with `[[val ok] (recv-ok! ch)]` destructuring. `(if ok ...)` works directly — conditions are truthy-wrapped (ADR-011). |
| `(with-lock mu body...)` | `func() any { mu.Lock(); defer mu.Unlock(); body }()` | IIFE ensures unlock even on panic. `mu` is evaluated twice — use a symbol, not a complex expr. Auto-imports `"sync"`. |
| `:timeout ms` in `select!` | `case <-time.After(ms * time.Millisecond):` | Add as a case in any `select!`; fires after `ms` milliseconds. Auto-imports `"time"`. |

**Collection helpers accept concrete slices (and sets)**: `_glispToSlice` is the single conversion point for all sequence helpers (`map`, `filter`, `reduce`, `first`, `rest`, `doseq`, `sort`, `contains?`, `get`, `conj`, `flatten`, …) and handles `[]any` plus common concrete slices: `[]string` (e.g. `os/args`), `[]int`, `[]int64`, `[]float64`, `[]bool`, `[]map[string]any` — and sets (`map[any]struct{}`), which enumerate in **sorted order** (insertion sort via `_glispKeyLess`; no sort-package dependency in the base runtime) so set iteration is deterministic. `(set coll)` (`_glispToSet`, gated on the `_set` pseudo-key) builds a set from any sequence; `into` accepts a `#{}` target. `_glispGet` on a slice is bounds-checked — out-of-range returns nil, matching map-lookup semantics. Unknown slice types convert to nil (helpers see an empty sequence).

**HOF function arguments — keyword lowering + typed-fn gate**: `emitRuntimeArg` (`emit_expr.go`) intercepts the function-position argument of every runtime-helper call listed in `fnArgHelpers` (`map`/`filter`/`sort-by`/`group-by`/`max-by`/`partial`/`comp`/`juxt`/…; index -1 = all args). Two behaviors: (1) a bare `*ast.KeywordLit` lowers to `func(_kwM any) any { return _glispGet(_kwM, "k") }`, enabling `(map :title coll)`; (2) a `*ast.Symbol` resolving to a user `defn` (via `e.symbols`, unless shadowed in `e.localVars`) whose signature isn't all-`any` is a position-tagged transpile error (`hofIncompatibleReason`) — the runtime helpers assert `func(any) any`, so a `func(int) int` would otherwise panic at runtime with an opaque interface-conversion message. Variadic fns are exempt (`apply`'s default case handles `func(...any) any`). When adding a new HOF runtime helper, add its row to `fnArgHelpers`.

**`defmethod` — receiver methods**: `(defmethod *ReceiverType name [self params...] -> RetType body)` emits `func (self *ReceiverType) Name(params) RetType { body }`. The receiver type before the method name uses `*T` for pointer receivers, `T` for value receivers. The first element of the params vector is the receiver variable name; remaining params are regular params. Together with `definterface` and `defstruct`, this is the full Go interface/struct/method triad. Calling: prefer dot-free dispatch `(area c)` / `(describe s)` on typed values (see "Dot-free method dispatch" above); `(.Area c)` interop remains for untyped/`any`/external values.

**Truthiness (ADR-011)**: `nil` and `false` are falsy; every other value is truthy. Conditions in `if`/`when`/`cond` (all positions, incl. loop tails), `and`/`or`/`not` operands, and `assert-true`/`assert-false` are emitted via `emitCondition` (`emit_expr.go`): expressions statically known to be Go `bool` — comparisons, logic ops, the `boolBuiltins` set, user fns declared `-> bool` (looked up in `e.symbols`) — emit as-is; everything else wraps in the always-present runtime helper `_glispTruthy(v)`. So `(if (get m "k") ...)`, `(when user ...)`, `(if ok ...)` after `recv-ok!` all work directly on `any` values. `and`/`or` still *return* Go `bool` (not the last value). `if-let`/`when-let`/`let-or` deliberately keep nil-guard (`!= nil`) semantics — a bound `false` is a present value. When adding a bool-returning built-in, add it to `boolBuiltins` so conditions skip the wrapper.

**Statement-only tails auto-return (ADR-011)**: `go`, `select!`, `par`, `for-chan`, `fan-out`, `defer`, `send!`, `close!` in the tail position of a value-returning function emit the statement followed by `return nil` (`emitReturnNode` in `transpiler.go`). The same rule applies in **loop tails** (incl. as `if`/`cond` branches reached through the tail): the statement is followed by `break` (expression position) or `return nil` (return position) — a `recur` inside a `select!` case emits `continue` and skips it (`emitLoopTailNode` in `emit_loop.go`). No trailing `nil` needed. `-> void` fns are unaffected (their bodies never hit return position). Related select!/statement absorptions: a `_` recv binding emits `case <-ch:` (no illegal `_ :=`), and bare scalar literals in statement position (e.g. a `nil` case body) emit nothing (`emitStmtNode`).

**`len` / `count`**: aliases; both emit `_glispLen`, which accepts `any` (strings, `[]any`, common concrete slices/maps, sets). Unknown types count as 0.

**`any`-type constraints** — the transpiler emits `any` for most values retrieved at runtime (map lookups, collection elements, loop vars). Remaining cases that cause Go compile errors (per ADR-011, each should eventually be absorbed or turned into a glisp-level diagnostic):

| Situation | What breaks | Fix |
|---|---|---|
| an *unknown* multi-return Go interop fn (e.g. `os/create`) as the tail of a `func(...) any` closure | `(T, error)` can't coerce to `any` | `(do (os/create ...) nil)`; the Go error is //line-mapped to the `.glsp` source. Known multi-return calls — the `multiReturnBuiltins` set (`parse-int`, `json/encode`, `read-file`, `http/*`, …) and user fns declared `-> [T E]` — are caught at transpile time instead: using one as a single value (fn/loop tail, `let`/`if-let`/`let-or`/`def` binding) is a position-tagged glisp error suggesting `if-err`. Statement position and pass-through from a multi-return fn stay legal. When adding a multi-return built-in, add it to `multiReturnBuiltins`. |
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

Golden files live in `internal/transpiler/testdata/`. Each `.glsp` has a matching `.go.golden`. Run with `-update` to regenerate after intentional output changes. `TestGoldenCompiles` additionally `go vet`s every golden and fails on **import-emission defects** (missing/spurious/bogus imports — the class text comparison can't catch); `any`-seam type mismatches in fragment goldens are ignored. It's skipped under `-short`.

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
The qualifier in glisp code is the last path segment: `(mathlib/add 3 4)`. A trailing Go major-version segment is skipped (`github.com/jackc/pgx/v5` → qualifier `pgx`), and an explicit alias may be given with `:as` — `(:import [github.com/mattn/go-sqlite3 :as sqlite])`. Both clauses accept bare paths in one vector, one vector per path, and nested `[path :as alias]` vectors interchangeably (parsed by `parseSpecList` in `parser.go`; qualifier matching in `isModuleAlias`/`pathQualifier` in `transpiler.go`).

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
glisp mod init [module-path]         create glisp.mod AND go.mod for a new module/app
glisp get [-go] <module>[@version]   fetch a glisp module, or a Go package (-go / auto-detect)
glisp build <dir/>                   auto-fetches missing deps from glisp.mod before building
```

**`go.mod` is a derived artifact of `glisp.mod`** (`module.EnsureProjectGoMod`): a `glisp.mod` + `*.glsp` checkout is sufficient to build. `glisp mod init` writes both files; `glisp build`/`run` (dir path) and `glisp get` create `go.mod` from `glisp.mod`'s module path (basename fallback) when it's missing, and sync every app-level `go-require` entry into `go.mod` (previously declared-but-ignored). So the §2.4 "run `glisp mod init`" suggestion is now true, and fresh clones build with one command.

**`glisp get` fetches Go packages, not just glisp modules** (go-interop-exploration P1): it tries glisp-module resolution first and, when the target clearly isn't a glisp module (`compiler.IsNotGlispModuleErr`: no `.glsp` files, no fetchable glisp release, unsupported host), falls back to `go get` — inheriting the Go toolchain's proxy, checksum db, GOPRIVATE auth, and semver/`@latest` resolution. `-go` forces the Go path. The resolved version is recorded under `go-require` in `glisp.mod` (the single source of truth); glisp modules are recorded under `require`. `get` also bootstraps a minimal `glisp.mod`+`go.mod` so it works in a fresh directory.

**How glisp-module deps are wired**: `glisp get` downloads the GitHub archive to `~/.glisp/pkg/mod/<path>@<version>/`, transpiles `.glsp` → `.go` there, writes a `go.mod` for the module, then runs `go mod edit -require` and `-replace` in the project's `go.mod` so standard `go build` can find it. The cache `replace` path is an absolute, per-machine path under `~/.glisp`, so a committed `replace` is invalid after a fresh clone (a different home dir). `glisp build`'s `ResolveDeps` self-heals this: for each `require` in `glisp.mod` it fetches the module from GitHub if it isn't cached, and — whether just fetched or already cached — rewrites the project's `replace` to this machine's cache whenever `module.ProjectReplaceValid` reports the existing one is missing or points at a non-existent directory (an absolute replace to an existing dir, or any relative replace, is left untouched, so a manual local fork is preserved).

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
  (:import [github.com/jackc/pgx/v5]))

(defn Connect [url string] -> [any error]
  (pgx/connect (ctx/background) url))

(defn Exec [conn any sql string] -> [any error]
  (let [typed (as *pgx/Conn conn)]
    (.Exec typed (ctx/background) sql)))
```

Key rules for Go-wrapping modules:
- Use `(:import [...])` for external/vendor Go packages (stdlib is auto-imported; no declaration needed); `(:require [...])` only for other glisp modules
- Return opaque Go types as `any`; callers type-assert with `(as *pkg/Type val)` when methods are needed
- Type syntax for pointer-to-external-type: `*pgx/Conn` → `*pgx.Conn` (slash→dot via `qualifiedTypeToGo`)
- The package qualifier comes from the last path segment: `(:import [github.com/jackc/pgx/v5])` → qualifier is `pgx`
- Go interop primitives available: `(.Method obj args)` for method calls, `(.-Field obj)` for field access, `(Type. {:field val})` for struct literals, `(as T val)` for type assertions
- **Bridge pattern for variadic Go APIs**: write a hand-written `bridge.go` (same package) with unexported Go helpers that handle variadic spreading and type assertions; call them from glisp as unqualified names. Unqualified calls use `identToGo` (camelCase, lowercase first letter), so `(bridge-query ...)` → `bridgeQuery`. The bridge functions must be unexported (lowercase) — they're only accessible within the package.
- **Collections accept common concrete slices**: `_glispToSlice` converts `[]string`, `[]int`, `[]int64`, `[]float64`, `[]bool`, and `[]map[string]any` (in addition to `[]any`), so bridge functions returning those work directly with `map`/`filter`/`reduce`/`first`/`rest`/`get`/`contains?`. A bridge returning some *other* concrete slice type should still convert to `[]any` before returning.

## Releases

Pushing a `v*` tag triggers `.github/workflows/release.yml`: a 6-target matrix (`linux`/`darwin`/`windows` × `amd64`/`arm64`) cross-compiles `glisp` + `glisp-lsp` with `CGO_ENABLED=0 -trimpath -ldflags "-s -w -X golisp/internal/version.Version=<tag>"`, packages each into a `tar.gz` (`.zip` for windows) named `glisp_<tag>_<os>_<arch>`, and a single release job downloads all artifacts, writes `SHA256SUMS`, and publishes them to the GitHub Release (`softprops/action-gh-release`, `generate_release_notes`). The runner drops the machine-local `glispdb` replace first (same as CI). `make dist [VERSION=…]` reproduces the archives locally. `install.sh` (advertised in the README via `curl … | sh`) detects OS/arch, resolves the latest tag from the GitHub API, verifies the checksum, and installs both binaries. `glisp version` prints `internal/version.Full()` — the injected tag at release, a VCS pseudo-version otherwise.

`go install <repo>@latest` is **not** supported: the Go module path is `golisp`, not the repo URL, so a remote `go install` can't resolve it — install via the script or from a source checkout.

## Go module

Go module name: `golisp`. Go version in `go.mod`.
