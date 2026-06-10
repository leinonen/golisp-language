package web

import (
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
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

func TestMatchPath_paramDecoded(t *testing.T) {
	params, ok := matchPath("/users/:name", "/users/john%20doe")
	if !ok {
		t.Fatal("expected match")
	}
	if params["name"] != "john doe" {
		t.Fatalf("expected decoded param, got %v", params["name"])
	}
}

func TestMatchPath_wildcard(t *testing.T) {
	params, ok := matchPath("/static/*path", "/static/css/main.css")
	if !ok {
		t.Fatal("expected match")
	}
	if params["path"] != "css/main.css" {
		t.Fatalf("expected path=css/main.css, got %v", params["path"])
	}
}

func TestMatchPath_wildcardEmpty(t *testing.T) {
	params, ok := matchPath("/static/*path", "/static/")
	if !ok {
		t.Fatal("expected match")
	}
	if params["path"] != "" {
		t.Fatalf("expected empty path, got %v", params["path"])
	}
}

func TestMatchPath_wildcardWithParam(t *testing.T) {
	params, ok := matchPath("/users/:id/files/*file", "/users/7/files/docs/a.txt")
	if !ok {
		t.Fatal("expected match")
	}
	if params["id"] != "7" || params["file"] != "docs/a.txt" {
		t.Fatalf("unexpected params: %v", params)
	}
}

func TestMatchPath_wildcardTooShort(t *testing.T) {
	_, ok := matchPath("/static/assets/*path", "/static")
	if ok {
		t.Fatal("expected no match")
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
		Get("/", func(req map[string]any) map[string]any {
			called = "home"
			return map[string]any{"status": 200}
		}),
		Post("/items", func(req map[string]any) map[string]any {
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
		Get("/items", func(req map[string]any) map[string]any {
			return map[string]any{"status": 200}
		}),
		Put("/items", func(req map[string]any) map[string]any {
			return map[string]any{"status": 200}
		}),
	)
	resp := app(map[string]any{"method": "POST", "path": "/items"})
	if resp["status"] != 405 {
		t.Fatalf("expected 405, got %v", resp["status"])
	}
	hdrs, _ := resp["headers"].(map[string]any)
	if hdrs["Allow"] != "GET, PUT" {
		t.Fatalf("expected Allow: GET, PUT, got %v", hdrs["Allow"])
	}
}

func TestRoutes_noMatch(t *testing.T) {
	app := Routes(
		Get("/home", func(req map[string]any) map[string]any {
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
		Get("/users/:id", func(req map[string]any) map[string]any {
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

func TestBuildRequest_keys(t *testing.T) {
	r := httptest.NewRequest("GET", "http://example.com/items?q=1", nil)
	req := buildRequest(r)
	if req["host"] != "example.com" {
		t.Fatalf("expected host=example.com, got %v", req["host"])
	}
	if req["scheme"] != "http" {
		t.Fatalf("expected scheme=http, got %v", req["scheme"])
	}
	addr, _ := req["remote-addr"].(string)
	if addr == "" {
		t.Fatal("expected remote-addr to be set")
	}
}

func TestStatusOf(t *testing.T) {
	cases := []struct {
		resp map[string]any
		want int
	}{
		{map[string]any{"status": 404}, 404},
		{map[string]any{"status": int64(405)}, 405},
		{map[string]any{"status": float64(503)}, 503},
		{map[string]any{}, 200},
		{nil, 200},
	}
	for _, c := range cases {
		if got := statusOf(c.resp); got != c.want {
			t.Fatalf("statusOf(%v) = %d, want %d", c.resp, got, c.want)
		}
	}
}

func TestContext_prefixesRoutes(t *testing.T) {
	app := Routes(
		Context("/api/v1",
			Get("/items/:id", func(req map[string]any) map[string]any {
				return map[string]any{"status": 200, "body": PathParam(req, "id")}
			}),
		),
		Get("/", func(req map[string]any) map[string]any {
			return map[string]any{"status": 200, "body": "home"}
		}),
	)
	resp := app(map[string]any{"method": "GET", "path": "/api/v1/items/7"})
	if resp["status"] != 200 || resp["body"] != "7" {
		t.Fatalf("expected prefixed route with param, got %v", resp)
	}
	resp = app(map[string]any{"method": "GET", "path": "/"})
	if resp["body"] != "home" {
		t.Fatalf("expected unprefixed route to still match, got %v", resp)
	}
	resp = app(map[string]any{"method": "GET", "path": "/items/7"})
	if resp["status"] != 404 {
		t.Fatalf("expected 404 without prefix, got %v", resp["status"])
	}
}

func TestContext_nested(t *testing.T) {
	app := Routes(
		Context("/api",
			Context("/v1",
				Get("/ping", func(req map[string]any) map[string]any {
					return map[string]any{"status": 200}
				}),
			),
		),
	)
	resp := app(map[string]any{"method": "GET", "path": "/api/v1/ping"})
	if resp["status"] != 200 {
		t.Fatalf("expected 200, got %v", resp["status"])
	}
}

func TestContext_rootPattern(t *testing.T) {
	app := Routes(
		Context("/api",
			Get("/", func(req map[string]any) map[string]any {
				return map[string]any{"status": 200}
			}),
		),
	)
	resp := app(map[string]any{"method": "GET", "path": "/api"})
	if resp["status"] != 200 {
		t.Fatalf("expected / under /api to match /api, got %v", resp["status"])
	}
}

func TestWrapRecover_logsAndReturns500(t *testing.T) {
	h := WrapRecover(func(req map[string]any) map[string]any {
		panic("secret detail")
	})
	resp := h(map[string]any{"method": "GET", "path": "/boom"})
	if resp["status"] != 500 {
		t.Fatalf("expected 500, got %v", resp["status"])
	}
	body, _ := resp["body"].(string)
	if strings.Contains(body, "secret detail") {
		t.Fatalf("expected panic value not leaked to client, got %q", body)
	}
}

func TestWrapCors_preflight(t *testing.T) {
	called := false
	h := WrapCors(func(req map[string]any) map[string]any {
		called = true
		return map[string]any{"status": 404, "body": "not found"}
	})
	resp := h(map[string]any{"method": "OPTIONS", "path": "/tasks"})
	if called {
		t.Fatal("expected preflight to short-circuit, but handler was called")
	}
	if resp["status"] != 204 {
		t.Fatalf("expected 204, got %v", resp["status"])
	}
	hdrs, _ := resp["headers"].(map[string]any)
	if hdrs["Access-Control-Allow-Origin"] != "*" {
		t.Fatalf("expected CORS headers on preflight, got %v", hdrs)
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

func TestWrapAuthFunc(t *testing.T) {
	mw := WrapAuthFunc(func(token string) bool { return token == "good" })
	var gotIdentity any
	h := mw(func(req map[string]any) map[string]any {
		gotIdentity = req["identity"]
		return map[string]any{"status": 200}
	})

	resp := h(map[string]any{
		"headers": map[string]any{"Authorization": "Bearer good"},
	})
	if resp["status"] != 200 || gotIdentity != "good" {
		t.Fatalf("expected accepted token, got status=%v identity=%v", resp["status"], gotIdentity)
	}

	resp = h(map[string]any{
		"headers": map[string]any{"Authorization": "Bearer bad"},
	})
	if resp["status"] != 401 {
		t.Fatalf("expected 401 for rejected token, got %v", resp["status"])
	}

	resp = h(map[string]any{"headers": map[string]any{}})
	if resp["status"] != 401 {
		t.Fatalf("expected 401 for missing header, got %v", resp["status"])
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

func TestFormParam(t *testing.T) {
	req := map[string]any{"body": "name=alice&tags=a%20b"}
	if FormParam(req, "name") != "alice" {
		t.Fatalf("expected alice, got %q", FormParam(req, "name"))
	}
	if FormParam(req, "tags") != "a b" {
		t.Fatalf("expected decoded value, got %q", FormParam(req, "tags"))
	}
	if FormParam(req, "missing") != "" {
		t.Fatalf("expected empty, got %q", FormParam(req, "missing"))
	}
}

func TestCookie(t *testing.T) {
	req := map[string]any{"headers": map[string]any{"Cookie": "session=abc123; theme=dark"}}
	if Cookie(req, "session") != "abc123" {
		t.Fatalf("expected abc123, got %q", Cookie(req, "session"))
	}
	if Cookie(req, "theme") != "dark" {
		t.Fatalf("expected dark, got %q", Cookie(req, "theme"))
	}
	if Cookie(req, "missing") != "" {
		t.Fatalf("expected empty, got %q", Cookie(req, "missing"))
	}
	if Cookie(map[string]any{}, "any") != "" {
		t.Fatal("expected empty for request without Cookie header")
	}
}

func TestSetCookie(t *testing.T) {
	resp := SetCookie(map[string]any{"status": 200}, "session", "abc123")
	hdrs, _ := resp["headers"].(map[string]any)
	v, _ := hdrs["Set-Cookie"].(string)
	if !strings.Contains(v, "session=abc123") || !strings.Contains(v, "Path=/") {
		t.Fatalf("unexpected Set-Cookie header: %q", v)
	}
}

func TestHeader_caseInsensitive(t *testing.T) {
	req := map[string]any{"headers": map[string]any{"Content-Type": "application/json"}}
	if Header(req, "content-type") != "application/json" {
		t.Fatalf("expected case-insensitive lookup, got %q", Header(req, "content-type"))
	}
}

func TestResponseHelpers(t *testing.T) {
	cases := []struct {
		resp   map[string]any
		status int
		body   string
	}{
		{BadRequest("oops"), 400, `{"error":"oops"}`},
		{Unauthorized("nope"), 401, `{"error":"nope"}`},
		{NotFound("missing"), 404, `{"error":"missing"}`},
		{ServerError("boom"), 500, `{"error":"boom"}`},
	}
	for _, c := range cases {
		if c.resp["status"] != c.status {
			t.Fatalf("expected status %d, got %v", c.status, c.resp["status"])
		}
		if c.resp["body"] != c.body {
			t.Fatalf("expected body %q, got %v", c.body, c.resp["body"])
		}
		hdrs, _ := c.resp["headers"].(map[string]any)
		if hdrs["Content-Type"] != "application/json" {
			t.Fatalf("expected JSON content type, got %v", hdrs)
		}
	}
}

func TestJsonResponse_marshalError(t *testing.T) {
	resp := JsonResponse(200, map[string]any{"ch": make(chan int)})
	if resp["status"] != 500 {
		t.Fatalf("expected 500 on unencodable body, got %v", resp["status"])
	}
	body, _ := resp["body"].(string)
	if !strings.Contains(body, "error") {
		t.Fatalf("expected JSON error body, got %q", body)
	}
}

func TestRoutes_headAndOptions(t *testing.T) {
	app := Routes(
		Head("/items", func(req map[string]any) map[string]any {
			return map[string]any{"status": 200}
		}),
		Options("/items", func(req map[string]any) map[string]any {
			return map[string]any{"status": 204}
		}),
	)
	resp := app(map[string]any{"method": "HEAD", "path": "/items"})
	if resp["status"] != 200 {
		t.Fatalf("expected 200 for HEAD, got %v", resp["status"])
	}
	resp = app(map[string]any{"method": "OPTIONS", "path": "/items"})
	if resp["status"] != 204 {
		t.Fatalf("expected 204 for OPTIONS, got %v", resp["status"])
	}
}

func TestNoContent(t *testing.T) {
	resp := NoContent()
	if resp["status"] != 204 {
		t.Fatalf("expected 204, got %v", resp["status"])
	}
	if resp["body"] != "" {
		t.Fatalf("expected empty body, got %v", resp["body"])
	}
}

func TestRoutes_wildcardServesFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/app.css", []byte("body{}"), 0644); err != nil {
		t.Fatal(err)
	}
	app := Routes(
		Get("/static/*path", ServeFiles("/static/", dir)),
		Get("/", func(req map[string]any) map[string]any {
			return map[string]any{"status": 200}
		}),
	)
	resp := app(map[string]any{"method": "GET", "path": "/static/app.css"})
	if resp["status"] != 200 {
		t.Fatalf("expected 200, got %v", resp["status"])
	}
	body, _ := resp["body"].([]byte)
	if string(body) != "body{}" {
		t.Fatalf("expected file contents, got %q", string(body))
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

func TestServeFiles_forwardsRangeHeader(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/data.txt", []byte("0123456789"), 0644); err != nil {
		t.Fatal(err)
	}
	h := ServeFiles("/static/", dir)
	resp := h(map[string]any{
		"path":    "/static/data.txt",
		"headers": map[string]any{"Range": "bytes=0-4"},
	})
	if resp["status"] != 206 {
		t.Fatalf("expected 206 partial content, got %v", resp["status"])
	}
	body, _ := resp["body"].([]byte)
	if string(body) != "01234" {
		t.Fatalf("expected first 5 bytes, got %q", string(body))
	}
}

func TestServeFiles_badPath(t *testing.T) {
	h := ServeFiles("/static/", t.TempDir())
	for _, p := range []string{"", "no-leading-slash"} {
		resp := h(map[string]any{"path": p})
		if resp["status"] != 400 {
			t.Fatalf("expected 400 for path %q, got %v", p, resp["status"])
		}
	}
}
