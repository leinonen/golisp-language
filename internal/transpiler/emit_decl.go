package transpiler

import (
	"strings"

	"golisp/internal/ast"
)

// emitDefDecl emits: var Name Type = Value
func (e *Emitter) emitDefDecl(n *ast.DefDecl) error {
	goName := identToGo(n.Name)
	if n.TypeAnnot != nil {
		e.writeIndent()
		e.writef("var %s %s = ", goName, typeExprToGo(n.TypeAnnot.Text))
		if err := e.emitExpr(n.Value); err != nil {
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
	goName := identToGo(n.Name)
	params, err := e.formatParams(n.Params)
	if err != nil {
		return err
	}
	retStr := e.formatReturnType(n.ReturnType)

	e.writeIndent()
	if retStr != "" {
		e.writef("func %s(%s) %s {", goName, params, retStr)
	} else {
		e.writef("func %s(%s) {", goName, params)
	}
	e.nl()
	e.push()
	if err := e.emitBody(n.Body, retStr != ""); err != nil {
		return err
	}
	e.pop()
	e.line("}")
	return nil
}

// emitStructDecl emits a struct type declaration.
func (e *Emitter) emitStructDecl(n *ast.StructDecl) error {
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
