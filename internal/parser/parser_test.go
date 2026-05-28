package parser

import (
	"testing"

	"golisp/internal/ast"
)

func mustParse(t *testing.T, src string) []ast.Node {
	t.Helper()
	nodes, err := ParseString(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return nodes
}

func TestParseLiterals(t *testing.T) {
	tests := []struct {
		src  string
		want ast.Node
	}{
		{"nil", &ast.NilLit{}},
		{"true", &ast.BoolLit{Value: true}},
		{"false", &ast.BoolLit{Value: false}},
		{"42", &ast.IntLit{Value: 42}},
		{"-7", &ast.IntLit{Value: -7}},
		{"3.14", &ast.FloatLit{Value: 3.14}},
		{`"hello"`, &ast.StringLit{Value: "hello"}},
		{":foo", &ast.KeywordLit{Value: "foo"}},
		{"bar", &ast.Symbol{Name: "bar"}},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			nodes := mustParse(t, tt.src)
			if len(nodes) != 1 {
				t.Fatalf("expected 1 node, got %d", len(nodes))
			}
			switch want := tt.want.(type) {
			case *ast.NilLit:
				if _, ok := nodes[0].(*ast.NilLit); !ok {
					t.Errorf("got %T, want *ast.NilLit", nodes[0])
				}
			case *ast.BoolLit:
				got, ok := nodes[0].(*ast.BoolLit)
				if !ok {
					t.Errorf("got %T, want *ast.BoolLit", nodes[0])
				} else if got.Value != want.Value {
					t.Errorf("value: got %v, want %v", got.Value, want.Value)
				}
			case *ast.IntLit:
				got, ok := nodes[0].(*ast.IntLit)
				if !ok {
					t.Errorf("got %T, want *ast.IntLit", nodes[0])
				} else if got.Value != want.Value {
					t.Errorf("value: got %v, want %v", got.Value, want.Value)
				}
			case *ast.FloatLit:
				got, ok := nodes[0].(*ast.FloatLit)
				if !ok {
					t.Errorf("got %T, want *ast.FloatLit", nodes[0])
				} else if got.Value != want.Value {
					t.Errorf("value: got %v, want %v", got.Value, want.Value)
				}
			case *ast.StringLit:
				got, ok := nodes[0].(*ast.StringLit)
				if !ok {
					t.Errorf("got %T, want *ast.StringLit", nodes[0])
				} else if got.Value != want.Value {
					t.Errorf("value: got %q, want %q", got.Value, want.Value)
				}
			case *ast.KeywordLit:
				got, ok := nodes[0].(*ast.KeywordLit)
				if !ok {
					t.Errorf("got %T, want *ast.KeywordLit", nodes[0])
				} else if got.Value != want.Value {
					t.Errorf("value: got %q, want %q", got.Value, want.Value)
				}
			case *ast.Symbol:
				got, ok := nodes[0].(*ast.Symbol)
				if !ok {
					t.Errorf("got %T, want *ast.Symbol", nodes[0])
				} else if got.Name != want.Name {
					t.Errorf("name: got %q, want %q", got.Name, want.Name)
				}
			}
		})
	}
}

func TestParseDefn(t *testing.T) {
	nodes := mustParse(t, `(defn ^int add [^int a ^int b] (+ a b))`)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	fn, ok := nodes[0].(*ast.DefnDecl)
	if !ok {
		t.Fatalf("got %T, want *ast.DefnDecl", nodes[0])
	}
	if fn.Name != "add" {
		t.Errorf("name: got %q, want %q", fn.Name, "add")
	}
	if fn.ReturnType == nil || fn.ReturnType.Text != "int" {
		t.Errorf("return type: got %v", fn.ReturnType)
	}
	if len(fn.Params) != 2 {
		t.Fatalf("params: got %d, want 2", len(fn.Params))
	}
	if fn.Params[0].Name != "a" || fn.Params[0].TypeAnnot.Text != "int" {
		t.Errorf("param[0]: got %v", fn.Params[0])
	}
	if fn.Params[1].Name != "b" || fn.Params[1].TypeAnnot.Text != "int" {
		t.Errorf("param[1]: got %v", fn.Params[1])
	}
	if len(fn.Body) != 1 {
		t.Errorf("body: got %d exprs", len(fn.Body))
	}
}

