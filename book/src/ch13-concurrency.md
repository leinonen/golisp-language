# Concurrency

GoLisp exposes Go's concurrency primitives directly: goroutines, channels, select, and synchronization.

## Goroutines

`go` spawns a goroutine:

```golisp
(go (println "running concurrently"))
```

`go-val` spawns a goroutine and returns a typed channel — the future pattern:

```golisp
(defn fetch [url string] -> (chan string)
  (go-val string
    (if-err [resp err] (http/get url)
      (str "error: " err)
      (:body resp))))

; Submit work
(let [ch (fetch "https://example.com")]
  ; ... do other things ...
  (let [result (recv! ch)]
    (println result)))
```

## Channels

Create a channel with `chan`:

```golisp
(chan string)      ; unbuffered string channel
(chan int 10)      ; buffered int channel with capacity 10
(chan any)         ; untyped channel
```

Send and receive:

```golisp
(send! ch "hello")           ; send (blocks if unbuffered and no receiver)
(recv! ch)                   ; receive (blocks until value available)
(let [[val ok] (recv-ok! ch)] ; receive with close detection
  (if ok (use val) (println "channel closed")))
(close! ch)                  ; close the channel
```

Range over a channel until it closes:

```golisp
(for-chan [msg ch]
  (println "received:" msg))
```

## select!

`select!` waits on multiple channel operations:

```golisp
(select!
  ([val ch1] (println "from ch1:" val))
  ([val ch2] (println "from ch2:" val))
  (:timeout 5000 (println "timed out after 5s"))
  (:default  (println "nothing ready")))
```

`:timeout` fires after N milliseconds. `:default` fires immediately if nothing else is ready.

## par — Parallel Execution

`par` runs expressions concurrently and waits for all to finish:

```golisp
(par
  (seed-database!)
  (warm-cache!)
  (start-metrics!))
```

Each expression runs in its own goroutine. `par` blocks until all complete.

## Atoms

An atom is a thread-safe holder for a single value. `swap!` updates it by applying a function, holding an internal lock so concurrent updates never conflict:

```golisp
(def counter (atom 0))

(swap! counter (fn [n] (+ n 1)))   ; apply f atomically, returns the new value
(reset! counter 0)                 ; set unconditionally
(deref counter)                    ; read the current value
```

A typed atom records its element type. `deref` then returns a concrete value — no cast needed — and `(Atom int)` documents the type in signatures:

```golisp
(defn make-counter [] -> (Atom int)
  (atom int 0))

(defn increment! [c (Atom int)] -> any
  (swap! c (fn [n] (+ (int n) 1))))

(defn current [c (Atom int)] -> int
  (deref c))    ; already an int
```

Atoms are the simplest way to share mutable state across goroutines — each spawns work, all `swap!` the same atom, and the final `deref` sees every update:

```golisp
(let [counter (atom int 0)
      done (chan bool 5)]
  (doseq [_ (range 5)]
    (go
      (doseq [_ (range 1000)] (increment! counter))
      (send! done true)))
  (doseq [_ (range 5)] (recv! done))
  (println "total:" (current counter)))    ; 5000, no data races
```

## with-open

`with-open` binds resources, runs the body, and closes each one on the way out — even if the body panics. Anything with a `Close` method is closed:

```golisp
(with-open [f (open-file "notes.txt")]
  (read-contents f))    ; f is closed before with-open returns
```

Multiple bindings open left-to-right and close in reverse (LIFO), so a resource is always released before the ones it depends on:

```golisp
(with-open [in  (open-file src)
            out (open-file dst)]
  (copy-file in out))   ; out closes first, then in
```

## defer

`defer` schedules cleanup to run when the enclosing function returns:

```golisp
(defn process-file [path string] -> error
  (if-err [f err] (os/open path)
    err
    (do
      (defer (.Close f))
      ; ... use f ...
      nil)))
```

## Practical Example: Concurrent Job Runner

```golisp
(ns main)

(defn run-job [name string delay time/Duration] -> string
  (time/sleep delay)
  (str "done:" name))

(defn submit [name string delay time/Duration] -> (chan string)
  (go-val string (run-job name delay)))

(defn main [] -> void
  (let [jobs [["fetch"  (* 50 time/Millisecond)]
              ["parse"  (* 30 time/Millisecond)]
              ["store"  (* 80 time/Millisecond)]]
        futures (map (fn [[name delay]] (submit name delay)) jobs)
        out (chan string (len futures))]
    ; Collect results with a 200ms timeout per job
    (doseq [ch futures]
      (select!
        ([result ch]
          (send! out result))
        (:timeout 200
          (send! out "timed out"))))
    (close! out)
    (for-chan [r out]
      (println r))))
```

## WaitGroup Pattern

For fire-and-forget goroutines that must all finish before continuing:

```golisp
(defn main [] -> void
  (let [wg (sync/WaitGroup. {})]
    (doseq [i (range 5)]
      (.Add wg 1)
      (go
        (defer (.Done wg))
        (println "worker" i)))
    (.Wait wg)
    (println "all done")))
```

## Pipelines: fan-in, pipeline, fan-out

Three forms compose channels into concurrent dataflows. `fan-in` merges several
channels into one. `pipeline` chains transformation stages, each running in its own
goroutine connected by internal channels — `[n merged]` binds each value from the
source, and every stage rebinds the symbol to its own result. `fan-out` runs `n`
worker goroutines that drain a channel in parallel:

```golisp
(defn emit [vals []any] -> (chan any)
  (let [ch (chan any 8)]
    (go (doseq [v vals] (send! ch v)) (close! ch))
    ch))

(defn main [] -> void
  (let [merged  (fan-in (emit [1 2 3]) (emit [10 20 30]))   ; one stream
        doubled (pipeline [n merged] (* (int n) 2))         ; staged transform
        results (chan any 16)]
    (go
      (fan-out 3 [n doubled]            ; 3 workers drain `doubled`
        (send! results (str "got " n)))
      (close! results))                 ; runs once all workers finish
    (for-chan [r results] (println (str r)))))
```

`fan-out` blocks until every worker is done, so wrapping it in `(go …)` lets the
main goroutine drain `results` while the workers run. Channel values are `any`, so
coerce them (`(int n)`) before arithmetic.
