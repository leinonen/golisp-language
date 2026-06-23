package transpiler

import (
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// goFunc is a loaded, package-level Go function signature (ADR-015, Phase 12a).
// It records just enough for the transpiler to validate and type interop calls
// without the user writing Go: the per-parameter Go types, whether the final
// parameter is variadic, and the single return type (empty for void/multi).
type goFunc struct {
	params   []string // Go type per parameter; the last is []T when variadic
	variadic bool
	ret      string   // single return Go type; "" if none, void, or multi-return
	results  []string // every return Go type (len >= 2 marks a multi-return fn)
	sig      string   // full Go signature for hover, e.g. "func strings.ToUpper(s string) string"
}

// goPkgIndex maps a package qualifier (the call-site prefix in `pkg/fn`) to its
// exported package-level functions, keyed by Go function name (e.g. "Printf").
// Built by LoadGoPackages from the Go toolchain's own type information — jank's
// model (Clang for C++) with go/types for Go. A package that fails to load is
// simply absent, so callers degrade to untyped emission (never a hard failure).
type goPkgIndex map[string]map[string]goFunc

// goTypeIndex maps an exported named Go type — keyed by its qualified type
// string (e.g. "pgx.Conn", as Go renders it) — to its exported method set,
// keyed by Go method name. It backs dot-free method dispatch and missing-method
// diagnostics on interop values (ADR-015, Phase 12c/12e): a value statically
// known to hold such a type can call its methods without the `(.Method o)` form.
type goTypeIndex map[string]map[string]goFunc

// goPackages bundles the loaded interop signatures the transpiler consults:
// package-level functions (for `pkg/fn` calls) and the method sets of exported
// named types (for dot-free dispatch on interop values). nil when nothing
// loaded — every accessor is nil-safe so callers degrade to untyped emission.
type goPackages struct {
	funcs goPkgIndex
	types goTypeIndex
}

// lookupFunc resolves a package-level function; nil-safe.
func (p *goPackages) lookupFunc(qualifier, goName string) (goFunc, bool) {
	if p == nil {
		return goFunc{}, false
	}
	return p.funcs.lookup(qualifier, goName)
}

// methodSet returns the exported method set of a named Go type (keyed by its
// qualified type string, pointer stripped), or nil if unknown; nil-safe.
func (p *goPackages) methodSet(typeKey string) map[string]goFunc {
	if p == nil || p.types == nil {
		return nil
	}
	return p.types[typeKey]
}

// hasType reports whether typeKey names a loaded external type with methods.
func (p *goPackages) hasType(typeKey string) bool {
	return p.methodSet(typeKey) != nil
}

// lookup resolves a `pkg/fn` call against the loaded index. qualifier is the
// part before the slash; goName is the exported Go name (fnToGo of the part
// after the slash). ok is false when the package wasn't loaded or has no such
// function — the signal to fall back to untyped emission.
func (idx goPkgIndex) lookup(qualifier, goName string) (goFunc, bool) {
	if idx == nil {
		return goFunc{}, false
	}
	fns, ok := idx[qualifier]
	if !ok {
		return goFunc{}, false
	}
	fn, ok := fns[goName]
	return fn, ok
}

// lookupGoCall resolves a `pkg/fn` call symbol against the emitter's loaded Go
// package index, converting the part after the slash to its Go name the same
// way emission does (fnToGo: new-string → NewString). ok is false when the
// symbol is unqualified or the package/function wasn't loaded.
func (e *Emitter) lookupGoCall(name string) (goFunc, bool) {
	slash := strings.Index(name, "/")
	if slash <= 0 {
		return goFunc{}, false
	}
	return e.goPkgs.lookupFunc(name[:slash], fnToGo(name[slash+1:]))
}

// paramHintsFor returns a per-argument Go-type hint slice for a call with n
// arguments to fn. For a fixed-arity fn it is just the parameter types; for a
// variadic fn, arguments at or past the final (variadic) parameter get its
// element type (the `[]T` slice type with the leading `[]` stripped), so each
// individual trailing argument is coerced to T rather than to the slice.
func paramHintsFor(fn goFunc, n int) []string {
	if len(fn.params) == 0 || n == 0 {
		return nil
	}
	last := len(fn.params) - 1
	elem := ""
	if fn.variadic {
		elem = strings.TrimPrefix(fn.params[last], "[]")
	}
	hints := make([]string, n)
	for i := 0; i < n; i++ {
		switch {
		case fn.variadic && i >= last:
			hints[i] = elem
		case i <= last:
			hints[i] = fn.params[i]
		}
	}
	return hints
}

// LoadGoPackages loads exported function signatures for the given Go packages
// using the Go toolchain (go/packages with type information). paths maps each
// package qualifier (the `pkg` in `pkg/fn`) to its full import path; the result
// is keyed by that same qualifier. dir is the module directory the load runs in
// (so module-local replace directives and the project's go.mod apply).
//
// This never returns an error: any package that fails to load — offline, an
// unresolved dependency, a build error in the dependency — is omitted from the
// result, and its calls emit exactly as they do today (untyped). Typed interop
// is an enhancement layered on top, never a build dependency (ADR-015).
// buildGoFunc extracts a goFunc from a Go signature, rendering every type with
// qualifier. Shared by package-level function and named-type method extraction.
// The rendered type strings are only ever used as map keys and coercion hints —
// never written to the generated Go — so cross-package qualification (e.g.
// "context.Context") needs no import in the user's file.
func buildGoFunc(sig *types.Signature, obj types.Object, qualifier types.Qualifier) goFunc {
	params := sig.Params()
	ptypes := make([]string, params.Len())
	for i := 0; i < params.Len(); i++ {
		ptypes[i] = types.TypeString(params.At(i).Type(), qualifier)
	}
	res := sig.Results()
	results := make([]string, res.Len())
	for i := 0; i < res.Len(); i++ {
		results[i] = types.TypeString(res.At(i).Type(), qualifier)
	}
	ret := ""
	if len(results) == 1 {
		ret = results[0]
	}
	return goFunc{
		params:   ptypes,
		variadic: sig.Variadic(),
		ret:      ret,
		results:  results,
		sig:      types.ObjectString(obj, qualifier),
	}
}

func LoadGoPackages(dir string, paths map[string]string) *goPackages {
	if len(paths) == 0 {
		return nil
	}
	// Reverse map: import path → qualifier, so we can key the result by the
	// qualifier call sites actually use. Load all paths in one invocation.
	qualOf := make(map[string]string, len(paths))
	list := make([]string, 0, len(paths))
	for qual, path := range paths {
		qualOf[path] = qual
		list = append(list, path)
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes,
		Dir:  dir,
	}
	loaded, err := packages.Load(cfg, list...)
	if err != nil {
		return nil
	}

	idx := goPkgIndex{}
	tidx := goTypeIndex{}
	for _, p := range loaded {
		if len(p.Errors) > 0 || p.Types == nil {
			continue // degrade: this package stays untyped
		}
		qual := qualOf[p.PkgPath]
		if qual == "" {
			qual = p.Name
		}
		fns := map[string]goFunc{}
		scope := p.Types.Scope()
		qualifier := func(other *types.Package) string { return other.Name() }
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			if !obj.Exported() {
				continue
			}
			switch o := obj.(type) {
			case *types.Func:
				sig, ok := o.Type().(*types.Signature)
				if !ok || sig.Recv() != nil {
					continue // methods come from the named-type pass below
				}
				fns[name] = buildGoFunc(sig, o, qualifier)
			case *types.TypeName:
				named, ok := o.Type().(*types.Named)
				if !ok {
					continue
				}
				if set := methodSetOf(named, qualifier); len(set) > 0 {
					tidx[types.TypeString(named, qualifier)] = set
				}
			}
		}
		if len(fns) > 0 {
			idx[qual] = fns
		}
	}
	if len(idx) == 0 && len(tidx) == 0 {
		return nil
	}
	return &goPackages{funcs: idx, types: tidx}
}

