package macro

import (
	_ "embed"
	"fmt"
	"sync"

	"golisp/internal/ast"
	"golisp/internal/parser"
)

// coreSource is the glisp prelude (core.glsp), embedded into the binary so the
// core macros are available in every compilation with no import.
//
//go:embed core.glsp
var coreSource string

var (
	coreOnce  sync.Once
	coreCache []*ast.MacroDecl
	coreErr   error
)

// CoreMacros returns the macros defined by the embedded core prelude. They are
// parsed once and cached. An error means core.glsp itself is malformed — a bug
// in glisp, surfaced by the tests.
func CoreMacros() ([]*ast.MacroDecl, error) {
	coreOnce.Do(func() {
		nodes, err := parser.ParseString(coreSource)
		if err != nil {
			coreErr = fmt.Errorf("internal: core prelude failed to parse: %w", err)
			return
		}
		for _, n := range nodes {
			md, ok := n.(*ast.MacroDecl)
			if !ok {
				coreErr = fmt.Errorf("internal: core prelude may only contain defmacro, found %T", n)
				return
			}
			coreCache = append(coreCache, md)
		}
	})
	return coreCache, coreErr
}
