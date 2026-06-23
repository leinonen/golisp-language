package transpiler

import (
	"fmt"
	"strings"

	"golisp/internal/ast"
)

// Dot-free method dispatch: (area s) emits s.Area() when s is a variable
// statically known to hold a declared struct or interface type with a matching
// method. This removes the need for the Go-interop (.Method obj) form on
// locally-declared types — the interop form remains available and is still
// required for types the transpiler cannot see (other files, wrapped Go
// packages, opaque any values).
//
// Resolution order at a call site (f a b): built-in forms win first (the
// emitCallExpr switch runs before dispatch is consulted), then user-defined
// top-level functions, then in-scope value bindings (params, let/loop
// bindings, def globals — a local closure named `area` shadows the method),
// and only then method dispatch. The method name converts like exported
// module calls (fnToGo): all-lowercase kebab → PascalCase (describe →
// Describe, to-string → ToString); a name with any uppercase passes through
// as-is.

// methodSet maps a Go method name to its signature.
type methodSet map[string]*fnSig

// collectMethodTables scans top-level declarations and returns the interface
// method table (interface name → methods), the receiver method table (bare
// struct name → methods from defmethod), and the set of def-bound global
// names (which shadow method dispatch like locals do).
func collectMethodTables(nodes []ast.Node) (ifaces, methods map[string]methodSet, defGlobals map[string]bool) {
	ifaces = map[string]methodSet{}
	methods = map[string]methodSet{}
	defGlobals = map[string]bool{}
	for _, n := range nodes {
		switch d := n.(type) {
		case *ast.InterfaceDecl:
			ms := methodSet{}
			for _, m := range d.Methods {
				// Interface method names are emitted verbatim (emitInterfaceDecl).
				ms[m.Name] = buildFnSig(m.Params, m.ReturnType)
			}
			ifaces[d.Name] = ms
		case *ast.MethodDecl:
			recv := strings.TrimPrefix(strings.TrimSpace(d.ReceiverType.Text), "*")
			if methods[recv] == nil {
				methods[recv] = methodSet{}
			}
			// defmethod names are emitted through identToGo (emitMethodDecl).
			methods[recv][identToGo(d.Name)] = buildFnSig(d.Params, d.ReturnType)
		case *ast.DefDecl:
			defGlobals[d.Name] = true
		}
	}
	return ifaces, methods, defGlobals
}

// namedTypeHint reports whether a Go type string names a locally-declared
// struct or interface (optionally behind a single pointer), returning the bare
// type name. Unlike structHint it also recognises interfaces, so variables of
// interface type participate in method dispatch.
func (e *Emitter) namedTypeHint(goType string) (string, bool) {
	goType = strings.TrimSpace(goType)
	goType = strings.TrimSpace(strings.TrimPrefix(goType, "*"))
	if _, ok := e.structs[goType]; ok {
		return goType, true
	}
	if _, ok := e.ifaces[goType]; ok {
		return goType, true
	}
	return "", false
}

// externalTypeHint reports whether a Go type string names a loaded external Go
// type with an exported method set (optionally behind a single pointer),
// returning its qualified type key (e.g. "*pgx.Conn" → "pgx.Conn"). This is the
// interop analog of namedTypeHint for types the transpiler reads from the Go
// toolchain rather than declares itself.
func (e *Emitter) externalTypeHint(goType string) (string, bool) {
	key := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(goType), "*"))
	if e.goPkgs.hasType(key) {
		return key, true
	}
	return "", false
}

// goMethodSet returns the loaded method set of an external Go type, or nil.
func (e *Emitter) goMethodSet(typeKey string) map[string]goFunc {
	return e.goPkgs.methodSet(typeKey)
}

// goFuncToSig adapts a loaded Go method signature to the *fnSig that method
// dispatch emission uses. Only the fixed parameters carry type hints; a variadic
// method's trailing args emit bare (they are `any`/interface in practice), and a
// multi-return method has an empty retType so it is not propagated as a value
// type (it needs if-err, like any multi-return interop).
func goFuncToSig(fn goFunc) *fnSig {
	fixed := len(fn.params)
	if fn.variadic && fixed > 0 {
		fixed--
	}
	pt := make([]string, fixed)
	copy(pt, fn.params[:fixed])
	return &fnSig{minArity: fixed, variadic: fn.variadic, paramTypes: pt, retType: fn.ret}
}

// methodSig looks up a method on a declared type: defmethod receivers first,
// then interface declarations.
func (e *Emitter) methodSig(typeName, goMethod string) (*fnSig, bool) {
	if ms := e.methods[typeName]; ms != nil {
		if sig, ok := ms[goMethod]; ok {
			return sig, true
		}
	}
	if ms := e.ifaces[typeName]; ms != nil {
		if sig, ok := ms[goMethod]; ok {
			return sig, true
		}
	}
	return nil, false
}

// methodCallInfo describes a resolved dot-free method call.
type methodCallInfo struct {
	recv     ast.Node // receiver expression (first call argument)
	method   string   // Go method name
	typeName string   // receiver's declared type (for error messages)
	sig      *fnSig
}

