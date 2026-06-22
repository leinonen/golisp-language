package lsp

import (
	"strings"

	"golisp/internal/ast"
	"golisp/internal/parser"
)

// DocumentSymbols returns the outline (top-level declarations) of source for
// textDocument/documentSymbol. Returns nil when the source cannot be parsed so
// the editor keeps the last good outline rather than flashing empty.
func DocumentSymbols(source string) []DocumentSymbol {
	nodes, err := parser.ParseString(source)
	if err != nil {
		return nil
	}
	lines := strings.Split(source, "\n")

	var syms []DocumentSymbol
	for _, node := range nodes {
		var name, detail string
		var kind int
		switch n := node.(type) {
		case *ast.NSDecl:
			name, kind = n.Name, SymbolModule
		case *ast.DefnDecl:
			name, detail, kind = n.Name, formatDefnSig(n), SymbolFunction
		case *ast.MacroDecl:
			name, detail, kind = n.Name, formatMacroSig(n), SymbolFunction
		case *ast.DefDecl:
			name, detail, kind = n.Name, formatDefSig(n), SymbolVariable
		case *ast.StructDecl:
			name, kind = n.Name, SymbolStruct
		case *ast.InterfaceDecl:
			name, kind = n.Name, SymbolInterface
		case *ast.MethodDecl:
			name, detail, kind = n.Name, formatMethodSig(n), SymbolMethod
		case *ast.DefTypeDecl:
			name, kind = n.Name, SymbolClass
		case *ast.DefTestDecl:
			name, kind = n.Name, SymbolFunction
		default:
			continue
		}
		if name == "" {
			continue
		}
		lineIdx := node.Pos().Line - 1
		full := Range{
			Start: Position{Line: lineIdx, Character: 0},
			End:   Position{Line: lineIdx, Character: lineLen(lines, lineIdx)},
		}
		sel := nameRange(lines, lineIdx, node.Pos().Column-1, name)
		syms = append(syms, DocumentSymbol{
			Name:           name,
			Detail:         detail,
			Kind:           kind,
			Range:          full,
			SelectionRange: sel,
		})
	}
	return syms
}

func lineLen(lines []string, idx int) int {
	if idx < 0 || idx >= len(lines) {
		return 0
	}
	return len([]rune(lines[idx]))
}

// nameRange locates the declaration name on its line so the editor can highlight
// just the identifier. It searches for the first whole-symbol occurrence of name
// at or after startCol (the opening paren), falling back to a 1-wide range at
// startCol when the name can't be found (keeps selectionRange ⊆ range).
func nameRange(lines []string, lineIdx, startCol int, name string) Range {
	fallback := Range{
		Start: Position{Line: lineIdx, Character: startCol},
		End:   Position{Line: lineIdx, Character: startCol + 1},
	}
	if lineIdx < 0 || lineIdx >= len(lines) {
		return fallback
	}
	runes := []rune(lines[lineIdx])
	nameRunes := []rune(name)
	nameLen := len(nameRunes)
	for i := startCol; i <= len(runes)-nameLen; i++ {
		if !runesMatch(runes[i:], nameRunes) {
			continue
		}
		startOk := i == 0 || !isSymbolRune(runes[i-1])
		endOk := i+nameLen >= len(runes) || !isSymbolRune(runes[i+nameLen])
		if startOk && endOk {
			return Range{
				Start: Position{Line: lineIdx, Character: i},
				End:   Position{Line: lineIdx, Character: i + nameLen},
			}
		}
	}
	return fallback
}
