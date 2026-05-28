// Package transpiler converts a glisp AST into Go source code.
package transpiler

import (
	"fmt"
	"strings"

	"golisp/internal/ast"
	"golisp/internal/lexer"
	"golisp/internal/parser"
)

// ParseError wraps a lexer or parser error.
type ParseError struct{ Err error }

func (e *ParseError) Error() string { return "parse error: " + e.Err.Error() }
func (e *ParseError) Unwrap() error { return e.Err }

// TranspileError wraps a code-generation error from the emitter.
type TranspileError struct{ Err error }

func (e *TranspileError) Error() string { return "transpile error: " + e.Err.Error() }
func (e *TranspileError) Unwrap() error { return e.Err }

// Transpile is the top-level entry point: source text → Go source text.
// The returned Go source is not gofmt'd; call gofmt externally.
func Transpile(src string) (string, error) {
	tokens, err := lexer.Tokenize(src)
	if err != nil {
		return "", &ParseError{Err: err}
	}
	nodes, err := parser.ParseSource(tokens, src)
	if err != nil {
		return "", &ParseError{Err: err}
	}
	e := newEmitter()
	if err := e.emitFile(nodes); err != nil {
		return "", &TranspileError{Err: err}
	}
	return e.buf.String(), nil
}

// TranspileNoRuntime transpiles source to Go without appending runtime helpers.
// It also returns the set of built-in packages used so the caller can generate
// a shared runtime file for multi-file package builds.
func TranspileNoRuntime(src string) (string, map[string]bool, error) {
	tokens, err := lexer.Tokenize(src)
	if err != nil {
		return "", nil, &ParseError{Err: err}
	}
	nodes, err := parser.ParseSource(tokens, src)
	if err != nil {
		return "", nil, &ParseError{Err: err}
	}
	e := newEmitter()
	e.emitRuntime = false
	if err := e.emitFile(nodes); err != nil {
		return "", nil, &TranspileError{Err: err}
	}
	return e.buf.String(), e.builtinImports, nil
}

// Emitter accumulates Go source text with indentation tracking.
type Emitter struct {
	buf     strings.Builder
	indent  int
	counter int // unique ID generator for temp vars

	// current package name (from ns declaration)
	pkg string
	// imports seen from ns declarations
	imports []ast.ImportSpec
	// loop binding names for the current loop scope
	loopBindings []string
	// loopInReturn: true when the current loop is in tail/return position
	loopInReturn bool
	// builtinImports tracks which built-in packages are needed
	builtinImports map[string]bool
	// emitRuntime controls whether runtime helpers are appended to the output.
	// True by default; set false for multi-file builds that use a shared runtime file.
	emitRuntime bool
}

func (e *Emitter) needImport(pkg string) {
	if e.builtinImports == nil {
		e.builtinImports = map[string]bool{}
	}
	e.builtinImports[pkg] = true
}

func newEmitter() *Emitter {
	return &Emitter{pkg: "main", emitRuntime: true}
}

func (e *Emitter) fresh(prefix string) string {
	e.counter++
	return fmt.Sprintf("_%s%d", prefix, e.counter)
}

func (e *Emitter) write(s string)                   { e.buf.WriteString(s) }
func (e *Emitter) writef(f string, a ...any)         { fmt.Fprintf(&e.buf, f, a...) }
func (e *Emitter) nl()                               { e.buf.WriteByte('\n') }
func (e *Emitter) writeIndent()                      { e.buf.WriteString(strings.Repeat("\t", e.indent)) }
func (e *Emitter) line(s string)                     { e.writeIndent(); e.write(s); e.nl() }
func (e *Emitter) linef(f string, a ...any)          { e.writeIndent(); e.writef(f, a...); e.nl() }
func (e *Emitter) push()                             { e.indent++ }
func (e *Emitter) pop()                              { e.indent-- }

