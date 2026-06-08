// Package parser converts a token stream into an AST.
package parser

import (
	"fmt"
	"strconv"
	"strings"

	"golisp/internal/ast"
	"golisp/internal/lexer"
)

// didYouMean maps common Lisp/Scheme/Clojure forms to their glisp equivalents.
var didYouMean = map[string]string{
	"defun":     "defn",
	"define":    "def or defn",
	"lambda":    "fn",
	"fun":       "fn or defn",
	"func":      "fn or defn",
	"begin":     "do",
	"progn":     "do",
	"defmacro":  "(macros not yet supported)",
	"defvar":    "def",
	"defrecord": "defstruct",
	"setq":      "def",
	"set!":      "def, or reset! for atoms",
	"let*":      "let",
	"letrec":    "let",
	"funcall":   "(just call the function directly)",
	"car":       "first",
	"cdr":       "rest",
	"cons":      "conj",
}

// CommentMap maps 1-based source line numbers to the comment on that line.
type CommentMap map[int]string

// ParseResult is returned by ParseWithComments.
type ParseResult struct {
	Nodes    []ast.Node
	Comments CommentMap
}

// ParseWithComments tokenizes src and returns both the AST nodes and a
// CommentMap capturing all ; and ;; comment lines by source line number.
// Intended for the formatter; other callers use ParseString/ParseSource.
func ParseWithComments(src string) (ParseResult, error) {
	tokens, err := lexer.Tokenize(src)
	if err != nil {
		return ParseResult{}, err
	}
	comments := make(CommentMap)
	filtered := make([]lexer.Token, 0, len(tokens))
	for _, tok := range tokens {
		if tok.Type == lexer.TokenComment {
			comments[tok.Line] = tok.Text
		} else {
			filtered = append(filtered, tok)
		}
	}
	p := &parser{tokens: filtered, src: src}
	nodes, err := p.parseAll()
	if err != nil {
		return ParseResult{}, err
	}
	return ParseResult{Nodes: nodes, Comments: comments}, nil
}

// Parse converts a token stream into a slice of top-level AST nodes.
func Parse(tokens []lexer.Token) ([]ast.Node, error) {
	p := &parser{tokens: tokens}
	return p.parseAll()
}

// ParseString tokenizes and parses source, attaching source context to errors.
func ParseString(src string) ([]ast.Node, error) {
	tokens, err := lexer.Tokenize(src)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens, src: src}
	return p.parseAll()
}

// ParseSource parses a pre-tokenized stream with source text for error context.
func ParseSource(tokens []lexer.Token, src string) ([]ast.Node, error) {
	p := &parser{tokens: tokens, src: src}
	return p.parseAll()
}

// ParseSourceFile is like ParseSource but also sets Position.File on every node.
func ParseSourceFile(tokens []lexer.Token, src, filename string) ([]ast.Node, error) {
	p := &parser{tokens: tokens, src: src, filename: filename}
	return p.parseAll()
}

// ParseStringFile is like ParseString but also sets Position.File on every node.
func ParseStringFile(src, filename string) ([]ast.Node, error) {
	tokens, err := lexer.Tokenize(src)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens, src: src, filename: filename}
	return p.parseAll()
}

type parser struct {
	tokens     []lexer.Token
	pos        int
	src        string // original source text for error context; may be empty
	filename   string // source file path; when set, Position.File is populated
	pendingDoc string // set by ;;; doc comment, consumed by parseDefn/parseDefmethod
	// openStack tracks unclosed opening delimiters ( [ { #{ #( so EOF errors can
	// point back at the opener instead of the end of the file.
	openStack []lexer.Token
}

