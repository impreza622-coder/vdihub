# ============================================================
# Build Script: Generate Thin-Client Desktop Installer
# Usage:
#   Option A: Right-click -> Run with PowerShell
#   Option B: Under cmd\desktop:  powershell -ExecutionPolicy Bypass -File .\build-installer.ps1
#
# Required tools (all under D:\kaifahuanjing\):
#   - Go           : D:\kaifahuanjing\go\bin\go.exe
#   - Node.js      : D:\kaifahuanjing\node\node.exe
#   - Wails CLI    : D:\kaifahuanjing\go\bin\wails.exe
#   - NSIS         : D:\kaifahuanjing\NSIS\Bin\makensis.exe
# ============================================================

param(
    [switch]$SkipWailsBuild,
    [switch]$SkipNsis,
    [switch]$NoOpenFolder
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $ScriptDir

Write-Host ""
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "  Thin-Client Installer Build" -ForegroundColor Cyan
Write-Host ("  Working Dir: " + $ScriptDir) -ForegroundColor Cyan
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host ""

$Env:Path = "D:\kaifahuanjing\node;" + $Env:Path
$Env:Path = "D:\kaifahuanjing\go\bin;" + $Env:Path
$NSIS_EXE = "D:\kaifahuanjing\NSIS\Bin\makensis.exe"

function Check-Exists($path, $name) {
    if (-not (Test-Path $path)) {
        Write-Host ("  [FAIL] " + $name + " not found: " + $path) -ForegroundColor Red
        throw "Build environment is not complete."
    }
    Write-Host ("  [ OK ] " + $name) -ForegroundColor Green
}

Write-Host "[1/4] Environment check..." -ForegroundColor Yellow
Check-Exists "D:\kaifahuanjing\go\bin\go.exe"    "Go"
Check-Exists "D:\kaifahuanjing\node\node.exe"     "Node.js"
Check-Exists "D:\kaifahuanjing\go\bin\wails.exe"  "Wails CLI"
Check-Exists $NSIS_EXE                            "NSIS makensis"
Check-Exists ".\wails.json"                       "wails.json"
Check-Exists ".\installer.nsi"                    "installer.nsi"
# Resolve Omnissa installer by wildcard (avoids PowerShell 5 GBK encoding issues with Chinese filename)
$HorizonInstaller = Get-ChildItem -LiteralPath "..\.." -Filter "Omnissa-Horizon8-Client*.exe" | Select-Object -First 1
if (-not $HorizonInstaller) { throw "Omnissa Horizon 8.15 installer not found in project root." }
Write-Host ("  [ OK ] Omnissa Horizon 8.15 installer: " + $HorizonInstaller.Name) -ForegroundColor Green
Write-Host ""

# Copy to a safe (ASCII) temp filename under prerequisites so installer.nsi can reference it without encoding issues
# NSIS needs the source file to exist at a relative path we can hardcode in ANSI
$SafeHorizon = ".\prerequisites\Omnissa-Horizon8-Client.exe"
Copy-Item -LiteralPath $HorizonInstaller.FullName -Destination $SafeHorizon -Force
Write-Host ("  [ OK ] Copied Horizon installer to: " + $SafeHorizon) -ForegroundColor DarkCyan

$ExePath = ".\build\bin\thin-client-desktop.exe"
if (-not $SkipWailsBuild) {
    Write-Host "[2/4] Wails build (wails build -clean) ..." -ForegroundColor Yellow
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    & wails build -clean
    $wailsExit = $LASTEXITCODE
    $sw.Stop()
    if (($wailsExit -ne 0) -and (-not (Test-Path $ExePath))) {
        throw ("wails build failed (exit=" + $wailsExit + ")")
    }
    if (Test-Path $ExePath) {
        $size = [math]::Round((Get-Item $ExePath).Length / 1MB, 1)
        Write-Host ("  [ OK ] Main EXE ready: " + $ExePath + " (" + $size + " MB, " + $sw.Elapsed.ToString("mm\:ss") + ")") -ForegroundColor Green
    } else {
        throw ("wails build finished but output missing: " + $ExePath)
    }
} else {
    Write-Host "[2/4] Skipping Wails build (--SkipWailsBuild)" -ForegroundColor Yellow
    if (-not (Test-Path $ExePath)) {
        throw ("SkipWailsBuild but missing: " + $ExePath)
    }
    $size = [math]::Round((Get-Item $ExePath).Length / 1MB, 1)
    Write-Host ("  [ OK ] Reuse main EXE: " + $ExePath + " (" + $size + " MB)") -ForegroundColor Green
}
Write-Host ""

Write-Host "[3/4] Verify main EXE ..." -ForegroundColor Yellow
if (-not (Test-Path $ExePath)) { throw ("Main EXE missing: " + $ExePath) }
$exeItem = Get-Item $ExePath
Write-Host ("  [ OK ] thin-client-desktop.exe: " + [math]::Round($exeItem.Length/1MB,1) + " MB, " + $exeItem.LastWriteTime) -ForegroundColor Green
Write-Host ""

$InstallerPath = ".\installer.exe"
if (-not $SkipNsis) {
    Write-Host "[4/4] NSIS build (makensis) — LZMA compressing, please wait ..." -ForegroundColor Yellow
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    & $NSIS_EXE ".\installer.nsi"
    $nsisExit = $LASTEXITCODE
    $sw.Stop()
    if ($nsisExit -ne 0) {
        throw ("makensis failed (exit=" + $nsisExit + ")")
    }
    if (-not (Test-Path $InstallerPath)) {
        throw ("makensis finished but installer missing: " + $InstallerPath)
    }
    $pkgItem = Get-Item $InstallerPath
    Write-Host ("  [ OK ] Installer built: " + $InstallerPath) -ForegroundColor Green
    Write-Host ("         Size : " + [math]::Round($pkgItem.Length / 1MB, 1) + " MB") -ForegroundColor Green
    Write-Host ("         Time : " + $pkgItem.LastWriteTime) -ForegroundColor Green
    Write-Host ("         Spent: " + $sw.Elapsed.ToString("hh\:mm\:ss")) -ForegroundColor Green
} else {
    Write-Host "[4/4] Skipping NSIS build (--SkipNsis)" -ForegroundColor Yellow
}
Write-Host ""

Write-Host "  Done." -ForegroundColor Yellow
Write-Host ""
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "  BUILD SUCCESS" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Cyan
if (Test-Path $InstallerPath) {
    $pkgItem = Get-Item $InstallerPath
    Write-Host ("  Installer file : " + $pkgItem.FullName)
    Write-Host ("  Installer size : " + [math]::Round($pkgItem.Length / 1MB, 1) + " MB")
    Write-Host ("  Generated at   : " + $pkgItem.LastWriteTime)
}
Write-Host ("  Main EXE       : " + (Resolve-Path $ExePath).Path)
Write-Host ""

if (-not $NoOpenFolder) {
    Write-Host "  -> Opening installer folder in Explorer ..." -ForegroundColor DarkCyan
    explorer.exe (Resolve-Path ".").Path
}

Write-Host ""
Write-Host "  Tips: Use --SkipWailsBuild to reuse existing build"
Write-Host "        Use --SkipNsis      to skip NSIS packaging"
Write-Host ""
