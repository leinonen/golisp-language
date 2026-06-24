package transpiler

import "fmt"

// RuntimeSource returns a complete Go source file containing the runtime helpers
// needed for the given set of built-in packages. Used by the multi-file compiler
// to emit helpers into a single shared file instead of every generated file.
func RuntimeSource(pkgName string, builtins map[string]bool) string {
	seen := map[string]bool{}
	var imports []string
	addImport := func(pkg string) {
		if !seen[pkg] {
			seen[pkg] = true
			imports = append(imports, `"`+pkg+`"`)
		}
	}
	// _glispToInt/_glispToFloat64 in glispRuntime (always emitted) parse numeric
	// strings via strconv.
	addImport("strconv")
	// _glispToSlice/_glispLen in glispRuntime fall back to reflection for
	// user-typed slices.
	addImport("reflect")
	if builtins["sort"] {
		addImport("sort")
	}
	if builtins["strings"] || builtins["_strruntime"] || builtins["net/http"] {
		addImport("strings")
	}
	if builtins["strings"] || builtins["_strruntime"] {
		addImport("fmt") // _glispJoin uses fmt.Sprintf for non-string elements
	}
	if builtins["encoding/json"] {
		addImport("encoding/json")
		addImport("fmt")
	}
	if builtins["encoding/csv"] {
		addImport("encoding/csv")
		addImport("strings")
		addImport("sort")
	}
	if builtins["net/http"] {
		addImport("fmt")
		addImport("io")
		addImport("net/http")
	}
	if builtins["os"] {
		addImport("fmt")
		addImport("os")
	}
	if builtins["_file"] {
		addImport("fmt")
		addImport("os")
	}
	if builtins["regexp"] {
		addImport("fmt")
		addImport("regexp")
	}
	if builtins["data"] {
		addImport("fmt")
	}
	if builtins["_pp"] {
		addImport("fmt")
	}
	if builtins["_atom"] {
		addImport("sync")
	}
	if builtins["_ctx"] {
		addImport("context")
		addImport("time")
	}
	if builtins["_proc"] {
		addImport("bytes")
		addImport("os/exec")
	}
	if builtins["_path"] {
		addImport("path/filepath")
	}
	if builtins["_walk"] {
		addImport("path/filepath")
		addImport("io/fs")
	}
	if builtins["_lines"] {
		addImport("bufio")
		addImport("os")
	}
	if builtins["_jsonstream"] {
		addImport("os")
	}
	// "_set" is a pseudo-package marker for set algebra helpers (no real import needed)
	// "_file" is a pseudo-package marker for file I/O helpers (real imports: os, fmt)
	// "_atom" is a pseudo-package marker for atom helpers (real import: sync)
	// "_close" is a pseudo-package marker for with-open's _glispClose (no real import needed)
	// "_ctx" is a pseudo-package marker for context helpers (real imports: context, time)
	// "regexp" gates regex helpers (real imports: regexp, fmt)

	s := fmt.Sprintf("package %s\n", pkgName)
	if len(imports) > 0 {
		s += "\nimport (\n"
		for _, imp := range imports {
			s += "\t" + imp + "\n"
		}
		s += ")\n"
	}
	s += glispRuntime
	if builtins["sort"] {
		s += glispSortRuntime
	}
	if builtins["strings"] || builtins["_strruntime"] {
		s += glispStrRuntime
	}
	if builtins["encoding/json"] {
		s += glispJsonRuntime
	}
	if builtins["encoding/csv"] {
		s += glispCsvRuntime
	}
	if builtins["_xf"] {
		s += glispXfRuntime
	}
	if builtins["_lines"] {
		s += glispLineRuntime
	}
	if builtins["_jsonstream"] {
		s += glispJsonStreamRuntime
	}
	if builtins["net/http"] {
		s += glispHttpRuntime
	}
	if builtins["os"] {
		s += glispEnvRuntime
	}
	if builtins["_file"] {
		s += glispFileRuntime
	}
	if builtins["regexp"] {
		s += glispReRuntime
	}
	if builtins["data"] {
		s += glispDataRuntime
	}
	if builtins["_pp"] {
		s += glispPPRuntime
	}
	if builtins["_num"] {
		s += glispNumRuntime
	}
	if builtins["_set"] {
		s += glispSetRuntime
	}
	if builtins["_atom"] {
		s += glispAtomRuntime
	}
	if builtins["_close"] {
		s += glispCloseRuntime
	}
	if builtins["_ctx"] {
		s += glispCtxRuntime
	}
	if builtins["_proc"] {
		s += glispProcRuntime
	}
	if builtins["_path"] {
		s += glispPathRuntime
	}
	if builtins["_walk"] {
		s += glispWalkRuntime
	}
	return s
}

