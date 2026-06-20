package formatter_test

import (
	"testing"

	"golisp/internal/formatter"
)

func TestFormat(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "nil literal",
			input: "nil",
			want:  "nil\n",
		},
		{
			name:  "bool literals",
			input: "true false",
			want:  "true\n\nfalse\n",
		},
		{
			name:  "int literal",
			input: "42",
			want:  "42\n",
		},
		{
			name:  "float literal keeps fraction",
			input: "8.5",
			want:  "8.5\n",
		},
		{
			name:  "whole-number float keeps .0 suffix",
			input: "8.0",
			want:  "8.0\n",
		},
		{
			name:  "string literal",
			input: `"hello"`,
			want:  "\"hello\"\n",
		},
		{
			name:  "keyword",
			input: ":foo",
			want:  ":foo\n",
		},
		{
			name:  "symbol",
			input: "foo",
			want:  "foo\n",
		},
		{
			name:  "symbol with type annot",
			input: "x",
			want:  "x\n",
		},
		{
			name:  "empty vector",
			input: "[]",
			want:  "[]\n",
		},
		{
			name:  "short vector",
			input: "[1 2 3]",
			want:  "[1 2 3]\n",
		},
		{
			name:  "empty map",
			input: "{}",
			want:  "{}\n",
		},
		{
			name:  "single-pair map inline",
			input: `{"key" "val"}`,
			want:  "{\"key\" \"val\"}\n",
		},
		{
			name:  "multi-pair map aligned",
			input: `{"a" 1 "bb" 2 "ccc" 3}`,
			want:  "{\"a\"   1\n \"bb\"  2\n \"ccc\" 3}\n",
		},
		{
			name:  "simple call",
			input: "(+ 1 2)",
			want:  "(+ 1 2)\n",
		},
		{
			name:  "ns without imports",
			input: "(ns main)",
			want:  "(ns main)\n",
		},
		{
			name:  "ns with imports",
			input: "(ns main (:import [fmt golisp/web]))",
			want:  "(ns main (:import [fmt golisp/web]))\n",
		},
		{
			name:  "ns with require",
			input: "(ns main (:require [github.com/user/mathlib]))",
			want:  "(ns main (:require [github.com/user/mathlib]))\n",
		},
		{
			name:  "ns with import and require",
			input: "(ns main (:import [fmt]) (:require [github.com/user/mathlib]))",
			want:  "(ns main (:import [fmt]) (:require [github.com/user/mathlib]))\n",
		},
		{
			name:  "def simple",
			input: "(def x 42)",
			want:  "(def x 42)\n",
		},
		{
			name:  "def with type",
			input: "(def x int 42)",
			want:  "(def x int 42)\n",
		},
		{
			name:  "defn simple",
			input: "(defn foo [] nil)",
			want:  "(defn foo []\n  nil)\n",
		},
		{
			name:  "defn with return type and params",
			input: "(defn add [a int b int] -> int (+ a b))",
			want:  "(defn add [a int b int] -> int\n  (+ a b))\n",
		},
		{
			name:  "defn multi-body",
			input: "(defn greet [name] (str \"hello \" name) (str \"bye \" name))",
			want:  "(defn greet [name]\n  (str \"hello \" name)\n  (str \"bye \" name))\n",
		},
		{
			name:  "fn inline",
			input: "(fn [x] x)",
			want:  "(fn [x] x)\n",
		},
		{
			name:  "fn multiline",
			input: "(fn [very-long-param-name string] (str very-long-param-name \" suffix that makes this too long\"))",
			want:  "(fn [very-long-param-name string]\n  (str very-long-param-name \" suffix that makes this too long\"))\n",
		},
		{
			name:  "let inline",
			input: "(let [a 1 b 2] (+ a b))",
			want:  "(let [a 1 b 2] (+ a b))\n",
		},
		{
			name:  "let multiline",
			input: "(let [some-long-name (some-function arg1 arg2) another-binding (other-function)] (process some-long-name another-binding))",
			want:  "(let [some-long-name (some-function arg1 arg2)\n      another-binding (other-function)]\n  (process some-long-name another-binding))\n",
		},
		{
			name:  "wide map destructure breaks multi-line and aligns",
			input: "(let [{title :title :- string priority :priority :- string assignee :assignee :- string done :done :- bool} m] (use title))",
			want:  "(let [{title    :title    :- string\n       priority :priority :- string\n       assignee :assignee :- string\n       done     :done     :- bool} m]\n  (use title))\n",
		},
		{
			name:  "narrow map destructure stays inline",
			input: "(let [{name :name :- string} m] name)",
			want:  "(let [{name :name :- string} m] name)\n",
		},
		{
			name:  "if inline",
			input: "(if x 1 2)",
			want:  "(if x 1 2)\n",
		},
		{
			name:  "if multiline",
			input: "(if (some-very-long-condition-expression) (do-some-long-thing-with-value) (fallback-value-expression))",
			want:  "(if (some-very-long-condition-expression)\n  (do-some-long-thing-with-value)\n  (fallback-value-expression))\n",
		},
		{
			name:  "if no else",
			input: "(if x (foo))",
			want:  "(if x (foo))\n",
		},
		{
			name:  "cond",
			input: "(cond (= x 1) :one (= x 2) :two :else :other)",
			want:  "(cond\n  (= x 1) :one\n  (= x 2) :two\n  :else :other)\n",
		},
		{
			name:  "do inline",
			input: "(do a b)",
			want:  "(do a b)\n",
		},
		{
			name:  "do multiline",
			input: "(do (some-very-long-side-effecting-call with args) (another-very-long-side-effecting-call with more-args))",
			want:  "(do\n  (some-very-long-side-effecting-call with args)\n  (another-very-long-side-effecting-call with more-args))\n",
		},
		{
			name:  "defstruct",
			input: "(defstruct Point x int y int)",
			want:  "(defstruct Point\n  x int\n  y int)\n",
		},
		{
			name:  "deftest",
			input: `(deftest my-test (assert= 1 1) (assert-true true))`,
			want:  "(deftest my-test\n  (assert= 1 1)\n  (assert-true true))\n",
		},
		{
			name:  "recur",
			input: "(recur (+ n 1) acc)",
			want:  "(recur (+ n 1) acc)\n",
		},
		{
			name:  "type assert",
			input: "(as int x)",
			want:  "(as int x)\n",
		},
		{
			name:  "method call",
			input: "(.Method obj arg)",
			want:  "(.Method obj arg)\n",
		},
		{
			name:  "field access",
			input: "(.-Field obj)",
			want:  "(.-Field obj)\n",
		},
		{
			name:  "chan",
			input: "(chan int)",
			want:  "(chan int)\n",
		},
		{
			name:  "send recv close",
			input: "(send! ch 42)\n(recv! ch)\n(close! ch)",
			want:  "(send! ch 42)\n\n(recv! ch)\n\n(close! ch)\n",
		},
		{
			name:  "defer",
			input: "(defer (close! ch))",
			want:  "(defer (close! ch))\n",
		},
		{
			name:  "loop inline",
			input: "(loop [i 0] (recur (+ i 1)))",
			want:  "(loop [i 0] (recur (+ i 1)))\n",
		},
		{
			name:  "loop multiline",
			input: "(loop [accumulator 0 index-value 0] (recur (+ accumulator index-value) (+ index-value 1)))",
			want:  "(loop [accumulator 0\n       index-value 0]\n  (recur (+ accumulator index-value) (+ index-value 1)))\n",
		},
		{
			name:  "when",
			input: "(when x (foo))",
			want:  "(when x (foo))\n",
		},
		{
			name:  "if-let inline",
			input: "(if-let [u (find x)] u \"none\")",
			want:  "(if-let [u (find x)] u \"none\")\n",
		},
		{
			name:  "if-let no else inline",
			input: "(if-let [u (find x)] (name u))",
			want:  "(if-let [u (find x)] (name u))\n",
		},
		{
			name:  "if-let map destructure inline",
			input: "(if-let [{n :name} (find x)] n \"anon\")",
			want:  "(if-let [{n :name} (find x)] n \"anon\")\n",
		},
		{
			name:  "when-let inline",
			input: "(when-let [u (find x)] (notify u))",
			want:  "(when-let [u (find x)] (notify u))\n",
		},
		{
			name:  "if-let multiline",
			input: "(if-let [user (find-user identifier)] (greet-the-user-warmly user) (return-anonymous-default))",
			want:  "(if-let [user (find-user identifier)]\n  (greet-the-user-warmly user)\n  (return-anonymous-default))\n",
		},
		{
			name:  "when-let multiline",
			input: "(when-let [user (find-user identifier)] (log-the-access user) (notify-the-subscribers user))",
			want:  "(when-let [user (find-user identifier)]\n  (log-the-access user)\n  (notify-the-subscribers user))\n",
		},
		{
			name:  "idempotent ns",
			input: "(ns main (:import [fmt golisp/web]))\n",
			want:  "(ns main (:import [fmt golisp/web]))\n",
		},
		{
			name:  "multiple top-level separated by blank line",
			input: "(def a 1)(def b 2)",
			want:  "(def a 1)\n\n(def b 2)\n",
		},
		{
			name:  "select! recv/timeout/default round-trip",
			input: "(defn f [ch any out any] (select! ([v ch] (send! out v) (log v)) (:timeout 50 (close! out)) (:default (noop))))",
			want:  "(defn f [ch any out any]\n  (select!\n    ([v ch]\n      (send! out v)\n      (log v))\n    (:timeout 50\n      (close! out))\n    (:default\n      (noop))))\n",
		},
		{
			name:  "par with-lock recv-ok multi-line",
			input: "(defn f [mu any ch any] (par (work-a) (work-b)) (with-lock mu (mutate) (commit)) (recv-ok! ch))",
			want:  "(defn f [mu any ch any]\n  (par (work-a) (work-b))\n  (with-lock mu (mutate) (commit))\n  (recv-ok! ch))\n",
		},
		{
			name:  "for-chan inline when it fits",
			input: "(defn f [out any] (for-chan [r out] (when (not= r nil) (print r)) (track r)))",
			want:  "(defn f [out any]\n  (for-chan [r out] (when (not= r nil) (print r)) (track r)))\n",
		},
		{
			name:  "go-val with element type",
			input: "(defn submit [job string] -> (chan string) (go-val string (process job)))",
			want:  "(defn submit [job string] -> (chan string)\n  (go-val string (process job)))\n",
		},
		{
			name:  "let-or flat bindings",
			input: "(defn f [m any] (let-or [a (get m \"a\") :none b (get m \"b\") :none] (use a b)))",
			want:  "(defn f [m any]\n  (let-or [a (get m \"a\") :none b (get m \"b\") :none] (use a b)))\n",
		},
		{
			name:  "call args align under first arg (Style A)",
			input: "(combine-results first-result second-result third-result fourth-result fifth-result)",
			want:  "(combine-results first-result\n                 second-result\n                 third-result\n                 fourth-result\n                 fifth-result)\n",
		},
		{
			name:  "long head falls back to 2-space hang past align threshold",
			input: "(this-is-a-very-long-function-name-indeed arg-one arg-two arg-three arg-four-here-x)",
			want:  "(this-is-a-very-long-function-name-indeed arg-one\n  arg-two\n  arg-three\n  arg-four-here-x)\n",
		},
		{
			name:  "assoc pairs key/value per line",
			input: "(assoc config :host \"localhost\" :port 8080 :timeout 30 :retries 3 :verbose enabled)",
			want:  "(assoc config\n       :host \"localhost\"\n       :port 8080\n       :timeout 30\n       :retries 3\n       :verbose enabled)\n",
		},
		{
			name:  "case formats like switch with trailing default",
			input: "(case status-code 200 :ok 404 :not-found 500 :server-error :unknown-status-default)",
			want:  "(case status-code\n  200 :ok\n  404 :not-found\n  500 :server-error\n  :unknown-status-default)\n",
		},
		{
			name:  "short case still hangs 2-space (like switch)",
			input: "(case n 0 \"zero\" \"many\")",
			want:  "(case n\n  0 \"zero\"\n  \"many\")\n",
		},
		{
			name:  "assert with message inline",
			input: "(assert (> n 0) \"must be positive\")",
			want:  "(assert (> n 0) \"must be positive\")\n",
		},
		{
			name:  "cond-> pairs test/expr per line",
			input: "(cond-> base-value pred-one (assoc :a 1) pred-two (assoc :b 2) pred-three (final-x))",
			want:  "(cond-> base-value\n        pred-one (assoc :a 1)\n        pred-two (assoc :b 2)\n        pred-three (final-x))\n",
		},
		{
			name:  "untyped atom",
			input: "(atom 0)",
			want:  "(atom 0)\n",
		},
		{
			name:  "typed atom scalar",
			input: "(atom int 0)",
			want:  "(atom int 0)\n",
		},
		{
			name:  "typed atom map round-trips",
			input: "(atom map[string]int {})",
			want:  "(atom map[string]int {})\n",
		},
		{
			name:  "Atom type in param round-trips",
			input: "(defn cur [c (Atom int)] -> int (deref c))",
			want:  "(defn cur [c (Atom int)] -> int\n  (deref c))\n",
		},
		{
			name:  "bare Atom struct field round-trips",
			input: "(defstruct R store Atom hits (Atom int))",
			want:  "(defstruct R\n  store Atom\n  hits (Atom int))\n",
		},
		{
			name:  "with-open inline",
			input: "(with-open [f (open p)] (read f))",
			want:  "(with-open [f (open p)] (read f))\n",
		},
		{
			name:  "with-open typed binding round-trips",
			input: "(with-open [f *os/File (open p)] (read f))",
			want:  "(with-open [f *os/File (open p)] (read f))\n",
		},
		{
			name:  "with-open multi-line body",
			input: "(with-open [reader (open-input-file path) writer (open-output-file dest)] (copy-all reader writer))",
			want:  "(with-open [reader (open-input-file path)\n            writer (open-output-file dest)]\n  (copy-all reader writer))\n",
		},
		{
			name:  "trailing comment on let binding preserved in place",
			input: "(let [x 1 ; the x\n y 2]\n  (+ x y))",
			want:  "(let [x 1 ; the x\n      y 2]\n  (+ x y))\n",
		},
		{
			name:  "trailing comment on last binding goes after bracket",
			input: "(let [x 1\n y 2] ; the y\n  (+ x y))",
			want:  "(let [x 1\n      y 2] ; the y\n  (+ x y))\n",
		},
		{
			name:  "trailing comment on single binding",
			input: "(let [x 1] ; only\n  x)",
			want:  "(let [x 1] ; only\n  x)\n",
		},
		{
			name:  "trailing comment on loop binding",
			input: "(loop [i 0 ; index\n acc 0]\n  acc)",
			want:  "(loop [i 0 ; index\n       acc 0]\n  acc)\n",
		},
		{
			name:  "trailing comment on with-open binding",
			input: "(with-open [f (open p) ; the file\n g (open q)]\n  (read f))",
			want:  "(with-open [f (open p) ; the file\n            g (open q)]\n  (read f))\n",
		},
		{
			name:  "trailing and own-line binding comments mix",
			input: "(let [x 1 ; trailing x\n ; own-line before y\n y 2]\n  (+ x y))",
			want:  "(let [x 1 ; trailing x\n      ; own-line before y\n      y 2]\n  (+ x y))\n",
		},
		{
			name:  "doto inline when it fits",
			input: "(doto sb (.WriteString \"a\") (.WriteString \"b\"))",
			want:  "(doto sb (.WriteString \"a\") (.WriteString \"b\"))\n",
		},
		{
			name:  "doto zero-arg method step round-trips",
			input: "(doto x (.Close))",
			want:  "(doto x (.Close))\n",
		},
		{
			name:  "doto multi-line when long",
			input: "(doto (make-builder-with-a-really-long-name) (.WriteString \"alpha\") (.WriteString \"beta\") (.Flush))",
			want:  "(doto (make-builder-with-a-really-long-name)\n  (.WriteString \"alpha\")\n  (.WriteString \"beta\")\n  (.Flush))\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := formatter.Format(tt.input)
			if err != nil {
				t.Fatalf("Format(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("Format(%q)\ngot:  %q\nwant: %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCommentPreservation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single comment before def",
			input: "; set x\n(def x 42)",
			want:  "; set x\n(def x 42)\n",
		},
		{
			name:  "double-semi comment before def",
			input: ";; important note\n(def x 42)",
			want:  ";; important note\n(def x 42)\n",
		},
		{
			name:  "comment between two defs",
			input: "(def a 1)\n;; b follows\n(def b 2)",
			want:  "(def a 1)\n\n;; b follows\n(def b 2)\n",
		},
		{
			name:  "multi-line comment block before form",
			input: "; line1\n; line2\n(def x 1)",
			want:  "; line1\n; line2\n(def x 1)\n",
		},
		{
			name:  "trailing comment after last form",
			input: "(def x 1)\n; trailing",
			want:  "(def x 1)\n\n; trailing\n",
		},
		{
			name:  "doc comment not duplicated",
			input: ";;; Docs\n(defn foo [] nil)",
			want:  ";;; Docs\n(defn foo []\n  nil)\n",
		},
		{
			name:  "multi-line doc comment preserved",
			input: ";;; First line.\n;;; Second line.\n(defn foo [] nil)",
			want:  ";;; First line.\n;;; Second line.\n(defn foo []\n  nil)\n",
		},
		{
			name:  "multi-line doc comment on defmethod",
			input: ";;; First.\n;;; Second.\n(defmethod Circle Name [c] -> string \"circle\")",
			want:  ";;; First.\n;;; Second.\n(defmethod Circle Name [c] -> string\n  \"circle\")\n",
		},
		{
			name:  "comment only file",
			input: "; just a comment",
			want:  "; just a comment\n",
		},
		{
			name:  "in-body comments between defn body forms stay in place",
			input: "(defn f [] -> void\n  ; step one\n  (foo)\n  ; step two\n  (bar))",
			want:  "(defn f [] -> void\n  ; step one\n  (foo)\n  ; step two\n  (bar))\n",
		},
		{
			name:  "in-body comment in last top-level form not dumped at EOF",
			input: "(defn main [] -> void\n  (a)\n  ; note\n  (b))",
			want:  "(defn main [] -> void\n  (a)\n  ; note\n  (b))\n",
		},
		{
			name:  "comment forces an otherwise-inline let multi-line",
			input: "(defn g [] -> int\n  (let [x 1\n        ; explain y\n        y 2]\n    ; compute\n    (+ x y)))",
			want:  "(defn g [] -> int\n  (let [x 1\n        ; explain y\n        y 2]\n    ; compute\n    (+ x y)))\n",
		},
		{
			name:  "comment inside nested form is not relocated to next sibling",
			input: "(defn f [] -> void\n  (when ok\n    ; inner\n    (go))\n  (after))",
			want:  "(defn f [] -> void\n  (when ok\n    ; inner\n    (go))\n  (after))\n",
		},
		{
			name:  "channel type keeps parens in as (round-trips)",
			input: "(defn f [w any] (as (chan string) w))",
			want:  "(defn f [w any]\n  (as (chan string) w))\n",
		},
		{
			name:  "channel return type keeps parens",
			input: "(defn mk [] -> (chan string) (chan string 1))",
			want:  "(defn mk [] -> (chan string)\n  (chan string 1))\n",
		},
		{
			name:  "orphan ;;; docstring before ns is preserved",
			input: ";;; File docstring.\n(ns main)",
			want:  ";;; File docstring.\n(ns main)\n",
		},
		{
			name:  "attached ;;; not duplicated when also orphan-eligible",
			input: ";;; header\n(def x 1)\n;;; doc\n(defn f [] nil)",
			want:  ";;; header\n(def x 1)\n\n;;; doc\n(defn f []\n  nil)\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := formatter.Format(tt.input)
			if err != nil {
				t.Fatalf("Format(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("Format(%q)\ngot:  %q\nwant: %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIdempotent(t *testing.T) {
	inputs := []string{
		"(defn add [a int b int] -> int\n  (+ a b))\n",
		"(let [a 1\n      b 2]\n  (+ a b))\n",
		"(cond\n  (= x 1) :one\n  (= x 2) :two\n  :else :other)\n",
		"(defstruct Point\n  x int\n  y int)\n",
		"; section header\n(def x 1)\n\n;; another note\n(def y 2)\n",
		"(combine-results first-result\n                 second-result\n                 third-result\n                 fourth-result\n                 fifth-result)\n",
		"(assoc config\n       :host \"localhost\"\n       :port 8080\n       :verbose enabled)\n",
		"(as-> m $ (assoc $ :k 1) (dissoc $ :old))\n",
		"(tap-> 5 (+ 3) (* 2))\n",
		"(pp {:a 1})\n",
		"(time-it (compute))\n",
	}
	for _, src := range inputs {
		once, err := formatter.Format(src)
		if err != nil {
			t.Fatalf("Format error: %v", err)
		}
		twice, err := formatter.Format(once)
		if err != nil {
			t.Fatalf("Format error on second pass: %v", err)
		}
		if once != twice {
			t.Errorf("Not idempotent for:\n%s\nFirst: %q\nSecond: %q", src, once, twice)
		}
	}
}

// TestFormatHiccup covers width-aware vector/map nesting: oversized vector
// children recurse instead of staying on one long line, and a keyword tag
// keeps a fitting attrs map on the bracket line (hiccup layout).
func TestFormatHiccup(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "fitting hiccup vector stays inline",
			input: `(def x [:li.todo {:id "1"} [:span "hi"]])`,
			want:  `(def x [:li.todo {:id "1"} [:span "hi"]])` + "\n",
		},
		{
			name:  "tag keeps fitting attrs on head line, children break",
			input: `(def x [:li.todo {:id "1"} [:input {:type "checkbox" :checked true :hx-post "/todos/1/toggle" :hx-target "closest li" :hx-swap "outerHTML"}] [:span "title text"]])`,
			want: `(def x
  [:li.todo {:id "1"}
   [:input
    {:type      "checkbox"
     :checked   true
     :hx-post   "/todos/1/toggle"
     :hx-target "closest li"
     :hx-swap   "outerHTML"}]
   [:span "title text"]])` + "\n",
		},
		{
			name:  "oversized attrs map leaves the head line",
			input: `(def x [:form {:hx-post "/todos" :hx-target "#todo-list" :hx-swap "outerHTML" "hx-on::after-request" "this.reset()"} [:button "add"]])`,
			want: `(def x
  [:form
   {:hx-post               "/todos"
    :hx-target             "#todo-list"
    :hx-swap               "outerHTML"
    "hx-on::after-request" "this.reset()"}
   [:button "add"]])` + "\n",
		},
		{
			name:  "long map value breaks recursively",
			input: `(def x {:supplier {:name "A very long supplier name indeed" :tier 1 :region "north-by-northwest"} :id 1})`,
			want: `(def x
  {:supplier {:name   "A very long supplier name indeed"
              :tier   1
              :region "north-by-northwest"}
   :id       1})` + "\n",
		},
		{
			name:  "non-keyword vector children still break by width",
			input: `(def xs [{:id 1 :name "Widget A" :category "hardware" :price 999 :stock 50 :supplier "x"} {:id 2 :name "B"}])`,
			want: `(def xs
  [{:id       1
    :name     "Widget A"
    :category "hardware"
    :price    999
    :stock    50
    :supplier "x"}
   {:id 2 :name "B"}])` + "\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := formatter.Format(tt.input)
			if err != nil {
				t.Fatalf("format error: %v", err)
			}
			if got != tt.want {
				t.Errorf("mismatch\n--- want ---\n%s\n--- got ---\n%s", tt.want, got)
			}
			again, err := formatter.Format(got)
			if err != nil {
				t.Fatalf("re-format error: %v", err)
			}
			if again != got {
				t.Errorf("not idempotent\n--- first ---\n%s\n--- second ---\n%s", got, again)
			}
		})
	}
}
