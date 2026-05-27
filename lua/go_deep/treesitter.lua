---@class go_deep.treesitter
local treesitter = {}

---@param bufnr integer
---@return TSNode | nil
---Parse and return the Go root node.
local function get_root_node(bufnr)
	if bufnr == nil then
		return nil
	end

	if not vim.api.nvim_buf_is_loaded(bufnr) then
		vim.fn.bufload(bufnr)
	end

	local ok, parser = pcall(vim.treesitter.get_parser, bufnr, "go")
	if not ok or parser == nil then
		return nil
	end
	local root = nil
	local ok2, parsed = pcall(parser.parse, parser)
	if ok2 and parsed ~= nil then
		root = parsed[1]:root()
	end
	return root
end

---@param bufnr integer
---@param package_alias string | nil
---@param import_path string
---Insert import.
function treesitter.add_import_statement(bufnr, package_alias, import_path)
	local root = get_root_node(bufnr)
	if root == nil then
		return
	end

	if not package_alias then
		package_alias = ""
	else
		package_alias = package_alias .. " "
	end

	local import_node = nil
	for i = 0, root:named_child_count() - 1 do
		local node = root:named_child(i)
		if node and node:type() == "import_declaration" then
			import_node = node
		end
	end

	if import_node then
		local start_row, _, end_row, _ = import_node:range()
		if import_node:named_child_count() == 1 then
			local child = import_node:named_child(0)
			if not child then
				return
			end

			local type = child:type()
			if type == "interpreted_string_literal" or type == "raw_string_literal" or type == "import_spec" then
				vim.api.nvim_buf_set_lines(bufnr, start_row, end_row + 1, false, {
					"import (",
					"\t" .. vim.treesitter.get_node_text(child, bufnr),
					"\t" .. package_alias .. '"' .. import_path .. '"',
					")",
				})
				return
			end
		end

		local lines = vim.api.nvim_buf_get_lines(bufnr, start_row, end_row + 1, false)
		if not lines[#lines]:match("^%s*%)") then
			return
		end

		table.insert(lines, #lines, "\t" .. package_alias .. '"' .. import_path .. '"')
		vim.api.nvim_buf_set_lines(bufnr, start_row, end_row + 1, false, lines)
	else
		local insert_line = 0
		for i, line in ipairs(vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)) do
			if line:match("^package%s+") then
				insert_line = i
				break
			end
		end

		vim.api.nvim_buf_set_lines(bufnr, insert_line, insert_line, false, {
			"",
			"import (",
			"\t" .. package_alias .. '"' .. import_path .. '"',
			")",
		})
	end
end

---@param bufnr integer
---@return table<string, string>
---Collect imported paths.
function treesitter.get_imported_paths(bufnr)
	local root = get_root_node(bufnr)
	if root == nil then
		return {}
	end

	local import_nodes = {}
	for i = 0, root:named_child_count() - 1 do
		local node = root:named_child(i)
		if node and node:type() == "import_declaration" then
			import_nodes[#import_nodes + 1] = node
		end
	end
	if #import_nodes == 0 then
		return {}
	end

	local imported_paths = {}
	---@param spec TSNode?
	local process_import_spec = function(spec)
		local path_node = spec and spec:field("path")[1]
		local name_node = spec and spec:field("name")[1]
		if path_node then
			local text = vim.treesitter.get_node_text(path_node, bufnr)
			if text then
				text = text:gsub('^["`]+', ""):gsub('["`]+$', "")
				local pkg_alias = name_node and vim.treesitter.get_node_text(name_node, bufnr)
				pkg_alias = pkg_alias or text:match("([^/]+)$")
				imported_paths[text] = pkg_alias
			end
		end
	end

	for _, import_node in ipairs(import_nodes) do
		for j = 0, import_node:named_child_count() - 1 do
			local child = import_node:named_child(j)
			if not child then
				goto continue
			end

			local type = child:type()
			if type == "import_spec" and child:named_child_count() > 0 then
				process_import_spec(child)
				goto continue
			end

			if type == "import_spec_list" then
				for k = 0, child:child_count() - 1 do
					process_import_spec(child:named_child(k))
				end
				break
			end

			::continue::
		end
	end

	return imported_paths
end

return treesitter
