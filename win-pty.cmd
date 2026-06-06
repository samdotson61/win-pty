@echo off
rem win-pty CLI launcher. Self-locating: runs the win-pty console script from
rem the .venv-win that lives next to this file (%~dp0), so it works wherever
rem win-pty is cloned. Sets MSYS=noglob and puts the MSYS2 tmux on PATH so the
rem CLI can drive native-Windows tmux from any shell or pane.
rem
rem Override MSYS2 location with WMUX_TMUXDIR if tmux isn't at C:\msys64\usr\bin.
if not defined WMUX_TMUXDIR set "WMUX_TMUXDIR=C:\msys64\usr\bin"
set "PATH=%WMUX_TMUXDIR%;%PATH%"
set "MSYS=noglob"
"%~dp0.venv-win\Scripts\win-pty.exe" %*
