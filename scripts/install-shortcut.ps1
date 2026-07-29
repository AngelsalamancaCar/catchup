# Instala Project Mapper como app de escritorio: copia el binario y el seed a
# un directorio de instalación y crea el atajo "Catchup" en el escritorio y en
# el menú Inicio.
#
# El atajo pasa la ruta absoluta del JSON de datos y fija WorkingDirectory al
# directorio de instalación, así que data/ y export/ siempre caen ahí, sin
# importar desde dónde se lance. Se abre minimizado: esa ventana de consola es
# el interruptor de apagado (cerrarla detiene el servidor).
#
# Es idempotente: se puede correr de nuevo tras cada rebuild y nunca toca
# data/projects.json si ya existe.
#
#   ./scripts/install-shortcut.ps1                  # instalar (compila primero)
#   ./scripts/install-shortcut.ps1 -NoBuild         # reusar dist/ como está
#   ./scripts/install-shortcut.ps1 -Remove          # borrar solo los atajos
#   ./scripts/install-shortcut.ps1 -Remove -Purge   # borrar atajos + datos
#
# Para un segundo portafolio independiente hay que darle nombre, directorio y
# puerto propios (-Name evita que el atajo pise el del primero):
#
#   ./scripts/install-shortcut.ps1 -Name "Catchup 2025" `
#       -InstallDir "$env:LOCALAPPDATA\Catchup-2025" -Port 8788
param(
    [string]$Name         = "Catchup",
    [string]$InstallDir   = "$env:LOCALAPPDATA\Catchup",
    [string]$ShortcutDir  = [Environment]::GetFolderPath('Desktop'),
    [string]$StartMenuDir = [Environment]::GetFolderPath('Programs'),
    [int]   $Port         = 8787,
    [switch]$NoBuild,
    [switch]$Remove,
    [switch]$Purge
)

$ErrorActionPreference = "Stop"

$Repo         = Split-Path -Parent $PSScriptRoot
$ShortcutName = "$Name.lnk"
$ExeName      = "catchup.exe"
$IconName     = "catchup.ico"

# Rutas de los dos atajos (el del menú Inicio hace que "catchup" aparezca al
# escribirlo en el buscador de Windows). Un directorio vacío se omite.
function Get-ShortcutPaths {
    $paths = @()
    if ($ShortcutDir)  { $paths += Join-Path $ShortcutDir $ShortcutName }
    if ($StartMenuDir) { $paths += Join-Path $StartMenuDir $ShortcutName }
    return $paths
}

# ---------- desinstalar ----------

if ($Remove) {
    foreach ($lnk in Get-ShortcutPaths) {
        if (Test-Path -LiteralPath $lnk) {
            Remove-Item -LiteralPath $lnk -Force
            Write-Host "borrado  $lnk"
        }
    }
    if ($Purge) {
        if (Test-Path -LiteralPath $InstallDir) {
            Write-Host ""
            Write-Host "ATENCIÓN: esto borra el portafolio completo en $InstallDir" -ForegroundColor Yellow
            Write-Host "(incluye data\projects.json y export\)." -ForegroundColor Yellow
            $answer = Read-Host "Escribí BORRAR para confirmar"
            if ($answer -ceq "BORRAR") {
                Remove-Item -LiteralPath $InstallDir -Recurse -Force
                Write-Host "borrado  $InstallDir"
            } else {
                Write-Host "cancelado: los datos siguen en $InstallDir"
            }
        }
    } else {
        Write-Host "los datos siguen intactos en $InstallDir (usá -Purge para borrarlos)"
    }
    Write-Host "listo."
    exit 0
}

# ---------- compilar ----------

if (-not $NoBuild) {
    & (Join-Path $Repo "build.ps1")
}

$SrcExe = Join-Path $Repo "dist\projectmapper-windows-amd64.exe"
if (-not (Test-Path -LiteralPath $SrcExe)) {
    throw "no encuentro $SrcExe — corré el script sin -NoBuild para compilarlo"
}

$SrcIcon = Join-Path $Repo "assets\$IconName"
if (-not (Test-Path -LiteralPath $SrcIcon)) {
    Write-Host "==> generando assets\$IconName"
    Push-Location $Repo
    try { go run ./scripts/mkicon } finally { Pop-Location }
}

# ---------- copiar a la carpeta de instalación ----------

$DataDir  = Join-Path $InstallDir "data"
$DataFile = Join-Path $DataDir "projects.json"
foreach ($dir in @($InstallDir, $DataDir, (Join-Path $InstallDir "export"))) {
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
}

$DstExe = Join-Path $InstallDir $ExeName

# Windows no deja reemplazar un .exe en uso: si la app está abierta, avisamos con
# claridad en vez de dejar que Copy-Item tire un error de "archivo en uso".
$running = Get-Process -ErrorAction SilentlyContinue |
    Where-Object { $_.Path -and $_.Path -eq $DstExe }
if ($running) {
    throw "Project Mapper está corriendo (PID $($running.Id -join ', ')) — cerrá su ventana de consola y volvé a correr el instalador"
}

Copy-Item -LiteralPath $SrcExe -Destination $DstExe -Force
Copy-Item -LiteralPath (Join-Path $Repo "data\seed.json") -Destination (Join-Path $DataDir "seed.json") -Force
if (Test-Path -LiteralPath $SrcIcon) {
    Copy-Item -LiteralPath $SrcIcon -Destination (Join-Path $InstallDir $IconName) -Force
}

# data\projects.json nunca se copia ni se sobreescribe: lo siembra el binario
# con -seed en el primer arranque y a partir de ahí es del usuario.
if (Test-Path -LiteralPath $DataFile) {
    Write-Host "==> conservando portafolio existente en $DataFile"
}

# ---------- crear los atajos ----------

$DstIcon = Join-Path $InstallDir $IconName
$shell = New-Object -ComObject WScript.Shell
foreach ($lnk in Get-ShortcutPaths) {
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $lnk) | Out-Null
    $sc = $shell.CreateShortcut($lnk)
    $sc.TargetPath       = $DstExe
    $sc.Arguments        = "-seed -port $Port -data `"$DataFile`""
    $sc.WorkingDirectory = $InstallDir
    $sc.Description      = "Project Mapper — mapa y priorización de actividades (local, 127.0.0.1)"
    $sc.WindowStyle      = 7   # minimizada: la consola queda en la barra de tareas
    if (Test-Path -LiteralPath $DstIcon) { $sc.IconLocation = $DstIcon }
    $sc.Save()
    Write-Host "atajo    $lnk"
}

Write-Host ""
Write-Host "listo. Doble click en '$Name' abre http://127.0.0.1:$Port en tu navegador."
Write-Host "  binario   $DstExe"
Write-Host "  datos     $DataFile"
Write-Host "  exports   $(Join-Path $InstallDir 'export')"
Write-Host "  detener   cerrá la ventana de consola minimizada (o Ctrl+C en ella)"
