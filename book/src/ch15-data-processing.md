# Data Processing

GoLisp's collection functions handle most transformations directly. When the
pipeline matters — because you want to compose steps, avoid intermediate
collections, or stream a file too large for memory — reach for transducers and the
streaming readers. All of it is in `core`, no imports.

## Transducers

Called with a single argument, `map`, `filter`, `remove`, `keep`, `take`, `drop`,
`take-while`, and `drop-while` return a **transducer** — a transformation with no
collection attached. `comp` composes them (data flows left to right), and you run
the result with `sequence`, `transduce`, or `into`:

```golisp
(def xf (comp (map (fn [x] (* x x)))
              (filter (fn [x] (> x 5)))
              (take 2)))

(sequence xf (range 1 1000000))           ; [9 16] — realize to a vector
(transduce xf (fn [a x] (+ a x)) 0 nums)  ; fold to a single value
(into [] (map (fn [x] (+ x 1))) [1 2 3])  ; [2 3 4] — pour into a collection
```

`take` and `take-while` short-circuit: the `(range 1 1000000)` above stops after
the first two passing elements, so the source is never fully walked. The same
`xf` works on any source — that reuse is the point of separating the transformation
from the data.

## CSV

`csv/parse` reads CSV text into a list of maps, keyed by the header row;
`csv/write` does the reverse. Both return `[value error]`, so pair them with
`if-err`:

```golisp
(spit "people.csv" "name,age\nalice,34\nbob,17\n")

(if-err [content err] (slurp "people.csv")
  (println "read failed:" err)
  (if-err [rows perr] (csv/parse content)
    (println "parse failed:" perr)
    (let [adults (filter (fn [r] (>= (int (:age r)) 18)) rows)]
      (if-err [out werr] (csv/write adults)
        (println "write failed:" werr)
        (spit "adults.csv" out)))))
```

Each row is a `map[string]any`, so keyword access (`(:age r)`) and all the map
functions work on it. `csv/write` takes the header from the first row's keys
(sorted).

## Streaming Large Inputs

`read-lines` returns a whole file's lines. When the file is too big to hold in
memory, `transduce-lines` streams it through a transducer pipeline in **constant
memory** — and because `take`/`take-while` stop early, it reads only as far as it
needs:

```golisp
; pull the first 100 ERROR lines out of an arbitrarily large log
(if-err [errs e] (transduce-lines
                   (comp (filter (fn [l] (str/includes? l "ERROR")))
                         (take 100))
                   (fn [acc l] (conj acc l)) [] "app.log")
  (println "read failed:" e)
  (spit "errors.log" (str/join "\n" errs)))
```

`transduce-json` does the same for the elements of a top-level JSON array,
streaming one element at a time instead of decoding the whole document:

```golisp
(if-err [top e] (transduce-json
                  (comp (filter (fn [o] (> (:score o) 90)))
                        (take 10))
                  (fn [acc o] (conj acc o)) [] "events.json")
  (println "read failed:" e)
  (spit "top.json" (json/encode top)))
```

The shape is always the same — a transducer describing *what* to keep and a
reducing function describing *how* to accumulate — whether the source is a vector
in memory, a CSV file, a log, or a JSON array streamed off disk.
