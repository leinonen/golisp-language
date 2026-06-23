// glisp is the compiler CLI for the glisp language.
// Usage:
//
//	glisp compile <file.glsp>          — transpile to <file.go>
//	glisp compile <file.glsp> -o out   — transpile to out.go
//	glisp build   <file.glsp>          — transpile + go build
//	glisp run     <file.glsp> [args]   — compile + run, no artifacts left behind
//	glisp         <file.glsp> [args]   — run a script directly (enables #! shebang)
//	glisp print   <file.glsp>          — print Go output to stdout
//	glisp test    <file.glsp>          — compile + go test
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"golisp/internal/ast"
	"golisp/internal/compiler"
	"golisp/internal/formatter"
	"golisp/internal/lsp"
	"golisp/internal/macro"
	"golisp/internal/module"
	"golisp/internal/parser"
	"golisp/internal/repl"
	"golisp/internal/transpiler"
	"golisp/internal/version"
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
	case "macroexpand":
		macroexpandCmd(os.Args[2:])
	case "repl":
		repl.Run(os.Stdin, os.Stdout)
	case "doc":
		docCmd(os.Args[2:])
	case "get":
		getCmd(os.Args[2:])
	case "mod":
		modCmd(os.Args[2:])
	case "version":
		fmt.Println(version.Full())
	default:
		// `glisp <file.glsp> [args]` runs the file — so a script with a
		// `#!/usr/bin/env glisp` shebang is directly executable (the kernel
		// invokes glisp with the script path as the first argument).
		if strings.HasSuffix(os.Args[1], ".glsp") {
			runCmd(os.Args[1:])
			return
		}
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

// macroexpandCmd prints the macro-expanded source of a file — a debugging aid
// for macro authors. By default it fully expands (what the transpiler sees);
// --once expands each top-level form's outermost macro a single step.
func macroexpandCmd(args []string) {
	fs := flag.NewFlagSet("macroexpand", flag.ExitOnError)
	once := fs.Bool("once", false, "expand each top-level form's outermost macro a single step")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "macroexpand: requires <file.glsp>")
		os.Exit(1)
	}
	path := fs.Arg(0)
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	nodes, err := parser.ParseString(string(src))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
		os.Exit(1)
	}
	var out []ast.Node
	if *once {
		out, err = macro.ExpandOnce(nodes, nil)
	} else {
		out, err = macro.Expand(nodes, nil)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
		os.Exit(1)
	}
	fmt.Print(formatter.FormatNodes(out))
}

func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	strict := fs.Bool("strict", false, "require type annotations on all defn params, struct fields, and def globals")
	watch := fs.Bool("watch", false, "rebuild and re-run on source changes (Ctrl-C to stop)")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "run: requires <file.glsp> or <dir/>")
		os.Exit(1)
	}
	opts := compiler.Options{Strict: *strict}
	if *watch {
		runWatch(fs.Arg(0), opts, fs.Args()[1:])
		return
	}
	// Everything after the target is passed to the program untouched.
	code, err := compiler.Run(fs.Arg(0), opts, fs.Args()[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if code != 0 {
		os.Exit(code)
	}
}

// runWatch rebuilds and re-runs target whenever a watched .glsp file changes,
// killing and restarting the previous process each time (so long-running
// programs like web servers restart cleanly). It polls modification times — no
// external dependency — and runs until interrupted (Ctrl-C). A build failure is
// reported and watching continues, so the next save can fix it.
func runWatch(target string, opts compiler.Options, progArgs []string) {
	tmpDir, err := os.MkdirTemp("", "glisp-watch-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	bin := filepath.Join(tmpDir, "app")

	var mu sync.Mutex
	var cur *exec.Cmd
	stop := func() {
		mu.Lock()
		defer mu.Unlock()
		if cur != nil {
			cur.Process.Kill() // ignore "already finished" for self-exited scripts
			cur.Wait()         // reap
			cur = nil
		}
	}
	start := func() {
		if err := compiler.BuildTarget(target, bin, opts); err != nil {
			fmt.Fprintf(os.Stderr, "glisp: build failed:\n%v\n", err)
			return
		}
		c := exec.Command(bin, progArgs...)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := c.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "glisp: %v\n", err)
			return
		}
		mu.Lock()
		cur = c
		mu.Unlock()
	}

	// Clean up the child and temp dir on Ctrl-C / SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		stop()
		os.RemoveAll(tmpDir)
		os.Exit(0)
	}()

	fmt.Fprintf(os.Stderr, "glisp: watching %s (Ctrl-C to stop)\n", target)
	start()
	last := watchSnapshot(watchFiles(target))
	for {
		time.Sleep(300 * time.Millisecond)
		if snap := watchSnapshot(watchFiles(target)); snap != last {
			last = snap
			fmt.Fprintf(os.Stderr, "\nglisp: change detected — rebuilding %s\n", target)
			stop()
			start()
		}
	}
}

