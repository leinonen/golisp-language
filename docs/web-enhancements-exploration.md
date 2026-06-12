# Exploration: web enhancements — hiccup, htmx, SSE, websockets

**Status**: Exploration, with working prototypes committed on this branch
(`web/html.go`, `web/sse.go`, `web/ws.go`, plus a small hook in
`RingToHTTP`). Everything below was validated hands-on against a freshly
built `glisp`: the htmx flow with curl, SSE with curl (including client
disconnect), websockets against an independent client implementation
(`coder/websocket`), and the renderer/stream/frame logic with Go unit tests
(`web/html_test.go`, `web/sse_test.go`, `web/ws_test.go`).
**Date**: 2026-06-12

## 0. Headline finding

All four enhancements fit entirely in the plain-Go `web` package — **zero
transpiler, parser, or formatter changes**. This falls out of two existing
language decisions:

- **Keywords in collection literals emit as plain strings** (`:div` →
  `"div"`), and vectors/maps are `[]any`/`map[string]any` — so a hiccup tree
  like `[:div {:class "card"} [:p "hi"]]` is *already* well-formed glisp data
  that arrives in Go as `[]any{"div", map[string]any{...}, []any{"p", "hi"}}`.
  Even htmx attribute keywords (`:hx-post`) and hiccup tag shorthand
  (`:div#main.card`) lex without complaint.
- **Channels are first-class** (`chan`, `send!`, `recv!`, `for-chan`,
  `select!`, `go`) — so streaming (SSE) and bidirectional messaging
  (websockets) map naturally onto `chan any` values that the web adapter
  bridges to the wire.

The one structural gap was the Ring adapter itself: `RingToHTTP` called the
handler, took the complete response map, and wrote it — no streaming, no
flushing, no connection hijack, no disconnect signal. The prototypes close
that gap with *special response values* (an escape-hatch pattern, §3.1)
rather than a new handler type, so the Ring model — handlers are pure
`Request → Response` functions — survives untouched.

## 1. Hiccup-style HTML rendering (`web/html.go`)

### 1.1 What exists

```clojure
(defn todo-item [todo any] -> any
  [:li.todo {:id (format "todo-%v" (:id todo))}
   [:span (:title todo)]
   [:button {:hx-delete (format "/todos/%v" (:id todo))
             :hx-target "closest li"
             :hx-swap   "outerHTML"} "x"]])

(web/render-response 200
  [:html
   [:head [:script {:src "https://unpkg.com/htmx.org@2.0.4"}]]
   [:body
    [:h1#title "Todos"]
    [:ul.list (map (fn [t] (todo-item t)) todos)]]])
```

| glisp value | Renders as |
|---|---|
| string | escaped text (`"a < b"` → `a &lt; b`) |
| `(web/raw s)` | unescaped markup (trusted content only) |
| `nil` | nothing |
| number / bool | formatted text |
| `[:tag ...]` (first element a string) | element; optional second-position attrs map |
| `[[:li "a"] [:li "b"]]` (first element not a string) | sequence splice — `map` output drops straight in |
| `:div#main.card.wide` tag | id/class shorthand, merged with any `:class` attr |
| attr value `true` / `false` / `nil` | bare attribute / omitted / omitted |
| void elements (`br`, `img`, `input`, …) | no closing tag |

Public API: `(web/html node)` → string, `(web/html-page node)` →
`<!DOCTYPE html>` + string, `(web/render-response status node)` → Response,
`(web/raw s)`. Attributes render in sorted order so output is deterministic
and testable. Escaping is on by default everywhere — interpolating user data
into `format`-built HTML strings (the only option before) was a standing XSS
hazard.

### 1.2 Why the renderer is Go-side, not a glisp library

A glisp implementation would need type dispatch on nodes, and the language
deliberately has no `string?` / `map?` / `vector?` predicates. It would also
re-walk `any` values through `_glispToSlice` at every level. The 190-line Go
renderer matches the existing design rule: *all web functionality lives in
`web/web.go` as plain Go*.

### 1.3 Friction found

- A `defn` can't be passed bare to `map` — `(map todo-item todos)` fails
  when `todo-item` has typed params, so child lists need
  `(map (fn [t] (todo-item t)) todos)`. *Update*: during this exploration
  the failure was a runtime interface-conversion panic; PR #31 (now merged)
  turned it into a position-tagged transpile error naming the lambda fix.
  The lambda is still required — only the failure mode improved.
