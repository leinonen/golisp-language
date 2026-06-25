# Appendix A: Built-in Reference

## Arithmetic

| Form | Description |
|------|-------------|
| `(+ a b ...)` | Add |
| `(- a b ...)` | Subtract |
| `(* a b ...)` | Multiply |
| `(/ a b)` | Divide |
| `(mod a b)` | Remainder |
| `(inc n)` | Add 1 |
| `(dec n)` | Subtract 1 |
| `(abs n)` | Absolute value |
| `(min a b ...)` | Minimum |
| `(max a b ...)` | Maximum |

## Comparison and Logic

| Form | Description |
|------|-------------|
| `(= a b)` | Equal |
| `(not= a b)` | Not equal |
| `(< a b)`, `(> a b)` | Less / greater |
| `(<= a b)`, `(>= a b)` | Less-or-equal / greater-or-equal |
| `(and a b ...)` | Logical and (short-circuit) |
| `(or a b ...)` | Logical or (short-circuit) |
| `(not x)` | Logical not |
| `(nil? x)` | True if nil |
| `(even? n)`, `(odd? n)` | Parity check |
| `(pos? n)`, `(neg? n)`, `(zero? n)` | Sign checks |

## Collections

| Form | Description |
|------|-------------|
| `(count coll)` / `(len coll)` | Number of elements |
| `(first coll)` | First element |
| `(last coll)` | Last element |
| `(rest coll)` | All but first |
| `(nth coll i)` | Element at index i |
| `(conj coll x)` | Append x |
| `(contains? coll x)` | Membership test |
| `(empty? coll)` | True if empty |
| `(reverse coll)` | Reversed copy |
| `(sort coll)` | Sorted copy |
| `(sort-by f coll)` | Sort by key function |
| `(take n coll)` | First n elements |
| `(drop n coll)` | All but first n |
| `(flatten coll)` | Flatten nested collections |
| `(distinct coll)` | Remove duplicates |
| `(range n)` / `(range a b)` | Integer range |
| `(repeat n x)` | Repeat x n times |

## Higher-Order Functions

| Form | Description |
|------|-------------|
| `(map f coll)` | Apply f to each element |
| `(map-indexed f coll)` | Apply f to (index, element) |
| `(for [x coll :when pred] expr)` | List comprehension |
| `(filter pred coll)` | Keep elements where pred is truthy |
| `(reduce f init coll)` | Fold with accumulator |
| `(some pred coll)` | First truthy result of pred, or nil |
| `(every? pred coll)` | True if pred is truthy for all |
| `(keep f coll)` | map, removing nil results |
| `(mapcat f coll)` | map then flatten |
| `(group-by f coll)` | Map from key to matching elements |
| `(frequencies coll)` | Map from element to count |
| `(partition n coll)` | Chunks of size n |
| `(take-while pred coll)` | Take while predicate holds |
| `(drop-while pred coll)` | Drop while predicate holds |
| `(sort-by f coll)` | Sort by key function |
| `(min-by f coll)` / `(max-by f coll)` | Extremum by key |

## Function Utilities

| Form | Description |
|------|-------------|
| `(comp f g ...)` | Compose functions right-to-left |
| `(partial f a ...)` | Fix some arguments |
| `(complement pred)` | Negate a predicate |
| `(juxt f g ...)` | Apply multiple functions, return vector |
| `(apply f coll)` | Call f with collection as args |
| `(identity x)` | Return x unchanged |
| `(constantly x)` | Function that always returns x |
| `(fnil f default)` | Replace nil arg with default |

## Threading and Debugging

| Form | Description |
|------|-------------|
| `(-> x form ...)` | Thread x as first argument |
| `(->> x form ...)` | Thread x as last argument |
| `(some-> x form ...)` | `->`, short-circuiting to nil on a nil step |
| `(some->> x form ...)` | `->>`, short-circuiting to nil on a nil step |
| `(cond-> x test form ...)` | `->` through forms whose test is truthy |
| `(cond->> x test form ...)` | `->>` through forms whose test is truthy |
| `(as-> x name form ...)` | Thread x bound to name (any position) |
| `(tap-> x form ...)` | `->`, printing each stage |
| `(tap->> x form ...)` | `->>`, printing each stage |
| `(pp x)` | Pretty-print x, return it unchanged |
| `(time-it expr)` | Print elapsed time, return expr's value |

## Maps

