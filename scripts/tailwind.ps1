# Genera internal/web/static/css/tailwind.css. Usa la CLI scaricata da
# setup.ps1. Passa -Watch per modalita' watch (dev).
param(
    [switch]$Watch
)
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$twExe = Join-Path $root "bin\tailwindcss.exe"
$config = Join-Path $root "web\tailwind\tailwind.config.js"
$input = Join-Path $root "web\tailwind\input.css"
$output = Join-Path $root "internal\web\static\css\tailwind.css"

if (-not (Test-Path $twExe)) {
    Write-Error "tailwindcss.exe non trovato. Esegui prima: pwsh scripts/setup.ps1"
    exit 1
}

New-Item -ItemType Directory -Force -Path (Split-Path $output) | Out-Null

$args = @("-c", $config, "-i", $input, "-o", $output)
if ($Watch) { $args += "--watch" } else { $args += "--minify" }

& $twExe @args
