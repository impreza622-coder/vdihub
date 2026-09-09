; ================================================================
;  云桌面客户端 (Thin Client Desktop)  NSIS 安装脚本
;  产物：build\bin\云桌面客户端_Setup_v1.0.0.exe
; ================================================================

; ------ 基本配置 ------
!define APP_NAME         "云桌面客户端"
!define APP_EXE          "thin-client-desktop.exe"
!define APP_PUBLISHER    "山东飞力达软件有限公司"
!define APP_VERSION      "1.0.0"
!define APP_URL          "https://www.feilida.com"
!define UNINSTALL_EXE    "uninst.exe"

; 安装 / 卸载 注册表子键
!define UNINSTALL_KEY    "Software\Microsoft\Windows\CurrentVersion\Uninstall\FeiLiDaThinClient"

; ------ NSIS 通用 ------
Unicode true
RequestExecutionLevel admin   ; 写 HKLM / Program Files 需要管理员
InstallDir "$PROGRAMFILES64\FeiLiDa\${APP_NAME}"
InstallDirRegKey HKLM "${UNINSTALL_KEY}" "InstallLocation"
ShowInstDetails show
ShowUnInstDetails show
SetCompressor /SOLID lzma
CRCCheck on

; ------ 界面：Modern UI 2 ------
!include "MUI2.nsh"
!include "x64.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"

; 品牌色 & 标题
Name "${APP_NAME}"
BrandingText "${APP_PUBLISHER}  ${APP_VERSION}"
Caption "${APP_NAME} 安装向导"

; ------ 品牌图标（使用项目内 appicon.png 生成的 build/windows/icon.ico） ------
!define APP_ICON_PATH        ".\build\windows\icon.ico"

; ------ 页面 ------
!define MUI_ABORTWARNING
!define MUI_ICON             "${APP_ICON_PATH}"
!define MUI_UNICON           "${APP_ICON_PATH}"
!define MUI_HEADERIMAGE
!define MUI_HEADERIMAGE_BITMAP "${NSISDIR}\Contrib\Graphics\Header\orange-nsis.bmp"
!define MUI_WELCOMEFINISHPAGE_BITMAP "${NSISDIR}\Contrib\Graphics\Wizard\orange.bmp"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN           "$INSTDIR\${APP_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT      "启动 ${APP_NAME}"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_PAGE_CUSTOMFUNCTION_SHOW FinishShow
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_WELCOME
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH

; ------ 语言：简体中文 ------
!insertmacro MUI_LANGUAGE "SimpChinese"

; ================================================================
;  FinishShow —— 完成页自定义显示：若安装了 .NET 则提示需重启
; ================================================================
Function FinishShow
    ReadIniStr $0 "$INSTDIR\_need_reboot.ini" "Flags" "NeedRebootAfterInstall"
    ${If} $0 == "1"
        ; 用 MUI_FINISHPAGE_RUN 的复选框无法自定义两个选项，这里弹一个 MessageBox
        MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON1 \
            "检测到安装了 .NET Framework 4.8，建议立即重启系统以生效。$\r$\n$\r$\n是否现在重启？" \
            /SD IDYES IDYES doReboot IDNO noReboot
    doReboot:
        ; 关安装程序 + 重启系统
        ExecWait '"$SYSDIR\shutdown.exe" /r /t 5 /c "云桌面客户端安装完成，为启用 .NET 4.8 正在重启系统..."'
    noReboot:
    ${EndIf}
FunctionEnd