- *Mitigation since PR #31*: keywords as functions —
  `(map :title todos)` — make pure-projection child lists lambda-free
  (verified against the merged tree, including inside a hiccup tree).
- Naming: `web/html-response` (takes a *string*) already exists;
  the node-tree variant is `web/render-response` to avoid a breaking change.

## 2. htmx

### 2.1 Already works — verified end-to-end

htmx needs nothing from the language: it is plain HTML attributes plus
fragment-returning endpoints. A counter app (page serve → `hx-post` →
fragment swap) built and ran with current `main`-era features:

```
--- GET /:        <!DOCTYPE html>...<button hx-post="/inc" hx-target="#count" ...>
--- POST /inc:    <span id="count">1</span>   (then 2, 3, …)
--- HX-Request:   (web/header req "HX-Request") → "true"   ✓ detected
```

The *entire* friction was hand-assembling HTML with `str`/`format` and
escaped quotes, with no escaping of interpolated data. Hiccup (§1) is the
htmx enabler; nothing else is required.

### 2.2 Small candidate helpers (not prototyped — sugar only)

- `(web/hx-request? req)` → bool — `HX-Request` header check, for routes
  that serve both full pages and fragments.
- `(web/hx-trigger resp event)` / `(web/hx-redirect url)` /
  `(web/hx-refresh)` — `HX-*` response-header setters.
- Embedding `htmx.min.js` in the `web` package (`//go:embed`) so
  `(web/get "/htmx.js" web/htmx-js)` works offline — fits the
  "single static binary" pitch (~50 kB). Left open; CDN works today.

### 2.3 Friction found

`(swap! counter inc)` fails — built-ins like `inc` are not first-class
values (`undefined: inc`). The workaround is `(fn [n] (+ (int n) 1))`.
Re-verified after PR #31: the new HOF gate diagnoses typed *`defn`s* at
transpile time, but a bare built-in still falls through to the raw Go
`undefined: inc` error; worth a position-tagged diagnostic of its own.

## 3. Server-sent events (`web/sse.go`)

### 3.1 The blocker, and the escape-hatch pattern

A Ring handler returns a finished response map; `writeResponse` writes it
once. SSE needs to hold the connection open and flush per event. The
prototype keeps the handler signature and instead lets the *adapter*
recognize a special response: `(web/sse-response ch)` returns
`{"status" 200 "sse" ch}`, and `RingToHTTP` — the only place that holds the
real `http.ResponseWriter`/`*http.Request` — streams from the channel:

```clojure
(defn handle-ticks [req web/Request] -> web/Response
  (let [events (chan any 8)]
    (go (loop [n 0]
          (if (< n 5)
            (do (send! events {:event "tick" :id n :data {:n n}})
                (recur (+ n 1)))
            (close! events))))
    (web/sse-response events)))
```

Event mapping: a string becomes a `data:` line; a map may carry `"event"`,
`"id"`, `"retry"`, `"data"` (non-string data JSON-encoded, multi-line data
split into multiple `data:` lines per the SSE spec). The stream ends when
the channel closes or the client disconnects. Verified with curl:

```
Content-Type: text/event-stream
event: tick
id: 0
data: {"n":0}
...
```

Named events make this compose directly with htmx's `sse` extension
(`sse-connect` + `sse-swap="tick"`).

### 3.2 Client disconnect: `(web/done req)`

Producers must find out when the client goes away or they leak goroutines.
`(web/done req)` returns a `chan any` that closes when the request context
is cancelled. A producer races it with `select!`:

```clojure
(let [done (web/done req)]
  (web/go-recover
    (fn []
      (do
        (defer (close! events))
        (loop [n 0]
          (select!
            ([_ done] (log/info "client gone"))
            ([(send! events (format "beat %d" n))]
              (do ... (recur (+ n 1))))))))))
```

Verified: killing curl mid-stream fired the `done` case and the producer
goroutine exited ("client gone at 4" in the server log).

**Promotion decisions** (the prototype's eager `req["done"]` bridge spawned
a goroutine on *every* request):

