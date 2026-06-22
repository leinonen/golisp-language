package macro

import "golisp/internal/ast"

// unparse converts a parser-specialized form (if/when/do/cond/let/loop/fn/quote)
// back into the generic list/vector AST it was read from, so the macro layer can
// treat every form uniformly as data. The child sub-forms are the *original*
// nodes (so any ~/~@ inside them are preserved for syntax-quote expansion).
//
// Leaf and already-generic nodes (literals, symbols, CallExpr, VectorLit, …)
// return (nil, false): the caller handles them directly.
//
// Limitation: the glisp parser specializes header-rigid forms (defn/def/
// defstruct/…) and requires concrete tokens in some positions (a defn name, let
// binding names), so templates like `(defn ~name …)` do not parse. Covering the
// header-rigid forms — and a generic reader for quoted contexts — is tracked for
// a later slice. unparse covers the expression forms whose children are
// ordinary expressions, which is what templates use in practice.
func unparse(n ast.Node) (ast.Node, bool) {
	switch v := n.(type) {
	case *ast.IfExpr:
		args := []ast.Node{v.Cond, v.Then}
		if v.Else != nil {
			args = append(args, v.Else)
		}
		return callNode(n.Pos(), "if", args...), true
	case *ast.WhenExpr:
		return callNode(n.Pos(), "when", prepend(v.Cond, v.Body)...), true
	case *ast.DoExpr:
		return callNode(n.Pos(), "do", v.Body...), true
	case *ast.CondExpr:
		var parts []ast.Node
		for _, c := range v.Clauses {
			parts = append(parts, c.Test, c.Body)
		}
		if v.Default != nil {
			parts = append(parts, ast.NewKeywordLit(n.Pos(), "else"), v.Default)
		}
		return callNode(n.Pos(), "cond", parts...), true
	case *ast.LetExpr:
		return callNode(n.Pos(), "let", prepend(bindingVector(n.Pos(), v.Bindings), v.Body)...), true
	case *ast.LoopExpr:
		return callNode(n.Pos(), "loop", prepend(bindingVector(n.Pos(), v.Bindings), v.Body)...), true
	case *ast.FnExpr:
		return callNode(n.Pos(), "fn", prepend(paramVector(n.Pos(), v.Params), v.Body)...), true
	case *ast.QuoteExpr:
		return callNode(n.Pos(), "quote", v.Form), true
	default:
		return nil, false
	}
}

func callNode(pos ast.Position, head string, args ...ast.Node) *ast.CallExpr {
	return ast.NewCallExpr(pos, ast.NewSymbol(pos, head), args)
}

func prepend(first ast.Node, rest []ast.Node) []ast.Node {
	return append([]ast.Node{first}, rest...)
}

// bindingVector reconstructs a let/loop binding vector [name value name value …].
// Type annotations on bindings are not preserved (templates rarely type them).
func bindingVector(pos ast.Position, bindings []ast.LetBinding) *ast.VectorLit {
	var elems []ast.Node
	for _, b := range bindings {
		elems = append(elems, b.Pattern, b.Value)
	}
	return ast.NewVectorLit(pos, elems)
}

// paramVector reconstructs an fn parameter vector. Type annotations are not
// preserved; a & rest param is rendered as the `&` symbol followed by the name.
func paramVector(pos ast.Position, params []ast.Param) *ast.VectorLit {
	var elems []ast.Node
	for _, p := range params {
		if p.IsRest {
			elems = append(elems, ast.NewSymbol(pos, "&"))
		}
		if p.Pattern != nil {
			elems = append(elems, p.Pattern)
		} else {
			elems = append(elems, ast.NewSymbol(pos, p.Name))
		}
	}
	return ast.NewVectorLit(pos, elems)
}
