#requires -version 5
<#
.SYNOPSIS
  Install win-pty (the Windows agent-pty fork) on this machine.
.DESCRIPTION
  Creates a native-Python venv next to this script, installs win-pty into it
  (so the `win-pty` / `win-pty-mcp` commands exist), puts this folder on your
  PATH so `win-pty` resolves, and prints how to register the MCP server.

  Needs a native CPython (python.org), NOT the MSYS2 Python — native Python has
  prebuilt wheels for pydantic-core/mcp. Run winmux's installer first (or
  install MSYS2 + tmux yourself); win-pty drives that tmux.

  Idempotent: re-running reuses the venv and won't duplicate PATH entries.
#>
[CmdletBinding()]
param(
    [string]$Msys2Root = 'C:\msys64'
)

$ErrorActionPreference = 'Stop'
function Info($m) { Write-Host "==> $m" -ForegroundColor Cyan }
function Warn($m) { Write-Host "!!  $m" -ForegroundColor Yellow }

$root = $PSScriptRoot
$venv = Join-Path $root '.venv-win'

# 1. Find a native Python (prefer the py launcher; avoid the MSYS2 python).
Info 'Locating a native Python (python.org)...'
$py = $null
if (Get-Command py -ErrorAction SilentlyContinue) {
    $py = @('py', '-3')
} elseif (Get-Command python -ErrorAction SilentlyContinue) {
    $src = (Get-Command python).Source
    if ($src -like '*msys64*') { throw "Found MSYS2 python ($src); install a native python.org Python (or the 'py' launcher) and re-run." }
    $py = @($src)
} else {
    throw 'No Python found. Install Python 3.11+ from python.org, then re-run.'
}

# 2. Create the venv and install win-pty (editable).
if (-not (Test-Path (Join-Path $venv 'Scripts\python.exe'))) {
    Info "Creating venv at $venv ..."
    & $py[0] @($py[1..($py.Count-1)]) -m venv $venv
} else {
    Info 'venv already exists; reusing.'
}
$venvPy = Join-Path $venv 'Scripts\python.exe'
Info 'Installing win-pty (pip install -e .) ...'
& $venvPy -m pip install --upgrade pip -q
& $venvPy -m pip install -e $root -q
if (-not (Test-Path (Join-Path $venv 'Scripts\win-pty.exe'))) {
    throw 'win-pty.exe was not created — check the pip output above.'
}
Info "Installed: $(& $venvPy -c 'from importlib.metadata import version; print(""win-pty"", version(""agent-pty""))')"

# 3. Put this folder on the user PATH so `win-pty` (win-pty.cmd) resolves.
$userPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
if (($userPath -split ';') -notcontains $root) {
    Info "Adding $root to your user PATH..."
    $userPath = if ([string]::IsNullOrEmpty($userPath)) { $root } else { "$root;$userPath" }
    [Environment]::SetEnvironmentVariable('PATH', $userPath, 'User')
    $env:PATH = "$root;$env:PATH"
    $pathChanged = $true
} else {
    Info 'Already on PATH.'
}

# 4. Check that tmux is available (win-pty drives it).
if (-not (Test-Path (Join-Path $Msys2Root 'usr\bin\tmux.exe'))) {
    Warn "tmux not found at $Msys2Root\usr\bin\tmux.exe."
    Warn 'Run the winmux installer first (it installs MSYS2 + tmux): https://github.com/samdotson61/winmux'
}

# 5. Report + MCP registration snippet.
$mcp = Join-Path $root 'win-pty-mcp.cmd'
Write-Host ''
Write-Host 'win-pty installed.' -ForegroundColor Green
Write-Host 'CLI:  win-pty spawn demo   /   win-pty list   /   win-pty --help'
Write-Host ''
Write-Host 'Register the MCP server (add to ~/.claude.json under "mcpServers"):' -ForegroundColor Green
$escaped = $mcp -replace '\\', '\\'
Write-Host @"
  "win-pty": {
    "type": "stdio",
    "command": "cmd.exe",
    "args": ["/c", "$escaped"]
  }
"@
if ($pathChanged) {
    Write-Host ''
    Write-Host 'NOTE: PATH was updated - open a NEW terminal for `win-pty` to resolve.' -ForegroundColor Yellow
}
