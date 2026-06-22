package lsp

import (
	"fmt"
	"strings"

	"golisp/internal/ast"
	"golisp/internal/parser"
)

// HoverResult holds the hover content for a symbol.
type HoverResult struct {
	Sig string
	Doc string
}

// FindHover returns the hover information for the symbol at (line, col) (0-based).
// Returns nil when no information is available.
func FindHover(source string, line, col int) *HoverResult {
	name := symbolAtPosition(source, line, col)
	if name == "" {
		return nil
	}
	nodes, err := parser.ParseString(source)
	if err != nil {
		return nil
	}
	symTable := buildSymbolTable(nodes)
	if info, ok := symTable[name]; ok {
		return &info
	}
	if bd, ok := BuiltinDocs[name]; ok {
		return &HoverResult{Sig: bd.Sig, Doc: bd.Doc}
	}
	if typeName, ok := findLocalBindingType(nodes, symTable, line, name); ok {
		if typeName != "" {
			return &HoverResult{Sig: fmt.Sprintf("(def %s %s)", name, typeName)}
		}
		return &HoverResult{Sig: fmt.Sprintf("(def %s)", name)}
	}
	return nil
}

// symbolAtPosition extracts the glisp symbol token under the cursor.
// line and col are 0-based.
func symbolAtPosition(source string, line, col int) string {
	lines := strings.Split(source, "\n")
	if line >= len(lines) {
		return ""
	}
	runes := []rune(lines[line])
	if col >= len(runes) || !isSymbolRune(runes[col]) {
		return ""
	}
	start := col
	for start > 0 && isSymbolRune(runes[start-1]) {
		start--
	}
	end := col
	for end < len(runes) && isSymbolRune(runes[end]) {
		end++
	}
	if start == end {
		return ""
	}
	return string(runes[start:end])
}

func isSymbolRune(r rune) bool {
	switch r {
	case '(', ')', '[', ']', '{', '}', ' ', '\t', '\n', '"', ';', ',', '^':
		return false
	}
	return true
}

// buildSymbolTable maps top-level definition names to their signatures and doc strings.
func buildSymbolTable(nodes []ast.Node) map[string]HoverResult {
	table := make(map[string]HoverResult)
	for _, node := range nodes {
		switch n := node.(type) {
		case *ast.DefnDecl:
			table[n.Name] = HoverResult{Sig: formatDefnSig(n), Doc: n.Doc}
		case *ast.MacroDecl:
			table[n.Name] = HoverResult{Sig: formatMacroSig(n), Doc: n.Doc}
		case *ast.DefDecl:
			table[n.Name] = HoverResult{Sig: formatDefSig(n)}
		case *ast.StructDecl:
			table[n.Name] = HoverResult{Sig: formatStructSig(n)}
		case *ast.InterfaceDecl:
			table[n.Name] = HoverResult{Sig: formatInterfaceSig(n)}
		case *ast.DefTypeDecl:
			table[n.Name] = HoverResult{Sig: fmt.Sprintf("(deftype %s %s)", n.Name, n.BaseType.Text)}
		case *ast.MethodDecl:
			table[n.Name] = HoverResult{Sig: formatMethodSig(n), Doc: n.Doc}
		}
	}
	return table
}

func formatMethodSig(n *ast.MethodDecl) string {
	var sb strings.Builder
	sb.WriteString("(defmethod ")
	sb.WriteString(n.ReceiverType.Text)
	sb.WriteString(" ")
	sb.WriteString(n.Name)
	sb.WriteString(" [")
	sb.WriteString(n.ReceiverName)
	for _, p := range n.Params {
		sb.WriteString(" ")
		if p.Pattern != nil {
			sb.WriteString(formatPatternSig(p.Pattern))
		} else {
			sb.WriteString(p.Name)
		}
		if p.TypeAnnot != nil {
			sb.WriteString(" ")
			sb.WriteString(p.TypeAnnot.Text)
		}
	}
	sb.WriteString("]")
	if n.ReturnType != nil {
		sb.WriteString(" -> ")
		sb.WriteString(n.ReturnType.Text)
	}
	sb.WriteString(")")
	return sb.String()
}

