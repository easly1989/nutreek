# Windows dev setup without GNU Make: downloads Tailwind v3's standalone CLI
# into ./bin/tailwindcss.exe. Idempotent.
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$bin = Join-Path $root "bin"
$twExe = Join-Path $bin "tailwindcss.exe"
$twVersion = "v3.4.17"
$twUrl = "https://github.com/tailwindlabs/tailwindcss/releases/download/$twVersion/tailwindcss-windows-x64.exe"

New-Item -ItemType Directory -Force -Path $bin | Out-Null

if (Test-Path $twExe) {
    Write-Host "tailwindcss.exe already there ($([math]::Round((Get-Item $twExe).Length / 1MB, 1)) MB). Skip."
} else {
    Write-Host "Downloading Tailwind $twVersion from $twUrl..."
    Invoke-WebRequest -Uri $twUrl -OutFile $twExe -UseBasicParsing
    Write-Host "OK: $twExe"
}

Write-Host "Setup done."
