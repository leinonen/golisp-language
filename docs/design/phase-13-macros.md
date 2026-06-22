# Phase 13 design — the macro engine

**Status**: Design (implements [ADR-017](adr/ADR-017-compile-time-macros.md);
gates v2 — see [ROADMAP.md](../ROADMAP.md) Phase 13)

ADR-017 accepted compile-time macros. This document is the engineering plan: the
concrete pipeline change, reader grammar, compile-time data model, evaluator,
expansion algorithm, hygiene, tooling impact, and a staged delivery order. It is
deliberately specific about *files and seams* in the current tree so the work can
start without re-deriving the architecture.

## 1. The shape of the change

A macro is a compile-time function from forms to a form. We add **one new pass**
— macroexpansion — between parse and emit. Nothing about the transpiler model
(ADR-002) changes: macro output is ordinary glisp AST and is transpiled exactly
as hand-written AST is. The evaluator is *not* a runtime, *not* a second
compilation target — it is an enrichment of the pre-pass, the same architectural
shape as ADR-013's cross-file `DeclSet` and ADR-015's signature loader.

```
source.glsp → lexer → parser → [MACROEXPAND] → transpiler → Go → gofmt → go build
                                     ▲
                          new: internal/macro
```

**Insertion point** (single, well-defined): `transpileInternal`
(`internal/transpiler/transpiler.go:157`) currently does
`tokens → parser.ParseSource(File) → e.emitFile(nodes)`. The new pass sits
between:

```go
nodes, parseErr = parser.ParseSource(tokens, src)   // unchanged
nodes, mErr := macro.Expand(nodes, macroEnv)        // NEW
if mErr != nil { return ... }                        // glisp-level error, //line-mapped
e.emitFile(nodes)                                     // unchanged
```

Three call sites must route through `Expand`:

1. **Single-file builds** — `transpileInternal` (above).
2. **Directory builds** — `TranspileNoRuntimeFileExt` (`compiler.go:380`). Macros
   defined in one file must be visible in sibling files of the same package, so
   macro collection joins the existing `DeclSet` pre-pass (`CollectDecls`): a
   first sweep registers every file's `defmacro`s, then each file is expanded
   with the package-wide macro env. This mirrors how cross-file structs/methods
   already work (ADR-013).
