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

local function get_current_file()
  return {
    text = "",
    start_line = 1,
    end_line = vim.api.nvim_buf_line_count(0),
  }
end

local function notify(msg, level)
  vim.notify(msg, level or vim.log.levels.INFO, { title = "trailboss" })
end

local function send(type, sel, steer)
  local path = vim.fn.expand("%:p")
  if path == "" then
    notify("current buffer has no file path", vim.log.levels.ERROR)
    return
  end

  local charset = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
  local id = ""
  for _ = 1, 6 do
    local i = math.random(1, #charset)
    id = id .. charset:sub(i, i)
  end

  local body = sel.text
  if steer and steer ~= "" then
    if body ~= "" then
      body = body .. "\n\nContext: " .. steer
    else
      body = steer
    end
  end
  if body == "" then
    notify("prompt required", vim.log.levels.ERROR)
    return
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
    notify("could not open " .. M.config.source_path, vim.log.levels.ERROR)
    return
  end
  f:write(record .. "\n")
  f:close()
  notify(string.format("%s queued (%s:%d)", type, vim.fn.expand("%:t"), sel.start_line))
end

local function prompt(type, sel)
  local sent = false
  vim.ui.input({ prompt = type .. ": " }, function(input)
    if input == nil or sent then return end
    sent = true
    send(type, sel, input)
  end)
end

function M.act()
  prompt("act", get_current_file())
end

function M.ask()
  prompt("ask", get_current_file())
end

function M.act_selection()
  prompt("act", get_visual_selection())
end

function M.ask_selection()
  prompt("ask", get_visual_selection())
end

M.answer = M.ask
M.answer_selection = M.ask_selection

function M.setup(opts)
  M.config = vim.tbl_deep_extend("force", M.config, opts or {})

  vim.keymap.set("n", M.config.keys.act, M.act, { desc = "trailboss: act on current file" })
  vim.keymap.set("n", M.config.keys.ask, M.ask, { desc = "trailboss: ask about current file" })

  vim.keymap.set("v", M.config.keys.act, function()
    vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes("<Esc>", true, false, true), "x", false)
    M.act_selection()
  end, { desc = "trailboss: act on selection" })

  vim.keymap.set("v", M.config.keys.ask, function()
    vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes("<Esc>", true, false, true), "x", false)
    M.ask_selection()
  end, { desc = "trailboss: ask about selection" })
end

return M
