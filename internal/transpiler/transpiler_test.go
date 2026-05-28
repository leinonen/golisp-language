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
