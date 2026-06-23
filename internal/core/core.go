// Package core embeds the glisp-authored standard library (the `core`
// vocabulary, Phase 14 / ADR-016) and parses it into AST declarations. The
// transpiler mangles and injects these so everyday glisp code can call
// glisp-native names (str/upper, …) that front the Go stdlib.
//
// This package only parses; mangling and injection live in the transpiler
// (which owns identToGo) to avoid an import cycle.
package core

import (
	_ "embed"
	"fmt"
	"sort"
	"sync"

	"golisp/internal/ast"
	"golisp/internal/parser"
)

// sources maps a namespace name to its embedded glisp source. The (ns …) in
// each file must match its key here. The "core" namespace holds the bare
// (auto-referred) functions, callable unqualified.
var sources = map[string]string{
	"str":  strSource,
	"sys":  sysSource,
	"core": bareSource,
}

//go:embed str.glsp
var strSource string

//go:embed sys.glsp
var sysSource string

//go:embed core_bare.glsp
var bareSource string

// BareNamespace is the namespace name whose functions are callable unqualified.
const BareNamespace = "core"

// Namespace holds the parsed function declarations of one core namespace.
type Namespace struct {
	Name  string
	Funcs []*ast.DefnDecl
}

var (
	loadOnce sync.Once
	loaded   map[string]*Namespace
	loadErr  error
)

// Namespaces returns the parsed core namespaces, keyed by name. Parsed once and
// cached; an error means a core source file is malformed — a bug in glisp,
// caught by the tests.
func Namespaces() (map[string]*Namespace, error) {
	loadOnce.Do(func() {
		loaded = make(map[string]*Namespace)
		for name, src := range sources {
			nodes, err := parser.ParseString(src)
			if err != nil {
				loadErr = fmt.Errorf("core/%s.glsp failed to parse: %w", name, err)
				return
			}
			ns := &Namespace{Name: name}
			for _, n := range nodes {
				switch d := n.(type) {
				case *ast.NSDecl:
					// the (ns name) header — ignored beyond documentation
				case *ast.DefnDecl:
					ns.Funcs = append(ns.Funcs, d)
				default:
					loadErr = fmt.Errorf("core/%s.glsp may only contain ns + defn, found %T", name, n)
					return
				}
			}
			loaded[name] = ns
		}
	})
	return loaded, loadErr
}

// SortedNames returns the core namespace names in deterministic order.
func SortedNames() ([]string, error) {
	nss, err := Namespaces()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(nss))
	for n := range nss {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}
