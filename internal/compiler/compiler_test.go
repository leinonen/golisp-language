package compiler

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsNotGlispModuleErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"no glsp files", errors.New("transpile x: no .glsp files found in /tmp/x"), true},
		{"404 release", errors.New("download https://github.com/google/uuid/...latest.tar.gz: HTTP 404"), true},
		{"unsupported host", errors.New("unsupported module host (only github.com supported): gitlab.com/foo/bar"), true},
		{"transient network", errors.New("download ...: dial tcp: i/o timeout"), false},
		{"real glisp error", errors.New("parse error: unexpected token"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsNotGlispModuleErr(c.err); got != c.want {
				t.Errorf("IsNotGlispModuleErr(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestBuildError(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "missing go.mod",
			output: "go: go.mod file not found in current directory or any parent directory; see 'go help modules'",
			want:   "glisp mod init",
		},
		{
			name:   "missing dependency",
			output: "db.go:5:2: github.com/leinonen/glispdb@v0.1.0: replacement directory /x/y does not exist",
			want:   "glisp get",
		},
		{
			name:   "runtime helper error",
			output: "./glisp_runtime.go:42:1: syntax error",
			want:   "glisp bug",
		},
		{
			name:   "ordinary compile error",
			output: "./main.glsp:3: invalid operation: mismatched types",
			want:   ".glsp source",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := buildError(c.output)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("buildError(%q) = %q, want it to contain %q", c.output, err.Error(), c.want)
			}
		})
	}
}

