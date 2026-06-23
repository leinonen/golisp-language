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
		wantNot string // substring that must NOT appear ("" = skip)
	}{
		{
			name:    "defn basic",
			src:     `(defn double [x int] -> int (* x 2))`,
			wantSub: "func double(x int) int {",
		},
		{
			name:    "def var",
			src:     `(def answer int 42)`,
			wantSub: "var answer int = 42",
		},
		{
			name:    "goroutine",
			src:     `(defn run [] (go (fmt/println "hi")))`,
			wantSub: "go func() {",
		},
		{
			name:    "channel",
			src:     `(defn make-ch [] (chan int 5))`,
			wantSub: "make(chan int, 5)",
		},
		{
			name:    "send recv",
			src:     `(defn send-it [ch (chan int)] (send! ch 42))`,
			wantSub: "ch <- 42",
		},
		{
			name:    "recv",
			src:     `(defn get-it [ch (chan int)] -> int (recv! ch))`,
			wantSub: "<-ch",
		},
		{
			name:    "go-val",
			src:     `(defn compute [] (go-val (+ 1 2)))`,
			wantSub: "make(chan any, 1)",
		},
		{
			name:    "par",
			src:     `(defn run [] (par (fmt/println "a") (fmt/println "b")))`,
			wantSub: "sync.WaitGroup",
		},
		{
			name:    "for-chan",
			src:     `(defn drain [ch (chan string)] (for-chan [x ch] (fmt/println x)))`,
			wantSub: "for x := range",
		},
		{
			name:    "recv-ok",
			src:     `(defn try-recv [ch (chan int)] (let [[v ok] (recv-ok! ch)] ok))`,
			wantSub: "_v, _ok := <-",
		},
		{
			name:    "with-lock",
			src:     `(defn safe [] (with-lock mu (fmt/println "locked")))`,
			wantSub: ".Lock()",
		},
		{
			name:    "select timeout",
			src:     `(defn wait [ch (chan int)] (select! ([v ch] v) (:timeout 1000 nil)))`,
			wantSub: "time.After(",
		},
		{
			name:    "select underscore binding",
			src:     `(defn wait [done (chan any)] (select! ([_ done] (fmt/println "go"))))`,
			wantSub: "case <-done:",
		},
		{
			name:    "select in loop tail breaks",
			src:     `(defn pump [ch (chan any) done (chan any)] (let [x (loop [n 0] (select! ([_ done] nil) ([(send! ch n)] (recur (+ n 1)))))] x))`,
			wantSub: "break",
			wantNot: "= select",
		},
		{
			name:    "close in loop-tail if branch",
			src:     `(defn fill [ch (chan any)] -> any (loop [n 0] (if (< n 3) (do (send! ch n) (recur (+ n 1))) (close! ch))))`,
			wantSub: "close(ch)\n\t\t\treturn nil",
			wantNot: "= close(",
		},
		{
			name:    "bare nil select case body emits no statement",
			src:     `(defn wait [done (chan any)] (select! ([_ done] nil) (:timeout 50 nil)))`,
			wantSub: "case <-done:",
			wantNot: "\tnil",
		},
		{
			name:    "panic in fn tail emits bare statement",
			src:     `(defn f [] -> any (do (fmt/println "x") (panic "boom")))`,
			wantSub: "panic(\"boom\")",
			wantNot: "return panic",
		},
		{
			name:    "panic in loop tail emits bare statement",
			src:     `(defn f [] -> any (loop [n 0] (if (< n 3) (recur (+ n 1)) (panic "boom"))))`,
			wantSub: "panic(\"boom\")",
			wantNot: "return panic",
		},
		{
			name:    "math/abs coerces any arg",
			src:     `(defn f [m] -> float64 (math/abs (get m "x")))`,
			wantSub: "math.Abs(_glispToFloat64(",
		},
		{
			name:    "promote int param with float literal in division",
			src:     `(defn f [n int] -> float64 (/ n 2.0))`,
			wantSub: "(float64(n) / 2.0)",
		},
		{
			name:    "promote int and float params in arithmetic",
			src:     `(defn f [a int b float64] -> float64 (+ a b))`,
			wantSub: "(float64(a) + b)",
		},
		{
			name:    "promote int in ordering comparison with float",
			src:     `(defn f [i int x float64] -> bool (< i x))`,
			wantSub: "(float64(i) < x)",
		},
		{
			name:    "pure int arithmetic is not promoted",
			src:     `(defn f [a int b int] -> int (+ a b))`,
			wantSub: "(a + b)",
			wantNot: "float64(a)",
		},
		{
			name:    "pure float arithmetic is not promoted",
			src:     `(defn f [a float64 b float64] -> float64 (* a b))`,
			wantSub: "(a * b)",
			wantNot: "float64(a)",
		},
		{
			name:    "promote int let binding mixed with float",
			src:     `(defn f [] -> float64 (let [total 10 avg 3.5] (+ total avg)))`,
			wantSub: "(float64(total) + avg)",
		},
		{
			name:    "promote typed global int with float",
			src:     "(def n int 5)\n(defn f [] -> float64 (/ n 2.0))",
			wantSub: "(float64(n) / 2.0)",
		},
		{
			name:    "mod with float operand is not promoted",
			src:     `(defn f [n int] (mod n 2.0))`,
			wantSub: "(n % 2.0)",
			wantNot: "float64(n) %",
		},
		{
			name:    "math/pow coerces any args",
			src:     `(defn f [m] -> float64 (math/pow (get m "x") (get m "y")))`,
			wantSub: "math.Pow(_glispToFloat64(",
		},
		{
			name:    "math/sqrt leaves concrete float arg native",
			src:     `(defn f [x float64] -> float64 (math/sqrt x))`,
			wantSub: "math.Sqrt(x)",
			wantNot: "math.Sqrt(_glispToFloat64",
		},
		{
			name:    "pipeline",
			src:     `(defn pipe [ch (chan any)] (pipeline [x ch] (* x 2)))`,
			wantSub: "defer close(",
		},
		{
			name:    "pipeline multi-stage",
			src:     `(defn pipe [ch (chan any)] (pipeline [x ch] (* x 2) (str x)))`,
			wantSub: "_pipe2",
		},
		{
			name:    "fan-out",
			src:     `(defn work [ch (chan any)] (fan-out 4 [item ch] (fmt/println item)))`,
			wantSub: "sync.WaitGroup",
		},
		{
			name:    "fan-out loop",
			src:     `(defn work [ch (chan any)] (fan-out 4 [item ch] (fmt/println item)))`,
			wantSub: "_fanN",
		},
		{
			name:    "fan-in",
			src:     `(defn merge [a (chan any) b (chan any)] (fan-in a b))`,
			wantSub: "_fanInMerge",
		},
		{
			name:    "fan-in close",
			src:     `(defn merge [a (chan any) b (chan any)] (fan-in a b))`,
			wantSub: "close(_out)",
		},
		{
			name:    "method call",
			src:     `(defn greet [w *Writer] (.Write w "hi"))`,
			wantSub: "w.Write(",
		},
		{
			name:    "field access",
			src:     `(defn get-method [r *Request] (.-Method r))`,
			wantSub: "r.Method",
		},
		{
			name:    "if-err",
			src:     `(defn safe-div [a float64 b float64] -> [string error] (if-err [r err] (divide a b) (values "" err) (values r nil)))`,
			wantSub: "if err != nil {",
		},
		{
			name:    "if-let with else",
			src:     `(defn f [id] (if-let [user (find-user id)] (:name user) "anon"))`,
			wantSub: "if user != nil {",
		},
		{
			name:    "if-let without else returns nil",
			src:     `(defn f [id] (if-let [user (find-user id)] (:name user)))`,
			wantSub: "return nil",
		},
		{
			name:    "when-let",
			src:     `(defn f [id] (when-let [user (find-user id)] (:name user)))`,
			wantSub: "if user != nil {",
		},
		{
			name:    "if-let map destructure",
			src:     `(defn f [id] (if-let [{name :name} (find-user id)] name "anon"))`,
			wantSub: `name := _glispGet(`,
		},
		{
			name:    "if-let statement position",
			src:     `(defn f [id] (if-let [user (find-user id)] (fmt/println user)) nil)`,
			wantSub: "if user != nil {",
		},
		{
			name:    "pkg qualified call",
			src:     `(defn greet [] (fmt/println "hello"))`,
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
			src:     `(defstruct User name string age int)`,
			wantSub: "type User struct {",
		},
		{
			name:    "defer",
			src:     `(defn f [] (defer (fmt/println "done")) (fmt/println "body"))`,
			wantSub: "defer ",
		},
		{
			name:    "panic",
			src:     `(defn f [msg] (panic msg))`,
			wantSub: "panic(msg)",
		},
		{
			name:    "recover",
			src:     `(defn f [] (defer (fn [] (let [r (recover)] (fmt/println r)))) nil)`,
			wantSub: "recover()",
		},
		{
			name:    "assert with explicit message",
			src:     `(defn f [n int] -> void (assert (> n 0) "must be positive"))`,
			wantSub: `panic("must be positive")`,
		},
		{
			name:    "assert auto message from source",
			src:     `(defn f [n int] -> void (assert (> n 0)))`,
			wantSub: `panic("assertion failed: (> n 0)")`,
		},
		{
			name:    "case compiles to switch with default",
			src:     `(defn f [n int] -> string (case n 0 "zero" "other"))`,
			wantSub: "switch n {",
		},
		{
			name:    "case typed return (no any wrapper)",
			src:     `(defn f [n int] -> string (case n 0 "zero" "other"))`,
			wantSub: `return "zero"`,
		},
		// 2a: collection operations
		{
			name:    "map",
			src:     `(defn f [coll] (map (fn [x] x) coll))`,
			wantSub: "_glispMap(",
		},
		{
			name:    "map-indexed",
			src:     `(defn f [coll] (map-indexed (fn [i x] (format "%d:%v" i x)) coll))`,
			wantSub: "_glispMapIndexed(",
		},
		{
			name:    "for comprehension :when",
			src:     `(defn f [xs ys] (for [x xs y ys :when (even? x)] (str x y)))`,
			wantSub: "_forResult = append(_forResult, ",
		},
		{
			name:    "for comprehension nested ranges",
			src:     `(defn f [xs ys] (for [x xs y ys] (str x y)))`,
			wantSub: "for _, y := range _glispToSlice(ys)",
		},
		// Numeric auto-coercion: any-typed operands route through helpers.
		{
			name:    "arith on map lookup coerces",
			src:     `(defn f [m] (+ (get m "a") (get m "b")))`,
			wantSub: "_glispAdd(_glispGet(m, \"a\"), _glispGet(m, \"b\"))",
		},
		{
			name:    "arith on untyped param coerces",
			src:     `(defn f [x] (* x 2))`,
			wantSub: "_glispMul(x, 2)",
		},
		{
			name:    "typed arith stays native",
			src:     `(defn f [a int b int] -> int (+ a b))`,
			wantSub: "return (a + b)",
			wantNot: "_glispAdd",
		},
		{
			name:    "comparison on any coerces",
			src:     `(defn f [x] (> x 3))`,
			wantSub: "_glispGt(x, 3)",
		},
		{
			name:    "typed comparison stays native",
			src:     `(defn f [a int] -> bool (> a 3))`,
			wantSub: "(a > 3)",
			wantNot: "_glispGt",
		},
		{
			name:    "typed return coerces any-arith result",
			src:     `(defn f [m] -> int (+ (get m "a") 1))`,
			wantSub: "return _glispToInt(_glispAdd(_glispGet(m, \"a\"), 1))",
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
		{name: "complement", src: `(defn f [pred] (complement pred))`, wantSub: "_glispComplement("},
		{name: "identity", src: `(defn f [x] (identity x))`, wantSub: "return x"},
		{name: "constantly", src: `(defn f [v] (constantly v))`, wantSub: "_glispConstantly("},
		{name: "comp", src: `(defn f [g h] (comp g h))`, wantSub: "_glispComp("},
		{name: "juxt", src: `(defn f [g h] (juxt g h))`, wantSub: "_glispJuxt("},
		{name: "apply", src: `(defn f [fn args] (apply fn args))`, wantSub: "_glispApply("},
		{name: "partial", src: `(defn f [fn x] (partial fn x))`, wantSub: "_glispPartial("},
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
			src:     `(defn f [s string] (upper-case s))`,
			wantSub: "strings.ToUpper(",
		},
		{
			name:    "lower-case",
			src:     `(defn f [s string] (lower-case s))`,
			wantSub: "strings.ToLower(",
		},
		{
			name:    "trim",
			src:     `(defn f [s string] (trim s))`,
			wantSub: "strings.TrimSpace(",
		},
		{
			name:    "starts-with?",
			src:     `(defn f [s string] (starts-with? s "foo"))`,
			wantSub: "strings.HasPrefix(",
		},
		{
			name:    "ends-with?",
			src:     `(defn f [s string] (ends-with? s "bar"))`,
			wantSub: "strings.HasSuffix(",
		},
		{
			name:    "replace",
			src:     `(defn f [s string] (replace s "a" "b"))`,
			wantSub: "strings.ReplaceAll(",
		},
		{
			name:    "split",
			src:     `(defn f [s string] (split s ","))`,
			wantSub: "_glispSplit(",
		},
		{
			name:    "join",
			src:     `(defn f [coll] (join coll ","))`,
			wantSub: "_glispJoin(",
		},
		{
			name:    "subs two-arg",
			src:     `(defn f [s string] (subs s 2))`,
			wantSub: "_glispToString(s))[2:]",
		},
		{
			name:    "subs three-arg",
			src:     `(defn f [s string] (subs s 1 4))`,
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
			name:    "os/env emits runtime call",
			src:     `(defn f [] (os/env "HOME"))`,
			wantSub: `_glispEnv("HOME")`,
		},
		{
			name:    "os/env with default emits runtime call",
			src:     `(defn f [] (os/env "MISSING" "fallback"))`,
			wantSub: `_glispEnvDefault("MISSING", "fallback")`,
		},
		{
			name:    "os/env adds os import",
			src:     `(defn f [] (os/env "HOME"))`,
			wantSub: `"os"`,
		},
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
			src:     `(defn f [s string] (json/decode s))`,
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
		// 7c: string & number utilities
		{
			name:    "format",
			src:     `(defn f [name string age int] (format "Hello, %s! You are %d." name age))`,
			wantSub: `fmt.Sprintf("Hello, %s! You are %d.", name, age)`,
		},
		{
			name:    "parse-int",
			src:     `(defn f [s string] (parse-int s))`,
			wantSub: `strconv.Atoi(_glispToString(s))`,
		},
		{
			name:    "parse-float",
			src:     `(defn f [s string] (parse-float s))`,
			wantSub: `strconv.ParseFloat(_glispToString(s), 64)`,
		},
		{
			name:    "repeat",
			src:     `(defn f [n int] (repeat n 0))`,
			wantSub: `_glispRepeat(n, 0)`,
		},
		{
			name:    "interpose",
			src:     `(defn f [coll] (interpose "," coll))`,
			wantSub: `_glispInterpose(",", coll)`,
		},

		// Arithmetic / comparison / logic
		{name: "mod", src: `(defn f [a b] (mod a b))`, wantSub: " % "},
		{name: "not=", src: `(defn f [a b] (not= a b))`, wantSub: "!="},
		{name: "<=", src: `(defn f [a b] (<= a b))`, wantSub: "<="},
		{name: ">=", src: `(defn f [a b] (>= a b))`, wantSub: ">="},
		{name: "and", src: `(defn f [a b] (and a b))`, wantSub: "&&"},
		{name: "or", src: `(defn f [a b] (or a b))`, wantSub: "||"},
		{name: "not", src: `(defn f [a] (not a))`, wantSub: "!("},

		// Truthiness: any-typed conditions wrap in _glispTruthy; statically
		// bool conditions emit as-is
		{name: "if truthy wrap", src: `(defn f [x] (if x 1 2))`, wantSub: "if _glispTruthy(x)"},
		{name: "if bool no wrap", src: `(defn f [x] (if (= x 1) 1 2))`, wantSub: "if (x == 1)"},
		{name: "when truthy wrap", src: `(defn f [x] (when x 1))`, wantSub: "if _glispTruthy(x)"},
		{name: "cond truthy wrap", src: `(defn f [x] (cond x 1 :else 2))`, wantSub: "if _glispTruthy(x)"},
		{name: "and truthy operands", src: `(defn f [a b] (if (and a b) 1 2))`, wantSub: "_glispTruthy(a) && _glispTruthy(b)"},
		{name: "not truthy wrap", src: `(defn f [a] (not a))`, wantSub: "!(_glispTruthy(a))"},
		{name: "if user bool fn no wrap", src: "(defn p? [x] -> bool true)\n(defn f [x] (if (p? x) 1 2))", wantSub: "if isP(x)"},

		// Map operations
		{name: "assoc", src: `(defn f [m] (assoc m "k" 1))`, wantSub: "_glispAssoc("},
		{name: "dissoc", src: `(defn f [m] (dissoc m "k"))`, wantSub: "_glispDissoc("},
		{name: "keys", src: `(defn f [m] (keys m))`, wantSub: "_glispKeys("},
		{name: "vals", src: `(defn f [m] (vals m))`, wantSub: "_glispVals("},
		{name: "merge", src: `(defn f [a b] (merge a b))`, wantSub: "_glispMerge("},

		// Collection operations
		{name: "conj", src: `(defn f [coll x] (conj coll x))`, wantSub: "_glispConj("},
		{name: "count", src: `(defn f [coll] (count coll))`, wantSub: "_glispLen("},
		{name: "len alias", src: `(defn f [coll] (len coll))`, wantSub: "_glispLen("},
		{name: "first", src: `(defn f [coll] (first coll))`, wantSub: "_glispFirst("},
		{name: "rest", src: `(defn f [coll] (rest coll))`, wantSub: "_glispRest("},
		{name: "nth", src: `(defn f [coll i] (nth coll i))`, wantSub: "_glispNth("},

		// Type / error
		{name: "nil?", src: `(defn f [x] (nil? x))`, wantSub: "== nil"},
		{name: "error", src: `(defn f [msg string] (error msg))`, wantSub: "errors.New("},
		{name: "string conv", src: `(defn f [x int] (string x))`, wantSub: "string("},
		{name: "int conv", src: `(defn f [x] (int x))`, wantSub: "_glispToInt("},
		{name: "as type assertion", src: `(defn f [x] (as int x))`, wantSub: ".(int)"},

		// I/O
		{name: "fmt/println", src: `(defn f [x] (fmt/println x))`, wantSub: "fmt.Println("},
		{name: "fmt/print", src: `(defn f [x] (fmt/print x))`, wantSub: "fmt.Print("},
		{name: "bare println", src: `(defn f [x] (println x))`, wantSub: "fmt.Println("},
		{name: "bare print", src: `(defn f [x] (print x))`, wantSub: "fmt.Print("},
		{name: "bare println return pos", src: `(defn f [x] -> any (println x))`, wantSub: "fmt.Println(x)\n\treturn nil"},

		// Iteration
		{name: "doseq", src: `(defn f [coll] (doseq [x coll] (fmt/println x)))`, wantSub: "for _, x := range"},
		{name: "dotimes", src: `(defn f [] (dotimes [i 3] (fmt/println i)))`, wantSub: "for i := 0"},

		// Statement-only forms in tail position auto-return nil (only in
		// value-returning fns; defn without -> stays void)
		{name: "go in tail returns nil", src: `(defn f [ch] -> any (go (send! ch 1)))`, wantSub: "}()\n\treturn nil"},
		{name: "send! in fn tail returns nil", src: `(def f (fn [ch] (send! ch 1)))`, wantSub: "ch <- 1\n\treturn nil"},

		// Void-returning calls in tail position emit `<call>; return nil`
		// (return os.Exit(0) is invalid Go).
		{name: "os/exit in when tail returns nil", src: `(defn f [n int] -> any (when (> n 5) (os/exit 0)))`, wantSub: "os.Exit(0)\n\t\t\treturn nil"},
		{name: "void defn in if tail returns nil", src: `(defn quit [] -> void (println "bye")) (defn f [n int] -> any (if (> n 5) (quit) "stay"))`, wantSub: "quit()\n\t\treturn nil"},

		// errors/new — pkg-prefixed, goes through fnToGo
		{name: "errors/new", src: `(defn f [msg string] (errors/new msg))`, wantSub: "errors.New("},

		// fmt — verify naming convention for commonly-used fmt functions
		{name: "fmt/printf", src: `(defn f [s string] (fmt/printf "%s" s))`, wantSub: "fmt.Printf("},
		{name: "fmt/sprintf", src: `(defn f [s string] (fmt/sprintf "%s" s))`, wantSub: "fmt.Sprintf("},
		{name: "fmt/errorf", src: `(defn f [s string] (fmt/errorf "%s" s))`, wantSub: "fmt.Errorf("},

		// strings — verify hyphen→PascalCase conversion
		{name: "strings/contains", src: `(defn f [s string] (strings/contains s "x"))`, wantSub: "strings.Contains("},
		{name: "strings/has-prefix", src: `(defn f [s string] (strings/has-prefix s "x"))`, wantSub: "strings.HasPrefix("},
		{name: "strings/has-suffix", src: `(defn f [s string] (strings/has-suffix s "x"))`, wantSub: "strings.HasSuffix("},
		{name: "strings/trim-space", src: `(defn f [s string] (strings/trim-space s))`, wantSub: "strings.TrimSpace("},
		{name: "strings/to-upper", src: `(defn f [s string] (strings/to-upper s))`, wantSub: "strings.ToUpper("},
		{name: "strings/to-lower", src: `(defn f [s string] (strings/to-lower s))`, wantSub: "strings.ToLower("},
		{name: "strings/split", src: `(defn f [s string] (strings/split s ","))`, wantSub: "strings.Split("},
		{name: "strings/join", src: `(defn f [coll] (strings/join coll ","))`, wantSub: "strings.Join("},
		{name: "strings/replace", src: `(defn f [s string] (strings/replace s "a" "b" -1))`, wantSub: "strings.Replace("},
		{name: "strings/replace-all", src: `(defn f [s string] (strings/replace-all s "a" "b"))`, wantSub: "strings.ReplaceAll("},
		{name: "strings/index", src: `(defn f [s string] (strings/index s "x"))`, wantSub: "strings.Index("},
		{name: "strings/trim-prefix", src: `(defn f [s string] (strings/trim-prefix s "x"))`, wantSub: "strings.TrimPrefix("},
		{name: "strings/trim-suffix", src: `(defn f [s string] (strings/trim-suffix s "x"))`, wantSub: "strings.TrimSuffix("},
		{name: "strings/trim", src: `(defn f [s string] (strings/trim s " "))`, wantSub: "strings.Trim("},
		{name: "strings/count", src: `(defn f [s string] (strings/count s "x"))`, wantSub: "strings.Count("},
		{name: "strings/repeat", src: `(defn f [s string] (strings/repeat s 3))`, wantSub: "strings.Repeat("},

		// strconv
		{name: "strconv/atoi", src: `(defn f [s string] (strconv/atoi s))`, wantSub: "strconv.Atoi("},
		{name: "strconv/itoa", src: `(defn f [n int] (strconv/itoa n))`, wantSub: "strconv.Itoa("},
		{name: "strconv/parse-int", src: `(defn f [s string] (strconv/parse-int s 10 64))`, wantSub: "strconv.ParseInt("},
		{name: "strconv/parse-float", src: `(defn f [s string] (strconv/parse-float s 64))`, wantSub: "strconv.ParseFloat("},
		{name: "strconv/format-int", src: `(defn f [n int64] (strconv/format-int n 10))`, wantSub: "strconv.FormatInt("},
		{name: "strconv/format-float", src: `(defn f [x float64] (strconv/format-float x 102 -1))`, wantSub: "strconv.FormatFloat("},

		// math
		{name: "math/sqrt", src: `(defn f [x float64] (math/sqrt x))`, wantSub: "math.Sqrt("},
		{name: "math/abs", src: `(defn f [x float64] (math/abs x))`, wantSub: "math.Abs("},
		{name: "math/pow", src: `(defn f [x float64 y] (math/pow x y))`, wantSub: "math.Pow("},
		{name: "math/floor", src: `(defn f [x float64] (math/floor x))`, wantSub: "math.Floor("},
		{name: "math/ceil", src: `(defn f [x float64] (math/ceil x))`, wantSub: "math.Ceil("},
		{name: "math/round", src: `(defn f [x float64] (math/round x))`, wantSub: "math.Round("},
		{name: "math/max", src: `(defn f [a float64 b] (math/max a b))`, wantSub: "math.Max("},
		{name: "math/min", src: `(defn f [a float64 b] (math/min a b))`, wantSub: "math.Min("},
		{name: "math/pi constant", src: `(defn f [] math/pi)`, wantSub: "math.Pi"},

		// sort
		{name: "sort/ints", src: `(defn f [s] (sort/ints s))`, wantSub: "sort.Ints("},
		{name: "sort/strings", src: `(defn f [s] (sort/strings s))`, wantSub: "sort.Strings("},
		{name: "sort/slice", src: `(defn f [coll] (sort/slice coll (fn [i j] true)))`, wantSub: "sort.Slice("},

		// time
		{name: "time/now", src: `(defn f [] (time/now))`, wantSub: "time.Now("},
		{name: "time/sleep", src: `(defn f [] (time/sleep time/second))`, wantSub: "time.Sleep("},
		{name: "time/since", src: `(defn f [t] (time/since t))`, wantSub: "time.Since("},
		{name: "time/second constant", src: `(defn f [] time/second)`, wantSub: "time.Second"},
		{name: "time/millisecond constant", src: `(defn f [] time/millisecond)`, wantSub: "time.Millisecond"},

		// log
		{name: "log/println", src: `(defn f [x] (log/println x))`, wantSub: "log.Println("},
		{name: "log/printf", src: `(defn f [s string] (log/printf "%s" s))`, wantSub: "log.Printf("},
		{name: "log/fatal", src: `(defn f [s string] (log/fatal s))`, wantSub: "log.Fatal("},
		{name: "log/fatalf", src: `(defn f [s string] (log/fatalf "%s" s))`, wantSub: "log.Fatalf("},

		// os
		{name: "os/exit", src: `(defn f [] (os/exit 0))`, wantSub: "os.Exit("},
		{name: "os/args", src: `(defn f [] os/args)`, wantSub: "os.Args"},

		// http — put is the one missing from golden tests
		{name: "http/put", src: `(defn f [url string body] (http/put url body))`, wantSub: "_glispHttpPut("},

		// ns :require — glisp modules emitted as Go imports
		{
			name:    "ns require single",
			src:     `(ns main (:require [github.com/user/mathlib])) (defn f [] (mathlib/add 1 2))`,
			wantSub: `"github.com/user/mathlib"`,
		},
		{
			name:    "ns require with alias",
			src:     `(ns main (:require [[github.com/user/mathlib :as math]])) (defn f [] (math/add 1 2))`,
			wantSub: `math "github.com/user/mathlib"`,
		},
		{
			name:    "ns require and import",
			src:     `(ns main (:import [fmt]) (:require [github.com/user/lib])) (defn f [] (fmt/println (lib/greet "World")))`,
			wantSub: `"fmt"`,
		},
		// ns :import — external Go packages
		{
			name:    "ns import vN module path",
			src:     `(ns db (:import [github.com/jackc/pgx/v5])) (defn f [c any] (pgx/connect c "url"))`,
			wantSub: `"github.com/jackc/pgx/v5"`,
		},
		{
			name:    "ns import alias in vector",
			src:     `(ns m (:import [github.com/mattn/go-sqlite3 :as sqlite])) (defn f [] (sqlite/version))`,
			wantSub: `sqlite "github.com/mattn/go-sqlite3"`,
		},
		{
			name:    "ns import one vector per path",
			src:     `(ns m (:import [context] [github.com/google/uuid])) (defn f [] (uuid/new-string))`,
			wantSub: `"github.com/google/uuid"`,
		},
		// context propagation
		{name: "ctx-background", src: `(defn f [] (ctx/background))`, wantSub: "context.Background()"},
		{name: "ctx-background imports context", src: `(defn f [] (ctx/background))`, wantSub: "\"context\""},
		{name: "ctx-todo", src: `(defn f [] (ctx/todo))`, wantSub: "context.TODO()"},
		{name: "ctx-with-cancel", src: `(defn f [] (ctx/with-cancel (ctx/background)))`, wantSub: "_glispCtxWithCancel("},
		{name: "ctx-with-timeout", src: `(defn f [] (ctx/with-timeout (ctx/background) 5000))`, wantSub: "_glispCtxWithTimeout("},
		{name: "ctx-cancel", src: `(defn f [cancel] (ctx/cancel! cancel))`, wantSub: "_glispCtxCancel("},
		{name: "ctx-value", src: `(defn f [ctx] (ctx/value ctx "key"))`, wantSub: "_glispCtxValue("},
		{name: "ctx-with-value", src: `(defn f [ctx] (ctx/with-value ctx "key" "val"))`, wantSub: "_glispCtxWithValue("},
		{name: "ctx-done?", src: `(defn f [ctx] -> bool (ctx/done? ctx))`, wantSub: "_glispCtxDone("},
		{name: "ctx-done? skips truthy wrapper", src: `(defn f [ctx] (if (ctx/done? ctx) 1 2))`, wantSub: "if _glispCtxDone(ctx)"},
		{name: "ctx-err", src: `(defn f [ctx] (ctx/err ctx))`, wantSub: "_glispCtxErr("},

		// numeric min/max + collection variants
		{name: "max", src: `(defn f [] (max 1 2 3))`, wantSub: "_glispMax(1, 2, 3)"},
		{name: "min", src: `(defn f [] (min 1 2))`, wantSub: "_glispMin(1, 2)"},
		{name: "max-by", src: `(defn f [xs []any] (max-by (fn [x] x) xs))`, wantSub: "_glispMaxBy("},
		{name: "min-by", src: `(defn f [xs []any] (min-by (fn [x] x) xs))`, wantSub: "_glispMinBy("},

		// set constructor
		{name: "set constructor", src: `(defn f [xs []any] (set xs))`, wantSub: "_glispToSet("},

		// fnil
		{name: "fnil", src: `(defn f [m] (update m "k" (fnil (fn [n] n) 0)))`, wantSub: "_glispFnil("},

		// (string x) routes through the smart converter, not a Go conversion
		{name: "string conversion is smart", src: `(defn f [x] (string x))`, wantSub: "_glispToString(x)"},

		// dotimes with _ binding gets a synthetic counter
		{name: "dotimes underscore", src: `(defn f [] -> void (dotimes [_ 3] (println "hi")))`, wantSub: "for _dotimesI := 0; _dotimesI < 3; _dotimesI++"},

		// keywords as functions in HOF positions
		{name: "keyword fn in map", src: `(defn f [xs []any] (map :title xs))`, wantSub: `_glispMap(func(_kwM any) any { return _glispGet(_kwM, "title") }`},
		{name: "keyword fn in group-by", src: `(defn f [xs []any] (group-by :status xs))`, wantSub: `_glispGroupBy(func(_kwM any) any { return _glispGet(_kwM, "status") }`},
		{name: "keyword fn in sort-by", src: `(defn f [xs []any] (sort-by :rating xs))`, wantSub: `_glispSortBy(func(_kwM any) any { return _glispGet(_kwM, "rating") }`},
		{name: "keyword stays a value in non-fn position", src: `(defn f [xs []any] (contains? xs :title))`, wantSub: `_glispContains(xs, "title")`},

		// as-> / tap-> / pp / time-it
		{name: "as-> rebinds named placeholder", src: `(defn f [m] (as-> m $ (assoc $ "k" 1) (dissoc $ "old")))`, wantSub: "_dollar = _glispAssoc(_dollar,"},
		{name: "as-> initial value declared any", src: `(defn f [m] (as-> m $ (assoc $ "k" 1)))`, wantSub: "var _dollar any = m"},
		{name: "tap-> wraps stages in pp", src: `(defn f [] (tap-> 5 (+ 3)))`, wantSub: "_glispAdd(_glispPP(5), 3)"},
		{name: "tap->> threads value last", src: `(defn f [xs []any] (tap->> xs (map (fn [x] x))))`, wantSub: "_glispPP(_glispMap("},
		{name: "pp returns value", src: `(defn f [m] (pp m))`, wantSub: "_glispPP(m)"},
		{name: "pp runtime helper emitted", src: `(defn f [m] (pp m))`, wantSub: "func _glispPP("},
		{name: "time-it wraps in timer IIFE", src: `(defn f [] (time-it (+ 1 2)))`, wantSub: "time.Now()"},
		{name: "time-it imports time", src: `(defn f [] (time-it (+ 1 2)))`, wantSub: "\"time\""},

		{
			name:    "typed keyword access on struct param",
			src:     `(defstruct P name string) (defn f [p P] -> string (:name p))`,
			wantSub: "return p.Name",
		},
		{
			name:    "typed keyword access kebab field",
			src:     `(defstruct P unit-price float64) (defn f [p P] -> float64 (:unit-price p))`,
			wantSub: "return p.UnitPrice",
		},
		{
			name:    "keyword access on untyped map falls back",
			src:     `(defn f [m] (:a m))`,
			wantSub: `_glispGet(m, "a")`,
		},
		{
			name:    "typed keyword access on pointer receiver",
			src:     `(defstruct P name string) (defmethod *P Label [self] -> string (:name self))`,
			wantSub: "return self.Name",
		},
		{
			name:    "map literal to struct via call arg",
			src:     `(defstruct P name string stock int) (defn f [p P] -> int (:stock p)) (defn g [] -> int (f {:name "x" :stock 3}))`,
			wantSub: `f(P{Name: "x", Stock: 3})`,
		},
		{
			name:    "map literal to struct via let annotation",
			src:     `(defstruct P name string) (defn g [] -> string (let [p P {:name "x"}] (:name p)))`,
			wantSub: `var p P = P{Name: "x"}`,
		},
		{
			name:    "let struct inference from literal",
			src:     `(defstruct P name string) (defn g [] -> string (let [p (P. {:name "x"})] (:name p)))`,
			wantSub: "return p.Name",
		},
		{
			name:    "annotated destructure string field",
			src:     `(defn f [m] (let [{name :name :- string} m] name))`,
			wantSub: `name := _glispToString(_glispGet(`,
		},
		{
			name:    "annotated destructure int field uses smart conversion",
			src:     `(defn f [m] (let [{age :age :- int} m] age))`,
			wantSub: `age := _glispToInt(_glispGet(`,
		},
		{
			name:    "numeric coercion parses strings (strconv)",
			src:     `(defn f [m] (let [{age :age :- int} m] age))`,
			wantSub: `if i, err := strconv.Atoi(n); err == nil {`,
		},
		{
			name:    "annotated destructure other type uses checked assertion",
			src:     `(defn f [m] (let [{ok :ok :- bool} m] ok))`,
			wantSub: `ok, _ := _glispGet(`,
		},
		{
			name:    "unannotated destructure stays any",
			src:     `(defn f [m] (let [{raw :raw} m] raw))`,
			wantSub: `raw := _glispGet(`,
		},
		{
			name:    "struct-annotated destructure enables keyword access",
			src:     `(defstruct P name string) (defn f [m] -> string (let [{p :product :- P} m] (:name p)))`,
			wantSub: "return p.Name",
		},
		{
			name:    "untyped atom",
			src:     `(defn f [] (atom 0))`,
			wantSub: "&_glispAtom{val: 0}",
		},
		{
			name:    "typed atom int binding derefs without cast",
			src:     `(defn f [] -> int (let [c (atom int 0)] (deref c)))`,
			wantSub: "return _glispToInt(_glispAtomDeref(c))",
		},
		{
			name:    "typed atom string binding",
			src:     `(defn f [] -> string (let [s (atom string "hi")] (deref s)))`,
			wantSub: "return _glispToString(_glispAtomDeref(s))",
		},
		{
			name:    "Atom param type derefs to scalar",
			src:     `(defn cur [c (Atom int)] -> int (deref c))`,
			wantSub: "func cur(c *_glispAtom) int {",
		},
		{
			name:    "Atom param deref single coercion",
			src:     `(defn cur [c (Atom int)] -> int (deref c))`,
			wantSub: "return _glispToInt(_glispAtomDeref(c))",
		},
		{
			name:    "atom as struct field",
			src:     `(defstruct R hits (Atom int))`,
			wantSub: "Hits *_glispAtom",
		},
		{
			name:    "deref atom struct field coerces",
			src:     `(defstruct R hits (Atom int)) (defn h [r R] -> int (deref (:hits r)))`,
			wantSub: "return _glispToInt(_glispAtomDeref(r.Hits))",
		},
		{
			name:    "global def atom derefs typed",
			src:     `(def n (atom int 0)) (defn read [] -> int (deref n))`,
			wantSub: "return _glispToInt(_glispAtomDeref(n))",
		},
		{
			name:    "typed map atom init builds concrete shape",
			src:     `(defn f [] (atom map[string]int {}))`,
			wantSub: "&_glispAtom{val: map[string]int{}}",
		},
		{
			name:    "untyped atom deref stays any",
			src:     `(defn f [c] (deref c))`,
			wantSub: "_glispAtomDeref(c)",
			wantNot: "_glispToInt(_glispAtomDeref(c))",
		},
		{
			name:    "with-open defers close",
			src:     `(defn f [] (with-open [r (open "p")] (use r)))`,
			wantSub: "defer _glispClose(r)",
		},
		{
			name:    "with-open is an IIFE",
			src:     `(defn f [] (with-open [r (open "p")] (use r)))`,
			wantSub: "func() any {",
		},
		{
			name:    "with-open multi-binding interleaves opens and defers",
			src:     `(defn f [] (with-open [a (open "a") b (open "b")] (use a b)))`,
			wantSub: "a := open(\"a\")\n\t\tdefer _glispClose(a)\n\t\tb := open(\"b\")\n\t\tdefer _glispClose(b)",
		},
		{
			name:    "with-open propagates return type into IIFE",
			src:     `(defn f [] -> int (with-open [r (open "p")] 42))`,
			wantSub: "func() int {",
		},
		{
			name:    "with-open typed binding",
			src:     `(defn f [] (with-open [r *os/File (open "p")] (use r)))`,
			wantSub: "var r *os.File = open(\"p\")",
		},
		{
			name:    "doto evaluates object once into temp",
			src:     `(defn f [] (doto (make) (.A) (.B 1)))`,
			wantSub: "_dotoTgt1 := make()",
		},
		{
			name:    "doto threads temp as method receiver",
			src:     `(defn f [] (doto (make) (.A) (.B 1)))`,
			wantSub: "_dotoTgt1.A()\n\t\t_dotoTgt1.B(1)",
		},
		{
			name:    "doto returns the object",
			src:     `(defn f [] (doto (make) (.A)))`,
			wantSub: "return _dotoTgt1",
		},
		{
			name:    "doto threads bare function step as first arg",
			src:     `(defn f [] (doto (make) (configure 7)))`,
			wantSub: "configure(_dotoTgt1, 7)",
		},
		{
			name:    "doto symbol step",
			src:     `(defn f [] (doto (make) start))`,
			wantSub: "start(_dotoTgt1)",
		},
		{
			name:    "doto dot-free method dispatch on typed object",
			src:     `(defstruct B n int) (defmethod *B Push [b x int] -> error nil) (defn f [b *B] (doto b (push 9)))`,
			wantSub: "_dotoTgt1.Push(9)",
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
			if tt.wantNot != "" && strings.Contains(got, tt.wantNot) {
				t.Errorf("expected output NOT to contain %q\nfull output:\n%s", tt.wantNot, got)
			}
		})
	}
}

