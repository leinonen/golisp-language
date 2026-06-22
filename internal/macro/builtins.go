package macro

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// gensymCounter backs the gensym built-in. Atomic so concurrent dir builds each
// get unique names even if the evaluator is ever shared.
var gensymCounter int64

// builtins returns the primitive functions available inside a macro body. The
// set is a curated macro-authoring toolkit (forms-as-data manipulation plus the
// arithmetic/predicates needed to compute with them), not a general runtime; it
// grows on demand.
func builtins() map[string]func([]Value) (Value, error) {
	m := map[string]func([]Value) (Value, error){
		// arithmetic
		"+":   biAdd,
		"-":   biSub,
		"*":   biMul,
		"/":   biDiv,
		"mod": biMod,
		"inc": func(a []Value) (Value, error) { return biAdd([]Value{must1("inc", a), int64(1)}) },
		"dec": func(a []Value) (Value, error) { return biSub([]Value{must1("dec", a), int64(1)}) },

		// comparison / equality / logic
		"<":    cmpBuiltin("<", func(x, y float64) bool { return x < y }),
		">":    cmpBuiltin(">", func(x, y float64) bool { return x > y }),
		"<=":   cmpBuiltin("<=", func(x, y float64) bool { return x <= y }),
		">=":   cmpBuiltin(">=", func(x, y float64) bool { return x >= y }),
		"=":    biEq,
		"not=": biNotEq,
		"not":  func(a []Value) (Value, error) { return !truthy(must1("not", a)), nil },

		// sequences
		"list":    func(a []Value) (Value, error) { return &List{Items: append([]Value{}, a...)}, nil },
		"vector":  func(a []Value) (Value, error) { return &Vector{Items: append([]Value{}, a...)}, nil },
		"vec":     biVec,
		"first":   biFirst,
		"second":  biSecond,
		"rest":    biRest,
		"last":    biLast,
		"nth":     biNth,
		"count":   biCount,
		"empty?":  biEmpty,
		"conj":    biConj,
		"concat":  biConcat,
		"reverse": biReverse,
		"map":     biMap,
		"filter":  biFilter,
		"reduce":  biReduce,

		// predicates
		"nil?":     pred(func(v Value) bool { return v == nil }),
		"symbol?":  pred(func(v Value) bool { _, ok := v.(*Sym); return ok }),
		"keyword?": pred(func(v Value) bool { _, ok := v.(Keyword); return ok }),
		"string?":  pred(func(v Value) bool { _, ok := v.(string); return ok }),
		"list?":    pred(func(v Value) bool { _, ok := v.(*List); return ok }),
		"vector?":  pred(func(v Value) bool { _, ok := v.(*Vector); return ok }),
		"map?":     pred(func(v Value) bool { _, ok := v.(*Map); return ok }),
		"number?":  pred(func(v Value) bool { _, ok := asFloat(v); return ok }),
		"fn?":      pred(isCallable),
		"seq?":     pred(func(v Value) bool { _, ok := seqItems(v); return ok && v != nil }),

		// symbols / keywords
		"symbol":  biSymbol,
		"keyword": biKeyword,
		"name":    biName,
		"gensym":  biGensym,

		// maps
		"hash-map": biHashMap,
		"get":      biGet,
		"assoc":    biAssoc,
		"keys":     biKeys,
		"vals":     biVals,

		// strings
		"str": func(a []Value) (Value, error) {
			var sb strings.Builder
			for _, v := range a {
				sb.WriteString(Str(v))
			}
			return sb.String(), nil
		},
	}
	return m
}

// ---------- helpers ----------

func must1(name string, args []Value) Value {
	if len(args) == 0 {
		return nil
	}
	return args[0]
}

func pred(f func(Value) bool) func([]Value) (Value, error) {
	return func(a []Value) (Value, error) {
		if err := requireArity("predicate", a, 1, 1); err != nil {
			return nil, err
		}
		return f(a[0]), nil
	}
}

func isCallable(v Value) bool {
	switch v.(type) {
	case *Closure, *Builtin:
		return true
	default:
		return false
	}
}

func numArg(name string, v Value) (float64, bool, error) {
	switch x := v.(type) {
	case int64:
		return float64(x), true, nil
	case float64:
		return x, false, nil
	default:
		return 0, false, fmt.Errorf("%s expects numbers, got %s", name, typeName(v))
	}
}

// foldNum folds a numeric reduction, tracking whether the result stays integral.
func foldNum(name string, args []Value, start float64, allInt bool, op func(acc, x float64) float64) (Value, error) {
	acc := start
	for _, a := range args {
		f, isInt, err := numArg(name, a)
		if err != nil {
			return nil, err
		}
		allInt = allInt && isInt
		acc = op(acc, f)
	}
	return numResult(acc, allInt), nil
}

