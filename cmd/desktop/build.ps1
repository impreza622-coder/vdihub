# ============================================================
# 飞力达云桌面 - 完整构建打包脚本（带版本号自增）
# ============================================================
# 用法:
#   powershell -ExecutionPolicy Bypass -File .\build.ps1              # 默认: 自增版本号 + 完整构建
#   powershell -ExecutionPolicy Bypass -File .\build.ps1 -SkipIncrement  # 用当前版本号, 不自增
#   powershell -ExecutionPolicy Bypass -File .\build.ps1 -SkipWailsBuild # 跳过 wails build, 只重打包
#   powershell -ExecutionPolicy Bypass -File .\build.ps1 -SkipNsis       # 只构建主 exe, 不打包安装包
#   powershell -ExecutionPolicy Bypass -File .\build.ps1 -Version 1.005  # 指定版本号, 覆盖自增
#   powershell -ExecutionPolicy Bypass -File .\build.ps1 -NoOpenFolder  # 构建后不自动打开输出目录
#
# 版本号规则:
#   VERSION 文件存 "1.001" 格式, 每次构建自增 -> "1.002" -> "1.003" ...
#   显示名: V1.001  /  SemVer: 1.0.1  /  NSIS APP_VERSION: 1.0.1
#   构建失败不写回 VERSION, 重跑会用同一版本号（不浪费号段）
# ============================================================

