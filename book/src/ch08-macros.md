# Macros

GoLisp code is data. A macro is a function that runs at compile time, taking
unevaluated forms and returning a new form to compile in their place. This is how
the language grows without changing the compiler — much of `core` (including `->`,
`->>`, `when-not`, and `if-not`) is written as macros, not built in.

## Defining a Macro

`defmacro` looks like `defn`, but its arguments arrive as code and its result is
code. Use syntax-quote `` ` `` to write a template, `~` to splice in a value, and
`~@` to splice in the elements of a sequence:

```golisp
;;; (unless test body...) — evaluate body only when test is falsy.
(defmacro unless [test & body]
  `(if ~test nil (do ~@body)))
```

Now `unless` reads like a built-in form:

```golisp
(unless (> x 100)
  (println "x is small"))
```

At compile time that expands to:

```golisp
(if (> x 100) nil (do (println "x is small")))
```

## The Reader Syntax

| Syntax | Name | Meaning |
|--------|------|---------|
| `` `form `` | syntax-quote | Build `form` as data (a template) |
| `~x` | unquote | Splice the *value* of `x` into the template |
| `~@xs` | unquote-splice | Splice the *elements* of sequence `xs` |
| `'form` | quote | `form` as data, evaluating nothing |

## Auto-gensym

A macro that introduces its own binding needs a name that can't collide with the
caller's code. Write `name#` inside a syntax-quote and each occurrence expands to
one fresh symbol:

```golisp
;;; (inc-all coll) — add 1 to every element.
(defmacro inc-all [coll]
  `(map (fn [n#] (+ n# 1)) ~coll))

(inc-all [1 2 3])    ; [2 3 4]
```

The `n#` becomes a unique symbol like `n__1__auto`, so the macro never captures a
variable from the call site. GoLisp's hygiene model is Clojure's: non-hygienic by
default, with auto-gensym (and the `gensym` built-in) as the tool you reach for.

## Inspecting Expansion

`glisp macroexpand file.glsp` prints the file with every macro call expanded —
the fastest way to see what your macro produces:

```bash
glisp macroexpand mymacros.glsp
```

```golisp
(defn main [] -> void
  (if (> x 100) nil (do (println "x is small")))
  (println (map (fn [n__1__auto] (+ n__1__auto 1)) [1 2 3])))
```

## What Runs in a Macro Body

Macro bodies run in the compiler, not the final program, so they see only a
compile-time subset of the language: `fn`, `let`, `if`, `cond`, `do`, the logic
operators, and pure list/map/symbol helpers (`first`, `rest`, `map`, `reduce`,
`conj`, `concat`, `symbol`, `keyword`, `gensym`, `str`, …). They cannot do I/O,
spawn goroutines, or call Go interop — a macro computes a form, nothing more.

This is enough to do real work. The thread-first macro is just a `reduce` over its
forms:

```golisp
;;; (-> x form...) — insert x as the first argument of each form.
(defmacro -> [x & forms]
  (reduce
    (fn [acc form]
      (if (list? form)
        `(~(first form) ~acc ~@(rest form))
        `(~form ~acc)))
    x
    forms))
```

Two rules to remember: macros are **define-before-use** (a macro must appear above
its first call, in a single top-to-bottom pass), and forms that must control Go's
type system or statement placement — `let` with typed bindings, `defn` itself —
stay built in rather than being macro-defined.
