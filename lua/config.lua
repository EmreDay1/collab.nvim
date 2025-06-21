local M = {}

-- Default configuration
M.defaults = {
  -- Go binary settings
  binary_name = "collab-nvim",
  auto_build = true,
  
  -- Keybindings
  create_key = "<leader>cc",
  join_key = "<leader>cj", 
  pass_control_key = "<leader>cp",
  leave_key = "<leader>cl",
  
  -- UI settings
  show_remote_cursors = true,
  cursor_colors = { "#ff6b6b", "#4ecdc4", "#45b7d1", "#96ceb4", "#ffeaa7" },
  show_status_line = true,
  
  -- Behavior settings  
  auto_sync = true,
  sync_debounce_ms = 100,
  heartbeat_interval_ms = 30000,
  
  -- Debug settings
  debug = false,
  log_level = "info", -- "debug", "info", "warn", "error"
}

-- Current configuration (will be merged with user options)
M.opts = {}

-- Setup configuration with user options
function M.setup(opts)
  M.opts = vim.tbl_deep_extend("force", M.defaults, opts or {})
  
  -- Validate configuration
  M.validate()
  
  -- Detect binary path
  M.opts.binary_path = M.get_binary_path()
  
  return M.opts
end

-- Validate configuration options
function M.validate()
  local opts = M.opts
  
  -- Check required string options
  local required_strings = { "binary_name" }
  for _, key in ipairs(required_strings) do
    if type(opts[key]) ~= "string" or opts[key] == "" then
      error(string.format("collab.nvim: %s must be a non-empty string", key))
    end
  end
  
  -- Check numeric options
  local numeric_options = { "sync_debounce_ms", "heartbeat_interval_ms" }
  for _, key in ipairs(numeric_options) do
    if type(opts[key]) ~= "number" or opts[key] <= 0 then
      error(string.format("collab.nvim: %s must be a positive number", key))
    end
  end
  
  -- Check boolean options
  local boolean_options = { "auto_build", "show_remote_cursors", "auto_sync", "debug" }
  for _, key in ipairs(boolean_options) do
    if type(opts[key]) ~= "boolean" then
      error(string.format("collab.nvim: %s must be a boolean", key))
    end
  end
  
  -- Check cursor colors array
  if type(opts.cursor_colors) ~= "table" then
    error("collab.nvim: cursor_colors must be a table")
  end
  
  -- Check log level
  local valid_log_levels = { "debug", "info", "warn", "error" }
  local log_level_valid = false
  for _, level in ipairs(valid_log_levels) do
    if opts.log_level == level then
      log_level_valid = true
      break
    end
  end
  if not log_level_valid then
    error("collab.nvim: log_level must be one of: " .. table.concat(valid_log_levels, ", "))
  end
end

-- Get the path to the Go binary
function M.get_binary_path()
  local opts = M.opts
  
  -- If user provided explicit path, use it
  if opts.binary_path then
    return vim.fn.expand(opts.binary_path)
  end
  
  -- Auto-detect plugin root directory
  local plugin_root = M.get_plugin_root()
  
  -- Try different possible locations
  local possible_paths = {
    plugin_root .. "/" .. opts.binary_name,           -- Root directory
    plugin_root .. "/bin/" .. opts.binary_name,       -- bin subdirectory
    plugin_root .. "/go/" .. opts.binary_name,        -- go subdirectory
  }
  
  -- Check if binary exists in any location
  for _, path in ipairs(possible_paths) do
    if vim.fn.executable(path) == 1 then
      if opts.debug then
        print("collab.nvim: Found binary at " .. path)
      end
      return path
    end
  end
  
  -- If auto_build is enabled, try to build it
  if opts.auto_build then
    local built_path = M.build_binary()
    if built_path then
      return built_path
    end
  end
  
  -- Binary not found and couldn't build
  local tried_paths = table.concat(possible_paths, "\n  ")
  error(string.format(
    "collab.nvim: Go binary not found. Tried:\n  %s\n\nPlease build the binary with:\n  cd %s && go build -o %s ./go",
    tried_paths, plugin_root, opts.binary_name
  ))
end

-- Get the plugin root directory
function M.get_plugin_root()
  -- Get the directory where this config.lua file is located
  local current_file = debug.getinfo(1, "S").source:sub(2)
  local config_dir = vim.fn.fnamemodify(current_file, ":p:h")
  
  -- Go up from lua/collab/ to plugin root
  local plugin_root = vim.fn.fnamemodify(config_dir, ":h:h")
  
  return plugin_root
end

-- Try to build the Go binary automatically
function M.build_binary()
  local opts = M.opts
  local plugin_root = M.get_plugin_root()
  local binary_path = plugin_root .. "/" .. opts.binary_name
  
  if opts.debug then
    print("collab.nvim: Attempting to build Go binary...")
  end
  
  -- Check if go directory exists
  local go_dir = plugin_root .. "/go"
  if vim.fn.isdirectory(go_dir) == 0 then
    if opts.debug then
      print("collab.nvim: Go source directory not found: " .. go_dir)
    end
    return nil
  end
  
  -- Build command
  local build_cmd = string.format("cd %s && go build -o %s ./go", plugin_root, opts.binary_name)
  
  if opts.debug then
    print("collab.nvim: Running: " .. build_cmd)
  end
  
  -- Execute build command
  local result = vim.fn.system(build_cmd)
  
  if vim.v.shell_error ~= 0 then
    print("collab.nvim: Failed to build Go binary:")
    print(result)
    return nil
  end
  
  -- Check if binary was created successfully
  if vim.fn.executable(binary_path) == 1 then
    print("collab.nvim: Successfully built Go binary at " .. binary_path)
    return binary_path
  else
    print("collab.nvim: Build succeeded but binary not found at " .. binary_path)
    return nil
  end
end

-- Get current configuration
function M.get()
  return M.opts
end

-- Update configuration at runtime
function M.update(new_opts)
  M.opts = vim.tbl_deep_extend("force", M.opts, new_opts or {})
  M.validate()
  return M.opts
end

-- Check if debug mode is enabled
function M.is_debug()
  return M.opts.debug or false
end

-- Log message based on configured log level
function M.log(level, message)
  local levels = { debug = 1, info = 2, warn = 3, error = 4 }
  local current_level = levels[M.opts.log_level] or 2
  local msg_level = levels[level] or 2
  
  if msg_level >= current_level then
    local prefix = string.format("[collab.nvim:%s]", level:upper())
    print(prefix .. " " .. message)
  end
end

return M
