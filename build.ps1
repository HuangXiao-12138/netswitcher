# NetSwitcher build script (PowerShell). Mirrors the Makefile for Windows
# environments without GNU make. Usage:
#   .\build.ps1            # full build (needs gcc)
#   .\build.ps1 -CliOnly   # service/CLI only (no gcc)
param(
  [switch]$CliOnly,
  [string]$Version = "0.1.0"
)

$ErrorActionPreference = "Stop"
$Binary = "netswitcher.exe"

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

$env:CGO_ENABLED = if ($CliOnly) { "0" } else { "1" }
Invoke "go build -ldflags `"-X main.version=$Version`" -o $Binary ./cmd/netswitcher"
Write-Host "✓ built $Binary"
