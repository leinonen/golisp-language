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

// webSource is the web routing DSL prelude (web.glsp): GET/POST/…/defroutes,
// always available like the language macros (the engine has no per-import macro
// loading). They expand to golisp/web calls and are inert unless web is imported.
//
//go:embed web.glsp
var webSource string

var (
	coreOnce  sync.Once
	coreCache []*ast.MacroDecl
	coreErr   error
)

// CoreMacros returns the macros defined by the embedded preludes (core.glsp plus
// the web routing DSL web.glsp). They are parsed once and cached. An error means
// a prelude itself is malformed — a bug in glisp, surfaced by the tests.
func CoreMacros() ([]*ast.MacroDecl, error) {
	coreOnce.Do(func() {
		for _, src := range []string{coreSource, webSource} {
			nodes, err := parser.ParseString(src)
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
		}
	})
	return coreCache, coreErr
}
