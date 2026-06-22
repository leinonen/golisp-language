// Package macro implements the compile-time macro evaluator for glisp
// (Phase 13, ADR-017). It is a tree-walking interpreter over the "macro subset"
// of the language — enough to run macro bodies during a macroexpansion pass — not
// a general runtime. See docs/design/phase-13-macros.md.
//
// This file defines the compile-time value model and the AST <-> Value bridge.
package macro

import (
	"fmt"
	"strconv"
	"strings"

	"golisp/internal/ast"
)

// Value is a compile-time macro value. Its dynamic type is always one of:
//
//	nil, bool, int64, float64, string,
//	Keyword, *Sym, *List, *Vector, *Map, *Closure, *Builtin
//
// These are the data a macro manipulates: forms-as-data plus the scalars and
// functions needed to compute with them.
type Value = any

// Keyword is a glisp keyword (`:foo`) as a macro value. The leading colon is
// not stored.
type Keyword string

// Sym is a symbol as data (the result of quoting an identifier, or of `gensym`).
type Sym struct {
	Name string
	Pos  ast.Position
}

// List is a parenthesized form as data: (a b c).
type List struct{ Items []Value }

// Vector is a bracketed form as data: [a b c].
type Vector struct{ Items []Value }

// MapEntry is one key/value pair in a Map.
type MapEntry struct {
	Key   Value
	Value Value
}

// Map is a brace form as data: {:k v ...}, order-preserving.
type Map struct{ Entries []MapEntry }

// Closure is a user function (fn) created during macro evaluation.
type Closure struct {
	Params []string   // positional parameter names
	Rest   string     // name of the & rest parameter, "" if none
	Body   []ast.Node // function body
	Env    *Env       // captured lexical environment
	Name   string     // for diagnostics; "" for anonymous
}

// Builtin is a primitive function available inside macro bodies.
type Builtin struct {
	Name string
	Fn   func(args []Value) (Value, error)
}

// ---------- AST -> Value ----------

// nodeToValue converts a parsed AST node into the macro value that represents it
// as data. Parenthesized special forms (if/when/do/cond/let/fn/quote) are
// "un-parsed" back into generic lists so a macro sees uniform list data
// regardless of whether the parser specialized the form.
func nodeToValue(n ast.Node) (Value, error) {
	switch v := n.(type) {
	case *ast.NilLit:
		return nil, nil
	case *ast.BoolLit:
		return v.Value, nil
	case *ast.IntLit:
		return v.Value, nil
	case *ast.FloatLit:
		return v.Value, nil
	case *ast.StringLit:
		return v.Value, nil
	case *ast.KeywordLit:
		return Keyword(v.Value), nil
	case *ast.Symbol:
		return &Sym{Name: v.Name, Pos: v.Pos()}, nil
	case *ast.VectorLit:
		items, err := nodesToValues(v.Elements)
		if err != nil {
			return nil, err
		}
		return &Vector{Items: items}, nil
	case *ast.MapLit:
		entries := make([]MapEntry, 0, len(v.Pairs))
		for _, p := range v.Pairs {
			k, err := nodeToValue(p.Key)
			if err != nil {
				return nil, err
			}
			val, err := nodeToValue(p.Value)
			if err != nil {
				return nil, err
			}
			entries = append(entries, MapEntry{Key: k, Value: val})
		}
		return &Map{Entries: entries}, nil
	case *ast.CallExpr:
		items := make([]Value, 0, len(v.Args)+1)
		head, err := nodeToValue(v.Head)
		if err != nil {
			return nil, err
		}
		items = append(items, head)
		rest, err := nodesToValues(v.Args)
		if err != nil {
			return nil, err
		}
		items = append(items, rest...)
		return &List{Items: items}, nil
	case *ast.QuoteExpr:
		return listOf(n.Pos(), "quote", v.Form)
	case *ast.IfExpr:
		parts := []ast.Node{v.Cond, v.Then}
		if v.Else != nil {
			parts = append(parts, v.Else)
		}
		return listOf(n.Pos(), "if", parts...)
	case *ast.WhenExpr:
		return listOf(n.Pos(), "when", append([]ast.Node{v.Cond}, v.Body...)...)
	case *ast.DoExpr:
		return listOf(n.Pos(), "do", v.Body...)
	case *ast.CondExpr:
		var parts []ast.Node
		for _, c := range v.Clauses {
			parts = append(parts, c.Test, c.Body)
		}
		if v.Default != nil {
			parts = append(parts, ast.NewKeywordLit(n.Pos(), "else"), v.Default)
		}
		return listOf(n.Pos(), "cond", parts...)
	case *ast.LetExpr:
		bindVec := &ast.VectorLit{Pos_: n.Pos()}
		for _, b := range v.Bindings {
			bindVec.Elements = append(bindVec.Elements, b.Pattern, b.Value)
		}
		parts := append([]ast.Node{bindVec}, v.Body...)
		return listOf(n.Pos(), "let", parts...)
	case *ast.FnExpr:
		paramVec := &ast.VectorLit{Pos_: n.Pos()}
		for _, p := range v.Params {
			if p.IsRest {
				paramVec.Elements = append(paramVec.Elements, ast.NewSymbol(n.Pos(), "&"))
			}
			if p.Pattern != nil {
				return nil, fmt.Errorf("cannot quote a fn with a destructured parameter (at %s)", n.Pos())
			}
			paramVec.Elements = append(paramVec.Elements, ast.NewSymbol(n.Pos(), p.Name))
		}
		parts := append([]ast.Node{paramVec}, v.Body...)
		return listOf(n.Pos(), "fn", parts...)
	default:
		return nil, fmt.Errorf("cannot use %T as quoted macro data yet (at %s)", n, n.Pos())
	}
}

