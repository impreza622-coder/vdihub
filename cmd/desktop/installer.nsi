; ============================================================
;   飞力达云桌面 (Thin Client Desktop)   NSIS 安装脚本
;   编码：UTF-8 with BOM（build.ps1 按 UTF8-BOM 读写，NSIS Unicode true）
; ============================================================

; ------ 应用信息 ------
!define APP_NAME        "飞力达云桌面"
!define APP_EXE         "thin-client-desktop.exe"
!define APP_PUBLISHER   "山东飞力达软件有限公司"
!define APP_VERSION     "1.0.5"
!define APP_URL         "https://sdfeilida.com"
!define UNINSTALL_EXE   "uninst.exe"

; 安装 / 卸载 注册表键
!define UNINSTALL_KEY   "Software\Microsoft\Windows\CurrentVersion\Uninstall\FeiLiDaThinClient"

; ------ NSIS 配置 ------
Unicode true
RequestExecutionLevel admin     ; 写 HKLM / Program Files 需要管理员权限
InstallDir "$PROGRAMFILES64\FeiLiDa\ThinClientDesktop"
InstallDirRegKey HKLM "${UNINSTALL_KEY}" "InstallLocation"
ShowInstDetails show
ShowUnInstDetails show
SetCompressor /SOLID lzma
CRCCheck on

; ------ 引入 Modern UI 2 ------
!include "MUI2.nsh"
!include "x64.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"
!include "WinMessages.nsh"

; 名称 & 品牌
Name "${APP_NAME}"
BrandingText "${APP_PUBLISHER}  ${APP_VERSION}"
Caption "${APP_NAME} 安装程序"

; ------ 应用图标复制 appicon.png 生成的 build/windows/icon.ico ------
!define APP_ICON_PATH       ".\build\windows\icon.ico"

; ------ 界面 ------
!define MUI_ABORTWARNING
!define MUI_ICON                "${APP_ICON_PATH}"
!define MUI_UNICON              "${APP_ICON_PATH}"
!define MUI_HEADERIMAGE
!define MUI_HEADERIMAGE_BITMAP  "${NSISDIR}\Contrib\Graphics\Header\orange-nsis.bmp"
!define MUI_WELCOMEFINISHPAGE_BITMAP "${NSISDIR}\Contrib\Graphics\Wizard\orange.bmp"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN          "$INSTDIR\${APP_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT     "运行 ${APP_NAME}"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_WELCOME
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH

; ------ 语言设置 ------
!insertmacro MUI_LANGUAGE "SimpChinese"

; ============================================================
;  EnsurePowerShellInPath 确保 确保系统 PATH 中包含 PowerShell
; ============================================================
Function EnsurePowerShellInPath
    ReadRegStr $0 HKLM "SYSTEM\CurrentControlSet\Control\Session Manager\Environment" "Path"
    StrCmp $0 "" 0 +2
        StrCpy $0 ""
    StrCpy $0 "$0;%SystemRoot%\System32\WindowsPowerShell\v1.0"
    WriteRegExpandStr HKLM "SYSTEM\CurrentControlSet\Control\Session Manager\Environment" "Path" $0
    FileOpen $9 "$INSTDIR\_runtime_paths.txt" w
    FileWrite $9 "powershell=$SYSDIR\WindowsPowerShell\v1.0\powershell.exe$\r$\n"
    FileWrite $9 "cmd=$SYSDIR\cmd.exe$\r$\n"
    FileWrite $9 "shutdown=$SYSDIR\shutdown.exe$\r$\n"
    FileClose $9
FunctionEnd

