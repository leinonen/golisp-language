# Interfaces and Methods

## Defining an Interface

`definterface` declares a Go interface — a set of methods a type must implement:

```golisp
(definterface Shape
  (Area    [] -> float64)
  (Describe [] -> string))
```

## Implementing Methods

`defmethod` attaches a method to a struct. The first parameter is the receiver:

```golisp
(defstruct Circle
  radius float64)

(defmethod Circle Area [c] -> float64
  (* math/pi (:radius c) (:radius c)))

(defmethod Circle Describe [c] -> string
  (fmt/sprintf "circle(r=%.2f)" (:radius c)))

(defstruct Rect
  width  float64
  height float64)

(defmethod Rect Area [r] -> float64
  (* (:width r) (:height r)))

(defmethod Rect Describe [r] -> string
  (fmt/sprintf "rect(%.2f × %.2f)" (:width r) (:height r)))
```

## Calling Methods

Methods are called as regular functions — the receiver is just the first argument. The compiler routes the call to the right method:

```golisp
(area c)       ; → c.Area()
(describe c)   ; → c.Describe()
```

This is dot-free dispatch. No `(.Area c)` syntax needed for your own methods.

## Using the Interface

Functions that accept an interface work with any implementing type:

```golisp
(defn print-shape [s Shape] -> void
  (fmt/printf "%s: area = %.4f\n" (describe s) (area s)))

(defn main [] -> void
  (let [shapes [
    (Circle. {:radius 5.0})
    (Rect.   {:width 4.0 :height 6.0})]]
    (doseq [s shapes]
      (print-shape s))))
```

Output:

```
circle(r=5.00): area = 78.5398
rect(4.00 × 6.00): area = 24.0000
```

## Methods on Named Types

`defmethod` works on `deftype` too:

```golisp
(deftype Celsius float64)
(deftype Fahrenheit float64)

(defmethod Celsius ToF [c] -> Fahrenheit
  (Fahrenheit (+ (* (float64 c) 9.0 / 5.0) 32.0)))

(let [c (Celsius 100.0)]
  (to-f c))    ; Fahrenheit(212.0)
```

## Full Example

```golisp
(ns main)

(definterface Shape
  (Area    [] -> float64)
  (Describe [] -> string))

(defstruct Circle  radius float64)
(defstruct Rect    width float64  height float64)
(defstruct Triangle a float64  b float64  c float64)

(defmethod Circle Area [c] -> float64
  (* math/pi (:radius c) (:radius c)))
(defmethod Circle Describe [c] -> string
  (fmt/sprintf "circle(r=%.2f)" (:radius c)))

(defmethod Rect Area [r] -> float64
  (* (:width r) (:height r)))
(defmethod Rect Describe [r] -> string
  (fmt/sprintf "rect(%.2f x %.2f)" (:width r) (:height r)))

(defmethod Triangle Area [t] -> float64
  (let [s (* 0.5 (+ (:a t) (:b t) (:c t)))]
    (math/sqrt (* s (- s (:a t)) (- s (:b t)) (- s (:c t))))))
(defmethod Triangle Describe [t] -> string
  (fmt/sprintf "triangle(%.2f, %.2f, %.2f)" (:a t) (:b t) (:c t)))

(defn total-area [shapes []Shape] -> float64
  (reduce (fn [sum s] (+ sum (area s))) 0.0 shapes))

(defn main [] -> void
  (let [shapes []Shape [
    (Circle.   {:radius 5.0})
    (Rect.     {:width 4.0 :height 6.0})
    (Triangle. {:a 3.0 :b 4.0 :c 5.0})]]
    (doseq [s shapes]
      (fmt/printf "  %-26s area = %.4f\n" (describe s) (area s)))
    (println "total area:" (total-area shapes))))
```
