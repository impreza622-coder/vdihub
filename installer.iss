; 瘦客户端完整安装程序脚本
; 使用 Inno Setup 6 编译

[Setup]
AppName=瘦客户端
AppVersion=1.0.0
AppPublisher=瘦客户端
AppPublisherURL=https://localhost
DefaultDirName={pf}\瘦客户端
DefaultGroupName=瘦客户端
UninstallDisplayIcon={app}\desktop.exe
UninstallDisplayName=瘦客户端
Compression=lzma2/ultra64
SolidCompression=yes
LZMAUseSeparateProcess=yes
ArchitecturesAllowed=x64
ArchitecturesInstallIn64BitMode=x64
OutputDir=dist
OutputBaseFilename=瘦客户端安装程序_v1.0.0
SetupIconFile=cmd\desktop\frontend\src\assets\logo.png
DisableProgramGroupPage=yes
DisableDirPage=no
WizardStyle=modern
ShowLanguageDialog=yes
LanguageDetectionMethod=none

[Languages]
Name: "chinesesimp"; MessagesFile: "compiler:Languages\ChineseSimplified.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "附加任务:"; Flags: checkedonce

[Files]
; 主程序
Source: "cmd\desktop\desktop.exe"; DestDir: "{app}"; Flags: ignoreversion

; FreeRDP 客户端（RDP 协议）
Source: "dependencies\clients\freerdp\*"; DestDir: "{app}\dependencies\clients\freerdp"; Flags: ignoreversion recursesubdirs createallsubdirs; Check: DirExists(ExpandConstant('{src}\dependencies\clients\freerdp'))

; Horizon 客户端（Horizon 协议）
Source: "dependencies\clients\horizon\*"; DestDir: "{app}\dependencies\clients\horizon"; Flags: ignoreversion recursesubdirs createallsubdirs

; TigerVNC 客户端（VNC 协议）
Source: "dependencies\clients\tigervnc\*"; DestDir: "{app}\dependencies\clients\tigervnc"; Flags: ignoreversion recursesubdirs createallsubdirs; Check: DirExists(ExpandConstant('{src}\dependencies\clients\tigervnc'))

; virt-viewer 客户端（SPICE 协议）
Source: "dependencies\clients\virt-viewer\*"; DestDir: "{app}\dependencies\clients\virt-viewer"; Flags: ignoreversion recursesubdirs createallsubdirs; Check: DirExists(ExpandConstant('{src}\dependencies\clients\virt-viewer'))

[Icons]
Name: "{group}\瘦客户端"; Filename: "{app}\desktop.exe"; WorkingDir: "{app}"; IconFilename: "{app}\desktop.exe"
Name: "{group}\卸载瘦客户端"; Filename: "{uninstallexe}"
Name: "{commondesktop}\瘦客户端"; Filename: "{app}\desktop.exe"; WorkingDir: "{app}"; IconFilename: "{app}\desktop.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\desktop.exe"; Description: "立即启动瘦客户端"; Flags: nowait postinstall skipifsilent; WorkingDir: "{app}"

[UninstallRun]
; 卸载时清理注册表（还原 Horizon Shade 设置）
Filename: "reg.exe"; Parameters: "DELETE ""HKCU\Software\VMware, Inc.\VMware VDM\Client"" /f"; Flags: runhidden; RunOnceId: "CleanHorizonReg1"
Filename: "reg.exe"; Parameters: "DELETE ""HKLM\Software\VMware, Inc.\VMware VDM\Client"" /f"; Flags: runhidden; RunOnceId: "CleanHorizonReg2"

[UninstallDelete]
Type: filesandordirs; Name: "{app}\dependencies"
Type: filesandordirs; Name: "{app}"
Type: filesandordirs; Name: "{group}"

[Code]
function InitializeSetup(): Boolean;
begin
  Result := True;
end;
