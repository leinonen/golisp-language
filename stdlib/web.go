// Package stdlib provides Ring-style web server helpers for glisp programs.
// A Ring handler is a function of type func(map[string]any) map[string]any.
// Request and response maps follow the Ring convention:
//
//	Request: {"method": "GET", "path": "/", "headers": {...}, "body": "..."}
//	Response: {"status": 200, "headers": {"Content-Type": "..."}, "body": "..."}
package stdlib

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// Handler is the type of a Ring-style handler function.
type Handler func(req map[string]any) map[string]any

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

func buildRequest(r *http.Request) map[string]any {
	body := ""
	if r.Body != nil {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}
	headers := map[string]any{}
	for k, vs := range r.Header {
		headers[k] = strings.Join(vs, ", ")
	}
	return map[string]any{
		"method":  r.Method,
		"path":    r.URL.Path,
		"query":   r.URL.RawQuery,
		"headers": headers,
		"body":    body,
	}
}

func writeResponse(w http.ResponseWriter, resp map[string]any) {
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
	status := 200
	if s, ok := resp["status"].(int); ok {
		status = s
	} else if s, ok := resp["status"].(int64); ok {
		status = int(s)
	}
	w.WriteHeader(status)

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
	return func(req map[string]any) map[string]any {
		method := req["method"]
		path := req["path"]
		resp := h(req)
		status := 200
		if s, ok := resp["status"].(int); ok {
			status = s
		}
		fmt.Printf("%s %s → %d\n", method, path, status)
		return resp
	}
}

// WrapRecover catches panics and returns a 500 response.
func WrapRecover(h Handler) Handler {
	return func(req map[string]any) (resp map[string]any) {
		defer func() {
			if r := recover(); r != nil {
				resp = map[string]any{
					"status": 500,
					"body":   fmt.Sprintf("internal error: %v", r),
				}
			}
		}()
		return h(req)
	}
}

// JsonResponse creates a Ring-style response with a JSON-encoded body.
func JsonResponse(status int, body any) map[string]any {
	b, _ := json.Marshal(body)
	return map[string]any{
		"status":  status,
		"headers": map[string]any{"Content-Type": "application/json"},
		"body":    string(b),
	}
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

// GET creates a route that matches GET requests on pattern.
func GET(pattern string, h Handler) Route { return Route{"GET", pattern, h} }

// POST creates a route that matches POST requests on pattern.
func POST(pattern string, h Handler) Route { return Route{"POST", pattern, h} }

// PUT creates a route that matches PUT requests on pattern.
func PUT(pattern string, h Handler) Route { return Route{"PUT", pattern, h} }

// DELETE creates a route that matches DELETE requests on pattern.
func DELETE(pattern string, h Handler) Route { return Route{"DELETE", pattern, h} }

// PATCH creates a route that matches PATCH requests on pattern.
func PATCH(pattern string, h Handler) Route { return Route{"PATCH", pattern, h} }

// Routes combines multiple routes into a single Handler. Tries each route in
// order; the first matching method+pattern wins. Path params (e.g. :id) are
// extracted and stored in req["params"]. Returns 404 if no route matches.
func Routes(rs ...Route) Handler {
	return func(req map[string]any) map[string]any {
		method, _ := req["method"].(string)
		path, _ := req["path"].(string)
		for _, r := range rs {
			if r.method != method {
				continue
			}
			params, ok := matchPath(r.pattern, path)
			if !ok {
				continue
			}
			req["params"] = params
			return r.handler(req)
		}
		return map[string]any{"status": 404, "body": "not found"}
	}
}

// WrapJson parses the request body as JSON and stores the result in req["json-body"].
// The original string body remains in req["body"]. Passes through on parse failure.
func WrapJson(h Handler) Handler {
	return func(req map[string]any) map[string]any {
		if body, ok := req["body"].(string); ok && body != "" {
			var v any
			if err := json.Unmarshal([]byte(body), &v); err == nil {
				req["json-body"] = v
			}
		}
		return h(req)
	}
}

// WrapCors adds permissive CORS headers to every response.
func WrapCors(h Handler) Handler {
	return func(req map[string]any) map[string]any {
		resp := h(req)
		if resp == nil {
			resp = map[string]any{"status": 200}
		}
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
}

// WrapAuth extracts a Bearer token from the Authorization header and stores it
// in req["identity"]. Returns 401 if the header is absent or not a Bearer token.
func WrapAuth(h Handler) Handler {
	return func(req map[string]any) map[string]any {
		headers, _ := req["headers"].(map[string]any)
		auth, _ := headers["Authorization"].(string)
		if !strings.HasPrefix(auth, "Bearer ") {
			return map[string]any{"status": 401, "body": "unauthorized"}
		}
		req["identity"] = strings.TrimPrefix(auth, "Bearer ")
		return h(req)
	}
}

// WrapTimeout wraps a handler with a deadline of `seconds` seconds.
// Returns 503 if the handler does not respond in time.
func WrapTimeout(seconds int) Middleware {
	d := time.Duration(seconds) * time.Second
	return func(h Handler) Handler {
		return func(req map[string]any) map[string]any {
			ch := make(chan map[string]any, 1)
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
// glisp usage: (stdlib/Wrap my-handler stdlib/WrapLogging stdlib/WrapCors)
func Wrap(h Handler, mws ...Middleware) Handler {
	return Compose(mws...)(h)
}

// QueryParam returns the named query parameter from req["query"].
func QueryParam(req map[string]any, name string) string {
	q, _ := req["query"].(string)
	vals, _ := url.ParseQuery(q)
	return vals.Get(name)
}

// PathParam returns the named path parameter from req["params"].
func PathParam(req map[string]any, name string) string {
	params, _ := req["params"].(map[string]any)
	v, _ := params[name].(string)
	return v
}

// BodyMap returns the JSON-decoded body as a map. Checks req["json-body"] first
// (set by WrapJson), then falls back to decoding req["body"].
func BodyMap(req map[string]any) map[string]any {
	if v, ok := req["json-body"].(map[string]any); ok {
		return v
	}
	body, _ := req["body"].(string)
	var m map[string]any
	json.Unmarshal([]byte(body), &m) //nolint:errcheck
	return m
}

// Header returns the named request header from req["headers"].
func Header(req map[string]any, name string) string {
	headers, _ := req["headers"].(map[string]any)
	v, _ := headers[name].(string)
	return v
}

// ServeFiles returns a Handler that serves static files from dir under prefix.
func ServeFiles(prefix, dir string) Handler {
	fs := http.StripPrefix(prefix, http.FileServer(http.Dir(dir)))
	return func(req map[string]any) map[string]any {
		path, _ := req["path"].(string)
		r := httptest.NewRequest("GET", path, nil)
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
// with ':' are wildcards that capture the corresponding path segment by name.
// Returns the captured params and true on success, or nil and false otherwise.
func matchPath(pattern, path string) (map[string]any, bool) {
	ps := strings.Split(strings.TrimPrefix(pattern, "/"), "/")
	vs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(ps) != len(vs) {
		return nil, false
	}
	params := map[string]any{}
	for i, p := range ps {
		if strings.HasPrefix(p, ":") {
			params[p[1:]] = vs[i]
		} else if p != vs[i] {
			return nil, false
		}
	}
	return params, true
}
