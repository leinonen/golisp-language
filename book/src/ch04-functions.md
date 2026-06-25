# Functions

## Defining Functions

`defn` declares a named function. Type annotations are positional — the type follows its argument:

```golisp
(defn add [a int b int] -> int
  (+ a b))

(defn greet [name string] -> string
  (str "Hello, " name "!"))
```

Call them like any other expression:

```golisp
(add 3 4)          ; 7
(greet "Alice")    ; "Hello, Alice!"
```

## Multiple Return Values

Functions can return multiple values — this is how Go error handling works:

```golisp
(defn divide [a float64 b float64] -> [float64 error]
  (if (= b 0.0)
    (values 0.0 (error "division by zero"))
    (values (/ a b) nil)))
```

Bind multiple return values with `let`:

```golisp
(let [[result err] (divide 10.0 3.0)]
  (if err
    (println "error:" err)
    (println "result:" result)))
```

## Anonymous Functions

`fn` creates an anonymous function:

```golisp
(fn [x int] -> int (* x x))

; Used inline:
(map (fn [x] (* x 2)) [1 2 3 4])    ; [2 4 6 8]
```

## Closures

Anonymous functions close over their enclosing scope:

```golisp
(defn make-adder [n int] -> any
  (fn [x int] -> int (+ x n)))

(let [add5 (make-adder 5)]
  (add5 10))    ; 15
```

## Variadic Functions

Use `& rest` to accept variable arguments:

```golisp
(defn sum [& nums] -> int
  (reduce (fn [acc n] (+ acc (int n))) 0 nums))

(sum 1 2 3 4 5)    ; 15
```

## Tail Recursion with loop/recur

GoLisp has no general TCO, but `loop`/`recur` gives you explicit tail calls:

```golisp
(defn fib [n int] -> int
  (loop [i int n  a int 0  b int 1]
    (if (<= i 0)
      a
      (recur (- i 1) b (+ a b)))))

(fib 10)    ; 55
```

`loop` establishes the bindings; `recur` jumps back to the top with new values. The type annotations in `loop` keep arithmetic concrete — no boxing to `any`.

## No-Argument and Void Functions

Functions with no parameters use an empty vector:

```golisp
(defn greet-world [] -> void
  (println "Hello, World!"))
```

`void` means the function returns nothing (Go's no-return-value convention).
