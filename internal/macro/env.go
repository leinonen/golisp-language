package macro

import "fmt"

// Env is a lexical environment: a frame of name->Value bindings with a parent
// chain. The root frame holds the built-ins (see NewGlobalEnv).
type Env struct {
	vars   map[string]Value
	parent *Env
}

// NewEnv returns a child environment of parent (parent may be nil for a root).
func NewEnv(parent *Env) *Env {
	return &Env{vars: make(map[string]Value), parent: parent}
}

// Get looks a name up through the parent chain.
func (e *Env) Get(name string) (Value, bool) {
	for env := e; env != nil; env = env.parent {
		if v, ok := env.vars[name]; ok {
			return v, true
		}
	}
	return nil, false
}

// Define binds a name in this frame (shadowing any outer binding).
func (e *Env) Define(name string, v Value) {
	e.vars[name] = v
}

// NewGlobalEnv returns a root environment populated with the macro built-ins.
func NewGlobalEnv() *Env {
	env := NewEnv(nil)
	for name, fn := range builtins() {
		env.Define(name, &Builtin{Name: name, Fn: fn})
	}
	return env
}

// requireArity is a small helper for built-ins to validate argument counts.
func requireArity(name string, args []Value, min, max int) error {
	n := len(args)
	if n < min || (max >= 0 && n > max) {
		if min == max {
			return fmt.Errorf("%s expects %d argument(s), got %d", name, min, n)
		}
		if max < 0 {
			return fmt.Errorf("%s expects at least %d argument(s), got %d", name, min, n)
		}
		return fmt.Errorf("%s expects %d to %d arguments, got %d", name, min, max, n)
	}
	return nil
}