func TestRunWithIO(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	dir := t.TempDir()

	t.Run("hello with args", func(t *testing.T) {
		src := `(ns main)
(defn main [] -> void
  (fmt/println "hello" (nth os/args 1)))
`
		path := filepath.Join(dir, "hello.glsp")
		if err := os.WriteFile(path, []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
		var out, errBuf bytes.Buffer
		code, err := RunWithIO(path, Options{}, []string{"world"}, nil, &out, &errBuf)
		if err != nil {
			t.Fatalf("RunWithIO: %v\nstderr: %s", err, errBuf.String())
		}
		if code != 0 {
			t.Fatalf("exit code = %d, want 0\nstderr: %s", code, errBuf.String())
		}
		if got := out.String(); !strings.Contains(got, "hello world") {
			t.Errorf("stdout = %q, want it to contain %q", got, "hello world")
		}
		// run leaves no .go artifact behind for a fresh single file
		if _, err := os.Stat(filepath.Join(dir, "hello.go")); !os.IsNotExist(err) {
			t.Errorf("hello.go left behind after run")
		}
	})

	t.Run("exit code propagates", func(t *testing.T) {
		src := `(ns main)
(defn main [] -> void
  (os/exit 3))
`
		path := filepath.Join(dir, "exit3.glsp")
		if err := os.WriteFile(path, []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
		var out, errBuf bytes.Buffer
		code, err := RunWithIO(path, Options{}, nil, nil, &out, &errBuf)
		if err != nil {
			t.Fatalf("RunWithIO: %v\nstderr: %s", err, errBuf.String())
		}
		if code != 3 {
			t.Errorf("exit code = %d, want 3", code)
		}
	})

	t.Run("missing target", func(t *testing.T) {
		if _, err := RunWithIO(filepath.Join(dir, "nope.glsp"), Options{}, nil, nil, io.Discard, io.Discard); err == nil {
			t.Error("expected an error for a missing target")
		}
	})

	t.Run("if-err recur in loop tail", func(t *testing.T) {
		// if-err with a recur in each branch, in loop-tail position: the recur must
		// lower to a loop continue (regression for the if-err loop-tail gap).
		src := `(ns main)
(defn sum-ints [xs []any] -> int
  (loop [i 0 acc 0]
    (if (>= i (count xs))
      acc
      (if-err [n err] (parse-int (str (nth xs i)))
        (recur (+ i 1) acc)
        (recur (+ i 1) (+ acc n))))))
(defn main [] -> void (fmt/println "sum" (sum-ints ["1" "x" "3" "10"])))
`
		path := filepath.Join(dir, "iferrloop.glsp")
		if err := os.WriteFile(path, []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
		var out, errBuf bytes.Buffer
		code, err := RunWithIO(path, Options{}, nil, nil, &out, &errBuf)
		if err != nil || code != 0 {
			t.Fatalf("RunWithIO: %v code=%d\nstderr: %s", err, code, errBuf.String())
		}
		if got := out.String(); !strings.Contains(got, "sum 14") { // 1+3+10, "x" skipped
			t.Errorf("stdout %q, want it to contain %q", got, "sum 14")
		}
	})

	t.Run("block expr coerces into typed position", func(t *testing.T) {
		// A symbol bound to an if/switch (which emit func() any) is `any`; using it
		// in a typed position must coerce (regression for the block-expr any gap).
		src := `(ns main)
(defn shout [s string] -> string (str/upper s))
(defn main [] -> void
  (let [name (if true "hello" "bye")]
    (fmt/println (shout name))))
`
		path := filepath.Join(dir, "blockany.glsp")
		if err := os.WriteFile(path, []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
		var out, errBuf bytes.Buffer
		code, err := RunWithIO(path, Options{}, nil, nil, &out, &errBuf)
		if err != nil || code != 0 {
			t.Fatalf("RunWithIO: %v code=%d\nstderr: %s", err, code, errBuf.String())
		}
		if got := out.String(); !strings.Contains(got, "HELLO") {
			t.Errorf("stdout %q, want it to contain %q", got, "HELLO")
		}
	})

	t.Run("proc run", func(t *testing.T) {
		src := `(ns main)
(defn main [] -> void
  (let [r (proc/run "echo" "hi" "there")]
    (fmt/println "out" (str/trim (:out r)) "exit" (:exit r) "ok" (:ok r)))
  (let [f (proc/run "false")]
    (fmt/println "false-exit" (:exit f) "false-ok" (:ok f))))
`
		path := filepath.Join(dir, "proc.glsp")
		if err := os.WriteFile(path, []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
		var out, errBuf bytes.Buffer
		code, err := RunWithIO(path, Options{}, nil, nil, &out, &errBuf)
		if err != nil || code != 0 {
			t.Fatalf("RunWithIO: %v code=%d\nstderr: %s", err, code, errBuf.String())
		}
		got := out.String()
		for _, want := range []string{"out hi there", "exit 0", "ok true", "false-exit 1", "false-ok false"} {
			if !strings.Contains(got, want) {
				t.Errorf("stdout %q missing %q", got, want)
			}
		}
	})

	t.Run("cli parse-opts", func(t *testing.T) {
		src := `(ns main)
(def specs []any
  [{:long "--port" :short "-p" :default 8080 :int true}
   {:long "--verbose" :short "-v" :flag true}])
(defn main [] -> void
  (let [p (cli/parse-opts (rest (sys/args)) specs)]
    (fmt/println "port" (:port (:options p))
                 "verbose" (:verbose (:options p))
                 "args" (:arguments p)
                 "errors" (:errors p))))
`
		path := filepath.Join(dir, "cli.glsp")
		if err := os.WriteFile(path, []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
		var out, errBuf bytes.Buffer
		code, err := RunWithIO(path, Options{}, []string{"-p", "3000", "-v", "extra"}, nil, &out, &errBuf)
		if err != nil || code != 0 {
			t.Fatalf("RunWithIO: %v code=%d\nstderr: %s", err, code, errBuf.String())
		}
		got := out.String()
		for _, want := range []string{"port 3000", "verbose true", "args [extra]", "errors []"} {
			if !strings.Contains(got, want) {
				t.Errorf("stdout %q missing %q", got, want)
			}
		}
	})
}

// TestRunMultiFileCrossType verifies that a directory build resolves struct
// field access and method dispatch on types declared in a sibling file
// (Phase 2e) — the whole package's declarations are collected before any file
// is emitted.
func TestRunMultiFileCrossType(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	dir := t.TempDir()
	// A directory build runs `go build <dir>`, which must resolve the target as
	// the main module — make the test's cwd the project dir.
	t.Chdir(dir)

	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("glisp.mod", "module crossmod\n")
	// types.glsp declares the struct, its method, and an interface.
	write("types.glsp", `(ns main)

(defstruct Circle radius float64)

(defmethod Circle Area [c] -> float64
  (* 3.14 (:radius c) (:radius c)))

(definterface Shape
  (Area [] -> float64))
`)
	// main.glsp uses Circle field access, its method, and the Shape interface —
	// none declared in this file.
	write("main.glsp", `(ns main)

(defn describe [s Shape] -> string
  (format "area=%.2f" (Area s)))

(defn main [] -> void
  (let [c (Circle. {:radius 2.0})]
    (fmt/println "radius:" (:radius c))
    (fmt/println "area:" (Area c))
    (fmt/println (describe c))))
`)

	var out, errBuf bytes.Buffer
	code, err := RunWithIO(dir, Options{}, nil, nil, &out, &errBuf)
	if err != nil {
		t.Fatalf("RunWithIO: %v\nstderr: %s", err, errBuf.String())
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", code, errBuf.String())
	}
	for _, want := range []string{"radius: 2", "area: 12.56", "area=12.56"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout missing %q\n--- stdout ---\n%s", want, out.String())
		}
	}
}

// TestRunConcreteSlices verifies the collection helpers work on concrete Go
// slices ([]string from os/args, []int from a typed def) — not just []any.
func TestRunConcreteSlices(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	dir := t.TempDir()
	src := `(ns main)

(def nums []int [3 1 2])

(defn main [] -> void
  (let [args (rest os/args)]
    (fmt/println "first:" (first args))
    (fmt/println "count:" (len args))
    (fmt/println "upper:" (join (map (fn [a] (upper-case (str a))) args) ","))
    (fmt/println "has-b:" (contains? args "b"))
    (fmt/println "get1:" (get os/args 1))
    (fmt/println "oob:" (get args 99))
    (fmt/println "sum:" (reduce (fn [acc n] (+ (int acc) (int n))) 0 nums))
    (fmt/println "min:" (first (sort nums)))))
`
	path := filepath.Join(dir, "slices.glsp")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code, err := RunWithIO(path, Options{}, []string{"a", "b"}, nil, &out, &errBuf)
	if err != nil {
		t.Fatalf("RunWithIO: %v\nstderr: %s", err, errBuf.String())
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", code, errBuf.String())
	}
	for _, want := range []string{
		"first: a", "count: 2", "upper: A,B", "has-b: true",
		"get1: a", "oob: <nil>", "sum: 6", "min: 1",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout missing %q\n--- stdout ---\n%s", want, out.String())
		}
	}
}

// TestRunStructFieldReflect verifies that (:field x) / (get x "field") work on
// an `any` value that holds a declared struct, via the reflect fallback in
// _glispGet — including kebab-case keys mapping to PascalCase Go fields and a
// missing field returning nil.
func TestRunStructFieldReflect(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	dir := t.TempDir()
	src := `(ns main)

(defstruct Book id string title string)
(defstruct Person first-name string age int)

(defn main [] -> void
  (let [bs [(Book. {:id "1" :title "Go"}) (Book. {:id "2" :title "Lisp"})]
        b (first bs)]
    (fmt/println "titles:" (join (map (fn [x] (:title x)) bs) ","))
    (fmt/println "kw:" (:title b))
    (fmt/println "get:" (get b "id")))
  (let [p (first [(Person. {:first-name "Ada" :age 36})])]
    (fmt/println "kebab:" (get p "first-name"))
    (fmt/println "age:" (:age p))
    (fmt/println "missing:" (:nope p))))
`
	path := filepath.Join(dir, "structs.glsp")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code, err := RunWithIO(path, Options{}, nil, nil, &out, &errBuf)
	if err != nil {
		t.Fatalf("RunWithIO: %v\nstderr: %s", err, errBuf.String())
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", code, errBuf.String())
	}
	for _, want := range []string{
		"titles: Go,Lisp", "kw: Go", "get: 1",
		"kebab: Ada", "age: 36", "missing: <nil>",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout missing %q\n--- stdout ---\n%s", want, out.String())
		}
	}
}

// TestRunDirFromForeignCWD guards the dir-build working directory: the build
// must run inside the target package's module, not the caller's CWD. This test
// process runs inside the golisp module, so before the fix a `go build <absDir>`
// of an external project failed with "directory … outside main module".
func TestRunDirFromForeignCWD(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "glisp.mod"), []byte("module example.com/dirbuild\n"), 0644); err != nil {
		t.Fatal(err)
	}
	src := `(ns main)
(defn main [] -> void
  (fmt/println "dir build ok"))
`
	if err := os.WriteFile(filepath.Join(dir, "main.glsp"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code, err := RunWithIO(dir, Options{}, nil, nil, &out, &errBuf)
	if err != nil {
		t.Fatalf("RunWithIO(dir): %v\nstderr: %s", err, errBuf.String())
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "dir build ok") {
		t.Errorf("stdout = %q, want it to contain %q", out.String(), "dir build ok")
	}
}
