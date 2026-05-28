package transpiler

import (
	"fmt"
	"strings"

	"golisp/internal/ast"
)

// emitVectorLit emits []T{...} or []any{...}
func (e *Emitter) emitVectorLit(n *ast.VectorLit) error {
	typeStr := "any"
	if n.TypeAnnot != nil {
		// TypeAnnot on VectorLit is the element type, wrapped in []
		t := typeExprToGo(n.TypeAnnot.Text)
		// If the annotation is already []T, use it directly
		if strings.HasPrefix(t, "[]") {
			typeStr = t[2:] // strip leading []
		} else {
			typeStr = t
		}
	}
	e.writef("[]%s{", typeStr)
	for i, el := range n.Elements {
		if i > 0 {
			e.write(", ")
		}
		if err := e.emitExpr(el); err != nil {
			return err
		}
	}
	e.write("}")
	return nil
}

// emitMapLit emits map[string]any{...} or map[K]V{...}
func (e *Emitter) emitMapLit(n *ast.MapLit) error {
	mapType := "map[string]any"
	if n.TypeAnnot != nil {
		mapType = typeExprToGo(n.TypeAnnot.Text)
	}
	e.writef("%s{", mapType)
	for i, pair := range n.Pairs {
		if i > 0 {
			e.write(", ")
		}
		if err := e.emitExpr(pair.Key); err != nil {
			return err
		}
		e.write(": ")
		if err := e.emitExpr(pair.Value); err != nil {
			return err
		}
	}
	e.write("}")
	return nil
}

// emitFnExpr emits an anonymous function literal.
// fn always returns any by default — every glisp expression has a value.
// Use ^void annotation to suppress the return type (for side-effect-only fns).
func (e *Emitter) emitFnExpr(n *ast.FnExpr) error {
	params, err := e.formatParams(n.Params)
	if err != nil {
		return err
	}
	retStr := e.formatReturnType(n.ReturnType)
	isVoid := retStr == "void"
	if isVoid {
		retStr = ""
	} else if retStr == "" {
		retStr = "any"
	}
	if retStr != "" {
		e.writef("func(%s) %s {", params, retStr)
	} else {
		e.writef("func(%s) {", params)
	}
	e.nl()
	e.push()
	if err := e.emitBody(n.Body, !isVoid); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}")
	return nil
}

