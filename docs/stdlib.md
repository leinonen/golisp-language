# Standard library / package functions

Package-prefixed functions available in glisp. Naming convention: lowercase with hyphens in glisp source (`fmt/println`, `strings/has-prefix`); the transpiler auto-converts to Go PascalCase (`fmt.Println`, `strings.HasPrefix`).

**No import declarations needed for stdlib packages.** The transpiler tracks every package-qualified symbol you reference (`math/pi`, `os/exit`, `strconv/atoi`, …) and emits the correct `import` automatically. Only declare `(:import […])` for external module packages like `golisp/web`.

---

## json

Transpiler built-ins. No import needed. Both return `[value error]`; use with `if-err`.

| Form | Returns | Description |
|---|---|---|
| `(json/encode x)` | `[string error]` | Encode x as a JSON string |
| `(json/decode s)` | `[any error]` | Decode JSON string s into a value (`any` handles objects and arrays) |

```clojure
(if-err [s (json/encode {"key" "value"})]
  (println "encode error:" err)
  (println s))
```

---

## http

Transpiler built-ins. No import needed. All return `[response error]`; use with `if-err`.

Response map keys: `"status"` (int), `"headers"` (map), `"body"` (string).

| Form | Returns | Description |
|---|---|---|
| `(http/get url)` | `[response error]` | GET request |
| `(http/get url headers)` | `[response error]` | GET with headers map |
| `(http/post url body)` | `[response error]` | POST with string body |
| `(http/post url body headers)` | `[response error]` | POST with headers |
| `(http/put url body)` | `[response error]` | PUT with string body |
| `(http/put url body headers)` | `[response error]` | PUT with headers |
| `(http/delete url)` | `[response error]` | DELETE request |
| `(http/request opts)` | `[response error]` | Full control; opts keys: `"method"`, `"url"`, `"body"`, `"headers"` |

```clojure
(if-err [resp (http/get "https://api.example.com/data")]
  (println "error:" err)
  (println (get resp "body")))
```

---

## os/env

Transpiler built-in. No import needed. Returns `string`.

| Form | Returns | Description |
|---|---|---|
| `(os/env name)` | string | Read environment variable; `""` if unset |
| `(os/env name default)` | string | Read environment variable; return default when unset |

```clojure
(def port (os/env "PORT" "3000"))
```

---

## fmt

Go `fmt` package. Auto-imported.

| Form | Returns | Description |
|---|---|---|
| `(fmt/println & args)` | `[int error]` | Print to stdout with newline |
| `(fmt/printf fmt & args)` | `[int error]` | Formatted print to stdout |
| `(fmt/sprintf fmt & args)` | string | Format and return a string |
| `(fmt/errorf fmt & args)` | error | Create a formatted error; use `%w` to wrap |
| `(fmt/fprintf w fmt & args)` | `[int error]` | Write formatted string to writer w |
| `(fmt/fprintln w & args)` | `[int error]` | Write args with newline to writer w |
| `(fmt/sscanf str fmt & args)` | `[int error]` | Parse values from str |

---

## os

Go `os` package. Auto-imported. Prefer `os/env` for environment variables.

| Form | Returns | Description |
|---|---|---|
| `(os/exit code)` | — | Exit the process with status code |
| `os/args` | `[]string` | Command-line arguments (program name first) |

---

## strconv

Go `strconv` package. Auto-imported.

| Form | Returns | Description |
|---|---|---|
| `(strconv/atoi s)` | `[int error]` | Parse decimal integer string |
| `(strconv/itoa i)` | string | Integer to decimal string |
| `(strconv/parse-int s base bit-size)` | `[int64 error]` | Parse integer in given base |
| `(strconv/parse-float s bit-size)` | `[float64 error]` | Parse float; use bit-size 64 |
| `(strconv/format-int i base)` | string | Format integer in given base |
| `(strconv/format-float f fmt prec)` | string | Format float with given verb and precision |

---

## strings

Go `strings` package. Auto-imported. Glisp also provides simpler wrappers — see [builtins.md](builtins.md).

