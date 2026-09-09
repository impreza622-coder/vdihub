﻿; ============================================================
;   ������ͻ��� (Thin Client Desktop)   NSIS ��װ�ű�
; ============================================================

; ------ Ӧ����Ϣ ------
!define APP_NAME        "������ͻ���"
!define APP_EXE         "thin-client-desktop.exe"
!define APP_PUBLISHER   "ɽ��������������޹�˾"
!define APP_VERSION     "1.0.5"
!define APP_URL         "https://www.feilida.com"
!define UNINSTALL_EXE   "uninst.exe"

; ��װ / ж�� ע����
!define UNINSTALL_KEY   "Software\Microsoft\Windows\CurrentVersion\Uninstall\FeiLiDaThinClient"

; ------ NSIS ���� ------
Unicode true
RequestExecutionLevel admin     ; д HKLM / Program Files ��Ҫ����ԱȨ��
InstallDir "$PROGRAMFILES64\FeiLiDa\ThinClientDesktop"
InstallDirRegKey HKLM "${UNINSTALL_KEY}" "InstallLocation"
ShowInstDetails show
ShowUnInstDetails show
SetCompressor /SOLID lzma
CRCCheck on

; ------ ���� Modern UI 2 ------
!include "MUI2.nsh"
!include "x64.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"
!include "WinMessages.nsh"

; ���� & Ʒ��
Name "${APP_NAME}"
BrandingText "${APP_PUBLISHER}  ${APP_VERSION}"
Caption "${APP_NAME} ��װ��"

; ------ ����ͼ�긴�� appicon.png ���ɵ� build/windows/icon.ico ------
!define APP_ICON_PATH       ".\build\windows\icon.ico"

; ------ ���� ------
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
!define MUI_FINISHPAGE_RUN_TEXT     "��� ${APP_NAME}"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_WELCOME
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH

; ------ �������� ------
!insertmacro MUI_LANGUAGE "SimpChinese"

; ============================================================
;  EnsurePowerShellInPath ���� ȷ��ϵͳ PATH �а��� PowerShell
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
;  .onInit ���� ������� + �ɰ����� + WebView2
; ============================================================
Function .onInit
    ; ��֧�� 64 λ Windows
    ${IfNot} ${RunningX64}
        MessageBox MB_ICONSTOP "�������֧�� 64 λ Windows 7 / 10 / 11 / Server 2016+��$\r$\n��ǰϵͳΪ 32 λ���޷���װ��" /SD IDOK
        Abort
    ${EndIf}

    ; �ر��������еľɰ汾����
    Push $R0
    nsExec::ExecToStack 'tasklist /FI "IMAGENAME eq ${APP_EXE}" /FO CSV /NH'
    Pop $R0
    ${If} $R0 == 0
        Pop $R0
        StrCmpS $R0 "" noKill
            DetailPrint "��⵽ ${APP_EXE} �������У�ǿ�ƹر� ..."
            nsExec::ExecToStack 'taskkill /F /IM ${APP_EXE} /T'
            Pop $R0
            Sleep 800
    noKill:
    ${EndIf}
    Pop $R0

    ; ����ɰ�����·��Ŀ¼����ֹ Horizon SSL DSO ������
    StrCpy $R8 "$PROGRAMFILES64\FeiLiDa\������ͻ���"
    ${If} ${FileExists} "$R8\${APP_EXE}"
        DetailPrint "��⵽�ɰ�����·��������ж�� ..."
        ${If} ${FileExists} "$R8\${UNINSTALL_EXE}"
            ExecWait '"$R8\${UNINSTALL_EXE}" /S _?=$R8' $R9
        ${EndIf}
        RMDir /r "$R8"
        DetailPrint "�ɰ�Ŀ¼�������"
    ${EndIf}

    ; ǿ��Ӣ�İ�װ·��
    StrCpy $INSTDIR "$PROGRAMFILES64\FeiLiDa\ThinClientDesktop"

    ; ��� WebView2 Runtime��Wails ���������LTSC Ĭ�ϲ��Դ���
    ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
    ${If} $0 == ""
        SetDetailsPrint textonly
        DetailPrint "��⵽δ��װ WebView2 Runtime�����ڰ�װ ..."
        SetDetailsPrint none
        ExecWait '"$INSTDIR\prerequisites\MicrosoftEdgeWebview2Setup.exe" /silent /install' $R8
        ${If} $R8 == 0
        ${OrIf} $R8 == 1638
        ${OrIf} $R8 == 1641
            DetailPrint "WebView2 Runtime ��װ�ɹ�"
        ${Else}
            MessageBox MB_ICONEXCLAMATION "WebView2 Runtime ��װʧ�ܣ�������=$R8����$\r$\n���ֶ���װ��$\r$\n$INSTDIR\prerequisites\MicrosoftEdgeWebview2Setup.exe" /SD IDOK
        ${EndIf}
    ${Else}
        DetailPrint "WebView2 Runtime (v$0) �Ѱ�װ��"
    ${EndIf}
