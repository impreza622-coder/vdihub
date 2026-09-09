package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"shoukehuduan/internal/protocol"
	"shoukehuduan/internal/tray"
)

var errWindowNotReady = errors.New("窗口未就绪")

// GetAppVersion 返回当前应用版本号（构建时通过 //go:embed VERSION 嵌入，格式 V1.003）
// 用于前端"关于我们"页面显示版本号，跟随 build.ps1 自增版本号自动更新
func (a *App) GetAppVersion() string {
	v := strings.TrimSpace(string(versionRaw))
	if v == "" {
		return "V1.0.0"
	}
	return "V" + v
}

// Rect 窗口位置和尺寸（前端精确拖拽使用）
type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

type App struct {
	ctx context.Context
	// kioskConfigPath 用于 kiosk 模式持久化，保证下次启动强制全屏无边框
	kioskConfigPath      string
	kioskMigrateFlagPath string
	trayMgr              *tray.Manager
}

func NewApp() *App {
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	a := &App{
		kioskConfigPath:      filepath.Join(dir, ".kiosk_mode"),
		kioskMigrateFlagPath: filepath.Join(dir, ".kiosk_migrate_v1"),
		trayMgr:              tray.New(),
	}
	// 设置托盘退出钩子：用户点击托盘"退出程序"时，先清理 Kiosk 标记 + 停止键盘钩子和守护进程，
	// 这样 OnBeforeClose 检测到 userRequestedQuit=true 后 return false，程序才能真正退出。
	a.trayMgr.SetQuitHook(func() {
		defer func() { _ = recover() }()
		// 1. 删除 Kiosk 标记文件（防止下次启动残留 Kiosk 状态）
		if a.kioskConfigPath != "" {
			_ = os.Remove(a.kioskConfigPath)
		}
		// 2. 停止 ALT+F4 键盘钩子
		stopAltF4Block()
		// 3. 停止 Kiosk 守护进程
		stopKioskStyleDaemon()
		// 4. 恢复窗口样式（去 Kiosk 锁定）
		platformRestoreMainWindow()
		// 5. 还原任务栏（Kiosk 模式下隐藏了 Shell_TrayWnd，必须还回来，
		//    否则用户退程序后系统任务栏仍然隐藏无法操作，要重启 Explorer 才恢复）
		showTaskbarWindows()
	})
	// 升级迁移：此版本开始默认无边框+自绘最小化/最大化/关闭按钮可见
	// 旧版本遗留的 .kiosk_mode 会让启动直接全屏、连自绘标题栏三按钮都被隐藏，
	// 所以首次运行新版本时主动清一次旧标记，让用户先看到自绘标题栏按钮
	if _, err := os.Stat(a.kioskMigrateFlagPath); os.IsNotExist(err) {
		if a.kioskConfigPath != "" {
			_ = os.Remove(a.kioskConfigPath)
		}
		_ = os.WriteFile(a.kioskMigrateFlagPath, []byte("1"), 0644)
	}
	return a
}

// Tray 允许外部（main）拿到托盘管理器句柄做初始化使用。
func (a *App) Tray() *tray.Manager {
	return a.trayMgr
}

func (a *App) StartUp(ctx context.Context) {
	a.ctx = ctx
	// 默认启动就去系统标题栏/边框，用前端自绘标题栏 + 最小化/最大化/关闭按钮
	platformApplyKioskFrameless()
	// 无边框之后马上把窗口移到屏幕中央，避免"卡在左上角"
	platformCenterWindowOnScreen()
	kiosk := a.isKioskFlagOnDisk()
	if kiosk {
		// kiosk 模式：全屏
		runtime.WindowFullscreen(ctx)
		platformApplyKioskFrameless()
		runtime.WindowFullscreen(ctx)
	}

	// 绑定并启动系统托盘（Kiosk 模式下禁用托盘）
	a.trayMgr.BindWailsContext(ctx)
	if kiosk {
		a.trayMgr.SetEnabled(false)
	} else {
		a.trayMgr.SetEnabled(true)
		a.trayMgr.Start()
	}
}

func (a *App) Shutdown(ctx context.Context) {
}

// isKioskFlagOnDisk 读取 kiosk 持久化标记
func (a *App) isKioskFlagOnDisk() bool {
	if a.kioskConfigPath == "" {
		return false
	}
	_, err := os.Stat(a.kioskConfigPath)
	return err == nil
}

// ApplyKioskMode 进入 Kiosk：瘦客户端 UI 强制全屏 + 去掉标题栏边框 + 隐藏系统托盘 + 封死 ALT+F4 + 隐藏任务栏 + TOPMOST 守护
func (a *App) ApplyKioskMode() error {
	if a.ctx == nil {
		return errWindowNotReady
	}
	runtime.WindowFullscreen(a.ctx)
	platformApplyKioskFrameless()
	runtime.WindowFullscreen(a.ctx)
	// ★ 关键：封掉主程序的 WS_SYSMENU → ALT+F4 / X / 系统菜单"关闭"全部失效
	//    防止用户按一下 Alt+F4 就能退到桌面（用户明确要求看不到桌面）
	platformLockdownMainWindowKioskMode()
	// ★★ Kiosk 硬锁：立即隐藏任务栏（主任务栏 + 多显示器副任务栏），
	//    否则 Horizon 登录窗体激活时会唤醒 Explorer → 任务栏浮出 → 用户鼠标点"显示桌面"
	//    就能最小化主窗口看到桌面，退出软件。
	hideTaskbarWindows()
	ensureKioskWindowOnTopAndFront()
	// ★★ 额外：开启 WH_KEYBOARD_LL 低级键盘钩子 + Kiosk 守护进程（每 0.8s 重去一次 WS_SYSMENU + 隐藏任务栏 + TOPMOST）
	//    只去 WS_SYSMENU 会被 WebView2 的内部重建窗口/DPI 变化又加回来，必须用钩子在系统层吞 ALT+F4
	startAltF4Block()
	if a.kioskConfigPath != "" {
		if err := os.WriteFile(a.kioskConfigPath, []byte("1"), 0644); err != nil {
			return fmt.Errorf("写入 kiosk 标记失败: %w", err)
		}
	}
	// Kiosk 下禁用系统托盘（避免用户通过托盘"退出程序"绕过全屏锁）
	a.trayMgr.SetEnabled(false)
	return nil
}

// ExitKioskMode 退出 Kiosk：恢复普通无边框+自绘标题栏窗口（仍然无系统边框）+ 恢复系统托盘 + 还原 ALT+F4 系统菜单 + 还原任务栏
func (a *App) ExitKioskMode() error {
	if a.ctx == nil {
		return errWindowNotReady
	}
	runtime.WindowUnfullscreen(a.ctx)
	// 仍然保留我们自己默认的无边框 + 自绘标题栏，不恢复系统标题栏
	platformApplyKioskFrameless()
	// 还原 WS_SYSMENU / MINBOX / MAXBOX（X/ALT+F4 恢复可用）
	platformRestoreMainWindow()
	// ★ 同时关闭低级键盘钩子的 ALT+F4 拦截，恢复最小/关闭按钮可用性
	stopAltF4Block()
	// ★★ 还原任务栏（进入 Kiosk 时 hideTaskbarWindows 隐藏的必须还回来）
	showTaskbarWindows()
	if a.kioskConfigPath != "" {
		_ = os.Remove(a.kioskConfigPath)
	}
	// 回到普通模式：重新启用系统托盘（如果当前未运行则启动）
	a.trayMgr.SetEnabled(true)
	a.trayMgr.Start()
	// ★★ 兜底：WindowUnfullscreen 后 Wails 经常把窗口尺寸停在屏幕大小（像最大化但实际不是），
	//    用户遇到的 1100×790 "变最大化无法还原" 就是这种情况，此时用户再点最大化/还原
	//    按钮因为 WindowIsMaximised 返回值错乱导致无反应。这里统一强制 WindowCenter 居中，
	//    让窗口脱离屏幕四边（伪最大化视觉消失），用户再用最大化/还原按钮就能正常工作。
	runtime.WindowCenter(a.ctx)
	// 保证窗口可见
	a.trayMgr.ShowWindow()
	return nil
}

// WindowMinimize 最小化窗口（前端标题栏按钮调用）
func (a *App) WindowMinimize() error {
	if a.ctx == nil {
		return errWindowNotReady
	}
	runtime.WindowMinimise(a.ctx)
	return nil
}

// WindowToggleMaximize 切换最大化/还原，返回切换后的最大化状态
func (a *App) WindowToggleMaximize() (bool, error) {
	if a.ctx == nil {
		return false, errWindowNotReady
	}
	if runtime.WindowIsMaximised(a.ctx) {
		runtime.WindowUnmaximise(a.ctx)
		// Wails Unmaximise 后需要再刷一次无边框样式
		platformApplyKioskFrameless()
		return false, nil
	}
	runtime.WindowMaximise(a.ctx)
	platformApplyKioskFrameless()
	return true, nil
}

// WindowClose 关闭按钮行为：
//   - Kiosk 模式：忽略（不允许通过关闭按钮退出，保持强制全屏）
//   - 普通模式：隐藏窗口到系统托盘（不退出），真正退出请通过托盘菜单"退出程序"或左侧菜单"退出"
func (a *App) WindowClose() error {
	if a.ctx == nil {
		return errWindowNotReady
	}
	if a.isKioskFlagOnDisk() {
		// Kiosk：静默忽略关闭请求，用户无法通过 X 退出
		return nil
	}
	a.trayMgr.HideWindow()
	return nil
}

// WindowIsMaximized 查询当前是否最大化（前端按钮图标切换用）
func (a *App) WindowIsMaximized() (bool, error) {
	if a.ctx == nil {
		return false, errWindowNotReady
	}
	return runtime.WindowIsMaximised(a.ctx), nil
}

// WindowStartDrag 开始拖拽移动窗口（自绘标题栏按下时调用，HTCAPTION 方式）
func (a *App) WindowStartDrag() error {
	if a.ctx == nil {
		return errWindowNotReady
	}
	platformStartWindowDrag()
	return nil
}

// WindowGetRect 返回窗口屏幕坐标 x,y,w,h（前端精确拖拽使用）
func (a *App) WindowGetRect() (*Rect, error) {
	if a.ctx == nil {
		return nil, errWindowNotReady
	}
	rx, ry, rw, rh, ok := platformWindowGetRect()
	if !ok {
		return nil, fmt.Errorf("获取窗口位置失败")
	}
	return &Rect{X: rx, Y: ry, W: rw, H: rh}, nil
}

// WindowSetPos 设置窗口位置（左上角屏幕坐标），保持尺寸不变
func (a *App) WindowSetPos(x, y int) error {
	if a.ctx == nil {
		return errWindowNotReady
	}
	platformWindowSetPos(x, y)
	return nil
}

// WindowCenter 手动居中（前端特殊情况调用）
func (a *App) WindowCenter() error {
	if a.ctx == nil {
		return errWindowNotReady
	}
	platformCenterWindowOnScreen()
	return nil
}

// -------- 开机启动（注册表 HKCU Run）---------

type AutostartStatus struct {
	Normal    bool `json:"normal"`
	KioskAuto bool `json:"kiosk_auto"`
}

// GetAutostart 读取当前两个 Run 条目开关状态（全局设置 UI 显示用）
func (a *App) GetAutostart() (*AutostartStatus, error) {
	n, k := autostartStatus()
	return &AutostartStatus{Normal: n, KioskAuto: k}, nil
}

// SetAutostart 写/删开机启动。kioskAuto=true 时附加 --kiosk-autostart（开机无感到首页，锁死 Kiosk）
//
//	如果 enabled=true 且 kioskAuto=false → 仅普通开机启动（弹出登录页）
//	如果 enabled=false → 两个 Run 条目都删
func (a *App) SetAutostart(enabled bool, kioskAuto bool) error {
	return setAutostart(enabled, kioskAuto)
}

// ==================== 硬盘保护 (UWF - Unified Write Filter) ====================
// 仅支持 Windows 10/11 Enterprise / LTSC 版本（LTSC 2019 / LTSC 2021 等）
// 专业版/家庭版没有 Client-UnifiedWriteFilter 功能
//
// 流程说明：
//
//	开启：dism 启用功能 → 重启①（驱动加载）→ uwfmgr filter enable + volume protect c: → 重启②生效
//	关闭：uwfmgr filter disable + volume unprotect c: → 重启生效
type UWFStatus struct {
	Supported       bool   `json:"supported"`       // 系统是否支持 UWF
	FeatureEnabled  bool   `json:"featureEnabled"`  // dism 功能是否已安装
	FilterEnabled   bool   `json:"filterEnabled"`   // uwfmgr filter 当前状态
	VolumeProtected bool   `json:"volumeProtected"` // C: 是否被保护
	PendingEnable   bool   `json:"pendingEnable"`   // 是否有待重启后生效的开启操作
	PendingDisable  bool   `json:"pendingDisable"`  // 是否有待重启后生效的关闭操作
	NeedRebootStep1 bool   `json:"needRebootStep1"` // 功能刚装，需要第一次重启（加载驱动）
	NeedRebootStep2 bool   `json:"needRebootStep2"` // 配置完成，需要第二次重启（真正生效/关闭）
	Message         string `json:"message"`         // 状态说明（中文）
}