// methodSetOf returns the exported method set of a named type keyed by Go method
// name. Structs use the pointer method set (a superset of the value set, so
// pointer- and value-typed receivers both resolve); interfaces use the type's
// own method set (a pointer-to-interface has none).
func methodSetOf(named *types.Named, qualifier types.Qualifier) map[string]goFunc {
	var ms *types.MethodSet
	if _, isIface := named.Underlying().(*types.Interface); isIface {
		ms = types.NewMethodSet(named)
	} else {
		ms = types.NewMethodSet(types.NewPointer(named))
	}
	set := map[string]goFunc{}
	for i := 0; i < ms.Len(); i++ {
		obj := ms.At(i).Obj()
		if !obj.Exported() {
			continue
		}
		fn, ok := obj.(*types.Func)
		if !ok {
			continue
		}
		sig, ok := fn.Type().(*types.Signature)
		if !ok {
			continue
		}
		set[fn.Name()] = buildGoFunc(sig, fn, qualifier)
	}
	return set
}

// GoSignatures is an exported, read-only view of loaded Go package signatures
// for editor tooling (the LSP — ADR-015 / Phase 12f). It wraps the unexported
// index so hover and completion can reuse the same loader the transpiler uses,
// without exposing its internals.
type GoSignatures struct {
	idx goPkgIndex
}

// GoCompletion is one completion candidate from an imported Go package: the
// exported function name and its full Go signature (for the item detail).
type GoCompletion struct {
	Name string
	Sig  string
}

// LoadGoSignatures loads the signatures of the given Go packages (qualifier →
// import path) for use by editor tooling. Returns a usable (possibly empty)
// value even when nothing loads, so callers needn't nil-check.
func LoadGoSignatures(dir string, paths map[string]string) *GoSignatures {
	var idx goPkgIndex
	if p := LoadGoPackages(dir, paths); p != nil {
		idx = p.funcs
	}
	return &GoSignatures{idx: idx}
}

// Signature returns the full Go signature for a `pkg/fn` call symbol (e.g.
// "strings/to-upper" → "func strings.ToUpper(s string) string"), converting the
// part after the slash to its Go name exactly as emission does. ok is false for
// an unqualified symbol or one whose package/function wasn't loaded.
func (g *GoSignatures) Signature(glispSym string) (sig string, ok bool) {
	slash := strings.Index(glispSym, "/")
	if slash <= 0 {
		return "", false
	}
	fn, found := g.idx.lookup(glispSym[:slash], fnToGo(glispSym[slash+1:]))
	if !found {
		return "", false
	}
	return fn.sig, true
}

// Completions returns the exported functions of the package named by qualifier
// whose Go name loosely matches partial (case-insensitive, hyphens ignored, so
// a kebab-case glisp prefix like "to-up" matches "ToUpper"). Results are sorted
// by name.
func (g *GoSignatures) Completions(qualifier, partial string) []GoCompletion {
	fns, ok := g.idx[qualifier]
	if !ok {
		return nil
	}
	want := normalizeGoMatch(partial)
	var out []GoCompletion
	for name, fn := range fns {
		if strings.HasPrefix(normalizeGoMatch(name), want) {
			out = append(out, GoCompletion{Name: name, Sig: fn.sig})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// normalizeGoMatch lowercases and strips hyphens so a kebab-case glisp prefix
// matches a PascalCase Go name (to-upper ↔ ToUpper).
func normalizeGoMatch(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), "-", "")
}
