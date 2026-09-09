package protocol

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Optional global config reference for client path overrides
// Set this from main after loading application config
var globalClientPaths *struct {
	RDPWindowsPath     string
	SPICEWindowsPath   string
	VNCWindowsPath     string
	HorizonWindowsPath string
}

// SetClientPaths allows setting custom client executable paths from config
func SetClientPaths(rdpWin, spiceWin, vncWin, horizonWin string) {
	globalClientPaths = &struct {
		RDPWindowsPath     string
		SPICEWindowsPath   string
		VNCWindowsPath     string
		HorizonWindowsPath string
	}{
		RDPWindowsPath:     rdpWin,
		SPICEWindowsPath:   spiceWin,
		VNCWindowsPath:     vncWin,
		HorizonWindowsPath: horizonWin,
	}
}

// getAppDir returns the base directory of the running application
// Defaults to the executable directory with fallback to working directory
func getAppDir() string {
	exe, err := os.Executable()
	if err == nil {
		return filepath.Dir(exe)
	}
	wd, err := os.Getwd()
	if err == nil {
		return wd
	}
	return "."
}

type ProtocolType string

const (
	ProtocolRDP     ProtocolType = "rdp"
	ProtocolSPICE   ProtocolType = "spice"
	ProtocolVNC     ProtocolType = "vnc"
	ProtocolSSH     ProtocolType = "ssh"
	ProtocolHorizon ProtocolType = "horizon"
)

type ConnectionConfig struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Password   string `json:"password"`
	TLSPort    int    `json:"tls_port,omitempty"`
	Username   string `json:"username,omitempty"`
	VMPassword string `json:"vm_password,omitempty"`
	BrokerURL  string `json:"broker_url,omitempty"`
	Width      int    `json:"width,omitempty"`       // 桌面显示宽度（0 = 用客户端默认）
	Height     int    `json:"height,omitempty"`      // 桌面显示高度（0 = 用客户端默认）
	Fullscreen bool   `json:"fullscreen,omitempty"`  // 是否全屏启动
	HideClient bool   `json:"hide_client,omitempty"` // Horizon: 启动远程会话后隐藏客户端悬浮菜单（-hideClientAfterLaunchSession）
	// 下面这些主要给 RDP 使用，每个协议按需读取：
	ColorDepth   int    `json:"color_depth,omitempty"`   // 色深 0/16/24/32（0 = 服务器默认）
	Audio        bool   `json:"audio,omitempty"`         // 声音重定向
	Clipboard    bool   `json:"clipboard,omitempty"`     // 剪贴板重定向
	Compression  bool   `json:"compression,omitempty"`   // RDP 位图压缩（默认开）
	DriveMapping bool   `json:"drive_mapping,omitempty"` // 驱动器重定向（把本机所有盘重定向到远程）
	Printer      bool   `json:"printer,omitempty"`       // 打印机重定向
	Smartcard    bool   `json:"smartcard,omitempty"`     // 智能卡重定向
	USBRedirect  bool   `json:"usb_redirect,omitempty"`  // USB 设备重定向
	USBMode      string `json:"usb_mode,omitempty"`      // 重定向模式: "auto" (全部) | "manual" (选择设备)
	USBDevices   string `json:"usb_devices,omitempty"`   // 手动模式：选中的 USB 设备（逗号分隔的 vid:pid，如 "046D:C534,046D:C52B"）
	HideToolbar  bool   `json:"hide_toolbar,omitempty"`  // 隐藏悬浮菜单/工具栏（FreeRDP: 不传 /floatbar；SPICE: --kiosk；Windows RDP: 借用承载多显示器开关）
	MultiMon     bool   `json:"multi_mon,omitempty"`     // 多显示器（FreeRDP: /multimon，每块屏独立显示通道；服务端需专业版/企业版/服务器版，家庭版不支持）
	BlockAltF4   bool   `json:"block_alt_f4,omitempty"`  // RDP/所有远程窗口：屏蔽 ALT+F4 关闭（移除 WS_SYSMENU + 键盘钩子参数）
	// SPICE 代理模式（PVE /spiceproxy 返回，经 PVE 节点 3128 代理转发，无需 VM 直暴露 SPICE 端口）：
	Proxy       string `json:"proxy,omitempty"`        // SPICE HTTP 代理地址（如 http://192.168.1.10:3128）
	CA          string `json:"ca,omitempty"`            // SPICE TLS CA 证书（PEM，.vv 中多行写入）
	HostSubject string `json:"host_subject,omitempty"`  // TLS 证书主机主题（CN，用于证书校验匹配）
	Title       string `json:"title,omitempty"`         // remote-viewer 窗口标题
}

// buildUSBArg 根据 config 构建 FreeRDP /usb 参数
//   - 自动模式 (USBMode="auto") → /usb:auto（重定向所有 USB 设备）
//   - 手动模式 (USBMode="manual") → /usb:id:VID:PID#VID:PID#...（只重定向选中的）
//   - 默认无选中设备时回退到 /usb:auto
func buildUSBArg(config *ConnectionConfig) string {
	if !config.USBRedirect {
		return ""
	}
	// 手动模式：解析选中的设备列表
	if config.USBMode == "manual" {
		pairs := []string{}
		for _, p := range strings.Split(config.USBDevices, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			parts := strings.Split(p, ":")
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				pairs = append(pairs, parts[0]+":"+parts[1])
			}
		}
		if len(pairs) > 0 {
			return "/usb:id:" + strings.Join(pairs, "#")
		}
		// 手动模式但没选设备 → 回退到 auto
		return "/usb:auto"
	}
	// 自动模式（默认）
	return "/usb:auto"
}

type ProtocolManager struct {
	tempDir string
}

func NewProtocolManager() *ProtocolManager {
	tempDir := filepath.Join(os.TempDir(), "thin-client-desktop")
	os.MkdirAll(tempDir, 0755)
	return &ProtocolManager{tempDir: tempDir}
}

func (m *ProtocolManager) LaunchProtocol(protocol ProtocolType, config *ConnectionConfig) error {
	switch protocol {
	case ProtocolRDP:
		return m.LaunchRDP(config)
	case ProtocolSPICE:
		return m.LaunchSPICE(config)
	case ProtocolVNC:
		return m.LaunchVNC(config)
	case ProtocolSSH:
		return m.LaunchSSH(config)
	case ProtocolHorizon:
		return m.LaunchHorizon(config)
	default:
		return fmt.Errorf("不支持的协议类型: %s", protocol)
	}
}

