package transpiler

import (
	"fmt"
	"strings"

	"golisp/internal/ast"
)

// emitLoopExpr emits a loop/recur construct.
// When inReturn is true (loop in tail position), base cases emit `return value` directly.
// When inReturn is false (loop used as expression), we use an IIFE with a result var.
func (e *Emitter) emitLoopExpr(n *ast.LoopExpr, inReturn bool) error {
	if !inReturn {
		// Expression position: wrap in IIFE with result variable
		retVar := e.fresh("loop")
		e.write("func() any {")
		e.nl()
		e.push()
		if err := e.emitLoopBlock(n, retVar, false); err != nil {
			return err
		}
		e.writef("return %s", retVar)
		e.nl()
		e.pop()
		e.writeIndent()
		e.write("}()")
		return nil
	}
	// Return position: base cases use `return value` directly — no result var needed.
	return e.emitLoopBlock(n, "", true)
}

// emitLoopBlock emits the core loop:
//  1. Declare bindings as mutable variables
//  2. Optionally declare result variable _loopN (when retVar != "")
//  3. Emit `for { if-recur/break block }`
// inReturn: when true, base cases emit `return value` directly instead of assigning retVar.
func (e *Emitter) emitLoopBlock(n *ast.LoopExpr, retVar string, inReturn bool) error {
	// Collect binding names for recur to update
	bindingNames := make([]string, len(n.Bindings))
	for i, b := range n.Bindings {
		sym, ok := b.Pattern.(*ast.Symbol)
		if !ok {
			return fmt.Errorf("loop binding pattern must be a symbol, got %T", b.Pattern)
		}
		bindingNames[i] = identToGo(sym.Name)
	}

	// Emit initial bindings.
	// Explicit TypeAnnot: emit `var name T = expr` — caller guarantees recur values match T.
	// Collection inits without annotation use `var name any = ...` so recur can rebind
	// with any-returning helpers (e.g. _glispConj returns any).
	// Scalar inits keep `:=` so Go infers the concrete type for direct arithmetic.
	for i, b := range n.Bindings {
		e.writeIndent()
		if b.TypeAnnot != nil {
			typeStr := typeExprToGo(b.TypeAnnot.Text)
			e.writef("var %s %s = ", bindingNames[i], typeStr)
			if err := e.emitExprWithHint(b.Value, typeStr); err != nil {
				return err
			}
		} else if isCollectionNode(b.Value) {
			e.writef("var %s any = ", bindingNames[i])
			if err := e.emitExpr(b.Value); err != nil {
				return err
			}
		} else {
			e.writef("%s := ", bindingNames[i])
			if err := e.emitExpr(b.Value); err != nil {
				return err
			}
		}
		e.nl()
	}

	// Declare result variable (only for expression-position loops)
	if retVar != "" {
		e.linef("var %s any", retVar)
	}

	// Enter loop with binding context for recur
	saved := e.loopBindings
	e.loopBindings = bindingNames
	e.loopInReturn = inReturn
	defer func() {
		e.loopBindings = saved
		e.loopInReturn = false
	}()

	e.line("for {")
	e.push()
	if err := e.emitLoopBody(n.Body, retVar); err != nil {
		return err
	}
	e.pop()
	e.line("}")
	return nil
}

// emitLoopBody emits the body of a loop. The last expression may be a recur
// (which updates bindings and continues) or a non-recur (which breaks).
// We need to detect recur in tail position to generate proper Go.
func (e *Emitter) emitLoopBody(body []ast.Node, retVar string) error {
	for i, node := range body {
		isLast := i == len(body)-1
		if isLast {
			if err := e.emitLoopTailNode(node, retVar); err != nil {
				return err
			}
		} else {
			e.writeIndent()
			if err := e.emitExpr(node); err != nil {
				return err
			}
			e.nl()
		}
	}
	return nil
}

