package transpiler

import (
	"fmt"
	"strings"

	"golisp/internal/ast"
	"golisp/internal/formatter"
)

// emitVectorLit emits []T{...} or []any{...}
func (e *Emitter) emitVectorLit(n *ast.VectorLit) error {
	typeStr := "any"
	if n.TypeAnnot != nil {
		// TypeAnnot on VectorLit is the element type, wrapped in []
		t := typeExprToGo(n.TypeAnnot.Text)
		// If the annotation is already []T, use it directly
		if strings.HasPrefix(t, "[]") {
			typeStr = t[2:] // strip leading []
		} else {
			typeStr = t
		}
	} else if inferred := inferHomogeneousElemType(n.Elements); inferred != "" {
		typeStr = inferred
	}
	return e.emitVectorLitElem(n, typeStr)
}

// inferHomogeneousElemType returns the Go element type when all elements are the same literal kind.
// Currently infers "string" only ([]string is safe since _glispToSlice already handles it).
// Returns "" when elements are mixed, non-literal, or empty.
func inferHomogeneousElemType(elems []ast.Node) string {
	if len(elems) == 0 {
		return ""
	}
	for _, el := range elems {
		if _, ok := el.(*ast.StringLit); !ok {
			return ""
		}
	}
	return "string"
}

// emitVectorLitElem emits []elemType{...} with an explicit element type override.
func (e *Emitter) emitVectorLitElem(n *ast.VectorLit, elemType string) error {
	e.writef("[]%s{", elemType)
	for i, el := range n.Elements {
		if i > 0 {
			e.write(", ")
		}
		if err := e.emitExpr(el); err != nil {
			return err
		}
	}
	e.write("}")
	return nil
}

// emitExprWithHint emits an expression, using typeHint to type collection literals.
// For VectorLit: hint "[]T" → []T{...} instead of []any{...}.
// For MapLit: hint "map[K]V" → map[K]V{...} instead of map[string]any{...}.
// All other nodes fall through to plain emitExpr.
func (e *Emitter) emitExprWithHint(n ast.Node, hint string) error {
	switch v := n.(type) {
	case *ast.VectorLit:
		if strings.HasPrefix(hint, "[]") {
			return e.emitVectorLitElem(v, hint[2:])
		}
	case *ast.MapLit:
		// An explicit map type annotation on the literal always wins.
		if v.TypeAnnot == nil {
			if name, ptr, ok := e.structHint(hint); ok {
				return e.emitStructLitFromMap(v, name, ptr)
			}
		}
		if strings.HasPrefix(hint, "map[") {
			return e.emitMapLitTyped(v, hint)
		}
	case *ast.IfExpr:
		if e.hintPropagatable(v, hint) {
			return e.emitTypedIIFE(hint, func() error { return e.emitIfExprReturn(v) })
		}
	case *ast.WhenExpr:
		if e.hintPropagatable(v, hint) {
			return e.emitTypedIIFE(hint, func() error { return e.emitWhenReturn(v) })
		}
	case *ast.CondExpr:
		if e.hintPropagatable(v, hint) {
			return e.emitTypedIIFE(hint, func() error { return e.emitCondExprReturn(v) })
		}
	case *ast.SwitchExpr:
		if e.hintPropagatable(v, hint) {
			return e.emitTypedIIFE(hint, func() error { return e.emitSwitchExprReturn(v) })
		}
	case *ast.DoExpr:
		if e.hintPropagatable(v, hint) {
			return e.emitTypedIIFE(hint, func() error { return e.emitDoExprReturn(v) })
		}
	case *ast.WithOpenExpr:
		if e.hintPropagatable(v, hint) {
			return e.emitTypedIIFE(hint, func() error { return e.emitWithOpenInner(v) })
		}
	case *ast.CallExpr:
		if done, err := e.tryEmitTypedMap(v, hint); done {
			return err
		}
		if done, err := e.tryEmitTypedSeq(v, hint); done {
			return err
		}
	}
	// Numeric coercion: an `any` value (e.g. any-arithmetic result or map lookup)
	// in a concrete numeric position (typed let binding or `-> int`/`-> float64`
	// return) is smart-converted instead of producing an invalid Go assignment.
	if pre, post, ok := numericCoercion(hint); ok && e.exprIsAny(n) {
		e.write(pre)
		if err := e.emitExpr(n); err != nil {
			return err
		}
		e.write(post)
		return nil
	}
	// String coercion: an `any` value in a `string` position is converted with
	// the forgiving _glispToString helper — matching the (string x) cast and
	// `:- string` destructuring — rather than a brittle `.(string)` assertion
	// (which would panic on a non-string). Only fires for provably-`any` values,
	// so a concrete string arg is emitted unchanged.
	if hint == "string" && e.exprIsAny(n) {
		e.write("_glispToString(")
		if err := e.emitExpr(n); err != nil {
			return err
		}
		e.write(")")
		return nil
	}
	// `any`-seam absorption: a call whose Go static type is `any` (map/slice
	// lookup, `reduce`, `conj`/`into`, a `-> any` fn/method) landing in a concrete
	// non-numeric position is asserted to the hint — mirroring (as T …) — instead
	// of emitting an uncompilable Go assignment. Typed slices/maps (`[]T`/`map[K]V`)
	// are excluded by assertableHint (they need element conversion, handled above).
	if call, ok := n.(*ast.CallExpr); ok && assertableHint(hint) && e.callReturnsGoAny(call) {
		if err := e.emitExpr(n); err != nil {
			return err
		}
		e.writef(".(%s)", hint)
		return nil
	}
	return e.emitExpr(n)
}

// numericCoercion returns the wrapper text (prefix, suffix) that smart-converts
// an `any` value to the given concrete numeric Go type, and whether hint is one.
func numericCoercion(hint string) (string, string, bool) {
	switch hint {
	case "int":
		return "_glispToInt(", ")", true
	case "int64":
		return "int64(_glispToInt(", "))", true
	case "float64":
		return "_glispToFloat64(", ")", true
	case "float32":
		return "float32(_glispToFloat64(", "))", true
	}
	return "", "", false
}

// hintPropagatable reports whether a concrete type hint can be pushed into the
// return type of a block-expression IIFE (if/when/do/cond/switch) instead of
// `any`. Constructs that fall through to an implicit `return nil` (when, an
// `if` with no else — left unchanged here, a cond/switch with no default) need
// a nilable hint; constructs where every path returns a value accept any hint.
func (e *Emitter) hintPropagatable(n ast.Node, hint string) bool {
	if hint == "" || hint == "any" {
		return false
	}
	switch v := n.(type) {
	case *ast.IfExpr:
		// A no-else `if` has no implicit return today; leave its emission
		// (broken in expression position) untouched rather than reshaping it.
		return v.Else != nil
	case *ast.DoExpr:
		return true
	case *ast.WithOpenExpr:
		// The body's last expr is always the value — like do, any hint is safe.
		return true
	case *ast.WhenExpr:
		return e.isNilableGoType(hint)
	case *ast.CondExpr:
		if v.Default != nil {
			return true
		}
		return e.isNilableGoType(hint)
	case *ast.SwitchExpr:
		if v.Default != nil {
			return true
		}
		return e.isNilableGoType(hint)
	}
	return false
}

// isNilableGoType reports whether a Go type accepts an untyped nil — slices,
// maps, pointers, channels, funcs, interfaces (any/error and declared
// interfaces). Drives hintPropagatable for IIFEs with an implicit nil tail.
func (e *Emitter) isNilableGoType(hint string) bool {
	hint = strings.TrimSpace(hint)
	switch hint {
	case "any", "error":
		return true
	}
	if strings.HasPrefix(hint, "[]") || strings.HasPrefix(hint, "map[") ||
		strings.HasPrefix(hint, "*") || strings.HasPrefix(hint, "chan ") ||
		strings.HasPrefix(hint, "func") {
		return true
	}
	if _, ok := e.ifaces[hint]; ok {
		return true
	}
	return false
}

