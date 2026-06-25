# Appendix B: Tooling

## glisp CLI

```
glisp <command> [args]
```

| Command | Description |
|---------|-------------|
| `run file.glsp [args]` | Compile and run, no output files |
| `run --watch file.glsp` | Re-run on every save |
| `file.glsp [args]` | Run directly (also via a `#!/usr/bin/env glisp` shebang) |
| `build file.glsp` | Compile to binary |
| `build dir/` | Compile all `.glsp` files in directory |
| `compile file.glsp` | Write generated `.go` file to disk |
| `print file.glsp` | Print generated Go to stdout |
| `macroexpand file.glsp` | Print the file with macros expanded |
| `fmt file.glsp` | Format file in-place |
| `fmt --check file.glsp` | Check formatting, exit 1 if needed |
| `test file.glsp` | Run `deftest` blocks |
| `test dir/` | Run all tests in directory |
| `doc [name]` | Show built-in documentation |
| `mod init [module-path]` | Initialize a `glisp.mod` |
| `get module[@version]` | Add GoLisp dependency |
| `get -go module[@version]` | Add Go dependency |
| `version` | Show compiler version |

## Formatter

`glisp fmt` formats `.glsp` files in place. It:

- Preserves comments (`;`, `;;`)
- Preserves docstrings (`;;;`)
- Keeps trailing inline comments on `let`/`loop`/`with-open` bindings in place
- Aligns map values
- Breaks long collections across lines
- Is round-trip safe — formatting is idempotent

Run as a pre-commit step:

```bash
glisp fmt --check .
```

## Language Server (LSP)

The `install.sh` script installs `glisp-lsp` alongside `glisp`. To build it from
source instead:

```bash
go install github.com/leinonen/golisp-language/cmd/glisp-lsp@latest
```

Features:

- Hover: function signatures and type info
- Jump to definition
- Completions: built-ins, user functions, web types
- Find all references
- Rename symbol
- Document outline
- Diagnostics on parse errors

## Neovim Setup

Install via Mason or manually. Add to your Neovim config:

```lua
vim.api.nvim_create_autocmd("FileType", {
  pattern = "glsp",
  callback = function()
    vim.lsp.start({
      name = "glisp-lsp",
      cmd = { "glisp-lsp" },
      root_dir = vim.fn.getcwd(),
    })
  end,
})
```

Set the file type for `.glsp` files:

```lua
vim.filetype.add({ extension = { glsp = "clojure" } })
```

Clojure syntax highlighting works well for GoLisp.

## VS Code

There is no dedicated GoLisp extension yet. Configure manually using Clojure syntax highlighting and the language server:

```json
{
  "files.associations": { "*.glsp": "clojure" },
  "lsp.servers": {
    "glisp-lsp": {
      "command": ["glisp-lsp"],
      "filetypes": ["clojure"],
      "rootPatterns": ["glisp.mod"]
    }
  }
}
```

## Inspecting Generated Go

When something behaves unexpectedly, look at what the compiler emits:

```bash
glisp print myfile.glsp
```

This prints the Go source. It's often the fastest way to understand a type error or an unexpected result.

## Docker Deployment

GoLisp produces static binaries. A minimal Dockerfile:

```dockerfile
FROM golang:1.25-alpine AS build
WORKDIR /app
COPY . .
RUN go install github.com/leinonen/golisp-language/cmd/glisp@latest
RUN glisp build main.glsp

FROM alpine:latest
COPY --from=build /app/main /app/main
ENTRYPOINT ["/app/main"]
```

The final image contains only the binary — no Go runtime, no interpreter.
