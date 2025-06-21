local config = require('config')
local p2p = require('p2p')

local M = {}

-- Plugin state
M.state = {
  initialized = false,
  session_id = nil,
  user_id = nil,
  peers = {},
  has_control = false,
  current_controller = nil,
}

-- Setup the plugin with user configuration
function M.setup(opts)
  -- Initialize configuration
  config.setup(opts or {})
  
  -- Initialize P2P manager
  p2p.init()
  
  -- Set up P2P event handlers
  p2p.set_handlers({
    on_message = M.handle_p2p_message,
    on_error = M.handle_p2p_error,
    on_disconnect = M.handle_p2p_disconnect,
  })
  
  -- Start the Go process
  local success = pcall(p2p.start)
  if not success then
    config.log("error", "Failed to start Go process")
    return false
  end
  
  M.state.initialized = true
  config.log("info", "collab.nvim initialized successfully")
  
  -- Set up autocmds for cleanup
  M.setup_autocmds()
  
  return true
end

-- Handle messages from Go process
function M.handle_p2p_message(message)
  config.log("debug", "Handling message: " .. message.type)
  
  if message.type == "session_created" then
    M.handle_session_created(message.data)
  elseif message.type == "session_joined" then
    M.handle_session_joined(message.data)
  elseif message.type == "peer_joined" then
    M.handle_peer_joined(message.data)
  elseif message.type == "peer_left" then
    M.handle_peer_left(message.data)
  elseif message.type == "control_status" then
    M.handle_control_status(message.data)
  elseif message.type == "error" then
    M.handle_error_message(message.data)
  elseif message.type == "status" then
    M.handle_status_message(message.data)
  else
    config.log("warn", "Unhandled message type: " .. message.type)
  end
end

-- Handle P2P errors
function M.handle_p2p_error(error_msg)
  config.log("error", "P2P error: " .. error_msg)
  vim.notify("collab.nvim: " .. error_msg, vim.log.levels.ERROR)
end

-- Handle P2P disconnection
function M.handle_p2p_disconnect(code, signal)
  config.log("warn", "Go process disconnected")
  M.state.session_id = nil
  M.state.peers = {}
  M.state.has_control = false
  
  vim.notify("collab.nvim: Connection lost", vim.log.levels.WARN)
  
  -- Auto-restart if configured
  if config.get().auto_restart then
    config.log("info", "Auto-restarting Go process")
    vim.defer_fn(function()
      p2p.restart()
    end, 2000)
  end
end

-- Session event handlers
function M.handle_session_created(data)
  M.state.session_id = data.session_id
  M.state.user_id = data.user_id
  M.state.has_control = true -- Creator starts with control
  
  config.log("info", "Session created: " .. data.session_id)
  vim.notify("Session created: " .. data.session_id, vim.log.levels.INFO)
  
  -- Copy session ID to clipboard if available
  if vim.fn.has('clipboard') == 1 then
    vim.fn.setreg('+', data.session_id)
    vim.notify("Session ID copied to clipboard", vim.log.levels.INFO)
  end
end