// emitLoopTailNode handles the last expression in a loop body.
// If it is a recur, emit the binding updates and continue.
// If it is an if with recur in one branch, handle both branches.
// Otherwise, assign to retVar and break.
func (e *Emitter) emitLoopTailNode(n ast.Node, retVar string) error {
	switch v := n.(type) {
	case *ast.RecurExpr:
		return e.emitRecurInLoop(v)
	case *ast.IfExpr:
		return e.emitIfLoopTail(v, retVar)
	case *ast.CondExpr:
		return e.emitCondLoopTail(v, retVar)
	case *ast.DoExpr:
		return e.emitLoopBody(v.Body, retVar)
	case *ast.LetExpr:
		if err := e.emitLetBindings(v.Bindings); err != nil {
			return err
		}
		return e.emitLoopBody(v.Body, retVar)
	default:
		if e.loopInReturn {
			// Return position: emit `return value` directly
			e.writeIndent()
			e.write("return ")
			if err := e.emitExpr(n); err != nil {
				return err
			}
			e.nl()
		} else {
			// Expression position: assign to result var and break
			e.writeIndent()
			e.writef("%s = ", retVar)
			if err := e.emitExpr(n); err != nil {
				return err
			}
			e.nl()
			e.line("break")
		}
		return nil
	}
}

func (e *Emitter) emitIfLoopTail(n *ast.IfExpr, retVar string) error {
	e.writeIndent()
	e.write("if ")
	if err := e.emitCondition(n.Cond); err != nil {
		return err
	}
	e.write(" {")
	e.nl()
	e.push()
	if err := e.emitLoopTailNode(n.Then, retVar); err != nil {
		return err
	}
	e.pop()
	if n.Else != nil {
		e.line("} else {")
		e.push()
		if err := e.emitLoopTailNode(n.Else, retVar); err != nil {
			return err
		}
		e.pop()
	}
	e.line("}")
	return nil
}

func (e *Emitter) emitCondLoopTail(n *ast.CondExpr, retVar string) error {
	for i, clause := range n.Clauses {
		e.writeIndent()
		if i == 0 {
			e.write("if ")
		} else {
			e.write("} else if ")
		}
		if err := e.emitCondition(clause.Test); err != nil {
			return err
		}
		e.write(" {")
		e.nl()
		e.push()
		if err := e.emitLoopTailNode(clause.Body, retVar); err != nil {
			return err
		}
		e.pop()
	}
	if n.Default != nil {
		if len(n.Clauses) > 0 {
			e.line("} else {")
		} else {
			e.line("{")
		}
		e.push()
		if err := e.emitLoopTailNode(n.Default, retVar); err != nil {
			return err
		}
		e.pop()
	}
	if len(n.Clauses) > 0 || n.Default != nil {
		e.line("}")
	}
	return nil
}

// isCollectionNode returns true for literal collection nodes (vector, map, set).
// Used to decide whether a loop binding needs `var name any = ...` rather than `:=`.
func isCollectionNode(n ast.Node) bool {
	switch n.(type) {
	case *ast.VectorLit, *ast.MapLit, *ast.SetLit:
		return true
	}
	return false
}

// emitRecurInLoop emits the recur update-and-continue logic.
// To avoid aliasing issues, we compute new values into temporaries first.
func (e *Emitter) emitRecurInLoop(n *ast.RecurExpr) error {
	if len(n.Args) != len(e.loopBindings) {
		return fmt.Errorf("recur has %d args but loop has %d bindings",
			len(n.Args), len(e.loopBindings))
	}

	// Compute new values into temp vars
	tmpNames := make([]string, len(n.Args))
	for i, arg := range n.Args {
		tmp := fmt.Sprintf("_r%d", i)
		tmpNames[i] = tmp
		e.writeIndent()
		e.writef("%s := ", tmp)
		if err := e.emitExpr(arg); err != nil {
			return err
		}
		e.nl()
	}

	// Assign temp vars to binding vars
	assignments := make([]string, len(tmpNames))
	for i, name := range e.loopBindings {
		assignments[i] = fmt.Sprintf("%s = %s", name, tmpNames[i])
	}
	e.line(strings.Join(assignments, "; "))
	e.line("continue")
	return nil
}

// emitRecurStmt handles recur outside of a loop context (error).
func (e *Emitter) emitRecurStmt(n *ast.RecurExpr) error {
	if e.loopBindings == nil {
		return fmt.Errorf("recur used outside of loop at %s", n.Pos())
	}
	return e.emitRecurInLoop(n)
}