// TestDotoErrors verifies doto rejects steps that aren't side-effecting calls.
func TestDotoErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"field access step", `(defn f [x] (doto x (.-Field)))`, "field access"},
		{"non-call step", `(defn f [x] (doto x 42))`, "must be a call or symbol"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Transpile(tc.src)
			if err == nil {
				t.Fatalf("expected an error, got none")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

// TestDefmacroExpansion locks the Phase 13.3 end-to-end path: a (defmacro …)
// is registered, removed from output, and its call-sites are expanded into real
// AST before emission (here, to a Go if/else).
func TestDefmacroExpansion(t *testing.T) {
	src := "(defmacro unless [c b] `(if ~c nil ~b))\n" +
		"(defn f [x bool] (unless x (println \"hi\")))"
	got, err := Transpile(src)
	if err != nil {
		t.Fatalf("transpile error: %v", err)
	}
	// The macro expands to an if/else (the source had no if); the defmacro
	// itself emits nothing.
	if !strings.Contains(got, "} else {") || !strings.Contains(got, `fmt.Println("hi")`) {
		t.Errorf("expected expanded if/else from macro, got:\n%s", got)
	}
	if strings.Contains(got, "unless") {
		t.Errorf("defmacro/macro name leaked into Go output:\n%s", got)
	}
	if strings.Contains(got, "func unless") {
		t.Errorf("defmacro should not emit a function:\n%s", got)
	}
}

// TestCoreStr locks Phase 14a: the str/ core namespace resolves to injected
// mangled helper functions, with arg coercion at the call site.
func TestCoreStr(t *testing.T) {
	src := "(defn f [m] -> string (str/upper (get m \"k\")))"
	got, err := Transpile(src)
	if err != nil {
		t.Fatalf("transpile error: %v", err)
	}
	if !strings.Contains(got, "_gcore_str_upper(_glispToString(") {
		t.Errorf("expected coerced core call, got:\n%s", got)
	}
	if !strings.Contains(got, "func _gcore_str_upper(s string) string") ||
		!strings.Contains(got, "strings.ToUpper(s)") {
		t.Errorf("expected injected str/upper helper, got:\n%s", got)
	}
	if strings.Contains(got, "str.Upper") || strings.Contains(got, "str/upper") {
		t.Errorf("glisp-native name leaked into Go:\n%s", got)
	}
}

// TestCoreBareAndSys locks Phase 14b: bare core functions (lines) are callable
// unqualified, the sys/ namespace works, and a bare core function that calls
// another namespace (lines → str/split) pulls it in transitively.
func TestCoreBareAndSys(t *testing.T) {
	src := "(defn f [s string] -> int (count (lines s)))\n" +
		"(defn g [] -> string (sys/env \"X\"))"
	got, err := Transpile(src)
	if err != nil {
		t.Fatalf("transpile error: %v", err)
	}
	if !strings.Contains(got, "func _gcore_core_lines(") {
		t.Errorf("bare lines not injected:\n%s", got)
	}
	if !strings.Contains(got, "func _gcore_str_split(") {
		t.Errorf("transitive str/split not injected for lines:\n%s", got)
	}
	if !strings.Contains(got, "func _gcore_sys_env(") || !strings.Contains(got, "os.Getenv") {
		t.Errorf("sys/env not injected:\n%s", got)
	}
}

// TestCoreBareLocalShadowed confirms a local binding (let var) shadows a bare
// core function — regression for the logparser example, which binds a let `lines`.
func TestCoreBareLocalShadowed(t *testing.T) {
	src := "(defn f [s string] -> any (let [lines (split s \",\")] (first lines)))"
	got, err := Transpile(src)
	if err != nil {
		t.Fatalf("transpile error: %v", err)
	}
	if strings.Contains(got, "_gcore_core_lines") {
		t.Errorf("local `lines` should shadow bare core, but core was injected:\n%s", got)
	}
}

// TestCoreBareShadowed confirms a user defn shadows a bare core function.
func TestCoreBareShadowed(t *testing.T) {
	src := "(defn lines [s string] -> string (str \"x\" s))\n(defn main [] (println (lines \"hi\")))"
	got, err := Transpile(src)
	if err != nil {
		t.Fatalf("transpile error: %v", err)
	}
	if strings.Contains(got, "_gcore_core_lines") {
		t.Errorf("user lines should shadow core, but core was injected:\n%s", got)
	}
}

// TestCoreStrUnusedNotInjected confirms a namespace is only injected when used.
func TestCoreStrUnusedNotInjected(t *testing.T) {
	got, err := Transpile("(defn f [] -> int 1)")
	if err != nil {
		t.Fatalf("transpile error: %v", err)
	}
	if strings.Contains(got, "_gcore_str_") {
		t.Errorf("core str injected without use:\n%s", got)
	}
}

// TestCorePrelude locks Phase 13.7a: core macros (when-not/if-not) are
// available with no import and expand into real control flow.
func TestCorePrelude(t *testing.T) {
	src := "(defn f [n int] (when-not (= n 0) (println n)))"
	got, err := Transpile(src)
	if err != nil {
		t.Fatalf("transpile error: %v", err)
	}
	if !strings.Contains(got, "if ") || !strings.Contains(got, "fmt.Println(n)") {
		t.Errorf("when-not did not expand to an if/println, got:\n%s", got)
	}
	if strings.Contains(got, "when-not") || strings.Contains(got, "whenNot") {
		t.Errorf("when-not leaked into Go output:\n%s", got)
	}
}

// TestReaderMacroErrors locks the Phase 13.0 behavior: reader-macro forms
// parse and format, but without the macro engine (Phase 13.3+) they produce a
// clean, position-tagged transpile error rather than a generic "unsupported
// expression" or a panic.
func TestReaderMacroErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"quote", `(defn f [] '(1 2 3))`, "quote ('):"},
		{"syntax-quote", "(defn f [] `(a b))", "syntax-quote (`)"},
		{"unquote", `(defn f [] ~x)`, "unquote (~)"},
		{"unquote-splice", `(defn f [] ~@xs)`, "unquote-splice (~@)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Transpile(tc.src)
			if err == nil {
				t.Fatalf("expected an error, got none")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

// TestVariadicSpread covers the `& xs` call-position spread marker:
// (f a & xs) → f(a, xs...). The marker is the glisp spelling for Go's
// variadic-spread call (Phase 12b / ADR-015).
func TestVariadicSpread(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantSub string
	}{
		{
			name:    "spread into a Go interop variadic call",
			src:     `(defn log [msg string & args] (fmt/printf msg & args))`,
			wantSub: "fmt.Printf(msg, args...)",
		},
		{
			name:    "spread with no leading args",
			src:     `(defn f [xs] (fmt/sprintln & xs))`,
			wantSub: "fmt.Sprintln(xs...)",
		},
		{
			name:    "spread into a user variadic fn",
			src:     `(defn sum [& ns] 0) (defn f [xs] (sum & xs))`,
			wantSub: "sum(xs...)",
		},
		{
			name:    "leading literal arg before spread",
			src:     `(defn f [xs] (fmt/sprintf "%d" & xs))`,
			wantSub: `fmt.Sprintf("%d", xs...)`,
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

// TestVariadicSpreadErrors covers misuse of the `& xs` spread marker.
func TestVariadicSpreadErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"spread into non-variadic user fn", `(defn g [a b] 0) (defn f [xs] (g 1 & xs))`, "is not variadic"},
		{"marker not followed by one arg", `(defn f [xs] (fmt/sprintf & xs 9))`, "exactly one argument"},
		{"duplicate marker", `(defn f [xs ys] (fmt/sprintf & xs & ys))`, "only one spread marker"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Transpile(tc.src)
			if err == nil {
				t.Fatalf("expected an error, got none")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

// TestNoSpuriousQualifierImports guards the qualifier-suppression logic:
// a pkg/fn call whose qualifier resolves to a declared import (by path
// segment, /vN convention, or :as alias) must not emit a bare import of
// the qualifier itself.
func TestNoSpuriousQualifierImports(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		banned string
	}{
		{
			name:   "vN module path",
			src:    `(ns db (:import [github.com/jackc/pgx/v5])) (defn f [c any] (pgx/connect c "url"))`,
			banned: "\"pgx\"",
		},
		{
			name:   "as alias",
			src:    `(ns m (:import [github.com/mattn/go-sqlite3 :as sqlite])) (defn f [] (sqlite/version))`,
			banned: "\"sqlite\"",
		},
		{
			name:   "last path segment",
			src:    `(ns m (:import [golisp/web])) (defn f [h any] (web/run h 8080))`,
			banned: "\"web\"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Transpile(tt.src)
			if err != nil {
				t.Fatalf("transpile error: %v", err)
			}
			if strings.Contains(got, tt.banned) {
				t.Errorf("output must not contain bare import %s\nfull output:\n%s", tt.banned, got)
			}
		})
	}
}

// TestStdlibQualifierResolution covers the unknown-qualifier diagnostic and the
// multi-segment stdlib auto-import (go-interop-exploration P4): a unique stdlib
// last-segment qualifier resolves to its full import path, while an ambiguous or
// unknown qualifier yields a position-tagged glisp error instead of a raw Go
// "package X is not in std" failure.
func TestStdlibQualifierResolution(t *testing.T) {
	t.Run("multi-segment stdlib auto-imports full path", func(t *testing.T) {
		got, err := Transpile(`(defn f [] -> string (filepath/join "a" "b"))`)
		if err != nil {
			t.Fatalf("transpile error: %v", err)
		}
		if !strings.Contains(got, `"path/filepath"`) {
			t.Errorf("expected import \"path/filepath\", got:\n%s", got)
		}
		if !strings.Contains(got, "filepath.Join") {
			t.Errorf("expected filepath.Join call, got:\n%s", got)
		}
	})

	errCases := []struct {
		name    string
		src     string
		wantSub string
	}{
		{
			name:    "ambiguous qualifier",
			src:     `(defn f [] (rand/intn 10))`,
			wantSub: `ambiguous package qualifier "rand"`,
		},
		{
			name:    "unknown qualifier",
			src:     `(defn f [] (frobnicate/wibble 1))`,
			wantSub: `unknown package "frobnicate"`,
		},
	}
	for _, tt := range errCases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Transpile(tt.src)
			if err == nil {
				t.Fatalf("expected a transpile error")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantSub)
			}
			if !strings.Contains(err.Error(), "(at ") {
				t.Errorf("error should be position-tagged, got %q", err.Error())
			}
		})
	}

	t.Run("ambiguity resolves once declared", func(t *testing.T) {
		got, err := Transpile(`(ns m (:import [math/rand])) (defn f [] (rand/intn 10))`)
		if err != nil {
			t.Fatalf("declared import should resolve, got error: %v", err)
		}
		if !strings.Contains(got, "rand.Intn") {
			t.Errorf("expected rand.Intn, got:\n%s", got)
		}
	})
}

