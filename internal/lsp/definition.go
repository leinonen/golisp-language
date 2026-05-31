package lsp

import (
	"golisp/internal/ast"
	"golisp/internal/parser"
)

// FindDefinition returns the Range of the definition of the symbol at (line, col) (0-based).
// Returns nil when no definition is found in the current source (builtins have no source location).
func FindDefinition(source string, line, col int) *Range {
	name := symbolAtPosition(source, line, col)
	if name == "" {
		return nil
	}
	return FindDeclByName(source, name)
}

// FindDeclByName searches source for a top-level declaration of name and returns its range.
func FindDeclByName(source, name string) *Range {
	nodes, err := parser.ParseString(source)
	if err != nil {
		return nil
	}
	for _, node := range nodes {
		var pos ast.Position
		var declName string
		switch n := node.(type) {
		case *ast.DefnDecl:
			pos, declName = n.Pos(), n.Name
		case *ast.DefDecl:
			pos, declName = n.Pos(), n.Name
		case *ast.StructDecl:
			pos, declName = n.Pos(), n.Name
		case *ast.InterfaceDecl:
			pos, declName = n.Pos(), n.Name
		}
		if declName == name {
			lspLine := pos.Line - 1
			lspChar := pos.Column - 1
			return &Range{
				Start: Position{Line: lspLine, Character: lspChar},
				End:   Position{Line: lspLine, Character: lspChar + 1},
			}
		}
	}
	return nil
}
