# Control Flow

## if and when

`if` takes a condition, a then-branch, and an optional else-branch:

```golisp
(if (> x 0) "positive" "non-positive")

; Without else — returns nil when condition is false
(if (> x 0) (println "positive"))
```

`when` is `if` without an else, but accepts multiple body expressions:

```golisp
(when (> x 0)
  (println "x is positive")
  (println "x =" x))
```

## cond

`cond` tests multiple conditions in order, returning the first truthy match:

```golisp
(defn grade [score int] -> string
  (cond
    (>= score 90) "A"
    (>= score 80) "B"
    (>= score 70) "C"
    (>= score 60) "D"
    :else         "F"))
```

`:else` is a common idiom for the default case — it's just a keyword, which is always truthy.

## switch and case

`switch` dispatches on equality, like Go's switch:

```golisp
(defn http-status [code int] -> string
  (switch code
    200 "OK"
    201 "Created"
    404 "Not Found"
    500 "Internal Server Error"
    :default "Unknown"))
```

`case` is the Clojure-style variant where the default is a trailing unpaired expression:

```golisp
(defn day-type [day string] -> string
  (case day
    "Saturday" "weekend"
    "Sunday"   "weekend"
    "weekday"))    ; trailing default
```

Both compile to Go's `switch` statement.

## Conditional Binding

`if-let` evaluates an expression, binds the result, and branches based on truthiness:

```golisp
(defn find-user [id string] -> string
  (if-let [user (lookup-user id)]
    (str "Found: " (:name user))
    "Not found"))
```

The else branch runs when the binding is `nil` or `false`.

`when-let` is the same but with no else — just runs the body when the binding is truthy:

```golisp
(when-let [session (get-session req)]
  (log! "session user:" (:user-id session)))
```

## do

`do` sequences multiple expressions, returning the last value:

```golisp
(if error
  (do
    (log/error "something went wrong" "err" error)
    (sys/exit 1))
  (println "ok"))
```

## Loops

`doseq` iterates a collection for side effects:

```golisp
(doseq [x [1 2 3 4 5]]
  (println x))
```

`dotimes` runs a body `n` times with an index:

```golisp
(dotimes [i 5]
  (println "iteration" i))
```

For loops that produce values or need recursion, see [Functions](ch04-functions.md) — `loop`/`recur`.
