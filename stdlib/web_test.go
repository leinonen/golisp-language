package stdlib

import (
	"testing"
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
