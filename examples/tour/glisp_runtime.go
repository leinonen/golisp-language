package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

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
