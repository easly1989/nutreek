# Dev loop su Windows: build CSS una volta (se necessario) e avvia il
# server con NUTREEK_ENV=dev. Per hot-reload CSS aprire un secondo
# terminale con:  pwsh scripts/tailwind.ps1 -Watch
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$css = Join-Path $root "internal\web\static\css\tailwind.css"

if (-not (Test-Path $css)) {
    Write-Host "tailwind.css mancante, lo genero..."
    & (Join-Path $PSScriptRoot "tailwind.ps1")
}

$env:NUTREEK_ENV = "dev"
Write-Host "Avvio nutreek su http://localhost:8080 (ctrl+C per stop)..."
& go run "$root\cmd\nutreek"
