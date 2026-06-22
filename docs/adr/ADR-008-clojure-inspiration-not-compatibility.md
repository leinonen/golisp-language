# ADR-008: Clojure inspiration, not compatibility

**Status**: Accepted (amended by ADR-016: the host relationship is now
explicitly modeled on Clojure↔Java — glisp has its own core vocabulary distinct
from the host's, and the macro non-goal below is lifted by ADR-017).

## Context

glisp looks like Clojure. It uses similar syntax, many of the same function names, and draws from the same functional programming tradition. The question is: how much Clojure compatibility should be a design goal?

## Decision

glisp is Clojure-inspired, not Clojure-compatible. Where Clojure idioms fit Go's model, we adopt them. Where they conflict, we follow Go.

## What we take from Clojure

- **Syntax** — S-expressions, keywords (`:key`), vectors (`[]`), maps (`{}`), `defn`, `let`, `fn`, `cond`, `when`, `do`
- **Function names** — `map`, `filter`, `reduce`, `assoc`, `dissoc`, `merge`, `conj`, `first`, `rest`, `get`, `keys`, `vals`, and the string/collection API
- **Threading macros** — `->`, `->>`, `as->`, `cond->`
- **Destructuring** — sequential and map destructuring in `let`, `fn`, and `defn`
- **Ring web convention** — handler as pure function (ADR-006)
- **Data-first design** — prefer plain maps and vectors over custom types; transform data with functions

## What we don't take from Clojure

- **Lazy sequences** — wrong abstraction for Go (ADR-003)
- **Reference types** — atoms, refs, agents, vars (ADR-004)
- **Macro system** — too complex for a transpiler (ADR-005)
- **Protocols** — `definterface` + `defmethod` cover the use case better
- **Namespaced keywords** — `:my.ns/key` — no practical need in a Go-compiled language
- **Metadata** — `^{:tag v}` on arbitrary forms — not expressible in the transpiler model
- **Numeric tower** — ratios, bignums — Go has no native equivalents
- **ClojureScript** — no cross-platform compilation target

## Go idioms we embrace

- **Explicit error handling** — `if-err` over Clojure's exception model
- **Goroutines + channels** — first-class concurrency forms (`go`, `chan`, `send!`, `recv!`, `select!`)
- **Pointer receivers** — `defmethod` supports `*T` receiver types
- **Multi-return values** — `[value error]` return type annotations
- **Type assertions** — `(as x SomeType)` for Go interface narrowing

## Consequences

- Clojure code cannot be mechanically ported to glisp
- Clojure users will recognize the style and most function names
- Go developers can learn glisp without abandoning Go's concurrency and error-handling philosophy
- Feature requests that are "Clojure does this" are evaluated on their merits for the Go/server-app context, not automatically accepted
