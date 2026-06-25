# Error Handling

GoLisp follows Go's error model: functions return errors as values. There's no try/catch.

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
