local config = require('config')

local M = {}

-- Process state
M.process_handle = nil
M.stdin = nil
M.stdout = nil
M.stderr = nil
M.is_running = false
M.message_queue = {}
M.response_callbacks = {}
M.next_message_id = 1

-- Event handlers
M.on_message = nil
M.on_error = nil
M.on_disconnect = nil

-- Initialize the P2P manager
function M.init()
  M.is_running = false
  M.message_queue = {}
  M.response_callbacks = {}
  M.next_message_id = 1
end

-- Start the Go process
function M.start()
  if M.is_running then
    config.log("warn", "Go process already running")
    return true
  end
  
  local opts = config.get()
  local binary_path = opts.binary_path
  
  config.log("debug", "Starting Go process: " .. binary_path)
  
  -- Check if binary exists
  if vim.fn.executable(binary_path) ~= 1 then
    local error_msg = "Go binary not found or not executable: " .. binary_path
    config.log("error", error_msg)
    error("collab.nvim: " .. error_msg)
  end
  
  -- Spawn the Go process
  local handle = vim.loop.spawn(binary_path, {
    args = {},
    stdio = {
      vim.loop.new_pipe(false), -- stdin
      vim.loop.new_pipe(false), -- stdout  
      vim.loop.new_pipe(false), -- stderr
    }
  }, function(code, signal)
    -- Process exit callback
    M.on_process_exit(code, signal)
  end)
  
  if not handle then
    error("collab.nvim: Failed to spawn Go process")
  end
  
  M.process_handle = handle
  M.stdin = handle.stdio[1]
  M.stdout = handle.stdio[2] 
  M.stderr = handle.stdio[3]
  M.is_running = true
  
  -- Set up stdout reading
  M.setup_stdout_reading()
  
  -- Set up stderr reading for logs
  M.setup_stderr_reading()
  
  config.log("info", "Go process started successfully")
  
  -- Send initial health check
  vim.defer_fn(function()
    M.health_check()
  end, 100)
  
  return true
end

-- Stop the Go process
function M.stop()
  if not M.is_running then
    return
  end
  
  config.log("debug", "Stopping Go process")
  
  -- Close stdin to signal shutdown
  if M.stdin then
    M.stdin:close()
  end
  
  -- Close stdout and stderr
  if M.stdout then
    M.stdout:close()
  end
  
  if M.stderr then
    M.stderr:close()
  end
  
  -- Close process handle
  if M.process_handle then
    M.process_handle:close()
  end
  
  M.cleanup()
  
  config.log("info", "Go process stopped")
end

-- Cleanup process state
function M.cleanup()
  M.process_handle = nil
  M.stdin = nil
  M.stdout = nil
  M.stderr = nil
  M.is_running = false
  M.message_queue = {}
  M.response_callbacks = {}
end

-- Handle process exit
function M.on_process_exit(code, signal)
  config.log("warn", string.format("Go process exited with code %d, signal %d", code or -1, signal or -1))
  
  M.cleanup()
  
  if M.on_disconnect then
    M.on_disconnect(code, signal)
  end
end

-- Set up stdout reading for JSON responses
function M.setup_stdout_reading()
  if not M.stdout then
    return
  end
  
  M.stdout:read_start(function(err, data)
    if err then
      config.log("error", "stdout read error: " .. err)
      return
    end
    
    if not data then
      config.log("debug", "stdout closed")
      return
    end
    
    -- Process received data (may contain multiple JSON messages)
    M.process_stdout_data(data)
  end)
end

-- Set up stderr reading for Go process logs
function M.setup_stderr_reading()
  if not M.stderr then
    return
  end
  
  M.stderr:read_start(function(err, data)
    if err then
      config.log("error", "stderr read error: " .. err)
      return
    end
    
    if not data then
      return
    end
    
    -- Log Go process output
    local lines = vim.split(data, "\n", { trimempty = true })
    for _, line in ipairs(lines) do
      if line ~= "" then
        config.log("debug", "Go: " .. line)
      end
    end
  end)
