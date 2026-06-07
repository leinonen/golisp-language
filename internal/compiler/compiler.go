// Package compiler orchestrates the full pipeline: source → Go file → binary.
package compiler

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golisp/internal/module"
	"golisp/internal/transpiler"
)

// Options configures compilation behavior.
type Options struct {
	Strict bool // require type annotations on defn params, struct fields, and def globals
}

// Compile reads a .glsp source file, transpiles it to Go, writes the output,
// and optionally runs gofmt on it.
func Compile(srcPath string, outPath string) error {
	return CompileWithOptions(srcPath, outPath, Options{})
}

// CompileWithOptions is like Compile but accepts compilation options.
func CompileWithOptions(srcPath string, outPath string, opts Options) error {
	src, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", srcPath, err)
	}

	absSrcPath, err := filepath.Abs(srcPath)
	if err != nil {
		return fmt.Errorf("abs path %s: %w", srcPath, err)
	}

	var goSrc string
	if opts.Strict {
		goSrc, err = transpiler.TranspileFileStrict(string(src), absSrcPath)
	} else {
		goSrc, err = transpiler.TranspileFile(string(src), absSrcPath)
	}
	if err != nil {
		var pe *transpiler.ParseError
		var te *transpiler.TranspileError
		switch {
		case errors.As(err, &pe):
			return fmt.Errorf("%s: parse error: %w", srcPath, pe.Err)
		case errors.As(err, &te):
			return fmt.Errorf("%s: transpile error: %w", srcPath, te.Err)
		default:
			return fmt.Errorf("%s: %w", srcPath, err)
		}
	}

	if outPath == "" {
		outPath = strings.TrimSuffix(srcPath, filepath.Ext(srcPath)) + ".go"
	}

	if err := os.WriteFile(outPath, []byte(goSrc), 0644); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}

	// Run gofmt; failures are non-fatal (still usable Go, just unformatted)
	if cmd := exec.Command("gofmt", "-w", outPath); cmd != nil {
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "gofmt warning: %v\n%s\n", err, out)
		}
	}

	return nil
}

// CompileTest compiles a .glsp file to a _test.go file and runs `go test`.
func CompileTest(srcPath string) error {
	base := strings.TrimSuffix(srcPath, filepath.Ext(srcPath))
	goPath := base + "_test.go"
	if err := Compile(srcPath, goPath); err != nil {
		return err
	}
	cmd := exec.Command("go", "test", filepath.Base(goPath))
	cmd.Dir = filepath.Dir(srcPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go test: %w", err)
	}
	return nil
}

// CompileAndBuild compiles a .glsp file to Go and then runs `go build`.
func CompileAndBuild(srcPath string, outBin string) error {
	return CompileAndBuildWithOptions(srcPath, outBin, Options{})
}

// CompileAndBuildWithOptions is like CompileAndBuild but accepts compilation options.
func CompileAndBuildWithOptions(srcPath string, outBin string, opts Options) error {
	goPath := strings.TrimSuffix(srcPath, filepath.Ext(srcPath)) + ".go"
	if err := CompileWithOptions(srcPath, goPath, opts); err != nil {
		return err
	}

	args := []string{"build"}
	if outBin != "" {
		args = append(args, "-o", outBin)
	}
	args = append(args, goPath)

	cmd := exec.Command("go", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build: %w", err)
	}
	return nil
}

// TranspileDir transpiles all .glsp files in srcDir to Go source without building.
// Use this when compiling a dependency module that will be imported by another package.
func TranspileDir(srcDir string) error {
	return compileDir(srcDir, "", false, Options{})
}

// GetModule downloads, transpiles, and registers a glisp module in projectDir.
// modulePath is like "github.com/user/lib" or "./local/path"; version like "v1.0.0".
func GetModule(projectDir, modulePath, version string) error {
	moduleDir, err := module.Download(projectDir, modulePath, version)
	if err != nil {
		return err
	}

	// Resolve canonical module path for local modules
	if module.IsLocalPath(modulePath) {
		modulePath = module.ResolveModulePath(moduleDir, filepath.Base(moduleDir))
	}

	// Read the module's glisp.mod to discover its Go dependencies
	modFile, err := module.ReadModFile(moduleDir)
	if err != nil {
		return fmt.Errorf("read glisp.mod for %s: %w", modulePath, err)
	}

	if err := module.EnsureGoMod(moduleDir, modulePath, modFile.GoRequires); err != nil {
		return fmt.Errorf("ensure go.mod for %s: %w", modulePath, err)
	}

	if err := TranspileDir(moduleDir); err != nil {
		return fmt.Errorf("transpile %s: %w", modulePath, err)
	}

	if err := module.RegisterInGoMod(projectDir, modulePath, version, moduleDir); err != nil {
		return err
	}

	// Propagate the module's Go dependencies into the project's go.mod
	for _, gr := range modFile.GoRequires {
		ref := gr.Path + "@" + gr.Version
		cmd := exec.Command("go", "mod", "edit", "-require="+ref)
		cmd.Dir = projectDir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("go mod edit -require=%s in project: %w\n%s", ref, err, out)
		}
	}

	return nil
}