// emitFile emits the full Go file: package, imports, declarations, runtime helpers.
// We use a two-pass approach: emit declarations into a temp buffer first to
// discover which built-in imports are needed, then prepend package+imports.
func (e *Emitter) emitFile(nodes []ast.Node) error {
	// Collect ns declaration
	for _, n := range nodes {
		if ns, ok := n.(*ast.NSDecl); ok {
			e.pkg = packageName(ns.Name)
			e.imports = ns.Imports
		}
	}

	// Pass 1: emit declarations into a side buffer to discover import needs
	declEmitter := newEmitter()
	declEmitter.pkg = e.pkg
	declEmitter.imports = e.imports
	for _, n := range nodes {
		if _, ok := n.(*ast.NSDecl); ok {
			continue
		}
		if err := declEmitter.emitTopLevel(n); err != nil {
			return err
		}
		declEmitter.nl()
	}

	// Pass 2: emit header into main buffer
	e.linef("package %s", e.pkg)
	e.nl()

	// Merge builtin import needs from decl pass
	e.builtinImports = declEmitter.builtinImports
	if err := e.emitImports(); err != nil {
		return err
	}

	// Append declarations
	e.write(declEmitter.buf.String())

	// Runtime helpers (omitted for multi-file builds that use a shared runtime file)
	if e.emitRuntime {
		e.write(glispRuntime)
		if e.builtinImports["sort"] {
			e.write(glispSortRuntime)
		}
		if e.builtinImports["strings"] {
			e.write(glispStrRuntime)
		}
		if e.builtinImports["encoding/json"] {
			e.write(glispJsonRuntime)
		}
	}
	return nil
}

// hasImport returns true if path is already in the import list.
func (e *Emitter) hasImport(path string) bool {
	for _, imp := range e.imports {
		if imp.Path == path {
			return true
		}
	}
	return false
}

func (e *Emitter) emitImports() error {
	allImports := make([]ast.ImportSpec, 0, len(e.imports)+2)
	// Add built-in imports that were actually needed during emission
	for _, pkg := range []string{"fmt", "errors", "strings", "sort", "testing", "encoding/json"} {
		if e.builtinImports[pkg] && !e.hasImport(pkg) {
			allImports = append(allImports, ast.ImportSpec{Path: pkg})
		}
	}
	allImports = append(allImports, e.imports...)

	if len(allImports) == 0 {
		return nil
	}
	e.line("import (")
	e.push()
	for _, imp := range allImports {
		if imp.Alias != "" {
			e.linef("%s %q", imp.Alias, imp.Path)
		} else {
			e.linef("%q", imp.Path)
		}
	}
	e.pop()
	e.line(")")
	e.nl()
	return nil
}

// emitTopLevel dispatches top-level declarations.
func (e *Emitter) emitTopLevel(n ast.Node) error {
	switch v := n.(type) {
	case *ast.DefDecl:
		return e.emitDefDecl(v)
	case *ast.DefnDecl:
		return e.emitDefnDecl(v)
	case *ast.StructDecl:
		return e.emitStructDecl(v)
	case *ast.InterfaceDecl:
		return e.emitInterfaceDecl(v)
	case *ast.MethodDecl:
		return e.emitMethodDecl(v)
	case *ast.DefTestDecl:
		return e.emitDefTestDecl(v)
	default:
		return fmt.Errorf("unsupported top-level form: %T at %s", n, n.Pos())
	}
}

