# ADR-004: No atoms, refs, or agents

**Status**: Accepted

## Context

Clojure provides four reference types for managing mutable state: `atom` (uncoordinated synchronous), `ref` (coordinated via STM), `agent` (asynchronous), and `var` (thread-local dynamic binding). These exist because the JVM's concurrency model lacks lightweight threads and the channel primitive.

## Decision

glisp does not implement atoms, refs, agents, or software transactional memory. Mutable shared state is handled through Go's native concurrency primitives.

## Reasons

- **Go already solved this** — goroutines are cheap (kilobytes of stack), channels provide safe communication, `sync.Mutex`/`sync.RWMutex` provide coordination; there is no JVM thread-per-goroutine cost that motivated Clojure's reference types
- **Go concurrency is idiomatic in glisp** — `go`, `chan`, `send!`, `recv!`, `select!`, `defer` are first-class forms; they cover every concurrency pattern atoms/agents would address
- **STM doesn't map to Go** — software transactional memory requires a runtime that can retry transactions; implementing it on top of Go would be complex and slower than a mutex
- **Simpler mental model** — one concurrency model (CSP via channels) rather than four reference type philosophies; easier for Go developers to reason about
- **`defstruct` + `sync.Mutex` covers the atom pattern** — when shared mutable state is needed, a struct with a mutex is idiomatic Go and fully expressible in glisp

## Consequences

- `(def counter (atom 0))` / `(swap! counter inc)` patterns don't exist
- Shared mutable state requires explicit struct + mutex, or a dedicated goroutine as a state machine with a request channel
- `dynamic var` binding (for thread-local configuration) is not available; use function parameters or context values
