// Package formatter pretty-prints glisp source from a parsed AST.
// ;;; doc comments are preserved via DefnDecl.Doc / MethodDecl.Doc.
// ; and ;; comments are preserved via a CommentMap from the parser and emitted
// in place — both as leading comments of top-level forms and interleaved within
// form bodies (see cfmt) — rather than relocated to the next form or EOF.
// Trailing inline comments (after code on the same line) are not preserved.
package formatter

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"golisp/internal/ast"
	"golisp/internal/parser"
)

const maxLine = 80

// Format parses src and returns the canonically formatted glisp source.
func Format(src string) (string, error) {
	result, err := parser.ParseWithComments(src)
	if err != nil {
		return "", err
	}
	return formatFile(result.Nodes, result.Comments), nil
}

// cfmt carries comment state through the recursive formatters so that ; and ;;
// comments inside forms are emitted in place rather than relocated to the next
// top-level form or the end of the file. Comments are consumed depth-first; the
// used set guarantees each is emitted exactly once.
type cfmt struct {
	comments parser.CommentMap
	used     map[int]bool
}

func formatFile(nodes []ast.Node, comments parser.CommentMap) string {
	c := &cfmt{comments: comments, used: map[int]bool{}}
	if len(nodes) == 0 {
		var sb strings.Builder
		for _, l := range sortedCommentLines(comments, 1, math.MaxInt) {
			sb.WriteString(comments[l] + "\n")
		}
		return sb.String()
	}
	var sb strings.Builder
	for i, n := range nodes {
		lo := 1
		if i > 0 {
			lo = nodes[i-1].Pos().Line + 1
		}
		// Leading comments of n. In-body comments of the previous top-level form
		// were consumed while formatting it, so they are excluded here.
		block := c.commentLines(lo, n.Pos().Line-1, 0)
		if i > 0 {
			sb.WriteString("\n\n")
		}
		for _, cl := range block {
			sb.WriteString(cl + "\n")
		}
		sb.WriteString(c.format(n, 0))
	}
	// Any comments not consumed in place (e.g. immediately before a closing
	// paren) are emitted after the last node so nothing is ever dropped.
	lo := nodes[len(nodes)-1].Pos().Line + 1
	trailing := c.commentLines(lo, math.MaxInt, 0)
	if len(trailing) > 0 {
		sb.WriteString("\n\n")
		for _, cl := range trailing {
			sb.WriteString(cl + "\n")
		}
		return sb.String()
	}
	sb.WriteString("\n")
	return sb.String()
}

// takeComments returns the not-yet-used comment texts on lines in [lo,hi] and
// marks them used. Callers add their own indentation.
func (c *cfmt) takeComments(lo, hi int) []string {
	var out []string
	for _, l := range sortedCommentLines(c.comments, lo, hi) {
		if c.used[l] {
			continue
		}
		c.used[l] = true
		out = append(out, c.comments[l])
	}
	return out
}

// commentLines is takeComments with each line prefixed by the given indent.
func (c *cfmt) commentLines(lo, hi, indent int) []string {
	out := c.takeComments(lo, hi)
	for i := range out {
		out[i] = ind(indent) + out[i]
	}
	return out
}

// hasComments reports whether any not-yet-used comment lies on a line in [lo,hi].
// Used to force a form multi-line when it contains comments that an inline
// rendering could not preserve.
func (c *cfmt) hasComments(lo, hi int) bool {
	for l := range c.comments {
		if l >= lo && l <= hi && !c.used[l] {
			return true
		}
	}
	return false
}

// inlineOK reports whether node n may be rendered on one line: it must fit and
// contain no comments within its source span (n's first line to its last
// descendant's line).
func (c *cfmt) inlineOK(n ast.Node, indent int, il string) bool {
	return fits(il, indent) && !c.hasComments(n.Pos().Line+1, nodeMaxLine(n))
}

// nodeMaxLine returns the largest source line among n and its descendants — an
// approximation of n's last line, used to bound the search for internal comments.
func nodeMaxLine(n ast.Node) int {
	max := n.Pos().Line
	consider := func(x ast.Node) {
		if x == nil {
			return
		}
		if m := nodeMaxLine(x); m > max {
			max = m
		}
	}
	switch v := n.(type) {
	case *ast.CallExpr:
		consider(v.Head)
		for _, a := range v.Args {
			consider(a)
		}
	case *ast.FnExpr:
		for _, b := range v.Body {
			consider(b)
		}
	case *ast.LetExpr:
		for _, b := range v.Bindings {
			consider(b.Pattern)
			consider(b.Value)
		}
		for _, b := range v.Body {
			consider(b)
		}
	case *ast.LoopExpr:
		for _, b := range v.Bindings {
			consider(b.Pattern)
			consider(b.Value)
		}
		for _, b := range v.Body {
			consider(b)
		}
	case *ast.IfExpr:
		consider(v.Cond)
		consider(v.Then)
		consider(v.Else)
	case *ast.WhenExpr:
		consider(v.Cond)
		for _, b := range v.Body {
			consider(b)
		}
	case *ast.DoExpr:
		for _, b := range v.Body {
			consider(b)
		}
	case *ast.CondExpr:
		for _, cl := range v.Clauses {
			consider(cl.Test)
			consider(cl.Body)
		}
		consider(v.Default)
	case *ast.SwitchExpr:
		consider(v.Expr)
		for _, sc := range v.Cases {
			consider(sc.Value)
			consider(sc.Body)
		}
		consider(v.Default)
	case *ast.IfErrExpr:
		consider(v.Expr)
		consider(v.OnErr)
		consider(v.OnOk)
	case *ast.IfLetExpr:
		consider(v.Expr)
		consider(v.Then)
		consider(v.Else)
	case *ast.WhenLetExpr:
		consider(v.Expr)
		for _, b := range v.Body {
			consider(b)
		}
	case *ast.VectorLit:
		for _, e := range v.Elements {
			consider(e)
		}
	case *ast.MapLit:
		for _, p := range v.Pairs {
			consider(p.Key)
			consider(p.Value)
		}
	case *ast.SelectStmt:
		for _, sc := range v.Cases {
			consider(sc.ChanExpr)
			consider(sc.SendVal)
			consider(sc.TimeoutMs)
			for _, b := range sc.Body {
				consider(b)
			}
		}
	case *ast.ForChanStmt:
		consider(v.Chan)
		for _, b := range v.Body {
			consider(b)
		}
	case *ast.ParStmt:
		for _, b := range v.Bodies {
			consider(b)
		}
	case *ast.WithLockExpr:
		consider(v.Mutex)
		for _, b := range v.Body {
			consider(b)
		}
	case *ast.PipelineExpr:
		consider(v.Source)
		for _, s := range v.Stages {
			consider(s)
		}
	case *ast.FanOutStmt:
		consider(v.N)
		consider(v.Chan)
		for _, b := range v.Body {
			consider(b)
		}
	case *ast.FanInExpr:
		for _, ch := range v.Chans {
			consider(ch)
		}
	case *ast.RecvOkExpr:
		consider(v.Chan)
	case *ast.LetOrExpr:
		for _, b := range v.Bindings {
			consider(b.Expr)
			consider(b.Fallback)
		}
		for _, b := range v.Body {
			consider(b)
		}
	case *ast.GoStmt:
		for _, b := range v.Body {
			consider(b)
		}
	case *ast.GoValExpr:
		for _, b := range v.Body {
			consider(b)
		}
	case *ast.DeferStmt:
		consider(v.Expr)
	}
	return max
}

