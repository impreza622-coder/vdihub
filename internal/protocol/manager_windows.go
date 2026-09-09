//go:build windows

package protocol

import (
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// getSysProcAttr returns Windows-specific process creation attributes
// 用于 FreeRDP / Horizon 等 GUI 程序：只用 CREATE_NO_WINDOW 防止附属控制台窗口弹出
// ★ 不能加 HideWindow=true，否则 GUI 程序的主窗口也会被隐藏
func getSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}

// horizonCertInfo holds extracted certificate metadata for Horizon Broker
type horizonCertInfo struct {
	CN         string // e.g. "W11-010"
	Subject    string // full subject string
	Thumbprint string // hex thumbprint
	NotBefore  string // certificate valid from
	NotAfter   string // certificate valid until
	CertDER    []byte // raw DER bytes
}

// ensureHorizonServerTrusted 确保 Horizon Client 信任指定服务器的证书。
// 支持新(Omnissa)和旧(VMware)版本的 Horizon Client 配置路径。
func ensureHorizonServerTrusted(serverHost string) {
	// 尝试多个可能的 prefs.txt 路径（新+旧版本）
	prefsPaths := []string{
		filepath.Join(os.Getenv("APPDATA"), "Omnissa", "Omnissa Horizon Client", "prefs.txt"),
		filepath.Join(os.Getenv("APPDATA"), "VMware", "VMware VDM", "Client", "prefs.txt"),
	}

	handled := false
	for _, prefsPath := range prefsPaths {
		dir := filepath.Dir(prefsPath)
		os.MkdirAll(dir, 0755)

		data, err := os.ReadFile(prefsPath)
		if err != nil {
			// 如果 prefs.txt 不存在，创建一个最小化的配置
			content := `<?xml version="1.0" encoding="utf-8"?>
<Root>
  <RecentServer serverName="` + serverHost + `" serverGuid="auto-` + serverHost + `" hasValidCert="true" mruUsername="" mruDomain="" lastLogInAsCurrentUser="false">
    <SecondaryServerList />
    <ShortCuts />
    <FileRedirection />
  </RecentServer>
  <BrokerJumpList />
  <DesktopJumpList />
  <ApplicationJumpList />
</Root>`
			if writeErr := os.WriteFile(prefsPath, []byte(content), 0644); writeErr == nil {
				fmt.Printf("Created prefs.txt at %s with trusted server %s\n", prefsPath, serverHost)
				handled = true
			}
			continue
		}

		content := string(data)

		// 1. 清理重复的 DisableCertCheck 标签
		content = removeDuplicateDisableCertCheck(content)

		// 2. 将所有 hasValidCert="false" 改为 "true"
		content = strings.ReplaceAll(content, `hasValidCert="false"`, `hasValidCert="true"`)

		// 3. 确保目标服务器在 RecentServer 列表中且 hasValidCert="true"
		if !strings.Contains(content, `serverName="`+serverHost+`"`) {
			newEntry := `  <RecentServer serverName="` + serverHost + `" serverGuid="auto-` + serverHost + `" hasValidCert="true" mruUsername="" mruDomain="" lastLogInAsCurrentUser="false">
    <SecondaryServerList />
    <ShortCuts />
    <FileRedirection />
  </RecentServer>
`
			if strings.Contains(content, "<RecentServer") {
				content = strings.Replace(content, "<RecentServer", newEntry+"<RecentServer", 1)
			} else {
				content = strings.Replace(content, "<Root>", "<Root>\n"+newEntry, 1)
			}
			fmt.Printf("Added trusted server %s to prefs.txt at %s\n", serverHost, prefsPath)
		} else {
			fmt.Printf("Server %s already in prefs.txt (%s)\n", serverHost, prefsPath)
		}

		if writeErr := os.WriteFile(prefsPath, []byte(content), 0644); writeErr == nil {
			handled = true
		} else {
			fmt.Printf("Failed to write prefs.txt %s: %v\n", prefsPath, writeErr)
		}
	}

	if !handled {
		fmt.Printf("Warning: Could not update any prefs.txt for server %s\n", serverHost)
	}
}

// removeDuplicateDisableCertCheck 清理 prefs.txt 中重复的 DisableCertCheck 标签
func removeDuplicateDisableCertCheck(content string) string {
	// 找到第一个 <DisableCertCheck>true</DisableCertCheck> 保留，删除其余的
	first := strings.Index(content, "<DisableCertCheck>")
	if first < 0 {
		return content
	}
	// 找到第一个的结束位置
	firstEnd := strings.Index(content[first:], "</DisableCertCheck>")
	if firstEnd < 0 {
		return content
	}
	firstEnd += first + len("</DisableCertCheck>")
	// 保留第一个，删除后面所有的
	rest := content[firstEnd:]
	rest = strings.ReplaceAll(rest, "<DisableCertCheck>true</DisableCertCheck>\n", "")
	rest = strings.ReplaceAll(rest, "<DisableCertCheck>true</DisableCertCheck>", "")
	return content[:firstEnd] + rest
}

// fetchHorizonBrokerCert connects to host:port via TLS (SkipVerify) and extracts the leaf certificate CN, thumbprint and DER bytes.
// Returns interface{} to match signature with the unix build stub (which returns a string for unsupported platforms).
func fetchHorizonBrokerCert(host string, port int) (interface{}, error) {
	if port <= 0 {
		port = 443
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	dialer := &tls.Dialer{
		Config: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         host,
		},
		// 3s 超时：防止 Broker 不可达时 TCP 默认超时（Windows 约 21s）拖慢启动
		NetDialer: &net.Dialer{Timeout: 3 * time.Second},
	}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("connect broker %s: %w", addr, err)
	}
	defer conn.Close()
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return nil, fmt.Errorf("not a tls conn")
	}
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return nil, fmt.Errorf("no peer certs")
	}
	leaf := state.PeerCertificates[0]
	cn := leaf.Subject.CommonName
	cn = strings.TrimPrefix(cn, "CN=")
	cn = strings.TrimSpace(cn)
	sum := sha256.Sum256(leaf.Raw)
	return &horizonCertInfo{
		CN:         cn,
		Subject:    leaf.Subject.String(),
		Thumbprint: fmt.Sprintf("%X", sum[:]),
		NotBefore:  leaf.NotBefore.Format("2006-01-02 15:04:05"),
		NotAfter:   leaf.NotAfter.Format("2006-01-02 15:04:05"),
		CertDER:    leaf.Raw,
	}, nil
}

// ensureHostsEntry writes an entry `ip  hostname` to the Windows hosts file.
// If an existing entry has the same hostname but different IP, it will be replaced.
// Returns true if a new entry was added or updated.
//
// ⚠️ 注意：hosts 文件写入需要管理员权限。如果失败，调用方会回退到其他证书绕过方法。
func ensureHostsEntry(ip, hostname string) (bool, error) {
	if ip == "" || hostname == "" {
		return false, nil
	}
	hostsPath := filepath.Join(os.Getenv("WINDIR"), "System32", "drivers", "etc", "hosts")
	data, err := os.ReadFile(hostsPath)
	if err != nil {
		return false, fmt.Errorf("read hosts: %w", err)
	}
	// normalize line endings
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	needle := strings.ToLower(strings.TrimSpace(hostname))
	ownIP := strings.ToLower(strings.TrimSpace(ip))

	// 检查是否存在相同 hostname 的条目
	hasExisting := false
	var newLines []string
	replaced := false
	for _, ln := range lines {
		clean := strings.TrimSpace(ln)
		if clean == "" || strings.HasPrefix(clean, "#") {
			newLines = append(newLines, ln)
			continue
		}
		fields := strings.Fields(clean)
		if len(fields) >= 2 {
			match := false
			for _, f := range fields[1:] {
				if strings.ToLower(strings.TrimSpace(f)) == needle {
					match = true
					break
				}
			}
			if match {
				hasExisting = true
				// 如果 IP 相同，跳过（已经正确）
				if strings.ToLower(strings.TrimSpace(fields[0])) == ownIP {
					newLines = append(newLines, ln)
				} else {
					// IP 不同，需要替换：删除旧条目和前面的注释行
					replaced = true
					// 移除这个条目（包括前面的 # 注释行）
					if len(newLines) > 0 && strings.HasPrefix(strings.TrimSpace(newLines[len(newLines)-1]), "#") {
						newLines = newLines[:len(newLines)-1]
					}
				}
			} else {
				newLines = append(newLines, ln)
			}
		} else {
			newLines = append(newLines, ln)
		}
	}

	// 如果已存在且 IP 正确，不需要修改
	if hasExisting && !replaced {
		return false, nil
	}

	// 写入修改后的 hosts 文件
	newText := strings.Join(newLines, "\r\n")
	// 确保末尾有换行
	newText = strings.TrimRight(newText, "\r\n") + "\r\n"
	// 添加新条目
	newText += fmt.Sprintf("# Horizon Broker CN auto-mapping (by shoukehuduan)\r\n%s %s\r\n", ip, hostname)

	if err := os.WriteFile(hostsPath, []byte(newText), 0644); err != nil {
		return false, fmt.Errorf("write hosts: %w (may need admin privileges)", err)
	}

	if replaced {
		return true, nil
	}
	return true, nil
}

