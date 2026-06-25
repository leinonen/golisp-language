# Structs and Types

## Defining Structs

`defstruct` declares a named struct with typed fields:

```golisp
(defstruct Point
  x float64
  y float64)

(defstruct User
  id     string
  name   string
  email  string
  active bool)
```

## Creating Struct Values

Use a map literal in a context where the type is known:

```golisp
(defn make-point [x float64 y float64] -> Point
  {:x x :y y})
```

Or use the explicit struct literal syntax:

```golisp
(Point. {:x 1.0 :y 2.0})
(User. {:id "1" :name "Alice" :email "alice@example.com" :active true})
```

## Accessing Fields

Keyword access on a struct becomes a direct field read:

```golisp
(let [p (Point. {:x 3.0 :y 4.0})]
  (:x p)    ; 3.0
  (:y p))   ; 4.0
```

## Updating Structs

Structs are values. "Update" means creating a new struct with changed fields:

```golisp
(defn move [p Point dx float64 dy float64] -> Point
  {:x (+ (:x p) dx)
   :y (+ (:y p) dy)})
```

## Named Types

`deftype` creates a new named type based on a primitive:

```golisp
(deftype UserId string)
(deftype Score int)
(deftype Percentage float64)
```

Named types prevent accidentally mixing values that have different meanings:

```golisp
(def user-id UserId "user-123")
(def session-id string "sess-456")

; These are different types even though both are strings
```

## Stateful Fields with Atoms

Struct fields are normally immutable values. When a struct needs its own mutable, thread-safe state, give it an atom field typed `(Atom T)` (or bare `Atom`):

```golisp
(defstruct Counter
  label string
  count (Atom int))

(defn new-counter [name string] -> Counter
  {:label name :count (atom int 0)})

(defn bump! [c Counter] -> any
  (swap! (:count c) (fn [n] (+ (int n) 1))))
```

The struct carries its own state, so there's no need for a module-level singleton. See [Atoms](ch13-concurrency.md#atoms) for `atom`, `swap!`, `reset!`, and `deref`.

## Gradual Typing

Start with a plain map and upgrade to a struct as the code grows. When a function's parameter is typed as a struct, map literals in call position are automatically compiled as struct literals:

```golisp
(defstruct Product
  name     string
  price    float64
  stock    int)

; This map literal compiles to a Product struct literal
; because the function declares Product as the parameter type
(defn price-with-tax [p Product] -> float64
  (* (:price p) 1.24))

(price-with-tax {:name "Widget" :price 9.99 :stock 42})
```

The benefit: misspell `:pricee` and Go's compiler catches it at build time.

## Example: A Complete Struct

```golisp
(ns main)

(defstruct Circle
  radius float64)

(defstruct Rect
  width  float64
  height float64)

(defn circle-area [c Circle] -> float64
  (* math/pi (:radius c) (:radius c)))

(defn rect-area [r Rect] -> float64
  (* (:width r) (:height r)))

(defn main [] -> void
  (let [c (Circle. {:radius 5.0})
        r (Rect. {:width 4.0 :height 6.0})]
    (println "circle area:" (circle-area c))
    (println "rect area:  " (rect-area r))))
```