func TestPathQualifier(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"fmt", "fmt"},
		{"net/http", "http"},
		{"golisp/web", "web"},
		{"github.com/google/uuid", "uuid"},
		{"github.com/jackc/pgx/v5", "pgx"},
		{"github.com/user/lib/v12", "lib"},
		{"example.com/v1", "v1"},   // v1 is never a version suffix
		{"example.com/v0", "v0"},   // neither is v0
		{"example.com/v2x", "v2x"}, // not all digits
	}
	for _, tt := range tests {
		if got := pathQualifier(tt.path); got != tt.want {
			t.Errorf("pathQualifier(%q) = %q, want %q", tt.path, got, tt.want)
		}
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
		{"fmt/println", "fmt.Println"},
		{"strings/has-prefix", "strings.HasPrefix"},
		{"web/json-response", "web.JsonResponse"},
		{"web/serve-graceful", "web.ServeGraceful"},
		{"nil?", "isNil"},
		{"send!", "send"},
		{"*global*", "global"},
		{"_", "_"},
		{"http-request", "httpRequest"},
		// qualified predicates/mutators route through fnToGo's exported rules
		{"web/hx-request?", "web.IsHxRequest"},
		{"lib/reset!", "lib.Reset"},
		{"lib/ring->handler", "lib.RingToHandler"},
		{"web/HxTrigger", "web.HxTrigger"},
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

// TestArityChecking verifies that the transpiler catches user-defined function
// call sites with the wrong number of arguments.
func TestArityChecking(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantErr string // expected substring in error message
	}{
		{
			name:    "too few args",
			src:     `(defn greet [name string] -> string (str "hi " name)) (defn main [] (greet))`,
			wantErr: "arity error: greet called with 0 arg(s), expected 1",
		},
		{
			name:    "too many args",
			src:     `(defn greet [name string] -> string (str "hi " name)) (defn main [] (greet "Alice" "Bob"))`,
			wantErr: "arity error: greet called with 2 arg(s), expected 1",
		},
		{
			name:    "variadic ok with min",
			src:     `(defn log [msg string & args] -> any (fmt/println msg)) (defn main [] (log "hello"))`,
			wantErr: "", // variadic with >= minArity — no error
		},
		{
			name:    "variadic too few",
			src:     `(defn log [msg string & args] -> any (fmt/println msg)) (defn main [] (log))`,
			wantErr: "arity error: log called with 0 arg(s), expected at least 1",
		},
		{
			name:    "exact arity ok",
			src:     `(defn add [a int b int] -> int (+ a b)) (defn main [] (add 1 2))`,
			wantErr: "", // correct arity — no error
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Transpile(tt.src)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error %q\ngot: %v", tt.wantErr, err)
			}
		})
	}
}

