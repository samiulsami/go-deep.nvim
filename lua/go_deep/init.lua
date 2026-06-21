local backend = require("go_deep.backend")
local native = require("go_deep.native")

---@class go_deep.Config
---@field notifications boolean show notifications. default: true
---@field index boolean use persisted stdlib index. default: true
---@field index_file_path string stdlib index path. default: vim.fn.stdpath("data") .. "/go_deep/go_deep.gob"
---@field min_keyword_length integer minimum prefix length. default: 3
---@field max_items integer maximum completion items. default: 30
---@field max_from_same_package integer maximum items from the same package. default: 4
---@field workspace_timeout integer workspace/symbol timeout in seconds. default: 15
---@field workspace_symbols boolean include workspace/gopls symbols. default: true
---@field exclude_imported_packages boolean exclude imported packages. default: true
---@field exclude_vendored_packages boolean exclude vendored packages. default: false
---@field exclude_internal_packages boolean exclude inaccessible internal packages. default: true
---@field exclude_test_files boolean exclude *_test.go symbols. default: true
---@field completion_cache boolean cache workspace/symbol results in-memory. default: true

---@class go_deep
local M = {}

---@type go_deep.Config
M.defaults = {
	notifications = true,
	index = true,
	index_file_path = vim.fn.stdpath("data") .. "/go_deep/go_deep.gob",
	min_keyword_length = 3,
	max_items = 30,
	max_from_same_package = 4,
	workspace_timeout = 15,
	workspace_symbols = true,
	exclude_imported_packages = true,
	exclude_vendored_packages = false,
	exclude_internal_packages = true,
	exclude_test_files = true,
	completion_cache = true,
}

---@param value any
---@return boolean
local function is_integer(value)
	return type(value) == "number" and value == math.floor(value)
end

---@param opts table | nil
---@param path string
---@param allow_notifications boolean
local function validate_opts(opts, path, allow_notifications)
	if opts == nil then
		return
	end
	if type(opts) ~= "table" then
		error(path .. " must be a table")
	end

	local errors = {}

	if opts.notifications ~= nil then
		if not allow_notifications then
			errors[#errors + 1] = path .. ".notifications is not allowed here"
		elseif type(opts.notifications) ~= "boolean" then
			errors[#errors + 1] = path .. ".notifications must be a boolean"
		end
	end
	if opts.index ~= nil then
		if not allow_notifications then
			errors[#errors + 1] = path .. ".index is not allowed here"
		elseif type(opts.index) ~= "boolean" then
			errors[#errors + 1] = path .. ".index must be a boolean"
		end
	end
	if opts.index_file_path ~= nil then
		if not allow_notifications then
			errors[#errors + 1] = path .. ".index_file_path is not allowed here"
		elseif type(opts.index_file_path) ~= "string" then
			errors[#errors + 1] = path .. ".index_file_path must be a string"
		elseif opts.index_file_path == "" then
			errors[#errors + 1] = path .. ".index_file_path must not be empty"
		end
	end
	if opts.min_keyword_length ~= nil then
		if not is_integer(opts.min_keyword_length) then
			errors[#errors + 1] = path .. ".min_keyword_length must be an integer"
		elseif opts.min_keyword_length < 1 then
			errors[#errors + 1] = path .. ".min_keyword_length must be >= 1"
		end
	end
	if opts.max_items ~= nil then
		if not is_integer(opts.max_items) then
			errors[#errors + 1] = path .. ".max_items must be an integer"
		elseif opts.max_items < 1 then
			errors[#errors + 1] = path .. ".max_items must be >= 1"
		end
	end
	if opts.max_from_same_package ~= nil then
		if not is_integer(opts.max_from_same_package) then
			errors[#errors + 1] = path .. ".max_from_same_package must be an integer"
		elseif opts.max_from_same_package < 0 then
			errors[#errors + 1] = path .. ".max_from_same_package must be >= 0"
		end
	end
	if opts.workspace_timeout ~= nil then
		if not allow_notifications then
			errors[#errors + 1] = path .. ".workspace_timeout is not allowed here"
		elseif not is_integer(opts.workspace_timeout) then
			errors[#errors + 1] = path .. ".workspace_timeout must be an integer"
		elseif opts.workspace_timeout < 1 then
			errors[#errors + 1] = path .. ".workspace_timeout must be >= 1"
		end
	end
	if opts.workspace_symbols ~= nil and type(opts.workspace_symbols) ~= "boolean" then
		errors[#errors + 1] = path .. ".workspace_symbols must be a boolean"
	end
	if opts.exclude_imported_packages ~= nil and type(opts.exclude_imported_packages) ~= "boolean" then
		errors[#errors + 1] = path .. ".exclude_imported_packages must be a boolean"
	end
	if opts.exclude_vendored_packages ~= nil and type(opts.exclude_vendored_packages) ~= "boolean" then
		errors[#errors + 1] = path .. ".exclude_vendored_packages must be a boolean"
	end
	if opts.exclude_internal_packages ~= nil and type(opts.exclude_internal_packages) ~= "boolean" then
		errors[#errors + 1] = path .. ".exclude_internal_packages must be a boolean"
	end
	if opts.exclude_test_files ~= nil and type(opts.exclude_test_files) ~= "boolean" then
		errors[#errors + 1] = path .. ".exclude_test_files must be a boolean"
	end
	if opts.completion_cache ~= nil then
		if not allow_notifications then
			errors[#errors + 1] = path .. ".completion_cache is not allowed here"
		elseif type(opts.completion_cache) ~= "boolean" then
			errors[#errors + 1] = path .. ".completion_cache must be a boolean"
		end
	end

	if #errors > 0 then
		error("go_deep: invalid config:\n" .. table.concat(errors, "\n"))
	end
end

---@param overrides go_deep.Config | nil
---@return go_deep.Config
function M.resolve_config(overrides)
	validate_opts(vim.g.go_deep, "vim.g.go_deep", true)
	validate_opts(overrides, "go_deep override", true)
	return vim.tbl_deep_extend("force", M.defaults, vim.g.go_deep or {}, overrides or {})
end

---@param overrides table | nil
---@param config go_deep.Config | nil
---@return go_deep.Config
function M.resolve_request_config(overrides, config)
	validate_opts(overrides, "go_deep request override", false)
	local resolved = config or M.resolve_config()
	if not overrides then
		return resolved
	end
	return vim.tbl_deep_extend("force", resolved, overrides)
end

---@param path string | nil
---@return boolean
function M.build(path)
	return backend.build(path, M.resolve_config())
end

---@param bufnr integer
function M.attach_to_buffer(bufnr)
	native.attach(bufnr, M.resolve_config())
end

return M
