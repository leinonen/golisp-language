package parser

import (
	"fmt"
	"strconv"
	"strings"

	"golisp/internal/ast"
	"golisp/internal/lexer"
)

// parseAnonFn parses a #(...) anonymous function literal.
// #(+ % 1) → (fn [_anonP1] (+ _anonP1 1))
// #(+ %1 %2) → (fn [_anonP1 _anonP2] (+ _anonP1 _anonP2))
// #(apply f %&) → (fn [& _anonPRest] (apply f _anonPRest))
func (p *parser) parseAnonFn() (*ast.FnExpr, error) {
	tok := p.advance() // consume #(
	pos := p.mkpos(tok)

	// Parse forms until matching )
	var forms []ast.Node
	for p.peekType() != lexer.TokenRParen && p.peekType() != lexer.TokenEOF {
		form, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		forms = append(forms, form)
	}
	if _, err := p.expect(lexer.TokenRParen); err != nil {
		return nil, err
	}

	// Build the single body expression
	var bodyExpr ast.Node
	switch len(forms) {
	case 0:
		bodyExpr = ast.NewNilLit(pos)
	case 1:
		bodyExpr = forms[0]
	default:
		bodyExpr = ast.NewCallExpr(pos, forms[0], forms[1:])
	}

	// Collect which % args are used
	maxN, hasRest := collectAnonArgs(bodyExpr)

	// Build params
	var params []ast.Param
	for i := 1; i <= maxN; i++ {
		params = append(params, ast.Param{Name: fmt.Sprintf("_anonP%d", i)})
	}
	if hasRest {
		params = append(params, ast.Param{Name: "_anonPRest", IsRest: true})
	}

	// Replace % symbols in body
	bodyExpr = replaceAnonArgs(bodyExpr)

	return ast.NewFnExpr(pos, params, nil, []ast.Node{bodyExpr}), nil
}

// anonParamName returns the Go param name for a % symbol, or ("", false) if not one.
func anonParamName(name string) (string, bool) {
	switch name {
	case "%", "%1":
		return "_anonP1", true
	case "%&":
		return "_anonPRest", true
	}
	if strings.HasPrefix(name, "%") {
		n, err := strconv.Atoi(name[1:])
		if err == nil && n >= 1 {
			return fmt.Sprintf("_anonP%d", n), true
		}
	}
	return "", false
}

// collectAnonArgs walks the AST to find the highest-numbered % arg and whether %& is used.
// Does not recurse into nested FnExpr (they have their own scope).
func collectAnonArgs(node ast.Node) (maxN int, hasRest bool) {
	if node == nil {
		return
	}
	switch n := node.(type) {
	case *ast.Symbol:
		switch n.Name {
		case "%", "%1":
			if maxN < 1 {
				maxN = 1
			}
		case "%&":
			hasRest = true
		default:
			if strings.HasPrefix(n.Name, "%") {
				if i, err := strconv.Atoi(n.Name[1:]); err == nil && i > maxN {
					maxN = i
				}
			}
		}
	case *ast.CallExpr:
		m, r := collectAnonArgs(n.Head)
		if m > maxN { maxN = m }
		if r { hasRest = true }
		for _, a := range n.Args {
			m, r = collectAnonArgs(a)
			if m > maxN { maxN = m }
			if r { hasRest = true }
		}
	case *ast.LetExpr:
		for _, b := range n.Bindings {
			m, r := collectAnonArgs(b.Value)
			if m > maxN { maxN = m }
			if r { hasRest = true }
		}
		for _, b := range n.Body {
			m, r := collectAnonArgs(b)
			if m > maxN { maxN = m }
			if r { hasRest = true }
		}
	case *ast.IfExpr:
		for _, sub := range []ast.Node{n.Cond, n.Then, n.Else} {
			m, r := collectAnonArgs(sub)
			if m > maxN { maxN = m }
			if r { hasRest = true }
		}
	case *ast.WhenExpr:
		m, r := collectAnonArgs(n.Cond)
		if m > maxN { maxN = m }
		if r { hasRest = true }
		for _, b := range n.Body {
			m, r = collectAnonArgs(b)
			if m > maxN { maxN = m }
			if r { hasRest = true }
		}
	case *ast.DoExpr:
		for _, b := range n.Body {
			m, r := collectAnonArgs(b)
			if m > maxN { maxN = m }
			if r { hasRest = true }
		}
	case *ast.CondExpr:
		for _, cl := range n.Clauses {
			for _, sub := range []ast.Node{cl.Test, cl.Body} {
				m, r := collectAnonArgs(sub)
				if m > maxN { maxN = m }
				if r { hasRest = true }
			}
		}
		m, r := collectAnonArgs(n.Default)
		if m > maxN { maxN = m }
		if r { hasRest = true }
	case *ast.VectorLit:
		for _, e := range n.Elements {
			m, r := collectAnonArgs(e)
			if m > maxN { maxN = m }
			if r { hasRest = true }
		}
	case *ast.MapLit:
		for _, pair := range n.Pairs {
			m, r := collectAnonArgs(pair.Value)
			if m > maxN { maxN = m }
			if r { hasRest = true }
		}
	case *ast.SetLit:
		for _, e := range n.Elements {
			m, r := collectAnonArgs(e)
			if m > maxN { maxN = m }
			if r { hasRest = true }
		}
	case *ast.MethodCallExpr:
		m, r := collectAnonArgs(n.Object)
		if m > maxN { maxN = m }
		if r { hasRest = true }
		for _, a := range n.Args {
			m, r = collectAnonArgs(a)
			if m > maxN { maxN = m }
			if r { hasRest = true }
		}
	case *ast.TypeAssertExpr:
		m, r := collectAnonArgs(n.Value)
		if m > maxN { maxN = m }
		if r { hasRest = true }
	case *ast.IfErrExpr:
		for _, sub := range []ast.Node{n.Expr, n.OnErr, n.OnOk} {
			m, r := collectAnonArgs(sub)
			if m > maxN { maxN = m }
			if r { hasRest = true }
		}
	case *ast.FnExpr:
		// don't recurse — nested fn has its own % scope
	}
	return
}

