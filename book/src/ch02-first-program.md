# A First Program

Let's build a command-line statistics tool. It reads numbers from arguments and prints count, sum, min, max, and mean. This example touches functions, collections, error handling, and CLI idioms without explaining each piece in depth — those come in later chapters.

Save this as `stats.glsp`:

```golisp
(ns main)

(def default-nums []int [3 1 4 1 5 9 2 6])

(defn parse-arg [s string] -> int
  (if-err [n err] (parse-int s)
    (do (println "not a number:" s) (sys/exit 1) 0)
    n))

(defn main [] -> void
  (let [args (rest (sys/args))]
    (when (contains? args "--help")
      (println "usage: stats [numbers...]")
      (sys/exit 0))
    (let [nums (if (empty? args)
                 default-nums
                 (map (fn [a] (parse-arg (str a))) args))
          sorted (sort nums)
          total  (reduce (fn [acc n] (+ (int acc) (int n))) 0 nums)]
      (when (empty? args) (println "(using defaults)"))
      (println "count:" (len nums))
      (println "sum:  " total)
      (println "min:  " (first sorted))
      (println "max:  " (last sorted))
      (println "mean: " (/ (float64 total) (float64 (len nums)))))))
```

Run it with the defaults:

```bash
glisp run stats.glsp
```

```
(using defaults)
count: 8
sum:   31
min:   1
max:   9
mean:  3.875
```

Run it with your own numbers:

```bash
glisp run stats.glsp 10 20 30 40
```

```
count: 4
sum:   100
min:   10
max:   40
mean:  25
```

## What's happening here

`(sys/args)` returns the command-line arguments (program name first). `rest` drops the program name.

`parse-int` returns two values: the parsed number and an error. `if-err` checks the error — if it's non-nil, the first branch runs; otherwise the second branch binds the result to `n`.

`map`, `filter`, `reduce` are the standard higher-order functions. They work on any collection.

`sort` returns a new sorted slice. `first` and `last` return the first and last elements.

Everything is an expression — `if`, `let`, `when`, function bodies — and the last expression's value is what's returned.

Build a binary and distribute it:

```bash
glisp build stats.glsp
./stats 7 3 8 1 9
```