- `web/done` is **lazy** — `RingToHTTP` stashes the raw request context
  under the private `"__context"` key, and the bridge goroutine is created
  on first call, cached in `req["done"]`. Only handlers that ask for it pay
  for it; outside the adapter (handler invoked directly in a test) the
  channel simply never closes. Bonus: the Go signature returns a concrete
  `chan any`, so the old `(as (chan any) (get req "done"))` cast is gone.
- **`web/go-recover`** is the goroutine analog of `wrap-recover`: a panic
  in a plain `(go ...)` producer kills the whole process; under
  `(web/go-recover (fn [] ...))` it is logged (with the `.glsp`-mapped
  frame) and contained. Pairing it with `(defer (close! ch))` ends the
  stream cleanly even on panic — verified end-to-end: the `/boom` endpoint
  panicked mid-stream, the stream closed, and the server kept serving.
- **Idle keepalive**: `streamSse` writes a `: keepalive` comment every 15 s
  while no events flow, so proxies/load balancers don't drop quiet
  streams. A `"keepalive"` key on the response (seconds) overrides; `0`
  disables.
- A producer that ignores `done` and sends on an unbuffered channel still
  blocks forever after a disconnect — the adapter cannot drain on its
  behalf (draining an infinite producer would spin forever). The
  `select!`-on-`done` pattern above is the documented contract (or
  buffered channels + finite streams).
- Middleware interplay: response headers set by middleware are merged into
  the stream's initial headers; `wrap-timeout` is harmless (the handler
  returns immediately — the deadline doesn't govern the stream itself).

Promoting the producer pattern also surfaced one more ADR-011 gap, fixed
alongside: `panic` in the tail of a value-returning fn emitted
`return panic(...)` — invalid Go. It now emits a bare `panic(...)`
statement (fn tails, `do` tails, and loop tails).

## 4. Websockets (`web/ws.go`)

### 4.1 Dependency or not

Go's stdlib has no websocket package. Options considered:

- **`golang.org/x/net/websocket`** — effectively deprecated, no ping/pong
  keepalive support. Rejected.
- **`gorilla/websocket` / `coder/websocket`** — correct and maintained, but
  would put a third-party dependency in `golisp`'s own `go.mod`, inherited
  by every glisp web binary.
- **Minimal in-tree RFC 6455** (prototyped) — ~230 lines: handshake
  (`Sec-WebSocket-Accept`), frame codec (7/16/64-bit lengths), client-mask
  enforcement, text messages, fragmentation reassembly, ping→pong, close
  negotiation. Out of scope: `permessage-deflate`, subprotocols, UTF-8
  validation, binary-message delivery (binary frames are read and dropped).

The prototype takes the dependency-free path, consistent with the rest of
`web/`. If real-world use demands compression or stricter spec conformance,
swapping the internals for `coder/websocket` later does not change the glisp
API.

### 4.2 The glisp API: channels in, channels out

```clojure
(web/get "/echo"
  (web/websocket
    (fn [req web/Request in (chan any) out (chan any)] -> any
      (do
        (send! out "welcome")
        (for-chan [msg in]
          (send! out (str "echo: " msg)))))))
```

Inbound text messages arrive on `in` (closed on disconnect); anything sent
on `out` becomes a text frame; when the handler returns, the adapter sends a
close frame. `for-chan` makes the read loop a one-liner, and routes compose
normally — path params (`/ws/:room`), middleware, `web/context` all apply,
because `web/websocket` is just a `Handler` returning a `{"websocket" h}`
escape-hatch response that the adapter upgrades (via `http.Hijacker`).

### 4.3 Validation

Against an **independent client** (`github.com/coder/websocket v1.8.14`)
talking to the glisp echo server above:

```
recv: "welcome"
recv: "echo: hello from coder/websocket"
recv big: len=70006 ...            ← 64-bit length frames round-trip
ping: ok                           ← ping/pong
close: <nil>                       ← clean close handshake
```

Plus raw-frame unit tests: handshake `Sec-WebSocket-Accept` value, masked
client frames, pong payload echo, 400 on a non-upgrade request, and
connection drop on an unmasked client frame (RFC 6455 §5.1 MUST).

## 5. Verified transpiler defects surfaced by this exploration

Writing SSE/websocket producer code is exactly `select!`-inside-`loop`
shaped, and it tripped four pre-existing emission bugs (all reproduced
minimally against current `main`; none are caused by the prototypes). All
violate "the user never debugs generated Go" (ADR-012 rule 3) — each emits
invalid Go with a raw Go error (line-mapped, but Go-worded):

