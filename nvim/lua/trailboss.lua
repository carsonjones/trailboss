local M = {}

M.config = {
  source_path = vim.fn.expand("~/.local/share/trailboss/comments.jsonl"),
  keys = {
    act = "<leader>tx",
    ask = "<leader>ta",
  },
}

local function get_visual_selection()
  local start_pos = vim.fn.getpos("'<")
  local end_pos = vim.fn.getpos("'>")
  local start_line = start_pos[2]
  local end_line = end_pos[2]
  local lines = vim.api.nvim_buf_get_lines(0, start_line - 1, end_line, false)
  return {
    text = table.concat(lines, "\n"),
    start_line = start_line,
    end_line = end_line,
  }
end

local function send(type, sel, steer)
  local path = vim.fn.expand("%:p")
  local charset = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
  local id = ""
  for _ = 1, 6 do
    local i = math.random(1, #charset)
    id = id .. charset:sub(i, i)
  end

  local body = sel.text
  if steer and steer ~= "" then
    body = body .. "\n\nContext: " .. steer
  end

  local record = vim.fn.json_encode({
    id = id,
    type = type,
    path = path,
    cwd = vim.fn.getcwd(),
    line = sel.start_line,
    end_line = sel.end_line,
    body = body,
  })

  local f = io.open(vim.fn.expand(M.config.source_path), "a")
  if not f then
    vim.notify("trailboss: could not open " .. M.config.source_path, vim.log.levels.ERROR)
    return
  end
  f:write(record .. "\n")
  f:close()
  vim.notify(string.format("trailboss: %s queued (%s:%d)", type, vim.fn.expand("%:t"), sel.start_line))
end

local function prompt(type)
  local sel = get_visual_selection()
  local sent = false
  vim.ui.input({ prompt = type .. ": " }, function(input)
    if input == nil or sent then return end
    sent = true
    send(type, sel, input)
  end)
end

function M.act()
  prompt("act")
end

function M.answer()
  prompt("ask")
end

function M.setup(opts)
  M.config = vim.tbl_deep_extend("force", M.config, opts or {})

  vim.keymap.set("v", M.config.keys.act, function()
    vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes("<Esc>", true, false, true), "x", false)
    M.act()
  end, { desc = "trailboss: act on selection" })

  vim.keymap.set("v", M.config.keys.ask, function()
    vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes("<Esc>", true, false, true), "x", false)
    M.answer()
  end, { desc = "trailboss: ask about selection" })
end

return M
