# ============================================
# Horizon SSL 报错 VERIFY bora\lib\ssl\ssl.c:2384 - 诊断脚本
# 用法：在测试机上右键此文件 -> 使用 PowerShell 运行
# 结果会输出到桌面 horizon-diagnose-result.txt
# ============================================

$ErrorActionPreference = "Continue"
$OutputFile = "$env:USERPROFILE\Desktop\horizon-diagnose-result.txt"
$HorizonDir = "$PSScriptRoot\dependencies\clients\horizon"
if (-not (Test-Path $HorizonDir)) {
    $HorizonDir = "$PSScriptRoot"
}
if (-not (Test-Path "$HorizonDir\horizon-client.exe")) {
    $HorizonDir = Read-Host "请输入安装目录下的 horizon 目录 (如：C:\Program Files\ThinClientDesktop\dependencies\clients\horizon)"
}

"" | Out-File $OutputFile -Encoding UTF8
"=== Horizon SSL VERIFY 诊断报告  $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') ===" | Out-File $OutputFile -Append -Encoding UTF8
"" | Out-File $OutputFile -Append

"--- [1] 操作系统版本 ---" | Out-File $OutputFile -Append
Get-ComputerInfo | Select-Object WindowsProductName, WindowsVersion, OsHardwareAbstractionLayer | Format-List | Out-File $OutputFile -Append
"" | Out-File $OutputFile -Append

"--- [2] VC++ 2015-2022 Runtime 注册表 ---" | Out-File $OutputFile -Append
try {
    $vcr = Get-ItemProperty "HKLM:\SOFTWARE\Microsoft\VisualStudio\14.0\VC\Runtimes\x64" -ErrorAction Stop
    $vcr | Select-Object Installed, Version, Major, Minor, Build, Revision | Format-List | Out-File $OutputFile -Append
} catch {
    "ERROR: 注册表项不存在（VC++ Runtime 未安装）" | Out-File $OutputFile -Append
}
"" | Out-File $OutputFile -Append

"--- [3] FIPS 算法策略 ---" | Out-File $OutputFile -Append
try {
    $fips = Get-ItemProperty "HKLM:\SYSTEM\CurrentControlSet\Control\Lsa\FipsAlgorithmPolicy" -ErrorAction Stop
    "Enabled = $($fips.Enabled)" | Out-File $OutputFile -Append
} catch {
    "读取失败（可能未配置 FIPS）" | Out-File $OutputFile -Append
}
"" | Out-File $OutputFile -Append

"--- [4] .NET 8 运行时检测 ---" | Out-File $OutputFile -Append
$dotnetHomes = @(
    "$env:ProgramFiles\dotnet\shared\Microsoft.WindowsDesktop.App",
    "${env:ProgramFiles(x86)}\dotnet\shared\Microsoft.WindowsDesktop.App"
)
foreach ($home in $dotnetHomes) {
    if (Test-Path $home) {
        "目录: $home" | Out-File $OutputFile -Append
        Get-ChildItem $home -Directory | ForEach-Object { "  - $($_.Name)" } | Out-File $OutputFile -Append
    } else {
        "不存在: $home" | Out-File $OutputFile -Append
    }
}
"" | Out-File $OutputFile -Append

"--- [5] Horizon 关键文件 SHA-256 哈希 ---" | Out-File $OutputFile -Append
$files = @("horizon-client.exe","crtbora.dll","libssl-3-x64.dll","libcrypto-3-x64.dll","fips.dll")
foreach ($f in $files) {
    $path = Join-Path $HorizonDir $f
    if (Test-Path $path) {
        $h = (Get-FileHash $path -Algorithm SHA256).Hash
        "$f : $h" | Out-File $OutputFile -Append
    } else {
        "$f : MISSING!!" | Out-File $OutputFile -Append
    }
}
"--- 本机基准哈希（对比用）---" | Out-File $OutputFile -Append
"horizon-client.exe : 37448B63F61CFCCD855B1A964DA4D3AAD51FC0144BDB5FD9C466BBC08E2BE613" | Out-File $OutputFile -Append
"crtbora.dll       : AEF7CC1426A0646180DD6BA25CCD8EBEF26F167DCF3BA3A25EC14F10B7DA7762" | Out-File $OutputFile -Append
"libssl-3-x64.dll  : 61FD1B2538078B7089BB8167D3CD85DAD93BB0CCA8DECA8B422CDAB906FD73CC" | Out-File $OutputFile -Append
"libcrypto-3-x64.dll: A68C127580634B17085C04789E3330DD9AC50E6E1CDDD960F36C0841756CB772" | Out-File $OutputFile -Append
"fips.dll          : FAF0BA2EE58959F4996F6DF7880CC86B154B7FFAF15B82BC97F75EA992B4CC18" | Out-File $OutputFile -Append
"" | Out-File $OutputFile -Append

"--- [6] 系统 PATH 环境变量 ---" | Out-File $OutputFile -Append
$env:PATH -split ';' | Out-File $OutputFile -Append
"" | Out-File $OutputFile -Append

"--- [7] System32 下是否存在冲突的 SSL/Crypto DLL ---" | Out-File $OutputFile -Append
$checkDlls = @("libssl-3-x64.dll","libcrypto-3-x64.dll","crtbora.dll","libssl.dll","libcrypto.dll","libeay32.dll","ssleay32.dll","fips.dll")
foreach ($d in $checkDlls) {
    $p = Join-Path "$env:SystemRoot\System32" $d
    if (Test-Path $p) {
        "⚠ CONFLICT FOUND: $p" | Out-File $OutputFile -Append
    }
}
"完成（若上面没有出现任何 CONFLICT FOUND 即干净）" | Out-File $OutputFile -Append
"" | Out-File $OutputFile -Append

"--- [8] Horizon 临时日志目录 ---" | Out-File $OutputFile -Append
$logDir = "$env:LOCALAPPDATA\Temp\omnissa-$env:USERNAME"
"日志目录: $logDir" | Out-File $OutputFile -Append
if (Test-Path $logDir) {
    $logs = Get-ChildItem $logDir -Filter "horizon-crtbora-*.log" | Sort-Object LastWriteTime -Descending | Select-Object -First 3
    if ($logs) {
        "最近3个日志文件：" | Out-File $OutputFile -Append
        $logs | ForEach-Object {
            "  >> $($_.FullName) ($($_.Length) bytes, $($_.LastWriteTime))" | Out-File $OutputFile -Append
            "----- BEGIN $($_.Name) -----" | Out-File $OutputFile -Append
            Get-Content $_.FullName -Tail 100 -Encoding UTF8 | Out-File $OutputFile -Append
            "----- END $($_.Name) -----" | Out-File $OutputFile -Append
        }
    } else {
        "未找到 horizon-crtbora-*.log 日志" | Out-File $OutputFile -Append
    }
} else {
    "日志目录不存在" | Out-File $OutputFile -Append
}
"" | Out-File $OutputFile -Append

"=== 诊断完成 ===" | Out-File $OutputFile -Append
"诊断报告已保存到：$OutputFile" | Out-File $OutputFile -Append

Write-Host ""
Write-Host "============================================"
Write-Host "  诊断完成！"
Write-Host "  报告文件：$OutputFile"
Write-Host "============================================"
Write-Host ""
Write-Host "请将此文件发送给开发人员以定位问题。"
Write-Host ""
Invoke-Item "$env:USERPROFILE\Desktop"
pause
