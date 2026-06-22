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
			res := sig.Results()
			results := make([]string, res.Len())
			for i := 0; i < res.Len(); i++ {
				results[i] = types.TypeString(res.At(i).Type(), qualifier)
			}
			ret := ""
			if len(results) == 1 {
				ret = results[0]
			}
			fns[name] = goFunc{
				params:   ptypes,
				variadic: sig.Variadic(),
				ret:      ret,
				results:  results,
				sig:      types.ObjectString(fn, qualifier),
			}
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
	return &GoSignatures{idx: LoadGoPackages(dir, paths)}
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
