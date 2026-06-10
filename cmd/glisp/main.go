// glisp is the compiler CLI for the glisp language.
// Usage:
//
//	glisp compile <file.glsp>          — transpile to <file.go>
//	glisp compile <file.glsp> -o out   — transpile to out.go
//	glisp build   <file.glsp>          — transpile + go build
//	glisp run     <file.glsp> [args]   — compile + run, no artifacts left behind
//	glisp print   <file.glsp>          — print Go output to stdout
//	glisp test    <file.glsp>          — compile + go test
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"golisp/internal/compiler"
	"golisp/internal/formatter"
	"golisp/internal/lsp"
	"golisp/internal/module"
	"golisp/internal/repl"
	"golisp/internal/transpiler"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "compile":
		compileCmd(os.Args[2:])
	case "build":
		buildCmd(os.Args[2:])
	case "run":
		runCmd(os.Args[2:])
	case "print":
		printCmd(os.Args[2:])
	case "test":
		testCmd(os.Args[2:])
	case "fmt":
		fmtCmd(os.Args[2:])
	case "repl":
		repl.Run(os.Stdin, os.Stdout)
	case "doc":
		docCmd(os.Args[2:])
	case "get":
		getCmd(os.Args[2:])
	case "mod":
		modCmd(os.Args[2:])
	case "version":
		fmt.Println("glisp 0.1.0")
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func compileCmd(args []string) {
	fs := flag.NewFlagSet("compile", flag.ExitOnError)
	out := fs.String("o", "", "output file (default: <input>.go)")
	strict := fs.Bool("strict", false, "require type annotations on all defn params, struct fields, and def globals")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "compile: requires <file.glsp>")
		os.Exit(1)
	}
	opts := compiler.Options{Strict: *strict}
	if err := compiler.CompileWithOptions(fs.Arg(0), *out, opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func buildCmd(args []string) {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	out := fs.String("o", "", "output binary")
	strict := fs.Bool("strict", false, "require type annotations on all defn params, struct fields, and def globals")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "build: requires <file.glsp> or <dir/>")
		os.Exit(1)
	}
	arg := fs.Arg(0)
	opts := compiler.Options{Strict: *strict}
	var buildErr error
	if info, err := os.Stat(arg); err == nil && info.IsDir() {
		buildErr = compiler.CompileDirWithOptions(arg, *out, opts)
	} else {
		buildErr = compiler.CompileAndBuildWithOptions(arg, *out, opts)
	}
	if buildErr != nil {
		fmt.Fprintln(os.Stderr, buildErr)
		os.Exit(1)
	}
}

func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	strict := fs.Bool("strict", false, "require type annotations on all defn params, struct fields, and def globals")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "run: requires <file.glsp> or <dir/>")
		os.Exit(1)
	}
	// Everything after the target is passed to the program untouched.
	code, err := compiler.Run(fs.Arg(0), compiler.Options{Strict: *strict}, fs.Args()[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if code != 0 {
		os.Exit(code)
	}
}

func printCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "print: requires <file.glsp>")
		os.Exit(1)
	}
	src, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out, err := transpiler.Transpile(string(src))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(out)
}

func testCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "test: requires <file.glsp>")
		os.Exit(1)
	}
	if err := compiler.CompileTest(args[0]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func fmtCmd(args []string) {
	fs := flag.NewFlagSet("fmt", flag.ExitOnError)
	check := fs.Bool("check", false, "exit non-zero if file is not already formatted")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "fmt: requires at least one <file.glsp>")
		os.Exit(1)
	}
	exitCode := 0
	for _, path := range fs.Args() {
		// "-" reads from stdin and writes the result to stdout (for editor integration).
		if path == "-" {
			src, err := io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			out, err := formatter.Format(string(src))
			if err != nil {
				fmt.Fprintf(os.Stderr, "<stdin>: %v\n", err)
				os.Exit(1)
			}
			if *check {
				if out != string(src) {
					exitCode = 1
				}
			} else {
				fmt.Print(out)
			}
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		out, err := formatter.Format(string(src))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
		if *check {
			if out != string(src) {
				fmt.Fprintf(os.Stderr, "%s: not formatted\n", path)
				exitCode = 1
			}
		} else {
			if err := os.WriteFile(path, []byte(out), 0644); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func normSig(sig string) string {
	parts := strings.SplitN(sig, "→", 2)
	if len(parts) != 2 {
		return sig
	}
	return strings.TrimRight(parts[0], " ") + "  →  " + strings.TrimLeft(parts[1], " ")
}

func docCmd(args []string) {
	if len(args) == 0 {
		names := make([]string, 0, len(lsp.BuiltinDocs))
		for name := range lsp.BuiltinDocs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Printf("%-20s  %s\n", name, normSig(lsp.BuiltinDocs[name].Sig))
		}
		return
	}
	name := args[0]
	bd, ok := lsp.BuiltinDocs[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "no doc for %q\n", name)
		os.Exit(1)
	}
	fmt.Printf("%s\n  %s\n", normSig(bd.Sig), bd.Doc)
}

func getCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "get: requires <module>[@version]")
		os.Exit(1)
	}
	spec := args[0]
	modulePath, version, _ := strings.Cut(spec, "@")
	if version == "" {
		version = "latest"
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := compiler.GetModule(cwd, modulePath, version); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Update glisp.mod
	mf, err := module.ReadModFile(cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	mf.AddRequire(modulePath, version)
	if err := module.WriteModFile(cwd, mf); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "added %s %s\n", modulePath, version)
}

func modCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "mod: requires subcommand (init)")
		os.Exit(1)
	}
	switch args[0] {
	case "init":
		modInitCmd(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "mod: unknown subcommand %q\n", args[0])
		os.Exit(1)
	}
}

func modInitCmd(args []string) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	modulePath := ""
	if len(args) > 0 {
		modulePath = args[0]
	}
	if err := module.InitModFile(cwd, modulePath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "created glisp.mod")
}

func usage() {
	fmt.Fprintln(os.Stderr, `glisp — Clojure-inspired language that transpiles to Go

Usage:
  glisp compile [-o output.go] [--strict] <file.glsp>   transpile to Go source
  glisp build   [-o binary]    [--strict] <file.glsp>   transpile + go build
  glisp build   [-o binary]    [--strict] <dir/>        compile all .glsp in dir
  glisp run     [--strict] <file.glsp|dir/> [args...]   compile and run (no artifacts)
  glisp print   <file.glsp>                  print Go output to stdout
  glisp test    <file.glsp>                  compile + run tests
  glisp fmt     [--check]      <file.glsp>   format source in-place (- = stdin→stdout)
  glisp doc     [name]                       show built-in docs (all if no name)
  glisp get     <module>[@version]           download + register a glisp module
  glisp mod     init [module-path]           create glisp.mod for a new module/app
  glisp repl                                 start interactive REPL
  glisp version                              print version`)
}
