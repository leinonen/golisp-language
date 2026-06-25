# Building Web Services

The `golisp/web` package provides a Ring-style HTTP framework: handlers are plain functions, requests and responses are maps.

## Handlers

A handler takes a request and returns a response:

```golisp
(ns main (:import [golisp/web]))

(defn hello [req web/Request] -> web/Response
  (web/json-response 200 {"message" "hello"}))
```

`web/Request` and `web/Response` are `map[string]any` — inspect and build them with standard map operations.

## Routing

`defroutes` defines a `web/Handler` from a list of routes. The verb macros
(`GET`, `POST`, `PUT`, `DELETE`, `PATCH`) turn each body into a handler, the
vector after the path binds named path parameters as locals, and `req` is in
scope throughout. An optional `:middleware` clause wraps the whole chain
(outermost first):

```golisp
(defroutes app
  :middleware
  [web/wrap-logging web/wrap-recover web/wrap-cors web/wrap-json]
  (GET    "/"          [] (home req))
  (GET    "/users"     [] (web/json-response 200 (all-users)))
  (GET    "/users/:id" [id]
    (if-let [u (find-user id)]
      (web/json-response 200 u)
      (web/not-found "user not found")))
  (POST   "/users"     [] (create-user req))
  (DELETE "/users/:id" [id] (delete-user id)))
```

A raw `(web/get path handler)` still composes inside `defroutes`, which is handy
for a route that needs its own per-route middleware:

```golisp
(defroutes app
  (GET "/" [] (home req))
  (web/get "/export" (web/wrap export-handler web/wrap-auth)))
```

Under the hood the DSL expands to `web/routes` plus `web/wrap`. You can write
that lower-level form directly when you don't want path-param binding:

```golisp
(def app web/Handler
  (web/routes
    (web/get  "/"          home)
    (web/post "/users"     create-user)))
```

## Request Helpers

```golisp
(web/path-param  req "id")           ; URL parameter
(web/query-param req "page")         ; ?page=2
(web/header      req "content-type") ; request header
(web/body-map    req)                ; parsed JSON body as map
```

## Response Helpers

```golisp
(web/json-response 200 {:id 1 :name "Alice"})
(web/text-response 200 "plain text")
(web/html-response 200 "<h1>Hello</h1>")
(web/redirect "/login")
(web/no-content)
(web/bad-request    "missing field")
(web/unauthorized   "invalid token")
(web/not-found      "user not found")
(web/server-error   "internal failure")
```

## Middleware

Wrap handlers with middleware using `web/wrap`. Middleware applies right-to-left:

```golisp
(def app web/Handler
  (web/wrap
    (web/routes ...)
    web/wrap-logging    ; logs each request
    web/wrap-recover    ; catches panics
    web/wrap-cors       ; adds CORS headers
    web/wrap-json))     ; parses JSON bodies
```

Write your own middleware:

```golisp
(defn wrap-auth [handler web/Handler] -> web/Handler
  (fn [req web/Request] -> web/Response
    (let [token (web/header req "authorization")]
      (if (valid-token? token)
        (handler (assoc req "identity" (parse-token token)))
        (web/unauthorized "invalid token")))))
```

## HTML with Hiccup

Build HTML from nested vectors:

```golisp
(web/html
  [:html
    [:head [:title "My App"]]
    [:body
      [:h1 {:class "title"} "Hello"]
      [:ul
        (map (fn [item] [:li item]) items)]]])
```

## Starting the Server

```golisp
(defn main [] -> void
  (let [port (sys/env "PORT" "8080")]
    (println "listening on :" port)
    (web/serve-graceful (str ":" port) app)))
```

`web/serve-graceful` handles OS signals (SIGTERM, SIGINT) and drains in-flight requests before shutting down.

## Complete Example: Task API

```golisp
(ns main (:import [golisp/web]))

(def tasks []any
  [{"id" "1" "title" "Buy groceries" "done" false}
   {"id" "2" "title" "Write code"    "done" true}])

(defn create-task [req web/Request] -> web/Response
  (if-let [body (web/body-map req)]
    (let [{title :title :- string} body]
      (if (= title "")
        (web/bad-request "title is required")
        (web/json-response 201 {"id" "new" "title" title "done" false})))
    (web/bad-request "invalid JSON body")))

(defroutes app
  :middleware
  [web/wrap-logging web/wrap-recover web/wrap-json]
  (GET "/tasks" []
    (switch (web/query-param req "done")
      "true"  (web/json-response 200 (filter (fn [t] (get t "done")) tasks))
      "false" (web/json-response 200 (filter (fn [t] (not (get t "done"))) tasks))
      :default (web/json-response 200 tasks)))
  (GET "/tasks/:id" [id]
    (if-let [task (some (fn [t] (if (= (get t "id") id) t nil)) tasks)]
      (web/json-response 200 task)
      (web/not-found (str "task " id " not found"))))
  (POST "/tasks" [] (create-task req)))

(defn main [] -> void
  (println "Tasks API on :4000")
  (web/serve-graceful ":4000" app))
```

## Server-Sent Events and WebSocket

```golisp
; SSE — pass a channel; the framework streams values as events
(defn events [req web/Request] -> web/Response
  (let [ch (chan string 10)]
    (go
      (doseq [i (range 5)]
        (send! ch (str "event " i))
        (time/sleep (* 500 time/Millisecond)))
      (close! ch))
    (web/sse-response ch)))

; WebSocket — handler receives in/out channels
(defn chat [req web/Request] -> web/Response
  (web/websocket
    (fn [req web/Request in (chan string) out (chan string)] -> void
      (for-chan [msg in]
        (send! out (str "echo: " msg))))))
```
