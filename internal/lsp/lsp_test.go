package lsp

import "testing"

// ── Diagnostics ──────────────────────────────────────────────────────────────

func TestDiagnostics_clean(t *testing.T) {
	src := `(defn ^int add [^int a ^int b] (+ a b))`
	if d := Diagnostics(src); d != nil {
		t.Errorf("expected nil diagnostics, got %v", d)
	}
}

func TestDiagnostics_lexError(t *testing.T) {
	// '@' is not a valid glisp character
	src := "(defn bad [] @x)"
	diags := Diagnostics(src)
	if len(diags) == 0 {
		t.Fatal("expected at least one diagnostic")
	}
	if diags[0].Severity != SeverityError {
		t.Errorf("expected error severity, got %d", diags[0].Severity)
	}
	if diags[0].Source != "glisp" {
		t.Errorf("expected source 'glisp', got %q", diags[0].Source)
	}
}

func TestDiagnostics_parseError(t *testing.T) {
	// Missing closing paren
	src := "(defn add [] (+ 1 2)"
	diags := Diagnostics(src)
	if len(diags) == 0 {
		t.Fatal("expected at least one diagnostic")
	}
	if diags[0].Severity != SeverityError {
		t.Errorf("expected error severity, got %d", diags[0].Severity)
	}
}

func TestDiagnostics_lineCol(t *testing.T) {
	// Error on line 2 (1-based) → LSP line 1 (0-based)
	src := "(def x 1)\n(def y @bad)"
	diags := Diagnostics(src)
	if len(diags) == 0 {
		t.Fatal("expected diagnostic")
	}
	if diags[0].Range.Start.Line != 1 {
		t.Errorf("expected LSP line 1, got %d", diags[0].Range.Start.Line)
	}
}

// ── errorToDiagnostic ─────────────────────────────────────────────────────────

func TestErrorToDiagnostic_basic(t *testing.T) {
	d := errorToDiagnostic("3:5: unexpected token ')'")
	if d.Range.Start.Line != 2 {
		t.Errorf("line: want 2, got %d", d.Range.Start.Line)
	}
	if d.Range.Start.Character != 4 {
		t.Errorf("char: want 4, got %d", d.Range.Start.Character)
	}
	if d.Message != "unexpected token ')'" {
		t.Errorf("message: %q", d.Message)
	}
}

func TestErrorToDiagnostic_withContext(t *testing.T) {
	// Parser errors include source context after a newline
	d := errorToDiagnostic("1:1: bad token\n  (foo)\n  ^")
	if d.Range.Start.Line != 0 {
		t.Errorf("line: want 0, got %d", d.Range.Start.Line)
	}
	if d.Message != "bad token" {
		t.Errorf("message: %q", d.Message)
	}
}

// ── Hover ─────────────────────────────────────────────────────────────────────

func TestHover_defn(t *testing.T) {
	// (defn foo [a b] (+ a b))
	// col:  0123456
	// "foo" starts at col 6
	src := "(defn foo [a b] (+ a b))"
	result := FindHover(src, 0, 6)
	if result == nil {
		t.Fatal("expected hover for 'foo'")
	}
	want := "(defn foo [a b])"
	if result.Contents != want {
		t.Errorf("want %q, got %q", want, result.Contents)
	}
}

func TestHover_defnWithTypes(t *testing.T) {
	// (defn ^int add [^int a ^int b] (+ a b))
	// col:  01234567890123
	// "add" at col 11
	src := "(defn ^int add [^int a ^int b] (+ a b))"
	result := FindHover(src, 0, 11)
	if result == nil {
		t.Fatal("expected hover for 'add'")
	}
	want := "(defn ^int add [^int a ^int b])"
	if result.Contents != want {
		t.Errorf("want %q, got %q", want, result.Contents)
	}
}