// resolveMethodCall reports whether n is a dot-free method call: an
// unqualified head symbol that is no built-in, user function, or in-scope
// value binding, whose first argument is statically known to hold a declared
// struct or interface type with a matching method. The receiver type comes
// from the local type environment for variables, and from value inference
// (struct literals, calls with declared return types) otherwise.
func (e *Emitter) resolveMethodCall(n *ast.CallExpr) (*methodCallInfo, bool) {
	sym, ok := n.Head.(*ast.Symbol)
	if !ok || len(n.Args) == 0 {
		return nil, false
	}
	name := sym.Name
	if name == "" || strings.ContainsRune(name, '/') {
		return nil, false
	}
	if _, builtin := builtinArity[name]; builtin {
		return nil, false
	}
	if boolBuiltins[name] {
		return nil, false
	}
	if _, userFn := e.symbols[name]; userFn {
		return nil, false
	}
	if e.localVars[name] || e.defGlobals[name] {
		return nil, false
	}
	recv := n.Args[0]
	var typeName string
	if rs, ok := recv.(*ast.Symbol); ok {
		typeName = e.localTypes[rs.Name]
	} else {
		typeName = e.inferValueStructType(recv)
	}
	if typeName == "" {
		return nil, false
	}
	// An external Go type (read from the toolchain) dispatches against its loaded
	// method set; its name carries a package qualifier ("pgx.Conn") that no
	// locally-declared type has, so the two tables never collide. A miss here
	// falls through — the missing-method diagnostic is raised at the call site.
	if mset := e.goMethodSet(typeName); mset != nil {
		goMethod := fnToGo(name)
		mfn, found := mset[goMethod]
		if !found {
			return nil, false
		}
		return &methodCallInfo{recv: recv, method: goMethod, typeName: typeName, sig: goFuncToSig(mfn)}, true
	}
	goMethod := fnToGo(name)
	sig, found := e.methodSig(typeName, goMethod)
	if !found {
		goMethod = identToGo(name)
		if sig, found = e.methodSig(typeName, goMethod); !found {
			return nil, false
		}
	}
	return &methodCallInfo{
		recv:     recv,
		method:   goMethod,
		typeName: typeName,
		sig:      sig,
	}, true
}

// checkExternalMethodTypo raises a position-tagged diagnostic when a call's head
// is a dot-free method spelling (no built-in, user fn, or in-scope binding) and
// its symbol receiver is statically known to hold an external Go type that has
// no such method — a typo that would otherwise emit an invalid free call and
// surface as an opaque Go error (ADR-015, Phase 12e). It fires only for symbol
// receivers with a known external type, so it never false-flags ordinary calls.
func (e *Emitter) checkExternalMethodTypo(n *ast.CallExpr) error {
	sym, ok := n.Head.(*ast.Symbol)
	if !ok || len(n.Args) == 0 {
		return nil
	}
	name := sym.Name
	if name == "" || strings.ContainsRune(name, '/') {
		return nil
	}
	if _, builtin := builtinArity[name]; builtin {
		return nil
	}
	if boolBuiltins[name] {
		return nil
	}
	if _, userFn := e.symbols[name]; userFn {
		return nil
	}
	if e.localVars[name] || e.defGlobals[name] {
		return nil
	}
	rs, ok := n.Args[0].(*ast.Symbol)
	if !ok {
		return nil
	}
	typeName := e.localTypes[rs.Name]
	if typeName == "" {
		return nil
	}
	mset := e.goMethodSet(typeName)
	if mset == nil {
		return nil
	}
	if _, found := mset[fnToGo(name)]; found {
		return nil
	}
	return fmt.Errorf("type %s has no exported method %s (at %s)", typeName, fnToGo(name), sym.Pos())
}

// emitMethodCall emits a resolved dot-free method call as recv.Method(args).
// Method parameter types are threaded to arguments as hints, like user
// function calls.
func (e *Emitter) emitMethodCall(n *ast.CallExpr, info *methodCallInfo) error {
	nargs := len(n.Args) - 1
	if info.sig.variadic {
		if nargs < info.sig.minArity {
			return fmt.Errorf("arity error: method %s on %s called with %d arg(s) after the receiver, expected at least %d (at %s)",
				info.method, info.typeName, nargs, info.sig.minArity, n.Pos())
		}
	} else if nargs != info.sig.minArity {
		return fmt.Errorf("arity error: method %s on %s called with %d arg(s) after the receiver, expected %d (at %s)",
			info.method, info.typeName, nargs, info.sig.minArity, n.Pos())
	}
	if err := e.emitExpr(info.recv); err != nil {
		return err
	}
	e.writef(".%s(", info.method)
	for i, arg := range n.Args[1:] {
		if i > 0 {
			e.write(", ")
		}
		if i < len(info.sig.paramTypes) && info.sig.paramTypes[i] != "" {
			if err := e.emitExprWithHint(arg, info.sig.paramTypes[i]); err != nil {
				return err
			}
		} else if err := e.emitExpr(arg); err != nil {
			return err
		}
	}
	e.write(")")
	return nil
}
