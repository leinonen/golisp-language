# ADR-002: Transpiler over interpreter

**Status**: Accepted

## Context

Two main implementation strategies for a new language: (1) transpile to an existing language, (2) implement a bytecode VM or tree-walking interpreter.

## Decision

glisp transpiles to Go source code. There is no runtime, no bytecode, no VM.

## Reasons

- **Full Go ecosystem** — transpiled code is Go code; any Go package, profiler, debugger, or tool works without adaptation
- **No runtime overhead** — no interpreter loop; generated Go compiles to native code with the same performance characteristics as hand-written Go
- **Go tooling just works** — `go test`, `go vet`, `go tool pprof`, race detector, coverage all apply to the generated output
- **Simpler implementation** — no GC, no bytecode spec, no VM to maintain; the transpiler is ~3000 lines of Go
- **Debuggability** — the generated Go is readable and `gofmt`-clean; when something goes wrong you can inspect the output
- **Deployment** — the result is a standard Go binary; no interpreter binary to ship alongside it

## Consequences

- glisp cannot be self-hosting without writing a Go compiler in glisp
- Dynamic features (eval, runtime `load`) are not possible
- The macro system (ADR-005) is constrained: macros must be compile-time transformations expressible without a running Lisp environment
- Generated code readability is a first-class concern — the transpiler must emit clean, idiomatic Go
