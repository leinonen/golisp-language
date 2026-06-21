package transpiler

import (
	"strings"
	"testing"
)

// TestGoImportPaths checks that declared (:import …) packages are collected as
// qualifier → import path, the input to LoadGoPackages.
func TestGoImportPaths(t *testing.T) {
	src := `(ns main
  (:import [github.com/jackc/pgx/v5])
  (:import [github.com/google/uuid :as id]))
(defn f [] -> void nil)`
	ds, err := CollectDecls(nil, src, "")
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	paths := ds.GoImportPaths()
	if got := paths["pgx"]; got != "github.com/jackc/pgx/v5" {
		t.Errorf("pgx qualifier (via /vN convention) = %q, want github.com/jackc/pgx/v5", got)
	}
	if got := paths["id"]; got != "github.com/google/uuid" {
		t.Errorf(":as alias qualifier = %q, want github.com/google/uuid", got)
	}
}

// TestGoImportPathsStdlib checks that referenced stdlib qualifiers (seen in
// `pkg/fn` tokens) are resolved to their import paths, while built-in
// namespaces, ambiguous qualifiers, and glisp module aliases are excluded.
func TestGoImportPathsStdlib(t *testing.T) {
	src := `(ns main
  (:require [github.com/user/mathlib]))
(defn f [] -> void
  (let [_ (os/create "x")
        _ (filepath/join "a" "b")
        _ (json/encode {})
        _ (mathlib/add 1 2)
        _ (rand/intn 5)]
    nil))`
	ds, err := CollectDecls(nil, src, "")
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	paths := ds.GoImportPaths()

	if paths["os"] != "os" {
		t.Errorf("os should resolve to os, got %q", paths["os"])
	}
	if paths["filepath"] != "path/filepath" {
		t.Errorf("filepath should resolve to path/filepath, got %q", paths["filepath"])
	}
	if _, ok := paths["json"]; ok {
		t.Error("json is a built-in namespace and must not be loaded")
	}
	if _, ok := paths["mathlib"]; ok {
		t.Error("mathlib is a glisp module require, not stdlib — must not be loaded")
	}
	if _, ok := paths["rand"]; ok {
		t.Error("rand is ambiguous (crypto/rand|math/rand) — must not be auto-loaded")
	}
}

// TestMultiReturnInterop checks that an imported Go function whose loaded
// signature returns 2+ values, used in single-value position, becomes the
// friendly if-err diagnostic rather than a raw Go "assignment mismatch"
// (ADR-015, go-interop-exploration §3.5).
func TestMultiReturnInterop(t *testing.T) {
	idx := LoadGoPackages(".", map[string]string{"os": "os"})
	if idx == nil {
		t.Fatal("failed to load os signatures")
	}
	// os.Create returns (*os.File, error); os.Getenv returns string.
	if fn, ok := idx.lookup("os", "Create"); !ok || len(fn.results) != 2 {
		t.Fatalf("os.Create should have 2 results, got %+v (ok=%v)", fn, ok)
	}

	withIndex := func(src string) (string, error) {
		ds, err := CollectDecls(nil, src, "")
		if err != nil {
			return "", err
		}
		ds.SetGoPackages(idx)
		out, _, err := TranspileNoRuntimeFileExt(src, "", ds, false)
		return out, err
	}

	// Multi-return used as a single value → diagnostic.
	_, err := withIndex(`(ns main)
(defn f [] -> any (let [c (os/create "x")] c))`)
	if err == nil {
		t.Fatal("expected multi-return diagnostic for os/create as single value")
	}
	if !strings.Contains(err.Error(), "returns multiple values") || !strings.Contains(err.Error(), "if-err") {
		t.Errorf("error %q should mention multiple values and if-err", err.Error())
	}

	// Single-return interop fn in the same position is fine.
	if _, err := withIndex(`(ns main)
(defn f [] -> any (let [v (os/getenv "X")] v))`); err != nil {
		t.Errorf("single-return os/getenv should not be flagged: %v", err)
	}
}

