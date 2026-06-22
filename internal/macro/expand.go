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
	ex, active, err := newExpander(nodes, external)
	if err != nil {
		return nil, err
	}
	if !active {
		return nodes, nil
	}
	out := make([]ast.Node, 0, len(nodes))
	for _, n := range nodes {
		if _, ok := n.(*ast.MacroDecl); ok {
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

// ExpandOnce expands the outermost macro call of each top-level form a single
// step (macroexpand-1 semantics applied per top-level form): no recursion into
// children and no re-expansion of the result. It is a debugging aid (see the
// `glisp macroexpand --once` command); the build path uses Expand. MacroDecls
// are removed from the result.
func ExpandOnce(nodes []ast.Node, external []*ast.MacroDecl) ([]ast.Node, error) {
	ex, active, err := newExpander(nodes, external)
	if err != nil {
		return nil, err
	}
	if !active {
		return nodes, nil
	}
	out := make([]ast.Node, 0, len(nodes))
	for _, n := range nodes {
		if _, ok := n.(*ast.MacroDecl); ok {
			continue
		}
		en, _, err := ex.expandTopOnce(n)
		if err != nil {
			return nil, err
		}
		out = append(out, en)
	}
	return out, nil
}

// newExpander builds the expander with every in-scope macro registered, in
// priority order (later registrations shadow earlier ones): the core prelude
// first, then external sibling-file macros, then this file's own. active is
// always true because the core prelude is always present; the bool is retained
// for the call sites' fast-path shape.
func newExpander(nodes []ast.Node, external []*ast.MacroDecl) (*expander, bool, error) {
	ex := &expander{macros: map[string]*Closure{}, env: NewGlobalEnv()}
	core, err := CoreMacros()
	if err != nil {
		return nil, false, err
	}
	register := func(mds []*ast.MacroDecl) error {
		for _, md := range mds {
			clo, err := makeClosureFromParts(md.Params, md.Body, ex.env, md.Name, md.Pos())
			if err != nil {
				return err
			}
			ex.macros[md.Name] = clo
		}
		return nil
	}
	if err := register(core); err != nil {
		return nil, false, err
	}
	if err := register(external); err != nil {
		return nil, false, err
	}
	var local []*ast.MacroDecl
	for _, n := range nodes {
		if md, ok := n.(*ast.MacroDecl); ok {
			local = append(local, md)
		}
	}
	if err := register(local); err != nil {
		return nil, false, err
	}
	return ex, true, nil
}

type expander struct {
	macros map[string]*Closure
	env    *Env
}

// expand returns n with all macro call-sites within it expanded. If n is itself
// a macro call, it is expanded and the result re-expanded (fixed point); depth
// guards that recursion. Otherwise n's children are expanded in place.
func (ex *expander) expand(n ast.Node, depth int) (ast.Node, error) {
	if depth > maxExpandDepth {
		return nil, fmt.Errorf("macro expansion did not terminate (at %s)", n.Pos())
	}
	node, fired, err := ex.expandTopOnce(n)
	if err != nil {
		return nil, err
	}
	if fired {
		// Re-expand the result (fixed point), then descend into its children.
		return ex.expand(node, depth+1)
	}
	return ex.expandChildren(n)
}

// expandTopOnce expands node's outermost macro call exactly once: if node is a
// macro call it is expanded a single step (no children walked, no re-expansion);
// otherwise it is returned unchanged. The bool reports whether expansion fired.
func (ex *expander) expandTopOnce(n ast.Node) (ast.Node, bool, error) {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return n, false, nil
	}
	sym, ok := call.Head.(*ast.Symbol)
	if !ok {
		return n, false, nil
	}
	macro, ok := ex.macros[sym.Name]
	if !ok {
		return n, false, nil
	}
	argData := make([]Value, 0, len(call.Args))
	for _, a := range call.Args {
		v, err := nodeToValue(a)
		if err != nil {
			return nil, false, fmt.Errorf("macro %s: %w", sym.Name, err)
		}
		argData = append(argData, v)
	}
	result, err := applyClosure(macro, argData)
	if err != nil {
		return nil, false, fmt.Errorf("expanding macro %s (at %s): %w", sym.Name, n.Pos(), err)
	}
	node, err := valueToNode(result, n.Pos())
	if err != nil {
		return nil, false, fmt.Errorf("macro %s produced an invalid form (at %s): %w", sym.Name, n.Pos(), err)
	}
	return node, true, nil
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

	// conditional-binding forms
	case *ast.IfLetExpr:
		v.Expr = ev(v.Expr)
		v.Then = ev(v.Then)
		v.Else = ev(v.Else)
	case *ast.WhenLetExpr:
		v.Expr = ev(v.Expr)
		evList(v.Body)
	case *ast.LetOrExpr:
		for i := range v.Bindings {
			v.Bindings[i].Expr = ev(v.Bindings[i].Expr)
			v.Bindings[i].Fallback = ev(v.Bindings[i].Fallback)
		}
		evList(v.Body)
	case *ast.IfErrExpr:
		v.Expr = ev(v.Expr)
		v.OnErr = ev(v.OnErr)
		v.OnOk = ev(v.OnOk)

	// concurrency / resource forms
	case *ast.GoStmt:
		evList(v.Body)
	case *ast.GoValExpr:
		evList(v.Body)
	case *ast.ParStmt:
		evList(v.Bodies)
	case *ast.DeferStmt:
		v.Expr = ev(v.Expr)
	case *ast.SendStmt:
		v.Chan = ev(v.Chan)
		v.Val = ev(v.Val)
	case *ast.RecvExpr:
		v.Chan = ev(v.Chan)
	case *ast.RecvOkExpr:
		v.Chan = ev(v.Chan)
	case *ast.CloseStmt:
		v.Chan = ev(v.Chan)
	case *ast.SelectStmt:
		for i := range v.Cases {
			v.Cases[i].ChanExpr = ev(v.Cases[i].ChanExpr)
			v.Cases[i].SendVal = ev(v.Cases[i].SendVal)
			v.Cases[i].TimeoutMs = ev(v.Cases[i].TimeoutMs)
			evList(v.Cases[i].Body)
		}
	case *ast.ForChanStmt:
		v.Chan = ev(v.Chan)
		evList(v.Body)
	case *ast.WithLockExpr:
		v.Mutex = ev(v.Mutex)
		evList(v.Body)
	case *ast.WithOpenExpr:
		for i := range v.Bindings {
			v.Bindings[i].Value = ev(v.Bindings[i].Value)
		}
		evList(v.Body)
	case *ast.DotoExpr:
		v.Object = ev(v.Object)
		evList(v.Steps)
	case *ast.PipelineExpr:
		v.Source = ev(v.Source)
		evList(v.Stages)
	case *ast.FanOutStmt:
		v.N = ev(v.N)
		v.Chan = ev(v.Chan)
		evList(v.Body)
	case *ast.FanInExpr:
		evList(v.Chans)
	case *ast.RecurExpr:
		evList(v.Args)

	// interop / misc expression forms
	case *ast.MethodCallExpr:
		v.Object = ev(v.Object)
		evList(v.Args)
	case *ast.FieldAccessExpr:
		v.Object = ev(v.Object)
	case *ast.StructLitExpr:
		for i := range v.Fields {
			v.Fields[i].Key = ev(v.Fields[i].Key)
			v.Fields[i].Value = ev(v.Fields[i].Value)
		}
	case *ast.TypeAssertExpr:
		v.Value = ev(v.Value)
	case *ast.AtomExpr:
		v.Init = ev(v.Init)

	case *ast.QuoteExpr:
		// quoted data is not expanded
	}
	// Leaf nodes (literals, symbols, keywords) and reader nodes have no
	// expression children to walk. Every container that can hold an expression
	// is handled above so a macro call is expanded wherever it appears.
	return n, err
}