// glispRuntime is emitted at the top of every generated Go file.
// It provides the runtime helpers used by built-in functions.
const glispRuntime = `
// --- glisp runtime helpers (generated) ---

func _glispGet(m any, key any) any {
	if mm, ok := m.(map[string]any); ok {
		if k, ok := key.(string); ok {
			return mm[k]
		}
		return nil
	}
	// Slices (including concrete []string, []int, … via _glispToSlice);
	// out-of-range indexes return nil, matching map-lookup semantics.
	if s := _glispToSlice(m); s != nil {
		idx := -1
		switch k := key.(type) {
		case int:
			idx = k
		case int64:
			idx = int(k)
		}
		if idx >= 0 && idx < len(s) {
			return s[idx]
		}
		return nil
	}
	// Struct (or pointer-to-struct) field access by name via reflection, so
	// (:field x) / (get x "field") works on an any value that happens to hold a
	// declared struct. The glisp key (kebab-case keyword/string) is matched
	// against exported Go field names ignoring case and hyphens/underscores
	// (Title from "title", FirstName from "first-name"). Missing field -> nil,
	// like a map miss.
	if k, ok := key.(string); ok {
		rv := reflect.ValueOf(m)
		for rv.Kind() == reflect.Ptr {
			if rv.IsNil() {
				return nil
			}
			rv = rv.Elem()
		}
		if rv.Kind() == reflect.Struct {
			rt := rv.Type()
			for i := 0; i < rt.NumField(); i++ {
				sf := rt.Field(i)
				if sf.PkgPath != "" { // unexported field
					continue
				}
				if _glispFieldEq(sf.Name, k) {
					return rv.Field(i).Interface()
				}
			}
		}
	}
	return nil
}

// _glispFieldEq reports whether a Go struct field name and a glisp key name
// refer to the same field, ignoring ASCII case and hyphens/underscores — so
// "first-name" matches the Go field FirstName. Avoids a strings-package
// dependency in the always-present runtime.
func _glispFieldEq(goName, key string) bool {
	return _glispNormField(goName) == _glispNormField(key)
}

func _glispNormField(s string) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '-' || c == '_' {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b = append(b, c)
	}
	return string(b)
}

func _glispGetD(m any, key any, def any) any {
	v := _glispGet(m, key)
	if v == nil {
		return def
	}
	return v
}

func _glispAssoc(m any, kvs ...any) map[string]any {
	result := map[string]any{}
	if mm, ok := m.(map[string]any); ok {
		for k, v := range mm {
			result[k] = v
		}
	}
	for i := 0; i+1 < len(kvs); i += 2 {
		result[kvs[i].(string)] = kvs[i+1]
	}
	return result
}

func _glispDissoc(m any, keys ...any) map[string]any {
	result := map[string]any{}
	if mm, ok := m.(map[string]any); ok {
		for k, v := range mm {
			result[k] = v
		}
	}
	for _, key := range keys {
		delete(result, key.(string))
	}
	return result
}

func _glispKeys(m any) []any {
	mm, ok := m.(map[string]any)
	if !ok {
		return nil
	}
	keys := make([]any, 0, len(mm))
	for k := range mm {
		keys = append(keys, k)
	}
	return keys
}

func _glispVals(m any) []any {
	mm, ok := m.(map[string]any)
	if !ok {
		return nil
	}
	vals := make([]any, 0, len(mm))
	for _, v := range mm {
		vals = append(vals, v)
	}
	return vals
}

func _glispMerge(maps ...any) map[string]any {
	result := map[string]any{}
	for _, m := range maps {
		if mm, ok := m.(map[string]any); ok {
			for k, v := range mm {
				result[k] = v
			}
		}
	}
	return result
}

func _glispToSlice(v any) []any {
	switch s := v.(type) {
	case []any:
		return s
	case []string:
		result := make([]any, len(s))
		for i, e := range s {
			result[i] = e
		}
		return result
	case []int:
		result := make([]any, len(s))
		for i, e := range s {
			result[i] = e
		}
		return result
	case []int64:
		result := make([]any, len(s))
		for i, e := range s {
			result[i] = e
		}
		return result
	case []float64:
		result := make([]any, len(s))
		for i, e := range s {
			result[i] = e
		}
		return result
	case []bool:
		result := make([]any, len(s))
		for i, e := range s {
			result[i] = e
		}
		return result
	case []map[string]any:
		result := make([]any, len(s))
		for i, e := range s {
			result[i] = e
		}
		return result
	case map[any]struct{}:
		// Sets enumerate in sorted order (Go map iteration is random, which
		// would make map/doseq/join over a set non-deterministic). Insertion
		// sort: no sort-package dependency in the always-present runtime.
		result := make([]any, 0, len(s))
		for k := range s {
			result = append(result, k)
		}
		for i := 1; i < len(result); i++ {
			for j := i; j > 0 && _glispKeyLess(result[j], result[j-1]); j-- {
				result[j], result[j-1] = result[j-1], result[j]
			}
		}
		return result
	}
	// Fallback: any other slice/array kind (e.g. []Book produced by a typed
	// map) via reflection, so collection helpers work on user-typed slices too.
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		result := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			result[i] = rv.Index(i).Interface()
		}
		return result
	}
	return nil
}

func _glispConj(coll any, elems ...any) any {
	if s, ok := coll.(map[any]struct{}); ok {
		result := make(map[any]struct{}, len(s)+len(elems))
		for k := range s {
			result[k] = struct{}{}
		}
		for _, e := range elems {
			result[e] = struct{}{}
		}
		return result
	}
	if coll == nil {
		return append([]any(nil), elems...)
	}
	return append(_glispToSlice(coll), elems...)
}

func _glispLen(v any) int {
	if v == nil {
		return 0
	}
	switch c := v.(type) {
	case []any:
		return len(c)
	case []string:
		return len(c)
	case []int:
		return len(c)
	case []float64:
		return len(c)
	case []map[string]any:
		return len(c)
	case map[string]any:
		return len(c)
	case map[string]string:
		return len(c)
	case map[any]struct{}:
		return len(c)
	case string:
		return len(c)
	}
	// Fallback for user-typed slices/arrays/maps via reflection.
	switch rv := reflect.ValueOf(v); rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.String:
		return rv.Len()
	}
	return 0
}

func _glispTruthy(v any) bool {
	return v != nil && v != false
}

func _glispFirst(v any) any {
	s := _glispToSlice(v)
	if len(s) == 0 {
		return nil
	}
	return s[0]
}

func _glispRest(v any) []any {
	s := _glispToSlice(v)
	if len(s) == 0 {
		return []any{}
	}
	return s[1:]
}

func _glispNth(v any, i any) any {
	return _glispToSlice(v)[_glispToInt(i)]
}

func _glispToInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		if i, err := strconv.Atoi(n); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return int(f)
		}
	}
	return 0
}

func _glispToFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case string:
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return f
		}
	}
	return 0.0
}

func _glispToString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	case int:
		return strconv.Itoa(s)
	case int64:
		return strconv.FormatInt(s, 10)
	case float64:
		return strconv.FormatFloat(s, 'g', -1, 64)
	case bool:
		if s {
			return "true"
		}
		return "false"
	}
	return ""
}

func _glispMax(args ...any) any {
	if len(args) == 0 {
		return nil
	}
	best := args[0]
	for _, a := range args[1:] {
		if _glispToFloat64(a) > _glispToFloat64(best) {
			best = a
		}
	}
	return best
}

func _glispMin(args ...any) any {
	if len(args) == 0 {
		return nil
	}
	best := args[0]
	for _, a := range args[1:] {
		if _glispToFloat64(a) < _glispToFloat64(best) {
			best = a
		}
	}
	return best
}

// _glispKeyLess orders int/int64/float64/string keys; mismatched or
// unsupported types compare as not-less (mirrors min-key/max-key).
func _glispKeyLess(a any, b any) bool {
	switch av := a.(type) {
	case int:
		if bv, ok := b.(int); ok {
			return av < bv
		}
	case int64:
		if bv, ok := b.(int64); ok {
			return av < bv
		}
	case float64:
		if bv, ok := b.(float64); ok {
			return av < bv
		}
	case string:
		if bv, ok := b.(string); ok {
			return av < bv
		}
	}
	return false
}

func _glispMinBy(f any, coll any) any {
	fn := f.(func(any) any)
	var best, bestKey any
	for i, v := range _glispToSlice(coll) {
		k := fn(v)
		if i == 0 || _glispKeyLess(k, bestKey) {
			best, bestKey = v, k
		}
	}
	return best
}

func _glispMaxBy(f any, coll any) any {
	fn := f.(func(any) any)
	var best, bestKey any
	for i, v := range _glispToSlice(coll) {
		k := fn(v)
		if i == 0 || _glispKeyLess(bestKey, k) {
			best, bestKey = v, k
		}
	}
	return best
}

func _glispMap(f any, coll any) []any {
	fn := f.(func(any) any)
	s := _glispToSlice(coll)
	result := make([]any, len(s))
	for i, v := range s {
		result[i] = fn(v)
	}
	return result
}

func _glispMapIndexed(f any, coll any) []any {
	fn := f.(func(any, any) any)
	s := _glispToSlice(coll)
	result := make([]any, len(s))
	for i, v := range s {
		result[i] = fn(int64(i), v)
	}
	return result
}

func _glispFilter(f any, coll any) []any {
	fn := f.(func(any) any)
	s := _glispToSlice(coll)
	var result []any
	for _, v := range s {
		r := fn(v)
		if r != nil && r != false {
			result = append(result, v)
		}
	}
	return result
}

func _glispReduce(f any, init any, coll any) any {
	fn := f.(func(any, any) any)
	acc := init
	for _, v := range _glispToSlice(coll) {
		acc = fn(acc, v)
	}
	return acc
}

func _glispReduce2(f any, coll any) any {
	fn := f.(func(any, any) any)
	s := _glispToSlice(coll)
	if len(s) == 0 {
		return nil
	}
	acc := s[0]
	for _, v := range s[1:] {
		acc = fn(acc, v)
	}
	return acc
}

func _glispReverse(coll any) []any {
	s := _glispToSlice(coll)
	result := make([]any, len(s))
	for i, v := range s {
		result[len(s)-1-i] = v
	}
	return result
}

func _glispContains(coll any, val any) bool {
	switch c := coll.(type) {
	case map[string]any:
		if k, ok := val.(string); ok {
			_, exists := c[k]
			return exists
		}
	case map[any]struct{}:
		_, exists := c[val]
		return exists
	case string:
		if sub, ok := val.(string); ok {
			if len(sub) == 0 {
				return true
			}
			if len(sub) > len(c) {
				return false
			}
			for i := 0; i <= len(c)-len(sub); i++ {
				if c[i:i+len(sub)] == sub {
					return true
				}
			}
		}
		return false
	}
	// Slices (including concrete []string, []int, … via _glispToSlice)
	for _, v := range _glispToSlice(coll) {
		if v == val {
			return true
		}
	}
	return false
}

func _glispSome(f any, coll any) any {
	fn := f.(func(any) any)
	for _, v := range _glispToSlice(coll) {
		if r := fn(v); r != nil && r != false {
			return r
		}
	}
	return nil
}

func _glispEvery(f any, coll any) bool {
	fn := f.(func(any) any)
	for _, v := range _glispToSlice(coll) {
		r := fn(v)
		if r == nil || r == false {
			return false
		}
	}
	return true
}

func _glispFlatten(coll any) []any {
	var result []any
	for _, v := range _glispToSlice(coll) {
		if inner := _glispToSlice(v); inner != nil {
			result = append(result, _glispFlatten(inner)...)
		} else {
			result = append(result, v)
		}
	}
	return result
}

func _glispRange(args ...any) []any {
	var start, end, step int
	switch len(args) {
	case 1:
		start, end, step = 0, _glispToInt(args[0]), 1
	case 2:
		start, end, step = _glispToInt(args[0]), _glispToInt(args[1]), 1
	case 3:
		start, end, step = _glispToInt(args[0]), _glispToInt(args[1]), _glispToInt(args[2])
	default:
		return nil
	}
	if step == 0 {
		return nil
	}
	var result []any
	for i := start; (step > 0 && i < end) || (step < 0 && i > end); i += step {
		result = append(result, i)
	}
	return result
}

func _glispTake(n any, coll any) []any {
	s := _glispToSlice(coll)
	k := _glispToInt(n)
	if k < 0 {
		k = 0
	}
	if k > len(s) {
		k = len(s)
	}
	result := make([]any, k)
	copy(result, s[:k])
	return result
}

func _glispDrop(n any, coll any) []any {
	s := _glispToSlice(coll)
	k := _glispToInt(n)
	if k < 0 {
		k = 0
	}
	if k > len(s) {
		k = len(s)
	}
	result := make([]any, len(s)-k)
	copy(result, s[k:])
	return result
}

func _glispComplement(pred any) any {
	fn := pred.(func(any) any)
	return func(x any) any {
		r := fn(x)
		return r == nil || r == false
	}
}

func _glispConstantly(v any) any {
	return func(_ any) any { return v }
}

func _glispFnil(f any, def any) any {
	fn := f.(func(any) any)
	return func(x any) any {
		if x == nil {
			x = def
		}
		return fn(x)
	}
}

func _glispApply(f any, args any) any {
	s := _glispToSlice(args)
	// A variadic function value (e.g. an operator like + passed as a value)
	// satisfies func(...any) any but none of the fixed-arity assertions below.
	if vf, ok := f.(func(...any) any); ok {
		return vf(s...)
	}
	switch len(s) {
	case 0:
		return f.(func() any)()
	case 1:
		return f.(func(any) any)(s[0])
	case 2:
		return f.(func(any, any) any)(s[0], s[1])
	case 3:
		return f.(func(any, any, any) any)(s[0], s[1], s[2])
	case 4:
		return f.(func(any, any, any, any) any)(s[0], s[1], s[2], s[3])
	case 5:
		return f.(func(any, any, any, any, any) any)(s[0], s[1], s[2], s[3], s[4])
	case 6:
		return f.(func(any, any, any, any, any, any) any)(s[0], s[1], s[2], s[3], s[4], s[5])
	default:
		return f.(func(...any) any)(s...)
	}
}

func _glispPartial(f any, fixedArgs ...any) any {
	return func(x any) any {
		allArgs := make([]any, len(fixedArgs)+1)
		copy(allArgs, fixedArgs)
		allArgs[len(fixedArgs)] = x
		return _glispApply(f, allArgs)
	}
}

func _glispComp(fns ...any) any {
	if len(fns) == 0 {
		return func(x any) any { return x }
	}
	if len(fns) == 1 {
		return fns[0]
	}
	return func(x any) any {
		v := fns[len(fns)-1].(func(any) any)(x)
		for i := len(fns) - 2; i >= 0; i-- {
			v = fns[i].(func(any) any)(v)
		}
		return v
	}
}

func _glispJuxt(fns ...any) any {
	return func(x any) any {
		result := make([]any, len(fns))
		for i, f := range fns {
			result[i] = f.(func(any) any)(x)
		}
		return result
	}
}

func _glispRepeat(n any, val any) []any {
	count := _glispToInt(n)
	result := make([]any, count)
	for i := range result {
		result[i] = val
	}
	return result
}

func _glispInterpose(sep any, coll any) []any {
	s := _glispToSlice(coll)
	if len(s) == 0 {
		return []any{}
	}
	result := make([]any, 0, len(s)*2-1)
	for i, v := range s {
		if i > 0 {
			result = append(result, sep)
		}
		result = append(result, v)
	}
	return result
}
func _glispSecond(coll any) any {
	s := _glispToSlice(coll)
	if len(s) < 2 {
		return nil
	}
	return s[1]
}

func _glispLast(coll any) any {
	s := _glispToSlice(coll)
	if len(s) == 0 {
		return nil
	}
	return s[len(s)-1]
}

func _glispIsEmpty(coll any) bool {
	if coll == nil {
		return true
	}
	switch v := coll.(type) {
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	case map[any]struct{}:
		return len(v) == 0
	case string:
		return len(v) == 0
	}
	return true
}

func _glispNotEmpty(coll any) any {
	if _glispIsEmpty(coll) {
		return nil
	}
	return coll
}

func _glispGetIn(m any, keys any) any {
	cur := m
	for _, k := range _glispToSlice(keys) {
		cur = _glispGet(cur, k)
		if cur == nil {
			return nil
		}
	}
	return cur
}

func _glispUpdate(m any, key any, f any) map[string]any {
	fn := f.(func(any) any)
	return _glispAssoc(m, key, fn(_glispGet(m, key)))
}

func _glispSelectKeys(m any, keys any) map[string]any {
	result := map[string]any{}
	mm, _ := m.(map[string]any)
	for _, k := range _glispToSlice(keys) {
		ks := k.(string)
		if v, ok := mm[ks]; ok {
			result[ks] = v
		}
	}
	return result
}

func _glispInto(target any, coll any) any {
	s := _glispToSlice(coll)
	switch t := target.(type) {
	case map[string]any:
		result := map[string]any{}
		for k, v := range t {
			result[k] = v
		}
		for _, pair := range s {
			p := _glispToSlice(pair)
			if len(p) >= 2 {
				result[p[0].(string)] = p[1]
			}
		}
		return result
	case []any:
		result := make([]any, len(t), len(t)+len(s))
		copy(result, t)
		return append(result, s...)
	case map[any]struct{}:
		result := make(map[any]struct{}, len(t)+len(s))
		for k := range t {
			result[k] = struct{}{}
		}
		for _, v := range s {
			result[v] = struct{}{}
		}
		return result
	}
	return s
}

func _glispConcat(colls ...any) []any {
	var result []any
	for _, coll := range colls {
		result = append(result, _glispToSlice(coll)...)
	}
	return result
}

func _glispMapcat(f any, coll any) []any {
	fn := f.(func(any) any)
	var result []any
	for _, item := range _glispToSlice(coll) {
		result = append(result, _glispToSlice(fn(item))...)
	}
	return result
}

func _glispTakeWhile(f any, coll any) []any {
	fn := f.(func(any) any)
	s := _glispToSlice(coll)
	for i, item := range s {
		v := fn(item)
		if v == nil || v == false {
			return s[:i]
		}
	}
	return s
}

func _glispDropWhile(f any, coll any) []any {
	fn := f.(func(any) any)
	s := _glispToSlice(coll)
	for i, item := range s {
		v := fn(item)
		if v == nil || v == false {
			return s[i:]
		}
	}
	return []any{}
}

func _glispPartition(n any, coll any) []any {
	size := _glispToInt(n)
	if size <= 0 {
		return []any{}
	}
	s := _glispToSlice(coll)
	var result []any
	for i := 0; i+size <= len(s); i += size {
		chunk := make([]any, size)
		copy(chunk, s[i:i+size])
		result = append(result, chunk)
	}
	return result
}

func _glispIsEven(n any) bool {
	return _glispToInt(n)%2 == 0
}

func _glispIsOdd(n any) bool {
	return _glispToInt(n)%2 != 0
}

func _glispIsPos(n any) bool {
	switch v := n.(type) {
	case int:
		return v > 0
	case float64:
		return v > 0
	}
	return _glispToInt(n) > 0
}

func _glispIsNeg(n any) bool {
	switch v := n.(type) {
	case int:
		return v < 0
	case float64:
		return v < 0
	}
	return _glispToInt(n) < 0
}

func _glispIsZero(n any) bool {
	switch v := n.(type) {
	case int:
		return v == 0
	case float64:
		return v == 0
	}
	return _glispToInt(n) == 0
}

func _glispInc(n any) any {
	switch v := n.(type) {
	case int:
		return v + 1
	case float64:
		return v + 1.0
	}
	return _glispToInt(n) + 1
}

func _glispDec(n any) any {
	switch v := n.(type) {
	case int:
		return v - 1
	case float64:
		return v - 1.0
	}
	return _glispToInt(n) - 1
}

// _glispStr concatenates the string form of each argument. Backs (str …) when it
// is passed as a function value — (map str coll), (apply str coll) — mirroring
// the inline (str a b) concatenation via the always-present _glispToString.
func _glispStr(args ...any) any {
	s := ""
	for _, a := range args {
		s += _glispToString(a)
	}
	return s
}

func _glispDistinct(coll any) []any {
	s := _glispToSlice(coll)
	seen := make(map[any]bool)
	var result []any
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

func _glispRemove(pred any, coll any) []any {
	fn := pred.(func(any) any)
	s := _glispToSlice(coll)
	var result []any
	for _, v := range s {
		r := fn(v)
		if r == nil || r == false {
			result = append(result, v)
		}
	}
	return result
}

func _glispKeep(f any, coll any) []any {
	fn := f.(func(any) any)
	s := _glispToSlice(coll)
	var result []any
	for _, v := range s {
		if r := fn(v); r != nil {
			result = append(result, r)
		}
	}
	return result
}

func _glispSplitAt(n any, coll any) []any {
	i := _glispToInt(n)
	s := _glispToSlice(coll)
	if i < 0 {
		i = 0
	}
	if i > len(s) {
		i = len(s)
	}
	before := make([]any, i)
	copy(before, s[:i])
	after := make([]any, len(s)-i)
	copy(after, s[i:])
	return []any{before, after}
}

func _glispSplitWith(pred any, coll any) []any {
	fn := pred.(func(any) any)
	s := _glispToSlice(coll)
	i := 0
	for i < len(s) {
		r := fn(s[i])
		if r == nil || r == false {
			break
		}
		i++
	}
	before := make([]any, i)
	copy(before, s[:i])
	after := make([]any, len(s)-i)
	copy(after, s[i:])
	return []any{before, after}
}

func _glispInterleave(colls ...any) []any {
	var slices [][]any
	for _, c := range colls {
		slices = append(slices, _glispToSlice(c))
	}
	if len(slices) == 0 {
		return nil
	}
	minLen := len(slices[0])
	for _, s := range slices[1:] {
		if len(s) < minLen {
			minLen = len(s)
		}
	}
	result := make([]any, 0, minLen*len(slices))
	for i := 0; i < minLen; i++ {
		for _, s := range slices {
			result = append(result, s[i])
		}
	}
	return result
}

func _glispNotAny(pred any, coll any) bool {
	fn := pred.(func(any) any)
	for _, v := range _glispToSlice(coll) {
		if r := fn(v); r != nil && r != false {
			return false
		}
	}
	return true
}

// --- end glisp runtime helpers ---
`

