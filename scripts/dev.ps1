# Dev loop on Windows: build the CSS once (if needed) and start the server
# with NUTREEK_ENV=dev. For CSS hot reload open a second terminal with:
#   pwsh scripts/tailwind.ps1 -Watch
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$css = Join-Path $root "internal\web\static\css\tailwind.css"

if (-not (Test-Path $css)) {
    Write-Host "tailwind.css missing, generating it..."
    & (Join-Path $PSScriptRoot "tailwind.ps1")
}

$env:NUTREEK_ENV = "dev"
Write-Host "Starting nutreek on http://localhost:8080 (ctrl+C to stop)..."
& go run "$root\cmd\nutreek"