func TestParseLet(t *testing.T) {
	nodes := mustParse(t, `(let [x 10 y 20] (+ x y))`)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	le, ok := nodes[0].(*ast.LetExpr)
	if !ok {
		t.Fatalf("got %T, want *ast.LetExpr", nodes[0])
	}
	if len(le.Bindings) != 2 {
		t.Fatalf("bindings: got %d, want 2", len(le.Bindings))
	}
}

func TestParseLetMultiReturn(t *testing.T) {
	nodes := mustParse(t, `(let [[val err] (read-file "x")] val)`)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	le, ok := nodes[0].(*ast.LetExpr)
	if !ok {
		t.Fatalf("got %T, want *ast.LetExpr", nodes[0])
	}
	if _, ok := le.Bindings[0].Pattern.(*ast.VectorLit); !ok {
		t.Errorf("pattern: expected *ast.VectorLit, got %T", le.Bindings[0].Pattern)
	}
}

func TestParseIf(t *testing.T) {
	nodes := mustParse(t, `(if true 1 2)`)
	ie, ok := nodes[0].(*ast.IfExpr)
	if !ok {
		t.Fatalf("got %T", nodes[0])
	}
	if ie.Else == nil {
		t.Error("expected else branch")
	}
}

func TestParseGoroutineChannel(t *testing.T) {
	nodes := mustParse(t, `
(let [ch (chan ^int 10)]
  (go (send! ch 42))
  (recv! ch))`)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	le, ok := nodes[0].(*ast.LetExpr)
	if !ok {
		t.Fatalf("got %T, want *ast.LetExpr", nodes[0])
	}
	// binding value should be ChanExpr
	ce, ok := le.Bindings[0].Value.(*ast.ChanExpr)
	if !ok {
		t.Fatalf("chan binding: got %T, want *ast.ChanExpr", le.Bindings[0].Value)
	}
	if ce.ElemType.Text != "int" {
		t.Errorf("chan elem type: got %q, want %q", ce.ElemType.Text, "int")
	}
}

func TestParseIfErr(t *testing.T) {
	nodes := mustParse(t, `(if-err [data err] (read-file "x") (println err) data)`)
	ie, ok := nodes[0].(*ast.IfErrExpr)
	if !ok {
		t.Fatalf("got %T, want *ast.IfErrExpr", nodes[0])
	}
	if ie.ValName != "data" {
		t.Errorf("val: got %q, want %q", ie.ValName, "data")
	}
	if ie.ErrName != "err" {
		t.Errorf("err: got %q, want %q", ie.ErrName, "err")
	}
}

func TestParseMethodCall(t *testing.T) {
	nodes := mustParse(t, `(.Write w data)`)
	mc, ok := nodes[0].(*ast.MethodCallExpr)
	if !ok {
		t.Fatalf("got %T, want *ast.MethodCallExpr", nodes[0])
	}
	if mc.Method != "Write" {
		t.Errorf("method: got %q, want %q", mc.Method, "Write")
	}
}

func TestParseFieldAccess(t *testing.T) {
	nodes := mustParse(t, `(.-Method req)`)
	fa, ok := nodes[0].(*ast.FieldAccessExpr)
	if !ok {
		t.Fatalf("got %T, want *ast.FieldAccessExpr", nodes[0])
	}
	if fa.Field != "Method" {
		t.Errorf("field: got %q, want %q", fa.Field, "Method")
	}
}

func TestParseNS(t *testing.T) {
	nodes := mustParse(t, `(ns myapp.server (:import [net/http fmt]))`)
	ns, ok := nodes[0].(*ast.NSDecl)
	if !ok {
		t.Fatalf("got %T, want *ast.NSDecl", nodes[0])
	}
	if ns.Name != "myapp.server" {
		t.Errorf("name: got %q, want %q", ns.Name, "myapp.server")
	}
	if len(ns.Imports) != 2 {
		t.Fatalf("imports: got %d, want 2", len(ns.Imports))
	}
}

func TestParseLoop(t *testing.T) {
	nodes := mustParse(t, `(loop [i 0 acc []] (if (>= i 10) acc (recur (+ i 1) (conj acc i))))`)
	lo, ok := nodes[0].(*ast.LoopExpr)
	if !ok {
		t.Fatalf("got %T, want *ast.LoopExpr", nodes[0])
	}
	if len(lo.Bindings) != 2 {
		t.Fatalf("bindings: got %d, want 2", len(lo.Bindings))
	}
}
