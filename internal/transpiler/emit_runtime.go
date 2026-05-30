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
	if builtins["sort"] {
		addImport("sort")
	}
	if builtins["strings"] || builtins["net/http"] {
		addImport("strings")
	}
	if builtins["encoding/json"] {
		addImport("encoding/json")
		addImport("fmt")
	}
	if builtins["net/http"] {
		addImport("fmt")
		addImport("io")
		addImport("net/http")
	}

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
	if builtins["strings"] {
		s += glispStrRuntime
	}
	if builtins["encoding/json"] {
		s += glispJsonRuntime
	}
	if builtins["net/http"] {
		s += glispHttpRuntime
	}
	return s
}

// glispRuntime is emitted at the top of every generated Go file.
// It provides the runtime helpers used by built-in functions.
const glispRuntime = `
// --- glisp runtime helpers (generated) ---

func _glispGet(m any, key any) any {
	switch mm := m.(type) {
	case map[string]any:
		switch k := key.(type) {
		case string:
			return mm[k]
		}
	case []any:
		switch k := key.(type) {
		case int:
			return mm[k]
		case int64:
			return mm[int(k)]
		}
	}
	return nil
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
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

func _glispToInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func _glispToString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
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
	case []any:
		for _, v := range c {
			if v == val {
				return true
			}
		}
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
		if inner, ok := v.([]any); ok {
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

func _glispApply(f any, args any) any {
	s := _glispToSlice(args)
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
		}
	}
	return strings.Join(parts, sep.(string))
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
		"status":  int64(resp.StatusCode),
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
