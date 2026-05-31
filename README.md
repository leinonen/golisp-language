# golisp (glisp)

Clojure-style S-expression language that transpiles to Go.

```clojure
(defn ^string greet [^string name]
  (str "Hello, " name "!"))

(defn main []
  (println (greet "World")))
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

; Collections: map, filter, reduce, range, take, drop, sort-by, flatten
; Sets:        #{1 2 3}, conj, contains?, union, intersection, difference
; Strings:     str, upper-case, lower-case, trim, split, join, replace
; Maps:        get, assoc, dissoc, merge, keys, vals, contains?

; Go interop
(go (println "async"))
(defer (println "cleanup"))
(let [ch (chan ^int 1)] (send! ch 42) (recv! ch))
(.Write w data)   ; method call
(.-Field obj)     ; field access
```

## Web servers

Handlers are `Request → Response` (both aliases for `map[string]any`).

```clojure
(ns main (:import [fmt golisp/web]))

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

## Examples

```
make examples
./examples/tour/tour   # language tour: fib, strings, goroutines, JSON
./examples/api/api     # REST API with routing, middleware, auth
```
