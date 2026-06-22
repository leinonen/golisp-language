package macro

import (
	"fmt"
	"strings"

	"golisp/internal/ast"
)

// Eval evaluates a node of the macro subset in env and returns its value.
// The subset is deliberately small (literals, symbols, vectors, maps, quote,
// if/when/do/cond/let/fn, and/or, and calls to built-ins or closures) — enough
// to run macro bodies, not the whole language.
func Eval(n ast.Node, env *Env) (Value, error) {
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
		// A qualified symbol (pkg/fn) has the slash past position 0; the bare
		// "/" division operator (slash at 0) is an ordinary symbol.
		if strings.Index(v.Name, "/") > 0 {
			return nil, fmt.Errorf("qualified symbol %q is not available in a macro body (at %s)", v.Name, v.Pos())
		}
		val, ok := env.Get(v.Name)
		if !ok {
			return nil, fmt.Errorf("unbound symbol %q in macro body (at %s)", v.Name, v.Pos())
		}
		return val, nil

	case *ast.VectorLit:
		items := make([]Value, 0, len(v.Elements))
		for _, e := range v.Elements {
			val, err := Eval(e, env)
			if err != nil {
				return nil, err
			}
			items = append(items, val)
		}
		return &Vector{Items: items}, nil

	case *ast.MapLit:
		entries := make([]MapEntry, 0, len(v.Pairs))
		for _, p := range v.Pairs {
			k, err := Eval(p.Key, env)
			if err != nil {
				return nil, err
			}
			val, err := Eval(p.Value, env)
			if err != nil {
				return nil, err
			}
			entries = append(entries, MapEntry{Key: k, Value: val})
		}
		return &Map{Entries: entries}, nil

	case *ast.QuoteExpr:
		return nodeToValue(v.Form)

	case *ast.IfExpr:
		cond, err := Eval(v.Cond, env)
		if err != nil {
			return nil, err
		}
		if truthy(cond) {
			return Eval(v.Then, env)
		}
		if v.Else == nil {
			return nil, nil
		}
		return Eval(v.Else, env)

	case *ast.WhenExpr:
		cond, err := Eval(v.Cond, env)
		if err != nil {
			return nil, err
		}
		if !truthy(cond) {
			return nil, nil
		}
		return evalBody(v.Body, env)

	case *ast.DoExpr:
		return evalBody(v.Body, env)

	case *ast.CondExpr:
		for _, c := range v.Clauses {
			t, err := Eval(c.Test, env)
			if err != nil {
				return nil, err
			}
			if truthy(t) {
				return Eval(c.Body, env)
			}
		}
		if v.Default != nil {
			return Eval(v.Default, env)
		}
		return nil, nil

	case *ast.LetExpr:
		child := NewEnv(env)
		for _, b := range v.Bindings {
			sym, ok := b.Pattern.(*ast.Symbol)
			if !ok {
				return nil, fmt.Errorf("destructuring is not supported in a macro let binding yet (at %s)", b.Value.Pos())
			}
			val, err := Eval(b.Value, child)
			if err != nil {
				return nil, err
			}
			child.Define(sym.Name, val)
		}
		return evalBody(v.Body, child)

	case *ast.FnExpr:
		return makeClosure(v, env, "")

	case *ast.CallExpr:
		return evalCall(v, env)

	case *ast.SyntaxQuoteExpr:
		return expandSyntaxQuote(v.Form, env, map[string]*Sym{})

	case *ast.UnquoteExpr:
		return nil, fmt.Errorf("unquote (~) is only valid inside a syntax-quote (`) (at %s)", n.Pos())

	case *ast.UnquoteSpliceExpr:
		return nil, fmt.Errorf("unquote-splice (~@) is only valid inside a syntax-quote (`) (at %s)", n.Pos())

	default:
		return nil, fmt.Errorf("cannot evaluate %T in a macro body (at %s)", n, n.Pos())
	}
}

func evalBody(body []ast.Node, env *Env) (Value, error) {
	var result Value
	for _, n := range body {
		v, err := Eval(n, env)
		if err != nil {
			return nil, err
		}
		result = v
	}
	return result, nil
}

func evalCall(n *ast.CallExpr, env *Env) (Value, error) {
	// and/or are short-circuiting special forms, so handle them before
	// evaluating arguments.
	if sym, ok := n.Head.(*ast.Symbol); ok {
		switch sym.Name {
		case "and":
			var last Value = true
			for _, a := range n.Args {
				v, err := Eval(a, env)
				if err != nil {
					return nil, err
				}
				if !truthy(v) {
					return v, nil
				}
				last = v
			}
			return last, nil
		case "or":
			for _, a := range n.Args {
				v, err := Eval(a, env)
				if err != nil {
					return nil, err
				}
				if truthy(v) {
					return v, nil
				}
			}
			return nil, nil
		}
	}

	fnVal, err := Eval(n.Head, env)
	if err != nil {
		return nil, err
	}
	args := make([]Value, 0, len(n.Args))
	for _, a := range n.Args {
		v, err := Eval(a, env)
		if err != nil {
			return nil, err
		}
		args = append(args, v)
	}
	res, err := apply(fnVal, args)
	if err != nil {
		return nil, fmt.Errorf("%s (at %s)", err, n.Pos())
	}
	return res, nil
}

func makeClosure(fn *ast.FnExpr, env *Env, name string) (*Closure, error) {
	return makeClosureFromParts(fn.Params, fn.Body, env, name, fn.Pos())
}

// makeClosureFromParts builds a Closure from a parameter list and body. Shared
// by fn evaluation and defmacro (a macro is a closure invoked on unevaluated
// argument forms).
func makeClosureFromParts(params []ast.Param, body []ast.Node, env *Env, name string, pos ast.Position) (*Closure, error) {
	c := &Closure{Body: body, Env: env, Name: name}
	for i, p := range params {
		if p.Pattern != nil {
			return nil, fmt.Errorf("destructuring is not supported in a macro fn/defmacro parameter yet (at %s)", pos)
		}
		if p.IsRest {
			if i != len(params)-1 {
				return nil, fmt.Errorf("& rest parameter must be last (at %s)", pos)
			}
			c.Rest = p.Name
			continue
		}
		c.Params = append(c.Params, p.Name)
	}
	return c, nil
}

// apply invokes a builtin or closure with already-evaluated arguments.
func apply(fn Value, args []Value) (Value, error) {
	switch f := fn.(type) {
	case *Builtin:
		return f.Fn(args)
	case *Closure:
		return applyClosure(f, args)
	default:
		return nil, fmt.Errorf("%s is not callable", typeName(fn))
	}
}

func applyClosure(c *Closure, args []Value) (Value, error) {
	child := NewEnv(c.Env)
	if c.Rest == "" {
		if len(args) != len(c.Params) {
			return nil, fmt.Errorf("%s expects %d argument(s), got %d", closureName(c), len(c.Params), len(args))
		}
	} else if len(args) < len(c.Params) {
		return nil, fmt.Errorf("%s expects at least %d argument(s), got %d", closureName(c), len(c.Params), len(args))
	}
	for i, name := range c.Params {
		child.Define(name, args[i])
	}
	if c.Rest != "" {
		rest := append([]Value{}, args[len(c.Params):]...)
		child.Define(c.Rest, &List{Items: rest})
	}
	return evalBody(c.Body, child)
}

func closureName(c *Closure) string {
	if c.Name != "" {
		return "fn " + c.Name
	}
	return "anonymous fn"
}
