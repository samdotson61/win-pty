@echo off
rem One-click installer for win-pty. Double-click this file, or run it from a
rem terminal. It checks for Go (installs via winget if missing), builds the
rem single win-pty.exe, and puts it on your PATH.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0install.ps1" %*
echo.
pause
