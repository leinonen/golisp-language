// Package web provides Ring-style web server helpers for glisp programs.
// A Ring handler is a function of type func(Request) Response.
// Request and response maps follow the Ring convention:
//
//	Request: {"method": "GET", "path": "/", "headers": {...}, "body": "..."}
//	Response: {"status": 200, "headers": {"Content-Type": "..."}, "body": "..."}
package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"
)

// Request is a Ring-style request map: {"method", "path", "query", "headers", "body"}.
type Request = map[string]any

// Response is a Ring-style response map: {"status", "headers", "body"}.
type Response = map[string]any

// Handler is the type of a Ring-style handler function.
type Handler func(req Request) Response

// Middleware wraps a handler to produce a new handler.
type Middleware func(Handler) Handler

// RingToHTTP adapts a Ring handler into an http.Handler.
func RingToHTTP(h Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := buildRequest(r)
		resp := h(req)
		writeResponse(w, resp)
	})
}

func buildRequest(r *http.Request) Request {
	body := ""
	if r.Body != nil {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}
	headers := map[string]any{}
	for k, vs := range r.Header {
		headers[k] = strings.Join(vs, ", ")
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return map[string]any{
		"method":      r.Method,
		"path":        r.URL.Path,
		"query":       r.URL.RawQuery,
		"headers":     headers,
		"body":        body,
		"remote-addr": r.RemoteAddr,
		"host":        r.Host,
		"scheme":      scheme,
	}
}

func writeResponse(w http.ResponseWriter, resp Response) {
	if resp == nil {
		w.WriteHeader(500)
		return
	}

	// Set headers
	if hdrs, ok := resp["headers"].(map[string]any); ok {
		for k, v := range hdrs {
			w.Header().Set(k, fmt.Sprintf("%v", v))
		}
	}

	// Write status
	w.WriteHeader(statusOf(resp))

	// Write body
	body := ""
	switch b := resp["body"].(type) {
	case string:
		body = b
	case []byte:
		w.Write(b) //nolint:errcheck
		return
	default:
		body = fmt.Sprintf("%v", b)
	}
	w.Write([]byte(body)) //nolint:errcheck
}

// WrapLogging adds simple request logging to a handler.
func WrapLogging(h Handler) Handler {
	return func(req Request) Response {
		method := req["method"]
		path := req["path"]
		resp := h(req)
		fmt.Printf("%s %s → %d\n", method, path, statusOf(resp))
		return resp
	}
}

// statusOf extracts the status code from a response, accepting the numeric
// types a glisp program can produce (int, int64, float64). Defaults to 200.
func statusOf(resp Response) int {
	switch s := resp["status"].(type) {
	case int:
		return s
	case int64:
		return int(s)
	case float64:
		return int(s)
	}
	return 200
}

// WrapRecover catches panics, logs them with a stack trace, and returns a
// generic 500 response (the panic value is not leaked to the client).
func WrapRecover(h Handler) Handler {
	return func(req Request) (resp Response) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("handler panic",
					"error", fmt.Sprint(r),
					"method", req["method"],
					"path", req["path"],
					"stack", string(debug.Stack()))
				resp = map[string]any{"status": 500, "body": "internal error"}
			}
		}()
		return h(req)
	}
}

// JsonResponse creates a Ring-style response with a JSON-encoded body.
// If the body cannot be encoded, logs the error and returns a 500 response.
func JsonResponse(status int, body any) Response {
	b, err := json.Marshal(body)
	if err != nil {
		slog.Error("json-response: encode failed", "error", err)
		return map[string]any{
			"status":  500,
			"headers": map[string]any{"Content-Type": "application/json"},
			"body":    `{"error":"failed to encode response body"}`,
		}
	}
	return map[string]any{
		"status":  status,
		"headers": map[string]any{"Content-Type": "application/json"},
		"body":    string(b),
	}
}

func HtmlResponse(status int, body string) Response {
	return map[string]any{
		"status":  status,
		"headers": map[string]any{"Content-Type": "text/html; charset=utf-8"},
		"body":    body,
	}
}

func TextResponse(status int, body string) Response {
	return map[string]any{
		"status":  status,
		"headers": map[string]any{"Content-Type": "text/plain; charset=utf-8"},
		"body":    body,
	}
}

func Redirect(location string) Response {
	return map[string]any{
		"status":  302,
		"headers": map[string]any{"Location": location},
		"body":    "",
	}
}

// BadRequest creates a 400 JSON response: {"error": msg}.
func BadRequest(msg string) Response {
	return JsonResponse(400, map[string]any{"error": msg})
}

