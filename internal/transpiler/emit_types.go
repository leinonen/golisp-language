package transpiler

import (
	"strings"
	"unicode"
)

// identToGo converts a Lisp identifier to a Go identifier.
//   my-func   → myFunc
//   MyType    → MyType (PascalCase preserved)
//   nil?      → isNil
//   send!     → send
//   *global*  → global
//   fmt/Println → fmt.Println  (slash→dot for qualified names)
//   _         → _
func identToGo(s string) string {
	if s == "_" {
		return "_"
	}
	// Package-qualified: contains / but not at start (not a division operator)
	if idx := strings.Index(s, "/"); idx > 0 {
		pkg := s[:idx]
		fn := s[idx+1:]
		return identToGo(pkg) + "." + fn
	}

	// Strip stars (earmuff convention *var*)
	s = strings.Trim(s, "*")

	// ? suffix → Is prefix  (nil? → isNil, empty? → isEmpty)
	hasQ := strings.HasSuffix(s, "?")
	if hasQ {
		s = s[:len(s)-1]
	}

	// ! suffix → strip (send! → send, reset! → reset)
	s = strings.TrimSuffix(s, "!")

	// Replace -> with -To- so ring->handler → ring-To-handler → ringToHandler
	s = strings.ReplaceAll(s, "->", "-To-")
	// Replace => with -Arrow- as fallback
	s = strings.ReplaceAll(s, "=>", "-Arrow-")

	// kebab-case → camelCase: split on -, title-case all parts after first
	parts := strings.Split(s, "-")
	var sb strings.Builder
	for i, part := range parts {
		if part == "" {
			continue
		}
		// Strip any remaining non-alphanumeric, non-underscore chars
		clean := cleanIdentPart(part)
		if clean == "" {
			continue
		}
		if i == 0 || sb.Len() == 0 {
			sb.WriteString(clean)
		} else {
			sb.WriteString(titleCase(clean))
		}
	}
	result := sb.String()

	// Apply Is prefix for ? predicates
	if hasQ {
		if len(result) > 0 {
			result = "is" + titleCase(result)
		} else {
			result = "is"
		}
	}

	return result
}

// cleanIdentPart removes characters invalid in Go identifiers from an ident part.
func cleanIdentPart(s string) string {
	var sb strings.Builder
	for _, ch := range s {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' {
			sb.WriteRune(ch)
		}
	}
	return sb.String()
}

// typeExprToGo converts a TypeExpr text to a Go type string.
// Input examples:
//   "int"             → "int"
//   "(chan int)"       → "chan int"       (strip outer parens)
//   "[string error]"  → "(string, error)" (multi-return, strip brackets)
//   "*http.Request"   → "*http.Request"  (unchanged)
//   "[]string"        → "[]string"       (unchanged)
func typeExprToGo(text string) string {
	text = strings.TrimSpace(text)
	// Multi-return: [T1 T2 ...]
	if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
		inner := strings.TrimSpace(text[1 : len(text)-1])
		parts := splitTypeList(inner)
		return "(" + strings.Join(parts, ", ") + ")"
	}
	// Channel type wrapped in parens: (chan T)
	if strings.HasPrefix(text, "(") && strings.HasSuffix(text, ")") {
		return strings.TrimSpace(text[1 : len(text)-1])
	}
	return text
}

// splitTypeList splits a space-separated list of Go type tokens,
// respecting brackets so "map[string]int string" splits correctly.
func splitTypeList(s string) []string {
	var parts []string
	var cur strings.Builder
	depth := 0
	for _, ch := range s {
		if ch == '[' || ch == '(' {
			depth++
			cur.WriteRune(ch)
		} else if ch == ']' || ch == ')' {
			depth--
			cur.WriteRune(ch)
		} else if unicode.IsSpace(ch) && depth == 0 {
			if cur.Len() > 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			}
		} else {
			cur.WriteRune(ch)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

func titleCase(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// zeroValueFor returns the Go zero value for a type string.
func zeroValueFor(typeText string) string {
	switch typeText {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "complex64", "complex128":
		return "0"
	case "string":
		return `""`
	case "bool":
		return "false"
	case "error":
		return "nil"
	}
	if strings.HasPrefix(typeText, "*") || strings.HasPrefix(typeText, "[]") ||
		strings.HasPrefix(typeText, "map[") || strings.HasPrefix(typeText, "chan ") ||
		strings.HasPrefix(typeText, "func(") {
		return "nil"
	}
	return "nil"
}