func numResult(f float64, isInt bool) Value {
	if isInt {
		return int64(f)
	}
	return f
}

func biAdd(a []Value) (Value, error) {
	return foldNum("+", a, 0, true, func(acc, x float64) float64 { return acc + x })
}

func biMul(a []Value) (Value, error) {
	return foldNum("*", a, 1, true, func(acc, x float64) float64 { return acc * x })
}

func biSub(a []Value) (Value, error) {
	if err := requireArity("-", a, 1, -1); err != nil {
		return nil, err
	}
	first, allInt, err := numArg("-", a[0])
	if err != nil {
		return nil, err
	}
	if len(a) == 1 {
		return numResult(-first, allInt), nil
	}
	return foldNum("-", a[1:], first, allInt, func(acc, x float64) float64 { return acc - x })
}

func biDiv(a []Value) (Value, error) {
	if err := requireArity("/", a, 1, -1); err != nil {
		return nil, err
	}
	// All-integer division stays integral (index math); any float promotes.
	allInt := true
	for _, v := range a {
		if _, isInt, err := numArg("/", v); err != nil {
			return nil, err
		} else if !isInt {
			allInt = false
		}
	}
	if allInt {
		acc := a[0].(int64)
		rest := a[1:]
		if len(a) == 1 {
			rest = a
			acc = 1
		}
		for _, v := range rest {
			d := v.(int64)
			if d == 0 {
				return nil, fmt.Errorf("/ by zero")
			}
			acc /= d
		}
		return acc, nil
	}
	first, _, _ := numArg("/", a[0])
	if len(a) == 1 {
		if first == 0 {
			return nil, fmt.Errorf("/ by zero")
		}
		return 1 / first, nil
	}
	acc := first
	for _, v := range a[1:] {
		d, _, _ := numArg("/", v)
		if d == 0 {
			return nil, fmt.Errorf("/ by zero")
		}
		acc /= d
	}
	return acc, nil
}

func biMod(a []Value) (Value, error) {
	if err := requireArity("mod", a, 2, 2); err != nil {
		return nil, err
	}
	x, ok := a[0].(int64)
	y, ok2 := a[1].(int64)
	if !ok || !ok2 {
		return nil, fmt.Errorf("mod expects integers")
	}
	if y == 0 {
		return nil, fmt.Errorf("mod by zero")
	}
	return x % y, nil
}

func cmpBuiltin(name string, ok func(x, y float64) bool) func([]Value) (Value, error) {
	return func(a []Value) (Value, error) {
		if err := requireArity(name, a, 2, -1); err != nil {
			return nil, err
		}
		prev, _, err := numArg(name, a[0])
		if err != nil {
			return nil, err
		}
		for _, v := range a[1:] {
			cur, _, err := numArg(name, v)
			if err != nil {
				return nil, err
			}
			if !ok(prev, cur) {
				return false, nil
			}
			prev = cur
		}
		return true, nil
	}
}

func biEq(a []Value) (Value, error) {
	if err := requireArity("=", a, 1, -1); err != nil {
		return nil, err
	}
	for _, v := range a[1:] {
		if !equalValues(a[0], v) {
			return false, nil
		}
	}
	return true, nil
}

func biNotEq(a []Value) (Value, error) {
	eq, err := biEq(a)
	if err != nil {
		return nil, err
	}
	return !eq.(bool), nil
}

// ---------- sequences ----------

func mustSeq(name string, v Value) ([]Value, error) {
	items, ok := seqItems(v)
	if !ok {
		return nil, fmt.Errorf("%s expects a list or vector, got %s", name, typeName(v))
	}
	return items, nil
}

func biVec(a []Value) (Value, error) {
	if err := requireArity("vec", a, 1, 1); err != nil {
		return nil, err
	}
	items, err := mustSeq("vec", a[0])
	if err != nil {
		return nil, err
	}
	return &Vector{Items: append([]Value{}, items...)}, nil
}

func biFirst(a []Value) (Value, error) {
	if err := requireArity("first", a, 1, 1); err != nil {
		return nil, err
	}
	items, err := mustSeq("first", a[0])
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return items[0], nil
}

func biSecond(a []Value) (Value, error) {
	if err := requireArity("second", a, 1, 1); err != nil {
		return nil, err
	}
	items, err := mustSeq("second", a[0])
	if err != nil {
		return nil, err
	}
	if len(items) < 2 {
		return nil, nil
	}
	return items[1], nil
}

func biLast(a []Value) (Value, error) {
	if err := requireArity("last", a, 1, 1); err != nil {
		return nil, err
	}
	items, err := mustSeq("last", a[0])
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return items[len(items)-1], nil
}

