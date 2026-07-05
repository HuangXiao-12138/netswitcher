# NetSwitcher build script (PowerShell). Mirrors the Makefile for Windows
# environments without GNU make. Usage:
#   .\build.ps1            # full build (needs gcc)
#   .\build.ps1 -CliOnly   # service/CLI only (no gcc)
param(
  [switch]$CliOnly,
  [string]$Version = "0.1.0"
)

$ErrorActionPreference = "Stop"
$Binary = "build/bin/netswitcher.exe"
New-Item -ItemType Directory -Force -Path "build/bin" | Out-Null

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
