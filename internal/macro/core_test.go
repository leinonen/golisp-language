package macro

import (
	"strings"
	"testing"

	"golisp/internal/ast"
	"golisp/internal/formatter"
	"golisp/internal/parser"
)

func TestCoreMacrosLoad(t *testing.T) {
	core, err := CoreMacros()
	if err != nil {
		t.Fatalf("core prelude failed to load: %v", err)
	}
	if len(core) == 0 {
		t.Fatal("core prelude has no macros")
	}
	names := map[string]bool{}
	for _, md := range core {
		names[md.Name] = true
	}
	for _, want := range []string{"when-not", "if-not", "->", "->>", "some->", "some->>", "cond->", "cond->>", "GET", "POST", "defroutes", "route-form"} {
		if !names[want] {
			t.Errorf("core prelude missing %q", want)
		}
	}
}

// expandTop parses a single form and returns its (fully) expanded node, with
// the core prelude available (no local/external macros).
func expandTop(t *testing.T, src string) ast.Node {
	t.Helper()
	nodes, err := parser.ParseString(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := Expand(nodes, nil)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 form, got %d", len(out))
	}
	return out[0]
}

func TestCoreWhenNot(t *testing.T) {
	// (when-not c x) => (if c nil (do x))
	iff, ok := expandTop(t, "(when-not c (f))").(*ast.IfExpr)
	if !ok {
		t.Fatalf("when-not did not expand to if: %T", expandTop(t, "(when-not c (f))"))
	}
	if _, ok := iff.Then.(*ast.NilLit); !ok {
		t.Errorf("when-not then-branch should be nil, got %T", iff.Then)
	}
	if _, ok := iff.Else.(*ast.DoExpr); !ok {
		t.Errorf("when-not else-branch should be a do, got %T", iff.Else)
	}
}

func TestCoreIfNot(t *testing.T) {
	// (if-not c x y) => (if c y x)
	iff, ok := expandTop(t, "(if-not c x y)").(*ast.IfExpr)
	if !ok {
		t.Fatalf("if-not did not expand to if: %T", expandTop(t, "(if-not c x y)"))
	}
	if sym, ok := iff.Then.(*ast.Symbol); !ok || sym.Name != "y" {
		t.Errorf("if-not then-branch should be y, got %v", iff.Then)
	}
	if sym, ok := iff.Else.(*ast.Symbol); !ok || sym.Name != "x" {
		t.Errorf("if-not else-branch should be x, got %v", iff.Else)
	}

	// (if-not c x) => (if c nil x) — optional else
	iff2 := expandTop(t, "(if-not c x)").(*ast.IfExpr)
	if _, ok := iff2.Then.(*ast.NilLit); !ok {
		t.Errorf("if-not without else: then-branch should be nil, got %T", iff2.Then)
	}
}

func TestCoreMacroShadowedByLocal(t *testing.T) {
	// A user-defined when-not takes precedence over the core one.
	nodes, err := parser.ParseString("(defmacro when-not [x] `(custom ~x))\n(defn f [] (when-not y))")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := Expand(nodes, nil)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	body := firstDefnBody(t, out)
	call, ok := body[0].(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected call, got %T", body[0])
	}
	if sym, ok := call.Head.(*ast.Symbol); !ok || sym.Name != "custom" {
		t.Errorf("local when-not should shadow core, got head %v", call.Head)
	}
}

// TestMacroTypedFnRoundTrip verifies the AST↔value bridge preserves single-symbol
// fn param types and the return type through a syntax-quote (the capability the
// routing DSL relies on to emit typed handlers).
// TestMacroOpaqueFormPassthrough verifies a specialized form the bridge can't
// decompose (switch) survives being passed through a macro as an argument — the
// general pass-through the routing DSL relies on for arbitrary handler bodies.
func TestMacroOpaqueFormPassthrough(t *testing.T) {
	nodes, err := parser.ParseString("(defmacro wrap [body] `(do ~body))\n(defn f [x] -> any (wrap (switch x 1 \"one\" :default \"other\")))")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := Expand(nodes, nil)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	got := formatter.FormatNode(firstDefnBody(t, out)[0])
	if !strings.Contains(got, "(switch x") {
		t.Errorf("switch form did not pass through the macro: %s", got)
	}
}

func TestMacroTypedFnRoundTrip(t *testing.T) {
	nodes, err := parser.ParseString("(defmacro mk [] `(fn [x int s string] -> bool x))\n(defn f [] -> any (mk))")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := Expand(nodes, nil)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	got := formatter.FormatNode(firstDefnBody(t, out)[0])
	if !strings.Contains(got, "(fn [x int s string] -> bool") {
		t.Errorf("typed fn did not round-trip through the macro bridge: %s", got)
	}
}

// TestMacroDefRoundTrip verifies a macro can emit a real top-level (def …) with a
// type annotation (built as data), which defroutes uses to define its handler.
func TestMacroDefRoundTrip(t *testing.T) {
	nodes, err := parser.ParseString("(defmacro mkdef [n] (list 'def n 'int 7))\n(mkdef answer)")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := Expand(nodes, nil)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	var def *ast.DefDecl
	for _, n := range out {
		if d, ok := n.(*ast.DefDecl); ok {
			def = d
		}
	}
	if def == nil {
		t.Fatalf("macro should expand to a *ast.DefDecl; got %d nodes", len(out))
	}
	if def.Name != "answer" || def.TypeAnnot == nil || def.TypeAnnot.Text != "int" {
		t.Errorf("expected `answer int 7`, got name=%q type=%v", def.Name, def.TypeAnnot)
	}
}

func TestCoreSomeThread(t *testing.T) {
	// (some-> x (f a) (g b)) => nested nil-guarded lets, threading first-arg.
	node := expandTop(t, "(some-> 5 (+ 1) (* 2))")
	if _, ok := node.(*ast.LetExpr); !ok {
		t.Fatalf("some-> should expand to a let, got %T", node)
	}
	out := formatter.FormatNode(node)
	for _, want := range []string{"(if (nil?", "(+ ", "(* "} {
		if !strings.Contains(out, want) {
			t.Errorf("some-> expansion missing %q\ngot: %s", want, out)
		}
	}
}

func TestCoreSomeThreadLast(t *testing.T) {
	// some->> threads the value as the LAST argument of each form.
	out := formatter.FormatNode(expandTop(t, "(some->> xs (map f))"))
	if !strings.Contains(out, "(map f ") {
		t.Errorf("some->> should thread value as last arg of (map f …): %s", out)
	}
	if !strings.Contains(out, "(if (nil?") {
		t.Errorf("some->> missing nil guard: %s", out)
	}
}

func TestCoreCondThread(t *testing.T) {
	// cond-> gates each form on its paired test, threading first-arg.
	node := expandTop(t, "(cond-> m c (assoc :a 1))")
	if _, ok := node.(*ast.LetExpr); !ok {
		t.Fatalf("cond-> should expand to a let, got %T", node)
	}
	out := formatter.FormatNode(node)
	if !strings.Contains(out, "(if c (assoc") {
		t.Errorf("cond-> should gate (assoc …) on test c: %s", out)
	}
}

func TestCoreCondThreadLast(t *testing.T) {
	// cond->> threads the value as the LAST argument of each chosen form.
	out := formatter.FormatNode(expandTop(t, "(cond->> xs c (map f))"))
	if !strings.Contains(out, "(if c (map f ") {
		t.Errorf("cond->> should thread value last in (map f …): %s", out)
	}
}
