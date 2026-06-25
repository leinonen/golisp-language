# Scripting and the CLI

GoLisp compiles to a single static binary, which makes it a good fit for
command-line tools and scripts. You can run a `.glsp` file directly, parse options
declaratively, shell out to other programs, and walk the filesystem — all from
`core`, with no imports.

## Running Scripts

For quick iteration, run a file without producing any artifacts:

```bash
glisp run stats.glsp 3 1 4 1 5
glisp run --watch server.glsp     # re-run on every save
```

Add a shebang and a file is directly executable:

```golisp
#!/usr/bin/env glisp
(ns main)

(defn main [] -> void
  (println "ahoy"))
```

```bash
chmod +x ahoy.glsp
./ahoy.glsp
```

When you want a distributable binary instead, `glisp build stats.glsp` produces one.

## Arguments and Environment

The `sys/` namespace fronts the process and its environment:

```golisp
(rest (sys/args))           ; arguments after the program name
(sys/env "PORT")            ; "" if unset
(sys/env "PORT" "8080")     ; with a default
(sys/exit 1)                ; exit with a status code
```

## Parsing Options

`cli/parse-opts` turns raw arguments into options and positionals. Each spec is a
map: `:long` (required), `:short`, `:desc`, `:default`, `:flag` (a boolean that
consumes no value), and `:int` (parse the value as an integer):

```golisp
(def specs []any
  [{:long "--name" :short "-n" :desc "Who to greet" :default "world"}
   {:long "--loud" :short "-l" :desc "Shout"        :flag true}])

(defn main [] -> void
  (let [parsed (cli/parse-opts (rest (sys/args)) specs)
        opts (:options parsed)
        msg  (str "hello, " (:name opts))]
    (when (not (empty? (:errors parsed)))
      (doseq [e (:errors parsed)] (println "error:" e))
      (println (:summary parsed))
      (sys/exit 1))
    (println (if (:loud opts) (str/upper msg) msg))
    (println "extra args:" (:arguments parsed))))
```

The result is a map: `:options` (keyed by each long name without `--`, so `:name`
and `:loud`), `:arguments` (the positionals), `:errors` (unknown options, missing
values, bad integers), and `:summary` (generated help text). `--name value`,
`--name=value`, and `-n value` are all accepted; `--` ends option parsing.

## Running Other Programs

`proc/run` executes a command directly; `proc/sh` runs a string through `sh -c`
so you get pipes, globs, and redirection. Both return `{:out :err :exit :ok}`:

```golisp
(let [r (proc/run "git" "rev-parse" "HEAD")]
  (if (:ok r)
    (println "commit:" (str/trim (:out r)))
    (println "git failed (" (:exit r) "):" (:err r))))

(str/trim (:out (proc/sh "ls -1 | wc -l")))   ; count files via the shell
```

## Files and Paths

`slurp` reads a whole file, `spit` writes one, and `lines` splits text on
newlines. Path helpers front `path/filepath`, and `glob`/`walk` find files:

```golisp
(spit "/tmp/note.txt" "hello\nworld")

(if-err [text err] (slurp "/tmp/note.txt")
  (println "read failed:" err)
  (doseq [l (lines text)] (println "line:" l)))

(path/join "src" "main.glsp")     ; "src/main.glsp"
(path/base "src/main.glsp")       ; "main.glsp"
(path/ext  "main.glsp")           ; ".glsp"

(glob "*.txt")                    ; matching files in the current dir
(doseq [f (walk "examples")]      ; every file under a tree, recursively
  (when (str/ends-with? f ".glsp")
    (println (path/base f))))
```

That is the whole scripting kit: read input, transform it, shell out where it's
easier, and write the result — compiled to one binary you can drop anywhere.
