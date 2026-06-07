package lsp

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golisp/internal/lexer"
	"golisp/internal/parser"
	"golisp/internal/transpiler"
)

// Diagnostics runs the glisp lexer, parser, and transpiler on source and returns
// LSP diagnostics. filename is the .glsp file path (used for error position context).
// Returns nil (not empty slice) when source is clean.
func Diagnostics(source, filename string) []Diagnostic {
	tokens, err := lexer.Tokenize(source)
	if err != nil {
		return []Diagnostic{errorToDiagnostic(err.Error())}
	}
	_, err = parser.Parse(tokens)
	if err != nil {
		return []Diagnostic{errorToDiagnostic(err.Error())}
	}

	// Run the transpiler to catch semantic glisp errors (unsupported forms, etc.).
	_, terr := transpiler.TranspileFile(source, filename)
	if terr != nil {
		var te *transpiler.TranspileError
		if errors.As(terr, &te) {
			return []Diagnostic{atPosToDiagnostic(te.Err.Error())}
		}
		return []Diagnostic{errorToDiagnostic(terr.Error())}
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

// atPosToDiagnostic parses a transpiler error in "message at line:col" or
// "message at file:line:col" format (1-based). Falls back to position 0:0 when
// no position info is present.
func atPosToDiagnostic(msg string) Diagnostic {
	if idx := strings.LastIndex(msg, " at "); idx >= 0 {
		posStr := msg[idx+4:]
		parts := strings.Split(posStr, ":")
		if len(parts) >= 2 {
			l, err1 := strconv.Atoi(parts[len(parts)-2])
			c, err2 := strconv.Atoi(parts[len(parts)-1])
			if err1 == nil && err2 == nil {
				pos := Position{Line: max0(l - 1), Character: max0(c - 1)}
				return Diagnostic{
					Range:    Range{Start: pos, End: pos},
					Severity: SeverityError,
					Source:   "glisp",
					Message:  msg[:idx],
				}
			}
		}
	}
	return Diagnostic{Severity: SeverityError, Source: "glisp", Message: msg}
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
