package transpiler

import (
	"fmt"

	"golisp/internal/ast"
)

// builtinArity is the canonical source of truth for how many arguments each
// built-in call form accepts. It is consulted by checkBuiltinArity at the very
// top of emitCallExpr — before any argument is indexed — so that a wrong-arity
// built-in call produces a single, position-tagged glisp error instead of a Go
// index-out-of-range panic deep in an emit helper.
//
// Each entry is [min, max]; max == -1 means "unbounded" (variadic). Forms whose
// argument count cannot be expressed as a simple range (e.g. assoc, which also
// requires an odd count) carry the loosest safe bound here and keep their
// precise downstream check. Forms not listed are unconstrained by this gate and
// rely on their own emit helper for validation.
var builtinArity = map[string][2]int{
	// comparison / logic operators
	"=": {2, 2}, "not=": {2, 2},
	"<": {2, 2}, ">": {2, 2}, "<=": {2, 2}, ">=": {2, 2},
	"and": {2, -1}, "or": {2, -1}, "not": {1, 1},

	// threading
	"->": {2, -1}, "->>": {2, -1},

	// core collection access
	"get": {2, 3}, "assoc": {3, -1}, "dissoc": {2, -1}, "conj": {2, -1},
	"count": {1, 1}, "first": {1, 1}, "rest": {1, 1}, "nth": {2, 2},
	"keys": {1, 1}, "vals": {1, 1}, "merge": {2, -1},
	"error": {1, 1}, "nil?": {1, 1},
	"string": {1, 1}, "int": {1, 1}, "float64": {1, 1},
	"doseq": {2, -1}, "dotimes": {2, -1},

	// collection / sequence operations
	"map": {2, 2}, "filter": {2, 2}, "reduce": {3, 3}, "reverse": {1, 1},
	"contains?": {2, 2}, "some": {2, 2}, "every?": {2, 2}, "sort-by": {2, 2},
	"flatten": {1, 1}, "take": {2, 2}, "drop": {2, 2},
	"second": {1, 1}, "last": {1, 1}, "empty?": {1, 1}, "not-empty": {1, 1},
	"get-in": {2, 2}, "assoc-in": {3, 3}, "update-in": {3, 3}, "update": {3, 3},
	"select-keys": {2, 2}, "rename-keys": {2, 2}, "group-by": {2, 2},
	"frequencies": {1, 1}, "into": {2, 2}, "mapcat": {2, 2},
	"take-while": {2, 2}, "drop-while": {2, 2}, "zipmap": {2, 2},
	"partition": {2, 2}, "partition-by": {2, 2}, "distinct": {1, 1},
	"remove": {2, 2}, "keep": {2, 2}, "split-at": {2, 2}, "split-with": {2, 2},
	"not-any?": {2, 2}, "interpose": {2, 2}, "repeat": {2, 2},

	// numeric predicates / arithmetic helpers
	"even?": {1, 1}, "odd?": {1, 1}, "pos?": {1, 1}, "neg?": {1, 1},
	"zero?": {1, 1}, "inc": {1, 1}, "dec": {1, 1}, "sort": {1, 1},

	// map conveniences
	"map-vals": {2, 2}, "map-keys": {2, 2}, "reduce-kv": {3, 3},

	// higher-order utilities
	"complement": {1, 1}, "identity": {1, 1}, "constantly": {1, 1},
	"apply": {2, 2}, "partial": {1, -1}, "format": {1, -1},

	// string operations
	"upper-case": {1, 1}, "lower-case": {1, 1}, "trim": {1, 1},
	"starts-with?": {2, 2}, "ends-with?": {2, 2}, "replace": {3, 3},
	"split": {2, 2}, "join": {2, 2}, "blank?": {1, 1}, "capitalize": {1, 1},
	"subs": {2, 3}, "parse-int": {1, 1}, "parse-float": {1, 1},

	// set algebra
	"union": {2, 2}, "intersection": {2, 2}, "difference": {2, 2},

	// effectful built-ins
	"panic": {1, 1}, "recover": {0, 0},
	"os/env":      {1, 2},
	"json/encode": {1, 1}, "json/decode": {1, 1},
	"http/get": {1, 2}, "http/post": {2, 3}, "http/put": {2, 3},
	"http/delete": {1, 1}, "http/request": {1, 1},

	// file I/O
	"read-file": {1, 1}, "write-file": {2, 2}, "append-file": {2, 2},
	"file-exists?": {1, 1}, "list-dir": {1, 1}, "mkdir": {1, 1},

	// regex
	"re/match": {2, 2}, "re/find": {2, 2}, "re/find-all": {2, 2},
	"re/replace": {3, 3}, "re/split": {2, 2},

	// error wrapping, atoms, context
	"wrap-error": {2, 2}, "errors/is?": {2, 2},
	"atom": {1, 1}, "swap!": {2, 2}, "reset!": {2, 2}, "deref": {1, 1},
	"ctx/background": {0, 0}, "ctx/todo": {0, 0}, "ctx/with-cancel": {1, 1},
	"ctx/with-timeout": {2, 2}, "ctx/cancel!": {1, 1}, "ctx/value": {2, 2},
	"ctx/with-value": {3, 3},
}

// checkBuiltinArity reports a position-tagged error if name is a known built-in
// called with the wrong number of arguments. It returns nil for names not in
// builtinArity (leaving validation to the form's own emit helper).
func (e *Emitter) checkBuiltinArity(name string, n *ast.CallExpr) error {
	spec, ok := builtinArity[name]
	if !ok {
		return nil
	}
	min, max := spec[0], spec[1]
	got := len(n.Args)
	if got < min || (max >= 0 && got > max) {
		return fmt.Errorf("%s expects %s, got %d (at %s)", name, describeArity(min, max), got, n.Pos())
	}
	return nil
}

// describeArity renders an arity range as human-readable text.
func describeArity(min, max int) string {
	switch {
	case max < 0:
		return fmt.Sprintf("at least %d argument(s)", min)
	case min == max:
		return fmt.Sprintf("%d argument(s)", min)
	default:
		return fmt.Sprintf("%d to %d arguments", min, max)
	}
}
