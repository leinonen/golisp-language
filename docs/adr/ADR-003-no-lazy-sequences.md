# ADR-003: No lazy sequences

**Status**: Accepted

## Context

Clojure's sequence model is lazy by default. `(map f (range))` works because the infinite sequence is never fully realized. Lazy-seq is fundamental to Clojure's collection API: `iterate`, `cycle`, `repeat` (infinite), and `take`/`drop` on infinite sources all depend on it.

## Decision

glisp has no lazy sequences. All collection operations are eager and operate on concrete Go slices.

## Reasons

- **Go has no native lazy abstraction** — implementing laziness requires closures + thunks, which generates ugly, slow Go
- **Channels cover the real use case** — streaming/infinite data in Go is idiomatically handled with channels and goroutines; glisp exposes these directly via `chan`, `go`, `send!`, `recv!`
- **Simplicity** — eager operations on slices are predictable, debuggable, and map 1:1 to Go semantics; lazy evaluation introduces ordering surprises and memory behavior that doesn't match what Go developers expect
- **Server apps don't need infinite sequences** — the primary use case is transforming finite data: HTTP payloads, database rows, config maps

## Alternatives considered

- **Channels as lazy seqs** — possible but requires explicit goroutine management; glisp exposes channels directly rather than hiding them behind a lazy abstraction
- **Optional lazy wrappers** — would add complexity without clear benefit for the target use case

## Consequences

- `(repeat n val)` is `(range n)` mapped to val — always finite
- Infinite iteration uses `loop`/`recur` or goroutine+channel patterns
- `range` always requires bounds; there is no `(range)` returning an infinite counter
