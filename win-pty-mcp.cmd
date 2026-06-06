@echo off
rem win-pty MCP server launcher (point your MCP client's stdio command here).
rem Self-locating via %~dp0: runs the MCP server from the sibling .venv-win, so
rem it works wherever win-pty is cloned. Prepends the MSYS2 tmux dir and
rem PowerShell 7 to PATH (keeping the full Windows PATH so pwsh panes work) and
rem sets MSYS=noglob so the cygwin runtime doesn't mangle tmux format strings or
rem keystroke payloads.
if not defined WMUX_TMUXDIR set "WMUX_TMUXDIR=C:\msys64\usr\bin"
set "PATH=%WMUX_TMUXDIR%;C:\Program Files\PowerShell\7;%PATH%"
set "MSYS=noglob"
"%~dp0.venv-win\Scripts\python.exe" -m agent_pty.mcp %*
