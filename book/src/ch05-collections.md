# Collections

GoLisp has three collection types: vectors, maps, and sets.

## Vectors

Vectors are ordered sequences, backed by Go slices:

```golisp
[1 2 3 4 5]
["alice" "bob" "carol"]
[]                          ; empty vector
[]string                    ; typed empty vector
```

Common operations:

```golisp
(count [1 2 3])             ; 3
(first [10 20 30])          ; 10
(last  [10 20 30])          ; 30
(rest  [10 20 30])          ; [20 30]
(nth   [10 20 30] 1)        ; 20
(conj  [1 2 3] 4)           ; [1 2 3 4]
(contains? [1 2 3] 2)       ; true
(empty? [])                 ; true
(reverse [1 2 3])           ; [3 2 1]
(sort [3 1 4 1 5])          ; [1 1 3 4 5]
```

## Maps

Maps are key-value pairs. Keys can be keywords, strings, or any comparable value:

```golisp
{:name "Alice" :age 30}
{"host" "localhost" "port" 8080}
{}                              ; empty map
```

Common operations:

```golisp
(get m :name)                   ; look up a key
(:name m)                       ; keyword shorthand
(assoc m :email "a@example.com"); add/update a key
(dissoc m :age)                 ; remove a key
(merge m1 m2)                   ; merge two maps
(keys m)                        ; all keys
(vals m)                        ; all values
(contains? m :name)             ; true if key exists
```

Nested access:

```golisp
(get-in m [:address :city])
(assoc-in m [:address :city] "Helsinki")
(update-in m [:score] inc)
```

## Sets

Sets are unordered collections of unique values:

```golisp
#{1 2 3}
#{"alice" "bob" "carol"}
```

Set operations:

```golisp
(contains? #{:a :b :c} :b)          ; true
(conj #{:a :b} :c)                  ; #{:a :b :c}
(union #{:a :b} #{:b :c})           ; #{:a :b :c}
(intersection #{:a :b} #{:b :c})    ; #{:b}
(difference #{:a :b} #{:b :c})      ; #{:a}
```

## Destructuring

Destructuring unpacks collections into named bindings.

Sequential (vectors):

```golisp
(let [[a b c] [10 20 30]]
  (println a b c))    ; 10 20 30

; Ignore elements with _
(let [[_ second _] [1 2 3]]
  second)    ; 2
```

Capture the rest of a sequence with `& rest`, and the whole value with `:as`:

```golisp
(let [[head & tail] [1 2 3 4]]
  (println head tail))         ; 1 [2 3 4]

(let [[a b & rest :as whole] [10 20 30 40]]
  (println a b rest whole))    ; 10 20 [30 40] [10 20 30 40]
```

Patterns nest — destructure a vector of vectors in one step:

```golisp
(let [[[x y] [u v]] [[1 2] [3 4]]]
  (+ x y u v))                 ; 10
```

Map destructuring:

```golisp
(let [{name :name age :age} {:name "Alice" :age 30}]
  (println name age))    ; Alice 30
```

`:keys` is shorthand when the binding name matches the key. `:or` supplies
defaults for missing keys, and `:as` binds the whole map:

```golisp
(let [{:keys [name age] :or {age 0} :as person}
      {:name "Alice"}]
  (println name age person))
; Alice 0 {:name "Alice"}
```

In function parameters:

```golisp
(defn greet [{name :name}] -> string
  (str "Hello, " name "!"))

(greet {:name "Alice"})    ; "Hello, Alice!"
```

Annotated map destructuring types each field as it is pulled out — handy for
unpacking an untyped request map into typed locals without per-field casts:

```golisp
(defn signup [{name :name :- string age :age :- int}] -> string
  (format "%s is %d" name age))
```

## Core Collection Functions

```golisp
(map (fn [x] (* x 2)) [1 2 3])         ; [2 4 6]
(map-indexed (fn [i x] [i x]) [:a :b]) ; [[0 :a] [1 :b]]
(filter even? [1 2 3 4 5 6])            ; [2 4 6]
(reduce + 0 [1 2 3 4 5])                ; 15
(take 3 [1 2 3 4 5])                    ; [1 2 3]
(drop 3 [1 2 3 4 5])                    ; [4 5]
(flatten [[1 2] [3 4] [5]])             ; [1 2 3 4 5]
(distinct [1 2 2 3 3 3])                ; [1 2 3]
(group-by even? [1 2 3 4 5])            ; {true [2 4] false [1 3 5]}
(zipmap [:a :b :c] [1 2 3])             ; {:a 1 :b 2 :c 3}
(into #{} [1 2 2 3])                    ; #{1 2 3}
```

`map-indexed` passes the index alongside each element — the callback takes `(index item)`.

## List Comprehension

`for` builds a vector by iterating over one or more bindings. An optional `:when` guard filters elements:

```golisp
(for [x [1 2 3 4 5] :when (> x 2)]
  (* x x))                       ; [9 16 25]
```

Multiple bindings nest as a cartesian product:

```golisp
(for [x [1 2] y [:a :b]]
  [x y])                         ; [[1 :a] [1 :b] [2 :a] [2 :b]]
```
