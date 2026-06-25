# Foreword

GoLisp is a compiled language with Lisp syntax and Go semantics.

You write S-expressions. The compiler produces Go. No interpreter, no VM, no runtime overhead — just your code, translated to idiomatic Go and built with `go build`.

The bet: Lisp's uniform syntax and functional core pair surprisingly well with Go's type system, concurrency model, and ecosystem. Parentheses feel less like noise when they're also your function call, your data literal, and your macro invocation.

GoLisp is not Clojure with a Go backend. It doesn't pretend to be. Immutable persistent data structures are out; goroutines and channels are in. The error model is Go's — multiple return values, checked explicitly. Structs have methods. Interfaces define contracts. The type system is Go's type system.

What GoLisp adds is expressiveness: closures as first-class values, destructuring in function parameters, a rich collection library, and a syntax that makes higher-order functions feel natural.

It is a general-purpose Lisp hosted on Go, not Go in parentheses. It has its own core vocabulary — the `str/`, `math/`, `sys/`, and `cli/` namespaces — and it grows through macros written in the library rather than changes to the compiler. It is as comfortable as a command-line script or a data-processing pipeline as it is running a web service. Go is the host platform, reached deliberately through first-class interop.

This book is a tour. Each chapter teaches one concept through working code. Read it start to finish, or jump to what you need.

All examples can be run with:

```
glisp run <file>.glsp
```
