package lexer

import (
	"testing"
)

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

func TestTokenizeTypeAnnot(t *testing.T) {
	tests := []struct {
		input    string
		wantText string
	}{
		{"^int", "int"},
		{"^string", "string"},
		{"^float64", "float64"},
		{"^bool", "bool"},
		{"^error", "error"},
		{"^*http.Request", "*http.Request"},
		{"^[]string", "[]string"},
		{"^map[string]int", "map[string]int"},
		{"^(chan int)", "(chan int)"},
		{"^[string error]", "[string error]"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			toks, err := Tokenize(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if toks[0].Type != TokenTypeAnnot {
				t.Errorf("type: got %v, want TokenTypeAnnot", toks[0].Type)
			}
			if toks[0].Text != tt.wantText {
				t.Errorf("text: got %q, want %q", toks[0].Text, tt.wantText)
			}
		})
	}
}

func TestTokenizeSequence(t *testing.T) {
	toks, err := Tokenize("(defn ^int add [^int a ^int b] (+ a b))")
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
		TokenLParen, TokenSymbol, TokenTypeAnnot, TokenSymbol,
		TokenLBracket, TokenTypeAnnot, TokenSymbol, TokenTypeAnnot, TokenSymbol, TokenRBracket,
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

func TestTokenizeComment(t *testing.T) {
	toks, err := Tokenize("; this is a comment\n42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if toks[0].Type != TokenInt {
		t.Errorf("expected int token, got %v", toks[0])
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
