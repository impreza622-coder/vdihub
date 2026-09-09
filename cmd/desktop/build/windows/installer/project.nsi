Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them with the values from ProjectInfo.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
##
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the ProjectInfo file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "MyProject" # Default "{{.Name}}"
## !define INFO_COMPANYNAME    "MyCompany" # Default "{{.Info.CompanyName}}"
## !define INFO_PRODUCTNAME    "MyProduct" # Default "{{.Info.ProductName}}"
## !define INFO_PRODUCTVERSION "1.0.0"     # Default "{{.Info.ProductVersion}}"
## !define INFO_COPYRIGHT      "Copyright" # Default "{{.Info.Copyright}}"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
####
## Include the wails tools
####
!include "wails_tools.nsh"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} 安装程序"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

# 中文界面文本
!define MUI_WELCOMEPAGE_TITLE "欢迎使用 ${INFO_PRODUCTNAME} 安装向导"
!define MUI_WELCOMEPAGE_TEXT "此向导将引导您完成 ${INFO_PRODUCTNAME} 的安装过程。\r\n\r\n建议在继续之前关闭所有其他应用程序。\r\n\r\n点击「下一步」继续，或点击「取消」退出安装。"
!define MUI_DIRECTORYPAGE_TEXT_TOP "请选择 ${INFO_PRODUCTNAME} 的安装目录，然后点击「安装」继续。"
!define MUI_DIRECTORYPAGE_TEXT_DESTINATION "安装目录："
!define MUI_INSTFILESPAGE_FINISHHEADER_TEXT "安装完成"
!define MUI_INSTFILESPAGE_FINISHHEADER_SUBTEXT "${INFO_PRODUCTNAME} 已成功安装到您的计算机。"
!define MUI_INSTFILESPAGE_ABORTHEADER_TEXT "安装中断"
!define MUI_INSTFILESPAGE_ABORTHEADER_SUBTEXT "${INFO_PRODUCTNAME} 安装未完成，请稍后重试。"
!define MUI_FINISHPAGE_TEXT "${INFO_PRODUCTNAME} 已成功安装到您的计算机。\r\n\r\n点击「完成」退出安装向导。"
!define MUI_ABORTWARNING_TEXT "您确定要退出 ${INFO_PRODUCTNAME} 安装吗？"

!insertmacro MUI_PAGE_WELCOME # 欢迎页面
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # 安装目录选择页面
!insertmacro MUI_PAGE_INSTFILES # 安装进度页面
!insertmacro MUI_PAGE_FINISH # 安装完成页面

!insertmacro MUI_UNPAGE_INSTFILES # 卸载页面

!insertmacro MUI_LANGUAGE "SimpChinese" # 设置安装程序语言为简体中文

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.
!ifdef WAILS_INSTALL_SCOPE
  !if "${WAILS_INSTALL_SCOPE}" == "user"
    InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
  !else
    InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
  !endif
!else
  InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!endif # Default installing folder ($PROGRAMFILES is Program Files folder).
ShowInstDetails show # This will always show the installation details.

Function .onInit
   !insertmacro wails.checkArchitecture
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

    ; ============================================================
    ;  Portable protocol clients (FreeRDP / Horizon / TigerVNC / virt-viewer)
    ;  Runtime code resolves <exeDir>\dependencies\clients\...
    ;  so we MUST install this directory alongside the EXE with the
    ;  exact same relative layout.
    ;  Source = cmd\desktop\dependencies (copied alongside exe in dev)
    ; ============================================================
    !define CLIENTS_SRC "..\..\..\dependencies"
    !if /FileExists "${CLIENTS_SRC}\clients\freerdp\mingw64\bin\sdl-freerdp.exe"
        SetOutPath "$INSTDIR\dependencies"
        File /r /x "*.log" /x "__extract_tmp" "${CLIENTS_SRC}\*.*"
    !else
        ; fallback: try absolute from project root when cwd is repo root
        SetOutPath "$INSTDIR\dependencies"
        File /r /x "*.log" /x "__extract_tmp" "..\..\..\..\..\cmd\desktop\dependencies\*.*"
    !endif

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd