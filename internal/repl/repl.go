// Package repl provides an interactive Read-Eval-Print Loop for glisp.
package repl

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golisp/internal/ast"
	"golisp/internal/lsp"
	"golisp/internal/parser"
	"golisp/internal/transpiler"
)

// Session holds accumulated top-level definitions so they persist across inputs.
type Session struct {
	defs []string
}

func New() *Session {
	return &Session{}
}

// Eval compiles and runs a single glisp input string.
// Returns (output, isDecl, error). On success with a declaration, the input
// is appended to the session for future evals.
func (s *Session) Eval(input string) (string, bool, error) {
	nodes, err := parser.ParseString(input)
	if err != nil {
		return "", false, err
	}
	if len(nodes) == 0 {
		return "", false, nil
	}

	decl := isDeclaration(nodes[0])
	prog := buildProgram(s.defs, input, decl)

	goSrc, err := transpiler.Transpile(prog)
	if err != nil {
		return "", decl, err
	}

	out, err := runGoSource(goSrc)
	if err != nil {
		return "", decl, err
	}

	if decl {
		s.defs = append(s.defs, input)
	}

	return strings.TrimRight(out, "\n"), decl, nil
}

// Run is the interactive REPL loop.
func Run(in io.Reader, out io.Writer) {
	r := bufio.NewReader(in)
	s := New()

	fmt.Fprintln(out, "glisp REPL  (ctrl-d to exit)")

	for {
		input, err := readExpr(r, out)
		if err == io.EOF {
			fmt.Fprintln(out, "\nBye!")
			return
		}
		if err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		if name, ok := parseDocCall(input); ok {
			if bd, found := lsp.BuiltinDocs[name]; found {
				fmt.Fprintf(out, "%s\n  %s\n", bd.Sig, bd.Doc)
			} else {
				fmt.Fprintf(out, "no doc for %q\n", name)
			}
			continue
		}

		result, isDecl, err := s.Eval(input)
		if err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
			continue
		}

		if isDecl {
			nodes, _ := parser.ParseString(input)
			if len(nodes) > 0 {
				fmt.Fprintf(out, "=> %s\n", declName(nodes[0]))
			}
		} else if result != "" {
			fmt.Fprintf(out, "%s\n", result)
		}
	}
}

func buildProgram(defs []string, input string, isDecl bool) string {
	var b strings.Builder
	b.WriteString("(ns main)\n")
	for _, d := range defs {
		b.WriteString(d)
		b.WriteByte('\n')
	}
	if isDecl {
		b.WriteString(input)
		b.WriteString("\n(defn -main [])\n")
	} else {
		b.WriteString("(defn -main []\n  (fmt/Println ")
		b.WriteString(input)
		b.WriteString("))\n")
	}
	return b.String()
}

func runGoSource(src string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "glisp-repl-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(tmpFile, []byte(src), 0644); err != nil {
		return "", err
	}

	cmd := exec.Command("go", "run", tmpFile)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}

func isDeclaration(n ast.Node) bool {
	switch n.(type) {
	case *ast.DefDecl, *ast.DefnDecl, *ast.StructDecl,
		*ast.InterfaceDecl, *ast.MethodDecl:
		return true
	}
	return false
}

func declName(n ast.Node) string {
	switch v := n.(type) {
	case *ast.DefDecl:
		return v.Name
	case *ast.DefnDecl:
		return v.Name
	case *ast.StructDecl:
		return v.Name
	case *ast.InterfaceDecl:
		return v.Name
	case *ast.MethodDecl:
		return v.Name
	}
	return ""
}

// readExpr reads a balanced parenthetical expression from r, printing prompts to w.
func readExpr(r *bufio.Reader, w io.Writer) (string, error) {
	var lines []string
	depth := 0
	first := true

	for {
		if first {
			fmt.Fprint(w, "=> ")
			first = false
		} else {
			fmt.Fprint(w, ".. ")
		}

		line, err := r.ReadString('\n')
		if line != "" {
			lines = append(lines, line)
			depth += parenDepth(line)
		}
		if err != nil {
			if err == io.EOF {
				if len(lines) == 0 {
					return "", io.EOF
				}
				break
			}
			return "", err
		}

		trimmed := strings.TrimSpace(strings.Join(lines, ""))
		if trimmed != "" && depth <= 0 {
			break
		}
	}

	return strings.Join(lines, ""), nil
}

// parseDocCall detects (doc name) and returns the name if matched.
func parseDocCall(input string) (string, bool) {
	s := strings.TrimSpace(input)
	if !strings.HasPrefix(s, "(doc ") || s[len(s)-1] != ')' {
		return "", false
	}
	name := strings.TrimSpace(s[5 : len(s)-1])
	if name == "" || strings.ContainsAny(name, " \t\n()[]{}") {
		return "", false
	}
	return name, true
}

// parenDepth counts the net open-bracket depth of a line, skipping string literals.
func parenDepth(s string) int {
	depth := 0
	inStr := false
	escaped := false
	for _, c := range s {
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inStr {
			escaped = true
			continue
		}
		if c == '"' {
			inStr = !inStr
			continue
		}
		if inStr {
			continue
		}
		switch c {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		}
	}
	return depth
}
