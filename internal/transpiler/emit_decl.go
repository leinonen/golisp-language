package transpiler

import (
	"fmt"
	"strings"

	"golisp/internal/ast"
)

// emitDefDecl emits: var Name Type = Value
func (e *Emitter) emitDefDecl(n *ast.DefDecl) error {
	e.lineDir(n.Pos_)
	if e.strict && n.TypeAnnot == nil {
		return fmt.Errorf("strict: def %q has no type annotation", n.Name)
	}
	if err := e.checkMultiReturnValue(n.Value); err != nil {
		return err
	}
	goName := identToGo(n.Name)
	if n.TypeAnnot != nil {
		typeStr := typeExprToGo(n.TypeAnnot.Text)
		e.writeIndent()
		e.writef("var %s %s = ", goName, typeStr)
		if err := e.emitExprWithHint(n.Value, typeStr); err != nil {
			return err
		}
		e.nl()
	} else {
		e.writeIndent()
		e.writef("var %s = ", goName)
		if err := e.emitExpr(n.Value); err != nil {
			return err
		}
		e.nl()
	}
	return nil
}

// emitDefnDecl emits a function declaration.
func (e *Emitter) emitDefnDecl(n *ast.DefnDecl) error {
	e.lineDir(n.Pos_)
	if e.strict {
		for _, p := range n.Params {
			// Skip rest params and destructured params — type annotation optional there.
			if p.IsRest || p.Pattern != nil {
				continue
			}
			if p.TypeAnnot == nil {
				return fmt.Errorf("strict: param %q in defn %q has no type annotation", p.Name, n.Name)
			}
		}
		if n.ReturnType == nil {
			return fmt.Errorf("strict: defn %q has no return type annotation (use -> void for functions with no return value)", n.Name)
		}
	}
	goName := identToGo(n.Name)
	sigParts, destructs, err := e.buildParamSig(n.Params)
	if err != nil {
		return err
	}
	retStr := e.formatReturnType(n.ReturnType)
	isVoid := retStr == "void"
	if isVoid {
		retStr = ""
	}

	e.writeIndent()
	if retStr != "" {
		e.writef("func %s(%s) %s {", goName, strings.Join(sigParts, ", "), retStr)
	} else {
		e.writef("func %s(%s) {", goName, strings.Join(sigParts, ", "))
	}
	e.nl()
	e.push()
	saved := e.pushTypeScope()
	savedRet := e.currentRetType
	e.currentRetType = retStr
	e.registerParamTypes(n.Params)
	if err := e.emitParamDestructs(destructs); err != nil {
		return err
	}
	if err := e.emitBody(n.Body, retStr != ""); err != nil {
		return err
	}
	e.currentRetType = savedRet
	e.popTypeScope(saved)
	e.pop()
	e.line("}")
	return nil
}

// emitDefTypeDecl emits: type Name BaseType
func (e *Emitter) emitDefTypeDecl(n *ast.DefTypeDecl) error {
	e.lineDir(n.Pos_)
	e.linef("type %s %s", n.Name, typeExprToGo(n.BaseType.Text))
	return nil
}

// emitStructDecl emits a struct type declaration.
func (e *Emitter) emitStructDecl(n *ast.StructDecl) error {
	e.lineDir(n.Pos_)
	if e.strict {
		for _, f := range n.Fields {
			if f.TypeAnnot == nil {
				return fmt.Errorf("strict: field %q in defstruct %q has no type annotation", f.Name, n.Name)
			}
		}
	}
	e.writeIndent()
	e.writef("type %s struct {", n.Name)
	e.nl()
	e.push()
	for _, f := range n.Fields {
		goName := titleCase(identToGo(f.Name))
		typeStr := "any"
		if f.TypeAnnot != nil {
			typeStr = typeExprToGo(f.TypeAnnot.Text)
		}
		if f.Tag != "" {
			e.linef("%s %s `%s`", goName, typeStr, f.Tag)
		} else {
			e.linef("%s %s", goName, typeStr)
		}
	}
	e.pop()
	e.line("}")
	return nil
}

// emitInterfaceDecl emits an interface type declaration.
func (e *Emitter) emitInterfaceDecl(n *ast.InterfaceDecl) error {
	e.lineDir(n.Pos_)
	e.writeIndent()
	e.writef("type %s interface {", n.Name)
	e.nl()
	e.push()
	for _, m := range n.Methods {
		params, err := e.formatParams(m.Params)
		if err != nil {
			return err
		}
		retStr := e.formatReturnType(m.ReturnType)
		if retStr == "void" {
			retStr = ""
		}
		if retStr != "" {
			e.linef("%s(%s) %s", m.Name, params, retStr)
		} else {
			e.linef("%s(%s)", m.Name, params)
		}
	}
	e.pop()
	e.line("}")
	return nil
}

// emitMethodDecl emits a method with a receiver.
func (e *Emitter) emitMethodDecl(n *ast.MethodDecl) error {
	e.lineDir(n.Pos_)
	receiverType := typeExprToGo(n.ReceiverType.Text)
	goName := identToGo(n.Name)
	sigParts, destructs, err := e.buildParamSig(n.Params)
	if err != nil {
		return err
	}
	retStr := e.formatReturnType(n.ReturnType)
	isVoid := retStr == "void"
	if isVoid {
		retStr = ""
	}
	e.writeIndent()
	if retStr != "" {
		e.writef("func (%s %s) %s(%s) %s {", n.ReceiverName, receiverType, goName, strings.Join(sigParts, ", "), retStr)
	} else {
		e.writef("func (%s %s) %s(%s) {", n.ReceiverName, receiverType, goName, strings.Join(sigParts, ", "))
	}
	e.nl()
	e.push()
	saved := e.pushTypeScope()
	savedRet := e.currentRetType
	e.currentRetType = retStr
	e.registerVarType(n.ReceiverName, receiverType)
	e.registerParamTypes(n.Params)
	if err := e.emitParamDestructs(destructs); err != nil {
		return err
	}
	if err := e.emitBody(n.Body, retStr != ""); err != nil {
		return err
	}
	e.currentRetType = savedRet
	e.popTypeScope(saved)
	e.pop()
	e.line("}")
	return nil
}

