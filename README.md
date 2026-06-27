# go-deep.nvim

A Go `deep-completion` source for Neovim native `completeFunc` and [blink.cmp](https://github.com/Saghen/blink.cmp) that suggests symbols from <b>unimported standard-library, local, and vendored packages</b>, then inserts the missing import on accept.

_Successor to [`cmp-go-deep`](https://github.com/samiulsami/cmp-go-deep), rewritten with a plugin-local Go backend._

#### Why?

At the time of writing, the Go Language Server (```gopls@v0.22.0```) doesn't seem to support deep completions for unimported packages. For example, with deep completion enabled, typing ```'cha'``` could suggest ```'rand.NewChaCha8()'``` as a possible completion option - but that is not the case no matter how high the completion budget is set for ```gopls```.

#### How?

Build a stdlib index upon first startup, query ```gopls's``` ```workspace/symbol``` endpoint upon each keystroke, merge the results with matching stdlib symbols, then feed them back into native completion / ```blink.cmp```

---

https://github.com/user-attachments/assets/85df65d9-ca38-4e69-8e4f-caa3d4a0252d

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

That builds `bin/go-deep` and, when `vim.g.go_deep.index ~= false`, prebuilds
the persisted stdlib index too.

The plugin does not use `go install`, `GOPATH`, or `PATH` for its backend.
The binary always lives at `bin/go-deep` under the plugin root.

## Setup
#### vim.pack + native completion

***NOTE: setting `vim.opt.autocomplete = true` may flood the screen with completion suggestions unless `go_deep.max_items` is reduced.***

```lua
vim.g.go_deep = {
    notifications = true,
    min_keyword_length = 3,
    max_items = 30,
    max_from_same_package = 4,
    exclude_imported_packages = true,
}

vim.api.nvim_create_autocmd("PackChanged", {
    callback = function(ev)
        if ev.data.spec.name ~= "go-deep.nvim" or (ev.data.kind ~= "install" and ev.data.kind ~= "update") then
            return
        end
        if not ev.data.active then
            vim.cmd.packadd("go-deep.nvim")
        end
        require("go_deep").build(ev.data.path)
    end,
})

vim.pack.add({
    { src = "https://github.com/samiulsami/go-deep.nvim" },
})

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
```

<details>
<summary>lazy.nvim + blink.cmp</summary>

#### lazy.nvim + blink.cmp

```lua
{
    "saghen/blink.cmp",
    branch = "main",
    dependencies = {
        { "saghen/blink.lib" },
        {
            "samiulsami/go-deep.nvim",
            opts = {
                min_keyword_length = 2,
                max_items = 5,
                max_from_same_package = 4,
                exclude_imported_packages = true,
            },
            build = ':lua require("go_deep").build()',
        },
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
    -- Enable/disable notifications.
    notifications = true,

    -- Build and use the persisted stdlib index (Takes at most ~1MB).
    index = true,

    -- Path to the persisted stdlib index file.
    index_file_path = vim.fn.stdpath("data") .. "/go_deep/go_deep.gob",

    -- Timeout in seconds for gopls workspace/symbol.
    workspace_timeout = 15,

    -- Include workspace symbols from gopls.
    workspace_symbols = true,

    -- Minimum prefix length before completions trigger.
    min_keyword_length = 3,

    -- Maximum completion items returned.
    max_items = 30,

    -- Maximum items from the same package per request.
    max_from_same_package = 4,

    -- Exclude packages already imported in the current file.
    exclude_imported_packages = true,

    -- Exclude symbols from vendor/ packages.
    exclude_vendored_packages = false,

    -- Exclude inaccessible internal packages.
    exclude_internal_packages = true,

    -- Exclude symbols from *_test.go files.
    exclude_test_files = true,

    -- Cache workspace/symbol results in memory.
    completion_cache = true,
}
```

***The plugin is Go-only and requires an attached `gopls` client. It does not support/use filetype-based activation.***

## blink.cmp


Use `go_deep.blink` as the provider shown in the setup example above.

Behavior:
- `providers.go_deep.opts` is merged over `vim.g.go_deep` for each request.
- enabled only when `gopls` is attached to the current buffer
- always treated as incomplete so blink can refetch
- accepting an item inserts the missing import

⚠️ Do not also call `attach_to_buffer()` for blink-managed buffers.

## Native completion

`attach_to_buffer()` prepends `Fv:lua.go_deep_completefunc` to buffer-local
`'complete'`, sends async completion requests to the backend, and inserts the
missing import on accept.

Only the vetted Go symbol kinds are surfaced: types, interfaces,
functions, variables, constants, and structs.

## Logs

- backend logs are written to `$CACHE/go_deep/logs/YYYYMMDD-HHMMSS-PID.log`
  (`$CACHE` is `os.UserCacheDir()`: `~/.cache` on Linux, `~/Library/Caches` on macOS)
- startup, build failures, and backend exits are reported with `vim.notify()`

## TODO

- [ ] Archive after [this issue](https://github.com/golang/go/issues/38528) is properly addressed.

## Help

See `:help go_deep`.