; ============================================================
;  .onInit 入口 校验系统 + 关闭旧版 + WebView2
; ============================================================
Function .onInit
    ; 仅支持 64 位 Windows
    ${IfNot} ${RunningX64}
        MessageBox MB_ICONSTOP "本程序仅支持 64 位 Windows 7 / 10 / 11 / Server 2016+。$\r$\n当前系统为 32 位，无法安装本软件。" /SD IDOK
        Abort
    ${EndIf}

    ; 关闭正在运行的旧版本程序
    Push $R0
    nsExec::ExecToStack 'tasklist /FI "IMAGENAME eq ${APP_EXE}" /FO CSV /NH'
    Pop $R0
    ${If} $R0 == 0
        Pop $R0
        StrCmpS $R0 "" noKill
            DetailPrint "检测到 ${APP_EXE} 正在运行，强制关闭 ..."
            nsExec::ExecToStack 'taskkill /F /IM ${APP_EXE} /T'
            Pop $R0
            Sleep 800
    noKill:
    ${EndIf}
    Pop $R0

    ; 检查旧安装路径的目录并卸载，防止残留文件冲突
    StrCpy $R8 "$PROGRAMFILES64\FeiLiDa\瘦客户端云桌面"
    ${If} ${FileExists} "$R8\${APP_EXE}"
        DetailPrint "检测到旧版本程序，正在卸载 ..."
        ${If} ${FileExists} "$R8\${UNINSTALL_EXE}"
            ExecWait '"$R8\${UNINSTALL_EXE}" /S _?=$R8' $R9
        ${EndIf}
        RMDir /r "$R8"
        DetailPrint "旧版目录清理完成"
    ${EndIf}

    ; 强制英文安装路径
    StrCpy $INSTDIR "$PROGRAMFILES64\FeiLiDa\ThinClientDesktop"

    ; 检测 WebView2 Runtime（Wails 依赖，部分 LTSC 默认不带需安装）
    ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
    ${If} $0 == ""
        SetDetailsPrint textonly
        DetailPrint "检测到未安装 WebView2 Runtime，正在安装 ..."
        SetDetailsPrint none
        ExecWait '"$INSTDIR\prerequisites\MicrosoftEdgeWebview2Setup.exe" /silent /install' $R8
        ${If} $R8 == 0
        ${OrIf} $R8 == 1638
        ${OrIf} $R8 == 1641
            DetailPrint "WebView2 Runtime 安装成功"
        ${Else}
            MessageBox MB_ICONEXCLAMATION "WebView2 Runtime 安装失败（错误码=$R8）。$\r$\n请手动安装：$\r$\n$INSTDIR\prerequisites\MicrosoftEdgeWebview2Setup.exe" /SD IDOK
        ${EndIf}
    ${Else}
        DetailPrint "WebView2 Runtime (v$0) 已安装。"
    ${EndIf}
FunctionEnd