// getSysNativePath 获取 64 位系统工具的完整路径
// 32位进程在 64位系统上访问 System32 会被重定向到 SysWOW64，
// 导致找不到 dism.exe/uwfmgr.exe（它们只在 System32 中）。
// Sysnative 是 32位进程访问真正 64位 System32 的唯一方式。
func getSysNativePath(filename string) string {
	sysroot := os.Getenv("SystemRoot")
	if sysroot == "" {
		sysroot = `C:\Windows`
	}
	// 优先使用 Sysnative（32位进程访问 64位系统文件的唯一方式）
	sysnativePath := sysroot + `\Sysnative\` + filename
	if _, err := os.Stat(sysnativePath); err == nil {
		return sysnativePath
	}
	// 回退到 System32（64位进程本身就可以直接访问）
	system32Path := sysroot + `\System32\` + filename
	if _, err := os.Stat(system32Path); err == nil {
		return system32Path
	}
	// 都找不到则直接返回 Sysnative 路径，让调用者处理错误
	return sysnativePath
}

// runCmdNoWindow 执行命令并隐藏控制台窗口，返回输出与错误
func runCmdNoWindow(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// GetUWFStatus 查询 UWF 当前状态（前端调用）
func (a *App) GetUWFStatus() (status *UWFStatus, err error) {
	// panic 兜底：任何意外都不会让前端收到 "查询失败"
	defer func() {
		if r := recover(); r != nil {
			status = &UWFStatus{Supported: false, Message: "UWF 查询异常: " + fmt.Sprintf("%v", r)}
			err = nil
		}
	}()
	s := &UWFStatus{Supported: true}

	// 0) 先通过注册表检查 OS 版本，判断是否为 Enterprise/LTSC（不需要管理员权限，永远可靠）
	//    即使后续 dism 因权限/语言问题失败，也能正确判断系统是否支持 UWF
	editionID := readRegistryString("HKLM", `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, "EditionID")
	productName := readRegistryString("HKLM", `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, "ProductName")
	editionLow := strings.ToLower(editionID + " " + productName)
	// 英文匹配 + 中文匹配（「企业版」= Enterprise，「企业版 LTSC」= Enterprise LTSC）
	isEnterpriseOrLTSC := strings.Contains(editionLow, "enterprise") ||
		strings.Contains(editionLow, "ltsc") ||
		strings.Contains(editionLow, "enterpriseg") ||
		strings.Contains(editionLow, "enterprisen") ||
		strings.Contains(editionLow, "企业版") ||
		strings.Contains(editionLow, "企业")

	// 1) 检查功能是否已安装（dism /Get-FeatureInfo）
	//    使用 Sysnative 路径访问 64 位系统文件（32位进程访问 System32 会被重定向到 SysWOW64）
	dismPath := getSysNativePath("dism.exe")
	out, err := runCmdNoWindow(dismPath, "/Online", "/Get-FeatureInfo", "/FeatureName:Client-UnifiedWriteFilter")
	if err != nil {
		// dism 报错：可能是权限不足或命令不可用
		// 用注册表判断的系统版本来决定 Supported
		if isEnterpriseOrLTSC {
			// 系统支持但 dism 失败（可能权限不足），仍允许操作
			s.Supported = true
			s.FeatureEnabled = false
			s.Message = "UWF 功能未安装，点击开启后将自动安装并提示重启"
			return s, nil
		}
		s.Supported = false
		s.Message = "当前系统不支持 UWF 硬盘保护（仅 LTSC / Enterprise 版可用）"
		return s, nil
	}
	low := strings.ToLower(out)

	// 匹配 dism 输出中的功能状态（兼容中英文系统）
	// 英文: "State : Enabled" / "State : Disabled" / "Enable Pending"
	// 中文: "状态 : 已启用" / "状态 : 已禁用" / "已启用挂起"
	isStateEnabled := strings.Contains(low, "state : enabled") || strings.Contains(low, "state: enabled") ||
		strings.Contains(low, "状态 : 已启用") || strings.Contains(low, "状态: 已启用") ||
		strings.Contains(low, "状态 : 已启用挂起")
	isStateDisabled := strings.Contains(low, "state : disabled") || strings.Contains(low, "state: disabled") ||
		strings.Contains(low, "状态 : 已禁用") || strings.Contains(low, "状态: 已禁用")
	isEnablePending := strings.Contains(low, "enable pending") ||
		strings.Contains(low, "已启用挂起") || strings.Contains(low, "启用挂起")

	if isStateEnabled && !isEnablePending {
		s.FeatureEnabled = true
	} else if isStateDisabled {
		s.FeatureEnabled = false
		s.Message = "UWF 功能未安装，点击开启后将自动安装并提示重启"
		return s, nil
	} else if isEnablePending {
		// 刚装完功能，等待第一次重启
		s.FeatureEnabled = true
		s.NeedRebootStep1 = true
		s.Message = "UWF 功能已安装，请重启计算机以完成驱动加载"
		return s, nil
	} else if isEnterpriseOrLTSC {
		// dism 没报错但也没匹配到已知状态（可能输出格式特殊），但系统版本支持
		s.FeatureEnabled = false
		s.Message = "UWF 功能未安装，点击开启后将自动安装并提示重启"
		return s, nil
	}

	// 2) 查询 uwfmgr 当前配置（功能已启用才能执行）
	uwfmgrPath := getSysNativePath("uwfmgr.exe")
	cfgOut, _ := runCmdNoWindow(uwfmgrPath, "get-config")
	cfgLow := strings.ToLower(cfgOut)

	// Filter state（兼容中英文输出）
	// 英文: "Filter State: On" / "Filter State: Off"
	// 中文: "筛选器状态: 打开" / "筛选器状态: 关闭"
	if strings.Contains(cfgLow, "filter state: on") || strings.Contains(cfgLow, "filter state:  on") ||
		strings.Contains(cfgLow, "筛选器状态: 打开") || strings.Contains(cfgLow, "筛选器状态:打开") {
		s.FilterEnabled = true
	}
	// 下次会话状态（pending）
	// 英文: "Next Session: On" / "Next Session: Off"
	// 中文: "下次会话: 打开" / "下次会话: 关闭"
	nextSessionOn := strings.Contains(cfgLow, "next session") && strings.Contains(cfgLow, "on") ||
		strings.Contains(cfgLow, "下次会话") && (strings.Contains(cfgLow, "打开") || strings.Contains(cfgLow, "启用"))
	nextSessionOff := strings.Contains(cfgLow, "next session") && strings.Contains(cfgLow, "off") ||
		strings.Contains(cfgLow, "下次会话") && (strings.Contains(cfgLow, "关闭") || strings.Contains(cfgLow, "禁用"))
	if nextSessionOff {
		s.PendingDisable = true
	}
	if nextSessionOn && !s.FilterEnabled {
		s.PendingEnable = true
	}
	// C: 卷保护（兼容中英文）
	// 英文: "Volume C: Protected" / "protect"
	// 中文: "卷 C: 已保护" / "保护"
	if strings.Contains(cfgLow, "volume c:") || strings.Contains(cfgLow, "卷 c:") {
		if strings.Contains(cfgLow, "protect") || strings.Contains(cfgLow, "protected") ||
			strings.Contains(cfgLow, "保护") || strings.Contains(cfgLow, "已保护") {
			s.VolumeProtected = true
		}
	}

	// 汇总状态消息
	switch {
	case s.FilterEnabled && s.VolumeProtected && !s.PendingDisable:
		s.Message = "硬盘保护已启用（C 盘只读，重启后所有更改将被还原）"
	case s.PendingEnable && s.PendingDisable == false && s.FilterEnabled == false:
		s.NeedRebootStep2 = true
		s.Message = "保护配置完成，请重启计算机使硬盘保护生效"
	case s.PendingDisable:
		s.NeedRebootStep2 = true
		s.Message = "已计划关闭保护，请重启计算机使更改生效"
	case s.FilterEnabled == false && s.FeatureEnabled:
		s.Message = "UWF 功能已就绪，但未启用保护。点击开启以配置 C 盘保护"
	default:
		s.Message = ""
	}
	return s, nil
}

// EnableUWF 启用 UWF 硬盘保护（分两阶段）
//
//	阶段 1：功能未安装 → dism 启用 + 提示重启（驱动加载）
//	阶段 2：功能已装但未保护 → uwfmgr filter enable + volume protect c: + 提示重启生效
func (a *App) EnableUWF() (string, error) {
	st, err := a.GetUWFStatus()
	if err != nil {
		return "", err
	}
	if !st.Supported {
		return "", errors.New("当前系统不支持 UWF（仅 Windows 10/11 Enterprise 或 LTSC 版可用）")
	}

	if !st.FeatureEnabled {
		// ===== 阶段 1：安装 UWF 功能 =====
		dismPath := getSysNativePath("dism.exe")
		out, err := runCmdNoWindow(dismPath, "/Online", "/Enable-Feature", "/FeatureName:Client-UnifiedWriteFilter", "/All", "/NoRestart")
		if err != nil {
			return "", fmt.Errorf("安装 UWF 功能失败: %v\n%s", err, out)
		}
		return "UWF 功能已安装。\n\n【下一步】请重启计算机以加载 UWF 驱动；重启后再次打开本开关完成 C 盘保护配置。", nil
	}

	if st.NeedRebootStep1 {
		return "", errors.New("UWF 功能刚安装，请先重启计算机，再执行保护配置")
	}

	// ===== 阶段 2：启用筛选器 + 保护 C 盘 =====
	uwfmgrPath := getSysNativePath("uwfmgr.exe")
	if out, err := runCmdNoWindow(uwfmgrPath, "filter", "enable"); err != nil {
		return "", fmt.Errorf("启用 UWF 筛选器失败: %v\n%s", err, out)
	}
	if out, err := runCmdNoWindow(uwfmgrPath, "volume", "protect", "c:"); err != nil {
		return "", fmt.Errorf("保护 C 盘失败: %v\n%s", err, out)
	}
	return "硬盘保护已配置完成。\n\n【下一步】请重启计算机使保护生效。重启后 C 盘将处于只读模式，所有写入重启后都会被还原。", nil
}

// DisableUWF 关闭 UWF 硬盘保护
func (a *App) DisableUWF() (string, error) {
	st, err := a.GetUWFStatus()
	if err != nil {
		return "", err
	}
	if !st.Supported {
		return "", errors.New("当前系统不支持 UWF")
	}
	if !st.FeatureEnabled {
		return "UWF 功能未安装，无需关闭", nil
	}

	// 取消 C 盘保护（如果配过）
	uwfmgrPath := getSysNativePath("uwfmgr.exe")
	_, _ = runCmdNoWindow(uwfmgrPath, "volume", "unprotect", "c:")
	// 关闭筛选器
	if out, err := runCmdNoWindow(uwfmgrPath, "filter", "disable"); err != nil {
		return "", fmt.Errorf("关闭 UWF 筛选器失败: %v\n%s", err, out)
	}
	return "硬盘保护已计划关闭。\n\n【下一步】请重启计算机使更改生效。", nil
}

// -------- 启动意图（是否带 --kiosk-autostart） --------
type StartHint struct {
	KioskAuto bool   `json:"kiosk_auto"` // true → 前端启动后自动尝试登录 + 进 Kiosk（用户看不到桌面）
	ExePath   string `json:"exe_path"`
}

var (
	_globalStartHint = &StartHint{}
)

func setGlobalStartHintFromArgs() {
	h := &StartHint{}
	if e, err := os.Executable(); err == nil {
		h.ExePath = e
	}
	for _, a := range os.Args {
		switch a {
		case "--kiosk-autostart", "-kiosk-autostart", "--auto-start", "--autostart":
			h.KioskAuto = true
		}
	}
	_globalStartHint = h
}

// GetStartHint 给前端用：知道"这次是不是开机自启+要无感进主页"
func (a *App) GetStartHint() (*StartHint, error) {
	if _globalStartHint == nil {
		setGlobalStartHintFromArgs()
	}
	h := *_globalStartHint
	return &h, nil
}

// -------- Kiosk 加强：调用时再去 WS_SYSMENU（Kiosk 模式下 ALT+F4 完全锁死） --------
//  双保险：
//   A) main.go 里 Wails OnBeforeClose(kiosk=true时 return true 不允许关)
//   B) 这里 platformLockdownMainWindowKioskMode() 去 WS_SYSMENU（即使焦点在 WebView2 里按 Alt+F4 也不会触发关窗）
// ApplyKioskMode 已经会写 kiosk 标记；这里再追加一个强化版锁定（Kiosk下 ALT+F4 / X / 系统菜单 都失效）

// LockdownKioskClose 在进入 Kiosk 时由前端/后端统一调用，完全封死主程序的"关闭/最小化/最大化"按钮和快捷键
func (a *App) LockdownKioskClose() error {
	platformLockdownMainWindowKioskMode()
	// 再次启动 WH_KEYBOARD_LL 钩子（如果 ApplyKioskMode 还没触发，提前让键盘钩子有效）
	startAltF4Block()
	return nil
}

// UnlockKioskClose 从 Kiosk 退出时还原
func (a *App) UnlockKioskClose() error {
	platformRestoreMainWindow()
	stopAltF4Block()
	return nil
}

// ShutdownMachine 关闭本地机器电源
// 在 Kiosk 模式下用户无法访问 Windows 关机选项，提供此方法让用户能正常关机
func (a *App) ShutdownMachine() error {
	// 1. 先停止键盘钩子，避免拦截系统关机流程
	stopAltF4Block()
	// 2. 退出 Kiosk 模式（还原窗口）
	_ = a.ExitKioskMode()
	// 3. 延迟 1 秒后执行关机，让应用程序有时间清理
	go func() {
		time.Sleep(1 * time.Second)
		// shutdown /s /t 5 /c "云桌面客户端正在关闭计算机"
		// /s = 关机, /t 5 = 5秒后执行, /c = 提示信息
		cmd := exec.Command("shutdown", "/s", "/t", "5", "/c", "飞力达云桌面客户端正在关闭计算机...")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		_ = cmd.Start()
		// 4. 退出本程序
		os.Exit(0)
	}()
	return nil
}

// RestartMachine 重启本地机器
func (a *App) RestartMachine() error {
	// 1. 先停止键盘钩子
	stopAltF4Block()
	// 2. 退出 Kiosk 模式
	_ = a.ExitKioskMode()
	// 3. 延迟 1 秒后执行重启
	go func() {
		time.Sleep(1 * time.Second)
		// shutdown /r /t 5 /c "云桌面客户端正在重启计算机"
		// /r = 重启, /t 5 = 5秒后执行, /c = 提示信息
		cmd := exec.Command("shutdown", "/r", "/t", "5", "/c", "飞力达云桌面客户端正在重启计算机...")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		_ = cmd.Start()
		// 4. 退出本程序
		os.Exit(0)
	}()
	return nil
}

// QuitApplication 真正退出程序（不是隐藏到托盘）。
// 供前端"全局设置 → 退出程序"按钮调用，走和托盘"退出程序"相同的清理+退出流程。
func (a *App) QuitApplication() error {
	// 通过 trayMgr.Quit() 走统一退出流程：
	// 1. 设置 userRequestedQuit 标志 → OnBeforeClose 放行
	// 2. 调用 quitHook → 清理 Kiosk 标记 + 停止键盘钩子/守护进程
	// 3. wailsruntime.Quit → 程序退出
	go func() {
		defer func() { _ = recover() }()
		time.Sleep(200 * time.Millisecond) // 给前端一点时间关闭弹窗
		a.trayMgr.Quit()
	}()
	return nil
}

// -------- 系统级嵌入 Kiosk（全局设置里开关） --------
// SystemKioskConfig 前端提交的系统嵌入配置（控制面板/Shell 替换/自动登录）
type SystemKioskConfig struct {
	// 控制面板
	RegisterUninstall  bool   `json:"registerUninstall"`
	UninstallPublisher string `json:"uninstallPublisher"`
	UninstallVersion   string `json:"uninstallVersion"`
	// Shell 替换（不进桌面，直接进云桌面）
	ReplaceShell bool `json:"replaceShell"`
	// 自动登录
	EnableAutoLogin bool   `json:"enableAutoLogin"`
	AutoLoginUser   string `json:"autoLoginUser"`
	AutoLoginPwd    string `json:"autoLoginPwd"`
	AutoLoginDomain string `json:"autoLoginDomain"`
	// 应用后是否立即调度 10 秒后重启
	ScheduleReboot bool `json:"scheduleReboot"`
}

// ApplyKioskResult 应用后结果（是否立即调度了重启）
type ApplyKioskResult struct {
	ScheduledReboot bool `json:"scheduledReboot"`
}

// GetSystemKioskStatus 前端打开管理设置时调用，显示当前系统嵌入的真实开关状态
func (a *App) GetSystemKioskStatus() (*SystemKioskStatus, error) {
	defer func() { _ = recover() }()
	st := readSystemKioskStatus()
	if st == nil {
		return &SystemKioskStatus{OurExePath: getExecutablePath()}, nil
	}
	return st, nil
}

// ApplySystemKioskConfig 应用系统级嵌入配置：如果当前非管理员 → 弹 UAC 自我提权
// 执行后成功会根据 scheduleReboot 决定是否立即调度重启
func (a *App) ApplySystemKioskConfig(cfg *SystemKioskConfig) (*ApplyKioskResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("参数为空")
	}
	p := privApplyPayload{
		Op:                 "apply",
		RegisterUninstall:  cfg.RegisterUninstall,
		UninstallPublisher: cfg.UninstallPublisher,
		UninstallVersion:   cfg.UninstallVersion,
		ReplaceShell:       cfg.ReplaceShell,
		EnableAutoLogin:    cfg.EnableAutoLogin,
		AutoLoginUser:      cfg.AutoLoginUser,
		AutoLoginPwd:       cfg.AutoLoginPwd,
		AutoLoginDomain:    cfg.AutoLoginDomain,
		ScheduleReboot:     cfg.ScheduleReboot,
	}
	// 序列化负载（JSON → 传给 runas 子进程的命令行参数）
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	handled, subErr := runElevatedSelf(string(b))
	if subErr != nil {
		return nil, subErr
	}
	if !handled {
		// 当前进程就是管理员，直接自己执行（不会弹 UAC，0 次）
		if err2 := doPrivApply(p); err2 != nil {
			return nil, err2
		}
	}
	return &ApplyKioskResult{ScheduledReboot: p.ScheduleReboot}, nil
}

// RevertSystemKiosk 解除系统级嵌入：还原 Shell=explorer.exe + 关自动登录 + 删控制面板条目
// scheduleReboot=true 时同样会调度 10 秒重启
func (a *App) RevertSystemKiosk(scheduleReboot bool) (*ApplyKioskResult, error) {
	p := privApplyPayload{
		Op:             "revert",
		ScheduleReboot: scheduleReboot,
	}
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	handled, subErr := runElevatedSelf(string(b))
	if subErr != nil {
		return nil, subErr
	}
	if !handled {
		if err2 := doPrivApply(p); err2 != nil {
			return nil, err2
		}
	}
	return &ApplyKioskResult{ScheduledReboot: scheduleReboot}, nil
}

// RebootSystemNow 立即重启系统（用户点击"立即重启"按钮时调用）
// 实际使用 shutdown /r /t 0，管理员/普通用户都有权限执行
func (a *App) RebootSystemNow() error {
	defer func() { _ = recover() }()
	cmd := exec.Command("shutdown", "/r", "/t", "0", "/c", "云桌面客户端正在重启系统以应用嵌入设置…")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd.Start()
}

type AuthService struct {
	app     *App
	token   string
	baseURL string
}

func NewAuthService(app *App) *AuthService {
	return &AuthService{
		app:     app,
		baseURL: "http://localhost:8080",
	}
}

func (s *AuthService) SetBaseURL(url string) {
	s.baseURL = url
}

func (s *AuthService) GetBaseURL() string {
	return s.baseURL
}

func (s *AuthService) SetToken(token string) {
	s.token = token
}

func (s *AuthService) GetToken() string {
	return s.token
}

func (s *AuthService) IsLoggedIn() bool {
	return s.token != ""
}

func (s *AuthService) Login(username, password string) (map[string]interface{}, error) {
	payload := map[string]string{
		"username": username,
		"password": password,
	}

	var result struct {
		Code    int                    `json:"code"`
		Message string                 `json:"message"`
		Data    map[string]interface{} `json:"data"`
	}

	err := s.doRequest("POST", "/api/v1/auth/login", payload, &result)
	if err != nil {
		return nil, err
	}

	if result.Code != 0 {
		return nil, fmt.Errorf(result.Message)
	}

	if token, ok := result.Data["token"].(string); ok {
		s.token = token
	}

	return result.Data, nil
}

func (s *AuthService) Register(username, password, email string) error {
	payload := map[string]string{
		"username": username,
		"password": password,
		"email":    email,
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	err := s.doRequest("POST", "/api/v1/auth/register", payload, &result)
	if err != nil {
		return err
	}

	if result.Code != 0 {
		return fmt.Errorf(result.Message)
	}

	return nil
}

func (s *AuthService) doRequest(method, path string, payload interface{}, result interface{}) error {
	url := s.baseURL + path

	var body io.Reader
	if payload != nil {
		jsonBody, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("序列化请求失败: %w", err)
		}
		body = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	if err := json.Unmarshal(respBody, result); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	return nil
}

type VMService struct {
	auth *AuthService
}

func NewVMService(auth *AuthService) *VMService {
	return &VMService{
		auth: auth,
	}
}

type VMInfo struct {
	VMID       int     `json:"vmid"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	Node       string  `json:"node"`
	CPUCores   int     `json:"cpu_cores"`
	MemoryMB   int     `json:"memory_mb"`
	DiskGB     float64 `json:"disk_gb"`
	OS         string  `json:"os"`
	Protocol   string  `json:"protocol"`
	IPAddress  string  `json:"ip_address"`
	VMUsername string  `json:"vm_username,omitempty"`
	VMPassword string  `json:"vm_password,omitempty"`
	// 电源控制后端（与协议解耦）: "none" | "pve" | "hyperv" | "horizon"
	PowerBackend string `json:"power_backend,omitempty"`
	// ---- PVE 后端专用字段（Proxmox VE API）----
	PVEEndpoint  string `json:"pve_endpoint,omitempty"`
	PVEUsername  string `json:"pve_username,omitempty"`
	PVEPassword  string `json:"pve_password,omitempty"`
	PVENode      string `json:"pve_node,omitempty"`
	PVEVMID      int    `json:"pve_vmid,omitempty"`
	PVEVerifySSL bool   `json:"pve_verify_ssl,omitempty"`
	// ---- Hyper-V 后端专用字段（本地 PowerShell Hyper-V Cmdlet）----
	HyperVHost          string `json:"hyperv_host,omitempty"`      // 主机名，"." 或空表示本机
	HyperVVMName        string `json:"hyperv_vmname,omitempty"`    // Hyper-V 虚拟机名称（精确匹配）
	HyperVUseWinRM      bool   `json:"hyperv_use_winrm,omitempty"` // 是否走 WinRM 远程（false=本机直连）
	HyperVUsername      string `json:"hyperv_username,omitempty"`  // WinRM 账号（本机可空）
	HyperVPassword      string `json:"hyperv_password,omitempty"`  // WinRM 密码
	HyperVForceShutdown bool   `json:"hyperv_force,omitempty"`     // 关机是否强制（true=TurnOff 硬关，false=Shutdown 软关）
}