| Form | Description |
|------|-------------|
| `(get m k)` | Value for key k |
| `(get m k default)` | Value for key k, or default |
| `(assoc m k v)` | Set key |
| `(dissoc m k)` | Remove key |
| `(merge m1 m2)` | Merge maps (m2 wins) |
| `(keys m)` | All keys |
| `(vals m)` | All values |
| `(get-in m [k1 k2])` | Nested get |
| `(assoc-in m [k1 k2] v)` | Nested set |
| `(update-in m [k1 k2] f)` | Nested update |
| `(update m k f)` | Apply f to value at k |
| `(select-keys m [k1 k2])` | Keep only these keys |
| `(rename-keys m {old new})` | Rename keys |
| `(map-vals f m)` | Apply f to all values |
| `(map-keys f m)` | Apply f to all keys |
| `(reduce-kv f init m)` | Fold over key-value pairs |
| `(zipmap keys vals)` | Build map from parallel sequences |

## Sets

| Form | Description |
|------|-------------|
| `(set coll)` | Convert to set |
| `(union s1 s2)` | All elements in either |
| `(intersection s1 s2)` | Elements in both |
| `(difference s1 s2)` | Elements in s1 but not s2 |

## Strings

The `str/` namespace is the canonical string library — prefer it. `str`,
`string`, and `format` are bare core forms; the other bare string functions
(`upper-case`, `split`, …) are legacy aliases of the `str/` surface, kept working
for compatibility.

| Form | Description |
|------|-------------|
| `(str a b ...)` | Concatenate (all types) |
| `(string x)` | Convert a single value to a string |
| `(format fmt args...)` | Printf-style formatting |
| `(str/upper s)` | Uppercase |
| `(str/lower s)` | Lowercase |
| `(str/trim s)` | Strip whitespace |
| `(str/trim-start s)` / `(str/trim-end s)` | Strip leading / trailing whitespace |
| `(str/blank? s)` | True if empty or whitespace |
| `(str/starts-with? s prefix)` | Prefix check |
| `(str/ends-with? s suffix)` | Suffix check |
| `(str/includes? s sub)` | Substring check |
| `(str/index-of s sub)` / `(str/last-index-of s sub)` | First / last index of sub |
| `(str/replace s old new)` | Replace all occurrences |
| `(str/replace-first s old new)` | Replace first occurrence |
| `(str/split s sep)` | Split by separator |
| `(str/join sep coll)` | Join with separator (separator first) |
| `(str/repeat s n)` | Repeat s n times |
| `(str/capitalize s)` | Uppercase first char, lowercase the rest |
| `(str/pad-left s width)` / `(str/pad-right s width)` | Pad to width |
| `(subs s start end?)` | Substring |

## Numbers

| Form | Description |
|------|-------------|
| `(int x)` | Convert to int |
| `(float64 x)` | Convert to float64 |
| `(parse-int s)` | Parse string → `[int error]` |
| `(parse-float s)` | Parse string → `[float64 error]` |

## Math

Go's `math` package is reached directly — `(math/sqrt x)`, `(math/floor x)`,
`(math/pow x y)`, `(math/abs x)`, the constant `math/pi`, and so on. The `math/`
core namespace adds everyday helpers:

| Form | Description |
|------|-------------|
| `(math/clamp x lo hi)` | Constrain x to `[lo, hi]` |
| `(math/sign x)` | -1, 0, or 1 by the sign of x |
| `(math/gcd a b)` | Greatest common divisor |
| `(math/lcm a b)` | Least common multiple |
| `(math/round-to x places)` | Round to the given decimal places |

## I/O

| Form | Description |
|------|-------------|
| `(println args...)` | Print with newline |
| `(print args...)` | Print without newline |

## File I/O

`slurp`/`spit`/`lines` are the glisp-native (clojure.core-style) forms; the
lower-level forms remain available.

| Form | Description |
|------|-------------|
| `(slurp path)` | Read whole file → `[string error]` |
| `(spit path content)` | Write content to file → `error` |
| `(lines s)` | Split a string into `[]string` lines |
| `(read-file path)` | `[string error]` |
| `(write-file path content)` | `error` |
| `(append-file path content)` | `error` |
| `(file-exists? path)` | `bool` |
| `(list-dir path)` | `[[]string error]` |
| `(mkdir path)` | `error` |

## Paths and Filesystem

| Form | Description |
|------|-------------|
| `(path/join & parts)` | Join with the OS separator |
| `(path/dir p)` | Directory part |
| `(path/base p)` | Last element (file name) |
| `(path/ext p)` | Extension, including the dot |
| `(path/clean p)` | Shortest equivalent path |
| `(glob pattern)` | Paths matching a single-level shell pattern |
| `(walk dir)` | Every regular file under dir, recursively |

## System and CLI

| Form | Description |
|------|-------------|
| `(sys/args)` | Command-line arguments (program name first) |
| `(sys/env name)` / `(sys/env name default)` | Environment variable, with optional default |
| `(sys/exit code)` | Exit the process |
| `(cli/parse-opts args specs)` | Parse options → `{:options :arguments :errors :summary}` |

## Subprocess

Both forms return `{:out :err :exit :ok}`.

