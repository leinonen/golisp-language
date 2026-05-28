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

	// fmt package
	"fmt/Println":  "(fmt/Println & args)              →  [int error]",
	"fmt/Printf":   "(fmt/Printf format & args)         →  [int error]",
	"fmt/Sprintf":  "(fmt/Sprintf format & args)        →  string",
	"fmt/Errorf":   "(fmt/Errorf format & args)         →  error",
	"fmt/Fprintf":  "(fmt/Fprintf w format & args)      →  [int error]",
	"fmt/Fprintln": "(fmt/Fprintln w & args)            →  [int error]",
	"fmt/Sscanf":   "(fmt/Sscanf str format & args)     →  [int error]",

	// errors package
	"errors/New": "(errors/New msg)  →  error",

	// os package
	"os/Exit":   "(os/Exit code)",
	"os/Getenv": "(os/Getenv key)   →  string",
	"os/Args":   "os/Args           →  []string",

	// strconv package
	"strconv/Atoi":        "(strconv/Atoi s)                    →  [int error]",
	"strconv/Itoa":        "(strconv/Itoa i)                    →  string",
	"strconv/ParseInt":    "(strconv/ParseInt s base bitSize)   →  [int64 error]",
	"strconv/ParseFloat":  "(strconv/ParseFloat s bitSize)      →  [float64 error]",
	"strconv/FormatInt":   "(strconv/FormatInt i base)          →  string",
	"strconv/FormatFloat": "(strconv/FormatFloat f fmt prec)    →  string",

	// strings package
	"strings/Contains":   "(strings/Contains s substr)     →  bool",
	"strings/HasPrefix":  "(strings/HasPrefix s prefix)    →  bool",
	"strings/HasSuffix":  "(strings/HasSuffix s suffix)    →  bool",
	"strings/TrimSpace":  "(strings/TrimSpace s)            →  string",
	"strings/ToUpper":    "(strings/ToUpper s)              →  string",
	"strings/ToLower":    "(strings/ToLower s)              →  string",
	"strings/Split":      "(strings/Split s sep)            →  []string",
	"strings/Join":       "(strings/Join elems sep)         →  string",
	"strings/Replace":    "(strings/Replace s old new n)   →  string",
	"strings/ReplaceAll": "(strings/ReplaceAll s old new)  →  string",
	"strings/Index":      "(strings/Index s substr)         →  int",
	"strings/TrimPrefix": "(strings/TrimPrefix s prefix)   →  string",
	"strings/TrimSuffix": "(strings/TrimSuffix s suffix)   →  string",
	"strings/Count":      "(strings/Count s substr)         →  int",
	"strings/Repeat":     "(strings/Repeat s count)         →  string",
	"strings/Trim":       "(strings/Trim s cutset)          →  string",

	// math package
	"math/Abs":   "(math/Abs x)     →  float64",
	"math/Max":   "(math/Max a b)   →  float64",
	"math/Min":   "(math/Min a b)   →  float64",
	"math/Floor": "(math/Floor x)   →  float64",
	"math/Ceil":  "(math/Ceil x)    →  float64",
	"math/Round": "(math/Round x)   →  float64",
	"math/Sqrt":  "(math/Sqrt x)    →  float64",
	"math/Pow":   "(math/Pow x y)   →  float64",

	// log package
	"log/Println": "(log/Println & args)",
	"log/Printf":  "(log/Printf format & args)",
	"log/Fatal":   "(log/Fatal & args)",
	"log/Fatalf":  "(log/Fatalf format & args)",

	// time package
	"time/Now":         "(time/Now)              →  time.Time",
	"time/Sleep":       "(time/Sleep duration)",
	"time/Since":       "(time/Since t)          →  time.Duration",
	"time/Until":       "(time/Until t)          →  time.Duration",
	"time/Second":      "time/Second             →  time.Duration  (1s)",
	"time/Millisecond": "time/Millisecond        →  time.Duration  (1ms)",
	"time/Minute":      "time/Minute             →  time.Duration  (1m)",
	"time/Hour":        "time/Hour               →  time.Duration  (1h)",

	// sort package
	"sort/Slice":   "(sort/Slice coll less-fn)",
	"sort/Ints":    "(sort/Ints s)",
	"sort/Strings": "(sort/Strings s)",

	// io
	"io/EOF": "io/EOF  →  error",

	// stdlib/web.go (golisp/stdlib)
	"stdlib/Routes":       "(stdlib/Routes & routes)                     →  Handler",
	"stdlib/GET":          "(stdlib/GET pattern handler)                 →  Route",
	"stdlib/POST":         "(stdlib/POST pattern handler)                →  Route",
	"stdlib/PUT":          "(stdlib/PUT pattern handler)                 →  Route",
	"stdlib/DELETE":       "(stdlib/DELETE pattern handler)              →  Route",
	"stdlib/PATCH":        "(stdlib/PATCH pattern handler)               →  Route",
	"stdlib/Wrap":         "(stdlib/Wrap handler & middlewares)          →  Handler",
	"stdlib/Compose":      "(stdlib/Compose & middlewares)               →  Middleware",
	"stdlib/WrapJson":     "(stdlib/WrapJson handler)                    →  Handler  — parses JSON body into req[\"json-body\"]",
	"stdlib/WrapCors":     "(stdlib/WrapCors handler)                    →  Handler",
	"stdlib/WrapAuth":     "(stdlib/WrapAuth handler)                    →  Handler  — Bearer token → req[\"identity\"]",
	"stdlib/WrapLogging":  "(stdlib/WrapLogging handler)                 →  Handler",
	"stdlib/WrapRecover":  "(stdlib/WrapRecover handler)                 →  Handler",
	"stdlib/WrapTimeout":  "(stdlib/WrapTimeout seconds)                 →  Middleware",
	"stdlib/JsonResponse": "(stdlib/JsonResponse status body)            →  map[string]any",
	"stdlib/QueryParam":   "(stdlib/QueryParam req name)                 →  string",
	"stdlib/PathParam":    "(stdlib/PathParam req name)                  →  string",
	"stdlib/BodyMap":      "(stdlib/BodyMap req)                         →  map[string]any",
	"stdlib/Header":       "(stdlib/Header req name)                     →  string",
	"stdlib/ServeFiles":   "(stdlib/ServeFiles prefix dir)               →  Handler",
	"stdlib/Serve":        "(stdlib/Serve addr handler)                  →  error",
	"stdlib/ServeGraceful": "(stdlib/ServeGraceful addr handler)         — blocks; drains on SIGINT/SIGTERM",

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
