# ADR-006: Ring-style web server

**Status**: Accepted

## Context

glisp needed a web server abstraction. Options: generate net/http handler code directly, wrap a Go web framework (gin, echo, chi), or adopt the Ring convention from Clojure.

## Decision

The stdlib web layer uses the Ring convention: a handler is a function that takes a request map and returns a response map. Middleware is a function from handler to handler.

```
Handler   = func(req map[string]any) map[string]any
Middleware = func(Handler) Handler
```

## Reasons

- **Pure functions** — handlers are data-in, data-out; they are trivially testable without spinning up an HTTP server
- **Composable** — middleware stacks are built by function composition: `(stdlib/Wrap handler WrapLogging WrapJson WrapAuth)`
- **Familiar to Clojure users** — Ring is the dominant Clojure web convention; the mental model transfers directly
- **No magic** — request and response are plain maps; no custom types, no reflection, no struct tags
- **Framework-independent routing** — the router is just a function; any routing strategy can be implemented as a handler
- **Testability** — `(handler {"method" "GET" "path" "/users"})` — call a handler directly in tests with no HTTP overhead

## Request map keys
```
"method"      — HTTP method string ("GET", "POST", ...)
"path"        — URL path
"query"       — query string
"headers"     — map[string]string
"body"        — raw body string
"params"      — path parameters extracted by router (:id → "id")
"json-body"   — parsed JSON body (set by WrapJson)
"identity"    — Bearer token (set by WrapAuth)
```

## Response map keys
```
"status"   — HTTP status code (int)
"headers"  — map[string]string
"body"     — response body string
```

## Consequences

- All values in request/response maps are `any`; callers must cast (e.g., `(int (get resp "status"))`)
- No streaming responses — body is always a complete string; large file transfers use `serve-files`
- Websocket support would require a different abstraction (hijacking the connection)
