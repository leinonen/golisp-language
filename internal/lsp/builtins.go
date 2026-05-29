package lsp

// BuiltinDoc holds structured documentation for a built-in symbol.
type BuiltinDoc struct {
	Sig string // glisp-syntax signature
	Doc string // one-line description in glisp terms; "" if none
}

// builtinDocs maps every glisp built-in name to its signature and description.
// User-defined names take precedence over these entries.
// All Doc strings use glisp terminology — no Go syntax or go doc examples.
var builtinDocs = map[string]BuiltinDoc{
	// Arithmetic
	"+":   {Sig: "(+ a b ...)  →  number", Doc: "Add one or more numbers."},
	"-":   {Sig: "(- a b ...)  →  number", Doc: "Subtract numbers. With one arg, negates."},
	"*":   {Sig: "(* a b ...)  →  number", Doc: "Multiply one or more numbers."},
	"/":   {Sig: "(/ a b ...)  →  number", Doc: "Divide numbers left to right."},
	"mod": {Sig: "(mod a b)    →  number", Doc: "Remainder of a divided by b."},

	// Comparison
	"=":    {Sig: "(= a b)   →  bool", Doc: "True when a equals b."},
	"not=": {Sig: "(not= a b)  →  bool", Doc: "True when a does not equal b."},
	"<":    {Sig: "(< a b)   →  bool", Doc: "True when a is less than b."},
	">":    {Sig: "(> a b)   →  bool", Doc: "True when a is greater than b."},
	"<=":   {Sig: "(<= a b)  →  bool", Doc: "True when a is less than or equal to b."},
	">=":   {Sig: "(>= a b)  →  bool", Doc: "True when a is greater than or equal to b."},

	// Logic
	"and": {Sig: "(and a b ...)  →  bool", Doc: "True when all arguments are truthy."},
	"or":  {Sig: "(or a b ...)   →  bool", Doc: "True when at least one argument is truthy."},
	"not": {Sig: "(not a)        →  bool", Doc: "Logical negation."},

	// Collections
	"get":       {Sig: "(get m k)             →  any", Doc: "Look up key k in map or slice m. Returns nil if absent."},
	"assoc":     {Sig: "(assoc m k v)         →  map", Doc: "Return a new map with key k set to v."},
	"dissoc":    {Sig: "(dissoc m k)          →  map", Doc: "Return a new map with key k removed."},
	"conj":      {Sig: "(conj coll x)         →  coll", Doc: "Append x to a slice, or add x to a set."},
	"count":     {Sig: "(count coll)          →  int", Doc: "Number of elements in coll."},
	"first":     {Sig: "(first coll)          →  any", Doc: "First element of coll."},
	"rest":      {Sig: "(rest coll)           →  []any", Doc: "All elements of coll after the first."},
	"nth":       {Sig: "(nth coll i)          →  any", Doc: "Element at index i of coll."},
	"keys":      {Sig: "(keys m)              →  []any", Doc: "All keys of map m."},
	"vals":      {Sig: "(vals m)              →  []any", Doc: "All values of map m."},
	"merge":     {Sig: "(merge m1 m2)         →  map", Doc: "Merge two maps; keys in m2 overwrite m1."},
	"map":       {Sig: "(map f coll)          →  []any", Doc: "Apply f to each element of coll and return the results as a slice."},
	"filter":    {Sig: "(filter pred coll)    →  []any", Doc: "Return elements of coll for which pred returns true."},
	"reduce":    {Sig: "(reduce f init coll)  →  any", Doc: "Reduce coll to a single value by applying f with accumulator starting at init."},
	"reverse":   {Sig: "(reverse coll)        →  []any", Doc: "Return coll in reverse order."},
	"contains?": {Sig: "(contains? coll x)   →  bool", Doc: "True when coll contains x (works for maps, slices, and sets)."},
	"some":      {Sig: "(some pred coll)      →  any", Doc: "Return the first element of coll for which pred is truthy, or nil."},
	"every?":    {Sig: "(every? pred coll)    →  bool", Doc: "True when pred returns true for every element of coll."},
	"sort-by":   {Sig: "(sort-by f coll)      →  []any", Doc: "Sort coll by the value returned by f for each element."},
	"flatten":   {Sig: "(flatten coll)        →  []any", Doc: "Recursively flatten nested slices into a single slice."},
	"range":     {Sig: "(range n) or (range start end)  →  []int", Doc: "Integer slice from 0 to n (exclusive), or from start to end."},
	"take":      {Sig: "(take n coll)         →  []any", Doc: "Return the first n elements of coll."},
	"drop":      {Sig: "(drop n coll)         →  []any", Doc: "Return coll with the first n elements removed."},

	// Higher-order utilities
	"complement": {Sig: "(complement pred)          →  fn", Doc: "Return a function that is the logical negation of pred."},
	"identity":   {Sig: "(identity x)               →  x", Doc: "Return x unchanged. Useful as a no-op placeholder function."},
	"constantly": {Sig: "(constantly v)             →  fn", Doc: "Return a function that always returns v regardless of its argument."},
	"comp":       {Sig: "(comp f g h ...)           →  fn", Doc: "Right-to-left function composition. Each fn must be unary."},
	"juxt":       {Sig: "(juxt f g h ...)           →  fn", Doc: "Return a function that applies each fn to its arg, returning a slice of results."},
	"apply":      {Sig: "(apply f args-coll)         →  any", Doc: "Call f with elements of args-coll as arguments. Supports arities 0–6. Works with fn/defn functions, not built-in operators."},
	"partial":    {Sig: "(partial f fixed-args ...)  →  fn", Doc: "Partial application: returns a unary fn with fixed-args pre-applied. Works with fn/defn functions, not built-in operators."},

	// Strings
	"str":          {Sig: "(str & args)              →  string", Doc: "Concatenate all args as strings."},
	"string":       {Sig: "(string x)                →  string", Doc: "Convert x to its string representation."},
	"upper-case":   {Sig: "(upper-case s)            →  string", Doc: "Return s with all letters uppercased."},
	"lower-case":   {Sig: "(lower-case s)            →  string", Doc: "Return s with all letters lowercased."},
	"trim":         {Sig: "(trim s)                  →  string", Doc: "Remove leading and trailing whitespace from s."},
	"starts-with?": {Sig: "(starts-with? s prefix)   →  bool", Doc: "True when string s begins with prefix."},
	"ends-with?":   {Sig: "(ends-with? s suffix)     →  bool", Doc: "True when string s ends with suffix."},
	"replace":      {Sig: "(replace s old new)       →  string", Doc: "Replace all occurrences of old with new in s."},
	"split":        {Sig: "(split s sep)             →  []string", Doc: "Split s on sep and return the parts as a slice."},
	"join":         {Sig: "(join sep coll)           →  string", Doc: "Join the elements of coll into a string separated by sep."},
	"subs":         {Sig: "(subs s start end?)       →  string", Doc: "Substring of s from start to end (exclusive). Omit end for the rest of the string."},

	// I/O
	"println": {Sig: "(println & args)", Doc: "Print args to stdout followed by a newline."},
	"print":   {Sig: "(print & args)", Doc: "Print args to stdout without a trailing newline."},

	// Type / error
	"int":   {Sig: "(int x)       →  int", Doc: "Convert x to int."},
	"error": {Sig: "(error msg)   →  error", Doc: "Create a new error with message msg."},
	"nil?":  {Sig: "(nil? x)      →  bool", Doc: "True when x is nil."},
	"as":    {Sig: "(as ^T x)     →  T  (type assertion)", Doc: "Assert that x is of type T. Panics if the assertion fails."},

	// Iteration
	"doseq":   {Sig: "(doseq [x coll] body...)", Doc: "Evaluate body for each element x in coll. Returns nil."},
	"dotimes": {Sig: "(dotimes [i n] body...)", Doc: "Evaluate body n times with i bound to 0, 1, ..., n-1. Returns nil."},

	// JSON
	"json/encode": {Sig: "(json/encode x)  →  [string error]", Doc: "Encode x as a JSON string. Use with if-err to handle errors."},
	"json/decode": {Sig: "(json/decode s)  →  [any error]", Doc: "Decode JSON string s into a value. Use with if-err to handle errors."},

	// HTTP client — all return [response error] for use with if-err.
	// Response map: {"status" <int> "headers" {...} "body" <string>}
	"http/get":     {Sig: "(http/get url)  →  [response error]\n(http/get url headers)", Doc: "HTTP GET request. Optional headers map. Returns response map with \"status\", \"headers\", \"body\" keys."},
	"http/post":    {Sig: "(http/post url body)  →  [response error]\n(http/post url body headers)", Doc: "HTTP POST request. body is a string. Optional headers map."},
	"http/put":     {Sig: "(http/put url body)  →  [response error]\n(http/put url body headers)", Doc: "HTTP PUT request. body is a string. Optional headers map."},
	"http/delete":  {Sig: "(http/delete url)  →  [response error]", Doc: "HTTP DELETE request."},
	"http/request": {Sig: "(http/request opts)  →  [response error]", Doc: "HTTP request with full control. opts map keys: \"method\", \"url\", \"body\", \"headers\"."},

	// Special forms
	"def":    {Sig: "(def ^T name value)", Doc: "Define a top-level variable. Optional ^T annotation declares the Go type."},
	"defn":   {Sig: "(defn ^ReturnType name [params...] body...)", Doc: "Define a named function. Optional ^ReturnType annotation. A leading string literal in the body is treated as a doc string."},
	"fn":     {Sig: "(fn [params...] body...)", Doc: "Create an anonymous function (closure)."},
	"let":    {Sig: "(let [name val ...] body...)", Doc: "Bind local names to values, evaluate body in that scope."},
	"if":     {Sig: "(if cond then else?)", Doc: "Evaluate then when cond is true, else (or nil) otherwise."},
	"when":   {Sig: "(when cond body...)", Doc: "Evaluate body forms when cond is true. Returns nil otherwise."},
	"cond":   {Sig: "(cond test1 val1 ... :else default)", Doc: "Return the value paired with the first truthy test. Use :else as the final fallthrough."},
	"do":     {Sig: "(do body...)", Doc: "Evaluate multiple expressions in order; return the last value."},
	"loop":   {Sig: "(loop [name init ...] body...)", Doc: "Establish a loop with bindings. Use recur to jump back to the top."},
	"recur":  {Sig: "(recur args...)", Doc: "Jump to the nearest enclosing loop or defn with new argument values."},
	"return": {Sig: "(return val?)", Doc: "Early return from the current function, optionally with a value."},
	"values": {Sig: "(values a b)  →  multi-return", Doc: "Return multiple values from a function (Go multi-return)."},
	"->":     {Sig: "(-> x f1 f2 ...)   thread-first", Doc: "Thread x as the first argument through each function call."},
	"->>":    {Sig: "(->> x f1 f2 ...)  thread-last", Doc: "Thread x as the last argument through each function call."},

	// Go interop
	"go":      {Sig: "(go body...)        goroutine", Doc: "Spawn a goroutine to evaluate body concurrently."},
	"defer":   {Sig: "(defer expr)", Doc: "Defer evaluation of expr until the surrounding function returns."},
	"chan":     {Sig: "(chan T cap?)       →  chan T", Doc: "Create a channel of type T with optional buffer capacity."},
	"send!":   {Sig: "(send! ch val)", Doc: "Send val on channel ch. Blocks until the value is received."},
	"recv!":   {Sig: "(recv! ch)          →  T", Doc: "Receive a value from channel ch. Blocks until a value is available."},
	"close!":  {Sig: "(close! ch)", Doc: "Close channel ch."},
	"select!": {Sig: "(select! cases...)", Doc: "Wait on multiple channel operations; execute the first ready case."},
	"if-err":  {Sig: "(if-err [val err] expr on-err on-ok)", Doc: "Destructure a [value error] pair; evaluate on-err if err is non-nil, on-ok otherwise."},

	// fmt package
	"fmt/Println":  {Sig: "(fmt/Println & args)              →  [int error]", Doc: "Print args to stdout with a newline. Returns bytes written and any error."},
	"fmt/Printf":   {Sig: "(fmt/Printf format & args)         →  [int error]", Doc: "Print a formatted string to stdout using Printf-style verbs like %s, %d, %v."},
	"fmt/Sprintf":  {Sig: "(fmt/Sprintf format & args)        →  string", Doc: "Format and return a string using Printf-style verbs like %s, %d, %v."},
	"fmt/Errorf":   {Sig: "(fmt/Errorf format & args)         →  error", Doc: "Create a formatted error. Use %w to wrap another error."},
	"fmt/Fprintf":  {Sig: "(fmt/Fprintf w format & args)      →  [int error]", Doc: "Write a formatted string to writer w."},
	"fmt/Fprintln": {Sig: "(fmt/Fprintln w & args)            →  [int error]", Doc: "Write args followed by a newline to writer w."},
	"fmt/Sscanf":   {Sig: "(fmt/Sscanf str format & args)     →  [int error]", Doc: "Parse values from str according to format, storing results into args."},

	// errors package
	"errors/New": {Sig: "(errors/New msg)  →  error", Doc: "Create a new error with the given message."},

	// os package
	"os/Exit":   {Sig: "(os/Exit code)", Doc: "Exit the process immediately with status code."},
	"os/Getenv": {Sig: "(os/Getenv key)   →  string", Doc: "Return the value of environment variable key, or empty string if unset."},
	"os/Args":   {Sig: "os/Args           →  []string", Doc: "Slice of command-line arguments starting with the program name."},

	// strconv package
	"strconv/Atoi":        {Sig: "(strconv/Atoi s)                    →  [int error]", Doc: "Parse decimal integer string s. Returns [value error]; use with if-err."},
	"strconv/Itoa":        {Sig: "(strconv/Itoa i)                    →  string", Doc: "Convert integer i to its decimal string representation."},
	"strconv/ParseInt":    {Sig: "(strconv/ParseInt s base bitSize)   →  [int64 error]", Doc: "Parse integer string s in the given base (e.g. 10, 16)."},
	"strconv/ParseFloat":  {Sig: "(strconv/ParseFloat s bitSize)      →  [float64 error]", Doc: "Parse floating-point string s. Use bitSize 64 for float64."},
	"strconv/FormatInt":   {Sig: "(strconv/FormatInt i base)          →  string", Doc: "Format integer i in the given base (e.g. 10, 16)."},
	"strconv/FormatFloat": {Sig: "(strconv/FormatFloat f fmt prec)    →  string", Doc: "Format float f with the given format verb and precision."},

	// strings package
	"strings/Contains":   {Sig: "(strings/Contains s substr)     →  bool", Doc: "True when s contains substr."},
	"strings/HasPrefix":  {Sig: "(strings/HasPrefix s prefix)    →  bool", Doc: "True when s begins with prefix."},
	"strings/HasSuffix":  {Sig: "(strings/HasSuffix s suffix)    →  bool", Doc: "True when s ends with suffix."},
	"strings/TrimSpace":  {Sig: "(strings/TrimSpace s)            →  string", Doc: "Remove leading and trailing whitespace from s."},
	"strings/ToUpper":    {Sig: "(strings/ToUpper s)              →  string", Doc: "Return s with all letters uppercased."},
	"strings/ToLower":    {Sig: "(strings/ToLower s)              →  string", Doc: "Return s with all letters lowercased."},
	"strings/Split":      {Sig: "(strings/Split s sep)            →  []string", Doc: "Split s on sep; empty sep splits into individual characters."},
	"strings/Join":       {Sig: "(strings/Join elems sep)         →  string", Doc: "Join string slice elems with sep between each element."},
	"strings/Replace":    {Sig: "(strings/Replace s old new n)   →  string", Doc: "Replace up to n occurrences of old with new in s. Use -1 to replace all."},
	"strings/ReplaceAll": {Sig: "(strings/ReplaceAll s old new)  →  string", Doc: "Replace all occurrences of old with new in s."},
	"strings/Index":      {Sig: "(strings/Index s substr)         →  int", Doc: "Return the byte index of the first occurrence of substr in s, or -1."},
	"strings/TrimPrefix": {Sig: "(strings/TrimPrefix s prefix)   →  string", Doc: "Return s with the leading prefix removed; unchanged if prefix is absent."},
	"strings/TrimSuffix": {Sig: "(strings/TrimSuffix s suffix)   →  string", Doc: "Return s with the trailing suffix removed; unchanged if suffix is absent."},
	"strings/Count":      {Sig: "(strings/Count s substr)         →  int", Doc: "Count non-overlapping occurrences of substr in s."},
	"strings/Repeat":     {Sig: "(strings/Repeat s count)         →  string", Doc: "Return s repeated count times."},
	"strings/Trim":       {Sig: "(strings/Trim s cutset)          →  string", Doc: "Remove leading and trailing characters in cutset from s."},

	// math package
	"math/Abs":   {Sig: "(math/Abs x)     →  float64", Doc: "Absolute value of x."},
	"math/Max":   {Sig: "(math/Max a b)   →  float64", Doc: "Larger of a and b."},
	"math/Min":   {Sig: "(math/Min a b)   →  float64", Doc: "Smaller of a and b."},
	"math/Floor": {Sig: "(math/Floor x)   →  float64", Doc: "Largest integer value not greater than x."},
	"math/Ceil":  {Sig: "(math/Ceil x)    →  float64", Doc: "Smallest integer value not less than x."},
	"math/Round": {Sig: "(math/Round x)   →  float64", Doc: "Round x to the nearest integer."},
	"math/Sqrt":  {Sig: "(math/Sqrt x)    →  float64", Doc: "Square root of x."},
	"math/Pow":   {Sig: "(math/Pow x y)   →  float64", Doc: "x raised to the power y."},

	// log package
	"log/Println": {Sig: "(log/Println & args)"},
	"log/Printf":  {Sig: "(log/Printf format & args)"},
	"log/Fatal":   {Sig: "(log/Fatal & args)", Doc: "Log args and exit the process with status 1."},
	"log/Fatalf":  {Sig: "(log/Fatalf format & args)", Doc: "Log a formatted message and exit the process with status 1."},

	// time package
	"time/Now":         {Sig: "(time/Now)              →  time.Time", Doc: "Return the current local time."},
	"time/Sleep":       {Sig: "(time/Sleep duration)", Doc: "Pause execution for duration. Use with time/Second etc."},
	"time/Since":       {Sig: "(time/Since t)          →  time.Duration", Doc: "Elapsed time since t."},
	"time/Until":       {Sig: "(time/Until t)          →  time.Duration", Doc: "Duration until time t."},
	"time/Second":      {Sig: "time/Second             →  time.Duration  (1s)"},
	"time/Millisecond": {Sig: "time/Millisecond        →  time.Duration  (1ms)"},
	"time/Minute":      {Sig: "time/Minute             →  time.Duration  (1m)"},
	"time/Hour":        {Sig: "time/Hour               →  time.Duration  (1h)"},

	// sort package
	"sort/Slice":   {Sig: "(sort/Slice coll less-fn)", Doc: "Sort coll in-place using less-fn as the comparison: (fn [i j] (< (nth coll i) (nth coll j)))."},
	"sort/Ints":    {Sig: "(sort/Ints s)", Doc: "Sort integer slice s in ascending order in-place."},
	"sort/Strings": {Sig: "(sort/Strings s)", Doc: "Sort string slice s in ascending order in-place."},

	// io
	"io/EOF": {Sig: "io/EOF  →  error", Doc: "Sentinel error returned by readers when there is no more input."},

	// stdlib/web.go (golisp/stdlib)
	"stdlib/Routes":        {Sig: "(stdlib/Routes & routes)                     →  Handler", Doc: "Combine route definitions into a single HTTP handler."},
	"stdlib/GET":           {Sig: "(stdlib/GET pattern handler)                 →  Route", Doc: "Define a GET route matching pattern, handled by handler."},
	"stdlib/POST":          {Sig: "(stdlib/POST pattern handler)                →  Route", Doc: "Define a POST route matching pattern, handled by handler."},
	"stdlib/PUT":           {Sig: "(stdlib/PUT pattern handler)                 →  Route", Doc: "Define a PUT route matching pattern, handled by handler."},
	"stdlib/DELETE":        {Sig: "(stdlib/DELETE pattern handler)              →  Route", Doc: "Define a DELETE route matching pattern, handled by handler."},
	"stdlib/PATCH":         {Sig: "(stdlib/PATCH pattern handler)               →  Route", Doc: "Define a PATCH route matching pattern, handled by handler."},
	"stdlib/Wrap":          {Sig: "(stdlib/Wrap handler & middlewares)          →  Handler", Doc: "Apply middlewares to handler outermost-first."},
	"stdlib/Compose":       {Sig: "(stdlib/Compose & middlewares)               →  Middleware", Doc: "Combine middlewares into a single middleware applied outermost-first."},
	"stdlib/WrapJson":      {Sig: "(stdlib/WrapJson handler)                    →  Handler", Doc: "Parse request JSON body and store it in req[\"json-body\"] for the handler."},
	"stdlib/WrapCors":      {Sig: "(stdlib/WrapCors handler)                    →  Handler", Doc: "Add permissive CORS headers to responses."},
	"stdlib/WrapAuth":      {Sig: "(stdlib/WrapAuth handler)                    →  Handler", Doc: "Extract Bearer token from Authorization header and store in req[\"identity\"]."},
	"stdlib/WrapLogging":   {Sig: "(stdlib/WrapLogging handler)                 →  Handler", Doc: "Log each request method, path, and status code."},
	"stdlib/WrapRecover":   {Sig: "(stdlib/WrapRecover handler)                 →  Handler", Doc: "Recover from panics and return a 500 response."},
	"stdlib/WrapTimeout":   {Sig: "(stdlib/WrapTimeout seconds)                 →  Middleware", Doc: "Abort the request with 503 if the handler takes longer than seconds."},
	"stdlib/JsonResponse":  {Sig: "(stdlib/JsonResponse status body)            →  map[string]any", Doc: "Build a Ring-style response map with the given HTTP status and JSON body."},
	"stdlib/QueryParam":    {Sig: "(stdlib/QueryParam req name)                 →  string", Doc: "Return the value of URL query parameter name from the request."},
	"stdlib/PathParam":     {Sig: "(stdlib/PathParam req name)                  →  string", Doc: "Return the value of URL path parameter name from the request."},
	"stdlib/BodyMap":       {Sig: "(stdlib/BodyMap req)                         →  map[string]any", Doc: "Return the pre-parsed JSON body map stored by WrapJson."},
	"stdlib/Header":        {Sig: "(stdlib/Header req name)                     →  string", Doc: "Return the value of HTTP header name from the request."},
	"stdlib/ServeFiles":    {Sig: "(stdlib/ServeFiles prefix dir)               →  Handler", Doc: "Serve static files from dir under the URL prefix."},
	"stdlib/Serve":         {Sig: "(stdlib/Serve addr handler)                  →  error", Doc: "Start an HTTP server on addr with handler. Blocks until an error occurs."},
	"stdlib/ServeGraceful": {Sig: "(stdlib/ServeGraceful addr handler)", Doc: "Start an HTTP server on addr; blocks and shuts down gracefully on SIGINT/SIGTERM."},

	// Declarations
	"ns":           {Sig: "(ns name (:import [...]))", Doc: "Declare the namespace and list Go package imports."},
	"defstruct":    {Sig: "(defstruct Name ^T1 field1 ...)", Doc: "Define a Go struct type with typed fields."},
	"definterface": {Sig: "(definterface Name (Method [params] ^Ret) ...)", Doc: "Define a Go interface type with method signatures."},
	"defmethod":    {Sig: "(defmethod ^*ReceiverType name [receiver params...] ^RetType body...)", Doc: "Define a method on a struct type. ^T is value receiver, ^*T is pointer receiver. First param is the receiver variable."},
	"deftest":      {Sig: "(deftest name body...)", Doc: "Define a test case. Use assert= and assert-true inside the body."},

	// Assertions
	"assert=":     {Sig: "(assert= expected actual)", Doc: "Fail the test when expected does not equal actual."},
	"assert-true":  {Sig: "(assert-true expr)", Doc: "Fail the test when expr is not true."},
	"assert-false": {Sig: "(assert-false expr)", Doc: "Fail the test when expr is not false."},
	"assert-nil":   {Sig: "(assert-nil expr)", Doc: "Fail the test when expr is not nil."},
	"assert-err":   {Sig: "(assert-err expr)", Doc: "Fail the test when expr does not return an error."},
}
