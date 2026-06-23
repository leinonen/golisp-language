package lsp

import (
	"strings"
	"sync"

	"golisp/internal/ast"
	"golisp/internal/core"
)

// coreDocsCache holds hover/completion entries for the glisp-native core library
// (Phase 14), generated from the parsed core signatures so they can never drift
// from the implementation. Keys are call-site names: "str/upper", "sys/env" for
// namespaced functions, and the bare name ("slurp") for the auto-referred core
// namespace.
var (
	coreDocsOnce sync.Once
	coreDocsMap  map[string]BuiltinDoc
)

// CoreDocs returns the documentation entries for the core library, keyed by
// call-site name. Best-effort: if core fails to load, the map is empty and
// hover/completion simply fall back to the built-ins.
func CoreDocs() map[string]BuiltinDoc {
	coreDocsOnce.Do(func() {
		coreDocsMap = map[string]BuiltinDoc{}
		nss, err := core.Namespaces()
		if err != nil {
			return
		}
		for ns, namespace := range nss {
			for _, fn := range namespace.Funcs {
				name := fn.Name
				if ns != core.BareNamespace {
					name = ns + "/" + fn.Name
				}
				coreDocsMap[name] = BuiltinDoc{Sig: formatCoreSig(name, fn), Doc: fn.Doc}
			}
		}
	})
	return coreDocsMap
}

// formatCoreSig renders a core function's signature under its call-site name,
// e.g. "(str/upper [s string] -> string)".
func formatCoreSig(callName string, fn *ast.DefnDecl) string {
	var sb strings.Builder
	sb.WriteString("(")
	sb.WriteString(callName)
	sb.WriteString(" [")
	for i, p := range fn.Params {
		if i > 0 {
			sb.WriteString(" ")
		}
		if p.IsRest {
			sb.WriteString("& ")
		}
		sb.WriteString(p.Name)
		if p.TypeAnnot != nil {
			sb.WriteString(" ")
			sb.WriteString(p.TypeAnnot.Text)
		}
	}
	sb.WriteString("]")
	if fn.ReturnType != nil {
		sb.WriteString(" -> ")
		sb.WriteString(fn.ReturnType.Text)
	}
	sb.WriteString(")")
	return sb.String()
}
