package transpiler

import (
	"strings"

	"golisp/internal/ast"
)

// structInfo records the fields of a declared struct so the emitter can resolve
// keyword access (`(:field x)` → `x.Field`) and typed map literals
// (`{:field v}` in a struct-typed position → `Struct{Field: v}`).
type structInfo struct {
	// fields maps a glisp field name (as written in defstruct) to its Go field name.
	fields map[string]string
}

// buildStructInfo builds a structInfo from a defstruct declaration.
func buildStructInfo(d *ast.StructDecl) *structInfo {
	si := &structInfo{fields: make(map[string]string, len(d.Fields))}
	for _, f := range d.Fields {
		si.fields[f.Name] = titleCase(identToGo(f.Name))
	}
	return si
}

// buildFnSig captures a function's arity, variadic flag, per-parameter Go types
// and return type. paramTypes/retType drive type hints (struct map literals) and
// let-binding type inference.
func buildFnSig(params []ast.Param, ret *ast.TypeExpr) *fnSig {
	minArity, variadic := countParams(params)
	pt := make([]string, 0, len(params))
	for _, p := range params {
		if p.IsRest {
			continue
		}
		if p.TypeAnnot != nil {
			pt = append(pt, typeExprToGo(p.TypeAnnot.Text))
		} else {
			pt = append(pt, "")
		}
	}
	rt := ""
	if ret != nil {
		rt = typeExprToGo(ret.Text)
	}
	return &fnSig{minArity: minArity, variadic: variadic, paramTypes: pt, retType: rt}
}

// structHint reports whether a Go type string names a declared struct (optionally
// behind a single pointer). It returns the bare struct name and whether it was a
// pointer type. Only locally-declared structs are recognised — package-qualified
// or built-in types never match.
func (e *Emitter) structHint(goType string) (name string, ptr bool, ok bool) {
	goType = strings.TrimSpace(goType)
	if strings.HasPrefix(goType, "*") {
		ptr = true
		goType = strings.TrimSpace(goType[1:])
	}
	if e.structs == nil {
		return "", false, false
	}
	if _, found := e.structs[goType]; found {
		return goType, ptr, true
	}
	return "", false, false
}

// pushTypeScope shallow-copies the current local type environment so that
// registrations inside a function/let body do not leak to sibling scopes. The
// returned value is passed back to popTypeScope on exit.
func (e *Emitter) pushTypeScope() map[string]string {
	saved := e.localTypes
	nw := make(map[string]string, len(saved))
	for k, v := range saved {
		nw[k] = v
	}
	e.localTypes = nw
	return saved
}

// popTypeScope restores the local type environment captured by pushTypeScope.
func (e *Emitter) popTypeScope(saved map[string]string) {
	e.localTypes = saved
}

// registerVarType records that the glisp variable glispName has the struct type
// described by goType, if goType in fact names a declared struct. Non-struct
// types are ignored (the variable stays untyped from keyword access's view).
func (e *Emitter) registerVarType(glispName, goType string) {
	if e.localTypes == nil || glispName == "" || glispName == "_" {
		return
	}
	if name, _, ok := e.structHint(goType); ok {
		e.localTypes[glispName] = name
	}
}

// registerParamTypes records struct types for any struct-typed (non-destructured)
// params in the current scope. Used at function/method entry.
func (e *Emitter) registerParamTypes(params []ast.Param) {
	for _, p := range params {
		if p.Pattern != nil || p.IsRest || p.TypeAnnot == nil {
			continue
		}
		e.registerVarType(p.Name, typeExprToGo(p.TypeAnnot.Text))
	}
}

// inferValueStructType returns the struct type name a binding value is known to
// produce, or "" if unknown. It recognises struct literals and calls to
// user-defined functions with a struct return type.
func (e *Emitter) inferValueStructType(value ast.Node) string {
	switch v := value.(type) {
	case *ast.StructLitExpr:
		if name, _, ok := e.structHint(v.TypeName); ok {
			return name
		}
	case *ast.CallExpr:
		if sym, ok := v.Head.(*ast.Symbol); ok && e.symbols != nil {
			if sig, found := e.symbols[sym.Name]; found {
				if name, _, ok := e.structHint(sig.retType); ok {
					return name
				}
			}
		}
	}
	return ""
}
