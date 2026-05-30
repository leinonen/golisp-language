// glisp is the compiler CLI for the glisp language.
// Usage:
//
//	glisp compile <file.glsp>          — transpile to <file.go>
//	glisp compile <file.glsp> -o out   — transpile to out.go
//	glisp build   <file.glsp>          — transpile + go build
//	glisp print   <file.glsp>          — print Go output to stdout
//	glisp test    <file.glsp>          — compile + go test
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"golisp/internal/compiler"
	"golisp/internal/formatter"
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
	case "print":
		printCmd(os.Args[2:])
	case "test":
		testCmd(os.Args[2:])
	case "fmt":
		fmtCmd(os.Args[2:])
	case "repl":
		repl.Run(os.Stdin, os.Stdout)
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
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "compile: requires <file.glsp>")
		os.Exit(1)
	}
	if err := compiler.Compile(fs.Arg(0), *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func buildCmd(args []string) {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	out := fs.String("o", "", "output binary")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "build: requires <file.glsp> or <dir/>")
		os.Exit(1)
	}
	arg := fs.Arg(0)
	var buildErr error
	if info, err := os.Stat(arg); err == nil && info.IsDir() {
		buildErr = compiler.CompileDir(arg, *out)
	} else {
		buildErr = compiler.CompileAndBuild(arg, *out)
	}
	if buildErr != nil {
		fmt.Fprintln(os.Stderr, buildErr)
		os.Exit(1)
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

func usage() {
	fmt.Fprintln(os.Stderr, `glisp — Clojure-inspired language that transpiles to Go

Usage:
  glisp compile [-o output.go] <file.glsp>   transpile to Go source
  glisp build   [-o binary]    <file.glsp>   transpile + go build
  glisp build   [-o binary]    <dir/>        compile all .glsp in dir
  glisp print   <file.glsp>                  print Go output to stdout
  glisp test    <file.glsp>                  compile + run tests
  glisp fmt     [--check]      <file.glsp>   format source in-place (- = stdin→stdout)
  glisp repl                                 start interactive REPL
  glisp version                              print version`)
}
