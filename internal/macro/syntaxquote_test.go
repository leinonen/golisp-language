package macro

import (
	"strings"
	"testing"
)

func TestSyntaxQuotePlain(t *testing.T) {
	// A plain syntax-quote with no unquotes is just quoted data.
	if got := Print(evalStr(t, "`(if a b c)")); got != "(if a b c)" {
		t.Errorf("plain syntax-quote = %s, want (if a b c)", got)
	}
	if got := Print(evalStr(t, "`[1 2 3]")); got != "[1 2 3]" {
		t.Errorf("syntax-quote vector = %s", got)
	}
}

func TestSyntaxQuoteUnquote(t *testing.T) {
	// ~x evaluates x and inserts the value.
	if got := Print(evalStr(t, "(let [x 5] `(a ~x c))")); got != "(a 5 c)" {
		t.Errorf("unquote = %s, want (a 5 c)", got)
	}
	// ~ of a compound expression
	if got := Print(evalStr(t, "(let [x 2] `(val ~(+ x 3)))")); got != "(val 5)" {
		t.Errorf("unquote expr = %s, want (val 5)", got)
	}
	// unquote inside a vector
	if got := Print(evalStr(t, "(let [x 9] `[~x ~x])")); got != "[9 9]" {
		t.Errorf("unquote in vector = %s", got)
	}
}

func TestSyntaxQuoteSplice(t *testing.T) {
	// ~@xs splices the elements of xs into the surrounding sequence.
	if got := Print(evalStr(t, "(let [xs '(1 2 3)] `(a ~@xs b))")); got != "(a 1 2 3 b)" {
		t.Errorf("splice = %s, want (a 1 2 3 b)", got)
	}
	// splice at the head position
	if got := Print(evalStr(t, "(let [xs '(f x)] `(~@xs y))")); got != "(f x y)" {
		t.Errorf("splice head = %s, want (f x y)", got)
	}
	// splice a vector into a vector
	if got := Print(evalStr(t, "(let [xs [1 2]] `[0 ~@xs 3])")); got != "[0 1 2 3]" {
		t.Errorf("splice vector = %s", got)
	}
	// splice of a non-sequence is an error
	if err := evalErr(t, "(let [x 1] `(a ~@x))"); err == nil {
		t.Error("expected splice-non-sequence error")
	}
}

func TestSyntaxQuoteNested(t *testing.T) {
	// A realistic macro-template shape: building a (when cond body...) form.
	got := Print(evalStr(t, "(let [c '(> n 0) body '((println n))] `(when ~c ~@body))"))
	if got != "(when (> n 0) (println n))" {
		t.Errorf("template = %s", got)
	}
}

func TestSyntaxQuoteAutoGensym(t *testing.T) {
	// foo# resolves to one consistent fresh symbol within a single template...
	v := evalStr(t, "`(let [x# 1] (+ x# x#))")
	out := Print(v)
	// extract the three symbol occurrences: the binding and two uses must match.
	if strings.Count(out, "__auto") != 3 {
		t.Fatalf("expected 3 auto-gensym occurrences, got: %s", out)
	}
	list := v.(*List)
	bindVec := list.Items[1].(*Vector)
	name := bindVec.Items[0].(*Sym).Name
	body := list.Items[2].(*List)
	use1 := body.Items[1].(*Sym).Name
	use2 := body.Items[2].(*Sym).Name
	if name != use1 || name != use2 {
		t.Errorf("auto-gensym not consistent within template: %s / %s / %s", name, use1, use2)
	}
	if !strings.HasPrefix(name, "x__") || !strings.HasSuffix(name, "__auto") {
		t.Errorf("auto-gensym name shape unexpected: %s", name)
	}

	// ...but two separate templates get distinct gensyms.
	a := evalStr(t, "`x#").(*Sym).Name
	b := evalStr(t, "`x#").(*Sym).Name
	if a == b {
		t.Errorf("auto-gensym not unique across templates: %s == %s", a, b)
	}
}