// applyHorizonShadeHide hides the Horizon full-screen session bar (aka "Shade", top control bar):
// 1) GPO/注册表：EnableShade=False/0（REG_SZ + DWORD 双写，兼容不同 Horizon 版本）
//    - HKLM + HKCU + Policies（含 Policies\...\Client 正确子键，修复 Omnissa 8.15 不生效问题）
// 2) view.autoHideToolbar=TRUE（全屏模式下自动收起 shade）
// 3) view.noMenuBar=TRUE（全屏模式下完全抑制 menu bar，Kiosk 模式用的）
// 4) ShadePosition=TopCenter / shadeShowsPinned=FALSE（防止 shade 强制卡到屏幕左边缘无法操作）
// 5) 命令行再配合 -nomenubar=true / -hideClientAfterLaunchSession=true 双重保险
// 全部失败也不影响连接（best-effort，失败忽略）。
func applyHorizonShadeHide() {
	// ---------- REG_SZ (字符串) 键 ----------
	strKeys := []struct {
		hive  string
		path  string
		name  string
		value string
	}{
		// VMware 原品牌路径（兼容老版本/绿色版）
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "EnableShade", "False"},
		{"HKLM", `Software\VMware, Inc.\VMware VDM\Client`, "EnableShade", "False"},
		{"HKCU", `Software\Policies\VMware, Inc.\VMware VDM\Client`, "EnableShade", "False"},
		{"HKLM", `Software\Policies\VMware, Inc.\VMware VDM\Client`, "EnableShade", "False"},
		// Omnissa 新品牌路径（注意 Policies 也要 ...\Horizon\Client，之前少了 \Client 子键导致不生效）
		{"HKCU", `Software\Omnissa\Horizon\Client`, "EnableShade", "False"},
		{"HKLM", `Software\Omnissa\Horizon\Client`, "EnableShade", "False"},
		{"HKCU", `Software\Policies\Omnissa\Horizon\Client`, "EnableShade", "False"},
		{"HKLM", `Software\Policies\Omnissa\Horizon\Client`, "EnableShade", "False"},
		// 兜底：保留旧的 Omnissa Policies 路径写法（没有 \Client）
		{"HKCU", `Software\Policies\Omnissa\Horizon`, "EnableShade", "False"},
		{"HKLM", `Software\Policies\Omnissa\Horizon`, "EnableShade", "False"},
		// view.autoHideToolbar / view.noMenuBar 对应的偏好设置键（Horizon 文档 Configuration Keys）
		{"HKCU", `Software\Omnissa\Horizon\Client\Preferences`, "autoHideToolbar", "TRUE"},
		{"HKCU", `Software\Omnissa\Horizon\Client\Preferences`, "noMenuBar", "TRUE"},
		{"HKCU", `Software\Omnissa\Horizon\Client`, "autoHideToolbar", "TRUE"},
		{"HKCU", `Software\Omnissa\Horizon\Client`, "noMenuBar", "TRUE"},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "autoHideToolbar", "TRUE"},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "noMenuBar", "TRUE"},
		// hideClientAfterLaunchSession（对应命令行 -hideClientAfterLaunchSession=true 的偏好持久化）
		{"HKCU", `Software\Omnissa\Horizon\Client`, "hideClientAfterLaunchSession", "TRUE"},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "hideClientAfterLaunchSession", "TRUE"},
		// Shade 位置：强制在屏幕上方正中，防止被吸到左边缘卡住无法操作（Top / TopCenter / Left / Right）
		// Policies 路径优先级最高，必须写入，否则可能被 Horizon 默认行为覆盖导致卡左边缘
		{"HKCU", `Software\Policies\Omnissa\Horizon\Client`, "ShadePosition", "TopCenter"},
		{"HKLM", `Software\Policies\Omnissa\Horizon\Client`, "ShadePosition", "TopCenter"},
		{"HKCU", `Software\Policies\VMware, Inc.\VMware VDM\Client`, "ShadePosition", "TopCenter"},
		{"HKLM", `Software\Policies\VMware, Inc.\VMware VDM\Client`, "ShadePosition", "TopCenter"},
		{"HKCU", `Software\Omnissa\Horizon\Client`, "ShadePosition", "TopCenter"},
		{"HKLM", `Software\Omnissa\Horizon\Client`, "ShadePosition", "TopCenter"},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "ShadePosition", "TopCenter"},
		{"HKLM", `Software\VMware, Inc.\VMware VDM\Client`, "ShadePosition", "TopCenter"},
		// shadeShowsPinned：不固定显示，鼠标不悬停时就自动藏起来（配合隐藏开关）
		{"HKCU", `Software\Policies\Omnissa\Horizon\Client`, "shadeShowsPinned", "FALSE"},
		{"HKLM", `Software\Policies\Omnissa\Horizon\Client`, "shadeShowsPinned", "FALSE"},
		{"HKCU", `Software\Policies\VMware, Inc.\VMware VDM\Client`, "shadeShowsPinned", "FALSE"},
		{"HKLM", `Software\Policies\VMware, Inc.\VMware VDM\Client`, "shadeShowsPinned", "FALSE"},
		{"HKCU", `Software\Omnissa\Horizon\Client`, "shadeShowsPinned", "FALSE"},
		{"HKLM", `Software\Omnissa\Horizon\Client`, "shadeShowsPinned", "FALSE"},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "shadeShowsPinned", "FALSE"},
		{"HKLM", `Software\VMware, Inc.\VMware VDM\Client`, "shadeShowsPinned", "FALSE"},
	}
	for _, k := range strKeys {
		_ = writeRegistryString(k.hive, k.path, k.name, k.value)
	}

	// ---------- DWORD (数值) 键 ----------
	// 有些 Horizon 版本/构建实际读 DWORD（0=关 1=开），与 REG_SZ 一起双写保证命中
	dwordKeys := []struct {
		hive  string
		path  string
		name  string
		value uint32
	}{
		// EnableShade = 0 (禁用 shade)
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "EnableShade", 0},
		{"HKLM", `Software\VMware, Inc.\VMware VDM\Client`, "EnableShade", 0},
		{"HKCU", `Software\Policies\VMware, Inc.\VMware VDM\Client`, "EnableShade", 0},
		{"HKLM", `Software\Policies\VMware, Inc.\VMware VDM\Client`, "EnableShade", 0},
		{"HKCU", `Software\Omnissa\Horizon\Client`, "EnableShade", 0},
		{"HKLM", `Software\Omnissa\Horizon\Client`, "EnableShade", 0},
		{"HKCU", `Software\Policies\Omnissa\Horizon\Client`, "EnableShade", 0},
		{"HKLM", `Software\Policies\Omnissa\Horizon\Client`, "EnableShade", 0},
		{"HKCU", `Software\Policies\Omnissa\Horizon`, "EnableShade", 0},
		{"HKLM", `Software\Policies\Omnissa\Horizon`, "EnableShade", 0},
		// noMenuBar = 1 (禁用菜单栏)
		{"HKCU", `Software\Omnissa\Horizon\Client`, "noMenuBar", 1},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "noMenuBar", 1},
		{"HKCU", `Software\Omnissa\Horizon\Client\Preferences`, "noMenuBar", 1},
		// autoHideToolbar = 1 (自动隐藏工具栏)
		{"HKCU", `Software\Omnissa\Horizon\Client`, "autoHideToolbar", 1},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "autoHideToolbar", 1},
		{"HKCU", `Software\Omnissa\Horizon\Client\Preferences`, "autoHideToolbar", 1},
		// hideClientAfterLaunchSession = 1
		{"HKCU", `Software\Omnissa\Horizon\Client`, "hideClientAfterLaunchSession", 1},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "hideClientAfterLaunchSession", 1},
		// shadeShowsPinned = 0 (不固定)
		{"HKCU", `Software\Policies\Omnissa\Horizon\Client`, "shadeShowsPinned", 0},
		{"HKLM", `Software\Policies\Omnissa\Horizon\Client`, "shadeShowsPinned", 0},
		{"HKCU", `Software\Policies\VMware, Inc.\VMware VDM\Client`, "shadeShowsPinned", 0},
		{"HKLM", `Software\Policies\VMware, Inc.\VMware VDM\Client`, "shadeShowsPinned", 0},
		{"HKCU", `Software\Omnissa\Horizon\Client`, "shadeShowsPinned", 0},
		{"HKLM", `Software\Omnissa\Horizon\Client`, "shadeShowsPinned", 0},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "shadeShowsPinned", 0},
		{"HKLM", `Software\VMware, Inc.\VMware VDM\Client`, "shadeShowsPinned", 0},
		// ShadePosition DWORD 兜底（某些版本可能读 DWORD 形式的位置编码）
		{"HKCU", `Software\Policies\Omnissa\Horizon\Client`, "ShadePosition", 1},
		{"HKLM", `Software\Policies\Omnissa\Horizon\Client`, "ShadePosition", 1},
		{"HKCU", `Software\Policies\VMware, Inc.\VMware VDM\Client`, "ShadePosition", 1},
		{"HKLM", `Software\Policies\VMware, Inc.\VMware VDM\Client`, "ShadePosition", 1},
		{"HKCU", `Software\Omnissa\Horizon\Client`, "ShadePosition", 1},
		{"HKLM", `Software\Omnissa\Horizon\Client`, "ShadePosition", 1},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "ShadePosition", 1},
		{"HKLM", `Software\VMware, Inc.\VMware VDM\Client`, "ShadePosition", 1},
	}
	for _, k := range dwordKeys {
		_ = writeRegistryDWORD(k.hive, k.path, k.name, k.value)
	}

	// 同时禁用 Windows 11 的边缘手势/Snap（防止点击屏幕左右边缘触发系统面板导致 Horizon 失焦退出，
	// 也避免 shade 的位置被 Windows 吸附逻辑强制拉到左边缘）
	disableWindowsEdgeGestures()
}