; ================================================================
;  .onInit  —— 检测系统架构 & WebView2 Runtime
; ================================================================
; ================================================================
;  依赖检测宏 / 函数
;  .NET 4.8 Release 注册表 ≥ 528040 = 已安装
;  PowerShell 默认在 system32\WindowsPowerShell\v1.0\powershell.exe
; ================================================================
Function EnsurePowerShellInPath
    ; 一些精简版系统 PATH 里漏了 PowerShell 目录，导致我们的程序报
    ; "exec: \"powershell.exe\": executable file not found in %PATH%"
    ; 这里确保把系统 PowerShell 目录写入系统级 PATH 环境变量
    ReadRegStr $0 HKLM "SYSTEM\CurrentControlSet\Control\Session Manager\Environment" "Path"
    ; 直接追加 PowerShell 路径（重复无害，避免依赖 StrFunc 插件）
    StrCmp $0 "" 0 +2
        StrCpy $0 ""
    StrCpy $0 "$0;%SystemRoot%\System32\WindowsPowerShell\v1.0"
    WriteRegExpandStr HKLM "SYSTEM\CurrentControlSet\Control\Session Manager\Environment" "Path" $0
    ; 把 powershell.exe 完整路径写一份到我们自己的安装目录下的 path_helper.txt，程序启动时也可以读它回退
    FileOpen $9 "$INSTDIR\_runtime_paths.txt" w
    FileWrite $9 "powershell=$SYSDIR\WindowsPowerShell\v1.0\powershell.exe$\r$\n"
    FileWrite $9 "cmd=$SYSDIR\cmd.exe$\r$\n"
    FileWrite $9 "shutdown=$SYSDIR\shutdown.exe$\r$\n"
    FileClose $9
FunctionEnd

Function CheckAndInstallDotNet48
    ; 读 .NET 4.x Full Release 值
    ClearErrors
    ReadRegDWORD $R9 HKLM "SOFTWARE\Microsoft\NET Framework Setup\NDP\v4\Full" "Release"
    IfErrors dotnet_need_install
    IntCmp $R9 528040 dotnet_ok dotnet_ok dotnet_need_install

dotnet_need_install:
    SetDetailsPrint textonly
    DetailPrint "检测到系统缺少 .NET Framework 4.8，正在自动安装…（约需 1-3 分钟，请耐心等待）"
    SetDetailsPrint none

    ; 把内嵌的安装包释放到临时目录
    InitPluginsDir
    File /oname=$PLUGINSDIR\ndp48.exe ".\prerequisites\ndp48-x86-x64-allos-enu.exe"

    ; 静默安装：/passive=仅显示进度条 /AcceptEULA=自动同意 /norestart=禁止自动重启
    ; 安装程序返回 0=成功, 1641=成功且需要重启, 3010=成功但需要重启
    nsExec::ExecToStack '"$PLUGINSDIR\ndp48.exe" /passive /AcceptEULA /norestart'
    Pop $R8   ; return code of nsExec (0=success)
    Pop $R7   ; exit code of the dotnet installer

    ${If} $R8 != 0
        ; nsExec 本身执行失败（极端情况），改用 ExecWait
        ExecWait '"$PLUGINSDIR\ndp48.exe" /passive /AcceptEULA /norestart' $R7
    ${EndIf}

    ; 0 / 1641 / 3010 都算成功
    ${If} $R7 == 0
    ${OrIf} $R7 == 1641
    ${OrIf} $R7 == 3010
        DetailPrint ".NET Framework 4.8 安装成功，安装完成后请重启系统。"
        ; 标记需要重启
        WriteIniStr "$INSTDIR\_need_reboot.ini" "Flags" "NeedRebootAfterInstall" "1"
    ${Else}
        MessageBox MB_ICONEXCLAMATION ".NET Framework 4.8 自动安装失败（返回码=$R7）。$\r$\n请在安装完成后手动运行：$\r$\n$INSTDIR\prerequisites\ndp48-x86-x64-allos-enu.exe" /SD IDOK
    ${EndIf}
    Goto dotnet_done

dotnet_ok:
    DetailPrint ".NET Framework 4.8 已安装，跳过。"

dotnet_done:
FunctionEnd

