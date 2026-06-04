# Built-in functions

Core functions available in every glisp program without any import.

## Arithmetic

| Form | Returns | Description |
|---|---|---|
| `(+ a b ...)` | number | Addition |
| `(- a b ...)` | number | Subtraction |
| `(* a b ...)` | number | Multiplication |
| `(/ a b)` | number | Division |
| `(mod a b)` | number | Remainder |

## Comparison

| Form | Returns | Description |
|---|---|---|
| `(= a b)` | bool | Equal |
| `(not= a b)` | bool | Not equal |
| `(< a b)` | bool | Less than |
| `(> a b)` | bool | Greater than |
| `(<= a b)` | bool | Less than or equal |
| `(>= a b)` | bool | Greater than or equal |

## Logic

| Form | Returns | Description |
|---|---|---|
| `(and a b ...)` | bool | True when all arguments are truthy |
| `(or a b ...)` | bool | True when at least one argument is truthy |
| `(not a)` | bool | Logical negation |

## Collections

| Form | Returns | Description |
|---|---|---|
| `(map f coll)` | `[]any` | Apply f to each element |
| `(filter pred coll)` | `[]any` | Elements where pred is truthy |
| `(reduce f init coll)` | any | Fold coll to a single value |
| `(range n)` | `[]int` | Integers 0..n-1 |
| `(range start end)` | `[]int` | Integers start..end-1 |
| `(take n coll)` | `[]any` | First n elements |
| `(drop n coll)` | `[]any` | All elements after the first n |
| `(reverse coll)` | `[]any` | Reversed copy |
| `(sort-by f coll)` | `[]any` | Sort by the value f returns for each element |
| `(flatten coll)` | `[]any` | Recursively flatten nested slices |
| `(repeat n val)` | `[]any` | Slice of n copies of val |
| `(interpose sep coll)` | `[]any` | Insert sep between each element |
| `(conj coll x)` | coll | Append x to a slice, or add x to a set |
| `(count coll)` | int | Number of elements (works for slices, maps, sets, strings) |
| `(first coll)` | any | First element |
| `(rest coll)` | `[]any` | All elements after the first |
| `(nth coll i)` | any | Element at index i |
| `(contains? coll x)` | bool | True when coll contains x (map, slice, set, string) |
| `(some pred coll)` | any | First element where pred is truthy, or nil |
| `(every? pred coll)` | bool | True when pred is truthy for every element |

## Maps

| Form | Returns | Description |
|---|---|---|
| `(get m k)` | any | Look up key k; nil if absent |
| `(assoc m k v)` | map | New map with k set to v |
| `(dissoc m k)` | map | New map with k removed |
| `(merge m1 m2)` | map | Merge two maps; m2 keys overwrite m1 |
| `(keys m)` | `[]any` | All keys |
| `(vals m)` | `[]any` | All values |

## Sets

Sets are unordered collections of unique values. Literal syntax: `#{1 2 3}`. Backed by `map[any]struct{}` in Go.

| Form | Returns | Description |
|---|---|---|
| `#{x y z}` | set | Set literal |
| `(conj s x)` | set | New set with x added |
| `(contains? s x)` | bool | O(1) membership test |
| `(count s)` | int | Number of elements |
| `(empty? s)` | bool | True when s has no elements |
| `(union s1 s2)` | set | Elements in s1 or s2 |
| `(intersection s1 s2)` | set | Elements in both s1 and s2 |
| `(difference s1 s2)` | set | Elements in s1 that are not in s2 |

```clojure
(let [a #{1 2 3}
      b #{2 3 4}]
  (contains? a 2)        ; true
  (conj a 4)             ; #{1 2 3 4}
  (union a b)            ; #{1 2 3 4}
  (intersection a b)     ; #{2 3}
  (difference a b))      ; #{1}
```

## Strings

| Form | Returns | Description |
|---|---|---|
| `(str & args)` | string | Concatenate all args as strings |
| `(string x)` | string | Convert x to its string representation |
| `(upper-case s)` | string | Uppercase |
| `(lower-case s)` | string | Lowercase |
| `(trim s)` | string | Strip leading/trailing whitespace |
| `(starts-with? s prefix)` | bool | True when s begins with prefix |
| `(ends-with? s suffix)` | bool | True when s ends with suffix |
| `(replace s old new)` | string | Replace all occurrences of old with new |
| `(split s sep)` | `[]string` | Split on sep |
| `(join sep coll)` | string | Join elements with sep |
| `(subs s start)` | string | Substring from start to end |
| `(subs s start end)` | string | Substring from start to end (exclusive) |
| `(format fmt & args)` | string | Printf-style formatting (`%s`, `%d`, `%v`, …) |