// emitExpr emits any expression inline (no trailing newline).
func (e *Emitter) emitExpr(n ast.Node) error {
	switch v := n.(type) {
	case *ast.NilLit:
		e.write("nil")
	case *ast.BoolLit:
		if v.Value {
			e.write("true")
		} else {
			e.write("false")
		}
	case *ast.IntLit:
		e.writef("%d", v.Value)
	case *ast.FloatLit:
		e.writef("%g", v.Value)
	case *ast.StringLit:
		e.writef("%q", v.Value)
	case *ast.KeywordLit:
		e.writef("%q", v.Value)
	case *ast.Symbol:
		goName := identToGo(v.Name)
		// If this is a package-qualified call like fmt/Println, mark fmt as needed
		// (only for packages not already in user imports)
		e.write(goName)
	case *ast.VectorLit:
		return e.emitVectorLit(v)
	case *ast.MapLit:
		return e.emitMapLit(v)
	case *ast.CallExpr:
		return e.emitCallExpr(v)
	case *ast.FnExpr:
		return e.emitFnExpr(v)
	case *ast.LetExpr:
		return e.emitLetExpr(v)
	case *ast.IfExpr:
		return e.emitIfExpr(v)
	case *ast.WhenExpr:
		return e.emitWhenExpr(v)
	case *ast.CondExpr:
		return e.emitCondExpr(v)
	case *ast.DoExpr:
		return e.emitDoExpr(v)
	case *ast.GoStmt:
		return e.emitGoStmt(v)
	case *ast.DeferStmt:
		return e.emitDeferStmt(v)
	case *ast.ChanExpr:
		return e.emitChanExpr(v)
	case *ast.SendStmt:
		return e.emitSendStmt(v)
	case *ast.RecvExpr:
		return e.emitRecvExpr(v)
	case *ast.CloseStmt:
		return e.emitCloseStmt(v)
	case *ast.SelectStmt:
		return e.emitSelectStmt(v)
	case *ast.LoopExpr:
		return e.emitLoopExpr(v, false)
	case *ast.RecurExpr:
		return e.emitRecurStmt(v)
	case *ast.ReturnExpr:
		return e.emitReturnExpr(v)
	case *ast.ValuesExpr:
		return e.emitValuesExpr(v)
	case *ast.IfErrExpr:
		return e.emitIfErrExpr(v)
	case *ast.MethodCallExpr:
		return e.emitMethodCallExpr(v)
	case *ast.FieldAccessExpr:
		return e.emitFieldAccessExpr(v)
	case *ast.StructLitExpr:
		return e.emitStructLitExpr(v)
	case *ast.TypeAssertExpr:
		return e.emitTypeAssertExpr(v)
	default:
		return fmt.Errorf("unsupported expression: %T at %s", n, n.Pos())
	}
	return nil
}

// emitStmtNode emits a node in statement position (no value required).
// let/if/cond/do/when are emitted as Go blocks; loops/goroutines as-is.
// This avoids the need to wrap them in IIFEs when their value is discarded.
func (e *Emitter) emitStmtNode(n ast.Node) error {
	switch v := n.(type) {
	case *ast.LetExpr:
		return e.emitLetStmt(v)
	case *ast.IfExpr:
		return e.emitIfStmt(v)
	case *ast.WhenExpr:
		return e.emitWhenStmt(v)
	case *ast.CondExpr:
		return e.emitCondStmt(v)
	case *ast.DoExpr:
		for _, node := range v.Body {
			if err := e.emitStmtNode(node); err != nil {
				return err
			}
		}
		return nil
	case *ast.GoStmt:
		e.writeIndent()
		return e.emitGoStmt(v)
	case *ast.DeferStmt:
		e.writeIndent()
		return e.emitDeferStmt(v)
	case *ast.SendStmt:
		e.writeIndent()
		if err := e.emitSendStmt(v); err != nil {
			return err
		}
		e.nl()
		return nil
	case *ast.CloseStmt:
		e.writeIndent()
		if err := e.emitCloseStmt(v); err != nil {
			return err
		}
		e.nl()
		return nil
	case *ast.ReturnExpr:
		e.writeIndent()
		return e.emitReturnExpr(v)
	default:
		// Generic expression statement: emit and discard value
		e.writeIndent()
		if err := e.emitExpr(n); err != nil {
			return err
		}
		e.nl()
		return nil
	}
}

// emitLetStmt emits a let in statement position (no IIFE).
func (e *Emitter) emitLetStmt(n *ast.LetExpr) error {
	if err := e.emitLetBindings(n.Bindings); err != nil {
		return err
	}
	for _, node := range n.Body {
		if err := e.emitStmtNode(node); err != nil {
			return err
		}
	}
	return nil
}

