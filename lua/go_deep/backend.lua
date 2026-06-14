local client = require("go_deep.client")

---@class go_deep.backend
---@field public plugin_root string | nil
---@field public built_binary string | nil
---@field public built_version_file string | nil
local M = {}

local module_path = debug.getinfo(1, "S").source:sub(2)
local backend_build_version = "v0.0.5"

M.plugin_root = vim.fn.fnamemodify(
	vim.api.nvim_get_runtime_file("lua/go_deep/init.lua", false)[1] or module_path,
	":p:h:h:h"
)
M.built_binary = M.plugin_root and vim.fs.joinpath(M.plugin_root, "bin", "go-deep")
M.built_version_file = M.plugin_root and vim.fs.joinpath(M.plugin_root, "bin", "go-deep.version")

---@param path string | nil
---@return boolean
local function read_build_version(path)
	if not path or vim.fn.filereadable(path) ~= 1 then
		return false
	end
	local ok, lines = pcall(vim.fn.readfile, path)
	if not ok or type(lines) ~= "table" or type(lines[1]) ~= "string" then
		return false
	end
	return vim.trim(lines[1]) == backend_build_version
end

---@return boolean
local function needs_rebuild()
	if not M.built_binary or not M.built_version_file then
		return true
	end
	if vim.fn.executable(M.built_binary) ~= 1 then
		return true
	end
	return not read_build_version(M.built_version_file)
end

---@param path string | nil
---@return boolean
function M.build(path)
	path = path or M.plugin_root
	if not path then
		vim.notify("[go-deep] could not infer plugin root. pass require('go_deep').build(path)", vim.log.levels.ERROR)
		return false
	end

	if vim.fn.executable("go") ~= 1 then
		vim.notify("[go-deep] Go is not available; cannot build plugin-local backend", vim.log.levels.ERROR)
		return false
	end

	local go_version = vim.trim(vim.fn.system({ "go", "env", "GOVERSION" }))
	local major, minor = go_version:match("^go(%d+)%.(%d+)")
	major = tonumber(major)
	minor = tonumber(minor)
	if not major or not minor or major < 1 or (major == 1 and minor < 25) then
		vim.notify(
			"[go-deep] Go 1.25+ is required to build from source; found " .. go_version,
			vim.log.levels.ERROR
		)
		return false
	end

	local go_dir = vim.fs.joinpath(path, "go")
	if vim.fn.isdirectory(go_dir) ~= 1 then
		vim.notify("[go-deep] missing Go source directory: " .. go_dir, vim.log.levels.ERROR)
		return false
	end

	local bin_path = vim.fs.joinpath(path, "bin", "go-deep")
	local stamp_path = vim.fs.joinpath(path, "bin", "go-deep.version")
	vim.fn.mkdir(vim.fs.dirname(bin_path), "p")

	vim.notify("[go-deep] building backend...", vim.log.levels.INFO)

	local result = vim.system({ "go", "build", "-o", bin_path, "." }, { cwd = go_dir, text = true }):wait()
	local exit_code = result.code
	local out = (result.stderr or "") .. (result.stdout or "")

	out = out:gsub("\n$", "")

	if exit_code == 0 then
		if stamp_path then
			vim.fn.writefile({ backend_build_version }, stamp_path)
		end
		vim.notify("[go-deep] backend built successfully", vim.log.levels.INFO)
	else
		vim.notify("[go-deep] build failed (exit " .. exit_code .. ")\n" .. out, vim.log.levels.ERROR)
	end
	return exit_code == 0
end

---@param opts table
function M.ensure(opts)
	if client.is_running() then
		return
	end
	if needs_rebuild() and not M.build() then
		return
	end

	local binary = M.built_binary and vim.fn.executable(M.built_binary) == 1 and M.built_binary or nil
	if not binary then
		if opts.notifications then
			vim.notify("[go-deep] backend binary not found under the plugin root", vim.log.levels.ERROR)
		end
		return
	end

	client.start(binary, opts)
end

return M
