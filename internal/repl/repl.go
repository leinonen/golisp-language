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

	"github.com/chzyer/readline"
	"golang.org/x/term"

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
		// Multi-return in single-value context — retry as statement and show a hint.
		if isMultiReturnError(err.Error()) {
			stmtProg := buildProgramStmt(s.defs, input)
			if stmtGoSrc, stmtErr := transpiler.Transpile(stmtProg); stmtErr == nil {
				if _, runErr := runGoSource(stmtGoSrc); runErr == nil {
					hint := "; multi-return — use (if-err ...) or (let [[a b] ...]) to capture values"
					if decl {
						s.defs = append(s.defs, input)
					}
					return hint, decl, nil
				}
			}
		}
		return "", decl, err
	}

	if decl {
		s.defs = append(s.defs, input)
	}

	return strings.TrimRight(out, "\n"), decl, nil
}

// Run is the interactive REPL loop.
func Run(in io.Reader, out io.Writer) {
	s := New()
	fmt.Fprintln(out, "glisp REPL  (ctrl-d to exit)")

	if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		runWithReadline(s, out)
	} else {
		runWithBufio(s, bufio.NewReader(in), out)
	}
}

func runWithReadline(s *Session, out io.Writer) {
	histFile := ""
	if home, err := os.UserHomeDir(); err == nil {
		histFile = filepath.Join(home, ".glisp_history")
	}

	rl, err := readline.NewEx(&readline.Config{
		Prompt:      "=> ",
		HistoryFile: histFile,
	})
	if err != nil {
		// Fall back to bufio if readline init fails.
		runWithBufio(s, bufio.NewReader(os.Stdin), out)
		return
	}
	defer rl.Close()

	for {
		input, err := readExprRL(rl)
		if err == readline.ErrInterrupt {
			continue
		}
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

		if handleBuiltin(s, input, out) {
			continue
		}

		evalAndPrint(s, input, out)
	}
}

func runWithBufio(s *Session, r *bufio.Reader, out io.Writer) {
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

		if handleBuiltin(s, input, out) {
			continue
		}

		evalAndPrint(s, input, out)
	}
}

// handleBuiltin processes special REPL commands like (doc name). Returns true if handled.
func handleBuiltin(s *Session, input string, out io.Writer) bool {
	if name, ok := parseDocCall(input); ok {
		if bd, found := lsp.BuiltinDocs[name]; found {
			fmt.Fprintf(out, "%s\n  %s\n", bd.Sig, bd.Doc)
		} else {
			fmt.Fprintf(out, "no doc for %q\n", name)
		}
		return true
	}
	return false
}

func evalAndPrint(s *Session, input string, out io.Writer) {
	result, isDecl, err := s.Eval(input)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return
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

// buildProgramStmt emits the expression as a bare statement (no println wrapper).
// Used as a fallback for multi-return expressions.
func buildProgramStmt(defs []string, input string) string {
	var b strings.Builder
	b.WriteString("(ns main)\n")
	for _, d := range defs {
		b.WriteString(d)
		b.WriteByte('\n')
	}
	b.WriteString("(defn -main []\n  ")
	b.WriteString(input)
	b.WriteString(")\n")
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

// readExprRL reads a balanced expression using a readline instance.
func readExprRL(rl *readline.Instance) (string, error) {
	var lines []string
	depth := 0
	rl.SetPrompt("=> ")

	for {
		line, err := rl.Readline()
		if err != nil {
			return "", err
		}
		lines = append(lines, line)
		depth += parenDepth(line)

		trimmed := strings.TrimSpace(strings.Join(lines, ""))
		if trimmed != "" && depth <= 0 {
			break
		}
		rl.SetPrompt(".. ")
	}

	rl.SetPrompt("=> ")
	return strings.Join(lines, "\n"), nil
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

// isMultiReturnError reports whether a Go compile error is caused by a multi-return expression
// used where a single value is expected.
func isMultiReturnError(msg string) bool {
	return strings.Contains(msg, "multiple-value") ||
		strings.Contains(msg, "assignment mismatch") ||
		strings.Contains(msg, "used in value context")
}

// parenDepth counts the net open-bracket depth of a line, skipping string literals and comments.
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
		if c == ';' {
			break
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