// applyHorizonShadeShow 恢复 Horizon 全屏悬浮菜单（Shade）—— applyHorizonShadeHide 的逆操作。
// 将 EnableShade 写回 True、autoHideToolbar/noMenuBar 写回 FALSE，并恢复 Windows 边缘手势，
// 保证「隐藏悬浮菜单」开关可以自由打开/关闭（best-effort，失败忽略）。
func applyHorizonShadeShow() {
	// ---------- REG_SZ 恢复 ----------
	strKeys := []struct {
		hive  string
		path  string
		name  string
		value string
	}{
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "EnableShade", "True"},
		{"HKLM", `Software\VMware, Inc.\VMware VDM\Client`, "EnableShade", "True"},
		{"HKCU", `Software\Policies\VMware, Inc.\VMware VDM\Client`, "EnableShade", "True"},
		{"HKLM", `Software\Policies\VMware, Inc.\VMware VDM\Client`, "EnableShade", "True"},
		{"HKCU", `Software\Omnissa\Horizon\Client`, "EnableShade", "True"},
		{"HKLM", `Software\Omnissa\Horizon\Client`, "EnableShade", "True"},
		{"HKCU", `Software\Policies\Omnissa\Horizon\Client`, "EnableShade", "True"},
		{"HKLM", `Software\Policies\Omnissa\Horizon\Client`, "EnableShade", "True"},
		{"HKCU", `Software\Policies\Omnissa\Horizon`, "EnableShade", "True"},
		{"HKLM", `Software\Policies\Omnissa\Horizon`, "EnableShade", "True"},
		{"HKCU", `Software\Omnissa\Horizon\Client\Preferences`, "autoHideToolbar", "FALSE"},
		{"HKCU", `Software\Omnissa\Horizon\Client\Preferences`, "noMenuBar", "FALSE"},
		{"HKCU", `Software\Omnissa\Horizon\Client`, "autoHideToolbar", "FALSE"},
		{"HKCU", `Software\Omnissa\Horizon\Client`, "noMenuBar", "FALSE"},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "autoHideToolbar", "FALSE"},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "noMenuBar", "FALSE"},
		{"HKCU", `Software\Omnissa\Horizon\Client`, "hideClientAfterLaunchSession", "FALSE"},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "hideClientAfterLaunchSession", "FALSE"},
		{"HKCU", `Software\Omnissa\Horizon\Client`, "shadeShowsPinned", "TRUE"},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "shadeShowsPinned", "TRUE"},
	}
	for _, k := range strKeys {
		_ = writeRegistryString(k.hive, k.path, k.name, k.value)
	}

	// ---------- DWORD 恢复 ----------
	dwordKeys := []struct {
		hive  string
		path  string
		name  string
		value uint32
	}{
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "EnableShade", 1},
		{"HKLM", `Software\VMware, Inc.\VMware VDM\Client`, "EnableShade", 1},
		{"HKCU", `Software\Policies\VMware, Inc.\VMware VDM\Client`, "EnableShade", 1},
		{"HKLM", `Software\Policies\VMware, Inc.\VMware VDM\Client`, "EnableShade", 1},
		{"HKCU", `Software\Omnissa\Horizon\Client`, "EnableShade", 1},
		{"HKLM", `Software\Omnissa\Horizon\Client`, "EnableShade", 1},
		{"HKCU", `Software\Policies\Omnissa\Horizon\Client`, "EnableShade", 1},
		{"HKLM", `Software\Policies\Omnissa\Horizon\Client`, "EnableShade", 1},
		{"HKCU", `Software\Policies\Omnissa\Horizon`, "EnableShade", 1},
		{"HKLM", `Software\Policies\Omnissa\Horizon`, "EnableShade", 1},
		{"HKCU", `Software\Omnissa\Horizon\Client`, "noMenuBar", 0},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "noMenuBar", 0},
		{"HKCU", `Software\Omnissa\Horizon\Client\Preferences`, "noMenuBar", 0},
		{"HKCU", `Software\Omnissa\Horizon\Client`, "autoHideToolbar", 0},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "autoHideToolbar", 0},
		{"HKCU", `Software\Omnissa\Horizon\Client\Preferences`, "autoHideToolbar", 0},
		{"HKCU", `Software\Omnissa\Horizon\Client`, "hideClientAfterLaunchSession", 0},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "hideClientAfterLaunchSession", 0},
		{"HKCU", `Software\Omnissa\Horizon\Client`, "shadeShowsPinned", 1},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "shadeShowsPinned", 1},
	}
	for _, k := range dwordKeys {
		_ = writeRegistryDWORD(k.hive, k.path, k.name, k.value)
	}

	// 恢复 Windows 边缘手势/Snap（与 disableWindowsEdgeGestures 对应）
	restoreWindowsEdgeGestures()
}

// restoreWindowsEdgeGestures 恢复 disableWindowsEdgeGestures 修改的系统设置（写回 Windows 默认值）
func restoreWindowsEdgeGestures() {
	defer func() { _ = recover() }()
	_ = writeRegistryDWORD("HKCU", `SOFTWARE\Policies\Microsoft\Windows\EdgeUI`, "AllowEdgeSwipe", 1)
	_ = writeRegistryDWORD("HKLM", `SOFTWARE\Policies\Microsoft\Windows\EdgeUI`, "AllowEdgeSwipe", 1)
	_ = writeRegistryString("HKCU", `Control Panel\Desktop`, "WindowArrangementActive", "1")
	_ = writeRegistryDWORD("HKCU", `Software\Microsoft\Windows\CurrentVersion\ImmersiveShell\EdgeUi`, "DisableTLcorner", 0)
	_ = writeRegistryDWORD("HKCU", `Software\Microsoft\Windows\CurrentVersion\ImmersiveShell\EdgeUi`, "DisableTRcorner", 0)
	_ = writeRegistryDWORD("HKCU", `Software\Microsoft\Windows\CurrentVersion\ImmersiveShell\EdgeUi`, "DisableBLcorner", 0)
	_ = writeRegistryDWORD("HKCU", `Software\Microsoft\Windows\CurrentVersion\ImmersiveShell\EdgeUi`, "DisableBRcorner", 0)
}

// applyHorizonIgnoreCert 让 Horizon Client 不验证服务器证书（解决自签名证书/CN mismatch 导致"证书无效/无法连接"）。
// 关键注册表项:
//
//	CertHashCheckMode=0  → 不验证证书哈希（主要项）
//	CertCheckMode=0     → 不执行证书检查（覆盖任何其他设置）
//	SecurityMode=0      → 禁用安全模式验证
//	IgnoreCertErrors=TRUE → 忽略证书错误（字符串）
//	SecureConnect=FALSE  → 禁用安全连接
//
// ⚠️ 重要：HKCU 键优先写入（不需要管理员权限，任何机器都能生效）
//
//	HKLM 键作为增强（需要管理员权限，失败不影响）
func applyHorizonIgnoreCert() {
	// 需要写入 0 的 DWORD 键（HKCU 在前，HKLM 在后）
	dwordKeys := []struct {
		hive string
		path string
		name string
	}{
		// CertHashCheckMode (0=Don't verify) - HKCU 优先
		{"HKCU", `Software\Omnissa\Horizon\Client`, "CertHashCheckMode"},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "CertHashCheckMode"},
		{"HKLM", `Software\Policies\Omnissa\Horizon\Client`, "CertHashCheckMode"},
		{"HKLM", `Software\Omnissa\Horizon\Client`, "CertHashCheckMode"},
		{"HKLM", `Software\Policies\VMware, Inc.\VMware VDM\Client`, "CertHashCheckMode"},
		{"HKLM", `Software\VMware, Inc.\VMware VDM\Client`, "CertHashCheckMode"},
		// CertCheckMode (0=Don't verify) - HKCU 优先
		{"HKCU", `Software\Omnissa\Horizon\Client`, "CertCheckMode"},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "CertCheckMode"},
		{"HKLM", `Software\Policies\Omnissa\Horizon\Client`, "CertCheckMode"},
		{"HKLM", `Software\Omnissa\Horizon\Client`, "CertCheckMode"},
		{"HKLM", `Software\Policies\VMware, Inc.\VMware VDM\Client`, "CertCheckMode"},
		{"HKLM", `Software\VMware, Inc.\VMware VDM\Client`, "CertCheckMode"},
		// SecurityMode (0=Disabled) - HKCU 优先
		{"HKCU", `Software\Omnissa\Horizon\Client`, "SecurityMode"},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "SecurityMode"},
		{"HKLM", `Software\Policies\Omnissa\Horizon\Client`, "SecurityMode"},
		{"HKLM", `Software\Omnissa\Horizon\Client`, "SecurityMode"},
		{"HKLM", `Software\Policies\VMware, Inc.\VMware VDM\Client`, "SecurityMode"},
		{"HKLM", `Software\VMware, Inc.\VMware VDM\Client`, "SecurityMode"},
	}
	for _, k := range dwordKeys {
		_ = writeRegistryDWORD(k.hive, k.path, k.name, 0)
	}
	// 字符串键（HKCU 在前）
	strKeys := []struct {
		hive  string
		path  string
		name  string
		value string
	}{
		// IgnoreCertErrors=TRUE（HKCU 优先）
		{"HKCU", `Software\Omnissa\Horizon\Client`, "IgnoreCertErrors", "TRUE"},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "IgnoreCertErrors", "TRUE"},
		{"HKLM", `Software\Omnissa\Horizon\Client`, "IgnoreCertErrors", "TRUE"},
		{"HKLM", `Software\VMware, Inc.\VMware VDM\Client`, "IgnoreCertErrors", "TRUE"},
		// SecureConnect=FALSE（HKCU 优先）
		{"HKCU", `Software\Omnissa\Horizon\Client`, "SecureConnect", "FALSE"},
		{"HKCU", `Software\VMware, Inc.\VMware VDM\Client`, "SecureConnect", "FALSE"},
		{"HKLM", `Software\Omnissa\Horizon\Client`, "SecureConnect", "FALSE"},
		{"HKLM", `Software\VMware, Inc.\VMware VDM\Client`, "SecureConnect", "FALSE"},
	}
	for _, k := range strKeys {
		_ = writeRegistryString(k.hive, k.path, k.name, k.value)
	}
}

// disableWindowsEdgeGestures 禁用 Windows 11 的边缘手势和窗口 Snap 功能。
// 根因：Horizon Client 全屏模式下，鼠标点击屏幕左右边缘 → Windows 11 触发
//   - 左边缘：Widgets 面板
//   - 右边缘：通知中心/Action Center
//   - 顶部边缘：Charms Bar（Win10）或 App 标题栏
//   - 任何边缘：窗口 Snap（拖动到边缘自动排列）
//
// 这些系统面板弹出会导致 Horizon Client 失焦 → 自动退出全屏 → 回到本地桌面。
// 解决：禁用所有边缘手势 + Snap，让 Horizon 全屏独占整个屏幕。
// ⚠️ HKCU 优先（不需要管理员权限），HKLM 作为增强
func disableWindowsEdgeGestures() {
	defer func() { _ = recover() }()

	// 1) 禁用 Edge Swipe（边缘滑动手势）
	// HKCU\SOFTWARE\Policies\Microsoft\Windows\EdgeUI\AllowEdgeSwipe = 0 (DWORD)
	_ = writeRegistryDWORD("HKCU", `SOFTWARE\Policies\Microsoft\Windows\EdgeUI`, "AllowEdgeSwipe", 0)
	_ = writeRegistryDWORD("HKLM", `SOFTWARE\Policies\Microsoft\Windows\EdgeUI`, "AllowEdgeSwipe", 0)

	// 2) 禁用窗口 Snap（拖动到边缘自动排列）
	// HKCU\Control Panel\Desktop\WindowArrangementActive = 0
	_ = writeRegistryString("HKCU", `Control Panel\Desktop`, "WindowArrangementActive", "0")

	// 3) 禁用 Hot Corners（角落热区：左上=Task View，右上=Charms）
	// HKCU\Software\Microsoft\Windows\CurrentVersion\ImmersiveShell\EdgeUi\
	_ = writeRegistryDWORD("HKCU", `Software\Microsoft\Windows\CurrentVersion\ImmersiveShell\EdgeUi`, "DisableTLcorner", 1)
	_ = writeRegistryDWORD("HKCU", `Software\Microsoft\Windows\CurrentVersion\ImmersiveShell\EdgeUi`, "DisableTRcorner", 1)
	_ = writeRegistryDWORD("HKCU", `Software\Microsoft\Windows\CurrentVersion\ImmersiveShell\EdgeUi`, "DisableBLcorner", 1)
	_ = writeRegistryDWORD("HKCU", `Software\Microsoft\Windows\CurrentVersion\ImmersiveShell\EdgeUi`, "DisableBRcorner", 1)

	// 4) 通知 Windows 设置已改变（让 Edge Swipe 禁用立即生效，不需重启）
	// 通过广播 WM_SETTINGCHANGE 消息
	procSendMessageTimeoutW.Call(
		uintptr(0xFFFF), // HWND_BROADCAST
		uintptr(0x001A), // WM_SETTINGCHANGE
		0, 0,
		uintptr(0x0002), // SMTO_ABORTIFHUNG
		uintptr(5000),   // 5 秒超时
		uintptr(unsafe.Pointer(new(uint32))),
	)
}

