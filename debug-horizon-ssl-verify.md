# Debug Session: horizon-ssl-verify
Status: [OPEN]
Session ID: horizon-ssl-verify
Created: 2026-08-15
Bug: Horizon 客户端启动时立即报错 "VERIFY bora\lib\ssl\ssl.c:2384" (host-2480/3428 等)

## 现象
- Horizon UI 能正常弹出（WPF 加载成功）
- 立即弹出 Omnissa Horizon Error 对话框
- 错误信息：`VERIFY bora\lib\ssl\ssl.c:2384`
- 退出码：exit status 0xffffffff / 0xe0434352
- 日志目录：`%TEMP%\omnissa-Administrator\horizon-crtbora-*.log`

## 可证伪假设

| ID | 假设 | 验证方式 | 状态 |
|----|------|----------|------|
| H1 | VC++ 2015-2022 x64 运行时未真正安装，导致 libssl-3-x64.dll / libcrypto-3-x64.dll / fips.dll 加载失败 | 检查注册表 `HKLM\SOFTWARE\Microsoft\VisualStudio\14.0\VC\Runtimes\x64\Version`；手动运行 vc_redist.x64.exe | **强相关 / 主要修复方向** |
| H2 | 直接双击 horizon-client.exe 也报同样错误 → 环境问题（非 Go 启动代码问题） | 在本机直接双击运行 horizon-client.exe | **已否决**：本机直接启动成功（5秒存活） |
| H3 | OpenSSL 3.x 的 providers 目录缺失（缺少 legacy.dll / default.dll），导致 FIPS 初始化失败 | 检查 Horizon 目录下是否有 ossl-modules/ 或 providers/ 目录 | **已否决**：缺失但本机仍能启动 |
| H4 | `cmd.Env` 被显式赋值后导致子进程丢失关键系统环境变量 | 移除 cmd.Env 赋值，让 exec.Command 完整继承 | **已修复**：不再设置任何 cmd.Env |
| H5 | 系统级 FIPS 策略组策略开启，与 Horizon 自带的 fips.dll 产生冲突 | 检查注册表 `HKLM\SYSTEM\CurrentControlSet\Control\Lsa\FipsAlgorithmPolicy\Enabled` | **已否决**：本机关闭(0) |
| **H6** | **设置 OPENSSL_CONF 干扰了 Horizon 的自定义 FIPS OpenSSL 初始化**（libssl 3.0.16 vs fips 3.0.9 版本不一致） | 移除 OPENSSL_CONF 环境变量 | **已修复**：删除了 OPENSSL_CONF 设置 |

## 证据记录

### 本机环境（启动成功机器）
- VC++ Runtime: v14.50.35719.00 Installed=1 ✅
- FIPS: 0 ✅
- .NET 8 Desktop: 8.0.10 ✅
- Horizon 双击直接运行: 成功（5秒进程不退出）✅

### 测试机（启动失败机器）
- 用户反馈：在"其他机器"安装新包后运行立即报 VERIFY 错误
- 修复假设：
  1. NSIS 静默安装 VC++ 被杀毒软件拦截 → 增加注册表二次校验 + 交互式降级
  2. 程序注入 OPENSSL_CONF → 已移除
  3. cmd.Env 截断系统环境 → 已移除，exec.Command 默认完整继承

### 代码修复
1. manager.go L384-L387：移除 OPENSSL_CONF 设置
2. installer.nsi CheckAndInstallVCRedist：增加静默安装后注册表再校验 → 失败降级交互式安装 → 再次失败强弹框手动安装

