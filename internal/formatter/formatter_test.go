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
			input: "^int x",
			want:  "^int x\n",
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
			input: "(def ^int x 42)",
			want:  "(def ^int x 42)\n",
		},
		{
			name:  "defn simple",
			input: "(defn foo [] nil)",
			want:  "(defn foo []\n  nil)\n",
		},
		{
			name:  "defn with return type and params",
			input: "(defn ^int add [^int a ^int b] (+ a b))",
			want:  "(defn ^int add [^int a ^int b]\n  (+ a b))\n",
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
			input: "(fn [^string very-long-param-name] (str very-long-param-name \" suffix that makes this too long\"))",
			want:  "(fn [^string very-long-param-name]\n  (str very-long-param-name \" suffix that makes this too long\"))\n",
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
			input: "(defstruct Point ^int x ^int y)",
			want:  "(defstruct Point\n  ^int x\n  ^int y)\n",
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
			input: "(as ^int x)",
			want:  "(as ^int x)\n",
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
			input: "(chan ^int)",
			want:  "(chan ^int)\n",
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
			name:  "comment only file",
			input: "; just a comment",
			want:  "; just a comment\n",
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
		"(defn ^int add [^int a ^int b]\n  (+ a b))\n",
		"(let [a 1\n      b 2]\n  (+ a b))\n",
		"(cond\n  (= x 1) :one\n  (= x 2) :two\n  :else :other)\n",
		"(defstruct Point\n  ^int x\n  ^int y)\n",
		"; section header\n(def x 1)\n\n;; another note\n(def y 2)\n",
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
