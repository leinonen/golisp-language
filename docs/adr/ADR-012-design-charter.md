# ADR-012: Design charter — small, data-first, one obvious way

**Status**: Accepted

## Context

glisp sits between two large languages and could grow without bound by
borrowing from either. Individual ADRs (003–005, 008) have refused specific
features, but the *criteria* for refusal lived in heads, not docs. This ADR
states the identity of the language and the rules that keep it small, so that
future proposals can be evaluated against something written down.

## Identity

glisp occupies one spot no other language does: **write Clojure-shaped code,
ship a single static Go binary.** Three properties define it:

1. **Data-first, types-optional.** Programs start as maps, vectors, and
   `defn`. Gradual typing (`defstruct` + typed map literals + typed keyword
   access, ADR-010 and `emit_typeinfo.go`) lets hot paths harden into real Go
   structs *without rewriting the code*. `any` is always a valid fallback,
   never a dead end.
2. **Go's runtime model, undiluted.** Goroutines, channels, `select!`,
   multi-return errors-as-values (`if-err`, `let-or`), one binary, standard
   `go build`. We do not paper over Go's concurrency or error model with
   foreign abstractions.
3. **Errors as values with Lisp ergonomics.** `if-err` and `let-or` are the
   flagship forms — nicer than Go's `if err != nil` boilerplate and safer
   than Clojure's exceptions. The validation-handler shape
   (`let-or` → `if-err` → response) is the language's signature idiom.

## The charter

Every feature proposal is tested against three rules:

1. **Data first, types when you want them.** A feature may *reward* type
   annotations; it must never *require* them. Anything that makes `any`-typed
   code a second-class citizen is rejected.
2. **One obvious way.** A new form must enable something currently
   impossible or painful — not provide a second spelling of something that
   works. Redundant convenience syntax is rejected even when cheap (precedent:
   `#(...)` was removed in favor of `(fn [x] ...)`). Current ceilings:
   - Conditional binding: `if-let`, `when-let`, `let-or` — **full**. No
     fourth form.
   - Threading: `->`, `->>` — additions need strong evidence from real code.
   - Concurrency sugar above `go`/`chan`/`select!`: `go-val`, `par`,
     `for-chan`, `pipeline`, `fan-out`, `fan-in`, `with-lock` — **full**.
3. **The user never debugs generated Go.** (ADR-011.) Failures surface as
   glisp-level diagnostics at `.glsp` positions, or the construct is absorbed
   by the transpiler. A documented workaround is a defect, not documentation.

### The anti-roadmap

Permanently out of scope (consolidating ADR-003/004/005/008, binding unless
superseded by a future ADR):

- Macros / reader macros
- Lazy sequences
- Atoms-as-STM: refs, agents, vars (the simple `atom` wrapper stays)
- Protocols and multimethods (`definterface` + `defmethod` cover it)
- Namespaced keywords, metadata, numeric tower
- A type-inference engine beyond local, syntactic struct hinting
- Exceptions as a control-flow idiom (`panic`/`recover` exist for Go interop
  and crashes, not for APIs)
- Generics surface syntax
- A second compilation target

"Clojure has it" and "Go has it" are observations, not arguments.

### Size and tooling invariants

- **One-sitting rule**: `examples/tour` must demonstrate every special form
  and stay readable in ~15 minutes. If a feature can't fit in the tour, the
  language has outgrown its goal.
- **Tooling parity**: a feature does not ship until `glisp fmt` round-trips
  it and the LSP knows it (hover/completion entry in
  `internal/lsp/builtins.go`). This is already practiced for `web`; it is now
  the rule for everything.
- **Feedback speed is a feature**: the toolchain owes the user a fast
  edit-run loop (`repl` today; a `glisp run` one-shot command is the accepted
  next step).

## Consequences

- Feature discussions get shorter: most proposals die on rule 2 or the
  anti-roadmap, by reference rather than re-litigation.
- The README should lead with the identity (a real JSON API as a single
  static binary), not a feature list.
- Removing redundancy can be valid work even with no feature attached.
- This ADR is the tiebreaker: when a change makes glisp bigger but not more
  itself, it is declined.