// ResolveDeps ensures all requires declared in glisp.mod are downloaded and registered.
func ResolveDeps(projectDir string) error {
	mf, err := module.ReadModFile(projectDir)
	if err != nil {
		return err
	}
	for _, req := range mf.Requires {
		if module.IsCached(req.Path, req.Version) {
			continue
		}
		fmt.Fprintf(os.Stderr, "glisp: fetching %s %s\n", req.Path, req.Version)
		if err := GetModule(projectDir, req.Path, req.Version); err != nil {
			return fmt.Errorf("get %s: %w", req.Path, err)
		}
	}
	return nil
}

// CompileDir compiles all .glsp files in srcDir into a single Go package and
// builds a binary. All files must share the same package name (ns last segment).
// Each .glsp produces a .go file in the same directory; a shared glisp_runtime.go
// is written with the union of runtime helpers needed across all files.
func CompileDir(srcDir string, outBin string) error {
	return CompileDirWithOptions(srcDir, outBin, Options{})
}

// CompileDirWithOptions is like CompileDir but accepts compilation options.
func CompileDirWithOptions(srcDir string, outBin string, opts Options) error {
	return compileDir(srcDir, outBin, true, opts)
}

func compileDir(srcDir string, outBin string, build bool, opts Options) error {
	// Resolve glisp module dependencies before transpiling
	if _, err := os.Stat(module.ModFilePath(srcDir)); err == nil {
		if err := ResolveDeps(srcDir); err != nil {
			return fmt.Errorf("resolve deps: %w", err)
		}
	}

	globs, err := filepath.Glob(filepath.Join(srcDir, "*.glsp"))
	if err != nil {
		return fmt.Errorf("glob %s: %w", srcDir, err)
	}
	if len(globs) == 0 {
		return fmt.Errorf("no .glsp files found in %s", srcDir)
	}

	type fileResult struct {
		goPath  string
		goSrc   string
		pkgName string
		builtins map[string]bool
	}

	results := make([]fileResult, 0, len(globs))
	mergedBuiltins := map[string]bool{}

	for _, srcPath := range globs {
		src, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", srcPath, err)
		}

		absSrcPath, _ := filepath.Abs(srcPath)
		var goSrc string
		var builtins map[string]bool
		if opts.Strict {
			goSrc, builtins, err = transpiler.TranspileNoRuntimeFileStrict(string(src), absSrcPath)
		} else {
			goSrc, builtins, err = transpiler.TranspileNoRuntimeFile(string(src), absSrcPath)
		}
		if err != nil {
			var pe *transpiler.ParseError
			var te *transpiler.TranspileError
			switch {
			case errors.As(err, &pe):
				return fmt.Errorf("%s: parse error: %w", srcPath, pe.Err)
			case errors.As(err, &te):
				return fmt.Errorf("%s: transpile error: %w", srcPath, te.Err)
			default:
				return fmt.Errorf("%s: %w", srcPath, err)
			}
		}

		// Extract package name from first line of generated source ("package <name>")
		pkgName := "main"
		if first := strings.SplitN(goSrc, "\n", 2)[0]; strings.HasPrefix(first, "package ") {
			pkgName = strings.TrimPrefix(first, "package ")
		}

		goPath := strings.TrimSuffix(srcPath, filepath.Ext(srcPath)) + ".go"
		results = append(results, fileResult{goPath: goPath, goSrc: goSrc, pkgName: pkgName, builtins: builtins})
		for k := range builtins {
			mergedBuiltins[k] = true
		}
	}

	// All files must share the same package name
	pkgName := results[0].pkgName
	for _, r := range results[1:] {
		if r.pkgName != pkgName {
			return fmt.Errorf("mixed package names in %s: %q and %q (all files must share the same ns)", srcDir, pkgName, r.pkgName)
		}
	}

	// Write per-file Go sources
	for _, r := range results {
		if err := os.WriteFile(r.goPath, []byte(r.goSrc), 0644); err != nil {
			return fmt.Errorf("write %s: %w", r.goPath, err)
		}
		if cmd := exec.Command("gofmt", "-w", r.goPath); cmd != nil {
			if out, err := cmd.CombinedOutput(); err != nil {
				fmt.Fprintf(os.Stderr, "gofmt warning: %v\n%s\n", err, out)
			}
		}
	}

	// Write shared runtime file
	runtimePath := filepath.Join(srcDir, "glisp_runtime.go")
	runtimeSrc := transpiler.RuntimeSource(pkgName, mergedBuiltins)
	if err := os.WriteFile(runtimePath, []byte(runtimeSrc), 0644); err != nil {
		return fmt.Errorf("write %s: %w", runtimePath, err)
	}
	if cmd := exec.Command("gofmt", "-w", runtimePath); cmd != nil {
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "gofmt warning: %v\n%s\n", err, out)
		}
	}

	if !build {
		return nil
	}

	// Build the package directory (absolute path so go build resolves it correctly)
	absSrcDir, err := filepath.Abs(srcDir)
	if err != nil {
		return fmt.Errorf("abs path %s: %w", srcDir, err)
	}
	args := []string{"build"}
	if outBin != "" {
		args = append(args, "-o", outBin)
	}
	args = append(args, absSrcDir)

	cmd := exec.Command("go", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build: %w", err)
	}
	return nil
}