func (m *ProtocolManager) LaunchHorizon(config *ConnectionConfig) error {
	switch runtime.GOOS {
	case "windows":
		return m.launchWindowsHorizon(config)
	case "darwin":
		return m.launchMacHorizon(config)
	case "linux":
		return m.launchLinuxHorizon(config)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

func (m *ProtocolManager) launchWindowsHorizon(config *ConnectionConfig) error {
	// .NET 8 检查仅在绿色版（bundled）时需要
	// 官方安装包自带所有依赖

	horizonClient, err := m.findWindowsHorizonClient()
	if err != nil {
		return err
	}

	// 写入日志文件用于调试
	logFile := filepath.Join(os.TempDir(), "horizon_launch.log")
	logFh, _ := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	writeLog := func(format string, args ...interface{}) {
		msg := fmt.Sprintf(format, args...)
		fmt.Print(msg)
		if logFh != nil {
			_, _ = fmt.Fprintf(logFh, "%s\n", msg)
		}
	}
	writeLog("=== Horizon Launch at %s ===", time.Now().Format(time.RFC3339))
	writeLog("horizonClient=%s", horizonClient)
	writeLog("config: host=%s port=%d username=%s brokerURL=%s fullscreen=%v hideClient=%v",
		config.Host, config.Port, config.Username, config.BrokerURL, config.Fullscreen, config.HideClient)

	// ============ Horizon 无人值守完整流程 ============
	// 阶段 A: 解析连接目标（优先 BrokerURL，否则回退 Host；占位符回退）
	serverURL := config.BrokerURL
	if serverURL == "" || strings.Contains(serverURL, "example.com") {
		serverURL = config.Host
	}
	// 拆出 host 和 scheme
	scheme := "https://"
	effectiveHost := serverURL
	hasExplicitPort := false
	explicitPortInURL := 0
	if strings.HasPrefix(serverURL, "http://") {
		scheme = "http://"
		effectiveHost = strings.TrimPrefix(serverURL, "http://")
	} else if strings.HasPrefix(serverURL, "https://") {
		scheme = "https://"
		effectiveHost = strings.TrimPrefix(serverURL, "https://")
	}
	// 去掉路径，但保留端口（之前版本把端口和路径一起裁掉，导致 Horizon 连错端口！）
	if slash := strings.Index(effectiveHost, "/"); slash >= 0 {
		effectiveHost = effectiveHost[:slash]
	}
	if colon := strings.Index(effectiveHost, ":"); colon >= 0 {
		// IPv6 形如 [2001::1]:443 的情况，先找最后一个冒号后面的端口
		if strings.HasPrefix(effectiveHost, "[") {
			if cb := strings.LastIndex(effectiveHost, "]"); cb > 0 && cb < len(effectiveHost)-1 && effectiveHost[cb+1] == ':' {
				if p, err := strconv.Atoi(effectiveHost[cb+2:]); err == nil && p > 0 {
					explicitPortInURL = p
					hasExplicitPort = true
					effectiveHost = effectiveHost[:cb+1]
				}
			}
		} else {
			if p, err := strconv.Atoi(effectiveHost[colon+1:]); err == nil && p > 0 {
				explicitPortInURL = p
				hasExplicitPort = true
				effectiveHost = effectiveHost[:colon]
			}
		}
	}
	effectivePort := config.Port
	if hasExplicitPort {
		effectivePort = explicitPortInURL
	} else if effectivePort <= 0 {
		// Horizon Broker 默认 443（之前写死 443 但没拼到 URL 里，导致缺端口时连不上）
		effectivePort = 443
	}

	// 阶段 B: 证书处理 - 多层保障确保任何机器都能连接
	useHost := effectiveHost
	if runtime.GOOS == "windows" && effectiveHost != "" {
		writeLog("=== Horizon certificate handling start ===")
		writeLog("Target server: %s:%d", effectiveHost, effectivePort)

		// ============ 第0层：立即写入 HKCU 注册表（不需要管理员权限，立即生效） ============
		writeLog("Applying HKCU registry certificate bypass (no admin required)...")
		applyHorizonIgnoreCert()

		// ============ 第1层：尝试获取服务器证书并导入信任存储 ============
		// ★ 快速通道：首次完整准备（取证书→导入→hosts→prefs）后写入缓存；
		//   证书有效期内（留 72h 余量）再次启动直接跳过网络取证书/certutil 导入/flushdns，
		//   启动前置耗时从 ~1-2s（最差 20s+）降到 ~10ms。注册表绕过每次仍重写（原生 API，微秒级）。
		type horizonPrepCache struct {
			Thumbprint string    `json:"thumbprint"`
			CN         string    `json:"cn"`
			UseHost    string    `json:"use_host"`
			NotAfter   time.Time `json:"not_after"`
		}
		prepFile := filepath.Join(os.TempDir(),
			fmt.Sprintf("shoukehuduan_horizon_prep_%s_%d.json", sanitizeHost(effectiveHost), effectivePort))
		prepOK := false
		certImported := false
		if data, errR := os.ReadFile(prepFile); errR == nil && len(data) > 0 {
			var pc horizonPrepCache
			if errJ := json.Unmarshal(data, &pc); errJ == nil && pc.UseHost != "" &&
				time.Now().Add(72*time.Hour).Before(pc.NotAfter) {
				useHost = pc.UseHost
				prepOK = true
				certImported = true
				writeLog("Fast path: cached cert prep valid (CN=%s NotAfter=%s), skip fetch/import", pc.CN, pc.NotAfter)
				// hosts 条目是本地文件操作（毫秒级）且必须始终保证正确（可能被外部改动），快速路径下也校验一次
				if pc.CN != "" && pc.CN != effectiveHost {
					if _, errH := ensureHostsEntry(effectiveHost, pc.CN); errH != nil {
						writeLog("Fast path hosts re-ensure failed (needs admin): %v", errH)
					}
				}
			}
		}
		if !prepOK {
			if certInfo, err := fetchHorizonBrokerCert(effectiveHost, effectivePort); err == nil {
				if ci, ok := certInfo.(*horizonCertInfo); ok && ci.CN != "" {
					writeLog("Cert CN: %s", ci.CN)
					writeLog("Cert Subject: %s", ci.Subject)
					writeLog("Cert Thumbprint: %s", ci.Thumbprint)
					writeLog("Cert NotBefore: %s", ci.NotBefore)
					writeLog("Cert NotAfter: %s", ci.NotAfter)

					// 1a. 导入证书到 Windows 受信任根存储（需要管理员权限，失败也不影响后续）
					if ci.CertDER != nil {
						if errImport := installCertToRoot(ci.CertDER, ci.CN); errImport == nil {
							writeLog("Cert imported to Windows trusted root store OK")
							certImported = true
						} else {
							writeLog("Cert import failed (needs admin): %v (continuing with other methods)", errImport)
						}
					}

					// 1b. 如果 CN 与 IP 不同，写入 hosts 文件 + 用 CN 连接
					if ci.CN != effectiveHost {
						writeLog("CN differs from IP, writing hosts entry...")
						writeLog("Mapping: %s -> %s", effectiveHost, ci.CN)
						if changed, err := ensureHostsEntry(effectiveHost, ci.CN); err != nil {
							writeLog("Hosts write FAILED: %v (will try IP directly with registry bypass)", err)
							// hosts 写入失败时，回退到 IP 直接连接
							// 依赖 HKCU 注册表和 prefs.txt 来绕过证书验证
						} else {
							useHost = ci.CN
							// 仅在条目真正变化时才 flushdns（~0.5s），未变化跳过
							if changed {
								writeLog("Hosts entry changed, flushing DNS...")
								flushCmd := exec.Command("ipconfig", "/flushdns")
								flushCmd.SysProcAttr = getSysProcAttr()
								flushCmd.Run()
								time.Sleep(200 * time.Millisecond)
							} else {
								writeLog("Hosts entry already correct, skip flushdns")
							}
						}
					} else {
						writeLog("CN matches IP, no hosts update needed")
					}

					// 1c. 将服务器添加到 prefs.txt 信任列表
					writeLog("Adding server to prefs.txt trusted list...")
					ensureHorizonServerTrusted(useHost)

					// 1d. 写入快速通道缓存（证书有效期内下次启动跳过整个网络/导入流程）
					if notAfter, errP := time.Parse("2006-01-02 15:04:05", ci.NotAfter); errP == nil {
						if data, errJ := json.Marshal(horizonPrepCache{
							Thumbprint: ci.Thumbprint,
							CN:         ci.CN,
							UseHost:    useHost,
							NotAfter:   notAfter,
						}); errJ == nil {
							_ = os.WriteFile(prepFile, data, 0644)
						}
					}
				} else {
					writeLog("Invalid cert info, will try registry fallback only")
				}
			} else {
				writeLog("Could not fetch certificate: %v", err)
				writeLog("Will rely on registry-based certificate bypass only")
			}
		} // end if !prepOK

		writeLog("Cert imported to root: %v, HKCU registry bypass: applied, Hosts mapping: %s", certImported, useHost)
		writeLog("=== Horizon certificate handling done ===")
	}

	// 用证书 CN（或 IP）作为 serverURL
	serverURL = scheme + useHost
	if effectivePort != 443 {
		serverURL = fmt.Sprintf("%s:%d", serverURL, effectivePort)
	}
	writeLog("Final serverURL: %s", serverURL)
	// 阶段 C: 构建 Horizon Client 参数
	// 注意: -ignore-cert-errors 不是有效参数，会导致 exit 9
	// 证书信任已通过: 1) 注册表 CertCheckMode=0  2) 导入到 Windows 受信任存储  来保证
	args := []string{
		"-serverURL=" + serverURL,
	}
	if config.Username != "" {
		args = append(args, "-userName="+config.Username)
	}
	if config.VMPassword != "" {
		args = append(args, "-password="+config.VMPassword)
	}
	if config.Fullscreen {
		args = append(args, "-desktopLayout=fullscreen")
	}
	if config.HideClient {
		args = append(args, "-hideClientAfterLaunchSession=true")
		// 注意：-nomenubar 不是 Horizon 客户端有效参数（exit code 9），
		// 悬浮窗隐藏完全通过注册表 EnableShade=False + noMenuBar=TRUE + autoHideToolbar=TRUE 实现
	}

	cmd := exec.Command(horizonClient, args...)
	cmd.Dir = filepath.Dir(horizonClient)
	cmd.SysProcAttr = getSysProcAttr()

	// 写入证书绕过注册表键（允许自签名证书，对私有部署常见）
	writeHorizonCertRegistry()

	// ★ Shade（悬浮窗）隐藏/显示配置必须在最后写入：
	//   防止前面的 writeHorizonCertRegistry() 等步骤意外覆盖 EnableShade 等键值。
	//   顺序错误会直接导致「隐藏悬浮菜单」开关失效、ShadePosition 不生效（卡左边缘）
	if config.HideClient {
		applyHorizonShadeHide()
	} else {
		applyHorizonShadeShow()
	}
	writeLog("Shade registry applied: hideClient=%v", config.HideClient)

	// 把 Horizon 自身目录放 PATH 最前，确保它能优先找到同目录下的 DLL
	horizonClientDir := filepath.Dir(horizonClient)
	cleanEnv := make([]string, 0, len(os.Environ()))
	hasPath := false
	newPath := horizonClientDir + ";" + os.Getenv("PATH")
	for _, kv := range os.Environ() {
		if strings.HasPrefix(strings.ToUpper(kv), "PATH=") {
			kv = "PATH=" + newPath
			hasPath = true
		}
		cleanEnv = append(cleanEnv, kv)
	}
	if !hasPath {
		cleanEnv = append(cleanEnv, "PATH="+newPath)
	}
	cmd.Env = cleanEnv

	// 重定向 Horizon 的输出到日志
	horizonLogFile := filepath.Join(os.TempDir(), "horizon_client_output.log")
	if hf, err := os.OpenFile(horizonLogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644); err == nil {
		cmd.Stdout = hf
		cmd.Stderr = hf
	}

	writeLog("Launching client=%s serverURL=%s", horizonClient, serverURL)
	writeLog("args: %v", args)
	writeLog("client stdout/stderr -> %s", horizonLogFile)
	if err := cmd.Start(); err != nil {
		writeLog("Failed to start: %v", err)
		if logFh != nil {
			_ = logFh.Close()
		}
		return err
	}
	writeLog("Started PID=%d", cmd.Process.Pid)

	// 读取 horizon 输出日志辅助诊断
	readHorizonOutputLog := func() string {
		data, e := os.ReadFile(horizonLogFile)
		if e != nil || len(data) == 0 {
			return ""
		}
		s := string(data)
		if len(s) > 1500 {
			s = s[len(s)-1500:]
		}
		return strings.TrimSpace(s)
	}

	// 同步等待 5 秒看进程是否立即退出（捕获错误码并返回给前端）
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()
	select {
	case err := <-waitCh:
		output := readHorizonOutputLog()
		if err != nil {
			writeLog("Horizon exited immediately with error: %v", err)
			if logFh != nil {
				_ = logFh.Close()
			}
			hint := ""
			if output != "" {
				hint = fmt.Sprintf("\r\nHorizon 输出:\r\n%s", output)
			}
			return fmt.Errorf("Horizon 启动失败：进程立即退出(错误=%v)。请尝试以管理员身份运行本软件，或查看日志 %s。%s", err, logFile, hint)
		}
		writeLog("Horizon exited normally (exit code=0)")
		if logFh != nil {
			_ = logFh.Close()
		}
		hint := ""
		if output != "" {
			hint = fmt.Sprintf("\r\nHorizon 输出:\r\n%s", output)
		}
		return fmt.Errorf("Horizon 启动失败：进程立即正常退出(exit=0)，可能 .NET 环境/参数有误。%s", hint)
	case <-time.After(5 * time.Second):
		writeLog("Horizon running after 5s (still alive, PID=%d)", cmd.Process.Pid)
		if logFh != nil {
			_ = logFh.Close()
		}
	}

	// 异步监控后续退出
	go func() {
		select {
		case err := <-waitCh:
			if err != nil {
				writeLog("Horizon exited later with error: %v", err)
			} else {
				writeLog("Horizon exited normally later")
			}
		case <-time.After(24 * time.Hour):
			writeLog("Horizon monitor timeout (24h)")
		}
	}()

	// 阶段 D: 自动 ENTER（登录按钮 + 桌面池选择）—— 不再固定 3 秒死等，改为窗口就绪即触发
	if runtime.GOOS == "windows" {
		runHorizonSendKeys(cmd.Process.Pid, 0)
	}
	return nil
}

func (m *ProtocolManager) launchMacHorizon(config *ConnectionConfig) error {
	horizonClient := "/Applications/VMware Horizon Client.app/Contents/MacOS/vmware-view"
	if _, err := os.Stat(horizonClient); err != nil {
		altClient := "/Applications/VMware Horizon Client.app/Contents/MacOS/VMware Horizon Client"
		if _, err := os.Stat(altClient); err != nil {
			return fmt.Errorf("未找到 VMware Horizon Client，请先安装")
		}
		horizonClient = altClient
	}

	args := []string{
		fmt.Sprintf("--server=%s", config.BrokerURL),
		fmt.Sprintf("--user=%s", config.Username),
		fmt.Sprintf("--password=%s", config.VMPassword),
		"--fullscreen",
	}

	cmd := exec.Command(horizonClient, args...)
	return cmd.Start()
}

func (m *ProtocolManager) launchLinuxHorizon(config *ConnectionConfig) error {
	horizonClient, err := m.findLinuxHorizonClient()
	if err != nil {
		return err
	}

	args := []string{
		fmt.Sprintf("--server=%s", config.BrokerURL),
		fmt.Sprintf("--user=%s", config.Username),
		fmt.Sprintf("--password=%s", config.VMPassword),
		"--fullscreen",
	}

	cmd := exec.Command(horizonClient, args...)
	cmd.SysProcAttr = getSysProcAttr()
	return cmd.Start()
}

func (m *ProtocolManager) findWindowsHorizonClient() (string, error) {
	// 1. 优先使用项目目录下的官方安装（由 NSIS 静默安装到 dependencies/clients/horizon）
	if bundled := findBundledHorizonClient(); bundled != "" {
		fmt.Printf("[Horizon] Found client (official install): %s\n", bundled)
		return bundled, nil
	}

	// 2. 系统默认安装路径（如果用户单独安装了官方版）
	officialPaths := []string{
		"C:\\Program Files\\Omnissa\\Omnissa Horizon Client\\horizon-client.exe",
		"C:\\Program Files (x86)\\Omnissa\\Omnissa Horizon Client\\horizon-client.exe",
		"C:\\Program Files\\VMware\\VMware Horizon Client\\vmware-view.exe",
		"C:\\Program Files (x86)\\VMware\\VMware Horizon Client\\vmware-view.exe",
	}
	for _, path := range officialPaths {
		if _, err := os.Stat(path); err == nil {
			fmt.Printf("[Horizon] Found system install: %s\n", path)
			return path, nil
		}
	}

	// 3. 配置文件指定路径
	if globalClientPaths != nil {
		cfgPath := globalClientPaths.HorizonWindowsPath
		if cfgPath != "" {
			if _, err := os.Stat(cfgPath); err == nil {
				return cfgPath, nil
			}
		}
	}

	// 4. PATH 查找
	for _, name := range []string{"horizon-client.exe", "vmware-view.exe"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("未找到 Horizon Client。请确认项目目录下 dependencies/clients/horizon 完整（含 horizon-client.exe），或安装 Omnissa Horizon Client")
}

func (m *ProtocolManager) findLinuxHorizonClient() (string, error) {
	clients := []string{
		"vmware-view",
		"horizon-client",
		"vmware-horizon-view",
	}

	for _, client := range clients {
		if path, err := exec.LookPath(client); err == nil {
			return path, nil
		}
	}

	paths := []string{
		"/usr/bin/vmware-view",
		"/usr/local/bin/vmware-view",
		"/opt/vmware/horizon/client/vmware-view",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("未找到 VMware Horizon Client，请先安装")
}

func (m *ProtocolManager) checkHorizonDependency() (bool, string, error) {
	switch runtime.GOOS {
	case "windows":
		if path, err := m.findWindowsHorizonClient(); err == nil {
			// 官方安装包自行管理所有依赖（.NET 8 / VC++），无需额外检查
			return true, fmt.Sprintf("已找到 Horizon Client: %s", path), nil
		} else {
			bundled := findBundledHorizonClient()
			return false, fmt.Sprintf("未找到 Horizon Client。diagnose: %s | bundled=%s | err=%v", diagnoseBundledDir(), bundled, err), nil
		}
	case "darwin":
		if _, err := os.Stat("/Applications/VMware Horizon Client.app"); err == nil {
			return true, "已找到 VMware Horizon Client for macOS", nil
		}
		return false, "请安装 VMware Horizon Client for macOS", nil
	case "linux":
		if _, err := m.findLinuxHorizonClient(); err == nil {
			return true, "已找到 VMware Horizon Client for Linux", nil
		}
		return false, "请安装 VMware Horizon Client for Linux", nil
	default:
		return false, "不支持的操作系统", nil
	}
}

func (m *ProtocolManager) LaunchRDP(config *ConnectionConfig) error {
	// ★ 默认全屏：如果用户没有明确设置窗口大小且没有关闭全屏，强制全屏
	//   前端 protocolConfig 可能异步加载未完成导致 fullscreen=false，这里兜底
	if !config.Fullscreen && config.Width == 0 && config.Height == 0 {
		config.Fullscreen = true
	}
	switch runtime.GOOS {
	case "windows":
		return m.launchWindowsRDP(config)
	case "darwin":
		return m.launchMacRDP(config)
	case "linux":
		return m.launchLinuxRDP(config)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

func (m *ProtocolManager) LaunchSPICE(config *ConnectionConfig) error {
	switch runtime.GOOS {
	case "windows":
		return m.launchWindowsSPICE(config)
	case "darwin":
		return m.launchMacSPICE(config)
	case "linux":
		return m.launchLinuxSPICE(config)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

func (m *ProtocolManager) LaunchVNC(config *ConnectionConfig) error {
	switch runtime.GOOS {
	case "windows":
		return m.launchWindowsVNC(config)
	case "darwin":
		return m.launchMacVNC(config)
	case "linux":
		return m.launchLinuxVNC(config)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

func (m *ProtocolManager) LaunchSSH(config *ConnectionConfig) error {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("SSH 客户端未安装: %w", err)
	}

	args := []string{
		fmt.Sprintf("%s@%s", "root", config.Host),
		"-p", fmt.Sprintf("%d", config.Port),
	}

	if config.Password != "" {
		args = append(args, "-o", "StrictHostKeyChecking=no")
	}

	cmd := exec.Command(sshPath, args...)
	return cmd.Start()
}

func (m *ProtocolManager) CheckProtocolDependency(protocol ProtocolType) (bool, string, error) {
	switch protocol {
	case ProtocolRDP:
		return m.checkRDPDependency()
	case ProtocolSPICE:
		return m.checkSPICEDependency()
	case ProtocolVNC:
		return m.checkVNCDependency()
	case ProtocolHorizon:
		return m.checkHorizonDependency()
	default:
		return false, "", fmt.Errorf("不支持的协议类型: %s", protocol)
	}
}

func (m *ProtocolManager) GetProtocolConfigTemplate(protocol ProtocolType, config *ConnectionConfig) (string, error) {
	switch protocol {
	case ProtocolRDP:
		return m.generateRDPFile(config)
	case ProtocolSPICE:
		return m.generateSPICEFile(config)
	case ProtocolHorizon:
		return m.generateHorizonConfigFile(config)
	default:
		return "", fmt.Errorf("不支持的协议类型: %s", protocol)
	}
}

func (m *ProtocolManager) generateHorizonConfigFile(config *ConnectionConfig) (string, error) {
	content := fmt.Sprintf(`[Horizon]
BrokerServer=%s
UserName=%s
Password=%s
Domain=%s
Fullscreen=true
Protocol=BLAST
`, config.BrokerURL, config.Username, config.VMPassword, config.Host)

	filePath := filepath.Join(m.tempDir, fmt.Sprintf("horizon_%s.ini", sanitizeHost(config.Host)))
	err := os.WriteFile(filePath, []byte(content), 0600)
	if err != nil {
		return "", fmt.Errorf("创建 Horizon 配置文件失败: %w", err)
	}

	return filePath, nil
}

func (m *ProtocolManager) launchWindowsRDP(config *ConnectionConfig) error {
	rdpClient, err := m.findWindowsRDPClient()
	if err != nil {
		return err
	}

	// ===== FreeRDP 参数 =====
	//
	// ====== 声音传参关键修复（为什么 Hyper-V 下 mstsc 有但 FreeRDP 没有？）
	// 本 mingw FreeRDP 3.x 使用命令名 /sound（输出）和 /microphone（输入），而非 /audio-output /audio-input
	// 后者在当前版本报「Unexpected keyword」→ 直接 exit=4294966295 全失败
	// Hyper-V 增强会话模式下的 Windows 10 TermService，/sound:sys:winmm + /audio-mode:1(redirect) 能正确协商出
	//    「Microsoft 远程桌面音频」设备；如果只写 /sound 不带 :sys:winmm，客户端自选后端可能落到 wasapi 被 Hyper-V 拒绝
	// soundVariant 顺序：带麦克风 winmm → 带麦克风 wasapi → 仅输出 winmm → 仅输出 wasapi → 最简 /sound
	//   0 = "/sound:sys:winmm /microphone:sys:winmm /audio-mode:1"
	//   1 = "/sound:sys:wasapi /microphone:sys:wasapi /audio-mode:1"
	//   2 = "/sound:sys:winmm /audio-mode:1"
	//   3 = "/sound:sys:wasapi /audio-mode:1"
	//   4-7 = 各种兼容兜底的 /sound 写法
	// forSDL: 目标客户端是 sdl-freerdp.exe 时为 true。
	//   实测 sdl-freerdp 3.30.0 不认识 -dvc / /kbd:localalt / /kbd:alt（直接报
	//   Invalid sigil / parsing failed 立即退出）→ 这些参数只能传给 wfreerdp。
	buildBase := func(level int, soundVariant int, dynRes, advancedToggles bool, forSDL bool) []string {
		a := []string{fmt.Sprintf("/v:%s:%d", config.Host, config.Port)}
		if config.Username != "" {
			a = append(a, fmt.Sprintf("/u:%s", config.Username))
		}
		if config.VMPassword != "" {
			a = append(a, fmt.Sprintf("/p:%s", config.VMPassword))
		}

		if level <= 2 {
			w, h := config.Width, config.Height
			if w <= 0 || h <= 0 {
				w, h = 1920, 1080
			}
			a = append(a, fmt.Sprintf("/w:%d", w), fmt.Sprintf("/h:%d", h))
			if config.ColorDepth > 0 {
				a = append(a, fmt.Sprintf("/bpp:%d", config.ColorDepth))
			} else {
				a = append(a, "/bpp:32")
			}
			// 注意：+fonts 参数在当前 mingw FreeRDP 3.x 不存在（帮助里只有 -fonts 关闭写法），不要硬加
			a = append(a, "/cert:ignore")
			if config.Fullscreen {
				a = append(a, "/f")
			}
			// 悬浮菜单（顶部 floatbar）：默认显示（用户可用它退出全屏/断开连接）。
			// 内置 wfreerdp 3.30.0 已修复 floatbar（旧版 sdl-freerdp 的 bug 不影响 wfreerdp；
			// 若兜底到 sdl-freerdp 可能不显示，但不影响连接）。仅在用户开启「隐藏悬浮菜单」时不传。
			if !config.HideToolbar {
				a = append(a, "/floatbar")
			}
			// 多显示器：每块屏独立显示通道（服务端需专业版/企业版/服务器版 Windows，家庭版不支持）
			if config.MultiMon {
				a = append(a, "/multimon")
			}
			// === Kiosk 级键盘配置 ===
			// 无论 BlockAltF4 是否开启，都强制启用键盘转发模式
			// 这样用户按 Ctrl+Alt+Del/Alt+F4 等快捷键时，
			// FreeRDP 会把它们转发到远程桌面，而不是在本地触发
			// -dvc: 禁止动态虚拟通道（DVC），防止本地拦截 Alt+Tab/Alt+F4
			// /kbd:localalt=off: 本地 Alt 不处理，直接转发到远程
			// /kbd:alt:off: 同含义（某些 FreeRDP 版本用这个命名）
			// ★ 这 3 个参数 sdl-freerdp 3.30.0 无法解析（Invalid sigil / parsing failed 直接退出），
			//   只能传给 wfreerdp；-window-drag 两者都支持。sdl 全屏时 SDL 自己独占键盘，
			//   配合本软件的 WH_KEYBOARD_LL 钩子（app_windows_extra.go）同样屏蔽 Alt+F4。
			if !forSDL {
				a = append(a,
					"-dvc",
					"/kbd:localalt=off",
					"/kbd:alt:off",
				)
			}
			a = append(a, "-window-drag")
			if config.BlockAltF4 {
				// 窗口层兜底已在 app_windows_extra.go 的 WH_KEYBOARD_LL 钩子中实现
				// 拦截 ALT+F4、Win 键、Alt+Tab、Win+D、Win+L、PrintScreen 等
			}
			// 兼容性参数（所有级别都加，/? 确认存在）：
			//   /timeout:10000    = 连接超时 10s（防止高延迟卡住）
			//   /network:auto     = 让客户端自动探测带宽（影响 RDP 压缩/RFX 决策）
			//   +auto-reconnect   = 断线自动重连（帮助里确认 +toggle 写法）
			//   /gdi:sw           = 软件 GDI 渲染（mingw 版硬件加速可能白屏/花屏，sw 兼容性最好）
			a = append(a, "/timeout:10000", "/network:auto", "+auto-reconnect", "/gdi:sw")
			if config.Audio {
				// 真实 /? 确认的参数名：/sound[:sys:<sys>]  和  /microphone[:sys:<sys>]
				// /audio-output 和 /audio-input 命名在本版本不存在 → Unexpected keyword
				//
				// ⚠️ Hyper-V Win10 TermService「Microsoft 远程桌面音频」注册关键注意点：
				// 1) mstsc 默认"不带麦克风"+"纯音频输出 winmm"，所以 Hyper-V 下优先试不带麦克风的组合
				// 2) TRACE 日志显示 AUDIO_PLAYBACK_DVC 出现 close 循环，说明动态 DVC 模式下
				//    TermService 对 rdpsnd/audin 同时协商会异常。尝试「禁用 DVC(-dvc) 强制静态 VC」
				//    能让 FreeRDP 在 Client Info PDU 里宣告 INFO_AUDIO(0x2000) flag ——
				//    这是老版 TermService 决定是否在虚拟机里挂"远程音频"设备的判据！
				// 3) AUDIO_PLAYBACK_LOSSY_DVC "no listener" 是 FreeRDP 正常行为（暂不支持有损），
				//    但只要无损通道协商成功就能出声，不必纠结。
				//
				// 优先级顺序（越靠前越先尝试，Hyper-V 越稳）：
				//   v8 = 禁 DVC 强制静态 VC + 纯输出 winmm + audio-mode:redirect（首选！对齐 mstsc）
				//   v9 = 禁 DVC + 纯输出 winmm + audio-mode:1
				//   v10= 禁 DVC + 带麦克风 winmm + audio-mode:redirect
				//   v2 = DVC 模式 + 纯输出 winmm + audio-mode:1（旧默认，保留兼容）
				//   v11= DVC 模式 + 纯输出 winmm + audio-mode:redirect + 格式协商显式声明
				//   v12= DVC 模式 + 纯输出 wasapi + audio-mode:redirect
				//   v0 = DVC + 带麦克风 winmm（老写法，最后兜底）
				//   ...其他旧变体继续保留
				switch soundVariant {
				case 8:
					// ★★★ Hyper-V 首选组合：强制静态 VC（触发 INFO_AUDIO flag）+ mstsc 同款纯输出 winmm ★★★
					if !forSDL {
						a = append(a, "-dvc")
					}
					a = append(a, "/sound:sys:winmm", "/audio-mode:redirect")
				case 9:
					if !forSDL {
						a = append(a, "-dvc")
					}
					a = append(a, "/sound:sys:winmm", "/audio-mode:1")
				case 10:
					if !forSDL {
						a = append(a, "-dvc")
					}
					a = append(a, "/sound:sys:winmm", "/microphone:sys:winmm", "/audio-mode:redirect")
				case 11:
					a = append(a, "/sound:sys:winmm,format:1", "/audio-mode:redirect")
				case 12:
					a = append(a, "/sound:sys:wasapi", "/audio-mode:redirect")
				case 0:
					a = append(a, "/sound:sys:winmm", "/microphone:sys:winmm", "/audio-mode:1")
				case 1:
					a = append(a, "/sound:sys:wasapi", "/microphone:sys:wasapi", "/audio-mode:1")
				case 2:
					a = append(a, "/sound:sys:winmm", "/audio-mode:1")
				case 3:
					a = append(a, "/sound:sys:wasapi", "/audio-mode:1")
				case 4:
					a = append(a, "/sound", "/audio-mode:1")
				case 5:
					a = append(a, "/sound", "/audio-mode:1")
				case 6:
					a = append(a, "/sound", "/audio-mode:redirect")
				case 7:
					a = append(a, "/sound")
				}
			}
		}

		if level == 0 && advancedToggles {
			// 帮助确认支持的写法（来自 sdl-freerdp /?）：
			//   +dynamic-resolution  (toggle，+ 开)
			//   [-|/]clipboard       (/ = + = 开启，- = 关闭)
			//   +drives              (toggle)
			//   /printer             (option，不是 +printer)
			//   /smartcard           (option，不是 +smartcard)
			//   压缩：帮助里只有 -compression (关闭)，没有 +compression；压缩默认开启
			if dynRes {
				a = append(a, "+dynamic-resolution")
			}
			if config.Clipboard {
				a = append(a, "/clipboard")
			} else {
				a = append(a, "-clipboard")
			}
			if config.DriveMapping {
				a = append(a, "+drives")
			}
			if config.Printer {
				a = append(a, "/printer")
			}
			if config.Smartcard {
				a = append(a, "/smartcard")
			}
			if config.USBRedirect {
				a = append(a, buildUSBArg(config))
			}
		}
		return a
	}

	// mstsc 兜底（理论上不会走到，因为 findWindowsRDPClient 不返回 mstsc，但保留以防万一）
	if strings.Contains(strings.ToLower(rdpClient), "mstsc") {
		rdpFile, err := m.generateRDPFile(config)
		if err != nil {
			return err
		}
		cmd := exec.Command(rdpClient, rdpFile)
		cmd.SysProcAttr = getSysProcAttr()
		return cmd.Start()
	}

	// ===== 先试 wfreerdp.exe；如果它因为参数/版本兼容启动失败立即退出，就 fallback 到 sdl-freerdp.exe =====
	// wfreerdp.exe 已经被官方标记 deprecated，替换方案就是 SDL 版 sdl-freerdp.exe（同目录下存在）
	//
	// blockAltF4Pid: -1 = 需要屏蔽 ALT+F4（进程 PID 在 Start 后从 cmd.Process.Pid 拿到后再调 removeSysMenuForPid）
	runOnce := func(exePath string, a []string, blockAltF4Pid int) (int, string, string, error) {
		logDir := os.TempDir()
		ts := time.Now().Format("20060102_150405")
		logFile := filepath.Join(logDir, fmt.Sprintf("shoukehuduan_freerdp_%s_%s.log", sanitizeHost(config.Host), ts))
		cmdFile := filepath.Join(logDir, fmt.Sprintf("shoukehuduan_freerdp_last_cmd_%s.txt", sanitizeHost(config.Host)))

		cmd := exec.Command(exePath, a...)
		exeDir := filepath.Dir(exePath)
		cmd.Dir = exeDir
		cmd.SysProcAttr = getSysProcAttr()

		// ============== OpenSSL 3.x Legacy Provider 三重注入（彻底消灭 legacy provider failed to load） ==============
		// 问题：mingw 版 FreeRDP 链接 OpenSSL 3.x，默认禁用 md4/rc4/des 等老算法
		// 后果：日志里出现 "OpenSSL LEGACY provider failed to load, no md4 support" → NTLM 认证能力降级
		//       → RDP 安全加密、rdpsnd DVC 通道协商能力缺失 → Hyper-V TermService 拒挂 Microsoft 远程桌面音频
		//
		// OpenSSL 3.x Legacy Provider 工作需要【同时】满足 3 个条件（缺一个就报 LEGACY provider failed）：
		//   条件 A) openssl.cnf 里声明了 [provider_sect] -> legacy = legacy_sect -> activate=1
		//   条件 B) 磁盘上存在物理文件 legacy.dll（OpenSSL 3.x 模块化加载的 provider 动态库）
		//   条件 C) OpenSSL 运行时能找到 legacy.dll → 通过 OPENSSL_MODULES 环境变量或默认模块路径
		//
		// ⚠️ 之前只满足了条件 A，完全缺条件 B+C，所以日志一直报 LEGACY provider failed！
		//
		// 注入方案：
		//   条件 A: ① 复制 openssl_legacy.cnf 到 exe 同级 openssl.cnf（OpenSSL 3 默认自动读）② OPENSSL_CONF 环境变量兜底
		//   条件 B: 查找并复制 legacy.dll 到 ossl-modules 目录（MSYS2 mingw64 OpenSSL 默认模块路径 = <mingw64>/lib/ossl-modules/legacy.dll）
		//   条件 C: ③ OPENSSL_MODULES 环境变量 = 绝对路径指向包含 legacy.dll 的目录
		//
		// 另外：/openssl-config:<cnf> 命令行参数在当前 mingw FreeRDP 3.x 版本【不存在】
		//      日志 ERROR 铁证：Failed at index 1 [/openssl-config:xxx] - Unexpected keyword → exit=4294966295
		//      所以【绝对】不能走命令行，只能用 cnf 文件+env 方式

		clientsRoot := resolveBundledClientsDir()
		envExtra := make([]string, 0, 4)
		opensslNote := ""

		// =============== 条件 A 处理：openssl.cnf（声明要加载 legacy provider）================
		opensslCnf := filepath.Join(exeDir, "openssl_legacy.cnf")
		if _, st := os.Stat(opensslCnf); st != nil {
			for _, v := range []string{
				filepath.Join("freerdp", "mingw64", "bin"),
				filepath.Join("freerdp", "vs-build", "bin"),
				filepath.Join("freerdp", "ucrt64", "bin"),
				filepath.Join("freerdp"),
			} {
				candidate := filepath.Join(clientsRoot, v, "openssl_legacy.cnf")
				if _, st2 := os.Stat(candidate); st2 == nil {
					opensslCnf = candidate
					break
				}
			}
		}
		if _, st := os.Stat(opensslCnf); st == nil {
			// --- 方式 A①: 复制到 exe 同级 openssl.cnf（OpenSSL 3 默认自动读取）
			defaultCnf := filepath.Join(exeDir, "openssl.cnf")
			if src, errR := os.ReadFile(opensslCnf); errR == nil {
				_ = os.WriteFile(defaultCnf, src, 0644)
			}
			// 再复制 3 个常见 fallback 位置（mingw OpenSSL 会递归向上找 openssl.cnf）
			for _, extra := range []string{
				filepath.Join(exeDir, "..", "ssl", "openssl.cnf"),
				filepath.Join(exeDir, "..", "..", "ssl", "openssl.cnf"),
				filepath.Join(exeDir, "..", "openssl.cnf"),
			} {
				dir := filepath.Dir(extra)
				_ = os.MkdirAll(dir, 0755)
				if src, errR := os.ReadFile(opensslCnf); errR == nil {
					_ = os.WriteFile(extra, src, 0644)
				}
			}
			// --- 方式 A②: OPENSSL_CONF 环境变量兜底
			envExtra = append(envExtra, "OPENSSL_CONF="+opensslCnf)
			opensslNote += "OPENSSL_CONF=" + opensslCnf + "\n"
		} else {
			opensslNote += "OPENSSL_CONF=(缺失！请检查 openssl_legacy.cnf 是否被删除)\n"
		}

		// =============== 条件 B + C 处理：legacy.dll（物理模块文件 + OPENSSL_MODULES 路径） ================
		// OpenSSL 3.x 模块默认搜索路径 = MINGW_PREFIX/lib/ossl-modules/ ，也就是 exeDir/../lib/ossl-modules/
		// 同时也支持 bin/ossl-modules/ 作为 fallback
		legacyDll := ""
		// 先在 exe 附近的标准位置找
		for _, p := range []string{
			filepath.Join(exeDir, "..", "lib", "ossl-modules", "legacy.dll"),
			filepath.Join(exeDir, "ossl-modules", "legacy.dll"),
		} {
			if _, st := os.Stat(p); st == nil {
				legacyDll = p
				break
			}
		}
		// 找不到就用 clientsRoot 回溯搜索（MSYS2 mingw64 的结构是 clientsRoot/freerdp/mingw64/lib/ossl-modules/legacy.dll）
		if legacyDll == "" {
			for _, variant := range []string{
				filepath.Join("freerdp", "mingw64", "lib", "ossl-modules"),
				filepath.Join("freerdp", "mingw64", "bin", "ossl-modules"),
				filepath.Join("freerdp", "ucrt64", "lib", "ossl-modules"),
				filepath.Join("freerdp", "vs-build", "lib", "ossl-modules"),
			} {
				candidate := filepath.Join(clientsRoot, variant, "legacy.dll")
				if _, st := os.Stat(candidate); st == nil {
					legacyDll = candidate
					break
				}
			}
		}
		// 再找不到就去系统 MSYS2 安装路径（开发机兜底）
		if legacyDll == "" {
			for _, sysP := range []string{
				`D:\kaifahuanjing\msys64\mingw64\lib\ossl-modules\legacy.dll`,
				`D:\kaifahuanjing\msys64\ucrt64\lib\ossl-modules\legacy.dll`,
				`C:\msys64\mingw64\lib\ossl-modules\legacy.dll`,
			} {
				if _, st := os.Stat(sysP); st == nil {
					legacyDll = sysP
					break
				}
			}
		}

		if legacyDll != "" {
			if data, errR := os.ReadFile(legacyDll); errR == nil {
				// 复制到 exe 相关的 2 个标准 ossl-modules 搜索路径（条件 B 部署）
				deployTargets := []string{
					filepath.Join(exeDir, "..", "lib", "ossl-modules", "legacy.dll"),
					filepath.Join(exeDir, "ossl-modules", "legacy.dll"),
				}
				modulesDir := filepath.Dir(deployTargets[0])
				for _, tgt := range deployTargets {
					if errM := os.MkdirAll(filepath.Dir(tgt), 0755); errM == nil {
						_ = os.WriteFile(tgt, data, 0644)
					}
				}
				// --- 条件 C: OPENSSL_MODULES 环境变量 = 包含 legacy.dll 的目录（绝对路径）
				absModules, _ := filepath.Abs(modulesDir)
				envExtra = append(envExtra,
					"OPENSSL_MODULES="+absModules,
					"OPENSSL_LEGACY=1",
				)
				opensslNote += "LEGACY_DLL_SRC=" + legacyDll + "\n"
				opensslNote += "OPENSSL_MODULES=" + absModules + "\n"
			} else {
				opensslNote += fmt.Sprintf("LEGACY_DLL=(读文件失败 %v)\n", errR)
			}
		} else {
			opensslNote += "LEGACY_DLL=(未找到！MSYS2 mingw64/lib/ossl-modules/legacy.dll 请复制到项目 dependencies/)\n"
		}

		// ============== 合并所有额外环境变量 ==============
		if len(envExtra) > 0 {
			cmd.Env = append(os.Environ(), envExtra...)
		}

		// ============== 构建诊断 cmd.txt 内容（必须放在所有参数最终敲定之后，保证真实） ==============
		fullCmd := exePath
		for _, arg := range a {
			if strings.HasPrefix(arg, "/p:") {
				fullCmd += fmt.Sprintf(" /p:***(len=%d)***", len(strings.TrimPrefix(arg, "/p:")))
			} else {
				fullCmd += " " + arg
			}
		}
		fullCmd += "\nWorkingDir: " + exeDir + "\nClient: " + filepath.Base(exePath) + "\n" + opensslNote
		// 把最终真实的 cmd.Args 也写进去（方便核对 exec.Command 是否被偷偷改过）
		fullCmd += "cmd.Args=" + strings.Join(cmd.Args, " ") + "\n"
		_ = os.WriteFile(cmdFile, []byte(fullCmd), 0644)

		logFh, errOpen := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if errOpen == nil {
			cmd.Stdout = logFh
			cmd.Stderr = logFh
		} else {
			logFh = nil
		}

		if err := cmd.Start(); err != nil {
			if logFh != nil {
				_ = logFh.Close()
			}
			return -1, cmdFile, logFile, err
		}

		// ★ 不再强制激活 FreeRDP 窗口到前台！
		//   之前用 forceForegroundWindow 干扰了 SDL 的 handleShow 流程，导致窗口创建失败
		//   SDL 全屏窗口会自己正确显示，不需要外部干扰

		// ============ BlockAltF4 钩子：启动后立即枚举 PID 对应 HWND → 移除 WS_SYSMENU（Alt+F4/X/菜单全失效） ============
		// ★ 原来只扫 5 次 x 500ms = 2.5s，FreeRDP 子进程/桌面窗口可能还没创建
		// ★ 现在：
		//   1) 总守护 900 次 × 1 秒 ≈ 15 分钟（覆盖断线重连、弹窗提示导致新窗口出现的整个阶段）
		//   2) 每轮 collectDescendantPIDs（收集所有后代进程）+ 根 PID，把所有可见顶层窗口的
		//      WS_SYSMENU / MINIMIZEBOX / MAXIMIZEBOX 全部去掉（FreeRDP 的 wfreerdp 会起 sdl-freerdp 子进程，真实桌面窗口在子进程里）
		if blockAltF4Pid < 0 {
			pid := cmd.Process.Pid
			go func() {
				defer func() { _ = recover() }()
				rootPID := uint32(pid)
				// 立即先做一次（根 PID 自己就有窗口的情况）
				_ = removeSysMenuForPid(rootPID)
				time.Sleep(1200 * time.Millisecond)
				// 15 分钟守护
				for round := 0; round < 900; round++ {
					allPIDs := collectDescendantPIDs(rootPID)
					allPIDs[rootPID] = true
					for p := range allPIDs {
						_ = removeSysMenuForPid(p)
					}
					time.Sleep(1 * time.Second)
				}
			}()
		}

		// ============== 判定启动成功的时间策略 ==============
		// 旧版 1.8s 对 Hyper-V 连接（协商慢）太激进：-dvc 静态 VC 需要 2~3.5s 才真正把桌面画出来
		// → 默认改成 4000ms；如果参数带 "-dvc"（静态 VC 强兼容），就算 3s 才启动也是对的
		// bootTimeout 语义（新版）：从启动起就每 250ms 探测窗口，一出现立即算成功；
		//   bootTimeout 只是「开始主动抢前台激活」的时间点，不再是固定等待时间
		//   （旧版傻等满 bootTimeout 才探测 → sdl 窗口 2~3s 就出来了却要等满 6s，多屏必现等待）。
		bootTimeout := 4000 * time.Millisecond
		// ★ sdl-freerdp 的桌面窗口要等整个协商完成后才创建（wfreerdp 连接后 ~1.3s 就弹窗，
		//   sdl 干净环境下也要 4~5s；若服务端还在回收上一个会话（被顶号/强杀残留），
		//   实测可拖到 10s+）。沿用 wfreerdp 的 4+3s 判定会把 sdl 误杀 → 矩阵全部失败后
		//   错误回落到 wfreerdp（多显示器拼接的元凶）。sdl 给更长的启动与窗口宽限期。
		isSDLClient := strings.Contains(strings.ToLower(filepath.Base(exePath)), "sdl")
		windowGrace := 3 * time.Second
		if isSDLClient {
			bootTimeout = 6000 * time.Millisecond
			windowGrace = 10 * time.Second
		}
		isStaticVC := false
		for _, arg := range a {
			if arg == "-dvc" {
				isStaticVC = true
				break
			}
		}
		_ = isStaticVC // 保留钩子，后续可继续加静态 VC 专属逻辑
		done := make(chan int, 1)
		go func() {
			_ = cmd.Wait()
			code := cmd.ProcessState.ExitCode()
			if logFh != nil {
				if f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644); err == nil {
					_, _ = fmt.Fprintf(f, "\n\n[瘦客户端 wrapper] pid=%d client=%s exit_code=%d OPENSSL_CONF_OK=%v static_vc=%v\n",
						cmd.Process.Pid, filepath.Base(exePath), code, opensslCnf != "", isStaticVC)
					_ = f.Close()
				}
				_ = logFh.Close()
			}
			done <- code
		}()

		// ★★★ 统一轮询（从进程启动起每 250ms 探测一次）：
		//   - 进程退出           → 立即返回真实退出码（矩阵快速 fallback，不拖慢下一组尝试）
		//   - 桌面窗口已出现      → 立即返回 9999（不再傻等整个 bootTimeout，
		//                          解决 sdl 多屏「窗口 2~3s 就出来却要等满 6s」的问题）
		//   - bootTimeout 之前    → 只探测 + 恢复「被最小化」的窗口（SW_RESTORE 绝对安全）；
		//                          不做 TOPMOST/抢前台，避免干扰 SDL 的 handleShow 建窗流程
		//   - bootTimeout 之后    → 开始主动激活（恢复 + TOPMOST 闪烁 + 拉前台），
		//                          处理窗口被最小化/压后台的情况
		//   - bootTimeout+windowGrace 后仍无窗口 → 卡在协商阶段 → 杀进程树 return -1
		activateAt := time.Now().Add(bootTimeout)
		graceDeadline := time.Now().Add(bootTimeout + windowGrace)
		for {
			// 在等待期间进程可能已经自己退出了
			select {
			case exitCode := <-done:
				return exitCode, cmdFile, logFile, nil
			default:
			}
			pid := uint32(cmd.Process.Pid)
			if time.Now().After(activateAt) {
				activateSessionWindow(pid)
			} else {
				restoreIconicSessionWindows(pid)
			}
			// 窗口 >50x50 → 桌面正在渲染 → 真成功
			if hasVisibleDesktopWindow(pid) {
				return 9999, cmdFile, logFile, nil
			}
			if time.Now().After(graceDeadline) {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		// 宽限期结束仍无窗口 → 卡住了，杀掉进程树避免僵尸
		pid := uint32(cmd.Process.Pid)
		killProcessTree(pid)
		<-done // 等 goroutine 收尸完毕
		return -1, cmdFile, logFile, nil
	}

	// ====== 多级兜底矩阵（彻底防止「连上但没声音」） ======
	// 维度 1: 客户端 sdl-freerdp → wfreerdp
	// 维度 2: 级别   完整集(0) → 基础集(1) → 功能集(2，全带分辨率+声音但不带高级特性)
	// 维度 3: 完整集细分 「带 dyn-res + 带 toggles」→「不带 dyn-res + 带 toggles」→「带 dyn-res + 不带 toggles」→「全不带」
	// 维度 4: soundVariant × 5 种（v1/v2/min/sys:winmm/sys:wasapi）
	// 极端保底：最后再跑一轮「只有 /v /u /p + /sound 最简 + /cert:ignore」，保证任何情况下都有 RDPSND
	// ★★★ 客户端优先级 ★★★
	//   单屏：wfreerdp.exe 优先！
	//     原因：sdl-freerdp.exe 的 /floatbar 在 Windows 全屏模式下有 bug（GitHub #12801），
	//           直到 3.25.0 才修复 wfreerdp.exe 的 floatbar（PR #12594），sdl-freerdp 仍未修复。
	//           用户需要顶部悬浮菜单 → 必须用 wfreerdp.exe
	//   多显示器（MultiMon）：sdl-freerdp.exe 优先！
	//     原因：实测 wfreerdp 3.30.0（3.x 重写版 Windows 客户端）走 /multimon 时在协议层
	//           只上报 MonitorCount=1（gcc_write_client_monitor_data 日志铁证），服务端只
	//           建一块 5120x1440 拼接宽屏 → 用户看到「两个屏公用一个分辨率」；
	//           sdl-freerdp 正确上报 MonitorCount=2（主 2560x1440@0,0 + 副 @2560,0），
	//           与 mstsc /multimon 行为一致（真·主副屏，每块屏独立显示通道）。
	clients := []string{}
	appendClient := func(p string) {
		if p == "" {
			return
		}
		for _, c := range clients {
			if c == p {
				return
			}
		}
		if _, st := os.Stat(p); st == nil {
			clients = append(clients, p)
		}
	}
	exeDir := filepath.Dir(rdpClient)
	wfPath := filepath.Join(exeDir, "wfreerdp.exe")
	sdlPath := filepath.Join(exeDir, "sdl-freerdp.exe")
	if config.MultiMon {
		// 多显示器：sdl 优先（wfreerdp 只会上报 1 块屏，必然拼接）
		appendClient(sdlPath)
		appendClient(rdpClient)
		appendClient(wfPath) // 兜底：sdl 缺失时至少能连（但显示为拼接屏）
	} else {
		// 单屏：wfreerdp 优先（floatbar 已修复）；sdl 兜底
		appendClient(wfPath)
		appendClient(rdpClient)
		appendClient(sdlPath)
	}

	// 每个（客户端×级别）先跑 4 种完整集变体（如果级别是 0 就有用，否则 dynRes/advToggles 被 buildBase 忽略）
	type lvT struct {
		level      int
		name       string
		dynRes     bool
		advToggles bool
	}
	matrix := []lvT{
		{0, "完整集+dyn+tog", true, true},
		{0, "完整集-dyn+tog", false, true},
		{0, "完整集+dyn-tog", true, false},
		{0, "完整集-dyn-tog", false, false},
		{1, "基础集", false, false},
		{2, "功能集(全分辨率声音)", false, false},
	}

	soundNames := map[int]string{
		// ★ Hyper-V 优先组合（触发 INFO_AUDIO flag + 强制静态 VC 不出 DVC close 循环）
		8:  "HV★-dvc+winmm+redir", // 首选：禁 DVC 静态 VC + 纯输出 winmm + redirect
		9:  "HV★-dvc+winmm+m1",    // 纯输出 winmm + audio-mode:1
		10: "HV★-dvc+winmm+mic",   // 带麦克风
		11: "HV_DVC_winmm_fmt1",   // DVC 模式 + 显式格式
		12: "HV_DVC_wasapi_redir", // DVC + wasapi
		// 兼容旧变体
		0: "aout:winmm+mic", 1: "aout:wasapi+mic", 2: "aout:winmm", 3: "aout:wasapi", 4: "aout:auto",
		5: "snd_v1(num)", 6: "snd_v2(str)", 7: "snd_min",
		// 8 在旧逻辑里代表 bare（无音频），不冲突（仅 Audio=false 时使用），13 以上已重命名 8-12
		100: "bare",
	}
	cliShort := func(e string) string {
		if strings.Contains(strings.ToLower(e), "sdl") {
			return "sdl-freerdp"
		}
		return "wfreerdp"
	}
	// ★★★ Hyper-V 下尝试顺序：先「-dvc 静态 VC」（解决 INFO_AUDIO flag 缺失 + DVC 关了又开循环）
	//          → 再「DVC 纯输出」→ 最后「带麦克风 + 旧写法兜底」
	// 这样能最快命中能让 Hyper-V TermService 注册远程音频设备的组合
	soundVariants := []int{8, 9, 10, 11, 12, 2, 3, 0, 1, 4, 5, 6, 7}
	if !config.Audio {
		soundVariants = []int{100}
	}

	// ============== ★★★ 上次成功组合缓存（解决「退出再连又没声」顽疾）★★★ ==============
	// Hyper-V TermService 只要命中过一次能注册远程音频的参数组合，下次就应该直接复用（不重新矩阵穷举）
	// 否则矩阵顺序会碰到 v11 等 DVC 模式，虽然能连上桌面但没音频
	type successKey struct {
		ExePath    string `json:"exe_path"`
		Level      int    `json:"level"`
		SV         int    `json:"sv"`
		DynRes     bool   `json:"dyn_res"`
		AdvToggles bool   `json:"adv_toggles"`
		IsBare     bool   `json:"is_bare"`
	}
	cacheFile := filepath.Join(os.TempDir(),
		fmt.Sprintf("shoukehuduan_rdp_success_%s.json", sanitizeHost(config.Host)))
	// ---------- 写成功缓存（成功命中即持久化，下次优先使用） ----------
	// 注意：必须声明在 tryCached 之前（tryCached 内部要调用它做客户端升级）
	writeCache := func(k successKey) {
		if data, errJ := json.Marshal(k); errJ == nil {
			_ = os.WriteFile(cacheFile, data, 0644)
		}
	}
	// ---------- 加载缓存并优先尝试 ----------
	tryCached := func() bool {
		if data, errR := os.ReadFile(cacheFile); errR == nil && len(data) > 0 {
			var k successKey
			if errJ := json.Unmarshal(data, &k); errJ == nil {
				if k.IsBare {
					// BARE_ULTIMATE 分支的成功缓存
					// ★ 保持参数与「终极保底」路径一致
					for _, exePath := range clients {
						a := []string{fmt.Sprintf("/v:%s:%d", config.Host, config.Port)}
						if config.Username != "" {
							a = append(a, fmt.Sprintf("/u:%s", config.Username))
						}
						if config.VMPassword != "" {
							a = append(a, fmt.Sprintf("/p:%s", config.VMPassword))
						}
						if config.Fullscreen {
							a = append(a, "/f")
						}
						// 悬浮菜单：默认显示，开启「隐藏悬浮菜单」时不传
						if !config.HideToolbar {
							a = append(a, "/floatbar")
						}
						// 多显示器：每块屏独立显示通道（服务端需专业版/企业版/服务器版 Windows）
						if config.MultiMon {
							a = append(a, "/multimon")
						}
						a = append(a, "/cert:ignore")
						if config.Audio {
							a = append(a, "/sound:sys:winmm")
						}
						if config.USBRedirect {
							a = append(a, buildUSBArg(config))
						}
						altF4Flag := 0
						if config.BlockAltF4 {
							altF4Flag = -1
						}
						if exit, _, _, _ := runOnce(exePath, a, altF4Flag); exit == 9999 {
							return true
						}
					}
				} else {
					// 矩阵分支的成功缓存：精确还原（客户端+级别+sv+dynRes+advToggles）
					exePath := k.ExePath
					if _, st := os.Stat(exePath); st != nil {
						// 客户端路径变化（比如目录移动），回退到当前 clients
						exePath = clients[0]
					}
					// ★ 多显示器模式：缓存的客户端若是 wfreerdp → 协议层只上报 1 块屏（拼接），
					//   必须改用 sdl-freerdp（正确上报双屏）。否则开过单屏的旧缓存会一直把
					//   多显示器连接钉在拼接屏上。
					if config.MultiMon && cliShort(exePath) != "sdl-freerdp" {
						for _, c := range clients {
							if cliShort(c) == "sdl-freerdp" {
								exePath = c
								break
							}
						}
					}
					altF4Flag := 0
					if config.BlockAltF4 {
						altF4Flag = -1
					}
					// ★ 用户要悬浮菜单（默认）时，/floatbar 只有 wfreerdp.exe 能正常显示
					//   （sdl-freerdp 的 floatbar 有已知 bug，GitHub #12801）。
					//   若缓存的客户端是 sdl-freerdp：先用同一参数组合试 wfreerdp，
					//   成功则升级缓存为 wfreerdp（以后直连）；失败再回退缓存的 sdl 客户端
					//   （保证能连上，代价是该主机没有悬浮菜单）。
					//   ※ 多显示器模式下禁止此升级：wfreerdp 的 /multimon 只上报 1 块屏（拼接），
					//     必须留在 sdl-freerdp。
					if !config.MultiMon && !config.HideToolbar && cliShort(exePath) != "wfreerdp" {
						for _, c := range clients {
							if cliShort(c) != "wfreerdp" || c == exePath {
								continue
							}
							au := buildBase(k.Level, k.SV, k.DynRes, k.AdvToggles, cliShort(c) == "sdl-freerdp")
							if exit, _, _, _ := runOnce(c, au, altF4Flag); exit == 9999 {
								writeCache(successKey{
									ExePath:    c,
									Level:      k.Level,
									SV:         k.SV,
									DynRes:     k.DynRes,
									AdvToggles: k.AdvToggles,
									IsBare:     false,
								})
								return true
							}
						}
					}
					a := buildBase(k.Level, k.SV, k.DynRes, k.AdvToggles, cliShort(exePath) == "sdl-freerdp")
					if exit, _, _, _ := runOnce(exePath, a, altF4Flag); exit == 9999 {
						if exePath != k.ExePath {
							// 客户端被多显示器逻辑改写（wfreerdp→sdl）→ 更新缓存，下次直连
							writeCache(successKey{
								ExePath:    exePath,
								Level:      k.Level,
								SV:         k.SV,
								DynRes:     k.DynRes,
								AdvToggles: k.AdvToggles,
								IsBare:     false,
							})
						}
						return true
					}
				}
			}
		}
		return false
	}
	if config.Audio && tryCached() {
		return nil
	}

	lastErr := fmt.Errorf("未知错误")
	lastDiag := ""
	total := 0
	for cliIdx, exePath := range clients {
		for _, m := range matrix {
			for _, sv := range soundVariants {
				a := buildBase(m.level, sv, m.dynRes, m.advToggles, cliShort(exePath) == "sdl-freerdp")
				// BlockAltF4: 启动后等 1.2s → 枚举本进程创建的 FreeRDP 窗口 → 移除 WS_SYSMENU（彻底屏蔽 Alt+F4 / X / 系统菜单）
				altF4Pid := 0
				if config.BlockAltF4 {
					altF4Pid = -1
				}
				exit, cmdFile, logFile, errRun := runOnce(exePath, a, altF4Pid)
				total++
				if exit == 9999 {
					// ★ 启动成功 → 立刻写入成功缓存，下次复用（不继续尝试其他组合！）
					writeCache(successKey{
						ExePath:    exePath,
						Level:      m.level,
						SV:         sv,
						DynRes:     m.dynRes,
						AdvToggles: m.advToggles,
						IsBare:     false,
					})
					return nil
				}
				svName := soundNames[sv]
				msg := fmt.Sprintf("[%s#%d / %s / %s] exit=%d cmd=%s log=%s",
					cliShort(exePath), cliIdx+1, m.name, svName, exit, cmdFile, logFile)
				if errRun != nil {
					lastErr = fmt.Errorf("%s err=%w", msg, errRun)
				} else {
					lastErr = fmt.Errorf("%s", msg)
				}
				lastDiag += msg + " | "
			}
		}
	}

	// 终极保底：任何情况都失败时，只带 /v /u /p + /f + /cert:ignore + /sound:sys:winmm（Hyper-V TermService 能识别的音频写法）
	fmt.Printf("[终极保底] Fullscreen=%v HideToolbar=%v Audio=%v\n", config.Fullscreen, config.HideToolbar, config.Audio)
	for cliIdx, exePath := range clients {
		a := []string{
			fmt.Sprintf("/v:%s:%d", config.Host, config.Port),
		}
		if config.Username != "" {
			a = append(a, fmt.Sprintf("/u:%s", config.Username))
		}
		if config.VMPassword != "" {
			a = append(a, fmt.Sprintf("/p:%s", config.VMPassword))
		}
		if config.Fullscreen {
			a = append(a, "/f")
		}
		// 悬浮菜单：默认显示，开启「隐藏悬浮菜单」时不传 /floatbar
		if !config.HideToolbar {
			a = append(a, "/floatbar")
		}
		// 多显示器：每块屏独立显示通道（服务端需专业版/企业版/服务器版 Windows；/multimon 要求全屏，本软件恒全屏）
		if config.MultiMon {
			a = append(a, "/multimon")
		}
		a = append(a, "/cert:ignore")
		if config.Audio {
			a = append(a, "/sound:sys:winmm") // 真实 /? 确认过的音频输出写法（Hyper-V Win10 TermService 能识别）
		}
		if config.USBRedirect {
			a = append(a, buildUSBArg(config))
		}
		altF4Flag := 0
		if config.BlockAltF4 {
			altF4Flag = -1
		}
		exit, cmdFile, logFile, errRun := runOnce(exePath, a, altF4Flag)
		total++
		if exit == 9999 {
			writeCache(successKey{ExePath: exePath, IsBare: true})
			return nil
		}
		msg := fmt.Sprintf("[%s#%d / BARE_ULTIMATE(v:/u:/p+cert+sound_winmm)] exit=%d cmd=%s log=%s",
			cliShort(exePath), cliIdx+1, exit, cmdFile, logFile)
		if errRun != nil {
			lastErr = fmt.Errorf("%s err=%w", msg, errRun)
		} else {
			lastErr = fmt.Errorf("%s", msg)
		}
		lastDiag += msg + " | "
	}
	return fmt.Errorf("RDP 客户端多级兜底全部失败（%d 次尝试）。可能原因：目标主机 3389 不通 / 账号密码错误 / NLA 未配置。%v 详细:%s",
		total, lastErr, lastDiag)
}

// resolveBundledClientsDir 从 exe 所在目录开始向上回溯，直到找到 dependencies/clients 文件夹
// 覆盖场景：
//  1. 开发期：EXE 位于 cmd/desktop/，dependencies 在项目根（../../dependencies/clients）
//  2. 开发期：EXE 位于项目根（desktop.exe），dependencies 同级（./dependencies/clients）
//  3. 生产部署：EXE 和 dependencies 同级
//  4. 当前工作目录 cwd 指向项目根
func resolveBundledClientsDir() string {
	candidates := []string{
		filepath.Join(getAppDir(), "dependencies", "clients"),
		filepath.Join(getAppDir(), "..", "dependencies", "clients"),
		filepath.Join(getAppDir(), "..", "..", "dependencies", "clients"),
		filepath.Join(getAppDir(), "..", "..", "..", "dependencies", "clients"),
		filepath.Join(getAppDir(), "..", "..", "..", "..", "dependencies", "clients"),
		filepath.Join(getAppDir(), "..", "..", "..", "..", "..", "dependencies", "clients"),
		filepath.Join(".", "dependencies", "clients"),
		filepath.Join(".", "..", "dependencies", "clients"),
		filepath.Join(".", "..", "..", "dependencies", "clients"),
		filepath.Join(".", "..", "..", "..", "dependencies", "clients"),
	}
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if st, err := os.Stat(abs); err == nil && st.IsDir() {
			return abs
		}
	}
	// 找不到时给一个默认，由调用方判断文件是否存在
	abs, _ := filepath.Abs(filepath.Join(getAppDir(), "dependencies", "clients"))
	return abs
}

// findBundledFreeRDPClient 搜索项目自带的 dependencies/clients/freerdp 目录
// 返回找到的 exe 绝对路径或空字符串
func findBundledFreeRDPClient() string {
	clientsRoot := resolveBundledClientsDir()
	variantDirs := []string{
		filepath.Join("freerdp", "vs-build", "bin"),
		filepath.Join("freerdp", "mingw64", "bin"),
		filepath.Join("freerdp", "ucrt64", "bin"),
		filepath.Join("freerdp"),
	}
	binNames := []string{"wfreerdp.exe", "sdl-freerdp.exe", "xfreerdp.exe"}
	for _, v := range variantDirs {
		for _, b := range binNames {
			p := filepath.Join(clientsRoot, v, b)
			if _, err := os.Stat(p); err == nil {
				abs, _ := filepath.Abs(p)
				return abs
			}
		}
	}
	return ""
}

// findBundledHorizonClient 搜索项目自带的 dependencies/clients/horizon 目录
// 返回找到的 horizon-client.exe（Omnissa 8.15+）或 vmware-view.exe（旧 VMware 版）绝对路径或空字符串
func findBundledHorizonClient() string {
	clientsRoot := resolveBundledClientsDir()
	fmt.Printf("[Horizon] resolveBundledClientsDir -> %s\n", clientsRoot)
	variantDirs := []string{
		filepath.Join("horizon"),
		filepath.Join("horizon", "bin"),
		filepath.Join("Omnissa Horizon Client"),
		filepath.Join("VMware Horizon Client"),
	}
	binNames := []string{"horizon-client.exe", "vmware-view.exe"}
	for _, v := range variantDirs {
		for _, b := range binNames {
			p := filepath.Join(clientsRoot, v, b)
			if _, err := os.Stat(p); err == nil {
				abs, _ := filepath.Abs(p)
				fmt.Printf("[Horizon] Found bundled client at: %s\n", abs)
				return abs
			}
		}
	}
	fmt.Printf("[Horizon] No bundled client found in %s\n", clientsRoot)
	return ""
}

func (m *ProtocolManager) findWindowsRDPClient() (string, error) {
	// 1. 优先找项目自带的 FreeRDP（dependencies/clients/freerdp）
	if bundled := findBundledFreeRDPClient(); bundled != "" {
		return bundled, nil
	}

	// 2. Check config-specified paths
	if globalClientPaths != nil {
		cfgPath := globalClientPaths.RDPWindowsPath
		if cfgPath != "" {
			if _, err := os.Stat(cfgPath); err == nil {
				return cfgPath, nil
			}
		}
	}

	// 3. Check system-installed FreeRDP locations (including MSYS2)
	systemPaths := []string{
		"D:\\kaifahuanjing\\msys64\\mingw64\\bin\\wfreerdp.exe",
		"D:\\kaifahuanjing\\msys64\\mingw64\\bin\\sdl-freerdp.exe",
		"C:\\msys64\\mingw64\\bin\\wfreerdp.exe",
		"C:\\msys64\\mingw64\\bin\\sdl-freerdp.exe",
		"C:\\Program Files\\FreeRDP\\bin\\wfreerdp.exe",
		"C:\\Program Files (x86)\\FreeRDP\\bin\\wfreerdp.exe",
		"C:\\Program Files\\FreeRDP\\wfreerdp.exe",
	}

	for _, path := range systemPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// 4. Check PATH for FreeRDP
	freeRDPClients := []string{
		"wfreerdp.exe",
		"sdl-freerdp.exe",
		"xfreerdp.exe",
	}

	for _, client := range freeRDPClients {
		if path, err := exec.LookPath(client); err == nil {
			return path, nil
		}
	}

	// 5. LAST FALLBACK: Only use mstsc.exe if explicitly allowed
	// User explicitly requested FreeRDP over mstsc, so return error rather than fallback
	return "", fmt.Errorf("未找到 FreeRDP 客户端 (wfreerdp.exe)。请确认项目目录下 dependencies/clients/freerdp 完整，或在配置文件中指定 FreeRDP 客户端路径")
}

func (m *ProtocolManager) launchMacRDP(config *ConnectionConfig) error {
	rdpFile, err := m.generateRDPFile(config)
	if err != nil {
		return err
	}

	openCmd := exec.Command("open", rdpFile)
	return openCmd.Start()
}

func (m *ProtocolManager) launchLinuxRDP(config *ConnectionConfig) error {
	rdpClient, err := m.findLinuxRDPClient()
	if err != nil {
		return err
	}

	args := []string{
		fmt.Sprintf("%s:%d", config.Host, config.Port),
	}

	if rdpClient == "freerdp" {
		args = append([]string{
			fmt.Sprintf("/v:%s:%d", config.Host, config.Port),
			fmt.Sprintf("/u:%s", config.Username),
			fmt.Sprintf("/p:%s", config.VMPassword),
			"/w:1920",
			"/h:1080",
			"/dynamic-resolution",
			"/cert:ignore",
		}, args...)
	} else if rdpClient == "rdesktop" {
		args = append([]string{
			fmt.Sprintf("-u %s", config.Username),
			fmt.Sprintf("-p %s", config.VMPassword),
			"-g 1920x1080",
		}, args...)
	}

	cmd := exec.Command(rdpClient, args...)
	cmd.SysProcAttr = getSysProcAttr()
	return cmd.Start()
}

func (m *ProtocolManager) generateRDPFile(config *ConnectionConfig) (string, error) {
	usernameLine := "username:s:"
	if config.Username != "" {
		usernameLine = fmt.Sprintf("username:s:%s", config.Username)
	}

	content := fmt.Sprintf(`screen mode id:i:2
desktopwidth:i:1920
desktopheight:i:1080
session bpp:i:32
winposstr:s:0,3,0,0,1920,1080
full address:s:%s:%d
%s
`, config.Host, config.Port, usernameLine)

	filePath := filepath.Join(m.tempDir, fmt.Sprintf("connection_%s.rdp", sanitizeHost(config.Host)))
	err := os.WriteFile(filePath, []byte(content), 0600)
	if err != nil {
		return "", fmt.Errorf("创建 RDP 文件失败: %w", err)
	}

	return filePath, nil
}

func (m *ProtocolManager) findLinuxRDPClient() (string, error) {
	clients := []string{
		"xfreerdp",
		"freerdp",
		"rdesktop",
		"remmina",
	}

	for _, client := range clients {
		if path, err := exec.LookPath(client); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("未找到 RDP 客户端，请安装 xfreerdp 或 rdesktop")
}

// diagnoseBundledDir 返回便携版目录诊断信息，用于调试时显示解析到哪个 clientsRoot
func diagnoseBundledDir() string {
	exe, exeErr := os.Executable()
	exeDir := ""
	if exeErr == nil {
		exeDir = filepath.Dir(exe)
	}
	cwd, _ := os.Getwd()
	return fmt.Sprintf("exeDir=%s | cwd=%s | clientsRoot=%s", exeDir, cwd, resolveBundledClientsDir())
}

func (m *ProtocolManager) checkRDPDependency() (bool, string, error) {
	switch runtime.GOOS {
	case "windows":
		// 1. 优先找项目自带的 FreeRDP
		if bundled := findBundledFreeRDPClient(); bundled != "" {
			return true, fmt.Sprintf("✅ 已找到便携版 FreeRDP: %s | %s", bundled, diagnoseBundledDir()), nil
		}

		// 2. Check config-specified paths
		if globalClientPaths != nil && globalClientPaths.RDPWindowsPath != "" {
			if _, err := os.Stat(globalClientPaths.RDPWindowsPath); err == nil {
				return true, fmt.Sprintf("✅ 已找到配置的 FreeRDP | %s", diagnoseBundledDir()), nil
			}
		}

		// 3. Check system-installed FreeRDP paths (including MSYS2)
		systemPaths := []string{
			"D:\\kaifahuanjing\\msys64\\mingw64\\bin\\wfreerdp.exe",
			"D:\\kaifahuanjing\\msys64\\mingw64\\bin\\sdl-freerdp.exe",
			"C:\\msys64\\mingw64\\bin\\wfreerdp.exe",
			"C:\\msys64\\mingw64\\bin\\sdl-freerdp.exe",
			"C:\\Program Files\\FreeRDP\\bin\\wfreerdp.exe",
			"C:\\Program Files (x86)\\FreeRDP\\bin\\wfreerdp.exe",
		}
		for _, path := range systemPaths {
			if _, err := os.Stat(path); err == nil {
				return true, fmt.Sprintf("✅ 系统安装的 FreeRDP (%s)", filepath.Base(path)), nil
			}
		}

		// 4. Check PATH for FreeRDP clients
		freeRDPClients := []string{"wfreerdp.exe", "sdl-freerdp.exe", "xfreerdp.exe"}
		for _, client := range freeRDPClients {
			if path, err := exec.LookPath(client); err == nil {
				return true, fmt.Sprintf("✅ PATH 中 FreeRDP (%s)", path), nil
			}
		}

		return false, fmt.Sprintf("❌ 未找到 FreeRDP | %s", diagnoseBundledDir()), nil

	case "darwin":
		// Check for FreeRDP on macOS via brew or similar
		if _, err := exec.LookPath("xfreerdp"); err == nil {
			return true, "已找到 FreeRDP (xfreerdp)", nil
		}
		if _, err := os.Stat("/Applications/Microsoft Remote Desktop.app"); err == nil {
			return true, "已找到 Microsoft Remote Desktop (macOS)", nil
		}
		return false, "请安装 FreeRDP (brew install freerdp) 或 Microsoft Remote Desktop", nil
	case "linux":
		rdpClient, err := m.findLinuxRDPClient()
		if err == nil {
			return true, fmt.Sprintf("已找到 RDP 客户端: %s", rdpClient), nil
		}
		return false, "请安装 FreeRDP (sudo apt install freerdp-x11 或 sudo pacman -S freerdp)", nil
	default:
		return false, "不支持的操作系统", nil
	}
}

// findBundledSPICEClient 搜索项目自带的 dependencies/clients/virt-viewer 绿色版
func findBundledSPICEClient() string {
	clientsRoot := resolveBundledClientsDir()
	dirCandidates := []string{
		filepath.Join("virt-viewer", "bin"),
		filepath.Join("virt-viewer"),
		filepath.Join("VirtViewer", "bin"),
		filepath.Join("VirtViewer v11.0-256", "bin"),
		filepath.Join("spice"),
	}
	binNames := []string{"remote-viewer.exe", "virt-viewer.exe"}
	for _, dir := range dirCandidates {
		for _, b := range binNames {
			p := filepath.Join(clientsRoot, dir, b)
			if _, err := os.Stat(p); err == nil {
				abs, _ := filepath.Abs(p)
				return abs
			}
		}
	}
	return ""
}

func (m *ProtocolManager) findWindowsSPICEClient() (string, error) {
	// 0. 优先找项目内置绿色版 virt-viewer（用户安装后整个目录复制进 dependencies/clients/ 即可）
	if bundled := findBundledSPICEClient(); bundled != "" {
		return bundled, nil
	}

	// 1. 检查用户自定义路径
	if globalClientPaths != nil && globalClientPaths.SPICEWindowsPath != "" {
		if _, err := os.Stat(globalClientPaths.SPICEWindowsPath); err == nil {
			return globalClientPaths.SPICEWindowsPath, nil
		}
	}

	// 2. Virt Viewer 官方安装路径（最常见）
	systemPaths := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "VirtViewer", "bin", "remote-viewer.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "VirtViewer", "bin", "virt-viewer.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "VirtViewer", "bin", "remote-viewer.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "VirtViewer", "bin", "virt-viewer.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "virt-viewer", "bin", "remote-viewer.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "virt-viewer", "bin", "remote-viewer.exe"),
		"C:\\Program Files\\VirtViewer v11.0-256\\bin\\remote-viewer.exe",
		"C:\\Program Files\\VirtViewer v11.0-256\\bin\\virt-viewer.exe",
	}
	for _, p := range systemPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// 3. 检查 PATH
	for _, client := range []string{"remote-viewer.exe", "virt-viewer.exe"} {
		if p, err := exec.LookPath(client); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("未找到 SPICE 客户端 (remote-viewer.exe)。将已安装的 VirtViewer 整个文件夹复制到 dependencies/clients/virt-viewer，或双击安装 dependencies/clients/virt-viewer-x64-11.0.msi")
}

func (m *ProtocolManager) launchWindowsSPICE(config *ConnectionConfig) error {
	spiceFile, err := m.generateSPICEFile(config)
	if err != nil {
		return err
	}

	spiceClient, err := m.findWindowsSPICEClient()
	if err != nil {
		return err
	}

	args := []string{}
	if config.HideToolbar {
		// 「隐藏悬浮菜单」开启：remote-viewer 没有单独隐藏工具栏的参数，
		// 用 kiosk 模式彻底隐藏顶部悬浮工具栏/菜单；
		// --kiosk-quit=on-disconnect 保证虚拟机断开/关机后 remote-viewer 自动退出，返回主程序
		args = append(args, "--kiosk", "--kiosk-quit=on-disconnect")
	}
	args = append(args, spiceFile)

	cmd := exec.Command(spiceClient, args...)
	// Critical: WorkingDirectory 必须设为 exe 所在目录，
	// 否则绿色版 portable virt-viewer 找不到同目录的 GTK/GLib/DLL 依赖
	cmd.Dir = filepath.Dir(spiceClient)
	cmd.SysProcAttr = getSysProcAttr()
	return cmd.Start()
}

func (m *ProtocolManager) launchMacSPICE(config *ConnectionConfig) error {
	spiceClient, err := exec.LookPath("remote-viewer")
	if err != nil {
		spiceClient = "/Applications/Virt Viewer.app/Contents/MacOS/remote-viewer"
		if _, err := os.Stat(spiceClient); err != nil {
			return fmt.Errorf("未找到 SPICE 客户端，请安装 virt-viewer for macOS")
		}
	}

	spiceFile, err := m.generateSPICEFile(config)
	if err != nil {
		return err
	}

	cmd := exec.Command(spiceClient, spiceFile)
	return cmd.Start()
}

func (m *ProtocolManager) launchLinuxSPICE(config *ConnectionConfig) error {
	spiceClient, err := m.findLinuxSPICEClient()
	if err != nil {
		return err
	}

	spiceFile, err := m.generateSPICEFile(config)
	if err != nil {
		return err
	}

	cmd := exec.Command(spiceClient, spiceFile)
	cmd.SysProcAttr = getSysProcAttr()
	return cmd.Start()
}

func (m *ProtocolManager) generateSPICEFile(config *ConnectionConfig) (string, error) {
	var content string

	if config.Proxy != "" {
		// PVE /spiceproxy 代理模式：remote-viewer 经 PVE 节点 3128 代理连接 SPICE，
		// host 为 pvespiceproxy:<token> 形式，需带 proxy/ca/host-subject/tls-port。
		// ca 为 PEM 多行证书（JSON 解析后 \n 已是真实换行），直接多行写入。
		title := config.Title
		if title == "" {
			title = "PVE SPICE Console"
		}
		content = fmt.Sprintf(`[virt-viewer]
type=spice
host=%s
password=%s
tls-port=%d
proxy=%s
host-subject=%s
ca=%s
title=%s
toggle-fullscreen=Shift+F11
release-cursor=Ctrl+Alt+R
secure-attention=Ctrl+Alt+Ins
delete-this-file=1
fullscreen=0
`, config.Host, config.VMPassword, config.TLSPort, config.Proxy, config.HostSubject, config.CA, title)
	} else {
		// SPICE 直连模式
		tlsPort := ""
		if config.TLSPort > 0 {
			tlsPort = fmt.Sprintf("tls-port=\"%d\"", config.TLSPort)
		}

		// SPICE 使用 VMPassword 作为虚拟机登录密码
		content = fmt.Sprintf(`[virt-viewer]
type=spice
host=%s
port=%d
password=%s
%s
fullscreen=0
`, config.Host, config.Port, config.VMPassword, tlsPort)
	}

	fileName := fmt.Sprintf("connection_%s.vv", sanitizeHost(config.Host))
	if config.Proxy != "" {
		// 代理模式 host 为 pvespiceproxy:<token>，用固定文件名避免特殊字符/过长
		fileName = "connection_spice_pve.vv"
	}
	filePath := filepath.Join(m.tempDir, fileName)
	err := os.WriteFile(filePath, []byte(content), 0600)
	if err != nil {
		return "", fmt.Errorf("创建 SPICE 连接文件失败: %w", err)
	}

	return filePath, nil
}

func (m *ProtocolManager) findLinuxSPICEClient() (string, error) {
	clients := []string{
		"remote-viewer",
		"virt-viewer",
		"spicy",
	}

	for _, client := range clients {
		if path, err := exec.LookPath(client); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("未找到 SPICE 客户端，请安装 virt-viewer (sudo apt install virt-viewer)")
}

func (m *ProtocolManager) checkSPICEDependency() (bool, string, error) {
	switch runtime.GOOS {
	case "windows":
		if client, err := m.findWindowsSPICEClient(); err == nil {
			return true, fmt.Sprintf("✅ 已找到 SPICE 客户端: %s | %s", client, diagnoseBundledDir()), nil
		}
		return false, fmt.Sprintf("❌ 未找到 SPICE 客户端 | %s", diagnoseBundledDir()), nil
	case "darwin":
		if _, err := os.Stat("/Applications/Virt Viewer.app"); err == nil {
			return true, "已找到 Virt Viewer", nil
		}
		return false, "请安装 Virt Viewer for macOS", nil
	case "linux":
		spiceClient, err := m.findLinuxSPICEClient()
		if err == nil {
			return true, fmt.Sprintf("已找到 SPICE 客户端: %s", spiceClient), nil
		}
		return false, "请安装 virt-viewer (sudo apt install virt-viewer)", nil
	default:
		return false, "不支持的操作系统", nil
	}
}

// findBundledVNCClient 搜索项目自带的 dependencies/clients/tigervnc 绿色版
func findBundledVNCClient() string {
	clientsRoot := resolveBundledClientsDir()
	dirCandidates := []string{
		filepath.Join("tigervnc"),
		filepath.Join("tigervnc", "bin"),
		filepath.Join("TigerVNC"),
		filepath.Join("TigerVNC Viewer"),
		filepath.Join("vnc"),
		filepath.Join("realvnc"),
		filepath.Join("tightvnc"),
	}
	binNames := []string{"vncviewer64.exe", "vncviewer.exe", "vncviewer-tigervnc.exe"}
	for _, dir := range dirCandidates {
		for _, b := range binNames {
			p := filepath.Join(clientsRoot, dir, b)
			if _, err := os.Stat(p); err == nil {
				abs, _ := filepath.Abs(p)
				return abs
			}
		}
	}
	return ""
}

func (m *ProtocolManager) findWindowsVNCClient() (string, error) {
	// 0. 优先找项目内置绿色版（复制安装后的文件夹到 dependencies/clients/tigervnc 即可）
	if bundled := findBundledVNCClient(); bundled != "" {
		return bundled, nil
	}

	// 1. 用户自定义路径
	if globalClientPaths != nil && globalClientPaths.VNCWindowsPath != "" {
		if _, err := os.Stat(globalClientPaths.VNCWindowsPath); err == nil {
			return globalClientPaths.VNCWindowsPath, nil
		}
	}

	// 2. 常见 VNC 客户端安装路径（TigerVNC / RealVNC / TightVNC / UltraVNC）
	systemPaths := []string{
		// TigerVNC (项目提供的安装包: tigervnc64-1.16.2.exe)
		filepath.Join(os.Getenv("ProgramFiles"), "TigerVNC", "vncviewer64.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "TigerVNC", "vncviewer.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "TigerVNC", "vncviewer.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "TigerVNC Viewer", "vncviewer64.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "TigerVNC Viewer", "vncviewer.exe"),
		// RealVNC
		filepath.Join(os.Getenv("ProgramFiles"), "RealVNC", "vncviewer.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "RealVNC", "vncviewer.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "RealVNC", "VNC Viewer", "vncviewer.exe"),
		// TightVNC
		filepath.Join(os.Getenv("ProgramFiles"), "TightVNC", "vncviewer.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "TightVNC", "vncviewer.exe"),
		// UltraVNC
		filepath.Join(os.Getenv("ProgramFiles"), "uvnc bvba", "UltraVNC", "vncviewer.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "uvnc bvba", "UltraVNC", "vncviewer.exe"),
	}
	for _, p := range systemPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// 3. PATH 搜索
	for _, client := range []string{"vncviewer.exe", "vncviewer64.exe", "vncviewer-tigervnc.exe"} {
		if p, err := exec.LookPath(client); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("未找到 VNC 客户端。将 TigerVNC 安装后的整个文件夹复制到 dependencies/clients/tigervnc，或双击安装 dependencies/clients/tigervnc64-1.16.2.exe")
}

func (m *ProtocolManager) launchWindowsVNC(config *ConnectionConfig) error {
	vncClient, err := m.findWindowsVNCClient()
	if err != nil {
		return err
	}

	display := fmt.Sprintf("%d", config.Port-5900)
	if display == "0" {
		display = "0"
	}
	// 兼容 display 模式和 port 模式
	hostPort := fmt.Sprintf("%s:%d", config.Host, config.Port)
	hostDisplay := fmt.Sprintf("%s:%s", config.Host, display)

	// 检测 VNC 客户端类型以确定密码传递方式
	clientName := strings.ToLower(filepath.Base(vncClient))

	var args []string
	switch {
	case strings.Contains(clientName, "tigervnc") || strings.Contains(clientName, "tightvnc"):
		// TigerVNC / TightVNC: vncviewer host:display password
		args = []string{hostDisplay, config.VMPassword}
	case strings.Contains(clientName, "realvnc") || strings.Contains(clientName, "vncviewer"):
		// RealVNC: vncviewer host:port -password=xxx  （兼容 TigerVNC 同样的语法）
		args = []string{hostPort}
		if config.VMPassword != "" {
			args = append(args, fmt.Sprintf("-password=%s", config.VMPassword))
		}
	default:
		args = []string{hostDisplay}
		if config.VMPassword != "" {
			args = append(args, config.VMPassword)
		}
	}

	cmd := exec.Command(vncClient, args...)
	// 对便携版 VNC 客户端设置 WorkingDir，避免找不到同目录下的 DLL/资源文件
	cmd.Dir = filepath.Dir(vncClient)
	cmd.SysProcAttr = getSysProcAttr()
	return cmd.Start()
}

func (m *ProtocolManager) launchMacVNC(config *ConnectionConfig) error {
	// macOS 使用 URL scheme 启动 VNC，支持密码自动填充
	url := fmt.Sprintf("vnc://:%s@%s:%d", config.VMPassword, config.Host, config.Port)
	cmd := exec.Command("open", url)
	return cmd.Start()
}

func (m *ProtocolManager) launchLinuxVNC(config *ConnectionConfig) error {
	vncClient, err := m.findLinuxVNCClient()
	if err != nil {
		return err
	}

	display := fmt.Sprintf("%d", config.Port-5900)
	clientName := filepath.Base(vncClient)

	var args []string
	switch {
	case strings.Contains(clientName, "tigervnc") || strings.Contains(clientName, "vncviewer"):
		// TigerVNC: vncviewer host:display password
		args = []string{
			fmt.Sprintf("%s:%s", config.Host, display),
			config.VMPassword,
		}
	case strings.Contains(clientName, "tightvnc"):
		// TightVNC: tightvncviewer host:display password
		args = []string{
			fmt.Sprintf("%s:%s", config.Host, display),
			config.VMPassword,
		}
	default:
		// 默认: 使用 -passwd 参数
		args = []string{
			fmt.Sprintf("%s:%s", config.Host, display),
			fmt.Sprintf("-passwd", config.VMPassword),
		}
	}

	cmd := exec.Command(vncClient, args...)
	cmd.SysProcAttr = getSysProcAttr()
	return cmd.Start()
}

func (m *ProtocolManager) findLinuxVNCClient() (string, error) {
	clients := []string{
		"vncviewer",
		"tigervncviewer",
		"xtightvncviewer",
	}

	for _, client := range clients {
		if path, err := exec.LookPath(client); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("未找到 VNC 客户端，请安装 tigervnc-tools")
}

func (m *ProtocolManager) checkVNCDependency() (bool, string, error) {
	switch runtime.GOOS {
	case "windows":
		if client, err := m.findWindowsVNCClient(); err == nil {
			return true, fmt.Sprintf("✅ 已找到 VNC 客户端: %s | %s", client, diagnoseBundledDir()), nil
		}
		return false, fmt.Sprintf("❌ 未找到 VNC 客户端 | %s", diagnoseBundledDir()), nil
	case "darwin":
		return true, "macOS 内置 VNC 支持", nil
	case "linux":
		vncClient, err := m.findLinuxVNCClient()
		if err == nil {
			return true, fmt.Sprintf("已找到 VNC 客户端: %s", vncClient), nil
		}
		return false, "请安装 tigervnc-tools", nil
	default:
		return false, "不支持的操作系统", nil
	}
}

func sanitizeHost(host string) string {
	return strings.ReplaceAll(strings.ReplaceAll(host, ":", "_"), ".", "_")
}

// writeHorizonCertRegistry writes VMware/Omnissa certificate trust registry keys.
// Horizon 8.15+ requires these keys (IgnoreCertErrors=TRUE, etc.) to successfully
// initialize Bora SSL (SSL_Init). Without them, Horizon crashes with
// VERIFY bora\lib\ssl\ssl.c:2384 on fresh systems.
// This function is a runtime fallback in case the installer didn't run or the
// registry keys were manually removed.
func writeHorizonCertRegistry() {
	if runtime.GOOS != "windows" {
		return
	}
	regValues := []struct {
		key   string
		name  string
		value string
		dword bool
	}{
		// VMware VDM Client (legacy path used by older Horizon versions)
		{`SOFTWARE\VMware, Inc.\VMware VDM\Client`, "IgnoreCertErrors", "TRUE", false},
		{`SOFTWARE\VMware, Inc.\VMware VDM\Client`, "EnableCertCertificateVerification", "false", false},
		{`SOFTWARE\VMware, Inc.\VMware VDM\Client`, "SecureConnect", "FALSE", false},
		{`SOFTWARE\VMware, Inc.\VMware VDM\Client`, "CertCheckMode", "0", true},
		{`SOFTWARE\VMware, Inc.\VMware VDM\Client`, "EnableCertHashCheck", "0", true},
		{`SOFTWARE\VMware, Inc.\VMware VDM\Client`, "CertHashCheckMode", "0", true},
		{`SOFTWARE\VMware, Inc.\VMware VDM\Client`, "SecurityMode", "0", true},
		{`SOFTWARE\VMware, Inc.\VMware VDM\Client\Security`, "IgnoreCertErrors", "3", true},
		{`SOFTWARE\VMware, Inc.\VMware VDM\Client\Security`, "CertCheckMode", "3", true},
		{`SOFTWARE\VMware, Inc.\VMware VDM\Client\Security`, "SecurityMode", "3", true},
		// Omnissa Horizon Client (new path used by Horizon 8.15+)
		{`SOFTWARE\Omnissa\Horizon\Client`, "IgnoreCertErrors", "TRUE", false},
		{`SOFTWARE\Omnissa\Horizon\Client`, "EnableCertCertificateVerification", "false", false},
		{`SOFTWARE\Omnissa\Horizon\Client`, "SecureConnect", "FALSE", false},
		{`SOFTWARE\Omnissa\Horizon\Client`, "CertCheckMode", "0", true},
		{`SOFTWARE\Omnissa\Horizon\Client`, "EnableCertHashCheck", "0", true},
		{`SOFTWARE\Omnissa\Horizon\Client`, "CertHashCheckMode", "0", true},
		{`SOFTWARE\Omnissa\Horizon\Client`, "SecurityMode", "0", true},
		{`SOFTWARE\Omnissa\Horizon\Security`, "IgnoreCertErrors", "3", true},
		{`SOFTWARE\Omnissa\Horizon\Security`, "CertCheckMode", "3", true},
		{`SOFTWARE\Omnissa\Horizon\Security`, "SecurityMode", "3", true},
	}
	// 注意：此函数绝对不能写 EnableShade/ShadePosition/noMenuBar 等 shade 相关键，
	// 因为这些键由 applyHorizonShadeHide()/applyHorizonShadeShow() 根据用户的
	// 「隐藏悬浮菜单」开关统一控制，在此写死会覆盖用户选择导致隐藏开关失效。
	for _, rv := range regValues {
		var args []string
		if rv.dword {
			args = []string{"add", `HKLM\` + rv.key, "/v", rv.name, "/t", "REG_DWORD", "/d", rv.value, "/f"}
		} else {
			args = []string{"add", `HKLM\` + rv.key, "/v", rv.name, "/t", "REG_SZ", "/d", rv.value, "/f"}
		}
		cmd := exec.Command("reg", args...)
		cmd.SysProcAttr = getSysProcAttr()
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("[registry] failed to write HKLM\\%s\\%s: %v (%s)\n", rv.key, rv.name, err, string(out))
		}
	}
	fmt.Println("[registry] Horizon certificate trust registry keys written")
}