// emitDefTestDecl emits a deftest as a Go test function.
func (e *Emitter) emitDefTestDecl(n *ast.DefTestDecl) error {
	e.lineDir(n.Pos_)
	e.needImport("testing")
	goName := "Test" + titleCase(identToGo(n.Name))
	e.linef("func %s(t *testing.T) {", goName)
	e.push()
	for _, node := range n.Body {
		if err := e.emitAssertStmt(node); err != nil {
			return err
		}
	}
	e.pop()
	e.line("}")
	return nil
}

// emitAssertStmt emits one statement inside a deftest body.
// Recognizes assert=, assert-true, assert-false, assert-nil, assert-err.
// Anything else is emitted as a regular statement.
func (e *Emitter) emitAssertStmt(n ast.Node) error {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return e.emitStmtNode(n)
	}
	sym, ok := call.Head.(*ast.Symbol)
	if !ok {
		return e.emitStmtNode(n)
	}
	// Map each assertion to its .glsp line so a failing t.Errorf reports the
	// assertion's own position, not a drifted line.
	e.lineDir(n.Pos())

	switch sym.Name {
	case "assert=":
		if len(call.Args) != 2 {
			return fmt.Errorf("assert= requires 2 arguments")
		}
		// Value equality via _glispEquals: native Go == is wrong across dynamic
		// numeric types and illegal on slices/maps (assert= on collections is a
		// common test idiom), so always route through the helper.
		e.needImport("_num")
		e.writeIndent()
		e.write("if !_glispEquals(")
		if err := e.emitExpr(call.Args[0]); err != nil {
			return err
		}
		e.write(", ")
		if err := e.emitExpr(call.Args[1]); err != nil {
			return err
		}
		e.write(") {")
		e.nl()
		e.push()
		e.lineDir(n.Pos())
		e.writeIndent()
		e.write(`t.Errorf("assert= failed: expected %v, got %v", `)
		if err := e.emitExpr(call.Args[1]); err != nil {
			return err
		}
		e.write(", ")
		if err := e.emitExpr(call.Args[0]); err != nil {
			return err
		}
		e.write(")")
		e.nl()
		e.pop()
		e.line("}")
		return nil

	case "assert-true":
		if len(call.Args) != 1 {
			return fmt.Errorf("assert-true requires 1 argument")
		}
		e.writeIndent()
		e.write("if !(")
		if err := e.emitCondition(call.Args[0]); err != nil {
			return err
		}
		e.write(") {")
		e.nl()
		e.push()
		e.lineDir(n.Pos())
		e.writeIndent()
		e.write(`t.Errorf("assert-true failed")`)
		e.nl()
		e.pop()
		e.line("}")
		return nil

	case "assert-false":
		if len(call.Args) != 1 {
			return fmt.Errorf("assert-false requires 1 argument")
		}
		e.writeIndent()
		e.write("if (")
		if err := e.emitCondition(call.Args[0]); err != nil {
			return err
		}
		e.write(") {")
		e.nl()
		e.push()
		e.lineDir(n.Pos())
		e.writeIndent()
		e.write(`t.Errorf("assert-false failed")`)
		e.nl()
		e.pop()
		e.line("}")
		return nil

	case "assert-nil":
		if len(call.Args) != 1 {
			return fmt.Errorf("assert-nil requires 1 argument")
		}
		e.writeIndent()
		e.write("if (")
		if err := e.emitExpr(call.Args[0]); err != nil {
			return err
		}
		e.write(") != nil {")
		e.nl()
		e.push()
		e.lineDir(n.Pos())
		e.writeIndent()
		e.write(`t.Errorf("assert-nil failed: got %v", `)
		if err := e.emitExpr(call.Args[0]); err != nil {
			return err
		}
		e.write(")")
		e.nl()
		e.pop()
		e.line("}")
		return nil

	case "assert-err":
		if len(call.Args) != 1 {
			return fmt.Errorf("assert-err requires 1 argument")
		}
		e.writeIndent()
		e.write("if (")
		if err := e.emitExpr(call.Args[0]); err != nil {
			return err
		}
		e.write(") == nil {")
		e.nl()
		e.push()
		e.lineDir(n.Pos())
		e.writeIndent()
		e.write(`t.Errorf("assert-err failed: expected non-nil error")`)
		e.nl()
		e.pop()
		e.line("}")
		return nil
	}

	return e.emitStmtNode(n)
}

// formatParams converts a param list to a Go parameter string.
func (e *Emitter) formatParams(params []ast.Param) (string, error) {
	parts := make([]string, 0, len(params))
	for _, p := range params {
		goName := identToGo(p.Name)
		typeStr := "any"
		if p.TypeAnnot != nil {
			typeStr = typeExprToGo(p.TypeAnnot.Text)
		}
		if p.IsRest {
			parts = append(parts, goName+" ..."+typeStr)
		} else {
			parts = append(parts, goName+" "+typeStr)
		}
	}
	return strings.Join(parts, ", "), nil
}

// formatReturnType converts a return type annotation to a Go type string.
// Returns "" when there is no annotation (will emit no return type = void func).
func (e *Emitter) formatReturnType(annot *ast.TypeExpr) string {
	if annot == nil {
		return ""
	}
	return typeExprToGo(annot.Text)
}
