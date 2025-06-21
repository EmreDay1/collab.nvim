local config = require('config')

local M = {}

-- Compatibility layer for standalone testing
local vim = vim or {
  log = { levels = { DEBUG = 1, INFO = 2, WARN = 3, ERROR = 4 } },
  api = { nvim_create_user_command = function() end },
  notify = function(msg) print("NOTIFY: " .. msg) end,
  ui = { input = function(opts, callback) callback("test") end },
  fn = { has = function() return 1 end, setreg = function() end },
  tbl_count = function(t) local c = 0; for _ in pairs(t) do c = c + 1 end; return c end,
  o = { columns = 80, lines = 24 },
}

-- UI state
M.state = {
  status_line_active = false,
  notifications_enabled = true,
  session_info_visible = false,
}

-- Notification levels
M.levels = {
  DEBUG = vim.log.levels.DEBUG,
  INFO = vim.log.levels.INFO,
  WARN = vim.log.levels.WARN,
  ERROR = vim.log.levels.ERROR,
}

-- Initialize UI components
function M.init()
  M.setup_commands()
  M.setup_status_line()
  M.state.notifications_enabled = true
end

-- Setup user commands
function M.setup_commands()
  -- Create session command
  vim.api.nvim_create_user_command('CollabCreate', function()
    require('collab').create_session()
  end, {
    desc = 'Create a new collaboration session'
  })

  -- Join session command
  vim.api.nvim_create_user_command('CollabJoin', function(opts)
    local session_id = opts.args
    if session_id == '' then
      M.prompt_session_id(function(id)
        if id then
          require('collab').join_session(id)
        end
      end)
    else
      require('collab').join_session(session_id)
    end
  end, {
    nargs = '?',
    desc = 'Join collaboration session',
    complete = function()
      -- TODO: Add session ID completion from history
      return {}
    end
  })

  -- Leave session command  
  vim.api.nvim_create_user_command('CollabLeave', function()
    require('collab').leave_session()
  end, {
    desc = 'Leave current collaboration session'
  })

  -- Request control command
  vim.api.nvim_create_user_command('CollabControl', function()
    require('collab').request_control()
  end, {
    desc = 'Request editing control'
  })

  -- Release control command
  vim.api.nvim_create_user_command('CollabRelease', function()
    require('collab').release_control()
  end, {
    desc = 'Release editing control'
  })

  -- Show session info command
  vim.api.nvim_create_user_command('CollabInfo', function()
    M.show_session_info()
  end, {
    desc = 'Show collaboration session information'
  })

  -- Show plugin status command
  vim.api.nvim_create_user_command('CollabStatus', function()
    M.show_status()
  end, {
    desc = 'Show collab.nvim status'
  })
end

-- Setup status line integration
function M.setup_status_line()
  local opts = config.get()
  
  if opts.show_status_line then
    -- TODO: Integrate with status line plugins
    M.state.status_line_active = true
  end
end

-- Show notification to user
function M.notify(message, level, opts)
  if not M.state.notifications_enabled then
    return
  end
  
  level = level or M.levels.INFO
  opts = opts or {}
  
  -- Always log to config system
  local log_level = level == M.levels.DEBUG and "debug" 
                 or level == M.levels.INFO and "info"
                 or level == M.levels.WARN and "warn" 
                 or "error"
  config.log(log_level, message)
  
  -- Show user notification
  local title = opts.title or "collab.nvim"
  
  if vim.notify then
    vim.notify(message, level, {
      title = title,
      timeout = opts.timeout,
    })
  else
    -- Fallback for older Neovim versions
    local prefix = level == M.levels.ERROR and "ERROR: "
                or level == M.levels.WARN and "WARN: "
                or ""
    print(title .. ": " .. prefix .. message)
  end
end

-- Show success message
function M.success(message, opts)
  M.notify(message, M.levels.INFO, opts)
end

-- Show warning message  
function M.warn(message, opts)
  M.notify(message, M.levels.WARN, opts)
end

-- Show error message
function M.error(message, opts)
  M.notify(message, M.levels.ERROR, opts)
end

-- Show debug message
function M.debug(message, opts)
  if config.is_debug() then
    M.notify(message, M.levels.DEBUG, opts)
  end
end

-- Prompt user for session ID
function M.prompt_session_id(callback)
  vim.ui.input({
    prompt = 'Enter session ID: ',
    default = '',
  }, function(input)
    if input and input ~= '' then
      callback(input)
    else
      callback(nil)
    end
  end)
end

-- Show session creation success with session ID
function M.show_session_created(session_id, user_id)
  M.success(string.format("Session created: %s", session_id))
  
  -- Copy to clipboard if available
  if vim.fn.has('clipboard') == 1 then
    vim.fn.setreg('+', session_id)
    M.debug("Session ID copied to clipboard")
  end
  
  -- Show in a floating window for better visibility
  M.show_session_details({
    session_id = session_id,
    user_id = user_id,
    status = "created",
    peers = 1,
  })
end

-- Show session join success
function M.show_session_joined(session_id, peers, content_length)
  local peer_count = type(peers) == "table" and #peers or (peers or 0)
  M.success(string.format("Joined session with %d peer(s)", peer_count))
  
  M.show_session_details({
    session_id = session_id,
    status = "joined",
    peers = peer_count,
    content_length = content_length,
  })
end

-- Show peer joined notification
function M.show_peer_joined(peer)
  local name = peer.name or peer.user_id or "Unknown"
  M.success(string.format("Peer joined: %s", name))
end

