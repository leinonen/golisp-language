package lsp

// FindRenameEdits returns TextEdits to rename the symbol at (line, col) to newName.
// Returns nil when no renameable symbol is found.
func FindRenameEdits(source string, line, col int, newName string) []TextEdit {
	name := symbolAtPosition(source, line, col)
	if name == "" {
		return nil
	}
	occurrences := findOccurrences(source, name)
	if len(occurrences) == 0 {
		return nil
	}
	edits := make([]TextEdit, len(occurrences))
	for i, r := range occurrences {
		edits[i] = TextEdit{Range: r, NewText: newName}
	}
	return edits
}
