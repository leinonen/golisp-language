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
| `(conj coll x)` | coll | Append x to a slice |
| `(count coll)` | int | Number of elements |
| `(first coll)` | any | First element |
| `(rest coll)` | `[]any` | All elements after the first |
| `(nth coll i)` | any | Element at index i |
| `(contains? coll x)` | bool | True when coll contains x (map, slice, string) |
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