// Unauthorized creates a 401 JSON response: {"error": msg}.
func Unauthorized(msg string) Response {
	return JsonResponse(401, map[string]any{"error": msg})
}

// NotFound creates a 404 JSON response: {"error": msg}.
func NotFound(msg string) Response {
	return JsonResponse(404, map[string]any{"error": msg})
}

// ServerError creates a 500 JSON response: {"error": msg}.
func ServerError(msg string) Response {
	return JsonResponse(500, map[string]any{"error": msg})
}

// NoContent creates an empty 204 response.
func NoContent() Response {
	return map[string]any{"status": 204, "body": ""}
}

// Serve starts an HTTP server on the given address with the given handler.
func Serve(addr string, h Handler) error {
	return http.ListenAndServe(addr, RingToHTTP(h))
}

// Route pairs an HTTP method and URL pattern with a handler.
type Route struct {
	method  string
	pattern string
	handler Handler
}

// Get creates a route that matches GET requests on pattern.
func Get(pattern string, h Handler) Route { return Route{"GET", pattern, h} }

// Post creates a route that matches POST requests on pattern.
func Post(pattern string, h Handler) Route { return Route{"POST", pattern, h} }

// Put creates a route that matches PUT requests on pattern.
func Put(pattern string, h Handler) Route { return Route{"PUT", pattern, h} }

// Delete creates a route that matches DELETE requests on pattern.
func Delete(pattern string, h Handler) Route { return Route{"DELETE", pattern, h} }

// Patch creates a route that matches PATCH requests on pattern.
func Patch(pattern string, h Handler) Route { return Route{"PATCH", pattern, h} }

// Head creates a route that matches HEAD requests on pattern.
func Head(pattern string, h Handler) Route { return Route{"HEAD", pattern, h} }

// Options creates a route that matches OPTIONS requests on pattern.
// Note: WrapCors answers OPTIONS preflight requests before routing, so
// explicit OPTIONS routes only fire when wrap-cors is not in the chain.
func Options(pattern string, h Handler) Route { return Route{"OPTIONS", pattern, h} }

// Context groups routes under a common path prefix. Accepts Route values and
// []Route groups, so contexts nest. glisp usage:
//
//	(web/routes
//	  (web/context "/api/v1"
//	    (web/get "/tasks" list-tasks)
//	    (web/get "/tasks/:id" get-task)))
func Context(prefix string, rs ...any) []Route {
	prefix = strings.TrimSuffix(prefix, "/")
	var out []Route
	for _, r := range flattenRoutes(rs) {
		p := r.pattern
		if p == "/" || p == "" {
			p = ""
		} else if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		pattern := prefix + p
		if pattern == "" {
			pattern = "/"
		}
		out = append(out, Route{r.method, pattern, r.handler})
	}
	return out
}

// flattenRoutes expands a mixed list of Route values and []Route groups.
func flattenRoutes(rs []any) []Route {
	var out []Route
	for _, r := range rs {
		switch v := r.(type) {
		case Route:
			out = append(out, v)
		case []Route:
			out = append(out, v...)
		default:
			panic(fmt.Sprintf("web: expected Route or []Route, got %T", r))
		}
	}
	return out
}

// Routes combines multiple routes into a single Handler. Accepts Route values
// and []Route groups (from Context). Tries each route in order; the first
// matching method+pattern wins. Path params (e.g. :id) are extracted and
// stored in req["params"]; a trailing *name segment captures the rest of the
// path. Returns 405 with an Allow header if the path matches a route but the
// method does not, or 404 if no route matches.
func Routes(routables ...any) Handler {
	rs := flattenRoutes(routables)
	return func(req Request) Response {
		method, _ := req["method"].(string)
		path, _ := req["path"].(string)
		var allowed []string
		seen := map[string]bool{}
		for _, r := range rs {
			params, ok := matchPath(r.pattern, path)
			if !ok {
				continue
			}
			if r.method != method {
				if !seen[r.method] {
					seen[r.method] = true
					allowed = append(allowed, r.method)
				}
				continue
			}
			req["params"] = params
			return r.handler(req)
		}
		if len(allowed) > 0 {
			return map[string]any{
				"status":  405,
				"headers": map[string]any{"Allow": strings.Join(allowed, ", ")},
				"body":    "method not allowed",
			}
		}
		return map[string]any{"status": 404, "body": "not found"}
	}
}

