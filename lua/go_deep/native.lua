local client = require("go_deep.client")
local utils = require("go_deep.utils")

local M = {}

---@class go_deep.native.Context
---@field public bufnr integer
---@field public row integer
---@field public start_col integer
---@field public prefix string

local state = {
	start_col = {},
	current = {},
	refreshing = {},
	config = {},
}

local augroup = vim.api.nvim_create_augroup("go_deep", { clear = false })
local native_complete_source = "Fv:lua.go_deep_completefunc"

---@return boolean
local function autocomplete_enabled()
	local ok, value = pcall(function()
		return vim.o.autocomplete
	end)
	return ok and value or false
end

---@param bufnr integer
local function ensure_complete_source(bufnr)
	vim.api.nvim_buf_call(bufnr, function()
		local complete = vim.opt_local.complete:get()
		if vim.list_contains(complete, native_complete_source) then
			return
		end
		vim.opt_local.complete:prepend({ native_complete_source })
	end)
end

---@param bufnr integer
local function clear(bufnr)
	state.current[bufnr] = nil
	state.start_col[bufnr] = nil
end

---@param bufnr integer
---@param prefix string
---@param start_col integer
---@return go_deep.native.Context
local function new_context(bufnr, prefix, start_col)
	local row = unpack(vim.api.nvim_win_get_cursor(0))
	return { bufnr = bufnr, row = row, start_col = start_col, prefix = prefix }
end

---@param ctx go_deep.native.Context
---@return boolean
local function context_matches(ctx)
	if vim.api.nvim_get_mode().mode:sub(1, 1) ~= "i" then
		return false
	end
	if vim.api.nvim_get_current_buf() ~= ctx.bufnr then
		return false
	end
	local row, col = unpack(vim.api.nvim_win_get_cursor(0))
	if row ~= ctx.row or ctx.start_col < 0 or ctx.start_col > col then
		return false
	end
	local prefix, current_start_col = utils.cursor_prefix()
	if current_start_col ~= ctx.start_col then
		return false
	end
	return utils.valid_prefix(prefix) and prefix == ctx.prefix
end

---@param ctx go_deep.native.Context
---@param items table[]
local function apply_completion(ctx, items)
	if type(ctx) ~= "table" or type(items) ~= "table" then
		return
	end

	vim.schedule(function()
		if state.current[ctx.bufnr] ~= ctx or not context_matches(ctx) then
			return
		end
		state.refreshing[ctx.bufnr] = true
		vim.fn.complete(ctx.start_col + 1, items)
		vim.schedule(function()
			state.refreshing[ctx.bufnr] = nil
		end)
	end)
end

---@param bufnr integer
---@param prefix string
---@param start_col integer
---@param opts go_deep.Config
local function complete(bufnr, prefix, start_col, opts)
	local ctx = new_context(bufnr, prefix, start_col)
	state.current[bufnr] = ctx
	client.complete(bufnr, prefix, opts, {
		on_items = function(reply)
			if state.current[bufnr] ~= ctx then
				return
			end
			apply_completion(ctx, reply.items)
		end,
	})
end

---@param findstart integer
---@param base string
---@return integer | table
function M.completefunc(findstart, base)
	local bufnr = vim.api.nvim_get_current_buf()
	local go_deep = require("go_deep")
	local buf_config = state.config[bufnr] or require("go_deep").resolve_config()
	if findstart == 1 then
		local _, start_col = utils.cursor_prefix()
		state.start_col[bufnr] = start_col
		return start_col
	end
	if not client.has_gopls(bufnr) then
		return { words = {}, refresh = "always" }
	end
	local opts = go_deep.resolve_request_config(nil, buf_config)
	if not opts then
		return { words = {}, refresh = "always" }
	end

	if not utils.is_valid_query(base, opts.min_keyword_length) then
		return { words = {}, refresh = "always" }
	end

	local start_col = state.start_col[bufnr]
	if not start_col or start_col < 0 then
		local _, recalculated = utils.cursor_prefix()
		start_col = recalculated
	end
	if start_col < 0 then
		return { words = {}, refresh = "always" }
	end

	if client.is_running() then
		complete(bufnr, base, start_col, opts)
		return -2
	end

	if M.attach(bufnr, buf_config) then
		complete(bufnr, base, start_col, opts)
		return -2
	end
	return { words = {}, refresh = "always" }
end

_G.go_deep_completefunc = M.completefunc

---@param bufnr integer
---@param user_opts go_deep.Config
function M.attach(bufnr, user_opts)
	local go_deep = require("go_deep")
	state.config[bufnr] = user_opts or go_deep.resolve_config()

	local backend = require("go_deep.backend")
	backend.ensure(go_deep.resolve_config(state.config[bufnr]))
	if not client.is_running() then
		return false
	end

	ensure_complete_source(bufnr)

	vim.api.nvim_clear_autocmds({ group = augroup, buffer = bufnr })

	vim.api.nvim_create_autocmd("CompleteDone", {
		group = augroup,
		buffer = bufnr,
		callback = function()
			clear(bufnr)
			local imports = require("go_deep.imports")
			imports.on_complete_done(bufnr)
		end,
	})

	vim.api.nvim_create_autocmd({ "CompleteDonePre", "InsertLeave" }, {
		group = augroup,
		buffer = bufnr,
		callback = function()
			clear(bufnr)
		end,
	})

	local on_text_changed = function(was_pum_visible, selected)
		local go_deep = require("go_deep")
		if not client.has_gopls(bufnr) then
			return
		end
		local opts = go_deep.resolve_request_config(nil, state.config[bufnr])
		if not opts then
			return
		end
		local prefix, start_col = utils.cursor_prefix()
		if not utils.is_valid_query(prefix, opts.min_keyword_length) then
			return
		end
		if start_col < 0 then
			return
		end

		if autocomplete_enabled() or (was_pum_visible and selected == -1) then
			complete(bufnr, prefix, start_col, opts)
		else
			client.warm(bufnr, prefix, opts)
		end
	end

	vim.api.nvim_create_autocmd("InsertCharPre", {
		group = augroup,
		buffer = bufnr,
		callback = function()
			local char = vim.v.char
			if not char or char == "" or not char:match("[%w_]") then
				return
			end

			local was_pum_visible = vim.fn.pumvisible() == 1
			local info = vim.fn.complete_info({ "selected" })
			local selected = tonumber(info.selected) or -1
			vim.schedule(function()
				on_text_changed(was_pum_visible, selected)
			end)
		end,
	})

	return true
end

return M