var procSendMessageTimeoutW = moduser32.NewProc("SendMessageTimeoutW")

var (
	procGetSystemMetrics    = moduser32.NewProc("GetSystemMetrics")
	procGetForegroundWindow = moduser32.NewProc("GetForegroundWindow")
	procIsIconic            = moduser32.NewProc("IsIconic")
	procGetWindowLongW      = moduser32.NewProc("GetWindowLongW")
	procSetWindowLongW      = moduser32.NewProc("SetWindowLongW")
	procSetWindowPos        = moduser32.NewProc("SetWindowPos")

	// 低级鼠标钩子相关
	procSetWindowsHookExW   = moduser32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx = moduser32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx      = moduser32.NewProc("CallNextHookEx")
	procGetMessageW         = moduser32.NewProc("GetMessageW")
	procPostThreadMessageW  = moduser32.NewProc("PostThreadMessageW")
	procGetModuleHandleW    = syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")
	procGetCurrentThreadId  = syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId")

	// Toolhelp32 快照（枚举所有进程的父/子关系，用于 collectDescendantPIDs RDP 进程树遍历）
	_modKernel32TH               = syscall.NewLazyDLL("kernel32.dll")
	procCreateToolhelp32Snapshot = _modKernel32TH.NewProc("CreateToolhelp32Snapshot")
	procProcess32First           = _modKernel32TH.NewProc("Process32FirstW")
	procProcess32Next            = _modKernel32TH.NewProc("Process32NextW")
	procCloseHandle              = _modKernel32TH.NewProc("CloseHandle")
)

const (
	SM_CXSCREEN = 0
	SM_CYSCREEN = 1
	// SetWindowPos flags
	SWP_NOSIZE       = 0x0001
	SWP_NOMOVE       = 0x0002
	SWP_NOZORDER     = 0x0004
	SWP_NOACTIVATE   = 0x0010
	SWP_SHOWWINDOW   = 0x0040
	SWP_FRAMECHANGED = 0x0020
	HWND_TOPMOST     = ^uintptr(0) // (HWND)-1
	HWND_NOTOPMOST   = ^uintptr(1) // (HWND)-2
	// Window styles
	WS_POPUP       = 0x80000000
	WS_VISIBLE     = 0x10000000
	WS_CAPTION     = 0x00C00000
	WS_THICKFRAME  = 0x00040000
	WS_MINIMIZEBOX = 0x00020000
	WS_MAXIMIZEBOX = 0x00010000
	WS_EX_TOPMOST  = 0x00000008
	// 低级鼠标钩子
	WH_MOUSE_LL    = 14
	WM_LBUTTONDOWN = 0x0201
	WM_LBUTTONUP   = 0x0202
	WM_RBUTTONDOWN = 0x0204
	WM_RBUTTONUP   = 0x0205
	WM_MBUTTONDOWN = 0x0207
	WM_QUIT        = 0x0012
)

// GetWindowLong 索引（避免负数常量溢出 uintptr）
var (
	GWL_STYLE   = int32(-16)
	GWL_EXSTYLE = int32(-20)
)

// POINT 鼠标坐标
type point struct {
	X, Y int32
}

// MSLLHOOKSTRUCT 低级鼠标钩子数据结构
type msllhookstruct struct {
	Pt          point
	MouseData   uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

// msg Windows 消息结构
type msg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

var (
	edgeMouseHook     uintptr
	edgeMouseThreadID uint32
	// 缓存屏幕分辨率，避免每次钩子回调都调用 GetSystemMetrics
	edgeScreenW int32
	edgeScreenH int32
)

// mouseLLCallback 低级鼠标钩子回调：拦截屏幕边缘点击
// ★ 只拦截左/右/下边缘，不拦截上边缘：
//    因为 Horizon 的 Shade（顶部悬浮导航条）就在屏幕顶部（高度约 32~40px），
//    如果拦截上边缘会导致 Shade 上的"最小化/关闭/设置/虚拟机名称"按钮全部无法点击。
//    同时把边缘阈值从 4px 减小到 2px，避免误伤 Shade 左侧/右侧的功能按钮。
var mouseLLCallback = syscall.NewCallback(func(code int32, wParam, lParam uintptr) uintptr {
	defer func() { _ = recover() }()
	if code < 0 {
		ret, _, _ := procCallNextHookEx.Call(edgeMouseHook, uintptr(code), wParam, lParam)
		return ret
	}
	// 只拦截鼠标按下事件（不拦截移动和释放）
	if wParam == WM_LBUTTONDOWN || wParam == WM_RBUTTONDOWN || wParam == WM_MBUTTONDOWN {
		info := (*msllhookstruct)(unsafe.Pointer(lParam))
		x := info.Pt.X
		y := info.Pt.Y
		// 只拦截左边缘 / 右边缘 / 下边缘（2px），不拦截上边缘
		// 上边缘留给 Horizon Shade 导航条使用
		const edgeThreshold = int32(2)
		if x < edgeThreshold || x > edgeScreenW-edgeThreshold ||
			y > edgeScreenH-edgeThreshold {
			// 返回非零值 = 吞掉事件，不传递给任何窗口
			return 1
		}
	}
	ret, _, _ := procCallNextHookEx.Call(edgeMouseHook, uintptr(code), wParam, lParam)
	return ret
})

// startEdgeMouseBlocker 安装全局低级鼠标钩子，拦截屏幕边缘 4px 内的所有鼠标点击。
// 根因：Horizon 全屏窗口的边缘点击会触发"退出全屏/退出会话"行为，
//
//	无论 TOPMOST 守护器还是禁用系统手势都无法阻止。
//	唯一可靠方案：在系统层（WH_MOUSE_LL）直接吞掉边缘点击事件，
//	让点击根本无法到达 Horizon 窗口。
//
// durationSec 秒后自动卸载钩子。
func startEdgeMouseBlocker(durationSec int) {
	defer func() { _ = recover() }()

	// 缓存屏幕分辨率
	cx, _, _ := procGetSystemMetrics.Call(uintptr(SM_CXSCREEN))
	cy, _, _ := procGetSystemMetrics.Call(uintptr(SM_CYSCREEN))
	edgeScreenW = int32(cx)
	edgeScreenH = int32(cy)

	// 在独立 goroutine + 消息循环中安装钩子（低级钩子需要消息循环）
	// ⚠️ 关键：必须 LockOSThread，否则 Go 运行时会在不同 OS 线程间迁移 goroutine，
	//   导致钩子和消息循环不在同一线程 → 钩子失效
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		// 获取当前线程 ID（用于退出时 PostThreadMessage）
		tid, _, _ := procGetCurrentThreadId.Call()
		edgeMouseThreadID = uint32(tid)

		// 获取模块句柄
		hMod, _, _ := procGetModuleHandleW.Call(0)

		// 安装低级鼠标钩子
		hHook, _, _ := procSetWindowsHookExW.Call(
			uintptr(WH_MOUSE_LL),
			mouseLLCallback,
			hMod,
			0, // 0 = 全局钩子（当前桌面所有线程）
		)
		if hHook == 0 {
			// 钩子安装失败，写日志
			logFile := filepath.Join(os.TempDir(), "shoukehuduan_edge_hook.log")
			_ = os.WriteFile(logFile, []byte(fmt.Sprintf(
				"[%s] SetWindowsHookEx FAILED: hHook=0 hMod=%v tid=%d screenW=%d screenH=%d\n",
				time.Now().Format("15:04:05"), hMod, tid, edgeScreenW, edgeScreenH)), 0644)
			return
		}
		edgeMouseHook = hHook

		// 钩子安装成功，写日志
		logFile := filepath.Join(os.TempDir(), "shoukehuduan_edge_hook.log")
		_ = os.WriteFile(logFile, []byte(fmt.Sprintf(
			"[%s] SetWindowsHookEx OK: hHook=%v tid=%d screenW=%d screenH=%d edgeThreshold=4px\n",
			time.Now().Format("15:04:05"), hHook, tid, edgeScreenW, edgeScreenH)), 0644)

		// 消息循环（低级钩子必须有消息循环才能工作）
		var m msg
		for {
			ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if ret == 0 || m.Message == WM_QUIT {
				break
			}
		}

		// 卸载钩子
		procUnhookWindowsHookEx.Call(edgeMouseHook)
		edgeMouseHook = 0
		// 写卸载日志
		_ = os.WriteFile(logFile, []byte(fmt.Sprintf(
			"[%s] Hook UNHOOKED (duration expired)\n",
			time.Now().Format("15:04:05"))), 0644)
	}()

	// durationSec 秒后发送 WM_QUIT 退出消息循环
	go func() {
		time.Sleep(time.Duration(durationSec) * time.Second)
		if edgeMouseThreadID != 0 {
			procPostThreadMessageW.Call(uintptr(edgeMouseThreadID), uintptr(WM_QUIT), 0, 0)
		}
	}()
}