FunctionEnd

; ============================================================
;  ��װ�� Section
; ============================================================
Section "������ (����)" SecMain
    SectionIn RO
    SetOutPath "$INSTDIR"

    ; 1. ��� prerequisites��WebView2 Bootstrapper + Omnissa Horizon 8.15 �ٷ���װ�� + Horizon ж�ؽű�
    SetOutPath "$INSTDIR\prerequisites"
    File ".\prerequisites\MicrosoftEdgeWebview2Setup.exe"
    File ".\prerequisites\Omnissa-Horizon8-Client.exe"
    File ".\prerequisites\uninstall-horizon.ps1"
    SetOutPath "$INSTDIR"

    ; 2. ���� build/bin �������ļ���exe��ͼ�ꡢdll �ȣ�
    SetOverwrite try
    File /r /x "*.pdb" /x "${UNINSTALL_EXE}" ".\build\bin\*.*"

    ; 3. ���������ͻ��ˣ�FreeRDP / SPICE / VNC
    ;    �ų���ɫ�� horizon Ŀ¼���ɰ�װ������ʱĿ¼
    SetOutPath "$INSTDIR\dependencies\clients"
    File /r /x "__extract_tmp" /x "freerdp_msvc" /x "horizon" /x "VMware-Horizon-Client-*.exe" /x "Omnissa-Horizon*.exe" /x "virt-viewer-x64-*.msi" /x "tigervnc64-*.exe" /x "*.log" ".\dependencies\clients\*.*"
    SetOutPath "$INSTDIR"

    ; 4. ��Ĭ��װ Omnissa Horizon Client 8.15 �ٷ��浽��ĿĿ¼
    ;    �ٷ���װ�����д����.NET 8 / VC++ / ֤������ / ע���
    ;    /norestart����ֹ Horizon ��װ������ϵͳ������ʾ
    ;    ��װ��ɺ�������� PendingFileRenameOperations����ֹ Windows ����"��������"��ʾ
    DetailPrint "���ھ�Ĭ��װ Omnissa Horizon Client 8.15 ..."
    ${IfNot} ${FileExists} "$INSTDIR\dependencies\clients\horizon\horizon-client.exe"
        ExecWait '"$INSTDIR\prerequisites\Omnissa-Horizon8-Client.exe" /S /norestart /D=$INSTDIR\dependencies\clients\horizon' $R8
        ${If} $R8 == 0
            DetailPrint "Omnissa Horizon Client ��װ�ɹ�"
        ${Else}
            DetailPrint "Omnissa Horizon Client ��װ������: $R8 (������Ҫ����)"
        ${EndIf}
        ; ��� Horizon ��װ��д��� pending reboot ��־����ֹ������װ����û��ɾ͵���������ʾ
        DeleteRegValue HKLM "SYSTEM\CurrentControlSet\Control\Session Manager" "PendingFileRenameOperations"
        DeleteRegValue HKLM "SYSTEM\CurrentControlSet\Control\Session Manager" "PendingFileRenameOperations2"
        DeleteRegKey /ifempty HKLM "SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired"
        DeleteRegKey /ifempty HKLM "SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending"
        DeleteRegKey /ifempty HKLM "SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootInProgress"
        DetailPrint "����� pending reboot ��־"
    ${Else}
        DetailPrint "Omnissa Horizon Client �Ѵ��ڣ�����"
    ${EndIf}

    ; 5. ȷ�� PowerShell ��ϵͳ PATH
    Call EnsurePowerShellInPath

    ; 6. д Horizon ֤������ע��������˽�в�����ǩ��֤�鲻�����
    ;    ע�⣺��д EnableShade���ü�������������ʱ�����û������������˵������ض�̬����
    DetailPrint "����д�� Horizon ֤������ע��� ..."
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
    DetailPrint "Horizon ֤������ע���д�����"

    ; 7. дж����Ϣ
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

    ; 8. ����ж�س���
    WriteUninstaller "$INSTDIR\${UNINSTALL_EXE}"

    ; 9. ��ʼ�˵���ݷ�ʽ
    CreateDirectory "$SMPROGRAMS\${APP_NAME}"
    CreateShortcut  "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk"     "$INSTDIR\${APP_EXE}" "" "$INSTDIR\${APP_EXE}" 0
    CreateShortcut  "$SMPROGRAMS\${APP_NAME}\ж�� ${APP_NAME}.lnk" "$INSTDIR\${UNINSTALL_EXE}" "" "$INSTDIR\${UNINSTALL_EXE}" 0

    ; 10. �����ݷ�ʽ
    CreateShortcut  "$DESKTOP\${APP_NAME}.lnk"  "$INSTDIR\${APP_EXE}" "" "$INSTDIR\${APP_EXE}" 0

    ; 11. �㲥ע�����
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
;  ж����
; ============================================================
Section "un.Uninstall"
    Push $R0
    nsExec::ExecToStack 'tasklist /FI "IMAGENAME eq ${APP_EXE}" /FO CSV /NH'
    Pop $R0
    ${If} $R0 == 0
        Pop $R0
        StrCmpS $R0 "" noKillU
            DetailPrint "��⵽ ${APP_EXE} �������У�ǿ�ƹر� ..."
            nsExec::ExecToStack 'taskkill /F /IM ${APP_EXE} /T'
            Pop $R0
            Sleep 800
    noKillU:
    ${EndIf}
    Pop $R0

    Delete "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk"
    Delete "$SMPROGRAMS\${APP_NAME}\ж�� ${APP_NAME}.lnk"
    RMDir  "$SMPROGRAMS\${APP_NAME}"
    Delete "$DESKTOP\${APP_NAME}.lnk"

    ; --- ж�� Omnissa / VMware Horizon Client���ؼ���ͨ��ע����̬���� MSI ж����ڣ� ---
    ;     Omnissa Horizon 8.15 �� MSI ����/D= ����������Ч��ʵ�ʻ�װ�� C:\Program Files\Omnissa\...
    ;     ���߼�д���� $INSTDIR\dependencies\clients\horizon\uninstall.exe �����Ҳ���������ж�ز��ˡ�
    ;     �·��������� PowerShell �ű����� 64/32 λע��� Uninstall ���ƥ�� Horizon/Omnissa ��Ʒ��
    ;             ��ȡ MSI GUID ���� msiexec /X{GUID} /qn ��Ĭж�أ��ٰ�д����ע��������Ŀ¼ȫ��ɾ���
    DetailPrint "����ж�� Omnissa Horizon Client ..."
    ${If} ${FileExists} "$INSTDIR\prerequisites\uninstall-horizon.ps1"
        nsExec::ExecToLog '"powershell.exe" -NoProfile -ExecutionPolicy Bypass -File "$INSTDIR\prerequisites\uninstall-horizon.ps1"'
        Pop $R0
        DetailPrint "Horizon uninstall script exit: $R0"
    ${Else}
        DetailPrint "Horizon uninstall script not found (skip)"
    ${EndIf}

    ; --- ���ף��������Ŀ dependencies\clients\horizon Ŀ¼������ Horizon װ����ϵͳĿ¼������Ĳ���ҲҪ�壩 ---
    RMDir /r "$INSTDIR\dependencies\clients\horizon"

    ; --- ���� Horizon ע����������գ��ű�����һ�ݣ�������ɨһ���ֹ©��֮�㣩 ---
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
    MessageBox MB_ICONQUESTION|MB_YESNO|MB_DEFBUTTON2 "ȷ��Ҫж�� ${APP_NAME} �������������" IDYES +2
        Abort
FunctionEnd

; ------ ���� ------
!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
    !insertmacro MUI_DESCRIPTION_TEXT ${SecMain} "��װ ${APP_NAME} ������Omnissa Horizon Client 8.15��WebView2 Runtime�������뿪ʼ�˵���ݷ�ʽ����ע��������ж����Ϣ��"
!insertmacro MUI_FUNCTION_DESCRIPTION_END
