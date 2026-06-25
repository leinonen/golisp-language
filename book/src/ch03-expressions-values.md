# Expressions and Values

GoLisp has no statements. Everything is an expression that produces a value.

## S-Expressions

Code is data. A function call is a list where the first element is the function and the rest are arguments:

```golisp
(+ 1 2)          ; 3
(* 3 (+ 1 2))    ; 9
(str "hello" " " "world")  ; "hello world"
```

This uniform syntax means there's no distinction between operators and functions — `+` is just a function that takes numbers.

## Primitive Types

```golisp
42          ; int
3.14        ; float64
"hello"     ; string
true        ; bool
false       ; bool
nil         ; zero value / absence
```

Numeric literals are `int` by default. Use `float64` to convert:

```golisp
(float64 42)   ; 42.0
(int 3.9)      ; 3
```

Arithmetic on `any`-typed values (such as map lookups) coerces numbers automatically, so these explicit conversions are needed only when you want to pin a type — see [Numeric Auto-Coercion](ch11-go-interop.md#numeric-auto-coercion).

## Let Bindings

`let` binds names to values within a scope:

```golisp
(let [x 10
      y 20]
  (+ x y))    ; 30
```

Bindings can include an explicit type annotation:

```golisp
(let [x int 10
      s string "hello"]
  (println s x))
```

Bindings are sequential — each one can reference the previous:

```golisp
(let [x 5
      y (* x 2)
      z (+ x y)]
  z)    ; 15
```

## Top-Level Definitions

`def` binds a value at the top level of a namespace:

```golisp
(def pi float64 3.14159)
(def greeting "Hello, GoLisp!")
(def primes []int [2 3 5 7 11 13])
```

## Keywords

Keywords are self-evaluating identifiers that start with `:`. They're used as map keys and field names:

```golisp
:name
:user-id
:enabled?
```

Keywords can look up values in maps:

```golisp
(let [m {:name "Alice" :age 30}]
  (:name m))    ; "Alice"
```

## Truthiness

Only two values are falsy: `nil` and `false`. Everything else is truthy — including `0`, empty strings, and empty collections.

```golisp
(if 0    "truthy" "falsy")    ; "truthy"
(if ""   "truthy" "falsy")    ; "truthy"
(if nil  "truthy" "falsy")    ; "falsy"
(if false "truthy" "falsy")   ; "falsy"
```

This means you can use map lookups directly as conditions without `(not= result nil)` boilerplate:

```golisp
(if (get m "key") "found" "missing")
```

## Equality

`=` compares by value, not identity. Collections are equal when their contents
are equal, and numbers compare across types, so an `int` and an equal `float64`
are `=`:

```golisp
(= [1 2 3] [1 2 3])         ; true
(= {:a 1} {:a 1})           ; true
(= 1 1.0)                   ; true
(= 1 "1")                   ; false  (different types of value)
```

`not=` is its negation. This matters when comparing values that flow through
`any`-typed maps or boxed arithmetic — a computed `int64` still compares equal to
an `int` literal.

## Comments

Single-line comments start with `;`:

```golisp
; This is a comment
(+ 1 2)  ; inline comment
```

Three semicolons mark a docstring — shown by `glisp doc`:

```golisp
;;; Returns the sum of a and b.
(defn add [a int b int] -> int (+ a b))
```
