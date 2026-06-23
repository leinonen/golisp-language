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
	// atomElems maps a glisp field name whose type is an atom (`Atom` / `(Atom T)`)
	// to that atom's element Go type, so (deref (:field r)) coerces.
	atomElems map[string]string
}

// buildStructInfo builds a structInfo from a defstruct declaration.
func buildStructInfo(d *ast.StructDecl) *structInfo {
	si := &structInfo{
		fields:    make(map[string]string, len(d.Fields)),
		atomElems: map[string]string{},
	}
	for _, f := range d.Fields {
		si.fields[f.Name] = titleCase(identToGo(f.Name))
		if f.TypeAnnot != nil {
			if elem, ok := atomElemTypeFromText(f.TypeAnnot.Text); ok {
				si.atomElems[f.Name] = elem
			}
		}
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

// typeScope captures the local type/binding environment saved by pushTypeScope.
type typeScope struct {
	types map[string]string
	vars  map[string]bool
	anys  map[string]bool
	atoms map[string]string
	nums  map[string]string
}

// pushTypeScope shallow-copies the current local type and binding environments
// so that registrations inside a function/let body do not leak to sibling
// scopes. The returned value is passed back to popTypeScope on exit.
func (e *Emitter) pushTypeScope() typeScope {
	saved := typeScope{types: e.localTypes, vars: e.localVars, anys: e.localAny, atoms: e.atomTypes, nums: e.localNumeric}
	nt := make(map[string]string, len(saved.types))
	for k, v := range saved.types {
		nt[k] = v
	}
	nv := make(map[string]bool, len(saved.vars))
	for k, v := range saved.vars {
		nv[k] = v
	}
	na := make(map[string]bool, len(saved.anys))
	for k, v := range saved.anys {
		na[k] = v
	}
	nm := make(map[string]string, len(saved.atoms))
	for k, v := range saved.atoms {
		nm[k] = v
	}
	nn := make(map[string]string, len(saved.nums))
	for k, v := range saved.nums {
		nn[k] = v
	}
	e.localTypes = nt
	e.localVars = nv
	e.localAny = na
	e.atomTypes = nm
	e.localNumeric = nn
	return saved
}

// popTypeScope restores the environments captured by pushTypeScope.
func (e *Emitter) popTypeScope(saved typeScope) {
	e.localTypes = saved.types
	e.localVars = saved.vars
	e.localAny = saved.anys
	e.atomTypes = saved.atoms
	e.localNumeric = saved.nums
}

// registerAnyVar records glispName as an in-scope binding statically known to
// hold Go `any`, so arithmetic/comparison on it routes through the numeric
// coercion helpers. Also registers it as a plain local var.
func (e *Emitter) registerAnyVar(glispName string) {
	e.registerLocalVar(glispName)
	if e.localAny == nil || glispName == "" || glispName == "_" {
		return
	}
	e.localAny[glispName] = true
}

// clearAnyVar marks glispName as NOT `any` in the current scope (a rebinding to
// a concrete-typed value shadows an outer any-binding of the same name).
func (e *Emitter) clearAnyVar(glispName string) {
	if e.localAny != nil {
		delete(e.localAny, glispName)
	}
}

// registerAtomType records that the in-scope binding glispName holds an atom
// whose element Go type is elem, so a typed (deref glispName) coerces. The name
// is also recorded as a plain local var.
func (e *Emitter) registerAtomType(glispName, elem string) {
	e.registerLocalVar(glispName)
	if e.atomTypes == nil || glispName == "" || glispName == "_" {
		return
	}
	e.atomTypes[glispName] = elem
}

// clearAtomType drops any atom-element record for glispName (a rebinding to a
// non-atom value shadows an outer atom binding of the same name).
func (e *Emitter) clearAtomType(glispName string) {
	if e.atomTypes != nil {
		delete(e.atomTypes, glispName)
	}
}

// inferAtomElemType returns the element Go type of an atom produced by value,
// or ("", false) if value is not a statically-recognised atom constructor.
// Only the direct (atom …) / (atom T …) form is recognised.
func (e *Emitter) inferAtomElemType(value ast.Node) (string, bool) {
	a, ok := value.(*ast.AtomExpr)
	if !ok {
		return "", false
	}
	if a.ElemType == nil {
		return "any", true
	}
	return typeExprToGo(a.ElemType.Text), true
}

// atomElemOfBinding determines a binding's atom element type: an explicit
// `(Atom T)` / `Atom` annotation wins, else the binding value is inspected.
func (e *Emitter) atomElemOfBinding(annot *ast.TypeExpr, value ast.Node) (string, bool) {
	if annot != nil {
		if elem, ok := atomElemTypeFromText(annot.Text); ok {
			return elem, true
		}
	}
	return e.inferAtomElemType(value)
}

// atomTypeOfExpr returns the atom element Go type of an atom-valued expression:
// an in-scope (or global def) atom variable, or a struct field declared with an
// atom type ((:field r) where r holds a known struct). Returns ("", false) when
// the expression's atom element type is not statically known.
func (e *Emitter) atomTypeOfExpr(n ast.Node) (string, bool) {
	switch v := n.(type) {
	case *ast.Symbol:
		if e.atomTypes != nil {
			if elem, ok := e.atomTypes[v.Name]; ok {
				return elem, true
			}
		}
		if e.globalAtomTypes != nil && !e.localVars[v.Name] {
			if elem, ok := e.globalAtomTypes[v.Name]; ok {
				return elem, true
			}
		}
	case *ast.CallExpr:
		// (:field r) — atom stored in a struct field.
		if kw, ok := v.Head.(*ast.KeywordLit); ok && len(v.Args) == 1 {
			if sym, ok := v.Args[0].(*ast.Symbol); ok && e.localTypes != nil {
				if typeName, found := e.localTypes[sym.Name]; found {
					if si := e.structs[typeName]; si != nil {
						if elem, ok := si.atomElems[kw.Value]; ok {
							return elem, true
						}
					}
				}
			}
		}
	}
	return "", false
}

// registerLocalVar records glispName as an in-scope value binding so it
// shadows dot-free method dispatch.
func (e *Emitter) registerLocalVar(glispName string) {
	if e.localVars == nil || glispName == "" || glispName == "_" {
		return
	}
	e.localVars[glispName] = true
}

// registerVarType records that the glisp variable glispName has the declared
// struct or interface type described by goType. Other types are ignored (the
// variable stays untyped from keyword access and method dispatch's view).
// A concrete numeric goType (int/float family) is recorded in localNumeric so
// arithmetic mixing it with the other kind auto-promotes. The name is always
// recorded as an in-scope binding.
func (e *Emitter) registerVarType(glispName, goType string) {
	e.registerLocalVar(glispName)
	if glispName == "" || glispName == "_" {
		return
	}
	if e.localTypes != nil {
		if name, ok := e.namedTypeHint(goType); ok {
			e.localTypes[glispName] = name
		} else if name, ok := e.externalTypeHint(goType); ok {
			// An external Go type (e.g. a `[c *pgx/Conn]` param) records its
			// qualified key so the value participates in dot-free method dispatch
			// against the loaded method set, like a locally-declared type.
			e.localTypes[glispName] = name
		}
	}
	if k := numericGoKind(goType); k != "" {
		e.registerNumericVar(glispName, k)
	} else {
		e.clearNumericVar(glispName)
	}
	// An explicitly `any`-typed binding (param `[n any]`, `(let [x any …])`)
	// behaves like an untyped one for the any-seam: arithmetic/comparison on it
	// must coerce (`any + int` is a Go error either way), so mark it. Callers
	// that bind a concrete type clear localAny separately.
	if goType == "any" {
		e.registerAnyVar(glispName)
	}
}

// numericGoKind classifies a Go type name as "int" (any integer family type),
// "float" (float32/float64), or "" (not a concrete numeric scalar). Drives
// int→float64 auto-promotion in mixed arithmetic/comparison.
func numericGoKind(goType string) string {
	switch goType {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "byte", "rune", "uintptr":
		return "int"
	case "float32", "float64":
		return "float"
	}
	return ""
}

// registerNumericVar records that the in-scope binding glispName holds a
// concrete numeric value of the given kind ("int" or "float").
func (e *Emitter) registerNumericVar(glispName, kind string) {
	if e.localNumeric == nil || glispName == "" || glispName == "_" || kind == "" {
		return
	}
	e.localNumeric[glispName] = kind
}

// clearNumericVar drops any numeric kind recorded for glispName (a rebinding to
// a non-numeric or unknown value shadows an outer numeric binding of the name).
func (e *Emitter) clearNumericVar(glispName string) {
	if e.localNumeric != nil {
		delete(e.localNumeric, glispName)
	}
}

// registerParamTypes records struct/interface types for any typed
// (non-destructured) params in the current scope, and every param name as an
// in-scope binding. Used at function/method entry.
func (e *Emitter) registerParamTypes(params []ast.Param) {
	for _, p := range params {
		if p.Pattern != nil {
			continue
		}
		if p.IsRest {
			e.registerLocalVar(p.Name)
			continue
		}
		if p.TypeAnnot == nil {
			// Untyped scalar param emits as `any` — mark it so arithmetic on it
			// coerces numerically instead of producing invalid `any + int` Go.
			e.registerAnyVar(p.Name)
			continue
		}
		if elem, ok := atomElemTypeFromText(p.TypeAnnot.Text); ok {
			e.registerAtomType(p.Name, elem)
			continue
		}
		e.registerVarType(p.Name, typeExprToGo(p.TypeAnnot.Text))
	}
}

// inferValueStructType returns the declared struct/interface type name a
// binding value is known to produce, or "" if unknown. It recognises struct
// literals, calls to user-defined functions with a declared return type, and
// dot-free method calls.
func (e *Emitter) inferValueStructType(value ast.Node) string {
	switch v := value.(type) {
	case *ast.StructLitExpr:
		if name, _, ok := e.structHint(v.TypeName); ok {
			return name
		}
	case *ast.CallExpr:
		if sym, ok := v.Head.(*ast.Symbol); ok {
			if e.symbols != nil {
				if sig, found := e.symbols[e.coreResolvedName(sym.Name)]; found {
					if name, ok := e.namedTypeHint(sig.retType); ok {
						return name
					}
					if name, ok := e.externalTypeHint(sig.retType); ok {
						return name
					}
					return ""
				}
			}
			// An imported Go function returning a named external type lets its
			// result dispatch methods dot-free (e.g. (let [c (pkg/new …)] (m c))).
			if fn, ok := e.lookupGoCall(sym.Name); ok {
				if name, ok := e.externalTypeHint(fn.ret); ok {
					return name
				}
				return ""
			}
		}
		if info, ok := e.resolveMethodCall(v); ok {
			if name, ok := e.namedTypeHint(info.sig.retType); ok {
				return name
			}
			// A method returning a named external type chains: (next (rows q)).
			if name, ok := e.externalTypeHint(info.sig.retType); ok {
				return name
			}
		}
	case *ast.FieldAccessExpr:
		// A field whose type is itself a named external type chains:
		// (.-Conn pool) → method dispatch on the resulting *pkg.Conn.
		if sym, ok := v.Object.(*ast.Symbol); ok {
			if typeName := e.localTypes[sym.Name]; typeName != "" {
				if fs := e.goFieldSet(typeName); fs != nil {
					if ft, ok := fs[v.Field]; ok {
						if name, ok := e.externalTypeHint(ft); ok {
							return name
						}
					}
				}
			}
		}
	}
	return ""
}
