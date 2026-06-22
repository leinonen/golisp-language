package macro

import (
	"fmt"

	"golisp/internal/ast"
)

// maxExpandDepth caps macro re-expansion so a non-terminating macro yields an
// error rather than hanging the build.
const maxExpandDepth = 200

// Expand runs the macroexpansion pass over a parsed file: it registers every
// top-level (defmacro …) — both this file's and any from sibling files of the
// same package (external) — expands all macro call-sites (to a fixed point, and
// recursively through the tree), and returns the node list with the macro
// definitions removed. Files with no in-scope macros and no macro call-sites
// pass through unchanged.
//
// This is the single pass inserted between parse and emit (ADR-017). external
// carries the package-wide macro definitions collected by the dir-build pre-pass
// (the DeclSet); pass nil for a standalone single-file transpile.
func Expand(nodes []ast.Node, external []*ast.MacroDecl) ([]ast.Node, error) {
	// Fast path: nothing to do unless a macro is in scope (defined here or in a
	// sibling file).
	hasLocalMacro := false
	for _, n := range nodes {
		if _, ok := n.(*ast.MacroDecl); ok {
			hasLocalMacro = true
			break
		}
	}
	if !hasLocalMacro && len(external) == 0 {
		return nodes, nil
	}

	ex := &expander{macros: map[string]*Closure{}, env: NewGlobalEnv()}
	// Register external (sibling-file) macros first; a same-named macro in this
	// file then takes precedence.
	for _, md := range external {
		clo, err := makeClosureFromParts(md.Params, md.Body, ex.env, md.Name, md.Pos())
		if err != nil {
			return nil, err
		}
		ex.macros[md.Name] = clo
	}
	out := make([]ast.Node, 0, len(nodes))
	for _, n := range nodes {
		if md, ok := n.(*ast.MacroDecl); ok {
			clo, err := makeClosureFromParts(md.Params, md.Body, ex.env, md.Name, md.Pos())
			if err != nil {
				return nil, err
			}
			ex.macros[md.Name] = clo
			continue
		}
		en, err := ex.expand(n, 0)
		if err != nil {
			return nil, err
		}
		out = append(out, en)
	}
	return out, nil
}

type expander struct {
	macros map[string]*Closure
	env    *Env
}

// expand returns n with all macro call-sites within it expanded. If n is itself
// a macro call, it is expanded and the result re-expanded (fixed point); depth
// guards that recursion. Otherwise n's children are expanded in place.
func (ex *expander) expand(n ast.Node, depth int) (ast.Node, error) {
	if call, ok := n.(*ast.CallExpr); ok {
		if sym, ok := call.Head.(*ast.Symbol); ok {
			if macro, ok := ex.macros[sym.Name]; ok {
				if depth > maxExpandDepth {
					return nil, fmt.Errorf("macro expansion of %q did not terminate (at %s)", sym.Name, n.Pos())
				}
				argData := make([]Value, 0, len(call.Args))
				for _, a := range call.Args {
					v, err := nodeToValue(a)
					if err != nil {
						return nil, fmt.Errorf("macro %s: %w", sym.Name, err)
					}
					argData = append(argData, v)
				}
				result, err := applyClosure(macro, argData)
				if err != nil {
					return nil, fmt.Errorf("expanding macro %s (at %s): %w", sym.Name, n.Pos(), err)
				}
				node, err := valueToNode(result, n.Pos())
				if err != nil {
					return nil, fmt.Errorf("macro %s produced an invalid form (at %s): %w", sym.Name, n.Pos(), err)
				}
				return ex.expand(node, depth+1)
			}
		}
	}
	return ex.expandChildren(n)
}

// expandChildren expands the expression children of a container node in place
// and returns it. Node kinds without expression children are returned as-is.
func (ex *expander) expandChildren(n ast.Node) (ast.Node, error) {
	var err error
	ev := func(x ast.Node) ast.Node {
		if err != nil || x == nil {
			return x
		}
		var r ast.Node
		r, err = ex.expand(x, 0)
		return r
	}
	evList := func(xs []ast.Node) {
		for i := range xs {
			xs[i] = ev(xs[i])
		}
	}

	switch v := n.(type) {
	// top-level declarations
	case *ast.DefnDecl:
		evList(v.Body)
	case *ast.MethodDecl:
		evList(v.Body)
	case *ast.DefDecl:
		v.Value = ev(v.Value)
	case *ast.DefTestDecl:
		evList(v.Body)

	// expression forms
	case *ast.CallExpr:
		v.Head = ev(v.Head)
		evList(v.Args)
	case *ast.FnExpr:
		evList(v.Body)
	case *ast.DoExpr:
		evList(v.Body)
	case *ast.IfExpr:
		v.Cond = ev(v.Cond)
		v.Then = ev(v.Then)
		v.Else = ev(v.Else)
	case *ast.WhenExpr:
		v.Cond = ev(v.Cond)
		evList(v.Body)
	case *ast.CondExpr:
		for i := range v.Clauses {
			v.Clauses[i].Test = ev(v.Clauses[i].Test)
			v.Clauses[i].Body = ev(v.Clauses[i].Body)
		}
		v.Default = ev(v.Default)
	case *ast.LetExpr:
		for i := range v.Bindings {
			v.Bindings[i].Value = ev(v.Bindings[i].Value)
		}
		evList(v.Body)
	case *ast.LoopExpr:
		for i := range v.Bindings {
			v.Bindings[i].Value = ev(v.Bindings[i].Value)
		}
		evList(v.Body)
	case *ast.SwitchExpr:
		v.Expr = ev(v.Expr)
		for i := range v.Cases {
			v.Cases[i].Value = ev(v.Cases[i].Value)
			v.Cases[i].Body = ev(v.Cases[i].Body)
		}
		v.Default = ev(v.Default)
	case *ast.ReturnExpr:
		evList(v.Args)
	case *ast.ValuesExpr:
		evList(v.Args)
	case *ast.VectorLit:
		evList(v.Elements)
	case *ast.MapLit:
		for i := range v.Pairs {
			v.Pairs[i].Key = ev(v.Pairs[i].Key)
			v.Pairs[i].Value = ev(v.Pairs[i].Value)
		}
	case *ast.QuoteExpr:
		// quoted data is not expanded
	}
	// Note: containers not listed here (e.g. some concurrency forms) do not yet
	// have their children walked for macro calls; tracked for a follow-up.
	return n, err
}