// emitForms appends a body sequence to sb — one form per line at indent —
// interleaving comments that appear between forms. afterLine is the source line
// of the enclosing form's header; comments between it and the first body form
// lead that form.
func (c *cfmt) emitForms(sb *strings.Builder, body []ast.Node, indent, afterLine int) {
	prev := afterLine
	for _, b := range body {
		for _, cl := range c.commentLines(prev+1, b.Pos().Line-1, indent) {
			sb.WriteString("\n" + cl)
		}
		sb.WriteString("\n" + c.format(b, indent))
		prev = b.Pos().Line
	}
}

func sortedCommentLines(cm parser.CommentMap, lo, hi int) []int {
	var lines []int
	for l := range cm {
		if l >= lo && l <= hi {
			lines = append(lines, l)
		}
	}
	sort.Ints(lines)
	return lines
}

func ind(n int) string {
	return strings.Repeat("  ", n)
}

func fits(s string, indent int) bool {
	return indent*2+len(s) <= maxLine
}

// tt renders a type expression's text. Channel types ((chan T)) lose their
// delimiting parens at parse time (Text is "chan T"); tt re-adds them so the
// formatted source round-trips. Bracketed types ([]T, [T1 T2], map[K]V) and
// plain names self-delimit and pass through unchanged.
func tt(t *ast.TypeExpr) string {
	if t == nil {
		return ""
	}
	return wrapChanType(t.Text)
}

func wrapChanType(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "chan ") {
		return "(chan " + wrapChanType(s[len("chan "):]) + ")"
	}
	return s
}

// format renders node with indent*2 leading spaces.
func (c *cfmt) format(n ast.Node, indent int) string {
	switch v := n.(type) {
	case *ast.NilLit, *ast.BoolLit, *ast.IntLit, *ast.FloatLit, *ast.StringLit, *ast.KeywordLit:
		return ind(indent) + inline(n)
	case *ast.Symbol:
		return ind(indent) + inline(n)
	case *ast.VectorLit:
		return c.formatVector(v, indent)
	case *ast.MapLit:
		return c.formatMap(v, indent)
	case *ast.SetLit:
		return ind(indent) + inline(n)
	case *ast.CallExpr:
		return c.formatCall(v, indent)
	case *ast.FnExpr:
		return c.formatFn(v, indent)
	case *ast.LetExpr:
		return c.formatLet("let", v.Bindings, v.Body, indent, v.Pos().Line)
	case *ast.LoopExpr:
		return c.formatLet("loop", v.Bindings, v.Body, indent, v.Pos().Line)
	case *ast.IfExpr:
		return c.formatIf(v, indent)
	case *ast.WhenExpr:
		return c.formatWhen(v, indent)
	case *ast.CondExpr:
		return c.formatCond(v, indent)
	case *ast.SwitchExpr:
		return c.formatSwitch(v, indent)
	case *ast.DoExpr:
		return c.formatDo(v, indent)
	case *ast.QuoteExpr:
		il := "'" + inline(v.Form)
		return ind(indent) + il
	case *ast.ReturnExpr:
		il := inline(n)
		if fits(il, indent) {
			return ind(indent) + il
		}
		return formatArgList("return", v.Args, indent)
	case *ast.ValuesExpr:
		il := inline(n)
		if fits(il, indent) {
			return ind(indent) + il
		}
		return formatArgList("values", v.Args, indent)
	case *ast.RecurExpr:
		il := inline(n)
		if fits(il, indent) {
			return ind(indent) + il
		}
		return formatArgList("recur", v.Args, indent)
	case *ast.GoStmt:
		il := inline(n)
		if fits(il, indent) {
			return ind(indent) + il
		}
		return c.formatBody("go", v.Body, indent, v.Pos().Line)
	case *ast.DeferStmt:
		il := inline(n)
		if fits(il, indent) {
			return ind(indent) + il
		}
		inner := c.format(v.Expr, indent+1)
		return ind(indent) + "(defer\n" + inner + ")"
	case *ast.SendStmt:
		return ind(indent) + inline(n)
	case *ast.RecvExpr:
		return ind(indent) + inline(n)
	case *ast.CloseStmt:
		return ind(indent) + inline(n)
	case *ast.ChanExpr:
		return ind(indent) + inline(n)
	case *ast.SelectStmt:
		return c.formatSelect(v, indent)
	case *ast.MethodCallExpr:
		il := inline(n)
		if fits(il, indent) {
			return ind(indent) + il
		}
		parts := []string{"." + v.Method, inline(v.Object)}
		for _, a := range v.Args {
			parts = append(parts, inline(a))
		}
		return formatRawArgs(parts, indent)
	case *ast.FieldAccessExpr:
		return ind(indent) + inline(n)
	case *ast.StructLitExpr:
		return formatStructLit(v, indent)
	case *ast.TypeAssertExpr:
		return ind(indent) + inline(n)
	case *ast.IfErrExpr:
		return c.formatIfErr(v, indent)
	case *ast.IfLetExpr:
		return c.formatIfLet(v, indent)
	case *ast.WhenLetExpr:
		return c.formatWhenLet(v, indent)
	case *ast.DefDecl:
		return c.formatDef(v, indent)
	case *ast.DefnDecl:
		return c.formatDefn(v, indent)
	case *ast.NSDecl:
		return formatNS(v, indent)
	case *ast.DefTypeDecl:
		return ind(indent) + inline(v)
	case *ast.StructDecl:
		return formatStruct(v, indent)
	case *ast.InterfaceDecl:
		return formatInterface(v, indent)
	case *ast.MethodDecl:
		return c.formatMethod(v, indent)
	case *ast.DefTestDecl:
		return c.formatDefTest(v, indent)
	case *ast.ForChanStmt:
		return c.formatForChan(v, indent)
	case *ast.ParStmt:
		return c.formatPar(v, indent)
	case *ast.WithLockExpr:
		return c.formatWithLock(v, indent)
	case *ast.PipelineExpr:
		return c.formatPipeline(v, indent)
	case *ast.FanOutStmt:
		return c.formatFanOut(v, indent)
	case *ast.FanInExpr:
		return c.formatFanIn(v, indent)
	case *ast.LetOrExpr:
		return c.formatLetOr(v, indent)
	case *ast.RecvOkExpr:
		return ind(indent) + inline(n)
	case *ast.GoValExpr:
		il := inline(v)
		if c.inlineOK(v, indent, il) {
			return ind(indent) + il
		}
		head := "(go-val"
		if v.ElemType != nil {
			head += " " + tt(v.ElemType)
		}
		var sb strings.Builder
		sb.WriteString(ind(indent) + head)
		c.emitForms(&sb, v.Body, indent+1, v.Pos().Line)
		sb.WriteString(")")
		return sb.String()
	}
	return ind(indent) + "???"
}

