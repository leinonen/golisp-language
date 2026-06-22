# ADR-005: Macro system deferred

**Status**: Superseded by ADR-017 (compile-time macros accepted). Preserved for
history; the reasoning below was sound for v1 and ADR-017 documents what changed.

## Context

Clojure's macro system is one of its most powerful features — macros are compile-time functions that transform code before evaluation. They enable DSLs, custom control flow, and zero-cost abstractions. They are also one of Clojure's most complex and misused features.

## Decision

No macro system in glisp for now. The decision is deferred, not rejected.

## Reasons

- **Transpiler model makes macros expensive** — macros require a full Lisp evaluator pass before transpilation. This means glisp would need an embedded interpreter, adding ~5x the current implementation complexity
- **Built-in special forms cover 95% of use cases** — threading macros (`->`, `->>`), `when`, `when-let`, `if-let`, `doto`, `with-open` cover the patterns most commonly addressed with macros in Clojure; these are added as built-in forms
- **Server app focus** — the primary use case is writing API servers and data pipelines, not authoring DSLs or language extensions; macro power is less valuable here
- **Macro hygiene is hard** — unhygienic macros cause subtle bugs; hygienic macros require `gensym` and careful scoping that is complex to implement correctly
- **Alternative: add built-in forms** — when a macro would be used, we consider adding it as a built-in special form if it's generally useful

## When to revisit

Reconsider if:
- A clear use case emerges that cannot be addressed by built-in forms (e.g., a SQL DSL or test framework)
- The language is stable enough that the implementation complexity is justified
- The implementation approach is clarified (e.g., a two-phase compile with a Lisp evaluator as phase 1)

## Consequences

- Users cannot extend the language's syntax
- DSL authoring (e.g., routing tables, HTML generation) must use function composition instead
- Some Clojure patterns (`clojure.test`, `core.async` macros) cannot be ported directly