3. **LSP** — parse-only paths (`internal/lsp/*`) should expand best-effort for
   diagnostics/hover but never hard-fail on a macro error (defense-in-depth like
   `Server.Handle`'s recover).

**Graceful degradation**: a file with no `defmacro` and no reader-macro syntax
skips the pass entirely (zero cost for the common case), exactly like the Phase
12 signature loader skips when no external packages are imported.

## 2. The reader

### 2.1 Lexer (`internal/lexer/lexer.go`)

| Syntax | Token | Notes |
|---|---|---|
| `'form` | `TokenQuote` | **already lexed** (lexer.go:168) |
| `` `form `` | `TokenSyntaxQuote` | new — backtick |
| `~form` | `TokenUnquote` | new |
| `~@form` | `TokenUnquoteSplice` | new — two-char, lex before `~` |

Edge cases to nail down in the lexer (verified against `isSymbolChar`,
lexer.go:342, which currently allows `- _ + * / ? ! = < > . & # % | ~ $`):

- `~@` must be matched before `~` (maximal munch).
- **`~` is currently a valid symbol char and must be removed from
  `isSymbolChar`** to become a reader token. This is the one (very minor) compat
  risk: any existing symbol containing `~` would re-lex — a quick repo/examples
  grep should confirm none do before flipping it. `` ` `` and `'` are *not*
  symbol chars today, so they're free to claim.
- Confirm `@` outside `~@` stays illegal in bare symbols (deref is the
  `(deref a)` form, not `@a`, so no conflict).
- **Auto-gensym** `name#` is *not* a lexer concern — `#` is **already** a valid
  symbol char, so a trailing `#` lexes as part of the symbol with no change. The
  `name#` → fresh-symbol rewrite happens during syntax-quote expansion (§4.3).
  (Note `#{` and `#(` are special-cased ahead of symbol reading — `#{` is set
  literal, `#(` is a hard error — so a leading `#` is already reserved; only the
  trailing/embedded `#` reaches symbols.)

### 2.2 Parser (`internal/parser/parser.go`)

`parseExprInner` already dispatches `TokenQuote → parseQuote`. Today `parseQuote`
*errors* ("quote not supported in transpiler", line 425) and the `QuoteExpr` AST
node (`ast/nodes.go:423`) is unused. We wire them up and add three siblings:

- `QuoteExpr{Form}` — reuse the existing node; `parseQuote` returns it instead of
  erroring.
- `SyntaxQuoteExpr{Form}`, `UnquoteExpr{Form}`, `UnquoteSpliceExpr{Form}` — new
  reader AST nodes. Each wraps the single following form.

These reader nodes are **transient**: they are consumed by the macro evaluator
and never reach the Go emitter, *except* runtime `quote` (§3.2). A bare `~`/`~@`
outside a syntax-quote is a position-tagged parse/expand error.

## 3. The compile-time data model

Macros manipulate *forms as data*. We need one representation the evaluator reads
and writes. The pragmatic choice: **the AST nodes are the data**, wrapped in a
thin evaluator value type so the interpreter can also hold numbers, strings,
booleans, closures, and lists/maps of values.

### 3.1 Form values

The evaluator (`internal/macro`) works over `Value = any` with a small closed set
of dynamic types:

| glisp | evaluator Value | AST round-trip |
|---|---|---|
| `42`, `1.5`, `"s"`, `:k`, `true`, `nil` | Go `int64`/`float64`/`string`/keyword/`bool`/`nil` | from/to the literal nodes |
| symbol `foo` | `Sym{Name, Pos}` | `*ast.Symbol` |
| list `(a b c)` | `List([]Value)` | `*ast.CallExpr`-shaped / generic list |
| vector `[a b]` | `Vec([]Value)` | `*ast.VectorLit` |
| map `{:k v}` | `Map` (ordered) | `*ast.MapLit` |
| fn / macro | `Closure` | n/a (compile-time only) |

Two converters bridge AST ↔ Value: `nodeToValue(ast.Node) Value` and
`valueToNode(Value) ast.Node` (preserving `Pos` for error mapping). A macro is
called with its **unevaluated argument forms** converted to Values; its return
Value is converted back to an AST node and spliced into the program.

> **Why not a full runtime homoiconic value protocol?** Because Phase 13 is
> *compile-time* macros. The evaluator's Value type lives only during expansion.
> Runtime quote (§3.2) is the one place data must survive to the binary, and it
> is deliberately scoped small.

### 3.2 Runtime `quote` (scoped narrow for Phase 13)

`'form` in *ordinary* (non-macro) code must produce a runtime value. Minimum
viable:

- quoted literals/keywords/strings/numbers → themselves
- quoted vector/map → `[]any` / `map[string]any` of quoted elements
- quoted list `'(1 2 3)` → `[]any{...}`
- quoted **symbol** → a new lightweight runtime type `_glispSymbol{ Name string }`
  (a new always-or-conditional runtime helper, like `_glispAtom`)

This is enough for data literals and `macroexpand` output inspection. **Full
runtime homoiconicity** (a complete symbol/list protocol usable as general
runtime data) is explicitly deferred — tracked as an open question. Macros do not
need it; they run at compile time over Values.

## 4. The evaluator (`internal/macro`)

A tree-walking interpreter over the **macro subset** of glisp. New package:

```
internal/macro/
  value.go      — Value types, nodeToValue / valueToNode
  env.go        — lexical environment (symbol → Value)
  eval.go       — eval(form, env): the interpreter
  builtins.go   — pure built-ins available inside macro bodies
  syntaxquote.go— syntax-quote / unquote / splice / auto-gensym expansion
  expand.go     — Expand(nodes, macroEnv): the program-level pass
  expand_test.go
```

### 4.1 Subset the evaluator understands

Enough to run macro bodies, not the whole language:

- **Special forms**: `fn`, `let`, `if`, `do`, `cond`, `when`, `and`, `or`, `not`,
  `quote`, syntax-quote/unquote/splice, `def` (macro-local helpers).
- **Built-ins** (pure, `builtins.go`): list/seq ops (`list`, `cons`, `first`,
  `rest`, `nth`, `count`, `conj`, `concat`, `map`, `filter`, `reduce`, `vec`),
  symbol/keyword ops (`symbol`, `keyword`, `name`, `gensym`, `symbol?`, `list?`,
  `seq?`), map ops (`hash-map`, `get`, `assoc`, `keys`, `vals`), strings
  (`str`, `format`), arithmetic and comparison. This set is curated and grows on
  demand — it is the macro-author's toolkit.
- **Explicitly excluded**: goroutines, channels, IO, Go interop, `defn`/`defmacro`
  *inside* a macro body. A macro body that reaches outside the subset is a
  position-tagged error ("`X` is not available in a macro body"), never a silent
  miscompile.

### 4.2 `defmacro`

`(defmacro name [params] body...)` — at expansion time, `eval` builds a `Closure`
over `body` and registers it in the macro env under `name`. It emits **no Go**
(removed from the node stream). Supports `& rest` params (same spelling as `fn`).

### 4.3 Syntax-quote

`syntaxquote.go` implements the template expansion:

- `` `x `` → builds the form for `x` as data.
- `~x` inside a syntax-quote → splice the *evaluated* `x` (a Value) into that
  position.
- `~@x` → the evaluated `x` must be a sequence; splice its elements.
- **Auto-gensym**: a symbol written `foo#` inside a single syntax-quote expands
  to one consistent fresh symbol (`foo__<n>__auto`) for that whole template —
  the primary hygiene tool.
- Nested syntax-quotes: handled to one level initially; deeper nesting is a
  documented limitation (matches how rarely it's needed; Clojure-compatible
  semantics can follow).

### 4.4 Hygiene (ADR-017: non-hygienic + gensym)

- Auto-gensym (`foo#`) for macro-introduced bindings — the 90% case.
- `gensym` built-in for manual cases.
- Optional, staged: syntax-quote **qualifies** symbols that resolve to known
  `core`/global names so a macro's `(if ...)` can't be shadowed by a caller's
  local `if`. Start *without* qualification (pure non-hygiene, documented),
  add qualification once `core` exists (Phase 14) since it needs a notion of
  "known global." Document the capture footgun prominently, as Clojure does.

## 5. Expansion algorithm (`expand.go`)

```
Expand(nodes, env):
  out = []
  for node in nodes (top-level, in order):
    if node is (defmacro ...):  register macro in env; continue (emit nothing)
    else: out.append(expandForm(node, env))
  return out

expandForm(form, env):
  if form is a call (head is symbol S) and env.macro(S) exists:
     expanded = invoke macro S with form's *unexpanded* arg forms
     return expandForm(expanded, env)        // re-expand to fixed point
  else:
     recurse into children, expandForm each   // expand nested macro uses
     return form
```

Rules:

- **Define-before-use**, single top-to-bottom pass (Clojure's rule). A macro used
  before its `defmacro` is an ordinary call (and fails later as unknown) — a
  clear, position-tagged diagnostic.
- **Fixed-point re-expansion**: a macro that expands into another macro call is
  re-expanded, with a recursion cap → "macro expansion did not terminate" error.
- **Cross-file**: dir builds register all files' macros first (DeclSet pre-pass),
  then expand each file.
- **Position propagation**: every produced node carries a `Pos` (caller's call
  site by default, original form's `Pos` when known) so `//line` mapping and LSP
  diagnostics point at `.glsp` source — "the user never debugs generated code"
  (ADR-011) now extends to generated *glisp*. Expansion errors render a small
  "in macroexpansion of `name` at L:C" frame.

## 6. Surface tools

- `(macroexpand-1 'form)` / `(macroexpand 'form)` — available in the REPL and as
  a CLI affordance (`glisp macroexpand file.glsp` or a `repl` command) for
  debugging. They run the evaluator and pretty-print the resulting form via the
  formatter (`formatter.FormatNode`).
- REPL integration: expand before transpiling the entered form.

## 7. Tooling parity (ADR-012 invariant — blocks "done")

- **Formatter** (`internal/formatter`): round-trip `'`, `` ` ``, `~`, `~@`, and
  `defmacro` (its own `format`/`inline`/`nodeMaxLine` cases, like the other
  forms). A user-macro *call* already round-trips via the generic call-form path
  — verify and add tests.
- **LSP**: `defmacro` in the document outline (`symbols.go`); hover/completion
  treat user macros like `defn`s (name + docstring via the existing `;;;`
  mechanism). Diagnostics run best-effort expansion.
- **`glisp doc`**: macros documented alongside fns.

## 8. Validation milestone — eat our own cooking

The proof the system is real: reimplement existing special forms as `core`
**macros** and diff against the built-in emission. Candidates (pure rewrites, no
statement/expression-placement subtlety): `when`, `when-not`, `->`, `->>`,
`as->`, `cond` (as nested `if`), `if-let`/`when-let`, `doto`, `assert`, `for`.

Strategy: implement each as a macro, gate it behind a flag, transpile a corpus
both ways, and assert identical (or equivalent-after-gofmt) Go. Retire the
bespoke transpiler emitter only when the macro version is at parity. Forms that
must control statement vs expression position, `//line` emission, or Go typing
(e.g. `let` with typed bindings, `if` in typed return position) **stay built-in**
— the goal is to *prove and shrink*, not to move everything dogmatically.

## 9. Delivery order (maps to ROADMAP Phase 13 checkboxes)

1. **13.0 Reader** — lexer tokens + parser nodes for `'`/`` ` ``/`~`/`~@`; round-trip
   in the formatter; golden tests. No evaluation yet (`'` of a literal can emit
   runtime data to validate the path end-to-end).
2. **13.1 Evaluator core** — `internal/macro` Value model, env, `eval` over the
   subset, `builtins.go`. Unit-tested in isolation.
3. **13.2 Syntax-quote** — template expansion + auto-gensym + `gensym`.
4. **13.3 `defmacro` + Expand pass** — wire into `transpileInternal`; single-file.
5. **13.4 Cross-file** — macro collection in the `DeclSet` pre-pass; dir builds.
6. **13.5 `macroexpand`(-1)** — REPL + CLI; position-mapped error frames.
7. **13.6 Tooling** — formatter/LSP/`glisp doc` parity.
8. **13.7 Validation** — reimplement the batch in §8; retire emitters at parity.

Each step is independently shippable and testable; 13.0–13.2 carry no risk to
existing programs (purely additive).

## 10. Risks & open questions

- **Runtime quote scope.** §3.2 keeps it narrow. If real programs need richer
  runtime homoiconic data, that's a follow-up with its own ADR (the open question
  already noted in the roadmap).
- **Evaluator subset breadth.** Start minimal; grow `builtins.go` on demand. The
  risk is scope creep toward "a second implementation of the language" — guard it
  by keeping the subset a *curated macro toolkit*, not a general runtime.
- **Hygiene depth.** Shipping non-hygienic-with-gensym is a conscious ADR-017
  tradeoff; symbol qualification is staged to land with `core`.
- **Error mapping through expansion.** The biggest UX risk; §5 position
  propagation is mandatory, not optional, and needs its own tests.
- **Interaction with two-pass emit & `//line`.** Expansion runs *before*
  `emitFile`, so the existing two-pass and `//line` machinery see only final AST
  — but produced nodes must carry sane `Pos` for `//line` to stay truthful.
- **Performance.** Expansion is O(forms); negligible next to `go build`. The
  evaluator is only invoked when macros exist.
