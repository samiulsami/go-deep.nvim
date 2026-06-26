local tmp_dir = vim.fn.stdpath("data") .. "/go-deep-test"
vim.fn.mkdir(tmp_dir, "p")

vim.opt.runtimepath:prepend(vim.fn.getcwd())

local plugins_dir = tmp_dir .. "/plugins"
vim.fn.mkdir(plugins_dir, "p")

local function clone_plugin(repo, name)
	local path = plugins_dir .. "/" .. name
	if vim.fn.isdirectory(path) == 0 then
		vim.fn.system({ "git", "clone", "--depth=1", "https://github.com/" .. repo, path })
	end
	vim.opt.runtimepath:prepend(path)
end

clone_plugin("nvim-treesitter/nvim-treesitter", "nvim-treesitter")

local site_dir = vim.fn.stdpath("data") .. "/site"
vim.opt.runtimepath:prepend(site_dir)

require("nvim-treesitter").install({ "go" }):wait()
vim.opt.runtimepath:prepend(site_dir)
assert(pcall(vim.treesitter.language.add, "go"))

-- Mirror the real-world setup: start treesitter whenever a filetype is set.
-- Without this, setting vim.bo.filetype in tests does not attach a parser,
-- so get_parser returns nil even when the language is installed.
vim.api.nvim_create_autocmd("FileType", {
	callback = function(args)
		pcall(vim.treesitter.start, args.buf)
	end,
})
