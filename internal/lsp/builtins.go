package lsp

// BuiltinDoc holds structured documentation for a built-in symbol.
type BuiltinDoc struct {
	Sig string // glisp-syntax signature
	Doc string // one-line description in glisp terms; "" if none
}

// BuiltinDocs maps every glisp built-in name to its signature and description.
// User-defined names take precedence over these entries.
// All Doc strings use glisp terminology — no Go syntax or go doc examples.
var BuiltinDocs = map[string]BuiltinDoc{
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
	"get":          {Sig: "(get m k)             →  any", Doc: "Look up key k in map or slice m. Returns nil if absent."},
	"assoc":        {Sig: "(assoc m k v)         →  map", Doc: "Return a new map with key k set to v."},
	"dissoc":       {Sig: "(dissoc m k)          →  map", Doc: "Return a new map with key k removed."},
	"conj":         {Sig: "(conj coll x)         →  coll", Doc: "Append x to a slice, or add x to a set."},
	"count":        {Sig: "(count coll)          →  int", Doc: "Number of elements in coll."},
	"first":        {Sig: "(first coll)          →  any", Doc: "First element of coll."},
	"rest":         {Sig: "(rest coll)           →  []any", Doc: "All elements of coll after the first."},
	"nth":          {Sig: "(nth coll i)          →  any", Doc: "Element at index i of coll."},
	"keys":         {Sig: "(keys m)              →  []any", Doc: "All keys of map m."},
	"vals":         {Sig: "(vals m)              →  []any", Doc: "All values of map m."},
	"merge":        {Sig: "(merge m1 m2)         →  map", Doc: "Merge two maps; keys in m2 overwrite m1."},
	"map":          {Sig: "(map f coll)          →  []any", Doc: "Apply f to each element of coll and return the results as a slice."},
	"filter":       {Sig: "(filter pred coll)    →  []any", Doc: "Return elements of coll for which pred returns true."},
	"reduce":       {Sig: "(reduce f init coll)  →  any", Doc: "Reduce coll to a single value by applying f with accumulator starting at init."},
	"reverse":      {Sig: "(reverse coll)        →  []any", Doc: "Return coll in reverse order."},
	"contains?":    {Sig: "(contains? coll x)   →  bool", Doc: "True when coll contains x (works for maps, slices, and sets)."},
	"some":         {Sig: "(some pred coll)      →  any", Doc: "Return the first element of coll for which pred is truthy, or nil."},
	"every?":       {Sig: "(every? pred coll)    →  bool", Doc: "True when pred returns true for every element of coll."},
	"sort-by":      {Sig: "(sort-by f coll)      →  []any", Doc: "Sort coll by the value returned by f for each element."},
	"sort":         {Sig: "(sort coll)           →  []any", Doc: "Sort coll in natural order (int/float64/string)."},
	"min-key":      {Sig: "(min-key f x y & more)  →  any", Doc: "Return the element with the smallest (f elem) value."},
	"max-key":      {Sig: "(max-key f x y & more)  →  any", Doc: "Return the element with the largest (f elem) value."},
	"flatten":      {Sig: "(flatten coll)        →  []any", Doc: "Recursively flatten nested slices into a single slice."},
	"range":        {Sig: "(range n) or (range start end)  →  []int", Doc: "Integer slice from 0 to n (exclusive), or from start to end."},
	"take":         {Sig: "(take n coll)         →  []any", Doc: "Return the first n elements of coll."},
	"drop":         {Sig: "(drop n coll)         →  []any", Doc: "Return coll with the first n elements removed."},
	"second":       {Sig: "(second coll)              →  any", Doc: "Return the second element of coll."},
	"last":         {Sig: "(last coll)                →  any", Doc: "Return the last element of coll."},
	"empty?":       {Sig: "(empty? coll)              →  bool", Doc: "Return true if coll is nil or has no elements."},
	"not-empty":    {Sig: "(not-empty coll)           →  any", Doc: "Return coll if not empty, nil otherwise."},
	"get-in":       {Sig: "(get-in m keys)            →  any", Doc: "Nested map/slice access via a vector of keys."},
	"assoc-in":     {Sig: "(assoc-in m keys v)        →  map", Doc: "Return m with value v nested at path keys."},
	"update-in":    {Sig: "(update-in m keys f)       →  map", Doc: "Apply f to the value nested at path keys."},
	"update":       {Sig: "(update m key f)           →  map", Doc: "Apply f to the value at key, return updated map."},
	"select-keys":  {Sig: "(select-keys m keys)       →  map", Doc: "Return a map containing only the given keys."},
	"rename-keys":  {Sig: "(rename-keys m kmap)       →  map", Doc: "Return m with keys renamed per kmap."},
	"group-by":     {Sig: "(group-by f coll)          →  map", Doc: "Group coll elements by key function f (fn or keyword)."},
	"frequencies":  {Sig: "(frequencies coll)         →  map", Doc: "Count occurrences of each element."},
	"into":         {Sig: "(into target coll)         →  any", Doc: "Build a collection: map target assocs pairs, vec target appends elements."},
	"concat":       {Sig: "(concat & colls)           →  []any", Doc: "Join sequences into one slice."},
	"mapcat":       {Sig: "(mapcat f coll)            →  []any", Doc: "Map f over coll then flatten one level."},
	"take-while":   {Sig: "(take-while pred coll)     →  []any", Doc: "Take elements while pred returns truthy."},
	"drop-while":   {Sig: "(drop-while pred coll)     →  []any", Doc: "Drop elements while pred returns truthy."},
	"zipmap":       {Sig: "(zipmap keys vals)         →  map", Doc: "Build a map from two sequences."},
	"partition":    {Sig: "(partition n coll)         →  []any", Doc: "Split coll into chunks of size n (incomplete last chunk is dropped)."},
	"partition-by": {Sig: "(partition-by f coll)      →  []any", Doc: "Split coll at each change in (f item)."},
	"distinct":     {Sig: "(distinct coll)            →  []any", Doc: "Remove duplicate elements from coll, preserving order."},
	"remove":       {Sig: "(remove pred coll)         →  []any", Doc: "Return elements of coll for which pred returns false/nil (inverse of filter)."},
	"keep":         {Sig: "(keep f coll)              →  []any", Doc: "Map f over coll, dropping nil results."},
	"split-at":     {Sig: "(split-at n coll)          →  [[before] [after]]", Doc: "Split coll into [[first n] [rest]] at index n."},
	"split-with":   {Sig: "(split-with pred coll)     →  [[taken] [rest]]", Doc: "Split coll into [[take-while pred] [drop-while pred]]."},
	"interleave":   {Sig: "(interleave & colls)       →  []any", Doc: "Interleave elements of colls; stops at the shortest."},
	"not-any?":     {Sig: "(not-any? pred coll)       →  bool", Doc: "True when pred returns false/nil for every element of coll."},
	"map-vals":     {Sig: "(map-vals f m)             →  map", Doc: "Return m with f applied to each value."},
	"map-keys":     {Sig: "(map-keys f m)             →  map", Doc: "Return m with f applied to each key (f must return a string)."},
	"reduce-kv":    {Sig: "(reduce-kv f init m)       →  any", Doc: "Reduce map m with a 3-arg fn (fn acc k v)."},

	// Sets
	"union":        {Sig: "(union s1 s2)              →  set", Doc: "Return a set containing all elements of s1 and s2."},
	"intersection": {Sig: "(intersection s1 s2)       →  set", Doc: "Return a set containing only elements in both s1 and s2."},
	"difference":   {Sig: "(difference s1 s2)         →  set", Doc: "Return a set of elements in s1 that are not in s2."},

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
	"format":       {Sig: "(format fmt & args)       →  string", Doc: "Format string using fmt.Sprintf directives (e.g. %s, %d, %v)."},
	"parse-int":    {Sig: "(parse-int s)             →  [int error]", Doc: "Parse string s as a base-10 integer. Returns (int, error) — use with if-err."},
	"parse-float":  {Sig: "(parse-float s)           →  [float64 error]", Doc: "Parse string s as a 64-bit float. Returns (float64, error) — use with if-err."},
	"repeat":       {Sig: "(repeat n val)            →  []any", Doc: "Return a slice of n copies of val."},
	"interpose":    {Sig: "(interpose sep coll)       →  []any", Doc: "Return a new seq with sep inserted between each element of coll."},
	"blank?":       {Sig: "(blank? s)                →  bool", Doc: "True when s is nil or contains only whitespace."},
	"capitalize":   {Sig: "(capitalize s)            →  string", Doc: "Return s with the first character uppercased and the rest lowercased."},

	// Numeric predicates and arithmetic
	"even?": {Sig: "(even? n)  →  bool", Doc: "True when n is even."},
	"odd?":  {Sig: "(odd? n)   →  bool", Doc: "True when n is odd."},
	"pos?":  {Sig: "(pos? n)   →  bool", Doc: "True when n is positive (> 0)."},
	"neg?":  {Sig: "(neg? n)   →  bool", Doc: "True when n is negative (< 0)."},
	"zero?": {Sig: "(zero? n)  →  bool", Doc: "True when n is zero."},
	"inc":   {Sig: "(inc n)    →  any", Doc: "Increment n by 1. Preserves int/float64 type."},
	"dec":   {Sig: "(dec n)    →  any", Doc: "Decrement n by 1. Preserves int/float64 type."},

	// Type / error
	"int":   {Sig: "(int x)       →  int", Doc: "Convert x to int."},
	"error": {Sig: "(error msg)   →  error", Doc: "Create a new error with message msg."},
	"nil?":  {Sig: "(nil? x)      →  bool", Doc: "True when x is nil."},
	"as":    {Sig: "(as T x)      →  T  (type assertion)", Doc: "Assert that x is of type T. Panics if the assertion fails."},

	// Iteration
	"doseq":    {Sig: "(doseq [x coll] body...)", Doc: "Evaluate body for each element x in coll. Returns nil."},
	"dotimes":  {Sig: "(dotimes [i n] body...)", Doc: "Evaluate body n times with i bound to 0, 1, ..., n-1. Returns nil."},
	"for-chan": {Sig: "(for-chan [x ch] body...)", Doc: "Range over channel ch until it is closed, binding each received value to x."},

	// Concurrency
	"go-val":    {Sig: "(go-val body...)  →  chan any", Doc: "Run body in a goroutine; return a buffered chan any immediately. Use (recv! ch) to block until the result arrives. Like Clojure's future."},
	"par":       {Sig: "(par expr1 expr2 ...)", Doc: "Run each expression in its own goroutine and block until all finish (sync.WaitGroup). No result collection — use go-val + recv! for that."},
	"recv-ok!":  {Sig: "(recv-ok! ch)  →  [val ok]", Doc: "Comma-ok channel receive. Returns []any{val, ok}. Destructure with [[val ok] (recv-ok! ch)]. Check ok with (= ok true) — it is any, not bool."},
	"with-lock": {Sig: "(with-lock mu body...)", Doc: "Execute body inside a mutex critical section. Emits mu.Lock()/defer mu.Unlock() inside an IIFE, so unlock is guaranteed even on panic."},

	// OS
	"os/env": {Sig: "(os/env name)  →  string\n(os/env name default)  →  string", Doc: "Read environment variable. Returns empty string if unset. With default, returns default when the variable is unset."},

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
	"def":    {Sig: "(def name type value)", Doc: "Define a top-level variable. Optional type declares the Go type (3-arg form)."},
	"defn":   {Sig: "(defn name [p1 T1 ...] -> RetType body...)", Doc: "Define a named function. -> RetType is optional. A leading string literal in the body is treated as a doc string."},
	"fn":     {Sig: "(fn [params...] body...)", Doc: "Create an anonymous function (closure)."},
	"let":    {Sig: "(let [name val ...] body...)", Doc: "Bind local names to values, evaluate body in that scope."},
	"if":     {Sig: "(if cond then else?)", Doc: "Evaluate then when cond is true, else (or nil) otherwise."},
	"when":   {Sig: "(when cond body...)", Doc: "Evaluate body forms when cond is true. Returns nil otherwise."},
	"cond":   {Sig: "(cond test1 val1 ... :else default)", Doc: "Return the value paired with the first truthy test. Use :else as the final fallthrough."},
	"switch": {Sig: "(switch expr val1 body1 ... :default body)", Doc: "Dispatch on the value of expr; evaluate the body paired with the matching val. Use :default for the fallthrough case."},
	"do":     {Sig: "(do body...)", Doc: "Evaluate multiple expressions in order; return the last value."},
	"loop":   {Sig: "(loop [name init ...] body...)", Doc: "Establish a loop with bindings. Use recur to jump back to the top."},
	"recur":  {Sig: "(recur args...)", Doc: "Jump to the nearest enclosing loop or defn with new argument values."},
	"return": {Sig: "(return val?)", Doc: "Early return from the current function, optionally with a value."},
	"values": {Sig: "(values a b)  →  multi-return", Doc: "Return multiple values from a function (Go multi-return)."},
	"->":     {Sig: "(-> x f1 f2 ...)   thread-first", Doc: "Thread x as the first argument through each function call."},
	"->>":    {Sig: "(->> x f1 f2 ...)  thread-last", Doc: "Thread x as the last argument through each function call."},

	// Go interop
	"go":       {Sig: "(go body...)        goroutine", Doc: "Spawn a goroutine to evaluate body concurrently."},
	"defer":    {Sig: "(defer expr)", Doc: "Defer evaluation of expr until the surrounding function returns."},
	"chan":     {Sig: "(chan T cap?)       →  chan T", Doc: "Create a channel of type T with optional buffer capacity."},
	"send!":    {Sig: "(send! ch val)", Doc: "Send val on channel ch. Blocks until the value is received."},
	"recv!":    {Sig: "(recv! ch)          →  T", Doc: "Receive a value from channel ch. Blocks until a value is available."},
	"close!":   {Sig: "(close! ch)", Doc: "Close channel ch."},
	"select!":  {Sig: "(select! cases...)", Doc: "Wait on multiple channel operations; execute the first ready case."},
	"if-err":   {Sig: "(if-err [val err] expr on-err on-ok)", Doc: "Destructure a [value error] pair; evaluate on-err if err is non-nil, on-ok otherwise."},
	"if-let":   {Sig: "(if-let [pat expr] then else?)", Doc: "Bind pat from expr; if the value is non-nil, evaluate then (bindings in scope), otherwise else (nil if omitted). Supports destructuring patterns."},
	"when-let": {Sig: "(when-let [pat expr] body...)", Doc: "Bind pat from expr; if the value is non-nil, evaluate body, otherwise nil. Supports destructuring patterns."},

	// fmt package
	"fmt/println":  {Sig: "(fmt/println & args)              →  nil", Doc: "Print args to stdout with a newline. Returns nil."},
	"fmt/print":    {Sig: "(fmt/print & args)                →  nil", Doc: "Print args to stdout without a trailing newline. Returns nil."},
	"fmt/printf":   {Sig: "(fmt/printf format & args)         →  [int error]", Doc: "Print a formatted string to stdout using Printf-style verbs like %s, %d, %v."},
	"fmt/sprintf":  {Sig: "(fmt/sprintf format & args)        →  string", Doc: "Format and return a string using Printf-style verbs like %s, %d, %v."},
	"fmt/errorf":   {Sig: "(fmt/errorf format & args)         →  error", Doc: "Create a formatted error. Use %w to wrap another error."},
	"fmt/fprintf":  {Sig: "(fmt/fprintf w format & args)      →  [int error]", Doc: "Write a formatted string to writer w."},
	"fmt/fprintln": {Sig: "(fmt/fprintln w & args)            →  [int error]", Doc: "Write args followed by a newline to writer w."},
	"fmt/sscanf":   {Sig: "(fmt/sscanf str format & args)     →  [int error]", Doc: "Parse values from str according to format, storing results into args."},

	// errors package
	"errors/new": {Sig: "(errors/new msg)  →  error", Doc: "Create a new error with the given message."},

	// os package
	"os/exit": {Sig: "(os/exit code)", Doc: "Exit the process immediately with status code."},
	"os/args": {Sig: "os/args  →  []string", Doc: "Slice of command-line arguments starting with the program name."},

	// strconv package
	"strconv/atoi":         {Sig: "(strconv/atoi s)                    →  [int error]", Doc: "Parse decimal integer string s. Returns [value error]; use with if-err."},
	"strconv/itoa":         {Sig: "(strconv/itoa i)                    →  string", Doc: "Convert integer i to its decimal string representation."},
	"strconv/parse-int":    {Sig: "(strconv/parse-int s base bitSize)   →  [int64 error]", Doc: "Parse integer string s in the given base (e.g. 10, 16)."},
	"strconv/parse-float":  {Sig: "(strconv/parse-float s bitSize)      →  [float64 error]", Doc: "Parse floating-point string s. Use bitSize 64 for float64."},
	"strconv/format-int":   {Sig: "(strconv/format-int i base)          →  string", Doc: "Format integer i in the given base (e.g. 10, 16)."},
	"strconv/format-float": {Sig: "(strconv/format-float f fmt prec)    →  string", Doc: "Format float f with the given format verb and precision."},

	// strings package
	"strings/contains":    {Sig: "(strings/contains s substr)     →  bool", Doc: "True when s contains substr."},
	"strings/has-prefix":  {Sig: "(strings/has-prefix s prefix)    →  bool", Doc: "True when s begins with prefix."},
	"strings/has-suffix":  {Sig: "(strings/has-suffix s suffix)    →  bool", Doc: "True when s ends with suffix."},
	"strings/trim-space":  {Sig: "(strings/trim-space s)            →  string", Doc: "Remove leading and trailing whitespace from s."},
	"strings/to-upper":    {Sig: "(strings/to-upper s)              →  string", Doc: "Return s with all letters uppercased."},
	"strings/to-lower":    {Sig: "(strings/to-lower s)              →  string", Doc: "Return s with all letters lowercased."},
	"strings/split":       {Sig: "(strings/split s sep)            →  []string", Doc: "Split s on sep; empty sep splits into individual characters."},
	"strings/join":        {Sig: "(strings/join elems sep)         →  string", Doc: "Join string slice elems with sep between each element."},
	"strings/replace":     {Sig: "(strings/replace s old new n)   →  string", Doc: "Replace up to n occurrences of old with new in s. Use -1 to replace all."},
	"strings/replace-all": {Sig: "(strings/replace-all s old new)  →  string", Doc: "Replace all occurrences of old with new in s."},
	"strings/index":       {Sig: "(strings/index s substr)         →  int", Doc: "Return the byte index of the first occurrence of substr in s, or -1."},
	"strings/trim-prefix": {Sig: "(strings/trim-prefix s prefix)   →  string", Doc: "Return s with the leading prefix removed; unchanged if prefix is absent."},
	"strings/trim-suffix": {Sig: "(strings/trim-suffix s suffix)   →  string", Doc: "Return s with the trailing suffix removed; unchanged if suffix is absent."},
	"strings/count":       {Sig: "(strings/count s substr)         →  int", Doc: "Count non-overlapping occurrences of substr in s."},
	"strings/repeat":      {Sig: "(strings/repeat s count)         →  string", Doc: "Return s repeated count times."},
	"strings/trim":        {Sig: "(strings/trim s cutset)          →  string", Doc: "Remove leading and trailing characters in cutset from s."},

	// math package
	"math/abs":   {Sig: "(math/abs x)     →  float64", Doc: "Absolute value of x."},
	"math/max":   {Sig: "(math/max a b)   →  float64", Doc: "Larger of a and b."},
	"math/min":   {Sig: "(math/min a b)   →  float64", Doc: "Smaller of a and b."},
	"math/floor": {Sig: "(math/floor x)   →  float64", Doc: "Largest integer value not greater than x."},
	"math/ceil":  {Sig: "(math/ceil x)    →  float64", Doc: "Smallest integer value not less than x."},
	"math/round": {Sig: "(math/round x)   →  float64", Doc: "Round x to the nearest integer."},
	"math/sqrt":  {Sig: "(math/sqrt x)    →  float64", Doc: "Square root of x."},
	"math/pow":   {Sig: "(math/pow x y)   →  float64", Doc: "x raised to the power y."},

	// log package
	"log/println": {Sig: "(log/println & args)"},
	"log/printf":  {Sig: "(log/printf format & args)"},
	"log/fatal":   {Sig: "(log/fatal & args)", Doc: "Log args and exit the process with status 1."},
	"log/fatalf":  {Sig: "(log/fatalf format & args)", Doc: "Log a formatted message and exit the process with status 1."},

	// time package
	"time/now":         {Sig: "(time/now)              →  time.Time", Doc: "Return the current local time."},
	"time/sleep":       {Sig: "(time/sleep duration)", Doc: "Pause execution for duration. Use with time/second etc."},
	"time/since":       {Sig: "(time/since t)          →  time.Duration", Doc: "Elapsed time since t."},
	"time/until":       {Sig: "(time/until t)          →  time.Duration", Doc: "Duration until time t."},
	"time/second":      {Sig: "time/second             →  time.Duration  (1s)"},
	"time/millisecond": {Sig: "time/millisecond        →  time.Duration  (1ms)"},
	"time/minute":      {Sig: "time/minute             →  time.Duration  (1m)"},
	"time/hour":        {Sig: "time/hour               →  time.Duration  (1h)"},

	// sort package
	"sort/slice":   {Sig: "(sort/slice coll less-fn)", Doc: "Sort coll in-place using less-fn as the comparison: (fn [i j] (< (nth coll i) (nth coll j)))."},
	"sort/ints":    {Sig: "(sort/ints s)", Doc: "Sort integer slice s in ascending order in-place."},
	"sort/strings": {Sig: "(sort/strings s)", Doc: "Sort string slice s in ascending order in-place."},

	// io
	"io/eof": {Sig: "io/EOF  →  error", Doc: "Sentinel error returned by readers when there is no more input."},

	// web/web.go (golisp/web) — types
	"web/Request":  {Sig: "type Request = map[string]any", Doc: "Ring-style request map. Keys: \"method\", \"path\", \"query\", \"headers\", \"body\", \"remote-addr\", \"host\", \"scheme\"."},
	"web/Response": {Sig: "type Response = map[string]any", Doc: "Ring-style response map. Keys: \"status\", \"headers\", \"body\"."},
	"web/Handler":  {Sig: "type Handler func(req Request) Response", Doc: "Ring-style handler: takes a request map and returns a response map."},

	// web/web.go (golisp/web)
	"web/routes":         {Sig: "(web/routes & routes)                     →  Handler", Doc: "Combine route definitions into a single HTTP handler. Returns 405 (with Allow header) when the path matches but the method does not; 404 when nothing matches."},
	"web/get":            {Sig: "(web/get pattern handler)                 →  Route", Doc: "Define a GET route matching pattern, handled by handler. Patterns support :name segment params and a trailing *name wildcard."},
	"web/post":           {Sig: "(web/post pattern handler)                →  Route", Doc: "Define a POST route matching pattern, handled by handler."},
	"web/put":            {Sig: "(web/put pattern handler)                 →  Route", Doc: "Define a PUT route matching pattern, handled by handler."},
	"web/delete":         {Sig: "(web/delete pattern handler)              →  Route", Doc: "Define a DELETE route matching pattern, handled by handler."},
	"web/patch":          {Sig: "(web/patch pattern handler)               →  Route", Doc: "Define a PATCH route matching pattern, handled by handler."},
	"web/head":           {Sig: "(web/head pattern handler)                →  Route", Doc: "Define a HEAD route matching pattern, handled by handler."},
	"web/options":        {Sig: "(web/options pattern handler)             →  Route", Doc: "Define an OPTIONS route matching pattern, handled by handler. Note: wrap-cors answers OPTIONS preflights before routing."},
	"web/context":        {Sig: "(web/context prefix & routes)             →  []Route", Doc: "Group routes under a common path prefix. Nests; pass the result to web/routes."},
	"web/wrap":           {Sig: "(web/wrap handler & middlewares)          →  Handler", Doc: "Apply middlewares to handler outermost-first."},
	"web/compose":        {Sig: "(web/compose & middlewares)               →  Middleware", Doc: "Combine middlewares into a single middleware applied outermost-first."},
	"web/wrap-json":      {Sig: "(web/wrap-json handler)                    →  Handler", Doc: "Parse request JSON body and store it in req[\"json-body\"] for the handler."},
	"web/wrap-cors":      {Sig: "(web/wrap-cors handler)                    →  Handler", Doc: "Add permissive CORS headers to responses and answer OPTIONS preflight requests with 204."},
	"web/wrap-auth":      {Sig: "(web/wrap-auth handler)                    →  Handler", Doc: "Extract Bearer token from Authorization header and store in req[\"identity\"]."},
	"web/wrap-logging":   {Sig: "(web/wrap-logging handler)                 →  Handler", Doc: "Log each request method, path, and status code."},
	"web/wrap-auth-func": {Sig: "(web/wrap-auth-func check)                 →  Middleware", Doc: "Like wrap-auth, but validates the Bearer token with check (fn [token string] -> bool); 401 if rejected."},
	"web/wrap-recover":   {Sig: "(web/wrap-recover handler)                 →  Handler", Doc: "Recover from panics: log the panic with a stack trace and return a generic 500 response."},
	"web/wrap-timeout":   {Sig: "(web/wrap-timeout seconds)                 →  Middleware", Doc: "Abort the request with 503 if the handler takes longer than seconds. The timed-out handler keeps running in the background; its response is discarded."},
	"web/json-response":  {Sig: "(web/json-response status body)            →  Response", Doc: "Build a Ring-style response map with the given HTTP status and JSON body. Returns a 500 error response if the body cannot be JSON-encoded."},
	"web/bad-request":    {Sig: "(web/bad-request msg)                      →  Response", Doc: "Build a 400 JSON response: {\"error\": msg}."},
	"web/unauthorized":   {Sig: "(web/unauthorized msg)                     →  Response", Doc: "Build a 401 JSON response: {\"error\": msg}."},
	"web/not-found":      {Sig: "(web/not-found msg)                        →  Response", Doc: "Build a 404 JSON response: {\"error\": msg}."},
	"web/server-error":   {Sig: "(web/server-error msg)                     →  Response", Doc: "Build a 500 JSON response: {\"error\": msg}."},
	"web/no-content":     {Sig: "(web/no-content)                           →  Response", Doc: "Build an empty 204 response."},
	"web/query-param":    {Sig: "(web/query-param req name)                 →  string", Doc: "Return the value of URL query parameter name from the request."},
	"web/form-param":     {Sig: "(web/form-param req name)                  →  string", Doc: "Return the named field from a urlencoded request body."},
	"web/cookie":         {Sig: "(web/cookie req name)                      →  string", Doc: "Return the value of the named cookie from the request's Cookie header."},
	"web/set-cookie":     {Sig: "(web/set-cookie resp name value)           →  Response", Doc: "Add a Set-Cookie header (path /) to the response and return it."},
	"web/path-param":     {Sig: "(web/path-param req name)                  →  string", Doc: "Return the value of URL path parameter name from the request."},
	"web/body-map":       {Sig: "(web/body-map req)                         →  map[string]any", Doc: "Return the JSON-decoded body as a plain map (not a Response — the body content itself)."},
	"web/header":         {Sig: "(web/header req name)                     →  string", Doc: "Return the value of HTTP header name from the request (case-insensitive)."},
	"web/serve-files":    {Sig: "(web/serve-files prefix dir)               →  Handler", Doc: "Serve static files from dir under the URL prefix. Forwards request headers, so conditional and Range requests work."},
	"web/serve":          {Sig: "(web/serve addr handler)                  →  error", Doc: "Start an HTTP server on addr with handler. Blocks until an error occurs."},
	"web/serve-graceful": {Sig: "(web/serve-graceful addr handler)", Doc: "Start an HTTP server on addr; blocks and shuts down gracefully on SIGINT/SIGTERM."},

	// Declarations
	"ns":           {Sig: "(ns name (:import [...]))", Doc: "Declare the namespace and list Go package imports."},
	"defstruct":    {Sig: "(defstruct Name field1 T1 ...)", Doc: "Define a Go struct type with typed fields (field name then type)."},
	"definterface": {Sig: "(definterface Name (Method [params] -> Ret) ...)", Doc: "Define a Go interface type with method signatures."},
	"defmethod":    {Sig: "(defmethod *ReceiverType name [receiver params...] -> RetType body...)", Doc: "Define a method on a struct type. *T is pointer receiver, T is value receiver. First param is the receiver variable."},
	"deftest":      {Sig: "(deftest name body...)", Doc: "Define a test case. Use assert= and assert-true inside the body."},

	// Assertions
	"assert=":      {Sig: "(assert= expected actual)", Doc: "Fail the test when expected does not equal actual."},
	"assert-true":  {Sig: "(assert-true expr)", Doc: "Fail the test when expr is not true."},
	"assert-false": {Sig: "(assert-false expr)", Doc: "Fail the test when expr is not false."},
	"assert-nil":   {Sig: "(assert-nil expr)", Doc: "Fail the test when expr is not nil."},
	"assert-err":   {Sig: "(assert-err expr)", Doc: "Fail the test when expr does not return an error."},

	// File I/O
	"read-file":    {Sig: "(read-file path)              →  [string error]", Doc: "Read the entire file at path and return its contents as a string."},
	"write-file":   {Sig: "(write-file path content)     →  error", Doc: "Write content to path, creating or truncating the file."},
	"append-file":  {Sig: "(append-file path content)    →  error", Doc: "Append content to path, creating the file if it does not exist."},
	"file-exists?": {Sig: "(file-exists? path)           →  bool", Doc: "Return true if a file or directory exists at path."},
	"list-dir":     {Sig: "(list-dir path)               →  [[]string error]", Doc: "Return the names of entries in the directory at path."},
	"mkdir":        {Sig: "(mkdir path)                  →  error", Doc: "Create path and any missing parent directories."},

	// Regex
	"re/match":    {Sig: "(re/match pattern s)           →  bool", Doc: "Return true if s matches the regular expression pattern. Panics on invalid pattern."},
	"re/find":     {Sig: "(re/find pattern s)            →  any", Doc: "Return the leftmost match of pattern in s, or nil if no match."},
	"re/find-all": {Sig: "(re/find-all pattern s)        →  []any", Doc: "Return all non-overlapping matches of pattern in s."},
	"re/replace":  {Sig: "(re/replace pattern s repl)    →  string", Doc: "Replace all matches of pattern in s with repl."},
	"re/split":    {Sig: "(re/split pattern s)           →  []any", Doc: "Split s into substrings separated by matches of pattern."},

	// Structured logging (log/slog)
	"log/info":  {Sig: "(log/info msg k v ...)  →  nil", Doc: "Log a message at INFO level with optional key-value pairs."},
	"log/debug": {Sig: "(log/debug msg k v ...) →  nil", Doc: "Log a message at DEBUG level with optional key-value pairs."},
	"log/warn":  {Sig: "(log/warn msg k v ...)  →  nil", Doc: "Log a message at WARN level with optional key-value pairs."},
	"log/error": {Sig: "(log/error msg k v ...) →  nil", Doc: "Log a message at ERROR level with optional key-value pairs."},

	// Error wrapping
	"wrap-error": {Sig: "(wrap-error msg err)   →  error", Doc: "Wrap err with a context message: returns an error whose message is \"msg: err\"."},
	"errors/is?": {Sig: "(errors/is? err target) →  bool", Doc: "Return true if err or any error in its chain matches target."},

	// Atom — shared mutable state
	"atom":   {Sig: "(atom init)       →  atom", Doc: "Create a thread-safe mutable reference wrapping init."},
	"swap!":  {Sig: "(swap! a f)       →  any", Doc: "Atomically update atom a by applying f to its current value. Returns the new value."},
	"reset!": {Sig: "(reset! a v)      →  any", Doc: "Atomically set atom a to v. Returns v."},
	"deref":  {Sig: "(deref a)         →  any", Doc: "Read the current value of atom a."},
}