// glispSortRuntime is appended when sort-by is used (requires "sort" import).
const glispSortRuntime = `
func _glispSortBy(f any, coll any) []any {
	fn := f.(func(any) any)
	s := _glispToSlice(coll)
	result := make([]any, len(s))
	copy(result, s)
	sort.SliceStable(result, func(i, j int) bool {
		ki := fn(result[i])
		kj := fn(result[j])
		switch a := ki.(type) {
		case int:
			if b, ok := kj.(int); ok {
				return a < b
			}
		case int64:
			if b, ok := kj.(int64); ok {
				return a < b
			}
		case float64:
			if b, ok := kj.(float64); ok {
				return a < b
			}
		case string:
			if b, ok := kj.(string); ok {
				return a < b
			}
		}
		return false
	})
	return result
}

func _glispSort(coll any) []any {
	s := _glispToSlice(coll)
	result := make([]any, len(s))
	copy(result, s)
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		switch av := a.(type) {
		case int:
			if bv, ok := b.(int); ok {
				return av < bv
			}
		case float64:
			if bv, ok := b.(float64); ok {
				return av < bv
			}
		case string:
			if bv, ok := b.(string); ok {
				return av < bv
			}
		}
		return false
	})
	return result
}

func _glispMinKey(f any, args ...any) any {
	fn := f.(func(any) any)
	if len(args) == 0 {
		return nil
	}
	best := args[0]
	bestVal := fn(best)
	for _, a := range args[1:] {
		v := fn(a)
		switch bv := bestVal.(type) {
		case int:
			if av, ok := v.(int); ok && av < bv {
				best, bestVal = a, v
			}
		case float64:
			if av, ok := v.(float64); ok && av < bv {
				best, bestVal = a, v
			}
		case string:
			if av, ok := v.(string); ok && av < bv {
				best, bestVal = a, v
			}
		}
	}
	return best
}

func _glispMaxKey(f any, args ...any) any {
	fn := f.(func(any) any)
	if len(args) == 0 {
		return nil
	}
	best := args[0]
	bestVal := fn(best)
	for _, a := range args[1:] {
		v := fn(a)
		switch bv := bestVal.(type) {
		case int:
			if av, ok := v.(int); ok && av > bv {
				best, bestVal = a, v
			}
		case float64:
			if av, ok := v.(float64); ok && av > bv {
				best, bestVal = a, v
			}
		case string:
			if av, ok := v.(string); ok && av > bv {
				best, bestVal = a, v
			}
		}
	}
	return best
}
`

