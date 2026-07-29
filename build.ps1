# Compila projectmapper para windows/amd64, darwin/arm64 y linux/amd64 —
# §3 Fase 4, tarea 5. Cada binario es standalone (stdlib + excelize, sin
# CGO), listo para copiar junto a data/ sin instalar Go en destino.
param(
    [string]$Version = "dev"
)

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

$OutDir = "dist"
$Ldflags = "-s -w"

$Targets = @(
    @{ GOOS = "windows"; GOARCH = "amd64"; Ext = ".exe" },
    @{ GOOS = "darwin";  GOARCH = "arm64"; Ext = "" },
    @{ GOOS = "linux";   GOARCH = "amd64"; Ext = "" }
)

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

foreach ($t in $Targets) {
    $out = Join-Path $OutDir "projectmapper-$($t.GOOS)-$($t.GOARCH)$($t.Ext)"
    Write-Host "==> building $out"
    $env:GOOS = $t.GOOS
    $env:GOARCH = $t.GOARCH
    $env:CGO_ENABLED = "0"
    go build -trimpath -ldflags $Ldflags -o $out ./cmd/projectmapper
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed for $($t.GOOS)/$($t.GOARCH)"
    }
}

Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue

Write-Host "listo. Binarios en $OutDir/ (versión: $Version)"
