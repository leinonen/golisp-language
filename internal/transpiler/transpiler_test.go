package transpiler

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

// gofmtString runs gofmt on Go source and returns the formatted version.
// If gofmt is unavailable or fails, returns the original.
func gofmtString(src string) string {
	cmd := exec.Command("gofmt")
	cmd.Stdin = bytes.NewReader([]byte(src))
	out, err := cmd.Output()
	if err != nil {
		return src
	}
	return string(out)
}

func TestTranspileGolden(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Skip("no testdata directory")
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".glsp") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".glsp")
		t.Run(name, func(t *testing.T) {
			srcPath := filepath.Join("testdata", entry.Name())
			src, err := os.ReadFile(srcPath)
			if err != nil {
				t.Fatalf("read source: %v", err)
			}
			raw, err := Transpile(string(src))
			if err != nil {
				t.Fatalf("transpile error: %v", err)
			}
			got := gofmtString(raw)
			goldenPath := filepath.Join("testdata", name+".go.golden")
			if *update {
				if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				return
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden (run with -update to create): %v", err)
			}
			if got != string(want) {
				t.Errorf("output mismatch\n--- want ---\n%s\n--- got ---\n%s", string(want), got)
			}
		})
	}
}

// TestTranspileSnippets verifies small snippets produce expected Go output
// without golden files.
func TestTranspileSnippets(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantSub string // substring that must appear in output
	}{
		{
			name:    "defn basic",
			src:     `(defn ^int double [^int x] (* x 2))`,
			wantSub: "func double(x int) int {",
		},
		{
			name:    "def var",
			src:     `(def ^int answer 42)`,
			wantSub: "var answer int = 42",
		},
		{
			name:    "goroutine",
			src:     `(defn run [] (go (println "hi")))`,
			wantSub: "go func() {",
		},
		{
			name:    "channel",
			src:     `(defn make-ch [] (chan ^int 5))`,
			wantSub: "make(chan int, 5)",
		},
		{
			name:    "send recv",
			src:     `(defn send-it [^(chan int) ch] (send! ch 42))`,
			wantSub: "ch <- 42",
		},
		{
			name:    "recv",
			src:     `(defn get-it [^(chan int) ch] ^int (recv! ch))`,
			wantSub: "<-ch",
		},
		{
			name:    "method call",
			src:     `(defn greet [^*Writer w] (.Write w "hi"))`,
			wantSub: "w.Write(",
		},
		{
			name:    "field access",
			src:     `(defn get-method [^*Request r] (.-Method r))`,
			wantSub: "r.Method",
		},
		{
			name:    "if-err",
			src:     `(defn ^[string error] safe-div [^float64 a ^float64 b] (if-err [r err] (divide a b) (values "" err) (values r nil)))`,
			wantSub: "if err != nil {",
		},
		{
			name:    "pkg qualified call",
			src:     `(defn greet [] (fmt/Println "hello"))`,
			wantSub: "fmt.Println(",
		},
		{
			name:    "map literal",
			src:     `(def m {:status 200 :body "ok"})`,
			wantSub: `map[string]any{"status": 200, "body": "ok"}`,
		},
		{
			name:    "vector literal",
			src:     `(def v [1 2 3])`,
			wantSub: "[]any{1, 2, 3}",
		},
		{
			name:    "struct decl",
			src:     `(defstruct User ^string name ^int age)`,
			wantSub: "type User struct {",
		},
		{
			name:    "defer",
			src:     `(defn f [] (defer (println "done")) (println "body"))`,
			wantSub: "defer ",
		},
		// 2a: collection operations
		{
			name:    "map",
			src:     `(defn f [coll] (map (fn [x] x) coll))`,
			wantSub: "_glispMap(",
		},
		{
			name:    "filter",
			src:     `(defn f [coll] (filter (fn [x] x) coll))`,
			wantSub: "_glispFilter(",
		},
		{
			name:    "reduce",
			src:     `(defn f [coll] (reduce (fn [acc x] acc) 0 coll))`,
			wantSub: "_glispReduce(",
		},
		{
			name:    "take",
			src:     `(defn f [coll] (take 3 coll))`,
			wantSub: "_glispTake(",
		},
		{
			name:    "drop",
			src:     `(defn f [coll] (drop 2 coll))`,
			wantSub: "_glispDrop(",
		},
		{
			name:    "reverse",
			src:     `(defn f [coll] (reverse coll))`,
			wantSub: "_glispReverse(",
		},
		{
			name:    "contains?",
			src:     `(defn f [m] (contains? m "key"))`,
			wantSub: "_glispContains(",
		},
		{
			name:    "some",
			src:     `(defn f [coll] (some (fn [x] x) coll))`,
			wantSub: "_glispSome(",
		},
		{
			name:    "every?",
			src:     `(defn f [coll] (every? (fn [x] x) coll))`,
			wantSub: "_glispEvery(",
		},
		{
			name:    "sort-by",
			src:     `(defn f [coll] (sort-by (fn [x] x) coll))`,
			wantSub: "_glispSortBy(",
		},
		{
			name:    "flatten",
			src:     `(defn f [coll] (flatten coll))`,
			wantSub: "_glispFlatten(",
		},
		{
			name:    "range",
			src:     `(defn f [] (range 10))`,
			wantSub: "_glispRange(",
		},
		// 2b: string operations
		{
			name:    "upper-case",
			src:     `(defn f [^string s] (upper-case s))`,
			wantSub: "strings.ToUpper(",
		},
		{
			name:    "lower-case",
			src:     `(defn f [^string s] (lower-case s))`,
			wantSub: "strings.ToLower(",
		},
		{
			name:    "trim",
			src:     `(defn f [^string s] (trim s))`,
			wantSub: "strings.TrimSpace(",
		},
		{
			name:    "starts-with?",
			src:     `(defn f [^string s] (starts-with? s "foo"))`,
			wantSub: "strings.HasPrefix(",
		},
		{
			name:    "ends-with?",
			src:     `(defn f [^string s] (ends-with? s "bar"))`,
			wantSub: "strings.HasSuffix(",
		},
		{
			name:    "replace",
			src:     `(defn f [^string s] (replace s "a" "b"))`,
			wantSub: "strings.ReplaceAll(",
		},
		{
			name:    "split",
			src:     `(defn f [^string s] (split s ","))`,
			wantSub: "_glispSplit(",
		},
		{
			name:    "join",
			src:     `(defn f [coll] (join coll ","))`,
			wantSub: "_glispJoin(",
		},
		{
			name:    "subs two-arg",
			src:     `(defn f [^string s] (subs s 2))`,
			wantSub: "_glispToString(s))[2:]",
		},
		{
			name:    "subs three-arg",
			src:     `(defn f [^string s] (subs s 1 4))`,
			wantSub: "_glispToString(s))[1:4]",
		},
		// 2d: test framework
		{
			name:    "deftest emits test func",
			src:     `(ns main) (deftest my-test (assert= (+ 1 2) 3))`,
			wantSub: "func TestMyTest(t *testing.T)",
		},
		{
			name:    "assert= emits comparison",
			src:     `(ns main) (deftest t (assert= 1 1))`,
			wantSub: `t.Errorf("assert= failed`,
		},
		{
			name:    "assert-true emits bool check",
			src:     `(ns main) (deftest t (assert-true true))`,
			wantSub: `t.Errorf("assert-true failed")`,
		},
		{
			name:    "assert-nil emits nil check",
			src:     `(ns main) (deftest t (assert-nil nil))`,
			wantSub: `t.Errorf("assert-nil failed`,
		},
		// fn default return type
		{
			name:    "fn defaults to any return",
			src:     `(defn f [] (map (fn [x] x) []))`,
			wantSub: "func(x any) any {",
		},
		// 3a: JSON operations
		{
			name:    "json/encode emits runtime call",
			src:     `(defn f [data] (json/encode data))`,
			wantSub: "_glispJsonEncode(",
		},
		{
			name:    "json/encode adds encoding/json import",
			src:     `(defn f [data] (json/encode data))`,
			wantSub: `"encoding/json"`,
		},
		{
			name:    "json/decode emits runtime call",
			src:     `(defn f [^string s] (json/decode s))`,
			wantSub: "_glispJsonDecode(",
		},
		// 6b: Destructuring
		{
			name:    "let sequential destructure uses _glispGet",
			src:     `(defn f [v] (let [[a b] v] a))`,
			wantSub: "_glispGet(_v",
		},
		{
			name:    "let sequential destructure index 0",
			src:     `(defn f [v] (let [[a b] v] a))`,
			wantSub: "int64(0)",
		},
		{
			name:    "let map destructure uses _glispGet with string key",
			src:     `(defn f [m] (let [{x :x} m] x))`,
			wantSub: `_glispGet(_m`,
		},
		{
			name:    "let map destructure key name",
			src:     `(defn f [m] (let [{x :x} m] x))`,
			wantSub: `"x"`,
		},
		{
			name:    "fn vector param destructure",
			src:     `(defn f [] (fn [[a b]] a))`,
			wantSub: "_glispGet(_p",
		},
		{
			name:    "fn map param destructure",
			src:     `(defn f [] (fn [{n :name}] n))`,
			wantSub: `"name"`,
		},
		{
			name:    "defn with destructured param",
			src:     `(defn greet [{name :name}] name)`,
			wantSub: `_glispGet(_p`,
		},
		{
			name:    "keyword as fn 1-arg",
			src:     `(defn f [m] (:name m))`,
			wantSub: `_glispGet(m, "name")`,
		},
		{
			name:    "keyword as fn 2-arg default",
			src:     `(defn f [m] (:age m 0))`,
			wantSub: `_glispGetD(m, "age", 0)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Transpile(tt.src)
			if err != nil {
				t.Fatalf("transpile error: %v", err)
			}
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("expected output to contain %q\nfull output:\n%s", tt.wantSub, got)
			}
		})
	}
}

func TestIdentToGo(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"my-func", "myFunc"},
		{"MyType", "MyType"},
		{"fmt/Println", "fmt.Println"},
		{"nil?", "isNil"},
		{"send!", "send"},
		{"*global*", "global"},
		{"_", "_"},
		{"http-request", "httpRequest"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := identToGo(tt.input)
			if got != tt.want {
				t.Errorf("identToGo(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTypeExprToGo(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"int", "int"},
		{"string", "string"},
		{"*http.Request", "*http.Request"},
		{"[]string", "[]string"},
		{"(chan int)", "chan int"},
		{"[string error]", "(string, error)"},
		{"map[string]int", "map[string]int"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := typeExprToGo(tt.input)
			if got != tt.want {
				t.Errorf("typeExprToGo(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
