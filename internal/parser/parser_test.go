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

func TestParseReaderMacros(t *testing.T) {
	// quote
	q := mustParse(t, "'x")
	if _, ok := q[0].(*ast.QuoteExpr); !ok {
		t.Fatalf("'x: got %T, want *ast.QuoteExpr", q[0])
	}
	// syntax-quote
	sq := mustParse(t, "`x")
	if _, ok := sq[0].(*ast.SyntaxQuoteExpr); !ok {
		t.Fatalf("`x: got %T, want *ast.SyntaxQuoteExpr", sq[0])
	}
	// unquote
	uq := mustParse(t, "~x")
	if _, ok := uq[0].(*ast.UnquoteExpr); !ok {
		t.Fatalf("~x: got %T, want *ast.UnquoteExpr", uq[0])
	}
	// unquote-splice
	us := mustParse(t, "~@xs")
	if _, ok := us[0].(*ast.UnquoteSpliceExpr); !ok {
		t.Fatalf("~@xs: got %T, want *ast.UnquoteSpliceExpr", us[0])
	}

	// Nested: `(a ~b ~@c) — a syntax-quote wrapping a list with an unquote and a splice.
	nested := mustParse(t, "`(a ~b ~@c)")
	sqn, ok := nested[0].(*ast.SyntaxQuoteExpr)
	if !ok {
		t.Fatalf("nested: got %T, want *ast.SyntaxQuoteExpr", nested[0])
	}
	list, ok := sqn.Form.(*ast.CallExpr)
	if !ok {
		t.Fatalf("nested form: got %T, want *ast.CallExpr", sqn.Form)
	}
	if _, ok := list.Args[0].(*ast.UnquoteExpr); !ok {
		t.Errorf("nested arg 0: got %T, want *ast.UnquoteExpr", list.Args[0])
	}
	if _, ok := list.Args[1].(*ast.UnquoteSpliceExpr); !ok {
		t.Errorf("nested arg 1: got %T, want *ast.UnquoteSpliceExpr", list.Args[1])
	}
}

