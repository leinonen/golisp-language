# Error Handling

GoLisp follows Go's error model: functions return errors as values. That's the idiomatic path for expected failures. For exceptional, panic-style control flow, GoLisp also offers Clojure-style `try`/`catch`/`finally` and `throw`, which lower to Go's `defer`/`recover`.

## Multiple Return Values

A function that can fail returns `[result error]`:

```golisp
(defn divide [a float64 b float64] -> [float64 error]
  (if (= b 0.0)
    (values 0.0 (error "division by zero"))
    (values (/ a b) nil)))
```

## if-err

`if-err` is the idiomatic way to check errors. It binds both return values and branches on whether the error is non-nil:

```golisp
(if-err [result err] (divide 10.0 3.0)
  (println "failed:" err)       ; error branch
  (println "result:" result))   ; success branch
```

The error branch runs first — this mirrors the Go convention of handling errors before using values.

Nested operations read naturally:

```golisp
(defn read-json [path string] -> [any error]
  (if-err [content err] (read-file path)
    (values nil err)
    (if-err [data err] (json/decode content)
      (values nil err)
      (values data nil))))
```

## Creating Errors

`error` creates an error from a string:

```golisp
(error "something went wrong")
```

`wrap-error` wraps an existing error with additional context:

```golisp
(wrap-error "failed to load config" err)
```

Check if an error matches a specific type:

```golisp
(errors/is? err io/EOF)
```

## let-or — Sequential Nil Guard

`let-or` chains operations where each step can fail. If any binding is nil, it short-circuits to the fallback:

```golisp
(defn process-request [req any] -> any
  (let-or
    [body  (web/body-map req)        {"error" "missing body"}
     title (get body "title")        {"error" "missing title"}
     user  (get body "user")         {"error" "missing user"}]
    {:title title :user user}))
```

Each binding has three parts: `name`, `expression`, `fallback`. The fallback is returned as soon as a binding resolves to nil.

## Panic and Recover

For truly unrecoverable situations, `panic` stops the program:

```golisp
(assert (> n 0) "n must be positive")
(panic "unreachable state")
```

`recover` can catch a panic inside a deferred function:

```golisp
(defn safe-call [f any] -> any
  (defer (fn [] -> void
    (let [r (recover)]
      (when r (println "caught panic:" r)))))
  (f))
```

Panics are rare. Most failures should return an error value.

## try / catch / finally

For code that raises panics — either your own `throw`/`panic` or a runtime fault like an out-of-range index — `try` catches them without hand-writing a `defer`/`recover` block:

```golisp
(defn safe-div [a int b int] -> any
  (try
    (if (= b 0)
      (throw "division by zero")
      (/ a b))
    (catch e
      (do (println "caught:" e) -1))))
```

The body runs first. If it panics, the panic value is bound to the `catch` symbol (`e`) and the handler runs. `try` returns the body's value normally, or the handler's value when a panic is caught — so it works as an expression:

```golisp
(def status (try (risky-call) (catch e "failed")))
```

`throw` raises any value — an error, a string, or a map you can inspect in the handler:

```golisp
(try
  (throw {:type "http" :status 500 :msg "boom"})
  (catch e
    (println "failed with" (get e "status"))))
```

A `finally` clause runs unconditionally after the body and any handler — even when the panic is *not* caught (it re-propagates afterward). Use it for cleanup:

```golisp
(defn process [path string] -> any
  (try
    (do-work path)
    (catch e
      (println "error:" e))
    (finally
      (println "processing finished"))))
```

Both `catch` and `finally` are optional, but a `try` needs at least one of them. Use `_` as the binding to ignore the caught value:

```golisp
(try (parse input) (catch _ nil))
```

Because GoLisp compiles to Go, `catch` maps to Go's single `recover` mechanism: it catches *any* panic (there is no exception-type filtering). Reach for `try`/`catch` at boundaries where you want to contain a panic; prefer `[value error]` returns and `if-err` for ordinary, expected failures.

## Practical Pattern

A typical function that reads, parses, and uses data:

```golisp
(defn load-config [path string] -> [any error]
  (if-err [raw err] (read-file path)
    (values nil (wrap-error "read failed" err))
    (if-err [cfg err] (json/decode raw)
      (values nil (wrap-error "parse failed" err))
      (values cfg nil))))

(defn main [] -> void
  (if-err [cfg err] (load-config "config.json")
    (do (println "error:" err) (sys/exit 1))
    (println "loaded:" cfg)))
```
