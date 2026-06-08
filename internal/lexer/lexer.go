package lexer

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Tokenize converts a source string into a slice of Tokens.
// Returns an error on unexpected input.
func Tokenize(source string) ([]Token, error) {
	l := &lexer{
		src:    []rune(source),
		line:   1,
		column: 1,
	}
	return l.tokenize()
}

type lexer struct {
	src    []rune
	pos    int
	line   int
	column int
}

func (l *lexer) peek() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}

func (l *lexer) peekAt(offset int) rune {
	idx := l.pos + offset
	if idx >= len(l.src) {
		return 0
	}
	return l.src[idx]
}

func (l *lexer) advance() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	ch := l.src[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.column = 1
	} else {
		l.column++
	}
	return ch
}

func (l *lexer) errorf(format string, args ...any) error {
	return l.errorfAt(l.line, l.column, format, args...)
}

// errorfAt formats a lexer error at a specific position, appending a two-line
// source-context snippet with a caret pointing at the offending column.
func (l *lexer) errorfAt(line, col int, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("%d:%d: %s%s", line, col, msg, l.sourceContext(line, col))
}

// sourceContext returns a two-line snippet: the offending source line and a
// caret aligned under col. Leading tabs are preserved so the caret stays aligned.
func (l *lexer) sourceContext(line, col int) string {
	lines := strings.Split(string(l.src), "\n")
	if line <= 0 || line > len(lines) {
		return ""
	}
	srcLine := lines[line-1]
	if col < 1 {
		col = 1
	}
	var b strings.Builder
	for i := 0; i < col-1; i++ {
		if i < len(srcLine) && srcLine[i] == '\t' {
			b.WriteByte('\t')
		} else {
			b.WriteByte(' ')
		}
	}
	b.WriteByte('^')
	return fmt.Sprintf("\n  %s\n  %s", srcLine, b.String())
}

func (l *lexer) tokenize() ([]Token, error) {
	var tokens []Token
	for {
		l.skipWhitespaceAndComments()
		if l.pos >= len(l.src) {
			tokens = append(tokens, Token{Type: TokenEOF, Line: l.line, Column: l.column})
			break
		}
		tok, err := l.nextToken()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
	}
	return tokens, nil
}

func (l *lexer) skipWhitespaceAndComments() {
	for l.pos < len(l.src) {
		ch := l.peek()
		if unicode.IsSpace(ch) {
			l.advance()
			continue
		}
		// any semicolon — let nextToken emit the appropriate token type
		if ch == ';' {
			break
		}
		// comma is whitespace in Lisp
		if ch == ',' {
			l.advance()
			continue
		}
		break
	}
}

func (l *lexer) nextToken() (Token, error) {
	line, col := l.line, l.column
	ch := l.peek()

	switch {
	case ch == '(':
		l.advance()
		return Token{Type: TokenLParen, Text: "(", Line: line, Column: col}, nil

	case ch == ')':
		l.advance()
		return Token{Type: TokenRParen, Text: ")", Line: line, Column: col}, nil

	case ch == '[':
		l.advance()
		return Token{Type: TokenLBracket, Text: "[", Line: line, Column: col}, nil

	case ch == ']':
		l.advance()
		return Token{Type: TokenRBracket, Text: "]", Line: line, Column: col}, nil

	case ch == '{':
		l.advance()
		return Token{Type: TokenLBrace, Text: "{", Line: line, Column: col}, nil

	case ch == '}':
		l.advance()
		return Token{Type: TokenRBrace, Text: "}", Line: line, Column: col}, nil

	case ch == '#' && l.peekAt(1) == '{':
		l.advance()
		l.advance()
		return Token{Type: TokenHashLBrace, Text: "#{", Line: line, Column: col}, nil

	case ch == '#' && l.peekAt(1) == '(':
		l.advance()
		l.advance()
		return Token{Type: TokenHashLParen, Text: "#(", Line: line, Column: col}, nil

	case ch == '\'':
		l.advance()
		return Token{Type: TokenQuote, Text: "'", Line: line, Column: col}, nil

	case ch == '"':
		return l.readString(line, col)

	case ch == ':':
		return l.readKeyword(line, col)

	case ch == ';':
		if l.peekAt(1) == ';' && l.peekAt(2) == ';' {
			return l.readDocComment(line, col)
		}
		return l.readComment(line, col)

	case ch == '-' && isDigit(l.peekAt(1)):
		return l.readNumber(line, col)

	case isDigit(ch):
		return l.readNumber(line, col)

	default:
		return l.readSymbol(line, col)
	}
}

