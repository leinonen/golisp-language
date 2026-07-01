package transpiler

import (
	"fmt"
	"strings"

	"golisp/internal/ast"
)

// emitGoStmt: (go body...) → go func() { body }()
func (e *Emitter) emitGoStmt(n *ast.GoStmt) error {
	e.write("go func() {")
	e.nl()
	e.push()
	if err := e.emitBody(n.Body, false); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}()")
	e.nl()
	return nil
}

// emitDeferStmt: (defer expr) → defer expr
func (e *Emitter) emitDeferStmt(n *ast.DeferStmt) error {
	e.write("defer ")
	if err := e.emitExpr(n.Expr); err != nil {
		return err
	}
	e.nl()
	return nil
}

// emitChanExpr: (chan ^T cap?) → make(chan T) or make(chan T, cap)
func (e *Emitter) emitChanExpr(n *ast.ChanExpr) error {
	elemType := typeExprToGo(n.ElemType.Text)
	if n.Cap == nil {
		e.writef("make(chan %s)", elemType)
	} else {
		e.writef("make(chan %s, ", elemType)
		if err := e.emitExpr(n.Cap); err != nil {
			return err
		}
		e.write(")")
	}
	return nil
}

// emitSendStmt: (send! ch val) → ch <- val
func (e *Emitter) emitSendStmt(n *ast.SendStmt) error {
	if err := e.emitExpr(n.Chan); err != nil {
		return err
	}
	e.write(" <- ")
	if err := e.emitExpr(n.Val); err != nil {
		return err
	}
	return nil
}

// emitRecvExpr: (recv! ch) → <-ch
func (e *Emitter) emitRecvExpr(n *ast.RecvExpr) error {
	e.write("<-")
	return e.emitExpr(n.Chan)
}

// emitCloseStmt: (close! ch) → close(ch)
func (e *Emitter) emitCloseStmt(n *ast.CloseStmt) error {
	e.write("close(")
	if err := e.emitExpr(n.Chan); err != nil {
		return err
	}
	e.write(")")
	return nil
}

// emitGoValExpr: (go-val [T] body...) → IIFE creating a buffered chan T, firing a
// goroutine that sends the result, returning the channel for later recv!.
// When ElemType is nil, the channel element type defaults to any.
func (e *Emitter) emitGoValExpr(n *ast.GoValExpr) error {
	elemType := "any"
	if n.ElemType != nil {
		elemType = typeExprToGo(n.ElemType.Text)
	}
	e.writef("func() chan %s {", elemType)
	e.nl()
	e.push()
	e.linef("_ch := make(chan %s, 1)", elemType)
	e.writeIndent()
	e.write("go func() {")
	e.nl()
	e.push()
	e.writeIndent()
	e.writef("_ch <- func() %s {", elemType)
	e.nl()
	e.push()
	// The body's tail value is sent on a `chan T`, so emit it under the element
	// type as the return hint — an `any` tail (e.g. (:body resp)) is then coerced
	// to T (_glispToString / numeric coercion / assertion) instead of producing a
	// raw "cannot use any as T" Go error.
	savedRet := e.currentRetType
	e.currentRetType = elemType
	err := e.emitBody(n.Body, true)
	e.currentRetType = savedRet
	if err != nil {
		return err
	}
	e.pop()
	e.line("}()")
	e.pop()
	e.line("}()")
	e.line("return _ch")
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitParStmt: (par body1 body2 ...) → sync.WaitGroup block, all bodies run in
// parallel goroutines, blocks until all complete.
func (e *Emitter) emitParStmt(n *ast.ParStmt) error {
	e.needImport("sync")
	e.write("{")
	e.nl()
	e.push()
	e.line("var _wg sync.WaitGroup")
	e.writeIndent()
	e.writef("_wg.Add(%d)", len(n.Bodies))
	e.nl()
	for _, body := range n.Bodies {
		e.writeIndent()
		e.write("go func() {")
		e.nl()
		e.push()
		e.line("defer _wg.Done()")
		if err := e.emitStmtNode(body); err != nil {
			return err
		}
		e.pop()
		e.line("}()")
	}
	e.line("_wg.Wait()")
	e.pop()
	e.writeIndent()
	e.write("}")
	return nil
}

// emitForChanStmt: (for-chan [x ch] body...) → for x := range ch { body }
// Iterates until the channel is closed.
func (e *Emitter) emitForChanStmt(n *ast.ForChanStmt) error {
	goName := identToGo(n.Binding.Name)
	e.writef("for %s := range ", goName)
	if err := e.emitExpr(n.Chan); err != nil {
		return err
	}
	e.write(" {")
	e.nl()
	e.push()
	if err := e.emitBody(n.Body, false); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}")
	return nil
}

