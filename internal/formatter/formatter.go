// Package formatter pretty-prints glisp source from a parsed AST.
// Comments are not preserved (they are stripped during lexing).
package formatter

import (
	"fmt"
	"strconv"
	"strings"

	"golisp/internal/ast"
	"golisp/internal/parser"
)

const maxLine = 80

// Format parses src and returns the canonically formatted glisp source.
func Format(src string) (string, error) {
	nodes, err := parser.ParseString(src)
	if err != nil {
		return "", err
	}
	return formatFile(nodes), nil
}

func formatFile(nodes []ast.Node) string {
	var parts []string
	for _, n := range nodes {
		parts = append(parts, format(n, 0))
	}
	return strings.Join(parts, "\n\n") + "\n"
}

func ind(n int) string {
	return strings.Repeat("  ", n)
}

func fits(s string, indent int) bool {
	return indent*2+len(s) <= maxLine
}

// format renders node with indent*2 leading spaces.
func format(n ast.Node, indent int) string {
	switch v := n.(type) {
	case *ast.NilLit, *ast.BoolLit, *ast.IntLit, *ast.FloatLit, *ast.StringLit, *ast.KeywordLit:
		return ind(indent) + inline(n)
	case *ast.Symbol:
		return ind(indent) + inline(n)
	case *ast.VectorLit:
		return formatVector(v, indent)
	case *ast.MapLit:
		return formatMap(v, indent)
	case *ast.SetLit:
		return ind(indent) + inline(n)
	case *ast.CallExpr:
		return formatCall(v, indent)
	case *ast.FnExpr:
		return formatFn(v, indent)
	case *ast.LetExpr:
		return formatLet("let", v.Bindings, v.Body, indent)
	case *ast.LoopExpr:
		return formatLet("loop", v.Bindings, v.Body, indent)
	case *ast.IfExpr:
		return formatIf(v, indent)
	case *ast.WhenExpr:
		return formatWhen(v, indent)
	case *ast.CondExpr:
		return formatCond(v, indent)
	case *ast.DoExpr:
		return formatDo(v, indent)
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
		return formatBody("go", v.Body, indent)
	case *ast.DeferStmt:
		il := inline(n)
		if fits(il, indent) {
			return ind(indent) + il
		}
		inner := format(v.Expr, indent+1)
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
		return formatSelect(v, indent)
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
		return formatIfErr(v, indent)
	case *ast.DefDecl:
		return formatDef(v, indent)
	case *ast.DefnDecl:
		return formatDefn(v, indent)
	case *ast.NSDecl:
		return formatNS(v, indent)
	case *ast.StructDecl:
		return formatStruct(v, indent)
	case *ast.InterfaceDecl:
		return formatInterface(v, indent)
	case *ast.MethodDecl:
		return formatMethod(v, indent)
	case *ast.DefTestDecl:
		return formatDefTest(v, indent)
	}
	return ind(indent) + "???"
}

