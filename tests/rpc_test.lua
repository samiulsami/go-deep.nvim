vim.g.go_deep = {
	notifications = false,
	min_keyword_length = 1,
	max_items = 5,
}

local original_get_clients = vim.lsp.get_clients
local passed = 0
local total = 0

local function check(name, ok)
	total = total + 1
	if ok then
		passed = passed + 1
		print("PASS: " .. name)
	else
		print("FAIL: " .. name)
	end
end

local ok, err = pcall(function()
	print("Starting RPC test...")

	local bufnr = vim.api.nvim_get_current_buf()

	-- Stub LSP detection.
	vim.lsp.get_clients = function(opts)
		if opts and (opts.bufnr == 0 or opts.bufnr == bufnr) then
			return { { name = "gopls" } }
		end
		return original_get_clients(opts)
	end

	-- Write a Go buffer so tree-sitter and prefix extraction work.
	vim.api.nvim_buf_set_lines(0, 0, -1, false, {
		"package main",
		"",
		"import (",
		"\t\"fmt\"",
		")",
		"",
		"func main() {",
		"\ttestSomething",
		"}",
	})
	vim.bo.filetype = "go"
	vim.api.nvim_win_set_cursor(0, { 8, 5 })

	require("go_deep").attach_to_buffer(0)

	-- Test 1: findstart
	local start_col = _G.go_deep_completefunc(1, "test")
	check("findstart for 'test' at col 5 returns col 1", start_col == 1)

	-- Test 2: async return
	local result = _G.go_deep_completefunc(0, "test")
	check("completefunc returns -2 (async)", result == -2)

	-- Test 3: client.has_gopls
	local client = require("go_deep.client")
	check("has_gopls returns true for gopls buffer", client.has_gopls(bufnr) == true)

	-- Test 4: no supported LSP -> false
	local stub_clients = vim.lsp.get_clients
	vim.lsp.get_clients = function() return {} end
	check("has_gopls returns false with no supported LSP", client.has_gopls(bufnr) == false)
	vim.lsp.get_clients = stub_clients

	-- Test 5: dispatch with valid reply
	local test_reply = {
		request_id = 42,
		items = {
			{ word = "NewChaCha8", abbr = "NewChaCha8", kind = "f", menu = "crypto", info = "" },
		},
	}
	require("go_deep.client")._dispatch(test_reply)
	check("_dispatch does not error on valid reply", true)

	-- Test 6: dispatch with malformed reply (missing request_id)
	require("go_deep.client")._dispatch({})
	check("_dispatch handles malformed reply gracefully", true)

	-- Test 7: dispatch with nil
	require("go_deep.client")._dispatch(nil)
	check("_dispatch handles nil reply gracefully", true)

	print(string.format("\n%d/%d tests passed", passed, total))
end)

vim.lsp.get_clients = original_get_clients
vim.cmd("bwipeout!")

if not ok then
	print("RPC test crashed: " .. tostring(err))
	vim.cmd("cquit 1")
elseif passed < total then
	print("Some tests failed")
	vim.cmd("cquit 1")
else
	print("All RPC tests passed!")
	vim.cmd("qa!")
end
