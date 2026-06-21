package lsp

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ── Diagnostics ──────────────────────────────────────────────────────────────

func TestDiagnostics_clean(t *testing.T) {
	src := `(defn add [a int b int] -> int (+ a b))`
	if d := Diagnostics(src, ""); d != nil {
		t.Errorf("expected nil diagnostics, got %v", d)
	}
}

func TestDiagnostics_lexError(t *testing.T) {
	// '@' is not a valid glisp character
	src := "(defn bad [] @x)"
	diags := Diagnostics(src, "")
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
	diags := Diagnostics(src, "")
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
	diags := Diagnostics(src, "")
	if len(diags) == 0 {
		t.Fatal("expected diagnostic")
	}
	if diags[0].Range.Start.Line != 1 {
		t.Errorf("expected LSP line 1, got %d", diags[0].Range.Start.Line)
	}
}

func TestDiagnostics_transpileError(t *testing.T) {
	// panic with wrong arg count — transpiler catches this
	src := "(defn bad [] (panic 1 2 3))"
	diags := Diagnostics(src, "")
	if len(diags) == 0 {
		t.Fatal("expected transpile diagnostic")
	}
	if diags[0].Severity != SeverityError {
		t.Errorf("expected error severity, got %d", diags[0].Severity)
	}
	if diags[0].Source != "glisp" {
		t.Errorf("expected source 'glisp', got %q", diags[0].Source)
	}
}

func TestAtPosToDiagnostic_withPos(t *testing.T) {
	d := atPosToDiagnostic("unsupported expression: *ast.Foo at 5:3")
	if d.Range.Start.Line != 4 {
		t.Errorf("line: want 4, got %d", d.Range.Start.Line)
	}
	if d.Range.Start.Character != 2 {
		t.Errorf("char: want 2, got %d", d.Range.Start.Character)
	}
	if d.Message != "unsupported expression: *ast.Foo" {
		t.Errorf("message: %q", d.Message)
	}
}

