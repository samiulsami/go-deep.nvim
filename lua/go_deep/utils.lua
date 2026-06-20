---@class go_deep.utils
local utils = {}

---@param prefix string
---@return boolean
function utils.valid_prefix(prefix)
	return prefix ~= "" and not prefix:match("[^%w_]")
end

---@param prefix string
---@param min_length integer
---@return boolean
function utils.is_valid_query(prefix, min_length)
	return #prefix >= min_length and utils.valid_prefix(prefix)
end

---@return string
---@return integer
function utils.cursor_prefix()
	local pos = vim.api.nvim_win_get_cursor(0)
	if #pos < 2 then
		return "", -3
	end

	local col = pos[2]
	local start_col = col

	local line = vim.api.nvim_get_current_line()

	while start_col > 0 and line:sub(start_col, start_col):match("[%w_]") do
		start_col = start_col - 1
	end

	if start_col > 0 then
		start_col = start_col + 1
	end

	local prefix = line:sub(start_col, col)
	if not utils.valid_prefix(prefix) then
		return prefix, -3
	end
	return prefix, col - #prefix
end

---@param user_data string | table | nil
---@return go_deep.UserData | nil
function utils.decode_complete_user_data(user_data)
	if type(user_data) == "table" then
		return user_data.go_deep
	end

	if type(user_data) ~= "string" or user_data == "" then
		return nil
	end

	local ok, decoded = pcall(vim.json.decode, user_data)
	if not ok or type(decoded) ~= "table" then
		return nil
	end

	return decoded.go_deep
end

return utils