func nodesToValues(ns []ast.Node) ([]Value, error) {
	out := make([]Value, 0, len(ns))
	for _, n := range ns {
		v, err := nodeToValue(n)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// listOf builds a List value whose head is the symbol `head` followed by the
// converted forms.
func listOf(pos ast.Position, head string, forms ...ast.Node) (Value, error) {
	items := make([]Value, 0, len(forms)+1)
	items = append(items, &Sym{Name: head, Pos: pos})
	for _, f := range forms {
		v, err := nodeToValue(f)
		if err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	return &List{Items: items}, nil
}

// ---------- Value -> AST ----------

// valueToNode converts a macro value back into an AST node so macro output can
// be transpiled. Lists become generic CallExprs; re-recognizing special forms
// (so `(if ...)` becomes an *ast.IfExpr) is handled by the expansion pass in a
// later slice (Phase 13.3) — this bridge produces uniform, generic nodes.
func valueToNode(v Value, pos ast.Position) (ast.Node, error) {
	switch x := v.(type) {
	case nil:
		return ast.NewNilLit(pos), nil
	case bool:
		return ast.NewBoolLit(pos, x), nil
	case int64:
		return ast.NewIntLit(pos, x), nil
	case float64:
		return ast.NewFloatLit(pos, x), nil
	case string:
		return ast.NewStringLit(pos, x), nil
	case Keyword:
		return ast.NewKeywordLit(pos, string(x)), nil
	case *Sym:
		p := x.Pos
		if p == (ast.Position{}) {
			p = pos
		}
		return ast.NewSymbol(p, x.Name), nil
	case *Vector:
		elems, err := valuesToNodes(x.Items, pos)
		if err != nil {
			return nil, err
		}
		return ast.NewVectorLit(pos, elems), nil
	case *Map:
		pairs := make([]ast.MapPair, 0, len(x.Entries))
		for _, e := range x.Entries {
			k, err := valueToNode(e.Key, pos)
			if err != nil {
				return nil, err
			}
			val, err := valueToNode(e.Value, pos)
			if err != nil {
				return nil, err
			}
			pairs = append(pairs, ast.MapPair{Key: k, Value: val})
		}
		return ast.NewMapLit(pos, pairs), nil
	case *List:
		if len(x.Items) == 0 {
			return nil, fmt.Errorf("cannot convert an empty list to a node (at %s)", pos)
		}
		head, err := valueToNode(x.Items[0], pos)
		if err != nil {
			return nil, err
		}
		args, err := valuesToNodes(x.Items[1:], pos)
		if err != nil {
			return nil, err
		}
		return ast.NewCallExpr(pos, head, args), nil
	default:
		return nil, fmt.Errorf("cannot convert %s back into a form", typeName(v))
	}
}

func valuesToNodes(vs []Value, pos ast.Position) ([]ast.Node, error) {
	out := make([]ast.Node, 0, len(vs))
	for _, v := range vs {
		n, err := valueToNode(v, pos)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

// ---------- printing / equality / helpers ----------

// truthy applies ADR-011 truthiness: nil and false are falsy, all else truthy.
func truthy(v Value) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return true
}

func typeName(v Value) string {
	switch v.(type) {
	case nil:
		return "nil"
	case bool:
		return "bool"
	case int64:
		return "int"
	case float64:
		return "float"
	case string:
		return "string"
	case Keyword:
		return "keyword"
	case *Sym:
		return "symbol"
	case *List:
		return "list"
	case *Vector:
		return "vector"
	case *Map:
		return "map"
	case *Closure:
		return "fn"
	case *Builtin:
		return "builtin"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// Print renders a value in readable (pr-style) form: strings are quoted.
func Print(v Value) string {
	var sb strings.Builder
	writeValue(&sb, v, true)
	return sb.String()
}

// Str renders a value in str-style form: strings are raw (unquoted).
func Str(v Value) string {
	var sb strings.Builder
	writeValue(&sb, v, false)
	return sb.String()
}

func writeValue(sb *strings.Builder, v Value, readable bool) {
	switch x := v.(type) {
	case nil:
		sb.WriteString("nil")
	case bool:
		if x {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
	case int64:
		sb.WriteString(strconv.FormatInt(x, 10))
	case float64:
		s := strconv.FormatFloat(x, 'g', -1, 64)
		if !strings.ContainsAny(s, ".eE") {
			s += ".0"
		}
		sb.WriteString(s)
	case string:
		if readable {
			sb.WriteString(strconv.Quote(x))
		} else {
			sb.WriteString(x)
		}
	case Keyword:
		sb.WriteByte(':')
		sb.WriteString(string(x))
	case *Sym:
		sb.WriteString(x.Name)
	case *List:
		writeSeq(sb, x.Items, "(", ")", readable)
	case *Vector:
		writeSeq(sb, x.Items, "[", "]", readable)
	case *Map:
		sb.WriteByte('{')
		for i, e := range x.Entries {
			if i > 0 {
				sb.WriteByte(' ')
			}
			writeValue(sb, e.Key, readable)
			sb.WriteByte(' ')
			writeValue(sb, e.Value, readable)
		}
		sb.WriteByte('}')
	case *Closure:
		sb.WriteString("#<fn")
		if x.Name != "" {
			sb.WriteByte(' ')
			sb.WriteString(x.Name)
		}
		sb.WriteByte('>')
	case *Builtin:
		sb.WriteString("#<builtin ")
		sb.WriteString(x.Name)
		sb.WriteByte('>')
	default:
		fmt.Fprintf(sb, "%v", v)
	}
}

func writeSeq(sb *strings.Builder, items []Value, open, close string, readable bool) {
	sb.WriteString(open)
	for i, it := range items {
		if i > 0 {
			sb.WriteByte(' ')
		}
		writeValue(sb, it, readable)
	}
	sb.WriteString(close)
}

// equalValues is structural equality. Numbers compare across int/float; symbols
// and keywords by name; sequences elementwise.
func equalValues(a, b Value) bool {
	if an, aok := asFloat(a); aok {
		if bn, bok := asFloat(b); bok {
			return an == bn
		}
		return false
	}
	switch x := a.(type) {
	case nil:
		return b == nil
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case string:
		y, ok := b.(string)
		return ok && x == y
	case Keyword:
		y, ok := b.(Keyword)
		return ok && x == y
	case *Sym:
		y, ok := b.(*Sym)
		return ok && x.Name == y.Name
	case *List:
		y, ok := b.(*List)
		return ok && equalSeq(x.Items, y.Items)
	case *Vector:
		y, ok := b.(*Vector)
		return ok && equalSeq(x.Items, y.Items)
	case *Map:
		y, ok := b.(*Map)
		if !ok || len(x.Entries) != len(y.Entries) {
			return false
		}
		for _, e := range x.Entries {
			found := false
			for _, e2 := range y.Entries {
				if equalValues(e.Key, e2.Key) && equalValues(e.Value, e2.Value) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func equalSeq(a, b []Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalValues(a[i], b[i]) {
			return false
		}
	}
	return true
}

// asFloat returns the numeric value of v as a float64 and whether v is numeric.
func asFloat(v Value) (float64, bool) {
	switch x := v.(type) {
	case int64:
		return float64(x), true
	case float64:
		return x, true
	default:
		return 0, false
	}
}

// seqItems returns the elements of a list or vector (the two sequence kinds),
// and whether v is a sequence.
func seqItems(v Value) ([]Value, bool) {
	switch x := v.(type) {
	case *List:
		return x.Items, true
	case *Vector:
		return x.Items, true
	case nil:
		return nil, true
	default:
		return nil, false
	}
}