// inline renders node as a single-line string with no leading whitespace.
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
		return strconv.FormatFloat(v.Value, 'f', -1, 64)
	case *ast.StringLit:
		return fmt.Sprintf("%q", v.Value)
	case *ast.KeywordLit:
		return ":" + v.Value
	case *ast.Symbol:
		if v.TypeAnnot != nil {
			return "^" + v.TypeAnnot.Text + " " + v.Name
		}
		return v.Name
	case *ast.VectorLit:
		if len(v.Elements) == 0 {
			if v.TypeAnnot != nil {
				return "^" + v.TypeAnnot.Text + " []"
			}
			return "[]"
		}
		parts := make([]string, len(v.Elements))
		for i, e := range v.Elements {
			parts[i] = inline(e)
		}
		s := "[" + strings.Join(parts, " ") + "]"
		if v.TypeAnnot != nil {
			return "^" + v.TypeAnnot.Text + " " + s
		}
		return s
	case *ast.MapLit:
		if len(v.Pairs) == 0 {
			if v.TypeAnnot != nil {
				return "^" + v.TypeAnnot.Text + " {}"
			}
			return "{}"
		}
		parts := make([]string, 0, len(v.Pairs)*2)
		for _, p := range v.Pairs {
			parts = append(parts, inline(p.Key), inline(p.Value))
		}
		s := "{" + strings.Join(parts, " ") + "}"
		if v.TypeAnnot != nil {
			return "^" + v.TypeAnnot.Text + " " + s
		}
		return s
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
			head += " ^" + v.ReturnType.Text
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
			return "(chan ^" + v.ElemType.Text + " " + inline(v.Cap) + ")"
		}
		return "(chan ^" + v.ElemType.Text + ")"
	case *ast.SelectStmt:
		return "(select! ...)"
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
		return "(as ^" + v.Type.Text + " " + inline(v.Value) + ")"
	case *ast.IfErrExpr:
		return "(if-err [" + v.ValName + " " + v.ErrName + "] " +
			inline(v.Expr) + " " + inline(v.OnErr) + " " + inline(v.OnOk) + ")"
	case *ast.DefDecl:
		s := "(def"
		if v.TypeAnnot != nil {
			s += " ^" + v.TypeAnnot.Text
		}
		return s + " " + v.Name + " " + inline(v.Value) + ")"
	case *ast.DefnDecl:
		head := "(defn"
		if v.ReturnType != nil {
			head += " ^" + v.ReturnType.Text
		}
		head += " " + v.Name + " " + inlineParams(v.Params)
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
			if f.TypeAnnot != nil {
				parts = append(parts, "^"+f.TypeAnnot.Text)
			}
			if f.Tag != "" {
				parts = append(parts, f.Name, fmt.Sprintf("%q", f.Tag))
			} else {
				parts = append(parts, f.Name)
			}
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *ast.InterfaceDecl:
		return "(definterface " + v.Name + " ...)"
	case *ast.MethodDecl:
		return "(defmethod ^" + v.ReceiverType.Text + " " + v.Name + " ...)"
	case *ast.DefTestDecl:
		parts := []string{"deftest", v.Name}
		for _, b := range v.Body {
			parts = append(parts, inline(b))
		}
		return "(" + strings.Join(parts, " ") + ")"
	}
	return "???"
}

// --- helpers ---

