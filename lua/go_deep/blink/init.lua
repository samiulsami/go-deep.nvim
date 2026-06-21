local client = require("go_deep.client")
local backend = require("go_deep.backend")
local imports = require("go_deep.imports")
local utils = require("go_deep.utils")
local go_deep = require("go_deep")

local completionItemKind = vim.lsp.protocol.CompletionItemKind
local lsp_kind_map = {
	t = completionItemKind.Class,
	i = completionItemKind.Interface,
	f = completionItemKind.Function,
	v = completionItemKind.Variable,
	c = completionItemKind.Constant,
	s = completionItemKind.Struct,
}

local empty_response = {
	items = {},
	is_incomplete_forward = true,
	is_incomplete_backward = true,
}

---@param ctx blink.cmp.Context
---@return string | nil prefix
local function extract_prefix(ctx)
	if not ctx or not ctx.line or not ctx.bounds then
		return nil
	end
	local bounds = ctx.bounds
	if not bounds.start_col or not bounds.length or bounds.length <= 0 then
		return nil
	end
	return ctx.line:sub(bounds.start_col, bounds.start_col + bounds.length - 1)
end

---@param item table go-deep completion item
---@param ctx blink.cmp.Context
---@return lsp.CompletionItem
local function map_item(item, ctx)
	local meta = utils.decode_complete_user_data(item.user_data)
	local label = item.abbr ~= "" and item.abbr or item.word
	local bounds = ctx.bounds
	return {
		label = label,
		filterText = item.word,
		textEdit = {
			newText = label,
			range = {
				start = { line = ctx.cursor[1] - 1, character = bounds.start_col - 1 },
				["end"] = { line = ctx.cursor[1] - 1, character = bounds.start_col - 1 + bounds.length },
			},
		},
		insertTextFormat = vim.lsp.protocol.InsertTextFormat.PlainText,
		kind = lsp_kind_map[item.kind],
		detail = item.menu,
		documentation = item.info ~= "" and { kind = "markdown", value = item.info } or nil,
		data = { go_deep = meta },
	}
end

---@class go_deep.blink.Source
local Source = {}

function Source.new(opts)
	return setmetatable({ opts = opts or {} }, { __index = Source })
end

function Source:enabled(ctx)
	local bufnr = ctx and ctx.bufnr or vim.api.nvim_get_current_buf()
	return client.has_gopls(bufnr)
end

function Source:get_completions(ctx, callback)
	local opts = go_deep.resolve_request_config(self.opts)
	if not opts then
		callback(empty_response)
		return function() end
	end
	if not opts.workspace_symbols and not opts.index then
		callback(empty_response)
		return function() end
	end
	if opts.workspace_symbols and not client.has_gopls(ctx.bufnr) then
		callback(empty_response)
		return function() end
	end
	if not client.is_running() then
		backend.ensure(go_deep.resolve_config())
		if not client.is_running() then
			callback(empty_response)
			return function() end
		end
	end

	local prefix = extract_prefix(ctx)
	if not prefix or not utils.is_valid_query(prefix, opts.min_keyword_length) then
		callback(empty_response)
		return function() end
	end

	local ok, cancel = client.complete(ctx.bufnr, prefix, opts, {
		on_items = function(reply)
			local items = {}
			for _, it in ipairs(reply.items or {}) do
				items[#items + 1] = map_item(it, ctx)
			end
			callback({
				items = items,
				is_incomplete_forward = true,
				is_incomplete_backward = true,
			})
		end,
		on_error = function(_)
			callback(empty_response)
		end,
	})
	if not ok then
		callback(empty_response)
		return function() end
	end
	return cancel
end

function Source:execute(ctx, item, callback, default_implementation)
	default_implementation()
	local meta = item and item.data and item.data.go_deep
	imports.apply(ctx.bufnr, meta and meta.import_path, meta and meta.package_alias, true)
	callback()
end

return Source