## Numbers

| Form | Returns | Description |
|---|---|---|
| `(int x)` | int | Convert x to int |
| `(parse-int s)` | `[int error]` | Parse decimal integer string; use with `if-err` |
| `(parse-float s)` | `[float64 error]` | Parse float string; use with `if-err` |

## Higher-order functions

| Form | Returns | Description |
|---|---|---|
| `(comp f g ...)` | fn | Right-to-left function composition (unary fns) |
| `(juxt f g ...)` | fn | Apply each fn to an arg, return slice of results |
| `(apply f coll)` | any | Call f with coll's elements as arguments |
| `(partial f & fixed)` | fn | Partial application; returns a unary fn |
| `(complement pred)` | fn | Logical negation of pred |
| `(identity x)` | x | Return x unchanged |
| `(constantly v)` | fn | Return a fn that always returns v |

## Iteration

| Form | Description |
|---|---|
| `(doseq [x coll] body...)` | Evaluate body for each element in coll |
| `(dotimes [i n] body...)` | Evaluate body n times with i bound to 0..n-1 |
| `(for-chan [x ch] body...)` | Evaluate body for each value received from channel `ch`; stops when `ch` is closed |

## Concurrency

Goroutines, channels, and synchronisation. `sync` and `time` imports are added automatically when needed — no `(:import [sync])` required.

### Channels

| Form | Returns | Description |
|---|---|---|
| `(chan ^T)` | `chan T` | Create an unbuffered channel of element type T |
| `(chan ^T n)` | `chan T` | Create a buffered channel with capacity n |
| `(send! ch val)` | — | Send val on channel ch (`ch <- val`) |
| `(recv! ch)` | T | Receive one value from ch (`<-ch`) |
| `(recv-ok! ch)` | `[]any` | Receive with closed-channel detection; returns `[val ok]`. Use `[[val ok] (recv-ok! ch)]` to destructure. Check ok with `(= ok true)` — it is `any`, not `bool`. |
| `(close! ch)` | — | Close the channel |

### Goroutines

| Form | Returns | Description |
|---|---|---|
| `(go body...)` | — | Run body in a new goroutine; no result |
| `(go-val body...)` | `chan any` | Run body in a goroutine; returns a buffered channel. Call `(recv! ch)` to block until the result arrives. |
| `(par expr1 expr2 ...)` | — | Run each expression in its own goroutine, then block until all finish (`sync.WaitGroup`). Use when you want fire-and-wait parallelism with no result collection. |

### Select

```clojure
(select!
  ([val ch1]        body...)   ; receive case — binds val
  ([(send! ch2 v)]  body...)   ; send case
  (:timeout 5000    body...)   ; fires after 5000 ms
  (:default         body...))  ; non-blocking fallback
```

### Synchronisation

| Form | Returns | Description |
|---|---|---|
| `(with-lock mu body...)` | any | Execute body inside a mutex critical section. Emits `mu.Lock() / defer mu.Unlock()` inside an IIFE so unlock is guaranteed even on panic. `mu` must be a `sync.Mutex` or `sync.RWMutex` value. |
| `(defer expr)` | — | Defer expr until the enclosing function returns |

```clojure
; Future pattern: submit work, collect later
(def result (go-val (expensive-computation x)))
; ... other work ...
(recv! result)   ; blocks until the goroutine finishes

; Parallel startup
(par
  (init-cache)
  (connect-db)
  (start-metrics))

; Drain a channel until closed
(for-chan [item results-ch]
  (process item))

; Closed-channel detection
(let [[val ok] (recv-ok! ch)]
  (if (= ok true)
    (process val)
    (fmt/println "channel closed")))

; Mutex-protected counter
(def mu (sync/Mutex. {}))
(defn safe-log [^string msg]
  (with-lock mu
    (fmt/println msg)))

; Select with timeout
(select!
  ([msg ch] (handle msg))
  (:timeout 1000 (fmt/println "timed out")))
```

## I/O

| Form | Returns | Description |
|---|---|---|
| `(println & args)` | — | Print args to stdout with newline |
| `(print & args)` | — | Print args to stdout without newline |

## Errors and types

| Form | Returns | Description |
|---|---|---|
| `(error msg)` | error | Create a new error |
| `(nil? x)` | bool | True when x is nil |
| `(as ^T x)` | T | Type assertion; panics if x is not T |