func inlineParams(params []ast.Param) string {
	parts := make([]string, 0, len(params))
	for _, p := range params {
		var s string
		if p.TypeAnnot != nil {
			s = "^" + p.TypeAnnot.Text + " "
		}
		if p.Pattern != nil {
			s += inline(p.Pattern)
		} else if p.IsRest {
			s += "& " + p.Name
		} else {
			s += p.Name
		}
		parts = append(parts, s)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func inlineLetBinding(b ast.LetBinding) string {
	pat := inline(b.Pattern)
	if b.TypeAnnot != nil {
		pat = "^" + b.TypeAnnot.Text + " " + pat
	}
	return pat + " " + inline(b.Value)
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
	if len(v.Imports) == 0 {
		return "(ns " + v.Name + ")"
	}
	parts := make([]string, len(v.Imports))
	for i, imp := range v.Imports {
		if imp.Alias != "" {
			parts[i] = "[" + imp.Path + " :as " + imp.Alias + "]"
		} else {
			parts[i] = imp.Path
		}
	}
	return "(ns " + v.Name + " (:import [" + strings.Join(parts, " ") + "]))"
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
func formatBody(keyword string, body []ast.Node, indent int) string {
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(" + keyword)
	for _, b := range body {
		sb.WriteString("\n" + format(b, indent+1))
	}
	sb.WriteString(")")
	return sb.String()
}

func formatVector(v *ast.VectorLit, indent int) string {
	il := inline(v)
	if fits(il, indent) {
		return ind(indent) + il
	}
	if len(v.Elements) == 0 {
		return ind(indent) + "[]"
	}
	var sb strings.Builder
	prefix := ""
	if v.TypeAnnot != nil {
		prefix = "^" + v.TypeAnnot.Text + " "
	}
	sb.WriteString(ind(indent) + prefix + "[")
	// align subsequent elements under first
	contPad := ind(indent) + prefix + " " + strings.Repeat(" ", len(prefix))
	for i, e := range v.Elements {
		if i == 0 {
			sb.WriteString(inline(e))
		} else {
			sb.WriteString("\n" + contPad + inline(e))
		}
	}
	sb.WriteString("]")
	return sb.String()
}

func formatMap(v *ast.MapLit, indent int) string {
	if len(v.Pairs) == 0 {
		s := "{}"
		if v.TypeAnnot != nil {
			s = "^" + v.TypeAnnot.Text + " " + s
		}
		return ind(indent) + s
	}
	il := inline(v)
	// only use inline for single pair that fits
	if len(v.Pairs) == 1 && fits(il, indent) {
		return ind(indent) + il
	}
	// multi-line aligned
	prefix := ""
	if v.TypeAnnot != nil {
		prefix = "^" + v.TypeAnnot.Text + " "
	}
	keyStrs := make([]string, len(v.Pairs))
	maxKeyW := 0
	for i, p := range v.Pairs {
		keyStrs[i] = inline(p.Key)
		if len(keyStrs[i]) > maxKeyW {
			maxKeyW = len(keyStrs[i])
		}
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + prefix + "{")
	// continuation: align under first key (indent*2 + len(prefix) + 1 spaces)
	contPad := strings.Repeat(" ", indent*2+len(prefix)+1)
	for i, p := range v.Pairs {
		if i > 0 {
			sb.WriteString("\n" + contPad)
		}
		padding := strings.Repeat(" ", maxKeyW-len(keyStrs[i]))
		sb.WriteString(keyStrs[i] + padding + " " + inline(p.Value))
	}
	sb.WriteString("}")
	return sb.String()
}

func formatCall(v *ast.CallExpr, indent int) string {
	il := inline(v)
	if fits(il, indent) {
		return ind(indent) + il
	}
	// head on first line, args indented
	headStr := inline(v.Head)
	if len(v.Args) == 0 {
		return ind(indent) + "(" + headStr + ")"
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(" + headStr + " " + inline(v.Args[0]))
	for _, a := range v.Args[1:] {
		sb.WriteString("\n" + format(a, indent+1))
	}
	sb.WriteString(")")
	return sb.String()
}

func formatFn(v *ast.FnExpr, indent int) string {
	il := inline(v)
	if fits(il, indent) {
		return ind(indent) + il
	}
	params := inlineParams(v.Params)
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(fn " + params)
	if v.ReturnType != nil {
		sb.WriteString(" ^" + v.ReturnType.Text)
	}
	for _, b := range v.Body {
		sb.WriteString("\n" + format(b, indent+1))
	}
	sb.WriteString(")")
	return sb.String()
}

func formatLet(keyword string, bindings []ast.LetBinding, body []ast.Node, indent int) string {
	il := inlineBindingForm(keyword, bindings, body)
	if fits(il, indent) {
		return ind(indent) + il
	}
	// multi-line: bindings vector may itself be multi-line
	// opening: (let [b1name b1val
	//               b2name b2val]
	//   body)
	prefix := "(" + keyword + " ["
	contPad := strings.Repeat(" ", indent*2+len(prefix))
	var sb strings.Builder
	sb.WriteString(ind(indent) + prefix)
	for i, b := range bindings {
		if i > 0 {
			sb.WriteString("\n" + contPad)
		}
		sb.WriteString(inlineLetBinding(b))
	}
	if len(bindings) == 0 {
		sb.WriteString("]")
	} else {
		sb.WriteString("]")
	}
	for _, b := range body {
		sb.WriteString("\n" + format(b, indent+1))
	}
	sb.WriteString(")")
	return sb.String()
}

func formatIf(v *ast.IfExpr, indent int) string {
	il := inline(v)
	if fits(il, indent) {
		return ind(indent) + il
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(if " + inline(v.Cond) + "\n")
	sb.WriteString(format(v.Then, indent+1))
	if v.Else != nil {
		sb.WriteString("\n" + format(v.Else, indent+1))
	}
	sb.WriteString(")")
	return sb.String()
}

func formatWhen(v *ast.WhenExpr, indent int) string {
	il := inline(v)
	if fits(il, indent) {
		return ind(indent) + il
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(when " + inline(v.Cond))
	for _, b := range v.Body {
		sb.WriteString("\n" + format(b, indent+1))
	}
	sb.WriteString(")")
	return sb.String()
}

func formatCond(v *ast.CondExpr, indent int) string {
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
			sb.WriteString("\n" + format(cl.Body, indent+2))
		}
	}
	if v.Default != nil {
		defStr := inline(v.Default)
		combined := ind(indent+1) + ":else " + defStr
		sb.WriteString("\n" + ind(indent+1) + ":else")
		if len(combined) <= maxLine {
			sb.WriteString(" " + defStr)
		} else {
			sb.WriteString("\n" + format(v.Default, indent+2))
		}
	}
	sb.WriteString(")")
	return sb.String()
}

func formatDo(v *ast.DoExpr, indent int) string {
	il := inline(v)
	if fits(il, indent) {
		return ind(indent) + il
	}
	return formatBody("do", v.Body, indent)
}

func formatDef(v *ast.DefDecl, indent int) string {
	il := inline(v)
	if fits(il, indent) {
		return ind(indent) + il
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(def")
	if v.TypeAnnot != nil {
		sb.WriteString(" ^" + v.TypeAnnot.Text)
	}
	sb.WriteString(" " + v.Name + "\n")
	sb.WriteString(format(v.Value, indent+1))
	sb.WriteString(")")
	return sb.String()
}

func formatDefn(v *ast.DefnDecl, indent int) string {
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(defn")
	if v.ReturnType != nil {
		sb.WriteString(" ^" + v.ReturnType.Text)
	}
	sb.WriteString(" " + v.Name + " " + inlineParams(v.Params))
	for _, b := range v.Body {
		sb.WriteString("\n" + format(b, indent+1))
	}
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
		sb.WriteString("\n" + ind(indent+1) + "(:import [" + strings.Join(parts, " ") + "]))")
	} else {
		sb.WriteString(")")
	}
	return sb.String()
}

func formatStruct(v *ast.StructDecl, indent int) string {
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(defstruct " + v.Name)
	for _, f := range v.Fields {
		sb.WriteString("\n" + ind(indent+1))
		if f.TypeAnnot != nil {
			sb.WriteString("^" + f.TypeAnnot.Text + " ")
		}
		sb.WriteString(f.Name)
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
			sb.WriteString(" ^" + m.ReturnType.Text)
		}
		sb.WriteString(")")
	}
	sb.WriteString(")")
	return sb.String()
}

func formatMethod(v *ast.MethodDecl, indent int) string {
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(defmethod ^" + v.ReceiverType.Text + " " + v.Name)
	allParams := append([]ast.Param{{Name: v.ReceiverName}}, v.Params...)
	sb.WriteString(" " + inlineParams(allParams))
	if v.ReturnType != nil {
		sb.WriteString(" ^" + v.ReturnType.Text)
	}
	for _, b := range v.Body {
		sb.WriteString("\n" + format(b, indent+1))
	}
	sb.WriteString(")")
	return sb.String()
}

func formatDefTest(v *ast.DefTestDecl, indent int) string {
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(deftest " + v.Name)
	for _, b := range v.Body {
		sb.WriteString("\n" + format(b, indent+1))
	}
	sb.WriteString(")")
	return sb.String()
}

func formatSelect(v *ast.SelectStmt, indent int) string {
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(select!")
	for _, c := range v.Cases {
		sb.WriteString("\n" + ind(indent+1))
		if c.IsDefault {
			sb.WriteString("(:default")
		} else if c.IsSend {
			sb.WriteString("(:send " + inline(c.ChanExpr) + " " + inline(c.SendVal))
		} else {
			if c.Binding != "" {
				sb.WriteString("(:recv " + c.Binding + " " + inline(c.ChanExpr))
			} else {
				sb.WriteString("(:recv " + inline(c.ChanExpr))
			}
		}
		for _, b := range c.Body {
			sb.WriteString("\n" + format(b, indent+2))
		}
		sb.WriteString(")")
	}
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

func formatIfErr(v *ast.IfErrExpr, indent int) string {
	il := inline(v)
	if fits(il, indent) {
		return ind(indent) + il
	}
	var sb strings.Builder
	sb.WriteString(ind(indent) + "(if-err [" + v.ValName + " " + v.ErrName + "] " + inline(v.Expr) + "\n")
	sb.WriteString(format(v.OnErr, indent+1) + "\n")
	sb.WriteString(format(v.OnOk, indent+1) + ")")
	return sb.String()
}