end

-- Process stdout data and parse JSON messages
function M.process_stdout_data(data)
  -- Split data by newlines (each JSON message should be on one line)
  local lines = vim.split(data, "\n", { trimempty = true })
  
  for _, line in ipairs(lines) do
    if line ~= "" then
      M.handle_json_message(line)
    end
  end
end

-- Handle a single JSON message from Go process
function M.handle_json_message(json_str)
  config.log("debug", "Received: " .. json_str)
  
  -- Parse JSON
  local ok, message = pcall(vim.json.decode, json_str)
  if not ok then
    config.log("error", "Failed to parse JSON: " .. json_str)
    config.log("error", "Parse error: " .. message)
    return
  end
  
  -- Validate message structure
  if type(message) ~= "table" or not message.type then
    config.log("error", "Invalid message format: " .. json_str)
    return
  end
  
  -- Handle response callbacks
  if message.id and M.response_callbacks[message.id] then
    local callback = M.response_callbacks[message.id]
    M.response_callbacks[message.id] = nil
    callback(message)
    return
  end
  
  -- Handle general messages
  if M.on_message then
    M.on_message(message)
  end
end

-- Send a message to the Go process
function M.send_message(message, callback)
  if not M.is_running or not M.stdin then
    config.log("error", "Cannot send message - Go process not running")
    return false
  end
  
  -- Add message ID for response tracking
  local message_id = M.next_message_id
  M.next_message_id = M.next_message_id + 1
  message.id = message_id
  
  -- Store callback if provided
  if callback then
    M.response_callbacks[message_id] = callback
  end
  
  -- Serialize to JSON
  local ok, json_str = pcall(vim.json.encode, message)
  if not ok then
    config.log("error", "Failed to serialize message: " .. vim.inspect(message))
    return false
  end
  
  config.log("debug", "Sending: " .. json_str)
  
  -- Send to Go process (add newline)
  local data = json_str .. "\n"
  M.stdin:write(data)
  
  return true
end

-- Send health check message
function M.health_check(callback)
  return M.send_message({
    type = "health_check"
  }, callback)
end

-- Create a new session
function M.create_session(file_path, content, callback)
  return M.send_message({
    type = "create_session",
    data = {
      file_path = file_path,
      content = content
    }
  }, callback)
end

-- Join an existing session
function M.join_session(session_id, callback)
  return M.send_message({
    type = "join_session", 
    data = {
      session_id = session_id
    }
  }, callback)
end

-- Leave current session
function M.leave_session(session_id, callback)
  return M.send_message({
    type = "leave_session",
    data = {
      session_id = session_id or ""
    }
  }, callback)
end

-- Send document operation
function M.send_operation(operation, callback)
  return M.send_message({
    type = "document_operation",
    data = operation
  }, callback)
end

-- Send cursor movement
function M.send_cursor_move(cursor_pos, callback)
  return M.send_message({
    type = "cursor_move",
    data = cursor_pos
  }, callback)
end

-- Request control
function M.request_control(user_id, callback)
  return M.send_message({
    type = "request_control",
    data = {
      requested_by = user_id
    }
  }, callback)
end

-- Release control
function M.release_control(callback)
  return M.send_message({
    type = "release_control"
  }, callback)
end

-- Set event handlers
function M.set_handlers(handlers)
  M.on_message = handlers.on_message
  M.on_error = handlers.on_error  
  M.on_disconnect = handlers.on_disconnect
end

-- Check if process is running
function M.is_process_running()
  return M.is_running
end

-- Restart the Go process
function M.restart()
  config.log("info", "Restarting Go process")
  M.stop()
  
  -- Wait a bit before restarting
  vim.defer_fn(function()
    M.start()
  end, 500)
end

-- Get process status
function M.get_status()
  return {
    running = M.is_running,
    binary_path = config.get().binary_path,
    pending_callbacks = vim.tbl_count(M.response_callbacks),
    queued_messages = #M.message_queue
  }
end

return M