// keepHorizonFullscreenTopmost 持续保持 Horizon 主窗口为 TOPMOST + 全屏覆盖。
// 根因：Horizon 全屏窗口在屏幕边缘可能留有 1-2px 缝隙，或被系统面板/其他窗口抢焦。
//
//	鼠标点击边缘 → 点到下层桌面/系统面板 → Horizon 失焦 → 自动退出全屏。
//
// 解决：
//  1. 设置 HWND_TOPMOST：永远在最顶层，任何窗口都盖不住它
//  2. 移除标题栏/边框（WS_CAPTION | WS_THICKFRAME）：消除边缘缝隙
//  3. SetWindowPos 全屏覆盖：窗口大小 = 屏幕全尺寸，覆盖任务栏
//  4. 持续监控：仅在窗口真正失焦时才用 AttachThreadInput 温和激活
//
// 注意：SetForegroundWindow 被 Windows 拒绝时，系统会自动让任务栏按钮闪烁
// （这是 Windows 的"请求注意"机制）。改用 AttachThreadInput 绕过此限制。
func keepHorizonFullscreenTopmost(targetPID uint32, durationSec int) {
	defer func() { _ = recover() }()
	deadline := time.Now().Add(time.Duration(durationSec) * time.Second)
	// 获取屏幕分辨率
	cx, _, _ := procGetSystemMetrics.Call(uintptr(SM_CXSCREEN))
	cy, _, _ := procGetSystemMetrics.Call(uintptr(SM_CYSCREEN))
	screenW := int32(cx)
	screenH := int32(cy)

	// 窗口样式只修改一次，避免每轮 SWP_FRAMECHANGED 导致任务栏闪烁
	styleFixed := false
	// 记录上次的窗口句柄，避免对同一个窗口重复 SetWindowPos
	lastHwnd := uintptr(0)

	for time.Now().Before(deadline) {
		// 找到 Horizon 主窗口（可能切换窗口，所以每次都重新查找）
		hwnd, ok := findMainWindowByPID(targetPID)
		if !ok || hwnd == 0 {
			time.Sleep(2000 * time.Millisecond)
			continue
		}

		// 窗口切换了 → 重置样式标记，新窗口需要重新修样式
		if hwnd != lastHwnd {
			styleFixed = false
			lastHwnd = hwnd
		}

		// 1) 只在首次或窗口切换时移除标题栏和边框 + 设置 TOPMOST + 全屏覆盖
		//    后续不再重复调用 SetWindowPos，避免触发窗口重绘和任务栏闪烁
		if !styleFixed {
			style, _, _ := procGetWindowLongW.Call(hwnd, uintptr(GWL_STYLE))
			newStyle := style &^ uintptr(WS_CAPTION|WS_THICKFRAME|WS_MINIMIZEBOX|WS_MAXIMIZEBOX)
			if newStyle != style {
				procSetWindowLongW.Call(hwnd, uintptr(GWL_STYLE), newStyle)
			}
			// 设置 TOPMOST + 全屏覆盖（仅在样式变化时用 SWP_FRAMECHANGED）
			procSetWindowPos.Call(hwnd, HWND_TOPMOST, 0, 0,
				uintptr(screenW), uintptr(screenH),
				uintptr(SWP_NOZORDER|SWP_NOACTIVATE|SWP_SHOWWINDOW|SWP_FRAMECHANGED))
			styleFixed = true
		}

		// 2) 如果窗口被最小化，恢复
		iconic, _, _ := procIsIconic.Call(hwnd)
		if iconic != 0 {
			procShowWindow.Call(hwnd, uintptr(SW_RESTORE))
		}

		// 3) 仅在窗口被其他程序抢焦时才温和激活
		//    ★ 如果前台窗口属于同一个 PID（如 Horizon 的登录弹窗/对话框），不需要抢前台
		fg, _, _ := procGetForegroundWindow.Call()
		if fg != hwnd && fg != 0 {
			// 检查前台窗口是否属于同一进程（Horizon 的子窗口/对话框）
			var fgPID uint32
			procGetWindowThreadProcessId.Call(fg, uintptr(unsafe.Pointer(&fgPID)))
			if fgPID != targetPID {
				// 前台窗口不属于 Horizon → 用 AttachThreadInput 温和激活
				// 直接 SetForegroundWindow 被 Windows 拒绝时会让任务栏按钮闪烁
				attachThreadInputAndActivate(hwnd, targetPID)
			}
		}

		// 4) 强制 Horizon Shade（导航条）位置在屏幕顶部中央
		//    防止 Horizon 内部逻辑把 Shade 重置到左边缘导致无法操作
		forceShadePosition()

		// 轮询间隔加大到 3 秒，进一步减少任务栏闪烁
		time.Sleep(3000 * time.Millisecond)
	}
}

// forceShadePosition 强制 Horizon Shade（导航条/悬浮菜单）位置在屏幕顶部中央
// Horizon Shade 窗口的类名是 "Horizon.Shade"，它可能在运行中被重置到左边缘
// 此函数每 3 秒检查一次 Shade 位置，如果不在顶部中央就强制移过去
func forceShadePosition() {
	defer func() { _ = recover() }()

	// Horizon Shade 窗口类名（Omnissa 8.x / VMware Horizon 7.x 通用）
	shadeClasses := []string{
		"Horizon.Shade",
		"VMware.Horizon.Shade",
		"Omnissa.Horizon.Shade",
	}

	cx, _, _ := procGetSystemMetrics.Call(uintptr(SM_CXSCREEN))
	screenW := int32(cx)

	for _, className := range shadeClasses {
		class16, _ := syscall.UTF16PtrFromString(className)
		hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(class16)), 0)
		if hwnd == 0 {
			continue
		}

		// 获取 Shade 当前位置
		var rect rect
		procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
		shadeW := rect.Right - rect.Left
		shadeH := rect.Bottom - rect.Top

		// Shade 高度通常是 32-40px，如果高度异常说明没找到正确窗口
		if shadeH < 20 || shadeH > 100 {
			continue
		}

		// 计算目标位置：屏幕顶部中央
		targetX := (screenW - shadeW) / 2
		targetY := int32(0)

		// 检查是否已经在正确位置（允许 5px 误差）
		if abs32(rect.Left-targetX) <= 5 && abs32(rect.Top-targetY) <= 5 {
			return // 位置正确，跳过
		}

		// 强制移动到目标位置
		procSetWindowPos.Call(hwnd, 0,
			uintptr(targetX), uintptr(targetY),
			uintptr(shadeW), uintptr(shadeH),
			uintptr(SWP_NOZORDER|SWP_NOACTIVATE|SWP_SHOWWINDOW))

		return // 成功处理一个 Shade 就返回
	}
}

// abs32 计算 int32 绝对值
func abs32(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}

// attachThreadInputAndActivate 用 AttachThreadInput 方式温和地把窗口激活到前台
// 直接调用 SetForegroundWindow 在窗口不属于当前线程时会被 Windows 拒绝，
// 拒绝后系统会让任务栏按钮闪烁（FlashWindow 效果）。
// AttachThreadInput 把目标窗口的线程输入队列附加到当前线程，
// 使 SetForegroundWindow 调用被视为"合法的前台切换"，不会被拒绝。
func attachThreadInputAndActivate(hwnd uintptr, targetPID uint32) {
	defer func() { _ = recover() }()

	// 获取当前线程 ID 和目标窗口的线程 ID
	currentThreadID, _, _ := procGetCurrentThreadId.Call()
	var targetThreadID uint32
	threadRet, _, _ := procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&targetPID)))
	targetThreadID = uint32(threadRet)

	if targetThreadID == 0 || targetThreadID == uint32(currentThreadID) {
		// 同一线程，直接 SetForegroundWindow 不会被拒绝
		procSetForegroundWindow.Call(hwnd)
		return
	}

	// AttachThreadInput：把目标窗口的线程输入队列附加到当前线程
	// 这样 SetForegroundWindow 会被视为合法的前台切换
	procAttachThreadInput.Call(currentThreadID, uintptr(targetThreadID), 1)

	// 恢复窗口（如果最小化）+ 设置前台
	iconic, _, _ := procIsIconic.Call(hwnd)
	if iconic != 0 {
		procShowWindow.Call(hwnd, uintptr(SW_RESTORE))
	}
	procSetForegroundWindow.Call(hwnd)
	procBringWindowToTop.Call(hwnd)

	// 解除 AttachThreadInput
	procAttachThreadInput.Call(currentThreadID, uintptr(targetThreadID), 0)
}

// writeRegistryDWORD 写入 DWORD 类型的注册表值
func writeRegistryDWORD(hive, path, name string, value uint32) error {
	var root syscall.Handle
	switch hive {
	case "HKCU":
		root = 0x80000001
	case "HKLM":
		root = 0x80000002
	default:
		return fmt.Errorf("unknown hive %s", hive)
	}
	var hKey syscall.Handle
	utf16Path, _ := syscall.UTF16PtrFromString(path)
	// 0xF003F = KEY_ALL_ACCESS；0x0100 = KEY_WOW64_64KEY（强制访问 64 位注册表视图）
	// 32 位进程写注册表默认会被重定向到 WOW6432Node，
	// 而 64 位 Horizon 客户端只读真正的 64 位路径，不加此标志所有注册表配置全部失效
	samDesired := uint32(0xF003F | 0x0100)
	ret, _, _ := procRegCreateKeyEx.Call(uintptr(root), uintptr(unsafe.Pointer(utf16Path)), 0, 0, 0, uintptr(samDesired), 0, uintptr(unsafe.Pointer(&hKey)), 0)
	if ret != 0 {
		return fmt.Errorf("regcreatekeyex err=%d", ret)
	}
	defer syscall.RegCloseKey(hKey)
	utf16Name, _ := syscall.UTF16PtrFromString(name)
	// REG_DWORD = 4
	ret, _, _ = procRegSetValueEx.Call(uintptr(hKey), uintptr(unsafe.Pointer(utf16Name)), 0, 4, uintptr(unsafe.Pointer(&value)), 4)
	if ret != 0 {
		return fmt.Errorf("regsetvalueex err=%d", ret)
	}
	return nil
}

func writeRegistryString(hive, path, name, value string) error {
	var root syscall.Handle
	switch hive {
	case "HKCU":
		root = 0x80000001 // HKEY_CURRENT_USER
	case "HKLM":
		root = 0x80000002 // HKEY_LOCAL_MACHINE
	default:
		return fmt.Errorf("unknown hive %s", hive)
	}
	// 创建/打开注册表路径
	var hKey syscall.Handle
	utf16Path, _ := syscall.UTF16PtrFromString(path)
	// 0xF003F = KEY_ALL_ACCESS；0x0100 = KEY_WOW64_64KEY（强制访问 64 位注册表视图）
	// 不使用 KEY_WOW64_64KEY 时 32 位进程写入会被重定向到 WOW6432Node，
	// 导致 64 位 Horizon 客户端读不到我们设置的 EnableShade/ShadePosition 等键
	samDesired := uint32(0xF003F | 0x0100)
	ret, _, _ := procRegCreateKeyEx.Call(uintptr(root), uintptr(unsafe.Pointer(utf16Path)), 0, 0, 0, uintptr(samDesired), 0, uintptr(unsafe.Pointer(&hKey)), 0)
	if ret != 0 {
		return fmt.Errorf("regcreatekeyex err=%d", ret)
	}
	defer syscall.RegCloseKey(hKey)
	utf16Name, _ := syscall.UTF16PtrFromString(name)
	utf16Value := syscall.StringToUTF16(value)
	// REG_SZ = 1
	utf16ValPtr := unsafe.Pointer(&utf16Value[0])
	valSize := uint32(len(utf16Value) * 2)
	ret, _, _ = procRegSetValueEx.Call(uintptr(hKey), uintptr(unsafe.Pointer(utf16Name)), 0, 1, uintptr(utf16ValPtr), uintptr(valSize))
	if ret != 0 {
		return fmt.Errorf("regsetvalueex err=%d", ret)
	}
	return nil
}

