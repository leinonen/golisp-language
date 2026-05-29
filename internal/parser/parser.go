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
	"defun":    "defn",
	"define":   "def or defn",
	"lambda":   "fn",
	"begin":    "do",
	"defmacro": "(macros not yet supported)",
	"defvar":   "def",
	"setq":     "def",
	"progn":    "do",
	"funcall":  "(just call the function directly)",
	"apply":    "(just call the function directly)",
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

type parser struct {
	tokens     []lexer.Token
	pos        int
	src        string // original source text for error context; may be empty
	pendingDoc string // set by ;;; doc comment, consumed by parseDefn/parseDefmethod
}

func (p *parser) peek() lexer.Token {
	if p.pos >= len(p.tokens) {
		return lexer.Token{Type: lexer.TokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *parser) peekType() lexer.TokenType { return p.peek().Type }

func (p *parser) advance() lexer.Token {
	tok := p.peek()
	if tok.Type != lexer.TokenEOF {
		p.pos++
	}
	return tok
}

func (p *parser) expect(tt lexer.TokenType) (lexer.Token, error) {
	tok := p.peek()
	if tok.Type != tt {
		return tok, p.errorf("expected %v, got %v (%q)", tt, tok.Type, tok.Text)
	}
	return p.advance(), nil
}

func (p *parser) errorf(format string, args ...any) error {
	tok := p.peek()
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
	if col < 1 {
		col = 1
	}
	pointer := strings.Repeat(" ", col-1) + "^"
	return fmt.Sprintf("\n  %s\n  %s", srcLine, pointer)
}

func (p *parser) mkpos(tok lexer.Token) ast.Position {
	return ast.Position{Line: tok.Line, Column: tok.Column}
}

// ---------- top-level ----------

func (p *parser) parseAll() ([]ast.Node, error) {
	var nodes []ast.Node
	for p.peekType() != lexer.TokenEOF {
		if p.peekType() == lexer.TokenDocComment {
			p.pendingDoc = p.advance().Text
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
	// Consume an optional leading type annotation.
	var annot *ast.TypeExpr
	if p.peekType() == lexer.TokenTypeAnnot {
		tok := p.advance()
		annot = ast.NewTypeExpr(p.mkpos(tok), tok.Text)
	}

	node, err := p.parseExprInner()
	if err != nil {
		return nil, err
	}

	// Attach annotation to collections (typed vector/map literals).
	if annot != nil {
		switch v := node.(type) {
		case *ast.VectorLit:
			v.TypeAnnot = annot
		case *ast.MapLit:
			v.TypeAnnot = annot
		case *ast.Symbol:
			v.TypeAnnot = annot
		}
	}
	return node, nil
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
	case lexer.TokenEOF:
		return nil, p.errorf("unexpected EOF")
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
	for p.peekType() != lexer.TokenRBrace && p.peekType() != lexer.TokenEOF {
		if _, err := p.parseExpr(); err != nil {
			return nil, err
		}
	}
	if _, err := p.expect(lexer.TokenRBrace); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("%d:%d: set literals not supported", open.Line, open.Column)
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
		case "defer":
			return p.parseDefer(pos)
		case "chan":
			return p.parseChan(pos)
		case "send!":
			return p.parseSend(pos)
		case "recv!":
			return p.parseRecv(pos)
		case "close!":
			return p.parseClose(pos)
		case "select!":
			return p.parseSelect(pos)
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
		case "as":
			return p.parseTypeAssert(pos)
		case "deftest":
			return p.parseDefTest(pos)
		}

		// "Did you mean?" hints for common mistakes from other Lisps.
		if hint, ok := didYouMean[head.Text]; ok {
			p.advance() // consume the bad symbol so error points at it
			return nil, p.errorf("%q is not valid glisp — did you mean %s?", head.Text, hint)
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
	for p.peekType() != lexer.TokenRParen && p.peekType() != lexer.TokenEOF {
		if p.peekType() != lexer.TokenLParen {
			return nil, p.errorf("expected import clause in ns")
		}
		p.advance() // (
		kwTok, err := p.expect(lexer.TokenKeyword)
		if err != nil {
			return nil, err
		}
		if kwTok.Text != "import" {
			return nil, fmt.Errorf("%d:%d: expected :import, got :%s", kwTok.Line, kwTok.Column, kwTok.Text)
		}
		specs, err := p.parseImportList()
		if err != nil {
			return nil, err
		}
		imports = append(imports, specs...)
		if _, err := p.expect(lexer.TokenRParen); err != nil {
			return nil, err
		}
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}
	return ast.NewNSDecl(pos, name, imports), nil
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

func (p *parser) parseDef(pos ast.Position) (*ast.DefDecl, error) {
	p.advance() // "def"
	var annot *ast.TypeExpr
	if p.peekType() == lexer.TokenTypeAnnot {
		tok := p.advance()
		annot = ast.NewTypeExpr(p.mkpos(tok), tok.Text)
	}
	nameTok, err := p.expect(lexer.TokenSymbol)
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
	var retType *ast.TypeExpr
	if p.peekType() == lexer.TokenTypeAnnot {
		tok := p.advance()
		retType = ast.NewTypeExpr(p.mkpos(tok), tok.Text)
	}
	nameTok, err := p.expect(lexer.TokenSymbol)
	if err != nil {
		return nil, err
	}
	params, err := p.parseParamList()
	if err != nil {
		return nil, err
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

func (p *parser) parseDefstruct(pos ast.Position) (*ast.StructDecl, error) {
	p.advance() // "defstruct"
	nameTok, err := p.expect(lexer.TokenSymbol)
	if err != nil {
		return nil, err
	}
	var fields []ast.StructField
	for p.peekType() != lexer.TokenRParen && p.peekType() != lexer.TokenEOF {
		var annot *ast.TypeExpr
		if p.peekType() == lexer.TokenTypeAnnot {
			tok := p.advance()
			annot = ast.NewTypeExpr(p.mkpos(tok), tok.Text)
		}
		fieldTok, err := p.expect(lexer.TokenSymbol)
		if err != nil {
			return nil, err
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
		var retType *ast.TypeExpr
		if p.peekType() == lexer.TokenTypeAnnot {
			tok := p.advance()
			retType = ast.NewTypeExpr(p.mkpos(tok), tok.Text)
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
	recvTypeTok, err := p.expect(lexer.TokenTypeAnnot)
	if err != nil {
		return nil, err
	}
	recvType := ast.NewTypeExpr(p.mkpos(recvTypeTok), recvTypeTok.Text)
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
	var retType *ast.TypeExpr
	if p.peekType() == lexer.TokenTypeAnnot {
		tok := p.advance()
		retType = ast.NewTypeExpr(p.mkpos(tok), tok.Text)
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
	var retType *ast.TypeExpr
	if p.peekType() == lexer.TokenTypeAnnot {
		tok := p.advance()
		retType = ast.NewTypeExpr(p.mkpos(tok), tok.Text)
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
		var annot *ast.TypeExpr
		if p.peekType() == lexer.TokenTypeAnnot {
			tok := p.advance()
			annot = ast.NewTypeExpr(p.mkpos(tok), tok.Text)
		}
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
	annotTok, err := p.expect(lexer.TokenTypeAnnot)
	if err != nil {
		return nil, err
	}
	elemType := ast.NewTypeExpr(p.mkpos(annotTok), annotTok.Text)
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
	if p.peekType() == lexer.TokenKeyword && p.peek().Text == "default" {
		p.advance()
		body, err := p.parseBody()
		if err != nil {
			return sc, err
		}
		sc.IsDefault = true
		sc.Body = body
		return sc, nil
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
		val, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		pat := ast.NewSymbol(p.mkpos(symTok), symTok.Text)
		bindings = append(bindings, ast.LetBinding{Pattern: pat, Value: val})
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

func (p *parser) parseTypeAssert(pos ast.Position) (*ast.TypeAssertExpr, error) {
	p.advance() // "as"
	annotTok, err := p.expect(lexer.TokenTypeAnnot)
	if err != nil {
		return nil, err
	}
	ty := ast.NewTypeExpr(p.mkpos(annotTok), annotTok.Text)
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
		var annot *ast.TypeExpr
		if p.peekType() == lexer.TokenTypeAnnot {
			tok := p.advance()
			annot = ast.NewTypeExpr(p.mkpos(tok), tok.Text)
		}
		if p.peekType() == lexer.TokenLBracket {
			v, err := p.parseVector()
			if err != nil {
				return nil, err
			}
			params = append(params, ast.Param{Pattern: v, TypeAnnot: annot})
			continue
		}
		if p.peekType() == lexer.TokenLBrace {
			m, err := p.parseMap()
			if err != nil {
				return nil, err
			}
			params = append(params, ast.Param{Pattern: m, TypeAnnot: annot})
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
			params = append(params, ast.Param{Name: restTok.Text, TypeAnnot: annot, IsRest: true})
			continue
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
