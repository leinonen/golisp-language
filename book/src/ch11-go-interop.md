# Go Interop

GoLisp compiles to Go. Every Go package is available — the interop syntax is lightweight and predictable.

## Calling Go Methods

Prefix a method name with `.` to call it on a value:

```golisp
(.Year t)              ; → t.Year()
(.Format t "2006-01-02")  ; → t.Format("2006-01-02")
(.Close conn)          ; → conn.Close()
(.String err)          ; → err.Error() ... actually err.String()
```

This is the syntax for Go methods you don't own. For your own `defmethod` methods, just call them by name.

## Accessing Fields

Prefix with `.-` to read a struct field:

```golisp
(.-Timeout client)     ; → client.Timeout
(.-Status resp)        ; → resp.Status
```

## Struct Literals

Use `TypeName.` followed by a map to create a Go struct:

```golisp
(http/Client. {})
(http/Client. {:Timeout (* 30 time/Second)})
(sync/Mutex. {})
(net/TCPAddr. {:IP net/IPv4loopback :Port 8080})
```

## Qualified Calls

Call functions from any Go package using `package/FunctionName`:

```golisp
(time/now)                         ; → time.Now()
(strings/has-prefix s "https://")  ; → strings.HasPrefix(s, "https://")
(strings/to-upper s)               ; → strings.ToUpper(s)
(math/sqrt 2.0)                    ; → math.Sqrt(2.0)
(fmt/sprintf "%.2f" 3.14)          ; → fmt.Sprintf("%.2f", 3.14)
(os/exit 1)                        ; → os.Exit(1)
```

## Package Constants and Variables

Access package-level constants and variables with `/`:

```golisp
math/pi             ; → math.Pi
time/Second         ; → time.Second
time/Millisecond    ; → time.Millisecond
os/stdin            ; → os.Stdin
io/EOF              ; → io.EOF
```

## Type Assertions

Use `as` to assert a value's type:

```golisp
(as *Circle val)        ; → val.(*Circle)
(as string raw)         ; → raw.(string)
(as error iface)        ; → iface.(error)
```

## Importing External Go Packages

Declare imports in `ns`:

```golisp
(ns main
  (:import [github.com/google/uuid]
           [github.com/jackc/pgx/v5]))

(defn new-id [] -> string
  (uuid/new-string))

(defn connect [url string] -> [any error]
  (pgx/connect (ctx/background) url))
```

Add the dependency to `glisp.mod`:

```
module github.com/myuser/myapp

go-require (
  github.com/google/uuid v1.6.0
  github.com/jackc/pgx/v5 v5.7.2
)
```

Then fetch it:

```bash
glisp get -go github.com/google/uuid@v1.6.0
```

## Common Patterns

**Time:**

```golisp
(let [now (time/now)
      year (.Year now)
      formatted (.Format now "2006-01-02")]
  (println year formatted))
```

**HTTP client:**

```golisp
(let [client (http/Client. {:Timeout (* 10 time/Second)})]
  (if-err [resp err] (.Get client "https://example.com")
    (println "error:" err)
    (println "status:" (.-StatusCode resp))))
```

**Sorting with a custom comparator:**

```golisp
(sort/slice items (fn [i int j int] -> bool
  (< (:priority (nth items i)) (:priority (nth items j)))))
```

## doto — Fluent Configuration

`doto` evaluates a value once, runs a sequence of side-effecting steps on it, and returns the value. A `(.method args)` step threads the value as the receiver, so builder-style APIs read as a chain:

```golisp
(doto (new-box)
  (.Add "a")
  (.Add "b")
  (.Show))    ; returns the box
```

A bare `(fn args)` step threads the value as the first argument instead. Either way, `doto` returns the original object — handy for "build, configure, hand back" patterns without a throwaway `let`.

## Numeric Auto-Coercion

Values typed as `any` — map lookups, untyped parameters, range variables — coerce automatically in arithmetic and comparisons. No explicit cast is needed:

```golisp
(defn total [m] -> int
  (+ (get m "a") (get m "b")))    ; map values are `any`; result coerces to int
```

Integer-ness is preserved when no float is involved. Typed numeric code stays native (no coercion overhead), and explicit `int` / `float64` remain available when you want to pin a conversion.

## Auto-Imported Packages

These packages are imported automatically when used — no `ns` declaration needed:

`fmt`, `os`, `strings`, `strconv`, `sort`, `math`, `time`, `sync`
