package main

import (
	"fmt"
	"golisp/stdlib"
	"log"
)

func homeHandler(req map[string]any) map[string]any {
	return map[string]any{"status": 200, "headers": map[string]any{"Content-Type": "text/html"}, "body": "<h1>Hello from glisp!</h1>"}
}

func echoHandler(req map[string]any) map[string]any {
	body := _glispGet(req, "body")
	return map[string]any{"status": 200, "headers": map[string]any{"Content-Type": "text/plain"}, "body": (fmt.Sprintf("%v", "echo: ") + fmt.Sprintf("%v", body))}
}

func router(req map[string]any) map[string]any {
	path := _glispGet(req, "path")
	if path == "/" {
		return homeHandler(req)
	} else if path == "/echo" {
		return echoHandler(req)
	} else {
		return map[string]any{"status": 404, "body": "not found"}
	}
}

func main() {
	fmt.Println("Starting server on :3000")
	err := stdlib.Serve(":3000", router)
	log.Fatal(err)
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

// --- end glisp runtime helpers ---
