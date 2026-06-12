# Built-in functions

Core functions available in every glisp program without any import.

**Truthiness**: in every condition position (`if`, `when`, `cond`, `and`, `or`,
`not`, test asserts), `nil` and `false` are falsy and every other value is
truthy — so `(if (get m "k") ...)` works directly on `any`-typed values.

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

Wherever a built-in takes a function (`map`, `filter`, `sort-by`, `group-by`,
`max-by`, …), a bare keyword works as a field accessor: `(map :title movies)`
is shorthand for `(map (fn [m] (:title m)) movies)`.

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
| `(count coll)` | int | Number of elements (works for slices, maps, sets, strings; accepts `any`) |
| `(len coll)` | int | Alias for `count` |
| `(first coll)` | any | First element |
| `(last coll)` | any | Last element; nil if empty |
| `(rest coll)` | `[]any` | All elements after the first |
| `(nth coll i)` | any | Element at index i |
| `(contains? coll x)` | bool | True when coll contains x (map, slice, set, string) |
| `(some pred coll)` | any | First element where pred is truthy, or nil |
| `(every? pred coll)` | bool | True when pred is truthy for every element |
| `(not-any? pred coll)` | bool | True when pred is falsy for every element |
| `(distinct coll)` | `[]any` | Remove duplicates, preserving order |
| `(remove pred coll)` | `[]any` | Elements where pred is falsy (inverse of `filter`) |
| `(keep f coll)` | `[]any` | Map f over coll, dropping nil results |
| `(split-at n coll)` | `[[before] [after]]` | Split into first-n and rest |
| `(split-with pred coll)` | `[[taken] [rest]]` | Split at first element where pred is falsy |
| `(interleave & colls)` | `[]any` | Interleave elements from multiple sequences; stops at the shortest |
| `(sort coll)` | `[]any` | Sort in natural order (int, float64, or string) |
| `(min-key f x y & more)` | any | Element with the smallest `(f elem)` value |
| `(max-key f x y & more)` | any | Element with the largest `(f elem)` value |
| `(min x y & more)` | any | Smallest of the numeric arguments |
| `(max x y & more)` | any | Largest of the numeric arguments |
| `(min-by f coll)` | any | Element of coll with the smallest `(f elem)` key; nil for empty coll |
| `(max-by f coll)` | any | Element of coll with the largest `(f elem)` key; nil for empty coll |

## Maps

| Form | Returns | Description |
|---|---|---|
| `(get m k)` | any | Look up key k; nil if absent |
| `(assoc m k v)` | map | New map with k set to v |
| `(dissoc m k)` | map | New map with k removed |
| `(merge m1 m2)` | map | Merge two maps; m2 keys overwrite m1 |
| `(keys m)` | `[]any` | All keys |
| `(vals m)` | `[]any` | All values |
| `(map-vals f m)` | map | Return m with f applied to each value |
| `(map-keys f m)` | map | Return m with f applied to each key (f must return a string) |
| `(reduce-kv f init m)` | any | Reduce map m with `(f acc k v)` over each entry |

## Sets

Sets are collections of unique values. Literal syntax: `#{1 2 3}`. Backed by
`map[any]struct{}` in Go. Sets are also valid sequences: `map`, `filter`,
`doseq`, `sort`, `join`, `into` and friends enumerate their elements in
sorted order (so output is deterministic).

| Form | Returns | Description |
|---|---|---|
| `#{x y z}` | set | Set literal |
| `(set coll)` | set | Build a set from any sequence, dropping duplicates |
| `(conj s x)` | set | New set with x added |
| `(contains? s x)` | bool | O(1) membership test |
| `(count s)` | int | Number of elements |
| `(empty? s)` | bool | True when s has no elements |
| `(union s1 s2)` | set | Elements in s1 or s2 |
| `(intersection s1 s2)` | set | Elements in both s1 and s2 |
| `(difference s1 s2)` | set | Elements in s1 that are not in s2 |
| `(into #{} coll)` | set | Collect a sequence into a set |

