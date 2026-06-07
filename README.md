# golisp (glisp)

Clojure-style S-expression language that transpiles to Go.

```clojure
(defn ^string greet [^string name]
  (str "Hello, " name "!"))

(defn main []
  (fmt/println (greet "World")))
```

## Install

```
make install
```

## Usage

```
glisp build   file.glsp         # compile to binary
glisp build   dir/              # compile all .glsp in dir to binary
glisp fmt     file.glsp         # format in-place
glisp fmt     --check file.glsp # exit 1 if not formatted
glisp compile file.glsp         # write file.go
glisp print   file.glsp         # show generated Go
glisp test    file.glsp         # run deftest cases
glisp doc     [name]            # show built-in docs (all if no name)
```

## Syntax highlights

```clojure
; Type annotations
(defn ^int add [^int a ^int b] (+ a b))
(defn ^[string error] parse [^string s] (values s nil))  ; multi-return
(defn ^web/Response handler [^web/Request req] ...)  ; package-qualified

; Control flow
(if cond then else)
(cond (= x 1) "one"  (= x 2) "two"  :else "other")
(let [x 1  y (+ x 1)] (* x y))
(loop [i 0] (if (>= i 10) i (recur (+ i 1))))

; Anonymous function shorthand
#(+ % 1)           ; (fn [x] (+ x 1))
#(+ %1 %2)         ; (fn [x y] (+ x y))
(map #(* % 2) xs)  ; common with map/filter/reduce

; Collections: map, filter, reduce, range, take, drop, sort-by, flatten
; Sets:        #{1 2 3}, conj, contains?, union, intersection, difference
; Strings:     str, upper-case, lower-case, trim, split, join, replace
; Maps:        get, assoc, dissoc, merge, keys, vals, contains?

; Go interop
(go (fmt/println "async"))
(defer (fmt/println "cleanup"))
(let [ch (chan ^int 1)] (send! ch 42) (recv! ch))
(.Write w data)   ; method call
(.-Field obj)     ; field access

; Concurrency
(def result (go-val (expensive-computation x)))  ; future → chan any
(recv! result)                                    ; block for result

(par                      ; parallel + WaitGroup
  (init-cache)
  (connect-db))

(for-chan [msg ch]        ; range until closed
  (process msg))

(with-lock mu             ; mutex critical section
  (fmt/println "safe"))

(select!                  ; select with timeout
  ([msg ch] (handle msg))
  (:timeout 1000 (fmt/println "timed out")))
```

## Web servers

Handlers are `Request → Response` (both aliases for `map[string]any`).

```clojure
(ns main (:import [golisp/web]))

(defn ^web/Response handler [^web/Request req]
  (web/json-response 200 {"message" "hello"}))

(defn main []
  (fmt/println "Listening on :3000")
  (web/serve-graceful ":3000"
    (web/wrap
      (web/routes
        (web/get "/" handler))
      web/wrap-logging
      web/wrap-cors
      web/wrap-json)))
```

## Docker packaging

`glisp build` produces a statically-linked binary with no external dependencies, so it runs in a `scratch` image.

```dockerfile
# Build stage
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git && \
    go install github.com/leinonen/golisp-language/cmd/glisp@latest

WORKDIR /app
COPY . .

# Produces a statically-linked binary
RUN CGO_ENABLED=0 glisp build src/

# Runtime stage — zero OS overhead
FROM scratch
COPY --from=builder /app/src /app
ENTRYPOINT ["/app/src"]
```

Build and run:

```
docker build -t myapp .
docker run -p 3000:3000 myapp
```

The final image contains only your binary. Typical size: 8–15 MB.

### Multi-file projects

For a directory build (`glisp build dir/`) the output binary name matches the directory name:

```dockerfile
RUN CGO_ENABLED=0 glisp build api/
COPY --from=builder /app/api /app
```

### Health checks

`scratch` has no shell, so use the `HEALTHCHECK` exec form with your app's own endpoint:

```dockerfile
HEALTHCHECK --interval=10s --timeout=3s \
  CMD ["/app/src", "--healthz"]   # implement a /healthz flag in main
```

Or use a sidecar/external probe and skip the `HEALTHCHECK` entirely.

## Editor support

### Neovim — syntax highlighting

Add the bundled plugin to your runtimepath in `init.lua`:

```lua
vim.opt.rtp:prepend("/path/to/golisp-language/editors/neovim")
```

Or with lazy.nvim:

```lua
{ dir = "/path/to/golisp-language/editors/neovim" }
```