// emitLetExpr emits a let binding block, returning the last expression.
// Inline approach: emit as a func closure that is immediately invoked.
// Simpler approach: emit bindings as := statements in a block.
// We use the block approach since closures add overhead and complexity.
// The result variable captures the return value.
func (e *Emitter) emitLetExpr(n *ast.LetExpr) error {
	// Let in expression position: use an immediately-invoked closure.
	// This is the only clean way to make let return a value in Go.
	e.write("func() any {")
	e.nl()
	e.push()
	if err := e.emitLetBody(n); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitLetExprReturn emits let in return position (no wrapping closure needed).
func (e *Emitter) emitLetExprReturn(n *ast.LetExpr) error {
	if err := e.emitLetBindings(n.Bindings); err != nil {
		return err
	}
	return e.emitBody(n.Body, true)
}

func (e *Emitter) emitLetBody(n *ast.LetExpr) error {
	if err := e.emitLetBindings(n.Bindings); err != nil {
		return err
	}
	return e.emitBody(n.Body, true)
}

func (e *Emitter) emitLetBindings(bindings []ast.LetBinding) error {
	for _, b := range bindings {
		switch pat := b.Pattern.(type) {
		case *ast.Symbol:
			goName := identToGo(pat.Name)
			typeStr := ""
			if b.TypeAnnot != nil {
				typeStr = typeExprToGo(b.TypeAnnot.Text)
			}
			e.writeIndent()
			if typeStr != "" {
				e.writef("var %s %s = ", goName, typeStr)
			} else {
				e.writef("%s := ", goName)
			}
			if err := e.emitExpr(b.Value); err != nil {
				return err
			}
			e.nl()
		case *ast.VectorLit:
			// Multi-value destructure: [[a b] expr] → a, b := expr
			names := make([]string, len(pat.Elements))
			for i, el := range pat.Elements {
				sym, ok := el.(*ast.Symbol)
				if !ok {
					return fmt.Errorf("multi-value destructure pattern must be symbols, got %T", el)
				}
				names[i] = identToGo(sym.Name)
			}
			e.writeIndent()
			e.writef("%s := ", strings.Join(names, ", "))
			if err := e.emitExpr(b.Value); err != nil {
				return err
			}
			e.nl()
		default:
			return fmt.Errorf("unsupported let pattern: %T", b.Pattern)
		}
	}
	return nil
}

// emitIfExpr emits an if expression.
// In expression position, we use an immediately-invoked closure.
func (e *Emitter) emitIfExpr(n *ast.IfExpr) error {
	// Expression position: IIFE
	e.write("func() any {")
	e.nl()
	e.push()
	if err := e.emitIfExprReturn(n); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

func (e *Emitter) emitIfExprReturn(n *ast.IfExpr) error {
	e.writeIndent()
	e.write("if ")
	if err := e.emitExpr(n.Cond); err != nil {
		return err
	}
	e.write(" {")
	e.nl()
	e.push()
	if err := e.emitReturnNode(n.Then); err != nil {
		return err
	}
	e.pop()
	if n.Else != nil {
		e.line("} else {")
		e.push()
		if err := e.emitReturnNode(n.Else); err != nil {
			return err
		}
		e.pop()
	}
	e.line("}")
	return nil
}

// emitWhenExpr emits a when block (no else).
func (e *Emitter) emitWhenExpr(n *ast.WhenExpr) error {
	e.write("func() any {")
	e.nl()
	e.push()
	e.writeIndent()
	e.write("if ")
	if err := e.emitExpr(n.Cond); err != nil {
		return err
	}
	e.write(" {")
	e.nl()
	e.push()
	if err := e.emitBody(n.Body, true); err != nil {
		return err
	}
	e.pop()
	e.line("}")
	e.line("return nil")
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitCondExpr emits a cond expression.
func (e *Emitter) emitCondExpr(n *ast.CondExpr) error {
	e.write("func() any {")
	e.nl()
	e.push()
	if err := e.emitCondExprReturn(n); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

func (e *Emitter) emitCondExprReturn(n *ast.CondExpr) error {
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
		if err := e.emitReturnNode(clause.Body); err != nil {
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
		if err := e.emitReturnNode(n.Default); err != nil {
			return err
		}
		e.pop()
	}
	if len(n.Clauses) > 0 || n.Default != nil {
		e.line("}")
	}
	return nil
}

// emitDoExpr emits a do block.
func (e *Emitter) emitDoExpr(n *ast.DoExpr) error {
	e.write("func() any {")
	e.nl()
	e.push()
	if err := e.emitBody(n.Body, true); err != nil {
		return err
	}
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

func (e *Emitter) emitDoExprReturn(n *ast.DoExpr) error {
	return e.emitBody(n.Body, true)
}

// emitCallExpr emits a function or operator call.
func (e *Emitter) emitCallExpr(n *ast.CallExpr) error {
	// Handle built-in operators
	if sym, ok := n.Head.(*ast.Symbol); ok {
		switch sym.Name {
		case "+", "-", "*", "/", "mod":
			return e.emitArith(sym.Name, n.Args)
		case "=":
			return e.emitBinOp("==", n.Args)
		case "not=":
			return e.emitBinOp("!=", n.Args)
		case "<", ">", "<=", ">=":
			return e.emitBinOp(sym.Name, n.Args)
		case "and":
			return e.emitLogicOp("&&", n.Args)
		case "or":
			return e.emitLogicOp("||", n.Args)
		case "not":
			if len(n.Args) != 1 {
				return fmt.Errorf("not requires 1 argument")
			}
			e.write("!(")
			if err := e.emitExpr(n.Args[0]); err != nil {
				return err
			}
			e.write(")")
			return nil
		case "str":
			return e.emitStr(n.Args)
		case "println", "print":
			return e.emitPrint(sym.Name, n.Args)
		case "get":
			return e.emitGet(n.Args)
		case "assoc":
			return e.emitAssoc(n.Args)
		case "dissoc":
			return e.emitDissoc(n.Args)
		case "conj":
			return e.emitConj(n.Args)
		case "count":
			return e.emitCount(n.Args)
		case "first":
			return e.emitFirst(n.Args)
		case "rest":
			return e.emitRest(n.Args)
		case "nth":
			return e.emitNth(n.Args)
		case "keys":
			return e.emitKeys(n.Args)
		case "vals":
			return e.emitVals(n.Args)
		case "merge":
			return e.emitMerge(n.Args)
		case "error":
			return e.emitError(n.Args)
		case "nil?":
			return e.emitNilQ(n.Args)
		case "string":
			return e.emitStringConv(n.Args)
		case "int":
			return e.emitIntConv(n.Args)
		case "float64":
			return e.emitFloat64Conv(n.Args)
		case "->":
			return e.emitThreadFirst(n.Args)
		case "->>":
			return e.emitThreadLast(n.Args)
		case "doseq":
			return e.emitDoseq(n.Args)
		case "dotimes":
			return e.emitDotimes(n.Args)
		// 2a: collection operations
		case "map":
			return e.emitRuntimeCall("_glispMap", n.Args, 2)
		case "filter":
			return e.emitRuntimeCall("_glispFilter", n.Args, 2)
		case "reduce":
			return e.emitRuntimeCall("_glispReduce", n.Args, 3)
		case "reverse":
			return e.emitRuntimeCall("_glispReverse", n.Args, 1)
		case "contains?":
			return e.emitRuntimeCall("_glispContains", n.Args, 2)
		case "some":
			return e.emitRuntimeCall("_glispSome", n.Args, 2)
		case "every?":
			return e.emitRuntimeCall("_glispEvery", n.Args, 2)
		case "sort-by":
			e.needImport("sort")
			return e.emitRuntimeCall("_glispSortBy", n.Args, 2)
		case "flatten":
			return e.emitRuntimeCall("_glispFlatten", n.Args, 1)
		case "range":
			return e.emitVariadicRuntimeCall("_glispRange", n.Args)
		case "take":
			return e.emitRuntimeCall("_glispTake", n.Args, 2)
		case "drop":
			return e.emitRuntimeCall("_glispDrop", n.Args, 2)
		// 2b: string operations
		case "upper-case":
			return e.emitStrOp("strings.ToUpper", n.Args, 1)
		case "lower-case":
			return e.emitStrOp("strings.ToLower", n.Args, 1)
		case "trim":
			return e.emitStrOp("strings.TrimSpace", n.Args, 1)
		case "starts-with?":
			return e.emitStrOp2("strings.HasPrefix", n.Args)
		case "ends-with?":
			return e.emitStrOp2("strings.HasSuffix", n.Args)
		case "replace":
			return e.emitReplace(n.Args)
		case "split":
			e.needImport("strings")
			return e.emitRuntimeCall("_glispSplit", n.Args, 2)
		case "join":
			e.needImport("strings")
			return e.emitRuntimeCall("_glispJoin", n.Args, 2)
		case "json/encode":
			e.needImport("encoding/json")
			return e.emitRuntimeCall("_glispJsonEncode", n.Args, 1)
		case "json/decode":
			e.needImport("encoding/json")
			return e.emitRuntimeCall("_glispJsonDecode", n.Args, 1)
		case "subs":
			return e.emitSubs(n.Args)
		}
	}

	// General function call: f(args...)
	if err := e.emitExpr(n.Head); err != nil {
		return err
	}
	e.write("(")
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

func (e *Emitter) emitArith(op string, args []ast.Node) error {
	if op == "mod" {
		op = "%"
	}
	if len(args) == 1 {
		// unary minus
		e.write("(-")
		if err := e.emitExpr(args[0]); err != nil {
			return err
		}
		e.write(")")
		return nil
	}
	e.write("(")
	for i, arg := range args {
		if i > 0 {
			e.writef(" %s ", op)
		}
		if err := e.emitExpr(arg); err != nil {
			return err
		}
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitBinOp(op string, args []ast.Node) error {
	if len(args) != 2 {
		return fmt.Errorf("%s requires exactly 2 arguments", op)
	}
	e.write("(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.writef(" %s ", op)
	if err := e.emitExpr(args[1]); err != nil {
		return err
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitLogicOp(op string, args []ast.Node) error {
	if len(args) < 2 {
		return fmt.Errorf("logical operator requires at least 2 arguments")
	}
	e.write("(")
	for i, arg := range args {
		if i > 0 {
			e.writef(" %s ", op)
		}
		if err := e.emitExpr(arg); err != nil {
			return err
		}
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitStr(args []ast.Node) error {
	if len(args) == 0 {
		e.write(`""`)
		return nil
	}
	e.needImport("fmt")
	e.write("(")
	for i, arg := range args {
		if i > 0 {
			e.write(" + ")
		}
		e.write("fmt.Sprintf(\"%v\", ")
		if err := e.emitExpr(arg); err != nil {
			return err
		}
		e.write(")")
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitPrint(fn string, args []ast.Node) error {
	e.needImport("fmt")
	goFn := "fmt.Println"
	if fn == "print" {
		goFn = "fmt.Print"
	}
	e.writef("%s(", goFn)
	for i, arg := range args {
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

func (e *Emitter) emitGet(args []ast.Node) error {
	if len(args) < 2 || len(args) > 3 {
		return fmt.Errorf("get requires 2 or 3 arguments")
	}
	// Emit helper call: glispGet(m, key) or glispGetDefault(m, key, default)
	// For Phase 1 we inline the logic as a type-switch map lookup.
	// Simple version: m.(map[string]any)[key.(string)]
	// Better: emit a helper function call that we inject at the top of the file.
	// For now: use direct map indexing with a cast.
	if len(args) == 2 {
		e.write("_glispGet(")
		if err := e.emitExpr(args[0]); err != nil {
			return err
		}
		e.write(", ")
		if err := e.emitExpr(args[1]); err != nil {
			return err
		}
		e.write(")")
	} else {
		e.write("_glispGetD(")
		if err := e.emitExpr(args[0]); err != nil {
			return err
		}
		e.write(", ")
		if err := e.emitExpr(args[1]); err != nil {
			return err
		}
		e.write(", ")
		if err := e.emitExpr(args[2]); err != nil {
			return err
		}
		e.write(")")
	}
	return nil
}

func (e *Emitter) emitAssoc(args []ast.Node) error {
	if len(args) < 3 || len(args)%2 == 0 {
		return fmt.Errorf("assoc requires map + key-value pairs")
	}
	e.write("_glispAssoc(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	for i := 1; i < len(args); i += 2 {
		e.write(", ")
		if err := e.emitExpr(args[i]); err != nil {
			return err
		}
		e.write(", ")
		if err := e.emitExpr(args[i+1]); err != nil {
			return err
		}
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitDissoc(args []ast.Node) error {
	if len(args) < 2 {
		return fmt.Errorf("dissoc requires map + keys")
	}
	e.write("_glispDissoc(")
	for i, arg := range args {
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

func (e *Emitter) emitConj(args []ast.Node) error {
	if len(args) < 2 {
		return fmt.Errorf("conj requires collection + element(s)")
	}
	e.write("append(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(".([]any)")
	for _, arg := range args[1:] {
		e.write(", ")
		if err := e.emitExpr(arg); err != nil {
			return err
		}
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitCount(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("count requires 1 argument")
	}
	e.write("len(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(".([]any))")
	return nil
}

func (e *Emitter) emitFirst(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("first requires 1 argument")
	}
	e.write("(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(".([]any)[0])")
	return nil
}

func (e *Emitter) emitRest(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("rest requires 1 argument")
	}
	e.write("(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(".([]any)[1:])")
	return nil
}

func (e *Emitter) emitNth(args []ast.Node) error {
	if len(args) != 2 {
		return fmt.Errorf("nth requires 2 arguments")
	}
	e.write("(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(".([]any)[")
	if err := e.emitExpr(args[1]); err != nil {
		return err
	}
	e.write("])")
	return nil
}

func (e *Emitter) emitKeys(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("keys requires 1 argument")
	}
	e.write("_glispKeys(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitVals(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("vals requires 1 argument")
	}
	e.write("_glispVals(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitMerge(args []ast.Node) error {
	if len(args) < 2 {
		return fmt.Errorf("merge requires at least 2 maps")
	}
	e.write("_glispMerge(")
	for i, arg := range args {
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

func (e *Emitter) emitError(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("error requires 1 argument (message string)")
	}
	e.needImport("errors")
	e.needImport("fmt")
	e.write("errors.New(fmt.Sprintf(\"%v\", ")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write("))")
	return nil
}

func (e *Emitter) emitNilQ(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("nil? requires 1 argument")
	}
	e.write("(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(" == nil)")
	return nil
}

func (e *Emitter) emitStringConv(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("string requires 1 argument")
	}
	e.write("string(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitIntConv(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("int requires 1 argument")
	}
	// Use _glispToInt so it works on any (e.g. values from range/map/filter).
	e.write("_glispToInt(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(")")
	return nil
}

func (e *Emitter) emitFloat64Conv(args []ast.Node) error {
	if len(args) != 1 {
		return fmt.Errorf("float64 requires 1 argument")
	}
	e.write("float64(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write(")")
	return nil
}

// emitThreadFirst: (-> x f1 f2) → f2(f1(x))
func (e *Emitter) emitThreadFirst(args []ast.Node) error {
	if len(args) < 2 {
		return fmt.Errorf("-> requires at least 2 forms")
	}
	node := args[0]
	for _, form := range args[1:] {
		switch f := form.(type) {
		case *ast.Symbol:
			// (-> x f) → f(x)
			node = ast.NewCallExpr(f.Pos_, f, []ast.Node{node})
		case *ast.CallExpr:
			// (-> x (f a b)) → f(x, a, b)
			newArgs := append([]ast.Node{node}, f.Args...)
			node = ast.NewCallExpr(f.Pos_, f.Head, newArgs)
		default:
			return fmt.Errorf("-> form must be a symbol or call, got %T", form)
		}
	}
	return e.emitExpr(node)
}

// emitThreadLast: (->> x f1 f2) → f2(f1(x)) but x is inserted last
func (e *Emitter) emitThreadLast(args []ast.Node) error {
	if len(args) < 2 {
		return fmt.Errorf("->> requires at least 2 forms")
	}
	node := args[0]
	for _, form := range args[1:] {
		switch f := form.(type) {
		case *ast.Symbol:
			node = ast.NewCallExpr(f.Pos_, f, []ast.Node{node})
		case *ast.CallExpr:
			newArgs := append(f.Args, node)
			node = ast.NewCallExpr(f.Pos_, f.Head, newArgs)
		default:
			return fmt.Errorf("->> form must be a symbol or call, got %T", form)
		}
	}
	return e.emitExpr(node)
}

// emitDoseq: (doseq [item coll] body...) → for _, item := range coll { body }
func (e *Emitter) emitDoseq(args []ast.Node) error {
	if len(args) < 2 {
		return fmt.Errorf("doseq requires binding vector + body")
	}
	bv, ok := args[0].(*ast.VectorLit)
	if !ok || len(bv.Elements) != 2 {
		return fmt.Errorf("doseq binding must be [item collection]")
	}
	sym, ok := bv.Elements[0].(*ast.Symbol)
	if !ok {
		return fmt.Errorf("doseq binding name must be a symbol")
	}
	goName := identToGo(sym.Name)
	e.writef("func() {")
	e.nl()
	e.push()
	e.writeIndent()
	e.writef("for _, %s := range ", goName)
	if err := e.emitExpr(bv.Elements[1]); err != nil {
		return err
	}
	e.write(".([]any) {")
	e.nl()
	e.push()
	for _, stmt := range args[1:] {
		e.writeIndent()
		if err := e.emitExpr(stmt); err != nil {
			return err
		}
		e.nl()
	}
	e.pop()
	e.line("}")
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitDotimes: (dotimes [i n] body...) → for i := 0; i < n; i++ { body }
func (e *Emitter) emitDotimes(args []ast.Node) error {
	if len(args) < 2 {
		return fmt.Errorf("dotimes requires binding vector + body")
	}
	bv, ok := args[0].(*ast.VectorLit)
	if !ok || len(bv.Elements) != 2 {
		return fmt.Errorf("dotimes binding must be [i n]")
	}
	sym, ok := bv.Elements[0].(*ast.Symbol)
	if !ok {
		return fmt.Errorf("dotimes binding name must be a symbol")
	}
	goName := identToGo(sym.Name)
	e.write("func() {")
	e.nl()
	e.push()
	e.writeIndent()
	e.writef("for %s := 0; %s < ", goName, goName)
	if err := e.emitExpr(bv.Elements[1]); err != nil {
		return err
	}
	e.writef("; %s++ {", goName)
	e.nl()
	e.push()
	for _, stmt := range args[1:] {
		e.writeIndent()
		if err := e.emitExpr(stmt); err != nil {
			return err
		}
		e.nl()
	}
	e.pop()
	e.line("}")
	e.pop()
	e.writeIndent()
	e.write("}()")
	return nil
}

// emitReturnExpr emits an explicit (return ...) form.
func (e *Emitter) emitReturnExpr(n *ast.ReturnExpr) error {
	if len(n.Args) == 0 {
		e.line("return")
		return nil
	}
	e.writeIndent()
	e.write("return ")
	for i, arg := range n.Args {
		if i > 0 {
			e.write(", ")
		}
		if err := e.emitExpr(arg); err != nil {
			return err
		}
	}
	e.nl()
	return nil
}

// emitRuntimeCall emits a call to a runtime helper with a fixed arity check.
func (e *Emitter) emitRuntimeCall(fn string, args []ast.Node, arity int) error {
	if len(args) != arity {
		return fmt.Errorf("%s requires %d argument(s), got %d", fn, arity, len(args))
	}
	e.writef("%s(", fn)
	for i, arg := range args {
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

// emitVariadicRuntimeCall emits a call to a runtime helper accepting any number of args.
func (e *Emitter) emitVariadicRuntimeCall(fn string, args []ast.Node) error {
	e.writef("%s(", fn)
	for i, arg := range args {
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

// emitStrOp emits a single-arg strings.Xxx(_glispToString(s)) call.
func (e *Emitter) emitStrOp(fn string, args []ast.Node, arity int) error {
	if len(args) != arity {
		return fmt.Errorf("%s requires %d argument(s)", fn, arity)
	}
	e.needImport("strings")
	e.writef("%s(_glispToString(", fn)
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write("))")
	return nil
}

// emitStrOp2 emits a two-arg strings.Xxx(_glispToString(s), _glispToString(t)) call.
func (e *Emitter) emitStrOp2(fn string, args []ast.Node) error {
	if len(args) != 2 {
		return fmt.Errorf("%s requires 2 arguments", fn)
	}
	e.needImport("strings")
	e.writef("%s(_glispToString(", fn)
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write("), _glispToString(")
	if err := e.emitExpr(args[1]); err != nil {
		return err
	}
	e.write("))")
	return nil
}

// emitReplace emits (replace s from to) → strings.ReplaceAll(s, from, to)
func (e *Emitter) emitReplace(args []ast.Node) error {
	if len(args) != 3 {
		return fmt.Errorf("replace requires 3 arguments: s from to")
	}
	e.needImport("strings")
	e.write("strings.ReplaceAll(_glispToString(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write("), _glispToString(")
	if err := e.emitExpr(args[1]); err != nil {
		return err
	}
	e.write("), _glispToString(")
	if err := e.emitExpr(args[2]); err != nil {
		return err
	}
	e.write("))")
	return nil
}

// emitSubs emits (subs s start) or (subs s start end)
func (e *Emitter) emitSubs(args []ast.Node) error {
	if len(args) < 2 || len(args) > 3 {
		return fmt.Errorf("subs requires 2 or 3 arguments")
	}
	e.write("(_glispToString(")
	if err := e.emitExpr(args[0]); err != nil {
		return err
	}
	e.write("))[")
	if err := e.emitExpr(args[1]); err != nil {
		return err
	}
	if len(args) == 3 {
		e.write(":")
		if err := e.emitExpr(args[2]); err != nil {
			return err
		}
	} else {
		e.write(":")
	}
	e.write("]")
	return nil
}

// emitValuesExpr emits (values a b) inline (used inside return statements).
func (e *Emitter) emitValuesExpr(n *ast.ValuesExpr) error {
	for i, arg := range n.Args {
		if i > 0 {
			e.write(", ")
		}
		if err := e.emitExpr(arg); err != nil {
			return err
		}
	}
	e.nl()
	return nil
}
