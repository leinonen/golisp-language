package macro

import (
	"fmt"
	"strings"
	"sync/atomic"

	"golisp/internal/ast"
)

// expandSyntaxQuote expands a syntax-quote template (the form after a backtick)
// into a macro value — forms-as-data with `~` holes evaluated and `~@` holes
// spliced. gensyms threads auto-gensym (`name#`) bindings so every occurrence
// of `foo#` within one template resolves to the same fresh symbol.
//
// Hygiene (ADR-017): non-hygienic. Ordinary symbols are emitted as plain data
// (no namespace qualification); auto-gensym and the `gensym` built-in are the
// tools for introducing safe, capture-free bindings.
func expandSyntaxQuote(n ast.Node, env *Env, gensyms map[string]*Sym) (Value, error) {
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
		if g, ok := autoGensym(v.Name, v.Pos(), gensyms); ok {
			return g, nil
		}
		return &Sym{Name: v.Name, Pos: v.Pos()}, nil

	case *ast.UnquoteExpr:
		// ~x — evaluate x and insert the resulting value.
		return Eval(v.Form, env)

	case *ast.UnquoteSpliceExpr:
		// ~@x is only meaningful inside a sequence, where sqSeq handles it.
		return nil, fmt.Errorf("unquote-splice (~@) is only valid inside a sequence in a syntax-quote (at %s)", n.Pos())

	case *ast.VectorLit:
		items, err := sqSeq(v.Elements, env, gensyms)
		if err != nil {
			return nil, err
		}
		return &Vector{Items: items}, nil

	case *ast.CallExpr:
		// A parenthesized form (a b c): head + args is the sequence.
		seq := append([]ast.Node{v.Head}, v.Args...)
		items, err := sqSeq(seq, env, gensyms)
		if err != nil {
			return nil, err
		}
		return &List{Items: items}, nil

	case *ast.MapLit:
		entries := make([]MapEntry, 0, len(v.Pairs))
		for _, p := range v.Pairs {
			k, err := expandSyntaxQuote(p.Key, env, gensyms)
			if err != nil {
				return nil, err
			}
			val, err := expandSyntaxQuote(p.Value, env, gensyms)
			if err != nil {
				return nil, err
			}
			entries = append(entries, MapEntry{Key: k, Value: val})
		}
		return &Map{Entries: entries}, nil

	case *ast.QuoteExpr:
		// 'x inside a syntax-quote builds a (quote <expanded x>) form.
		inner, err := expandSyntaxQuote(v.Form, env, gensyms)
		if err != nil {
			return nil, err
		}
		return &List{Items: []Value{&Sym{Name: "quote", Pos: n.Pos()}, inner}}, nil

	case *ast.SyntaxQuoteExpr:
		return nil, fmt.Errorf("nested syntax-quote is not supported yet (at %s)", n.Pos())

	default:
		// Parser-specialized forms (if/when/do/cond/let/loop/fn/quote) are
		// un-parsed to their generic list shape, then expanded as a template so
		// ~/~@/auto-gensym inside them still apply.
		if g, ok := unparse(n); ok {
			return expandSyntaxQuote(g, env, gensyms)
		}
		return nil, fmt.Errorf("cannot use %T inside a syntax-quote yet (at %s)", n, n.Pos())
	}
}

// sqSeq expands the elements of a sequence inside a syntax-quote, splicing any
// ~@ element's evaluated sequence into the result.
func sqSeq(nodes []ast.Node, env *Env, gensyms map[string]*Sym) ([]Value, error) {
	var out []Value
	for _, node := range nodes {
		if sp, ok := node.(*ast.UnquoteSpliceExpr); ok {
			v, err := Eval(sp.Form, env)
			if err != nil {
				return nil, err
			}
			items, ok := seqItems(v)
			if !ok {
				return nil, fmt.Errorf("unquote-splice (~@) expects a list or vector, got %s (at %s)", typeName(v), sp.Pos())
			}
			out = append(out, items...)
			continue
		}
		v, err := expandSyntaxQuote(node, env, gensyms)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// autoGensym recognizes a `name#` symbol and returns a stable fresh symbol for
// it within one template (recorded in gensyms). Returns ok=false for ordinary
// symbols. A bare "#" is not an auto-gensym.
func autoGensym(name string, pos ast.Position, gensyms map[string]*Sym) (*Sym, bool) {
	if len(name) < 2 || !strings.HasSuffix(name, "#") {
		return nil, false
	}
	if existing, ok := gensyms[name]; ok {
		return existing, true
	}
	base := strings.TrimSuffix(name, "#")
	n := atomic.AddInt64(&gensymCounter, 1)
	g := &Sym{Name: fmt.Sprintf("%s__%d__auto", base, n), Pos: pos}
	gensyms[name] = g
	return g, true
}