func (p *parser) peek() lexer.Token {
	if p.pos >= len(p.tokens) {
		return lexer.Token{Type: lexer.TokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *parser) peekAt(offset int) lexer.Token {
	i := p.pos + offset
	if i >= len(p.tokens) {
		return lexer.Token{Type: lexer.TokenEOF}
	}
	return p.tokens[i]
}

// isBindingTypeStart is like isTypeExprStart but used in let/loop binding position
// where we must disambiguate type annotations from value expressions.
//
// Key ambiguities:
//   - "[]" is an empty vector literal; "[]string" is a slice type.
//     Tokens: both start with '[' ']', but "[]string" has a type symbol after ']'.
//   - "(chan int 10)" is channel creation (value); "(chan T)" could be a type but
//     is indistinguishable from a 3-token (chan T cap) form — exclude '(' entirely.
func (p *parser) isBindingTypeStart() bool {
	switch p.peekType() {
	case lexer.TokenLBracket:
		// Only "[]T" slice types are useful as binding annotations.
		// "[]T" tokenizes as '[' ']' typeSymbol.
		// "[]" alone tokenizes as '[' ']' (end), "['a' 'b']" has non-] after '['.
		if p.peekAt(1).Type != lexer.TokenRBracket {
			return false // not "[]..." form — could be [a b] vector
		}
		// '[' ']' is followed by something — type if that something is a type symbol
		t := p.peekAt(2)
		if t.Type == lexer.TokenSymbol {
			return isTypeSymbol(t.Text)
		}
		if t.Type == lexer.TokenLBracket {
			return true // [][]T nested slice
		}
		return false
	case lexer.TokenSymbol:
		return isTypeSymbol(p.peek().Text)
	}
	return false
}

func (p *parser) peekType() lexer.TokenType { return p.peek().Type }

func (p *parser) advance() lexer.Token {
	tok := p.peek()
	if tok.Type != lexer.TokenEOF {
		p.pos++
		switch tok.Type {
		case lexer.TokenLParen, lexer.TokenLBracket, lexer.TokenLBrace,
			lexer.TokenHashLBrace, lexer.TokenHashLParen:
			p.openStack = append(p.openStack, tok)
		case lexer.TokenRParen, lexer.TokenRBracket, lexer.TokenRBrace:
			if n := len(p.openStack); n > 0 {
				p.openStack = p.openStack[:n-1]
			}
		}
	}
	return tok
}

func (p *parser) expect(tt lexer.TokenType) (lexer.Token, error) {
	tok := p.peek()
	if tok.Type != tt {
		// Running off the end of the input while expecting a closing delimiter is
		// almost always an unbalanced-parens mistake — point at the opener.
		if tok.Type == lexer.TokenEOF && len(p.openStack) > 0 {
			return tok, p.errUnclosed(p.openStack[0])
		}
		return tok, p.errorf("expected %v, got %v (%q)", tt, tok.Type, tok.Text)
	}
	return p.advance(), nil
}

// unexpectedEOF reports a friendly end-of-input error, pointing at the
// outermost unclosed delimiter when one is open.
func (p *parser) unexpectedEOF() error {
	if len(p.openStack) > 0 {
		return p.errUnclosed(p.openStack[0])
	}
	return p.errorf("unexpected end of input")
}

// errUnclosed reports an unterminated delimiter, with the caret pointing at the
// opening delimiter that was never closed rather than at the end of the file.
func (p *parser) errUnclosed(open lexer.Token) error {
	want := matchingClose(open.Type)
	msg := fmt.Sprintf("unclosed %q (opened at line %d, column %d) — missing %q",
		open.Text, open.Line, open.Column, want)
	if p.src != "" {
		return fmt.Errorf("%d:%d: %s%s", open.Line, open.Column, msg, p.sourceContext(open.Line, open.Column))
	}
	return fmt.Errorf("%d:%d: %s", open.Line, open.Column, msg)
}

// matchingClose returns the closing delimiter string for an opening token type.
func matchingClose(open lexer.TokenType) string {
	switch open {
	case lexer.TokenLBracket:
		return "]"
	case lexer.TokenLBrace, lexer.TokenHashLBrace:
		return "}"
	default: // TokenLParen, TokenHashLParen
		return ")"
	}
}

func (p *parser) errorf(format string, args ...any) error {
	return p.errorfTok(p.peek(), format, args...)
}

// errorfTok is like errorf but anchors the position and caret at a specific token.
func (p *parser) errorfTok(tok lexer.Token, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	if p.src != "" {
		ctx := p.sourceContext(tok.Line, tok.Column)
		return fmt.Errorf("%d:%d: %s%s", tok.Line, tok.Column, msg, ctx)
	}
	return fmt.Errorf("%d:%d: %s", tok.Line, tok.Column, msg)
}

// sourceContext returns a two-line string showing the offending source line
// and a ^ pointer to the column.
func (p *parser) sourceContext(line, col int) string {
	lines := strings.Split(p.src, "\n")
	if line <= 0 || line > len(lines) {
		return ""
	}
	srcLine := lines[line-1]
	return fmt.Sprintf("\n  %s\n  %s", srcLine, caretLine(srcLine, col))
}

// caretLine builds a "^" pointer line aligned under column col of srcLine.
// Leading tabs in srcLine are preserved in the pointer so the caret stays
// aligned in terminals that render tabs as more than one column.
func caretLine(srcLine string, col int) string {
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
	return b.String()
}

func (p *parser) mkpos(tok lexer.Token) ast.Position {
	return ast.Position{File: p.filename, Line: tok.Line, Column: tok.Column}
}

// ---------- top-level ----------

func (p *parser) parseAll() ([]ast.Node, error) {
	var nodes []ast.Node
	for p.peekType() != lexer.TokenEOF {
		if p.peekType() == lexer.TokenDocComment {
			p.pendingDoc = p.advance().Text
			continue
		}
		if p.peekType() == lexer.TokenComment {
			p.advance()
			continue
		}
		node, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		p.pendingDoc = ""
		nodes = append(nodes, node)
	}
	return nodes, nil
}

// ---------- expression dispatch ----------

func (p *parser) parseExpr() (ast.Node, error) {
	return p.parseExprInner()
}

func (p *parser) parseExprInner() (ast.Node, error) {
	switch p.peekType() {
	case lexer.TokenLParen:
		return p.parseList()
	case lexer.TokenLBracket:
		return p.parseVector()
	case lexer.TokenLBrace:
		return p.parseMap()
	case lexer.TokenHashLBrace:
		return p.parseSet()
	case lexer.TokenHashLParen:
		return p.parseAnonFn()
	case lexer.TokenQuote:
		return p.parseQuote()
	case lexer.TokenNil:
		tok := p.advance()
		return ast.NewNilLit(p.mkpos(tok)), nil
	case lexer.TokenTrue:
		tok := p.advance()
		return ast.NewBoolLit(p.mkpos(tok), true), nil
	case lexer.TokenFalse:
		tok := p.advance()
		return ast.NewBoolLit(p.mkpos(tok), false), nil
	case lexer.TokenInt:
		return p.parseInt()
	case lexer.TokenFloat:
		return p.parseFloat()
	case lexer.TokenString:
		tok := p.advance()
		return ast.NewStringLit(p.mkpos(tok), tok.Text), nil
	case lexer.TokenKeyword:
		tok := p.advance()
		return ast.NewKeywordLit(p.mkpos(tok), tok.Text), nil
	case lexer.TokenSymbol:
		tok := p.advance()
		return ast.NewSymbol(p.mkpos(tok), tok.Text), nil
	case lexer.TokenDocComment:
		p.advance()
		return p.parseExprInner()
	case lexer.TokenComment:
		p.advance()
		return p.parseExprInner()
	case lexer.TokenEOF:
		return nil, p.unexpectedEOF()
	case lexer.TokenRParen, lexer.TokenRBracket, lexer.TokenRBrace:
		tok := p.peek()
		return nil, p.errorf("unexpected %q — no matching opening delimiter", tok.Text)
	default:
		return nil, p.errorf("unexpected token %v (%q)", p.peekType(), p.peek().Text)
	}
}

func (p *parser) parseInt() (ast.Node, error) {
	tok := p.advance()
	v, err := strconv.ParseInt(tok.Text, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%d:%d: invalid int %q", tok.Line, tok.Column, tok.Text)
	}
	return ast.NewIntLit(p.mkpos(tok), v), nil
}

func (p *parser) parseFloat() (ast.Node, error) {
	tok := p.advance()
	v, err := strconv.ParseFloat(tok.Text, 64)
	if err != nil {
		return nil, fmt.Errorf("%d:%d: invalid float %q", tok.Line, tok.Column, tok.Text)
	}
	return ast.NewFloatLit(p.mkpos(tok), v), nil
}

func (p *parser) parseQuote() (ast.Node, error) {
	tok := p.advance() // '
	_, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("%d:%d: quote not supported in transpiler", tok.Line, tok.Column)
}

// ---------- collections ----------

func (p *parser) parseVector() (*ast.VectorLit, error) {
	open := p.advance() // [
	var elems []ast.Node
	for p.peekType() != lexer.TokenRBracket && p.peekType() != lexer.TokenEOF {
		el, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		elems = append(elems, el)
	}
	if _, err := p.expect(lexer.TokenRBracket); err != nil {
		return nil, err
	}
	return ast.NewVectorLit(p.mkpos(open), elems), nil
}

func (p *parser) parseMap() (*ast.MapLit, error) {
	open := p.advance() // {
	var pairs []ast.MapPair
	for p.peekType() != lexer.TokenRBrace && p.peekType() != lexer.TokenEOF {
		k, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.peekType() == lexer.TokenRBrace {
			return nil, fmt.Errorf("%d:%d: map literal has odd number of forms", open.Line, open.Column)
		}
		v, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, ast.MapPair{Key: k, Value: v})
	}
	if _, err := p.expect(lexer.TokenRBrace); err != nil {
		return nil, err
	}
	return ast.NewMapLit(p.mkpos(open), pairs), nil
}

func (p *parser) parseSet() (ast.Node, error) {
	open := p.advance() // #{
	var elems []ast.Node
	for p.peekType() != lexer.TokenRBrace && p.peekType() != lexer.TokenEOF {
		el, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		elems = append(elems, el)
	}
	if _, err := p.expect(lexer.TokenRBrace); err != nil {
		return nil, err
	}
	return ast.NewSetLit(p.mkpos(open), elems), nil
}

// ---------- list / special form dispatch ----------

func (p *parser) parseList() (ast.Node, error) {
	open := p.advance() // (
	pos := p.mkpos(open)

	if p.peekType() == lexer.TokenRParen {
		p.advance()
		return ast.NewVectorLit(pos, nil), nil
	}

	head := p.peek()
	if head.Type == lexer.TokenSymbol {
		switch head.Text {
		case "ns":
			return p.parseNS(pos)
		case "def":
			return p.parseDef(pos)
		case "defn":
			return p.parseDefn(pos)
		case "deftype":
			return p.parseDeftype(pos)
		case "defstruct":
			return p.parseDefstruct(pos)
		case "definterface":
			return p.parseDefinterface(pos)
		case "defmethod":
			return p.parseDefmethod(pos)
		case "fn":
			return p.parseFn(pos)
		case "let":
			return p.parseLet(pos)
		case "if":
			return p.parseIf(pos)
		case "when":
			return p.parseWhen(pos)
		case "cond":
			return p.parseCond(pos)
		case "do":
			return p.parseDo(pos)
		case "go":
			return p.parseGo(pos)
		case "go-val":
			return p.parseGoVal(pos)
		case "par":
			return p.parsePar(pos)
		case "defer":
			return p.parseDefer(pos)
		case "chan":
			return p.parseChan(pos)
		case "send!":
			return p.parseSend(pos)
		case "recv!":
			return p.parseRecv(pos)
		case "recv-ok!":
			return p.parseRecvOk(pos)
		case "close!":
			return p.parseClose(pos)
		case "select!":
			return p.parseSelect(pos)
		case "for-chan":
			return p.parseForChan(pos)
		case "with-lock":
			return p.parseWithLock(pos)
		case "pipeline":
			return p.parsePipeline(pos)
		case "fan-out":
			return p.parseFanOut(pos)
		case "fan-in":
			return p.parseFanIn(pos)
		case "loop":
			return p.parseLoop(pos)
		case "recur":
			return p.parseRecur(pos)
		case "return":
			return p.parseReturn(pos)
		case "values":
			return p.parseValues(pos)
		case "if-err":
			return p.parseIfErr(pos)
		case "if-let":
			return p.parseIfLet(pos)
		case "when-let":
			return p.parseWhenLet(pos)
		case "let-or":
			return p.parseLetOr(pos)
		case "switch":
			return p.parseSwitch(pos)
		case "as":
			return p.parseTypeAssert(pos)
		case "deftest":
			return p.parseDefTest(pos)
		}

		// "Did you mean?" hints for common mistakes from other Lisps.
		if hint, ok := didYouMean[head.Text]; ok {
			return nil, p.errorfTok(head, "%q is not valid glisp — did you mean %s?", head.Text, hint)
		}

		// Method call: (.MethodName obj args...)
		if strings.HasPrefix(head.Text, ".") && len(head.Text) > 1 && head.Text[1] != '-' {
			return p.parseMethodCall(pos)
		}
		// Field access: (.-FieldName obj)
		if strings.HasPrefix(head.Text, ".-") {
			return p.parseFieldAccess(pos)
		}
		// Struct literal: (TypeName. {...}) — symbol ending in .
		if strings.HasSuffix(head.Text, ".") && len(head.Text) > 1 {
			return p.parseStructLit(pos)
		}
	}

	return p.parseCall(pos)
}

// ---------- special forms ----------

func (p *parser) parseNS(pos ast.Position) (*ast.NSDecl, error) {
	p.advance() // "ns"
	nameTok, err := p.expect(lexer.TokenSymbol)
	if err != nil {
		return nil, err
	}
	name := nameTok.Text

	var imports []ast.ImportSpec
	var requires []ast.RequireSpec
	for p.peekType() != lexer.TokenRParen && p.peekType() != lexer.TokenEOF {
		if p.peekType() != lexer.TokenLParen {
			return nil, p.errorf("expected :import or :require clause in ns")
		}
		p.advance() // (
		kwTok, err := p.expect(lexer.TokenKeyword)
		if err != nil {
			return nil, err
		}
		switch kwTok.Text {
		case "import":
			specs, err := p.parseImportList()
			if err != nil {
				return nil, err
			}
			imports = append(imports, specs...)
		case "require":
			specs, err := p.parseRequireList()
			if err != nil {
				return nil, err
			}
			requires = append(requires, specs...)
		default:
			return nil, fmt.Errorf("%d:%d: expected :import or :require, got :%s", kwTok.Line, kwTok.Column, kwTok.Text)
		}
		if _, err := p.expect(lexer.TokenRParen); err != nil {
			return nil, err
		}
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewNSDecl(pos, name, imports, requires), nil
}

func (p *parser) parseImportList() ([]ast.ImportSpec, error) {
	if _, err := p.expect(lexer.TokenLBracket); err != nil {
		return nil, err
	}
	var specs []ast.ImportSpec
	for p.peekType() != lexer.TokenRBracket && p.peekType() != lexer.TokenEOF {
		if p.peekType() == lexer.TokenLBracket {
			p.advance()
			pathTok, err := p.expect(lexer.TokenSymbol)
			if err != nil {
				return nil, err
			}
			spec := ast.ImportSpec{Path: pathTok.Text}
			if p.peekType() == lexer.TokenKeyword && p.peek().Text == "as" {
				p.advance()
				aliasTok, err := p.expect(lexer.TokenSymbol)
				if err != nil {
					return nil, err
				}
				spec.Alias = aliasTok.Text
			}
			if _, err := p.expect(lexer.TokenRBracket); err != nil {
				return nil, err
			}
			specs = append(specs, spec)
		} else {
			pathTok, err := p.expect(lexer.TokenSymbol)
			if err != nil {
				return nil, err
			}
			specs = append(specs, ast.ImportSpec{Path: pathTok.Text})
		}
	}
	if _, err := p.expect(lexer.TokenRBracket); err != nil {
		return nil, err
	}
	return specs, nil
}

func (p *parser) parseRequireList() ([]ast.RequireSpec, error) {
	if _, err := p.expect(lexer.TokenLBracket); err != nil {
		return nil, err
	}
	var specs []ast.RequireSpec
	for p.peekType() != lexer.TokenRBracket && p.peekType() != lexer.TokenEOF {
		if p.peekType() == lexer.TokenLBracket {
			p.advance()
			pathTok, err := p.expect(lexer.TokenSymbol)
			if err != nil {
				return nil, err
			}
			spec := ast.RequireSpec{Path: pathTok.Text}
			if p.peekType() == lexer.TokenKeyword && p.peek().Text == "as" {
				p.advance()
				aliasTok, err := p.expect(lexer.TokenSymbol)
				if err != nil {
					return nil, err
				}
				spec.Alias = aliasTok.Text
			}
			if _, err := p.expect(lexer.TokenRBracket); err != nil {
				return nil, err
			}
			specs = append(specs, spec)
		} else {
			pathTok, err := p.expect(lexer.TokenSymbol)
			if err != nil {
				return nil, err
			}
			specs = append(specs, ast.RequireSpec{Path: pathTok.Text})
		}
	}
	if _, err := p.expect(lexer.TokenRBracket); err != nil {
		return nil, err
	}
	return specs, nil
}

func (p *parser) parseDef(pos ast.Position) (*ast.DefDecl, error) {
	p.advance() // "def"
	nameTok, err := p.expect(lexer.TokenSymbol)
	if err != nil {
		return nil, err
	}
	// (def name type value) — 3-arg form with optional type annotation
	// If next token looks like a type and isn't the last thing before ), read it as the type.
	var annot *ast.TypeExpr
	if p.isTypeExprStart() && p.peekType() != lexer.TokenRParen {
		// Save position to allow backtracking if needed
		savedPos := p.pos
		t, err := p.parseTypeExpr()
		if err != nil || p.peekType() == lexer.TokenRParen {
			// No value would follow — treat as no type annotation, restore
			p.pos = savedPos
		} else {
			annot = t
		}
	}
	val, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewDefDecl(pos, nameTok.Text, annot, val), nil
}

// extractDoc treats the first body element as a doc string only when body has
// 2+ forms, so a lone string-returning function is not misidentified.
func extractDoc(body []ast.Node) (doc string, rest []ast.Node) {
	if len(body) >= 2 {
		if s, ok := body[0].(*ast.StringLit); ok {
			return s.Value, body[1:]
		}
	}
	return "", body
}

func (p *parser) parseDefn(pos ast.Position) (*ast.DefnDecl, error) {
	p.advance() // "defn"
	nameTok, err := p.expect(lexer.TokenSymbol)
	if err != nil {
		return nil, err
	}
	params, err := p.parseParamList()
	if err != nil {
		return nil, err
	}
	// (defn name [params] -> return-type body...)
	var retType *ast.TypeExpr
	if p.peekType() == lexer.TokenSymbol && p.peek().Text == "->" {
		p.advance() // consume "->"
		retType, err = p.parseTypeExpr()
		if err != nil {
			return nil, err
		}
	}
	rawBody, err := p.parseBody()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	doc, body := extractDoc(rawBody)
	if p.pendingDoc != "" {
		doc = p.pendingDoc
	}
	return ast.NewDefnDecl(pos, nameTok.Text, params, retType, doc, body), nil
}

func (p *parser) parseDeftype(pos ast.Position) (*ast.DefTypeDecl, error) {
	p.advance() // "deftype"
	nameTok, err := p.expect(lexer.TokenSymbol)
	if err != nil {
		return nil, err
	}
	baseType, err := p.parseTypeExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewDefTypeDecl(pos, nameTok.Text, baseType), nil
}

func (p *parser) parseDefstruct(pos ast.Position) (*ast.StructDecl, error) {
	p.advance() // "defstruct"
	nameTok, err := p.expect(lexer.TokenSymbol)
	if err != nil {
		return nil, err
	}
	var fields []ast.StructField
	for p.peekType() != lexer.TokenRParen && p.peekType() != lexer.TokenEOF {
		fieldTok, err := p.expect(lexer.TokenSymbol)
		if err != nil {
			return nil, err
		}
		// Field type follows name: (defstruct Name field type ...)
		var annot *ast.TypeExpr
		if p.isTypeExprStart() {
			annot, err = p.parseTypeExpr()
			if err != nil {
				return nil, err
			}
		}
		tag := ""
		if p.peekType() == lexer.TokenString {
			tagTok := p.advance()
			tag = tagTok.Text
		}
		fields = append(fields, ast.StructField{Name: fieldTok.Text, TypeAnnot: annot, Tag: tag})
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewStructDecl(pos, nameTok.Text, fields), nil
}

func (p *parser) parseDefinterface(pos ast.Position) (*ast.InterfaceDecl, error) {
	p.advance() // "definterface"
	nameTok, err := p.expect(lexer.TokenSymbol)
	if err != nil {
		return nil, err
	}
	var methods []ast.InterfaceMethod
	for p.peekType() != lexer.TokenRParen && p.peekType() != lexer.TokenEOF {
		if _, err := p.expect(lexer.TokenLParen); err != nil {
			return nil, err
		}
		methodTok, err := p.expect(lexer.TokenSymbol)
		if err != nil {
			return nil, err
		}
		params, err := p.parseParamList()
		if err != nil {
			return nil, err
		}
		// (Method [params] -> return-type)
		var retType *ast.TypeExpr
		if p.peekType() == lexer.TokenSymbol && p.peek().Text == "->" {
			p.advance() // consume "->"
			retType, err = p.parseTypeExpr()
			if err != nil {
				return nil, err
			}
		}
		if _, err := p.expect(lexer.TokenRParen); err != nil {
			return nil, err
		}
		methods = append(methods, ast.InterfaceMethod{Name: methodTok.Text, Params: params, ReturnType: retType})
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewInterfaceDecl(pos, nameTok.Text, methods), nil
}

func (p *parser) parseDefmethod(pos ast.Position) (*ast.MethodDecl, error) {
	p.advance() // "defmethod"
	recvType, err := p.parseTypeExpr()
	if err != nil {
		return nil, err
	}
	nameTok, err := p.expect(lexer.TokenSymbol)
	if err != nil {
		return nil, err
	}
	allParams, err := p.parseParamList()
	if err != nil {
		return nil, err
	}
	if len(allParams) == 0 {
		return nil, p.errorf("defmethod requires at least a receiver param")
	}
	recvName := allParams[0].Name
	params := allParams[1:]
	// (defmethod ReceiverType name [params] -> return-type body...)
	var retType *ast.TypeExpr
	if p.peekType() == lexer.TokenSymbol && p.peek().Text == "->" {
		p.advance() // consume "->"
		retType, err = p.parseTypeExpr()
		if err != nil {
			return nil, err
		}
	}
	rawBody, err := p.parseBody()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	doc, body := extractDoc(rawBody)
	if p.pendingDoc != "" {
		doc = p.pendingDoc
	}
	return ast.NewMethodDecl(pos, recvType, recvName, nameTok.Text, params, retType, doc, body), nil
}

func (p *parser) parseFn(pos ast.Position) (*ast.FnExpr, error) {
	p.advance() // "fn"
	params, err := p.parseParamList()
	if err != nil {
		return nil, err
	}
	// (fn [params] -> return-type body...)
	var retType *ast.TypeExpr
	if p.peekType() == lexer.TokenSymbol && p.peek().Text == "->" {
		p.advance() // consume "->"
		retType, err = p.parseTypeExpr()
		if err != nil {
			return nil, err
		}
	}
	body, err := p.parseBody()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewFnExpr(pos, params, retType, body), nil
}

func (p *parser) parseLet(pos ast.Position) (*ast.LetExpr, error) {
	p.advance() // "let"
	if _, err := p.expect(lexer.TokenLBracket); err != nil {
		return nil, err
	}
	var bindings []ast.LetBinding
	for p.peekType() != lexer.TokenRBracket && p.peekType() != lexer.TokenEOF {
		var pattern ast.Node
		if p.peekType() == lexer.TokenLBracket {
			v, err := p.parseVector()
			if err != nil {
				return nil, err
			}
			pattern = v
		} else if p.peekType() == lexer.TokenLBrace {
			m, err := p.parseMap()
			if err != nil {
				return nil, err
			}
			pattern = m
		} else {
			symTok, err := p.expect(lexer.TokenSymbol)
			if err != nil {
				return nil, err
			}
			pattern = ast.NewSymbol(p.mkpos(symTok), symTok.Text)
		}
		// Optional type annotation between name and value: (let [x string "hello"] ...)
		var annot *ast.TypeExpr
		if _, ok := pattern.(*ast.Symbol); ok && p.isBindingTypeStart() {
			t, err := p.parseTypeExpr()
			if err != nil {
				return nil, err
			}
			annot = t
		}
		val, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, ast.LetBinding{Pattern: pattern, TypeAnnot: annot, Value: val})
	}
	if _, err := p.expect(lexer.TokenRBracket); err != nil {
		return nil, err
	}
	body, err := p.parseBody()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewLetExpr(pos, bindings, body), nil
}

func (p *parser) parseIf(pos ast.Position) (*ast.IfExpr, error) {
	p.advance() // "if"
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	then, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	var els ast.Node
	if p.peekType() != lexer.TokenRParen {
		els, err = p.parseExpr()
		if err != nil {
			return nil, err
		}
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewIfExpr(pos, cond, then, els), nil
}

func (p *parser) parseWhen(pos ast.Position) (*ast.WhenExpr, error) {
	p.advance() // "when"
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	body, err := p.parseBody()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewWhenExpr(pos, cond, body), nil
}

func (p *parser) parseCond(pos ast.Position) (*ast.CondExpr, error) {
	p.advance() // "cond"
	var clauses []ast.CondClause
	var def ast.Node
	for p.peekType() != lexer.TokenRParen && p.peekType() != lexer.TokenEOF {
		test, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		body, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if kw, ok := test.(*ast.KeywordLit); ok && kw.Value == "else" {
			def = body
			continue
		}
		clauses = append(clauses, ast.CondClause{Test: test, Body: body})
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewCondExpr(pos, clauses, def), nil
}

func (p *parser) parseSwitch(pos ast.Position) (*ast.SwitchExpr, error) {
	p.advance() // "switch"
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	var cases []ast.SwitchCase
	var def ast.Node
	for p.peekType() != lexer.TokenRParen && p.peekType() != lexer.TokenEOF {
		val, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		body, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if kw, ok := val.(*ast.KeywordLit); ok && kw.Value == "default" {
			def = body
			continue
		}
		cases = append(cases, ast.SwitchCase{Value: val, Body: body})
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewSwitchExpr(pos, expr, cases, def), nil
}

func (p *parser) parseDo(pos ast.Position) (*ast.DoExpr, error) {
	p.advance() // "do"
	body, err := p.parseBody()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewDoExpr(pos, body), nil
}

func (p *parser) parseGo(pos ast.Position) (*ast.GoStmt, error) {
	p.advance() // "go"
	body, err := p.parseBody()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewGoStmt(pos, body), nil
}

func (p *parser) parseDefer(pos ast.Position) (*ast.DeferStmt, error) {
	p.advance() // "defer"
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewDeferStmt(pos, expr), nil
}

func (p *parser) parseChan(pos ast.Position) (*ast.ChanExpr, error) {
	p.advance() // "chan"
	elemType, err := p.parseTypeExpr()
	if err != nil {
		return nil, err
	}
	var cap ast.Node
	if p.peekType() != lexer.TokenRParen {
		cap, err = p.parseExpr()
		if err != nil {
			return nil, err
		}
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewChanExpr(pos, elemType, cap), nil
}

func (p *parser) parseSend(pos ast.Position) (*ast.SendStmt, error) {
	p.advance() // "send!"
	ch, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	val, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewSendStmt(pos, ch, val), nil
}

func (p *parser) parseRecv(pos ast.Position) (*ast.RecvExpr, error) {
	p.advance() // "recv!"
	ch, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewRecvExpr(pos, ch), nil
}

func (p *parser) parseClose(pos ast.Position) (*ast.CloseStmt, error) {
	p.advance() // "close!"
	ch, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewCloseStmt(pos, ch), nil
}

func (p *parser) parseGoVal(pos ast.Position) (*ast.GoValExpr, error) {
	p.advance() // "go-val"
	// Optional element type: (go-val string body...) → chan string
	var elemType *ast.TypeExpr
	if p.isBindingTypeStart() {
		t, err := p.parseTypeExpr()
		if err != nil {
			return nil, err
		}
		elemType = t
	}
	body, err := p.parseBody()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewGoValExpr(pos, elemType, body), nil
}

func (p *parser) parsePar(pos ast.Position) (*ast.ParStmt, error) {
	p.advance() // "par"
	var bodies []ast.Node
	for p.peekType() != lexer.TokenRParen && p.peekType() != lexer.TokenEOF {
		node, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		bodies = append(bodies, node)
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewParStmt(pos, bodies), nil
}

func (p *parser) parseRecvOk(pos ast.Position) (*ast.RecvOkExpr, error) {
	p.advance() // "recv-ok!"
	ch, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewRecvOkExpr(pos, ch), nil
}

func (p *parser) parseForChan(pos ast.Position) (*ast.ForChanStmt, error) {
	p.advance() // "for-chan"
	if _, err := p.expect(lexer.TokenLBracket); err != nil {
		return nil, err
	}
	bindTok, err := p.expect(lexer.TokenSymbol)
	if err != nil {
		return nil, err
	}
	ch, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRBracket); err != nil {
		return nil, err
	}
	body, err := p.parseBody()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	sym := ast.NewSymbol(p.mkpos(bindTok), bindTok.Text)
	return ast.NewForChanStmt(pos, sym, ch, body), nil
}

func (p *parser) parseWithLock(pos ast.Position) (*ast.WithLockExpr, error) {
	p.advance() // "with-lock"
	mu, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	body, err := p.parseBody()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewWithLockExpr(pos, mu, body), nil
}

func (p *parser) parsePipeline(pos ast.Position) (*ast.PipelineExpr, error) {
	p.advance() // "pipeline"
	if _, err := p.expect(lexer.TokenLBracket); err != nil {
		return nil, err
	}
	bindTok, err := p.expect(lexer.TokenSymbol)
	if err != nil {
		return nil, err
	}
	source, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRBracket); err != nil {
		return nil, err
	}
	var stages []ast.Node
	for p.peekType() != lexer.TokenRParen && p.peekType() != lexer.TokenEOF {
		stage, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		stages = append(stages, stage)
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	sym := ast.NewSymbol(p.mkpos(bindTok), bindTok.Text)
	return ast.NewPipelineExpr(pos, sym, source, stages), nil
}

func (p *parser) parseFanOut(pos ast.Position) (*ast.FanOutStmt, error) {
	p.advance() // "fan-out"
	n, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenLBracket); err != nil {
		return nil, err
	}
	bindTok, err := p.expect(lexer.TokenSymbol)
	if err != nil {
		return nil, err
	}
	ch, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRBracket); err != nil {
		return nil, err
	}
	body, err := p.parseBody()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	sym := ast.NewSymbol(p.mkpos(bindTok), bindTok.Text)
	return ast.NewFanOutStmt(pos, n, sym, ch, body), nil
}

func (p *parser) parseFanIn(pos ast.Position) (*ast.FanInExpr, error) {
	p.advance() // "fan-in"
	var chans []ast.Node
	for p.peekType() != lexer.TokenRParen && p.peekType() != lexer.TokenEOF {
		ch, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		chans = append(chans, ch)
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewFanInExpr(pos, chans), nil
}

func (p *parser) parseSelect(pos ast.Position) (*ast.SelectStmt, error) {
	p.advance() // "select!"
	var cases []ast.SelectCase
	for p.peekType() != lexer.TokenRParen && p.peekType() != lexer.TokenEOF {
		if _, err := p.expect(lexer.TokenLParen); err != nil {
			return nil, err
		}
		c, err := p.parseSelectCase()
		if err != nil {
			return nil, err
		}
		cases = append(cases, c)
		if _, err := p.expect(lexer.TokenRParen); err != nil {
			return nil, err
		}
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewSelectStmt(pos, cases), nil
}

func (p *parser) parseSelectCase() (ast.SelectCase, error) {
	var sc ast.SelectCase
	if p.peekType() == lexer.TokenKeyword {
		kw := p.peek().Text
		if kw == "default" {
			p.advance()
			body, err := p.parseBody()
			if err != nil {
				return sc, err
			}
			sc.IsDefault = true
			sc.Body = body
			return sc, nil
		}
		if kw == "timeout" {
			p.advance() // :timeout
			ms, err := p.parseExpr()
			if err != nil {
				return sc, err
			}
			body, err := p.parseBody()
			if err != nil {
				return sc, err
			}
			sc.IsTimeout = true
			sc.TimeoutMs = ms
			sc.Body = body
			return sc, nil
		}
	}
	if _, err := p.expect(lexer.TokenLBracket); err != nil {
		return sc, err
	}
	if p.peekType() == lexer.TokenLParen {
		p.advance() // (
		sym, err := p.expect(lexer.TokenSymbol)
		if err != nil || sym.Text != "send!" {
			return sc, fmt.Errorf("expected send! in select send case")
		}
		ch, err := p.parseExpr()
		if err != nil {
			return sc, err
		}
		val, err := p.parseExpr()
		if err != nil {
			return sc, err
		}
		if _, err := p.expect(lexer.TokenRParen); err != nil {
			return sc, err
		}
		if _, err := p.expect(lexer.TokenRBracket); err != nil {
			return sc, err
		}
		sc.IsSend = true
		sc.ChanExpr = ch
		sc.SendVal = val
	} else {
		bindTok, err := p.expect(lexer.TokenSymbol)
		if err != nil {
			return sc, err
		}
		ch, err := p.parseExpr()
		if err != nil {
			return sc, err
		}
		if _, err := p.expect(lexer.TokenRBracket); err != nil {
			return sc, err
		}
		sc.Binding = bindTok.Text
		sc.ChanExpr = ch
	}
	body, err := p.parseBody()
	if err != nil {
		return sc, err
	}
	sc.Body = body
	return sc, nil
}

func (p *parser) parseLoop(pos ast.Position) (*ast.LoopExpr, error) {
	p.advance() // "loop"
	if _, err := p.expect(lexer.TokenLBracket); err != nil {
		return nil, err
	}
	var bindings []ast.LetBinding
	for p.peekType() != lexer.TokenRBracket && p.peekType() != lexer.TokenEOF {
		symTok, err := p.expect(lexer.TokenSymbol)
		if err != nil {
			return nil, err
		}
		// Optional type annotation: (loop [xs []string [] n int 0] ...)
		var annot *ast.TypeExpr
		if p.isBindingTypeStart() {
			t, err := p.parseTypeExpr()
			if err != nil {
				return nil, err
			}
			annot = t
		}
		val, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		pat := ast.NewSymbol(p.mkpos(symTok), symTok.Text)
		bindings = append(bindings, ast.LetBinding{Pattern: pat, TypeAnnot: annot, Value: val})
	}
	if _, err := p.expect(lexer.TokenRBracket); err != nil {
		return nil, err
	}
	body, err := p.parseBody()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewLoopExpr(pos, bindings, body), nil
}

func (p *parser) parseRecur(pos ast.Position) (*ast.RecurExpr, error) {
	p.advance() // "recur"
	args, err := p.parseArgs()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewRecurExpr(pos, args), nil
}

func (p *parser) parseReturn(pos ast.Position) (*ast.ReturnExpr, error) {
	p.advance() // "return"
	args, err := p.parseArgs()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewReturnExpr(pos, args), nil
}

func (p *parser) parseValues(pos ast.Position) (*ast.ValuesExpr, error) {
	p.advance() // "values"
	args, err := p.parseArgs()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewValuesExpr(pos, args), nil
}

func (p *parser) parseIfErr(pos ast.Position) (*ast.IfErrExpr, error) {
	p.advance() // "if-err"
	if _, err := p.expect(lexer.TokenLBracket); err != nil {
		return nil, err
	}
	valTok, err := p.expect(lexer.TokenSymbol)
	if err != nil {
		return nil, err
	}
	errTok, err := p.expect(lexer.TokenSymbol)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRBracket); err != nil {
		return nil, err
	}
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	onErr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	onOk, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewIfErrExpr(pos, valTok.Text, errTok.Text, expr, onErr, onOk), nil
}

// parseBindPattern parses a single binding pattern — a symbol, a sequential
// vector destructure, or a map destructure — matching the forms accepted in
// let bindings. Used by if-let / when-let.
func (p *parser) parseBindPattern() (ast.Node, error) {
	switch p.peekType() {
	case lexer.TokenLBracket:
		return p.parseVector()
	case lexer.TokenLBrace:
		return p.parseMap()
	default:
		symTok, err := p.expect(lexer.TokenSymbol)
		if err != nil {
			return nil, err
		}
		return ast.NewSymbol(p.mkpos(symTok), symTok.Text), nil
	}
}

func (p *parser) parseIfLet(pos ast.Position) (*ast.IfLetExpr, error) {
	p.advance() // "if-let"
	if _, err := p.expect(lexer.TokenLBracket); err != nil {
		return nil, err
	}
	pattern, err := p.parseBindPattern()
	if err != nil {
		return nil, err
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRBracket); err != nil {
		return nil, err
	}
	then, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	var els ast.Node
	if p.peekType() != lexer.TokenRParen {
		els, err = p.parseExpr()
		if err != nil {
			return nil, err
		}
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewIfLetExpr(pos, pattern, value, then, els), nil
}

func (p *parser) parseLetOr(pos ast.Position) (*ast.LetOrExpr, error) {
	p.advance() // "let-or"
	if _, err := p.expect(lexer.TokenLBracket); err != nil {
		return nil, err
	}
	var bindings []ast.LetOrBinding
	for p.peekType() != lexer.TokenRBracket && p.peekType() != lexer.TokenEOF {
		nameTok, err := p.expect(lexer.TokenSymbol)
		if err != nil {
			return nil, err
		}
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		fallback, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, ast.LetOrBinding{
			Name:     nameTok.Text,
			Expr:     expr,
			Fallback: fallback,
		})
	}
	if _, err := p.expect(lexer.TokenRBracket); err != nil {
		return nil, err
	}
	body, err := p.parseBody()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return &ast.LetOrExpr{Pos_: pos, Bindings: bindings, Body: body}, nil
}

func (p *parser) parseWhenLet(pos ast.Position) (*ast.WhenLetExpr, error) {
	p.advance() // "when-let"
	if _, err := p.expect(lexer.TokenLBracket); err != nil {
		return nil, err
	}
	pattern, err := p.parseBindPattern()
	if err != nil {
		return nil, err
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRBracket); err != nil {
		return nil, err
	}
	body, err := p.parseBody()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewWhenLetExpr(pos, pattern, value, body), nil
}

func (p *parser) parseTypeAssert(pos ast.Position) (*ast.TypeAssertExpr, error) {
	p.advance() // "as"
	ty, err := p.parseTypeExpr()
	if err != nil {
		return nil, err
	}
	val, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewTypeAssertExpr(pos, ty, val), nil
}

func (p *parser) parseMethodCall(pos ast.Position) (*ast.MethodCallExpr, error) {
	symTok := p.advance() // .Method
	method := symTok.Text[1:]
	obj, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	args, err := p.parseArgs()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewMethodCallExpr(pos, method, obj, args), nil
}

func (p *parser) parseFieldAccess(pos ast.Position) (*ast.FieldAccessExpr, error) {
	symTok := p.advance() // .-Field
	field := symTok.Text[2:]
	obj, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewFieldAccessExpr(pos, field, obj), nil
}

func (p *parser) parseStructLit(pos ast.Position) (*ast.StructLitExpr, error) {
	symTok := p.advance() // TypeName.
	typeName := symTok.Text[:len(symTok.Text)-1]
	if p.peekType() != lexer.TokenLBrace {
		return nil, p.errorf("struct literal requires a map literal {}, got %v", p.peekType())
	}
	m, err := p.parseMap()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewStructLitExpr(pos, typeName, m.Pairs), nil
}

func (p *parser) parseCall(pos ast.Position) (*ast.CallExpr, error) {
	head, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	args, err := p.parseArgs()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewCallExpr(pos, head, args), nil
}

// ---------- helpers ----------

func (p *parser) parseParamList() ([]ast.Param, error) {
	if _, err := p.expect(lexer.TokenLBracket); err != nil {
		return nil, err
	}
	var params []ast.Param
	for p.peekType() != lexer.TokenRBracket && p.peekType() != lexer.TokenEOF {
		// Destructuring patterns have no type annotation
		if p.peekType() == lexer.TokenLBracket {
			v, err := p.parseVector()
			if err != nil {
				return nil, err
			}
			params = append(params, ast.Param{Pattern: v})
			continue
		}
		if p.peekType() == lexer.TokenLBrace {
			m, err := p.parseMap()
			if err != nil {
				return nil, err
			}
			params = append(params, ast.Param{Pattern: m})
			continue
		}
		symTok, err := p.expect(lexer.TokenSymbol)
		if err != nil {
			return nil, err
		}
		if symTok.Text == "&" {
			restTok, err := p.expect(lexer.TokenSymbol)
			if err != nil {
				return nil, err
			}
			// rest param may have a type annotation
			var restAnnot *ast.TypeExpr
			if p.isTypeExprStart() {
				restAnnot, err = p.parseTypeExpr()
				if err != nil {
					return nil, err
				}
			}
			params = append(params, ast.Param{Name: restTok.Text, TypeAnnot: restAnnot, IsRest: true})
			continue
		}
		// Type annotation follows the param name when the next token looks like a type
		var annot *ast.TypeExpr
		if p.isTypeExprStart() {
			annot, err = p.parseTypeExpr()
			if err != nil {
				return nil, err
			}
		}
		params = append(params, ast.Param{Name: symTok.Text, TypeAnnot: annot})
	}
	if _, err := p.expect(lexer.TokenRBracket); err != nil {
		return nil, err
	}
	return params, nil
}

func (p *parser) parseBody() ([]ast.Node, error) {
	var body []ast.Node
	for p.peekType() != lexer.TokenRParen && p.peekType() != lexer.TokenEOF {
		node, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		body = append(body, node)
	}
	return body, nil
}

func (p *parser) parseArgs() ([]ast.Node, error) {
	return p.parseBody()
}

// ---------- type expression parsing ----------

// parseTypeExpr reads a type expression in a position where a Go type is expected.
// Handles: symbols (int, string, *Foo, web/Bar), []T, [T1 T2] (multi-return),
// (chan T), and map[K]V — without needing a ^ prefix.
func (p *parser) parseTypeExpr() (*ast.TypeExpr, error) {
	tok := p.peek()
	pos := p.mkpos(tok)

	switch p.peekType() {
	case lexer.TokenLBracket:
		p.advance() // consume [
		if p.peekType() == lexer.TokenRBracket {
			// []T — slice type
			p.advance() // consume ]
			elem, err := p.parseTypeExpr()
			if err != nil {
				return nil, err
			}
			return ast.NewTypeExpr(pos, "[]"+elem.Text), nil
		}
		// [T1 T2 ...] — multi-return type
		var parts []string
		for p.peekType() != lexer.TokenRBracket && p.peekType() != lexer.TokenEOF {
			t, err := p.parseTypeExpr()
			if err != nil {
				return nil, err
			}
			parts = append(parts, t.Text)
		}
		if _, err := p.expect(lexer.TokenRBracket); err != nil {
			return nil, err
		}
		return ast.NewTypeExpr(pos, "["+strings.Join(parts, " ")+"]"), nil

	case lexer.TokenLParen:
		// (chan T) — channel type
		p.advance() // consume (
		chanTok, err := p.expect(lexer.TokenSymbol)
		if err != nil {
			return nil, err
		}
		if chanTok.Text != "chan" {
			return nil, p.errorf("expected 'chan' in type expression, got %q", chanTok.Text)
		}
		elem, err := p.parseTypeExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.TokenRParen); err != nil {
			return nil, err
		}
		return ast.NewTypeExpr(pos, "chan "+elem.Text), nil

	case lexer.TokenSymbol:
		symTok := p.advance()
		text := symTok.Text
		// Handle map[K]V
		if text == "map" && p.peekType() == lexer.TokenLBracket {
			p.advance() // consume [
			keyType, err := p.parseTypeExpr()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.TokenRBracket); err != nil {
				return nil, err
			}
			valType, err := p.parseTypeExpr()
			if err != nil {
				return nil, err
			}
			return ast.NewTypeExpr(pos, "map["+keyType.Text+"]"+valType.Text), nil
		}
		return ast.NewTypeExpr(pos, text), nil

	default:
		return nil, p.errorf("expected type expression, got %v (%q)", p.peekType(), p.peek().Text)
	}
}

// isTypeExprStart reports whether the current token can begin a type expression
// in a position where a type annotation is expected after a name.
func (p *parser) isTypeExprStart() bool {
	switch p.peekType() {
	case lexer.TokenLBracket, lexer.TokenLParen:
		return true
	case lexer.TokenSymbol:
		return isTypeSymbol(p.peek().Text)
	}
	return false
}

// isTypeSymbol reports whether a symbol text looks like a Go type name rather
// than a variable name. Used to disambiguate param/binding syntax.
func isTypeSymbol(text string) bool {
	switch text {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "complex64", "complex128",
		"bool", "byte", "rune", "string", "error", "any", "uintptr",
		"map":
		return true
	}
	if strings.Contains(text, "/") {
		return true // qualified type like web/Request or sync/Mutex
	}
	if strings.HasPrefix(text, "*") {
		return true // pointer type like *Circle
	}
	if len(text) > 0 && text[0] >= 'A' && text[0] <= 'Z' {
		return true // exported/PascalCase type like Circle, Handler
	}
	return false
}

// ---------- deftest ----------

func (p *parser) parseDefTest(pos ast.Position) (*ast.DefTestDecl, error) {
	p.advance() // consume "deftest"
	nameTok, err := p.expect(lexer.TokenSymbol)
	if err != nil {
		return nil, err
	}
	body, err := p.parseBody()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewDefTestDecl(pos, nameTok.Text, body), nil
}