var (
	modadvapi32        = syscall.NewLazyDLL("advapi32.dll")
	procRegCreateKeyEx = modadvapi32.NewProc("RegCreateKeyExW")
	procRegSetValueEx  = modadvapi32.NewProc("RegSetValueExW")
)

// installCertToRoot installs a DER-encoded certificate into the Windows trusted root store using certutil.
// 先尝试导入 LocalMachine\Root（需要管理员权限），失败则回退到 CurrentUser\Root（不需要管理员权限）。
func installCertToRoot(der []byte, label string) error {
	if len(der) == 0 {
		return nil
	}
	// write temp .cer
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("shoukehuduan_horizon_cert_%s.cer", label))
	if err := os.WriteFile(tmp, der, 0644); err != nil {
		return err
	}
	defer os.Remove(tmp)

	// 先尝试 LocalMachine\Root（需要管理员权限）
	cmd := exec.Command("certutil.exe", "-addstore", "-f", "Root", tmp)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	_, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	// LocalMachine 失败（可能没有管理员权限），回退到 CurrentUser\Root
	fmt.Printf("Cert install to LM-Root failed: %v, trying CU-Root...\n", err)
	cmd2 := exec.Command("certutil.exe", "-addstore", "-f", "-user", "Root", tmp)
	cmd2.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	out2, err2 := cmd2.CombinedOutput()
	if err2 != nil {
		return fmt.Errorf("certutil addstore Root failed (LM: %v, CU: %v), out=%s", err, err2, strings.TrimSpace(string(out2)))
	}
	return nil
}

// user32.dll 函数声明
var (
	moduser32                    = syscall.NewLazyDLL("user32.dll")
	procAllowSetForegroundWindow = moduser32.NewProc("AllowSetForegroundWindow")
	procSetForegroundWindow      = moduser32.NewProc("SetForegroundWindow")
	procShowWindow               = moduser32.NewProc("ShowWindow")
	procSetFocus                 = moduser32.NewProc("SetFocus")
	procGetWindowThreadProcessId = moduser32.NewProc("GetWindowThreadProcessId")
	procEnumWindows              = moduser32.NewProc("EnumWindows")
	procEnumChildWindows         = moduser32.NewProc("EnumChildWindows")
	procIsWindowVisible          = moduser32.NewProc("IsWindowVisible")
	procGetWindowTextW           = moduser32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW     = moduser32.NewProc("GetWindowTextLengthW")
	procGetClassNameW            = moduser32.NewProc("GetClassNameW")
	procGetWindowRect            = moduser32.NewProc("GetWindowRect")
	procGetParent                = moduser32.NewProc("GetParent")
	prockeybd_event              = moduser32.NewProc("keybd_event")
	procAttachThreadInput        = moduser32.NewProc("AttachThreadInput")
	procBringWindowToTop         = moduser32.NewProc("BringWindowToTop")
	procFindWindowW              = moduser32.NewProc("FindWindowW")
)

const (
	SW_RESTORE      = 9
	SW_MAXIMIZE     = 3
	SW_HIDE         = 0
	VK_RETURN       = 0x0D
	KEYEVENTF_KEYUP = 0x0002
)

type rect struct {
	Left, Top, Right, Bottom int32
}

// shadeHuntResult 保存 Shade 窗口搜索结果
type shadeHuntResult struct {
	TargetPID uint32
	FoundHW   []uintptr
}

// enumWndResult 保存窗口枚举结果
type enumWndResult struct {
	PID   uint32
	HWND  uintptr
	Found bool
}

var enumCallback = syscall.NewCallback(func(hwnd, lParam uintptr) uintptr {
	res := (*enumWndResult)(unsafe.Pointer(lParam))
	var visible uintptr
	procIsWindowVisible.Call(hwnd)
	visible, _, _ = procIsWindowVisible.Call(hwnd)
	if visible == 0 {
		return 1
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == res.PID {
		res.HWND = hwnd
		res.Found = true
		return 0 // stop
	}
	return 1
})

// findMainWindowByPID 枚举所有窗口，找到属于指定 PID 的可见顶层窗口（主窗口）
func findMainWindowByPID(pid uint32) (uintptr, bool) {
	res := &enumWndResult{PID: pid}
	procEnumWindows.Call(uintptr(enumCallback), uintptr(unsafe.Pointer(res)))
	return res.HWND, res.Found
}

func keyPressVK(vk byte, times int, gapMs int) {
	for i := 0; i < times; i++ {
		// DOWN
		prockeybd_event.Call(uintptr(vk), 0, 0, 0)
		time.Sleep(time.Duration(60) * time.Millisecond)
		// UP
		prockeybd_event.Call(uintptr(vk), 0, KEYEVENTF_KEYUP, 0)
		time.Sleep(time.Duration(gapMs) * time.Millisecond)
	}
}

// huntAndHideShade 安全隐藏 Horizon Shade（顶部控制条/会话栏）。
// 严格限定：① 必须属于 horizon-client 进程（含子进程）；② 不在系统窗口白名单内；
// ③ 几何特征精确（宽>500、高 1-80px、Top<50、Left<50）。
// 只用 SW_HIDE，**不调用 SetWindowPos 搬移**——搬移会破坏 Horizon 边缘热区窗口导致点击边缘退出。
func huntAndHideShade(targetPID uint32, durationSec int, intervalMs int) {
	defer func() { _ = recover() }()
	deadline := time.Now().Add(time.Duration(durationSec) * time.Second)
	// 收集 horizon-client 进程及其子进程的所有 PID
	pids := collectHorizonPIDs(targetPID)
	if len(pids) == 0 {
		pids = map[uint32]bool{targetPID: true}
	}
	for time.Now().Before(deadline) {
		huntOnceSafe(pids)
		time.Sleep(time.Duration(intervalMs) * time.Millisecond)
	}
}

// collectHorizonPIDs 收集目标 PID 及其所有子进程 PID（Horizon 可能用子进程承载 Shade）
func collectHorizonPIDs(rootPID uint32) map[uint32]bool {
	result := map[uint32]bool{rootPID: true}
	// 用 WMIC 查子进程（快、不依赖外部包）
	cmd := exec.Command("wmic", "process", "where", fmt.Sprintf("ParentProcessId=%d", rootPID),
		"get", "ProcessId", "/value")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	out, err := cmd.Output()
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ProcessId=") {
			pidStr := strings.TrimPrefix(line, "ProcessId=")
			if pid, err2 := strconv.ParseUint(pidStr, 10, 32); err2 == nil {
				result[uint32(pid)] = true
			}
		}
	}
	return result
}

// collectDescendantPIDs 通用版：递归收集 rootPID 及其所有后代子进程（包括孙子/曾孙…）。
// 用于 RDP 的 ALT+F4 屏蔽：wfreerdp.exe → sdl-freerdp.exe → 可能还有 swf-protocol.exe 等，
// 必须把整个进程树里的所有窗口都去 WS_SYSMENU，才能保证用户按 ALT+F4 不会关掉 RDP 会话。
func collectDescendantPIDs(rootPID uint32) map[uint32]bool {
	result := map[uint32]bool{rootPID: true}
	// 1) 先通过 Toolhelp32 建一个全局「父PID → 子PID列表」快照（比 WMIC 快得多，900 轮不影响性能）
	snap, _, _ := procCreateToolhelp32Snapshot.Call(0x00000002, 0) // TH32CS_SNAPPROCESS = 2
	if snap == uintptr(^uint32(0)) {                               // INVALID_HANDLE_VALUE
		// Toolhelp 失败：回退到 collectHorizonPIDs（WMIC 一层结果，至少扫一层）
		return collectHorizonPIDs(rootPID)
	}
	defer func() { _, _, _ = procCloseHandle.Call(snap) }()

	type pe32 struct {
		dwSize              uint32
		cntUsage            uint32
		th32ProcessID       uint32
		th32DefaultHeapID   uintptr
		th32ModuleID        uint32
		cntThreads          uint32
		th32ParentProcessID uint32
		pcPriClassBase      int32
		dwFlags             uint32
		szExeFile           [260]uint16
	}
	var pe pe32
	pe.dwSize = uint32(unsafe.Sizeof(pe))
	parentToChildren := map[uint32][]uint32{}
	first, _, _ := procProcess32First.Call(snap, uintptr(unsafe.Pointer(&pe)))
	if first == 0 {
		return collectHorizonPIDs(rootPID)
	}
	for {
		if pe.th32ParentProcessID != pe.th32ProcessID && pe.th32ParentProcessID != 0 {
			parentToChildren[pe.th32ParentProcessID] = append(parentToChildren[pe.th32ParentProcessID], pe.th32ProcessID)
		}
		pe.dwSize = uint32(unsafe.Sizeof(pe))
		next, _, _ := procProcess32Next.Call(snap, uintptr(unsafe.Pointer(&pe)))
		if next == 0 {
			break
		}
	}
	// 2) BFS 从 rootPID 向下遍历所有后代
	queue := []uint32{rootPID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, child := range parentToChildren[cur] {
			if !result[child] {
				result[child] = true
				queue = append(queue, child)
			}
		}
	}
	return result
}

// 系统窗口类名白名单：这些类名绝对不能隐藏（哪怕几何特征匹配）
var systemClassBlacklist = []string{
	"Shell_TrayWnd",            // 任务栏
	"Shell_SecondaryTrayWnd",   // 多显示器任务栏
	"Progman",                  // 桌面
	"WorkerW",                  // 桌面壁纸层
	"Button",                   // 开始按钮
	"IME",                      // 输入法
	"MSCTFIME UI",              // 输入法 UI
	"tooltips_class32",         // 工具提示
	"NotifyIconOverflowWindow", // 托盘溢出
}