| Form | Description |
|------|-------------|
| `(proc/run cmd & args)` | Run a command directly (no shell) |
| `(proc/sh command)` | Run via `sh -c` (pipes, globs, redirection) |

## JSON

| Form | Description |
|------|-------------|
| `(json/encode x)` | `[string error]` |
| `(json/decode s)` | `[any error]` |

## CSV

Header-mapped: the first record is the header; each later record becomes a
`map[string]any`. Both forms return `[value error]`.

| Form | Description |
|------|-------------|
| `(csv/parse text)` | Parse CSV to a list of header-keyed maps |
| `(csv/write rows)` | Write a sequence of maps as CSV |

## Transducers

Called with a single argument (no collection), `map`, `filter`, `remove`, `keep`,
`take`, `drop`, `take-while`, and `drop-while` return a transducer. `comp` composes
them (data flows left-to-right).

| Form | Description |
|------|-------------|
| `(map f)`, `(filter pred)`, … | The 1-arg form is a transducer |
| `(transduce xform rf init coll)` | Reduce coll, items transformed through xform |
| `(sequence xform coll)` | Apply xform, return a vector |
| `(into to xform coll)` | Pour transformed items into `to` |

## Line- and JSON-Oriented I/O

`transduce-lines`/`transduce-json` stream through a transducer in constant memory;
`take`/`take-while` stop reading early. All return `[value error]`.

| Form | Description |
|------|-------------|
| `(read-lines path)` | A file's lines as a vector → `[[]any error]` |
| `(transduce-lines xform rf init path)` | Stream a file's lines through xform + rf |
| `(transduce-json xform rf init path)` | Stream a top-level JSON array's elements |

## HTTP Client

| Form | Description |
|------|-------------|
| `(http/get url)` | GET request |
| `(http/post url body)` | POST request |
| `(http/put url body)` | PUT request |
| `(http/delete url)` | DELETE request |
| `(http/request opts)` | Custom request |

## Regex (RE2)

| Form | Description |
|------|-------------|
| `(re/match pattern s)` | `bool` |
| `(re/find pattern s)` | First match or nil |
| `(re/find-all pattern s)` | All matches |
| `(re/replace pattern s repl)` | Replace matches |
| `(re/split pattern s)` | Split by pattern |

## Concurrency

| Form | Description |
|------|-------------|
| `(go body...)` | Spawn goroutine |
| `(go-val T body...)` | Goroutine returning `chan T` |
| `(par expr...)` | Run in parallel, wait for all |
| `(chan T)` | Unbuffered channel |
| `(chan T n)` | Buffered channel |
| `(send! ch val)` | Send value |
| `(recv! ch)` | Receive value |
| `(recv-ok! ch)` | Receive with close detection `[val ok]` |
| `(close! ch)` | Close channel |
| `(for-chan [x ch] body...)` | Range over channel |
| `(select! clauses...)` | Select on channels |
| `(pipeline [x src] stage...)` | Chain goroutine stages, returns a channel |
| `(fan-out n [item ch] body...)` | n workers draining ch, blocks until done |
| `(fan-in ch1 ch2 ...)` | Merge channels into one |
| `(with-lock mu body...)` | Mutex critical section |
| `(with-open [x res ...] body...)` | Bind resources, Close on exit (LIFO) |
| `(doto x form...)` | Run side-effecting forms on x, return x |
| `(defer expr)` | Defer to function end |

## Atoms

| Form | Description |
|------|-------------|
| `(atom v)` | Create a thread-safe atom holding v |
| `(atom T v)` | Typed atom; `deref` returns a concrete T |
| `(swap! a f)` | Atomically apply f, return new value |
| `(reset! a v)` | Set value unconditionally |
| `(deref a)` | Read current value |

## Context

| Form | Description |
|------|-------------|
| `(ctx/background)` | Background context |
| `(ctx/with-cancel ctx)` | `[context cancel]` |
| `(ctx/with-timeout ctx ms)` | `[context cancel]` |
| `(ctx/cancel! cancel)` | Cancel a context |
| `(ctx/done? ctx)` | True if context cancelled |
| `(ctx/value ctx key)` | Context value |
| `(ctx/with-value ctx key val)` | Context with value |

## Logging

| Form | Description |
|------|-------------|
| `(log/info msg k v ...)` | Structured info log |
| `(log/debug msg k v ...)` | Debug log |
| `(log/warn msg k v ...)` | Warning log |
| `(log/error msg k v ...)` | Error log |

## Errors

| Form | Description |
|------|-------------|
| `(error msg)` | Create error |
| `(wrap-error msg err)` | Wrap error with context |
| `(errors/is? err target)` | Check error type |
| `(panic msg)` | Panic |
| `(recover)` | Catch panic (in deferred fn) |
| `(assert cond)` | Panic if falsy |
| `(assert cond msg)` | Panic with message if falsy |
