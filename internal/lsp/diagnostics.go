package lsp

import (
	"fmt"
	"strings"

	"golisp/internal/lexer"
	"golisp/internal/parser"
)

// Diagnostics runs the glisp lexer and parser on source and returns LSP diagnostics.
// Returns nil (not empty slice) when source is clean.
func Diagnostics(source string) []Diagnostic {
	tokens, err := lexer.Tokenize(source)
	if err != nil {
		return []Diagnostic{errorToDiagnostic(err.Error())}
	}
	_, err = parser.Parse(tokens)
	if err != nil {
		return []Diagnostic{errorToDiagnostic(err.Error())}
	}
	return nil
}

// errorToDiagnostic converts a glisp error string to an LSP Diagnostic.
// Glisp errors start with "line:col: message" (1-based); LSP uses 0-based positions.
func errorToDiagnostic(msg string) Diagnostic {
	// Take only the first line — parser may append source context after a newline.
	firstLine := msg
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		firstLine = msg[:i]
	}

	// Split at first ": " to separate "N:M" from the rest.
	sep := strings.Index(firstLine, ": ")
	if sep < 0 {
		return Diagnostic{Severity: SeverityError, Source: "glisp", Message: msg}
	}
	coords := firstLine[:sep]
	message := firstLine[sep+2:]

	var line, col int
	if n, err := fmt.Sscanf(coords, "%d:%d", &line, &col); n != 2 || err != nil {
		return Diagnostic{Severity: SeverityError, Source: "glisp", Message: msg}
	}

	// Convert to 0-based.
	lspLine := max0(line - 1)
	lspChar := max0(col - 1)
	pos := Position{Line: lspLine, Character: lspChar}
	return Diagnostic{
		Range:    Range{Start: pos, End: pos},
		Severity: SeverityError,
		Source:   "glisp",
		Message:  message,
	}
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