// inline renders node as a single-line string with no leading whitespace.
// FormatNode renders a single AST node to a compact one-line string. It is the
// node-level entry point reused by the transpiler (e.g. to embed an assert's
// condition source in its panic message); comments are not consulted.
func FormatNode(n ast.Node) string { return inline(n) }

func inline(n ast.Node) string {
	switch v := n.(type) {
	case *ast.NilLit:
		return "nil"
	case *ast.BoolLit:
		if v.Value {
			return "true"
		}
		return "false"
	case *ast.IntLit:
		return fmt.Sprintf("%d", v.Value)
	case *ast.FloatLit:
		s := strconv.FormatFloat(v.Value, 'f', -1, 64)
		// A whole-number float must keep its ".0" suffix, or the round-trip
		// turns the literal into an int (changing the emitted Go type).
		if !strings.ContainsAny(s, ".eE") {
			s += ".0"
		}
		return s
	case *ast.StringLit:
		return fmt.Sprintf("%q", v.Value)
	case *ast.KeywordLit:
		return ":" + v.Value
	case *ast.Symbol:
		return v.Name
	case *ast.VectorLit:
		if len(v.Elements) == 0 {
			return "[]"
		}
		parts := make([]string, len(v.Elements))
		for i, e := range v.Elements {
			parts[i] = inline(e)
		}
		return "[" + strings.Join(parts, " ") + "]"
	case *ast.MapLit:
		if len(v.Pairs) == 0 {
			return "{}"
		}
		parts := make([]string, 0, len(v.Pairs)*2)
		for _, p := range v.Pairs {
			parts = append(parts, inline(p.Key), inline(p.Value))
		}
		return "{" + strings.Join(parts, " ") + "}"
	case *ast.SetLit:
		parts := make([]string, len(v.Elements))
		for i, e := range v.Elements {
			parts[i] = inline(e)
		}
		return "#{" + strings.Join(parts, " ") + "}"
	case *ast.CallExpr:
		parts := []string{inline(v.Head)}
		for _, a := range v.Args {
			parts = append(parts, inline(a))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.FnExpr:
		head := "(fn " + inlineParams(v.Params)
		if v.ReturnType != nil {
			head += " -> " + tt(v.ReturnType)
		}
		var bodyParts []string
		for _, b := range v.Body {
			bodyParts = append(bodyParts, inline(b))
		}
		return head + " " + strings.Join(bodyParts, " ") + ")"
	case *ast.LetExpr:
		return inlineBindingForm("let", v.Bindings, v.Body)
	case *ast.LoopExpr:
		return inlineBindingForm("loop", v.Bindings, v.Body)
	case *ast.IfExpr:
		parts := []string{"if", inline(v.Cond), inline(v.Then)}
		if v.Else != nil {
			parts = append(parts, inline(v.Else))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.WhenExpr:
		parts := []string{"when", inline(v.Cond)}
		for _, b := range v.Body {
			parts = append(parts, inline(b))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.CondExpr:
		parts := []string{"cond"}
		for _, cl := range v.Clauses {
			parts = append(parts, inline(cl.Test), inline(cl.Body))
		}
		if v.Default != nil {
			parts = append(parts, ":else", inline(v.Default))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.SwitchExpr:
		head := v.Head
		if head == "" {
			head = "switch"
		}
		parts := []string{head, inline(v.Expr)}
		for _, sc := range v.Cases {
			parts = append(parts, inline(sc.Value), inline(sc.Body))
		}
		if v.Default != nil {
			// switch spells the fallback `:default body`; case as a trailing body.
			if head == "case" {
				parts = append(parts, inline(v.Default))
			} else {
				parts = append(parts, ":default", inline(v.Default))
			}
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.DoExpr:
		parts := []string{"do"}
		for _, b := range v.Body {
			parts = append(parts, inline(b))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.QuoteExpr:
		return "'" + inline(v.Form)
	case *ast.ReturnExpr:
		parts := []string{"return"}
		for _, a := range v.Args {
			parts = append(parts, inline(a))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.ValuesExpr:
		parts := []string{"values"}
		for _, a := range v.Args {
			parts = append(parts, inline(a))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.RecurExpr:
		parts := []string{"recur"}
		for _, a := range v.Args {
			parts = append(parts, inline(a))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.GoStmt:
		parts := []string{"go"}
		for _, b := range v.Body {
			parts = append(parts, inline(b))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.DeferStmt:
		return "(defer " + inline(v.Expr) + ")"
	case *ast.SendStmt:
		return "(send! " + inline(v.Chan) + " " + inline(v.Val) + ")"
	case *ast.RecvExpr:
		return "(recv! " + inline(v.Chan) + ")"
	case *ast.CloseStmt:
		return "(close! " + inline(v.Chan) + ")"
	case *ast.ChanExpr:
		if v.Cap != nil {
			return "(chan " + tt(v.ElemType) + " " + inline(v.Cap) + ")"
		}
		return "(chan " + tt(v.ElemType) + ")"
	case *ast.SelectStmt:
		parts := []string{"select!"}
		for _, sc := range v.Cases {
			caseStr := "(" + selectCaseHead(sc)
			for _, b := range sc.Body {
				caseStr += " " + inline(b)
			}
			parts = append(parts, caseStr+")")
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.MethodCallExpr:
		parts := []string{"." + v.Method, inline(v.Object)}
		for _, a := range v.Args {
			parts = append(parts, inline(a))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.FieldAccessExpr:
		return "(.-" + v.Field + " " + inline(v.Object) + ")"
	case *ast.StructLitExpr:
		var pairParts []string
		for _, p := range v.Fields {
			pairParts = append(pairParts, inline(p.Key)+" "+inline(p.Value))
		}
		return "(" + v.TypeName + ". {" + strings.Join(pairParts, " ") + "})"
	case *ast.TypeAssertExpr:
		return "(as " + tt(v.Type) + " " + inline(v.Value) + ")"
	case *ast.IfErrExpr:
		return "(if-err [" + v.ValName + " " + v.ErrName + "] " +
			inline(v.Expr) + " " + inline(v.OnErr) + " " + inline(v.OnOk) + ")"
	case *ast.IfLetExpr:
		s := "(if-let [" + inline(v.Pattern) + " " + inline(v.Expr) + "] " + inline(v.Then)
		if v.Else != nil {
			s += " " + inline(v.Else)
		}
		return s + ")"
	case *ast.WhenLetExpr:
		parts := []string{"when-let [" + inline(v.Pattern) + " " + inline(v.Expr) + "]"}
		for _, b := range v.Body {
			parts = append(parts, inline(b))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.DefDecl:
		s := "(def " + v.Name
		if v.TypeAnnot != nil {
			s += " " + tt(v.TypeAnnot)
		}
		return s + " " + inline(v.Value) + ")"
	case *ast.DefnDecl:
		head := "(defn " + v.Name + " " + inlineParams(v.Params)
		if v.ReturnType != nil {
			head += " -> " + tt(v.ReturnType)
		}
		var bodyParts []string
		for _, b := range v.Body {
			bodyParts = append(bodyParts, inline(b))
		}
		return head + " " + strings.Join(bodyParts, " ") + ")"
	case *ast.NSDecl:
		return inlineNS(v)
	case *ast.StructDecl:
		parts := []string{"defstruct", v.Name}
		for _, f := range v.Fields {
			if f.Tag != "" {
				parts = append(parts, f.Name)
				if f.TypeAnnot != nil {
					parts = append(parts, tt(f.TypeAnnot))
				}
				parts = append(parts, fmt.Sprintf("%q", f.Tag))
			} else {
				parts = append(parts, f.Name)
				if f.TypeAnnot != nil {
					parts = append(parts, tt(f.TypeAnnot))
				}
			}
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.InterfaceDecl:
		return "(definterface " + v.Name + " ...)"
	case *ast.MethodDecl:
		return "(defmethod " + tt(v.ReceiverType) + " " + v.Name + " ...)"
	case *ast.DefTypeDecl:
		return "(deftype " + v.Name + " " + tt(v.BaseType) + ")"
	case *ast.GoValExpr:
		parts := []string{"go-val"}
		if v.ElemType != nil {
			parts = append(parts, tt(v.ElemType))
		}
		for _, b := range v.Body {
			parts = append(parts, inline(b))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.DefTestDecl:
		parts := []string{"deftest", v.Name}
		for _, b := range v.Body {
			parts = append(parts, inline(b))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.ForChanStmt:
		parts := []string{"for-chan", "[" + v.Binding.Name + " " + inline(v.Chan) + "]"}
		for _, b := range v.Body {
			parts = append(parts, inline(b))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.ParStmt:
		parts := []string{"par"}
		for _, b := range v.Bodies {
			parts = append(parts, inline(b))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.RecvOkExpr:
		return "(recv-ok! " + inline(v.Chan) + ")"
	case *ast.WithLockExpr:
		parts := []string{"with-lock", inline(v.Mutex)}
		for _, b := range v.Body {
			parts = append(parts, inline(b))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.PipelineExpr:
		parts := []string{"pipeline", "[" + v.Binding.Name + " " + inline(v.Source) + "]"}
		for _, s := range v.Stages {
			parts = append(parts, inline(s))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.FanOutStmt:
		parts := []string{"fan-out", inline(v.N), "[" + v.Binding.Name + " " + inline(v.Chan) + "]"}
		for _, b := range v.Body {
			parts = append(parts, inline(b))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.FanInExpr:
		parts := []string{"fan-in"}
		for _, ch := range v.Chans {
			parts = append(parts, inline(ch))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.LetOrExpr:
		var bps []string
		for _, b := range v.Bindings {
			bps = append(bps, b.Name, inline(b.Expr), inline(b.Fallback))
		}
		parts := []string{"let-or", "[" + strings.Join(bps, " ") + "]"}
		for _, b := range v.Body {
			parts = append(parts, inline(b))
		}
		return "(" + strings.Join(parts, " ") + ")"
	}
	return "???"
}

// --- helpers ---

func inlineParams(params []ast.Param) string {
	parts := make([]string, 0, len(params)*2)
	for _, p := range params {
		if p.Pattern != nil {
			parts = append(parts, inline(p.Pattern))
		} else if p.IsRest {
			parts = append(parts, "& "+p.Name)
			if p.TypeAnnot != nil {
				parts[len(parts)-1] += " " + tt(p.TypeAnnot)
			}
		} else {
			parts = append(parts, p.Name)
			if p.TypeAnnot != nil {
				parts = append(parts, tt(p.TypeAnnot))
			}
		}
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func inlineLetBinding(b ast.LetBinding) string {
	if b.TypeAnnot != nil {
		return inline(b.Pattern) + " " + tt(b.TypeAnnot) + " " + inline(b.Value)
	}
	return inline(b.Pattern) + " " + inline(b.Value)
}

func inlineBindingForm(keyword string, bindings []ast.LetBinding, body []ast.Node) string {
	var bindParts []string
	for _, b := range bindings {
		bindParts = append(bindParts, inlineLetBinding(b))
	}
	parts := []string{keyword, "[" + strings.Join(bindParts, " ") + "]"}
	for _, b := range body {
		parts = append(parts, inline(b))
	}
	return "(" + strings.Join(parts, " ") + ")"
}

func inlineNS(v *ast.NSDecl) string {
	if len(v.Imports) == 0 && len(v.Requires) == 0 {
		return "(ns " + v.Name + ")"
	}
	var clauses []string
	if len(v.Imports) > 0 {
		parts := make([]string, len(v.Imports))
		for i, imp := range v.Imports {
			if imp.Alias != "" {
				parts[i] = "[" + imp.Path + " :as " + imp.Alias + "]"
			} else {
				parts[i] = imp.Path
			}
		}
		clauses = append(clauses, "(:import ["+strings.Join(parts, " ")+"])")
	}
	if len(v.Requires) > 0 {
		parts := make([]string, len(v.Requires))
		for i, req := range v.Requires {
			if req.Alias != "" {
				parts[i] = "[" + req.Path + " :as " + req.Alias + "]"
			} else {
				parts[i] = req.Path
			}
		}
		clauses = append(clauses, "(:require ["+strings.Join(parts, " ")+"])")
	}
	return "(ns " + v.Name + " " + strings.Join(clauses, " ") + ")"
}

// formatRawArgs renders (parts[0] parts[1] ...) with continuation indented under head.
func formatRawArgs(parts []string, indent int) string {
	if len(parts) == 0 {
		return ind(indent) + "()"
	}
	// first arg same line as head, rest indented
	head := "(" + parts[0]
	if len(parts) == 1 {
		return ind(indent) + head + ")"
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + head + " " + parts[1])
	argIndent := ind(indent+1)
	for _, p := range parts[2:] {
		sb.WriteString("\n" + argIndent + p)
	}
	sb.WriteString(")")
	return sb.String()
}

// formatArgList renders (keyword args...) with multi-line args.
func formatArgList(keyword string, args []ast.Node, indent int) string {
	if len(args) == 0 {
		return ind(indent) + "(" + keyword + ")"
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(" + keyword + " " + inline(args[0]))
	for _, a := range args[1:] {
		sb.WriteString("\n" + ind(indent+1) + inline(a))
	}
	sb.WriteString(")")
	return sb.String()
}

// formatBody renders (keyword body...) multi-line.
func (c *cfmt) formatBody(keyword string, body []ast.Node, indent, afterLine int) string {
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(" + keyword)
	c.emitForms(&sb, body, indent+1, afterLine)
	sb.WriteString(")")
	return sb.String()
}

// elemAt renders e starting at absolute column col. The first line carries
// no leading pad (the caller positions it); continuation lines are padded to
// col. Elements whose inline rendering fits stay on one line; oversized ones
// recurse through the full formatter — this is what lets hiccup trees break
// into readable nested layout instead of one long line.
func (c *cfmt) elemAt(e ast.Node, col int) string {
	il := inline(e)
	if col+len(il) <= maxLine && !c.hasComments(e.Pos().Line+1, nodeMaxLine(e)) {
		return il
	}
	block := c.format(e, col/2)
	shift := col - (col/2)*2
	lines := strings.Split(block, "\n")
	for i, ln := range lines {
		if i == 0 {
			lines[i] = strings.TrimLeft(ln, " ")
		} else if shift > 0 && ln != "" {
			lines[i] = strings.Repeat(" ", shift) + ln
		}
	}
	return strings.Join(lines, "\n")
}

func (c *cfmt) formatVector(v *ast.VectorLit, indent int) string {
	il := inline(v)
	if fits(il, indent) {
		return ind(indent) + il
	}
	if len(v.Elements) == 0 {
		return ind(indent) + "[]"
	}
	col := indent*2 + 1
	var sb strings.Builder
	sb.WriteString(ind(indent) + "[")
	head := c.elemAt(v.Elements[0], col)
	sb.WriteString(head)
	rest := v.Elements[1:]
	// Hiccup head: a keyword tag keeps a fitting attrs map on its line,
	// so [:li.todo {:id x} child...] breaks after the attrs, not before.
	if _, isKw := v.Elements[0].(*ast.KeywordLit); isKw && len(rest) > 0 {
		if m, ok := rest[0].(*ast.MapLit); ok {
			attrs := inline(m)
			if col+len(head)+1+len(attrs) <= maxLine &&
				!c.hasComments(m.Pos().Line+1, nodeMaxLine(m)) {
				sb.WriteString(" " + attrs)
				rest = rest[1:]
			}
		}
	}
	contPad := ind(indent) + " "
	for _, e := range rest {
		sb.WriteString("\n" + contPad + c.elemAt(e, col))
	}
	sb.WriteString("]")
	return sb.String()
}

func (c *cfmt) formatMap(v *ast.MapLit, indent int) string {
	if len(v.Pairs) == 0 {
		return ind(indent) + "{}"
	}
	il := inline(v)
	// only use inline for single pair that fits
	if len(v.Pairs) == 1 && fits(il, indent) {
		return ind(indent) + il
	}
	// multi-line aligned
	keyStrs := make([]string, len(v.Pairs))
	maxKeyW := 0
	for i, p := range v.Pairs {
		keyStrs[i] = inline(p.Key)
		if len(keyStrs[i]) > maxKeyW {
			maxKeyW = len(keyStrs[i])
		}
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "{")
	contPad := strings.Repeat(" ", indent*2+1)
	for i, p := range v.Pairs {
		if i > 0 {
			sb.WriteString("\n" + contPad)
		}
		padding := strings.Repeat(" ", maxKeyW-len(keyStrs[i]))
		valueCol := indent*2 + 1 + maxKeyW + 1
		sb.WriteString(keyStrs[i] + padding + " " + c.elemAt(p.Value, valueCol))
	}
	sb.WriteString("}")
	return sb.String()
}

// alignThreshold is the largest column at which a multi-line call still aligns
// its continuation arguments under the first argument (Style A, cljfmt-style).
// Past it — a long head symbol or deep nesting — alignment is abandoned in
// favour of a flat 2-space hang so continuation args aren't crowded against the
// right margin. maxLine/2 keeps at least half the line available for arguments.
const alignThreshold = maxLine / 2

// pairForms are call heads whose trailing arguments are logical key/value (or
// test/result) pairs. When such a call breaks across lines its args are emitted
// two-per-line aligned under the anchor argument, rather than fanning out one
// token per line. The leading anchor (the map for assoc, the dispatch value for
// case/cond->) sits on the head line; a lone trailing arg (e.g. a case default)
// gets its own line.
var pairForms = map[string]bool{
	"assoc":   true,
	"cond->":  true,
	"cond->>": true,
}

func (c *cfmt) formatCall(v *ast.CallExpr, indent int) string {
	il := inline(v)
	if c.inlineOK(v, indent, il) {
		return ind(indent) + il
	}
	headStr := inline(v.Head)
	if len(v.Args) == 0 {
		return ind(indent) + "(" + headStr + ")"
	}
	// Pair-forms (assoc, case, cond->, …) render their args two-per-line.
	if pairForms[headStr] {
		if s, ok := c.formatPairForm(v, headStr, indent); ok {
			return s
		}
	}
	// Style A: align continuation args under the first argument when it is a
	// simple atom on the head line and the alignment column is within threshold.
	if s, ok := c.formatAligned(v, headStr, indent); ok {
		return s
	}
	// Fallback: flat 2-space hang. Also the path for a call-headed first arg
	// (e.g. each route in web/routes) which reads better hung than aligned.
	var sb strings.Builder
	firstInline := inline(v.Args[0])
	if _, isCall := v.Args[0].(*ast.CallExpr); !isCall && fits("("+headStr+" "+firstInline, indent) {
		sb.WriteString(ind(indent) + "(" + headStr + " " + firstInline)
		c.emitForms(&sb, v.Args[1:], indent+1, v.Args[0].Pos().Line)
	} else {
		sb.WriteString(ind(indent) + "(" + headStr)
		c.emitForms(&sb, v.Args, indent+1, v.Pos().Line)
	}
	sb.WriteString(")")
	return sb.String()
}

// alignAnchor decides whether a multi-line call may put its first argument on
// the head line and align continuations under it. It returns the head line
// ("(head firstArg") and the alignment column, or ok=false to fall back to a
// 2-space hang. Alignment is declined when the first arg is itself a call or
// won't fit on the head line, when the column exceeds alignThreshold, or when
// the form carries comments an aligned rendering couldn't preserve.
func (c *cfmt) alignAnchor(v *ast.CallExpr, headStr string, indent int) (headLine string, col int, ok bool) {
	if _, isCall := v.Args[0].(*ast.CallExpr); isCall {
		return "", 0, false
	}
	headLine = "(" + headStr + " " + inline(v.Args[0])
	if !fits(headLine, indent) {
		return "", 0, false
	}
	col = indent*2 + len("("+headStr+" ")
	if col > alignThreshold {
		return "", 0, false
	}
	if c.hasComments(v.Pos().Line+1, nodeMaxLine(v)) {
		return "", 0, false
	}
	return headLine, col, true
}

// formatAligned renders v with continuation args aligned under the first
// argument (Style A). It applies only when every continuation arg fits on a
// single line at the alignment column; an arg that would itself wrap, or that
// overflows maxLine, makes it return ok=false so formatCall falls back.
func (c *cfmt) formatAligned(v *ast.CallExpr, headStr string, indent int) (string, bool) {
	if len(v.Args) < 2 {
		return "", false
	}
	headLine, col, ok := c.alignAnchor(v, headStr, indent)
	if !ok {
		return "", false
	}
	pad := strings.Repeat(" ", col)
	lines := make([]string, 0, len(v.Args)-1)
	for _, a := range v.Args[1:] {
		s := inline(a)
		if col+len(s)+1 > maxLine { // +1 leaves room for the closing paren
			return "", false
		}
		lines = append(lines, pad+s)
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + headLine)
	for _, ln := range lines {
		sb.WriteString("\n" + ln)
	}
	sb.WriteString(")")
	return sb.String(), true
}

// formatPairForm renders a pair-form call (assoc, case, cond->, …) with its
// trailing args grouped two-per-line and aligned under the anchor argument. A
// lone trailing arg (e.g. case's default) is emitted on its own line. Returns
// ok=false — so formatCall falls back — when there is no anchor+pair to render,
// the anchor can't head the line, or any pair overflows maxLine.
func (c *cfmt) formatPairForm(v *ast.CallExpr, headStr string, indent int) (string, bool) {
	if len(v.Args) < 3 {
		return "", false
	}
	headLine, col, ok := c.alignAnchor(v, headStr, indent)
	if !ok {
		return "", false
	}
	rest := v.Args[1:]
	pad := strings.Repeat(" ", col)
	var lines []string
	i := 0
	for ; i+1 < len(rest); i += 2 {
		line := pad + inline(rest[i]) + " " + inline(rest[i+1])
		if len(line)+1 > maxLine {
			return "", false
		}
		lines = append(lines, line)
	}
	if i < len(rest) { // lone trailing arg (e.g. case default)
		line := pad + inline(rest[i])
		if len(line)+1 > maxLine {
			return "", false
		}
		lines = append(lines, line)
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + headLine)
	for _, ln := range lines {
		sb.WriteString("\n" + ln)
	}
	sb.WriteString(")")
	return sb.String(), true
}

func (c *cfmt) formatFn(v *ast.FnExpr, indent int) string {
	il := inline(v)
	if c.inlineOK(v, indent, il) {
		return ind(indent) + il
	}
	params := inlineParams(v.Params)
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(fn " + params)
	if v.ReturnType != nil {
		sb.WriteString(" -> " + tt(v.ReturnType))
	}
	c.emitForms(&sb, v.Body, indent+1, v.Pos().Line)
	sb.WriteString(")")
	return sb.String()
}

func (c *cfmt) formatLet(keyword string, bindings []ast.LetBinding, body []ast.Node, indent, headLine int) string {
	il := inlineBindingForm(keyword, bindings, body)
	// Inline only when it fits and the form holds no comments to preserve.
	if fits(il, indent) && !c.hasComments(headLine+1, letMaxLine(bindings, body, headLine)) {
		return ind(indent) + il
	}
	// multi-line: bindings vector may itself be multi-line
	// opening: (let [b1name b1val
	//               b2name b2val]
	//   body)
	prefix := "(" + keyword + " ["
	bindCol := indent*2 + len(prefix)
	contPad := strings.Repeat(" ", bindCol)
	var sb strings.Builder
	sb.WriteString(ind(indent) + prefix)
	prevLine := headLine
	for i, b := range bindings {
		if i > 0 {
			// Comments between bindings, aligned under the binding column.
			for _, ct := range c.takeComments(prevLine+1, b.Pattern.Pos().Line-1) {
				sb.WriteString("\n" + contPad + ct)
			}
			sb.WriteString("\n" + contPad)
		}
		inlineB := inlineLetBinding(b)
		// A wide map destructure pattern is broken onto multiple aligned lines
		// rather than overflowing as one long line.
		if mapPat, ok := b.Pattern.(*ast.MapLit); ok && b.TypeAnnot == nil &&
			bindCol+len(inlineB) > maxLine {
			if ml, ok := formatDestructurePattern(mapPat, bindCol); ok {
				sb.WriteString(ml + " " + inline(b.Value))
				prevLine = b.Value.Pos().Line
				continue
			}
		}
		sb.WriteString(inlineB)
		prevLine = b.Value.Pos().Line
	}
	sb.WriteString("]")
	// Body comments start after the last binding (or the header if no bindings).
	afterLine := headLine
	if len(bindings) > 0 {
		afterLine = bindings[len(bindings)-1].Value.Pos().Line
	}
	c.emitForms(&sb, body, indent+1, afterLine)
	sb.WriteString(")")
	return sb.String()
}

// fmtDestructEntry is one binding of a map destructure pattern, reconstructed
// from the MapLit pairs for multi-line rendering.
type fmtDestructEntry struct {
	bind string // local name (symbol)
	key  string // source keyword, including leading ":"
	typ  string // ":- Type" annotation type, "" if none
}

// destructureEntries reconstructs the logical bindings of a map destructure
// pattern, folding each ":- Type" annotation pair into the binding it follows.
// Returns false if the map is not destructure-shaped (symbol → keyword pairs).
func destructureEntries(pat *ast.MapLit) ([]fmtDestructEntry, bool) {
	var entries []fmtDestructEntry
	pairs := pat.Pairs
	for i := 0; i < len(pairs); i++ {
		sym, ok := pairs[i].Key.(*ast.Symbol)
		if !ok {
			return nil, false
		}
		kw, ok := pairs[i].Value.(*ast.KeywordLit)
		if !ok {
			return nil, false
		}
		ent := fmtDestructEntry{bind: sym.Name, key: ":" + kw.Value}
		if i+1 < len(pairs) {
			if ak, ok := pairs[i+1].Key.(*ast.KeywordLit); ok && ak.Value == "-" {
				if tsym, ok := pairs[i+1].Value.(*ast.Symbol); ok {
					ent.typ = tsym.Name
					i++ // consume the annotation pair
				}
			}
		}
		entries = append(entries, ent)
	}
	return entries, len(entries) > 0
}

// formatDestructurePattern renders a map destructure pattern across multiple
// lines with bind/key columns aligned, the opening "{" at column col and the
// closing "}" attached to the last entry. Returns false to fall back to inline.
func formatDestructurePattern(pat *ast.MapLit, col int) (string, bool) {
	entries, ok := destructureEntries(pat)
	if !ok || len(entries) < 2 {
		return "", false
	}
	bindW, keyW := 0, 0
	for _, e := range entries {
		if len(e.bind) > bindW {
			bindW = len(e.bind)
		}
		if len(e.key) > keyW {
			keyW = len(e.key)
		}
	}
	lines := make([]string, len(entries))
	for i, e := range entries {
		s := fmt.Sprintf("%-*s %-*s", bindW, e.bind, keyW, e.key)
		if e.typ != "" {
			s += " :- " + e.typ
		}
		lines[i] = strings.TrimRight(s, " ")
	}
	pad := strings.Repeat(" ", col+1) // align under the char after "{"
	var sb strings.Builder
	sb.WriteString("{" + lines[0])
	for i := 1; i < len(lines); i++ {
		sb.WriteString("\n" + pad + lines[i])
	}
	sb.WriteString("}")
	return sb.String(), true
}

// letMaxLine returns the largest source line spanned by a let's bindings and body.
func letMaxLine(bindings []ast.LetBinding, body []ast.Node, headLine int) int {
	max := headLine
	bump := func(n ast.Node) {
		if n != nil {
			if m := nodeMaxLine(n); m > max {
				max = m
			}
		}
	}
	for _, b := range bindings {
		bump(b.Pattern)
		bump(b.Value)
	}
	for _, b := range body {
		bump(b)
	}
	return max
}

func (c *cfmt) formatIf(v *ast.IfExpr, indent int) string {
	il := inline(v)
	if c.inlineOK(v, indent, il) {
		return ind(indent) + il
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(if " + inline(v.Cond) + "\n")
	sb.WriteString(c.format(v.Then, indent+1))
	if v.Else != nil {
		sb.WriteString("\n" + c.format(v.Else, indent+1))
	}
	sb.WriteString(")")
	return sb.String()
}

func (c *cfmt) formatWhen(v *ast.WhenExpr, indent int) string {
	il := inline(v)
	if c.inlineOK(v, indent, il) {
		return ind(indent) + il
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(when " + inline(v.Cond))
	c.emitForms(&sb, v.Body, indent+1, v.Cond.Pos().Line)
	sb.WriteString(")")
	return sb.String()
}

func (c *cfmt) formatCond(v *ast.CondExpr, indent int) string {
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(cond")
	for _, cl := range v.Clauses {
		testStr := inline(cl.Test)
		bodyStr := inline(cl.Body)
		// test on its own line, body on next line if long
		sb.WriteString("\n" + ind(indent+1) + testStr)
		combined := ind(indent+1) + testStr + " " + bodyStr
		if len(combined) <= maxLine {
			sb.WriteString(" " + bodyStr)
		} else {
			sb.WriteString("\n" + c.format(cl.Body, indent+2))
		}
	}
	if v.Default != nil {
		defStr := inline(v.Default)
		combined := ind(indent+1) + ":else " + defStr
		sb.WriteString("\n" + ind(indent+1) + ":else")
		if len(combined) <= maxLine {
			sb.WriteString(" " + defStr)
		} else {
			sb.WriteString("\n" + c.format(v.Default, indent+2))
		}
	}
	sb.WriteString(")")
	return sb.String()
}

func (c *cfmt) formatSwitch(v *ast.SwitchExpr, indent int) string {
	head := v.Head
	if head == "" {
		head = "switch"
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(" + head + " " + inline(v.Expr))
	for _, sc := range v.Cases {
		valStr := inline(sc.Value)
		bodyStr := inline(sc.Body)
		sb.WriteString("\n" + ind(indent+1) + valStr)
		combined := ind(indent+1) + valStr + " " + bodyStr
		if len(combined) <= maxLine {
			sb.WriteString(" " + bodyStr)
		} else {
			sb.WriteString("\n" + c.format(sc.Body, indent+2))
		}
	}
	if v.Default != nil {
		defStr := inline(v.Default)
		// switch spells the fallback as a `:default` clause; case as a trailing
		// unpaired body (Clojure style). Both hang 2 spaces under the head.
		if head == "case" {
			if len(ind(indent+1)+defStr) <= maxLine {
				sb.WriteString("\n" + ind(indent+1) + defStr)
			} else {
				sb.WriteString("\n" + c.format(v.Default, indent+1))
			}
		} else {
			combined := ind(indent+1) + ":default " + defStr
			sb.WriteString("\n" + ind(indent+1) + ":default")
			if len(combined) <= maxLine {
				sb.WriteString(" " + defStr)
			} else {
				sb.WriteString("\n" + c.format(v.Default, indent+2))
			}
		}
	}
	sb.WriteString(")")
	return sb.String()
}

func (c *cfmt) formatDo(v *ast.DoExpr, indent int) string {
	il := inline(v)
	if c.inlineOK(v, indent, il) {
		return ind(indent) + il
	}
	return c.formatBody("do", v.Body, indent, v.Pos().Line)
}

func (c *cfmt) formatDef(v *ast.DefDecl, indent int) string {
	il := inline(v)
	if fits(il, indent) {
		return ind(indent) + il
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(def " + v.Name)
	if v.TypeAnnot != nil {
		sb.WriteString(" " + tt(v.TypeAnnot))
	}
	sb.WriteString("\n")
	sb.WriteString(c.format(v.Value, indent+1))
	sb.WriteString(")")
	return sb.String()
}

// formatDoc renders a (possibly multi-line) ;;; docstring, one ;;; line per
// line of doc, at the given indent. Returns "" for an empty docstring.
func formatDoc(doc string, indent int) string {
	if doc == "" {
		return ""
	}
	var sb strings.Builder
	for _, line := range strings.Split(doc, "\n") {
		sb.WriteString(ind(indent) + ";;; " + line + "\n")
	}
	return sb.String()
}

func (c *cfmt) formatDefn(v *ast.DefnDecl, indent int) string {
	var sb strings.Builder
	sb.WriteString(formatDoc(v.Doc, indent))
	sb.WriteString(ind(indent) + "(defn " + v.Name + " " + inlineParams(v.Params))
	if v.ReturnType != nil {
		sb.WriteString(" -> " + tt(v.ReturnType))
	}
	c.emitForms(&sb, v.Body, indent+1, v.Pos().Line)
	sb.WriteString(")")
	return sb.String()
}

func formatNS(v *ast.NSDecl, indent int) string {
	il := inlineNS(v)
	if fits(il, indent) {
		return ind(indent) + il
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(ns " + v.Name)
	if len(v.Imports) > 0 {
		parts := make([]string, len(v.Imports))
		for i, imp := range v.Imports {
			if imp.Alias != "" {
				parts[i] = "[" + imp.Path + " :as " + imp.Alias + "]"
			} else {
				parts[i] = imp.Path
			}
		}
		sb.WriteString("\n" + ind(indent+1) + "(:import [" + strings.Join(parts, " ") + "])")
	}
	if len(v.Requires) > 0 {
		parts := make([]string, len(v.Requires))
		for i, req := range v.Requires {
			if req.Alias != "" {
				parts[i] = "[" + req.Path + " :as " + req.Alias + "]"
			} else {
				parts[i] = req.Path
			}
		}
		sb.WriteString("\n" + ind(indent+1) + "(:require [" + strings.Join(parts, " ") + "])")
	}
	sb.WriteString(")")
	return sb.String()
}

func formatStruct(v *ast.StructDecl, indent int) string {
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(defstruct " + v.Name)
	for _, f := range v.Fields {
		sb.WriteString("\n" + ind(indent+1) + f.Name)
		if f.TypeAnnot != nil {
			sb.WriteString(" " + tt(f.TypeAnnot))
		}
		if f.Tag != "" {
			sb.WriteString(" " + fmt.Sprintf("%q", f.Tag))
		}
	}
	sb.WriteString(")")
	return sb.String()
}

func formatInterface(v *ast.InterfaceDecl, indent int) string {
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(definterface " + v.Name)
	for _, m := range v.Methods {
		sb.WriteString("\n" + ind(indent+1) + "(" + m.Name + " " + inlineParams(m.Params))
		if m.ReturnType != nil {
			sb.WriteString(" -> " + tt(m.ReturnType))
		}
		sb.WriteString(")")
	}
	sb.WriteString(")")
	return sb.String()
}

func (c *cfmt) formatMethod(v *ast.MethodDecl, indent int) string {
	var sb strings.Builder
	sb.WriteString(formatDoc(v.Doc, indent))
	sb.WriteString(ind(indent) + "(defmethod " + tt(v.ReceiverType) + " " + v.Name)
	allParams := append([]ast.Param{{Name: v.ReceiverName}}, v.Params...)
	sb.WriteString(" " + inlineParams(allParams))
	if v.ReturnType != nil {
		sb.WriteString(" -> " + tt(v.ReturnType))
	}
	c.emitForms(&sb, v.Body, indent+1, v.Pos().Line)
	sb.WriteString(")")
	return sb.String()
}

func (c *cfmt) formatDefTest(v *ast.DefTestDecl, indent int) string {
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(deftest " + v.Name)
	c.emitForms(&sb, v.Body, indent+1, v.Pos().Line)
	sb.WriteString(")")
	return sb.String()
}

// selectCaseHead renders a select! case head in the syntax the parser accepts:
// recv -> [binding chan], send -> [(send! chan val)], plus :default / :timeout ms.
func selectCaseHead(sc ast.SelectCase) string {
	switch {
	case sc.IsDefault:
		return ":default"
	case sc.IsTimeout:
		return ":timeout " + inline(sc.TimeoutMs)
	case sc.IsSend:
		return "[(send! " + inline(sc.ChanExpr) + " " + inline(sc.SendVal) + ")]"
	default: // recv
		return "[" + sc.Binding + " " + inline(sc.ChanExpr) + "]"
	}
}

// selectCaseLine returns the source line of a case's head expression, used to
// anchor in-body comment interleaving.
func selectCaseLine(sc ast.SelectCase) int {
	switch {
	case sc.IsTimeout && sc.TimeoutMs != nil:
		return sc.TimeoutMs.Pos().Line
	case sc.ChanExpr != nil:
		return sc.ChanExpr.Pos().Line
	case len(sc.Body) > 0:
		return sc.Body[0].Pos().Line - 1
	}
	return 0
}

func (c *cfmt) formatSelect(v *ast.SelectStmt, indent int) string {
	il := inline(v)
	if c.inlineOK(v, indent, il) {
		return ind(indent) + il
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(select!")
	for _, sc := range v.Cases {
		sb.WriteString("\n" + ind(indent+1) + "(" + selectCaseHead(sc))
		c.emitForms(&sb, sc.Body, indent+2, selectCaseLine(sc))
		sb.WriteString(")")
	}
	sb.WriteString(")")
	return sb.String()
}

func (c *cfmt) formatForChan(v *ast.ForChanStmt, indent int) string {
	il := inline(v)
	if c.inlineOK(v, indent, il) {
		return ind(indent) + il
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(for-chan [" + v.Binding.Name + " " + inline(v.Chan) + "]")
	c.emitForms(&sb, v.Body, indent+1, v.Chan.Pos().Line)
	sb.WriteString(")")
	return sb.String()
}

func (c *cfmt) formatPar(v *ast.ParStmt, indent int) string {
	il := inline(v)
	if c.inlineOK(v, indent, il) {
		return ind(indent) + il
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(par")
	c.emitForms(&sb, v.Bodies, indent+1, v.Pos().Line)
	sb.WriteString(")")
	return sb.String()
}

func (c *cfmt) formatWithLock(v *ast.WithLockExpr, indent int) string {
	il := inline(v)
	if c.inlineOK(v, indent, il) {
		return ind(indent) + il
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(with-lock " + inline(v.Mutex))
	c.emitForms(&sb, v.Body, indent+1, v.Mutex.Pos().Line)
	sb.WriteString(")")
	return sb.String()
}

func (c *cfmt) formatPipeline(v *ast.PipelineExpr, indent int) string {
	il := inline(v)
	if c.inlineOK(v, indent, il) {
		return ind(indent) + il
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(pipeline [" + v.Binding.Name + " " + inline(v.Source) + "]")
	c.emitForms(&sb, v.Stages, indent+1, v.Source.Pos().Line)
	sb.WriteString(")")
	return sb.String()
}

func (c *cfmt) formatFanOut(v *ast.FanOutStmt, indent int) string {
	il := inline(v)
	if c.inlineOK(v, indent, il) {
		return ind(indent) + il
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(fan-out " + inline(v.N) + " [" + v.Binding.Name + " " + inline(v.Chan) + "]")
	c.emitForms(&sb, v.Body, indent+1, v.Chan.Pos().Line)
	sb.WriteString(")")
	return sb.String()
}

func (c *cfmt) formatFanIn(v *ast.FanInExpr, indent int) string {
	il := inline(v)
	if c.inlineOK(v, indent, il) {
		return ind(indent) + il
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(fan-in")
	c.emitForms(&sb, v.Chans, indent+1, v.Pos().Line)
	sb.WriteString(")")
	return sb.String()
}

func (c *cfmt) formatLetOr(v *ast.LetOrExpr, indent int) string {
	il := inline(v)
	if c.inlineOK(v, indent, il) {
		return ind(indent) + il
	}
	prefix := "(let-or ["
	contPad := strings.Repeat(" ", indent*2+len(prefix))
	var sb strings.Builder
	sb.WriteString(ind(indent) + prefix)
	for i, b := range v.Bindings {
		if i > 0 {
			sb.WriteString("\n" + contPad)
		}
		sb.WriteString(b.Name + " " + inline(b.Expr) + " " + inline(b.Fallback))
	}
	sb.WriteString("]")
	afterLine := v.Pos().Line
	if n := len(v.Bindings); n > 0 {
		afterLine = nodeMaxLine(v.Bindings[n-1].Fallback)
	}
	c.emitForms(&sb, v.Body, indent+1, afterLine)
	sb.WriteString(")")
	return sb.String()
}

func formatStructLit(v *ast.StructLitExpr, indent int) string {
	il := inline(v)
	if fits(il, indent) {
		return ind(indent) + il
	}
	// multi-line struct literal
	keyStrs := make([]string, len(v.Fields))
	maxKeyW := 0
	for i, p := range v.Fields {
		keyStrs[i] = inline(p.Key)
		if len(keyStrs[i]) > maxKeyW {
			maxKeyW = len(keyStrs[i])
		}
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(" + v.TypeName + ". {")
	contPad := strings.Repeat(" ", indent*2+len(v.TypeName)+4)
	for i, p := range v.Fields {
		if i > 0 {
			sb.WriteString("\n" + contPad)
		}
		padding := strings.Repeat(" ", maxKeyW-len(keyStrs[i]))
		sb.WriteString(keyStrs[i] + padding + " " + inline(p.Value))
	}
	sb.WriteString("})")
	return sb.String()
}

func (c *cfmt) formatIfErr(v *ast.IfErrExpr, indent int) string {
	il := inline(v)
	if c.inlineOK(v, indent, il) {
		return ind(indent) + il
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(if-err [" + v.ValName + " " + v.ErrName + "] " + inline(v.Expr) + "\n")
	sb.WriteString(c.format(v.OnErr, indent+1) + "\n")
	sb.WriteString(c.format(v.OnOk, indent+1) + ")")
	return sb.String()
}

func (c *cfmt) formatIfLet(v *ast.IfLetExpr, indent int) string {
	il := inline(v)
	if c.inlineOK(v, indent, il) {
		return ind(indent) + il
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(if-let [" + inline(v.Pattern) + " " + inline(v.Expr) + "]\n")
	sb.WriteString(c.format(v.Then, indent+1))
	if v.Else != nil {
		sb.WriteString("\n" + c.format(v.Else, indent+1))
	}
	sb.WriteString(")")
	return sb.String()
}

func (c *cfmt) formatWhenLet(v *ast.WhenLetExpr, indent int) string {
	il := inline(v)
	if c.inlineOK(v, indent, il) {
		return ind(indent) + il
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(when-let [" + inline(v.Pattern) + " " + inline(v.Expr) + "]")
	c.emitForms(&sb, v.Body, indent+1, v.Expr.Pos().Line)
	sb.WriteString(")")
	return sb.String()
}
