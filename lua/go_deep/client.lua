local treesitter = require("go_deep.treesitter")

---@class go_deep.client
local M = {}

---@class go_deep.client.State
---@field public channel integer | nil
---@field public job_id integer | nil
---@field public next_request_id integer
---@field public pending table<integer, go_deep.client.PendingCallback>

---@class go_deep.client.PendingCallback
---@field public on_items fun(reply: go_deep.client.Reply)
---@field public on_error fun(err: string)

---@class go_deep.client.Reply
---@field public request_id integer
---@field public items table[]

---@type go_deep.client.State
local state = {
	channel = nil,
	job_id = nil,
	next_request_id = 0,
	pending = {},
}

---@param err string
local function fail_pending(err)
	local pending = state.pending
	state.pending = {}
	for _, handlers in pairs(pending) do
		if handlers and handlers.on_error then
			pcall(handlers.on_error, err)
		end
	end
end

---@return boolean
function M.is_running()
	if not state.job_id then
		return false
	end
	return vim.fn.jobwait({ state.job_id }, 0)[1] == -1
end

---@param binary string
---@param opts table
function M.start(binary, opts)
	if M.is_running() then
		return
	end

	local ok, result = pcall(function()
		return vim.fn.jobstart({
			binary,
			"serve",
			"--cache=" .. tostring(opts.cache),
			"--index=" .. tostring(opts.index),
			"--index-db-path=" .. tostring(opts.index_db_path or ""),
			"--min-prefix-length=" .. opts.min_keyword_length,
			"--max-items=" .. opts.max_items,
			"--max-from-same-package=" .. (opts.max_from_same_package or 4),
			"--workspace-timeout=" .. (opts.workspace_timeout or 15),
			"--exclude-imported=" .. tostring(opts.exclude_imported_packages),
			"--exclude-vendored=" .. tostring(opts.exclude_vendored_packages),
			"--exclude-internal=" .. tostring(opts.exclude_internal_packages),
			"--exclude-test-files=" .. tostring(opts.exclude_test_files),
		}, {
			rpc = true,
			on_exit = function(_, code, _)
				state.channel = nil
				state.job_id = nil
				fail_pending("backend exited with code " .. code)
				if code ~= 0 and code ~= 143 then
					if opts.notifications then
						vim.notify("[go-deep] backend exited with code " .. code, vim.log.levels.ERROR)
					end
				end
			end,
		})
	end)

	if not ok or result <= 0 then
		if opts.notifications then
			vim.notify("[go-deep] failed to start backend: " .. tostring(result), vim.log.levels.ERROR)
		end
		return
	end

	state.job_id = result
	state.channel = result
end

---@class go_deep.client.Request
---@field public prefix string
---@field public filepath string
---@field public cwd string
---@field public imported_paths table<string, string>
---@field public warm_only boolean
---@field public min_prefix_length integer | nil
---@field public options table | nil

---@class go_deep.client.RequestHandlers
---@field public on_items fun(reply: go_deep.client.Reply) | nil
---@field public on_error fun(err: string) | nil

---@param req go_deep.client.Request
---@param handlers go_deep.client.RequestHandlers | nil
---@return boolean
---@return fun()
local function request(method, req, handlers)
	if not state.channel then
		return false, function() end
	end

	state.next_request_id = state.next_request_id + 1
	local request_id = state.next_request_id
	if handlers then
		state.pending[request_id] = handlers
	end

	local ok, err = pcall(
		vim.rpcnotify,
		state.channel,
		method,
		vim.tbl_extend("keep", {
			request_id = request_id,
		}, req)
	)

	if not ok then
		state.pending[request_id] = nil
		if handlers and handlers.on_error then
			handlers.on_error(tostring(err))
		end
		return false, function() end
	end

	return true, function()
		state.pending[request_id] = nil
	end
end

---@param opts go_deep.Config
---@return table
local function to_request_options(opts)
	return {
		max_items = opts.max_items,
		max_from_same_package = opts.max_from_same_package,
		exclude_imported = opts.exclude_imported_packages,
		exclude_vendored = opts.exclude_vendored_packages,
		exclude_internal = opts.exclude_internal_packages,
		exclude_test_files = opts.exclude_test_files,
	}
end

---@param bufnr integer
---@return boolean
function M.has_gopls(bufnr)
	for _, lsp in ipairs(vim.lsp.get_clients({ bufnr = bufnr })) do
		if lsp.name == "gopls" then
			return true
		end
	end
	return false
end

---@param bufnr integer
---@param prefix string
---@param opts go_deep.Config
---@param warm_only boolean
---@return go_deep.client.Request
local function build_payload(bufnr, prefix, opts, warm_only)
	local imported_paths = treesitter.get_imported_paths(bufnr)
	-- Empty semantic maps must use vim.empty_dict() so msgpack encodes a map.
	if vim.tbl_isempty(imported_paths) then
		imported_paths = vim.empty_dict()
	end

	local req = {
		prefix = prefix,
		filepath = vim.api.nvim_buf_get_name(bufnr),
		cwd = vim.fn.getcwd(),
		imported_paths = imported_paths,
		warm_only = warm_only,
	}
	if opts then
		req.min_prefix_length = opts.min_keyword_length
		req.options = to_request_options(opts)
	end
	return req
end

---@param bufnr integer
---@param prefix string
---@param opts go_deep.Config
function M.warm(bufnr, prefix, opts)
	local req = build_payload(bufnr, prefix, opts, true)
	request("symbols", req, nil)
end

---@param bufnr integer
---@param prefix string
---@param opts go_deep.Config
---@param handlers go_deep.client.RequestHandlers
---@return boolean
---@return fun()
function M.complete(bufnr, prefix, opts, handlers)
	local req = build_payload(bufnr, prefix, opts, false)
	return request("symbols", req, handlers)
end

---@param reply go_deep.client.Reply
function M._dispatch(reply)
	if type(reply) ~= "table" or type(reply.request_id) ~= "number" then
		return
	end

	local pending = state.pending[reply.request_id]
	state.pending[reply.request_id] = nil
	if not pending then
		return
	end

	if pending.on_items then
		pending.on_items(reply)
	end
end

return M
