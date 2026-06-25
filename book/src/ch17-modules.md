# Modules

GoLisp's module system maps onto Go modules. A module is a directory with a `glisp.mod` file.

## glisp.mod

```
module github.com/myuser/myapp

require (
  github.com/myuser/utils v1.2.0
)

go-require (
  github.com/google/uuid v1.6.0
  github.com/jackc/pgx/v5 v5.7.2
)
```

`require` pulls in GoLisp modules. `go-require` pulls in Go packages for use via interop.

## Initializing a Module

```bash
glisp mod init github.com/myuser/myapp
```

Creates a `glisp.mod` in the current directory.

## Adding Dependencies

Add a GoLisp module:

```bash
glisp get github.com/myuser/utils@v1.2.0
```

Add a Go package:

```bash
glisp get -go github.com/google/uuid@v1.6.0
```

## Namespaces

Every `.glsp` file declares a namespace with `ns`. The namespace becomes the Go package name:

```golisp
(ns main)                           ; package main (the entry point)
(ns utils)                          ; package utils
(ns db (:import [golisp/web]))      ; imports the web framework
```

For external dependencies:

```golisp
(ns server
  (:import [golisp/web])
  (:require [github.com/myuser/utils]))
```

## Writing a Library

Functions are exported by using PascalCase names:

```golisp
; utils/math.glsp
(ns utils)

;;; Returns the absolute value of n.
(defn Abs [n int] -> int
  (if (< n 0) (- n) n))

;;; Clamps n to the range [min, max].
(defn Clamp [n int min int max int] -> int
  (cond
    (< n min) min
    (> n max) max
    :else n))
```

## Consuming a Library

Call exported functions with the package name prefix, but in lowercase:

```golisp
(ns main
  (:require [github.com/myuser/utils]))

(defn main [] -> void
  (println (utils/abs -5))        ; calls utils.Abs(-5)
  (println (utils/clamp 15 0 10))) ; calls utils.Clamp(15, 0, 10)
```

The compiler converts `utils/abs` to `utils.Abs` automatically.

## Multi-File Projects

Split a program across multiple files — they share a namespace:

```
myapp/
  main.glsp      ; (ns main)
  handlers.glsp  ; (ns main)
  models.glsp    ; (ns main)
```

Build the directory:

```bash
glisp build myapp/
```

All `.glsp` files with the same namespace compile into one Go package.

## Publishing a Module

Create a `glisp.mod`, push to GitHub, tag a version:

```bash
git tag v1.0.0
git push origin v1.0.0
```

Users then `glisp get github.com/youruser/yourlib@v1.0.0`.
