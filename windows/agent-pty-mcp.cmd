@echo off
rem Launcher for the agent-pty MCP server on native Windows.
rem Prepends the MSYS2 tmux dir and PowerShell 7 to the inherited PATH so the
rem server finds tmux.exe and panes find pwsh, while keeping every standard
rem Windows path (System32, etc.) that pwsh panes rely on. MSYS=noglob stops
rem the cygwin runtime from mangling tmux format strings and keystroke payloads.
set "PATH=C:\msys64\usr\bin;C:\Program Files\PowerShell\7;%PATH%"
set "MSYS=noglob"
rem Adjust this path to wherever you cloned agent-pty (MSYS2 home shown).
"C:\msys64\home\%USERNAME%\agent-pty\.venv-win\Scripts\python.exe" -m agent_pty.mcp %*