// glispStrRuntime is appended when split/join are used (requires "strings" import).
const glispStrRuntime = `
func _glispSplit(s any, sep any) []any {
	parts := strings.Split(s.(string), sep.(string))
	result := make([]any, len(parts))
	for i, p := range parts {
		result[i] = p
	}
	return result
}

func _glispJoin(coll any, sep any) string {
	s := _glispToSlice(coll)
	parts := make([]string, len(s))
	for i, v := range s {
		if str, ok := v.(string); ok {
			parts[i] = str
		} else {
			parts[i] = fmt.Sprintf("%v", v)
		}
	}
	return strings.Join(parts, fmt.Sprintf("%v", sep))
}

func _glispIsBlank(s any) bool {
	if s == nil {
		return true
	}
	return strings.TrimSpace(s.(string)) == ""
}

func _glispCapitalize(s any) string {
	str := s.(string)
	if str == "" {
		return str
	}
	r := []rune(str)
	return strings.ToUpper(string(r[0:1])) + strings.ToLower(string(r[1:]))
}
`

// glispJsonRuntime is appended when json/encode or json/decode are used (requires "encoding/json" import).
const glispJsonRuntime = `
func _glispJsonEncode(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func _glispJsonDecode(s any) (any, error) {
	var result any
	err := json.Unmarshal([]byte(fmt.Sprintf("%v", s)), &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
`