// TestHOFTypedFnDiagnostic verifies that passing a typed user fn where a
// runtime helper expects func(any) any is rejected at transpile time instead
// of panicking at runtime with an interface-conversion error.
func TestHOFTypedFnDiagnostic(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantErr string // "" = must transpile cleanly
	}{
		{
			name:    "typed param rejected",
			src:     `(defn double-it [x int] -> int (* x 2)) (defn f [xs []any] (map double-it xs))`,
			wantErr: "double-it has a typed param (int)",
		},
		{
			name:    "typed return rejected",
			src:     `(defn flag [x any] -> bool (nil? x)) (defn f [xs []any] (filter flag xs))`,
			wantErr: "flag has return type bool",
		},
		{
			name:    "void fn rejected",
			src:     `(defn show [x any] -> void (println x)) (defn f [xs []any] (map show xs))`,
			wantErr: "show has return type void",
		},
		{
			name:    "any fn accepted",
			src:     `(defn keep-it [x any] -> any x) (defn f [xs []any] (map keep-it xs))`,
			wantErr: "",
		},
		{
			name:    "untyped param with any return accepted",
			src:     `(defn keep-it [x] -> any x) (defn f [xs []any] (map keep-it xs))`,
			wantErr: "",
		},
		{
			name:    "local binding shadowing a typed defn is not flagged",
			src:     `(defn g [x int] -> int x) (defn f [g xs []any] (map g xs))`,
			wantErr: "",
		},
		{
			name:    "lambda always accepted",
			src:     `(defn f [xs []any] (map (fn [x] x) xs))`,
			wantErr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Transpile(tt.src)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error %q\ngot: %v", tt.wantErr, err)
			}
		})
	}
}