func biRest(a []Value) (Value, error) {
	if err := requireArity("rest", a, 1, 1); err != nil {
		return nil, err
	}
	items, err := mustSeq("rest", a[0])
	if err != nil {
		return nil, err
	}
	if len(items) <= 1 {
		return &List{}, nil
	}
	return &List{Items: append([]Value{}, items[1:]...)}, nil
}

func biNth(a []Value) (Value, error) {
	if err := requireArity("nth", a, 2, 3); err != nil {
		return nil, err
	}
	items, err := mustSeq("nth", a[0])
	if err != nil {
		return nil, err
	}
	idx, ok := a[1].(int64)
	if !ok {
		return nil, fmt.Errorf("nth index must be an integer, got %s", typeName(a[1]))
	}
	if idx < 0 || int(idx) >= len(items) {
		if len(a) == 3 {
			return a[2], nil
		}
		return nil, fmt.Errorf("nth index %d out of bounds (len %d)", idx, len(items))
	}
	return items[idx], nil
}

func biCount(a []Value) (Value, error) {
	if err := requireArity("count", a, 1, 1); err != nil {
		return nil, err
	}
	switch x := a[0].(type) {
	case nil:
		return int64(0), nil
	case string:
		return int64(len(x)), nil
	case *Map:
		return int64(len(x.Entries)), nil
	default:
		items, err := mustSeq("count", a[0])
		if err != nil {
			return nil, err
		}
		return int64(len(items)), nil
	}
}

func biEmpty(a []Value) (Value, error) {
	n, err := biCount(a)
	if err != nil {
		return nil, err
	}
	return n.(int64) == 0, nil
}

func biConj(a []Value) (Value, error) {
	if err := requireArity("conj", a, 1, -1); err != nil {
		return nil, err
	}
	switch coll := a[0].(type) {
	case *Vector:
		return &Vector{Items: append(append([]Value{}, coll.Items...), a[1:]...)}, nil
	case *List, nil:
		items, _ := seqItems(coll)
		out := append([]Value{}, a[1:]...) // prepend, Clojure list semantics
		// reverse the added elements so (conj '(1) 2 3) => (3 2 1)
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
		return &List{Items: append(out, items...)}, nil
	default:
		return nil, fmt.Errorf("conj expects a list or vector, got %s", typeName(a[0]))
	}
}