// glispHttpRuntime is appended when http/* built-ins are used (requires "net/http", "io", "strings" imports).
const glispHttpRuntime = `
func _glispHttpDo(method, url, body, headers any) (map[string]any, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = strings.NewReader(fmt.Sprintf("%v", body))
	}
	req, err := http.NewRequest(fmt.Sprintf("%v", method), fmt.Sprintf("%v", url), reqBody)
	if err != nil {
		return nil, err
	}
	if headers != nil {
		if hdrs, ok := headers.(map[string]any); ok {
			for k, v := range hdrs {
				req.Header.Set(k, fmt.Sprintf("%v", v))
			}
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	respHeaders := make(map[string]any)
	for k := range resp.Header {
		respHeaders[k] = resp.Header.Get(k)
	}
	return map[string]any{
		"status":  resp.StatusCode,
		"headers": respHeaders,
		"body":    string(bodyBytes),
	}, nil
}

func _glispHttpGet(url any) (map[string]any, error) {
	return _glispHttpDo("GET", url, nil, nil)
}

func _glispHttpGetH(url, headers any) (map[string]any, error) {
	return _glispHttpDo("GET", url, nil, headers)
}

func _glispHttpPost(url, body any) (map[string]any, error) {
	return _glispHttpDo("POST", url, body, nil)
}

func _glispHttpPostH(url, body, headers any) (map[string]any, error) {
	return _glispHttpDo("POST", url, body, headers)
}

func _glispHttpPut(url, body any) (map[string]any, error) {
	return _glispHttpDo("PUT", url, body, nil)
}

func _glispHttpPutH(url, body, headers any) (map[string]any, error) {
	return _glispHttpDo("PUT", url, body, headers)
}

func _glispHttpDelete(url any) (map[string]any, error) {
	return _glispHttpDo("DELETE", url, nil, nil)
}

func _glispHttpRequest(opts any) (map[string]any, error) {
	m, ok := opts.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("http/request: opts must be a map")
	}
	method := "GET"
	if v, ok := m["method"]; ok && v != nil {
		method = fmt.Sprintf("%v", v)
	}
	url := ""
	if v, ok := m["url"]; ok && v != nil {
		url = fmt.Sprintf("%v", v)
	}
	var body any
	if v, ok := m["body"]; ok {
		body = v
	}
	var headers any
	if v, ok := m["headers"]; ok {
		headers = v
	}
	return _glispHttpDo(method, url, body, headers)
}
`

// glispFileRuntime is appended when file I/O built-ins are used (requires "os", "fmt" imports).
const glispFileRuntime = `
func _glispReadFile(path any) (string, error) {
	b, err := os.ReadFile(fmt.Sprintf("%v", path))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func _glispWriteFile(path, content any) error {
	return os.WriteFile(fmt.Sprintf("%v", path), []byte(fmt.Sprintf("%v", content)), 0644)
}

func _glispAppendFile(path, content any) error {
	f, err := os.OpenFile(fmt.Sprintf("%v", path), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(fmt.Sprintf("%v", content))
	return err
}

func _glispFileExists(path any) bool {
	_, err := os.Stat(fmt.Sprintf("%v", path))
	return !os.IsNotExist(err)
}

func _glispListDir(path any) ([]any, error) {
	entries, err := os.ReadDir(fmt.Sprintf("%v", path))
	if err != nil {
		return nil, err
	}
	result := make([]any, len(entries))
	for i, entry := range entries {
		result[i] = entry.Name()
	}
	return result, nil
}

func _glispMkdir(path any) error {
	return os.MkdirAll(fmt.Sprintf("%v", path), 0755)
}
`

// glispReRuntime is appended when re/* built-ins are used (requires "regexp", "fmt" imports).
const glispReRuntime = `
func _glispReMatch(pattern, s any) bool {
	matched, err := regexp.MatchString(fmt.Sprintf("%v", pattern), fmt.Sprintf("%v", s))
	if err != nil {
		panic(err)
	}
	return matched
}

func _glispReFind(pattern, s any) any {
	str := fmt.Sprintf("%v", s)
	re := regexp.MustCompile(fmt.Sprintf("%v", pattern))
	loc := re.FindStringIndex(str)
	if loc == nil {
		return nil
	}
	return str[loc[0]:loc[1]]
}

func _glispReFindAll(pattern, s any) []any {
	re := regexp.MustCompile(fmt.Sprintf("%v", pattern))
	matches := re.FindAllString(fmt.Sprintf("%v", s), -1)
	result := make([]any, len(matches))
	for i, m := range matches {
		result[i] = m
	}
	return result
}

func _glispReReplace(pattern, s, repl any) string {
	re := regexp.MustCompile(fmt.Sprintf("%v", pattern))
	return re.ReplaceAllString(fmt.Sprintf("%v", s), fmt.Sprintf("%v", repl))
}

func _glispReSplit(pattern, s any) []any {
	re := regexp.MustCompile(fmt.Sprintf("%v", pattern))
	parts := re.Split(fmt.Sprintf("%v", s), -1)
	result := make([]any, len(parts))
	for i, p := range parts {
		result[i] = p
	}
	return result
}
`

// glispEnvRuntime is appended when os/env is used (requires "os" import).
const glispEnvRuntime = `
func _glispEnv(name any) string {
	return os.Getenv(fmt.Sprintf("%v", name))
}

func _glispEnvDefault(name any, fallback any) string {
	if val, ok := os.LookupEnv(fmt.Sprintf("%v", name)); ok {
		return val
	}
	return fmt.Sprintf("%v", fallback)
}
`

