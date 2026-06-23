package transpiler

import (
	"fmt"
	"strings"

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

	// threading (-> and ->> are core macros, validated by the expander)
	"as->": {2, -1}, "tap->": {2, -1}, "tap->>": {2, -1},

	// debugging helpers
	"pp": {1, 1}, "time-it": {1, 1},

	// core collection access
	"get": {2, 3}, "assoc": {3, -1}, "dissoc": {2, -1}, "conj": {2, -1},
	"count": {1, 1}, "first": {1, 1}, "rest": {1, 1}, "nth": {2, 2},
	"keys": {1, 1}, "vals": {1, 1}, "merge": {2, -1},
	"error": {1, 1}, "nil?": {1, 1},
	"string": {1, 1}, "int": {1, 1}, "float64": {1, 1},
	"doseq": {2, -1}, "dotimes": {2, -1},

	// collection / sequence operations
	"map": {1, 2}, "map-indexed": {2, 2}, "filter": {1, 2}, "reduce": {3, 3}, "reverse": {1, 1},
	"contains?": {2, 2}, "some": {2, 2}, "every?": {2, 2}, "sort-by": {2, 2},
	"flatten": {1, 1}, "take": {1, 2}, "drop": {1, 2}, "for": {2, -1}, "mod": {2, 2},
	"transduce": {4, 4}, "sequence": {2, 2},
	"read-lines": {1, 1}, "transduce-lines": {4, 4},
	"second": {1, 1}, "last": {1, 1}, "empty?": {1, 1}, "not-empty": {1, 1},
	"get-in": {2, 2}, "assoc-in": {3, 3}, "update-in": {3, 3}, "update": {3, 3},
	"select-keys": {2, 2}, "rename-keys": {2, 2}, "group-by": {2, 2},
	"frequencies": {1, 1}, "into": {2, 3}, "mapcat": {2, 2},
	"take-while": {1, 2}, "drop-while": {1, 2}, "zipmap": {2, 2},
	"partition": {2, 2}, "partition-by": {2, 2}, "distinct": {1, 1},
	"remove": {1, 2}, "keep": {1, 2}, "split-at": {2, 2}, "split-with": {2, 2},
	"not-any?": {2, 2}, "interpose": {2, 2}, "repeat": {2, 2},

	// numeric predicates / arithmetic helpers
	"even?": {1, 1}, "odd?": {1, 1}, "pos?": {1, 1}, "neg?": {1, 1},
	"zero?": {1, 1}, "inc": {1, 1}, "dec": {1, 1}, "sort": {1, 1},

	// map conveniences
	"map-vals": {2, 2}, "map-keys": {2, 2}, "reduce-kv": {3, 3},

	// higher-order utilities
	"complement": {1, 1}, "identity": {1, 1}, "constantly": {1, 1},
	"apply": {2, 2}, "partial": {1, -1}, "format": {1, -1},
	"fnil": {2, 2},

	// numeric min/max
	"max": {1, -1}, "min": {1, -1}, "max-by": {2, 2}, "min-by": {2, 2},

	// string operations
	"upper-case": {1, 1}, "lower-case": {1, 1}, "trim": {1, 1},
	"starts-with?": {2, 2}, "ends-with?": {2, 2}, "replace": {3, 3},
	"split": {2, 2}, "join": {2, 2}, "blank?": {1, 1}, "capitalize": {1, 1},
	"subs": {2, 3}, "parse-int": {1, 1}, "parse-float": {1, 1},

	// set algebra
	"union": {2, 2}, "intersection": {2, 2}, "difference": {2, 2},
	"set": {1, 1},

	// effectful built-ins
	"panic": {1, 1}, "recover": {0, 0}, "assert": {1, 2},
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
	"proc/run": {1, -1}, "proc/sh": {1, 1},
	"path/join": {1, -1}, "path/dir": {1, 1}, "path/base": {1, 1},
	"path/ext": {1, 1}, "path/clean": {1, 1}, "glob": {1, 1}, "walk": {1, 1},
	"csv/parse": {1, 1}, "csv/write": {1, 1},

	// error wrapping, atoms, context
	// (atom …) parses to *ast.AtomExpr (typed form needs a type arg), so it is
	// not a symbol-dispatched call form and has no arity row here.
	"wrap-error": {2, 2}, "errors/is?": {2, 2},
	"swap!": {2, 2}, "reset!": {2, 2}, "deref": {1, 1},
	"ctx/background": {0, 0}, "ctx/todo": {0, 0}, "ctx/with-cancel": {1, 1},
	"ctx/with-timeout": {2, 2}, "ctx/cancel!": {1, 1}, "ctx/value": {2, 2},
	"ctx/with-value": {3, 3}, "ctx/done?": {1, 1}, "ctx/err": {1, 1},
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

// multiReturnBuiltins lists built-in forms that emit a Go multi-value
// (value, error) expression. They are consumed with if-err; using one as a
// single value (function tail, let/def binding, loop result) cannot compile,
// so checkMultiReturnValue reports a glisp-level error instead of leaking
// the Go one.
var multiReturnBuiltins = map[string]string{
	"parse-int":    "(int, error)",
	"parse-float":  "(float64, error)",
	"json/encode":  "(string, error)",
	"json/decode":  "(any, error)",
	"csv/parse":    "([]any, error)",
	"csv/write":    "(string, error)",
	"read-file":    "(string, error)",
	"read-lines":   "([]any, error)",
	"transduce-lines": "(any, error)",
	"list-dir":     "([]any, error)",
	"http/get":     "(response, error)",
	"http/post":    "(response, error)",
	"http/put":     "(response, error)",
	"http/delete":  "(response, error)",
	"http/request": "(response, error)",
}

// multiReturnCall reports whether n is a call statically known to produce
// multiple Go return values: one of multiReturnBuiltins, or a user function
// declared with a multi-return type (-> [T1 T2]). User definitions shadow
// built-in names.
func (e *Emitter) multiReturnCall(n ast.Node) (name, shape string, ok bool) {
	call, isCall := n.(*ast.CallExpr)
	if !isCall {
		return "", "", false
	}
	sym, isSym := call.Head.(*ast.Symbol)
	if !isSym {
		return "", "", false
	}
	if sig, found := e.symbols[sym.Name]; found {
		// typeExprToGo renders a multi-return [T1 T2] as "(T1, T2)"
		if strings.HasPrefix(sig.retType, "(") {
			return sym.Name, sig.retType, true
		}
		return "", "", false
	}
	if shape, found := multiReturnBuiltins[sym.Name]; found {
		return sym.Name, shape, true
	}
	if info, ok := e.resolveMethodCall(call); ok && strings.HasPrefix(info.sig.retType, "(") {
		return sym.Name, info.sig.retType, true
	}
	// An imported Go function whose loaded signature returns 2+ values (e.g.
	// pgx.Connect → (*pgx.Conn, error)) — used as a single value, this can't
	// compile, so report it like a multi-return built-in instead of leaking
	// Go's "assignment mismatch" error (ADR-015, go-interop-exploration §3.5).
	if fn, found := e.lookupGoCall(sym.Name); found && len(fn.results) >= 2 {
		return sym.Name, "(" + strings.Join(fn.results, ", ") + ")", true
	}
	return "", "", false
}

// checkMultiReturnValue reports a position-tagged error when n — about to be
// used as a single value — is a call known to produce multiple Go return
// values. Tail-position callers must skip this check when the surrounding
// function is itself multi-return, where `return f()` is legal Go.
func (e *Emitter) checkMultiReturnValue(n ast.Node) error {
	name, shape, ok := e.multiReturnCall(n)
	if !ok {
		return nil
	}
	return fmt.Errorf("%s returns multiple values %s, which cannot be used as a single value — bind them with (if-err [v err] (%s ...) ...) or discard with (do (%s ...) nil) (at %s)", name, shape, name, name, n.Pos())
}
