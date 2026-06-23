package lexer

import (
	"strings"
	"testing"
)

func TestShebangSkipped(t *testing.T) {
	// A leading "#!" line is skipped; the first real token is the '(' on line 2.
	toks, err := Tokenize("#!/usr/bin/env glisp\n(def x 1)")
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	if toks[0].Type != TokenLParen {
		t.Fatalf("first token = %v, want LParen", toks[0].Type)
	}
	if toks[0].Line != 2 {
		t.Errorf("'(' line = %d, want 2 (shebang occupies line 1)", toks[0].Line)
	}
	// "#!" only counts as a shebang at the very start; elsewhere "#" keeps its
	// normal meaning (it is a symbol character), so it is not silently skipped.
	toks2, err := Tokenize("(def x 1)\n#!nope")
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	found := false
	for _, tk := range toks2 {
		if tk.Type == TokenSymbol && strings.Contains(tk.Text, "#!") {
			found = true
		}
	}
	if !found {
		t.Error("a mid-file #! should lex as a symbol, not be skipped as a shebang")
	}
}

func TestTokenizeBasic(t *testing.T) {
	tests := []struct {
		input    string
		wantType TokenType
		wantText string
	}{
		{"(", TokenLParen, "("},
		{")", TokenRParen, ")"},
		{"[", TokenLBracket, "["},
		{"]", TokenRBracket, "]"},
		{"{", TokenLBrace, "{"},
		{"}", TokenRBrace, "}"},
		{"#{", TokenHashLBrace, "#{"},
		{"'", TokenQuote, "'"},
		{"`", TokenSyntaxQuote, "`"},
		{"~", TokenUnquote, "~"},
		{"~@", TokenUnquoteSplice, "~@"},
		{"nil", TokenNil, "nil"},
		{"true", TokenTrue, "true"},
		{"false", TokenFalse, "false"},
		{"42", TokenInt, "42"},
		{"-7", TokenInt, "-7"},
		{"3.14", TokenFloat, "3.14"},
		{`"hello"`, TokenString, "hello"},
		{`"a\nb"`, TokenString, "a\nb"},
		{":foo", TokenKeyword, "foo"},
		{":my-key", TokenKeyword, "my-key"},
		{"foo", TokenSymbol, "foo"},
		{"my-func", TokenSymbol, "my-func"},
		{"fmt/Println", TokenSymbol, "fmt/Println"},
		{"nil?", TokenSymbol, "nil?"},
		{"send!", TokenSymbol, "send!"},
		{"$", TokenSymbol, "$"},
		{"$acc", TokenSymbol, "$acc"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			toks, err := Tokenize(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(toks) < 1 {
				t.Fatal("expected at least one token")
			}
			tok := toks[0]
			if tok.Type != tt.wantType {
				t.Errorf("type: got %v, want %v", tok.Type, tt.wantType)
			}
			if tok.Text != tt.wantText {
				t.Errorf("text: got %q, want %q", tok.Text, tt.wantText)
			}
		})
	}
}

func TestTokenizeSequence(t *testing.T) {
	toks, err := Tokenize("(defn add [a int b int] -> int (+ a b))")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Spot-check types
	types := make([]TokenType, 0, len(toks))
	for _, tok := range toks {
		if tok.Type == TokenEOF {
			break
		}
		types = append(types, tok.Type)
	}
	want := []TokenType{
		TokenLParen, TokenSymbol, TokenSymbol,
		TokenLBracket, TokenSymbol, TokenSymbol, TokenSymbol, TokenSymbol, TokenRBracket,
		TokenSymbol, TokenSymbol,
		TokenLParen, TokenSymbol, TokenSymbol, TokenSymbol, TokenRParen,
		TokenRParen,
	}
	if len(types) != len(want) {
		t.Fatalf("token count: got %d, want %d\ntokens: %v", len(types), len(want), toks)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Errorf("token[%d]: got %v, want %v", i, types[i], want[i])
		}
	}
}

func TestTokenizeReaderMacros(t *testing.T) {
	// `(a ~b ~@c) — backtick, then unquote/unquote-splice must NOT glue to the
	// following symbol, and ~@ must be munched before ~.
	toks, err := Tokenize("`(a ~b ~@c)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var types []TokenType
	for _, tok := range toks {
		if tok.Type == TokenEOF {
			break
		}
		types = append(types, tok.Type)
	}
	want := []TokenType{
		TokenSyntaxQuote, TokenLParen,
		TokenSymbol, // a
		TokenUnquote, TokenSymbol, // ~b
		TokenUnquoteSplice, TokenSymbol, // ~@c
		TokenRParen,
	}
	if len(types) != len(want) {
		t.Fatalf("token count: got %d, want %d\ntokens: %v", len(types), len(want), toks)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Errorf("token[%d]: got %v, want %v", i, types[i], want[i])
		}
	}
	// The symbol after ~ should be exactly "b", not "~b".
	if toks[4].Text != "b" {
		t.Errorf("unquoted symbol: got %q, want %q", toks[4].Text, "b")
	}
}

func TestTokenizeComment(t *testing.T) {
	toks, err := Tokenize("; this is a comment\n42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if toks[0].Type != TokenComment {
		t.Errorf("expected comment token, got %v", toks[0])
	}
	if toks[0].Text != "; this is a comment" {
		t.Errorf("comment text: got %q, want %q", toks[0].Text, "; this is a comment")
	}
	if toks[1].Type != TokenInt {
		t.Errorf("expected int token at [1], got %v", toks[1])
	}
}

func TestTokenizeCommentDoubleColon(t *testing.T) {
	toks, err := Tokenize(";; double semi\n(def x 1)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if toks[0].Type != TokenComment {
		t.Errorf("expected comment token, got %v", toks[0])
	}
	if toks[0].Text != ";; double semi" {
		t.Errorf("comment text: got %q, want %q", toks[0].Text, ";; double semi")
	}
}

func TestTokenizeDocCommentUnchanged(t *testing.T) {
	toks, err := Tokenize(";;; docstring\n(defn foo [] nil)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if toks[0].Type != TokenDocComment {
		t.Errorf("expected doc-comment token, got %v", toks[0])
	}
}

func TestTokenizePosition(t *testing.T) {
	toks, err := Tokenize("(\n  foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if toks[0].Line != 1 {
		t.Errorf("line: got %d, want 1", toks[0].Line)
	}
	if toks[1].Line != 2 {
		t.Errorf("line: got %d, want 2", toks[1].Line)
	}
}

func TestUnterminatedStringContext(t *testing.T) {
	_, err := Tokenize(`(def name "hello)`)
	if err == nil {
		t.Fatal("expected error for unterminated string")
	}
	msg := err.Error()
	// Reports the position of the opening quote (column 11) with a caret snippet.
	if !strings.Contains(msg, "unterminated string") {
		t.Errorf("error %q should mention unterminated string", msg)
	}
	if !strings.Contains(msg, "1:11") {
		t.Errorf("error %q should point at the opening quote (1:11)", msg)
	}
	if !strings.Contains(msg, "^") {
		t.Errorf("error %q should include a caret pointer", msg)
	}
}

func TestAnonFnShorthandRejected(t *testing.T) {
	_, err := Tokenize(`(map #(+ % 1) xs)`)
	if err == nil {
		t.Fatal("expected error for #(...) anonymous function shorthand")
	}
	if msg := err.Error(); !strings.Contains(msg, "(fn [x] ...)") {
		t.Errorf("error %q should suggest the (fn [x] ...) form", msg)
	}
}
