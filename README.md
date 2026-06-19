# golisp

Clojure-style S-expression language that transpiles to Go.

```clojure
(defn greet [name string] -> string
  (str "Hello, " name "!"))

(defn main []
  (fmt/println (greet "World")))
```

`.glsp` files compile to Go source, then to a statically-linked binary via standard
`go build` — no runtime, no external dependencies.

## Install

**Prebuilt binary** (Linux/macOS, amd64/arm64):

```
curl -fsSL https://raw.githubusercontent.com/leinonen/golisp-language/main/install.sh | sh
```

Installs `glisp` and `glisp-lsp` to `/usr/local/bin` (or `~/.local/bin`). Override
with `GLISP_VERSION` / `GLISP_INSTALL_DIR`. Windows users can download the `.zip`
from the [releases page](https://github.com/leinonen/golisp-language/releases).

**From source** (requires the Go toolchain):

```
git clone https://github.com/leinonen/golisp-language && cd golisp-language
make install        # go install ./cmd/glisp ./cmd/glisp-lsp
```

`make dist` cross-compiles release archives into `dist/`. Check your build with
`glisp version`.

## Usage

```
glisp build   file.glsp         # compile to binary
glisp build   dir/              # compile all .glsp in dir to binary
glisp run     file.glsp [args]  # compile and run, no artifacts left behind
glisp fmt     file.glsp         # format in-place
glisp fmt     --check file.glsp # exit 1 if not formatted
glisp compile file.glsp         # write file.go
glisp print   file.glsp         # show generated Go
glisp test    file.glsp         # run deftest cases
glisp doc     [name]            # show built-in docs (all if no name)
```

## Syntax at a glance

```clojure
; Positional types — name then type in params, -> for return type
(defn add [a int b int] -> int (+ a b))
(defn parse [s string] -> [string error] (values s nil))  ; multi-return

; Typed let/loop bindings — annotation goes right after the name
(let [name string "Alice"  xs []int [1 2 3]]
  (str name " has " (len xs) " items"))

; Control flow
(cond (= x 1) "one"  (= x 2) "two"  :else "other")
(loop [i 0] (if (>= i 10) i (recur (+ i 1))))

; Anonymous functions, collections, sets, strings, maps
(map (fn [x] (* x 2)) xs)
(filter even? (range 10))
#{1 2 3}

; Structs, interfaces, methods — with dot-free dispatch on typed values
(defstruct Circle radius float64)
(defmethod Circle Area [c] -> float64 (* math/pi (:radius c) (:radius c)))
(area c)          ; → c.Area()
(:radius c)       ; → c.Radius

; Concurrency
(def result (go-val string (compute-name x)))  ; future → typed channel
(par (init-cache) (connect-db))                ; parallel + WaitGroup
(for-chan [msg ch] (process msg))              ; range until closed
(with-lock mu (fmt/println "safe"))            ; mutex critical section

; State & resources
(def hits (atom int 0))                        ; typed atom
(swap! hits (fn [n] (+ n 1)))                  ; atomic update; (deref hits) → int
(with-open [f (open-file path)] (read f))      ; binds f, defers Close() (LIFO, even on panic)
```

See [`docs/builtins.md`](docs/builtins.md) and [`docs/stdlib.md`](docs/stdlib.md) for the
full reference.

## Web servers

Handlers are `Request → Response` (both aliases for `map[string]any`).

```clojure
(ns main (:import [golisp/web]))

(defn handler [req web/Request] -> web/Response
  (web/json-response 200 {"message" "hello"}))

(defn main []
  (web/serve-graceful ":3000"
    (web/wrap
      (web/routes (web/get "/" handler))
      web/wrap-logging
      web/wrap-cors
      web/wrap-json)))
```

## Documentation

- [`docs/builtins.md`](docs/builtins.md) — all built-in forms and functions
- [`docs/stdlib.md`](docs/stdlib.md) — standard library / package reference
- [`docs/architecture.md`](docs/architecture.md) — how the transpiler works
- [`docs/deployment.md`](docs/deployment.md) — Docker packaging (`scratch` images)
- [`docs/editor-setup.md`](docs/editor-setup.md) — Neovim syntax highlighting + LSP
- [`docs/adr/`](docs/adr/) — architecture decision records
- [`ROADMAP.md`](ROADMAP.md) — planned features

## Examples

```
make examples
./examples/tour/tour            # language tour: fib, strings, goroutines, JSON
glisp run examples/cli/main.glsp 3 1 4   # CLI stats tool — os/args, no build step
./examples/api/api              # REST API with routing, middleware, auth
./examples/inventory/inventory  # gradual struct typing: map literals + (:field x) → typed structs
./examples/movienight/movienight # sets, threading macros, partial/comp/juxt, ctx deadlines
./examples/todos/todos          # server-rendered todos: hiccup + htmx + SSE live stats + websocket chat
```