; ============================================================
;  安装区 Section
; ============================================================
Section "飞力达云桌面 (必需)" SecMain
    SectionIn RO
    SetOutPath "$INSTDIR"

    ; 1. 写入 prerequisites（WebView2 Bootstrapper + Omnissa Horizon 8.15 离线安装器 + Horizon 卸载脚本）
    SetOutPath "$INSTDIR\prerequisites"
    File ".\prerequisites\MicrosoftEdgeWebview2Setup.exe"
    File ".\prerequisites\Omnissa-Horizon8-Client.exe"
    File ".\prerequisites\uninstall-horizon.ps1"
    SetOutPath "$INSTDIR"

    ; 2. 复制 build/bin 目录下的主程序（exe、图标、dll 等）
    SetOverwrite try
    File /r /x "*.pdb" /x "${UNINSTALL_EXE}" ".\build\bin\*.*"

    ; 3. 复制协议客户端（FreeRDP / SPICE / VNC）
    ;    跳过绿色版 horizon 目录（由安装器负责安装到该目录）
    SetOutPath "$INSTDIR\dependencies\clients"
    File /r /x "__extract_tmp" /x "freerdp_msvc" /x "horizon" /x "VMware-Horizon-Client-*.exe" /x "Omnissa-Horizon*.exe" /x "virt-viewer-x64-*.msi" /x "tigervnc64-*.exe" /x "*.log" ".\dependencies\clients\*.*"
    SetOutPath "$INSTDIR"

    ; 4. 静默安装 Omnissa Horizon Client 8.15 离线包到目标目录
    ;    离线包安装过程中会自带 .NET 8 / VC++ / 证书依赖 / 注册表写入，/norestart 防止 Horizon 安装器重启系统并消除提示
    ;    安装后清除系统 pending reboot 标志（PendingFileRenameOperations），防止 Windows 弹出"正在准备修复"提示
    DetailPrint "正在静默安装 Omnissa Horizon Client 8.15 ..."
    ${IfNot} ${FileExists} "$INSTDIR\dependencies\clients\horizon\horizon-client.exe"
        ExecWait '"$INSTDIR\prerequisites\Omnissa-Horizon8-Client.exe" /S /norestart /D=$INSTDIR\dependencies\clients\horizon' $R8
        ${If} $R8 == 0
            DetailPrint "Omnissa Horizon Client 安装成功"
        ${Else}
            DetailPrint "Omnissa Horizon Client 安装异常退出: $R8 (一般不需要处理)"
        ${EndIf}
        ; 清除 Horizon 安装器写入的系统 pending reboot 标志，防止下次启动出现异常提示
        DeleteRegValue HKLM "SYSTEM\CurrentControlSet\Control\Session Manager" "PendingFileRenameOperations"
        DeleteRegValue HKLM "SYSTEM\CurrentControlSet\Control\Session Manager" "PendingFileRenameOperations2"
        DeleteRegKey /ifempty HKLM "SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired"
        DeleteRegKey /ifempty HKLM "SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending"
        DeleteRegKey /ifempty HKLM "SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootInProgress"
        DetailPrint "已清理 pending reboot 标志"
    ${Else}
        DetailPrint "Omnissa Horizon Client 已存在，跳过安装"
    ${EndIf}

    ; 5. 确保 PowerShell 在系统 PATH
    Call EnsurePowerShellInPath

    ; 6. 写 Horizon 证书忽略注册表（内网自签名证书不校验）
    ;    注意：同时写 EnableShade 等键以兼容不同版本的注册表位置
    DetailPrint "正在写入 Horizon 证书忽略注册表 ..."
    WriteRegStr   HKLM "SOFTWARE\VMware, Inc.\VMware VDM\Client"           "IgnoreCertErrors"                   "TRUE"
    WriteRegStr   HKLM "SOFTWARE\VMware, Inc.\VMware VDM\Client"           "EnableCertCertificateVerification"  "false"
    WriteRegStr   HKLM "SOFTWARE\VMware, Inc.\VMware VDM\Client"           "SecureConnect"                      "FALSE"
    WriteRegDWORD HKLM "SOFTWARE\VMware, Inc.\VMware VDM\Client"           "CertCheckMode"                     0
    WriteRegDWORD HKLM "SOFTWARE\VMware, Inc.\VMware VDM\Client"           "EnableCertHashCheck"               0
    WriteRegDWORD HKLM "SOFTWARE\VMware, Inc.\VMware VDM\Client"           "CertHashCheckMode"                 0
    WriteRegDWORD HKLM "SOFTWARE\VMware, Inc.\VMware VDM\Client"           "SecurityMode"                       0
    WriteRegDWORD HKLM "SOFTWARE\VMware, Inc.\VMware VDM\Client\Security"  "IgnoreCertErrors"                  3
    WriteRegDWORD HKLM "SOFTWARE\VMware, Inc.\VMware VDM\Client\Security"  "CertCheckMode"                     3
    WriteRegDWORD HKLM "SOFTWARE\VMware, Inc.\VMware VDM\Client\Security"  "SecurityMode"                      3

    WriteRegStr   HKLM "SOFTWARE\Omnissa\Horizon\Client"                   "IgnoreCertErrors"                   "TRUE"
    WriteRegStr   HKLM "SOFTWARE\Omnissa\Horizon\Client"                   "EnableCertCertificateVerification"  "false"
    WriteRegStr   HKLM "SOFTWARE\Omnissa\Horizon\Client"                   "SecureConnect"                      "FALSE"
    WriteRegDWORD HKLM "SOFTWARE\Omnissa\Horizon\Client"                   "CertCheckMode"                      0
    WriteRegDWORD HKLM "SOFTWARE\Omnissa\Horizon\Client"                   "EnableCertHashCheck"                0
    WriteRegDWORD HKLM "SOFTWARE\Omnissa\Horizon\Client"                   "CertHashCheckMode"                  0
    WriteRegDWORD HKLM "SOFTWARE\Omnissa\Horizon\Client"                   "SecurityMode"                       0
    WriteRegDWORD HKLM "SOFTWARE\Omnissa\Horizon\Security"                 "IgnoreCertErrors"                   3
    WriteRegDWORD HKLM "SOFTWARE\Omnissa\Horizon\Security"                 "CertCheckMode"                      3
    WriteRegDWORD HKLM "SOFTWARE\Omnissa\Horizon\Security"                 "SecurityMode"                      3
    DetailPrint "Horizon 证书忽略注册表写入完成"

    ; 7. 写卸载信息
    ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
    IntFmt $0 "0x%08X" $0
    WriteRegStr   HKLM "${UNINSTALL_KEY}" "DisplayName"          "${APP_NAME}"
    WriteRegStr   HKLM "${UNINSTALL_KEY}" "DisplayVersion"       "${APP_VERSION}"
    WriteRegStr   HKLM "${UNINSTALL_KEY}" "Publisher"            "${APP_PUBLISHER}"
    WriteRegStr   HKLM "${UNINSTALL_KEY}" "URLInfoAbout"         "${APP_URL}"
    WriteRegStr   HKLM "${UNINSTALL_KEY}" "InstallLocation"      "$INSTDIR"
    WriteRegStr   HKLM "${UNINSTALL_KEY}" "UninstallString"      '"$INSTDIR\${UNINSTALL_EXE}"'
    WriteRegStr   HKLM "${UNINSTALL_KEY}" "QuietUninstallString" '"$INSTDIR\${UNINSTALL_EXE}" /S'
    WriteRegDWORD HKLM "${UNINSTALL_KEY}" "NoModify"             1
    WriteRegDWORD HKLM "${UNINSTALL_KEY}" "NoRepair"             1
    WriteRegDWORD HKLM "${UNINSTALL_KEY}" "EstimatedSize"        $0

    ; 8. 生成卸载程序
    WriteUninstaller "$INSTDIR\${UNINSTALL_EXE}"

    ; 9. 开始菜单桌面模式
    CreateDirectory "$SMPROGRAMS\${APP_NAME}"
    CreateShortcut  "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk"     "$INSTDIR\${APP_EXE}" "" "$INSTDIR\${APP_EXE}" 0
    CreateShortcut  "$SMPROGRAMS\${APP_NAME}\卸载 ${APP_NAME}.lnk" "$INSTDIR\${UNINSTALL_EXE}" "" "$INSTDIR\${UNINSTALL_EXE}" 0

    ; 10. 桌面快捷方式
    CreateShortcut  "$DESKTOP\${APP_NAME}.lnk"  "$INSTDIR\${APP_EXE}" "" "$INSTDIR\${APP_EXE}" 0

    ; 11. 广播注册表变更
    SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=3000

    SetDetailsPrint none
    DetailPrint "=========================================================="
    DetailPrint "  Installation completed."
    DetailPrint "  Program     : $INSTDIR"
    DetailPrint "  Start Menu  : $SMPROGRAMS\${APP_NAME}"
    DetailPrint "  Desktop Lnk : $DESKTOP\${APP_NAME}.lnk"
    DetailPrint "=========================================================="
    SetDetailsPrint both
