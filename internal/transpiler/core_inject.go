package transpiler

import (
	"sort"
	"strings"
	"sync"

	"golisp/internal/ast"
	"golisp/internal/core"
)

// Phase 14 (ADR-016): the `core` standard library is authored in glisp
// (internal/core/*.glsp) and fronts the Go stdlib with glisp-native names
// (str/upper, …). It is NOT compiled to Go packages — a namespace named
// `string`/`math`/`io` would collide with Go's predeclared type / stdlib
// packages at the import-alias level. Instead each core function is mangled to a
// flat Go function `_gcore_<ns>_<goName>` and injected like a runtime helper,
// gated on use. Namespaces exist only at the glisp level.
//
// builtinImports carries the gating: a used namespace marks the pseudo-key
// "core/<ns>" (never a real Go import — emitImports skips it). In single-file
// builds emitFile emits the needed core functions inline; in dir builds the
// compiler emits them once into glisp_core.go via CoreSource.

const coreManglePrefix = "_gcore_"

type coreIndex struct {
	fnNames map[string]map[string]bool // ns -> set of glisp local names (e.g. "blank?")
	decls   map[string][]ast.Node      // ns -> mangled DefnDecl nodes
}

var (
	coreOnce  sync.Once
	coreData  *coreIndex
	coreLoadE error
)

// coreNeededKey is the builtinImports pseudo-key marking a core namespace used.
func coreNeededKey(ns string) string { return "core/" + ns }

// loadCore parses the embedded core library and mangles each function's name to
// its flat Go helper name. Self-/cross-namespace calls inside core bodies are
// left as ordinary qualified calls — they resolve through the same call-site
// path at emission, so no body rewrite is needed.
func loadCore() (*coreIndex, error) {
	coreOnce.Do(func() {
		nss, err := core.Namespaces()
		if err != nil {
			coreLoadE = err
			return
		}
		ci := &coreIndex{
			fnNames: map[string]map[string]bool{},
			decls:   map[string][]ast.Node{},
		}
		for name, ns := range nss {
			ci.fnNames[name] = map[string]bool{}
			for _, fn := range ns.Funcs {
				ci.fnNames[name][fn.Name] = true
				mangled := *fn // shallow copy: only the name changes
				mangled.Name = coreMangledName(name, fn.Name)
				ci.decls[name] = append(ci.decls[name], &mangled)
			}
		}
		coreData = ci
	})
	return coreData, coreLoadE
}

// coreMangledName is the flat Go helper name for a core function. The same
// formula is used for the injected defn and the call site (both then pass
// through identToGo unchanged, since the result is already Go-clean), so they
// always agree.
func coreMangledName(ns, localName string) string {
	return coreManglePrefix + ns + "_" + identToGo(localName)
}

// resolveCoreCall maps a qualified glisp name (str/upper) to its mangled helper
// name, when the qualifier is a core namespace and names a real core function.
func resolveCoreCall(name string) (mangled, ns string, ok bool) {
	i := strings.Index(name, "/")
	if i <= 0 {
		return "", "", false
	}
	ci, err := loadCore()
	if err != nil || ci == nil {
		return "", "", false
	}
	ns = name[:i]
	local := name[i+1:]
	if fns, isCore := ci.fnNames[ns]; isCore && fns[local] {
		return coreMangledName(ns, local), ns, true
	}
	return "", "", false
}

// resolveCoreBare maps an unqualified name (slurp) to its mangled helper when it
// is a bare core function (defined in the core.BareNamespace). Callers must check
// that the name is not user-defined and not a built-in first, so those win.
func resolveCoreBare(name string) (mangled, ns string, ok bool) {
	if strings.Contains(name, "/") {
		return "", "", false
	}
	ci, err := loadCore()
	if err != nil || ci == nil {
		return "", "", false
	}
	if fns, isCore := ci.fnNames[core.BareNamespace]; isCore && fns[name] {
		return coreMangledName(core.BareNamespace, name), core.BareNamespace, true
	}
	return "", "", false
}

// coreBareShadowed reports whether a bare name is bound by something that must
// win over a bare core function: a user top-level defn, an in-scope local
// binding (let/loop/param/…), or a def global. (Built-ins are handled ahead of
// bare-core resolution by the call switch and never overlap the bare names.)
func (e *Emitter) coreBareShadowed(name string) bool {
	if _, ok := e.symbols[name]; ok {
		return true
	}
	if e.localVars[name] {
		return true
	}
	if e.defGlobals[name] {
		return true
	}
	return false
}

// allCoreDecls returns the mangled declarations of every core namespace, for
// seeding the pre-pass type tables (signatures drive arg coercion + arity at
// core call sites). Whether a function is *emitted* is gated separately by use.
func allCoreDecls() []ast.Node {
	ci, err := loadCore()
	if err != nil || ci == nil {
		return nil
	}
	var out []ast.Node
	for _, ns := range sortedCoreNames(ci) {
		out = append(out, ci.decls[ns]...)
	}
	return out
}

// coreDeclsFor returns the mangled declarations for the given namespaces.
func coreDeclsFor(namespaces map[string]bool) []ast.Node {
	ci, err := loadCore()
	if err != nil || ci == nil {
		return nil
	}
	var names []string
	for ns := range namespaces {
		names = append(names, ns)
	}
	sort.Strings(names)
	var out []ast.Node
	for _, ns := range names {
		out = append(out, ci.decls[ns]...)
	}
	return out
}

// CoreSource emits a Go file (package pkgName) containing the mangled functions
// of the given core namespaces. Used by multi-file (dir) builds, which emit the
// core library once into glisp_core.go rather than inline per file. It returns
// the source plus the builtin-import set the core functions need, so the caller
// can fold those into the shared runtime. Returns "" when no namespaces are
// needed.
func CoreSource(pkgName string, namespaces map[string]bool) (string, map[string]bool, error) {
	if len(namespaces) == 0 {
		return "", nil, nil
	}
	// Close over transitive core deps: a core function may call another core
	// namespace (bare `lines` → `str/split`), which must also be emitted here, or
	// glisp_core.go would reference an undefined helper. Re-emit until the needed
	// set is stable; the final emitter's buffer is the result.
	needed := map[string]bool{}
	for ns := range namespaces {
		needed[ns] = true
	}
	for {
		e := newEmitter()
		e.emitRuntime = false // the shared glisp_runtime.go carries the helpers
		e.pkg = pkgName
		if err := e.emitFile(coreDeclsFor(needed)); err != nil {
			return "", nil, err
		}
		extra := NeededCoreNamespaces(e.builtinImports)
		grew := false
		for ns := range extra {
			if !needed[ns] {
				needed[ns] = true
				grew = true
			}
		}
		if !grew {
			return e.buf.String(), e.builtinImports, nil
		}
	}
}

func sortedStringSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedCoreNames(ci *coreIndex) []string {
	names := make([]string, 0, len(ci.decls))
	for ns := range ci.decls {
		names = append(names, ns)
	}
	sort.Strings(names)
	return names
}

// neededCoreNamespaces extracts the core namespaces marked in a builtinImports
// set (the "core/<ns>" pseudo-keys).
func NeededCoreNamespaces(builtins map[string]bool) map[string]bool {
	out := map[string]bool{}
	for k := range builtins {
		if strings.HasPrefix(k, "core/") {
			out[strings.TrimPrefix(k, "core/")] = true
		}
	}
	return out
}
