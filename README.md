# glisp

Clojure-style S-expression language that transpiles to Go. Write Ring-style web servers without the JVM.

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

; Control flow
(if cond then else)
(cond (= x 1) "one"  (= x 2) "two"  :else "other")
(let [x 1  y (+ x 1)] (* x y))
(loop [i 0] (if (>= i 10) i (recur (+ i 1))))

; Collections: map, filter, reduce, range, take, drop, sort-by, flatten
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

Handlers are `map[string]any → map[string]any`.

```clojure
(ns main (:import [fmt golisp/stdlib]))

(defn ^map[string]any handler [^map[string]any req]
  (stdlib/JsonResponse 200 {"message" "hello"}))

(defn main []
  (fmt/Println "Listening on :3000")
  (stdlib/ServeGraceful ":3000"
    (stdlib/Wrap
      (stdlib/Routes
        (stdlib/GET "/" handler))
      stdlib/WrapLogging
      stdlib/WrapCors
      stdlib/WrapJson)))
```

## Editor support

### Neovim — syntax highlighting

Add the bundled plugin to your runtimepath in `init.lua`:

```lua
vim.opt.rtp:prepend("/path/to/go-lisp-2/editors/neovim")
```

Or with lazy.nvim:

```lua
{ dir = "/path/to/go-lisp-2/editors/neovim" }
```

This gives you filetype detection, `commentstring`, `iskeyword` tuning, and syntax
highlighting that inherits from Clojure (parens, strings, keywords, comments, core
special forms) plus glisp-specific rules (type annotations, `defstruct`, `if-err`,
`send!`, etc.).

### Neovim — LSP (diagnostics + hover)

## LSP (Neovim 0.12+)

`glisp-lsp` is a Language Server that provides diagnostics (parse errors highlighted inline) and hover (show `defn`/`def` signatures).

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

Diagnostics appear automatically as you edit. Hover with `K` (default Neovim mapping) over any `defn` or `def` name to see its signature.

## Examples

```
make examples
./examples/tour/tour   # language tour: fib, strings, goroutines, JSON
./examples/api/api     # REST API with routing, middleware, auth
```