// emitIfStmt emits an if in statement position.
func (e *Emitter) emitIfStmt(n *ast.IfExpr) error {
	e.writeIndent()
	e.write("if ")
	if err := e.emitExpr(n.Cond); err != nil {
		return err
	}
	e.write(" {")
	e.nl()
	e.push()
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

// emitWhenStmt emits a when in statement position.
func (e *Emitter) emitWhenStmt(n *ast.WhenExpr) error {
	e.writeIndent()
	e.write("if ")
	if err := e.emitExpr(n.Cond); err != nil {
		return err
	}
	e.write(" {")
	e.nl()
	e.push()
	for _, node := range n.Body {
		if err := e.emitStmtNode(node); err != nil {
			return err
		}
	}
	e.pop()
	e.line("}")
	return nil
}

// emitCondStmt emits a cond in statement position.
func (e *Emitter) emitCondStmt(n *ast.CondExpr) error {
	for i, clause := range n.Clauses {
		e.writeIndent()
		if i == 0 {
			e.write("if ")
		} else {
			e.write("} else if ")
		}
		if err := e.emitExpr(clause.Test); err != nil {
			return err
		}
		e.write(" {")
		e.nl()
		e.push()
		if err := e.emitStmtNode(clause.Body); err != nil {
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
		if err := e.emitStmtNode(n.Default); err != nil {
			return err
		}
		e.pop()
	}
	if len(n.Clauses) > 0 || n.Default != nil {
		e.line("}")
	}
	return nil
}

// emitBody emits a sequence of statements; the last is treated as a return value
// when inReturn is true.
func (e *Emitter) emitBody(body []ast.Node, inReturn bool) error {
	for i, node := range body {
		isLast := i == len(body)-1
		if isLast && inReturn {
			if err := e.emitReturnNode(node); err != nil {
				return err
			}
		} else {
			if err := e.emitStmtNode(node); err != nil {
				return err
			}
		}
	}
	return nil
}

// emitReturnNode emits a node in return position.
// Certain statement-like nodes (GoStmt, DeferStmt, SendStmt, CloseStmt) are not
// returned even in tail position — they're just emitted as statements.
func (e *Emitter) emitReturnNode(n ast.Node) error {
	switch v := n.(type) {
	case *ast.GoStmt:
		e.writeIndent()
		return e.emitGoStmt(v)
	case *ast.DeferStmt:
		e.writeIndent()
		return e.emitDeferStmt(v)
	case *ast.SendStmt:
		e.writeIndent()
		if err := e.emitSendStmt(v); err != nil {
			return err
		}
		e.nl()
		return nil
	case *ast.CloseStmt:
		e.writeIndent()
		if err := e.emitCloseStmt(v); err != nil {
			return err
		}
		e.nl()
		return nil
	case *ast.ReturnExpr:
		e.writeIndent()
		return e.emitReturnExpr(v)
	case *ast.IfErrExpr:
		return e.emitIfErrExprReturn(v)
	case *ast.LoopExpr:
		return e.emitLoopExpr(v, true)
	case *ast.ValuesExpr:
		e.writeIndent()
		e.write("return ")
		return e.emitValuesExpr(v)
	case *ast.IfExpr:
		return e.emitIfExprReturn(v)
	case *ast.CondExpr:
		return e.emitCondExprReturn(v)
	case *ast.DoExpr:
		return e.emitDoExprReturn(v)
	case *ast.LetExpr:
		return e.emitLetExprReturn(v)
	default:
		e.writeIndent()
		e.write("return ")
		if err := e.emitExpr(n); err != nil {
			return err
		}
		e.nl()
		return nil
	}
}

// packageName extracts the last segment of a dotted package name.
// "myapp.server" → "server", "main" → "main"
func packageName(s string) string {
	parts := strings.Split(s, ".")
	return parts[len(parts)-1]
}
