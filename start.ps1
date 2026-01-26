$ErrorActionPreference = "Stop"

Set-Location $PSScriptRoot

if (-not (Get-Command pnpm -ErrorAction SilentlyContinue)) {
  Write-Error "pnpm not found. Install pnpm first."
  exit 1
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
  Write-Error "go not found. Install Go first."
  exit 1
}

if (-not (Test-Path "node_modules") -or -not (Test-Path "web/node_modules")) {
  pnpm install
}

Write-Host "Starting ControlCCX (production-like)..."
Write-Host "URL: http://127.0.0.1:5174"
pnpm start -- @args

