package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func fetch(url string) any {
	resp, err := _glispHttpGet(url)
	if err != nil {
		fmt.Println("GET failed:", err.Error())
		return nil
	}
	body, err2 := _glispJsonDecode(_glispGet(resp, "body"))
	if err2 != nil {
		fmt.Println("decode failed:", err2.Error())
		return nil
	}
	return body
}

func postJson(url string, payload map[string]any) any {
	encoded, err := _glispJsonEncode(payload)
	if err != nil {
		fmt.Println("encode failed:", err.Error())
		return nil
	}
	resp, err2 := _glispHttpPostH(url, encoded, map[string]any{"Content-Type": "application/json"})
	if err2 != nil {
		fmt.Println("POST failed:", err2.Error())
		return nil
	}
	body, err3 := _glispJsonDecode(_glispGet(resp, "body"))
	if err3 != nil {
		fmt.Println("decode failed:", err3.Error())
		return nil
	}
	return body
}

func main() {
	fmt.Println("=== GET /get ===")
	getBody := fetch("https://httpbin.org/get")
	if getBody != nil {
		fmt.Println("url:", _glispGet(getBody, "url"))
	}
	fmt.Println("\n=== GET /get?foo=bar ===")
	qbody := fetch("https://httpbin.org/get?foo=bar")
	if qbody != nil {
		fmt.Println("foo=", _glispGet(_glispGet(qbody, "args"), "foo"))
	}
	fmt.Println("\n=== GET /bearer ===")
	func() any {
		bresp, berr := _glispHttpGetH("https://httpbin.org/bearer", map[string]any{"Authorization": "Bearer secret-token"})
		if berr != nil {
			fmt.Println("failed:", berr.Error())
			return nil
		}
		fmt.Println("status:", _glispGet(bresp, "status"))
		return nil
	}()
	fmt.Println("\n=== POST /post ===")
	postBody := postJson("https://httpbin.org/post", map[string]any{"name": "alice", "score": 42})
	if postBody != nil {
		echoed := _glispGet(postBody, "json")
		fmt.Println("echoed name: ", _glispGet(echoed, "name"))
		fmt.Println("echoed score:", _glispGet(echoed, "score"))
	}
	fmt.Println("\n=== PUT /put ===")
	func() any {
		presp, perr := _glispHttpPutH("https://httpbin.org/put", "{\"updated\":true}", map[string]any{"Content-Type": "application/json"})
		if perr != nil {
			fmt.Println("PUT failed:", perr.Error())
			return nil
		}
		fmt.Println("status:", _glispGet(presp, "status"))
		return nil
	}()
	fmt.Println("\n=== DELETE /delete ===")
	func() any {
		dresp, derr := _glispHttpRequest(map[string]any{"method": "DELETE", "url": "https://httpbin.org/delete"})
		if derr != nil {
			fmt.Println("DELETE failed:", derr.Error())
			return nil
		}
		fmt.Println("status:", _glispGet(dresp, "status"))
		return nil
	}()
}

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

// --- end glisp runtime helpers ---

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