// WrapJson parses the request body as JSON and stores the result in req["json-body"].
// The original string body remains in req["body"]. Passes through on parse failure.
func WrapJson(h Handler) Handler {
	return func(req Request) Response {
		if body, ok := req["body"].(string); ok && body != "" {
			var v any
			if err := json.Unmarshal([]byte(body), &v); err == nil {
				req["json-body"] = v
			}
		}
		return h(req)
	}
}

// WrapCors adds permissive CORS headers to every response and answers
// OPTIONS preflight requests directly with 204.
func WrapCors(h Handler) Handler {
	return func(req Request) Response {
		if method, _ := req["method"].(string); method == "OPTIONS" {
			return addCorsHeaders(map[string]any{"status": 204, "body": ""})
		}
		resp := h(req)
		if resp == nil {
			resp = map[string]any{"status": 200}
		}
		return addCorsHeaders(resp)
	}
}

func addCorsHeaders(resp Response) Response {
	hdrs, ok := resp["headers"].(map[string]any)
	if !ok {
		hdrs = map[string]any{}
		resp["headers"] = hdrs
	}
	hdrs["Access-Control-Allow-Origin"] = "*"
	hdrs["Access-Control-Allow-Methods"] = "GET, POST, PUT, DELETE, PATCH, OPTIONS"
	hdrs["Access-Control-Allow-Headers"] = "Content-Type, Authorization"
	return resp
}

// WrapAuth extracts a Bearer token from the Authorization header and stores it
// in req["identity"]. Returns 401 if the header is absent or not a Bearer token.
func WrapAuth(h Handler) Handler {
	return func(req Request) Response {
		headers, _ := req["headers"].(map[string]any)
		auth, _ := headers["Authorization"].(string)
		if !strings.HasPrefix(auth, "Bearer ") {
			return map[string]any{"status": 401, "body": "unauthorized"}
		}
		req["identity"] = strings.TrimPrefix(auth, "Bearer ")
		return h(req)
	}
}

// WrapAuthFunc is like WrapAuth but validates the Bearer token with check.
// Returns 401 if the header is absent, not a Bearer token, or rejected.
// glisp usage: (web/wrap handler (web/wrap-auth-func (fn [token string] -> bool ...)))
func WrapAuthFunc(check func(token string) bool) Middleware {
	return func(h Handler) Handler {
		return func(req Request) Response {
			headers, _ := req["headers"].(map[string]any)
			auth, _ := headers["Authorization"].(string)
			if !strings.HasPrefix(auth, "Bearer ") {
				return Unauthorized("unauthorized")
			}
			token := strings.TrimPrefix(auth, "Bearer ")
			if !check(token) {
				return Unauthorized("unauthorized")
			}
			req["identity"] = token
			return h(req)
		}
	}
}

// WrapTimeout wraps a handler with a deadline of `seconds` seconds.
// Returns 503 if the handler does not respond in time. Note: Ring handlers
// take no context, so the timed-out handler keeps running in the background;
// its eventual response is discarded.
func WrapTimeout(seconds int) Middleware {
	d := time.Duration(seconds) * time.Second
	return func(h Handler) Handler {
		return func(req Request) Response {
			ch := make(chan Response, 1)
			go func() { ch <- h(req) }()
			select {
			case resp := <-ch:
				return resp
			case <-time.After(d):
				return map[string]any{"status": 503, "body": "timeout"}
			}
		}
	}
}

// Compose chains middlewares into one (left = outermost).
func Compose(mws ...Middleware) Middleware {
	return func(h Handler) Handler {
		for i := len(mws) - 1; i >= 0; i-- {
			h = mws[i](h)
		}
		return h
	}
}

// Wrap applies middlewares (outermost-first) to handler.
// glisp usage: (web/Wrap my-handler web/WrapLogging web/WrapCors)
func Wrap(h Handler, mws ...Middleware) Handler {
	return Compose(mws...)(h)
}

// QueryParam returns the named query parameter from req["query"].
func QueryParam(req Request, name string) string {
	q, _ := req["query"].(string)
	vals, _ := url.ParseQuery(q)
	return vals.Get(name)
}

// PathParam returns the named path parameter from req["params"].
func PathParam(req Request, name string) string {
	params, _ := req["params"].(map[string]any)
	v, _ := params[name].(string)
	return v
}

// BodyMap returns the JSON-decoded body as a map. Checks req["json-body"] first
// (set by WrapJson), then falls back to decoding req["body"].
func BodyMap(req Request) map[string]any {
	if v, ok := req["json-body"].(map[string]any); ok {
		return v
	}
	body, _ := req["body"].(string)
	var m map[string]any
	json.Unmarshal([]byte(body), &m) //nolint:errcheck
	return m
}

