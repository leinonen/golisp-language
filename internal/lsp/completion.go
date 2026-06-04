package lsp

import (
	"fmt"
	"sort"
	"strings"

	"golisp/internal/ast"
	"golisp/internal/parser"
)

// FindCompletions returns completion items whose label starts with the prefix at (line, col) (0-based).
// User-defined symbols are returned first (they shadow builtins with the same name).
func FindCompletions(source string, line, col int) []CompletionItem {
	prefix := prefixAtPosition(source, line, col)
	seen := make(map[string]bool)
	var items []CompletionItem

	nodes, err := parser.ParseString(source)
	if err == nil {
		for _, node := range nodes {
			var label, detail, doc string
			var kind int
			switch n := node.(type) {
			case *ast.DefnDecl:
				label, detail, kind, doc = n.Name, formatDefnSig(n), 3, n.Doc
			case *ast.DefDecl:
				label, detail, kind = n.Name, formatDefSig(n), 6
			case *ast.StructDecl:
				label = n.Name
				detail = fmt.Sprintf("(defstruct %s)", n.Name)
				kind = 22
			case *ast.InterfaceDecl:
				label = n.Name
				detail = fmt.Sprintf("(definterface %s)", n.Name)
				kind = 8
			case *ast.MethodDecl:
				label, detail, kind, doc = n.Name, formatMethodSig(n), 2, n.Doc
			}
			if label != "" && strings.HasPrefix(label, prefix) {
				seen[label] = true
				items = append(items, CompletionItem{Label: label, Kind: kind, Detail: detail, Documentation: doc})
			}
		}
	}

	for name, bd := range BuiltinDocs {
		if !seen[name] && strings.HasPrefix(name, prefix) {
			items = append(items, CompletionItem{Label: name, Kind: 14, Detail: bd.Sig, Documentation: bd.Doc})
		}
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
	return items
}

// prefixAtPosition returns the symbol characters immediately to the left of col (exclusive).
func prefixAtPosition(source string, line, col int) string {
	lines := strings.Split(source, "\n")
	if line >= len(lines) {
		return ""
	}
	runes := []rune(lines[line])
	end := col
	if end > len(runes) {
		end = len(runes)
	}
	start := end
	for start > 0 && isSymbolRune(runes[start-1]) {
		start--
	}
	return string(runes[start:end])
}