func isSystemWindow(hwnd uintptr) bool {
	var cn [256]uint16
	ret, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&cn[0])), uintptr(len(cn)))
	if ret == 0 {
		return false
	}
	cls := syscall.UTF16ToString(cn[:])
	for _, black := range systemClassBlacklist {
		if strings.EqualFold(cls, black) {
			return true
		}
	}
	return false
}

// isShadeByGeometry 严格几何判断：必须是屏幕顶部的薄条
func isShadeByGeometry(r *rect) bool {
	w := r.Right - r.Left
	h := r.Bottom - r.Top
	// 宽度必须>500px（Shade 通常覆盖屏幕宽度的大部分）
	if w < 500 {
		return false
	}
	// 高度 1-80px（Shade 展开最厚约 60px，收缩 1-3px）
	if h < 1 || h > 80 {
		return false
	}
	// 必须贴着屏幕顶部（Top<50px）和左边缘（Left<50px）
	if r.Top > 50 || r.Top < -10 {
		return false
	}
	if r.Left > 50 || r.Left < -10 {
		return false
	}
	return true
}

// shadeSafeEnumCB 安全枚举回调：只隐藏 horizon-client 进程的、非系统窗口的、几何匹配的窗口
var shadeSafeEnumCB = syscall.NewCallback(func(hwnd, lParam uintptr) uintptr {
	defer func() { _ = recover() }()
	// 必须可见
	vis, _, _ := procIsWindowVisible.Call(hwnd)
	if vis == 0 {
		return 1
	}
	// 必须是 horizon-client 进程
	pids := *(*map[uint32]bool)(unsafe.Pointer(lParam))
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if !pids[pid] {
		return 1
	}
	// 排除系统窗口
	if isSystemWindow(hwnd) {
		return 1
	}
	// 几何特征匹配
	var r rect
	ret, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	if ret == 0 {
		return 1
	}
	if !isShadeByGeometry(&r) {
		return 1
	}
	// 只 SW_HIDE，**不要 SetWindowPos 搬移**——搬移会破坏边缘热区窗口导致点击边缘退出
	procShowWindow.Call(hwnd, uintptr(SW_HIDE))
	return 1
})

var shadeSafeChildCB = syscall.NewCallback(func(hwnd, lParam uintptr) uintptr {
	defer func() { _ = recover() }()
	vis, _, _ := procIsWindowVisible.Call(hwnd)
	if vis == 0 {
		return 1
	}
	pids := *(*map[uint32]bool)(unsafe.Pointer(lParam))
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if !pids[pid] {
		return 1
	}
	if isSystemWindow(hwnd) {
		return 1
	}
	var r rect
	ret, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	if ret == 0 {
		return 1
	}
	if !isShadeByGeometry(&r) {
		return 1
	}
	procShowWindow.Call(hwnd, uintptr(SW_HIDE))
	return 1
})

func huntOnceSafe(pids map[uint32]bool) {
	defer func() { _ = recover() }()
	pidsPtr := uintptr(unsafe.Pointer(&pids))
	// 顶层窗口
	procEnumWindows.Call(uintptr(shadeSafeEnumCB), pidsPtr)
	// 子窗口（Shade 可能是主窗口的子窗口）
	var collectChild = syscall.NewCallback(func(hwnd, lParam uintptr) uintptr {
		procEnumChildWindows.Call(hwnd, uintptr(shadeSafeChildCB), lParam)
		return 1
	})
	procEnumWindows.Call(uintptr(collectChild), pidsPtr)
}

// runHorizonSendKeys 无人值守自动登录（高性能版）：
// 1) 以 100ms 间隔主动轮询 Horizon 主窗口，一出现就立即处理（不再固定 3 秒死等）
// 2) AllowSetForegroundWindow + ShowWindow(SW_RESTORE) + SetForegroundWindow 强制激活
// 3) 立即多发 VK_RETURN（登录按钮 = 默认按钮 → ENTER 触发；后续桌面池 ENTER 确认）
// 4) 各阶段等待也缩短，整体流程从 ~8 秒降到 < 2 秒
func runHorizonSendKeys(processID int, waitSeconds int) {
	go func() {
		defer func() { _ = recover() }()
		const VK_CONTROL = 0x11
		const VK_MENU = 0x12 // ALT
		pid := uint32(processID)
		_ = VK_CONTROL
		_ = VK_MENU
		// 阶段0: 立即启动「边缘鼠标拦截器」（全局低级鼠标钩子）
		startEdgeMouseBlocker(600)

		// 阶段1: 轮询等待登录窗口出现（100ms 间隔，最多 8 秒）
		// ★ 之前固定 waitSeconds=3 秒导致用户看到「页面停留好几秒才点登录」
		//   现改为窗口就绪即立即触发（通常 600~900ms 就出现，相比之前快 2 秒以上）
		var hwnd uintptr = 0
		deadline := time.Now().Add(8 * time.Second)
		// 先兼容老接口的 waitSeconds：如果传入了旧值，先 sleep 一次兜底
		if waitSeconds > 0 {
			time.Sleep(time.Duration(waitSeconds) * time.Second)
		}
		for time.Now().Before(deadline) {
			var ok bool
			hwnd, ok = findMainWindowByPID(pid)
			if ok && hwnd != 0 {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if hwnd == 0 {
			return
		}

		// 阶段2: 强制激活（各步骤间隔压缩为原时长的 ~1/2）
		procAllowSetForegroundWindow.Call(uintptr(pid))
		time.Sleep(30 * time.Millisecond)
		procShowWindow.Call(hwnd, uintptr(SW_RESTORE))
		time.Sleep(30 * time.Millisecond)
		procSetForegroundWindow.Call(hwnd)
		time.Sleep(60 * time.Millisecond)
		procSetFocus.Call(hwnd)
		time.Sleep(40 * time.Millisecond)

		// 阶段3: 第1轮 ENTER（登录按钮）—— 4 次 × 150ms（原 300ms，快一倍）
		keyPressVK(VK_RETURN, 4, 150)

		// 阶段4: 1 秒后第2轮 ENTER（桌面池选择 / 后续弹窗）—— 原 2.5s → 1.0s
		time.Sleep(1000 * time.Millisecond)
		if hwnd2, ok2 := findMainWindowByPID(pid); ok2 && hwnd2 != 0 {
			procAllowSetForegroundWindow.Call(uintptr(pid))
			procSetForegroundWindow.Call(hwnd2)
			time.Sleep(60 * time.Millisecond)
		}
		keyPressVK(VK_RETURN, 4, 150)

		// 阶段5: 再 1 秒后第3轮 ENTER（保险兜底）—— 原 2s → 1.0s
		time.Sleep(1000 * time.Millisecond)
		keyPressVK(VK_RETURN, 2, 250)

		// 阶段7: 启动「TOPMOST 全屏守护器」—— 持续保持 Horizon 窗口置顶 + 全屏覆盖
		keepHorizonFullscreenTopmost(pid, 300)

		// ========== 阶段8：ALT+F4 / X 屏蔽（用户不允许退出远程会话） ==========
		go func() {
			defer func() { _ = recover() }()
			for i := 0; i < 40; i++ {
				_ = removeSysMenuForPid(uint32(pid))
				time.Sleep(500 * time.Millisecond)
			}
		}()
	}()
}

// forceForegroundWindow 强制把指定窗口激活到前台（覆盖在所有窗口上面）
// 用于 FreeRDP/Horizon 启动后把全屏窗口激活到前台，防止卡在任务栏
func forceForegroundWindow(hwnd uintptr) {
	defer func() { _ = recover() }()
	if hwnd == 0 {
		return
	}
	// 如果窗口最小化了，先恢复（SW_RESTORE = 9）
	iconic, _, _ := procIsIconicFG.Call(hwnd)
	if iconic != 0 {
		procShowWindowFG.Call(hwnd, 9)
	}
	// 尝试多种方式激活到前台（Windows 限制前台窗口权限，需要用 AttachThreadInput 绕过）
	// 方法1：直接 SetForegroundWindow
	procSetForegroundWindowFG.Call(hwnd)
	// 方法2：BringWindowToTop
	procBringWindowToTopFG.Call(hwnd)
	// 方法3：ShowWindow(SW_SHOW) + SetForegroundWindow
	procShowWindowFG.Call(hwnd, 5) // SW_SHOW = 5
	procSetForegroundWindowFG.Call(hwnd)
}

var (
	procSetForegroundWindowFG = moduser32.NewProc("SetForegroundWindow")
	procBringWindowToTopFG    = moduser32.NewProc("BringWindowToTop")
	procShowWindowFG          = moduser32.NewProc("ShowWindow")
	procIsIconicFG            = moduser32.NewProc("IsIconic")
)

// removeSysMenuForPid 枚举指定 pid 的所有可见顶层 HWND → 移除 WS_SYSMENU / WS_MINIMIZEBOX / WS_MAXIMIZEBOX
// 返回是否至少处理了一个窗口。
// 效果：ALT+F4 / 标题栏 X / 右键标题栏"关闭"全部失效（因为 WM_SYSCOMMAND/SC_CLOSE 需要 WS_SYSMENU 才会触发）
func removeSysMenuForPid(pid uint32) bool {
	type ctxT struct {
		pid  uint32
		done bool
	}
	ctx := &ctxT{pid: pid}
	cb := syscall.NewCallback(func(hwnd, lparam uintptr) uintptr {
		c := (*ctxT)(unsafe.Pointer(lparam))
		var outPid uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&outPid)))
		if outPid != c.pid {
			return 1
		}
		vis, _, _ := procIsWindowVisible.Call(hwnd)
		if vis == 0 {
			return 1
		}
		parent, _, _ := procGetParent.Call(hwnd)
		if parent != 0 {
			return 1
		}
		// 移除关闭/最小化/最大化按钮 + 系统菜单
		gwlStyle := uintptr(^uint32(15)) // -16 = GWL_STYLE
		style, _, _ := procGetWindowLongW.Call(hwnd, gwlStyle)
		// ★ 窗口化模式（有标题栏 WS_CAPTION）跳过剥离：
		//   用户通过 floatbar 切到窗口化后，窗口需要完整系统菜单才能正常最小化/还原；
		//   此时剥离 WS_SYSMENU 会损坏 wfreerdp 的窗口化状态机（最小化后无法从任务栏恢复）。
		//   全屏时 FreeRDP 是无边框 popup 窗口（无标题栏），剥离照常生效，不影响屏蔽 Alt+F4。
		const wsCaption = 0x00C00000
		if style&wsCaption == wsCaption {
			return 1
		}
		const (
			wsSysMenu     = 0x00080000
			wsMinimizeBox = 0x00020000
			wsMaximizeBox = 0x00010000
		)
		newStyle := style &^ (wsSysMenu | wsMinimizeBox | wsMaximizeBox)
		if newStyle != style {
			procSetWindowLongW.Call(hwnd, gwlStyle, newStyle)
			const (
				swpNoZOrder     = 0x0004
				swpNoActivate   = 0x0010
				swpShowWindow   = 0x0040
				swpFrameChanged = 0x0020
			)
			procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0,
				swpNoZOrder|swpNoActivate|swpShowWindow|swpFrameChanged)
		}
		c.done = true
		return 1
	})
	procEnumWindows.Call(cb, uintptr(unsafe.Pointer(ctx)))
	return ctx.done
}

