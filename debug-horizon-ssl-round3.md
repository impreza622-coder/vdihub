# 调试会话：horizon-ssl-round3
**状态：[OPEN]**
**会话 ID：** `horizon-ssl-round3`
**创建时间：** 2026-08-15 04:30

## 症状描述

在测试机（其他机器）上：
1. 安装 `installer.exe`（2026/8/15 04:12 编译）
2. 直接双击运行 `dependencies\clients\horizon\horizon-client.exe` 
3. 立即弹出 `VERIFY bora\lib\ssl\ssl.c:2384` 错误框
4. 日志路径：`C:\Users\Administrator\AppData\Local\Temp\omnissa-Administrator\horizon-crtbora-XXXX.log`

在开发本机上：
- 直接双击同一文件运行 → **完全正常，能看到"添加服务器"界面**
- VC++ Runtime: v14.50.35719.00 Installed=1 ✅
- FIPS: 0 ✅

## 根因分析

直接双击也报错 → **Go 代码完全不参与** → **纯测试机环境问题**

## 可证伪假设

| ID | 假设 | 验证方式 | 状态 |
|----|------|----------|------|
| H11 | **测试机上 crtbora.dll / libssl-3-x64.dll 被系统文件劫持** — Windows DLL 搜索顺序：应用程序目录 → System32 → PATH。如果 System32 有同名被污染/旧版本文件，会被加载 | 对比两机 crtbora.dll SHA-256；检查测试机 System32 下是否有同名 DLL | **优先验证** |
| H12 | **测试机 Windows 版本过旧** — Horizon 8.x 需要 Windows 10 1809+，Win 7/旧版 Win10 缺少必要 API | 测试机运行 `winver` 或 `Get-ComputerInfo` | **关键** |
| H13 | **测试机上 VC++ Runtime 注册表仍缺失** — 静默安装被杀毒软件拦截，或安装包执行返回 0 但实际失败 | `Get-ItemProperty "HKLM:\SOFTWARE\Microsoft\VisualStudio\14.0\VC\Runtimes\x64"` | 需用户配合 |
| H14 | **NSIS 打包的文件损坏/被截断** — 大文件压缩时 LZMA 出错 | 对比本机/测试机 `horizon-client.exe` 的 SHA-256 | **可自动检查** |
| H15 | **测试机上系统 FIPS 策略开启**（与 H5 类似） | `Get-ItemProperty "HKLM:\SYSTEM\CurrentControlSet\Control\Lsa\FipsAlgorithmPolicy"` | 需用户配合 |

## 证据收集计划

### 自动收集（本机）
- [ ] 本机 horizon-client.exe SHA-256
- [ ] 本机 crtbora.dll SHA-256
- [ ] 本机 Horizon 目录完整文件清单
- [ ] 本机 Horizon 日志（如有）

### 需用户配合收集（测试机）
- [ ] 测试机 `horizon-client.exe` SHA-256（对比 H14）
- [ ] 测试机 Windows 版本 `winver`（H12）
- [ ] 测试机 VC++ Runtime 注册表（H13）
- [ ] 测试机 Horizon 日志 `horizon-crtbora-*.log`（关键！）

## 当前进度

- [x] 确认症状：直接双击也报错（A1）→ 纯环境问题
- [ ] 收集测试机日志和系统信息
- [ ] 对比文件哈希
- [ ] 定位根因
