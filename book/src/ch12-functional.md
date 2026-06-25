# Functional Programming

Functions are first-class values. Pass them, return them, compose them.

## map, filter, reduce

The core trio:

```golisp
(map (fn [x] (* x x)) [1 2 3 4 5])    ; [1 4 9 16 25]

(filter even? [1 2 3 4 5 6])           ; [2 4 6]

(reduce + 0 [1 2 3 4 5])               ; 15
(reduce str "" ["a" "b" "c"])          ; "abc"
```

`map` and `filter` accept any callable — named function, anonymous function, or keyword.

## Keywords as Functions

A bare keyword acts as a field accessor — useful with `map`:

```golisp
(def movies
  [{:title "Arrival"   :year 2016 :rating 7.9}
   {:title "Heat"      :year 1995 :rating 8.3}
   {:title "Parasite"  :year 2019 :rating 8.5}])

(map :title movies)     ; ["Arrival" "Heat" "Parasite"]
(map :rating movies)    ; [7.9 8.3 8.5]
```

## partial

`partial` fixes some arguments, returning a new function for the rest:

```golisp
(def add5 (partial + 5))
(add5 10)    ; 15
(add5 20)    ; 25

(def short? (partial shorter-than? 120))
(filter short? movies)
```

## comp

`comp` chains functions right-to-left:

```golisp
(def loud-greet (comp str/upper (partial str "Hello, ")))
(loud-greet "alice")    ; "HELLO, ALICE"
```

## complement

`complement` negates a predicate:

```golisp
(def odd? (complement even?))
(filter odd? [1 2 3 4 5])    ; [1 3 5]
```

## juxt

`juxt` applies multiple functions to the same value, returning a vector of results:

```golisp
((juxt :title :year :rating) (first movies))
; ["Arrival" 2016 7.9]

(map (juxt :title :rating) movies)
; [["Arrival" 7.9] ["Heat" 8.3] ["Parasite" 8.5]]
```

## apply

`apply` calls a function with a collection as its argument list:

```golisp
(apply + [1 2 3 4 5])       ; 15
(apply str ["a" "b" "c"])   ; "abc"
(apply max [3 1 4 1 5 9])   ; 9
```

## Advanced Collection Functions

```golisp
; Sort by a derived key
(sort-by :rating movies)
(sort-by (comp - :rating) movies)    ; descending

; Group into a map
(group-by :year movies)
; {2016 [{...}] 1995 [{...}] 2019 [{...}]}

; Count occurrences
(frequencies [:a :b :a :c :b :a])
; {:a 3 :b 2 :c 1}

; Partition into chunks
(partition 2 [1 2 3 4 5 6])
; [[1 2] [3 4] [5 6]]

; Take/drop while a condition holds
(take-while even? [2 4 6 1 2])    ; [2 4 6]
(drop-while even? [2 4 6 1 2])    ; [1 2]

; like map but filters nils
(keep (fn [x] (when (even? x) (* x 10))) [1 2 3 4])
; [20 40]
```

## Pipelines Without Threading Macros

Chain operations with intermediate `let` bindings:

```golisp
(defn top-picks [catalog any watched any] -> []any
  (let [unseen (filter (fn [m] (not (contains? watched (:title m)))) catalog)
        ranked (sort-by (fn [m] (- (float64 (:rating m)))) unseen)
        top    (take 3 ranked)]
    (map :title top)))
```

Or with threading macros (if you prefer):

```golisp
(defn top-picks [catalog any watched any] -> []any
  (->> catalog
       (filter (fn [m] (not (contains? watched (:title m)))))
       (sort-by (fn [m] (- (float64 (:rating m)))))
       (take 3)
       (map :title)))
```

`->>` threads each result as the last argument. `->` threads as the first argument.

When the threaded value needs to land in a different position from one step to the next, `as->` binds it to a name you choose:

```golisp
(as-> {} $
  (assoc $ "k" 1)
  (dissoc $ "old")
  (merge $ {"done" true}))
```

Each form may reference `$` anywhere — first arg, last arg, or buried in the middle.

## Nil-Safe and Conditional Threading

`some->` and `some->>` thread like `->`/`->>` but short-circuit to `nil` the
moment any step yields `nil` — they collapse a stack of nil-guards into one line:

```golisp
; Returns nil if any key is missing, instead of panicking on a nil map
(some-> req
        (get "user")
        (get "email")
        str/lower)

(some->> orders
         (filter paid?)
         (map :total)
         (reduce +))
```

`cond->` and `cond->>` thread the value only through the forms whose paired test
is truthy. Unlike `cond`, *every* test is checked, so it is ideal for building a
value up from optional pieces:

```golisp
(defn build-query [base any opts any] -> any
  (cond-> base
    (:active opts)   (assoc :status "active")
    (:since opts)    (assoc :since (:since opts))
    (:limit opts)    (assoc :limit (:limit opts))))
```

`cond->>` is the same but threads as the last argument of each chosen form.

## Calling Function Values

A function stored in an `any`-typed binding — the result of `comp`, `juxt`,
`partial`, or a map lookup — is callable directly, no `apply` needed:

```golisp
(defn run-twice [f any x any] -> any
  (f (f x)))

(run-twice (partial + 3) 10)    ; 16

(let [handlers {:greet (fn [n] (str "hi " n))}]
  ((:greet handlers) "Ada"))    ; "hi Ada"
```

## Bare Functions as Arguments

A single-argument named or core function can be passed straight into a
higher-order function — the compiler wraps it to fit:

```golisp
(map str/upper ["a" "b"])       ; ["A" "B"]
(filter str/blank? lines)
(map :title movies)
```

Multi-argument functions like `+` still need an explicit wrapper in these
positions: `(reduce (fn [a b] (+ a b)) 0 nums)`. If a function's shape doesn't
fit the slot, the compiler reports it at the call site and suggests the wrapper.

## Debugging Pipelines

`pp` pretty-prints a value (sorted map keys, indented nesting) and returns it unchanged, so it drops into any expression without disturbing the result:

```golisp
(pp {:b 2 :a 1})    ; prints the map, returns it
```

`tap->` and `tap->>` are `->`/`->>` that pretty-print each intermediate stage — a pipeline you can watch:

```golisp
(tap-> 5 (+ 3) (* 2))    ; prints 5, then 8, then 16; returns 16
```

`time-it` evaluates an expression, prints how long it took, and returns its value:

```golisp
(time-it (expensive-computation))
```

All three pass their value through untouched, so you can wrap a subexpression to inspect it and remove the wrapper later without changing behavior.
