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
	if info, ok := buildSymbolTable(nodes)[name]; ok {
		return &info
	}
	if bd, ok := builtinDocs[name]; ok {
		return &HoverResult{Sig: bd.Sig, Doc: bd.Doc}
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
	case '(', ')', '[', ']', '{', '}', ' ', '\t', '\n', '"', ';', ',':
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
		case *ast.DefDecl:
			table[n.Name] = HoverResult{Sig: formatDefSig(n)}
		case *ast.StructDecl:
			table[n.Name] = HoverResult{Sig: fmt.Sprintf("(defstruct %s)", n.Name)}
		case *ast.InterfaceDecl:
			table[n.Name] = HoverResult{Sig: fmt.Sprintf("(definterface %s)", n.Name)}
		case *ast.MethodDecl:
			table[n.Name] = HoverResult{Sig: formatMethodSig(n), Doc: n.Doc}
		}
	}
	return table
}

func formatMethodSig(n *ast.MethodDecl) string {
	var sb strings.Builder
	sb.WriteString("(defmethod ^")
	sb.WriteString(n.ReceiverType.Text)
	sb.WriteString(" ")
	sb.WriteString(n.Name)
	sb.WriteString(" [")
	sb.WriteString(n.ReceiverName)
	for _, p := range n.Params {
		sb.WriteString(" ")
		if p.TypeAnnot != nil {
			sb.WriteString("^")
			sb.WriteString(p.TypeAnnot.Text)
			sb.WriteString(" ")
		}
		sb.WriteString(p.Name)
	}
	sb.WriteString("]")
	if n.ReturnType != nil {
		sb.WriteString(" ^")
		sb.WriteString(n.ReturnType.Text)
	}
	sb.WriteString(")")
	return sb.String()
}

func formatDefnSig(n *ast.DefnDecl) string {
	var sb strings.Builder
	sb.WriteString("(defn ")
	if n.ReturnType != nil {
		sb.WriteString("^")
		sb.WriteString(n.ReturnType.Text)
		sb.WriteString(" ")
	}
	sb.WriteString(n.Name)
	sb.WriteString(" [")
	for i, p := range n.Params {
		if i > 0 {
			sb.WriteString(" ")
		}
		if p.TypeAnnot != nil {
			sb.WriteString("^")
			sb.WriteString(p.TypeAnnot.Text)
			sb.WriteString(" ")
		}
		if p.IsRest {
			sb.WriteString("& ")
		}
		sb.WriteString(p.Name)
	}
	sb.WriteString("])")
	return sb.String()
}

func formatDefSig(n *ast.DefDecl) string {
	if n.TypeAnnot != nil {
		return fmt.Sprintf("(def ^%s %s)", n.TypeAnnot.Text, n.Name)
	}
	return fmt.Sprintf("(def %s)", n.Name)
}
