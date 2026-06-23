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
	if fn, ok := idx.lookupFunc("os", "Create"); !ok || len(fn.results) != 2 {
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

// TestArgCoercionInterop checks that an `any` argument at a typed Go parameter
// of a loaded function is coerced/asserted at the call site (ADR-015, Phase
// 12d), generalizing the math-only stdlibNumericParams table.
func TestArgCoercionInterop(t *testing.T) {
	idx := LoadGoPackages(".", map[string]string{
		"strings": "strings", "strconv": "strconv", "filepath": "path/filepath",
	})
	if idx == nil {
		t.Fatal("failed to load stdlib signatures")
	}
	emit := func(src string) string {
		ds, err := CollectDecls(nil, src, "")
		if err != nil {
			t.Fatalf("collect: %v", err)
		}
		ds.SetGoPackages(idx)
		out, _, err := TranspileNoRuntimeFileExt(src, "", ds, false)
		if err != nil {
			t.Fatalf("transpile: %v", err)
		}
		return out
	}

	cases := []struct{ name, src, want string }{
		{
			"string param coerces via _glispToString",
			`(ns main)
(defn f [xs []any] -> string (strings/to-upper (first xs)))`,
			"strings.ToUpper(_glispToString(_glispFirst(xs)))",
		},
		{
			"int param coerces via _glispToInt (non-math loaded fn)",
			`(ns main)
(defn f [xs []any] -> string (strconv/itoa (first xs)))`,
			"strconv.Itoa(_glispToInt(_glispFirst(xs)))",
		},
		{
			"variadic element type coerces per-arg, not the slice",
			`(ns main)
(defn f [xs []any] -> string (filepath/join (first xs) (second xs)))`,
			"filepath.Join(_glispToString(_glispFirst(xs)), _glispToString(_glispSecond(xs)))",
		},
		{
			"concrete arg is not wrapped",
			`(ns main)
(defn f [s string] -> string (strings/to-upper s))`,
			"strings.ToUpper(s)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := emit(tc.src); !strings.Contains(got, tc.want) {
				t.Errorf("expected %q in output:\n%s", tc.want, got)
			}
		})
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

// TestGoInteropDiagnostics covers the Phase 15f interop diagnostics: a literal
// argument of the wrong kind for a loaded Go parameter, and a multi-return
// external method used in single-value position (ADR-015, 12e).
func TestGoInteropDiagnostics(t *testing.T) {
	idx := LoadGoPackages(".", map[string]string{"strings": "strings"})
	if idx == nil {
		t.Fatal("failed to load strings signatures")
	}
	build := func(src string) error {
		ds, err := CollectDecls(nil, src, "")
		if err != nil {
			return err
		}
		ds.SetGoPackages(idx)
		_, _, err = TranspileNoRuntimeFileExt(src, "", ds, false)
		return err
	}

	// strings.Repeat(s string, count int): a string literal in the int slot.
	if err := build(`(ns main)
(defn f [] -> string (strings/repeat "x" "3"))`); err == nil {
		t.Error("expected a wrong-typed-arg error for a string literal in an int param")
	} else if !strings.Contains(err.Error(), "string literal") || !strings.Contains(err.Error(), "numeric") {
		t.Errorf("error %q should flag the string literal vs numeric param", err.Error())
	}
	// Correct literal kinds pass.
	if err := build(`(ns main)
(defn f [] -> string (strings/repeat "x" 3))`); err != nil {
		t.Errorf("correct literal kinds should pass, got: %v", err)
	}

	// strings.Builder.WriteString → (int, error): dot-free in single-value position.
	if err := build(`(ns main)
(defn f [b *strings/Builder] -> any (let [n (write-string b "hi")] n))`); err == nil {
		t.Error("expected a multi-return error for a dot-free external method as a single value")
	} else if !strings.Contains(err.Error(), "multiple values") {
		t.Errorf("error %q should mention 'multiple values'", err.Error())
	}
	// With if-err it is fine.
	if err := build(`(ns main)
(defn f [b *strings/Builder] -> any (if-err [n e] (write-string b "hi") e n))`); err != nil {
		t.Errorf("multi-return method with if-err should pass, got: %v", err)
	}
}

// TestGoCallArityValidation checks that, with loaded signatures in scope, a
// wrong-arity call to an imported Go function is a position-tagged transpile
// error (ADR-015, Phase 12e) rather than an opaque Go compile error, while
// correct arities — including variadic — pass through.
func TestGoCallArityValidation(t *testing.T) {
	idx := LoadGoPackages(".", map[string]string{"fmt": "fmt", "strings": "strings"})
	if idx == nil {
		t.Fatal("failed to load stdlib signatures")
	}
	transpile := func(body string) error {
		src := "(ns main)\n(defn f [s string xs []any] -> any " + body + ")"
		ds, err := CollectDecls(nil, src, "")
		if err != nil {
			return err
		}
		ds.SetGoPackages(idx)
		_, _, err = TranspileNoRuntimeFileExt(src, "", ds, false)
		return err
	}

	// Fixed-arity strings.ToUpper(string) string.
	if err := transpile(`(strings/to-upper s)`); err != nil {
		t.Errorf("correct arity should pass, got: %v", err)
	}
	if err := transpile(`(strings/to-upper s s)`); err == nil {
		t.Error("expected arity error for strings/to-upper with 2 args")
	} else if !strings.Contains(err.Error(), "expected 1") {
		t.Errorf("error %q should say 'expected 1'", err.Error())
	}

	// Fixed-arity strings.Repeat(string, int) string — too few args.
	if err := transpile(`(strings/repeat s)`); err == nil {
		t.Error("expected arity error for strings/repeat with 1 arg")
	} else if !strings.Contains(err.Error(), "expected 2") {
		t.Errorf("error %q should say 'expected 2'", err.Error())
	}

	// Variadic fmt.Sprintf(string, ...any) string: zero args is below the fixed
	// minimum (Sprintf has a single return, so the multi-return gate is silent).
	if err := transpile(`(fmt/sprintf)`); err == nil {
		t.Error("expected arity error for fmt/sprintf with no format arg")
	} else if !strings.Contains(err.Error(), "at least 1") {
		t.Errorf("error %q should say 'at least 1'", err.Error())
	}
	// Variadic with the format plus extra args is fine.
	if err := transpile(`(fmt/sprintf s s s)`); err != nil {
		t.Errorf("variadic call with extra args should pass, got: %v", err)
	}
}

// TestGoMethodDispatch checks dot-free method dispatch on interop values
// (ADR-015, Phase 12c/12e): a value whose external Go type is known — from a
// type annotation or a typed interop return — calls that type's methods without
// the (.Method o) form, and a non-existent method is a position-tagged error.
func TestGoMethodDispatch(t *testing.T) {
	idx := LoadGoPackages(".", map[string]string{"strings": "strings", "regexp": "regexp"})
	if idx == nil {
		t.Fatal("failed to load stdlib signatures")
	}
	if idx.methodSet("strings.Builder") == nil {
		t.Fatal("strings.Builder method set not loaded")
	}
	emit := func(src string) (string, error) {
		ds, err := CollectDecls(nil, src, "")
		if err != nil {
			return "", err
		}
		ds.SetGoPackages(idx)
		out, _, err := TranspileNoRuntimeFileExt(src, "", ds, false)
		return out, err
	}

	// 1. Dot-free dispatch on an annotated external-typed param.
	out, err := emit(`(ns main)
(defn f [b *strings/Builder] -> void (write-string b "hi"))`)
	if err != nil {
		t.Fatalf("dispatch on annotated param: %v", err)
	}
	if !strings.Contains(out, `b.WriteString("hi")`) {
		t.Errorf("expected b.WriteString(\"hi\") in:\n%s", out)
	}

	// 2. Return-type propagation: an interop function's typed result dispatches.
	out, err = emit(`(ns main)
(defn f [s string] -> bool
  (let [re (regexp/must-compile "x")] (match-string re s)))`)
	if err != nil {
		t.Fatalf("dispatch on interop return: %v", err)
	}
	if !strings.Contains(out, "re.MatchString(s)") {
		t.Errorf("expected re.MatchString(s) in:\n%s", out)
	}

	// 3. A non-existent method on a known external type is a diagnostic.
	if _, err := emit(`(ns main)
(defn f [b *strings/Builder] -> void (frobnicate b))`); err == nil {
		t.Error("expected error for non-existent method on strings.Builder")
	} else if !strings.Contains(err.Error(), "has no exported method") {
		t.Errorf("error %q should mention 'has no exported method'", err.Error())
	}
}

// TestGoFieldAccess checks dot-free field access on interop values (ADR-015,
// Phase 12e): a value whose external Go struct type is known reads its exported
// fields via (.-Field x) and (:field x), and a non-existent field is a
// position-tagged error.
func TestGoFieldAccess(t *testing.T) {
	idx := LoadGoPackages(".", map[string]string{"url": "net/url"})
	if idx == nil {
		t.Fatal("failed to load net/url signatures")
	}
	if fs := idx.fieldSet("url.URL"); fs == nil || fs["Scheme"] == "" {
		t.Fatalf("url.URL field set not loaded: %v", fs)
	}
	emit := func(src string) (string, error) {
		ds, err := CollectDecls(nil, src, "")
		if err != nil {
			return "", err
		}
		ds.SetGoPackages(idx)
		out, _, err := TranspileNoRuntimeFileExt(src, "", ds, false)
		return out, err
	}

	// 1. (.-Field x) interop accessor on an annotated external-typed param.
	out, err := emit(`(ns main)
(defn f [u *url/URL] -> string (.-Scheme u))`)
	if err != nil {
		t.Fatalf("(.-Scheme u): %v", err)
	}
	if !strings.Contains(out, "u.Scheme") {
		t.Errorf("expected u.Scheme in:\n%s", out)
	}

	// 2. (:field x) keyword access maps to the Go field, like a local struct.
	out, err = emit(`(ns main)
(defn f [u *url/URL] -> string (:host u))`)
	if err != nil {
		t.Fatalf("(:host u): %v", err)
	}
	if !strings.Contains(out, "u.Host") {
		t.Errorf("expected u.Host in:\n%s", out)
	}

	// 3. A non-existent field is a diagnostic, in both spellings.
	if _, err := emit(`(ns main)
(defn f [u *url/URL] -> string (.-Bogus u))`); err == nil {
		t.Error("expected error for (.-Bogus u)")
	} else if !strings.Contains(err.Error(), "has no exported field") {
		t.Errorf("error %q should mention 'has no exported field'", err.Error())
	}
	if _, err := emit(`(ns main)
(defn f [u *url/URL] -> string (:bogus u))`); err == nil {
		t.Error("expected error for (:bogus u)")
	} else if !strings.Contains(err.Error(), "has no exported field") {
		t.Errorf("error %q should mention 'has no exported field'", err.Error())
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
	if fn, ok := idx.lookupFunc("fmt", "Printf"); !ok {
		t.Error("fmt.Printf not found")
	} else if !fn.variadic {
		t.Errorf("fmt.Printf should be variadic, got %+v", fn)
	}

	// strings.ToUpper(s string) string — fixed arity, single return.
	fn, ok := idx.lookupFunc("strings", "ToUpper")
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
	if fn, ok := idx.lookupFunc("strings", "Join"); !ok {
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
	if _, ok := idx.lookupFunc("nope", "Anything"); ok {
		t.Error("bogus package should not appear in the index")
	}
}

// TestGoSignatures covers the exported LSP-facing hover/completion API
// (ADR-015, Phase 12f).
func TestGoSignatures(t *testing.T) {
	g := LoadGoSignatures(".", map[string]string{"strings": "strings"})

	sig, ok := g.Signature("strings/to-upper")
	if !ok {
		t.Fatal("no signature for strings/to-upper")
	}
	if !strings.Contains(sig, "ToUpper") || !strings.Contains(sig, "string") {
		t.Errorf("signature %q should describe ToUpper(string) string", sig)
	}

	// Unqualified or unknown → not ok.
	if _, ok := g.Signature("toupper"); ok {
		t.Error("unqualified symbol should have no Go signature")
	}

	// Kebab-case partial matches the PascalCase Go name.
	comps := g.Completions("strings", "to-up")
	found := false
	for _, c := range comps {
		if c.Name == "ToUpper" {
			found = true
			if !strings.Contains(c.Sig, "ToUpper") {
				t.Errorf("completion sig %q missing ToUpper", c.Sig)
			}
		}
	}
	if !found {
		t.Errorf("Completions(strings, to-up) should include ToUpper, got %+v", comps)
	}
}
