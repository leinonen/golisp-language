package macro

import (
	"testing"

	"golisp/internal/ast"
	"golisp/internal/parser"
)

// evalStr parses a single glisp form and evaluates it in a fresh global env.
func evalStr(t *testing.T, src string) Value {
	t.Helper()
	nodes, err := parser.ParseString(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if len(nodes) != 1 {
		t.Fatalf("want 1 form, got %d for %q", len(nodes), src)
	}
	v, err := Eval(nodes[0], NewGlobalEnv())
	if err != nil {
		t.Fatalf("eval %q: %v", src, err)
	}
	return v
}

func evalErr(t *testing.T, src string) error {
	t.Helper()
	nodes, err := parser.ParseString(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	_, err = Eval(nodes[0], NewGlobalEnv())
	return err
}

func wantInt(t *testing.T, v Value, want int64) {
	t.Helper()
	got, ok := v.(int64)
	if !ok {
		t.Fatalf("want int64, got %s (%v)", typeName(v), v)
	}
	if got != want {
		t.Errorf("got %d, want %d", got, want)
	}
}

func TestEvalArithmetic(t *testing.T) {
	wantInt(t, evalStr(t, "(+ 1 2 3)"), 6)
	wantInt(t, evalStr(t, "(- 10 3 2)"), 5)
	wantInt(t, evalStr(t, "(* 2 3 4)"), 24)
	wantInt(t, evalStr(t, "(/ 12 3 2)"), 2) // all-int division stays integral
	wantInt(t, evalStr(t, "(mod 7 3)"), 1)
	wantInt(t, evalStr(t, "(inc 41)"), 42)
	wantInt(t, evalStr(t, "(dec 1)"), 0)

	// int/float promotion
	if got, ok := evalStr(t, "(+ 1 2.5)").(float64); !ok || got != 3.5 {
		t.Errorf("(+ 1 2.5) = %v, want 3.5", evalStr(t, "(+ 1 2.5)"))
	}
}

func TestEvalComparisonAndLogic(t *testing.T) {
	cases := map[string]bool{
		"(< 1 2 3)":      true,
		"(< 1 3 2)":      false,
		"(>= 3 3 2)":     true,
		"(= 1 1 1)":      true,
		"(= 1 2)":        false,
		"(not= 1 2)":     true,
		"(not false)":    true,
		"(not nil)":      true,
		"(not 0)":        false, // 0 is truthy (ADR-011)
		"(and 1 2 true)": true,
		"(and 1 false)":  false,
		"(or false 7)":   true,
	}
	for src, want := range cases {
		got, ok := evalStr(t, src).(bool)
		if !ok {
			// `and`/`or` return the value, not always a bool — coerce via truthy
			got = truthy(evalStr(t, src))
		}
		if got != want {
			t.Errorf("%s = %v, want %v", src, got, want)
		}
	}
}

func TestEvalLetIfCond(t *testing.T) {
	wantInt(t, evalStr(t, "(let [x 2 y 3] (* x y))"), 6)
	wantInt(t, evalStr(t, "(let [x 2 y (+ x 1)] (+ x y))"), 5) // sequential binding
	wantInt(t, evalStr(t, "(if (< 1 2) 10 20)"), 10)
	wantInt(t, evalStr(t, "(if (> 1 2) 10 20)"), 20)
	wantInt(t, evalStr(t, "(when true 1 2 3)"), 3)
	if v := evalStr(t, "(when false 1)"); v != nil {
		t.Errorf("(when false 1) = %v, want nil", v)
	}
	wantInt(t, evalStr(t, "(cond false 1 true 2 :else 3)"), 2)
	wantInt(t, evalStr(t, "(cond false 1 false 2 :else 3)"), 3)
	wantInt(t, evalStr(t, "(do 1 2 (+ 1 2))"), 3)
}

func TestEvalClosures(t *testing.T) {
	wantInt(t, evalStr(t, "((fn [x] (* x x)) 5)"), 25)
	wantInt(t, evalStr(t, "(let [add (fn [a b] (+ a b))] (add 3 4))"), 7)
	// closure captures lexical env
	wantInt(t, evalStr(t, "(let [n 10] ((fn [x] (+ x n)) 5))"), 15)
	// rest params
	v := evalStr(t, "((fn [a & more] more) 1 2 3)")
	list, ok := v.(*List)
	if !ok || len(list.Items) != 2 {
		t.Fatalf("rest param: got %s %v", typeName(v), v)
	}
	wantInt(t, list.Items[0], 2)
}

func TestEvalHOF(t *testing.T) {
	v := evalStr(t, "(map (fn [x] (* x 2)) [1 2 3])")
	list, ok := v.(*List)
	if !ok || len(list.Items) != 3 {
		t.Fatalf("map: got %s %v", typeName(v), v)
	}
	wantInt(t, list.Items[0], 2)
	wantInt(t, list.Items[2], 6)

	v = evalStr(t, "(filter (fn [x] (> x 1)) [0 1 2 3])")
	list = v.(*List)
	if len(list.Items) != 2 {
		t.Errorf("filter len = %d, want 2", len(list.Items))
	}

	wantInt(t, evalStr(t, "(reduce (fn [a b] (+ a b)) 0 [1 2 3 4])"), 10)
	wantInt(t, evalStr(t, "(reduce (fn [a b] (+ a b)) [1 2 3 4])"), 10)
}

func TestEvalQuoteProducesData(t *testing.T) {
	v := evalStr(t, "'(if a b c)")
	list, ok := v.(*List)
	if !ok {
		t.Fatalf("quote: got %s, want list", typeName(v))
	}
	if len(list.Items) != 4 {
		t.Fatalf("quoted (if a b c) has %d items, want 4", len(list.Items))
	}
	head, ok := list.Items[0].(*Sym)
	if !ok || head.Name != "if" {
		t.Errorf("head = %v, want symbol if", list.Items[0])
	}
	// quoted symbols are Syms, not evaluated
	if s, ok := list.Items[1].(*Sym); !ok || s.Name != "a" {
		t.Errorf("item 1 = %v, want symbol a", list.Items[1])
	}

	// quoted vector / keyword / scalar
	if _, ok := evalStr(t, "'[1 2]").(*Vector); !ok {
		t.Errorf("'[1 2] should be a vector")
	}
	if kw, ok := evalStr(t, "':k").(Keyword); !ok || kw != "k" {
		t.Errorf("':k = %v, want keyword k", evalStr(t, "':k"))
	}
}

func TestEvalSeqBuiltins(t *testing.T) {
	wantInt(t, evalStr(t, "(first [10 20 30])"), 10)
	wantInt(t, evalStr(t, "(count [1 2 3 4])"), 4)
	wantInt(t, evalStr(t, "(nth [1 2 3] 1)"), 2)
	wantInt(t, evalStr(t, "(nth [1 2 3] 9 -1)"), -1)
	if Print(evalStr(t, "(conj '(1 2) 0)")) != "(0 1 2)" { // conj on a list prepends
		t.Errorf("conj list: %s", Print(evalStr(t, "(conj '(1 2) 0)")))
	}
	if Print(evalStr(t, "(concat '(1 2) '(3 4))")) != "(1 2 3 4)" {
		t.Errorf("concat: %s", Print(evalStr(t, "(concat '(1 2) '(3 4))")))
	}
	if Print(evalStr(t, "(reverse '(1 2 3))")) != "(3 2 1)" {
		t.Errorf("reverse: %s", Print(evalStr(t, "(reverse '(1 2 3))")))
	}
	if Print(evalStr(t, "(conj [1 2] 3)")) != "[1 2 3]" {
		t.Errorf("conj vector: %s", Print(evalStr(t, "(conj [1 2] 3)")))
	}
	if got := evalStr(t, "(empty? [])"); got != true {
		t.Errorf("(empty? []) = %v", got)
	}
}

func TestEvalPredicates(t *testing.T) {
	cases := map[string]bool{
		"(symbol? 'x)":    true,
		"(symbol? 1)":     false,
		"(keyword? :k)":   true,
		"(list? '(1))":    true,
		"(vector? [1])":   true,
		"(nil? nil)":      true,
		"(number? 1.5)":   true,
		"(string? \"s\")": true,
	}
	for src, want := range cases {
		if got := evalStr(t, src); got != want {
			t.Errorf("%s = %v, want %v", src, got, want)
		}
	}
}

func TestEvalSymbolsAndStr(t *testing.T) {
	s, ok := evalStr(t, `(symbol "foo")`).(*Sym)
	if !ok || s.Name != "foo" {
		t.Errorf("(symbol \"foo\") = %v", evalStr(t, `(symbol "foo")`))
	}
	if got := evalStr(t, `(name :hello)`); got != "hello" {
		t.Errorf("(name :hello) = %v", got)
	}
	if got := evalStr(t, `(str "a" 1 :b)`); got != "a1:b" {
		t.Errorf("str = %v, want a1:b", got)
	}
}

func TestEvalMaps(t *testing.T) {
	wantInt(t, evalStr(t, "(get {:a 1 :b 2} :b)"), 2)
	wantInt(t, evalStr(t, "(get {:a 1} :z 99)"), 99)
	wantInt(t, evalStr(t, "(get (assoc {:a 1} :b 2) :b)"), 2)
	wantInt(t, evalStr(t, "(count (keys {:a 1 :b 2}))"), 2)
}

func TestGensymUnique(t *testing.T) {
	a := evalStr(t, "(gensym)").(*Sym)
	b := evalStr(t, "(gensym)").(*Sym)
	if a.Name == b.Name {
		t.Errorf("gensym not unique: %s == %s", a.Name, b.Name)
	}
	p := evalStr(t, `(gensym "x")`).(*Sym)
	if p.Name[0] != 'x' {
		t.Errorf("gensym prefix not applied: %s", p.Name)
	}
}

func TestEvalErrors(t *testing.T) {
	if err := evalErr(t, "(undefined-symbol)"); err == nil {
		t.Error("expected unbound-symbol error")
	}
	if err := evalErr(t, "(fmt/println 1)"); err == nil {
		t.Error("expected qualified-symbol error")
	}
	if err := evalErr(t, "`(a b)"); err == nil {
		t.Error("expected syntax-quote-not-implemented error")
	}
}

func TestValueToNodeRoundTrip(t *testing.T) {
	// A generic list round-trips through value <-> node at the value level.
	v := evalStr(t, "'(foo 1 :k [2 3])")
	node, err := valueToNode(v, ast.Position{})
	if err != nil {
		t.Fatalf("valueToNode: %v", err)
	}
	back, err := nodeToValue(node)
	if err != nil {
		t.Fatalf("nodeToValue: %v", err)
	}
	if !equalValues(v, back) {
		t.Errorf("round-trip mismatch:\n in:  %s\n out: %s", Print(v), Print(back))
	}
}