func formatDefnSig(n *ast.DefnDecl) string {
	var sb strings.Builder
	sb.WriteString("(defn ")
	sb.WriteString(n.Name)
	sb.WriteString(" [")
	for i, p := range n.Params {
		if i > 0 {
			sb.WriteString(" ")
		}
		if p.IsRest {
			sb.WriteString("& ")
		}
		if p.Pattern != nil {
			sb.WriteString(formatPatternSig(p.Pattern))
		} else {
			sb.WriteString(p.Name)
		}
		if p.TypeAnnot != nil {
			sb.WriteString(" ")
			sb.WriteString(p.TypeAnnot.Text)
		}
	}
	sb.WriteString("]")
	if n.ReturnType != nil {
		sb.WriteString(" -> ")
		sb.WriteString(n.ReturnType.Text)
	}
	sb.WriteString(")")
	return sb.String()
}

func formatMacroSig(n *ast.MacroDecl) string {
	var sb strings.Builder
	sb.WriteString("(defmacro ")
	sb.WriteString(n.Name)
	sb.WriteString(" [")
	for i, p := range n.Params {
		if i > 0 {
			sb.WriteString(" ")
		}
		if p.IsRest {
			sb.WriteString("& ")
		}
		if p.Pattern != nil {
			sb.WriteString(formatPatternSig(p.Pattern))
		} else {
			sb.WriteString(p.Name)
		}
	}
	sb.WriteString("])")
	return sb.String()
}

// formatPatternSig renders a destructure pattern as glisp source for hover display.
func formatPatternSig(pattern ast.Node) string {
	switch pat := pattern.(type) {
	case *ast.VectorLit:
		parts := make([]string, len(pat.Elements))
		for i, el := range pat.Elements {
			if sym, ok := el.(*ast.Symbol); ok {
				parts[i] = sym.Name
			}
		}
		return "[" + strings.Join(parts, " ") + "]"
	case *ast.MapLit:
		parts := make([]string, len(pat.Pairs))
		for i, pair := range pat.Pairs {
			sym, _ := pair.Key.(*ast.Symbol)
			kw, _ := pair.Value.(*ast.KeywordLit)
			if sym != nil && kw != nil {
				parts[i] = sym.Name + " :" + kw.Value
			}
		}
		return "{" + strings.Join(parts, " ") + "}"
	}
	return "_"
}

func formatDefSig(n *ast.DefDecl) string {
	if n.TypeAnnot != nil {
		return fmt.Sprintf("(def %s %s)", n.Name, n.TypeAnnot.Text)
	}
	return fmt.Sprintf("(def %s)", n.Name)
}

func formatStructSig(n *ast.StructDecl) string {
	if len(n.Fields) == 0 {
		return fmt.Sprintf("(defstruct %s)", n.Name)
	}
	var sb strings.Builder
	sb.WriteString("(defstruct ")
	sb.WriteString(n.Name)
	for _, f := range n.Fields {
		sb.WriteString("\n  ")
		sb.WriteString(f.Name)
		if f.TypeAnnot != nil {
			sb.WriteString(" ")
			sb.WriteString(f.TypeAnnot.Text)
		}
	}
	sb.WriteString(")")
	return sb.String()
}

func formatInterfaceSig(n *ast.InterfaceDecl) string {
	if len(n.Methods) == 0 {
		return fmt.Sprintf("(definterface %s)", n.Name)
	}
	var sb strings.Builder
	sb.WriteString("(definterface ")
	sb.WriteString(n.Name)
	for _, m := range n.Methods {
		sb.WriteString("\n  (")
		sb.WriteString(m.Name)
		sb.WriteString(" [")
		for i, p := range m.Params {
			if i > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(p.Name)
			if p.TypeAnnot != nil {
				sb.WriteString(" ")
				sb.WriteString(p.TypeAnnot.Text)
			}
		}
		sb.WriteString("]")
		if m.ReturnType != nil {
			sb.WriteString(" -> ")
			sb.WriteString(m.ReturnType.Text)
		}
		sb.WriteString(")")
	}
	sb.WriteString(")")
	return sb.String()
}