func TestAtPosToDiagnostic_noPos(t *testing.T) {
	d := atPosToDiagnostic("some error without position")
	if d.Range.Start.Line != 0 || d.Range.Start.Character != 0 {
		t.Errorf("expected 0:0, got %d:%d", d.Range.Start.Line, d.Range.Start.Character)
	}
	if d.Message != "some error without position" {
		t.Errorf("message: %q", d.Message)
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

func TestHover_defnMultiLineDocString(t *testing.T) {
	src := ";;; First line.\n;;; Second line.\n(defn greet [name] (str \"Hi \" name))"
	result := FindHover(src, 2, 6)
	if result == nil {
		t.Fatal("expected hover for 'greet'")
	}
	if result.Doc != "First line.\nSecond line." {
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
	// (defn add [a int b int] -> int (+ a b))
	// col:  012345678901
	// "add" at col 6
	src := "(defn add [a int b int] -> int (+ a b))"
	result := FindHover(src, 0, 6)
	if result == nil {
		t.Fatal("expected hover for 'add'")
	}
	want := "(defn add [a int b int] -> int)"
	if result.Sig != want {
		t.Errorf("want %q, got %q", want, result.Sig)
	}
}

func TestHover_def(t *testing.T) {
	// (def port int 8080)
	// col:  0123456789
	// "port" at col 5
	src := "(def port int 8080)"
	result := FindHover(src, 0, 5)
	if result == nil {
		t.Fatal("expected hover for 'port'")
	}
	want := "(def port int)"
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
	if result.Sig != BuiltinDocs["map"].Sig {
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

func TestHover_builtin_ifLet(t *testing.T) {
	src := "(if-let [u (find x)] u nil)"
	result := FindHover(src, 0, 1)
	if result == nil {
		t.Fatal("expected hover for 'if-let'")
	}
}

func TestHover_builtin_whenLet(t *testing.T) {
	src := "(when-let [u (find x)] u)"
	result := FindHover(src, 0, 1)
	if result == nil {
		t.Fatal("expected hover for 'when-let'")
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

func TestHover_webTypes(t *testing.T) {
	// (defn health [req web/Request] -> web/Response)
	// col:  0         1         2         3         4
	//       0123456789012345678901234567890123456789012
	//                    col 18: 'w' of web/Request
	//                                        col 36: 'w' of web/Response
	src := "(defn health [req web/Request] -> web/Response)"
	for _, tc := range []struct {
		col  int
		want string
	}{
		{18, "web/Request"},  // cursor on 'w' of web/Request in params
		{34, "web/Response"}, // cursor on 'w' of web/Response after ->
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
	if result.Sig == BuiltinDocs["map"].Sig {
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

func TestDefinition_crossFile_openDocs(t *testing.T) {
	fileA := "file:///project/a.glsp"
	fileB := "file:///project/b.glsp"
	srcA := "(ns main)\n(helper 42)"
	srcB := "(ns main)\n(defn helper [x] x)"

	s := NewServer()
	s.docs[fileA] = srcA
	s.docs[fileB] = srcB

	name := symbolAtPosition(srcA, 1, 1)
	if name != "helper" {
		t.Fatalf("expected symbol 'helper', got %q", name)
	}
	r := FindDeclByName(srcB, name)
	if r == nil {
		t.Fatal("expected to find 'helper' in file B")
	}
	if r.Start.Line != 1 {
		t.Errorf("want line 1 (0-based), got %d", r.Start.Line)
	}
}

func TestDefinition_crossFile_filesystem(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.glsp")
	fileB := filepath.Join(dir, "b.glsp")
	if err := os.WriteFile(fileA, []byte("(ns main)\n(helper 42)"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte("(ns main)\n(defn helper [x] x)"), 0644); err != nil {
		t.Fatal(err)
	}

	uriA := "file://" + fileA
	s := NewServer()
	s.docs[uriA] = "(ns main)\n(helper 42)" // only A is open

	loc := s.searchSiblingFiles(uriA, "helper")
	if loc == nil {
		t.Fatal("expected to find 'helper' in sibling file on disk")
	}
	if loc.URI != "file://"+fileB {
		t.Errorf("unexpected URI: %s", loc.URI)
	}
	if loc.Range.Start.Line != 1 {
		t.Errorf("want line 1, got %d", loc.Range.Start.Line)
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
	src := "(defn add [a b] (+ a b))\n(def port 8080)\n(defstruct User name string)"
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

// ── Rename ────────────────────────────────────────────────────────────────────

func TestRename_basic(t *testing.T) {
	src := "(defn add [a b] (+ a b))\n(add 1 2)"
	// cursor on "add" at line 0, col 6
	edits := FindRenameEdits(src, 0, 6, "sum")
	if len(edits) != 2 {
		t.Fatalf("expected 2 edits, got %d", len(edits))
	}
	// first edit: "add" in defn line
	if edits[0].Range.Start.Line != 0 || edits[0].Range.Start.Character != 6 {
		t.Errorf("edit[0] start: want {0,6}, got {%d,%d}", edits[0].Range.Start.Line, edits[0].Range.Start.Character)
	}
	if edits[0].NewText != "sum" {
		t.Errorf("edit[0] newText: want %q, got %q", "sum", edits[0].NewText)
	}
	// second edit: "add" in call line
	if edits[1].Range.Start.Line != 1 || edits[1].Range.Start.Character != 1 {
		t.Errorf("edit[1] start: want {1,1}, got {%d,%d}", edits[1].Range.Start.Line, edits[1].Range.Start.Character)
	}
}

func TestRename_noSymbol(t *testing.T) {
	src := "(defn foo [] nil)"
	// cursor on '(' — not a symbol
	edits := FindRenameEdits(src, 0, 0, "bar")
	if edits != nil {
		t.Errorf("expected nil edits for non-symbol position, got %v", edits)
	}
}

func TestRename_noPartialMatch(t *testing.T) {
	// "add" should not match "addTwo"
	src := "(defn add [] nil)\n(defn addTwo [] nil)\n(add 1)"
	edits := FindRenameEdits(src, 0, 6, "sum")
	if len(edits) != 2 {
		t.Fatalf("expected 2 edits (add, not addTwo), got %d", len(edits))
	}
}

func TestRename_rangeEnd(t *testing.T) {
	src := "(defn foo [] nil)"
	edits := FindRenameEdits(src, 0, 6, "bar")
	if len(edits) != 1 {
		t.Fatalf("expected 1 edit, got %d", len(edits))
	}
	// "foo" is at cols 6-8, end should be 9
	if edits[0].Range.End.Character != 9 {
		t.Errorf("edit end char: want 9, got %d", edits[0].Range.End.Character)
	}
}

func TestCompletions_detail(t *testing.T) {
	src := "(defn add [a int b int] (+ a b))"
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
		{"(defn foo [])", 0, 0, ""}, // '('
		{"(defn foo [])", 0, 5, ""}, // ' '
		{"(defn foo [])", 0, 9, ""}, // ' '
		{"hello world", 0, 6, "world"},
		{"hello world", 1, 0, ""}, // line out of range
	}
	for _, c := range cases {
		got := symbolAtPosition(c.src, c.line, c.col)
		if got != c.want {
			t.Errorf("symbolAtPosition(%q, %d, %d) = %q, want %q",
				c.src, c.line, c.col, got, c.want)
		}
	}
}

func TestDiagnostics_transpileErrorLine(t *testing.T) {
	// panic on line 4 (1-based) → LSP line 3 (0-based)
	src := "(ns main)\n\n(defn bad []\n  (panic 1 2 3))"
	diags := Diagnostics(src, "/fake/file.glsp")
	if len(diags) == 0 {
		t.Fatal("expected diagnostic")
	}
	t.Logf("diag: line=%d char=%d msg=%q", diags[0].Range.Start.Line, diags[0].Range.Start.Character, diags[0].Message)
	if diags[0].Range.Start.Line != 3 {
		t.Errorf("want LSP line 3 (source line 4), got %d", diags[0].Range.Start.Line)
	}
}

func TestDiagnostics_transpileError_exactMsg(t *testing.T) {
	// Check the exact error message and position for a known transpile error
	// "panic" on line 5 col 3 of a file with specific indentation
	src := "(ns main)\n\n(defn bad []\n  ; comment\n  (panic 1 2))"
	// line 5 = "  (panic 1 2))"
	diags := Diagnostics(src, "/tmp/myfile.glsp")
	if len(diags) == 0 {
		t.Fatal("expected diagnostic")
	}
	d := diags[0]
	t.Logf("line=%d (expect 4), char=%d, msg=%q", d.Range.Start.Line, d.Range.Start.Character, d.Message)
	// (panic ...) is on source line 5 (1-based) → LSP line 4 (0-based)
	if d.Range.Start.Line != 4 {
		t.Errorf("want LSP line 4 (source line 5), got %d", d.Range.Start.Line)
	}
}

// ── References ────────────────────────────────────────────────────────────────

func TestReferences_basic(t *testing.T) {
	src := "(defn add [a b] (+ a b))\n(add 1 2)\n(add 3 4)"
	// cursor on "add" in the defn (line 0, col 6)
	refs := FindReferences(src, 0, 6)
	if len(refs) != 3 {
		t.Fatalf("expected 3 references (decl + 2 calls), got %d: %+v", len(refs), refs)
	}
	if refs[0].Start.Line != 0 || refs[0].Start.Character != 6 {
		t.Errorf("ref[0] start: want {0,6}, got {%d,%d}", refs[0].Start.Line, refs[0].Start.Character)
	}
	if refs[1].Start.Line != 1 || refs[2].Start.Line != 2 {
		t.Errorf("call refs on wrong lines: %d, %d", refs[1].Start.Line, refs[2].Start.Line)
	}
}

func TestReferences_skipsCommentLines(t *testing.T) {
	src := "(defn add [a b] (+ a b))\n; add is great\n(add 1 2)"
	refs := FindReferences(src, 0, 6)
	if len(refs) != 2 {
		t.Fatalf("expected 2 references (comment line skipped), got %d: %+v", len(refs), refs)
	}
	for _, r := range refs {
		if r.Start.Line == 1 {
			t.Errorf("reference found on comment line 1: %+v", r)
		}
	}
}

func TestReferences_noPartialMatch(t *testing.T) {
	src := "(defn add [] nil)\n(addTwo)\n(add)"
	refs := FindReferences(src, 0, 6)
	if len(refs) != 2 {
		t.Fatalf("expected 2 references (add, not addTwo), got %d", len(refs))
	}
}

func TestReferences_noSymbol(t *testing.T) {
	src := "(defn foo [] nil)"
	if refs := FindReferences(src, 0, 0); refs != nil {
		t.Errorf("expected nil for non-symbol position, got %+v", refs)
	}
}

func TestReferences_crossFileOpenDocs(t *testing.T) {
	fileA := "file:///project/a.glsp"
	fileB := "file:///project/b.glsp"
	s := NewServer()
	s.docs[fileA] = "(ns main)\n(defn helper [x] x)"
	s.docs[fileB] = "(ns main)\n(helper 1)\n(helper 2)"

	params, _ := json.Marshal(ReferenceParams{
		TextDocument: TextDocumentIdentifier{URI: fileA},
		Position:     Position{Line: 1, Character: 6}, // "helper" in the defn
	})
	resp := s.handleReferences(&Request{Params: params})
	locs, ok := resp.Result.([]Location)
	if !ok {
		t.Fatalf("result is %T, want []Location", resp.Result)
	}
	// 1 decl in A + 2 calls in B = 3
	if len(locs) != 3 {
		t.Fatalf("expected 3 cross-file references, got %d: %+v", len(locs), locs)
	}
	var inA, inB int
	for _, l := range locs {
		switch l.URI {
		case fileA:
			inA++
		case fileB:
			inB++
		}
	}
	if inA != 1 || inB != 2 {
		t.Errorf("want 1 ref in A and 2 in B, got %d in A, %d in B", inA, inB)
	}
}

// ── Document symbols ──────────────────────────────────────────────────────────

func TestDocumentSymbols_basic(t *testing.T) {
	src := "(ns main)\n(def pi 3.14)\n(defn add [a int b int] -> int (+ a b))\n(defstruct Point x int y int)"
	syms := DocumentSymbols(src)
	if len(syms) != 4 {
		t.Fatalf("expected 4 symbols, got %d: %+v", len(syms), syms)
	}
	want := []struct {
		name string
		kind int
	}{
		{"main", SymbolModule},
		{"pi", SymbolVariable},
		{"add", SymbolFunction},
		{"Point", SymbolStruct},
	}
	for i, w := range want {
		if syms[i].Name != w.name || syms[i].Kind != w.kind {
			t.Errorf("sym[%d]: got (%q,%d), want (%q,%d)", i, syms[i].Name, syms[i].Kind, w.name, w.kind)
		}
	}
	// defn detail should carry the signature
	if syms[2].Detail == "" {
		t.Errorf("expected a detail/signature for defn add")
	}
}

func TestDocumentSymbols_selectionRangeOnName(t *testing.T) {
	src := "(defn greet [name string] -> string name)"
	syms := DocumentSymbols(src)
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(syms))
	}
	sel := syms[0].SelectionRange
	// "greet" starts at column 6 (0-based) on line 0
	if sel.Start.Line != 0 || sel.Start.Character != 6 || sel.End.Character != 11 {
		t.Errorf("selectionRange: got %+v, want name span [0,6)-[0,11)", sel)
	}
	// selectionRange must be contained within range
	r := syms[0].Range
	if sel.Start.Character < r.Start.Character || sel.End.Character > r.End.Character {
		t.Errorf("selectionRange %+v not contained in range %+v", sel, r)
	}
}

func TestDocumentSymbols_methodAndTest(t *testing.T) {
	src := "(defmethod *Circle Area [c] -> float64 1.0)\n(deftest my-test (assert= 1 1))"
	syms := DocumentSymbols(src)
	if len(syms) != 2 {
		t.Fatalf("expected 2 symbols, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "Area" || syms[0].Kind != SymbolMethod {
		t.Errorf("sym[0]: got (%q,%d), want (Area,%d)", syms[0].Name, syms[0].Kind, SymbolMethod)
	}
	if syms[1].Name != "my-test" || syms[1].Kind != SymbolFunction {
		t.Errorf("sym[1]: got (%q,%d), want (my-test,%d)", syms[1].Name, syms[1].Kind, SymbolFunction)
	}
}

func TestDocumentSymbols_parseErrorReturnsNil(t *testing.T) {
	if syms := DocumentSymbols("(defn broken ["); syms != nil {
		t.Errorf("expected nil on parse error, got %+v", syms)
	}
}

// TestHover_goPackage covers Go-package interop hover (ADR-015 / Phase 12f):
// the server loads a referenced stdlib package's signatures and reports the
// real Go signature for a `pkg/fn` symbol.
func TestHover_goPackage(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module hovertest\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	src := "(ns main)\n(defn f [s string] -> string (strings/to-upper s))"
	uri := "file://" + filepath.Join(dir, "main.glsp")

	s := NewServer()
	s.docs[uri] = src

	g := s.goSignatures(uri, src)
	sig, ok := g.Signature("strings/to-upper")
	if !ok {
		t.Fatal("expected a Go signature for strings/to-upper")
	}
	if !strings.Contains(sig, "ToUpper") {
		t.Errorf("signature %q should mention ToUpper", sig)
	}

	// Second call hits the cache (same instance).
	if g2 := s.goSignatures(uri, src); g2 != g {
		t.Error("expected goSignatures to be cached per dir+paths")
	}
}
