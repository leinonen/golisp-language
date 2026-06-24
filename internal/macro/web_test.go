package macro

import (
	"strings"
	"testing"

	"golisp/internal/ast"
	"golisp/internal/formatter"
)

// TestWebVerbExpand checks a verb macro expands to a web/get call wrapping a
// typed handler that binds path params via web/path-param.
func TestWebVerbExpand(t *testing.T) {
	out := formatter.FormatNode(expandTop(t, `(GET "/users/:id" [id] (web/json-response 200 {"id" id}))`))
	for _, want := range []string{
		`(web/get "/users/:id"`,
		`(fn [req web/Request] -> web/Response`, // typed handler survives the bridge
		`(let [id (web/path-param req "id")]`,   // path param bound as a local
	} {
		if !strings.Contains(out, want) {
			t.Errorf("GET expansion missing %q\n--- got ---\n%s", want, out)
		}
	}
}

// TestWebVerbNoBindings checks an empty binding vector emits the handler body
// directly with no surrounding let.
func TestWebVerbNoBindings(t *testing.T) {
	out := formatter.FormatNode(expandTop(t, `(POST "/users" [] (web/json-response 201 {}))`))
	if !strings.Contains(out, `(web/post "/users"`) {
		t.Errorf("POST should expand to web/post: %s", out)
	}
	if strings.Contains(out, "web/path-param") || strings.Contains(out, "(let [") {
		t.Errorf("empty bindings should produce no let / path-param: %s", out)
	}
}

// TestDefroutesExpand checks defroutes builds a typed def, wrapping the routes in
// web/wrap when a :middleware clause is present and omitting it otherwise.
func TestDefroutesExpand(t *testing.T) {
	withMW := expandTop(t, `(defroutes app :middleware [web/wrap-json web/wrap-recover] (GET "/h" [] (web/json-response 200 {})))`)
	def, ok := withMW.(*ast.DefDecl)
	if !ok {
		t.Fatalf("defroutes should expand to a def, got %T", withMW)
	}
	if def.Name != "app" || def.TypeAnnot == nil || def.TypeAnnot.Text != "web/Handler" {
		t.Errorf("defroutes def should be `app web/Handler`, got name=%q type=%v", def.Name, def.TypeAnnot)
	}
	out := formatter.FormatNode(withMW)
	for _, want := range []string{"(web/wrap", "(web/routes", "web/wrap-json", "web/wrap-recover", "(web/get \"/h\""} {
		if !strings.Contains(out, want) {
			t.Errorf("defroutes (with middleware) missing %q\n--- got ---\n%s", want, out)
		}
	}

	noMW := formatter.FormatNode(expandTop(t, `(defroutes app (GET "/h" [] (web/json-response 200 {})))`))
	if strings.Contains(noMW, "web/wrap") {
		t.Errorf("defroutes without :middleware should not emit web/wrap: %s", noMW)
	}
	if !strings.Contains(noMW, "(web/routes") {
		t.Errorf("defroutes should always emit web/routes: %s", noMW)
	}
}