1. **`select!` in `loop` tail position** emits `_loopN = select { … }` —
   `select` is a statement. The statement-only-tails rule (ADR-011) covers
   *function* tails but not *loop* tails.
   ```clojure
   (loop [n 0] (select! ([v done] ...) ([(send! ch n)] (recur (+ n 1)))))
   ; → expected operand, found 'select'
   ```
   Workaround: a trailing `nil` after the `select!`.
2. **`_` binding in a `select!` recv case** emits `case _ := <-ch:` —
   "no new variables on left side of :=". The documented
   `([_ done] body)` pattern cannot compile; should emit `case <-ch:`.
   Workaround: bind a name and use it.
3. **Bare `nil` as a `select!` case body** emits a `nil` statement —
   "nil is not used". Workaround: any void expression, e.g. `(print "")`.
4. **Statement-only forms as `if` branches in a loop tail**:
   `(if cond (do ... (recur ...)) (close! ch))` emits `close(ch)` in value
   position — "close(ch) (no value) used as value". Same ADR-011 family as
   item 1. Workaround: `(do (close! ch) nil)`.

Also surfaced, since fixed on `main` (PR #31): typed `defn`s passed to HOFs
— a runtime panic during this exploration, now a position-tagged transpile
error. Still open: built-ins are not values (`(swap! a inc)` →
`undefined: inc`).

> **Status update (post-merge of PR #31)**: items 1–4 above were re-run
> against `main` after the keyword-fns/HOF-gate/dotimes fixes landed — all
> four still reproduced at that point.
>
> **Status update 2 — all four fixed (P1 done)**: statement-only forms in a
> loop tail (incl. as `if`/`cond` branches) now emit the statement plus
> `break`/`return nil`, matching the fn-tail rule (`emitLoopTailNode`,
> `emit_loop.go`); a `_` recv binding emits `case <-ch:`
> (`emitSelectStmt`, `emit_concurrency.go`); bare scalar literals in
> statement position are skipped (`emitStmtNode`, `transpiler.go`). The
> §3.1/§3.2 producer code now compiles **as originally written** — no
> trailing `nil`, no named dummy bindings — verified end-to-end with curl
> (stream + disconnect). Regression coverage: snippet tests +
> `testdata/select_loop.glsp` golden (vetted by `TestGoldenCompiles`).

## 6. What deliberately stays the same

- **The Ring model.** Handlers remain pure `Request → Response` map
  functions. Streaming and upgrades are *response values* the adapter
  interprets, not new handler types or transpiler forms — `web/Handler`,
  middleware, and `web/routes` are untouched.
- **No new special forms, no parser/formatter work.** Hiccup trees are
  ordinary vector/map literals; `glisp fmt` already formats them.
- **htmx stays a library-free pattern** — attributes + fragments; only the
  optional helpers in §2.2 are on the table.
- **Escaping by default**, with `web/raw` as the single explicit opt-out.

## 7. Remaining work to ship, suggested sequencing

| Step | Scope | Risk | Notes |
|---|---|---|---|
| P1 Fix the §5 bug cluster (select!/loop-tail emission) | transpiler, isolated | low | ✅ done — hits exactly the code style SSE/WS demand |
| P2 Hiccup: promote prototype (CLAUDE.md web-API entry, example app) | docs + examples | low | ✅ done — promoted in CLAUDE.md; reference app `examples/todos` (hiccup + htmx) |
| P3 SSE: decide `req["done"]` vs lazy `(web/done req)`; document the leak/panic caveats | `web/` | low | ✅ done — lazy `(web/done req)` (no cast needed), `web/go-recover` for producer panics, idle keepalive comments; promoted in CLAUDE.md |
| P4 Websockets: harden (UTF-8 validation, close-code pass-through, write deadlines, max-frame config) or swap internals for `coder/websocket` | `web/ws.go` | medium | glisp API stable either way |
| P5 htmx sugar (`hx-request?`, `HX-*` setters, embedded htmx.js) | `web/` | low | optional; decide after a real example app |
| P6 Example app (`examples/todos`: hiccup + htmx + SSE ticker + WS chat) | examples | low | doubles as regression surface for P1 |
