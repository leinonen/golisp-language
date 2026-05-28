package stdlib

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestMatchPath_exact(t *testing.T) {
	params, ok := matchPath("/users", "/users")
	if !ok {
		t.Fatal("expected match")
	}
	if len(params) != 0 {
		t.Fatalf("expected no params, got %v", params)
	}
}

func TestMatchPath_param(t *testing.T) {
	params, ok := matchPath("/users/:id", "/users/42")
	if !ok {
		t.Fatal("expected match")
	}
	if params["id"] != "42" {
		t.Fatalf("expected id=42, got %v", params["id"])
	}
}

func TestMatchPath_multiParam(t *testing.T) {
	params, ok := matchPath("/orgs/:org/repos/:repo", "/orgs/acme/repos/widget")
	if !ok {
		t.Fatal("expected match")
	}
	if params["org"] != "acme" || params["repo"] != "widget" {
		t.Fatalf("unexpected params: %v", params)
	}
}

func TestMatchPath_lengthMismatch(t *testing.T) {
	_, ok := matchPath("/users/:id", "/users/42/extra")
	if ok {
		t.Fatal("expected no match")
	}
}

func TestMatchPath_literalMismatch(t *testing.T) {
	_, ok := matchPath("/users", "/posts")
	if ok {
		t.Fatal("expected no match")
	}
}

func TestMatchPath_root(t *testing.T) {
	params, ok := matchPath("/", "/")
	if !ok {
		t.Fatal("expected match for root")
	}
	if len(params) != 0 {
		t.Fatalf("expected no params, got %v", params)
	}
}

func TestRoutes_dispatch(t *testing.T) {
	called := ""
	app := Routes(
		GET("/", func(req map[string]any) map[string]any {
			called = "home"
			return map[string]any{"status": 200}
		}),
		POST("/items", func(req map[string]any) map[string]any {
			called = "create"
			return map[string]any{"status": 201}
		}),
	)

	resp := app(map[string]any{"method": "GET", "path": "/"})
	if called != "home" {
		t.Fatalf("expected home handler, got %q", called)
	}
	if resp["status"] != 200 {
		t.Fatalf("expected 200, got %v", resp["status"])
	}

	resp = app(map[string]any{"method": "POST", "path": "/items"})
	if called != "create" {
		t.Fatalf("expected create handler, got %q", called)
	}
	if resp["status"] != 201 {
		t.Fatalf("expected 201, got %v", resp["status"])
	}
}

func TestRoutes_methodMismatch(t *testing.T) {
	app := Routes(
		GET("/items", func(req map[string]any) map[string]any {
			return map[string]any{"status": 200}
		}),
	)
	resp := app(map[string]any{"method": "POST", "path": "/items"})
	if resp["status"] != 404 {
		t.Fatalf("expected 404, got %v", resp["status"])
	}
}

func TestRoutes_noMatch(t *testing.T) {
	app := Routes(
		GET("/home", func(req map[string]any) map[string]any {
			return map[string]any{"status": 200}
		}),
	)
	resp := app(map[string]any{"method": "GET", "path": "/other"})
	if resp["status"] != 404 {
		t.Fatalf("expected 404, got %v", resp["status"])
	}
}

func TestRoutes_pathParams(t *testing.T) {
	var gotParams map[string]any
	app := Routes(
		GET("/users/:id", func(req map[string]any) map[string]any {
			gotParams, _ = req["params"].(map[string]any)
			return map[string]any{"status": 200}
		}),
	)
	app(map[string]any{"method": "GET", "path": "/users/99"})
	if gotParams["id"] != "99" {
		t.Fatalf("expected id=99, got %v", gotParams["id"])
	}
}

func TestWrapJson_parsesBody(t *testing.T) {
	var got any
	h := WrapJson(func(req map[string]any) map[string]any {
		got = req["json-body"]
		return map[string]any{"status": 200}
	})
	h(map[string]any{"body": `{"name":"alice"}`})
	m, ok := got.(map[string]any)
	if !ok || m["name"] != "alice" {
		t.Fatalf("expected json-body map with name=alice, got %v", got)
	}
}

func TestWrapJson_invalidBody(t *testing.T) {
	var got any
	h := WrapJson(func(req map[string]any) map[string]any {
		got = req["json-body"]
		return map[string]any{"status": 200}
	})
	h(map[string]any{"body": "not json"})
	if got != nil {
		t.Fatalf("expected no json-body on invalid JSON, got %v", got)
	}
}

func TestWrapCors_addsHeaders(t *testing.T) {
	h := WrapCors(func(req map[string]any) map[string]any {
		return map[string]any{"status": 200}
	})
	resp := h(map[string]any{})
	hdrs, _ := resp["headers"].(map[string]any)
	if hdrs["Access-Control-Allow-Origin"] != "*" {
		t.Fatalf("expected CORS header, got %v", hdrs)
	}
}

func TestWrapCors_preservesExistingHeaders(t *testing.T) {
	h := WrapCors(func(req map[string]any) map[string]any {
		return map[string]any{
			"status":  200,
			"headers": map[string]any{"Content-Type": "application/json"},
		}
	})
	resp := h(map[string]any{})
	hdrs, _ := resp["headers"].(map[string]any)
	if hdrs["Content-Type"] != "application/json" {
		t.Fatalf("expected Content-Type preserved, got %v", hdrs)
	}
	if hdrs["Access-Control-Allow-Origin"] != "*" {
		t.Fatalf("expected CORS header added, got %v", hdrs)
	}
}