```clojure
(let [a #{1 2 3}
      b #{2 3 4}]
  (contains? a 2)        ; true
  (conj a 4)             ; #{1 2 3 4}
  (union a b)            ; #{1 2 3 4}
  (intersection a b)     ; #{2 3}
  (difference a b)       ; #{1}
  (set [3 1 2 1])        ; #{1 2 3}
  (join (set ["b" "a"]) ","))  ; "a,b" — sorted enumeration
```

## Strings

| Form | Returns | Description |
|---|---|---|
| `(str & args)` | string | Concatenate all args as strings |
| `(string x)` | string | Convert x to a string: strings pass through, numbers/bools render decimally (`(string 65)` → `"65"`), anything else becomes `""` |
| `(upper-case s)` | string | Uppercase |
| `(lower-case s)` | string | Lowercase |
| `(trim s)` | string | Strip leading/trailing whitespace |
| `(blank? s)` | bool | True when s is nil or contains only whitespace |
| `(capitalize s)` | string | Uppercase first character, lowercase the rest |
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
| `(float64 x)` | float64 | Convert x to float64 |
| `(parse-int s)` | `[int error]` | Parse decimal integer string; use with `if-err` |
| `(parse-float s)` | `[float64 error]` | Parse float string; use with `if-err` |
| `(inc n)` | any | Increment n by 1; preserves int/float64 type |
| `(dec n)` | any | Decrement n by 1; preserves int/float64 type |
| `(even? n)` | bool | True when n is even |
| `(odd? n)` | bool | True when n is odd |
| `(pos? n)` | bool | True when n is positive (> 0) |
| `(neg? n)` | bool | True when n is negative (< 0) |
| `(zero? n)` | bool | True when n is zero |

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
| `(fnil f default)` | fn | Wrap f so a nil argument becomes default — `(update m k (fnil (fn [n] (inc n)) 0))` |

