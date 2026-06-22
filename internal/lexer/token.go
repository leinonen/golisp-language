package lexer

import "fmt"

// TokenType identifies what kind of token this is.
type TokenType int

const (
	// Delimiters
	TokenLParen     TokenType = iota // (
	TokenRParen                      // )
	TokenLBracket                    // [
	TokenRBracket                    // ]
	TokenLBrace                      // {
	TokenRBrace                      // }
	TokenHashLBrace                  // #{

	// Literals
	TokenNil
	TokenTrue
	TokenFalse
	TokenInt
	TokenFloat
	TokenString
	TokenKeyword // :foo
	TokenSymbol  // any identifier

	// Special
	TokenQuote         // '
	TokenSyntaxQuote   // `
	TokenUnquote       // ~
	TokenUnquoteSplice // ~@
	TokenDocComment    // ;;; doc comment
	TokenComment       // ; or ;; regular comment

	TokenEOF
)

var tokenTypeNames = map[TokenType]string{
	TokenLParen:        "(",
	TokenRParen:        ")",
	TokenLBracket:      "[",
	TokenRBracket:      "]",
	TokenLBrace:        "{",
	TokenRBrace:        "}",
	TokenHashLBrace:    "#{",
	TokenNil:           "nil",
	TokenTrue:          "true",
	TokenFalse:         "false",
	TokenInt:           "int",
	TokenFloat:         "float",
	TokenString:        "string",
	TokenKeyword:       "keyword",
	TokenSymbol:        "symbol",
	TokenQuote:         "'",
	TokenSyntaxQuote:   "`",
	TokenUnquote:       "~",
	TokenUnquoteSplice: "~@",
	TokenDocComment:    "doc-comment",
	TokenComment:       "comment",
	TokenEOF:           "EOF",
}

func (t TokenType) String() string {
	if s, ok := tokenTypeNames[t]; ok {
		return s
	}
	return fmt.Sprintf("token(%d)", int(t))
}

// Token is a single lexed unit.
type Token struct {
	Type   TokenType
	Text   string // raw text from source
	Line   int
	Column int
}

func (t Token) String() string {
	return fmt.Sprintf("Token(%s, %q, %d:%d)", t.Type, t.Text, t.Line, t.Column)
}