This gives you filetype detection, `commentstring`, `iskeyword` tuning, and syntax
highlighting that inherits from Clojure (parens, strings, keywords, comments, core
special forms) plus glisp-specific rules (type annotations, `defstruct`, `if-err`,
`send!`, etc.).

### Neovim — LSP (diagnostics + hover)

## LSP (Neovim 0.12+)

`glisp-lsp` is a Language Server that provides diagnostics (parse errors highlighted inline), hover (show `defn`/`def` signatures and web package type definitions like `web/Request`), jump-to-definition, and completions.

### Install

```
go install ./cmd/glisp-lsp
# or: make install
```

### Neovim setup

```lua
-- ~/.config/nvim/after/ftplugin/glsp.lua  (or in your init.lua)

-- filetype detection
vim.filetype.add({ extension = { glsp = "glsp" } })

-- register and enable the server
vim.lsp.config["glisp"] = {
  cmd          = { "glisp-lsp" },
  filetypes    = { "glsp" },
  root_markers = { "go.mod", ".git" },
}
vim.lsp.enable("glisp")
```

Diagnostics appear automatically as you edit. Hover with `K` (default Neovim mapping) over any `defn` or `def` name to see its signature. Jump to definition with `gd`. Completions trigger automatically as you type.

## Transpiler architecture

```
source.glsp → lexer → parser → transpiler → Go source → gofmt → go build → binary
```

The transpiler lives in `internal/transpiler/` and is split across several files by concern:

| File | Role |
|---|---|
| `transpiler.go` | `Emitter` struct, two-pass `emitFile`, import resolution |
| `emit_decl.go` | Top-level declarations: `def`, `defn`, `defstruct`, `definterface`, `defmethod` |
| `emit_expr.go` | Expressions: `fn`, `let`, `if`, `cond`, `do`, `loop`/`recur`, built-ins |
| `emit_concurrency.go` | Concurrency forms: `go`, `chan`, `send!`, `recv!`, `select!`, `par`, `with-lock`, … |
| `emit_types.go` | Type annotation conversion: `identToGo`, `typeExprToGo`, `qualifiedTypeToGo` |
| `emit_runtime.go` | Inline Go runtime helpers appended to generated files |

### Two-pass emission

`emitFile` uses two passes to solve a chicken-and-egg problem: Go requires the `import` block at the top of the file, but which packages are needed is only known after emitting all declarations.

1. **Pass 1** — emit all declarations into a scratch `Emitter`. This discovers which packages are needed by setting flags in `builtinImports` and `directImports`.
2. **Pass 2** — write `package …` and `import (…)` into the real buffer using the discovered sets, then append the pass-1 output.

### Import tracking

The emitter maintains two import sets:

- **`builtinImports`** — set by `needImport("pkg")` inside built-in form handlers (e.g. `needImport("sort")` when `sort-by` is used). Used to gate which runtime helper blocks are appended.
- **`directImports`** — set when the user writes a qualified symbol directly (`fmt/println` → adds `"fmt"`). Always emitted unconditionally.

Stdlib packages never require a user `(:import […])` declaration — the emitter adds them automatically. `(:import […])` in an `ns` form is only needed for external Go module packages (`golisp/web`, `github.com/jackc/pgx/v5`, etc.).

### Statement vs expression position

Most forms (`let`, `if`, `cond`, `do`, `when`) can appear in both positions:

- **Statement position** — emitted as plain Go blocks via `emitStmtNode`.
- **Expression position** — wrapped in an immediately-invoked function literal `func() any { … }()` so the form produces a value.

The emitter tracks position through the call stack: top-level body statements go through `emitStmtNode`; anything used as an argument or binding RHS goes through `emitExpr`.

### Runtime helpers

Every generated file ends with inline Go helper functions (`_glispGet`, `_glispAssoc`, `_glispConj`, `_glispReduce`, etc.) that implement glisp's dynamic collection semantics. Conditional blocks (`glispSortRuntime`, `glispStrRuntime`, etc.) are appended only when the corresponding built-ins are actually used, keeping output lean.

For directory builds (`glisp build dir/`) the helpers are emitted once into a shared `glisp_runtime.go` file instead of duplicated in every file. Individual files are transpiled with `TranspileNoRuntime` which omits the helpers.

## Documentation

- [`docs/builtins.md`](docs/builtins.md) — all built-in forms and functions
- [`docs/stdlib.md`](docs/stdlib.md) — standard library reference
- [`docs/adr/`](docs/adr/) — architecture decision records

## Examples

```
make examples
./examples/tour/tour   # language tour: fib, strings, goroutines, JSON
./examples/api/api     # REST API with routing, middleware, auth
```
