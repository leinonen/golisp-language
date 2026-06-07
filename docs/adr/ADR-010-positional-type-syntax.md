# ADR-010: Positional type syntax — no `^` prefix

**Status**: Accepted

## Context

glisp's original type annotation syntax borrowed the `^` prefix from Clojure's metadata reader:

```clojure
(defn ^string greet [^string name] ...)
(defstruct Circle ^float64 radius)
(defmethod ^*Circle Area [c] ^float64 ...)
(chan ^any 10)
(as ^*pgx/Conn conn)
```

This created several friction points:

- **Visual noise** — `^` appears before every type, making signatures feel cluttered compared to Go source or other typed languages.
- **Clojure false cognate** — in Clojure, `^` attaches metadata to any form; in glisp it was only valid in specific positions and meant something different (Go type annotation, not arbitrary metadata). New readers were misled.
- **Lexer coupling** — the lexer had to handle `^` as a special token (`TokenTypeAnnot`) that read the raw type text as a single blob, making complex types like `^map[string]any` fragile and hard to extend.
- **Return type ambiguity required `^`** — for `defn`, the return type came _before_ the function name (`(defn ^string greet [...])`), which reads backwards and required scanning past the name to find the param list.

## Decision

Remove `^` entirely. Use **positional types** throughout:

| Form | Old | New |
|---|---|---|
| `defn` — single return | `(defn ^string f [^int x] ...)` | `(defn f [x int] -> string ...)` |
| `defn` — multi-return | `(defn ^[string error] f [^int x] ...)` | `(defn f [x int] -> [string error] ...)` |
| `fn` | `(fn [^int x] ^string body)` | `(fn [x int] -> string body)` |
| `defmethod` | `(defmethod ^*Circle Area [c] ^float64 body)` | `(defmethod *Circle Area [c] -> float64 body)` |
| `defstruct` | `(defstruct Circle ^float64 radius)` | `(defstruct Circle radius float64)` |
| `definterface` | `(definterface S (Area [] ^float64))` | `(definterface S (Area [] -> float64))` |
| `def` (typed) | `(def ^int x 42)` | `(def x int 42)` |
| channel type | `(chan ^any n)` | `(chan any n)` |
| type assertion | `(as ^*Circle val)` | `(as *Circle val)` |

**Params**: name then type — `[score int]`, `[a int b int]`. Both are required when typing a param; omit both for untyped params.

**Return type**: the `->` marker after the params vector makes the return type unambiguous. Without it, a bare symbol after `[params]` is indistinguishable from the first body expression. `->` reads naturally ("returns"), matches Rust/Haskell/Swift convention, and is already a valid glisp identifier (threading macro).

**`defstruct`**: field name first, type second — `(defstruct Circle radius float64 height float64)` — consistent with params.

**`as` and `chan`**: type is the first positional argument. The special-form context makes it unambiguous; no marker needed.

**`let` bindings**: type annotations in `let` are dropped. Use `(as T expr)` wrapping at the call site when a concrete type is needed.

## Implementation

The `^` token is removed from the lexer entirely — `^` is now an "unexpected character" parse error. A new `parseTypeExpr()` function in the parser reconstructs complex types (`[]T`, `map[K]V`, `chan T`, `*T`, `pkg/T`) from individual tokens without needing a single blob token. `isTypeSymbol()` distinguishes type names from variable names in param vectors by checking against known Go primitive types and naming conventions (uppercase initial → type, lowercase only with no `/` → variable).

The transpiler output is unchanged — only parsing and formatting are affected. All existing golden test outputs remain identical.

## Alternatives considered

**Keep `^` but fix the return type position** — put the return type after params instead of before the name, still with `^`: `(defn f [^int x] ^string ...)`. Eliminated: `^` is still visual noise and the Clojure false cognate remains.

**Haskell-style `::` annotation** — `(defn f [x :: int] :: string ...)`. Eliminated: `::` is not a natural glisp symbol; it adds two characters where zero suffice once types are positional.

**Type inference — omit types entirely** — let the transpiler infer Go types. Eliminated: glisp transpiles to Go without a full type-inference pass; explicit types are needed at boundaries where Go requires them (exported functions, struct fields, interface methods, function signatures).

**Keyword annotation** — `(defn f [:ret string x :int] ...)`. Eliminated: mixes control flow keywords with identifier/type pairs; hard to parse and read.

## Consequences

- **Breaking change** — all existing `.glsp` source files must be updated. The old `^` syntax raises a parse error.
- **Cleaner signatures** — `(defn handler [req web/Request] -> web/Response ...)` reads left-to-right without prefix noise.
- **Simpler lexer** — `TokenTypeAnnot` and `readTypeAnnot` are deleted; the lexer is ~30 lines shorter.
- **Richer type parser** — `parseTypeExpr()` handles all Go type forms composably, making future type-language extensions (generics, constraints) straightforward to add.
- **`->` dual role** — `->` is both the threading macro and the return-type separator. In practice these never appear in the same syntactic position (threading macros appear in expression position; `->` after a param vector is unambiguously a return-type marker). No ambiguity arises.
- **Type highlighting** — the Neovim syntax plugin highlights known primitive type names (`int`, `string`, `any`, etc.) as `Type` wherever they appear, replacing the old `^`-prefix pattern match.