// glispDataRuntime is appended when data-transformation built-ins are used (requires "fmt" import).
const glispDataRuntime = `
func _glispAssocInSlice(m any, ks []any, v any) map[string]any {
	result := map[string]any{}
	if mm, ok := m.(map[string]any); ok {
		for k, val := range mm {
			result[k] = val
		}
	}
	if len(ks) == 0 {
		return result
	}
	key := ks[0].(string)
	if len(ks) == 1 {
		result[key] = v
		return result
	}
	result[key] = _glispAssocInSlice(result[key], ks[1:], v)
	return result
}

func _glispAssocIn(m any, keys any, v any) map[string]any {
	return _glispAssocInSlice(m, _glispToSlice(keys), v)
}

func _glispUpdateIn(m any, keys any, f any) map[string]any {
	fn := f.(func(any) any)
	ks := _glispToSlice(keys)
	return _glispAssocInSlice(m, ks, fn(_glispGetIn(m, keys)))
}

func _glispRenameKeys(m any, kmap any) map[string]any {
	result := map[string]any{}
	km, _ := kmap.(map[string]any)
	if mm, ok := m.(map[string]any); ok {
		for k, v := range mm {
			if nk, found := km[k]; found {
				result[fmt.Sprintf("%v", nk)] = v
			} else {
				result[k] = v
			}
		}
	}
	return result
}

func _glispGroupBy(f any, coll any) map[string]any {
	result := map[string]any{}
	for _, item := range _glispToSlice(coll) {
		var key string
		switch fn := f.(type) {
		case func(any) any:
			key = fmt.Sprintf("%v", fn(item))
		case string:
			key = fmt.Sprintf("%v", _glispGet(item, fn))
		}
		existing, _ := result[key].([]any)
		result[key] = append(existing, item)
	}
	return result
}

func _glispFrequencies(coll any) map[string]any {
	result := map[string]any{}
	for _, item := range _glispToSlice(coll) {
		key := fmt.Sprintf("%v", item)
		count, _ := result[key].(int64)
		result[key] = count + 1
	}
	return result
}

func _glispZipmap(keys any, vals any) map[string]any {
	ks := _glispToSlice(keys)
	vs := _glispToSlice(vals)
	result := map[string]any{}
	for i, k := range ks {
		if i >= len(vs) {
			break
		}
		result[fmt.Sprintf("%v", k)] = vs[i]
	}
	return result
}

func _glispPartitionBy(f any, coll any) []any {
	fn := f.(func(any) any)
	s := _glispToSlice(coll)
	if len(s) == 0 {
		return []any{}
	}
	var result []any
	var chunk []any
	prev := fmt.Sprintf("%v", fn(s[0]))
	for _, item := range s {
		cur := fmt.Sprintf("%v", fn(item))
		if cur != prev {
			result = append(result, chunk)
			chunk = nil
			prev = cur
		}
		chunk = append(chunk, item)
	}
	if len(chunk) > 0 {
		result = append(result, chunk)
	}
	return result
}

func _glispMapVals(f any, m any) map[string]any {
	fn := f.(func(any) any)
	result := make(map[string]any)
	if mm, ok := m.(map[string]any); ok {
		for k, v := range mm {
			result[k] = fn(v)
		}
	}
	return result
}

func _glispMapKeys(f any, m any) map[string]any {
	fn := f.(func(any) any)
	result := make(map[string]any)
	if mm, ok := m.(map[string]any); ok {
		for k, v := range mm {
			if nk, ok := fn(k).(string); ok {
				result[nk] = v
			}
		}
	}
	return result
}

func _glispReduceKV(f any, init any, m any) any {
	fn := f.(func(any, any, any) any)
	acc := init
	if mm, ok := m.(map[string]any); ok {
		for k, v := range mm {
			acc = fn(acc, k, v)
		}
	}
	return acc
}
`

// glispNumRuntime backs numeric auto-coercion (pseudo-key "_num", no real
// imports). Arithmetic/comparison on `any`-typed values routes here instead of
// native Go operators, which don't type-check on interfaces.
const glispNumRuntime = `
// _glispNumVal extracts a numeric value from a as both int64 and float64,
// reporting whether it was a floating-point value. Non-numbers contribute 0.
func _glispNumVal(a any) (int64, float64, bool) {
	switch v := a.(type) {
	case int:
		return int64(v), float64(v), false
	case int8:
		return int64(v), float64(v), false
	case int16:
		return int64(v), float64(v), false
	case int32:
		return int64(v), float64(v), false
	case int64:
		return v, float64(v), false
	case uint:
		return int64(v), float64(v), false
	case uint8:
		return int64(v), float64(v), false
	case uint16:
		return int64(v), float64(v), false
	case uint32:
		return int64(v), float64(v), false
	case uint64:
		return int64(v), float64(v), false
	case float32:
		return 0, float64(v), true
	case float64:
		return 0, v, true
	}
	return 0, 0, false
}

func _glispAdd(args ...any) any {
	var isum int64
	var fsum float64
	anyFloat := false
	for _, a := range args {
		i, f, isF := _glispNumVal(a)
		isum += i
		fsum += f
		if isF {
			anyFloat = true
		}
	}
	if anyFloat {
		return fsum
	}
	return isum
}

func _glispSub(args ...any) any {
	if len(args) == 0 {
		return int64(0)
	}
	isum, fsum, anyFloat := _glispNumVal(args[0])
	for _, a := range args[1:] {
		i, f, isF := _glispNumVal(a)
		isum -= i
		fsum -= f
		if isF {
			anyFloat = true
		}
	}
	if anyFloat {
		return fsum
	}
	return isum
}

func _glispMul(args ...any) any {
	iprod := int64(1)
	fprod := 1.0
	anyFloat := false
	for _, a := range args {
		i, f, isF := _glispNumVal(a)
		iprod *= i
		fprod *= f
		if isF {
			anyFloat = true
		}
	}
	if anyFloat {
		return fprod
	}
	return iprod
}

func _glispDiv(args ...any) any {
	if len(args) == 0 {
		return int64(0)
	}
	anyFloat := false
	for _, a := range args {
		if _, _, isF := _glispNumVal(a); isF {
			anyFloat = true
		}
	}
	if anyFloat {
		_, res, _ := _glispNumVal(args[0])
		for _, a := range args[1:] {
			_, f, _ := _glispNumVal(a)
			res /= f
		}
		return res
	}
	res, _, _ := _glispNumVal(args[0])
	for _, a := range args[1:] {
		i, _, _ := _glispNumVal(a)
		res /= i
	}
	return res
}

func _glispMod(a, b any) any {
	ai, _, _ := _glispNumVal(a)
	bi, _, _ := _glispNumVal(b)
	return ai % bi
}

func _glispNumCmp(a, b any) int {
	_, af, _ := _glispNumVal(a)
	_, bf, _ := _glispNumVal(b)
	switch {
	case af < bf:
		return -1
	case af > bf:
		return 1
	default:
		return 0
	}
}

func _glispLt(a, b any) bool { return _glispNumCmp(a, b) < 0 }
func _glispGt(a, b any) bool { return _glispNumCmp(a, b) > 0 }
func _glispLe(a, b any) bool { return _glispNumCmp(a, b) <= 0 }
func _glispGe(a, b any) bool { return _glispNumCmp(a, b) >= 0 }

// _glispEqNum reports whether v holds a Go numeric value, returning it as both
// int64 and float64 plus whether it is a floating-point (vs integral) type.
// Unlike _glispNumVal it distinguishes non-numbers (isNum=false) from a zero
// value, which _glispEquals needs to decide between numeric and value equality.
func _glispEqNum(v any) (i int64, f float64, isFloat bool, isNum bool) {
	switch n := v.(type) {
	case int:
		return int64(n), float64(n), false, true
	case int8:
		return int64(n), float64(n), false, true
	case int16:
		return int64(n), float64(n), false, true
	case int32:
		return int64(n), float64(n), false, true
	case int64:
		return n, float64(n), false, true
	case uint:
		return int64(n), float64(n), false, true
	case uint8:
		return int64(n), float64(n), false, true
	case uint16:
		return int64(n), float64(n), false, true
	case uint32:
		return int64(n), float64(n), false, true
	case uint64:
		return int64(n), float64(n), false, true
	case float32:
		return 0, float64(n), true, true
	case float64:
		return 0, n, true, true
	}
	return 0, 0, false, false
}

// _glispEquals is the = / not= comparison over any-typed values. Two numbers
// compare by value regardless of their dynamic Go type — integral pairs exactly
// as int64 (closing the int64(1) == int(1) footgun, where boxed arithmetic
// results never matched int literals), a pair involving a float as float64
// (so (= 1 1.0) is true, consistent with the ordering operators). Anything else
// uses reflect.DeepEqual, which gives value equality for strings/bools and for
// slices/maps without the runtime panic native == raises on uncomparable types.
func _glispEquals(a, b any) bool {
	ai, af, aFloat, aNum := _glispEqNum(a)
	bi, bf, bFloat, bNum := _glispEqNum(b)
	if aNum && bNum {
		if aFloat || bFloat {
			return af == bf
		}
		return ai == bi
	}
	return reflect.DeepEqual(a, b)
}
`