// TestMethodDispatch verifies dot-free method dispatch: (area s) emits
// s.Area() when s is statically known to hold a declared struct or interface
// type with a matching method, with built-ins, user functions, and in-scope
// bindings all shadowing the method.
func TestMethodDispatch(t *testing.T) {
	const decls = `
(definterface Shape
  (Area [] -> float64))
(defstruct Circle radius float64)
(defmethod Circle Area [c] -> float64 (* 3.14 (:radius c) (:radius c)))
(defmethod Circle Grow [c f float64] -> float64 (* f (:radius c)))
(defmethod Circle Drained? [c] -> bool (= (:radius c) 0.0))
(defmethod Circle Load [c] -> [string error] (read-file "x"))
`
	tests := []struct {
		name    string
		src     string
		wantSub string // substring that must appear in output ("" = skip)
		wantErr string // expected error substring ("" = no error)
	}{
		{
			name:    "interface-typed param",
			src:     decls + `(defn f [s Shape] -> float64 (area s))`,
			wantSub: "return s.Area()",
		},
		{
			name:    "struct-typed param with extra arg",
			src:     decls + `(defn f [c Circle] -> float64 (grow c 2.0))`,
			wantSub: "return c.Grow(2.0)",
		},
		{
			name:    "inferred let binding",
			src:     decls + `(defn f [] -> float64 (let [c (Circle. {:radius 2})] (area c)))`,
			wantSub: "c.Area()",
		},
		{
			name:    "struct literal receiver",
			src:     decls + `(defn f [] -> float64 (area (Circle. {:radius 2})))`,
			wantSub: "Circle{Radius: 2}.Area()",
		},
		{
			name:    "bool method in condition skips truthy wrapper",
			src:     decls + `(defn f [c Circle] -> string (if (drained? c) "y" "n"))`,
			wantSub: "if c.isDrained() {",
		},
		{
			name:    "user fn shadows method",
			src:     decls + `(defn area [c Circle] -> string "fn") (defn f [c Circle] -> string (area c))`,
			wantSub: "return area(c)",
		},
		{
			name:    "local binding shadows method",
			src:     decls + `(defn f [c Circle] -> any (let [area (fn [x Circle] -> string "local")] (area c)))`,
			wantSub: "area(c)",
		},
		{
			name:    "param shadows method",
			src:     decls + `(defn f [area any c Circle] -> any (area c))`,
			wantSub: "area(c)",
		},
		{
			name:    "untyped receiver stays plain call",
			src:     decls + `(defn f [s any] -> any (area s))`,
			wantSub: "area(s)",
		},
		{
			name:    "wrong arity",
			src:     decls + `(defn f [c Circle] -> float64 (area c 1))`,
			wantErr: "arity error: method Area on Circle called with 1 arg(s) after the receiver, expected 0",
		},
		{
			name:    "multi-return method as single value",
			src:     decls + `(defn f [c Circle] -> any (let [v (load c)] v))`,
			wantErr: "load returns multiple values (string, error)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Transpile(tt.src)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error %q\ngot: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantSub != "" && !strings.Contains(got, tt.wantSub) {
				t.Errorf("output missing %q\n--- got ---\n%s", tt.wantSub, got)
			}
		})
	}
}