func biConcat(a []Value) (Value, error) {
	var out []Value
	for _, v := range a {
		items, err := mustSeq("concat", v)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return &List{Items: out}, nil
}

func biReverse(a []Value) (Value, error) {
	if err := requireArity("reverse", a, 1, 1); err != nil {
		return nil, err
	}
	items, err := mustSeq("reverse", a[0])
	if err != nil {
		return nil, err
	}
	out := make([]Value, len(items))
	for i, v := range items {
		out[len(items)-1-i] = v
	}
	return &List{Items: out}, nil
}

func biMap(a []Value) (Value, error) {
	if err := requireArity("map", a, 2, 2); err != nil {
		return nil, err
	}
	items, err := mustSeq("map", a[1])
	if err != nil {
		return nil, err
	}
	out := make([]Value, 0, len(items))
	for _, it := range items {
		v, err := apply(a[0], []Value{it})
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return &List{Items: out}, nil
}

func biFilter(a []Value) (Value, error) {
	if err := requireArity("filter", a, 2, 2); err != nil {
		return nil, err
	}
	items, err := mustSeq("filter", a[1])
	if err != nil {
		return nil, err
	}
	var out []Value
	for _, it := range items {
		keep, err := apply(a[0], []Value{it})
		if err != nil {
			return nil, err
		}
		if truthy(keep) {
			out = append(out, it)
		}
	}
	return &List{Items: out}, nil
}

func biReduce(a []Value) (Value, error) {
	if err := requireArity("reduce", a, 2, 3); err != nil {
		return nil, err
	}
	var acc Value
	var items []Value
	var err error
	if len(a) == 3 {
		acc = a[1]
		if items, err = mustSeq("reduce", a[2]); err != nil {
			return nil, err
		}
	} else {
		if items, err = mustSeq("reduce", a[1]); err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return apply(a[0], []Value{})
		}
		acc, items = items[0], items[1:]
	}
	for _, it := range items {
		acc, err = apply(a[0], []Value{acc, it})
		if err != nil {
			return nil, err
		}
	}
	return acc, nil
}

// ---------- symbols / keywords ----------

func biSymbol(a []Value) (Value, error) {
	if err := requireArity("symbol", a, 1, 1); err != nil {
		return nil, err
	}
	switch x := a[0].(type) {
	case string:
		return &Sym{Name: x}, nil
	case *Sym:
		return x, nil
	case Keyword:
		return &Sym{Name: string(x)}, nil
	default:
		return nil, fmt.Errorf("symbol expects a string, got %s", typeName(a[0]))
	}
}

func biKeyword(a []Value) (Value, error) {
	if err := requireArity("keyword", a, 1, 1); err != nil {
		return nil, err
	}
	switch x := a[0].(type) {
	case string:
		return Keyword(x), nil
	case Keyword:
		return x, nil
	case *Sym:
		return Keyword(x.Name), nil
	default:
		return nil, fmt.Errorf("keyword expects a string, got %s", typeName(a[0]))
	}
}

func biName(a []Value) (Value, error) {
	if err := requireArity("name", a, 1, 1); err != nil {
		return nil, err
	}
	switch x := a[0].(type) {
	case string:
		return x, nil
	case Keyword:
		return string(x), nil
	case *Sym:
		return x.Name, nil
	default:
		return nil, fmt.Errorf("name expects a symbol, keyword, or string, got %s", typeName(a[0]))
	}
}

func biGensym(a []Value) (Value, error) {
	if err := requireArity("gensym", a, 0, 1); err != nil {
		return nil, err
	}
	prefix := "G__"
	if len(a) == 1 {
		switch x := a[0].(type) {
		case string:
			prefix = x
		case *Sym:
			prefix = x.Name
		default:
			return nil, fmt.Errorf("gensym prefix must be a string, got %s", typeName(a[0]))
		}
	}
	n := atomic.AddInt64(&gensymCounter, 1)
	return &Sym{Name: fmt.Sprintf("%s%d", prefix, n)}, nil
}

// ---------- maps ----------

func biHashMap(a []Value) (Value, error) {
	if len(a)%2 != 0 {
		return nil, fmt.Errorf("hash-map expects an even number of arguments, got %d", len(a))
	}
	m := &Map{}
	for i := 0; i < len(a); i += 2 {
		m.Entries = append(m.Entries, MapEntry{Key: a[i], Value: a[i+1]})
	}
	return m, nil
}

func biGet(a []Value) (Value, error) {
	if err := requireArity("get", a, 2, 3); err != nil {
		return nil, err
	}
	def := Value(nil)
	if len(a) == 3 {
		def = a[2]
	}
	switch coll := a[0].(type) {
	case *Map:
		for _, e := range coll.Entries {
			if equalValues(e.Key, a[1]) {
				return e.Value, nil
			}
		}
		return def, nil
	case *Vector, *List:
		items, _ := seqItems(coll)
		if idx, ok := a[1].(int64); ok && idx >= 0 && int(idx) < len(items) {
			return items[idx], nil
		}
		return def, nil
	case nil:
		return def, nil
	default:
		return nil, fmt.Errorf("get expects a map or sequence, got %s", typeName(a[0]))
	}
}

func biAssoc(a []Value) (Value, error) {
	if err := requireArity("assoc", a, 3, -1); err != nil {
		return nil, err
	}
	if (len(a)-1)%2 != 0 {
		return nil, fmt.Errorf("assoc expects key/value pairs after the map")
	}
	var entries []MapEntry
	if m, ok := a[0].(*Map); ok {
		entries = append(entries, m.Entries...)
	} else if a[0] != nil {
		return nil, fmt.Errorf("assoc expects a map, got %s", typeName(a[0]))
	}
	out := &Map{Entries: entries}
	for i := 1; i < len(a); i += 2 {
		key, val := a[i], a[i+1]
		replaced := false
		for j := range out.Entries {
			if equalValues(out.Entries[j].Key, key) {
				out.Entries[j].Value = val
				replaced = true
				break
			}
		}
		if !replaced {
			out.Entries = append(out.Entries, MapEntry{Key: key, Value: val})
		}
	}
	return out, nil
}

func biKeys(a []Value) (Value, error) {
	if err := requireArity("keys", a, 1, 1); err != nil {
		return nil, err
	}
	m, ok := a[0].(*Map)
	if !ok {
		return nil, fmt.Errorf("keys expects a map, got %s", typeName(a[0]))
	}
	out := make([]Value, 0, len(m.Entries))
	for _, e := range m.Entries {
		out = append(out, e.Key)
	}
	return &List{Items: out}, nil
}

func biVals(a []Value) (Value, error) {
	if err := requireArity("vals", a, 1, 1); err != nil {
		return nil, err
	}
	m, ok := a[0].(*Map)
	if !ok {
		return nil, fmt.Errorf("vals expects a map, got %s", typeName(a[0]))
	}
	out := make([]Value, 0, len(m.Entries))
	for _, e := range m.Entries {
		out = append(out, e.Value)
	}
	return &List{Items: out}, nil
}