Functions passed to these (and to the collection HOFs) must be `any`-typed:
a `defn` with concrete param or return types is rejected at transpile time
with a hint to wrap it in a lambda, because the runtime dispatch asserts
`func(any) any`.

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
| `(chan T)` | `chan T` | Create an unbuffered channel of element type T |
| `(chan T n)` | `chan T` | Create a buffered channel with capacity n |
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
(defn safe-log [msg string]
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
| `(println & args)` | — | Print args to stdout with newline (bare alias for `fmt/println`) |
| `(print & args)` | — | Print args to stdout without newline (bare alias for `fmt/print`) |
| `(fmt/println & args)` | — | Print args to stdout with newline |
| `(fmt/print & args)` | — | Print args to stdout without newline |

## File I/O

No import required. All file operations work with plain strings for paths and content.

| Form | Returns | Description |
|---|---|---|
| `(read-file path)` | `[string error]` | Read entire file as a string |
| `(write-file path content)` | `error` | Write content to file, creating or truncating |
| `(append-file path content)` | `error` | Append content to file, creating if absent |
| `(file-exists? path)` | `bool` | True when a file or directory exists at path |
| `(list-dir path)` | `[[]string error]` | Names of entries in a directory |
| `(mkdir path)` | `error` | Create directory and all missing parents |

```clojure
; Read a config file, handling missing file gracefully
(if-err [content err] (read-file "config.json")
  (fmt/println "no config:" err)
  (process-config content))

; Write then check error directly (write-file returns error, not [val error])
(let [err (write-file "/tmp/out.txt" result)]
  (when (not= err nil)
    (log/error "write failed" "err" err)))

; List migrations directory
(if-err [files err] (list-dir "migrations/")
  (fmt/println "cannot read migrations:" err)
  (doseq [f files] (fmt/println "migration:" f)))
```

## Logging

Structured logging via Go's `log/slog`. No import required. Each call takes a message string followed by zero or more key-value attribute pairs.

| Form | Returns | Description |
|---|---|---|
| `(log/info msg k v ...)` | — | Log at INFO level |
| `(log/debug msg k v ...)` | — | Log at DEBUG level |
| `(log/warn msg k v ...)` | — | Log at WARN level |
| `(log/error msg k v ...)` | — | Log at ERROR level |

```clojure
; Simple message
(log/info "server started" "port" 8080)

; Multiple attributes
(log/info "request" "method" method "path" path "status" 200 "ms" elapsed)

; In statement position — direct slog call, no wrapper
(log/warn "slow query" "sql" q "ms" 1420)

; Error with wrapped error
(log/error "db failed" "op" "query" "err" err)
```

Default output is text to stderr. To switch to JSON or configure a custom handler, use `slog.SetDefault` via Go interop.

## Regex

RE2-syntax regular expressions via Go's `regexp` package. No import required. Patterns are compiled at call time; `regexp.MustCompile` is used internally — an invalid pattern panics.

| Form | Returns | Description |
|---|---|---|
| `(re/match pattern s)` | `bool` | True when s matches pattern |
| `(re/find pattern s)` | `any` | Leftmost match, or nil if none |
| `(re/find-all pattern s)` | `[]any` | All non-overlapping matches |
| `(re/replace pattern s repl)` | `string` | Replace all matches with repl |
| `(re/split pattern s)` | `[]any` | Split s on pattern |

```clojure
; Validate email
(re/match "^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$" email)

; Extract first number from a string
(re/find "\\d+" "order-42-confirmed")    ; → "42"

; All words
(re/find-all "\\w+" "hello world foo")   ; → ["hello" "world" "foo"]

; Normalise whitespace
(re/replace "\\s+" text " ")

; Parse CSV (simple, no quoted fields)
(re/split "," "alice,bob,carol")         ; → ["alice" "bob" "carol"]
```

**RE2 note**: Go uses RE2 syntax — no lookaheads, lookbehinds, or backreferences. Use `regexp.Compile` via Go interop if you need error-checked compilation at startup.

## Context

Pass `context.Context` to Go APIs that support cancellation and deadlines. No import declaration needed.

| Form | Returns | Description |
|---|---|---|
| `(ctx/background)` | context | `context.Background()` — root context, never cancelled |
| `(ctx/todo)` | context | `context.TODO()` — placeholder when context is unclear |
| `(ctx/with-cancel ctx)` | `[]any{ctx, cancel}` | Derive a cancellable child context |
| `(ctx/with-timeout ctx ms)` | `[]any{ctx, cancel}` | Derive a child context that cancels after `ms` milliseconds |
| `(ctx/cancel! cancel)` | nil | Call the cancel function returned by `with-cancel` / `with-timeout` |
| `(ctx/value ctx key)` | any | Read a value stored in the context |
| `(ctx/with-value ctx key val)` | context | Derive a child context with an added key-value pair |
| `(ctx/done? ctx)` | bool | True once the context was cancelled or its deadline passed |
| `(ctx/err ctx)` | error | nil while live; `context.Canceled` / `context.DeadlineExceeded` after |

```clojure
; Timeout example — always call cancel! to release resources
(defn fetch-with-deadline [url string] -> any
  (let [[ctx cancel] (ctx/with-timeout (ctx/background) 3000)]
    (defer (ctx/cancel! cancel))
    (http/get url)))

; Cancellation — cancel from another goroutine
(defn run-with-cancel [] -> any
  (let [[ctx cancel] (ctx/with-cancel (ctx/background))]
    (go (do (fmt/println "working...") (ctx/cancel! cancel)))
    ctx))

; Context values — propagate request-scoped data
(defn with-user [ctx any user string] -> any
  (ctx/with-value ctx "user" user))

(defn get-user [ctx any] -> any
  (ctx/value ctx "user"))
```

## Errors and types

| Form | Returns | Description |
|---|---|---|
| `(error msg)` | error | Create a new error |
| `(wrap-error msg err)` | error | Wrap err with context: message becomes `"msg: err"` |
| `(errors/is? err target)` | bool | True when err or any error in its chain matches target |
| `(nil? x)` | bool | True when x is nil |
| `(as T x)` | T | Type assertion; panics if x is not T |

```clojure
; Sentinel errors
(def ErrNotFound (error "not found"))
(def ErrForbidden (error "forbidden"))

; Wrap with context as errors travel up the call stack
(defn get-user [id int] -> [any error]
  (if-err [row err] (db-query-one conn "SELECT * FROM users WHERE id=$1" [id])
    (wrap-error "get-user" err)
    [row nil]))

; Unwrap chain to check cause
(if (errors/is? err ErrNotFound)
  (web/json-response 404 {"error" "user not found"})
  (web/json-response 500 {"error" "internal error"}))
```