const glispSetRuntime = `
func _glispToSet(coll any) map[any]struct{} {
	result := make(map[any]struct{})
	if s, ok := coll.(map[any]struct{}); ok {
		for k := range s {
			result[k] = struct{}{}
		}
		return result
	}
	for _, v := range _glispToSlice(coll) {
		result[v] = struct{}{}
	}
	return result
}

func _glispSetUnion(a any, b any) map[any]struct{} {
	result := make(map[any]struct{})
	if s, ok := a.(map[any]struct{}); ok {
		for k := range s {
			result[k] = struct{}{}
		}
	}
	if s, ok := b.(map[any]struct{}); ok {
		for k := range s {
			result[k] = struct{}{}
		}
	}
	return result
}

func _glispSetIntersection(a any, b any) map[any]struct{} {
	result := make(map[any]struct{})
	as, aok := a.(map[any]struct{})
	bs, bok := b.(map[any]struct{})
	if !aok || !bok {
		return result
	}
	for k := range as {
		if _, exists := bs[k]; exists {
			result[k] = struct{}{}
		}
	}
	return result
}

func _glispSetDifference(a any, b any) map[any]struct{} {
	result := make(map[any]struct{})
	as, aok := a.(map[any]struct{})
	bs, bok := b.(map[any]struct{})
	if !aok {
		return result
	}
	for k := range as {
		if bok {
			if _, exists := bs[k]; exists {
				continue
			}
		}
		result[k] = struct{}{}
	}
	return result
}
`

// glispPPRuntime backs (pp v): pretty-print a value with indentation and return
// it unchanged (so it threads through tap->/tap->>). Uses only fmt + strconv,
// both always available, so it needs no conditional package import beyond fmt.
const glispPPRuntime = `
func _glispPP(v any) any {
	fmt.Println(_glispPPString(v, ""))
	return v
}

func _glispPPString(v any, indent string) string {
	inner := indent + "  "
	switch x := v.(type) {
	case map[string]any:
		if len(x) == 0 {
			return "{}"
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		for i := 1; i < len(keys); i++ {
			for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
				keys[j], keys[j-1] = keys[j-1], keys[j]
			}
		}
		s := "{\n"
		for _, k := range keys {
			s += inner + strconv.Quote(k) + " " + _glispPPString(x[k], inner) + "\n"
		}
		return s + indent + "}"
	case []any:
		if len(x) == 0 {
			return "[]"
		}
		s := "[\n"
		for _, el := range x {
			s += inner + _glispPPString(el, inner) + "\n"
		}
		return s + indent + "]"
	case string:
		return strconv.Quote(x)
	default:
		return fmt.Sprintf("%v", v)
	}
}
`

const glispAtomRuntime = `
type _glispAtom struct {
	mu  sync.Mutex
	val any
}

func _glispAtomSwap(a any, f any) any {
	atom := a.(*_glispAtom)
	fn := f.(func(any) any)
	atom.mu.Lock()
	defer atom.mu.Unlock()
	atom.val = fn(atom.val)
	return atom.val
}

func _glispAtomReset(a any, v any) any {
	atom := a.(*_glispAtom)
	atom.mu.Lock()
	defer atom.mu.Unlock()
	atom.val = v
	return v
}

func _glispAtomDeref(a any) any {
	atom := a.(*_glispAtom)
	atom.mu.Lock()
	defer atom.mu.Unlock()
	return atom.val
}
`

const glispCloseRuntime = `
// _glispClose closes v if it implements Close() error (io.Closer); any other
// value is ignored. Backs (with-open …)'s deferred cleanup without requiring the
// bound resource to be statically typed.
func _glispClose(v any) {
	if c, ok := v.(interface{ Close() error }); ok {
		_ = c.Close()
	}
}
`

const glispCtxRuntime = `
func _glispCtxWithCancel(parent any) []any {
	ctx, cancel := context.WithCancel(parent.(context.Context))
	return []any{ctx, cancel}
}

func _glispCtxWithTimeout(parent any, ms any) []any {
	var dur time.Duration
	switch v := ms.(type) {
	case int64:
		dur = time.Duration(v) * time.Millisecond
	case int:
		dur = time.Duration(v) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(parent.(context.Context), dur)
	return []any{ctx, cancel}
}

func _glispCtxCancel(cancel any) any {
	cancel.(context.CancelFunc)()
	return nil
}

func _glispCtxValue(ctx any, key any) any {
	return ctx.(context.Context).Value(key)
}

func _glispCtxWithValue(ctx any, key any, val any) any {
	return context.WithValue(ctx.(context.Context), key, val)
}

func _glispCtxDone(ctx any) bool {
	return ctx.(context.Context).Err() != nil
}

func _glispCtxErr(ctx any) error {
	return ctx.(context.Context).Err()
}
`

// glispProcRuntime is appended when proc/* built-ins are used (real imports:
// bytes, os/exec). Each helper runs a command, capturing stdout/stderr and the
// exit code into a map {"out" "err" "exit" "ok"} so glisp gets plain data — the
// unsafe *exec.ExitError type assertion stays in Go.
const glispProcRuntime = `
// --- proc/* subprocess helpers (generated) ---

func _glispExec(cmd *exec.Cmd) map[string]any {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exit := int64(0)
	ok := true
	if err != nil {
		ok = false
		if ee, isExit := err.(*exec.ExitError); isExit {
			exit = int64(ee.ExitCode())
		} else {
			exit = int64(-1)
			if stderr.Len() == 0 {
				stderr.WriteString(err.Error())
			}
		}
	}
	return map[string]any{
		"out":  stdout.String(),
		"err":  stderr.String(),
		"exit": exit,
		"ok":   ok,
	}
}

func _glispProcRun(args ...any) map[string]any {
	if len(args) == 0 {
		return map[string]any{"out": "", "err": "proc/run: no command", "exit": int64(-1), "ok": false}
	}
	name := _glispToString(args[0])
	rest := make([]string, 0, len(args)-1)
	for _, a := range args[1:] {
		rest = append(rest, _glispToString(a))
	}
	return _glispExec(exec.Command(name, rest...))
}

func _glispProcSh(cmd any) map[string]any {
	return _glispExec(exec.Command("sh", "-c", _glispToString(cmd)))
}
`

// glispPathRuntime is appended when path/* or glob are used (real import:
// path/filepath). Path helpers front filepath, so joins/splits respect the OS
// separator. _glispGlob returns matches as []any (empty on a bad pattern).
const glispPathRuntime = `
// --- path/* and glob helpers (generated) ---

func _glispPathJoin(parts ...any) string {
	s := make([]string, len(parts))
	for i, p := range parts {
		s[i] = _glispToString(p)
	}
	return filepath.Join(s...)
}

func _glispPathDir(p any) string   { return filepath.Dir(_glispToString(p)) }
func _glispPathBase(p any) string  { return filepath.Base(_glispToString(p)) }
func _glispPathExt(p any) string   { return filepath.Ext(_glispToString(p)) }
func _glispPathClean(p any) string { return filepath.Clean(_glispToString(p)) }

func _glispGlob(pattern any) []any {
	m, err := filepath.Glob(_glispToString(pattern))
	if err != nil {
		return []any{}
	}
	out := make([]any, len(m))
	for i, p := range m {
		out[i] = p
	}
	return out
}
`