// bindingCandidate holds a typed local binding found during AST walk.
type bindingCandidate struct {
	line     int
	typeName string
}

// findLocalBindingType walks all AST nodes collecting bindings of symName,
// then returns the type from the latest binding at or before `line`.
// line is 0-based (LSP convention); AST positions are 1-based.
func findLocalBindingType(nodes []ast.Node, symTable map[string]HoverResult, line int, symName string) (string, bool) {
	astLine := line + 1 // convert LSP 0-based to AST 1-based
	var candidates []bindingCandidate
	for _, n := range nodes {
		collectBindings(n, symName, symTable, &candidates)
	}
	var best *bindingCandidate
	for i := range candidates {
		b := &candidates[i]
		if b.line <= astLine && (best == nil || b.line > best.line) {
			best = b
		}
	}
	if best != nil {
		return best.typeName, true
	}
	return "", false
}

// collectBindings recursively walks node, appending bindings of symName to out.
func collectBindings(node ast.Node, symName string, symTable map[string]HoverResult, out *[]bindingCandidate) {
	if node == nil {
		return
	}
	rec := func(n ast.Node) { collectBindings(n, symName, symTable, out) }
	recAll := func(ns []ast.Node) {
		for _, n := range ns {
			collectBindings(n, symName, symTable, out)
		}
	}
	addParam := func(p ast.Param, line int) {
		if p.Name != symName {
			return
		}
		typeName := ""
		if p.TypeAnnot != nil {
			typeName = p.TypeAnnot.Text
		}
		*out = append(*out, bindingCandidate{line, typeName})
	}
	// processLetBindings handles a []LetBinding in order, threading inferred types
	// forward so later bindings can see earlier ones (e.g. sorted depends on nums).
	processLetBindings := func(bindings []ast.LetBinding) {
		letTypes := map[string]string{} // types inferred so far in this let/loop
		for _, b := range bindings {
			sym, ok := b.Pattern.(*ast.Symbol)
			if !ok {
				continue
			}
			ext := mergeLocalTypes(symTable, letTypes)
			typeName := ""
			if b.TypeAnnot != nil {
				typeName = b.TypeAnnot.Text
			} else {
				typeName = inferExprType(b.Value, ext)
			}
			letTypes[sym.Name] = typeName
			if sym.Name == symName {
				*out = append(*out, bindingCandidate{sym.Pos().Line, typeName})
			}
		}
	}

	switch n := node.(type) {
	case *ast.DefnDecl:
		for _, p := range n.Params {
			addParam(p, n.Pos().Line)
		}
		recAll(n.Body)
	case *ast.MacroDecl:
		for _, p := range n.Params {
			addParam(p, n.Pos().Line)
		}
		recAll(n.Body)
	case *ast.MethodDecl:
		if n.ReceiverName == symName {
			*out = append(*out, bindingCandidate{n.Pos().Line, n.ReceiverType.Text})
		}
		for _, p := range n.Params {
			addParam(p, n.Pos().Line)
		}
		recAll(n.Body)
	case *ast.FnExpr:
		for _, p := range n.Params {
			addParam(p, n.Pos().Line)
		}
		recAll(n.Body)
	case *ast.LetExpr:
		processLetBindings(n.Bindings)
		recAll(n.Body)
	case *ast.LoopExpr:
		processLetBindings(n.Bindings)
		recAll(n.Body)
	case *ast.IfLetExpr:
		if sym, ok := n.Pattern.(*ast.Symbol); ok && sym.Name == symName {
			*out = append(*out, bindingCandidate{n.Pos().Line, inferExprType(n.Expr, symTable)})
		}
		rec(n.Then)
		rec(n.Else)
	case *ast.WhenLetExpr:
		if sym, ok := n.Pattern.(*ast.Symbol); ok && sym.Name == symName {
			*out = append(*out, bindingCandidate{n.Pos().Line, inferExprType(n.Expr, symTable)})
		}
		recAll(n.Body)
	case *ast.LetOrExpr:
		for _, b := range n.Bindings {
			if b.Name == symName {
				*out = append(*out, bindingCandidate{n.Pos().Line, inferExprType(b.Expr, symTable)})
			}
		}
		recAll(n.Body)
	case *ast.IfErrExpr:
		if n.ValName == symName {
			// type of the successful value — hard to infer without knowing the expr's return type
			*out = append(*out, bindingCandidate{n.Pos().Line, ""})
		}
		if n.ErrName == symName {
			*out = append(*out, bindingCandidate{n.Pos().Line, "error"})
		}
		rec(n.OnErr)
		rec(n.OnOk)
	case *ast.IfExpr:
		rec(n.Cond)
		rec(n.Then)
		rec(n.Else)
	case *ast.WhenExpr:
		rec(n.Cond)
		recAll(n.Body)
	case *ast.DoExpr:
		recAll(n.Body)
	case *ast.CondExpr:
		for _, c := range n.Clauses {
			rec(c.Test)
			rec(c.Body)
		}
		rec(n.Default)
	case *ast.SwitchExpr:
		rec(n.Expr)
		for _, c := range n.Cases {
			rec(c.Body)
		}
		rec(n.Default)
	case *ast.CallExpr:
		for _, a := range n.Args {
			rec(a)
		}
	case *ast.GoStmt:
		recAll(n.Body)
	case *ast.DeferStmt:
		rec(n.Expr)
	case *ast.DefTestDecl:
		recAll(n.Body)
	}
}

// inferExprType returns a best-effort type string for a value expression.
func inferExprType(node ast.Node, symTable map[string]HoverResult) string {
	if node == nil {
		return ""
	}
	switch n := node.(type) {
	case *ast.StringLit:
		return "string"
	case *ast.IntLit:
		return "int64"
	case *ast.FloatLit:
		return "float64"
	case *ast.BoolLit:
		return "bool"
	case *ast.VectorLit:
		if n.TypeAnnot != nil {
			return n.TypeAnnot.Text
		}
		return "[]any"
	case *ast.MapLit:
		if n.TypeAnnot != nil {
			return n.TypeAnnot.Text
		}
		return "map[string]any"
	case *ast.TypeAssertExpr:
		return n.Type.Text
	case *ast.StructLitExpr:
		return n.TypeName
	case *ast.Symbol:
		return inferSymbolType(n.Name, symTable)
	case *ast.CallExpr:
		return inferCallType(n, symTable)
	case *ast.IfExpr:
		return pickBestType(
			inferExprType(n.Then, symTable),
			inferExprType(n.Else, symTable),
		)
	case *ast.CondExpr:
		for _, c := range n.Clauses {
			if t := inferExprType(c.Body, symTable); t != "" {
				return t
			}
		}
		return inferExprType(n.Default, symTable)
	case *ast.DoExpr:
		if len(n.Body) > 0 {
			return inferExprType(n.Body[len(n.Body)-1], symTable)
		}
	case *ast.LetExpr:
		if len(n.Body) > 0 {
			return inferExprType(n.Body[len(n.Body)-1], symTable)
		}
	case *ast.WhenExpr:
		if len(n.Body) > 0 {
			return inferExprType(n.Body[len(n.Body)-1], symTable)
		}
	case *ast.SwitchExpr:
		for _, c := range n.Cases {
			if t := inferExprType(c.Body, symTable); t != "" {
				return t
			}
		}
		return inferExprType(n.Default, symTable)
	}
	return ""
}

// pickBestType returns the more specific of two candidate types.
// Prefers non-empty and non-"any" over "any" over "".
func pickBestType(a, b string) string {
	specific := func(t string) bool { return t != "" && t != "any" }
	if specific(a) {
		return a
	}
	if specific(b) {
		return b
	}
	if a != "" {
		return a
	}
	return b
}

// inferSymbolType returns the type for a bare symbol reference.
func inferSymbolType(name string, symTable map[string]HoverResult) string {
	switch name {
	case "os/args":
		return "[]string"
	}
	if info, ok := symTable[name]; ok {
		return extractTypeFromSig(info.Sig)
	}
	return ""
}

// inferCallType returns the inferred return type of a call expression.
func inferCallType(n *ast.CallExpr, symTable map[string]HoverResult) string {
	head, ok := n.Head.(*ast.Symbol)
	if !ok {
		return ""
	}
	collArg := func(i int) string {
		if i < len(n.Args) {
			return inferExprType(n.Args[i], symTable)
		}
		return ""
	}
	propagateSlice := func(t string) string {
		if strings.HasPrefix(t, "[]") {
			return t
		}
		return "[]any"
	}
	switch head.Name {
	// string-returning
	case "str", "upper-case", "lower-case", "trim", "triml", "trimr", "trim-newline",
		"replace", "re/replace", "os/env", "format", "sprintf", "fmt/sprintf":
		return "string"
	// int-returning
	case "count", "len", "parse-int":
		return "int"
	// []string-returning
	case "split", "re/split", "re/find-all":
		return "[]string"
	// bool-returning
	case "re/match", "contains?", "starts-with?", "ends-with?", "empty?", "nil?":
		return "bool"
	// propagate slice type from first arg
	case "rest", "butlast", "reverse", "sort", "distinct", "shuffle":
		return propagateSlice(collArg(0))
	// propagate slice type from second arg (skip n/pred)
	case "take", "drop", "take-while", "drop-while", "take-last", "drop-last":
		return propagateSlice(collArg(1))
	// map-returning
	case "assoc", "dissoc", "merge", "select-keys":
		return "map[string]any"
	// []any-returning
	case "map", "mapv", "filter", "remove", "keep", "keys", "vals",
		"concat", "flatten", "vec", "range", "sort-by", "group-by":
		return "[]any"
	// any-returning (can't know without more context)
	case "first", "last", "second", "nth", "get", "reduce", "find":
		return "any"
	// type assertion: (as T val) → T
	case "as":
		if len(n.Args) >= 1 {
			if sym, ok2 := n.Args[0].(*ast.Symbol); ok2 {
				return sym.Name
			}
		}
	default:
		// user-defined function
		if info, ok2 := symTable[head.Name]; ok2 {
			return extractTypeFromSig(info.Sig)
		}
	}
	return ""
}

// mergeLocalTypes returns a symTable extended with locally-inferred binding types.
// Returns symTable unchanged when localTypes is empty (avoids allocation).
func mergeLocalTypes(symTable map[string]HoverResult, localTypes map[string]string) map[string]HoverResult {
	if len(localTypes) == 0 {
		return symTable
	}
	ext := make(map[string]HoverResult, len(symTable)+len(localTypes))
	for k, v := range symTable {
		ext[k] = v
	}
	for name, typ := range localTypes {
		sig := fmt.Sprintf("(def %s)", name)
		if typ != "" {
			sig = fmt.Sprintf("(def %s %s)", name, typ)
		}
		ext[name] = HoverResult{Sig: sig}
	}
	return ext
}

// extractTypeFromSig extracts the type from a hover sig string.
//
//	"(defn area [s Circle] -> float64)" → "float64"
//	"(def x int)" → "int"
func extractTypeFromSig(sig string) string {
	// defn/defmethod: look for " -> RetType)"
	if idx := strings.Index(sig, " -> "); idx >= 0 {
		ret := strings.TrimSuffix(strings.TrimSpace(sig[idx+4:]), ")")
		if strings.HasPrefix(ret, "[") {
			return "" // multi-return — caller must use if-err
		}
		if ret == "void" {
			return ""
		}
		return ret
	}
	// def: "(def name T)" → T
	s := strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(sig), ")"), "(")
	fields := strings.Fields(s)
	if len(fields) >= 3 && fields[0] == "def" {
		return strings.Join(fields[2:], " ")
	}
	return ""
}