-- Show peer left notification
function M.show_peer_left(user_id)
  M.warn(string.format("Peer left: %s", user_id))
end

-- Show control status change
function M.show_control_status(has_control, controller)
  if has_control then
    M.success("You now have editing control")
  elseif controller then
    M.notify(string.format("Control is with: %s", controller), M.levels.INFO)
  else
    M.notify("No one has control", M.levels.INFO)
  end
end

-- Show session details in floating window
function M.show_session_details(details)
  local lines = {
    "=== Collaboration Session ===",
    "",
    "Session ID: " .. (details.session_id or "Unknown"),
    "Status: " .. (details.status or "Unknown"),
    "Peers: " .. (details.peers or 0),
  }
  
  if details.user_id then
    table.insert(lines, "Your ID: " .. details.user_id)
  end
  
  if details.content_length then
    table.insert(lines, "Content: " .. details.content_length .. " chars")
  end
  
  table.insert(lines, "")
  table.insert(lines, "Press 'q' to close")
  
  M.show_floating_window(lines, "Session Info")
end

-- Show current session information
function M.show_session_info()
  local ok, collab = pcall(require, 'collab')
  if not ok or not collab.get_state then
    M.warn("Plugin not loaded or initialized")
    return
  end
  
  local state = collab.get_state()
  local plugin_state = state.plugin_state
  
  if not plugin_state.session_id then
    M.warn("Not in a collaboration session")
    return
  end
  
  local peer_count = vim.tbl_count(plugin_state.peers)
  local control_status = plugin_state.has_control and "You" or (plugin_state.current_controller or "None")
  
  local lines = {
    "=== Current Session ===",
    "",
    "Session ID: " .. plugin_state.session_id,
    "User ID: " .. (plugin_state.user_id or "Unknown"),
    "Peers: " .. peer_count,
    "Control: " .. control_status,
    "",
    "=== Connected Peers ===",
  }
  
  if peer_count > 0 then
    for user_id, peer in pairs(plugin_state.peers) do
      local name = peer.name or user_id
      table.insert(lines, "- " .. name)
    end
  else
    table.insert(lines, "No other peers connected")
  end
  
  table.insert(lines, "")
  table.insert(lines, "Press 'q' to close")
  
  M.show_floating_window(lines, "Session Info")
end

-- Show plugin status
function M.show_status()
  -- Try to get state safely
  local ok, collab = pcall(require, 'collab')
  if not ok then
    M.show_floating_window({
      "=== collab.nvim Status ===",
      "",
      "Plugin: Not loaded",
      "Error: " .. tostring(collab),
      "",
      "Press 'q' to close"
    }, "Plugin Status")
    return
  end
  
  if not collab.get_state then
    M.show_floating_window({
      "=== collab.nvim Status ===", 
      "",
      "Plugin: Loaded but not initialized",
      "Run: require('collab').setup()",
      "",
      "Press 'q' to close"
    }, "Plugin Status")
    return
  end
  
  local state = collab.get_state()
  
  local lines = {
    "=== collab.nvim Status ===",
    "",
    "Plugin: " .. (state.plugin_state.initialized and "Initialized" or "Not initialized"),
    "Go Process: " .. (state.p2p_status.running and "Running" or "Stopped"),
    "Binary: " .. (state.p2p_status.binary_path or "Unknown"),
    "",
    "Session: " .. (state.plugin_state.session_id or "None"),
    "Control: " .. (state.plugin_state.has_control and "You" or "Other/None"),
    "",
    "Pending Operations: " .. (state.p2p_status.pending_callbacks or 0),
    "Debug Mode: " .. (config.is_debug() and "Enabled" or "Disabled"),
    "",
    "Press 'q' to close"
  }
  
  M.show_floating_window(lines, "Plugin Status")
end

-- Show floating window with content
function M.show_floating_window(lines, title)
  -- Calculate window size
  local width = 0
  for _, line in ipairs(lines) do
    width = math.max(width, #line)
  end
  width = math.min(width + 4, vim.o.columns - 10)
  local height = math.min(#lines + 2, vim.o.lines - 10)
  
  -- Create buffer
  local buf = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  vim.api.nvim_buf_set_option(buf, 'modifiable', false)
  vim.api.nvim_buf_set_option(buf, 'filetype', 'collab-info')
  
  -- Calculate position (center of screen)
  local row = math.floor((vim.o.lines - height) / 2)
  local col = math.floor((vim.o.columns - width) / 2)
  
  -- Create window
  local win = vim.api.nvim_open_win(buf, true, {
    relative = 'editor',
    row = row,
    col = col,
    width = width,
    height = height,
    border = 'rounded',
    title = title and (' ' .. title .. ' ') or nil,
    title_pos = 'center',
  })
  
  -- Set window options
  vim.api.nvim_win_set_option(win, 'wrap', false)
  vim.api.nvim_win_set_option(win, 'cursorline', true)
  
  -- Set up key mappings to close window
  local close_keys = { 'q', '<Esc>', '<CR>' }
  for _, key in ipairs(close_keys) do
    vim.api.nvim_buf_set_keymap(buf, 'n', key, '<cmd>close<cr>', {
      nowait = true,
      noremap = true,
      silent = true
    })
  end
end

-- Enable/disable notifications
function M.toggle_notifications()
  M.state.notifications_enabled = not M.state.notifications_enabled
  local status = M.state.notifications_enabled and "enabled" or "disabled"
  print("collab.nvim notifications " .. status)
end

-- Get UI state
function M.get_state()
  return M.state
end

return M
