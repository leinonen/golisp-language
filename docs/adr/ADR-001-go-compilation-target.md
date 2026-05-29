# ADR-001: Go as compilation target

**Status**: Accepted

## Context

glisp needs a host language to compile to. The candidates were: Go, JavaScript/Node, Python, WebAssembly, JVM bytecode, or a custom bytecode VM.

## Decision

Compile glisp source to Go source, then use the standard `go build` toolchain.

## Reasons

- **Single static binary** — `go build` produces a self-contained binary with no runtime dependencies; ideal for server deployment
- **First-class concurrency** — goroutines and channels are a natural fit for the concurrent, channel-oriented style glisp exposes
- **Stdlib quality** — Go's standard library covers HTTP, JSON, crypto, and OS primitives without third-party dependencies
- **Ecosystem access** — any Go package is usable via interop forms (`.Method`, `TypeName.`, type annotations); no FFI layer needed
- **Deployment simplicity** — `GOOS=linux go build` cross-compiles; Docker images need only copy the binary
- **Tooling** — `gofmt`, `go test`, `go vet`, and the broader Go tooling work on the generated output

## Consequences

- Generated Go must be valid, `gofmt`-clean, and compilable — adds complexity to the transpiler
- `any`-typed runtime values require explicit casts at Go API boundaries (see ADR-007)
- Go's type system is not expressible in glisp — no generics, no type inference; type annotations are mostly pass-through strings
- Interop is verbose for some Go patterns (e.g., multi-return, pointer receivers) — this is an acceptable tradeoff
