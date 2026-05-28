// Package stdlib provides Ring-style web server helpers for glisp programs.
// A Ring handler is a function of type func(map[string]any) map[string]any.
// Request and response maps follow the Ring convention:
//
//	Request: {"method": "GET", "path": "/", "headers": {...}, "body": "..."}
//	Response: {"status": 200, "headers": {"Content-Type": "..."}, "body": "..."}
package stdlib

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