// emitRecvOkExpr: (recv-ok! ch) → []any{val, ok} from comma-ok channel receive.
// Use with [[val ok] (recv-ok! ch)] destructuring in let.
func (e *Emitter) emitRecvOkExpr(n *ast.RecvOkExpr) error {
	e.write("func() []any { _v, _ok := <-")
	if err := e.emitExpr(n.Chan); err != nil {
		return err
	}
	e.write("; return []any{_v, _ok} }()")
	return nil
}

// emitWithLockExpr: (with-lock mu body...) → IIFE with Lock()/defer Unlock().
func (e *Emitter) emitWithLockExpr(n *ast.WithLockExpr) error {
	e.needImport("sync")
	e.write("func() any {")
	e.nl()
	e.push()
	e.writeIndent()
	if err := e.emitExpr(n.Mutex); err != nil {
		return err
	}
	e.write(".Lock()")
	e.nl()
	e.writeIndent()
	e.write("defer ")
	if err := e.emitExpr(n.Mutex); err != nil {
		return err
	}
	e.write(".Unlock()")
	e.nl()
	if err := e.emitBody(n.Body, true); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitWithOpenExpr: (with-open [f resource ...] body...) → an IIFE that binds
// each resource and `defer`s _glispClose on it, so Close() runs when the form
// exits (the IIFE gives the defers function scope — a bare Go block would defer
// to the *enclosing* function's return instead). Returns the body's value.
func (e *Emitter) emitWithOpenExpr(n *ast.WithOpenExpr) error {
	e.write("func() any {")
	e.nl()
	e.push()
	if err := e.emitWithOpenInner(n); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitWithOpenInner emits the binding+defer+body statements of a with-open,
// without the surrounding closure — shared by the plain `func() any` IIFE and
// the typed-return IIFE (emitTypedIIFE). Each binding is emitted then its defer,
// so a panic while opening a later resource still closes the earlier ones.
func (e *Emitter) emitWithOpenInner(n *ast.WithOpenExpr) error {
	e.needImport("_close")
	saved := e.pushTypeScope()
	defer e.popTypeScope(saved)
	for _, b := range n.Bindings {
		sym, ok := b.Pattern.(*ast.Symbol)
		if !ok {
			return fmt.Errorf("with-open binding must be a name (at %s)", b.Value.Pos())
		}
		if err := e.emitLetBindings([]ast.LetBinding{b}); err != nil {
			return err
		}
		e.linef("defer _glispClose(%s)", identToGo(sym.Name))
	}
	return e.emitBody(n.Body, true)
}

// emitDotoExpr: (doto obj form...) → an IIFE that binds obj to a temp once,
// runs each form with the temp threaded in (as the receiver of a (.method …)
// step, else as the first argument), and returns the temp. The temp name is
// identToGo-stable (no hyphens/special chars), so it can also seed the threaded
// AST as a plain symbol. obj's inferred struct type is registered on the temp so
// dot-free method dispatch resolves on it inside the steps.
func (e *Emitter) emitDotoExpr(n *ast.DotoExpr) error {
	if err := e.checkMultiReturnValue(n.Object); err != nil {
		return err
	}
	tmp := e.fresh("dotoTgt")
	saved := e.pushTypeScope()
	defer e.popTypeScope(saved)
	e.write("func() any {")
	e.nl()
	e.push()
	e.writeIndent()
	e.writef("%s := ", tmp)
	if err := e.emitExpr(n.Object); err != nil {
		return err
	}
	e.nl()
	e.registerLocalVar(tmp)
	// Carry obj's struct/interface type to the temp so dot-free method dispatch
	// (a bare (method …) step) resolves on it. inferValueStructType covers struct
	// literals / typed-return calls; a bare object symbol already in localTypes
	// (typed param or let binding) is propagated directly.
	st := e.inferValueStructType(n.Object)
	if st == "" {
		if sym, ok := n.Object.(*ast.Symbol); ok {
			st = e.localTypes[sym.Name]
		}
	}
	if st != "" && e.localTypes != nil {
		e.localTypes[tmp] = st
	}
	tmpSym := ast.NewSymbol(n.Pos(), tmp)
	for _, step := range n.Steps {
		threaded, err := dotoThread(tmpSym, step)
		if err != nil {
			return err
		}
		if err := e.emitStmtNode(threaded); err != nil {
			return err
		}
	}
	e.linef("return %s", tmp)
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// dotoThread rewrites one doto step to insert target as its receiver — for a
// (.method …) step, a CallExpr headed by a "."-prefixed symbol — or as its
// first argument (any other call, or a bare function symbol).
func dotoThread(target ast.Node, step ast.Node) (ast.Node, error) {
	switch f := step.(type) {
	case *ast.Symbol:
		return ast.NewCallExpr(f.Pos_, f, []ast.Node{target}), nil
	case *ast.CallExpr:
		if head, ok := f.Head.(*ast.Symbol); ok && strings.HasPrefix(head.Name, ".") {
			if strings.HasPrefix(head.Name, ".-") {
				return nil, fmt.Errorf("doto step cannot be a field access (%s) (at %s)", head.Name, f.Pos())
			}
			return ast.NewMethodCallExpr(f.Pos_, head.Name[1:], target, f.Args), nil
		}
		newArgs := append([]ast.Node{target}, f.Args...)
		return ast.NewCallExpr(f.Pos_, f.Head, newArgs), nil
	default:
		return nil, fmt.Errorf("doto step must be a call or symbol, got %T (at %s)", step, step.Pos())
	}
}

// emitSelectStmt emits a select statement.
func (e *Emitter) emitSelectStmt(n *ast.SelectStmt) error {
	for _, sc := range n.Cases {
		if sc.IsTimeout {
			e.needImport("time")
			break
		}
	}
	e.write("select {")
	e.nl()
	for _, sc := range n.Cases {
		if sc.IsDefault {
			e.line("default:")
			e.push()
			if err := e.emitBody(sc.Body, false); err != nil {
				return err
			}
			e.pop()
		} else if sc.IsTimeout {
			e.writeIndent()
			e.write("case <-time.After(")
			if err := e.emitExpr(sc.TimeoutMs); err != nil {
				return err
			}
			e.write(" * time.Millisecond):")
			e.nl()
			e.push()
			if err := e.emitBody(sc.Body, false); err != nil {
				return err
			}
			e.pop()
		} else if sc.IsSend {
			e.writeIndent()
			e.write("case ")
			if err := e.emitExpr(sc.ChanExpr); err != nil {
				return err
			}
			e.write(" <- ")
			if err := e.emitExpr(sc.SendVal); err != nil {
				return err
			}
			e.write(":")
			e.nl()
			e.push()
			if err := e.emitBody(sc.Body, false); err != nil {
				return err
			}
			e.pop()
		} else {
			e.writeIndent()
			if sc.Binding != "" && sc.Binding != "_" {
				e.writef("case %s := <-", sc.Binding)
			} else {
				// No binding (or `_`): `case _ := <-ch` is illegal Go.
				e.write("case <-")
			}
			if err := e.emitExpr(sc.ChanExpr); err != nil {
				return err
			}
			e.write(":")
			e.nl()
			e.push()
			if err := e.emitBody(sc.Body, false); err != nil {
				return err
			}
			e.pop()
		}
	}
	e.writeIndent()
	e.write("}")
	return nil
}

// emitIfErrExpr emits an if-err form in expression position (IIFE).
func (e *Emitter) emitIfErrExpr(n *ast.IfErrExpr) error {
	e.write("func() any {")
	e.nl()
	e.push()
	if err := e.emitIfErrExprReturn(n); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitIfErrExprReturn emits if-err in return position (no closure wrapper).
func (e *Emitter) emitIfErrExprReturn(n *ast.IfErrExpr) error {
	goVal := identToGo(n.ValName)
	goErr := identToGo(n.ErrName)
	e.writeIndent()
	e.writef("%s, %s := ", goVal, goErr)
	if err := e.emitExpr(n.Expr); err != nil {
		return err
	}
	e.nl()
	e.writeIndent()
	e.writef("if %s != nil {", goErr)
	e.nl()
	e.push()
	if err := e.emitReturnNode(n.OnErr); err != nil {
		return err
	}
	e.pop()
	e.line("}")
	// Wrap ok-branch in a block when it is another if-err so that nested
	// if-err chains with the same error variable name don't cause "no new
	// variables on left side of :=" errors in Go.
	if _, nested := n.OnOk.(*ast.IfErrExpr); nested {
		e.line("{")
		e.push()
		if err := e.emitReturnNode(n.OnOk); err != nil {
			return err
		}
		e.pop()
		e.line("}")
		return nil
	}
	return e.emitReturnNode(n.OnOk)
}

// emitTryExpr: (try body... (catch e h...) (finally c...)) → an IIFE whose
// deferred funcs implement catch (via recover) and finally. Go runs deferred
// funcs LIFO, so finally is deferred FIRST (runs last) and the recover handler
// SECOND (runs first): the catch handler runs, then finally. The IIFE returns
// the body's value, or the catch handler's value when a panic is caught. The
// body and catch handler are each wrapped in their own `func() any` so the
// existing return-position machinery yields their last expression's value.
func (e *Emitter) emitTryExpr(n *ast.TryExpr) error {
	e.write("func() (_tryResult any) {")
	e.nl()
	e.push()
	if n.Finally != nil {
		e.line("defer func() {")
		e.push()
		if err := e.emitBody(n.Finally, false); err != nil {
			return err
		}
		e.pop()
		e.line("}()")
	}
	if n.HasCatch {
		e.line("defer func() {")
		e.push()
		e.line("if _r := recover(); _r != nil {")
		e.push()
		saved := e.pushTypeScope()
		if n.CatchBinding != "_" && n.CatchBinding != "" {
			bind := identToGo(n.CatchBinding)
			e.linef("%s := _r", bind)
			e.linef("_ = %s", bind)
			e.registerAnyVar(n.CatchBinding)
		}
		e.writeIndent()
		e.write("_tryResult = func() any {")
		e.nl()
		e.push()
		if err := e.emitBody(n.CatchBody, true); err != nil {
			e.popTypeScope(saved)
			return err
		}
		e.pop()
		e.line("}()")
		e.popTypeScope(saved)
		e.pop()
		e.line("}")
		e.pop()
		e.line("}()")
	}
	e.writeIndent()
	e.write("_tryResult = func() any {")
	e.nl()
	e.push()
	if err := e.emitBody(n.Body, true); err != nil {
		return err
	}
	e.pop()
	e.line("}()")
	e.line("return")
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitPipelineExpr: (pipeline [x src] stage1 stage2 ...) → IIFE chaining goroutines via channels.
// Each stage reads from the previous channel, transforms x, sends to the next channel.
// Returns the final chan any.
func (e *Emitter) emitPipelineExpr(n *ast.PipelineExpr) error {
	binding := identToGo(n.Binding.Name)
	e.write("func() chan any {")
	e.nl()
	e.push()
	e.writeIndent()
	e.write("_pipe0 := ")
	if err := e.emitExpr(n.Source); err != nil {
		return err
	}
	e.nl()
	for i, stage := range n.Stages {
		inPipe := fmt.Sprintf("_pipe%d", i)
		outPipe := fmt.Sprintf("_pipe%d", i+1)
		e.writeIndent()
		e.writef("%s := make(chan any)", outPipe)
		e.nl()
		e.writeIndent()
		e.write("go func() {")
		e.nl()
		e.push()
		e.writeIndent()
		e.writef("defer close(%s)", outPipe)
		e.nl()
		e.writeIndent()
		e.writef("for %s := range %s {", binding, inPipe)
		e.nl()
		e.push()
		e.writeIndent()
		e.writef("%s <- func() any { return ", outPipe)
		if err := e.emitExpr(stage); err != nil {
			return err
		}
		e.write(" }()")
		e.nl()
		e.pop()
		e.line("}")
		e.pop()
		e.line("}()")
	}
	e.writeIndent()
	e.writef("return _pipe%d", len(n.Stages))
	e.nl()
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitFanOutStmt: (fan-out n [item ch] body...) → n goroutines draining ch via WaitGroup.
// Blocks until ch is closed and all workers finish.
func (e *Emitter) emitFanOutStmt(n *ast.FanOutStmt) error {
	e.needImport("sync")
	binding := identToGo(n.Binding.Name)
	e.write("{")
	e.nl()
	e.push()
	e.writeIndent()
	e.write("_fanN := int(")
	if err := e.emitExpr(n.N); err != nil {
		return err
	}
	e.write(")")
	e.nl()
	e.line("var _wg sync.WaitGroup")
	e.line("_wg.Add(_fanN)")
	e.writeIndent()
	e.write("for _fanI := 0; _fanI < _fanN; _fanI++ {")
	e.nl()
	e.push()
	e.writeIndent()
	e.write("go func() {")
	e.nl()
	e.push()
	e.line("defer _wg.Done()")
	e.writeIndent()
	e.writef("for %s := range ", binding)
	if err := e.emitExpr(n.Chan); err != nil {
		return err
	}
	e.write(" {")
	e.nl()
	e.push()
	if err := e.emitBody(n.Body, false); err != nil {
		return err
	}
	e.pop()
	e.line("}")
	e.pop()
	e.line("}()")
	e.pop()
	e.line("}")
	e.line("_wg.Wait()")
	e.pop()
	e.writeIndent()
	e.write("}")
	return nil
}

// emitFanInExpr: (fan-in ch1 ch2 ...) → IIFE merging N chan any inputs into one chan any.
// Input channels must be chan any. Closes output when all inputs are exhausted.
func (e *Emitter) emitFanInExpr(n *ast.FanInExpr) error {
	e.needImport("sync")
	e.write("func() chan any {")
	e.nl()
	e.push()
	e.line("_out := make(chan any)")
	e.line("var _wg sync.WaitGroup")
	e.writeIndent()
	e.writef("_wg.Add(%d)", len(n.Chans))
	e.nl()
	e.writeIndent()
	e.write("_fanInMerge := func(_c chan any) {")
	e.nl()
	e.push()
	e.line("defer _wg.Done()")
	e.writeIndent()
	e.write("for _v := range _c {")
	e.nl()
	e.push()
	e.line("_out <- _v")
	e.pop()
	e.line("}")
	e.pop()
	e.line("}")
	for _, ch := range n.Chans {
		e.writeIndent()
		e.write("go _fanInMerge(")
		if err := e.emitExpr(ch); err != nil {
			return err
		}
		e.write(")")
		e.nl()
	}
	e.writeIndent()
	e.write("go func() {")
	e.nl()
	e.push()
	e.line("_wg.Wait()")
	e.line("close(_out)")
	e.pop()
	e.line("}()")
	e.line("return _out")
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitTypeAssertExpr: (as ^T val) → val.(T)
func (e *Emitter) emitTypeAssertExpr(n *ast.TypeAssertExpr) error {
	if err := e.emitExpr(n.Value); err != nil {
		return err
	}
	e.writef(".(%s)", typeExprToGo(n.Type.Text))
	return nil
}

// emitMethodCallExpr: (.Method obj args...) → obj.Method(args...)
func (e *Emitter) emitMethodCallExpr(n *ast.MethodCallExpr) error {
	if err := e.emitExpr(n.Object); err != nil {
		return err
	}
	e.writef(".%s(", n.Method)
	for i, arg := range n.Args {
		if i > 0 {
			e.write(", ")
		}
		if err := e.emitExpr(arg); err != nil {
			return err
		}
	}
	e.write(")")
	return nil
}

// emitFieldAccessExpr: (.-Field obj) → obj.Field. When obj is a symbol whose
// external Go type is known, a field naming no exported field of that type is a
// position-tagged diagnostic (ADR-015, Phase 12e) instead of an opaque Go error.
func (e *Emitter) emitFieldAccessExpr(n *ast.FieldAccessExpr) error {
	if sym, ok := n.Object.(*ast.Symbol); ok {
		if typeName := e.localTypes[sym.Name]; typeName != "" {
			if fs := e.goFieldSet(typeName); fs != nil {
				if _, found := fs[n.Field]; !found {
					return fmt.Errorf("type %s has no exported field %s (at %s)", typeName, n.Field, n.Pos())
				}
			}
		}
	}
	if err := e.emitExpr(n.Object); err != nil {
		return err
	}
	e.writef(".%s", n.Field)
	return nil
}

// emitStructLitExpr: (TypeName. {:field val}) → TypeName{Field: val}
func (e *Emitter) emitStructLitExpr(n *ast.StructLitExpr) error {
	typeName := identToGo(n.TypeName)
	if idx := strings.Index(n.TypeName, "/"); idx > 0 {
		pkg := n.TypeName[:idx]
		if !e.isModuleAlias(pkg) {
			// Resolve the qualifier through the stdlib map so (http/Client. {}) →
			// net/http, not a bogus bare `import "http"`. Mirrors qualified-symbol
			// resolution; reached only for non-module qualifiers.
			if err := e.resolveDirectImportAt(pkg, n.Pos()); err != nil {
				return err
			}
		}
	}
	e.writef("%s{", typeName)
	for i, pair := range n.Fields {
		if i > 0 {
			e.write(", ")
		}
		// Key must be a keyword; convert to Go field name (Title case)
		switch k := pair.Key.(type) {
		case *ast.KeywordLit:
			e.writef("%s: ", titleCase(identToGo(k.Value)))
		case *ast.Symbol:
			e.writef("%s: ", titleCase(identToGo(k.Name)))
		default:
			return fmt.Errorf("struct literal field key must be keyword or symbol, got %T", pair.Key)
		}
		if err := e.emitExpr(pair.Value); err != nil {
			return err
		}
	}
	e.write("}")
	return nil
}
