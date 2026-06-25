# Testing

GoLisp has a built-in test system. Tests live alongside the code they test.

## deftest

`deftest` groups assertions under a name:

```golisp
(ns main)

(defn add [a int b int] -> int (+ a b))

(deftest add-test
  (assert= (add 1 2) 3)
  (assert= (add 0 0) 0)
  (assert= (add -1 1) 0))
```

## Assert Forms

| Form | Passes when |
|------|-------------|
| `(assert= actual expected)` | values are equal |
| `(assert-true expr)` | expr is truthy |
| `(assert-false expr)` | expr is falsy |
| `(assert-nil expr)` | expr is nil |
| `(assert-err expr)` | expr is a non-nil error |

## Running Tests

```bash
glisp test myfile.glsp
glisp test mydir/
```

Output:

```
PASS  add-test
1 test passed
```

On failure:

```
FAIL  add-test
  assert= failed: got 4, expected 3
1 test failed
```

## Testing Patterns

**Test edge cases with descriptive names:**

```golisp
(deftest grade-boundaries
  (assert= (grade 90) "A")
  (assert= (grade 89) "B")
  (assert= (grade 60) "D")
  (assert= (grade 59) "F"))
```

**Test collections:**

```golisp
(deftest filter-test
  (assert= (filter even? [1 2 3 4 5]) [2 4])
  (assert= (filter odd?  [1 2 3 4 5]) [1 3 5])
  (assert= (filter even? []) []))
```

## Philosophy

GoLisp tests are direct: set up inputs, call functions, assert outputs. No mocking framework, no test doubles — if your function needs a database, connect to a test database.

Keep tests small and specific. One `deftest` per behavior, not per function. Test the observable contract, not the implementation.