func TestHover_def(t *testing.T) {
	// (def ^int port 8080)
	// col:  0123456789012345
	// "port" at col 10
	src := "(def ^int port 8080)"
	result := FindHover(src, 0, 10)
	if result == nil {
		t.Fatal("expected hover for 'port'")
	}
	want := "(def ^int port)"
	if result.Contents != want {
		t.Errorf("want %q, got %q", want, result.Contents)
	}
}

func TestHover_miss_paren(t *testing.T) {
	src := "(defn foo [] nil)"
	// Cursor on '(' — not a symbol char
	result := FindHover(src, 0, 0)
	if result != nil {
		t.Errorf("expected nil hover for '(', got %v", result)
	}
}

func TestHover_miss_whitespace(t *testing.T) {
	src := "(defn foo [] nil)"
	// Cursor on space between 'defn' and 'foo'
	result := FindHover(src, 0, 5)
	if result != nil {
		t.Errorf("expected nil hover for whitespace, got %v", result)
	}
}

func TestHover_callSite(t *testing.T) {
	// Hover over a call to 'add' also works — same symbol table lookup
	src := "(defn add [a b] (+ a b))\n(def result (add 1 2))"
	// "add" in second line: (def result (add 1 2))
	// col:                   0123456789012345
	// "add" at col 13
	result := FindHover(src, 1, 13)
	if result == nil {
		t.Fatal("expected hover for call-site 'add'")
	}
	if result.Contents != "(defn add [a b])" {
		t.Errorf("unexpected contents: %q", result.Contents)
	}
}

// ── Built-in hover ───────────────────────────────────────────────────────────

func TestHover_builtin_map(t *testing.T) {
	src := "(map inc xs)"
	result := FindHover(src, 0, 1)
	if result == nil {
		t.Fatal("expected hover for built-in 'map'")
	}
	if result.Contents != builtinDocs["map"] {
		t.Errorf("unexpected contents: %q", result.Contents)
	}
}

func TestHover_builtin_thread(t *testing.T) {
	src := "(-> x inc str)"
	result := FindHover(src, 0, 1)
	if result == nil {
		t.Fatal("expected hover for '->'")
	}
}

func TestHover_builtin_ifErr(t *testing.T) {
	src := "(if-err [v e] expr e v)"
	result := FindHover(src, 0, 1)
	if result == nil {
		t.Fatal("expected hover for 'if-err'")
	}
}

func TestHover_builtin_json(t *testing.T) {
	// json/encode contains '/' which is a symbol rune
	src := "(json/encode x)"
	result := FindHover(src, 0, 1)
	if result == nil {
		t.Fatal("expected hover for 'json/encode'")
	}
}

func TestHover_userDefn_overrides_builtin(t *testing.T) {
	// A user-defined 'map' should shadow the built-in entry.
	src := "(defn map [f xs] nil)"
	result := FindHover(src, 0, 6)
	if result == nil {
		t.Fatal("expected hover")
	}
	if result.Contents == builtinDocs["map"] {
		t.Error("user defn should override built-in")
	}
}

// ── symbolAtPosition ──────────────────────────────────────────────────────────

func TestSymbolAtPosition(t *testing.T) {
	cases := []struct {
		src  string
		line int
		col  int
		want string
	}{
		{"(defn foo [])", 0, 6, "foo"},
		{"(defn foo [])", 0, 7, "foo"},
		{"(defn foo [])", 0, 8, "foo"},
		{"(defn foo [])", 0, 0, ""},   // '('
		{"(defn foo [])", 0, 5, ""},   // ' '
		{"(defn foo [])", 0, 9, ""},   // ' '
		{"hello world", 0, 6, "world"},
		{"hello world", 1, 0, ""},     // line out of range
	}
	for _, c := range cases {
		got := symbolAtPosition(c.src, c.line, c.col)
		if got != c.want {
			t.Errorf("symbolAtPosition(%q, %d, %d) = %q, want %q",
				c.src, c.line, c.col, got, c.want)
		}
	}
}
