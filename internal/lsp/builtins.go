package lsp

// builtinDocs maps every glisp built-in name to its hover signature.
// User-defined names take precedence over these entries.
var builtinDocs = map[string]string{
	// Arithmetic
	"+":   "(+ a b ...)  →  number",
	"-":   "(- a b ...)  →  number",
	"*":   "(* a b ...)  →  number",
	"/":   "(/ a b ...)  →  number",
	"mod": "(mod a b)    →  number",

	// Comparison
	"=":   "(= a b)   →  bool",
	"not=": "(not= a b)  →  bool",
	"<":   "(< a b)   →  bool",
	">":   "(> a b)   →  bool",
	"<=":  "(<= a b)  →  bool",
	">=":  "(>= a b)  →  bool",

	// Logic
	"and": "(and a b ...)  →  bool",
	"or":  "(or a b ...)   →  bool",
	"not": "(not a)        →  bool",

	// Collections
	"get":      "(get m k)             →  any",
	"assoc":    "(assoc m k v)         →  map",
	"dissoc":   "(dissoc m k)          →  map",
	"conj":     "(conj coll x)         →  coll",
	"count":    "(count coll)          →  int",
	"first":    "(first coll)          →  any",
	"rest":     "(rest coll)           →  []any",
	"nth":      "(nth coll i)          →  any",
	"keys":     "(keys m)              →  []any",
	"vals":     "(vals m)              →  []any",
	"merge":    "(merge m1 m2)         →  map",
	"map":      "(map f coll)          →  []any",
	"filter":   "(filter pred coll)    →  []any",
	"reduce":   "(reduce f init coll)  →  any",
	"reverse":  "(reverse coll)        →  []any",
	"contains?": "(contains? coll x)   →  bool",
	"some":     "(some pred coll)      →  any",
	"every?":   "(every? pred coll)    →  bool",
	"sort-by":  "(sort-by f coll)      →  []any",
	"flatten":  "(flatten coll)        →  []any",
	"range":    "(range n) or (range start end)  →  []int",
	"take":     "(take n coll)         →  []any",
	"drop":     "(drop n coll)         →  []any",

	// Strings
	"str":          "(str & args)              →  string",
	"string":       "(string x)                →  string",
	"upper-case":   "(upper-case s)            →  string",
	"lower-case":   "(lower-case s)            →  string",
	"trim":         "(trim s)                  →  string",
	"starts-with?": "(starts-with? s prefix)   →  bool",
	"ends-with?":   "(ends-with? s suffix)     →  bool",
	"replace":      "(replace s old new)       →  string",
	"split":        "(split s sep)             →  []string",
	"join":         "(join sep coll)           →  string",
	"subs":         "(subs s start end?)       →  string",

	// I/O
	"println": "(println & args)",
	"print":   "(print & args)",

	// Type / error
	"int":   "(int x)       →  int",
	"error": "(error msg)   →  error",
	"nil?":  "(nil? x)      →  bool",
	"as":    "(as ^T x)     →  T  (type assertion)",

	// Iteration
	"doseq":   "(doseq [x coll] body...)",
	"dotimes": "(dotimes [i n] body...)",

	// JSON
	"json/encode": "(json/encode x)  →  [string error]",
	"json/decode": "(json/decode s)  →  [any error]",

	// Special forms
	"def":    "(def ^T name value)",
	"defn":   "(defn ^ReturnType name [params...] body...)",
	"fn":     "(fn [params...] body...)",
	"let":    "(let [name val ...] body...)",
	"if":     "(if cond then else?)",
	"when":   "(when cond body...)",
	"cond":   "(cond test1 val1 ... :else default)",
	"do":     "(do body...)",
	"loop":   "(loop [name init ...] body...)",
	"recur":  "(recur args...)",
	"return": "(return val?)",
	"values": "(values a b)  →  multi-return",
	"->":     "(-> x f1 f2 ...)   thread-first",
	"->>":    "(->> x f1 f2 ...)  thread-last",

	// Go interop
	"go":      "(go body...)        goroutine",
	"defer":   "(defer expr)",
	"chan":     "(chan T cap?)       →  chan T",
	"send!":   "(send! ch val)",
	"recv!":   "(recv! ch)          →  T",
	"close!":  "(close! ch)",
	"select!": "(select! cases...)",
	"if-err":  "(if-err [val err] expr on-err on-ok)",

	// Declarations
	"ns":           "(ns name (:import [...]))",
	"defstruct":    "(defstruct Name ^T1 field1 ...)",
	"definterface": "(definterface Name (Method [params] ^Ret) ...)",
	"deftest":      "(deftest name body...)",

	// Assertions
	"assert=":     "(assert= expected actual)",
	"assert-true":  "(assert-true expr)",
	"assert-false": "(assert-false expr)",
	"assert-nil":   "(assert-nil expr)",
	"assert-err":   "(assert-err expr)",
}
