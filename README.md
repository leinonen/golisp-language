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
glisp print  file.glsp        # show generated Go
glisp compile file.glsp       # write file.go
glisp build  file.glsp        # compile to binary
glisp build -o myapp file.glsp
```

## Syntax

### Functions and variables

```clojure
(def ^int port 3000)

(defn ^string greet [^string name]
  (str "Hello, " name "!"))

(defn ^[string error] parse [^string s]   ; multi-return
  (values s nil))
```

### Type annotations

`^T` before a name declares its Go type. Required on function signatures.

| glisp | Go |
|---|---|
| `^int` `^string` `^bool` | primitive types |
| `^*http.Request` `^[]string` | pointer / slice |
| `^map[string]int` | map |
| `^(chan int)` | channel |
| `^[string error]` | `(string, error)` multi-return |

### Control flow

```clojure
(if condition then-expr else-expr)

(cond
  (= x 1) "one"
  (= x 2) "two"
  :else    "other")

(when condition
  (side-effect)
  result)

(let [x 1
      y (+ x 1)]
  (* x y))
```

### Tail-recursive loops

```clojure
(loop [i 0 acc []]
  (if (>= i 10)
    acc
    (recur (+ i 1) (conj acc i))))
```

### Go concurrency

```clojure
(go (println "async"))

(let [ch (chan ^int 1)]
  (send! ch 42)
  (recv! ch))

(defer (println "cleanup"))
```

### Error handling

```clojure
(defn ^[string error] read-file [^string path]
  (if-err [data err] (os/ReadFile path)
    (values "" err)
    (values (string data) nil)))
```

`if-err` destructures a `(value, error)` return: runs the error branch when `err != nil`, otherwise the ok branch.

### Methods and fields

```clojure
(.Write w data)       ; w.Write(data)
(.-Method r)          ; r.Method  (field access)
```

### Web servers (Ring style)

Handlers are plain functions: `map[string]any → map[string]any`.

```clojure
(ns main
  (:import [fmt golisp/stdlib]))

(defn ^map[string]any handler [^map[string]any req]
  {:status 200
   :headers {"Content-Type" "text/plain"}
   :body "hello"})

(defn main []
  (fmt/Println "Listening on :3000")
  (let [err (stdlib/Serve ":3000" handler)]
    (log/Fatal err)))
```

Request map keys: `"method"` `"path"` `"query"` `"headers"` `"body"`.

### Identifiers

| glisp | Go |
|---|---|
| `my-func` | `myFunc` |
| `ring->handler` | `ringToHandler` |
| `nil?` | `isNil` |
| `send!` | `send` |
| `fmt/Println` | `fmt.Println` |

## Examples

```
make examples
./examples/hello/hello
./examples/webserver/webserver
```