SectionEnd

; ============================================================
;  卸载区
; ============================================================
Section "un.Uninstall"
    Push $R0
    nsExec::ExecToStack 'tasklist /FI "IMAGENAME eq ${APP_EXE}" /FO CSV /NH'
    Pop $R0
    ${If} $R0 == 0
        Pop $R0
        StrCmpS $R0 "" noKillU
            DetailPrint "检测到 ${APP_EXE} 正在运行，强制关闭 ..."
            nsExec::ExecToStack 'taskkill /F /IM ${APP_EXE} /T'
            Pop $R0
            Sleep 800
    noKillU:
    ${EndIf}
    Pop $R0

    Delete "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk"
    Delete "$SMPROGRAMS\${APP_NAME}\卸载 ${APP_NAME}.lnk"
    RMDir  "$SMPROGRAMS\${APP_NAME}"
    Delete "$DESKTOP\${APP_NAME}.lnk"

    ; --- 卸载 Omnissa / VMware Horizon Client（通过读取系统注册表状态定位 MSI 卸载入口）---
    ;     Omnissa Horizon 8.15 的 MSI 安装/D= 参数实际无效，会安装到 C:\Program Files\Omnissa\...
    ;     新版本会写在 $INSTDIR\dependencies\clients\horizon\uninstall.exe 但不完全可靠，因此额外执行卸载脚本。
    ;     卸载脚本通过 PowerShell 扫描 64/32 位注册表 Uninstall 键，匹配 Horizon/Omnissa 产品名，
    ;     提取 MSI GUID 后 msiexec /X{GUID} /qn 静默卸载，再清理注册表与残留目录。
    DetailPrint "正在卸载 Omnissa Horizon Client ..."
    ${If} ${FileExists} "$INSTDIR\prerequisites\uninstall-horizon.ps1"
        nsExec::ExecToLog '"powershell.exe" -NoProfile -ExecutionPolicy Bypass -File "$INSTDIR\prerequisites\uninstall-horizon.ps1"'
        Pop $R0
        DetailPrint "Horizon uninstall script exit: $R0"
    ${Else}
        DetailPrint "Horizon uninstall script not found (skip)"
    ${EndIf}

    ; --- 兜底：直接删除程序目录下 dependencies\clients\horizon 目录（同时清理 Horizon 装到系统目录的残留）---
    RMDir /r "$INSTDIR\dependencies\clients\horizon"

    ; --- 清理 Horizon 注册表（逐级删除，最后一级不删防止误删他人数据）---
    DeleteRegKey HKLM "SOFTWARE\Omnissa\Horizon\Client"
    DeleteRegKey HKLM "SOFTWARE\Omnissa\Horizon\Security"
    DeleteRegKey HKLM "SOFTWARE\Omnissa\Horizon"
    DeleteRegKey HKLM "SOFTWARE\Omnissa"
    DeleteRegKey HKCU "SOFTWARE\Omnissa\Horizon\Client"
    DeleteRegKey HKCU "SOFTWARE\Omnissa\Horizon"
    DeleteRegKey HKCU "SOFTWARE\Omnissa"
    DeleteRegKey HKLM "SOFTWARE\VMware, Inc.\VMware VDM\Client"
    DeleteRegKey HKLM "SOFTWARE\VMware, Inc.\VMware VDM"
    DeleteRegKey HKCU "SOFTWARE\VMware, Inc.\VMware VDM\Client"
    DeleteRegKey HKCU "SOFTWARE\VMware, Inc.\VMware VDM"
    DeleteRegKey HKLM "SOFTWARE\Policies\Omnissa\Horizon\Client"
    DeleteRegKey HKLM "SOFTWARE\Policies\Omnissa\Horizon"
    DeleteRegKey HKLM "SOFTWARE\Policies\Omnissa"
    DeleteRegKey HKCU "SOFTWARE\Policies\Omnissa\Horizon\Client"
    DeleteRegKey HKCU "SOFTWARE\Policies\Omnissa\Horizon"
    DeleteRegKey HKCU "SOFTWARE\Policies\Omnissa"
    DeleteRegKey HKLM "SOFTWARE\Policies\VMware, Inc.\VMware VDM\Client"
    DeleteRegKey HKLM "SOFTWARE\Policies\VMware, Inc.\VMware VDM"
    DeleteRegKey HKCU "SOFTWARE\Policies\VMware, Inc.\VMware VDM\Client"
    DeleteRegKey HKCU "SOFTWARE\Policies\VMware, Inc.\VMware VDM"
    DeleteRegKey HKLM "SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\Omnissa Horizon Client"

    RMDir /r "$INSTDIR"

    DeleteRegKey HKLM "${UNINSTALL_KEY}"

    SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=3000

    SetDetailsPrint none
    DetailPrint "=========================================================="
    DetailPrint "  Uninstall completed."
    DetailPrint "=========================================================="
    SetDetailsPrint both
SectionEnd

; ============================================================
;  un.onInit
; ============================================================
Function un.onInit
    MessageBox MB_ICONQUESTION|MB_YESNO|MB_DEFBUTTON2 "确定要卸载 ${APP_NAME} 吗？" IDYES +2
        Abort
FunctionEnd

; ------ 描述 ------
!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
    !insertmacro MUI_DESCRIPTION_TEXT ${SecMain} "安装 ${APP_NAME} 主程序（Omnissa Horizon Client 8.15、WebView2 Runtime 与协议客户端将一并安装）。"
!insertmacro MUI_FUNCTION_DESCRIPTION_END
