package lsp

import "strings"

// findOccurrences returns the ranges of every whole-symbol occurrence of name in
// source. "Whole-symbol" means the match is not flanked by other symbol runes, so
// "sum" does not match inside "summary". Used by both rename and find-references.
//
// Matching is textual, so occurrences inside string literals are included; this
// mirrors rename's long-standing behaviour. Lines that are entirely a comment are
// skipped so a name mentioned in prose doesn't produce spurious references.
func findOccurrences(source, name string) []Range {
	if name == "" {
		return nil
	}
	nameRunes := []rune(name)
	nameLen := len(nameRunes)

	var ranges []Range
	for lineIdx, lineStr := range strings.Split(source, "\n") {
		if isCommentLine(lineStr) {
			continue
		}
		runes := []rune(lineStr)
		for i := 0; i <= len(runes)-nameLen; i++ {
			if !runesMatch(runes[i:], nameRunes) {
				continue
			}
			startOk := i == 0 || !isSymbolRune(runes[i-1])
			endOk := i+nameLen >= len(runes) || !isSymbolRune(runes[i+nameLen])
			if startOk && endOk {
				ranges = append(ranges, Range{
					Start: Position{Line: lineIdx, Character: i},
					End:   Position{Line: lineIdx, Character: i + nameLen},
				})
			}
		}
	}
	return ranges
}

// isCommentLine reports whether a line is blank-then-comment (starts with ; after
// optional leading whitespace).
func isCommentLine(line string) bool {
	t := strings.TrimLeft(line, " \t")
	return strings.HasPrefix(t, ";")
}

// FindReferences returns the ranges of every reference to the symbol at (line, col)
// within source, including the declaration itself. Returns nil when no symbol is
// under the cursor.
func FindReferences(source string, line, col int) []Range {
	name := symbolAtPosition(source, line, col)
	if name == "" {
		return nil
	}
	return findOccurrences(source, name)
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
