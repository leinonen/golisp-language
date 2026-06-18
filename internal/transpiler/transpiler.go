// Package transpiler converts a glisp AST into Go source code.
package transpiler

import (
	"fmt"
	"sort"
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

// transpileInternal is the unified internal entry point for all transpile variants.
func transpileInternal(src, filename string, runtime, strict bool) (out string, imports map[string]bool, err error) {
	// Defensive boundary: a malformed program should never crash the host
	// process (the CLI prints a stack trace; the LSP server dies outright).
	// Any panic escaping the lexer/parser/emitter is converted into a normal
	// TranspileError so callers can surface it as a diagnostic.
	defer func() {
		if r := recover(); r != nil {
			out = ""
			imports = nil
			err = &TranspileError{Err: fmt.Errorf("internal transpiler error: %v", r)}
		}
	}()

	tokens, err := lexer.Tokenize(src)
	if err != nil {
		return "", nil, &ParseError{Err: err}
	}
	var nodes []ast.Node
	var parseErr error
	if filename != "" {
		nodes, parseErr = parser.ParseSourceFile(tokens, src, filename)
	} else {
		nodes, parseErr = parser.ParseSource(tokens, src)
	}
	if parseErr != nil {
		return "", nil, &ParseError{Err: parseErr}
	}
	e := newEmitter()
	e.emitRuntime = runtime
	e.strict = strict
	if err := e.emitFile(nodes); err != nil {
		return "", nil, &TranspileError{Err: err}
	}
	return e.buf.String(), e.builtinImports, nil
}

// Transpile is the top-level entry point: source text → Go source text.
// The returned Go source is not gofmt'd; call gofmt externally.
func Transpile(src string) (string, error) {
	out, _, err := transpileInternal(src, "", true, false)
	return out, err
}

// TranspileNoRuntime transpiles source to Go without appending runtime helpers.
// It also returns the set of built-in packages used so the caller can generate
// a shared runtime file for multi-file package builds.
func TranspileNoRuntime(src string) (string, map[string]bool, error) {
	return transpileInternal(src, "", false, false)
}

// TranspileFile is like Transpile but embeds //line directives so go build
// error messages report .glsp file locations instead of generated .go locations.
func TranspileFile(src, filename string) (string, error) {
	out, _, err := transpileInternal(src, filename, true, false)
	return out, err
}

// TranspileNoRuntimeFile is like TranspileNoRuntime but embeds //line directives.
func TranspileNoRuntimeFile(src, filename string) (string, map[string]bool, error) {
	return transpileInternal(src, filename, false, false)
}

// TranspileFileStrict is like TranspileFile with strict mode enabled.
// In strict mode, defn params, defstruct fields, and def globals must have type annotations.
func TranspileFileStrict(src, filename string) (string, error) {
	out, _, err := transpileInternal(src, filename, true, true)
	return out, err
}

// TranspileNoRuntimeFileStrict is like TranspileNoRuntimeFile with strict mode enabled.
func TranspileNoRuntimeFileStrict(src, filename string) (string, map[string]bool, error) {
	return transpileInternal(src, filename, false, true)
}

// fnSig holds the arity information for a user-defined function.
type fnSig struct {
	minArity   int
	variadic   bool
	paramTypes []string // Go type per positional (non-rest) param; "" if untyped
	retType    string   // Go return type; "" if none/void/multi
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
	// requires: glisp module paths emitted as Go imports
	requires []ast.RequireSpec
	// loop binding names for the current loop scope
	loopBindings []string
	// loopInReturn: true when the current loop is in tail/return position
	loopInReturn bool
	// builtinImports tracks which built-in packages are needed (runtime-backed forms)
	builtinImports map[string]bool
	// directImports tracks packages referenced directly via qualified symbols (os/exit,
	// math/Pi, etc.) — emitted unconditionally, no runtime-only filtering.
	directImports map[string]bool
	// emitRuntime controls whether runtime helpers are appended to the output.
	// True by default; set false for multi-file builds that use a shared runtime file.
	emitRuntime bool
	// strict: when true, require type annotations on defn params, struct fields, def globals.
	strict bool
	// symbols: user-defined function signatures for arity checking (populated by pre-pass).
	symbols map[string]*fnSig
	// structs: declared struct types by glisp name (populated by pre-pass). Drives
	// typed map literals and keyword field access.
	structs map[string]*structInfo
	// localTypes: in-scope variables (glisp name) known to hold a declared struct
	// or interface type. Managed with pushTypeScope/popTypeScope around function
	// and let bodies.
	localTypes map[string]string
	// localVars: every in-scope value binding by glisp name (params, receiver,
	// let/loop bindings), regardless of type. A binding here shadows dot-free
	// method dispatch — (area s) with a local `area` stays a plain call. Scoped
	// together with localTypes.
	localVars map[string]bool
	// localAny: in-scope bindings statically known to hold Go `any` (untyped
	// params, range loop vars, map/index lookups). Drives numeric auto-coercion:
	// arithmetic/comparison on these routes through the _glisp{Add,Lt,…} helpers
	// instead of native Go operators (which don't type-check on `any`). Scoped
	// together with localTypes/localVars.
	localAny map[string]bool
	// ifaces: definterface method tables by interface name (populated by pre-pass).
	ifaces map[string]methodSet
	// methods: defmethod method tables by bare receiver struct name (pre-pass).
	methods map[string]methodSet
	// defGlobals: names bound by top-level def (pre-pass); they shadow method
	// dispatch like locals do.
	defGlobals map[string]bool
	// currentRetType: Go return type of the function currently being emitted, used
	// to hint collection/struct literals in tail/return position. "" when none.
	currentRetType string
	// sawLineDir: true once any //line directive has been emitted (file mode);
	// gates the //line reset in front of the appended runtime helpers.
	sawLineDir bool
}

func (e *Emitter) needImport(pkg string) {
	if e.builtinImports == nil {
		e.builtinImports = map[string]bool{}
	}
	e.builtinImports[pkg] = true
}

// isModuleAlias returns true if alias matches the Go package qualifier of any
// user-declared import or require — either an explicit :as alias or the
// qualifier derived from the path (e.g. "web" for "golisp/web", "pgx" for
// "github.com/jackc/pgx/v5").
// Used to avoid emitting a bare import "web" when the real path is "golisp/web".
func (e *Emitter) isModuleAlias(alias string) bool {
	for _, imp := range e.imports {
		if imp.Alias == alias || pathQualifier(imp.Path) == alias {
			return true
		}
	}
	for _, req := range e.requires {
		if req.Alias == alias || pathQualifier(req.Path) == alias {
			return true
		}
	}
	return false
}

// resolveDirectImport records the Go import a bare qualified symbol needs
// (filepath/join → "path/filepath"), resolving the qualifier through the stdlib
// package map. A multi-segment stdlib qualifier auto-imports its full path; an
// ambiguous or unknown qualifier yields a position-tagged glisp error so the
// user never sees a raw "package X is not in std" Go error. Reached only for
// qualifiers that are not declared module/import aliases — and since a bare
// import resolves only for stdlib, erroring here cannot break a working build.
func (e *Emitter) resolveDirectImport(sym *ast.Symbol, qualifier string) error {
	paths, ok := stdlibByQualifier[qualifier]
	if !ok {
		return fmt.Errorf("unknown package %q — not a stdlib package; declare it in ns, e.g. (:import [path/to/%s]) (at %s)", qualifier, qualifier, sym.Pos())
	}
	if len(paths) > 1 {
		opts := make([]string, len(paths))
		for i, p := range paths {
			opts[i] = "(:import [" + p + "])"
		}
		return fmt.Errorf("ambiguous package qualifier %q — declare which one in ns: %s (at %s)", qualifier, strings.Join(opts, " or "), sym.Pos())
	}
	e.directImports[paths[0]] = true
	return nil
}

// pathQualifier returns the default Go package qualifier for an import path:
// the last path segment, or the one before it when the last segment is a
// major-version suffix per the Go module convention
// ("github.com/jackc/pgx/v5" → "pgx").
func pathQualifier(path string) string {
	segs := strings.Split(path, "/")
	last := segs[len(segs)-1]
	if len(segs) >= 2 && isMajorVersionSegment(last) {
		last = segs[len(segs)-2]
	}
	return last
}

// isMajorVersionSegment reports whether s is a Go module major-version path
// segment: v2, v3, ... (v0/v1 never appear as path suffixes per the convention).
func isMajorVersionSegment(s string) bool {
	if len(s) < 2 || s[0] != 'v' || s[1] == '0' || s == "v1" {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func newEmitter() *Emitter {
	return &Emitter{
		pkg:            "main",
		emitRuntime:    true,
		builtinImports: map[string]bool{},
		directImports:  map[string]bool{},
	}
}

func (e *Emitter) fresh(prefix string) string {
	e.counter++
	return fmt.Sprintf("_%s%d", prefix, e.counter)
}

func (e *Emitter) write(s string)            { e.buf.WriteString(s) }
func (e *Emitter) writef(f string, a ...any) { fmt.Fprintf(&e.buf, f, a...) }
func (e *Emitter) nl()                       { e.buf.WriteByte('\n') }
func (e *Emitter) writeIndent()              { e.buf.WriteString(strings.Repeat("\t", e.indent)) }
func (e *Emitter) line(s string)             { e.writeIndent(); e.write(s); e.nl() }
func (e *Emitter) linef(f string, a ...any)  { e.writeIndent(); e.writef(f, a...); e.nl() }
func (e *Emitter) push()                     { e.indent++ }
func (e *Emitter) pop()                      { e.indent-- }

// lineDir emits a //line directive at column 0 so the Go compiler attributes
// the next line to pos in the original .glsp source (for error messages).
// No-op when pos.File == "" — all existing Transpile/TranspileNoRuntime paths.
func (e *Emitter) lineDir(pos ast.Position) {
	if pos.File == "" {
		return
	}
	e.sawLineDir = true
	fmt.Fprintf(&e.buf, "//line %s:%d\n", pos.File, pos.Line)
}

// countParams returns the minimum arity and variadic flag for a param list.
func countParams(params []ast.Param) (minArity int, variadic bool) {
	for _, p := range params {
		if p.IsRest {
			variadic = true
		} else {
			minArity++
		}
	}
	return
}

// emitFile emits the full Go file: package, imports, declarations, runtime helpers.
// We use a two-pass approach: emit declarations into a temp buffer first to
// discover which built-in imports are needed, then prepend package+imports.
func (e *Emitter) emitFile(nodes []ast.Node) error {
	// Collect ns declaration
	for _, n := range nodes {
		if ns, ok := n.(*ast.NSDecl); ok {
			e.pkg = packageName(ns.Name)
			e.imports = ns.Imports
			e.requires = ns.Requires
		}
	}

	// Pre-pass: collect user-defined function signatures (arity + types),
	// declared struct types, and method tables for dot-free method dispatch.
	symbols := map[string]*fnSig{}
	structs := map[string]*structInfo{}
	for _, n := range nodes {
		switch d := n.(type) {
		case *ast.DefnDecl:
			symbols[d.Name] = buildFnSig(d.Params, d.ReturnType)
		case *ast.StructDecl:
			structs[d.Name] = buildStructInfo(d)
		}
	}
	ifaces, methods, defGlobals := collectMethodTables(nodes)

	// Pass 1: emit declarations into a side buffer to discover import needs
	declEmitter := newEmitter()
	declEmitter.emitRuntime = e.emitRuntime
	declEmitter.strict = e.strict
	declEmitter.symbols = symbols
	declEmitter.structs = structs
	declEmitter.ifaces = ifaces
	declEmitter.methods = methods
	declEmitter.defGlobals = defGlobals
	declEmitter.pkg = e.pkg
	declEmitter.imports = e.imports
	declEmitter.requires = e.requires
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

	// Merge import needs from decl pass
	e.builtinImports = declEmitter.builtinImports
	e.directImports = declEmitter.directImports
	// Runtimes that use fmt/os: mark those imports needed in single-file mode only.
	// RuntimeSource handles them for multi-file.
	if e.emitRuntime {
		// _glispToInt/_glispToFloat64 (always in glispRuntime) parse numeric
		// strings via strconv, so the runtime always needs it.
		e.needImport("strconv")
		if e.builtinImports["data"] {
			e.needImport("fmt")
		}
		if e.builtinImports["_pp"] {
			e.needImport("fmt")
		}
		if e.builtinImports["_file"] {
			e.needImport("fmt")
			e.needImport("os")
		}
		if e.builtinImports["regexp"] {
			e.needImport("fmt")
			e.needImport("regexp")
		}
		// String runtime helpers (_glispJoin, _glispSplit, etc.) use the strings
		// package internally, and _glispJoin uses fmt.Sprintf for non-string
		// elements. In single-file mode the whole glispStrRuntime block is inlined
		// in this file (gated on "strings" || "_strruntime"), so we must import both
		// strings and fmt here. In multi-file mode the runtime file handles its own
		// imports; _strruntime does NOT add a per-file strings import.
		if e.builtinImports["strings"] || e.builtinImports["_strruntime"] {
			e.needImport("strings")
			e.needImport("fmt")
		}
		if e.builtinImports["_atom"] {
			e.needImport("sync")
		}
		if e.builtinImports["_ctx"] {
			e.needImport("context")
			e.needImport("time")
		}
	}
	if err := e.emitImports(); err != nil {
		return err
	}

	// Append declarations
	e.write(declEmitter.buf.String())
	// //line directives live in the declaration buffer; propagate the flag so
	// the runtime-helper reset below fires in file mode.
	e.sawLineDir = e.sawLineDir || declEmitter.sawLineDir

	// Runtime helpers (omitted for multi-file builds that use a shared runtime file)
	if e.emitRuntime {
		// In //line mode the helpers would otherwise inherit the last user-code
		// directive, so panics inside them would point at a bogus line of the
		// .glsp source. Re-anchor them to a virtual glisp_runtime.go — the same
		// name the shared runtime file has in multi-file builds.
		if e.sawLineDir {
			e.write("//line glisp_runtime.go:1\n")
		}
		e.write(glispRuntime)
		if e.builtinImports["sort"] {
			e.write(glispSortRuntime)
		}
		if e.builtinImports["strings"] || e.builtinImports["_strruntime"] {
			e.write(glispStrRuntime)
		}
		if e.builtinImports["encoding/json"] {
			e.write(glispJsonRuntime)
		}
		if e.builtinImports["net/http"] {
			e.write(glispHttpRuntime)
		}
		if e.builtinImports["os"] {
			e.write(glispEnvRuntime)
		}
		if e.builtinImports["_file"] {
			e.write(glispFileRuntime)
		}
		if e.builtinImports["regexp"] {
			e.write(glispReRuntime)
		}
		if e.builtinImports["data"] {
			e.write(glispDataRuntime)
		}
		if e.builtinImports["_pp"] {
			e.write(glispPPRuntime)
		}
		if e.builtinImports["_num"] {
			e.write(glispNumRuntime)
		}
		if e.builtinImports["_set"] {
			e.write(glispSetRuntime)
		}
		if e.builtinImports["_atom"] {
			e.write(glispAtomRuntime)
		}
		if e.builtinImports["_ctx"] {
			e.write(glispCtxRuntime)
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
	// Add built-in imports that were actually needed during emission.
	// In multi-file mode (emitRuntime==false), sort and encoding/json are only
	// used by the runtime helpers in glisp_runtime.go, not by user code directly.
	runtimeOnlyPkgs := map[string]bool{"sort": true, "encoding/json": true, "net/http": true, "io": true, "os": true, "regexp": true}
	for _, pkg := range []string{"fmt", "errors", "strings", "strconv", "sort", "testing", "encoding/json", "net/http", "io", "os", "regexp", "sync", "time", "log/slog", "context"} {
		if e.builtinImports[pkg] && !e.hasImport(pkg) {
			if !e.emitRuntime && runtimeOnlyPkgs[pkg] {
				continue
			}
			allImports = append(allImports, ast.ImportSpec{Path: pkg})
		}
	}
	// Add packages referenced directly via qualified symbols (math/Pi, os/exit, etc.).
	// Emitted unconditionally — these are real user-code references, not runtime-helper-backed.
	{
		pkgs := make([]string, 0, len(e.directImports))
		for pkg := range e.directImports {
			pkgs = append(pkgs, pkg)
		}
		sort.Strings(pkgs)
		for _, pkg := range pkgs {
			if e.hasImport(pkg) {
				continue
			}
			already := false
			for _, ai := range allImports {
				if ai.Path == pkg {
					already = true
					break
				}
			}
			if !already {
				allImports = append(allImports, ast.ImportSpec{Path: pkg})
			}
		}
	}
	allImports = append(allImports, e.imports...)
	for _, req := range e.requires {
		if req.Alias != "" {
			allImports = append(allImports, ast.ImportSpec{Path: req.Path, Alias: req.Alias})
		} else {
			allImports = append(allImports, ast.ImportSpec{Path: req.Path})
		}
	}

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
	case *ast.DefTypeDecl:
		return e.emitDefTypeDecl(v)
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
		s := fmt.Sprintf("%g", v.Value)
		if !strings.ContainsAny(s, ".e") {
			s += ".0"
		}
		e.write(s)
	case *ast.StringLit:
		e.writef("%q", v.Value)
	case *ast.KeywordLit:
		e.writef("%q", v.Value)
	case *ast.Symbol:
		goName := identToGo(v.Name)
		// Track packages used directly via qualified names (math/Pi, os/exit, etc.).
		// Skip aliases that resolve to module imports (e.g. "web" → "golisp/web").
		if idx := strings.Index(v.Name, "/"); idx > 0 {
			pkg := v.Name[:idx]
			if !e.isModuleAlias(pkg) {
				if err := e.resolveDirectImport(v, pkg); err != nil {
					return err
				}
			}
		}
		e.write(goName)
	case *ast.VectorLit:
		return e.emitVectorLit(v)
	case *ast.MapLit:
		return e.emitMapLit(v)
	case *ast.SetLit:
		return e.emitSetLit(v)
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
	case *ast.SwitchExpr:
		return e.emitSwitchExpr(v)
	case *ast.DoExpr:
		return e.emitDoExpr(v)
	case *ast.GoStmt:
		return e.emitGoStmt(v)
	case *ast.GoValExpr:
		return e.emitGoValExpr(v)
	case *ast.DeferStmt:
		return e.emitDeferStmt(v)
	case *ast.ChanExpr:
		return e.emitChanExpr(v)
	case *ast.SendStmt:
		return e.emitSendStmt(v)
	case *ast.RecvExpr:
		return e.emitRecvExpr(v)
	case *ast.RecvOkExpr:
		return e.emitRecvOkExpr(v)
	case *ast.CloseStmt:
		return e.emitCloseStmt(v)
	case *ast.SelectStmt:
		return e.emitSelectStmt(v)
	case *ast.WithLockExpr:
		return e.emitWithLockExpr(v)
	case *ast.PipelineExpr:
		return e.emitPipelineExpr(v)
	case *ast.FanInExpr:
		return e.emitFanInExpr(v)
	case *ast.FanOutStmt:
		e.write("func() any {")
		e.nl()
		e.push()
		e.writeIndent()
		if err := e.emitFanOutStmt(v); err != nil {
			return err
		}
		e.nl()
		e.line("return nil")
		e.pop()
		e.writeIndent()
		e.write("}()")
		return nil
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
	case *ast.IfLetExpr:
		return e.emitIfLetExpr(v)
	case *ast.WhenLetExpr:
		return e.emitWhenLetExpr(v)
	case *ast.LetOrExpr:
		return e.emitLetOrExpr(v)
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
	switch n.(type) {
	case *ast.NilLit, *ast.BoolLit, *ast.IntLit, *ast.FloatLit,
		*ast.StringLit, *ast.KeywordLit:
		// A bare scalar literal in statement position is a no-op, and an
		// expression statement like `nil` is illegal Go ("nil is not used").
		return nil
	}
	e.lineDir(n.Pos())
	switch v := n.(type) {
	case *ast.LetExpr:
		return e.emitLetStmt(v)
	case *ast.IfExpr:
		return e.emitIfStmt(v)
	case *ast.WhenExpr:
		return e.emitWhenStmt(v)
	case *ast.CondExpr:
		return e.emitCondStmt(v)
	case *ast.SwitchExpr:
		return e.emitSwitchStmt(v)
	case *ast.IfLetExpr:
		return e.emitIfLetStmt(v)
	case *ast.WhenLetExpr:
		return e.emitWhenLetStmt(v)
	case *ast.LetOrExpr:
		return e.emitLetOrStmt(v)
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
	case *ast.ParStmt:
		e.writeIndent()
		if err := e.emitParStmt(v); err != nil {
			return err
		}
		e.nl()
		return nil
	case *ast.ForChanStmt:
		e.writeIndent()
		if err := e.emitForChanStmt(v); err != nil {
			return err
		}
		e.nl()
		return nil
	case *ast.FanOutStmt:
		e.writeIndent()
		if err := e.emitFanOutStmt(v); err != nil {
			return err
		}
		e.nl()
		return nil
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
	case *ast.CallExpr:
		if sym, ok := v.Head.(*ast.Symbol); ok {
			switch sym.Name {
			case "fmt/println", "fmt/print", "println", "print":
				e.writeIndent()
				if err := e.emitFmtPrintCall(sym.Name, v.Args); err != nil {
					return err
				}
				e.nl()
				return nil
			case "log/info", "log/debug", "log/warn", "log/error":
				e.writeIndent()
				if err := e.emitSlogCall(sym.Name, v.Args); err != nil {
					return err
				}
				e.nl()
				return nil
			case "assert":
				// Statement position: bare guard, no IIFE.
				e.writeIndent()
				if err := e.emitAssertGuard(v); err != nil {
					return err
				}
				e.nl()
				return nil
			}
		}
		e.writeIndent()
		if err := e.emitExpr(v); err != nil {
			return err
		}
		e.nl()
		return nil
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
	saved := e.pushTypeScope()
	defer e.popTypeScope(saved)
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
	if err := e.emitCondition(n.Cond); err != nil {
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
	if err := e.emitCondition(n.Cond); err != nil {
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
		if err := e.emitCondition(clause.Test); err != nil {
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
// Statement-only nodes (GoStmt, DeferStmt, SendStmt, CloseStmt, SelectStmt, …)
// have no value: they're emitted as statements followed by `return nil`, so a
// (go ...) or (select! ...) in tail position never leaves a missing return.
func (e *Emitter) emitReturnNode(n ast.Node) error {
	e.lineDir(n.Pos())
	switch v := n.(type) {
	case *ast.SelectStmt:
		e.writeIndent()
		if err := e.emitSelectStmt(v); err != nil {
			return err
		}
		e.nl()
		e.line("return nil")
		return nil
	case *ast.GoStmt:
		e.writeIndent()
		if err := e.emitGoStmt(v); err != nil {
			return err
		}
		e.line("return nil")
		return nil
	case *ast.ParStmt:
		e.writeIndent()
		if err := e.emitParStmt(v); err != nil {
			return err
		}
		e.nl()
		e.line("return nil")
		return nil
	case *ast.ForChanStmt:
		e.writeIndent()
		if err := e.emitForChanStmt(v); err != nil {
			return err
		}
		e.nl()
		e.line("return nil")
		return nil
	case *ast.FanOutStmt:
		e.writeIndent()
		if err := e.emitFanOutStmt(v); err != nil {
			return err
		}
		e.nl()
		e.line("return nil")
		return nil
	case *ast.DeferStmt:
		e.writeIndent()
		if err := e.emitDeferStmt(v); err != nil {
			return err
		}
		e.line("return nil")
		return nil
	case *ast.SendStmt:
		e.writeIndent()
		if err := e.emitSendStmt(v); err != nil {
			return err
		}
		e.nl()
		e.line("return nil")
		return nil
	case *ast.CloseStmt:
		e.writeIndent()
		if err := e.emitCloseStmt(v); err != nil {
			return err
		}
		e.nl()
		e.line("return nil")
		return nil
	case *ast.ReturnExpr:
		e.writeIndent()
		return e.emitReturnExpr(v)
	case *ast.IfErrExpr:
		return e.emitIfErrExprReturn(v)
	case *ast.IfLetExpr:
		return e.emitIfLetReturn(v)
	case *ast.WhenLetExpr:
		return e.emitWhenLetReturn(v)
	case *ast.LetOrExpr:
		return e.emitLetOrReturn(v)
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
	case *ast.SwitchExpr:
		return e.emitSwitchExprReturn(v)
	case *ast.DoExpr:
		return e.emitDoExprReturn(v)
	case *ast.LetExpr:
		return e.emitLetExprReturn(v)
	case *ast.CallExpr:
		if sym, ok := v.Head.(*ast.Symbol); ok {
			switch sym.Name {
			case "fmt/println", "fmt/print", "println", "print":
				e.writeIndent()
				if err := e.emitFmtPrintCall(sym.Name, v.Args); err != nil {
					return err
				}
				e.nl()
				e.writeIndent()
				e.write("return nil\n")
				return nil
			case "log/info", "log/debug", "log/warn", "log/error":
				e.writeIndent()
				if err := e.emitSlogCall(sym.Name, v.Args); err != nil {
					return err
				}
				e.nl()
				e.writeIndent()
				e.write("return nil\n")
				return nil
			case "assert":
				// Return/tail position: guard, then return nil.
				e.writeIndent()
				if err := e.emitAssertGuard(v); err != nil {
					return err
				}
				e.nl()
				e.writeIndent()
				e.write("return nil\n")
				return nil
			case "panic":
				// panic never returns; `return panic(...)` is invalid Go and
				// a bare panic satisfies Go's termination analysis.
				e.writeIndent()
				if err := e.emitCallExpr(v); err != nil {
					return err
				}
				e.nl()
				return nil
			}
		}
		// A void-returning call (os/exit, a user `-> void` fn/method) can't be a
		// return value: emit the bare statement, then `return nil` — mirroring the
		// statement-only-form rule above. Fixes `(when c (os/exit 0))` in tail
		// position emitting an invalid `return os.Exit(0)`.
		if e.isVoidCall(v) {
			e.writeIndent()
			if err := e.emitCallExpr(v); err != nil {
				return err
			}
			e.nl()
			e.writeIndent()
			e.write("return nil\n")
			return nil
		}
		// `return f()` from a multi-return fn is legal Go; everywhere else a
		// known multi-return call can't be a single return value — diagnose.
		if !strings.Contains(e.currentRetType, ",") {
			if err := e.checkMultiReturnValue(v); err != nil {
				return err
			}
		}
		e.writeIndent()
		e.write("return ")
		if e.currentRetType != "" {
			if err := e.emitExprWithHint(v, e.currentRetType); err != nil {
				return err
			}
		} else if err := e.emitExpr(v); err != nil {
			return err
		}
		e.nl()
		return nil
	default:
		e.writeIndent()
		e.write("return ")
		if e.currentRetType != "" {
			if err := e.emitExprWithHint(n, e.currentRetType); err != nil {
				return err
			}
		} else if err := e.emitExpr(n); err != nil {
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
