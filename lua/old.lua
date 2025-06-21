if not _G.vim then
  _G.vim = {
    fn = {
      stdpath = function(what)
        if what == 'data' then return '/tmp/nvim-data' end
        if what == 'log' then return '/tmp/nvim-log' end
        return '/tmp'
      end,
      expand = function(path) 
        return path:gsub('~', os.getenv('HOME') or '/tmp') 
      end,
      executable = function(path) 
        local f = io.open(path, 'r')
        if f then f:close() return 1 else return 0 end
      end,
      isdirectory = function(path) return 0 end,
      mkdir = function(path, mode) return true end,
      fnamemodify = function(path, modifier) 
        if modifier == ':h' then
          return path:match('(.*/)')
        end
        return '/tmp' 
      end,
      has = function(feature) return 0 end,
    },
    log = { levels = { INFO = 2, WARN = 3, ERROR = 4 } },
    notify = function(msg, level) 
      local level_names = { [2] = 'INFO', [3] = 'WARN', [4] = 'ERROR' }
      print(string.format('[%s] %s', level_names[level] or 'INFO', msg)) 
    end,
    split = function(str, sep) 
      local parts = {}
      for part in str:gmatch('[^' .. sep:gsub('%.', '%%.') .. ']+') do
        table.insert(parts, part)
      end
      return parts
    end,
    deepcopy = function(t)
      if type(t) ~= 'table' then return t end
      local copy = {}
      for k, v in pairs(t) do
        copy[k] = vim.deepcopy(v)
      end
      return copy
    end,
    api = {
      nvim_set_hl = function(ns, name, opts) end
    }
  }
end

local M = {}

M.defaults = {
  go_binary = {
    path = nil,
    auto_download = true,
    version = "latest",
  },
  keymaps = {
    create_session = '<leader>cc',
    join_session = '<leader>cj',
    leave_session = '<leader>cl',
    pass_control = '<leader>cp',
    request_control = '<leader>cr',
  },
  ui = {
    cursors = {
      enabled = true,
      show_names = true,
      colors = { '#FF6B6B', '#4ECDC4', '#45B7D1', '#96CEB4' },
    },
    notifications = {
      enabled = true,
      level = 2,
      timeout = 3000,
    },
  },
  sync = {
    auto_sync = true,
    debounce_delay = 100,
    max_operation_size = 10000,
  },
  network = {
    timeout = 30000,
    heartbeat_interval = 10000,
    max_peers = 10,
  },
  debug = {
    enabled = false,
    log_level = 'info',
    log_file = '/tmp/nvim-log/collab.log',
  },
}

M.config = {}

local function deep_merge(base, override)
  local result = {}
  
  for k, v in pairs(base) do
    if type(v) == 'table' then
      result[k] = deep_merge(v, {})
    else
      result[k] = v
    end
  end
  
  for k, v in pairs(override) do
    if type(v) == 'table' and type(result[k]) == 'table' then
      result[k] = deep_merge(result[k], v)
    else
      result[k] = v
    end
  end
  
  return result
end

local function validate_config(config)
  if config.sync and config.sync.debounce_delay then
    if type(config.sync.debounce_delay) ~= 'number' then
      vim.notify('Invalid type for sync.debounce_delay: expected number, got ' .. type(config.sync.debounce_delay), vim.log.levels.ERROR)
      return false
    end
    if config.sync.debounce_delay < 0 then
      vim.notify('sync.debounce_delay must be >= 0', vim.log.levels.ERROR)
      return false
    end
  end
  
  if config.network and config.network.timeout then
    if type(config.network.timeout) ~= 'number' or config.network.timeout < 1000 then
      vim.notify('network.timeout must be a number >= 1000', vim.log.levels.ERROR)
      return false
    end
  end
  
  return true
end

local function resolve_go_binary_path()
  local possible_paths = {
    vim.fn.expand('~/CS/collab.nvim/go/collab-nvim'),
    vim.fn.expand('~/CS/collab.nvim/go/collab-nvim.exe'),
    'collab-nvim',
    'collab-nvim.exe',
  }
  
  for _, path in ipairs(possible_paths) do
    if vim.fn.executable(path) == 1 then
      return path
    end
  end
  
  return nil
end

function M.setup(user_config)
  user_config = user_config or {}
  
  M.config = deep_merge(M.defaults, user_config)
  
  if not validate_config(M.config) then
    vim.notify('Configuration validation failed, using defaults', vim.log.levels.ERROR)
    M.config = vim.deepcopy(M.defaults)
  end
  
  M.config.go_binary.resolved_path = resolve_go_binary_path()
  
  return M.config
end

function M.get_value(path)
  local keys = vim.split(path, '%.', { plain = true })
  local current = M.config
  
  for _, key in ipairs(keys) do
    if type(current) ~= 'table' or current[key] == nil then
      return nil
    end
    current = current[key]
  end
  
  return current
end

function M.set_value(path, value)
  local keys = vim.split(path, '%.', { plain = true })
  local current = M.config
  
  for i = 1, #keys - 1 do
    local key = keys[i]
    if type(current[key]) ~= 'table' then
      current[key] = {}
    end
    current = current[key]
  end
  
  current[keys[#keys]] = value
end

function M.is_go_binary_available()
  return M.config.go_binary and M.config.go_binary.resolved_path ~= nil
end

function M.get_go_binary_path()
  return M.config.go_binary and M.config.go_binary.resolved_path
end

function M.get()
  return M.config
end

return M