// TestVariadicSpreadLoaderValidation checks that, with loaded signatures in
// scope, spreading into a non-variadic imported function is a transpile error
// while spreading into a variadic one is accepted (ADR-015, Phase 12a — the
// external-call validation 12b deferred to the loader).
func TestVariadicSpreadLoaderValidation(t *testing.T) {
	idx := LoadGoPackages(".", map[string]string{"fmt": "fmt", "strings": "strings"})
	if idx == nil {
		t.Fatal("failed to load stdlib signatures")
	}
	withIndex := func(src string) (string, error) {
		ds, err := CollectDecls(nil, src, "")
		if err != nil {
			return "", err
		}
		ds.SetGoPackages(idx)
		out, _, err := TranspileNoRuntimeFileExt(src, "", ds, false)
		return out, err
	}

	// Non-variadic: strings.ToUpper(string) string → spread is an error.
	if _, err := withIndex(`(ns main)
(defn f [xs] (strings/to-upper "x" & xs))`); err == nil {
		t.Error("expected error spreading into non-variadic strings/to-upper")
	} else if !strings.Contains(err.Error(), "is not variadic") {
		t.Errorf("error %q does not mention 'is not variadic'", err.Error())
	}

	// Variadic: fmt.Printf(string, ...any) → spread is accepted.
	out, err := withIndex(`(ns main)
(defn f [fmtStr string xs] (fmt/printf fmtStr & xs))`)
	if err != nil {
		t.Fatalf("unexpected error spreading into variadic fmt/printf: %v", err)
	}
	if !strings.Contains(out, "fmt.Printf(fmtStr, xs...)") {
		t.Errorf("expected fmt.Printf(fmtStr, xs...) in output:\n%s", out)
	}
}

// TestLoadGoPackages checks the go/packages signature extraction against the
// standard library (always available offline). It is the foundation of
// ADR-015 / Phase 12a: the transpiler reading Go signatures the way jank reads
// C++ via Clang.
func TestLoadGoPackages(t *testing.T) {
	idx := LoadGoPackages(".", map[string]string{
		"fmt":     "fmt",
		"strings": "strings",
	})
	if idx == nil {
		t.Fatal("expected a non-nil index for stdlib packages")
	}

	// fmt.Printf(format string, a ...any) — variadic.
	if fn, ok := idx.lookup("fmt", "Printf"); !ok {
		t.Error("fmt.Printf not found")
	} else if !fn.variadic {
		t.Errorf("fmt.Printf should be variadic, got %+v", fn)
	}

	// strings.ToUpper(s string) string — fixed arity, single return.
	fn, ok := idx.lookup("strings", "ToUpper")
	if !ok {
		t.Fatal("strings.ToUpper not found")
	}
	if fn.variadic {
		t.Errorf("strings.ToUpper should not be variadic")
	}
	if len(fn.params) != 1 || fn.params[0] != "string" {
		t.Errorf("strings.ToUpper params = %v, want [string]", fn.params)
	}
	if fn.ret != "string" {
		t.Errorf("strings.ToUpper ret = %q, want string", fn.ret)
	}

	// strings.Join([]string, string) string.
	if fn, ok := idx.lookup("strings", "Join"); !ok {
		t.Error("strings.Join not found")
	} else if len(fn.params) != 2 || fn.params[0] != "[]string" {
		t.Errorf("strings.Join params = %v, want [[]string string]", fn.params)
	}
}

// TestLoadGoPackagesDegrades verifies graceful degradation: an unresolvable
// package yields no entry rather than an error (untyped fallback).
func TestLoadGoPackagesDegrades(t *testing.T) {
	if idx := LoadGoPackages(".", nil); idx != nil {
		t.Errorf("empty paths should yield nil index, got %v", idx)
	}
	// A bogus import path must not panic or fail the caller; it is simply absent.
	idx := LoadGoPackages(".", map[string]string{"nope": "example.com/does/not/exist/ever"})
	if _, ok := idx.lookup("nope", "Anything"); ok {
		t.Error("bogus package should not appear in the index")
	}
}
