// Package compiler orchestrates the full pipeline: source → Go file → binary.
package compiler

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golisp/internal/transpiler"
)

// Compile reads a .glsp source file, transpiles it to Go, writes the output,
// and optionally runs gofmt on it.
func Compile(srcPath string, outPath string) error {
	src, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", srcPath, err)
	}

	goSrc, err := transpiler.Transpile(string(src))
	if err != nil {
		return fmt.Errorf("transpile %s: %w", srcPath, err)
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

// CompileAndBuild compiles a .glsp file to Go and then runs `go build`.
func CompileAndBuild(srcPath string, outBin string) error {
	goPath := strings.TrimSuffix(srcPath, filepath.Ext(srcPath)) + ".go"
	if err := Compile(srcPath, goPath); err != nil {
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