Function .onInit
    ; 必须 64 位 Windows
    ${IfNot} ${RunningX64}
        MessageBox MB_ICONSTOP "本软件仅支持 64 位 Windows 7 / 10 / 11 / Server 2016+。$\r$\n当前系统为 32 位，无法安装。" /SD IDOK
        Abort
    ${EndIf}

    ; 关闭旧版本进程（如果正在运行）
    Push "$R0"
    nsExec::ExecToStack 'tasklist /FI "IMAGENAME eq ${APP_EXE}" /FO CSV /NH'
    Pop $R0
    ${If} $R0 == 0
        Pop $R0
        StrCmpS $R0 "" noKill
            DetailPrint "检测到 ${APP_EXE} 正在运行，尝试关闭…"
            nsExec::ExecToStack 'taskkill /F /IM ${APP_EXE} /T'
            Pop $R0
            Sleep 800
    noKill:
    ${EndIf}
    Pop "$R0"

    ; 检测 WebView2 Runtime（Evergreen Bootstrapper）：缺失则弹一次提示
    ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
    ${If} $0 == ""
        MessageBox MB_ICONINFORMATION|MB_OK "WebView2 Runtime not detected. Please install Evergreen Bootstrapper from:$\r$\nhttps://developer.microsoft.com/microsoft-edge/webview2/" /SD IDOK
        ExecShell "open" "https://developer.microsoft.com/microsoft-edge/webview2/"
    ${EndIf}
FunctionEnd

; ================================================================
;  安装段 Section
; ================================================================
Section "主程序 (必选)" SecMain
    SectionIn RO
    SetOutPath "$INSTDIR"

    ; 1. 拷贝 .NET 4.8 安装包（给用户手动安装留底，同时安装流程里也会用到文件释放）
    SetOutPath "$INSTDIR\prerequisites"
    File ".\prerequisites\ndp48-x86-x64-allos-enu.exe"
    SetOutPath "$INSTDIR"

    ; 2. 递归拷贝 build/bin 下所有文件（exe、迁移标记、dll 等）
    SetOverwrite try
    File /r /x "*.pdb" /x "${UNINSTALL_EXE}" ".\build\bin\*.*"

    ; 3. 递归拷贝 dependencies/clients 下所有协议客户端（FreeRDP / Horizon / SPICE / VNC）
    ;    程序运行时在 exe 同级目录的 dependencies\clients\ 下查找各协议客户端
    ;    排除 __extract_tmp（临时解压目录）和 freerdp_msvc（空目录）
    SetOutPath "$INSTDIR\dependencies\clients"
    File /r /x "__extract_tmp" /x "freerdp_msvc" /x "VMware-Horizon-Client-*.exe" /x "virt-viewer-x64-*.msi" /x "tigervnc64-*.exe" /x "*.log" ".\dependencies\clients\*.*"
    SetOutPath "$INSTDIR"

    ; 4. .NET Framework 4.8 自动检测 + 静默安装
    Call CheckAndInstallDotNet48

    ; 5. 确保 PowerShell 在系统 PATH 里（解决 "powershell.exe not found in %PATH%" 错误）
    Call EnsurePowerShellInPath

    ; 6. 写注册表：安装路径 + 卸载信息（控制面板可见）
    ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
    IntFmt $0 "0x%08X" $0
    WriteRegStr   HKLM "${UNINSTALL_KEY}" "DisplayName"     "${APP_NAME}"
    WriteRegStr   HKLM "${UNINSTALL_KEY}" "DisplayVersion"  "${APP_VERSION}"
    WriteRegStr   HKLM "${UNINSTALL_KEY}" "Publisher"       "${APP_PUBLISHER}"
    WriteRegStr   HKLM "${UNINSTALL_KEY}" "URLInfoAbout"    "${APP_URL}"
    WriteRegStr   HKLM "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"
    WriteRegStr   HKLM "${UNINSTALL_KEY}" "UninstallString" '"$INSTDIR\${UNINSTALL_EXE}"'
    WriteRegStr   HKLM "${UNINSTALL_KEY}" "QuietUninstallString" '"$INSTDIR\${UNINSTALL_EXE}" /S'
    WriteRegDWORD HKLM "${UNINSTALL_KEY}" "NoModify"        1
    WriteRegDWORD HKLM "${UNINSTALL_KEY}" "NoRepair"        1
    WriteRegDWORD HKLM "${UNINSTALL_KEY}" "EstimatedSize"   $0

    ; 3. 生成卸载程序
    WriteUninstaller "$INSTDIR\${UNINSTALL_EXE}"

    ; 4. 开始菜单快捷方式
    CreateDirectory "$SMPROGRAMS\${APP_NAME}"
    CreateShortCut   "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk"    "$INSTDIR\${APP_EXE}" "" "$INSTDIR\${APP_EXE}" 0
    CreateShortCut   "$SMPROGRAMS\${APP_NAME}\卸载 ${APP_NAME}.lnk" "$INSTDIR\${UNINSTALL_EXE}" "" "$INSTDIR\${UNINSTALL_EXE}" 0

    ; 5. 桌面快捷方式（所有用户）
    CreateShortCut   "$DESKTOP\${APP_NAME}.lnk"   "$INSTDIR\${APP_EXE}" "" "$INSTDIR\${APP_EXE}" 0

    ; 6. 刷新注册表（立即显示到控制面板）
    SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=3000

    SetDetailsPrint none
    DetailPrint "============================================================"
    DetailPrint "  Installation completed."
    DetailPrint "  Program     : $INSTDIR"
    DetailPrint "  Start Menu  : $SMPROGRAMS\${APP_NAME}"
    DetailPrint "  Desktop Lnk : $DESKTOP\${APP_NAME}.lnk"
    DetailPrint "============================================================"
    SetDetailsPrint both
