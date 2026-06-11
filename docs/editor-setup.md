# Editor support

## Neovim — syntax highlighting

Add the bundled plugin to your runtimepath in `init.lua`:

```lua
vim.opt.rtp:prepend("/path/to/golisp-language/editors/neovim")
```

Or with lazy.nvim:

```lua
{ dir = "/path/to/golisp-language/editors/neovim" }
```

This gives you filetype detection, `commentstring`, `iskeyword` tuning, and syntax
highlighting that inherits from Clojure (parens, strings, keywords, comments, core
special forms) plus glisp-specific rules (positional type names, `defstruct`, `if-err`,
`send!`, etc.).

## LSP (Neovim 0.12+)

`glisp-lsp` is a Language Server that provides diagnostics (parse errors highlighted
inline), hover (show `defn`/`def` signatures and web package type definitions like
`web/Request`), jump-to-definition, find-references (project-wide, across open and
sibling `.glsp` files), a document outline (`documentSymbol`), rename, formatting, and
completions.

### Install

```
go install ./cmd/glisp-lsp
# or: make install
```

### Neovim setup

```lua
-- ~/.config/nvim/after/ftplugin/glsp.lua  (or in your init.lua)

-- filetype detection
vim.filetype.add({ extension = { glsp = "glsp" } })

-- register and enable the server
vim.lsp.config["glisp"] = {
  cmd          = { "glisp-lsp" },
  filetypes    = { "glsp" },
  root_markers = { "go.mod", ".git" },
}
vim.lsp.enable("glisp")
```

Diagnostics appear automatically as you edit. Hover with `K` (default Neovim mapping)
over any `defn` or `def` name to see its signature. Jump to definition with `gd`, list
all references with `grr`, and open the document outline with `gO` (Neovim 0.11+
defaults). Completions trigger automatically as you type.