// emitTypedIIFE emits a block-expression IIFE with the concrete return type
// hint instead of `any`, threading hint through e.currentRetType so each inner
// return is emitted (and coerced) for that type. inner emits the body in return
// position (e.g. emitIfExprReturn).
func (e *Emitter) emitTypedIIFE(hint string, inner func() error) error {
	saved := e.currentRetType
	e.currentRetType = hint
	defer func() { e.currentRetType = saved }()
	e.writef("func() %s {", hint)
	e.nl()
	e.push()
	if err := inner(); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// tryEmitTypedMap recognises `(map (fn [x] ...) coll)` in a `[]T` position and
// emits a typed loop (`var r []T; for _, x := range _glispToSlice(coll) { r =
// append(r, fn(x)) }`) instead of `_glispMap`, which always returns `[]any` and
// so can't satisfy a typed slice binding/return. It only fires for a single,
// untyped-param lambda (signature `func(any) R`): the element from
// _glispToSlice passes straight in. When the lambda's Go return type is `any`
// (the common `(fn [v] (as T v))` case) the result is asserted to the element
// type. Returns (true, err) when it handled the call.
func (e *Emitter) tryEmitTypedMap(n *ast.CallExpr, hint string) (bool, error) {
	sym, ok := n.Head.(*ast.Symbol)
	if !ok || sym.Name != "map" || len(n.Args) != 2 {
		return false, nil
	}
	if !strings.HasPrefix(hint, "[]") {
		return false, nil
	}
	elem := strings.TrimSpace(hint[2:])
	if elem == "" || elem == "any" {
		return false, nil
	}
	fn, ok := n.Args[0].(*ast.FnExpr)
	if !ok {
		return false, nil
	}
	// Single untyped, non-destructured param only.
	if len(fn.Params) != 1 {
		return false, nil
	}
	p := fn.Params[0]
	if p.IsRest || p.Pattern != nil || p.TypeAnnot != nil {
		return false, nil
	}
	ret := e.formatReturnType(fn.ReturnType)
	switch ret {
	case "void":
		return false, nil
	case "":
		ret = "any"
	}

	r := e.fresh("m")
	x := e.fresh("x")
	e.writef("func() %s {", hint)
	e.nl()
	e.push()
	e.linef("%s := %s{}", r, hint)
	e.writeIndent()
	e.writef("for _, %s := range _glispToSlice(", x)
	if err := e.emitExpr(n.Args[1]); err != nil {
		return true, err
	}
	e.write(") {")
	e.nl()
	e.push()
	e.writeIndent()
	e.writef("%s = append(%s, (", r, r)
	if err := e.emitExpr(fn); err != nil {
		return true, err
	}
	e.writef(")(%s)", x)
	if ret == "any" {
		e.writef(".(%s)", elem)
	}
	e.write(")")
	e.nl()
	e.pop()
	e.line("}")
	e.linef("return %s", r)
	e.pop()
	e.writeIndent()
	e.write("}()")
	return true, nil
}

// typedSeqBuiltins lists sequence-returning built-ins whose result is a sequence
// of the *input* elements, so a per-element conversion to T is valid. A call to
// one in a `[]T` (T != any) position is wrapped in an element-converting loop —
// like tryEmitTypedMap — so it satisfies the typed slice instead of yielding the
// uncoercible `[]any` / `any` these helpers actually return.
var typedSeqBuiltins = map[string]bool{
	"filter": true, "remove": true, "conj": true, "into": true,
	"concat": true, "distinct": true, "take": true, "drop": true,
	"take-while": true, "drop-while": true, "reverse": true, "rest": true,
	"sort": true, "sort-by": true, "shuffle": true, "flatten": true,
}

// tryEmitTypedSeq recognises a sequence-returning built-in (typedSeqBuiltins) in
// a `[]T` (T != any) position and emits an element-converting IIFE instead of the
// raw `[]any`/`any` the helper returns, so it satisfies a typed slice binding or
// `-> []T` return. Returns (true, err) when it handled the call.
func (e *Emitter) tryEmitTypedSeq(v *ast.CallExpr, hint string) (bool, error) {
	if !strings.HasPrefix(hint, "[]") {
		return false, nil
	}
	elem := strings.TrimSpace(hint[2:])
	if elem == "" || elem == "any" {
		return false, nil
	}
	sym, ok := v.Head.(*ast.Symbol)
	if !ok || !typedSeqBuiltins[sym.Name] {
		return false, nil
	}
	return true, e.emitTypedSliceConv(hint, elem, func() error { return e.emitExpr(v) })
}

// emitTypedSliceConv emits an IIFE that ranges the (any/[]any-typed) sequence
// produced by inner and converts each element to elem, yielding a real []elem.
// Numeric element types route through the smart-coercion helpers (glisp ints are
// int64, so a blind `.(int)` would panic); every other type uses a `.(elem)`
// assertion (matching tryEmitTypedMap).
func (e *Emitter) emitTypedSliceConv(hint, elem string, inner func() error) error {
	r := e.fresh("s")
	x := e.fresh("x")
	e.writef("func() %s {", hint)
	e.nl()
	e.push()
	e.linef("%s := %s{}", r, hint)
	e.writeIndent()
	e.writef("for _, %s := range _glispToSlice(", x)
	if err := inner(); err != nil {
		return err
	}
	e.write(") {")
	e.nl()
	e.push()
	e.linef("%s = append(%s, %s)", r, r, elemConvExpr(elem, x))
	e.pop()
	e.line("}")
	e.linef("return %s", r)
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// elemConvExpr returns the Go text converting loop variable x (typed `any`) to
// elem: smart numeric coercion for numeric types, a type assertion otherwise.
func elemConvExpr(elem, x string) string {
	if pre, post, ok := numericCoercion(elem); ok {
		return pre + x + post
	}
	return x + ".(" + elem + ")"
}

// assertableHint reports whether a concrete type hint can be satisfied by
// asserting an `any` value to it (`v.(hint)`). Excludes "" / "any" (no-op),
// numeric types (smart-coerced elsewhere), and typed slices/maps `[]T`/`map[K]V`
// (T/V != any) — those need element conversion (tryEmitTypedSeq / typed map
// literals), not a blind assertion of the `[]any`/map the helpers return.
func assertableHint(hint string) bool {
	hint = strings.TrimSpace(hint)
	switch hint {
	case "", "any":
		return false
	}
	if _, _, ok := numericCoercion(hint); ok {
		return false
	}
	if strings.HasPrefix(hint, "[]") {
		return strings.TrimSpace(hint[2:]) == "any"
	}
	if strings.HasPrefix(hint, "map[") {
		return hint == "map[string]any"
	}
	return true
}

// callReturnsGoAny reports whether a call expression's Go static return type is
// the empty interface `any` — so a type assertion `.(T)` is legal on it. It is
// exprIsAny's call cases plus `conj`/`into`, which return `any` but are not part
// of the numeric-coercion any-set.
func (e *Emitter) callReturnsGoAny(v *ast.CallExpr) bool {
	if sym, ok := v.Head.(*ast.Symbol); ok {
		switch sym.Name {
		case "conj", "into":
			return true
		}
	}
	return e.exprIsAny(v)
}

// emitAtomExpr emits (atom init) → &_glispAtom{val: init} and the typed
// (atom T init) form, where the init is emitted under the element-type hint so
// a typed map/struct/numeric literal is built in its concrete shape.
func (e *Emitter) emitAtomExpr(n *ast.AtomExpr) error {
	e.needImport("_atom")
	e.write("&_glispAtom{val: ")
	hint := ""
	if n.ElemType != nil {
		hint = typeExprToGo(n.ElemType.Text)
	}
	if hint != "" && hint != "any" {
		if err := e.emitExprWithHint(n.Init, hint); err != nil {
			return err
		}
	} else if err := e.emitExpr(n.Init); err != nil {
		return err
	}
	e.write("}")
	return nil
}

// emitStructLitFromMap emits a struct literal from a plain map literal when the
// surrounding context expects a declared struct type. Keyword/symbol/string keys
// are matched against the struct's fields; an unknown field is a compile-time
// glisp error (catching typos before Go ever sees the code).
func (e *Emitter) emitStructLitFromMap(n *ast.MapLit, typeName string, ptr bool) error {
	si := e.structs[typeName]
	if ptr {
		e.write("&")
	}
	e.writef("%s{", typeName)
	for i, pair := range n.Pairs {
		if i > 0 {
			e.write(", ")
		}
		field, err := mapLitFieldName(pair.Key)
		if err != nil {
			return err
		}
		goField, ok := si.fields[field]
		if !ok {
			return fmt.Errorf("struct literal: %s has no field %q (at %s)", typeName, field, n.Pos())
		}
		e.writef("%s: ", goField)
		if err := e.emitExpr(pair.Value); err != nil {
			return err
		}
	}
	e.write("}")
	return nil
}

// mapLitFieldName extracts the glisp field name from a map-literal key node.
func mapLitFieldName(key ast.Node) (string, error) {
	switch k := key.(type) {
	case *ast.KeywordLit:
		return k.Value, nil
	case *ast.Symbol:
		return k.Name, nil
	case *ast.StringLit:
		return k.Value, nil
	default:
		return "", fmt.Errorf("struct literal field key must be a keyword, symbol, or string, got %T", key)
	}
}

// emitMapLitTyped emits a map literal with an explicit map type string.
func (e *Emitter) emitMapLitTyped(n *ast.MapLit, mapType string) error {
	e.writef("%s{", mapType)
	for i, pair := range n.Pairs {
		if i > 0 {
			e.write(", ")
		}
		if err := e.emitExpr(pair.Key); err != nil {
			return err
		}
		e.write(": ")
		if err := e.emitExpr(pair.Value); err != nil {
			return err
		}
	}
	e.write("}")
	return nil
}

// emitMapLit emits map[string]any{...} or map[K]V{...}
func (e *Emitter) emitMapLit(n *ast.MapLit) error {
	mapType := "map[string]any"
	if n.TypeAnnot != nil {
		mapType = typeExprToGo(n.TypeAnnot.Text)
	}
	return e.emitMapLitTyped(n, mapType)
}

// emitSetLit emits map[any]struct{}{...}
func (e *Emitter) emitSetLit(n *ast.SetLit) error {
	e.write("map[any]struct{}{")
	for i, elem := range n.Elements {
		if i > 0 {
			e.write(", ")
		}
		if err := e.emitExpr(elem); err != nil {
			return err
		}
		e.write(": {}")
	}
	e.write("}")
	return nil
}

// emitFnExpr emits an anonymous function literal.
// fn always returns any by default — every glisp expression has a value.
// Use ^void annotation to suppress the return type (for side-effect-only fns).
func (e *Emitter) emitFnExpr(n *ast.FnExpr) error {
	sigParts, destructs, err := e.buildParamSig(n.Params)
	if err != nil {
		return err
	}
	retStr := e.formatReturnType(n.ReturnType)
	isVoid := retStr == "void"
	if isVoid {
		retStr = ""
	} else if retStr == "" {
		retStr = "any"
	}
	sig := strings.Join(sigParts, ", ")
	if retStr != "" {
		e.writef("func(%s) %s {", sig, retStr)
	} else {
		e.writef("func(%s) {", sig)
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
	if err := e.emitBody(n.Body, !isVoid); err != nil {
		return err
	}
	e.currentRetType = savedRet
	e.popTypeScope(saved)
	e.pop()
	e.writeIndent()
	e.write("}")
	return nil
}

// paramDestruct holds a generated temp name and its destructuring pattern.
type paramDestruct struct {
	name    string
	pattern ast.Node
}

// buildParamSig builds Go signature parts for a param list.
// Returns signature strings and any destructured params needing body prologues.
func (e *Emitter) buildParamSig(params []ast.Param) ([]string, []paramDestruct, error) {
	parts := make([]string, 0, len(params))
	var destructs []paramDestruct
	for _, p := range params {
		typeStr := "any"
		if p.TypeAnnot != nil {
			typeStr = typeExprToGo(p.TypeAnnot.Text)
		}
		if p.Pattern != nil {
			name := e.fresh("p")
			if p.IsRest {
				parts = append(parts, name+" ..."+typeStr)
			} else {
				parts = append(parts, name+" "+typeStr)
			}
			destructs = append(destructs, paramDestruct{name, p.Pattern})
		} else {
			goName := identToGo(p.Name)
			if p.IsRest {
				parts = append(parts, goName+" ..."+typeStr)
			} else {
				parts = append(parts, goName+" "+typeStr)
			}
		}
	}
	return parts, destructs, nil
}

// emitParamDestructs emits destructuring bindings for fn/defn params.
func (e *Emitter) emitParamDestructs(destructs []paramDestruct) error {
	for _, d := range destructs {
		if err := e.emitDestructureBindings(d.name, d.pattern); err != nil {
			return err
		}
	}
	return nil
}

// mapDestructEntry is one binding from a map destructure pattern: the local name,
// the source key it reads, and an optional Go type from a ":- Type" annotation.
type mapDestructEntry struct {
	bind string
	key  string
	typ  string // Go type, "" when untyped
}

// mapDestructureEntries flattens a map destructure pattern's pairs into bindings,
// folding any ":- Type" annotation pair into the binding it follows. The pattern
// {name :name :- string age :age} yields name:string (typed) and age (untyped).
// Annotation types must be simple type names (string, int, Product, *Product,
// web/Request); bracketed types like []string are not supported here.
func mapDestructureEntries(pat *ast.MapLit) ([]mapDestructEntry, error) {
	var entries []mapDestructEntry
	pairs := pat.Pairs
	for i := 0; i < len(pairs); i++ {
		if isAnnotKey(pairs[i].Key) {
			return nil, fmt.Errorf("destructure annotation ':-' has no preceding binding")
		}
		sym, ok := pairs[i].Key.(*ast.Symbol)
		if !ok {
			return nil, fmt.Errorf("map destructure keys must be symbols, got %T", pairs[i].Key)
		}
		kw, ok := pairs[i].Value.(*ast.KeywordLit)
		if !ok {
			return nil, fmt.Errorf("map destructure values must be keywords, got %T", pairs[i].Value)
		}
		ent := mapDestructEntry{bind: sym.Name, key: kw.Value}
		if i+1 < len(pairs) && isAnnotKey(pairs[i+1].Key) {
			tsym, ok := pairs[i+1].Value.(*ast.Symbol)
			if !ok {
				return nil, fmt.Errorf("destructure type annotation for %q must be a simple type name, got %T", sym.Name, pairs[i+1].Value)
			}
			ent.typ = typeExprToGo(tsym.Name)
			i++ // consume the annotation pair
		}
		entries = append(entries, ent)
	}
	return entries, nil
}

// isAnnotKey reports whether a map-pair key is the ":-" destructure annotation marker.
func isAnnotKey(key ast.Node) bool {
	kw, ok := key.(*ast.KeywordLit)
	return ok && kw.Value == "-"
}

// emitDestructureBindings emits variable bindings from a VectorLit or MapLit pattern.
func (e *Emitter) emitDestructureBindings(src string, pattern ast.Node) error {
	switch pat := pattern.(type) {
	case *ast.VectorLit:
		for i, el := range pat.Elements {
			sym, ok := el.(*ast.Symbol)
			if !ok {
				return fmt.Errorf("sequential destructure elements must be symbols, got %T", el)
			}
			if sym.Name == "_" {
				continue // discard: emitting `_ := ...` is illegal Go
			}
			e.registerAnyVar(sym.Name) // _glispGet returns any
			e.writeIndent()
			e.writef("%s := _glispGet(%s, int64(%d))\n", identToGo(sym.Name), src, i)
		}
	case *ast.MapLit:
		entries, err := mapDestructureEntries(pat)
		if err != nil {
			return err
		}
		for _, ent := range entries {
			if ent.bind == "_" {
				continue // discard
			}
			name := identToGo(ent.bind)
			e.writeIndent()
			switch ent.typ {
			case "":
				// Untyped: value stays any via the runtime lookup.
				e.writef("%s := _glispGet(%s, %q)\n", name, src, ent.key)
			case "string":
				e.writef("%s := _glispToString(_glispGet(%s, %q))\n", name, src, ent.key)
			case "int":
				e.writef("%s := _glispToInt(_glispGet(%s, %q))\n", name, src, ent.key)
			case "float64":
				e.writef("%s := _glispToFloat64(_glispGet(%s, %q))\n", name, src, ent.key)
			default:
				// Checked assertion: zero value if absent or the wrong type.
				e.writef("%s, _ := _glispGet(%s, %q).(%s)\n", name, src, ent.key, ent.typ)
			}
			if ent.typ != "" {
				e.registerVarType(ent.bind, ent.typ)
			} else {
				e.registerAnyVar(ent.bind) // untyped _glispGet returns any
			}
		}
	default:
		return fmt.Errorf("unsupported destructure pattern: %T", pattern)
	}
	return nil
}

// emitLetExpr emits a let binding block, returning the last expression.
// Inline approach: emit as a func closure that is immediately invoked.
// Simpler approach: emit bindings as := statements in a block.
// We use the block approach since closures add overhead and complexity.
// The result variable captures the return value.
func (e *Emitter) emitLetExpr(n *ast.LetExpr) error {
	// Let in expression position: use an immediately-invoked closure.
	// This is the only clean way to make let return a value in Go.
	e.write("func() any {")
	e.nl()
	e.push()
	if err := e.emitLetBody(n); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitLetExprReturn emits let in return position (no wrapping closure needed).
func (e *Emitter) emitLetExprReturn(n *ast.LetExpr) error {
	saved := e.pushTypeScope()
	defer e.popTypeScope(saved)
	if err := e.emitLetBindings(n.Bindings); err != nil {
		return err
	}
	return e.emitBody(n.Body, true)
}

func (e *Emitter) emitLetBody(n *ast.LetExpr) error {
	saved := e.pushTypeScope()
	defer e.popTypeScope(saved)
	if err := e.emitLetBindings(n.Bindings); err != nil {
		return err
	}
	return e.emitBody(n.Body, true)
}

func (e *Emitter) emitLetBindings(bindings []ast.LetBinding) error {
	for _, b := range bindings {
		// A binding holds one value; a known multi-return call can't compile here.
		if err := e.checkMultiReturnValue(b.Value); err != nil {
			return err
		}
		switch pat := b.Pattern.(type) {
		case *ast.Symbol:
			goName := identToGo(pat.Name)
			typeStr := ""
			if b.TypeAnnot != nil {
				typeStr = typeExprToGo(b.TypeAnnot.Text)
			}
			e.writeIndent()
			if typeStr != "" {
				e.writef("var %s %s = ", goName, typeStr)
				if err := e.emitExprWithHint(b.Value, typeStr); err != nil {
					return err
				}
				e.registerVarType(pat.Name, typeStr)
				if typeStr != "any" {
					e.clearAnyVar(pat.Name) // explicit concrete type
				}
				if elem, ok := e.atomElemOfBinding(b.TypeAnnot, b.Value); ok {
					e.registerAtomType(pat.Name, elem)
				} else {
					e.clearAtomType(pat.Name)
				}
			} else {
				// Evaluate any-ness of the RHS before the binding shadows an
				// outer same-named var in localAny.
				valueIsAny := e.exprIsAny(b.Value)
				e.writef("%s := ", goName)
				if err := e.emitExpr(b.Value); err != nil {
					return err
				}
				if valueIsAny {
					e.registerAnyVar(pat.Name) // Go infers `any` from the RHS
					e.clearNumericVar(pat.Name)
				} else {
					e.registerLocalVar(pat.Name)
					e.clearAnyVar(pat.Name)
					// Track a concrete numeric RHS so arithmetic on the binding
					// promotes (e.g. (let [r (math/sqrt x)] (+ r int-var))).
					if k := e.bindingNumericKind(b.Value); k != "" {
						e.registerNumericVar(pat.Name, k)
					} else {
						e.clearNumericVar(pat.Name)
					}
				}
				// Infer struct type from the value (struct literal or known fn return)
				// so keyword access on the binding resolves to field access.
				if name := e.inferValueStructType(b.Value); name != "" && e.localTypes != nil {
					e.localTypes[pat.Name] = name
				}
				// Track an atom binding so a typed (deref name) coerces.
				if elem, ok := e.atomElemOfBinding(b.TypeAnnot, b.Value); ok {
					e.registerAtomType(pat.Name, elem)
				} else {
					e.clearAtomType(pat.Name)
				}
			}
			e.nl()
		case *ast.VectorLit:
			// Sequential destructure: [[a b c] coll] → positional _glispGet indexing
			tmp := e.fresh("v")
			e.writeIndent()
			e.writef("%s := ", tmp)
			if err := e.emitExpr(b.Value); err != nil {
				return err
			}
			e.nl()
			if err := e.emitDestructureBindings(tmp, pat); err != nil {
				return err
			}
		case *ast.MapLit:
			// Map destructure: [{k :key} m] → _glispGet by string key
			tmp := e.fresh("m")
			e.writeIndent()
			e.writef("%s := ", tmp)
			if err := e.emitExpr(b.Value); err != nil {
				return err
			}
			e.nl()
			if err := e.emitDestructureBindings(tmp, pat); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported let pattern: %T", b.Pattern)
		}
	}
	return nil
}

// boolBuiltins are built-in forms whose emission is statically a Go bool
// expression, so a condition built from them needs no _glispTruthy wrapper.
// Note: "some" and "not-empty" return values (not bool) and are excluded.
var boolBuiltins = map[string]bool{
	"=": true, "not=": true, "<": true, ">": true, "<=": true, ">=": true,
	"and": true, "or": true, "not": true,
	"nil?": true, "empty?": true, "contains?": true, "every?": true,
	"not-any?": true, "even?": true, "odd?": true, "pos?": true,
	"neg?": true, "zero?": true, "blank?": true,
	"starts-with?": true, "ends-with?": true,
	"file-exists?": true, "re/match": true, "errors/is?": true,
	"ctx/done?": true,
}

// voidReturnBuiltins is the set of built-in / stdlib-qualified call forms whose
// Go emission returns nothing. Used as a return value (e.g. the tail of a
// value-returning fn's when/do/if branch) they need a `<call>; return nil`
// split — `return os.Exit(0)` is invalid Go. fmt/print* and log/* are handled
// separately (they have dedicated emitters), so they're not listed here.
var voidReturnBuiltins = map[string]bool{
	"os/exit": true,
}

// isVoidCall reports whether n emits a Go call that returns no value: a known
// void built-in, or a user defn / method statically declared `-> void`.
func (e *Emitter) isVoidCall(n *ast.CallExpr) bool {
	sym, ok := n.Head.(*ast.Symbol)
	if !ok {
		return false
	}
	if voidReturnBuiltins[sym.Name] {
		return true
	}
	if !e.localVars[sym.Name] {
		if sig, ok := e.symbols[sym.Name]; ok {
			return sig.retType == "void"
		}
	}
	if info, ok := e.resolveMethodCall(n); ok {
		return info.sig.retType == "void"
	}
	return false
}

// anyReturningBuiltins are built-in forms whose Go emission is always typed
// `any` (interface), regardless of operand types — primarily map/slice lookups.
// An arithmetic/comparison operand drawn from one of these must be coerced
// numerically rather than fed to a native Go operator.
var anyReturningBuiltins = map[string]bool{
	"get": true, "get-in": true, "nth": true, "first": true, "second": true,
	"last": true, "peek": true, "rand-nth": true, "find": true, "deref": true,
	"reduce": true, "apply": true,
	// debugging / threading helpers that pass a value through as `any`
	"pp": true, "time-it": true, "as->": true, "tap->": true, "tap->>": true,
}

// exprIsAny reports whether n is statically known to emit a Go `any` value.
// Conservative: it only returns true for values it can prove are `any` (so that
// arithmetic/comparison on them routes through the numeric coercion helpers);
// anything else is treated as native to avoid changing the static type of
// currently-compiling typed code.
func (e *Emitter) exprIsAny(n ast.Node) bool {
	switch v := n.(type) {
	case *ast.Symbol:
		return e.localAny[v.Name]
	case *ast.CallExpr:
		sym, ok := v.Head.(*ast.Symbol)
		if !ok {
			// Keyword access (:kw x): `any` only when the target is itself `any`
			// (untyped map lookup → _glispGet). Typed struct fields keep their
			// real Go type, so they are not `any`.
			if _, isKw := v.Head.(*ast.KeywordLit); isKw && len(v.Args) == 1 {
				return e.exprIsAny(v.Args[0])
			}
			return false
		}
		switch sym.Name {
		case "+", "-", "*", "/", "mod":
			// Arithmetic over any `any` operand routes through a helper → `any`.
			return e.anyOperand(v.Args)
		case "deref":
			// A typed atom whose element coerces to a concrete scalar derefs to
			// that scalar, not `any` — so no extra numeric wrapping is applied.
			if len(v.Args) == 1 {
				if elem, ok := e.atomTypeOfExpr(v.Args[0]); ok {
					if _, _, ok := derefCoercion(elem); ok {
						return false
					}
				}
			}
			return true
		}
		if anyReturningBuiltins[sym.Name] {
			return true
		}
		if !e.localVars[sym.Name] {
			if sig, found := e.symbols[sym.Name]; found {
				return sig.retType == "any"
			}
		}
		if info, ok := e.resolveMethodCall(v); ok {
			return info.sig.retType == "any"
		}
	}
	return false
}

// numericKind classifies n as a concrete numeric Go value of kind "int" or
// "float", or "" when it is not statically a concrete numeric — untyped
// constants (an int/float literal adapts to either kind in Go), `any` values,
// and non-numeric expressions all return "". Drives int→float64 auto-promotion
// when an arithmetic/comparison form mixes concrete int and float operands.
func (e *Emitter) numericKind(n ast.Node) string {
	switch v := n.(type) {
	case *ast.FloatLit:
		// A float literal forces a float context (its value may not be an exact
		// integer, so a sibling typed-int operand must be promoted).
		return "float"
	case *ast.IntLit:
		return ""
	case *ast.Symbol:
		if k, ok := e.localNumeric[v.Name]; ok {
			return k
		}
		return e.globalNumeric[v.Name]
	case *ast.TypeAssertExpr:
		// (as float64 x) / (as int x) yields a concrete numeric.
		if v.Type != nil {
			return numericGoKind(typeExprToGo(v.Type.Text))
		}
	case *ast.CallExpr:
		return e.callNumericKind(v)
	}
	return ""
}

// callNumericKind returns the numeric kind a call expression statically yields.
func (e *Emitter) callNumericKind(n *ast.CallExpr) string {
	sym, ok := n.Head.(*ast.Symbol)
	if !ok {
		return ""
	}
	switch sym.Name {
	case "int":
		return "int"
	case "float64", "float32":
		return "float"
	case "+", "-", "*", "/", "mod":
		// Arithmetic over an `any` operand yields `any`, not a concrete numeric.
		if e.anyOperand(n.Args) {
			return ""
		}
		// Result is float if any operand is float (this function's own promotion
		// makes the form float64), else int if any operand is a concrete int.
		kind := ""
		for _, a := range n.Args {
			switch e.numericKind(a) {
			case "float":
				return "float"
			case "int":
				kind = "int"
			}
		}
		return kind
	}
	// math/* all return float64.
	if _, ok := stdlibNumericParams[sym.Name]; ok {
		return "float"
	}
	// A typed (deref atom) of a numeric element type.
	if sym.Name == "deref" && len(n.Args) == 1 {
		if elem, ok := e.atomTypeOfExpr(n.Args[0]); ok {
			return numericGoKind(elem)
		}
	}
	// A user fn / method with a declared numeric return type.
	if !e.localVars[sym.Name] {
		if sig, found := e.symbols[sym.Name]; found {
			return numericGoKind(sig.retType)
		}
	}
	if info, ok := e.resolveMethodCall(n); ok {
		return numericGoKind(info.sig.retType)
	}
	return ""
}

// bindingNumericKind returns the numeric kind of the Go variable a `:=` binding
// over value produces. An int/float literal init makes the variable concretely
// int/float64 (Go's inference), even though the bare literal is an untyped
// constant for numericKind's purposes; other inits defer to numericKind.
func (e *Emitter) bindingNumericKind(value ast.Node) string {
	switch value.(type) {
	case *ast.IntLit:
		return "int"
	case *ast.FloatLit:
		return "float"
	}
	return e.numericKind(value)
}

// mixesIntFloat reports whether args contain both a concrete int operand and a
// concrete float operand — the case Go won't compile without an explicit
// conversion, so an int→float64 promotion is needed.
func (e *Emitter) mixesIntFloat(args []ast.Node) bool {
	hasInt, hasFloat := false, false
	for _, a := range args {
		switch e.numericKind(a) {
		case "int":
			hasInt = true
		case "float":
			hasFloat = true
		}
	}
	return hasInt && hasFloat
}

// emitPromotedOperand emits a, wrapping it in float64(...) when promote is set
// and a is a concrete int operand (auto-promotion in mixed int/float forms).
func (e *Emitter) emitPromotedOperand(a ast.Node, promote bool) error {
	if promote && e.numericKind(a) == "int" {
		e.write("float64(")
		if err := e.emitExpr(a); err != nil {
			return err
		}
		e.write(")")
		return nil
	}
	return e.emitExpr(a)
}

// anyOperand reports whether any of args is statically Go `any`.
func (e *Emitter) anyOperand(args []ast.Node) bool {
	for _, a := range args {
		if e.exprIsAny(a) {
			return true
		}
	}
	return false
}

// isBoolExpr reports whether n is statically known to emit a Go bool.
// User-defined functions count when their declared return type is bool.
func (e *Emitter) isBoolExpr(n ast.Node) bool {
	switch v := n.(type) {
	case *ast.BoolLit:
		return true
	case *ast.CallExpr:
		sym, ok := v.Head.(*ast.Symbol)
		if !ok {
			return false
		}
		if sig, ok := e.symbols[sym.Name]; ok {
			return sig.retType == "bool"
		}
		if boolBuiltins[sym.Name] {
			return true
		}
		if info, ok := e.resolveMethodCall(v); ok {
			return info.sig.retType == "bool"
		}
		return false
	}
	return false
}

// emitCondition emits n as a Go boolean condition: statically-bool expressions
// emit as-is, everything else is wrapped in _glispTruthy (nil/false falsy,
// everything else truthy). This is what lets (if x ...) work on any-typed
// values without an explicit (not= x nil).
func (e *Emitter) emitCondition(n ast.Node) error {
	if e.isBoolExpr(n) {
		return e.emitExpr(n)
	}
	e.write("_glispTruthy(")
	if err := e.emitExpr(n); err != nil {
		return err
	}
	e.write(")")
	return nil
}

// emitIfExpr emits an if expression.
// In expression position, we use an immediately-invoked closure.
func (e *Emitter) emitIfExpr(n *ast.IfExpr) error {
	// Expression position: IIFE
	e.write("func() any {")
	e.nl()
	e.push()
	if err := e.emitIfExprReturn(n); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

func (e *Emitter) emitIfExprReturn(n *ast.IfExpr) error {
	e.writeIndent()
	e.write("if ")
	if err := e.emitCondition(n.Cond); err != nil {
		return err
	}
	e.write(" {")
	e.nl()
	e.push()
	if err := e.emitReturnNode(n.Then); err != nil {
		return err
	}
	e.pop()
	if n.Else != nil {
		e.line("} else {")
		e.push()
		if err := e.emitReturnNode(n.Else); err != nil {
			return err
		}
		e.pop()
	}
	e.line("}")
	return nil
}

// emitWhenExpr emits a when block (no else).
func (e *Emitter) emitWhenExpr(n *ast.WhenExpr) error {
	e.write("func() any {")
	e.nl()
	e.push()
	if err := e.emitWhenReturn(n); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitWhenReturn emits a when's body in return position (the inside of its
// IIFE): the truthy branch returns its last expr, the false case returns nil.
func (e *Emitter) emitWhenReturn(n *ast.WhenExpr) error {
	e.writeIndent()
	e.write("if ")
	if err := e.emitCondition(n.Cond); err != nil {
		return err
	}
	e.write(" {")
	e.nl()
	e.push()
	if err := e.emitBody(n.Body, true); err != nil {
		return err
	}
	e.pop()
	e.line("}")
	e.line("return nil")
	return nil
}

// emitBindLetPrologue emits the binding for an if-let/when-let pattern at the
// current indentation and returns the Go name to nil-test. For destructuring
// patterns (*VectorLit / *MapLit) the value is bound to a fresh temp and the
// pattern is returned so the caller can emit the destructured bindings inside
// the truthy branch only (keeping them out of scope in the else/nil branch).
func (e *Emitter) emitBindLetPrologue(pattern, expr ast.Node) (string, ast.Node, error) {
	if err := e.checkMultiReturnValue(expr); err != nil {
		return "", nil, err
	}
	if sym, ok := pattern.(*ast.Symbol); ok {
		name := identToGo(sym.Name)
		e.registerLocalVar(sym.Name)
		e.writeIndent()
		e.writef("%s := ", name)
		if err := e.emitExpr(expr); err != nil {
			return "", nil, err
		}
		e.nl()
		return name, nil, nil
	}
	tmp := e.fresh("t")
	e.writeIndent()
	e.writef("%s := ", tmp)
	if err := e.emitExpr(expr); err != nil {
		return "", nil, err
	}
	e.nl()
	return tmp, pattern, nil
}

// emitIfLetExpr emits an if-let form in expression position (IIFE).
func (e *Emitter) emitIfLetExpr(n *ast.IfLetExpr) error {
	e.write("func() any {")
	e.nl()
	e.push()
	if err := e.emitIfLetReturn(n); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitIfLetReturn emits if-let in return position (no closure wrapper).
func (e *Emitter) emitIfLetReturn(n *ast.IfLetExpr) error {
	name, destruct, err := e.emitBindLetPrologue(n.Pattern, n.Expr)
	if err != nil {
		return err
	}
	e.writeIndent()
	e.writef("if %s != nil {", name)
	e.nl()
	e.push()
	if destruct != nil {
		if err := e.emitDestructureBindings(name, destruct); err != nil {
			return err
		}
	}
	if err := e.emitReturnNode(n.Then); err != nil {
		return err
	}
	e.pop()
	if n.Else != nil {
		e.line("} else {")
		e.push()
		if err := e.emitReturnNode(n.Else); err != nil {
			return err
		}
		e.pop()
		e.line("}")
		return nil
	}
	e.line("}")
	e.line("return nil")
	return nil
}

// emitIfLetStmt emits an if-let in statement position.
func (e *Emitter) emitIfLetStmt(n *ast.IfLetExpr) error {
	name, destruct, err := e.emitBindLetPrologue(n.Pattern, n.Expr)
	if err != nil {
		return err
	}
	e.writeIndent()
	e.writef("if %s != nil {", name)
	e.nl()
	e.push()
	if destruct != nil {
		if err := e.emitDestructureBindings(name, destruct); err != nil {
			return err
		}
	}
	if err := e.emitStmtNode(n.Then); err != nil {
		return err
	}
	e.pop()
	if n.Else != nil {
		e.line("} else {")
		e.push()
		if err := e.emitStmtNode(n.Else); err != nil {
			return err
		}
		e.pop()
	}
	e.line("}")
	return nil
}

// emitLetOrReturn emits let-or in return position: flat sequential nil guards.
func (e *Emitter) emitLetOrReturn(n *ast.LetOrExpr) error {
	for _, b := range n.Bindings {
		if err := e.checkMultiReturnValue(b.Expr); err != nil {
			return err
		}
		goName := identToGo(b.Name)
		e.registerLocalVar(b.Name)
		e.writeIndent()
		e.writef("%s := ", goName)
		if err := e.emitExpr(b.Expr); err != nil {
			return err
		}
		e.nl()
		e.writeIndent()
		e.writef("if %s == nil {", goName)
		e.nl()
		e.push()
		if err := e.emitReturnNode(b.Fallback); err != nil {
			return err
		}
		e.pop()
		e.line("}")
	}
	return e.emitBody(n.Body, true)
}

// emitLetOrExpr emits let-or in expression position (IIFE).
func (e *Emitter) emitLetOrExpr(n *ast.LetOrExpr) error {
	e.write("func() any {")
	e.nl()
	e.push()
	if err := e.emitLetOrReturn(n); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitLetOrStmt emits let-or in statement position (no return wrapper).
func (e *Emitter) emitLetOrStmt(n *ast.LetOrExpr) error {
	for _, b := range n.Bindings {
		if err := e.checkMultiReturnValue(b.Expr); err != nil {
			return err
		}
		goName := identToGo(b.Name)
		e.registerLocalVar(b.Name)
		e.writeIndent()
		e.writef("%s := ", goName)
		if err := e.emitExpr(b.Expr); err != nil {
			return err
		}
		e.nl()
		e.writeIndent()
		e.writef("if %s == nil {", goName)
		e.nl()
		e.push()
		if err := e.emitStmtNode(b.Fallback); err != nil {
			return err
		}
		e.pop()
		e.line("}")
	}
	return e.emitBody(n.Body, false)
}

// emitWhenLetExpr emits a when-let form in expression position (IIFE).
func (e *Emitter) emitWhenLetExpr(n *ast.WhenLetExpr) error {
	e.write("func() any {")
	e.nl()
	e.push()
	if err := e.emitWhenLetReturn(n); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitWhenLetReturn emits when-let in return position (no closure wrapper).
func (e *Emitter) emitWhenLetReturn(n *ast.WhenLetExpr) error {
	name, destruct, err := e.emitBindLetPrologue(n.Pattern, n.Expr)
	if err != nil {
		return err
	}
	e.writeIndent()
	e.writef("if %s != nil {", name)
	e.nl()
	e.push()
	if destruct != nil {
		if err := e.emitDestructureBindings(name, destruct); err != nil {
			return err
		}
	}
	if err := e.emitBody(n.Body, true); err != nil {
		return err
	}
	e.pop()
	e.line("}")
	e.line("return nil")
	return nil
}

// emitWhenLetStmt emits a when-let in statement position.
func (e *Emitter) emitWhenLetStmt(n *ast.WhenLetExpr) error {
	name, destruct, err := e.emitBindLetPrologue(n.Pattern, n.Expr)
	if err != nil {
		return err
	}
	e.writeIndent()
	e.writef("if %s != nil {", name)
	e.nl()
	e.push()
	if destruct != nil {
		if err := e.emitDestructureBindings(name, destruct); err != nil {
			return err
		}
	}
	for _, node := range n.Body {
		if err := e.emitStmtNode(node); err != nil {
			return err
		}
	}
	e.pop()
	e.line("}")
	return nil
}

// emitCondExpr emits a cond expression.
func (e *Emitter) emitCondExpr(n *ast.CondExpr) error {
	e.write("func() any {")
	e.nl()
	e.push()
	if err := e.emitCondExprReturn(n); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

func (e *Emitter) emitCondExprReturn(n *ast.CondExpr) error {
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
		if err := e.emitReturnNode(clause.Body); err != nil {
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
		if err := e.emitReturnNode(n.Default); err != nil {
			return err
		}
		e.pop()
	}
	if len(n.Clauses) > 0 || n.Default != nil {
		e.line("}")
	}
	return nil
}

// emitAssertGuard writes `if !(<cond>) { panic(<msg>) }` with no surrounding
// indentation or newline. With one arg the panic message is auto-generated from
// the condition's source text; a second arg supplies an explicit message.
func (e *Emitter) emitAssertGuard(n *ast.CallExpr) error {
	// Central arity gate (emitCallExpr already ran it for the expression path;
	// re-run here so the statement/return paths report the same canonical error).
	if err := e.checkBuiltinArity("assert", n); err != nil {
		return err
	}
	e.write("if !(")
	if err := e.emitCondition(n.Args[0]); err != nil {
		return err
	}
	e.write(") { panic(")
	if len(n.Args) == 2 {
		if err := e.emitExpr(n.Args[1]); err != nil {
			return err
		}
	} else {
		e.writef("%q", "assertion failed: "+formatter.FormatNode(n.Args[0]))
	}
	e.write(") }")
	return nil
}

// emitSwitchExpr emits a switch expression (IIFE wrapper for expression position).
func (e *Emitter) emitSwitchExpr(n *ast.SwitchExpr) error {
	e.write("func() any {")
	e.nl()
	e.push()
	if err := e.emitSwitchExprReturn(n); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitSwitchExprReturn emits a switch in return position (no IIFE).
func (e *Emitter) emitSwitchExprReturn(n *ast.SwitchExpr) error {
	e.writeIndent()
	e.write("switch ")
	if err := e.emitExpr(n.Expr); err != nil {
		return err
	}
	e.write(" {")
	e.nl()
	for _, sc := range n.Cases {
		e.writeIndent()
		e.write("case ")
		if err := e.emitExpr(sc.Value); err != nil {
			return err
		}
		e.write(":")
		e.nl()
		e.push()
		if err := e.emitReturnNode(sc.Body); err != nil {
			return err
		}
		e.pop()
	}
	if n.Default != nil {
		e.line("default:")
		e.push()
		if err := e.emitReturnNode(n.Default); err != nil {
			return err
		}
		e.pop()
	}
	e.line("}")
	if n.Default == nil {
		e.line("return nil")
	}
	return nil
}

// emitSwitchStmt emits a switch in statement position (no IIFE, no return).
func (e *Emitter) emitSwitchStmt(n *ast.SwitchExpr) error {
	e.writeIndent()
	e.write("switch ")
	if err := e.emitExpr(n.Expr); err != nil {
		return err
	}
	e.write(" {")
	e.nl()
	for _, sc := range n.Cases {
		e.writeIndent()
		e.write("case ")
		if err := e.emitExpr(sc.Value); err != nil {
			return err
		}
		e.write(":")
		e.nl()
		e.push()
		if err := e.emitStmtNode(sc.Body); err != nil {
			return err
		}
		e.pop()
	}
	if n.Default != nil {
		e.line("default:")
		e.push()
		if err := e.emitStmtNode(n.Default); err != nil {
			return err
		}
		e.pop()
	}
	e.line("}")
	return nil
}

// emitDoExpr emits a do block.
func (e *Emitter) emitDoExpr(n *ast.DoExpr) error {
	e.write("func() any {")
	e.nl()
	e.push()
	if err := e.emitBody(n.Body, true); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

func (e *Emitter) emitDoExprReturn(n *ast.DoExpr) error {
	return e.emitBody(n.Body, true)
}

// emitCallExpr emits a function or operator call.
func (e *Emitter) emitCallExpr(n *ast.CallExpr) error {
	// Core library resolution (Phase 14): a (str/upper …) call whose qualifier
	// is a core namespace is rewritten to its mangled helper name and proceeds
	// through the normal user-fn path (so arity + arg coercion apply). User defns
	// are checked first below — but a user defn shadows core only by bare name,
	// and core calls are always qualified, so there is no conflict here.
	if sym, ok := n.Head.(*ast.Symbol); ok {
		if mangled, ns, ok := resolveCoreCall(sym.Name); ok {
			e.needImport(coreNeededKey(ns))
			n = ast.NewCallExpr(n.Pos_, ast.NewSymbol(sym.Pos_, mangled), n.Args)
		} else if !e.coreBareShadowed(sym.Name) {
			// Bare core function (slurp, …), callable unqualified. Resolved only
			// when not shadowed by a user defn, a local binding, or a def global;
			// built-ins are matched by the switch below first since the bare core
			// names never overlap them.
			if mangled, ns, ok := resolveCoreBare(sym.Name); ok {
				e.needImport(coreNeededKey(ns))
				n = ast.NewCallExpr(n.Pos_, ast.NewSymbol(sym.Pos_, mangled), n.Args)
			}
		}
	}
	// Handle built-in operators
	if sym, ok := n.Head.(*ast.Symbol); ok {
		// Front-gate every built-in against the canonical arity table so a
		// wrong-arity call yields a positioned glisp error rather than an index
		// panic in a downstream emit helper. Names absent from the table are
		// validated by their own handler below.
		if err := e.checkBuiltinArity(sym.Name, n); err != nil {
			return err
		}
		switch sym.Name {
		case "+", "-", "*", "/", "mod":
			return e.emitArith(sym.Name, n.Args)
		case "=":
			return e.emitBinOp("==", n.Args)
		case "not=":
			return e.emitBinOp("!=", n.Args)
		case "<", ">", "<=", ">=":
			return e.emitBinOp(sym.Name, n.Args)
		case "and":
			return e.emitLogicOp("&&", n.Args)
		case "or":
			return e.emitLogicOp("||", n.Args)
		case "not":
			if len(n.Args) != 1 {
				return fmt.Errorf("not requires 1 argument")
			}
			e.write("!(")
			if err := e.emitCondition(n.Args[0]); err != nil {
				return err
			}
			e.write(")")
			return nil
		case "str":
			return e.emitStr(n.Args)
		case "fmt/println", "fmt/print", "println", "print":
			return e.emitFmtPrint(sym.Name, n.Args)
		case "get":
			return e.emitGet(n.Args)
		case "assoc":
			return e.emitAssoc(n.Args)
		case "dissoc":
			return e.emitDissoc(n.Args)
		case "conj":
			return e.emitConj(n.Args)
		case "count", "len":
			return e.emitCount(n.Args)
		case "first":
			return e.emitFirst(n.Args)
		case "rest":
			return e.emitRest(n.Args)
		case "nth":
			return e.emitNth(n.Args)
		case "keys":
			return e.emitKeys(n.Args)
		case "vals":
			return e.emitVals(n.Args)
		case "merge":
			return e.emitMerge(n.Args)
		case "error":
			return e.emitError(n.Args)
		case "nil?":
			return e.emitNilQ(n.Args)
		case "string":
			return e.emitStringConv(n.Args)
		case "int":
			return e.emitIntConv(n.Args)
		case "float64":
			return e.emitFloat64Conv(n.Args)
		// -> and ->> are core macros (internal/macro/core.glsp); they are
		// expanded before emission and never reach here.
		case "as->":
			return e.emitAsThread(n.Args)
		case "tap->":
			return e.emitTapFirst(n.Args)
		case "tap->>":
			return e.emitTapLast(n.Args)
		case "pp":
			e.needImport("_pp")
			return e.emitRuntimeCall("_glispPP", n.Args, 1)
		case "time-it":
			return e.emitTimeIt(n.Args)
		case "doseq":
			return e.emitDoseq(n.Args)
		case "dotimes":
			return e.emitDotimes(n.Args)
		// 2a: collection operations
		case "map":
			return e.emitRuntimeCall("_glispMap", n.Args, 2)
		case "for":
			return e.emitFor(n.Args)
		case "map-indexed":
			return e.emitRuntimeCall("_glispMapIndexed", n.Args, 2)
		case "filter":
			return e.emitRuntimeCall("_glispFilter", n.Args, 2)
		case "reduce":
			return e.emitRuntimeCall("_glispReduce", n.Args, 3)
		case "reverse":
			return e.emitRuntimeCall("_glispReverse", n.Args, 1)
		case "contains?":
			return e.emitRuntimeCall("_glispContains", n.Args, 2)
		case "some":
			return e.emitRuntimeCall("_glispSome", n.Args, 2)
		case "every?":
			return e.emitRuntimeCall("_glispEvery", n.Args, 2)
		case "sort-by":
			e.needImport("sort")
			return e.emitRuntimeCall("_glispSortBy", n.Args, 2)
		case "flatten":
			return e.emitRuntimeCall("_glispFlatten", n.Args, 1)
		case "range":
			return e.emitVariadicRuntimeCall("_glispRange", n.Args)
		case "take":
			return e.emitRuntimeCall("_glispTake", n.Args, 2)
		case "drop":
			return e.emitRuntimeCall("_glispDrop", n.Args, 2)
		// 7b: data transformation
		case "second":
			return e.emitRuntimeCall("_glispSecond", n.Args, 1)
		case "last":
			return e.emitRuntimeCall("_glispLast", n.Args, 1)
		case "empty?":
			return e.emitRuntimeCall("_glispIsEmpty", n.Args, 1)
		case "not-empty":
			return e.emitRuntimeCall("_glispNotEmpty", n.Args, 1)
		case "get-in":
			return e.emitRuntimeCall("_glispGetIn", n.Args, 2)
		case "assoc-in":
			e.needImport("data")
			return e.emitRuntimeCall("_glispAssocIn", n.Args, 3)
		case "update-in":
			e.needImport("data")
			return e.emitRuntimeCall("_glispUpdateIn", n.Args, 3)
		case "update":
			return e.emitRuntimeCall("_glispUpdate", n.Args, 3)
		case "select-keys":
			return e.emitRuntimeCall("_glispSelectKeys", n.Args, 2)
		case "rename-keys":
			e.needImport("data")
			return e.emitRuntimeCall("_glispRenameKeys", n.Args, 2)
		case "group-by":
			e.needImport("data")
			return e.emitRuntimeCall("_glispGroupBy", n.Args, 2)
		case "frequencies":
			e.needImport("data")
			return e.emitRuntimeCall("_glispFrequencies", n.Args, 1)
		case "into":
			return e.emitRuntimeCall("_glispInto", n.Args, 2)
		case "concat":
			return e.emitVariadicRuntimeCall("_glispConcat", n.Args)
		case "mapcat":
			return e.emitRuntimeCall("_glispMapcat", n.Args, 2)
		case "take-while":
			return e.emitRuntimeCall("_glispTakeWhile", n.Args, 2)
		case "drop-while":
			return e.emitRuntimeCall("_glispDropWhile", n.Args, 2)
		case "zipmap":
			e.needImport("data")
			return e.emitRuntimeCall("_glispZipmap", n.Args, 2)
		case "partition":
			return e.emitRuntimeCall("_glispPartition", n.Args, 2)
		case "partition-by":
			e.needImport("data")
			return e.emitRuntimeCall("_glispPartitionBy", n.Args, 2)
		// sequence conveniences
		case "distinct":
			return e.emitRuntimeCall("_glispDistinct", n.Args, 1)
		case "remove":
			return e.emitRuntimeCall("_glispRemove", n.Args, 2)
		case "keep":
			return e.emitRuntimeCall("_glispKeep", n.Args, 2)
		case "split-at":
			return e.emitRuntimeCall("_glispSplitAt", n.Args, 2)
		case "split-with":
			return e.emitRuntimeCall("_glispSplitWith", n.Args, 2)
		case "interleave":
			return e.emitVariadicRuntimeCall("_glispInterleave", n.Args)
		case "not-any?":
			return e.emitRuntimeCall("_glispNotAny", n.Args, 2)
		// numeric predicates and arithmetic
		case "even?":
			return e.emitRuntimeCall("_glispIsEven", n.Args, 1)
		case "odd?":
			return e.emitRuntimeCall("_glispIsOdd", n.Args, 1)
		case "pos?":
			return e.emitRuntimeCall("_glispIsPos", n.Args, 1)
		case "neg?":
			return e.emitRuntimeCall("_glispIsNeg", n.Args, 1)
		case "zero?":
			return e.emitRuntimeCall("_glispIsZero", n.Args, 1)
		case "inc":
			return e.emitRuntimeCall("_glispInc", n.Args, 1)
		case "dec":
			return e.emitRuntimeCall("_glispDec", n.Args, 1)
		// sort conveniences
		case "sort":
			e.needImport("sort")
			return e.emitRuntimeCall("_glispSort", n.Args, 1)
		case "min-key":
			e.needImport("sort")
			return e.emitVariadicRuntimeCall("_glispMinKey", n.Args)
		case "max-key":
			e.needImport("sort")
			return e.emitVariadicRuntimeCall("_glispMaxKey", n.Args)
		case "max":
			return e.emitVariadicRuntimeCall("_glispMax", n.Args)
		case "min":
			return e.emitVariadicRuntimeCall("_glispMin", n.Args)
		case "max-by":
			return e.emitRuntimeCall("_glispMaxBy", n.Args, 2)
		case "min-by":
			return e.emitRuntimeCall("_glispMinBy", n.Args, 2)
		case "set":
			e.needImport("_set")
			return e.emitRuntimeCall("_glispToSet", n.Args, 1)
		// map conveniences
		case "map-vals":
			e.needImport("data")
			return e.emitRuntimeCall("_glispMapVals", n.Args, 2)
		case "map-keys":
			e.needImport("data")
			return e.emitRuntimeCall("_glispMapKeys", n.Args, 2)
		case "reduce-kv":
			e.needImport("data")
			return e.emitRuntimeCall("_glispReduceKV", n.Args, 3)
		// 2c: higher-order utilities
		case "complement":
			return e.emitRuntimeCall("_glispComplement", n.Args, 1)
		case "fnil":
			return e.emitRuntimeCall("_glispFnil", n.Args, 2)
		case "identity":
			if len(n.Args) != 1 {
				return fmt.Errorf("identity requires exactly 1 argument")
			}
			return e.emitExpr(n.Args[0])
		case "constantly":
			return e.emitRuntimeCall("_glispConstantly", n.Args, 1)
		case "comp":
			return e.emitVariadicRuntimeCall("_glispComp", n.Args)
		case "juxt":
			return e.emitVariadicRuntimeCall("_glispJuxt", n.Args)
		case "apply":
			return e.emitRuntimeCall("_glispApply", n.Args, 2)
		case "partial":
			if len(n.Args) < 1 {
				return fmt.Errorf("partial requires at least 1 argument")
			}
			return e.emitVariadicRuntimeCall("_glispPartial", n.Args)
		// 2b: string operations
		case "upper-case":
			return e.emitStrOp("strings.ToUpper", n.Args, 1)
		case "lower-case":
			return e.emitStrOp("strings.ToLower", n.Args, 1)
		case "trim":
			return e.emitStrOp("strings.TrimSpace", n.Args, 1)
		case "starts-with?":
			return e.emitStrOp2("strings.HasPrefix", n.Args)
		case "ends-with?":
			return e.emitStrOp2("strings.HasSuffix", n.Args)
		case "replace":
			return e.emitReplace(n.Args)
		case "split":
			e.needImport("_strruntime") // signals runtime to include string helpers; not a per-file import
			return e.emitRuntimeCall("_glispSplit", n.Args, 2)
		case "join":
			e.needImport("_strruntime")
			return e.emitRuntimeCall("_glispJoin", n.Args, 2)
		case "blank?":
			e.needImport("_strruntime")
			return e.emitRuntimeCall("_glispIsBlank", n.Args, 1)
		case "capitalize":
			e.needImport("_strruntime")
			return e.emitRuntimeCall("_glispCapitalize", n.Args, 1)
		case "panic":
			if len(n.Args) != 1 {
				return fmt.Errorf("panic: expected 1 argument, got %d at %s", len(n.Args), n.Pos())
			}
			e.write("panic(")
			if err := e.emitExpr(n.Args[0]); err != nil {
				return err
			}
			e.write(")")
			return nil
		case "recover":
			if len(n.Args) != 0 {
				return fmt.Errorf("recover: expected 0 arguments, got %d at %s", len(n.Args), n.Pos())
			}
			e.write("recover()")
			return nil
		case "assert":
			// Expression position: wrap the guard in an IIFE that yields nil so
			// (assert ...) is also usable as a value. Statement and return
			// positions emit the bare guard (see emitStmtNode / emitReturnNode).
			e.write("func() any { ")
			if err := e.emitAssertGuard(n); err != nil {
				return err
			}
			e.write("; return nil }()")
			return nil
		case "os/env":
			e.needImport("os")
			if e.emitRuntime {
				e.needImport("fmt")
			}
			if len(n.Args) == 1 {
				return e.emitRuntimeCall("_glispEnv", n.Args, 1)
			} else if len(n.Args) == 2 {
				return e.emitRuntimeCall("_glispEnvDefault", n.Args, 2)
			}
			return fmt.Errorf("os/env requires 1 or 2 arguments, got %d", len(n.Args))
		case "json/encode":
			e.needImport("encoding/json")
			return e.emitRuntimeCall("_glispJsonEncode", n.Args, 1)
		case "json/decode":
			e.needImport("encoding/json")
			return e.emitRuntimeCall("_glispJsonDecode", n.Args, 1)
		case "http/get":
			e.needImport("net/http")
			if e.emitRuntime {
				e.needImport("io")
				e.needImport("strings")
				e.needImport("fmt")
			}
			if len(n.Args) == 1 {
				return e.emitRuntimeCall("_glispHttpGet", n.Args, 1)
			}
			return e.emitRuntimeCall("_glispHttpGetH", n.Args, 2)
		case "http/post":
			e.needImport("net/http")
			if e.emitRuntime {
				e.needImport("io")
				e.needImport("strings")
				e.needImport("fmt")
			}
			if len(n.Args) == 2 {
				return e.emitRuntimeCall("_glispHttpPost", n.Args, 2)
			}
			return e.emitRuntimeCall("_glispHttpPostH", n.Args, 3)
		case "http/put":
			e.needImport("net/http")
			if e.emitRuntime {
				e.needImport("io")
				e.needImport("strings")
				e.needImport("fmt")
			}
			if len(n.Args) == 2 {
				return e.emitRuntimeCall("_glispHttpPut", n.Args, 2)
			}
			return e.emitRuntimeCall("_glispHttpPutH", n.Args, 3)
		case "http/delete":
			e.needImport("net/http")
			if e.emitRuntime {
				e.needImport("io")
				e.needImport("strings")
				e.needImport("fmt")
			}
			return e.emitRuntimeCall("_glispHttpDelete", n.Args, 1)
		case "http/request":
			e.needImport("net/http")
			if e.emitRuntime {
				e.needImport("io")
				e.needImport("strings")
				e.needImport("fmt")
			}
			return e.emitRuntimeCall("_glispHttpRequest", n.Args, 1)
		case "subs":
			return e.emitSubs(n.Args)
		case "format":
			return e.emitFormat(n.Args)
		case "parse-int":
			return e.emitParseInt(n.Args)
		case "parse-float":
			return e.emitParseFloat(n.Args)
		case "repeat":
			return e.emitRuntimeCall("_glispRepeat", n.Args, 2)
		case "interpose":
			return e.emitRuntimeCall("_glispInterpose", n.Args, 2)
		// 7d: set algebra
		case "union":
			e.needImport("_set")
			return e.emitRuntimeCall("_glispSetUnion", n.Args, 2)
		case "intersection":
			e.needImport("_set")
			return e.emitRuntimeCall("_glispSetIntersection", n.Args, 2)
		case "difference":
			e.needImport("_set")
			return e.emitRuntimeCall("_glispSetDifference", n.Args, 2)
		// File I/O
		case "read-file":
			e.needImport("_file")
			return e.emitRuntimeCall("_glispReadFile", n.Args, 1)
		case "write-file":
			e.needImport("_file")
			return e.emitRuntimeCall("_glispWriteFile", n.Args, 2)
		case "append-file":
			e.needImport("_file")
			return e.emitRuntimeCall("_glispAppendFile", n.Args, 2)
		case "file-exists?":
			e.needImport("_file")
			return e.emitRuntimeCall("_glispFileExists", n.Args, 1)
		case "list-dir":
			e.needImport("_file")
			return e.emitRuntimeCall("_glispListDir", n.Args, 1)
		case "mkdir":
			e.needImport("_file")
			return e.emitRuntimeCall("_glispMkdir", n.Args, 1)
		// Regex
		case "re/match":
			e.needImport("regexp")
			return e.emitRuntimeCall("_glispReMatch", n.Args, 2)
		case "re/find":
			e.needImport("regexp")
			return e.emitRuntimeCall("_glispReFind", n.Args, 2)
		case "re/find-all":
			e.needImport("regexp")
			return e.emitRuntimeCall("_glispReFindAll", n.Args, 2)
		case "re/replace":
			e.needImport("regexp")
			return e.emitRuntimeCall("_glispReReplace", n.Args, 3)
		case "re/split":
			e.needImport("regexp")
			return e.emitRuntimeCall("_glispReSplit", n.Args, 2)
		// Structured logging (log/slog) — void in Go, IIFE wrapper in expression position
		case "log/info", "log/debug", "log/warn", "log/error":
			e.write("func() any { ")
			if err := e.emitSlogCall(sym.Name, n.Args); err != nil {
				return err
			}
			e.write("; return nil }()")
			return nil
		// Error wrapping
		case "wrap-error":
			if len(n.Args) != 2 {
				return fmt.Errorf("wrap-error requires 2 arguments (msg err), got %d", len(n.Args))
			}
			e.needImport("fmt")
			e.write("fmt.Errorf(\"%s: %w\", ")
			if err := e.emitExpr(n.Args[0]); err != nil {
				return err
			}
			e.write(", ")
			if err := e.emitExpr(n.Args[1]); err != nil {
				return err
			}
			e.write(")")
			return nil
		case "errors/is?":
			e.needImport("errors")
			return e.emitRuntimeCall("errors.Is", n.Args, 2)
		// atom — shared mutable state ((atom …) parses to *ast.AtomExpr; only the
		// mutators remain symbol-dispatched call forms).
		case "swap!":
			e.needImport("_atom")
			if len(n.Args) != 2 {
				return fmt.Errorf("swap! requires 2 arguments (atom f), got %d", len(n.Args))
			}
			return e.emitRuntimeCall("_glispAtomSwap", n.Args, 2)
		case "reset!":
			e.needImport("_atom")
			if len(n.Args) != 2 {
				return fmt.Errorf("reset! requires 2 arguments (atom value), got %d", len(n.Args))
			}
			return e.emitRuntimeCall("_glispAtomReset", n.Args, 2)
		case "deref":
			e.needImport("_atom")
			if len(n.Args) != 1 {
				return fmt.Errorf("deref requires 1 argument, got %d", len(n.Args))
			}
			// Typed atom: coerce the `any` result to the declared scalar element
			// type so e.g. (deref c) on (Atom int) yields int without (int …).
			if elem, ok := e.atomTypeOfExpr(n.Args[0]); ok {
				if pre, post, ok := derefCoercion(elem); ok {
					e.write(pre)
					e.write("_glispAtomDeref(")
					if err := e.emitExpr(n.Args[0]); err != nil {
						return err
					}
					e.write(")")
					e.write(post)
					return nil
				}
			}
			return e.emitRuntimeCall("_glispAtomDeref", n.Args, 1)
		// Context propagation
		case "ctx/background":
			e.needImport("context")
			if len(n.Args) != 0 {
				return fmt.Errorf("ctx/background: expected 0 arguments, got %d", len(n.Args))
			}
			e.write("context.Background()")
			return nil
		case "ctx/todo":
			e.needImport("context")
			if len(n.Args) != 0 {
				return fmt.Errorf("ctx/todo: expected 0 arguments, got %d", len(n.Args))
			}
			e.write("context.TODO()")
			return nil
		case "ctx/with-cancel":
			e.needImport("_ctx")
			if len(n.Args) != 1 {
				return fmt.Errorf("ctx/with-cancel: expected 1 argument, got %d", len(n.Args))
			}
			return e.emitRuntimeCall("_glispCtxWithCancel", n.Args, 1)
		case "ctx/with-timeout":
			e.needImport("_ctx")
			if len(n.Args) != 2 {
				return fmt.Errorf("ctx/with-timeout: expected 2 arguments (ctx ms), got %d", len(n.Args))
			}
			return e.emitRuntimeCall("_glispCtxWithTimeout", n.Args, 2)
		case "ctx/cancel!":
			e.needImport("_ctx")
			if len(n.Args) != 1 {
				return fmt.Errorf("ctx/cancel!: expected 1 argument, got %d", len(n.Args))
			}
			return e.emitRuntimeCall("_glispCtxCancel", n.Args, 1)
		case "ctx/value":
			e.needImport("_ctx")
			if len(n.Args) != 2 {
				return fmt.Errorf("ctx/value: expected 2 arguments (ctx key), got %d", len(n.Args))
			}
			return e.emitRuntimeCall("_glispCtxValue", n.Args, 2)
		case "ctx/with-value":
			e.needImport("_ctx")
			if len(n.Args) != 3 {
				return fmt.Errorf("ctx/with-value: expected 3 arguments (ctx key val), got %d", len(n.Args))
			}
			return e.emitRuntimeCall("_glispCtxWithValue", n.Args, 3)
		case "ctx/done?":
			e.needImport("_ctx")
			if len(n.Args) != 1 {
				return fmt.Errorf("ctx/done?: expected 1 argument, got %d", len(n.Args))
			}
			return e.emitRuntimeCall("_glispCtxDone", n.Args, 1)
		case "ctx/err":
			e.needImport("_ctx")
			if len(n.Args) != 1 {
				return fmt.Errorf("ctx/err: expected 1 argument, got %d", len(n.Args))
			}
			return e.emitRuntimeCall("_glispCtxErr", n.Args, 1)
		}
	}

	// (:key coll) → _glispGet(coll, "key")
	// (:key coll default) → _glispGetD(coll, "key", default)
	if kw, ok := n.Head.(*ast.KeywordLit); ok {
		if len(n.Args) < 1 || len(n.Args) > 2 {
			return fmt.Errorf("keyword call requires 1 or 2 arguments")
		}
		// Typed field access: when the single argument is a variable statically
		// known to hold a declared struct, emit direct field access (x.Field)
		// instead of the untyped runtime lookup. A keyword that names no field is
		// a compile-time error — the struct cannot also be used as a map.
		if len(n.Args) == 1 {
			if sym, ok := n.Args[0].(*ast.Symbol); ok && e.localTypes != nil {
				if typeName, found := e.localTypes[sym.Name]; found {
					if si := e.structs[typeName]; si != nil {
						goField, ok := si.fields[kw.Value]
						if !ok {
							return fmt.Errorf("%s has no field :%s (at %s)", typeName, kw.Value, n.Pos())
						}
						e.writef("%s.%s", identToGo(sym.Name), goField)
						return nil
					}
					// External Go struct: (:scheme u) → u.Scheme against the
					// loaded field set, uniform with locally-declared structs.
					if fs := e.goFieldSet(typeName); fs != nil {
						goField := fnToGo(kw.Value)
						if _, ok := fs[goField]; !ok {
							if _, ok := fs[kw.Value]; ok {
								goField = kw.Value
							} else {
								return fmt.Errorf("type %s has no exported field :%s (at %s)", typeName, kw.Value, n.Pos())
							}
						}
						e.writef("%s.%s", identToGo(sym.Name), goField)
						return nil
					}
				}
			}
		}
		fn := "_glispGet"
		if len(n.Args) == 2 {
			fn = "_glispGetD"
		}
		e.writef("%s(", fn)
		if err := e.emitExpr(n.Args[0]); err != nil {
			return err
		}
		e.writef(", %q", kw.Value)
		if len(n.Args) == 2 {
			e.write(", ")
			if err := e.emitExpr(n.Args[1]); err != nil {
				return err
			}
		}
		e.write(")")
		return nil
	}

	// Arity check for user-defined function calls.
	if sym, ok := n.Head.(*ast.Symbol); ok && len(e.symbols) > 0 {
		if sig, found := e.symbols[sym.Name]; found {
			leading, _, hasSpread, serr := spreadArgs(n.Args)
			if serr != nil {
				return serr
			}
			switch {
			case hasSpread && !sig.variadic:
				// Spreading into a fixed-arity fn can't be right — catch it at
				// transpile time rather than emitting invalid Go.
				return fmt.Errorf("%s is not variadic — cannot spread arguments with & (at %s)", sym.Name, sym.Pos())
			case hasSpread:
				// The spread fills the variadic tail; the leading args must still
				// cover the required (non-variadic) parameters.
				if len(leading) < sig.minArity {
					return fmt.Errorf("arity error: %s called with %d arg(s) before & spread, expected at least %d (at %s)", sym.Name, len(leading), sig.minArity, n.Pos())
				}
			case sig.variadic:
				if len(n.Args) < sig.minArity {
					return fmt.Errorf("arity error: %s called with %d arg(s), expected at least %d (at %s)", sym.Name, len(n.Args), sig.minArity, n.Pos())
				}
			default:
				if len(n.Args) != sig.minArity {
					return fmt.Errorf("arity error: %s called with %d arg(s), expected %d (at %s)", sym.Name, len(n.Args), sig.minArity, n.Pos())
				}
			}
		}
	}

	// Dot-free method dispatch: (area s) → s.Area() when s is statically known
	// to hold a declared struct or interface type with a matching method and the
	// head names no built-in, user function, or in-scope binding.
	if info, ok := e.resolveMethodCall(n); ok {
		return e.emitMethodCall(n, info)
	}
	// A dot-free method spelling on an external-typed receiver that names no
	// method of that Go type is a typo, not a free call — flag it (Phase 12e).
	if err := e.checkExternalMethodTypo(n); err != nil {
		return err
	}

	// General function call: f(args...)
	// A trailing `& xs` spreads a slice into a Go variadic parameter:
	// (f a b & xs) → f(a, b, xs...). This is the glisp spelling for Go's
	// variadic-spread call, replacing the hand-written bridge.go that wrapping a
	// variadic Go API (pgx.Query(ctx, sql, args...), fmt.Errorf) used to need.
	leading, spread, hasSpread, err := spreadArgs(n.Args)
	if err != nil {
		return err
	}
	// When the callee is a known user function, thread each parameter's type to
	// its argument so struct-typed params accept plain map literals.
	var paramTypes []string
	if sym, ok := n.Head.(*ast.Symbol); ok {
		if e.symbols != nil {
			if sig, found := e.symbols[sym.Name]; found {
				paramTypes = sig.paramTypes
			}
		}
		// A known stdlib numeric function (e.g. math/abs → math.Abs, a
		// func(float64) float64) gets its param types so an `any`-typed argument
		// is coerced (_glispToFloat64) instead of producing a raw Go
		// "cannot use … (any) as float64" error. Only fills in when the head is
		// not a user fn — those win above.
		if paramTypes == nil {
			paramTypes = stdlibNumericParams[sym.Name]
		}
		// A loaded imported Go function (ADR-015) supplies its real parameter
		// types, so an `any` argument is coerced/asserted at the call site
		// (strings.ToUpper(_glispToString(x)), …) — generalizing the math-only
		// table above to every loaded package. Variadic-aware: trailing args get
		// the element type, not the []T slice type. The same loaded signature
		// drives the spread and arity diagnostics below.
		if fn, found := e.lookupGoCall(sym.Name); found {
			if paramTypes == nil {
				paramTypes = paramHintsFor(fn, len(leading))
			}
			// Spreading into an imported Go function whose loaded signature is not
			// variadic can't be right — catch it at transpile time (ADR-011 rule 3)
			// rather than emitting invalid Go. Unloaded packages trust the marker.
			if hasSpread && !fn.variadic {
				return fmt.Errorf("%s is not variadic — cannot spread arguments with & (at %s)", sym.Name, sym.Pos())
			}
			// Arity diagnostics (Phase 15 / ADR-015 12e): a wrong-arity interop
			// call becomes a clean position-tagged glisp error instead of an
			// opaque Go compile error. Only loaded packages are gated; an unloaded
			// one degrades to untyped emission and the Go compiler reports it.
			if err := checkGoCallArity(sym.Name, fn, len(leading), hasSpread, sym.Pos()); err != nil {
				return err
			}
		}
	}
	if err := e.emitExpr(n.Head); err != nil {
		return err
	}
	e.write("(")
	for i, arg := range leading {
		if i > 0 {
			e.write(", ")
		}
		if i < len(paramTypes) && paramTypes[i] != "" {
			if err := e.emitExprWithHint(arg, paramTypes[i]); err != nil {
				return err
			}
		} else if err := e.emitExpr(arg); err != nil {
			return err
		}
	}
	if hasSpread {
		if len(leading) > 0 {
			e.write(", ")
		}
		if err := e.emitExpr(spread); err != nil {
			return err
		}
		e.write("...")
	}
	e.write(")")
	return nil
}

// checkGoCallArity validates the argument count of a call to a loaded Go
// function (Phase 15 / ADR-015 12e). leading is the count of non-spread
// arguments; hasSpread is true when a trailing `& xs` supplies the variadic
// tail. It returns a position-tagged glisp error on mismatch so interop arity
// mistakes surface in the .glsp source rather than as opaque Go compile errors.
// Methods are never in the loaded index, so only package-level calls are gated.
func checkGoCallArity(name string, fn goFunc, leading int, hasSpread bool, pos ast.Position) error {
	if fn.variadic {
		fixed := len(fn.params) - 1 // last param is the variadic []T slice
		if hasSpread {
			// The spread is the whole variadic tail; Go forbids mixing it with
			// explicit variadic args, so the non-spread args must be exactly the
			// fixed params.
			if leading != fixed {
				return fmt.Errorf("arity error: %s called with %d arg(s) before & spread, expected exactly %d (at %s)", name, leading, fixed, pos)
			}
			return nil
		}
		if leading < fixed {
			return fmt.Errorf("arity error: %s called with %d arg(s), expected at least %d (at %s)", name, leading, fixed, pos)
		}
		return nil
	}
	// Fixed-arity: spreading into it is already rejected as "not variadic".
	if hasSpread {
		return nil
	}
	if leading != len(fn.params) {
		return fmt.Errorf("arity error: %s called with %d arg(s), expected %d (at %s)", name, leading, len(fn.params), pos)
	}
	return nil
}

// spreadArgs splits a call's arguments around an optional `& xs` spread marker.
// A bare `&` symbol marks the next (and final) argument for Go variadic
// spreading: (f a b & xs) → leading=[a b], spread=xs, ok=true. With no `&`
// present, ok is false and leading is the unchanged argument list. A misplaced
// or duplicated marker is a position-tagged error.
func spreadArgs(args []ast.Node) (leading []ast.Node, spread ast.Node, ok bool, err error) {
	amp := -1
	for i, a := range args {
		if s, isSym := a.(*ast.Symbol); isSym && s.Name == "&" {
			if amp >= 0 {
				return nil, nil, false, fmt.Errorf("only one spread marker & is allowed in a call (at %s)", s.Pos())
			}
			amp = i
		}
	}
	if amp < 0 {
		return args, nil, false, nil
	}
	ampSym := args[amp].(*ast.Symbol)
	if amp != len(args)-2 {
		return nil, nil, false, fmt.Errorf("spread marker & must be followed by exactly one argument, e.g. (f a & xs) (at %s)", ampSym.Pos())
	}
	return args[:amp], args[amp+1], true, nil
}

var arithHelpers = map[string]string{
	"+": "_glispAdd", "-": "_glispSub", "*": "_glispMul", "/": "_glispDiv", "mod": "_glispMod",
}

// stdlibNumericParams maps a stdlib-qualified call form to the Go parameter
// types its arguments should be coerced toward. Used by the general call path so
// an `any`-typed argument (map lookup, untyped param) at a known numeric stdlib
// call site is smart-converted (via emitExprWithHint → numericCoercion) instead
// of emitting an uncompilable Go assignment. Concrete-typed args are unaffected
// (the coercion only fires for statically-`any` values). The `math` package is
// all-float64; covering it removes the documented `(math/abs any-expr)` shim.
var stdlibNumericParams = map[string][]string{
	// func(float64) float64
	"math/abs":   {"float64"},
	"math/sqrt":  {"float64"},
	"math/cbrt":  {"float64"},
	"math/floor": {"float64"},
	"math/ceil":  {"float64"},
	"math/round": {"float64"},
	"math/trunc": {"float64"},
	"math/exp":   {"float64"},
	"math/exp2":  {"float64"},
	"math/log":   {"float64"},
	"math/log2":  {"float64"},
	"math/log10": {"float64"},
	"math/sin":   {"float64"},
	"math/cos":   {"float64"},
	"math/tan":   {"float64"},
	"math/asin":  {"float64"},
	"math/acos":  {"float64"},
	"math/atan":  {"float64"},
	"math/sinh":  {"float64"},
	"math/cosh":  {"float64"},
	"math/tanh":  {"float64"},
	// func(float64, float64) float64
	"math/pow":       {"float64", "float64"},
	"math/atan2":     {"float64", "float64"},
	"math/hypot":     {"float64", "float64"},
	"math/mod":       {"float64", "float64"},
	"math/max":       {"float64", "float64"},
	"math/min":       {"float64", "float64"},
	"math/copysign":  {"float64", "float64"},
	"math/dim":       {"float64", "float64"},
	"math/remainder": {"float64", "float64"},
}

func (e *Emitter) emitArith(op string, args []ast.Node) error {
	// Numeric auto-coercion: when any operand is statically Go `any` (map/slice
	// lookups, untyped params, range loop vars), native Go arithmetic won't
	// type-check (`any + int`). Route through a runtime helper that coerces each
	// operand to int64/float64 (preserving integer-ness when no float is present).
	if e.anyOperand(args) {
		e.needImport("_num")
		helper := arithHelpers[op]
		if op == "-" && len(args) == 1 {
			// unary minus → 0 - x
			e.writef("%s(int64(0), ", helper)
			if err := e.emitExpr(args[0]); err != nil {
				return err
			}
			e.write(")")
			return nil
		}
		e.writef("%s(", helper)
		for i, arg := range args {
			if i > 0 {
				e.write(", ")
			}
			if err := e.emitExpr(arg); err != nil {
				return err
			}
		}
		e.write(")")
		return nil
	}
	// Auto-promote across concrete numeric types: Go has no implicit int↔float64
	// conversion, so `(/ int-var 2.0)` / `(+ int-var float-var)` won't compile.
	// When a form mixes a concrete int and a concrete float operand, wrap each
	// int operand in float64(...). Skipped for `mod` — `%` is integer-only, so a
	// float operand there is a genuine error left for the Go compiler to report.
	promote := op != "mod" && e.mixesIntFloat(args)
	if op == "mod" {
		op = "%"
	}
	if len(args) == 1 {
		// unary minus
		e.write("(-")
		if err := e.emitExpr(args[0]); err != nil {
			return err
		}
		e.write(")")
		return nil
	}
	e.write("(")
	for i, arg := range args {
		if i > 0 {
			e.writef(" %s ", op)
		}
		if err := e.emitPromotedOperand(arg, promote); err != nil {
			return err
		}
	}
	e.write(")")
	return nil
}

// cmpHelpers maps a numeric comparison operator to its coercing runtime helper.
var cmpHelpers = map[string]string{
	"<": "_glispLt", ">": "_glispGt", "<=": "_glispLe", ">=": "_glispGe",
}

func (e *Emitter) emitBinOp(op string, args []ast.Node) error {
	if len(args) != 2 {
		return fmt.Errorf("%s requires exactly 2 arguments", op)
	}
	// Numeric auto-coercion for ordering comparisons: native `any < int` is a Go
	// compile error, so route through a helper that coerces both sides to float64.
	// `==`/`!=` are left native — interface comparison compiles as-is.
	if helper, ok := cmpHelpers[op]; ok && e.anyOperand(args) {
		e.needImport("_num")
		e.writef("%s(", helper)
		if err := e.emitExpr(args[0]); err != nil {
			return err
		}
		e.write(", ")
		if err := e.emitExpr(args[1]); err != nil {
			return err
		}
		e.write(")")
		return nil
	}
	// Ordering comparisons mixing a concrete int and float operand need the same
	// int→float64 promotion as arithmetic (`int-var < float-var` won't compile).
	// `==`/`!=` stay native interface comparisons (handled by their own callers).
	promote := cmpHelpers[op] != "" && e.mixesIntFloat(args)
	e.write("(")
	if err := e.emitPromotedOperand(args[0], promote); err != nil {
		return err
	}
	e.writef(" %s ", op)
	if err := e.emitPromotedOperand(args[1], promote); err != nil {
		return err
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitLogicOp(op string, args []ast.Node) error {
	if len(args) < 2 {
		return fmt.Errorf("logical operator requires at least 2 arguments")
	}
	e.write("(")
	for i, arg := range args {
		if i > 0 {
			e.writef(" %s ", op)
		}
		if err := e.emitCondition(arg); err != nil {
			return err
		}
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitStr(args []ast.Node) error {
	if len(args) == 0 {
		e.write(`""`)
		return nil
	}
	e.needImport("fmt")
	e.write("(")
	for i, arg := range args {
		if i > 0 {
			e.write(" + ")
		}
		e.write("fmt.Sprintf(\"%v\", ")
		if err := e.emitExpr(arg); err != nil {
			return err
		}
		e.write(")")
	}
	e.write(")")
	return nil
}

// emitSlogCall emits a raw slog.Info/Warn/Error/Debug call (no IIFE, no return).
// Used in statement and return position.
func (e *Emitter) emitSlogCall(fn string, args []ast.Node) error {
	e.needImport("log/slog")
	if len(args) < 1 {
		return fmt.Errorf("%s requires at least 1 argument (message)", fn)
	}
	goFn := map[string]string{
		"log/info":  "slog.Info",
		"log/debug": "slog.Debug",
		"log/warn":  "slog.Warn",
		"log/error": "slog.Error",
	}[fn]
	e.write(goFn + "(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	for _, arg := range args[1:] {
		e.write(", ")
		if err := e.emitExpr(arg); err != nil {
			return err
		}
	}
	e.write(")")
	return nil
}

// emitFmtPrintCall emits a raw fmt.Println/fmt.Print call (no IIFE, no return).
// Used in statement and return position.
func (e *Emitter) emitFmtPrintCall(fn string, args []ast.Node) error {
	e.needImport("fmt")
	goFn := "fmt.Println"
	if fn == "fmt/print" || fn == "print" {
		goFn = "fmt.Print"
	}
	e.writef("%s(", goFn)
	for i, arg := range args {
		if i > 0 {
			e.write(", ")
		}
		if err := e.emitExpr(arg); err != nil {
			return err
		}
	}
	e.write(")")
	return nil
}

// emitFmtPrint emits fmt.Println/fmt.Print wrapped in an IIFE returning nil,
// for use in expression position.
func (e *Emitter) emitFmtPrint(fn string, args []ast.Node) error {
	e.write("func() any { ")
	if err := e.emitFmtPrintCall(fn, args); err != nil {
		return err
	}
	e.write("; return nil }()")
	return nil
}

func (e *Emitter) emitGet(args []ast.Node) error {
	if len(args) < 2 || len(args) > 3 {
		return fmt.Errorf("get requires 2 or 3 arguments")
	}
	// Emit helper call: glispGet(m, key) or glispGetDefault(m, key, default)
	// For Phase 1 we inline the logic as a type-switch map lookup.
	// Simple version: m.(map[string]any)[key.(string)]
	// Better: emit a helper function call that we inject at the top of the file.
	// For now: use direct map indexing with a cast.
	if len(args) == 2 {
		e.write("_glispGet(")
		if err := e.emitExpr(args[0]); err != nil {
			return err
		}
		e.write(", ")
		if err := e.emitExpr(args[1]); err != nil {
			return err
		}
		e.write(")")
	} else {
		e.write("_glispGetD(")
		if err := e.emitExpr(args[0]); err != nil {
			return err
		}
		e.write(", ")
		if err := e.emitExpr(args[1]); err != nil {
			return err
		}
		e.write(", ")
		if err := e.emitExpr(args[2]); err != nil {
			return err
		}
		e.write(")")
	}
	return nil
}

func (e *Emitter) emitAssoc(args []ast.Node) error {
	if len(args) < 3 || len(args)%2 == 0 {
		return fmt.Errorf("assoc requires map + key-value pairs")
	}
	e.write("_glispAssoc(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	for i := 1; i < len(args); i += 2 {
		e.write(", ")
		if err := e.emitExpr(args[i]); err != nil {
			return err
		}
		e.write(", ")
		if err := e.emitExpr(args[i+1]); err != nil {
			return err
		}
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitDissoc(args []ast.Node) error {
	if len(args) < 2 {
		return fmt.Errorf("dissoc requires map + keys")
	}
	e.write("_glispDissoc(")
	for i, arg := range args {
		if i > 0 {
			e.write(", ")
		}
		if err := e.emitExpr(arg); err != nil {
			return err
		}
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitConj(args []ast.Node) error {
	if len(args) < 2 {
		return fmt.Errorf("conj requires collection + element(s)")
	}
	e.write("_glispConj(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	for _, arg := range args[1:] {
		e.write(", ")
		if err := e.emitExpr(arg); err != nil {
			return err
		}
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitCount(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("count requires 1 argument")
	}
	e.write("_glispLen(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitFirst(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("first requires 1 argument")
	}
	e.write("_glispFirst(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitRest(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("rest requires 1 argument")
	}
	e.write("_glispRest(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitNth(args []ast.Node) error {
	if len(args) != 2 {
		return fmt.Errorf("nth requires 2 arguments")
	}
	e.write("_glispNth(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(", ")
	if err := e.emitExpr(args[1]); err != nil {
		return err
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitKeys(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("keys requires 1 argument")
	}
	e.write("_glispKeys(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitVals(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("vals requires 1 argument")
	}
	e.write("_glispVals(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitMerge(args []ast.Node) error {
	if len(args) < 2 {
		return fmt.Errorf("merge requires at least 2 maps")
	}
	e.write("_glispMerge(")
	for i, arg := range args {
		if i > 0 {
			e.write(", ")
		}
		if err := e.emitExpr(arg); err != nil {
			return err
		}
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitError(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("error requires 1 argument (message string)")
	}
	e.needImport("errors")
	e.needImport("fmt")
	e.write("errors.New(fmt.Sprintf(\"%v\", ")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write("))")
	return nil
}

func (e *Emitter) emitNilQ(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("nil? requires 1 argument")
	}
	e.write("(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(" == nil)")
	return nil
}

func (e *Emitter) emitStringConv(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("string requires 1 argument")
	}
	// _glispToString, not a raw Go string(...) conversion: the raw form is a
	// compile error on any-typed values and the int→rune footgun on numbers.
	e.write("_glispToString(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitIntConv(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("int requires 1 argument")
	}
	// Use _glispToInt so it works on any (e.g. values from range/map/filter).
	e.write("_glispToInt(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitFloat64Conv(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("float64 requires 1 argument")
	}
	// Use _glispToFloat64 so it works on any (e.g. values from map/filter/nth).
	e.write("_glispToFloat64(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(")")
	return nil
}

// emitAsThread: (as-> x $ form1 form2 ...) threads x through each form with the
// previous result rebound to the named binding $. Emitted as an IIFE that
// assigns the binding step by step, so the thread position can vary per form.
func (e *Emitter) emitAsThread(args []ast.Node) error {
	if len(args) < 2 {
		return fmt.Errorf("as-> requires an initial value and a binding name (at %s)", args[0].Pos())
	}
	name, ok := args[1].(*ast.Symbol)
	if !ok {
		return fmt.Errorf("as-> binding must be a symbol, got %T (at %s)", args[1], args[1].Pos())
	}
	if err := e.checkMultiReturnValue(args[0]); err != nil {
		return err
	}
	saved := e.pushTypeScope()
	defer e.popTypeScope(saved)
	goName := identToGo(name.Name)
	e.registerAnyVar(name.Name) // binding holds an `any`-typed running value
	e.write("func() any {")
	e.nl()
	e.push()
	e.writeIndent()
	e.writef("var %s any = ", goName)
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.nl()
	for _, form := range args[2:] {
		e.writeIndent()
		e.writef("%s = ", goName)
		if err := e.emitExpr(form); err != nil {
			return err
		}
		e.nl()
	}
	e.writeIndent()
	e.writef("return %s\n", goName)
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// ppWrap wraps a node in a (pp node) call so tap->/tap->> pretty-print each
// intermediate value while threading the value itself through unchanged.
func ppWrap(n ast.Node) ast.Node {
	return ast.NewCallExpr(n.Pos(), ast.NewSymbol(n.Pos(), "pp"), []ast.Node{n})
}

// emitTapFirst: (tap-> x f1 f2 ...) is -> with each stage (incl. the initial
// value) wrapped in pp, so every intermediate value is pretty-printed. The final
// value is still returned.
func (e *Emitter) emitTapFirst(args []ast.Node) error {
	if len(args) < 2 {
		return fmt.Errorf("tap-> requires at least 2 forms")
	}
	node := ppWrap(args[0])
	for _, form := range args[1:] {
		switch f := form.(type) {
		case *ast.Symbol:
			node = ast.NewCallExpr(f.Pos_, f, []ast.Node{node})
		case *ast.CallExpr:
			newArgs := append([]ast.Node{node}, f.Args...)
			node = ast.NewCallExpr(f.Pos_, f.Head, newArgs)
		default:
			return fmt.Errorf("tap-> form must be a symbol or call, got %T", form)
		}
		node = ppWrap(node)
	}
	return e.emitExpr(node)
}

// emitTapLast: (tap->> x f1 f2 ...) is ->> with each stage wrapped in pp.
func (e *Emitter) emitTapLast(args []ast.Node) error {
	if len(args) < 2 {
		return fmt.Errorf("tap->> requires at least 2 forms")
	}
	node := ppWrap(args[0])
	for _, form := range args[1:] {
		switch f := form.(type) {
		case *ast.Symbol:
			node = ast.NewCallExpr(f.Pos_, f, []ast.Node{node})
		case *ast.CallExpr:
			newArgs := append(append([]ast.Node{}, f.Args...), node)
			node = ast.NewCallExpr(f.Pos_, f.Head, newArgs)
		default:
			return fmt.Errorf("tap->> form must be a symbol or call, got %T", form)
		}
		node = ppWrap(node)
	}
	return e.emitExpr(node)
}

// emitTimeIt: (time-it expr) evaluates expr, prints how long it took (tagged
// with the expression's source text), and returns the value unchanged.
func (e *Emitter) emitTimeIt(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("time-it requires 1 argument, got %d", len(args))
	}
	if err := e.checkMultiReturnValue(args[0]); err != nil {
		return err
	}
	e.needImport("time")
	e.needImport("fmt")
	start := e.fresh("start")
	val := e.fresh("v")
	src := formatter.FormatNode(args[0])
	e.writef("func() any { %s := time.Now(); %s := ", start, val)
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.writef("; fmt.Printf(\"%%s => %%v\\n\", %q, time.Since(%s)); return %s }()", src, start, val)
	return nil
}

// emitFor: (for [x coll y coll2 :when pred] body...) → a []any list
// comprehension. Multiple [name coll] bindings nest (cartesian product); a
// :when pred guards everything inside it. Emits an IIFE that builds and returns
// the result slice. The last body expr is the value collected per iteration;
// any earlier exprs run as side-effecting statements.
func (e *Emitter) emitFor(args []ast.Node) error {
	if len(args) < 2 {
		return fmt.Errorf("for requires a binding vector + body")
	}
	bv, ok := args[0].(*ast.VectorLit)
	if !ok || len(bv.Elements) < 2 {
		return fmt.Errorf("for binding must be [name collection ...]")
	}
	saved := e.pushTypeScope()
	defer e.popTypeScope(saved)

	e.write("func() []any {")
	e.nl()
	e.push()
	e.line("var _forResult []any")

	// Walk the binding vector, opening a range loop per [name coll] pair and an
	// if-guard per :when pred. Track the open-brace depth to close them after
	// the body in reverse.
	depth := 0
	els := bv.Elements
	for i := 0; i < len(els); {
		if kw, ok := els[i].(*ast.KeywordLit); ok {
			if kw.Value != "when" {
				return fmt.Errorf("for: unsupported modifier :%s (only :when is supported) (at %s)", kw.Value, kw.Pos())
			}
			if i+1 >= len(els) {
				return fmt.Errorf("for: :when requires a predicate (at %s)", kw.Pos())
			}
			e.writeIndent()
			e.write("if ")
			if err := e.emitCondition(els[i+1]); err != nil {
				return err
			}
			e.write(" {")
			e.nl()
			e.push()
			depth++
			i += 2
			continue
		}
		sym, ok := els[i].(*ast.Symbol)
		if !ok {
			return fmt.Errorf("for binding name must be a symbol (at %s)", els[i].Pos())
		}
		if i+1 >= len(els) {
			return fmt.Errorf("for: binding %s has no collection (at %s)", sym.Name, sym.Pos())
		}
		goName := identToGo(sym.Name)
		e.registerAnyVar(sym.Name) // range over _glispToSlice yields any
		e.writeIndent()
		if goName == "_" {
			e.write("for range _glispToSlice(")
		} else {
			e.writef("for _, %s := range _glispToSlice(", goName)
		}
		if err := e.emitExpr(els[i+1]); err != nil {
			return err
		}
		e.write(") {")
		e.nl()
		e.push()
		depth++
		i += 2
	}

	body := args[1:]
	for _, stmt := range body[:len(body)-1] {
		if err := e.emitStmtNode(stmt); err != nil {
			return err
		}
	}
	e.writeIndent()
	e.write("_forResult = append(_forResult, ")
	if err := e.emitExpr(body[len(body)-1]); err != nil {
		return err
	}
	e.write(")")
	e.nl()

	for ; depth > 0; depth-- {
		e.pop()
		e.line("}")
	}
	e.line("return _forResult")
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitDoseq: (doseq [item coll] body...) → for _, item := range coll { body }
func (e *Emitter) emitDoseq(args []ast.Node) error {
	if len(args) < 2 {
		return fmt.Errorf("doseq requires binding vector + body")
	}
	bv, ok := args[0].(*ast.VectorLit)
	if !ok || len(bv.Elements) != 2 {
		return fmt.Errorf("doseq binding must be [item collection]")
	}
	sym, ok := bv.Elements[0].(*ast.Symbol)
	if !ok {
		return fmt.Errorf("doseq binding name must be a symbol")
	}
	goName := identToGo(sym.Name)
	saved := e.pushTypeScope()
	defer e.popTypeScope(saved)
	e.registerAnyVar(sym.Name) // range over _glispToSlice yields any
	e.writef("func() {")
	e.nl()
	e.push()
	e.writeIndent()
	if goName == "_" {
		e.write("for range _glispToSlice(")
	} else {
		e.writef("for _, %s := range _glispToSlice(", goName)
	}
	if err := e.emitExpr(bv.Elements[1]); err != nil {
		return err
	}
	e.write(") {")
	e.nl()
	e.push()
	for _, stmt := range args[1:] {
		e.writeIndent()
		if err := e.emitExpr(stmt); err != nil {
			return err
		}
		e.nl()
	}
	e.pop()
	e.line("}")
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitDotimes: (dotimes [i n] body...) → for i := 0; i < n; i++ { body }
func (e *Emitter) emitDotimes(args []ast.Node) error {
	if len(args) < 2 {
		return fmt.Errorf("dotimes requires binding vector + body")
	}
	bv, ok := args[0].(*ast.VectorLit)
	if !ok || len(bv.Elements) != 2 {
		return fmt.Errorf("dotimes binding must be [i n]")
	}
	sym, ok := bv.Elements[0].(*ast.Symbol)
	if !ok {
		return fmt.Errorf("dotimes binding name must be a symbol")
	}
	goName := identToGo(sym.Name)
	// A `_` binding would emit illegal Go (`for _ := 0; _ < n`); substitute a
	// synthetic counter — it's used by the loop header, so Go accepts it.
	if goName == "_" {
		goName = "_dotimesI"
	}
	e.write("func() {")
	e.nl()
	e.push()
	e.writeIndent()
	e.writef("for %s := 0; %s < ", goName, goName)
	if err := e.emitExpr(bv.Elements[1]); err != nil {
		return err
	}
	e.writef("; %s++ {", goName)
	e.nl()
	e.push()
	for _, stmt := range args[1:] {
		e.writeIndent()
		if err := e.emitExpr(stmt); err != nil {
			return err
		}
		e.nl()
	}
	e.pop()
	e.line("}")
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitReturnExpr emits an explicit (return ...) form.
func (e *Emitter) emitReturnExpr(n *ast.ReturnExpr) error {
	if len(n.Args) == 0 {
		e.line("return")
		return nil
	}
	e.writeIndent()
	e.write("return ")
	for i, arg := range n.Args {
		if i > 0 {
			e.write(", ")
		}
		if err := e.emitExpr(arg); err != nil {
			return err
		}
	}
	e.nl()
	return nil
}

// emitRuntimeCall emits a call to a runtime helper with a fixed arity check.
// fnArgHelpers maps runtime helper names to the argument index that expects a
// function value (-1 = every argument, for comp/juxt). A bare keyword at such
// a position is lowered to a lookup closure, enabling the Clojure idiom
// (map :title coll) / (group-by :status users) without a lambda.
var fnArgHelpers = map[string]int{
	"_glispMap": 0, "_glispFilter": 0, "_glispSome": 0, "_glispEvery": 0,
	"_glispNotAny": 0, "_glispRemove": 0, "_glispKeep": 0, "_glispSortBy": 0,
	"_glispGroupBy": 0, "_glispPartitionBy": 0, "_glispMapcat": 0,
	"_glispTakeWhile": 0, "_glispDropWhile": 0, "_glispSplitWith": 0,
	"_glispMinKey": 0, "_glispMaxKey": 0, "_glispMinBy": 0, "_glispMaxBy": 0,
	"_glispMapVals": 0, "_glispMapKeys": 0,
	"_glispComplement": 0, "_glispFnil": 0, "_glispPartial": 0, "_glispApply": 0,
	"_glispComp": -1, "_glispJuxt": -1,
}

// emitRuntimeArg emits one argument of a runtime-helper call. In a function
// position it (a) lowers a bare keyword to a _glispGet lookup closure and
// (b) rejects user fns with typed signatures at transpile time — the runtime
// helpers assert func(any) any, so a func(int) int would panic at runtime
// with an opaque interface-conversion message.
func (e *Emitter) emitRuntimeArg(fn string, idx int, arg ast.Node) error {
	if fnPos, ok := fnArgHelpers[fn]; ok && (fnPos == -1 || fnPos == idx) {
		if kw, ok := arg.(*ast.KeywordLit); ok {
			e.writef("func(_kwM any) any { return _glispGet(_kwM, %q) }", kw.Value)
			return nil
		}
		if sym, ok := arg.(*ast.Symbol); ok && !e.localVars[sym.Name] {
			if sig, found := e.symbols[sym.Name]; found {
				if reason := hofIncompatibleReason(sig); reason != "" {
					return fmt.Errorf("%s has %s and cannot be passed as a function value (built-in higher-order forms require any-typed params and -> any); wrap it in a lambda like (fn [x] (%s x)) or declare it with any types (at %s)",
						sym.Name, reason, sym.Name, sym.Pos())
				}
			}
		}
	}
	return e.emitExpr(arg)
}

// hofIncompatibleReason returns a non-empty description when a user fn's
// signature can't satisfy the func(any) any assertion in the runtime helpers.
// Variadic fns are left to the runtime (apply handles func(...any) any).
func hofIncompatibleReason(sig *fnSig) string {
	if sig.variadic {
		return ""
	}
	for _, pt := range sig.paramTypes {
		if pt != "" && pt != "any" {
			return fmt.Sprintf("a typed param (%s)", pt)
		}
	}
	switch sig.retType {
	case "any":
		return ""
	case "":
		return "no declared return type (it emits a void Go func)"
	default:
		return fmt.Sprintf("return type %s", sig.retType)
	}
}

func (e *Emitter) emitRuntimeCall(fn string, args []ast.Node, arity int) error {
	if len(args) != arity {
		return fmt.Errorf("%s requires %d argument(s), got %d", fn, arity, len(args))
	}
	e.writef("%s(", fn)
	for i, arg := range args {
		if i > 0 {
			e.write(", ")
		}
		if err := e.emitRuntimeArg(fn, i, arg); err != nil {
			return err
		}
	}
	e.write(")")
	return nil
}

// emitVariadicRuntimeCall emits a call to a runtime helper accepting any number of args.
func (e *Emitter) emitVariadicRuntimeCall(fn string, args []ast.Node) error {
	e.writef("%s(", fn)
	for i, arg := range args {
		if i > 0 {
			e.write(", ")
		}
		if err := e.emitRuntimeArg(fn, i, arg); err != nil {
			return err
		}
	}
	e.write(")")
	return nil
}

// emitStrOp emits a single-arg strings.Xxx(_glispToString(s)) call.
func (e *Emitter) emitStrOp(fn string, args []ast.Node, arity int) error {
	if len(args) != arity {
		return fmt.Errorf("%s requires %d argument(s)", fn, arity)
	}
	e.needImport("strings")
	e.writef("%s(_glispToString(", fn)
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write("))")
	return nil
}

// emitStrOp2 emits a two-arg strings.Xxx(_glispToString(s), _glispToString(t)) call.
func (e *Emitter) emitStrOp2(fn string, args []ast.Node) error {
	if len(args) != 2 {
		return fmt.Errorf("%s requires 2 arguments", fn)
	}
	e.needImport("strings")
	e.writef("%s(_glispToString(", fn)
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write("), _glispToString(")
	if err := e.emitExpr(args[1]); err != nil {
		return err
	}
	e.write("))")
	return nil
}

// emitReplace emits (replace s from to) → strings.ReplaceAll(s, from, to)
func (e *Emitter) emitReplace(args []ast.Node) error {
	if len(args) != 3 {
		return fmt.Errorf("replace requires 3 arguments: s from to")
	}
	e.needImport("strings")
	e.write("strings.ReplaceAll(_glispToString(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write("), _glispToString(")
	if err := e.emitExpr(args[1]); err != nil {
		return err
	}
	e.write("), _glispToString(")
	if err := e.emitExpr(args[2]); err != nil {
		return err
	}
	e.write("))")
	return nil
}

// emitSubs emits (subs s start) or (subs s start end)
func (e *Emitter) emitSubs(args []ast.Node) error {
	if len(args) < 2 || len(args) > 3 {
		return fmt.Errorf("subs requires 2 or 3 arguments")
	}
	e.write("(_glispToString(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write("))[")
	if err := e.emitExpr(args[1]); err != nil {
		return err
	}
	if len(args) == 3 {
		e.write(":")
		if err := e.emitExpr(args[2]); err != nil {
			return err
		}
	} else {
		e.write(":")
	}
	e.write("]")
	return nil
}

// emitFormat emits (format fmt-str args...) → fmt.Sprintf(fmt-str, args...)
func (e *Emitter) emitFormat(args []ast.Node) error {
	if len(args) < 1 {
		return fmt.Errorf("format requires at least 1 argument")
	}
	e.needImport("fmt")
	e.write("fmt.Sprintf(")
	for i, arg := range args {
		if i > 0 {
			e.write(", ")
		}
		if err := e.emitExpr(arg); err != nil {
			return err
		}
	}
	e.write(")")
	return nil
}

// emitParseInt emits (parse-int s) → strconv.Atoi(_glispToString(s))
func (e *Emitter) emitParseInt(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("parse-int requires exactly 1 argument")
	}
	e.needImport("strconv")
	e.write("strconv.Atoi(_glispToString(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write("))")
	return nil
}

// emitParseFloat emits (parse-float s) → strconv.ParseFloat(_glispToString(s), 64)
func (e *Emitter) emitParseFloat(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("parse-float requires exactly 1 argument")
	}
	e.needImport("strconv")
	e.write("strconv.ParseFloat(_glispToString(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write("), 64)")
	return nil
}

// emitValuesExpr emits (values a b) inline (used inside return statements).
func (e *Emitter) emitValuesExpr(n *ast.ValuesExpr) error {
	for i, arg := range n.Args {
		if i > 0 {
			e.write(", ")
		}
		if err := e.emitExpr(arg); err != nil {
			return err
		}
	}
	e.nl()
	return nil
}
