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
	if err := e.emitBody(n.Body, true); err != nil {
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

// emitFieldAccessExpr: (.-Field obj) → obj.Field
func (e *Emitter) emitFieldAccessExpr(n *ast.FieldAccessExpr) error {
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
			e.directImports[pkg] = true
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