// replaceAnonArgs walks the AST and replaces % symbols with their _anonPN names.
// Does not recurse into nested FnExpr.
func replaceAnonArgs(node ast.Node) ast.Node {
	if node == nil {
		return nil
	}
	switch n := node.(type) {
	case *ast.Symbol:
		if name, ok := anonParamName(n.Name); ok {
			return ast.NewSymbol(n.Pos(), name)
		}
	case *ast.CallExpr:
		n.Head = replaceAnonArgs(n.Head)
		for i, a := range n.Args {
			n.Args[i] = replaceAnonArgs(a)
		}
	case *ast.LetExpr:
		for i := range n.Bindings {
			n.Bindings[i].Value = replaceAnonArgs(n.Bindings[i].Value)
		}
		for i, b := range n.Body {
			n.Body[i] = replaceAnonArgs(b)
		}
	case *ast.IfExpr:
		n.Cond = replaceAnonArgs(n.Cond)
		n.Then = replaceAnonArgs(n.Then)
		n.Else = replaceAnonArgs(n.Else)
	case *ast.WhenExpr:
		n.Cond = replaceAnonArgs(n.Cond)
		for i, b := range n.Body {
			n.Body[i] = replaceAnonArgs(b)
		}
	case *ast.DoExpr:
		for i, b := range n.Body {
			n.Body[i] = replaceAnonArgs(b)
		}
	case *ast.CondExpr:
		for i := range n.Clauses {
			n.Clauses[i].Test = replaceAnonArgs(n.Clauses[i].Test)
			n.Clauses[i].Body = replaceAnonArgs(n.Clauses[i].Body)
		}
		n.Default = replaceAnonArgs(n.Default)
	case *ast.VectorLit:
		for i, e := range n.Elements {
			n.Elements[i] = replaceAnonArgs(e)
		}
	case *ast.MapLit:
		for i := range n.Pairs {
			n.Pairs[i].Value = replaceAnonArgs(n.Pairs[i].Value)
		}
	case *ast.SetLit:
		for i, e := range n.Elements {
			n.Elements[i] = replaceAnonArgs(e)
		}
	case *ast.MethodCallExpr:
		n.Object = replaceAnonArgs(n.Object)
		for i, a := range n.Args {
			n.Args[i] = replaceAnonArgs(a)
		}
	case *ast.TypeAssertExpr:
		n.Value = replaceAnonArgs(n.Value)
	case *ast.IfErrExpr:
		n.Expr = replaceAnonArgs(n.Expr)
		n.OnErr = replaceAnonArgs(n.OnErr)
		n.OnOk = replaceAnonArgs(n.OnOk)
	case *ast.FnExpr:
		// don't recurse — nested fn has its own % scope
	}
	return node
}
