package macro

import (
	"testing"

	"golisp/internal/ast"
	"golisp/internal/parser"
)

func expandStr(t *testing.T, src string) []ast.Node {
	t.Helper()
	nodes, err := parser.ParseString(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := Expand(nodes, nil)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	return out
}

// firstDefnBody returns the body of the first DefnDecl in the expanded nodes.
func firstDefnBody(t *testing.T, nodes []ast.Node) []ast.Node {
	t.Helper()
	for _, n := range nodes {
		if d, ok := n.(*ast.DefnDecl); ok {
			return d.Body
		}
	}
	t.Fatal("no defn in expanded output")
	return nil
}

func TestExpandRemovesMacroDecls(t *testing.T) {
	out := expandStr(t, "(defmacro m [x] `(+ ~x 1))\n(defn f [] (m 2))")
	for _, n := range out {
		if _, ok := n.(*ast.MacroDecl); ok {
			t.Error("MacroDecl should be removed from expanded output")
		}
	}
	if len(out) != 1 {
		t.Errorf("expected 1 node after expansion, got %d", len(out))
	}
}

func TestExpandSimpleCall(t *testing.T) {
	body := firstDefnBody(t, expandStr(t, "(defmacro double [x] `(* ~x 2))\n(defn f [] (double 5))"))
	call, ok := body[0].(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr, got %T", body[0])
	}
	if sym, ok := call.Head.(*ast.Symbol); !ok || sym.Name != "*" {
		t.Errorf("head = %v, want *", call.Head)
	}
	if lit, ok := call.Args[0].(*ast.IntLit); !ok || lit.Value != 5 {
		t.Errorf("arg 0 = %v, want 5", call.Args[0])
	}
}

func TestExpandToSpecialForm(t *testing.T) {
	// A macro expanding to (if …) must yield a real *ast.IfExpr, not a CallExpr.
	body := firstDefnBody(t, expandStr(t, "(defmacro unless [c b] `(if ~c nil ~b))\n(defn f [] (unless x y))"))
	if _, ok := body[0].(*ast.IfExpr); !ok {
		t.Fatalf("expected *ast.IfExpr, got %T", body[0])
	}
}

func TestExpandFixedPoint(t *testing.T) {
	// foo expands to a twice call, which itself expands to a do.
	src := "(defmacro twice [x] `(do ~x ~x))\n" +
		"(defmacro foo [x] `(twice ~x))\n" +
		"(defn f [] (foo (g)))"
	body := firstDefnBody(t, expandStr(t, src))
	do, ok := body[0].(*ast.DoExpr)
	if !ok {
		t.Fatalf("expected *ast.DoExpr, got %T", body[0])
	}
	if len(do.Body) != 2 {
		t.Errorf("do has %d forms, want 2", len(do.Body))
	}
}

func TestExpandNestedInLet(t *testing.T) {
	// A macro call nested inside a let body is expanded.
	src := "(defmacro double [x] `(* ~x 2))\n(defn f [] (let [a 1] (double a)))"
	body := firstDefnBody(t, expandStr(t, src))
	let, ok := body[0].(*ast.LetExpr)
	if !ok {
		t.Fatalf("expected *ast.LetExpr, got %T", body[0])
	}
	if _, ok := let.Body[0].(*ast.CallExpr); !ok {
		t.Errorf("let body not expanded: %T", let.Body[0])
	}
}

func TestExpandArityError(t *testing.T) {
	nodes, err := parser.ParseString("(defmacro m [x y] `(+ ~x ~y))\n(defn f [] (m 1))")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := Expand(nodes, nil); err == nil {
		t.Error("expected arity error for macro called with too few args")
	}
}

func TestExpandNonTerminating(t *testing.T) {
	nodes, err := parser.ParseString("(defmacro loopy [] `(loopy))\n(defn f [] (loopy))")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := Expand(nodes, nil); err == nil {
		t.Error("expected non-termination error")
	}
}

func TestExpandCrossFile(t *testing.T) {
	// A macro defined in a sibling file (external) is in scope when expanding a
	// file that does not define it locally.
	libNodes, err := parser.ParseString("(defmacro double [x] `(* ~x 2))")
	if err != nil {
		t.Fatalf("parse lib: %v", err)
	}
	var external []*ast.MacroDecl
	for _, n := range libNodes {
		if md, ok := n.(*ast.MacroDecl); ok {
			external = append(external, md)
		}
	}

	appNodes, err := parser.ParseString("(defn f [] (double 5))")
	if err != nil {
		t.Fatalf("parse app: %v", err)
	}
	out, err := Expand(appNodes, external)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	call, ok := firstDefnBody(t, out)[0].(*ast.CallExpr)
	if !ok {
		t.Fatalf("cross-file macro not expanded: %T", firstDefnBody(t, out)[0])
	}
	if sym, ok := call.Head.(*ast.Symbol); !ok || sym.Name != "*" {
		t.Errorf("expanded head = %v, want *", call.Head)
	}

	// Without the external macro it stays an unexpanded call to `double`.
	appNodes2, _ := parser.ParseString("(defn f [] (double 5))")
	out2, err := Expand(appNodes2, nil)
	if err != nil {
		t.Fatalf("expand (no external): %v", err)
	}
	if sym, ok := firstDefnBody(t, out2)[0].(*ast.CallExpr).Head.(*ast.Symbol); !ok || sym.Name != "double" {
		t.Errorf("without external macro, call should stay `double`, got %v", firstDefnBody(t, out2)[0])
	}
}

func TestExpandOnceSingleStep(t *testing.T) {
	// macroexpand-1 over top-level forms: outermost macro expands one step only,
	// the nested macro call is left intact.
	src := "(defmacro twice [x] `(do ~x ~x))\n" +
		"(defmacro unless [c b] `(if ~c nil ~b))\n" +
		"(unless x (twice y))"
	nodes, err := parser.ParseString(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := ExpandOnce(nodes, nil)
	if err != nil {
		t.Fatalf("expand-once: %v", err)
	}
	// the macro defs are removed, leaving the one expanded form
	if len(out) != 1 {
		t.Fatalf("expected 1 form, got %d", len(out))
	}
	iff, ok := out[0].(*ast.IfExpr)
	if !ok {
		t.Fatalf("outermost not expanded to if: %T", out[0])
	}
	// the inner (twice y) must still be an unexpanded call, not a do
	call, ok := iff.Else.(*ast.CallExpr)
	if !ok {
		t.Fatalf("inner should remain a call, got %T", iff.Else)
	}
	if sym, ok := call.Head.(*ast.Symbol); !ok || sym.Name != "twice" {
		t.Errorf("inner macro should be unexpanded `twice`, got %v", call.Head)
	}
}

func TestExpandNoMacrosPassthrough(t *testing.T) {
	nodes, err := parser.ParseString("(defn f [] (+ 1 2))")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := Expand(nodes, nil)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if len(out) != len(nodes) {
		t.Errorf("passthrough changed node count: %d -> %d", len(nodes), len(out))
	}
}