func TestWrapAuth_validBearer(t *testing.T) {
	var gotIdentity any
	h := WrapAuth(func(req map[string]any) map[string]any {
		gotIdentity = req["identity"]
		return map[string]any{"status": 200}
	})
	resp := h(map[string]any{
		"headers": map[string]any{"Authorization": "Bearer mytoken123"},
	})
	if resp["status"] != 200 {
		t.Fatalf("expected 200, got %v", resp["status"])
	}
	if gotIdentity != "mytoken123" {
		t.Fatalf("expected identity=mytoken123, got %v", gotIdentity)
	}
}

func TestWrapAuth_missing(t *testing.T) {
	h := WrapAuth(func(req map[string]any) map[string]any {
		return map[string]any{"status": 200}
	})
	resp := h(map[string]any{"headers": map[string]any{}})
	if resp["status"] != 401 {
		t.Fatalf("expected 401, got %v", resp["status"])
	}
}

func TestWrapTimeout_responds(t *testing.T) {
	h := WrapTimeout(1)(func(req map[string]any) map[string]any {
		return map[string]any{"status": 200}
	})
	resp := h(map[string]any{})
	if resp["status"] != 200 {
		t.Fatalf("expected 200, got %v", resp["status"])
	}
}

func TestWrapTimeout_expires(t *testing.T) {
	h := WrapTimeout(1)(func(req map[string]any) map[string]any {
		time.Sleep(3 * time.Second)
		return map[string]any{"status": 200}
	})
	resp := h(map[string]any{})
	if resp["status"] != 503 {
		t.Fatalf("expected 503, got %v", resp["status"])
	}
}

func TestCompose_order(t *testing.T) {
	var order []string
	mw := func(name string) Middleware {
		return func(h Handler) Handler {
			return func(req map[string]any) map[string]any {
				order = append(order, name+"-in")
				resp := h(req)
				order = append(order, name+"-out")
				return resp
			}
		}
	}
	h := Compose(mw("A"), mw("B"))(func(req map[string]any) map[string]any {
		order = append(order, "handler")
		return map[string]any{"status": 200}
	})
	h(map[string]any{})
	want := []string{"A-in", "B-in", "handler", "B-out", "A-out"}
	if fmt.Sprint(order) != fmt.Sprint(want) {
		t.Fatalf("expected order %v, got %v", want, order)
	}
}

func TestWrap_order(t *testing.T) {
	var order []string
	mw := func(name string) Middleware {
		return func(h Handler) Handler {
			return func(req map[string]any) map[string]any {
				order = append(order, name)
				return h(req)
			}
		}
	}
	handler := func(req map[string]any) map[string]any { return map[string]any{"status": 200} }
	Wrap(handler, mw("A"), mw("B"))(map[string]any{})
	want := []string{"A", "B"}
	if fmt.Sprint(order) != fmt.Sprint(want) {
		t.Fatalf("expected order %v, got %v", want, order)
	}
}

func TestQueryParam(t *testing.T) {
	req := map[string]any{"query": "name=alice&age=30"}
	if QueryParam(req, "name") != "alice" {
		t.Fatalf("expected alice, got %q", QueryParam(req, "name"))
	}
	if QueryParam(req, "age") != "30" {
		t.Fatalf("expected 30, got %q", QueryParam(req, "age"))
	}
	if QueryParam(req, "missing") != "" {
		t.Fatalf("expected empty, got %q", QueryParam(req, "missing"))
	}
}

func TestPathParam(t *testing.T) {
	req := map[string]any{"params": map[string]any{"id": "42"}}
	if PathParam(req, "id") != "42" {
		t.Fatalf("expected 42, got %q", PathParam(req, "id"))
	}
	if PathParam(req, "missing") != "" {
		t.Fatalf("expected empty, got %q", PathParam(req, "missing"))
	}
}

func TestBodyMap_fromJsonBody(t *testing.T) {
	req := map[string]any{"json-body": map[string]any{"x": "y"}}
	m := BodyMap(req)
	if m["x"] != "y" {
		t.Fatalf("expected y, got %v", m["x"])
	}
}

func TestBodyMap_fromRawBody(t *testing.T) {
	req := map[string]any{"body": `{"a":"b"}`}
	m := BodyMap(req)
	if m["a"] != "b" {
		t.Fatalf("expected b, got %v", m["a"])
	}
}

func TestHeader(t *testing.T) {
	req := map[string]any{"headers": map[string]any{"Content-Type": "application/json"}}
	if Header(req, "Content-Type") != "application/json" {
		t.Fatalf("expected application/json, got %q", Header(req, "Content-Type"))
	}
	if Header(req, "Missing") != "" {
		t.Fatalf("expected empty, got %q", Header(req, "Missing"))
	}
}

func TestServeFiles_found(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/hello.txt", []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}
	h := ServeFiles("/static/", dir)
	resp := h(map[string]any{"path": "/static/hello.txt"})
	if resp["status"] != 200 {
		t.Fatalf("expected 200, got %v", resp["status"])
	}
	body, _ := resp["body"].([]byte)
	if string(body) != "hello world" {
		t.Fatalf("expected hello world, got %q", string(body))
	}
}

func TestServeFiles_notFound(t *testing.T) {
	h := ServeFiles("/static/", t.TempDir())
	resp := h(map[string]any{"path": "/static/nope.txt"})
	if resp["status"] != 404 {
		t.Fatalf("expected 404, got %v", resp["status"])
	}
}