// watchFiles returns the .glsp files to watch for target: the file itself, or
// every .glsp directly in a directory target.
func watchFiles(target string) []string {
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return []string{target}
	}
	entries, _ := os.ReadDir(target)
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".glsp") {
			files = append(files, filepath.Join(target, e.Name()))
		}
	}
	return files
}

// watchSnapshot fingerprints the watched files by path, mtime, and size, so any
// edit, addition, or removal changes the result.
func watchSnapshot(files []string) string {
	var b strings.Builder
	for _, f := range files {
		if st, err := os.Stat(f); err == nil {
			fmt.Fprintf(&b, "%s:%d:%d;", f, st.ModTime().UnixNano(), st.Size())
		}
	}
	return b.String()
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
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	goFlag := fs.Bool("go", false, "fetch the target as a Go package (skip glisp-module resolution)")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "get: requires <module>[@version]")
		os.Exit(1)
	}
	modulePath, version, _ := strings.Cut(fs.Arg(0), "@")
	if version == "" {
		version = "latest"
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Bootstrap a minimal glisp.mod (and, via EnsureProjectGoMod, go.mod) so
	// `glisp get` works in a fresh directory and the toolchain can wire the dep.
	if _, statErr := os.Stat(module.ModFilePath(cwd)); os.IsNotExist(statErr) {
		if err := module.InitModFile(cwd, filepath.Base(cwd)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if err := module.EnsureProjectGoMod(cwd); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Forced Go-package path, or auto: try glisp module first, fall back to a Go
	// package when the target clearly isn't a glisp module.
	if *goFlag {
		getGoPackage(cwd, modulePath, version)
		return
	}
	err = compiler.GetModule(cwd, modulePath, version)
	if compiler.IsNotGlispModuleErr(err) {
		fmt.Fprintf(os.Stderr, "glisp: %s is not a glisp module; fetching as a Go package\n", modulePath)
		getGoPackage(cwd, modulePath, version)
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Record the glisp module dependency in glisp.mod.
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

// getGoPackage fetches a Go package via the Go toolchain and records the
// resolved version under go-require in glisp.mod (the single source of truth),
// then exits non-zero on failure.
func getGoPackage(cwd, pkg, version string) {
	resolved, err := compiler.GetGoPackage(cwd, pkg, version)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if resolved == "" {
		resolved = version
	}
	mf, err := module.ReadModFile(cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	mf.AddGoRequire(pkg, resolved)
	if err := module.WriteModFile(cwd, mf); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "added Go package %s %s\n", pkg, resolved)
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
	// Also write go.mod so the project builds with the Go toolchain immediately
	// — glisp.mod alone never satisfied `go build` (see go-interop-exploration §2.4).
	if err := module.EnsureProjectGoMod(cwd); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "created glisp.mod and go.mod")
}

func usage() {
	fmt.Fprintln(os.Stderr, `glisp — Clojure-inspired language that transpiles to Go

Usage:
  glisp compile [-o output.go] [--strict] <file.glsp>   transpile to Go source
  glisp build   [-o binary]    [--strict] <file.glsp>   transpile + go build
  glisp build   [-o binary]    [--strict] <dir/>        compile all .glsp in dir
  glisp run     [--strict] <file.glsp|dir/> [args...]   compile and run (no artifacts)
  glisp run     --watch    <file.glsp|dir/> [args...]   re-run on source changes
  glisp         <file.glsp> [args...]        run a script (enables #! shebang)
  glisp print   <file.glsp>                  print Go output to stdout
  glisp test    <file.glsp>                  compile + run tests
  glisp fmt     [--check]      <file.glsp>   format source in-place (- = stdin→stdout)
  glisp macroexpand [--once]   <file.glsp>   show the macro-expanded source
  glisp doc     [name]                       show built-in docs (all if no name)
  glisp get     [-go] <module>[@version]     fetch a glisp module, or a Go package (-go / auto)
  glisp mod     init [module-path]           create glisp.mod for a new module/app
  glisp repl                                 start interactive REPL
  glisp version                              print version`)
}