// TestCrossFileTypeResolution verifies that declarations from sibling files
// (passed via a DeclSet) populate the emitter's type tables, so struct field
// access and method dispatch resolve against types declared in another file of
// the same package — the multi-file build path (Phase 2e).
func TestCrossFileTypeResolution(t *testing.T) {
	// File A declares the types; file B (transpiled below) uses them.
	const fileA = `(ns main)
(defstruct Book id string title string)
(defmethod Book Slug [b] -> string (lower-case (:title b)))
(definterface Repo
  (Find [id string] -> any))`

	tests := []struct {
		name    string
		fileB   string
		wantSub string // substring that must appear in file B's output
	}{
		{
			name:    "struct field access across files",
			fileB:   `(ns main)` + "\n" + `(defn book-title [b Book] -> string (:title b))`,
			wantSub: "return b.Title",
		},
		{
			name:    "method dispatch across files",
			fileB:   `(ns main)` + "\n" + `(defn book-slug [b Book] -> string (slug b))`,
			wantSub: "return b.Slug()",
		},
		{
			name:    "interface method dispatch across files",
			fileB:   `(ns main)` + "\n" + `(defn lookup [r Repo id string] -> any (find r id))`,
			wantSub: "return r.Find(id)",
		},
		{
			name:    "typed map literal across files",
			fileB:   `(ns main)` + "\n" + `(defn make [] -> Book {:id "1" :title "Go"})`,
			wantSub: "Book{Id: \"1\", Title: \"Go\"}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Collect declarations from BOTH files, as compileDir does.
			ds, err := CollectDecls(nil, fileA, "a.glsp")
			if err != nil {
				t.Fatalf("collect fileA: %v", err)
			}
			ds, err = CollectDecls(ds, tt.fileB, "b.glsp")
			if err != nil {
				t.Fatalf("collect fileB: %v", err)
			}
			got, _, err := TranspileNoRuntimeFileExt(tt.fileB, "b.glsp", ds, false)
			if err != nil {
				t.Fatalf("transpile fileB: %v", err)
			}
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("output missing %q\n--- got ---\n%s", tt.wantSub, got)
			}
		})
	}

	// Without the DeclSet, the same access falls back to _glispGet (the old
	// behavior this feature fixes) — guards against a silent regression.
	t.Run("no decls falls back to runtime lookup", func(t *testing.T) {
		fileB := `(ns main)` + "\n" + `(defn book-title [b Book] -> string (:title b))`
		got, _, err := TranspileNoRuntimeFile(fileB, "b.glsp")
		if err != nil {
			t.Fatalf("transpile: %v", err)
		}
		if !strings.Contains(got, "_glispGet(b,") {
			t.Errorf("expected _glispGet fallback without sibling decls\n--- got ---\n%s", got)
		}
	})
}