function M.handle_session_joined(data)
  M.state.session_id = data.session_id or M.state.session_id
  M.state.user_id = data.user_id
  M.state.peers = data.peers or {}
  M.state.has_control = false -- Joiners don't start with control
  
  config.log("info", "Joined session with " .. #M.state.peers .. " peers")
  vim.notify("Joined collaboration session", vim.log.levels.INFO)
end

function M.handle_peer_joined(data)
  local peer = data.peer
  M.state.peers[peer.user_id] = peer
  
  config.log("info", "Peer joined: " .. peer.user_id)
  vim.notify("Peer joined: " .. (peer.name or peer.user_id), vim.log.levels.INFO)
end

function M.handle_peer_left(data)
  local user_id = data.user_id
  M.state.peers[user_id] = nil
  
  config.log("info", "Peer left: " .. user_id)
  vim.notify("Peer left: " .. user_id, vim.log.levels.INFO)
end

function M.handle_control_status(data)
  M.state.current_controller = data.current_controller
  M.state.has_control = data.has_control
  
  if data.has_control then
    config.log("info", "You now have control")
    vim.notify("You have editing control", vim.log.levels.INFO)
  else
    local controller = data.current_controller or "someone else"
    config.log("info", "Control is with: " .. controller)
    vim.notify("Control is with: " .. controller, vim.log.levels.INFO)
  end
end

function M.handle_error_message(data)
  local error_msg = data.message or "Unknown error"
  config.log("error", "Go process error: " .. error_msg)
  vim.notify("collab.nvim error: " .. error_msg, vim.log.levels.ERROR)
end

function M.handle_status_message(data)
  local status_msg = data.status or "Unknown status"
  config.log("info", "Status: " .. status_msg)
  
  if data.info then
    config.log("debug", "Status info: " .. data.info)
  end
end

-- Public API functions

-- Create a new collaboration session
function M.create_session()
  if not M.state.initialized then
    vim.notify("collab.nvim not initialized. Run require('collab').setup() first", vim.log.levels.ERROR)
    return
  end
  
  local file_path = vim.fn.expand('%:p')
  if file_path == '' then
    vim.notify("Please open a file first", vim.log.levels.ERROR)
    return
  end
  
  -- Get current buffer content
  local lines = vim.api.nvim_buf_get_lines(0, 0, -1, false)
  local content = table.concat(lines, '\n')
  
  config.log("info", "Creating session for file: " .. file_path)
  
  p2p.create_session(file_path, content, function(response)
    if response.type == "session_created" then
      M.handle_session_created(response.data)
    elseif response.type == "error" then
      M.handle_error_message(response.data)
    end
  end)
end

-- Join an existing collaboration session
function M.join_session(session_id)
  if not M.state.initialized then
    vim.notify("collab.nvim not initialized. Run require('collab').setup() first", vim.log.levels.ERROR)
    return
  end
  
  if not session_id or session_id == "" then
    -- Prompt for session ID
    vim.ui.input({ prompt = "Enter session ID: " }, function(input)
      if input and input ~= "" then
        M.join_session(input)
      end
    end)
    return
  end
  
  config.log("info", "Joining session: " .. session_id)
  
  p2p.join_session(session_id, function(response)
    if response.type == "session_joined" then
      M.handle_session_joined(response.data)
    elseif response.type == "error" then
      M.handle_error_message(response.data)
    end
  end)
end

-- Leave current session
function M.leave_session()
  if not M.state.session_id then
    vim.notify("Not in a collaboration session", vim.log.levels.WARN)
    return
  end
  
  config.log("info", "Leaving session: " .. M.state.session_id)
  
  p2p.leave_session(M.state.session_id, function(response)
    if response.type == "session_left" or response.type == "status" then
      M.state.session_id = nil
      M.state.peers = {}
      M.state.has_control = false
      M.state.current_controller = nil
      
      vim.notify("Left collaboration session", vim.log.levels.INFO)
    elseif response.type == "error" then
      M.handle_error_message(response.data)
    end
  end)
end

-- Request editing control
function M.request_control()
  if not M.state.session_id then
    vim.notify("Not in a collaboration session", vim.log.levels.WARN)
    return
  end
  
  if M.state.has_control then
    vim.notify("You already have control", vim.log.levels.INFO)
    return
  end
  
  config.log("info", "Requesting control")
  
  p2p.request_control(M.state.user_id, function(response)
    if response.type == "control_status" then
      M.handle_control_status(response.data)
    elseif response.type == "error" then
      M.handle_error_message(response.data)
    end
  end)
end

-- Release editing control
function M.release_control()
  if not M.state.session_id then
    vim.notify("Not in a collaboration session", vim.log.levels.WARN)
    return
  end
  
  if not M.state.has_control then
    vim.notify("You don't have control to release", vim.log.levels.WARN)
    return
  end
  
  config.log("info", "Releasing control")
  
  p2p.release_control(function(response)
    if response.type == "control_status" then
      M.handle_control_status(response.data)
    elseif response.type == "error" then
      M.handle_error_message(response.data)
    end
  end)
end

-- Health check
function M.health_check()
  if not p2p.is_process_running() then
    return { status = "error", message = "Go process not running" }
  end
  
  p2p.health_check(function(response)
    if response.type == "status" then
      config.log("info", "Health check: " .. response.data.status)
    end
  end)
  
  return { status = "ok", message = "Health check sent" }
end

-- Get current plugin state
function M.get_state()
  return {
    plugin_state = M.state,
    p2p_status = p2p.get_status(),
    config = config.get(),
  }
end

-- Stop the plugin
function M.stop()
  config.log("info", "Stopping collab.nvim")
  
  if M.state.session_id then
    M.leave_session()
  end
  
  p2p.stop()
  M.state.initialized = false
  
  vim.notify("collab.nvim stopped", vim.log.levels.INFO)
end

-- Restart the plugin
function M.restart()
  config.log("info", "Restarting collab.nvim")
  M.stop()
  
  vim.defer_fn(function()
    M.setup(config.get())
  end, 1000)
end

-- Set up autocmds for cleanup
function M.setup_autocmds()
  local group = vim.api.nvim_create_augroup("CollabNvim", { clear = true })
  
  -- Cleanup on VimLeavePre
  vim.api.nvim_create_autocmd("VimLeavePre", {
    group = group,
    callback = function()
      if M.state.initialized then
        M.stop()
      end
    end,
  })
end

return M
