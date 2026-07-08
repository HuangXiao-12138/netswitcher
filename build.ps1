# NetSwitcher build script (PowerShell). Mirrors the Makefile for Windows
# environments without GNU make. Usage:
#   .\build.ps1            # full build (needs gcc)
#   .\build.ps1 -CliOnly   # service/CLI only (no gcc)
param(
  [switch]$CliOnly,
  [string]$Version
)

# Default VERSION to the git-derived version (a tag like "v1.2.0", or
# "v1.2.0-3-gabcdef" / "v1.2.0-dirty" when ahead of a tag) so local builds
# match CI's semantics — checking for updates compares against the same scheme
# GitHub Releases uses. Pass -Version X to override.
if (-not $Version) {
  $Version = (git describe --tags --always --dirty 2>$null)
  if (-not $Version) { $Version = "0.0.0" }
}

$ErrorActionPreference = "Stop"
$Binary = "build/bin/NetSwitcher.exe"
New-Item -ItemType Directory -Force -Path "build/bin" | Out-Null
# Clear any old-casing binary first: Windows is case-insensitive, so a leftover
# netswitcher.exe would otherwise make go build keep the old name.
Remove-Item "build/bin/netswitcher.exe", "build/bin/NetSwitcher.exe" -ErrorAction SilentlyContinue

function Invoke([string]$Cmd) {
  Write-Host "» $Cmd"
  & ([scriptblock]::Create($Cmd))
  if ($LASTEXITCODE -ne 0) { throw "command failed: $Cmd" }
}

if (-not $CliOnly) {
  Push-Location frontend
  try {
    if (-not (Test-Path node_modules)) { Invoke "npm install" }
    Invoke "npm run build"
  } finally { Pop-Location }
}

if ($CliOnly) {
  $env:CGO_ENABLED = "0"
  Invoke "go build -ldflags `"-X main.version=$Version`" -o $Binary ./cmd/netswitcher"
} else {
  # Wails needs the `desktop` build tag or its runtime shows a "missing build
  # tags" error dialog; -H windowsgui makes double-click not pop a console.
  $env:CGO_ENABLED = "1"
  Invoke "go build -tags desktop,production -ldflags `"-X main.version=$Version -H windowsgui`" -o $Binary ./cmd/netswitcher"
}
Write-Host "✓ built $Binary"