// FormParam returns the named field from a urlencoded request body.
func FormParam(req Request, name string) string {
	body, _ := req["body"].(string)
	vals, _ := url.ParseQuery(body)
	return vals.Get(name)
}

// Header returns the named request header from req["headers"].
// The lookup is case-insensitive: headers are stored under Go's canonical
// form (e.g. "Content-Type"), and name is canonicalized before lookup.
func Header(req Request, name string) string {
	headers, _ := req["headers"].(map[string]any)
	if v, ok := headers[name].(string); ok {
		return v
	}
	v, _ := headers[textproto.CanonicalMIMEHeaderKey(name)].(string)
	return v
}

// Cookie returns the value of the named cookie from the Cookie header,
// or "" if absent.
func Cookie(req Request, name string) string {
	cookies, err := http.ParseCookie(Header(req, "Cookie"))
	if err != nil {
		return ""
	}
	for _, c := range cookies {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

// SetCookie adds a Set-Cookie header (path /) to resp and returns resp.
func SetCookie(resp Response, name, value string) Response {
	c := http.Cookie{Name: name, Value: value, Path: "/"}
	hdrs, ok := resp["headers"].(map[string]any)
	if !ok {
		hdrs = map[string]any{}
		resp["headers"] = hdrs
	}
	hdrs["Set-Cookie"] = c.String()
	return resp
}

// ServeFiles returns a Handler that serves static files from dir under prefix.
// Request headers are forwarded, so conditional (If-Modified-Since/ETag) and
// Range requests work.
func ServeFiles(prefix, dir string) Handler {
	fs := http.StripPrefix(prefix, http.FileServer(http.Dir(dir)))
	return func(req Request) Response {
		path, _ := req["path"].(string)
		u, err := url.ParseRequestURI(path)
		if err != nil {
			return map[string]any{"status": 400, "body": "bad request"}
		}
		r := &http.Request{
			Method:     "GET",
			URL:        u,
			Proto:      "HTTP/1.1",
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     http.Header{},
			Host:       "localhost",
		}
		if headers, ok := req["headers"].(map[string]any); ok {
			for k, v := range headers {
				if s, ok := v.(string); ok {
					r.Header.Set(k, s)
				}
			}
		}
		w := httptest.NewRecorder()
		fs.ServeHTTP(w, r)
		result := w.Result()
		hdrs := map[string]any{}
		for k, vs := range result.Header {
			hdrs[k] = strings.Join(vs, ", ")
		}
		body, _ := io.ReadAll(result.Body)
		return map[string]any{
			"status":  result.StatusCode,
			"headers": hdrs,
			"body":    body,
		}
	}
}

// ServeGraceful starts an HTTP server and blocks until SIGINT or SIGTERM,
// then drains in-flight requests with a 5-second shutdown deadline.
func ServeGraceful(addr string, h Handler) {
	srv := &http.Server{Addr: addr, Handler: RingToHTTP(h)}
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("listen error: %v\n", err)
		}
	}()
	<-quit
	fmt.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		fmt.Printf("shutdown error: %v\n", err)
	}
}

// matchPath matches a URL pattern against a concrete path. Segments starting
// with ':' capture the corresponding path segment by name; a trailing segment
// starting with '*' captures the rest of the path (joined with '/'). Captured
// values are URL-decoded. Returns the captured params and true on success, or
// nil and false otherwise.
func matchPath(pattern, path string) (map[string]any, bool) {
	ps := strings.Split(strings.TrimPrefix(pattern, "/"), "/")
	vs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	wildcard := len(ps) > 0 && strings.HasPrefix(ps[len(ps)-1], "*")
	n := len(ps)
	if wildcard {
		n--
		if len(vs) < n {
			return nil, false
		}
	} else if len(ps) != len(vs) {
		return nil, false
	}
	params := map[string]any{}
	for i := 0; i < n; i++ {
		p := ps[i]
		if strings.HasPrefix(p, ":") {
			params[p[1:]] = pathUnescape(vs[i])
		} else if p != vs[i] {
			return nil, false
		}
	}
	if wildcard {
		rest := ""
		if len(vs) > n {
			rest = strings.Join(vs[n:], "/")
		}
		if name := ps[len(ps)-1][1:]; name != "" {
			params[name] = pathUnescape(rest)
		}
	}
	return params, true
}

// pathUnescape URL-decodes a captured path value, returning it unchanged if
// it is not valid percent-encoding.
func pathUnescape(s string) string {
	if u, err := url.PathUnescape(s); err == nil {
		return u
	}
	return s
}