func TestParseDefn(t *testing.T) {
	nodes := mustParse(t, `(defn add [a int b int] -> int (+ a b))`)
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
(let [ch (chan int 10)]
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

func TestParseNSImportForms(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []ast.ImportSpec
	}{
		{
			name: "bare paths in one vector",
			src:  `(ns m (:import [net/http fmt]))`,
			want: []ast.ImportSpec{{Path: "net/http"}, {Path: "fmt"}},
		},
		{
			name: "one vector per path",
			src:  `(ns m (:import [context] [github.com/google/uuid]))`,
			want: []ast.ImportSpec{{Path: "context"}, {Path: "github.com/google/uuid"}},
		},
		{
			name: "alias inside the vector",
			src:  `(ns m (:import [github.com/jackc/pgx/v5 :as pgx]))`,
			want: []ast.ImportSpec{{Path: "github.com/jackc/pgx/v5", Alias: "pgx"}},
		},
		{
			name: "nested alias vector",
			src:  `(ns m (:import [context [github.com/jackc/pgx/v5 :as pgx]]))`,
			want: []ast.ImportSpec{{Path: "context"}, {Path: "github.com/jackc/pgx/v5", Alias: "pgx"}},
		},
		{
			name: "alias applies to preceding path",
			src:  `(ns m (:import [context github.com/google/uuid :as id]))`,
			want: []ast.ImportSpec{{Path: "context"}, {Path: "github.com/google/uuid", Alias: "id"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := mustParse(t, tt.src)
			ns, ok := nodes[0].(*ast.NSDecl)
			if !ok {
				t.Fatalf("got %T, want *ast.NSDecl", nodes[0])
			}
			if len(ns.Imports) != len(tt.want) {
				t.Fatalf("imports: got %v, want %v", ns.Imports, tt.want)
			}
			for i, w := range tt.want {
				if ns.Imports[i] != w {
					t.Errorf("import %d: got %+v, want %+v", i, ns.Imports[i], w)
				}
			}
		})
	}
}

func TestParseNSImportErrors(t *testing.T) {
	for _, src := range []string{
		`(ns m (:import [:as pgx]))`,
		`(ns m (:import [a :as x :as y]))`,
	} {
		if _, err := ParseString(src); err == nil {
			t.Errorf("expected parse error for %s", src)
		}
	}
}

func TestParseCase(t *testing.T) {
	// case parses into a SwitchExpr with Head "case"; a trailing unpaired form
	// is the default.
	nodes := mustParse(t, `(case n 0 "zero" 1 "one" "many")`)
	sw, ok := nodes[0].(*ast.SwitchExpr)
	if !ok {
		t.Fatalf("got %T, want *ast.SwitchExpr", nodes[0])
	}
	if sw.Head != "case" {
		t.Errorf("Head: got %q, want \"case\"", sw.Head)
	}
	if len(sw.Cases) != 2 {
		t.Errorf("cases: got %d, want 2", len(sw.Cases))
	}
	if sw.Default == nil {
		t.Error("expected a trailing default")
	}

	// A degenerate (case x) with no clauses or default is a parse error.
	if _, err := ParseString(`(case n)`); err == nil {
		t.Error("expected parse error for (case n) with no clauses or default")
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

func TestErrorSourceContext(t *testing.T) {
	_, err := ParseString("(defn foo []\n  (defun bad))")
	if err == nil {
		t.Fatal("expected parse error")
	}
	msg := err.Error()
	if !contains(msg, "defun") {
		t.Errorf("error should mention defun, got: %s", msg)
	}
	if !contains(msg, "defn") {
		t.Errorf("error should suggest defn, got: %s", msg)
	}
	// should include source line
	if !contains(msg, "(defun bad)") {
		t.Errorf("error should include source line, got: %s", msg)
	}
	// should include ^ pointer
	if !contains(msg, "^") {
		t.Errorf("error should include ^ pointer, got: %s", msg)
	}
}

func TestDidYouMean(t *testing.T) {
	cases := []struct {
		src  string
		hint string
	}{
		{"(defun foo [] 1)", "defn"},
		{"(lambda [x] x)", "fn"},
		{"(begin 1 2)", "do"},
	}
	for _, c := range cases {
		_, err := ParseString(c.src)
		if err == nil {
			t.Errorf("%q: expected parse error", c.src)
			continue
		}
		if !contains(err.Error(), c.hint) {
			t.Errorf("%q: error %q should contain hint %q", c.src, err.Error(), c.hint)
		}
	}
}

func TestParseDefTest(t *testing.T) {
	nodes := mustParse(t, `(deftest my-test (assert= 1 1) (assert-true true))`)
	dt, ok := nodes[0].(*ast.DefTestDecl)
	if !ok {
		t.Fatalf("got %T, want *ast.DefTestDecl", nodes[0])
	}
	if dt.Name != "my-test" {
		t.Errorf("name: got %q, want %q", dt.Name, "my-test")
	}
	if len(dt.Body) != 2 {
		t.Errorf("body len: got %d, want 2", len(dt.Body))
	}
}

func TestDocComment(t *testing.T) {
	t.Run("sets Doc on defn", func(t *testing.T) {
		nodes := mustParse(t, ";;; Returns the sum.\n(defn add [a int b int] -> int (+ a b))")
		fn := nodes[0].(*ast.DefnDecl)
		if fn.Doc != "Returns the sum." {
			t.Errorf("Doc: got %q, want %q", fn.Doc, "Returns the sum.")
		}
	})

	t.Run("overrides string literal doc", func(t *testing.T) {
		nodes := mustParse(t, ";;; Comment doc.\n(defn f [] -> int \"String doc.\" 1)")
		fn := nodes[0].(*ast.DefnDecl)
		if fn.Doc != "Comment doc." {
			t.Errorf("Doc: got %q, want %q", fn.Doc, "Comment doc.")
		}
	})

	t.Run("consecutive comments accumulate into a multi-line docstring", func(t *testing.T) {
		nodes := mustParse(t, ";;; First.\n;;; Second.\n(defn f [] nil)")
		fn := nodes[0].(*ast.DefnDecl)
		if fn.Doc != "First.\nSecond." {
			t.Errorf("Doc: got %q, want %q", fn.Doc, "First.\nSecond.")
		}
	})

	t.Run("not attached to next defn when separated by another form", func(t *testing.T) {
		nodes := mustParse(t, ";;; Doc for f.\n(defn f [] nil)\n(defn g [] nil)")
		g := nodes[1].(*ast.DefnDecl)
		if g.Doc != "" {
			t.Errorf("g.Doc: got %q, want empty", g.Doc)
		}
	})

	t.Run("regular comments still ignored", func(t *testing.T) {
		nodes := mustParse(t, "; single\n;; double\n(defn f [] nil)")
		fn := nodes[0].(*ast.DefnDecl)
		if fn.Doc != "" {
			t.Errorf("Doc: got %q, want empty", fn.Doc)
		}
	})

	t.Run("orphan ;;; before non-defn surfaces as a comment", func(t *testing.T) {
		res, err := ParseWithComments(";;; File doc.\n(ns main)")
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if got := res.Comments[1]; got != ";;; File doc." {
			t.Errorf("orphan doc: got %q, want %q", got, ";;; File doc.")
		}
	})

	t.Run("attached ;;; is not surfaced as a comment", func(t *testing.T) {
		res, err := ParseWithComments(";;; Attached.\n(defn f [] nil)")
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if got, ok := res.Comments[1]; ok {
			t.Errorf("attached doc leaked into comments: %q", got)
		}
		if res.Nodes[0].(*ast.DefnDecl).Doc != "Attached." {
			t.Errorf("Doc not attached: %q", res.Nodes[0].(*ast.DefnDecl).Doc)
		}
	})
}

func TestTrailingCommentInBindings(t *testing.T) {
	// A trailing ; comment after a binding value used to fail with
	// "expected symbol, got comment" on the transpiler parse path (the
	// formatter filters comments first). The parser now skips comment tokens
	// between bindings in let/loop/let-or/with-open vectors.
	cases := []struct {
		name string
		src  string
	}{
		{"let", "(let [x 1 ; why\n y 2] (+ x y))"},
		{"let leading comment", "(let [; head\n x 1] x)"},
		{"loop", "(loop [i 0 ; counter\n acc 0] acc)"},
		{"let-or", "(let-or [a (f) 0 ; default\n b (g) 1] a)"},
		{"with-open", "(with-open [f (open) ; resource\n g (open)] f)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ParseString(c.src); err != nil {
				t.Errorf("parse error: %v", err)
			}
		})
	}
}

func TestUnclosedDelimiter(t *testing.T) {
	// Missing the final ) on a multi-line defn: error should point at the
	// opening ( on line 1, not the end of the file.
	_, err := ParseString("(defn foo []\n  (+ 1 2)")
	if err == nil {
		t.Fatal("expected parse error for unclosed delimiter")
	}
	msg := err.Error()
	for _, want := range []string{"unclosed", "1:1", `missing ")"`} {
		if !contains(msg, want) {
			t.Errorf("error %q should contain %q", msg, want)
		}
	}
}

func TestUnclosedBracket(t *testing.T) {
	// A vector literal that runs off the end of the input.
	_, err := ParseString("(def xs [1 2 3")
	if err == nil {
		t.Fatal("expected parse error for unclosed bracket")
	}
	if msg := err.Error(); !contains(msg, "unclosed") {
		t.Errorf("error %q should report an unclosed delimiter", msg)
	}
}

func TestStrayClosingDelimiter(t *testing.T) {
	_, err := ParseString("(+ 1 2))")
	if err == nil {
		t.Fatal("expected parse error for stray )")
	}
	if msg := err.Error(); !contains(msg, "no matching opening delimiter") {
		t.Errorf("error %q should report a stray closing delimiter", msg)
	}
}

func TestDidYouMeanExtended(t *testing.T) {
	cases := []struct {
		src  string
		hint string
	}{
		{"(let* [x 1] x)", "let"},
		{"(car xs)", "first"},
		{"(cdr xs)", "rest"},
		{"(cons 1 xs)", "conj"},
		{"(defrecord Point [x y])", "defstruct"},
	}
	for _, c := range cases {
		_, err := ParseString(c.src)
		if err == nil {
			t.Errorf("%q: expected parse error", c.src)
			continue
		}
		if !contains(err.Error(), c.hint) {
			t.Errorf("%q: error %q should contain hint %q", c.src, err.Error(), c.hint)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