| Form | Returns | Description |
|---|---|---|
| `(strings/contains s substr)` | bool | True when s contains substr |
| `(strings/has-prefix s prefix)` | bool | True when s begins with prefix |
| `(strings/has-suffix s suffix)` | bool | True when s ends with suffix |
| `(strings/trim-space s)` | string | Strip leading/trailing whitespace |
| `(strings/to-upper s)` | string | Uppercase |
| `(strings/to-lower s)` | string | Lowercase |
| `(strings/split s sep)` | `[]string` | Split on sep |
| `(strings/join elems sep)` | string | Join with sep |
| `(strings/replace s old new n)` | string | Replace up to n occurrences; -1 for all |
| `(strings/replace-all s old new)` | string | Replace all occurrences |
| `(strings/index s substr)` | int | Byte index of first occurrence; -1 if absent |
| `(strings/trim-prefix s prefix)` | string | Remove leading prefix |
| `(strings/trim-suffix s suffix)` | string | Remove trailing suffix |
| `(strings/trim s cutset)` | string | Remove leading/trailing chars in cutset |
| `(strings/count s substr)` | int | Count non-overlapping occurrences |
| `(strings/repeat s n)` | string | Repeat s n times |

---

## sort

Go `sort` package. Auto-imported. Glisp also provides `sort-by` — see [builtins.md](builtins.md).

| Form | Description |
|---|---|
| `(sort/slice coll less-fn)` | Sort coll in-place; `less-fn` is `(fn [i j] ...)` returning bool |
| `(sort/ints s)` | Sort integer slice in ascending order in-place |
| `(sort/strings s)` | Sort string slice in ascending order in-place |

---

## math

Go `math` package. Auto-imported.

| Form | Returns | Description |
|---|---|---|
| `(math/abs x)` | float64 | Absolute value |
| `(math/sqrt x)` | float64 | Square root |
| `(math/pow x y)` | float64 | x to the power y |
| `(math/floor x)` | float64 | Round down |
| `(math/ceil x)` | float64 | Round up |
| `(math/round x)` | float64 | Round to nearest integer |
| `(math/max a b)` | float64 | Larger of a and b |
| `(math/min a b)` | float64 | Smaller of a and b |
| `math/pi` | float64 | π |

---

## sync

Go `sync` package. Auto-imported whenever `sync/Mutex.`, `sync/WaitGroup.`, etc. are used directly. `par` and `with-lock` also auto-import it.

| Form | Description |
|---|---|
| `(sync/Mutex. {})` | Create a new mutex (value type — use a pointer `^*sync/Mutex` when sharing across goroutines) |
| `(sync/RWMutex. {})` | Create a read/write mutex |
| `(sync/WaitGroup. {})` | Create a WaitGroup; prefer `(par ...)` unless you need dynamic goroutine counts |
| `(.Lock mu)` | Acquire exclusive lock |
| `(.Unlock mu)` | Release lock |
| `(.RLock mu)` | Acquire read lock (RWMutex only) |
| `(.RUnlock mu)` | Release read lock |
| `(.Add wg n)` | Add n to WaitGroup counter |
| `(.Done wg)` | Decrement WaitGroup counter by 1 |
| `(.Wait wg)` | Block until WaitGroup counter reaches 0 |

For most locking needs, prefer `(with-lock mu body...)` — it emits Lock/defer-Unlock correctly.

---

## time

Go `time` package. Auto-imported.

| Form | Returns | Description |
|---|---|---|
| `(time/now)` | time.Time | Current local time |
| `(time/sleep duration)` | — | Pause for duration |
| `(time/since t)` | time.Duration | Elapsed time since t |
| `(time/until t)` | time.Duration | Duration until t |
| `time/second` | time.Duration | 1 second |
| `time/millisecond` | time.Duration | 1 millisecond |
| `time/minute` | time.Duration | 1 minute |
| `time/hour` | time.Duration | 1 hour |

---

## log

Go `log` package. Auto-imported.

| Form | Description |
|---|---|
| `(log/println & args)` | Log args with timestamp |
| `(log/printf fmt & args)` | Log formatted string with timestamp |
| `(log/fatal & args)` | Log args and exit with status 1 |
| `(log/fatalf fmt & args)` | Log formatted string and exit with status 1 |
