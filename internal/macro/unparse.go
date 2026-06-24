package macro

import (
	"fmt"

	"golisp/internal/ast"
	"golisp/internal/parser"
)

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
		args := []ast.Node{paramVector(n.Pos(), v.Params)}
		// Preserve a single-symbol return type as `-> ret` so a typed fn template
		// round-trips (fnNode reads it back). Compound return types are dropped.
		if v.ReturnType != nil && parser.IsTypeSymbol(v.ReturnType.Text) {
			args = append(args, ast.NewSymbol(n.Pos(), "->"), ast.NewSymbol(n.Pos(), v.ReturnType.Text))
		}
		args = append(args, v.Body...)
		return callNode(n.Pos(), "fn", args...), true
	case *ast.QuoteExpr:
		return callNode(n.Pos(), "quote", v.Form), true
	case *ast.DefDecl:
		args := []ast.Node{ast.NewSymbol(n.Pos(), v.Name)}
		if v.TypeAnnot != nil && parser.IsTypeSymbol(v.TypeAnnot.Text) {
			args = append(args, ast.NewSymbol(n.Pos(), v.TypeAnnot.Text))
		}
		args = append(args, v.Value)
		return callNode(n.Pos(), "def", args...), true
	default:
		return nil, false
	}
}

// specialFormNode is the inverse of unparse: given a list head symbol and the
// remaining items (as values), it builds the parser-specialized AST node so
// macro output is emitted like hand-written code. handled=false means the head
// is not one of the recognized special forms (the caller emits a generic call).
// Covers the same set as unparse: if/when/do/cond/let/loop/fn/quote.
func specialFormNode(head string, items []Value, pos ast.Position) (ast.Node, bool, error) {
	switch head {
	case "if":
		if len(items) < 2 || len(items) > 3 {
			return nil, true, fmt.Errorf("if expects 2 or 3 forms, got %d (at %s)", len(items), pos)
		}
		nodes, err := valuesToNodes(items, pos)
		if err != nil {
			return nil, true, err
		}
		var els ast.Node
		if len(nodes) == 3 {
			els = nodes[2]
		}
		return ast.NewIfExpr(pos, nodes[0], nodes[1], els), true, nil
	case "when":
		if len(items) < 1 {
			return nil, true, fmt.Errorf("when expects a condition (at %s)", pos)
		}
		nodes, err := valuesToNodes(items, pos)
		if err != nil {
			return nil, true, err
		}
		return ast.NewWhenExpr(pos, nodes[0], nodes[1:]), true, nil
	case "do":
		nodes, err := valuesToNodes(items, pos)
		if err != nil {
			return nil, true, err
		}
		return ast.NewDoExpr(pos, nodes), true, nil
	case "quote":
		if len(items) != 1 {
			return nil, true, fmt.Errorf("quote expects 1 form, got %d (at %s)", len(items), pos)
		}
		form, err := valueToNode(items[0], pos)
		if err != nil {
			return nil, true, err
		}
		return ast.NewQuoteExpr(pos, form), true, nil
	case "cond":
		return condNode(items, pos)
	case "let":
		return bindingFormNode("let", items, pos)
	case "loop":
		return bindingFormNode("loop", items, pos)
	case "fn":
		return fnNode(items, pos)
	case "def":
		return defNode(items, pos)
	default:
		return nil, false, nil
	}
}

// defNode rebuilds a top-level (def name value) or (def name type value) from a
// macro expansion, so a macro like defroutes can emit a real declaration. A
// single-symbol type annotation (parser.IsTypeSymbol) between name and value is
// recognized; compound types aren't representable here.
func defNode(items []Value, pos ast.Position) (ast.Node, bool, error) {
	if len(items) < 2 {
		return nil, true, fmt.Errorf("def expects a name and a value (at %s)", pos)
	}
	name, ok := items[0].(*Sym)
	if !ok {
		return nil, true, fmt.Errorf("def name must be a symbol, got %s (at %s)", typeName(items[0]), pos)
	}
	rest := items[1:]
	var annot *ast.TypeExpr
	if len(rest) == 2 {
		if ts, ok := rest[0].(*Sym); ok && parser.IsTypeSymbol(ts.Name) {
			annot = ast.NewTypeExpr(pos, ts.Name)
			rest = rest[1:]
		}
	}
	if len(rest) != 1 {
		return nil, true, fmt.Errorf("def expects a single value (at %s)", pos)
	}
	val, err := valueToNode(rest[0], pos)
	if err != nil {
		return nil, true, err
	}
	return ast.NewDefDecl(pos, name.Name, annot, val), true, nil
}

