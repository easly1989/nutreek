# Setup dev su Windows senza GNU Make: scarica la CLI standalone di
# Tailwind v3 in ./bin/tailwindcss.exe. Idempotente.
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$bin = Join-Path $root "bin"
$twExe = Join-Path $bin "tailwindcss.exe"
$twVersion = "v3.4.17"
$twUrl = "https://github.com/tailwindlabs/tailwindcss/releases/download/$twVersion/tailwindcss-windows-x64.exe"

New-Item -ItemType Directory -Force -Path $bin | Out-Null

if (Test-Path $twExe) {
    Write-Host "tailwindcss.exe gia' presente ($([math]::Round((Get-Item $twExe).Length / 1MB, 1)) MB). Skip."
} else {
    Write-Host "Scarico Tailwind $twVersion da $twUrl..."
    Invoke-WebRequest -Uri $twUrl -OutFile $twExe -UseBasicParsing
    Write-Host "OK: $twExe"
}

Write-Host "Setup completato."
