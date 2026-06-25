# Getting Started

## Installation

The quickest way is the install script, which fetches a prebuilt binary:

```bash
curl -fsSL https://raw.githubusercontent.com/leinonen/golisp-language/main/install.sh | sh
```

Or build from source — GoLisp requires Go 1.25.5 or later:

```bash
go install github.com/leinonen/golisp-language/cmd/glisp@latest
```

Verify it works:

```bash
glisp version
```

## Hello, World

Create a file named `hello.glsp`:

```golisp
(ns main)

(defn main [] -> void
  (println "Hello, World!"))
```

Run it:

```bash
glisp run hello.glsp
```

Build a binary:

```bash
glisp build hello.glsp
./hello
```

## The Compiler Commands

| Command | What it does |
|---------|-------------|
| `glisp run file.glsp` | Compile and run, no output files |
| `glisp run --watch file.glsp` | Re-run on every save |
| `glisp file.glsp` | Run directly (also works via a `#!/usr/bin/env glisp` shebang) |
| `glisp build file.glsp` | Compile to a binary |
| `glisp build dir/` | Compile all `.glsp` files in a directory |
| `glisp fmt file.glsp` | Format in-place |
| `glisp compile file.glsp` | Emit the generated `.go` file |
| `glisp print file.glsp` | Print generated Go to stdout |
| `glisp macroexpand file.glsp` | Print the file with macros expanded |
| `glisp test file.glsp` | Run test blocks |
| `glisp doc [name]` | Show documentation for a built-in |

## Project Layout

Single-file programs need no setup — just a `.glsp` file with `(ns main)` and a `main` function.

For multi-file projects, create a directory:

```
myapp/
  main.glsp
  handlers.glsp
  models.glsp
```

Build the whole directory:

```bash
glisp build myapp/
```

For projects with external dependencies, add a `glisp.mod` file (see [Modules](ch17-modules.md)).