param(
    [switch]$SkipIncrement,
    [switch]$SkipWailsBuild,
    [switch]$SkipNsis,
    [switch]$NoOpenFolder,
    [string]$Version
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $ScriptDir

# ====== 路径 / 环境配置（优先使用 PATH 中的工具，回退到 D:\kaifahuanjing） ======
function Find-Tool($name) {
    $cmd = Get-Command $name -ErrorAction SilentlyContinue
    if ($cmd -and $cmd.Source) { return $cmd.Source }
    if ($name -eq 'go') {
        foreach ($p in @("D:\kaifahuanjing\go\bin\go.exe", "C:\Program Files\Go\bin\go.exe")) { if (Test-Path $p) { return $p } }
    }
    if ($name -eq 'node') {
        foreach ($p in @("D:\kaifahuanjing\node\node.exe", "C:\Program Files\nodejs\node.exe")) { if (Test-Path $p) { return $p } }
    }
    if ($name -eq 'wails') {
        foreach ($p in @("D:\kaifahuanjing\go\bin\wails.exe", "$env:USERPROFILE\go\bin\wails.exe")) { if (Test-Path $p) { return $p } }
    }
    if ($name -eq 'makensis') {
        foreach ($p in @("D:\kaifahuanjing\NSIS\Bin\makensis.exe", "C:\Program Files (x86)\NSIS\Bin\makensis.exe", "C:\Program Files\NSIS\Bin\makensis.exe")) { if (Test-Path $p) { return $p } }
    }
    throw "$name not found in PATH or common install locations"
}
$GoExe    = Find-Tool 'go'
$NodeExe  = Find-Tool 'node'
$WailsExe = Find-Tool 'wails'
$NSIS_EXE = Find-Tool 'makensis'
# 把 Go/Node/NSIS 加入 PATH（wails build 内部会调用 go 和 node）
$toolDir = Split-Path $GoExe -Parent
if ($Env:Path -notlike "*$toolDir*") { $Env:Path = "$toolDir;$Env:Path" }
$nodeDir = Split-Path $NodeExe -Parent
if ($Env:Path -notlike "*$nodeDir*") { $Env:Path = "$nodeDir;$Env:Path" }
$nsisDir = Split-Path (Split-Path $NSIS_EXE -Parent) -Parent
if ($Env:Path -notlike "*$nsisDir*") { $Env:Path = "$nsisDir;$Env:Path" }

$ProjectRoot  = Split-Path (Split-Path $ScriptDir -Parent) -Parent
$VersionFile  = Join-Path $ScriptDir "VERSION"
$WailsJson    = Join-Path $ScriptDir "wails.json"
$InstallerNsi = Join-Path $ScriptDir "installer.nsi"
$ExePath      = Join-Path $ScriptDir "build\bin\vdihub.exe"
$InstallerRaw = Join-Path $ScriptDir "installer.exe"
$DistDir      = Join-Path $ProjectRoot "installer_out"

# ====== 输出辅助 ======
function Write-Step($i, $total, $msg) {
    Write-Host "" -NoNewline
    Write-Host ""
    Write-Host "[$i/$total] $msg" -ForegroundColor Yellow
}
function Write-OK($msg)    { Write-Host ("  [ OK ] " + $msg) -ForegroundColor Green }
function Write-Info($msg) { Write-Host ("  [INFO] " + $msg) -ForegroundColor DarkCyan }
function Write-Fail($msg) { Write-Host ("  [FAIL] " + $msg) -ForegroundColor Red }
function Check-Exists($path, $name) {
    if (-not (Test-Path $path)) { Write-Fail ("{0} not found: {1}" -f $name, $path); throw "Build environment is not complete." }
    Write-OK $name
}

# ====== 版本号解析/自增 ======
# "1.001" -> @{ Major=1; Build=1; Display="V1.001"; Semver="1.0.1"; Raw="1.001" }
function Parse-Version($v) {
    if ($v -notmatch '^\d+\.\d+$') { throw "Invalid version format: '$v' (expected like 1.001)" }
    $parts = $v -split '\.'
    $major = [int]$parts[0]
    $build = [int]$parts[1]
    return @{
        Major   = $major
        Build   = $build
        Display = "V{0}.{1:D3}" -f $major, $build
        Semver  = "{0}.0.{1}" -f $major, $build
        Raw     = "{0}.{1:D3}" -f $major, $build
    }
}
function Increment-Version($v) {
    $p = Parse-Version $v
    return "{0}.{1:D3}" -f $p.Major, ($p.Build + 1)
}

# UTF8 with BOM 读写（installer.nsi 原本就是 UTF8-BOM）
$Utf8Bom    = [System.Text.UTF8Encoding]::new($true)
$Utf8NoBom  = [System.Text.UTF8Encoding]::new($false)
function Read-File($p) { return [System.IO.File]::ReadAllText($p, $Utf8Bom) }
function Write-File($p, $content, $useBom) {
    $enc = if ($useBom) { $Utf8Bom } else { $Utf8NoBom }
    [System.IO.File]::WriteAllText($p, $content, $enc)
}

# ============================================================
Write-Host ""
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "  飞力达云桌面 - 完整构建打包脚本" -ForegroundColor Cyan
Write-Host ("  Working Dir: " + $ScriptDir) -ForegroundColor Cyan
Write-Host "============================================================" -ForegroundColor Cyan

# ====== [1/6] 环境检查 ======
Write-Step 1 6 "环境检查..."
Check-Exists $GoExe "Go"
Check-Exists $NodeExe "Node.js"
Check-Exists $WailsExe "Wails CLI"
Check-Exists $NSIS_EXE "NSIS makensis"
Check-Exists $WailsJson "wails.json"
Check-Exists $InstallerNsi "installer.nsi"

$HorizonInstaller = Get-ChildItem -LiteralPath $ProjectRoot -Filter "Omnissa-Horizon8-Client*.exe" | Select-Object -First 1
if (-not $HorizonInstaller) {
    # 回退：使用 prerequisites 中已有的副本（无需重新获取根目录安装包）
    $fallbackHorizon = Join-Path $ScriptDir "prerequisites\Omnissa-Horizon8-Client.exe"
    if (Test-Path $fallbackHorizon) {
        Write-Info "根目录未找到 Horizon 安装包, 使用 prerequisites 中的副本"
        $HorizonInstaller = Get-Item $fallbackHorizon
    }
}
if (-not $HorizonInstaller) { throw ("未找到 Omnissa Horizon 8 安装包 (根目录或 prerequisites): " + $ProjectRoot) }
Write-OK ("Omnissa Horizon 8 安装包: " + $HorizonInstaller.FullName)

$SafeHorizon = ".\prerequisites\Omnissa-Horizon8-Client.exe"
if (-not (Test-Path (Join-Path $ScriptDir "prerequisites"))) {
    New-Item -ItemType Directory -Path (Join-Path $ScriptDir "prerequisites") -Force | Out-Null
}
if ($HorizonInstaller.FullName -ne (Join-Path $ScriptDir "prerequisites\Omnissa-Horizon8-Client.exe")) {
    Copy-Item -LiteralPath $HorizonInstaller.FullName -Destination $SafeHorizon -Force
    Write-OK "已复制 Horizon 安装包到 prerequisites"
} else {
    Write-OK "Horizon 安装包已在 prerequisites"
}

# ====== [2/6] 版本号管理 ======
Write-Step 2 6 "版本号管理..."

# 确保 VERSION 文件存在
if (-not (Test-Path $VersionFile)) {
    "1.000" | Out-File -FilePath $VersionFile -Encoding ascii -NoNewline
    Write-Info "VERSION 文件不存在, 初始化为 1.000"
}
$rawVersion = (Get-Content $VersionFile -Raw -Encoding ascii).Trim()
$curP = Parse-Version $rawVersion
Write-OK ("当前版本号: " + $curP.Display + " (" + $curP.Semver + ")")

# 计算本次构建的版本号
if ($Version) {
    $newVersion = $Version
    Write-Info ("使用命令行指定版本号: " + $newVersion)
} elseif ($SkipIncrement) {
    $newVersion = $rawVersion
    Write-Info "跳过版本号自增 (-SkipIncrement), 使用当前版本"
} else {
    $newVersion = Increment-Version $rawVersion
    Write-OK ("版本号自增: " + $rawVersion + " -> " + $newVersion)
}

$newP = Parse-Version $newVersion
Write-OK ("本次构建版本: " + $newP.Display + " | SemVer: " + $newP.Semver)

# 同步更新 wails.json 的 productVersion
$wailsContent = Read-File $WailsJson
$wailsNew = [System.Text.RegularExpressions.Regex]::Replace(
    $wailsContent,
    '("productVersion"\s*:\s*")[^"]*(")',
    '${1}' + $newP.Semver + '${2}'
)
Write-File $WailsJson $wailsNew $false
Write-OK ("已更新 wails.json productVersion -> " + $newP.Semver)

# 同步更新 installer.nsi 的 APP_VERSION
$nsiContent = Read-File $InstallerNsi
$nsiNew = [System.Text.RegularExpressions.Regex]::Replace(
    $nsiContent,
    '(!define\s+APP_VERSION\s+")[^"]*(")',
    '${1}' + $newP.Semver + '${2}'
)
Write-File $InstallerNsi $nsiNew $true
Write-OK ("已更新 installer.nsi APP_VERSION -> " + $newP.Semver)

# ====== [3/6] Wails 构建 ======
if (-not $SkipWailsBuild) {
    Write-Step 3 6 "Wails 构建 (wails build -clean) ..."
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    & wails build -clean
    $wailsExit = $LASTEXITCODE
    $sw.Stop()
    if (-not (Test-Path $ExePath)) {
        throw ("wails build 失败 (exit=" + $wailsExit + "), 输出缺失: " + $ExePath)
    }
    $size = [math]::Round((Get-Item $ExePath).Length / 1MB, 1)
    Write-OK ("主程序构建完成: " + $ExePath + " (" + $size + " MB, " + $sw.Elapsed.ToString("mm\:ss") + ")")
} else {
    Write-Step 3 6 "跳过 Wails 构建 (-SkipWailsBuild)"
    if (-not (Test-Path $ExePath)) { throw ("跳过构建但主程序缺失: " + $ExePath) }
    Write-OK ("复用主程序: " + [math]::Round((Get-Item $ExePath).Length/1MB,1) + " MB")
}

# ====== [4/6] NSIS 打包 ======
if (-not $SkipNsis) {
    # 清理: 若上次的 installer.exe 被占用（旧安装程序仍在运行），先终止，避免 makensis "Can't open output file"
    if (Test-Path $InstallerRaw) {
        try {
            $fs = [System.IO.File]::Open($InstallerRaw, 'Open', 'ReadWrite', 'None')
            $fs.Close(); $fs.Dispose()
        } catch {
            Write-Info "检测到 installer.exe 被占用，终止占用进程..."
            Get-Process | Where-Object { $_.Path -eq $InstallerRaw } -ErrorAction SilentlyContinue | ForEach-Object {
                Write-Info ("  -> 终止 PID={0} ({1})" -f $_.Id, $_.ProcessName)
                Stop-Process -Id $_.Id -Force -ErrorAction SilentlyContinue
            }
            Start-Sleep -Seconds 2
        }
    }
    Write-Step 4 6 "NSIS 打包 (makensis) - LZMA 压缩中, 请稍候 ..."
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    & $NSIS_EXE ".\installer.nsi"
    $nsisExit = $LASTEXITCODE
    $sw.Stop()
    if ($nsisExit -ne 0 -or -not (Test-Path $InstallerRaw)) {
        throw ("makensis 失败 (exit=" + $nsisExit + ")")
    }
    $size = [math]::Round((Get-Item $InstallerRaw).Length / 1MB, 1)
    Write-OK ("安装包构建完成: " + $InstallerRaw + " (" + $size + " MB, " + $sw.Elapsed.ToString("hh\:mm\:ss") + ")")
} else {
    Write-Step 4 6 "跳过 NSIS 打包 (-SkipNsis)"
    if (-not (Test-Path $InstallerRaw)) { throw ("跳过打包但安装包缺失: " + $InstallerRaw) }
    Write-OK ("复用安装包: " + [math]::Round((Get-Item $InstallerRaw).Length/1MB,1) + " MB")
}

# ====== [5/6] 输出带版本号的产物 ======
Write-Step 5 6 "输出带版本号的产物..."
if (-not (Test-Path $DistDir)) { New-Item -ItemType Directory -Path $DistDir -Force | Out-Null }

$PkgName = "飞力达云桌面_" + $newP.Display + "_Setup.exe"
$PkgPath = Join-Path $DistDir $PkgName
Copy-Item -Path $InstallerRaw -Destination $PkgPath -Force
$pkgItem = Get-Item $PkgPath
Write-OK ("安装包: " + $pkgItem.FullName)
Write-OK ("大小  : " + [math]::Round($pkgItem.Length/1MB,1) + " MB")

$ExePkgName = "vdihub_" + $newP.Display + ".exe"
$ExePkgPath = Join-Path $DistDir $ExePkgName
Copy-Item -Path $ExePath -Destination $ExePkgPath -Force
Write-OK ("主程序: " + $ExePkgPath)

# 生成构建记录到 VERSION_HISTORY.log
$HistoryFile = Join-Path $ScriptDir "VERSION_HISTORY.log"
$histLine = "{0:yyyy-MM-dd HH:mm:ss}  {1}  ({2})  {3} MB" -f (Get-Date), $newP.Display, $newP.Semver, [math]::Round($pkgItem.Length/1MB,1)
Add-Content -Path $HistoryFile -Value $histLine -Encoding UTF8
Write-OK ("构建记录已追加到 VERSION_HISTORY.log")

# ★ 关键: 全部成功后才写回 VERSION 文件（失败不消耗版本号）
if (-not $SkipIncrement -and -not $Version) {
    $newP.Raw | Out-File -FilePath $VersionFile -Encoding ascii -NoNewline
    Write-OK ("已写回 VERSION 文件: " + $newP.Raw)
} else {
    Write-Info "未写回 VERSION 文件（指定版本号或跳过自增模式）"
}

# ====== [6/6] 构建摘要 ======
Write-Step 6 6 "完成"
Write-Host ""
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "  BUILD SUCCESS" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host ("  版本号      : " + $newP.Display + " (" + $newP.Semver + ")") -ForegroundColor Green
Write-Host ("  安装包      : " + $pkgItem.FullName) -ForegroundColor Green
Write-Host ("  安装包大小  : " + [math]::Round($pkgItem.Length/1MB,1) + " MB") -ForegroundColor Green
Write-Host ("  主程序      : " + $ExePkgPath) -ForegroundColor Green
Write-Host ("  生成时间    : " + $pkgItem.LastWriteTime) -ForegroundColor Green
Write-Host ""

# ====== 自动打开安装包所在目录（默认行为，构建和打包完成后立即执行） ======
# 用 explorer.exe /select, 打开目录并高亮选中安装包文件，便于用户立即取用
# 传入 -NoOpenFolder 可跳过此步（CI 场景）
if (-not $NoOpenFolder) {
    Write-Host "  -> 构建完成，自动打开安装包目录并选中安装包..." -ForegroundColor DarkCyan
    $selectArg = '/select,"' + $pkgItem.FullName + '"'
    try {
        Start-Process -FilePath "explorer.exe" -ArgumentList $selectArg -ErrorAction SilentlyContinue
        Write-OK ("已打开目录: " + $DistDir)
    } catch {
        # 回退方案: 仅打开目录（不选中文件）
        try { Start-Process -FilePath "explorer.exe" -ArgumentList $DistDir -ErrorAction SilentlyContinue }
        catch { Write-Info ("打开目录失败: " + $_.Exception.Message) }
    }
} else {
    Write-Info "已跳过自动打开目录 (-NoOpenFolder)"
}

Write-Host ""
Write-Host "  Tips:" -ForegroundColor DarkGray
Write-Host "    -SkipIncrement   不自增版本号, 用当前版本重新打包" -ForegroundColor DarkGray
Write-Host "    -SkipWailsBuild  跳过 wails build, 只重打包" -ForegroundColor DarkGray
Write-Host "    -SkipNsis        跳过 NSIS 打包, 只构建主 exe" -ForegroundColor DarkGray
Write-Host "    -Version 1.005   指定版本号 (不写回 VERSION 文件)" -ForegroundColor DarkGray
Write-Host "    -NoOpenFolder    构建后不自动打开输出目录" -ForegroundColor DarkGray
Write-Host ""