type ConnectionInfo struct {
	SessionID  string    `json:"session_id"`
	Protocol   string    `json:"protocol"`
	Host       string    `json:"host"`
	Port       int       `json:"port"`
	Password   string    `json:"password"`
	ExpiresAt  time.Time `json:"expires_at"`
	TLSPort    int       `json:"tls_port,omitempty"`
	Username   string    `json:"username,omitempty"`
	VMPassword string    `json:"vm_password,omitempty"`
	BrokerURL  string    `json:"broker_url,omitempty"`
}

func (v *VMService) ListVMs() ([]VMInfo, error) {
	if !v.auth.IsLoggedIn() {
		return nil, fmt.Errorf("请先登录")
	}

	var result struct {
		Code    int      `json:"code"`
		Message string   `json:"message"`
		Data    []VMInfo `json:"data"`
	}

	err := v.auth.doRequest("GET", "/api/v1/vms", nil, &result)
	if err != nil {
		return nil, err
	}

	if result.Code != 0 {
		return nil, fmt.Errorf(result.Message)
	}

	return result.Data, nil
}

func (v *VMService) StartVM(vmid int) error {
	if !v.auth.IsLoggedIn() {
		return fmt.Errorf("请先登录")
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	err := v.auth.doRequest("POST", fmt.Sprintf("/api/v1/vms/%d/start", vmid), nil, &result)
	if err != nil {
		return err
	}

	if result.Code != 0 {
		return fmt.Errorf(result.Message)
	}

	return nil
}

func (v *VMService) StopVM(vmid int) error {
	if !v.auth.IsLoggedIn() {
		return fmt.Errorf("请先登录")
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	err := v.auth.doRequest("POST", fmt.Sprintf("/api/v1/vms/%d/stop", vmid), nil, &result)
	if err != nil {
		return err
	}

	if result.Code != 0 {
		return fmt.Errorf(result.Message)
	}

	return nil
}

func (v *VMService) RebootVM(vmid int) error {
	if !v.auth.IsLoggedIn() {
		return fmt.Errorf("请先登录")
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	err := v.auth.doRequest("POST", fmt.Sprintf("/api/v1/vms/%d/reboot", vmid), nil, &result)
	if err != nil {
		return err
	}

	if result.Code != 0 {
		return fmt.Errorf(result.Message)
	}

	return nil
}

func (v *VMService) GetConnection(vmid int) (*ConnectionInfo, error) {
	if !v.auth.IsLoggedIn() {
		return nil, fmt.Errorf("请先登录")
	}

	var result struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    *ConnectionInfo `json:"data"`
	}

	err := v.auth.doRequest("POST", fmt.Sprintf("/api/v1/vms/%d/connect", vmid), nil, &result)
	if err != nil {
		return nil, err
	}

	if result.Code != 0 {
		return nil, fmt.Errorf(result.Message)
	}

	return result.Data, nil
}

type ProtocolService struct {
	app     *App
	manager *protocol.ProtocolManager
}

func NewProtocolService(app *App) *ProtocolService {
	return &ProtocolService{
		app:     app,
		manager: protocol.NewProtocolManager(),
	}
}

func (p *ProtocolService) LaunchRDP(host string, port int, password string) error {
	config := &protocol.ConnectionConfig{
		Host:     host,
		Port:     port,
		Password: password,
	}
	return p.manager.LaunchRDP(config)
}

// USBDevice 表示一个本地 USB 设备
type USBDevice struct {
	Name      string `json:"name"`      // 设备友好名称
	VID       string `json:"vid"`       // Vendor ID（如 "046D"）
	PID       string `json:"pid"`       // Product ID（如 "C534"")
	DeviceID  string `json:"device_id"` // 完整 PNPDeviceID
	Available bool   `json:"available"` // 是否当前已连接
}

// ListUSBDevices 枚举本地 USB 设备（含 VID/PID）
func (p *ProtocolService) ListUSBDevices() ([]USBDevice, error) {
	return listUSBDevicesOS()
}

func (p *ProtocolService) LaunchRDPWithAuth(host string, port int, username string, password string) error {
	config := &protocol.ConnectionConfig{
		Host:       host,
		Port:       port,
		Username:   username,
		VMPassword: password,
	}
	return p.manager.LaunchRDP(config)
}

func (p *ProtocolService) LaunchSPICE(host string, port int, tlsPort int, password string) error {
	config := &protocol.ConnectionConfig{
		Host:     host,
		Port:     port,
		Password: password,
		TLSPort:  tlsPort,
	}
	return p.manager.LaunchSPICE(config)
}

// LaunchSPICEViaPVE 通过 PVE API 获取临时 SPICE 凭据后启动 remote-viewer
// 参数:
//   - pveEndpoint: PVE 服务器地址（如 https://192.168.10.10:8006）
//   - pveUsername: PVE 用户名（如 root@pam）
//   - pvePassword: PVE 密码
//   - pveNode: PVE 节点名（如 pve）
//   - pveVMID: PVE 中的虚拟机 ID（如 100）
//   - verifySSL: 是否验证 SSL 证书
func (p *ProtocolService) LaunchSPICEViaPVE(pveEndpoint, pveUsername, pvePassword, pveNode string, pveVMID int, verifySSL bool) error {
	// 1. 登录 PVE 获取 ticket（裸用户名自动补全认证域：1000 → 1000@pve，root → root@pam）
	httpClient, ticket, csrfToken, err := pveLogin(pveEndpoint, pveUsername, pvePassword, verifySSL)
	if err != nil {
		return err
	}

	// 节点名为空时按 VMID 自动反查（兼容旧卡片数据缺节点字段）
	pveNode, err = ensurePVENode(httpClient, ticket, pveEndpoint, pveNode, pveVMID)
	if err != nil {
		return err
	}

	// 2. 调用 SPICE 代理 API 获取临时连接信息
	// PVE 8.x 使用 /spiceproxy（Web 控制台同款）：返回 .vv 配置，remote-viewer 经
	// PVE 节点 3128 代理连接 SPICE；旧的 /spice 直连端点在新版 PVE 上返回 501。
	spiceURL := fmt.Sprintf("%s/api2/json/nodes/%s/qemu/%d/spiceproxy", strings.TrimRight(pveEndpoint, "/"), pveNode, pveVMID)
	spiceReq, err := http.NewRequest("POST", spiceURL, nil)
	if err != nil {
		return fmt.Errorf("创建SPICE请求失败: %w", err)
	}
	// 注意：PVE 写操作（POST）不接受 Content-Type: application/json（会返回 501），
	// 无请求体时不要设置 Content-Type
	spiceReq.AddCookie(&http.Cookie{Name: "PVEAuthCookie", Value: ticket})
	spiceReq.Header.Set("CSRFPreventionToken", csrfToken)

	spiceResp, err := httpClient.Do(spiceReq)
	if err != nil {
		return fmt.Errorf("获取SPICE连接信息失败: %w", err)
	}
	defer spiceResp.Body.Close()

	spiceRespBody, _ := io.ReadAll(spiceResp.Body)
	if spiceResp.StatusCode >= 400 {
		return fmt.Errorf("PVE SPICE API错误 [%d]: %s", spiceResp.StatusCode, string(spiceRespBody))
	}

	// spiceproxy 返回的键名与 .vv 文件一致（连字符风格）
	var spiceInfo struct {
		Data struct {
			Host        string `json:"host"`         // pvespiceproxy:<token>:...（代理令牌，非真实主机）
			TLSPort     int    `json:"tls-port"`     // SPICE TLS 端口（代理令牌内寻址）
			Password    string `json:"password"`     // SPICE 临时密码
			Proxy       string `json:"proxy"`        // HTTP 代理，如 http://pve.com:3128
			CA          string `json:"ca"`           // PVE 集群 CA 证书（PEM）
			HostSubject string `json:"host-subject"` // TLS 证书主题（CN=pve.com）
			Title       string `json:"title"`        // 窗口标题
		} `json:"data"`
	}
	if err := json.Unmarshal(spiceRespBody, &spiceInfo); err != nil {
		return fmt.Errorf("解析SPICE信息失败: %w", err)
	}
	if spiceInfo.Data.Host == "" || spiceInfo.Data.TLSPort == 0 {
		return fmt.Errorf("PVE SPICE 返回数据不完整（虚拟机可能未运行或显示设备非 SPICE 类型）: %s", string(spiceRespBody))
	}

	// 3. proxy 字段中的主机名（如 pve.com）瘦客户机通常无法解析，
	//    替换为 PVE 服务器实际地址（从 endpoint 提取 host:3128）
	spiceProxy := spiceInfo.Data.Proxy
	if epu, perr := url.Parse(pveEndpoint); perr == nil && epu.Hostname() != "" {
		if pu, perr2 := url.Parse(spiceProxy); perr2 == nil {
			port := pu.Port()
			if port == "" {
				port = "3128"
			}
			pu.Host = net.JoinHostPort(epu.Hostname(), port)
			spiceProxy = pu.String()
		}
	}

	// 4. 使用获取到的临时凭据启动 remote-viewer（代理模式 .vv）
	config := &protocol.ConnectionConfig{
		Host:        spiceInfo.Data.Host,
		TLSPort:     spiceInfo.Data.TLSPort,
		Password:    spiceInfo.Data.Password,
		VMPassword:  spiceInfo.Data.Password,
		Proxy:       spiceProxy,
		CA:          spiceInfo.Data.CA,
		HostSubject: spiceInfo.Data.HostSubject,
		Title:       spiceInfo.Data.Title,
	}
	return p.manager.LaunchSPICE(config)
}

// ControlVMViaPVE 通过 PVE API 控制虚拟机电源（启动/停止/重启）
// action: "start" | "stop" | "reboot" | "suspend" | "resume"
func (p *ProtocolService) ControlVMViaPVE(pveEndpoint, pveUsername, pvePassword, pveNode string, pveVMID int, action string, verifySSL bool) error {
	// 1. 登录 PVE（裸用户名自动补全认证域：1000 → 1000@pve，root → root@pam）
	httpClient, ticket, csrfToken, err := pveLogin(pveEndpoint, pveUsername, pvePassword, verifySSL)
	if err != nil {
		return err
	}

	// 节点名为空时按 VMID 自动反查（兼容旧卡片数据缺节点字段，避免 /nodes//qemu 返回 501）
	pveNode, err = ensurePVENode(httpClient, ticket, pveEndpoint, pveNode, pveVMID)
	if err != nil {
		return err
	}

	// 2. 调用对应的电源控制 API
	// PVE API 参考：
	//   - start: 开机（Resume from paused 也是 start）
	//   - shutdown: 软关（通过 QEMU Guest Agent / ACPI，安全关闭，等 Guest OS 退出）
	//   - stop: 硬关（直接断电 Kill 掉 VM 进程，相当于强制断电，数据未保存会丢失）
	//   - reset: 硬重启（直接复位，不通知 Guest OS）
	//   - reboot: 软重启（先 shutdown 再 start，或通过 Guest Agent reboot）
	//   - suspend/resume: 暂停/恢复
	var apiPath string
	switch action {
	case "start":
		apiPath = fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/status/start", pveNode, pveVMID)
	case "shutdown": // ★ 新增：软关（安全），原来的 handleStopVM 默认走这个
		apiPath = fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/status/shutdown", pveNode, pveVMID)
	case "stop": // 硬关（强制断电），前端卡片设置的"强制断电关机"按钮走这个
		apiPath = fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/status/stop", pveNode, pveVMID)
	case "reset": // ★ 新增：硬重启（相当于断电再开机，风险提示）
		apiPath = fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/status/reset", pveNode, pveVMID)
	case "reboot": // 软重启（shutdown + start）
		apiPath = fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/status/reboot", pveNode, pveVMID)
	case "suspend":
		apiPath = fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/status/suspend", pveNode, pveVMID)
	case "resume":
		apiPath = fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/status/resume", pveNode, pveVMID)
	default:
		return fmt.Errorf("不支持的操作: %s", action)
	}

	req, err := http.NewRequest("POST", strings.TrimRight(pveEndpoint, "/")+apiPath, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	// 注意：PVE 写操作（POST）不接受 Content-Type: application/json（会返回 501），
	// 电源操作无请求体，不设置 Content-Type
	req.AddCookie(&http.Cookie{Name: "PVEAuthCookie", Value: ticket})
	req.Header.Set("CSRFPreventionToken", csrfToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("执行%s操作失败: %w", action, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("PVE API错误 [%d]: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// GetVMStatusViaPVE 通过 PVE API 获取虚拟机当前状态
func (p *ProtocolService) GetVMStatusViaPVE(pveEndpoint, pveUsername, pvePassword, pveNode string, pveVMID int, verifySSL bool) (string, error) {
	// 1. 登录 PVE（裸用户名自动补全认证域：1000 → 1000@pve，root → root@pam）
	httpClient, ticket, _, err := pveLogin(pveEndpoint, pveUsername, pvePassword, verifySSL)
	if err != nil {
		return "", err
	}

	// 节点名为空时按 VMID 自动反查（兼容旧卡片数据缺节点字段，避免 /nodes//qemu 返回 501）
	pveNode, err = ensurePVENode(httpClient, ticket, pveEndpoint, pveNode, pveVMID)
	if err != nil {
		return "", err
	}

	// 2. 查询虚拟机状态
	statusURL := fmt.Sprintf("%s/api2/json/nodes/%s/qemu/%d/status/current", strings.TrimRight(pveEndpoint, "/"), pveNode, pveVMID)
	req, err := http.NewRequest("GET", statusURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.AddCookie(&http.Cookie{Name: "PVEAuthCookie", Value: ticket})

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("查询状态失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("PVE API错误 [%d]: %s", resp.StatusCode, string(respBody))
	}

	var statusResult struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &statusResult); err != nil {
		return "", fmt.Errorf("解析状态响应失败: %w", err)
	}

	return statusResult.Data.Status, nil
}

// normalizePVEUsername 规范化 PVE 登录用户名：
// PVE 要求用户名必须带认证域（realm）后缀，裸用户名会直接返回 401（如 1000 必须写成 1000@pve）。
//   - 已含 @（user@pve / user@pam / user@AD.REALM 等）→ 原样使用
//   - root → root@pam（系统管理员账号位于 Linux PAM 认证域）
//   - 其他裸用户名 → 自动追加 @pve（PVE 内建认证域，普通账号默认创建在这里）
func normalizePVEUsername(username string) string {
	username = strings.TrimSpace(username)
	if username == "" || strings.Contains(username, "@") {
		return username
	}
	if username == "root" {
		return "root@pam"
	}
	return username + "@pve"
}

// pveLogin 登录 PVE 并返回可复用的 http.Client、认证 ticket（PVEAuthCookie）与 CSRFPreventionToken。
// 登录失败时错误信息附带 HTTP 状态码与响应体，便于区分「用户名/密码错误」「地址不通」「证书问题」等情况。
func pveLogin(endpoint, username, password string, verifySSL bool) (*http.Client, string, string, error) {
	loginURL := strings.TrimRight(endpoint, "/") + "/api2/json/access/ticket"
	loginPayload := map[string]string{
		"username": normalizePVEUsername(username),
		"password": password,
	}
	loginBody, _ := json.Marshal(loginPayload)

	httpClient := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: !verifySSL}},
		Timeout:   30 * time.Second,
	}

	loginReq, err := http.NewRequest("POST", loginURL, bytes.NewBuffer(loginBody))
	if err != nil {
		return nil, "", "", fmt.Errorf("创建PVE登录请求失败: %w", err)
	}
	loginReq.Header.Set("Content-Type", "application/json")

	loginResp, err := httpClient.Do(loginReq)
	if err != nil {
		return nil, "", "", fmt.Errorf("连接PVE服务器失败（请检查地址、网络及SSL证书设置）: %w", err)
	}
	defer loginResp.Body.Close()

	loginRespBody, _ := io.ReadAll(loginResp.Body)
	var loginResult struct {
		Data struct {
			Ticket              string `json:"ticket"`
			CSRFPreventionToken string `json:"CSRFPreventionToken"`
		} `json:"data"`
	}
	if err := json.Unmarshal(loginRespBody, &loginResult); err != nil {
		return nil, "", "", fmt.Errorf("解析PVE登录响应失败: %w（响应: %s）", err, string(loginRespBody))
	}
	if loginResp.StatusCode != http.StatusOK || loginResult.Data.Ticket == "" {
		return nil, "", "", fmt.Errorf(
			"PVE登录失败（HTTP %d，用户: %s）：用户名或密码错误；注意PVE用户名需带认证域后缀（如 1000@pve、root@pam）。响应: %s",
			loginResp.StatusCode, normalizePVEUsername(username), string(loginRespBody))
	}

	return httpClient, loginResult.Data.Ticket, loginResult.Data.CSRFPreventionToken, nil
}

// pveResolveNode 根据 VMID 自动发现其所在节点。
// PVE 集群中 VMID 全局唯一，/cluster/resources?type=vm 会返回每台 VM 的 node 字段。
// 当调用方未传节点名（旧卡片数据缺字段/用户漏填）时用此函数兜底，
// 避免拼出 /nodes//qemu/... 这种空节点路径（PVE 对该路径返回 501）。
// 同时兼容虚拟机在集群内被迁移到其他节点的情况。
func pveResolveNode(httpClient *http.Client, ticket, endpoint string, vmid int) (string, error) {
	resourcesURL := strings.TrimRight(endpoint, "/") + "/api2/json/cluster/resources?type=vm"
	req, err := http.NewRequest("GET", resourcesURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建节点查询请求失败: %w", err)
	}
	req.AddCookie(&http.Cookie{Name: "PVEAuthCookie", Value: ticket})

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("查询PVE节点失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("PVE 资源 API 错误 [%d]: %s", resp.StatusCode, string(body))
	}

	var resources struct {
		Data []struct {
			VMID int    `json:"vmid"`
			Node string `json:"node"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resources); err != nil {
		return "", fmt.Errorf("解析PVE资源响应失败: %w", err)
	}
	for _, r := range resources.Data {
		if r.VMID == vmid && r.Node != "" {
			return r.Node, nil
		}
	}
	return "", fmt.Errorf("在 PVE 集群中未找到 VMID=%d（该账号可能无此虚拟机权限，或虚拟机不存在）", vmid)
}

// ensurePVENode 确保节点名非空：调用方未提供时自动按 VMID 反查。
func ensurePVENode(httpClient *http.Client, ticket, endpoint, node string, vmid int) (string, error) {
	node = strings.TrimSpace(node)
	if node != "" {
		return node, nil
	}
	resolved, err := pveResolveNode(httpClient, ticket, endpoint, vmid)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

// PveVMInfo PVE 上一台虚拟机的摘要信息（用于「查询 PVE 虚拟机」弹窗的列表项）
type PveVMInfo struct {
	VMID   int    `json:"vmid"`
	Name   string `json:"name"`
	Node   string `json:"node"`
	Status string `json:"status"`   // running / stopped / suspended
	Type   string `json:"type"`     // qemu / lxc
	Tags   string `json:"tags,omitempty"`
}

// ListVMsViaPVE 通过 PVE API 拉取该账号可见的所有虚拟机（含 QEMU 和 LXC 容器）
//
// 返回: 每台 VM 的 VMID、名称、所在节点、运行状态；前端直接渲染成下拉列表
//   - pveEndpoint:  PVE 服务器地址，如 https://192.168.11.222:8006
//   - pveUsername:  PVE 登录用户名，如 root@pam、1000@pve
//   - pvePassword:  PVE 登录密码
//   - verifySSL:    是否校验 SSL 证书（自签名证书请传 false）
func (p *ProtocolService) ListVMsViaPVE(pveEndpoint, pveUsername, pvePassword string, verifySSL bool) ([]PveVMInfo, error) {
	// 1. 登录 PVE 获取 ticket（裸用户名自动补全认证域：1000 → 1000@pve，root → root@pam）
	httpClient, ticket, _, err := pveLogin(pveEndpoint, pveUsername, pvePassword, verifySSL)
	if err != nil {
		return nil, err
	}

	// 2. 调用 /api2/json/cluster/resources 拉取该账号有权限访问的所有资源（QEMU VM + LXC）
	//    这个接口会自动根据 ticket 对应账号的 ACL 过滤，只能看到被授权的 VM
	//    注意：路径必须是 /cluster/resources（漏掉 /cluster 会返回 501）
	resourcesURL := strings.TrimRight(pveEndpoint, "/") + "/api2/json/cluster/resources?type=vm"
	req, err := http.NewRequest("GET", resourcesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建查询PVE资源请求失败: %w", err)
	}
	req.AddCookie(&http.Cookie{Name: "PVEAuthCookie", Value: ticket})

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("查询PVE虚拟机列表失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("PVE 资源 API 错误 [%d]: %s", resp.StatusCode, string(respBody))
	}

	var resources struct {
		Data []struct {
			VMID   int    `json:"vmid"`
			Name   string `json:"name"`
			Node   string `json:"node"`
			Status string `json:"status"`
			Type   string `json:"type"`
			Tags   string `json:"tags,omitempty"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &resources); err != nil {
		return nil, fmt.Errorf("解析PVE资源响应失败: %w", err)
	}

	// 组装返回值
	result := make([]PveVMInfo, 0, len(resources.Data))
	for _, r := range resources.Data {
		if r.VMID <= 0 {
			continue
		}
		result = append(result, PveVMInfo{
			VMID:   r.VMID,
			Name:   r.Name,
			Node:   r.Node,
			Status: r.Status,
			Type:   r.Type,
			Tags:   r.Tags,
		})
	}
	// 按 VMID 升序排序，符合 PVE Web 界面的习惯
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].VMID < result[j].VMID
	})
	return result, nil
}

// ---- Hyper-V 电源控制（通过 Windows PowerShell Hyper-V 模块 Cmdlet）----
//  本机：直接 powershell.exe -Command "Start-VM ..."
//  远程（WinRM）：-ComputerName 参数，凭据用 PSCredential 构造

// runHyperVPS 执行 Hyper-V 相关 PowerShell 命令，返回合并后的 stdout/stderr
func runHyperVPS(command string) (string, error) {
	if goruntime.GOOS != "windows" {
		return "", fmt.Errorf("Hyper-V 电源控制仅支持 Windows 系统")
	}
	// ★ 用绝对路径调用 PowerShell，避免精简版 Windows / PATH 被截断时找不到 powershell.exe
	psExe := findPowerShellExe()
	if psExe == "" {
		return "", fmt.Errorf("未找到 PowerShell，请确认系统已安装 Windows PowerShell（通常位于 C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe）")
	}
	// 统一编码，避免乱码
	fullCmd := "$OutputEncoding = [Console]::OutputEncoding = [System.Text.Encoding]::UTF8; " +
		"$ErrorActionPreference = 'Stop'; " + command
	cmd := exec.Command(psExe, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", fullCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW 防弹窗
	}
	stdout, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("Hyper-V PowerShell 错误: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("执行 Hyper-V 命令失败: %w", err)
	}
	return strings.TrimSpace(string(stdout)), nil
}

// buildHyperVHostArg 拼接 -ComputerName 与凭据参数（WinRM 远程场景）
func buildHyperVHostArg(host, username, password string, useWinRM bool) string {
	if !useWinRM {
		// 本机：不传 -ComputerName（空 host 也直接执行本机）
		return ""
	}
	h := host
	if h == "" || h == "." {
		return ""
	}
	// 带用户名密码 → 生成 PSCredential 并通过 Invoke-Command 执行
	if username != "" {
		// 这段字符串会拼进外层 -Command，所以用单引号转义一下敏感字符
		return fmt.Sprintf(
			`$secpass = ConvertTo-SecureString '%s' -AsPlainText -Force; `+
				`$cred = New-Object System.Management.Automation.PSCredential('%s', $secpass); `,
			strings.ReplaceAll(strings.ReplaceAll(password, "'", "''"), "`", "``"),
			strings.ReplaceAll(strings.ReplaceAll(username, "'", "''"), "`", "``"),
		)
	}
	return ""
}

// normalizeHyperVUsername 规范化凭据用户名：
// Hyper-V CIM -Credential 不接受裸用户名，必须是 主机\用户名 或 域\用户名 或 UPN(user@domain)。
// - 本地模式 / 已有 \ 或 @ → 原样
// - 远程模式 + 裸用户名 → 自动补成 「主机名\用户名」
func normalizeHyperVUsername(hypervHost, username string, useWinRM bool) string {
	hypervHost = strings.TrimSpace(hypervHost)
	username = strings.TrimSpace(username)
	if username == "" {
		return ""
	}
	// ★ remote 判断放宽：只要 host 不是 '' 和 '.' 就算远程（不管用户有没有勾选 useWinRM）
	//   避免用户填了 192.168.10.88 但漏勾选 WinRM/远程开关 → 走本机查询当然匹配不到 ceshi → 假死或超时
	remote := hypervHost != "" && hypervHost != "."
	if !remote {
		return username
	}
	if strings.ContainsAny(username, `\@`) {
		return username
	}
	// 没有前缀 → 自动补主机名前缀
	return hypervHost + `\` + username
}

// escapePSQuote 转义 PowerShell 单引号字符串里的特殊字符
func escapePSQuote(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "`", "``"), "'", "''")
}

// escapeWQLFilter 转义 WQL Filter 字符串（Msvm_ComputerSystem ElementName='xxx'）
func escapeWQLFilter(s string) string {
	// WQL 字符串里的单引号和反斜杠转义：' → \' , \ → \\
	r := strings.ReplaceAll(s, `\`, `\\`)
	r = strings.ReplaceAll(r, `'`, `\'`)
	return r
}

// buildHyperVCimSessionHeader 构造 PowerShell 头部（凭据 + CimSession）：
//   - 本地：返回空串 + CimSession 变量为空
//   - 远程（host!=” 且 host!='.'）：建立 DCOM CimSession（不管用户是否勾选 useWinRM，因为主机地址是远程的就必须 DCOM 过去），$s 变量
//     ★ 判断放宽：只要 hyperv_host 填了 IP/主机名（不是”也不是'.'），就强制走 CimSession DCOM 远程。
//     之前"必须 useWinRM=true 才远程"的判断会导致用户漏勾选开关时 → 本机 Hyper-V 查询不到 ceshi → 无意义地等待/RPC报错。
//
// 返回值 (header, sessionArg)：header 是 PS 脚本前缀，sessionArg 是后面每个 CIM cmdlet 要附加的 -CimSession $s 字符串（或空串表示本地）
func buildHyperVCimSessionHeader(hypervHost, username, password string, useWinRM bool) (header string, sessionArg string) {
	hypervHost = strings.TrimSpace(hypervHost)
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	// ★ 关键修复：只要 host 不是空也不是 '.' 就判定为远程（不看 useWinRM 勾选与否）
	remote := hypervHost != "" && hypervHost != "."
	if !remote {
		return "", ""
	}
	hostPS := escapePSQuote(hypervHost)
	// New-CimSessionOption：Protocol Dcom（Hyper-V WMI 走 DCOM，不走 WinRM WSMan）
	// ★ 注意：PowerShell 5.1（Windows 自带版本）的 New-CimSessionOption DcomParameterSet 不支持 -TimeoutSec/-OperationTimeoutSec
	//     任何超时参数都会报 "找不到与参数名称 TimeoutSec 匹配的参数"，所以 Dcom 选项只传 -Protocol Dcom。
	//     会话级别的操作超时统一在后面 New-CimSession 用 -OperationTimeoutSec 30 控制。
	sessionOpt := `New-CimSessionOption -Protocol Dcom`
	// New-CimSession：
	// ★ OperationTimeoutSec 20 → 30：实测 DCOM 首次握手需要 4~6s，给足 30s 防止跨网段 RPC 握手慢
	newCmd := fmt.Sprintf(`New-CimSession -ComputerName '%s' -SessionOption (%s) -OperationTimeoutSec 30 -ErrorAction Stop`, hostPS, sessionOpt)

	normalizedUser := normalizeHyperVUsername(hypervHost, username, useWinRM)
	if normalizedUser != "" {
		// 有凭据
		cred := fmt.Sprintf(
			`$secpass = ConvertTo-SecureString '%s' -AsPlainText -Force; `+
				`$cred = New-Object System.Management.Automation.PSCredential('%s', $secpass); `,
			escapePSQuote(password),
			escapePSQuote(normalizedUser),
		)
		header = cred + fmt.Sprintf(`$s = %s -Credential $cred; `, newCmd)
	} else {
		header = fmt.Sprintf(`$s = %s; `, newCmd)
	}
	sessionArg = ` -CimSession $s`
	return header, sessionArg
}

// ControlVMViaHyperV 通过 Hyper-V WMI (CIM DCOM) 控制虚拟机
// 使用 root\virtualization\v2 命名空间 Msvm_ComputerSystem::RequestStateChange
// action: "start" | "stop" | "reboot" | "suspend" | "resume"
func (p *ProtocolService) ControlVMViaHyperV(hypervHost, hypervVMName string, useWinRM bool, username, password string, forceShutdown bool, action string) error {
	hypervHost = strings.TrimSpace(hypervHost)
	hypervVMName = strings.TrimSpace(hypervVMName)
	username = strings.TrimSpace(username)
	if hypervVMName == "" {
		return fmt.Errorf("Hyper-V 虚拟机名称不能为空")
	}

	// RequestedState 值（来自 Hyper-V Msvm_ComputerSystem.RequestStateChange 文档）
	var requestedState int
	switch action {
	case "start":
		requestedState = 2 // Enabled
	case "stop":
		if forceShutdown {
			requestedState = 3 // Disabled (Turn Off - 硬关)
		} else {
			requestedState = 4 // Shut Down via Integration Services (软关)
		}
	case "reboot":
		if forceShutdown {
			requestedState = 10 // Reset (硬重启)
		} else {
			requestedState = 11 // Reboot via Integration Services (软重启)
		}
	case "suspend":
		requestedState = 32770 // Pause
	case "resume":
		requestedState = 32771 // Resume (from Paused/Saved)
	default:
		return fmt.Errorf("不支持的 Hyper-V 操作: %s", action)
	}

	// ★ 优先用 COM/WMI 直连（不依赖 PowerShell，适用于精简版/无 PowerShell 的系统）
	//    Hyper-V 远程判断基于非空 hypervHost（不是 '.'），与 WinRM 复选框无关
	comErr := hyperVControlViaCOM(hypervHost, hypervVMName, username, password, requestedState)
	if comErr == nil {
		return nil // COM 成功
	}
	fmt.Printf("[ControlVMViaHyperV] COM 失败，尝试 PowerShell fallback: %v\n", comErr)

	// ★ fallback：PowerShell（用绝对路径，避免 PATH 被截断时找不到 powershell.exe）
	if findPowerShellExe() == "" {
		return fmt.Errorf("Hyper-V 操作失败（COM: %v），且系统未安装 PowerShell，无法回退", comErr)
	}

	header, sessionArg := buildHyperVCimSessionHeader(hypervHost, username, password, useWinRM)
	vmFilter := `ElementName='` + escapeWQLFilter(hypervVMName) + `'`
	// 二层转义：vmFilter 内部含 WQL 单引号（例如 ElementName='xxx'），嵌入到 PS 的 -Filter '...' 单引号字符串时，
	// 必须把里面的单引号再转义成 ''（PS 单引号字符串规则），否则会出现 PositionalParameterNotFound 错误。
	vmFilterPS := escapePSQuote(vmFilter)

	// 脚本：建 Session → 查 VM → 发 RequestStateChange → 校验 ReturnValue → 关 Session
	var psCmd string
	if sessionArg == "" {
		// 本地
		psCmd = fmt.Sprintf(
			`$vm = Get-CimInstance -Namespace root\virtualization\v2 -ClassName Msvm_ComputerSystem -Filter '%s' -ErrorAction Stop; `+
				`if (-not $vm) { throw 'Hyper-V 虚拟机不存在' }; `+
				`$r = Invoke-CimMethod -InputObject $vm -MethodName RequestStateChange -Arguments @{ RequestedState = %d } -ErrorAction Stop; `+
				`if ($r.ReturnValue -ne 0 -and $r.ReturnValue -ne 4096) { throw 'Hyper-V 操作失败 ReturnCode=' + $r.ReturnValue }`,
			vmFilterPS, requestedState,
		)
	} else {
		// 远程：finally 里关 CimSession
		psCmd = fmt.Sprintf(
			`try { $vm = Get-CimInstance%s -Namespace root\virtualization\v2 -ClassName Msvm_ComputerSystem -Filter '%s' -ErrorAction Stop; `+
				`if (-not $vm) { throw 'Hyper-V 虚拟机不存在' }; `+
				`$r = Invoke-CimMethod -InputObject $vm -MethodName RequestStateChange -Arguments @{ RequestedState = %d } -ErrorAction Stop; `+
				`if ($r.ReturnValue -ne 0 -and $r.ReturnValue -ne 4096) { throw 'Hyper-V 操作失败 ReturnCode=' + $r.ReturnValue } `+
				`} finally { Remove-CimSession -CimSession $s -ErrorAction SilentlyContinue }`,
			sessionArg, vmFilterPS, requestedState,
		)
	}

	full := header + psCmd
	if _, err := runHyperVPS(full); err != nil {
		return err
	}
	return nil
}

// GetVMStatusViaHyperV 查询 Hyper-V 虚拟机当前状态（running/stopped/paused）
// 使用 root\virtualization\v2 命名空间 Msvm_ComputerSystem::EnabledState
func (p *ProtocolService) GetVMStatusViaHyperV(hypervHost, hypervVMName string, useWinRM bool, username, password string) (string, error) {
	// ★ 和 ControlVMViaHyperV 保持一致：所有入参 TrimSpace，修复 IP/名称末尾空格导致匹配失败/RPC错误
	hypervHost = strings.TrimSpace(hypervHost)
	hypervVMName = strings.TrimSpace(hypervVMName)
	username = strings.TrimSpace(username)
	if hypervVMName == "" {
		return "", fmt.Errorf("Hyper-V 虚拟机名称不能为空")
	}

	// ★ 优先用 COM/WMI 直连（不依赖 PowerShell，适用于精简版/无 PowerShell 的系统）
	enabledState, comErr := hyperVStatusViaCOM(hypervHost, hypervVMName, username, password)
	if comErr == nil {
		return mapEnabledStateToString(enabledState), nil
	}
	fmt.Printf("[GetVMStatusViaHyperV] COM 失败，尝试 PowerShell fallback: %v\n", comErr)

	// ★ fallback：PowerShell
	if findPowerShellExe() == "" {
		return "", fmt.Errorf("Hyper-V 状态查询失败（COM: %v），且系统未安装 PowerShell，无法回退", comErr)
	}

	header, sessionArg := buildHyperVCimSessionHeader(hypervHost, username, password, useWinRM)
	vmFilter := `ElementName='` + escapeWQLFilter(hypervVMName) + `'`
	// 二层转义（同 ControlVMViaHyperV）
	vmFilterPS := escapePSQuote(vmFilter)

	var psCmd string
	if sessionArg == "" {
		psCmd = fmt.Sprintf(
			`$vm = Get-CimInstance -Namespace root\virtualization\v2 -ClassName Msvm_ComputerSystem -Filter '%s' -ErrorAction Stop; `+
				`if (-not $vm) { throw 'Hyper-V 虚拟机不存在' }; `+
				`[int]$vm.EnabledState`,
			vmFilterPS,
		)
	} else {
		// ★ 小优化：try 里只要拿到 $vm 就立刻输出 EnabledState，
		//   finally Remove-CimSession 执行（关闭 RPC 通道）还需要几百毫秒，
		//   写在前面可以让 stdout 先 flush，Go 端 runHyperVPS 提前几毫秒收到输出。
		psCmd = fmt.Sprintf(
			`try { $vm = Get-CimInstance%s -Namespace root\virtualization\v2 -ClassName Msvm_ComputerSystem -Filter '%s' -ErrorAction Stop; `+
				`if (-not $vm) { throw 'Hyper-V 虚拟机不存在' }; `+
				`[int]$vm.EnabledState `+
				`} finally { Remove-CimSession -CimSession $s -ErrorAction SilentlyContinue }`,
			sessionArg, vmFilterPS,
		)
	}

	psCmd = header + psCmd
	out, err := runHyperVPS(psCmd)
	if err != nil {
		fmt.Printf("[GetVMStatusViaHyperV] host=%s vm=%s PowerShell 失败: %v 输出=%q\n", hypervHost, hypervVMName, err, out)
		return "", err
	}

	// ★ 修复 Out-String 含多余空白/换行导致字符串 switch 失配：
	//   先正则提取所有数字再转 int，用整数比较更稳妥
	s := strings.TrimSpace(out)
	numRe := regexp.MustCompile(`\d+`)
	matches := numRe.FindStringSubmatch(s)
	if len(matches) == 0 {
		fmt.Printf("[GetVMStatusViaHyperV] host=%s vm=%s 无法解析输出为整数: out=%q trimmed=%q\n", hypervHost, hypervVMName, out, s)
		return "", fmt.Errorf("无法解析 Hyper-V 虚拟机状态: %q", s)
	}
	state, atoiErr := strconv.Atoi(matches[0])
	if atoiErr != nil {
		fmt.Printf("[GetVMStatusViaHyperV] host=%s vm=%s 整数转换失败: %v raw=%q\n", hypervHost, hypervVMName, atoiErr, s)
		return "", fmt.Errorf("无法解析 Hyper-V 虚拟机状态值: %q", s)
	}
	fmt.Printf("[GetVMStatusViaHyperV] host=%s vm=%s EnabledState=%d (raw out=%q)\n", hypervHost, hypervVMName, state, out)

	// Msvm_ComputerSystem.EnabledState 值来自 Hyper-V WMI 文档：
	//   2=Enabled(Running), 3=Disabled(Off), 32768=Paused, 32769=Saved,
	//   32770=Starting, 32771=Not Applicable / Resuming, 32773=Saving,
	//   32774=Stopping, 32776=Pausing, 32777=Resuming
	switch state {
	case 2, 32770, 32774, 32773, 32776, 32777, 32771:
		// Running / Starting / Stopping / Saving / Pausing / Resuming / 其他瞬时中间态：统一显示 running
		return "running", nil
	case 3:
		return "stopped", nil
	case 32769, 32768:
		return "paused", nil
	default:
		// 未知状态不要默认 stopped，返回空字符串让前端 fallback 用本地缓存状态，并打印日志让管理员排查
		fmt.Printf("[GetVMStatusViaHyperV] host=%s vm=%s 未知 EnabledState=%d → 保持本地状态\n", hypervHost, hypervVMName, state)
		return "", nil
	}
}

// CheckRDPPortStatus 通过 TCP 检测 RDP 端口判断虚拟机状态
// 用于没有配置 power_backend 的 RDP/FreeRDP 虚拟机
func (p *ProtocolService) CheckRDPPortStatus(host string, port int) (string, error) {
	return checkRDPPort(host, port, 3*time.Second)
}

// CheckAnyPortStatus 通过多端口探测判断虚拟机是否在线
// 用于没有配置 power_backend 的 Horizon/SPICE/VNC 虚拟机
// 依次探测 3389/RDP → 443/HTTPS → 5900/VNC，有一个通即算 running
func (p *ProtocolService) CheckAnyPortStatus(host string) (string, error) {
	return checkAnyPort(host)
}

func (p *ProtocolService) LaunchVNC(host string, port int, password string) error {
	config := &protocol.ConnectionConfig{
		Host:     host,
		Port:     port,
		Password: password,
	}
	return p.manager.LaunchVNC(config)
}

func (p *ProtocolService) LaunchHorizon(brokerURL string, username string, password string, domain string) error {
	config := &protocol.ConnectionConfig{
		Host:       domain,
		Port:       443,
		Username:   username,
		VMPassword: password,
		BrokerURL:  brokerURL,
	}
	return p.manager.LaunchHorizon(config)
}

func (p *ProtocolService) LaunchProtocolFull(protocolType string, host string, port int, username string, password string, tlsPort int, brokerURL string, width int, height int, fullscreen bool, colorDepth int, audio bool, clipboard bool, compression bool, driveMapping bool, printer bool, smartcard bool, hideClient bool, hideToolbar bool, blockAltF4 bool, usbRedirect bool, usbDevices string, usbMode string, multiMon bool) error {
	// ★ 调试日志：打印前端传递的参数
	fmt.Printf("[LaunchProtocolFull] proto=%s host=%s:%d fullscreen=%v width=%d height=%d hideToolbar=%v blockAltF4=%v multiMon=%v\n",
		protocolType, host, port, fullscreen, width, height, hideToolbar, blockAltF4, multiMon)

	config := &protocol.ConnectionConfig{
		Host:         host,
		Port:         port,
		Username:     username,
		VMPassword:   password,
		Password:     password,
		TLSPort:      tlsPort,
		BrokerURL:    brokerURL,
		Width:        width,
		Height:       height,
		Fullscreen:   fullscreen,
		HideClient:   hideClient,
		HideToolbar:  hideToolbar,
		MultiMon:     multiMon,
		BlockAltF4:   blockAltF4,
		ColorDepth:   colorDepth,
		Audio:        audio,
		Clipboard:    clipboard,
		Compression:  compression,
		DriveMapping: driveMapping,
		Printer:      printer,
		Smartcard:    smartcard,
		USBRedirect:  usbRedirect,
		USBMode:      usbMode,
		USBDevices:   usbDevices,
	}

	var proto protocol.ProtocolType
	switch protocolType {
	case "rdp":
		proto = protocol.ProtocolRDP
	case "spice":
		proto = protocol.ProtocolSPICE
	case "vnc":
		proto = protocol.ProtocolVNC
	case "ssh":
		proto = protocol.ProtocolSSH
	case "horizon":
		proto = protocol.ProtocolHorizon
	case "windowsrdp":
		// Windows 远程桌面（mstsc.exe + RDP 文件）不走 manager
		// 此协议仅 Windows 平台可用，非 Windows 直接报错返回
		if goruntime.GOOS != "windows" {
			return fmt.Errorf("Windows RDP 协议仅支持 Windows 系统")
		}
	default:
		return fmt.Errorf("不支持的协议类型: %s", protocolType)
	}

	// ★ 启动远程桌面前：
	//   【条件化 Kiosk 激活】—— 只在原本就是 Kiosk 模式（启动参数 / 开机 Kiosk 守护 / 管理设置强制全屏）
	//   的情况下才激活 Kiosk 独占 + 全局键盘拦截；普通非 Kiosk 用户点一键连接时
	//   绝对不要动主窗口尺寸/最大化/全屏/WS_SYSMENU，否则会导致 1100×790 等小分辨率
	//   下主窗口被 WindowFullscreen 强行拉成屏幕大小（伪最大化）+ 封掉 X 号 +
	//   退出后 WindowUnfullscreen 无法还原原尺寸，用户看到的是"程序变最大化且无法还原，X号失效"。
	wasKiosk := p.app.isKioskFlagOnDisk()
	if wasKiosk && p.app.ctx != nil {
		p.app.ApplyKioskMode()
	}
	// 全局键盘拦截（ALT+F4/Win/Alt+Tab）同样仅在 Kiosk 模式激活：
	//   非 Kiosk 用户有系统标题栏/任务栏/最小化/关闭按钮，不需要拦截快捷键，
	//   拦截反而容易让用户误以为程序卡死或快捷键失灵。
	if wasKiosk {
		activateGlobalKeyboardBlock()
	}

	// ★ 暂停 Kiosk daemon：防止主窗口守护器（每 800ms SetForegroundWindow）
	//   与远程桌面客户端（Horizon/FreeRDP）抢前台，导致任务栏反复闪烁。
	//   远程桌面退出后在下方 goroutine 中恢复。
	platformPauseKioskDaemon()

	var err error
	if protocolType == "windowsrdp" {
		err = p.LaunchWindowsRDP(config)
	} else {
		err = p.manager.LaunchProtocol(proto, config)
	}

	// ★ 启动失败：保持 Kiosk 状态，不退出
	if err != nil {
		// 启动失败时恢复 Kiosk daemon（上面暂停了，但远程桌面没启动成功不会走退出恢复逻辑）
		platformResumeKioskDaemon(wasKiosk)
		return err
	}

	// ★ 启动成功：主窗口保持全屏独占，键盘钩子持续拦截
	// goroutine 监控远程桌面退出，退出后恢复正常状态
	if p.app.ctx != nil {
		go func() {
			defer func() { _ = recover() }()
			var procNames []string
			switch proto {
			case protocol.ProtocolRDP:
				procNames = []string{"sdl-freerdp", "wfreerdp", "freerdp"}
			case protocol.ProtocolHorizon:
				procNames = []string{"horizon-client", "vmware-view"}
			case protocol.ProtocolSPICE:
				procNames = []string{"remote-viewer", "spice"}
			case protocol.ProtocolVNC:
				procNames = []string{"vncviewer", "vnc"}
			}
			if protocolType == "windowsrdp" {
				procNames = []string{"mstsc"}
			}
			// 先等 3 秒让远程桌面进程稳定启动
			time.Sleep(3 * time.Second)
			// 监控远程桌面进程退出（最多 2 小时）
			for i := 0; i < 3600; i++ {
				running := false
				for _, name := range procNames {
					if isProcessRunning(name) {
						running = true
						break
					}
				}
				if !running {
					break
				}
				time.Sleep(2 * time.Second)
			}
			// 远程桌面已退出：
			//   - 如果之前是 Kiosk 模式，必须严格重新回到"强制全屏锁死"状态：
			//       ① 重新全屏 + 去 WS_SYSMENU（防止远程桌面退出后 WebView2 恢复边框/菜单）
			//       ② 隐藏任务栏（防止远程桌面期间 Explorer 被唤醒后把任务栏又显示出来）
			//       ③ 置 TOPMOST + 强制拉回前台（防止用户在远程退出瞬间点了显示桌面/切窗口导致主窗口最小化）
			//       ④ 保持键盘拦截仍然生效
			//   - 如果之前不是 Kiosk 模式，什么都不用做：
			//     因为上面启动时已经不再 ApplyKioskMode / activateGlobalKeyboardBlock，
			//     主窗口尺寸/系统菜单/键盘状态本来就没被改，所以这里不需要 ExitKioskMode
			//     （避免让 1100×790 用户在一次正常连接后被 ExitKioskMode 触发 WindowUnfullscreen）
			if wasKiosk {
				// ① 重新全屏 + 刷无边框 + 封 WS_SYSMENU
				if p.app.ctx != nil {
					runtime.WindowShow(p.app.ctx)
					runtime.WindowFullscreen(p.app.ctx)
				}
				platformApplyKioskFrameless()
				if p.app.ctx != nil {
					runtime.WindowFullscreen(p.app.ctx)
				}
				platformLockdownMainWindowKioskMode()
				// ② 重新隐藏任务栏（远程桌面期间可能被 Explorer 唤醒显示）
				hideTaskbarWindows()
				// ③ 置 TOPMOST + 强行拉回前台（防止远程桌面退出瞬间主窗口被最小化）
				ensureKioskWindowOnTopAndFront()
				bringWindowToFront()
				// ④ 保持键盘拦截仍然生效
				activateGlobalKeyboardBlock()
				// ⑤ 恢复 Kiosk daemon（远程桌面期间已暂停，现在远程桌面退出了，恢复守护）
				platformResumeKioskDaemon(true)
			}
		}()
	}

	return err
}

func (p *ProtocolService) LaunchProtocol(protocolType string, host string, port int, password string, tlsPort int) error {
	return p.LaunchProtocolFull(protocolType, host, port, "", password, tlsPort, "", 0, 0, false, 32, true, true, true, false, false, false, false, false, false, false, "", "auto", false)
}

// LaunchWindowsRDP 生成 Windows 官方 RDP 文件并通过 mstsc.exe 一键连接
// 生成的 .rdp 文件保存在 <软件exe目录>/rdp_files/<主机名-时间戳>.rdp，方便用户回看/编辑
// 用户名/密码通过 cmdkey 临时写入 Windows 凭据管理器（TERMSRV/<host>），实现免输入自动登录
// 配置开关对应：全屏模式 / 多显示器 / 音频 / 剪贴板 / 驱动器 / 打印机 / 智能卡 / 压缩 / 颜色深度 / 分辨率 / 顶部连接栏
func (p *ProtocolService) LaunchWindowsRDP(config *protocol.ConnectionConfig) error {
	if goruntime.GOOS != "windows" {
		return fmt.Errorf("Windows RDP 协议仅支持 Windows 系统")
	}
	if config.Port <= 0 {
		config.Port = 3389
	}

	// 1. 确定 RDP 文件存放目录：<EXE 同级>/rdp_files/，目录不存在则创建
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	appDir := filepath.Dir(exe)
	rdpDir := filepath.Join(appDir, "rdp_files")
	if err := os.MkdirAll(rdpDir, 0755); err != nil {
		return fmt.Errorf("创建 rdp_files 目录失败: %w", err)
	}

	// 2. 生成唯一 RDP 文件名：主机名-IP-端口-时间戳.rdp，非法字符替换为 "_"
	safeHost := strings.ReplaceAll(config.Host, ":", "_")
	safeHost = strings.ReplaceAll(safeHost, "/", "_")
	safeHost = strings.ReplaceAll(safeHost, "\\", "_")
	fileName := fmt.Sprintf("%s-%d-%d.rdp", safeHost, config.Port, time.Now().Unix())
	fileName = strings.ReplaceAll(fileName, "__", "_")
	rdpPath := filepath.Join(rdpDir, fileName)

	// ★ 兜底规则（project_memory 硬约束）：前端异步加载配置时，
	//    fullscreen 可能偶发为 false 但 width=0 且 height=0（用户明明勾选了"全屏"）
	//    → 后端必须强制走全屏模式，否则 mstsc 会打开一个小窗口（windowsrdp 默认用户就是要全屏）
	if !config.Fullscreen && config.Width <= 0 && config.Height <= 0 {
		fmt.Printf("[LaunchWindowsRDP] 应用兜底规则：fullscreen=false 且 width=height=0 → 强制全屏\n")
		config.Fullscreen = true
	}

	// 3. 拼接 RDP 文件内容（Windows .rdp 文件为 UTF-16 LE + BOM，但纯 ASCII 内容也可正常识别）
	//   screen mode id : 1=窗口, 2=全屏
	screenMode := 1
	if config.Fullscreen {
		screenMode = 2
	}
	//   use multimon : 多显示器支持
	useMultiMon := 0
	// 这里用 HideToolbar 字段的布尔取反来承载「多显示器」开关（避免向 ConnectionConfig 新增字段影响 manager）
	if config.HideToolbar {
		useMultiMon = 1
	}
	//   分辨率（非全屏生效；全屏模式 mstsc 会按主显示器实际分辨率适配，写死反而会强制缩放产生黑边）
	width := config.Width
	height := config.Height
	if !config.Fullscreen {
		if width <= 0 {
			width = 1920
		}
		if height <= 0 {
			height = 1080
		}
	}
	//   颜色深度
	bpp := config.ColorDepth
	if bpp <= 0 {
		bpp = 32
	}
	//   音频：0=带到本机，1=留在服务器，2=不播放
	audioMode := 2
	if config.Audio {
		audioMode = 0
	}
	//   显示顶部连接栏
	connBar := 0
	// 这里借用 BlockAltF4 字段的取反来承载「顶部连接栏」开关
	if !config.BlockAltF4 {
		connBar = 1
	}
	rdpLines := []string{
		"screen mode id:i:" + itoa(screenMode),
		"use multimon:i:" + itoa(useMultiMon),
		"desktopwidth:i:" + itoa(width),
		"desktopheight:i:" + itoa(height),
		"session bpp:i:" + itoa(bpp),
		"winposstr:s:0,3,0,0,800,600",
		"compression:i:" + boolInt(config.Compression),
		"keyboardhook:i:2",
		"audiocapturemode:i:0",
		"videoplaybackmode:i:1",
		"connection type:i:7",
		"networkautodetect:i:1",
		"bandwidthautodetect:i:1",
		"displayconnectionbar:i:" + itoa(connBar),
		"enableworkspacereconnect:i:0",
		"disable wallpaper:i:0",
		"allow font smoothing:i:1",
		"allow desktop composition:i:0",
		"disable full window drag:i:1",
		"disable menu anims:i:1",
		"disable themes:i:0",
		"disable cursor setting:i:0",
		"bitmapcachepersistenable:i:1",
		"full address:s:" + config.Host + ":" + itoa(config.Port),
		"audiomode:i:" + itoa(audioMode),
		"redirectprinters:i:" + boolInt(config.Printer),
		"redirectcomports:i:0",
		"redirectsmartcards:i:" + boolInt(config.Smartcard),
		"redirectclipboard:i:" + boolInt(config.Clipboard),
		"redirectposdevices:i:0",
		"drivestoredirect:s:" + mapBoolToAll(config.DriveMapping),
		"autoreconnection enabled:i:1",
		"authentication level:i:0",
		"prompt for credentials:i:0",
		"negotiate security layer:i:1",
		"remoteapplicationmode:i:0",
		"alternate shell:s:",
		"shell working directory:s:",
		"gatewayhostname:s:",
		"gatewayusagemethod:i:4",
		"gatewaycredentialssource:i:4",
		"gatewayprofileusagemethod:i:0",
		"promptcredentialonce:i:0",
		"use redirection server name:i:0",
		"rdgiskdcproxy:i:0",
		"kdcproxyname:s:",
	}
	if config.Username != "" {
		rdpLines = append(rdpLines, "username:s:"+config.Username)
	}
	// ★ 全屏模式下：移除会覆盖 /f 全屏参数的三行，否则 mstsc 会强制按 1920x1080/800x600
	//   的窗口尺寸启动，导致"明明设了全屏但仍然是窗口/居中假全屏"
	if config.Fullscreen {
		filtered := make([]string, 0, len(rdpLines))
		for _, line := range rdpLines {
			if strings.HasPrefix(line, "desktopwidth:") ||
				strings.HasPrefix(line, "desktopheight:") ||
				strings.HasPrefix(line, "winposstr:") {
				continue
			}
			filtered = append(filtered, line)
		}
		rdpLines = filtered
	}
	rdpContent := strings.Join(rdpLines, "\r\n")
	// 调试日志：打印决定全屏/分辨率的关键参数，便于排查"为什么仍然是窗口模式"
	fmt.Printf("[LaunchWindowsRDP] final: fullscreen=%v screenMode=%d width=%d height=%d multimon=%d args(has /f later)=%v\n",
		config.Fullscreen, screenMode, width, height, useMultiMon, config.Fullscreen)
	// 写入 UTF-8（带 BOM，防止 Windows 自带记事本编辑后乱码，mstsc 支持 UTF-8 BOM）
	utf8Bom := []byte{0xEF, 0xBB, 0xBF}
	body := append(utf8Bom, []byte(rdpContent)...)
	if err := os.WriteFile(rdpPath, body, 0644); err != nil {
		return fmt.Errorf("写入 RDP 文件失败 [%s]: %w", rdpPath, err)
	}

	// 3.5. 消除 mstsc.exe 「未知发布者 / 未知远程连接」安全警告弹窗（四步组合）：
	//      A) 删除 .rdp 文件的"Zone.Identifier"备用数据流，避免 Windows SmartScreen 把它标记为"来自互联网"
	//      B) 写 HKCU 注册表（调用 reg.exe 直接执行，路径空格由 exec.Command 自动引用）
	//         - Terminal Server Client\PromptForUnknownPublisher=0 → 关闭未签名 RDP 发布者询问
	//         - Terminal Server Client\PromptForCredSharing=0     → 关闭凭据共享提示
	//         - Terminal Server Client\LocalDevices\<host>[:port]=1 → 目标主机加入受信任本地设备
	//         - Policies\Windows NT\Terminal Services\fUnsignedFilePromptBehavior=0 → GPO 级不提示
	//         - Policies\Associations\LowRiskFileTypes=".rdp"     → Shell 附件服务把 .rdp 视为低风险
	//         - Policies\Attachments\SaveZoneInformation=0        → 不为下载/本地生成文件添加 Mark-of-the-Web
	//      C) 复制配置到 %USERPROFILE%\Documents\Default.rdp（mstsc 内置默认配置文件，永不会触发发布者校验）
	//      D) 启动 mstsc.exe 时【不】传入 .rdp 文件参数，改用 /v:server:port /f /multimon 直接连服务器；
	//         mstsc 会自动加载 Default.rdp 中的音频/剪贴板/驱动器映射等高级配置，且完全绕过发布者警告弹窗。
	runCmd := func(name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.SysProcAttr = sysProcAttrNoWindow()
		_ = cmd.Run()
	}
	runCmd("cmd", "/c", "del", "/q", "/f", rdpPath+":Zone.Identifier")
	runCmd("cmd", "/c", "del", "/q", "/f", rdpPath+":Zone.Identifier:$DATA")

	runReg := func(args ...string) {
		// 直接调用 reg.exe，由 exec.Command 负责对含空格的路径正确加引号
		cmd := exec.Command("reg", args...)
		cmd.SysProcAttr = sysProcAttrNoWindow()
		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("[LaunchWindowsRDP] reg %v 信息: %v out=%s\n", args, err, string(out))
		}
	}
	tscKey := `HKCU\Software\Microsoft\Terminal Server Client`
	runReg("add", tscKey, "/v", "PromptForUnknownPublisher", "/t", "REG_DWORD", "/d", "0", "/f")
	runReg("add", tscKey, "/v", "PromptForCredSharing", "/t", "REG_DWORD", "/d", "0", "/f")
	localDev := tscKey + `\LocalDevices`
	runReg("add", localDev, "/v", config.Host, "/t", "REG_DWORD", "/d", "1", "/f")
	if config.Port > 0 && config.Port != 3389 {
		runReg("add", localDev, "/v", fmt.Sprintf("%s:%d", config.Host, config.Port), "/t", "REG_DWORD", "/d", "1", "/f")
	}
	// GPO: 未签名 .rdp 文件提示行为 = 0（安全打开，不提示）
	runReg("add", `HKCU\Software\Policies\Microsoft\Windows NT\Terminal Services`,
		"/v", "fUnsignedFilePromptBehavior", "/t", "REG_DWORD", "/d", "0", "/f")
	// Shell 附件执行服务：把 .rdp 加入低风险扩展名列表，彻底关闭"发布者无法验证"系统级对话框
	runReg("add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Policies\Associations`,
		"/v", "LowRiskFileTypes", "/t", "REG_SZ", "/d", ".rdp", "/f")
	// 不为本地/下载文件写入 Zone.Identifier ADS（避免 Mark-of-the-Web 触发 Shell 警告）
	runReg("add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Policies\Attachments`,
		"/v", "SaveZoneInformation", "/t", "REG_DWORD", "/d", "1", "/f")

	// C) 写入 Default.rdp —— mstsc.exe 自己的用户级默认配置，加载时永远不做发布者签名校验
	homeDir, homeErr := os.UserHomeDir()
	if homeErr == nil && homeDir != "" {
		defaultRdp := filepath.Join(homeDir, "Documents", "Default.rdp")
		if stat, statErr := os.Stat(filepath.Join(homeDir, "Documents")); statErr != nil || !stat.IsDir() {
			defaultRdp = filepath.Join(homeDir, "Default.rdp")
		}
		if writeErr := os.WriteFile(defaultRdp, body, 0644); writeErr != nil {
			fmt.Printf("[LaunchWindowsRDP] 写 Default.rdp 警告: %s\n", writeErr.Error())
		} else {
			// 同样清掉 Default.rdp 上可能存在的 Zone.Identifier
			runCmd("cmd", "/c", "del", "/q", "/f", defaultRdp+":Zone.Identifier")
		}
	}

	// 4. 如果启用了自动注入凭据（HideClient 字段客串开关）且用户名+密码均非空，
	//    通过 cmdkey.exe 将凭据写入 Windows 凭据管理器
	//    目标名：TERMSRV/<host> 或 TERMSRV/<host>:port —— 这样 mstsc 会直接自动匹配使用
	cmdkeyTarget := "TERMSRV/" + config.Host
	if config.Port != 3389 {
		cmdkeyTarget = "TERMSRV/" + config.Host + ":" + itoa(config.Port)
	}
	if config.HideClient && config.Username != "" && config.VMPassword != "" {
		// 先删除可能已存在的旧凭据（忽略错误）
		cmd := exec.Command("cmdkey", "/delete", cmdkeyTarget)
		cmd.SysProcAttr = sysProcAttrNoWindow()
		_ = cmd.Run()
		// 再添加新凭据
		userPart := config.Username
		if config.BrokerURL != "" {
			// BrokerURL 字段客串保存「域名」（前端 Horizon 使用了此语义，复用避免改 ConnectionConfig）
			userPart = config.BrokerURL + "\\" + config.Username
		}
		cmd = exec.Command("cmdkey", "/generic", cmdkeyTarget, "/user:"+userPart, "/pass:"+config.VMPassword)
		cmd.SysProcAttr = sysProcAttrNoWindow()
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("[LaunchWindowsRDP] cmdkey 添加凭据警告: %s (output=%s)\n", err.Error(), string(out))
			// 凭据注入失败不阻塞连接，让用户手动输入
		}
	}

	// 5. 启动 mstsc.exe —— 关键：不传任何 .rdp 文件参数！
	//    mstsc 自动加载 %USERPROFILE%\Documents\Default.rdp（或用户根目录 Default.rdp），
	//    这个内置默认配置文件是 mstsc 自己维护的，永远不触发"发布者未知"签名校验。
	//    命令行只传：/v:server[:port]  + 可选的 /f（全屏，如配置为全屏可省略，因为 Default.rdp 已有）。
	if config.Host == "" {
		return fmt.Errorf("Windows RDP 连接失败：主机地址不能为空")
	}
	mstscPath, lookErr := exec.LookPath("mstsc.exe")
	if lookErr != nil {
		sys32 := filepath.Join(os.Getenv("windir"), "System32", "mstsc.exe")
		if _, statErr := os.Stat(sys32); statErr == nil {
			mstscPath = sys32
		} else {
			return fmt.Errorf("未找到 mstsc.exe，请确认系统为 Windows 专业版/企业版且远程桌面服务已启用: %w", lookErr)
		}
	}
	hostPort := config.Host
	if config.Port > 0 && config.Port != 3389 {
		hostPort = fmt.Sprintf("%s:%d", config.Host, config.Port)
	}
	args := []string{"/v:" + hostPort}
	// 根据 windowsrdp_fullscreen / windowsrdp_multiMonitor / windowsrdp_useSavedResolution 等参数，
	// 附加 /f 或 /multimon 命令行开关作为显式覆盖（即使 Default.rdp 已写入对应字段，这里再次明确指定）
	if fullscreenVal := config.Fullscreen; fullscreenVal {
		args = append(args, "/f")
	}
	// 复用其他 ConnectionConfig bool 字段客串承载 windowsrdp 多显示器参数：
	// ConnectionConfig 没有专门的 windowsrdp 多显示器字段，但在 App.tsx handleConnect 中：
	//   hideToolbar = 多显示器；blockAltF4 = !显示顶部连接栏；hideClient = 自动注入凭据
	// 这里只要把相关命令行开关在 Default.rdp 里写了就会生效，下面仅在有必要时额外加。
	// 启动命令（不传递 .rdp 文件）：
	launchCmd := exec.Command(mstscPath, args...)
	// mstsc.exe 是 GUI 程序，不能用 HideWindow=true（会导致窗口不显示）
	// 只设 CREATE_NO_WINDOW 防止可能的控制台附属窗口
	launchCmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	if err := launchCmd.Start(); err != nil {
		return fmt.Errorf("启动 mstsc.exe 失败: %w", err)
	}

	// 关键修复：检测 mstsc.exe 是否在启动后立即退出（静默失败）
	// mstsc.exe 可能因目标地址不可达、凭据错误等原因启动后立即退出，导致无任何弹窗但连接失败。
	// 这里通过 Wait() 协程和定时器来检测这一情况。
	done := make(chan error, 1)
	go func() {
		done <- launchCmd.Wait()
	}()

	// 等待最多 3 秒让 mstsc.exe 完成启动
	select {
	case err := <-done:
		// mstsc.exe 在 3 秒内退出了
		if err != nil {
			fmt.Printf("[LaunchWindowsRDP] mstsc.exe 启动后立即退出，错误: %v\n", err)
			return fmt.Errorf("Windows RDP 启动失败：mstsc.exe 启动后立即退出。请检查目标主机是否可达、凭据是否正确，或远程桌面服务是否启用。")
		}
		// mstsc.exe 成功完成并退出（很少发生，但以防万一）
		fmt.Printf("[LaunchWindowsRDP] mstsc.exe 启动后立即完成\n")
		return nil
	case <-time.After(3 * time.Second):
		// 3 秒后 mstsc.exe 仍在运行，说明启动成功
		fmt.Printf("[LaunchWindowsRDP] 已启动 mstsc: %s %v (Default.rdp 已写入高级配置)\n", mstscPath, args)
		return nil
	}
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

func boolInt(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func mapBoolToAll(b bool) string {
	if b {
		return "*"
	}
	return ""
}

// sysProcAttrNoWindow 返回无窗口的进程创建属性（避免弹黑色 cmd 窗口）
// 具体实现在平台专用文件（app_windows_extra.go / app_other.go）
var sysProcAttrNoWindow = func() *syscall.SysProcAttr { return nil }

// ProtocolDependencyCheckResult 用于返回单个协议的依赖检测结果
// Wails v2 的 Go→JS 绑定对多返回值解析不稳定，统一使用结构体返回避免 Promise 永远 pending
type ProtocolDependencyCheckResult struct {
	Available bool   `json:"available"`
	Message   string `json:"message"`
}

func (p *ProtocolService) CheckDependency(protocolType string) (*ProtocolDependencyCheckResult, error) {
	if protocolType == "windowsrdp" {
		if goruntime.GOOS != "windows" {
			return &ProtocolDependencyCheckResult{Available: false, Message: "仅支持 Windows 系统"}, nil
		}
		sys32 := filepath.Join(os.Getenv("windir"), "System32", "mstsc.exe")
		if _, err := os.Stat(sys32); err == nil {
			return &ProtocolDependencyCheckResult{Available: true, Message: "mstsc.exe 就绪（Windows RDP 客户端）"}, nil
		}
		if _, lookErr := exec.LookPath("mstsc.exe"); lookErr == nil {
			return &ProtocolDependencyCheckResult{Available: true, Message: "mstsc.exe 就绪"}, nil
		}
		return &ProtocolDependencyCheckResult{Available: false, Message: "未找到 mstsc.exe，请确认系统为 Windows 专业版/企业版，并在「此电脑-属性-远程设置」中启用远程桌面"}, nil
	}
	var proto protocol.ProtocolType
	switch protocolType {
	case "rdp":
		proto = protocol.ProtocolRDP
	case "spice":
		proto = protocol.ProtocolSPICE
	case "vnc":
		proto = protocol.ProtocolVNC
	case "horizon":
		proto = protocol.ProtocolHorizon
	default:
		return nil, fmt.Errorf("不支持的协议类型: %s", protocolType)
	}

	avail, msg, err := p.manager.CheckProtocolDependency(proto)
	if err != nil {
		return nil, err
	}
	return &ProtocolDependencyCheckResult{Available: avail, Message: msg}, nil
}

func (p *ProtocolService) GetConfigTemplate(protocolType string, host string, port int, password string, tlsPort int) (string, error) {
	config := &protocol.ConnectionConfig{
		Host:     host,
		Port:     port,
		Password: password,
		TLSPort:  tlsPort,
	}

	var proto protocol.ProtocolType
	switch protocolType {
	case "rdp":
		proto = protocol.ProtocolRDP
	case "windowsrdp":
		// Windows 远程桌面：返回一份标准 RDP 文件内容模板
		if port <= 0 {
			port = 3389
		}
		tmpl := "screen mode id:i:2\r\n" +
			"desktopwidth:i:1920\r\n" +
			"desktopheight:i:1080\r\n" +
			"session bpp:i:32\r\n" +
			"compression:i:1\r\n" +
			"keyboardhook:i:2\r\n" +
			"audiomode:i:0\r\n" +
			"redirectprinters:i:0\r\n" +
			"redirectsmartcards:i:0\r\n" +
			"redirectclipboard:i:1\r\n" +
			"drivestoredirect:s:*\r\n" +
			"autoreconnection enabled:i:1\r\n" +
			"authentication level:i:0\r\n" +
			"prompt for credentials:i:0\r\n" +
			"negotiate security layer:i:1\r\n" +
			fmt.Sprintf("full address:s:%s:%d\r\n", host, port)
		return tmpl, nil
	case "spice":
		proto = protocol.ProtocolSPICE
	case "horizon":
		proto = protocol.ProtocolHorizon
	default:
		return "", fmt.Errorf("不支持的协议类型: %s", protocolType)
	}

	return p.manager.GetProtocolConfigTemplate(proto, config)
}
