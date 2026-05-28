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

### Collections

```clojure
(map    (fn [x] (* (int x) 2)) [1 2 3])          ; → [2 4 6]
(filter (fn [x] (> (int x) 2)) [1 2 3 4])        ; → [3 4]
(reduce (fn [acc x] (+ (int acc) (int x))) 0 [1 2 3]) ; → 6
(range 5)                                          ; → [0 1 2 3 4]
(range 2 7)                                        ; → [2 3 4 5 6]
(take 3 [1 2 3 4])                                 ; → [1 2 3]
(drop 2 [1 2 3 4])                                 ; → [3 4]
(reverse [1 2 3])                                  ; → [3 2 1]
(flatten [[1 2] [3 [4 5]]])                        ; → [1 2 3 4 5]
(sort-by (fn [x] x) ["b" "a" "c"])                ; → ["a" "b" "c"]
(some    (fn [x] (= (int x) 2)) [1 2 3])          ; → 2
(every?  (fn [x] (> (int x) 0)) [1 2 3])          ; → true
(contains? [1 2 3] 2)   ; slice membership
(contains? {"a" 1} "a") ; map key presence
```

Map operations:

```clojure
(get    m "key")              ; lookup (returns nil if missing)
(assoc  m "key" val)          ; returns new map with key set
(dissoc m "key")              ; returns new map with key removed
(merge  m1 m2)                ; merge maps (m2 wins on conflict)
(keys   m)                    ; → ["k1" "k2" ...]
(vals   m)                    ; → [v1 v2 ...]
```

### Strings

```clojure
(upper-case "hello")          ; → "HELLO"
(lower-case "HELLO")          ; → "hello"
(trim "  hi  ")               ; → "hi"
(split "a,b,c" ",")           ; → ["a" "b" "c"]
(join ["a" "b" "c"] "-")      ; → "a-b-c"
(starts-with? "hello" "he")   ; → true
(ends-with?   "hello" "lo")   ; → true
(contains?    "hello" "ell")  ; → true
(replace "hello" "l" "r")     ; → "herro"
(subs "hello" 1 3)            ; → "el"
(str "a" "b" "c")             ; → "abc"  (variadic concat, accepts any type)
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

### JSON

```clojure
(if-err [encoded err] (json/encode {"name" "alice" "score" 42})
  (println "encode failed")
  (println encoded))           ; → {"name":"alice","score":42}

(if-err [decoded err] (json/decode "{\"ok\":true}")
  (println "decode failed")
  (println (get decoded "ok"))) ; → true
```

`json/encode` returns `[string error]`. `json/decode` returns `[any error]` — handles both JSON objects and arrays. Use with `if-err`.

### Web servers (Ring style)

Handlers are plain functions: `map[string]any → map[string]any`.

```clojure
(ns main
  (:import [fmt golisp/stdlib]))

(defn ^map[string]any handler [^map[string]any req]
  (stdlib/JsonResponse 200 {"message" "hello"}))

(defn main []
  (fmt/Println "Listening on :3000")
  (stdlib/ServeGraceful ":3000" handler))
```

Request map keys: `"method"` `"path"` `"query"` `"headers"` `"body"`.

`stdlib/JsonResponse` sets `Content-Type: application/json` and JSON-encodes the body map.

#### Routing

```clojure
(def app
  (stdlib/Routes
    (stdlib/GET "/"         home-handler)
    (stdlib/POST "/items"   create-handler)
    (stdlib/GET "/items/:id" item-handler)))   ; path params in req["params"]
```

#### Middleware

```clojure
; Wrap applies middlewares outermost-first to a handler
(def app (stdlib/Wrap router
           stdlib/WrapLogging   ; logs method + path + status
           stdlib/WrapRecover   ; catches panics → 500
           stdlib/WrapCors      ; adds Access-Control-Allow-Origin: *
           stdlib/WrapJson      ; parses JSON body → req["json-body"]
           (stdlib/WrapTimeout 30)))  ; 503 after 30 s

; WrapAuth requires Bearer token; stores it in req["identity"]
(def secured (stdlib/Wrap handler stdlib/WrapAuth))

; Compose builds a reusable Middleware value; apply it like a function
(def api-mw (stdlib/Compose stdlib/WrapLogging stdlib/WrapCors stdlib/WrapJson))
(def app (api-mw router))
```

#### Request helpers

```clojure
(stdlib/QueryParam req "page")          ; parses req["query"]
(stdlib/PathParam  req "id")            ; reads req["params"]
(stdlib/BodyMap    req)                 ; JSON body as map (works with or without WrapJson)
(stdlib/Header     req "Authorization") ; reads req["headers"]
```

#### Static file serving

`ServeFiles` returns a `Handler` — use it directly or as a route handler. The router does exact-segment matching, so serve a static directory by itself or alongside API routes that don't overlap in path depth.

```clojure
; Serve a directory on its own
(stdlib/ServeGraceful ":3000" (stdlib/ServeFiles "/" "public/"))

; Mix with API routes (static files under /static/<name>)
(def app
  (stdlib/Routes
    (stdlib/GET "/api/tasks"    tasks-handler)
    (stdlib/GET "/static/:file" (stdlib/ServeFiles "/static/" "public/"))))
```

#### Graceful shutdown

```clojure
; Blocks until SIGINT/SIGTERM, then drains in-flight requests (5 s deadline)
(stdlib/ServeGraceful ":3000" app)
```

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
```

### `examples/tour` — language tour

Fibonacci, string processing, concurrent grade analysis with goroutines, and JSON export. Demonstrates core language features end-to-end.

```
./examples/tour/tour
```

### `examples/api` — REST API

Tasks CRUD server with a full middleware stack (logging, CORS, JSON body parsing, panic recovery) and a Bearer-token-protected export endpoint. Uses `QueryParam`, `PathParam`, `BodyMap`, and `ServeGraceful`.

```
./examples/api/api
# then in another terminal:
curl http://localhost:4000/tasks
curl "http://localhost:4000/tasks?done=false"
curl http://localhost:4000/tasks/2
curl -X POST http://localhost:4000/tasks -H 'Content-Type: application/json' -d '{"title":"New task"}'
curl http://localhost:4000/export -H 'Authorization: Bearer mytoken'
```