func condNode(items []Value, pos ast.Position) (ast.Node, bool, error) {
	var clauses []ast.CondClause
	var def ast.Node
	i := 0
	for i < len(items) {
		if i+1 >= len(items) {
			return nil, true, fmt.Errorf("cond expects test/body pairs (at %s)", pos)
		}
		testVal, bodyVal := items[i], items[i+1]
		if kw, ok := testVal.(Keyword); ok && kw == "else" {
			body, err := valueToNode(bodyVal, pos)
			if err != nil {
				return nil, true, err
			}
			def = body
			i += 2
			continue
		}
		test, err := valueToNode(testVal, pos)
		if err != nil {
			return nil, true, err
		}
		body, err := valueToNode(bodyVal, pos)
		if err != nil {
			return nil, true, err
		}
		clauses = append(clauses, ast.CondClause{Test: test, Body: body})
		i += 2
	}
	return ast.NewCondExpr(pos, clauses, def), true, nil
}

func bindingFormNode(head string, items []Value, pos ast.Position) (ast.Node, bool, error) {
	if len(items) < 1 {
		return nil, true, fmt.Errorf("%s expects a binding vector (at %s)", head, pos)
	}
	vec, ok := items[0].(*Vector)
	if !ok {
		return nil, true, fmt.Errorf("%s expects a binding vector, got %s (at %s)", head, typeName(items[0]), pos)
	}
	if len(vec.Items)%2 != 0 {
		return nil, true, fmt.Errorf("%s binding vector needs an even number of forms (at %s)", head, pos)
	}
	var bindings []ast.LetBinding
	for i := 0; i < len(vec.Items); i += 2 {
		pat, err := valueToNode(vec.Items[i], pos)
		if err != nil {
			return nil, true, err
		}
		val, err := valueToNode(vec.Items[i+1], pos)
		if err != nil {
			return nil, true, err
		}
		bindings = append(bindings, ast.LetBinding{Pattern: pat, Value: val})
	}
	body, err := valuesToNodes(items[1:], pos)
	if err != nil {
		return nil, true, err
	}
	if head == "loop" {
		return ast.NewLoopExpr(pos, bindings, body), true, nil
	}
	return ast.NewLetExpr(pos, bindings, body), true, nil
}

func fnNode(items []Value, pos ast.Position) (ast.Node, bool, error) {
	if len(items) < 1 {
		return nil, true, fmt.Errorf("fn expects a parameter vector (at %s)", pos)
	}
	vec, ok := items[0].(*Vector)
	if !ok {
		return nil, true, fmt.Errorf("fn expects a parameter vector, got %s (at %s)", typeName(items[0]), pos)
	}
	var params []ast.Param
	rest := false
	for i := 0; i < len(vec.Items); i++ {
		pv := vec.Items[i]
		if s, ok := pv.(*Sym); ok && s.Name == "&" {
			rest = true
			continue
		}
		if s, ok := pv.(*Sym); ok {
			p := ast.Param{Name: s.Name, IsRest: rest}
			rest = false
			// An immediately following type symbol annotates this param, mirroring
			// the parser's [name type] rule (parser.IsTypeSymbol disambiguation).
			if i+1 < len(vec.Items) {
				if ts, ok := vec.Items[i+1].(*Sym); ok && parser.IsTypeSymbol(ts.Name) {
					p.TypeAnnot = ast.NewTypeExpr(pos, ts.Name)
					i++
				}
			}
			params = append(params, p)
			continue
		}
		// destructure pattern (vector/map)
		pat, err := valueToNode(pv, pos)
		if err != nil {
			return nil, true, err
		}
		params = append(params, ast.Param{Pattern: pat, IsRest: rest})
		rest = false
	}
	bodyItems := items[1:]
	// Optional `-> ret` return type before the body (single-symbol types only).
	var ret *ast.TypeExpr
	if len(bodyItems) >= 2 {
		if arrow, ok := bodyItems[0].(*Sym); ok && arrow.Name == "->" {
			if ts, ok := bodyItems[1].(*Sym); ok && parser.IsTypeSymbol(ts.Name) {
				ret = ast.NewTypeExpr(pos, ts.Name)
				bodyItems = bodyItems[2:]
			}
		}
	}
	body, err := valuesToNodes(bodyItems, pos)
	if err != nil {
		return nil, true, err
	}
	return ast.NewFnExpr(pos, params, ret, body), true, nil
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

// paramVector reconstructs an fn parameter vector. A single-symbol type
// annotation (primitive, qualified like web/Request, pointer, or PascalCase) is
// preserved so macro-generated fns stay typed — fnNode reads it back with the
// same parser.IsTypeSymbol rule. Compound types ([]string, map[...]) aren't
// representable as one symbol and are dropped (the prior behavior). A & rest
// param is rendered as the `&` symbol followed by the name.
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
		if p.TypeAnnot != nil && parser.IsTypeSymbol(p.TypeAnnot.Text) {
			elems = append(elems, ast.NewSymbol(pos, p.TypeAnnot.Text))
		}
	}
	return ast.NewVectorLit(pos, elems)
}