// glispWalkRuntime is appended when walk is used (real imports: path/filepath,
// io/fs). Returns every regular file under root, recursively, as []any.
const glispWalkRuntime = `
// --- walk helper (generated) ---

func _glispWalk(root any) []any {
	out := []any{}
	filepath.WalkDir(_glispToString(root), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	return out
}
`

// glispCsvRuntime is appended when csv/parse or csv/write are used (real imports:
// encoding/csv, strings, sort). Rows are header-mapped: parse turns the first
// record into keys, write takes the header from the first row's keys (sorted for
// determinism). Both return (value, error) for use with if-err.
const glispCsvRuntime = `
// --- csv/* helpers (generated) ---

func _glispCsvParse(text any) ([]any, error) {
	records, err := csv.NewReader(strings.NewReader(_glispToString(text))).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return []any{}, nil
	}
	header := records[0]
	rows := make([]any, 0, len(records)-1)
	for _, rec := range records[1:] {
		m := make(map[string]any, len(header))
		for i, h := range header {
			if i < len(rec) {
				m[h] = rec[i]
			} else {
				m[h] = ""
			}
		}
		rows = append(rows, m)
	}
	return rows, nil
}

func _glispCsvWrite(rows any) (string, error) {
	s := _glispToSlice(rows)
	var buf strings.Builder
	w := csv.NewWriter(&buf)
	if len(s) > 0 {
		if first, ok := s[0].(map[string]any); ok {
			header := make([]string, 0, len(first))
			for k := range first {
				header = append(header, k)
			}
			sort.Strings(header)
			w.Write(header)
			for _, row := range s {
				m, _ := row.(map[string]any)
				rec := make([]string, len(header))
				for i, h := range header {
					rec[i] = _glispToString(m[h])
				}
				w.Write(rec)
			}
		}
	}
	w.Flush()
	return buf.String(), w.Error()
}
`

// glispXfRuntime is appended when transducers are used (map/filter/take/… with a
// single argument, transduce, sequence, into-with-xform). A transducer is a
// unary func(rf) rf (rf is a reducing func(acc, x) any), so the existing comp
// composes them. _glispReduced signals early termination (take/take-while). No
// real imports — only the always-present _glispToSlice/_glispToInt/_glispTruthy/
// _glispConj helpers.
const glispXfRuntime = `
// --- transducers (generated) ---

type _glispReduced struct{ val any }

func _glispEnsureReduced(x any) any {
	if _, ok := x.(_glispReduced); ok {
		return x
	}
	return _glispReduced{x}
}

func _glispRF(rf any) func(any, any) any { return rf.(func(any, any) any) }

func _glispMapXf(f any) any {
	ff := f.(func(any) any)
	return func(rf any) any {
		step := _glispRF(rf)
		return func(acc any, x any) any { return step(acc, ff(x)) }
	}
}

func _glispFilterXf(pred any) any {
	pf := pred.(func(any) any)
	return func(rf any) any {
		step := _glispRF(rf)
		return func(acc any, x any) any {
			if _glispTruthy(pf(x)) {
				return step(acc, x)
			}
			return acc
		}
	}
}

func _glispRemoveXf(pred any) any {
	pf := pred.(func(any) any)
	return func(rf any) any {
		step := _glispRF(rf)
		return func(acc any, x any) any {
			if _glispTruthy(pf(x)) {
				return acc
			}
			return step(acc, x)
		}
	}
}

func _glispKeepXf(f any) any {
	ff := f.(func(any) any)
	return func(rf any) any {
		step := _glispRF(rf)
		return func(acc any, x any) any {
			if v := ff(x); v != nil {
				return step(acc, v)
			}
			return acc
		}
	}
}

func _glispTakeXf(n any) any {
	limit := _glispToInt(n)
	return func(rf any) any {
		step := _glispRF(rf)
		taken := 0
		return func(acc any, x any) any {
			if taken >= limit {
				return _glispEnsureReduced(acc)
			}
			taken++
			acc = step(acc, x)
			if taken >= limit {
				return _glispEnsureReduced(acc)
			}
			return acc
		}
	}
}

func _glispDropXf(n any) any {
	limit := _glispToInt(n)
	return func(rf any) any {
		step := _glispRF(rf)
		seen := 0
		return func(acc any, x any) any {
			if seen < limit {
				seen++
				return acc
			}
			return step(acc, x)
		}
	}
}

func _glispTakeWhileXf(pred any) any {
	pf := pred.(func(any) any)
	return func(rf any) any {
		step := _glispRF(rf)
		return func(acc any, x any) any {
			if _glispTruthy(pf(x)) {
				return step(acc, x)
			}
			return _glispEnsureReduced(acc)
		}
	}
}

func _glispDropWhileXf(pred any) any {
	pf := pred.(func(any) any)
	return func(rf any) any {
		step := _glispRF(rf)
		dropping := true
		return func(acc any, x any) any {
			if dropping && _glispTruthy(pf(x)) {
				return acc
			}
			dropping = false
			return step(acc, x)
		}
	}
}

func _glispXfReduce(xform, rf, init, coll any) any {
	step := xform.(func(any) any)(rf).(func(any, any) any)
	acc := init
	for _, x := range _glispToSlice(coll) {
		acc = step(acc, x)
		if r, ok := acc.(_glispReduced); ok {
			return r.val
		}
	}
	return acc
}

func _glispTransduce(xform, rf, init, coll any) any {
	return _glispXfReduce(xform, rf, init, coll)
}

func _glispSequence(xform, coll any) []any {
	rf := func(acc any, x any) any { return append(acc.([]any), x) }
	return _glispXfReduce(xform, any(rf), any([]any{}), coll).([]any)
}

func _glispIntoXf(to, xform, coll any) any {
	rf := func(acc any, x any) any { return _glispConj(acc, x) }
	return _glispXfReduce(xform, any(rf), to, coll)
}
`

// glispLineRuntime is appended when read-lines or transduce-lines are used (real
// imports: bufio, os). transduce-lines streams a file's lines through a
// transducer pipeline in constant memory, honoring _glispReduced (so (take n)
// stops reading early). It references _glispReduced from glispXfRuntime, so the
// dispatch pulls "_xf" alongside "_lines".
const glispLineRuntime = `
// --- line-oriented IO (generated) ---

func _glispReadLines(path any) ([]any, error) {
	f, err := os.Open(_glispToString(path))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := []any{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out, sc.Err()
}

func _glispTransduceLines(xform, rf, init, path any) (any, error) {
	f, err := os.Open(_glispToString(path))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	step := xform.(func(any) any)(rf).(func(any, any) any)
	acc := init
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		acc = step(acc, sc.Text())
		if r, ok := acc.(_glispReduced); ok {
			return r.val, nil
		}
	}
	return acc, sc.Err()
}
`

// glispJsonStreamRuntime is appended when transduce-json is used (real import:
// os; encoding/json comes from the encoding/json key). It streams a top-level
// JSON array's elements one at a time through a transducer pipeline via
// json.Decoder — constant memory, honoring _glispReduced for early termination.
const glispJsonStreamRuntime = `
// --- streaming JSON (generated) ---

func _glispTransduceJson(xform, rf, init, path any) (any, error) {
	f, err := os.Open(_glispToString(path))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return nil, fmt.Errorf("transduce-json: expected a top-level JSON array")
	}
	step := xform.(func(any) any)(rf).(func(any, any) any)
	acc := init
	for dec.More() {
		var v any
		if err := dec.Decode(&v); err != nil {
			return acc, err
		}
		acc = step(acc, v)
		if r, ok := acc.(_glispReduced); ok {
			return r.val, nil
		}
	}
	return acc, nil
}
`
