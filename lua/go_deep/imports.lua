---@class go_deep.imports
local imports = {}

local treesitter = require("go_deep.treesitter")
local utils = require("go_deep.utils")

local pending = {}

---@param bufnr integer
---@param import_path string | nil
---@param package_alias string | nil
---@param schedule boolean | nil
---@return boolean
---Insert one missing import.
function imports.apply(bufnr, import_path, package_alias, schedule)
	if not import_path then
		return false
	end
	pending[bufnr] = pending[bufnr] or {}
	if pending[bufnr][import_path] then
		return false
	end
	local existing = treesitter.get_imported_paths(bufnr)
	if existing[import_path] then
		return false
	end
	pending[bufnr][import_path] = true

	local apply = function()
		local ok, err = pcall(treesitter.add_import_statement, bufnr, package_alias, import_path)
		pending[bufnr][import_path] = nil
		if not ok then
			vim.notify("[go-deep] failed to apply import: " .. tostring(err), vim.log.levels.ERROR)
		end
	end
	if schedule then
		vim.schedule(apply)
	else
		apply()
	end
	return true
end

---@param bufnr integer
function imports.on_complete_done(bufnr)
	local completed_item = vim.v.completed_item or {}
	if type(vim.v.event) == "table" and type(vim.v.event.completed_item) == "table" then
		completed_item = vim.v.event.completed_item
	end

	local user_data = utils.decode_complete_user_data(completed_item.user_data)
	if not user_data or not user_data.import_path then
		return
	end
	imports.apply(bufnr, user_data.import_path, user_data.package_alias, true)
end

---@param bufnr integer
function imports.clear(bufnr)
	pending[bufnr] = nil
end

return imports