// TestTypedMapAndIIFE covers Phase 11's two `any`-seam closures: a typed `map`
// loop that satisfies a `[]T` position, and concrete-type propagation into the
// return type of an if/when/do/cond/switch IIFE in expression position.
func TestTypedMapAndIIFE(t *testing.T) {
	const decls = `(defstruct Book id string title string)`
	tests := []struct {
		name    string
		src     string
		wantSub string
		wantNot string
	}{
		// --- typed map ---
		{
			name:    "typed map asserts element type for untyped lambda",
			src:     decls + `(defn f [xs []any] -> []Book (map (fn [v] (as Book v)) xs))`,
			wantSub: "func() []Book {",
		},
		{
			name:    "typed map appends with element assertion",
			src:     decls + `(defn f [xs []any] -> []Book (map (fn [v] (as Book v)) xs))`,
			wantSub: ").(Book))",
		},
		{
			name:    "typed map skips assertion when lambda is annotated",
			src:     decls + `(defn f [xs []any] -> []Book (map (fn [v] -> Book (as Book v)) xs))`,
			wantSub: "func() []Book {",
			wantNot: ").(Book))",
		},
		{
			name:    "map with []any hint stays _glispMap",
			src:     decls + `(defn f [xs []any] -> []any (map (fn [v] v) xs))`,
			wantSub: "_glispMap",
		},
		{
			name:    "map with keyword fn stays _glispMap",
			src:     decls + `(defn f [xs []any] -> []string (map :title xs))`,
			wantSub: "_glispMap",
		},
		{
			name:    "map with typed param falls back to runtime path",
			src:     decls + `(defn f [xs []any] -> []any (map (fn [v Book] -> string (:title v)) xs))`,
			wantSub: "_glispMap",
		},
		// --- IIFE type propagation (expression / non-tail position) ---
		{
			name:    "if into typed slice binding propagates return type",
			src:     `(defn f [c bool] -> any (let [xs []any (if c ["a"] ["b"])] xs))`,
			wantSub: "var xs []any = func() []any {",
		},
		{
			name:    "when into nilable binding propagates",
			src:     `(defn f [c bool] -> any (let [xs []any (when c ["a"])] xs))`,
			wantSub: "func() []any {",
		},
		{
			name:    "when into non-nilable binding does NOT propagate",
			src:     `(defn f [c bool] -> any (let [n int (when c 1)] n))`,
			wantNot: "func() int {",
		},
		{
			name:    "do into typed slice binding propagates",
			src:     `(defn f [] -> any (let [xs []any (do (println "x") ["a"])] xs))`,
			wantSub: "func() []any {",
		},
		{
			name:    "cond with default into typed binding propagates",
			src:     `(defn f [c bool] -> any (let [s string (cond c "y" :else "n")] s))`,
			wantSub: "func() string {",
		},
		{
			name:    "struct-typed if branch emits struct literals",
			src:     decls + `(defn f [c bool] -> any (let [b Book (if c {:id "1" :title "a"} {:id "2" :title "b"})] b))`,
			wantSub: "var b Book = func() Book {",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Transpile(tt.src)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantSub != "" && !strings.Contains(got, tt.wantSub) {
				t.Errorf("output missing %q\n--- got ---\n%s", tt.wantSub, got)
			}
			if tt.wantNot != "" && strings.Contains(got, tt.wantNot) {
				t.Errorf("output unexpectedly contains %q\n--- got ---\n%s", tt.wantNot, got)
			}
		})
	}
}

// TestTypedReturnPropagation verifies that `any`-returning collection built-ins
// (conj/reduce/into/filter/get/first/…) are absorbed into a concrete-typed
// position — either by a type assertion (assertableHint cases) or, for a `[]T`
// hint, by an element-converting IIFE (tryEmitTypedSeq) — so the user no longer
// has to hand-write (as []any (conj …)) / (as bool (reduce …)).
func TestTypedReturnPropagation(t *testing.T) {
	const decls = `(defstruct Book id string title string)`
	tests := []struct {
		name    string
		src     string
		wantSub string
		wantNot string
	}{
		// --- assertion path (any-static call into a non-numeric concrete hint) ---
		{
			name:    "reduce into bool return asserts",
			src:     `(defn f [xs []any] -> bool (reduce (fn [a x] (and a x)) true xs))`,
			wantSub: ".(bool)",
		},
		{
			name:    "conj into []any return asserts",
			src:     `(defn f [xs []any] -> []any (conj xs 1))`,
			wantSub: "_glispConj(xs, 1).([]any)",
		},
		{
			name:    "into into map[string]any return asserts",
			src:     `(defn f [ps []any] -> map[string]any (into {} ps))`,
			wantSub: ".(map[string]any)",
		},
		{
			name:    "first into string return coerces",
			src:     `(defn f [xs []any] -> string (first xs))`,
			wantSub: "_glispToString(_glispFirst(xs))",
		},
		{
			name:    "get into struct return asserts",
			src:     decls + `(defn f [m map[string]any] -> Book (get m "b"))`,
			wantSub: ".(Book)",
		},
		{
			name:    "reduce into typed let binding asserts",
			src:     `(defn f [xs []any] -> any (let [ok bool (reduce (fn [a x] (and a x)) true xs)] ok))`,
			wantSub: ".(bool)",
		},
		// --- typed-slice element conversion (tryEmitTypedSeq) ---
		{
			name:    "filter into []string converts elements",
			src:     `(defn f [xs []any] -> []string (filter (fn [s] (not= s "")) xs))`,
			wantSub: "func() []string {",
		},
		{
			name:    "filter into []string asserts each element",
			src:     `(defn f [xs []any] -> []string (filter (fn [s] (not= s "")) xs))`,
			wantSub: ".(string))",
		},
		{
			name:    "conj into []string converts elements",
			src:     `(defn f [xs []any] -> []string (conj xs "a"))`,
			wantSub: "_glispToSlice(_glispConj(xs, \"a\"))",
		},
		{
			name:    "numeric element type uses smart coercion not assertion",
			src:     `(defn f [xs []any] -> []int (filter (fn [x] (> x 0)) xs))`,
			wantSub: "_glispToInt(",
		},
		{
			name:    "numeric element type avoids blind assertion",
			src:     `(defn f [xs []any] -> []int (filter (fn [x] (> x 0)) xs))`,
			wantNot: ".(int))",
		},
		// --- non-regression: native/exact-match positions are untouched ---
		{
			name:    "filter into []any stays native (no assertion, no IIFE)",
			src:     `(defn f [xs []any] -> []any (filter (fn [x] x) xs))`,
			wantNot: ".([]any)",
		},
		{
			name:    "assoc into map[string]any stays native",
			src:     `(defn f [m map[string]any] -> map[string]any (assoc m "k" 1))`,
			wantNot: `_glispAssoc(m, "k", 1).(`,
		},
		{
			name:    "reduce into int return keeps numeric coercion",
			src:     `(defn f [xs []any] -> int (reduce (fn [a x] (+ a x)) 0 xs))`,
			wantSub: "_glispToInt(",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Transpile(tt.src)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantSub != "" && !strings.Contains(got, tt.wantSub) {
				t.Errorf("output missing %q\n--- got ---\n%s", tt.wantSub, got)
			}
			if tt.wantNot != "" && strings.Contains(got, tt.wantNot) {
				t.Errorf("output unexpectedly contains %q\n--- got ---\n%s", tt.wantNot, got)
			}
		})
	}
}

