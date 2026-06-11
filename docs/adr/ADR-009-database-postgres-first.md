# ADR-009: Database integration — postgres first

**Status**: Superseded by [ADR-014](ADR-014-database-out-of-language-scope.md).
Database access is no longer a language feature — it is delivered opt-in via
packages (e.g. a `glispdb` wrapper over `pgx`). The decision below is retained
for historical context; the built-in `db/*` forms and query builder it proposed
were never implemented and will not be.

## Context

Professional server applications need database access. glisp needs a database story. Options: raw SQL strings, an ORM, a query builder, or a thin map-oriented wrapper.

## Decision

postgres-first, via `pgx`. Results as `[]map[string]any`. A lightweight HoneySQL-inspired query builder for common cases; raw SQL always available.

## Reasons

**Why postgres first:**
- Dominant database for new Go server applications
- `pgx` is the best-in-class Go postgres driver: typed parameters, connection pooling, COPY support, prepared statements
- pgx's row scanning to `map[string]string` / `map[string]any` fits glisp's data model naturally
- Other databases (MySQL, SQLite) can follow the same pattern once postgres is proven

**Why rows as `[]map[string]any`:**
- Fits glisp's map-centric data model — results can be immediately transformed with `map`, `filter`, `group-by`, `select-keys`
- No struct definitions needed; schema changes don't require AST changes
- Consistent with the Ring request/response convention (ADR-006): everything is a map

**Why a query builder (not ORM):**
- ORMs hide SQL and make optimization hard; glisp users should understand the queries they're running
- A query builder (like HoneySQL) is data-in, SQL-out: `(db/select [:id :name] :from :users :where [:= :id id])` is just a function call that returns a SQL string + params
- Raw SQL is always available via `db/query` and `db/exec` — the query builder is additive, not mandatory

**Why `db/transaction` with auto-rollback:**
- Transactions are the most error-prone database pattern; auto-rollback on error/panic prevents leaked transactions
- The callback form `(db/transaction conn (fn [tx] ...))` is more composable than manual begin/commit/rollback

## API sketch

```clojure
(def pool (if-err [conn err] (db/connect (os/Getenv "DATABASE_URL"))
            (do (println "db error:" err) nil)))

;; query returns []map[string]any
(if-err [rows err] (db/query pool "SELECT id, name FROM users WHERE active = $1" [true])
  (handle-error err)
  (map (fn [r] (select-keys r [:id :name])) rows))

;; transaction
(db/transaction pool
  (fn [tx]
    (db/exec tx "INSERT INTO orders ..." [...])
    (db/exec tx "UPDATE inventory ..." [...])))
```

## Consequences

- `pgx` becomes a dependency; adds ~2MB to binaries
- Rows as `any` maps require type assertions for numeric operations (consistent with ADR-007)
- No connection-per-request patterns — `db/connect` returns a pool, meant to be shared
- Migration tooling (`goose`) is a separate binary; glisp wraps it via CLI subcommand
