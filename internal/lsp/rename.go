package lsp

import "strings"

// FindRenameEdits returns TextEdits to rename the symbol at (line, col) to newName.
// Returns nil when no renameable symbol is found.
func FindRenameEdits(source string, line, col int, newName string) []TextEdit {
	name := symbolAtPosition(source, line, col)
	if name == "" {
		return nil
	}
	nameRunes := []rune(name)
	nameLen := len(nameRunes)

	var edits []TextEdit
	for lineIdx, lineStr := range strings.Split(source, "\n") {
		runes := []rune(lineStr)
		for i := 0; i <= len(runes)-nameLen; i++ {
			if !runesMatch(runes[i:], nameRunes) {
				continue
			}
			startOk := i == 0 || !isSymbolRune(runes[i-1])
			endOk := i+nameLen >= len(runes) || !isSymbolRune(runes[i+nameLen])
			if startOk && endOk {
				edits = append(edits, TextEdit{
					Range: Range{
						Start: Position{Line: lineIdx, Character: i},
						End:   Position{Line: lineIdx, Character: i + nameLen},
					},
					NewText: newName,
				})
			}
		}
	}
	return edits
}

func runesMatch(haystack, needle []rune) bool {
	if len(haystack) < len(needle) {
		return false
	}
	for i, r := range needle {
		if haystack[i] != r {
			return false
		}
	}
	return true
}