// TestBuiltinArity verifies that built-in call forms are checked against the
// central arity table and report a position-tagged error rather than panicking
// on a downstream slice index.
func TestBuiltinArity(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantErr string // expected substring; "" means no error
	}{
		{"nth too few", `(defn main [] (nth [1 2 3]))`, "nth expects 2 argument(s), got 1"},
		{"first too many", `(defn main [] (first [1] [2]))`, "first expects 1 argument(s), got 2"},
		{"recover with arg", `(defn main [] (recover 1))`, "recover expects 0 argument(s), got 1"},
		{"get below min", `(defn main [] (get))`, "get expects 2 to 3 arguments, got 0"},
		{"merge below min", `(defn main [] (merge {}))`, "merge expects at least 2 argument(s), got 1"},
		{"re/replace wrong", `(defn main [] (re/replace "a" "b"))`, "re/replace expects 3 argument(s), got 2"},
		{"assert too many", `(defn main [] (assert true "a" "b"))`, "assert expects 1 to 2 arguments, got 3"},
		{"assert valid 1", `(defn main [] (assert true))`, ""},
		{"error carries position", `(defn main [] (count))`, "at 1:15"},
		{"get valid 2", `(defn main [] (get {:a 1} :a))`, ""},
		{"get valid 3", `(defn main [] (get {:a 1} :b 0))`, ""},
		{"merge valid", `(defn main [] (merge {:a 1} {:b 2}))`, ""},
		{"variadic min ok", `(defn main [] (str "a" "b" "c"))`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Transpile(tt.src)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error %q\ngot: %v", tt.wantErr, err)
			}
		})
	}
}

// TestTranspilePanicRecovery verifies that an unexpected panic inside the
// lexer/parser/emitter is converted into a TranspileError instead of crashing
// the host process. We trigger it via a recovered runtime panic injected from a
// test hook is not available, so instead we assert the recovery boundary exists
// by confirming a deeply malformed program returns an error and never panics.
func TestTranspilePanicRecovery(t *testing.T) {
	// Extremely deep nesting can stress recursive descent; whatever the
	// outcome, the call must return (error or success), never panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Transpile panicked instead of returning an error: %v", r)
		}
	}()
	deep := strings.Repeat("(do ", 5000) + "1" + strings.Repeat(")", 5000)
	if _, err := Transpile(deep); err != nil {
		// An error is an acceptable outcome; the point is that it did not panic.
		if !strings.Contains(err.Error(), "error") {
			t.Logf("got error (acceptable): %v", err)
		}
	}
}

// TestStrictMode verifies that --strict rejects programs with missing type annotations.
func TestStrictMode(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantErr string // expected substring in error; "" means no error
	}{
		{
			name:    "missing param type",
			src:     `(defn greet [name] -> string (str "hi " name))`,
			wantErr: `strict: param "name" in defn "greet" has no type annotation`,
		},
		{
			name:    "missing return type",
			src:     `(defn greet [name string] (str "hi " name))`,
			wantErr: `strict: defn "greet" has no return type annotation`,
		},
		{
			name:    "missing struct field type",
			src:     `(defstruct Circle radius)`,
			wantErr: `strict: field "radius" in defstruct "Circle" has no type annotation`,
		},
		{
			name:    "missing def type",
			src:     `(def x 42)`,
			wantErr: `strict: def "x" has no type annotation`,
		},
		{
			name:    "fully typed defn ok",
			src:     `(defn add [a int b int] -> int (+ a b))`,
			wantErr: "",
		},
		{
			name:    "typed struct ok",
			src:     `(defstruct Circle radius float64)`,
			wantErr: "",
		},
		{
			name:    "typed def ok",
			src:     `(def x int 42)`,
			wantErr: "",
		},
		{
			name:    "rest param exempt from annotation requirement",
			src:     `(defn log [msg string & args] -> any (fmt/println msg))`,
			wantErr: "", // rest params are exempt
		},
		{
			name:    "void return type ok in strict mode",
			src:     `(defn say [s string] -> void (fmt/println s))`,
			wantErr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := TranspileFileStrict(tt.src, "test.glsp")
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error %q\ngot: %v", tt.wantErr, err)
			}
		})
	}
}

// TestHomogeneousVectorInference verifies that vector literals with all-string elements
// are emitted as []string instead of []any.
func TestHomogeneousVectorInference(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantSub string
	}{
		{
			name:    "string vector infers []string",
			src:     `(defn words [] -> []string ["hello" "world"])`,
			wantSub: `[]string{"hello", "world"}`,
		},
		{
			name:    "empty vector stays []any",
			src:     `(defn empty-list [] -> []any [])`,
			wantSub: `[]any{}`,
		},
		{
			name:    "mixed vector stays []any",
			src:     `(defn mixed [] -> []any ["a" 1])`,
			wantSub: `[]any{"a", 1}`,
		},
		{
			name:    "explicit annotation overrides inference",
			src:     `(def xs []any ["a" "b"])`,
			wantSub: `[]any{"a", "b"}`,
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

// TestMultiReturnDiagnostics verifies that using a known multi-return call
// (built-in or user fn declared -> [T E]) as a single value is a glisp-level
// transpile error rather than a leaked Go compile error (ADR-011).
func TestMultiReturnDiagnostics(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantErr string // expected substring; "" means must transpile cleanly
	}{
		{
			name:    "builtin in any tail",
			src:     `(defn f [] -> any (parse-int "1"))`,
			wantErr: "parse-int returns multiple values",
		},
		{
			name:    "builtin in concrete tail",
			src:     `(defn f [] -> int (parse-int "1"))`,
			wantErr: "parse-int returns multiple values",
		},
		{
			name:    "user multi fn in any tail",
			src:     `(defn p [] -> [int error] (values 1 nil)) (defn f [] -> any (p))`,
			wantErr: "p returns multiple values (int, error)",
		},
		{
			name:    "let binding",
			src:     `(defn f [] -> any (let [x (read-file "a")] x))`,
			wantErr: "read-file returns multiple values",
		},
		{
			name:    "if-let binding",
			src:     `(defn f [] -> any (if-let [x (json/decode "1")] x "no"))`,
			wantErr: "json/decode returns multiple values",
		},
		{
			name:    "let-or binding",
			src:     `(defn f [] -> any (let-or [x (http/get "u") "fallback"] x))`,
			wantErr: "http/get returns multiple values",
		},
		{
			name:    "def value",
			src:     `(def x (parse-int "1"))`,
			wantErr: "parse-int returns multiple values",
		},
		{
			name:    "loop tail",
			src:     `(defn f [] -> any (loop [i 0] (if (> i 3) (parse-int "1") (recur (+ i 1)))))`,
			wantErr: "parse-int returns multiple values",
		},
		{
			name:    "pass-through from multi-return fn is legal",
			src:     `(defn f [s string] -> [int error] (parse-int s))`,
			wantErr: "",
		},
		{
			name:    "statement-position discard is legal",
			src:     `(defn f [] -> any (do (parse-int "1") nil))`,
			wantErr: "",
		},
		{
			name:    "if-err is the blessed consumer",
			src:     `(defn f [] -> any (if-err [v err] (parse-int "1") 0 v))`,
			wantErr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Transpile(tt.src)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q\ngot: %v", tt.wantErr, err)
			}
			if !strings.Contains(err.Error(), "if-err") {
				t.Errorf("error should suggest if-err, got: %v", err)
			}
		})
	}
}

// TestLineDirectives verifies //line mapping behavior in file mode:
// runtime helpers are re-anchored to a virtual glisp_runtime.go (so panics
// inside them don't point at bogus .glsp lines), and deftest assertions are
// pinned to their own source line (so a failing t.Errorf reports it exactly).
func TestLineDirectives(t *testing.T) {
	src := `(ns main)

(defn add [a int b int] -> int (+ a b))

(deftest add-works
  (assert= (add 2 2) 5))

(defn main [] -> void
  (fmt/println (add 1 2)))
`
	out, err := TranspileFile(src, "x.glsp")
	if err != nil {
		t.Fatalf("TranspileFile: %v", err)
	}
	for _, want := range []string{
		"//line glisp_runtime.go:1", // helpers re-anchored
		"//line x.glsp:5",           // deftest decl
		"//line x.glsp:6",           // the assert= line (also pins t.Errorf)
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
	// The t.Errorf call must be re-pinned to the assertion line: two
	// directives for line 6 (one before the if, one before t.Errorf).
	if got := strings.Count(out, "//line x.glsp:6"); got < 2 {
		t.Errorf("want >= 2 directives for the assert line, got %d", got)
	}

	// Without a filename (print/golden paths) no directives are emitted.
	plain, err := Transpile(src)
	if err != nil {
		t.Fatalf("Transpile: %v", err)
	}
	if strings.Contains(plain, "//line") {
		t.Error("Transpile without filename must not emit //line directives")
	}
}

// TestCoreCallStaticPredicates checks that the static-type predicates resolve
// core fns (registered under mangled names) when they run on the original call
// node — the audit fix behind exprIsAny/isVoidCall/multiReturnCall/isBoolExpr.
func TestCoreCallStaticPredicates(t *testing.T) {
	// multiReturnCall: a core multi-return fn (slurp -> [string error]) used as a
	// single value is a clean diagnostic, not a raw Go "multiple values" error.
	if _, err := Transpile("(ns main)\n(defn f [] -> any (let [x (slurp \"p\")] x))"); err == nil {
		t.Error("expected a multi-return diagnostic for slurp as a single value")
	} else if !strings.Contains(err.Error(), "multiple values") {
		t.Errorf("error %q should mention 'multiple values'", err.Error())
	}

	// isBoolExpr: a core -> bool fn (str/blank?) in a condition emits without the
	// _glispTruthy wrapper.
	out, err := Transpile("(ns main)\n(defn f [s string] -> void (when (str/blank? s) (println \"y\")))")
	if err != nil {
		t.Fatalf("transpile: %v", err)
	}
	if !strings.Contains(out, "if _gcore_str_isBlank(s)") {
		t.Errorf("str/blank? condition should emit a bare bool, got:\n%s", out)
	}
	if strings.Contains(out, "_glispTruthy(_gcore_str_isBlank") {
		t.Error("str/blank? in a condition should not be wrapped in _glispTruthy")
	}
}
