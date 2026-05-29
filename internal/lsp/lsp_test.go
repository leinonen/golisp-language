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
	if result.Sig != want {
		t.Errorf("want %q, got %q", want, result.Sig)
	}
}

func TestHover_defnDocString(t *testing.T) {
	src := `(defn greet [name] "Greet a person by name." (str "Hi " name))`
	result := FindHover(src, 0, 6)
	if result == nil {
		t.Fatal("expected hover for 'greet'")
	}
	if result.Sig != "(defn greet [name])" {
		t.Errorf("unexpected sig: %q", result.Sig)
	}
	if result.Doc != "Greet a person by name." {
		t.Errorf("unexpected doc: %q", result.Doc)
	}
}

func TestHover_defnLoneStringNotDoc(t *testing.T) {
	// A defn whose sole body form is a string — should NOT be treated as a doc string.
	src := `(defn greeting [] "Hello, World!")`
	result := FindHover(src, 0, 6)
	if result == nil {
		t.Fatal("expected hover for 'greeting'")
	}
	if result.Doc != "" {
		t.Errorf("lone string should not become doc, got: %q", result.Doc)
	}
}

func TestHover_builtinDoc(t *testing.T) {
	src := "(map inc xs)"
	result := FindHover(src, 0, 1)
	if result == nil {
		t.Fatal("expected hover for built-in 'map'")
	}
	if result.Doc == "" {
		t.Error("expected non-empty doc for 'map'")
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
	if result.Sig != want {
		t.Errorf("want %q, got %q", want, result.Sig)
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
	if result.Sig != want {
		t.Errorf("want %q, got %q", want, result.Sig)
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
	if result.Sig != "(defn add [a b])" {
		t.Errorf("unexpected contents: %q", result.Sig)
	}
}

// ── Built-in hover ───────────────────────────────────────────────────────────

func TestHover_builtin_map(t *testing.T) {
	src := "(map inc xs)"
	result := FindHover(src, 0, 1)
	if result == nil {
		t.Fatal("expected hover for built-in 'map'")
	}
	if result.Sig != builtinDocs["map"].Sig {
		t.Errorf("unexpected sig: %q", result.Sig)
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

func TestHover_stdlibTypes(t *testing.T) {
	// (defn ^stdlib/Response health [^stdlib/Request req]
	// col:  0    5 7              22     30 32
	//                col 7: 's' of stdlib/Response (right after ^)
	//                                          col 32: 's' of stdlib/Request (right after ^)
	src := "(defn ^stdlib/Response health [^stdlib/Request req])"
	for _, tc := range []struct {
		col  int
		want string
	}{
		{7, "stdlib/Response"}, // cursor on 's' of stdlib/Response (after ^)
		{32, "stdlib/Request"}, // cursor on 's' of stdlib/Request (after ^)
	} {
		name := symbolAtPosition(src, 0, tc.col)
		if name != tc.want {
			t.Errorf("col %d: symbolAtPosition want %q, got %q", tc.col, tc.want, name)
		}
		result := FindHover(src, 0, tc.col)
		if result == nil {
			t.Fatalf("col %d: expected hover for %q", tc.col, tc.want)
		}
		if result.Sig == "" {
			t.Errorf("col %d: expected non-empty sig for %q", tc.col, tc.want)
		}
	}
}

func TestHover_userDefn_overrides_builtin(t *testing.T) {
	// A user-defined 'map' should shadow the built-in entry.
	src := "(defn map [f xs] nil)"
	result := FindHover(src, 0, 6)
	if result == nil {
		t.Fatal("expected hover")
	}
	if result.Sig == builtinDocs["map"].Sig {
		t.Error("user defn should override built-in")
	}
}

// ── symbolAtPosition ──────────────────────────────────────────────────────────

// ── Definition ───────────────────────────────────────────────────────────────

func TestDefinition_callSite(t *testing.T) {
	src := "(defn add [a b] (+ a b))\n(add 1 2)"
	// "add" at line 1, col 1
	r := FindDefinition(src, 1, 1)
	if r == nil {
		t.Fatal("expected definition range")
	}
	// defn starts at line 0, col 0 (the opening '(')
	if r.Start.Line != 0 || r.Start.Character != 0 {
		t.Errorf("want {0,0}, got {%d,%d}", r.Start.Line, r.Start.Character)
	}
}

func TestDefinition_def(t *testing.T) {
	src := "(def port 8080)\n(println port)"
	// "port" at line 1, col 10: "(println port)"  0123456789012
	r := FindDefinition(src, 1, 10)
	if r == nil {
		t.Fatal("expected definition range")
	}
	if r.Start.Line != 0 || r.Start.Character != 0 {
		t.Errorf("want {0,0}, got {%d,%d}", r.Start.Line, r.Start.Character)
	}
}

func TestDefinition_unknown(t *testing.T) {
	src := "(defn foo [] bar)"
	// "bar" at col 13 — not defined
	r := FindDefinition(src, 0, 13)
	if r != nil {
		t.Errorf("expected nil for unknown symbol, got %v", r)
	}
}

func TestDefinition_builtin(t *testing.T) {
	src := "(map inc xs)"
	// "map" at col 1 — builtin has no source location
	r := FindDefinition(src, 0, 1)
	if r != nil {
		t.Errorf("expected nil for builtin, got %v", r)
	}
}

func TestDefinition_multiLine(t *testing.T) {
	src := "(def a 1)\n(def b 2)\n(defn add [x y] (+ x y))\n(add a b)"
	// "add" at line 3, col 1
	r := FindDefinition(src, 3, 1)
	if r == nil {
		t.Fatal("expected definition range")
	}
	// defn is on line 2 (0-based)
	if r.Start.Line != 2 {
		t.Errorf("want line 2, got %d", r.Start.Line)
	}
}

// ── Completions ───────────────────────────────────────────────────────────────

func TestCompletions_includesUserDefs(t *testing.T) {
	src := "(defn add [a b] (+ a b))\n(def port 8080)"
	items := FindCompletions(src, 0, 0) // col 0 on '(' → empty prefix → all
	labels := map[string]bool{}
	for _, item := range items {
		labels[item.Label] = true
	}
	if !labels["add"] {
		t.Error("expected 'add' in completions")
	}
	if !labels["port"] {
		t.Error("expected 'port' in completions")
	}
}

func TestCompletions_includesBuiltins(t *testing.T) {
	src := "(defn foo [] nil)"
	items := FindCompletions(src, 0, 0)
	labels := map[string]bool{}
	for _, item := range items {
		labels[item.Label] = true
	}
	if !labels["map"] {
		t.Error("expected builtin 'map' in completions")
	}
	if !labels["filter"] {
		t.Error("expected builtin 'filter' in completions")
	}
}

func TestCompletions_prefix(t *testing.T) {
	src := "(defn foo [] nil)\n(defn foobar [] nil)"
	// col 8 in "(defn foo..." → runes[6]='f' runes[7]='o', end=8 → prefix "fo"
	items := FindCompletions(src, 0, 8)
	labels := map[string]bool{}
	for _, item := range items {
		labels[item.Label] = true
	}
	if !labels["foo"] {
		t.Error("expected 'foo' with prefix 'fo'")
	}
	if !labels["foobar"] {
		t.Error("expected 'foobar' with prefix 'fo'")
	}
	if labels["bar"] {
		t.Error("'bar' should not appear with prefix 'fo'")
	}
}

func TestCompletions_noMatch(t *testing.T) {
	// "(zzzz" fails to parse → no user defs; no builtin starts with "zzzz"
	src := "(defn foo [] nil)\n(zzzz"
	items := FindCompletions(src, 1, 5) // col 5 on "(zzzz" → prefix "zzzz"
	if len(items) != 0 {
		t.Errorf("expected 0 completions for prefix 'zzzz', got %d", len(items))
	}
}

func TestCompletions_kinds(t *testing.T) {
	src := "(defn add [a b] (+ a b))\n(def port 8080)\n(defstruct User ^string name)"
	items := FindCompletions(src, 0, 0)
	kindFor := map[string]int{}
	for _, item := range items {
		kindFor[item.Label] = item.Kind
	}
	if kindFor["add"] != 3 {
		t.Errorf("defn kind: want 3, got %d", kindFor["add"])
	}
	if kindFor["port"] != 6 {
		t.Errorf("def kind: want 6, got %d", kindFor["port"])
	}
	if kindFor["User"] != 22 {
		t.Errorf("defstruct kind: want 22, got %d", kindFor["User"])
	}
}

func TestCompletions_userOverridesBuiltin(t *testing.T) {
	src := "(defn map [f xs] nil)"
	items := FindCompletions(src, 0, 0)
	var mapItem *CompletionItem
	for i := range items {
		if items[i].Label == "map" {
			mapItem = &items[i]
			break
		}
	}
	if mapItem == nil {
		t.Fatal("expected 'map' in completions")
	}
	if mapItem.Kind == 14 {
		t.Error("user-defined 'map' should not have builtin kind 14")
	}
}

func TestCompletions_sorted(t *testing.T) {
	src := "(defn zoo [] nil)\n(defn abc [] nil)"
	items := FindCompletions(src, 0, 0)
	abcIdx, zooIdx := -1, -1
	for i, item := range items {
		if item.Label == "abc" {
			abcIdx = i
		}
		if item.Label == "zoo" {
			zooIdx = i
		}
	}
	if abcIdx == -1 {
		t.Fatal("'abc' not found")
	}
	if zooIdx == -1 {
		t.Fatal("'zoo' not found")
	}
	if abcIdx > zooIdx {
		t.Errorf("expected 'abc' before 'zoo', got abc=%d zoo=%d", abcIdx, zooIdx)
	}
}

func TestCompletions_detail(t *testing.T) {
	src := "(defn add [^int a ^int b] (+ a b))"
	items := FindCompletions(src, 0, 0)
	for _, item := range items {
		if item.Label == "add" {
			if item.Detail == "" {
				t.Error("expected non-empty detail for 'add'")
			}
			return
		}
	}
	t.Error("'add' not found in completions")
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