SectionEnd

; ================================================================
;  卸载段
; ================================================================
Section "un.Uninstall"

    ; 1. 关闭正在运行的程序
    Push "$R0"
    nsExec::ExecToStack 'tasklist /FI "IMAGENAME eq ${APP_EXE}" /FO CSV /NH'
    Pop $R0
    ${If} $R0 == 0
        Pop $R0
        StrCmpS $R0 "" noKillU
            DetailPrint "检测到 ${APP_EXE} 正在运行，尝试关闭…"
            nsExec::ExecToStack 'taskkill /F /IM ${APP_EXE} /T'
            Sleep 800
    noKillU:
    ${EndIf}
    Pop "$R0"

    ; 2. 删除快捷方式
    Delete "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk"
    Delete "$SMPROGRAMS\${APP_NAME}\卸载 ${APP_NAME}.lnk"
    RMDir  "$SMPROGRAMS\${APP_NAME}"
    Delete "$DESKTOP\${APP_NAME}.lnk"

    ; 3. 删除安装目录（含 dependencies/clients 协议客户端）
    RMDir /r "$INSTDIR"

    ; 4. 删除卸载注册表
    DeleteRegKey HKLM "${UNINSTALL_KEY}"

    SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=3000

    SetDetailsPrint none
    DetailPrint "============================================================"
    DetailPrint "  Uninstall completed."
    DetailPrint "============================================================"
    SetDetailsPrint both
SectionEnd

; ================================================================
;  un.onInit
; ================================================================
Function un.onInit
    MessageBox MB_ICONQUESTION|MB_YESNO|MB_DEFBUTTON2 "确实要完全删除 ${APP_NAME} 及其所有组件？" IDYES +2
        Abort
FunctionEnd

; ------ 段描述（可选） ------
!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
    !insertmacro MUI_DESCRIPTION_TEXT ${SecMain} "安装 ${APP_NAME} 主程序、依赖文件、桌面与开始菜单快捷方式，并写入控制面板卸载信息。"
!insertmacro MUI_FUNCTION_DESCRIPTION_END
