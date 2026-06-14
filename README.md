# go-deep.nvim

A Go ```deep-completion``` source for Neovim native `completeFunc` and [blink.cmp](https://github.com/Saghen/blink.cmp), that works alongside the LSP source(s) and provides completion suggestions for <b> "the standard library, local, and vendored packages".</b>

#### Why?

At the time of writing, the Go Language Server (```gopls@v0.22.0```) doesn't seem to support deep completions for unimported packages. For example, with deep completion enabled, typing ```'cha'``` could suggest ```'rand.NewChaCha8()'``` as a possible completion option - but that is not the case no matter how high the completion budget is set for ```gopls```.

#### How?

Query  ```gopls's``` ```workspace/symbol``` endpoint, keep project symbols in memory, build a persisted stdlib index, convert the resulting ```SymbolInformation``` into ```completionItemKinds```, filter the resuls then feed them back into native completion / ```blink.cmp```

---

⚠️ <i> First startup may stutter while the stdlib index is being built </i>

## Requirements

- Neovim 0.12+
- `gopls` on `PATH`
- Tree-sitter Go parser
- Go 1.25+ (to build the plugin-local backend)

## Backend

The plugin needs a Go binary at `bin/go-deep` under the plugin root. It tries to
auto-build it on first use. If that fails or you prefer not to auto-build:

**Download a prebuilt archive** from the [releases page](https://github.com/samiulsami/go-deep.nvim/releases),
pick the asset for your OS/arch, and extract it into the plugin root. The
archive already contains `bin/go-deep`.

**Or build manually** from inside Neovim:
```
:lua require("go_deep").build()
```

The plugin does not use `go install`, `GOPATH`, or `PATH` for its backend.
The binary always lives at `bin/go-deep` under the plugin root.

## Setup
#### vim.pack + native completion

```lua
vim.pack.add({
    { src = "https://github.com/samiulsami/go-deep.nvim" },
})

vim.g.go_deep = {
    notifications = true,
    min_keyword_length = 3,
    max_items = 30,
    max_from_same_package = 4,
    exclude_imported_packages = true,
}

vim.api.nvim_create_autocmd("LspAttach", {
    callback = function(ev)
        local client = vim.lsp.get_client_by_id(ev.data.client_id)
        if client and client.name == "gopls" then
            require("go_deep").attach_to_buffer(ev.buf)
        end
    end,
})

vim.opt.completeopt = { "menu", "menuone", "popup", "noselect", "fuzzy" }
vim.opt.complete = { "o", ".", "w", "b", "u", "t" }
vim.opt.omnifunc = "v:lua.vim.lsp.omnifunc"
vim.opt.autocomplete = true
```

#### vim.pack + blink.cmp

```lua
vim.pack.add({
    { src = "https://github.com/saghen/blink.cmp" },
    { src = "https://github.com/saghen/blink.lib" },
    { src = "https://github.com/samiulsami/go-deep.nvim" },
})

require("blink.cmp").setup({
    sources = {
        default = { "lsp", "path", "buffer", "go_deep" },
        providers = {
            go_deep = {
                module = "go_deep.blink",
                async = true,
                opts = {
                    min_keyword_length = 2,
                    max_items = 5,
                    max_from_same_package = 4,
                    exclude_imported_packages = true,
                },
            },
        },
    },
})
```

<details>
<summary>lazy.nvim</summary>

#### lazy.nvim + native completion

```lua
{
    "samiulsami/go-deep.nvim",
    opts = {
        min_keyword_length = 3,
        max_items = 30,
        max_from_same_package = 4,
        exclude_imported_packages = true,
    },
    config = function(_, opts)
        vim.g.go_deep = opts
        vim.api.nvim_create_autocmd("LspAttach", {
            callback = function(ev)
                local client = vim.lsp.get_client_by_id(ev.data.client_id)
                if client and client.name == "gopls" then
                    require("go_deep").attach_to_buffer(ev.buf)
                end
            end,
        })
    end,
}
```

#### lazy.nvim + blink.cmp

```lua
{
    "saghen/blink.cmp",
    branch = "main",
    dependencies = {
        { "saghen/blink.lib" },
        { "samiulsami/go-deep.nvim" },
    },
    opts = {
        sources = {
            default = { "lsp", "path", "buffer", "go_deep" },
            providers = {
                go_deep = {
                    module = "go_deep.blink",
                    async = true,
                    opts = {
                        min_keyword_length = 2,
                        max_items = 5,
                        max_from_same_package = 4,
                        exclude_imported_packages = true,
                    },
                },
            },
        },
    },
}
```

</details>

## Default options

```lua
vim.g.go_deep = {
    notifications = true,
    cache = true,
    index = true,
    index_db_path = vim.fn.stdpath("data") .. "/go_deep/go_deep.gob",
    workspace_timeout = 15,
    min_keyword_length = 3,
    max_items = 30,
    max_from_same_package = 4,
    exclude_imported_packages = true,
    exclude_vendored_packages = false,
    exclude_internal_packages = true,
    exclude_test_files = true,
}
```

The plugin is Go-only and requires an attached `gopls` client. It does not use
filetype-based routing.

`providers.go_deep.opts` is merged over `vim.g.go_deep` for that request.
These overrides affect only request-local keys:

- `min_keyword_length`
- `max_items`
- `max_from_same_package`
- `exclude_imported_packages`
- `exclude_vendored_packages`
- `exclude_internal_packages`
- `exclude_test_files`

Everything else stays global-only in `vim.g.go_deep`.

`workspace_timeout` is a backend startup configuration option; it is not a
request-local blink override.

## blink.cmp

Use `go_deep.blink` as the provider shown in the setup example above.

Behavior:

- enabled only when `gopls` is attached to the current buffer
- always treated as incomplete so blink can refetch
- accepting an item inserts the missing import

Do not also call `attach_to_buffer()` for blink-managed buffers.

## Native completion

`attach_to_buffer()` prepends `Fv:lua.go_deep_completefunc` to buffer-local
`'complete'`, sends async completion requests to the backend, and inserts the
missing import on accept.

Only the vetted Go symbol kinds are surfaced: types, enums, interfaces,
functions, variables, constants, and structs.

## Logs

- backend logs are written to `$CACHE/go_deep/logs/YYYYMMDD-HHMMSS-PID.log`
  (`$CACHE` is `os.UserCacheDir()`: `~/.cache` on Linux, `~/Library/Caches` on macOS)
- startup, build failures, and backend exits are reported with `vim.notify()`

## TODO

- [ ] Add more tests
- [ ] Archive after [this issue](https://github.com/golang/go/issues/38528) is properly addressed.

## Help

See `:help go_deep`.
