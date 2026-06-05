# agent-pty on native Windows (MSYS2 tmux + PowerShell panes)

This directory adds **native Windows** support to agent-pty: a real `tmux.exe`
(from MSYS2 — no WSL) driven by agent-pty, with **PowerShell 7 panes by
default** and MSYS2 `bash` one keystroke away.

> Credit: agent-pty is by **[AakeshF](https://github.com/AakeshF/agent-pty)**.
> This is a Windows port of that project; all the terminal-multiplexer ideas and
> the core `Pty` API are upstream's. The Windows-specific glue here is by
> **Sam Dotson**.

## Why this is non-trivial

`tmux` is a Unix program and `libtmux` is documented as Unix-only. Driving the
cygwin-runtime MSYS2 `tmux.exe` from a **native** Windows Python hits three
distinct failures, each fixed in the code (`agent_pty/session.py`,
`agent_pty/io.py`) so the public `Pty` API is unchanged and POSIX behaviour is
untouched:

1. **Server-start hang.** The first tmux command leaks libtmux's captured
   stdout pipe into the forked tmux daemon, so `communicate()` blocks forever.
   Fix: every tmux call goes through a single `subprocess` choke point with
   `stdin=DEVNULL`, and a hidden `sleep infinity` keepalive session keeps the
   server up so commands only ever *connect* — they never start it.
2. **Argument brace-mangling.** The cygwin runtime brace-expands a native
   program's argv, so `#{session_name}` arrives as `#session_name` and a
   keystroke payload like `${VAR}` or `@{}` is silently corrupted. Fix:
   `MSYS=noglob` for every tmux subprocess.
3. **Quote corruption.** Windows→cygwin command-line quoting turns `"` into `\`.
   Fix: literal keystrokes are injected via a tmux paste buffer loaded from
   **stdin** (`load-buffer -` + `paste-buffer`), which no command-line parser
   touches. Verified with quotes, backslashes, `$()`, `;`, and Unicode.

## Setup

### 1. Install MSYS2 and tmux

```powershell
winget install MSYS2.MSYS2
# native tmux + winpty (a real tmux.exe, no WSL)
C:\msys64\usr\bin\bash.exe -lc "pacman -S --noconfirm --needed tmux winpty"
```

`C:\msys64\usr\bin\tmux.exe -V` should report tmux 3.5+.

### 2. Install agent-pty under a native Python

Use a **native** CPython (python.org), *not* the MSYS2 Python — native Python
has prebuilt wheels for `pydantic-core`/`mcp` and matches PyPI exactly.

```powershell
git clone https://github.com/<you>/agent-pty.git C:\msys64\home\%USERNAME%\agent-pty
cd C:\msys64\home\%USERNAME%\agent-pty
py -3 -m venv .venv-win
.\.venv-win\Scripts\python -m pip install -e .
```

### 3. Install the tmux config

Copy [`tmux.conf`](tmux.conf) to your MSYS2 home as `~/.tmux.conf`
(`C:\msys64\home\<you>\.tmux.conf`). It sets PowerShell 7 as the default pane
shell and binds bash to `prefix + B` / `prefix + b` / splits.

### 4. Register the MCP server with Claude Code

Edit the launcher [`agent-pty-mcp.cmd`](agent-pty-mcp.cmd) so its paths match
your machine, then point your MCP config at it. In `~/.claude.json`:

```json
"agent-pty": {
  "type": "stdio",
  "command": "cmd.exe",
  "args": ["/c", "C:\\msys64\\home\\<you>\\agent-pty\\agent-pty-mcp.cmd"]
}
```

The `.cmd` wrapper prepends the MSYS2 `tmux` dir and PowerShell 7 to `PATH`
(keeping the full Windows `PATH` so pwsh panes work normally) and sets
`MSYS=noglob`. Using a `.cmd` instead of an `env` block avoids depending on
whether the MCP host expands `${PATH}`.

Restart Claude Code (or reconnect via `/mcp`).

## Use

- `pty_spawn` with no command → a PowerShell 7 pane. Pass `cmd="bash -l"` for
  bash. The 6 `pty_*` tools (spawn/send/snapshot/wait_for/list/kill) work as
  upstream.
- Watch or take over any session from any PowerShell window while the MCP is
  running:

  ```powershell
  C:\msys64\usr\bin\tmux.exe attach -t agent-pty-<name>
  ```

  Inside tmux: `prefix + B` opens a bash window, `prefix + b` splits the current
  pane into bash, `prefix + R` reloads the config. (Default prefix is `Ctrl-b`.)

## Verify

```powershell
$env:PYTHONUTF8 = "1"
.\.venv-win\Scripts\python -u examples\mcp_smoke_wrapper.py
```

Launches the MCP exactly as Claude Code does (via the `.cmd` wrapper), confirms
a default pane is PowerShell 7 with a full `PATH`, and exercises spawn/send/
wait/kill over real stdio JSON-RPC. `examples/mcp_smoke_win.py` is a variant
that launches the venv Python directly.

## Notes / limitations

- Sessions live as long as the agent-pty (MCP) process does. When Claude Code
  exits, the tmux server and its sessions go with it.
- The keepalive session is named `agentpty_srv_keepalive` and is hidden from
  `pty_list`.
