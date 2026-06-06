#requires -version 5
<#
.SYNOPSIS
  Install win-pty (the native-Windows agent-pty fork) — Go edition.
.DESCRIPTION
  Checks for Go (installs it via winget if missing), compiles the single static
  win-pty.exe (tool + all deps in one binary — no Python/venv), puts it on your
  PATH, and prints the MCP registration. Needs the MSYS2 tmux (install winmux
  first if you don't have it). Idempotent.
#>
[CmdletBinding()]
param([string]$Msys2Root = 'C:\msys64')

$ErrorActionPreference = 'Stop'
function Info($m) { Write-Host "==> $m" -ForegroundColor Cyan }
function Warn($m) { Write-Host "!!  $m" -ForegroundColor Yellow }

$root = $PSScriptRoot
$goSrc = Join-Path $root 'go'
$exe   = Join-Path $root 'win-pty.exe'

# 1. Ensure Go.
function Resolve-Go {
    $c = Get-Command go -ErrorAction SilentlyContinue
    if ($c) { return $c.Source }
    $p = Join-Path $env:ProgramFiles 'Go\bin\go.exe'
    if (Test-Path $p) { return $p }
    return $null
}
$go = Resolve-Go
if (-not $go) {
    Info 'Go not found — installing via winget (GoLang.Go)...'
    winget install --id GoLang.Go --accept-source-agreements --accept-package-agreements --disable-interactivity
    $go = Resolve-Go
    if (-not $go) { throw 'Go install did not produce go.exe. Install Go from https://go.dev/dl and re-run.' }
}
Info "Using Go: $(& $go version)"

# 2. Build the single static binary.
Info 'Building win-pty.exe ...'
Push-Location $goSrc
try {
    & $go build -o $exe .
    if ($LASTEXITCODE -ne 0) { throw 'go build failed.' }
} finally { Pop-Location }
if (-not (Test-Path $exe)) { throw 'win-pty.exe was not produced.' }
Info "Built: $exe ($([math]::Round((Get-Item $exe).Length/1MB,1)) MB, no runtime deps)"

# 3. Put the repo dir on the user PATH so `win-pty` resolves.
$userPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
if (($userPath -split ';') -notcontains $root) {
    Info "Adding $root to your user PATH..."
    $userPath = if ([string]::IsNullOrEmpty($userPath)) { $root } else { "$root;$userPath" }
    [Environment]::SetEnvironmentVariable('PATH', $userPath, 'User')
    $env:PATH = "$root;$env:PATH"
    $pathChanged = $true
} else { Info 'Already on PATH.' }

# 4. tmux present?
if (-not (Test-Path (Join-Path $Msys2Root 'usr\bin\tmux.exe'))) {
    Warn "tmux not found at $Msys2Root\usr\bin\tmux.exe — run the winmux installer first:"
    Warn '  https://github.com/samdotson61/winmux'
}

# 5. Report + MCP registration (direct exe, no cmd wrapper / no env block needed).
Write-Host ''
Write-Host 'win-pty installed.' -ForegroundColor Green
Write-Host 'CLI:  win-pty spawn demo   /   win-pty list   /   win-pty --help'
Write-Host ''
Write-Host 'Register the MCP server (add to ~/.claude.json under "mcpServers"):' -ForegroundColor Green
$escaped = $exe -replace '\\', '\\'
Write-Host @"
  "win-pty": {
    "type": "stdio",
    "command": "$escaped",
    "args": ["mcp"]
  }
"@
if ($pathChanged) {
    Write-Host ''
    Write-Host 'NOTE: PATH was updated - open a NEW terminal for `win-pty` to resolve.' -ForegroundColor Yellow
}
