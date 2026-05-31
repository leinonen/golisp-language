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
			src:     `(defn run [] (go (fmt/println "hi")))`,
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
			src:     `(defstruct User ^string name ^int age)`,
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
		// 7c: string & number utilities
		{
			name:    "format",
			src:     `(defn f [^string name ^int age] (format "Hello, %s! You are %d." name age))`,
			wantSub: `fmt.Sprintf("Hello, %s! You are %d.", name, age)`,
		},
		{
			name:    "parse-int",
			src:     `(defn f [^string s] (parse-int s))`,
			wantSub: `strconv.Atoi(_glispToString(s))`,
		},
		{
			name:    "parse-float",
			src:     `(defn f [^string s] (parse-float s))`,
			wantSub: `strconv.ParseFloat(_glispToString(s), 64)`,
		},
		{
			name:    "repeat",
			src:     `(defn f [^int n] (repeat n 0))`,
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

		// Map operations
		{name: "assoc", src: `(defn f [m] (assoc m "k" 1))`, wantSub: "_glispAssoc("},
		{name: "dissoc", src: `(defn f [m] (dissoc m "k"))`, wantSub: "_glispDissoc("},
		{name: "keys", src: `(defn f [m] (keys m))`, wantSub: "_glispKeys("},
		{name: "vals", src: `(defn f [m] (vals m))`, wantSub: "_glispVals("},
		{name: "merge", src: `(defn f [a b] (merge a b))`, wantSub: "_glispMerge("},

		// Collection operations
		{name: "conj", src: `(defn f [coll x] (conj coll x))`, wantSub: "_glispConj("},
		{name: "count", src: `(defn f [coll] (count coll))`, wantSub: "_glispLen("},
		{name: "first", src: `(defn f [coll] (first coll))`, wantSub: "_glispFirst("},
		{name: "rest", src: `(defn f [coll] (rest coll))`, wantSub: "_glispRest("},
		{name: "nth", src: `(defn f [coll i] (nth coll i))`, wantSub: "_glispNth("},

		// Type / error
		{name: "nil?", src: `(defn f [x] (nil? x))`, wantSub: "== nil"},
		{name: "error", src: `(defn f [^string msg] (error msg))`, wantSub: "errors.New("},
		{name: "string conv", src: `(defn f [^int x] (string x))`, wantSub: "string("},
		{name: "int conv", src: `(defn f [x] (int x))`, wantSub: "_glispToInt("},
		{name: "as type assertion", src: `(defn f [x] (as ^int x))`, wantSub: ".(int)"},

		// I/O
		{name: "fmt/println", src: `(defn f [x] (fmt/println x))`, wantSub: "fmt.Println("},
		{name: "fmt/print", src: `(defn f [x] (fmt/print x))`, wantSub: "fmt.Print("},

		// Iteration
		{name: "doseq", src: `(defn f [coll] (doseq [x coll] (fmt/println x)))`, wantSub: "for _, x := range"},
		{name: "dotimes", src: `(defn f [] (dotimes [i 3] (fmt/println i)))`, wantSub: "for i := 0"},

		// errors/new — pkg-prefixed, goes through fnToGo
		{name: "errors/new", src: `(defn f [^string msg] (errors/new msg))`, wantSub: "errors.New("},

		// fmt — verify naming convention for commonly-used fmt functions
		{name: "fmt/printf", src: `(defn f [^string s] (fmt/printf "%s" s))`, wantSub: "fmt.Printf("},
		{name: "fmt/sprintf", src: `(defn f [^string s] (fmt/sprintf "%s" s))`, wantSub: "fmt.Sprintf("},
		{name: "fmt/errorf", src: `(defn f [^string s] (fmt/errorf "%s" s))`, wantSub: "fmt.Errorf("},

		// strings — verify hyphen→PascalCase conversion
		{name: "strings/contains", src: `(defn f [^string s] (strings/contains s "x"))`, wantSub: "strings.Contains("},
		{name: "strings/has-prefix", src: `(defn f [^string s] (strings/has-prefix s "x"))`, wantSub: "strings.HasPrefix("},
		{name: "strings/has-suffix", src: `(defn f [^string s] (strings/has-suffix s "x"))`, wantSub: "strings.HasSuffix("},
		{name: "strings/trim-space", src: `(defn f [^string s] (strings/trim-space s))`, wantSub: "strings.TrimSpace("},
		{name: "strings/to-upper", src: `(defn f [^string s] (strings/to-upper s))`, wantSub: "strings.ToUpper("},
		{name: "strings/to-lower", src: `(defn f [^string s] (strings/to-lower s))`, wantSub: "strings.ToLower("},
		{name: "strings/split", src: `(defn f [^string s] (strings/split s ","))`, wantSub: "strings.Split("},
		{name: "strings/join", src: `(defn f [coll] (strings/join coll ","))`, wantSub: "strings.Join("},
		{name: "strings/replace", src: `(defn f [^string s] (strings/replace s "a" "b" -1))`, wantSub: "strings.Replace("},
		{name: "strings/replace-all", src: `(defn f [^string s] (strings/replace-all s "a" "b"))`, wantSub: "strings.ReplaceAll("},
		{name: "strings/index", src: `(defn f [^string s] (strings/index s "x"))`, wantSub: "strings.Index("},
		{name: "strings/trim-prefix", src: `(defn f [^string s] (strings/trim-prefix s "x"))`, wantSub: "strings.TrimPrefix("},
		{name: "strings/trim-suffix", src: `(defn f [^string s] (strings/trim-suffix s "x"))`, wantSub: "strings.TrimSuffix("},
		{name: "strings/trim", src: `(defn f [^string s] (strings/trim s " "))`, wantSub: "strings.Trim("},
		{name: "strings/count", src: `(defn f [^string s] (strings/count s "x"))`, wantSub: "strings.Count("},
		{name: "strings/repeat", src: `(defn f [^string s] (strings/repeat s 3))`, wantSub: "strings.Repeat("},

		// strconv
		{name: "strconv/atoi", src: `(defn f [^string s] (strconv/atoi s))`, wantSub: "strconv.Atoi("},
		{name: "strconv/itoa", src: `(defn f [^int n] (strconv/itoa n))`, wantSub: "strconv.Itoa("},
		{name: "strconv/parse-int", src: `(defn f [^string s] (strconv/parse-int s 10 64))`, wantSub: "strconv.ParseInt("},
		{name: "strconv/parse-float", src: `(defn f [^string s] (strconv/parse-float s 64))`, wantSub: "strconv.ParseFloat("},
		{name: "strconv/format-int", src: `(defn f [^int64 n] (strconv/format-int n 10))`, wantSub: "strconv.FormatInt("},
		{name: "strconv/format-float", src: `(defn f [^float64 x] (strconv/format-float x 102 -1))`, wantSub: "strconv.FormatFloat("},

		// math
		{name: "math/sqrt", src: `(defn f [^float64 x] (math/sqrt x))`, wantSub: "math.Sqrt("},
		{name: "math/abs", src: `(defn f [^float64 x] (math/abs x))`, wantSub: "math.Abs("},
		{name: "math/pow", src: `(defn f [^float64 x y] (math/pow x y))`, wantSub: "math.Pow("},
		{name: "math/floor", src: `(defn f [^float64 x] (math/floor x))`, wantSub: "math.Floor("},
		{name: "math/ceil", src: `(defn f [^float64 x] (math/ceil x))`, wantSub: "math.Ceil("},
		{name: "math/round", src: `(defn f [^float64 x] (math/round x))`, wantSub: "math.Round("},
		{name: "math/max", src: `(defn f [^float64 a b] (math/max a b))`, wantSub: "math.Max("},
		{name: "math/min", src: `(defn f [^float64 a b] (math/min a b))`, wantSub: "math.Min("},
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
		{name: "log/printf", src: `(defn f [^string s] (log/printf "%s" s))`, wantSub: "log.Printf("},
		{name: "log/fatal", src: `(defn f [^string s] (log/fatal s))`, wantSub: "log.Fatal("},
		{name: "log/fatalf", src: `(defn f [^string s] (log/fatalf "%s" s))`, wantSub: "log.Fatalf("},

		// os
		{name: "os/exit", src: `(defn f [] (os/exit 0))`, wantSub: "os.Exit("},
		{name: "os/args", src: `(defn f [] os/args)`, wantSub: "os.Args"},

		// http — put is the one missing from golden tests
		{name: "http/put", src: `(defn f [^string url body] (http/put url body))`, wantSub: "_glispHttpPut("},

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
		{"fmt/println", "fmt.Println"},
		{"strings/has-prefix", "strings.HasPrefix"},
		{"web/json-response", "web.JsonResponse"},
		{"web/serve-graceful", "web.ServeGraceful"},
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
