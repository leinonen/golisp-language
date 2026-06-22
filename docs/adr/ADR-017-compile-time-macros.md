# ADR-017: Compile-time macros

**Status**: Accepted (supersedes ADR-005; enabled by ADR-016)

## Context

ADR-005 deferred macros. Its reasoning was sound *for v1*: macros need a Lisp
evaluator before transpilation; built-in special forms covered the server use
case; the server focus made macro power less valuable; hygiene is hard. The
anti-roadmap in ADR-012 then made the deferral effectively permanent.

Three things have since changed:

1. **The identity changed (ADR-016).** glisp is now a general-purpose Lisp with
   its own standard library. ADR-005's own revisit condition — "a clear use case
   that cannot be addressed by built-in forms" — is now the *default* case: a
   broad `core` library cannot be grown by patching the transpiler for every
   form. **The library must be able to grow without the compiler.**
2. **The "add a built-in form" strategy has visibly hit its ceiling.** Phases
   6–11 added dozens of special forms (`when-let`, `as->`, `doto`, `for`,
   `case`, `assert`, `tap->`, …), each a bespoke emitter, formatter case, LSP
   entry, and arity-table row. Every one is a compiler change for what is, in a
   real Lisp, a library function. ADR-012's "one obvious way" is strained by the
   growing pile of near-redundant forms.
3. **The hard-compiler-work objection is empty now.** The team already built the
   ADR-015 typed-interop loader (shelling to `go/packages`, folding `go/types`
   into the pre-pass). A tree-walking evaluator over glisp's own AST is *less*
   exotic than that, and reuses the same pre-pass architecture.

## Decision

glisp gets **compile-time macros**: `defmacro`, with syntax-quote, run by a
tree-walking evaluator during a **macroexpansion pass between parse and
transpile**.

- **Reader**: syntax-quote `` ` ``, unquote `~`, unquote-splice `~@`, `'` quote
  as a data-producing form, and auto-gensym (`name#`).
- **Homoiconic compile-time data**: forms (symbol / keyword / list / vector /
  map) are first-class values the evaluator constructs and destructures. `quote`
  yields data, not a string. (Today `defmacro` parses to the placeholder
  `"(macros not yet supported)"`; this replaces that.)
- **Evaluator**: a tree-walking interpreter over the macro subset of glisp,
  sufficient to run macro bodies. It is *not* a second compilation target and
  *not* a general runtime — it is an enrichment of the pre-pass, the same
  architectural shape as ADR-013's cross-file `DeclSet` and ADR-015's signature
  loader. Macro output is ordinary glisp AST and is transpiled normally.
- **Surface**: `defmacro`, `macroexpand`, `macroexpand-1`.
- **Hygiene**: non-hygienic, with syntax-quote symbol qualification + `gensym`
  — **Clojure's model.** This is a conscious, bounded tradeoff, documented for
  users. (It is precisely the worry ADR-005 raised; we accept it the way Clojure
  did, rather than block macros on full hygiene.)

## Why a tree-walking compile-time evaluator (not alternatives)

- **Reader/template macros only** (the rejected middle option): cover simple
  syntactic sugar but not arbitrary compile-time computation, so a real `core`
  still couldn't be expressed in glisp. Insufficient for the v2 goal.
- **A full runtime VM**: overkill, and conflicts with the transpiler model
  (ADR-002) and the single-binary build. The evaluator only needs to run
  *macro-expansion-time* code, a small, bounded subset.

## Scope and non-goals (initial)

- **In**: `defmacro`, syntax-quote/unquote/splice, `gensym`/auto-gensym,
  `macroexpand`(-1). Enough to reimplement existing control-flow forms as `core`
  macros (the validation milestone in the roadmap).
- **Out, for now** (each its own future ADR if pursued): reader macros that
  change *lexing*; user-defined tagged literals (`#inst`/`#uuid`); full hygiene;
  `eval` of arbitrary runtime code. The macro evaluator is compile-time only.

## Consequences

- **The library can grow instead of the compiler.** Many existing special forms
  (`when`, `->`, `->>`, `as->`, `cond`, `if-let`, `when-let`, `doto`, `assert`,
  `for`, …) become `core` macros; their bespoke emitters can be retired where it
  simplifies the compiler. (Forms that must control statement/expression
  placement or touch Go's type system may stay built-in — the goal is to prove
  and shrink, not to move everything dogmatically.)
- **New tooling burdens** (all governed by the ADR-012 tooling-parity rule):
  `glisp fmt` and the LSP must degrade gracefully on user-macro call-sites
  (generic call-form path); the REPL gains `macroexpand`.
- **New error-surface front (ADR-011).** Macro-expansion errors must report at
  `.glsp` source positions, including inside expanded forms — "the user never
  debugs generated code" now extends to generated *glisp*.
- **A reusable evaluator.** Built carefully, it can later back an interpreted
  `glisp run` fast-path for scripting (roadmap open question; separate ADR).
- ADR-005 is superseded; the ADR-012 anti-roadmap line "Macros / reader macros"
  is lifted for `defmacro` (reader macros remain out pending a future ADR).
