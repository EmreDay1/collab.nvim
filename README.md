# collab.nvim

### Two people, one text editor

**Author:** dayangac

---

## Overview

`collab.nvim` is a collaborative editing plugin for Neovim that allows two or more people to work on the same buffer in real time. It uses a hybrid Lua–Go architecture where the Lua layer manages Neovim events, UI, and configuration, while the Go backend handles session management, peer-to-peer communication (WebRTC), and operational transformation for conflict-free synchronization.

---

## Installation

Using [lazy.nvim](https://github.com/folke/lazy.nvim):

```lua
{
  "dayangac/collab.nvim",
  build = "go build -o collab-nvim ./go",
  config = function()
    require("collab").setup({
      auto_build = true,
      show_remote_cursors = true,
      debug = false
    })
  end
}
```

---

## Usage

### Core Commands

* `:CollabCreate` — Start a new collaboration session.
* `:CollabJoin <session_id>` — Join an existing session.
* `:CollabLeave` — Leave the current session.
* `:CollabControl` — Request edit control.
* `:CollabRelease` — Release edit control.
* `:CollabInfo` — Show current session details.
* `:CollabStatus` — Display plugin status.

Each command is registered dynamically in the UI layer (`ui.lua`) and interacts with the collaboration core in `init.lua`.

---

## Configuration

Default configuration (from `config.lua`):

```lua
{
  binary_name = "collab-nvim",
  auto_build = true,
  create_key = "<leader>cc",
  join_key = "<leader>cj",
  pass_control_key = "<leader>cp",
  leave_key = "<leader>cl",
  show_remote_cursors = true,
  cursor_colors = { "#ff6b6b", "#4ecdc4", "#45b7d1", "#96ceb4", "#ffeaa7" },
  auto_sync = true,
  sync_debounce_ms = 100,
  heartbeat_interval_ms = 30000,
  log_level = "info"
}
```

You can override these values in your Neovim config using:

```lua
require("collab").setup({
  debug = true,
  log_level = "debug"
})
```

---

## Architecture

### Lua Layer

* `config.lua`: Defines plugin configuration and handles binary discovery and building.
* `init.lua`: Entry point; loads Go backend, sets up RPC communication.
* `p2p.lua`: Manages the Go process (spawning, reading JSON messages, sending commands).
* `ui.lua`: Provides user-facing commands, notifications, and floating windows.

### Go Layer

Located under the `go/` directory.

* `main.go`: Central entry for the Go backend; handles JSON-RPC messages from Lua.
* `p2p.go`: WebRTC-based peer-to-peer connection manager.
* `session.go`: Session creation, joining, and control management.
* `protocol.go`: Defines message types and structures exchanged with Lua.
* `sync.go`: Implements Operational Transformation (OT) for real-time, conflict-free text synchronization.

The Lua client communicates with the Go process over pipes using JSON messages, enabling real-time synchronization and peer updates.

---

## Development

To build the Go backend manually:

```bash
go build -o collab-nvim ./go
```

To run locally in Neovim:

```bash
git clone https://github.com/dayangac/collab.nvim
cd collab.nvim
nvim --cmd 'set rtp+=.'
```

---

## Contributing

If you want to contribute to the plugin, please contact **[emreday01@gmail.com](mailto:emreday01@gmail.com)** or send in a pull request.

---

## License

MIT License © 2025 Dayangaç