func (l *lexer) readString(line, col int) (Token, error) {
	l.advance() // consume opening "
	var sb strings.Builder
	for l.pos < len(l.src) {
		ch := l.advance()
		if ch == '"' {
			return Token{Type: TokenString, Text: sb.String(), Line: line, Column: col}, nil
		}
		if ch == '\\' {
			esc := l.advance()
			switch esc {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case 'r':
				sb.WriteByte('\r')
			case '"':
				sb.WriteByte('"')
			case '\\':
				sb.WriteByte('\\')
			default:
				sb.WriteByte('\\')
				sb.WriteRune(esc)
			}
			continue
		}
		sb.WriteRune(ch)
	}
	return Token{}, l.errorfAt(line, col, "unterminated string literal (opened here, missing closing \")")
}


func (l *lexer) readKeyword(line, col int) (Token, error) {
	l.advance() // consume :
	var sb strings.Builder
	for l.pos < len(l.src) && isSymbolChar(l.peek()) {
		sb.WriteRune(l.advance())
	}
	text := sb.String()
	if text == "" {
		return Token{}, l.errorf("empty keyword")
	}
	return Token{Type: TokenKeyword, Text: text, Line: line, Column: col}, nil
}

func (l *lexer) readNumber(line, col int) (Token, error) {
	var sb strings.Builder
	isFloat := false

	if l.peek() == '-' {
		sb.WriteRune(l.advance())
	}
	for l.pos < len(l.src) && isDigit(l.peek()) {
		sb.WriteRune(l.advance())
	}
	if l.pos < len(l.src) && l.peek() == '.' && isDigit(l.peekAt(1)) {
		isFloat = true
		sb.WriteRune(l.advance()) // '.'
		for l.pos < len(l.src) && isDigit(l.peek()) {
			sb.WriteRune(l.advance())
		}
	}
	// optional exponent
	if l.pos < len(l.src) && (l.peek() == 'e' || l.peek() == 'E') {
		isFloat = true
		sb.WriteRune(l.advance())
		if l.pos < len(l.src) && (l.peek() == '+' || l.peek() == '-') {
			sb.WriteRune(l.advance())
		}
		for l.pos < len(l.src) && isDigit(l.peek()) {
			sb.WriteRune(l.advance())
		}
	}

	text := sb.String()
	if isFloat {
		_, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return Token{}, l.errorf("invalid float: %s", text)
		}
		return Token{Type: TokenFloat, Text: text, Line: line, Column: col}, nil
	}
	_, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return Token{}, l.errorf("invalid integer: %s", text)
	}
	return Token{Type: TokenInt, Text: text, Line: line, Column: col}, nil
}

func (l *lexer) readSymbol(line, col int) (Token, error) {
	var sb strings.Builder
	for l.pos < len(l.src) && isSymbolChar(l.peek()) {
		sb.WriteRune(l.advance())
	}
	text := sb.String()
	if text == "" {
		ch := l.advance()
		return Token{}, l.errorf("unexpected character %q", ch)
	}
	switch text {
	case "nil":
		return Token{Type: TokenNil, Text: text, Line: line, Column: col}, nil
	case "true":
		return Token{Type: TokenTrue, Text: text, Line: line, Column: col}, nil
	case "false":
		return Token{Type: TokenFalse, Text: text, Line: line, Column: col}, nil
	}
	return Token{Type: TokenSymbol, Text: text, Line: line, Column: col}, nil
}

func (l *lexer) readComment(line, col int) (Token, error) {
	var buf strings.Builder
	buf.WriteRune(l.advance()) // first ;
	if l.pos < len(l.src) && l.peek() == ';' {
		buf.WriteRune(l.advance()) // optional second ;
	}
	for l.pos < len(l.src) && l.peek() != '\n' {
		buf.WriteRune(l.advance())
	}
	return Token{Type: TokenComment, Text: strings.TrimRight(buf.String(), " \t"), Line: line, Column: col}, nil
}

func (l *lexer) readDocComment(line, col int) (Token, error) {
	l.advance() // ;
	l.advance() // ;
	l.advance() // ;
	// skip optional single space after ;;;
	if l.pos < len(l.src) && l.peek() == ' ' {
		l.advance()
	}
	var sb strings.Builder
	for l.pos < len(l.src) && l.peek() != '\n' {
		sb.WriteRune(l.advance())
	}
	return Token{Type: TokenDocComment, Text: strings.TrimRight(sb.String(), " \t"), Line: line, Column: col}, nil
}

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

// isSymbolChar returns true for characters valid in a symbol.
// Includes letters, digits, and Lisp-conventional punctuation.
func isSymbolChar(ch rune) bool {
	if unicode.IsLetter(ch) || unicode.IsDigit(ch) {
		return true
	}
	switch ch {
	case '-', '_', '+', '*', '/', '?', '!', '=', '<', '>', '.', '&', '#', '%', '|', '~':
		return true
	}
	return false
}
