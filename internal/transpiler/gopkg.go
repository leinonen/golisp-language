package transpiler

import (
	"go/types"
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
	ret      string // single return Go type; "" if none, void, or multi-return
}

// goPkgIndex maps a package qualifier (the call-site prefix in `pkg/fn`) to its
// exported package-level functions, keyed by Go function name (e.g. "Printf").
// Built by LoadGoPackages from the Go toolchain's own type information — jank's
// model (Clang for C++) with go/types for Go. A package that fails to load is
// simply absent, so callers degrade to untyped emission (never a hard failure).
type goPkgIndex map[string]map[string]goFunc

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
	return e.goPkgs.lookup(name[:slash], fnToGo(name[slash+1:]))
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
func LoadGoPackages(dir string, paths map[string]string) goPkgIndex {
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
			fn, ok := obj.(*types.Func)
			if !ok {
				continue
			}
			sig, ok := fn.Type().(*types.Signature)
			if !ok || sig.Recv() != nil {
				continue // methods are dispatched separately
			}
			params := sig.Params()
			ptypes := make([]string, params.Len())
			for i := 0; i < params.Len(); i++ {
				ptypes[i] = types.TypeString(params.At(i).Type(), qualifier)
			}
			ret := ""
			if res := sig.Results(); res.Len() == 1 {
				ret = types.TypeString(res.At(0).Type(), qualifier)
			}
			fns[name] = goFunc{params: ptypes, variadic: sig.Variadic(), ret: ret}
		}
		if len(fns) > 0 {
			idx[qual] = fns
		}
	}
	if len(idx) == 0 {
		return nil
	}
	return idx
}