// hasVisibleDesktopWindow 检查指定 PID（含所有子进程）是否拥有至少一个
// 「可见且尺寸非零」的顶层窗口。用于区分 FreeRDP「真正连上桌面」和「进程活着但卡住」。
//
//	场景：runOnce 4 秒超时返回 9999 时，进程可能已经连上（窗口有尺寸），
//	也可能卡在 TLS/NLA 协商（窗口 0x0）。用这个函数判断是否真正成功。
func hasVisibleDesktopWindow(rootPID uint32) bool {
	type ctxT struct {
		pid   map[uint32]bool
		found bool
	}
	allPIDs := collectDescendantPIDs(rootPID)
	ctx := &ctxT{pid: allPIDs}
	cb := syscall.NewCallback(func(hwnd, lparam uintptr) uintptr {
		c := (*ctxT)(unsafe.Pointer(lparam))
		var outPid uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&outPid)))
		if !c.pid[outPid] {
			return 1
		}
		vis, _, _ := procIsWindowVisible.Call(hwnd)
		if vis == 0 {
			return 1
		}
		parent, _, _ := procGetParent.Call(hwnd)
		if parent != 0 {
			return 1
		}
		var r rect
		ret, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
		if ret == 0 {
			return 1
		}
		w := r.Right - r.Left
		h := r.Bottom - r.Top
		// 尺寸必须 > 50x50（过滤掉 0x0 的 SDL 初始化占位窗口）
		if w > 50 && h > 50 {
			c.found = true
			return 0 // 找到了，停止枚举
		}
		return 1
	})
	procEnumWindows.Call(cb, uintptr(unsafe.Pointer(ctx)))
	return ctx.found
}

// activateSessionWindow 恢复并置顶指定进程树的远程桌面窗口。
// 背景：sdl-freerdp 的桌面窗口要等整个协议协商完成后才创建（比 wfreerdp 晚数秒），
// 此时本软件主窗口的前台抢占时机已过（Windows 前台锁定机制），新窗口只能开在后台/
// 最小化在任务栏里闪，用户看到的就是「先最小化到任务栏，等好几秒才进入」。
// 对策：AllowSetForegroundWindow 授权 + IsIconic→SW_RESTORE + TOPMOST 闪烁 + SetForegroundWindow。
// 返回是否找到了该进程树的可激活顶层窗口。
func activateSessionWindow(rootPID uint32) bool {
	allPIDs := collectDescendantPIDs(rootPID)
	allPIDs[rootPID] = true
	found := false
	cb := syscall.NewCallback(func(hwnd, lparam uintptr) uintptr {
		var outPid uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&outPid)))
		if !allPIDs[outPid] {
			return 1
		}
		vis, _, _ := procIsWindowVisible.Call(hwnd)
		if vis == 0 {
			return 1
		}
		parent, _, _ := procGetParent.Call(hwnd)
		if parent != 0 {
			return 1
		}
		// 最小化（躺在任务栏）→ 立即恢复
		iconic, _, _ := procIsIconic.Call(hwnd)
		if iconic != 0 {
			procShowWindow.Call(hwnd, uintptr(SW_RESTORE))
		}
		// 前台抢占三连：授权目标进程置前 → TOPMOST 闪烁打破 z-order → 设为前台
		procAllowSetForegroundWindow.Call(uintptr(outPid))
		procSetWindowPos.Call(hwnd, HWND_TOPMOST, 0, 0, 0, 0,
			uintptr(SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE))
		procSetWindowPos.Call(hwnd, HWND_NOTOPMOST, 0, 0, 0, 0,
			uintptr(SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE))
		procSetForegroundWindow.Call(hwnd)
		found = true
		return 1 // 继续枚举（多显示器下可能有多个顶层窗口，全部激活无害）
	})
	procEnumWindows.Call(cb, 0)
	return found
}

// restoreIconicSessionWindows 仅恢复「被最小化到任务栏」的会话窗口（SW_RESTORE + 拉前台）。
// 与 activateSessionWindow 的区别：不做 TOPMOST 闪烁、不碰正常显示的窗口。
// 被最小化的窗口必然已完成创建，因此在 SDL 建窗早期阶段调用也是安全的。
func restoreIconicSessionWindows(rootPID uint32) bool {
	allPIDs := collectDescendantPIDs(rootPID)
	allPIDs[rootPID] = true
	found := false
	cb := syscall.NewCallback(func(hwnd, lparam uintptr) uintptr {
		var outPid uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&outPid)))
		if !allPIDs[outPid] {
			return 1
		}
		parent, _, _ := procGetParent.Call(hwnd)
		if parent != 0 {
			return 1
		}
		// 只处理最小化窗口（IsWindowVisible 对最小化窗口也返回 true，无需再查）
		iconic, _, _ := procIsIconic.Call(hwnd)
		if iconic == 0 {
			return 1
		}
		procShowWindow.Call(hwnd, uintptr(SW_RESTORE))
		procAllowSetForegroundWindow.Call(uintptr(outPid))
		procSetForegroundWindow.Call(hwnd)
		found = true
		return 1 // 继续枚举（多显示器下可能有多个顶层窗口）
	})
	procEnumWindows.Call(cb, 0)
	return found
}

// getBundledDotNetRuntimePath 返回项目自带的 .NET 运行时路径（如果存在）
// 优先使用项目目录下的 dependencies/dotnet-runtime-8.0/
// 返回空字符串表示没有自带运行时，需要使用系统运行时
func getBundledDotNetRuntimePath() string {
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}
	baseDir := filepath.Dir(exePath)
	bundledPath := filepath.Join(baseDir, "dependencies", "dotnet-runtime-8.0", "shared")
	info, err := os.Stat(bundledPath)
	if err != nil || !info.IsDir() {
		return ""
	}
	// 检查是否有足够的组件
	required := []string{"Microsoft.NETCore.App", "Microsoft.WindowsDesktop.App", "Microsoft.AspNetCore.App"}
	count := 0
	for _, req := range required {
		if _, err := os.Stat(filepath.Join(bundledPath, req)); err == nil {
			count++
		}
	}
	if count >= 2 {
		return filepath.Join(baseDir, "dependencies", "dotnet-runtime-8.0")
	}
	return ""
}

// checkDotNet8DesktopRuntime 验证 .NET 8.0 完整运行时是否已安装
// 优先检查项目自带的运行时，然后检查系统运行时
// Horizon 客户端需要三个组件：
//   1. Microsoft.NETCore.App (基础运行时)
//   2. Microsoft.WindowsDesktop.App (WPF/WinForms)
//   3. Microsoft.AspNetCore.App (ASP.NET Core)
func checkDotNet8DesktopRuntime() (bool, string) {
	// 1. 优先检查项目自带的运行时
	if bundledPath := getBundledDotNetRuntimePath(); bundledPath != "" {
		bundledShared := filepath.Join(bundledPath, "shared")
		missing := []string{}
		found := []string{}
		required := []string{
			"Microsoft.NETCore.App",
			"Microsoft.WindowsDesktop.App",
			"Microsoft.AspNetCore.App",
		}
		for _, req := range required {
			componentPath := filepath.Join(bundledShared, req)
			info, err := os.Stat(componentPath)
			if err != nil || !info.IsDir() {
				missing = append(missing, req)
				continue
			}
			entries, err := os.ReadDir(componentPath)
			if err != nil || len(entries) == 0 {
				missing = append(missing, req)
				continue
			}
			versionDir := ""
			for _, e := range entries {
				if e.IsDir() && strings.HasPrefix(e.Name(), "8.0") {
					versionDir = filepath.Join(componentPath, e.Name())
					break
				}
			}
			if versionDir != "" {
				found = append(found, fmt.Sprintf("%s=%s", req, versionDir))
			} else {
				missing = append(missing, req)
			}
		}
		if len(missing) == 0 {
			return true, fmt.Sprintf("使用项目自带运行时: %v", found)
		}
		// 项目自带的不完整，继续检查系统的
	}

	// 2. 检查系统运行时
	roots := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "dotnet", "shared"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "dotnet", "shared"),
	}

	required := []string{
		"Microsoft.NETCore.App",
		"Microsoft.WindowsDesktop.App",
		"Microsoft.AspNetCore.App",
	}

	missing := []string{}
	found := []string{}

	for _, req := range required {
		foundDir := ""
		for _, root := range roots {
			componentPath := filepath.Join(root, req)
			info, err := os.Stat(componentPath)
			if err != nil || !info.IsDir() {
				continue
			}
			entries, err := os.ReadDir(componentPath)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				name := e.Name()
				if strings.HasPrefix(name, "8.0") {
					foundDir = filepath.Join(componentPath, name)
					break
				}
			}
			if foundDir != "" {
				break
			}
		}
		if foundDir != "" {
			found = append(found, fmt.Sprintf("%s=%s", req, foundDir))
		} else {
			missing = append(missing, req)
		}
	}

	if len(missing) > 0 {
		return false, fmt.Sprintf("缺少 .NET 8.0 组件: %v。已找到: %v。请运行安装目录 prerequisites\\windowsdesktop-runtime-8.0.10-win-x64.exe 和 prerequisites\\dotnet-hosting-8.0.10-win.exe", missing, found)
	}
	return true, fmt.Sprintf("使用系统运行时: %v", found)
}

// killProcessTree 杀掉指定 PID 及其所有子进程（taskkill /F /T /PID）
func killProcessTree(rootPID uint32) {
	// 用 taskkill 强杀整个进程树（/T = 递归子进程，/F = 强制）
	cmd := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", rootPID))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Run()
}
