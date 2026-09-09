//go:build windows

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

// findPowerShellExe 查找 PowerShell 可执行文件的绝对路径
// 优先级：System32 标准路径 → SysWOW64 → PATH 查找 → pwsh.exe (PowerShell 7+)
// 某些精简版 Windows 或 PATH 被截断的系统中，"powershell.exe" 不在 %PATH% 里，
// exec.Command("powershell.exe", ...) 会报 "executable file not found in %PATH%"。
// 用绝对路径调用可彻底规避此问题。
var cachedPowerShellPath string

func findPowerShellExe() string {
	if cachedPowerShellPath != "" {
		return cachedPowerShellPath
	}
	// 1. System32 标准路径（最常见，几乎所有 Windows 都有）
	candidates := []string{
		`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`,
		`C:\Windows\SysWOW64\WindowsPowerShell\v1.0\powershell.exe`,
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			cachedPowerShellPath = p
			return p
		}
	}
	// 2. PATH 查找 powershell.exe
	if p, err := exec.LookPath("powershell.exe"); err == nil {
		cachedPowerShellPath = p
		return p
	}
	// 3. PATH 查找 powershell（不带 .exe 后缀，某些系统）
	if p, err := exec.LookPath("powershell"); err == nil {
		cachedPowerShellPath = p
		return p
	}
	// 4. PowerShell 7+ Core (pwsh.exe)
	if p, err := exec.LookPath("pwsh.exe"); err == nil {
		cachedPowerShellPath = p
		return p
	}
	return ""
}

var (
	modkernel32       = syscall.NewLazyDLL("kernel32.dll")
	_procCreateMutexW = modkernel32.NewProc("CreateMutexW")
	_procReleaseMutex = modkernel32.NewProc("ReleaseMutex")
	_procCloseHandle  = modkernel32.NewProc("CloseHandle")
	_procOpenMutexW   = modkernel32.NewProc("OpenMutexW")

	// Toolhelp32 快照（用于 isProcessRunning 检查远程桌面进程是否退出）
	_procCreateToolhelp32Snapshot = modkernel32.NewProc("CreateToolhelp32Snapshot")
	_procProcess32FirstW          = modkernel32.NewProc("Process32FirstW")
	_procProcess32NextW           = modkernel32.NewProc("Process32NextW")

	// 窗口前台激活
	_procSetForegroundWindow = moduser32.NewProc("SetForegroundWindow")
	_procShowWindow          = moduser32.NewProc("ShowWindow")
	_procIsIconic            = moduser32.NewProc("IsIconic")
	_procBringWindowToTop    = moduser32.NewProc("BringWindowToTop")

	moduser32                      = syscall.NewLazyDLL("user32.dll")
	_procEnumWindowsSingleInstance = moduser32.NewProc("EnumWindows")
	_procGetWindowThreadProcessId_ = moduser32.NewProc("GetWindowThreadProcessId")
	_procGetWindowTextWSingle      = moduser32.NewProc("GetWindowTextW")
	_procSetForegroundWindow_      = moduser32.NewProc("SetForegroundWindow")
	_procShowWindow_               = moduser32.NewProc("ShowWindow")
	_procAllowSetForegroundWindow_ = moduser32.NewProc("AllowSetForegroundWindow")
	_procIsIconic_                 = moduser32.NewProc("IsIconic")
	_procGetWindowLong_W           = moduser32.NewProc("GetWindowLongW")
	_procSetWindowLong_W           = moduser32.NewProc("SetWindowLongW")
	_procSetWindowPos_             = moduser32.NewProc("SetWindowPos")
	_procGetParent_                = moduser32.NewProc("GetParent")
	_procIsWindowVisible_          = moduser32.NewProc("IsWindowVisible")
)

// init：覆盖 app.go 中 sysProcAttrNoWindow 的默认实现（返回 nil），
// 在 Windows 下启动 mstsc/cmdkey/cmd.exe 等子进程时使用 HideWindow + CREATE_NO_WINDOW 双重保险，
// 避免弹黑色控制台窗口（有些 cmd.exe 调用仅 CREATE_NO_WINDOW 仍会闪一下，必须加 HideWindow=true）
func init() {
	const CREATE_NO_WINDOW = 0x08000000
	sysProcAttrNoWindow = func() *syscall.SysProcAttr {
		return &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: CREATE_NO_WINDOW,
		}
	}
}

// ==========================================================
// 1) 单实例互斥
// ==========================================================
const (
	errorAlreadyExists = 183
	_mutexName         = `Global\FeiLiDaThinClient_Mutex_v1`
	_swzRestore        = 9
	_swzShow           = 5
	_synchronize       = 0x00100000
	_mutexAllAccess    = 0x001F0001
)

// ensureSingleInstance 返回已存在实例句柄（nil 表示自己是首个实例），且如果已存在则激活老实例主窗口
func ensureSingleInstance() (unsafe.Pointer, bool) {
	namePtr, _ := syscall.UTF16PtrFromString(_mutexName)
	h, _, err := _procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(namePtr)))
	if h == 0 {
		// CreateMutex 失败：不阻塞启动（安全兜底）
		return nil, false
	}
	if err != nil && err.(syscall.Errno) == syscall.Errno(errorAlreadyExists) {
		// 已有实例 → 关掉我们刚拿到的句柄，再激活老实例
		_, _, _ = _procCloseHandle.Call(h)
		activateExistingInstance()
		return nil, true
	}
	return unsafe.Pointer(h), false
}

func releaseSingleInstance(h unsafe.Pointer) {
	if h == nil {
		return
	}
	_, _, _ = _procReleaseMutex.Call(uintptr(h))
	_, _, _ = _procCloseHandle.Call(uintptr(h))
}

// activateExistingInstance 找到属于我们进程的顶层窗口并 SetForegroundWindow
func activateExistingInstance() {
	// 先 OpenMutex 拿 ASFS（AllowSetForegroundWindow）需要的目标进程 ID → 直接枚举所有窗口找我们的主窗口
	type ctxT struct {
		bestHWND uintptr
	}
	ctx := &ctxT{}
	cb := syscall.NewCallback(func(hwnd, lparam uintptr) uintptr {
		c := (*ctxT)(unsafe.Pointer(lparam))
		// 跳过非可见/子窗口
		vis, _, _ := _procIsWindowVisible_.Call(hwnd)
		if vis == 0 {
			return 1
		}
		par, _, _ := _procGetParent_.Call(hwnd)
		if par != 0 {
			return 1
		}
		// 标题含"云桌面"或窗口类名属于 thin-client-desktop（Wails 通常是 "WebView2Host"/"Chrome_WidgetWin_1"，因此改用窗口标题判断）
		title := getWindowText(hwnd)
		if contains(title, "云桌面") || contains(title, "瘦客户") || contains(title, "Thin Client") || contains(title, "FeiLiDa") {
			c.bestHWND = hwnd
			return 0
		}
		return 1
	})
	_, _, _ = _procEnumWindowsSingleInstance.Call(cb, uintptr(unsafe.Pointer(ctx)))
	if ctx.bestHWND == 0 {
		return
	}
	// 如果最小化，先恢复
	iconic, _, _ := _procIsIconic_.Call(ctx.bestHWND)
	if iconic != 0 {
		_, _, _ = _procShowWindow_.Call(ctx.bestHWND, _swzRestore)
	}
	_, _, _ = _procAllowSetForegroundWindow_.Call(^uintptr(0)) // ASFW_ANY = -1
	_, _, _ = _procSetForegroundWindow_.Call(ctx.bestHWND)
	_, _, _ = _procShowWindow_.Call(ctx.bestHWND, _swzShow)
}

func getWindowText(hwnd uintptr) string {
	const maxLen = 512
	buf := make([]uint16, maxLen)
	n, _, _ := _procGetWindowTextWSingle.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), maxLen)
	if n == 0 {
		return ""
	}
	return string(utf16.Decode(buf[:n]))
}

func contains(s, sub string) bool {
	ls, lsub := len(s), len(sub)
	if lsub == 0 || ls < lsub {
		return ls == 0 && lsub == 0
	}
	for i := 0; i <= ls-lsub; i++ {
		if s[i:i+lsub] == sub {
			return true
		}
		// 大小写不敏感匹配（简单转一次 ASCII）
		match := true
		for j := 0; j < lsub; j++ {
			a := s[i+j]
			b := sub[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// ==========================================================
//  2. 开机启动注册表写删 + 读取状态
//     HKCU\Software\Microsoft\Windows\CurrentVersion\Run  \  FeiLiDaThinClient
//
// ==========================================================
var (
	_modAdvapi32          = syscall.NewLazyDLL("advapi32.dll")
	_procRegOpenKeyExW    = _modAdvapi32.NewProc("RegOpenKeyExW")
	_procRegCreateKeyExW  = _modAdvapi32.NewProc("RegCreateKeyExW")
	_procRegSetValueExW   = _modAdvapi32.NewProc("RegSetValueExW")
	_procRegQueryValueExW = _modAdvapi32.NewProc("RegQueryValueExW")
	_procRegDeleteValueW  = _modAdvapi32.NewProc("RegDeleteValueW")
	_procRegCloseKey      = _modAdvapi32.NewProc("RegCloseKey")
)

const (
	_hkeyCurrentUser    = uintptr(0x80000001)
	_hkeyLocalMachine   = uintptr(0x80000002)
	_regSz              = 1
	_regDWORD           = 4
	_keyWrite           = 0x20006 // KEY_READ|KEY_WRITE
	_keyAllAccess       = 0xF003F // KEY_ALL_ACCESS
	_keyQueryValue      = 0x0001
	_keyWow64_64        = 0x0100 // KEY_WOW64_64KEY (32位进程访问64位注册表视图)
	_runSubKey          = `Software\Microsoft\Windows\CurrentVersion\Run`
	_runValueName       = "FeiLiDaThinClient"
	_runValueNameAuto   = "FeiLiDaThinClient_AutoKiosk"
	_policySystemPath   = `Software\Microsoft\Windows\CurrentVersion\Policies\System`
	_policyExplorerPath = `Software\Microsoft\Windows\CurrentVersion\Policies\Explorer`
)

// setAutostart 写开机启动注册表项。kioskMode=true 时额外附加 --kiosk-autostart 参数（开机直接进全屏）
func setAutostart(enabled bool, kioskMode bool) error {
	subKeyPtr, _ := syscall.UTF16PtrFromString(_runSubKey)
	var hKey uintptr
	r0, _, _ := _procRegOpenKeyExW.Call(_hkeyCurrentUser, uintptr(unsafe.Pointer(subKeyPtr)), 0, _keyWrite, uintptr(unsafe.Pointer(&hKey)))
	if r0 != 0 {
		return errnoErr(r0, "RegOpenKeyExW")
	}
	defer func() { _, _, _ = _procRegCloseKey.Call(hKey) }()

	valNamePtr, _ := syscall.UTF16PtrFromString(_runValueName)
	valNameAutoPtr, _ := syscall.UTF16PtrFromString(_runValueNameAuto)
	if !enabled {
		_procRegDeleteValueW.Call(hKey, uintptr(unsafe.Pointer(valNamePtr)))
		_procRegDeleteValueW.Call(hKey, uintptr(unsafe.Pointer(valNameAutoPtr)))
		return nil
	}

	// enabled=true：至少一条 Run。优先 kiosk 条目（含 --kiosk-autostart），如果不带 kiosk 就写普通条目
	exePath := getExecutablePath()
	cmd := `"` + exePath + `"`
	cmdAuto := cmd + ` --kiosk-autostart`
	if kioskMode {
		// kiosk 模式：先删旧普通条目（避免两个都执行），只写 autoKiosk
		_procRegDeleteValueW.Call(hKey, uintptr(unsafe.Pointer(valNamePtr)))
		if err := regSetSZ(hKey, valNameAutoPtr, cmdAuto); err != nil {
			return err
		}
		return nil
	}
	// 普通开机启动：不进 Kiosk；删掉 Kiosk 条目，写普通
	_procRegDeleteValueW.Call(hKey, uintptr(unsafe.Pointer(valNameAutoPtr)))
	return regSetSZ(hKey, valNamePtr, cmd)
}

// autostartStatus 返回 (普通开机启动, Kiosk开机启动)
func autostartStatus() (bool, bool) {
	subKeyPtr, _ := syscall.UTF16PtrFromString(_runSubKey)
	var hKey uintptr
	r0, _, _ := _procRegOpenKeyExW.Call(_hkeyCurrentUser, uintptr(unsafe.Pointer(subKeyPtr)), 0, _keyQueryValue, uintptr(unsafe.Pointer(&hKey)))
	if r0 != 0 {
		return false, false
	}
	defer func() { _, _, _ = _procRegCloseKey.Call(hKey) }()

	n1, _ := syscall.UTF16PtrFromString(_runValueName)
	n2, _ := syscall.UTF16PtrFromString(_runValueNameAuto)
	return regHasNonEmptySZ(hKey, n1), regHasNonEmptySZ(hKey, n2)
}

func regSetSZ(hKey uintptr, name *uint16, value string) error {
	w := utf16.Encode([]rune(value + "\x00"))
	r0, _, _ := _procRegSetValueExW.Call(
		hKey,
		uintptr(unsafe.Pointer(name)),
		0,
		_regSz,
		uintptr(unsafe.Pointer(&w[0])),
		uintptr(len(w)*2),
	)
	if r0 != 0 {
		return errnoErr(r0, "RegSetValueExW")
	}
	return nil
}

func regHasNonEmptySZ(hKey uintptr, name *uint16) bool {
	var typ uint32
	buf := make([]uint16, 2048)
	buflen := uint32(len(buf) * 2)
	r0, _, _ := _procRegQueryValueExW.Call(
		hKey,
		uintptr(unsafe.Pointer(name)),
		0,
		uintptr(unsafe.Pointer(&typ)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&buflen)),
	)
	if r0 != 0 || typ != _regSz || buflen < 4 {
		return false
	}
	count := int(buflen / 2)
	if count > len(buf) {
		count = len(buf)
	}
	s := string(utf16.Decode(buf[:count]))
	s = trimNull(s)
	return s != ""
}

func trimNull(s string) string {
	for i, r := range s {
		if r == 0 {
			return s[:i]
		}
	}
	return s
}

func errnoErr(r uintptr, op string) error {
	return syscall.Errno(r)
}

var _ = errnoErr // 保留引用（后续可扩展为自定义错误类型）

func getExecutablePath() string {
	if p, err := getExePathReal(); err == nil && p != "" {
		return p
	}
	return ""
}

func getExePathReal() (string, error) {
	// 直接用 kernel32 GetModuleFileNameW，避免 os.Executable 读到 symlink 路径（注册表 Run 必须是真 EXE 路径）
	const maxPath = 32768
	buf := make([]uint16, maxPath)
	n, _, _ := modkernel32.NewProc("GetModuleFileNameW").Call(0, uintptr(unsafe.Pointer(&buf[0])), maxPath)
	if n == 0 {
		return "", syscall.GetLastError()
	}
	return string(utf16.Decode(buf[:n])), nil
}

// ==========================================================
//  3. 主程序 ALT+F4 屏蔽（Kiosk/强制全屏模式下）
//     通过 GWL_STYLE 去 WS_SYSMENU + 安装一个短暂的 WH_KEYBOARD_LL 钩子吞掉 VK_F4 当 VK_MENU(Alt) 按下
//     这里先做窗口风格层的移除；WM_SYSCOMMAND/Close 在 main.go OnBeforeClose 里也会 return true 兜底
//
// ==========================================================
const (
	wsSysMenuSingle = 0x00080000
	wsMinBoxSingle  = 0x00020000
	wsMaxBoxSingle  = 0x00010000
)

// platformLockdownMainWindowKioskMode 把我们自己主程序的主窗口去 WS_SYSMENU → ALT+F4 / 系统菜单全部失效
// （用户不允许退出，要保持主页一直显示，看不到桌面）
func platformLockdownMainWindowKioskMode() {
	hwnd := findOwnHWND()
	if hwnd == 0 {
		return
	}
	gwlStyle := uintptr(^uint32(15)) // -16 = GWL_STYLE
	style, _, _ := _procGetWindowLong_W.Call(hwnd, gwlStyle)
	newStyle := style &^ (wsSysMenuSingle | wsMinBoxSingle | wsMaxBoxSingle)
	if newStyle != style {
		_procSetWindowLong_W.Call(hwnd, gwlStyle, newStyle)
	}
	// ★ 刷新 frame：必须同时设置 SWP_NOSIZE 和 SWP_NOMOVE，否则 SetWindowPos 会把窗口大小设为 0x0！
	//   之前漏了这两个标志 → 窗口大小变成 0x0 → "主页面打不开"
	const (
		swpNoSize       = 0x0001
		swpNoMove       = 0x0002
		swpNoZOrder     = 0x0004
		swpNoActivate   = 0x0010
		swpShowWindow   = 0x0040
		swpFrameChanged = 0x0020
	)
	_procSetWindowPos_.Call(hwnd, 0, 0, 0, 0, 0,
		swpNoSize|swpNoMove|swpNoZOrder|swpNoActivate|swpShowWindow|swpFrameChanged)
}

// platformRestoreMainWindow 恢复系统菜单（退出 Kiosk 时调用）
func platformRestoreMainWindow() {
	hwnd := findOwnHWND()
	if hwnd == 0 {
		return
	}
	gwlStyle := uintptr(^uint32(15))
	style, _, _ := _procGetWindowLong_W.Call(hwnd, gwlStyle)
	newStyle := style | wsSysMenuSingle | wsMinBoxSingle | wsMaxBoxSingle
	if newStyle != style {
		_procSetWindowLong_W.Call(hwnd, gwlStyle, newStyle)
	}
	// ★ 同样需要 SWP_NOSIZE | SWP_NOMOVE，否则窗口大小被设为 0
	const (
		swpNoSize       = 0x0001
		swpNoMove       = 0x0002
		swpNoZOrder     = 0x0004
		swpNoActivate   = 0x0010
		swpShowWindow   = 0x0040
		swpFrameChanged = 0x0020
	)
	_procSetWindowPos_.Call(hwnd, 0, 0, 0, 0, 0,
		swpNoSize|swpNoMove|swpNoZOrder|swpNoActivate|swpShowWindow|swpFrameChanged)
}

// isProcessRunning 检查指定名称的进程是否正在运行（用于监控远程桌面退出）
func isProcessRunning(nameLower string) bool {
	defer func() { _ = recover() }()
	snap, _, _ := _procCreateToolhelp32Snapshot.Call(0x00000002, 0) // TH32CS_SNAPPROCESS
	if snap == uintptr(^uint32(0)) {
		return false
	}
	defer func() { _, _, _ = _procCloseHandle.Call(snap) }()
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
	first, _, _ := _procProcess32FirstW.Call(snap, uintptr(unsafe.Pointer(&pe)))
	if first == 0 {
		return false
	}
	target := strings.ToLower(nameLower)
	for {
		exe := syscall.UTF16ToString(pe.szExeFile[:])
		if strings.Contains(strings.ToLower(exe), target) {
			return true
		}
		pe.dwSize = uint32(unsafe.Sizeof(pe))
		next, _, _ := _procProcess32NextW.Call(snap, uintptr(unsafe.Pointer(&pe)))
		if next == 0 {
			break
		}
	}
	return false
}

// bringWindowToFront 把主窗口从最小化状态恢复并激活到前台
func bringWindowToFront() {
	defer func() { _ = recover() }()
	hwnd := findOwnHWND()
	if hwnd == 0 {
		return
	}
	// 如果窗口最小化了，先恢复（SW_RESTORE = 9）
	iconic, _, _ := _procIsIconic.Call(hwnd)
	if iconic != 0 {
		_procShowWindow.Call(hwnd, 9)
	}
	// 尝试设置前台窗口（Windows 可能限制前台窗口权限，用 BringWindowToTop 兜底）
	_procSetForegroundWindow.Call(hwnd)
	_procBringWindowToTop.Call(hwnd)
}

// ==========================================================
//  4. WH_KEYBOARD_LL 全局低级键盘钩子 —— 真正吞掉 ALT+F4（Kiosk 模式强制开启）
//     为什么需要：
//     - 只去 WS_SYSMENU + OnBeforeClose 不够。WebView2 获得焦点时，
//     Windows 在调用 WM_SYSCOMMAND/SC_CLOSE 之前就会先触发默认的 ALT+F4 行为，
//     导致 Wails 先收到"关闭"，OnBeforeClose 虽然能阻止销毁，
//     但窗口会被最小化/隐藏，用户能看到桌面——违反用户需求。
//     - 低级键盘钩子 WH_KEYBOARD_LL 在 SendMessage/DefWindowProc 之前拦截，
//     只要检测到 ALT+F4 就直接返回非 0，操作系统彻底收不到这组按键。
//
// ==========================================================
var (
	_procSetWindowsHookExW   = moduser32.NewProc("SetWindowsHookExW")
	_procUnhookWindowsHookEx = moduser32.NewProc("UnhookWindowsHookEx")
	_procCallNextHookEx      = moduser32.NewProc("CallNextHookEx")
	_procGetMessageW         = moduser32.NewProc("GetMessageW")
	_procGetAsyncKeyState    = moduser32.NewProc("GetAsyncKeyState")
	_procKeybdEvent          = moduser32.NewProc("keybd_event")
	_procFindWindowW         = moduser32.NewProc("FindWindowW")
	_procGetForegroundWindow = moduser32.NewProc("GetForegroundWindow")
)

const (
	_whKeyboardLL     = 13     // WH_KEYBOARD_LL（低级键盘钩子，全局）
	_hcAction         = 0      // HC_ACTION
	_wmKeyDown        = 0x0100 // WM_KEYDOWN
	_wmSysKeyDown     = 0x0104 // WM_SYSKEYDOWN
	_wmKeyUp          = 0x0101
	_wmSysKeyUp       = 0x0105
	_vkF4             = 0x73   // VK_F4
	_vkF10            = 0x79   // VK_F10
	_vkF11            = 0x7A   // VK_F11
	_vkMenu           = 0x12   // VK_MENU（Alt）
	_vkControl        = 0x11   // VK_CONTROL
	_vkShift          = 0x10   // VK_SHIFT
	_vkDelete         = 0x2E   // VK_DEL
	_vkEnd            = 0x23   // VK_END
	_vkLWin           = 0x5B   // VK_LWIN
	_vkRWin           = 0x5C   // VK_RWIN
	_vkTab            = 0x09   // VK_TAB
	_vkEscape         = 0x1B   // VK_ESCAPE
	_vkSpace          = 0x20   // VK_SPACE
	_vkD              = 0x44   // 'D'
	_vkL              = 0x4C   // 'L'
	_vkE              = 0x45   // 'E'
	_vkR              = 0x52   // 'R'
	_vkI              = 0x49   // 'I'
	_vkS              = 0x53   // 'S'
	_vkLeft           = 0x25   // VK_LEFT
	_vkRight          = 0x27   // VK_RIGHT
	_vkUp             = 0x26   // VK_UP
	_vkDown           = 0x28   // VK_DOWN
	_vkPrintScreen    = 0x2C   // VK_SNAPSHOT (Print Screen)
	_kbKeyEventFKeyUp = 0x0002 // KEYEVENTF_KEYUP
)

// _altF4BlockActive: 全局开关，由 ApplyKioskMode/LockdownKioskClose 置 true，ExitKioskMode/UnlockKioskClose 置 false
var (
	_altF4HookHandle   uintptr = 0
	_altF4BlockActive  bool    = false
	_kioskDaemonStopCh         = make(chan struct{}, 1)
	_kioskDaemonOn     bool    = false
)

// lowLevelKeyboardProc 是 WH_KEYBOARD_LL 的回调。
// Kiosk 级拦截：屏蔽所有可能让用户看到本地系统的快捷键。
//   - ALT+F4（关闭窗口）
//   - Win 键（开始菜单）
//   - Alt+Tab（切换窗口）
//   - Win+D（显示桌面）
//   - Win+L（锁屏）
//   - Ctrl+Alt+Del（安全注意序列 — 转发给远程桌面）
//   - Ctrl+Alt+End（远程 Ctrl+Alt+Del）
//   - Win+E（资源管理器）
//   - Win+R（运行对话框）
//   - PrintScreen（截屏到本地）
//
// 返回 1 表示"已处理，不再向下传递"；否则 CallNextHookEx 让默认行为继续。
var lowLevelKeyboardProc = syscall.NewCallback(func(nCode int32, wParam uintptr, lParam uintptr) uintptr {
	if nCode < _hcAction {
		r, _, _ := _procCallNextHookEx.Call(_altF4HookHandle, uintptr(nCode), wParam, lParam)
		return r
	}
	// 只有 Kiosk 模式才全量拦截
	if !_altF4BlockActive {
		r, _, _ := _procCallNextHookEx.Call(_altF4HookHandle, uintptr(nCode), wParam, lParam)
		return r
	}
	if wParam == _wmSysKeyDown || wParam == _wmKeyDown {
		vkCode := *(*uint32)(unsafe.Pointer(lParam))
		ctrlDown, _, _ := _procGetAsyncKeyState.Call(_vkControl)
		altDown, _, _ := _procGetAsyncKeyState.Call(_vkMenu)
		winDown := false
		if vkCode == _vkLWin || vkCode == _vkRWin {
			winDown = true
		}
		hasCtrl := (ctrlDown & 0x8000) != 0
		hasAlt := (altDown & 0x8000) != 0
		hasWin := winDown

		// === 1. ALT+F4 ===
		if vkCode == _vkF4 && hasAlt {
			return 1
		}
		// === 2. WIN 键本身（阻止开始菜单）===
		if vkCode == _vkLWin || vkCode == _vkRWin {
			return 1
		}
		// === 3. ALT+TAB ===
		if vkCode == _vkTab && hasAlt {
			return 1
		}
		// === 4. WIN+D (显示桌面) ===
		if vkCode == _vkD && hasWin {
			return 1
		}
		// === 5. WIN+L (锁屏) ===
		if vkCode == _vkL && hasWin {
			return 1
		}
		// === 6. WIN+E (资源管理器) ===
		if vkCode == 0x45 && hasWin {
			return 1
		}
		// === 7. WIN+R (运行) ===
		if vkCode == 0x52 && hasWin {
			return 1
		}
		// === 8. WIN+I (设置) ===
		if vkCode == 0x49 && hasWin {
			return 1
		}
		// === 9. WIN+S (搜索) ===
		if vkCode == 0x53 && hasWin {
			// 如果同时有 Alt，让 Win+Shift+S 可能被别的键组合，简单拦截
			return 1
		}
		// === 10. PrintScreen (本地截屏) ===
		if vkCode == _vkPrintScreen {
			return 1
		}
		// === 11. Ctrl+Alt+Del 转发给远程 ===
		// Windows 的 Ctrl+Alt+Del 无法被用户级钩子拦截（安全注意序列 SAS）
		// 但我们可以拦截 Ctrl+Alt+End 并转发为远程 Ctrl+Alt+Del
		if vkCode == _vkEnd && hasCtrl && hasAlt {
			forwardCtrlAltDelToRDP()
			return 1
		}
		// === 12. WIN+Tab (虚拟桌面切换) ===
		if vkCode == _vkTab && hasWin {
			return 1
		}
		// === 13. WIN+方向键 (窗口吸附/移动) ===
		if (vkCode == _vkLeft || vkCode == _vkRight || vkCode == _vkUp || vkCode == _vkDown) && hasWin {
			return 1
		}
		// === 14. Alt+Shift+Tab (反向切换) ===
		if vkCode == _vkTab && hasAlt {
			return 1
		}
		// === 15. Ctrl+Esc (开始菜单) ===
		if vkCode == _vkEscape && hasCtrl {
			return 1
		}
		// === 16. WIN+ESC (游戏栏) ===
		if vkCode == _vkEscape && hasWin {
			return 1
		}
		// === 17. Shift+Win+S (截图工具) ===
		if vkCode == 0x53 && hasWin {
			return 1
		}
		// === 18. Ctrl+Shift+Esc (任务管理器) ===
		if vkCode == _vkEscape && hasCtrl {
			shiftDown, _, _ := _procGetAsyncKeyState.Call(_vkShift)
			if (shiftDown & 0x8000) != 0 {
				return 1
			}
		}
		// === 19. Win+X (电源用户菜单) ===
		if vkCode == 0x58 && hasWin {
			return 1
		}
		// === 20. Win+M (最小化所有窗口) ===
		if vkCode == 0x4D && hasWin {
			return 1
		}
		// === 21. Win+P (投影/第二屏幕) ===
		if vkCode == 0x50 && hasWin {
			return 1
		}
		// === 22. Win+Space (输入法切换) ===
		if vkCode == _vkSpace && hasWin {
			return 1
		}
		// === 23. Win+U (辅助功能) ===
		if vkCode == 0x55 && hasWin {
			return 1
		}
		// === 24. Win+A (操作中心/通知) ===
		if vkCode == 0x41 && hasWin {
			return 1
		}
		// === 25. Win+G (游戏栏) ===
		if vkCode == 0x47 && hasWin {
			return 1
		}
		// === 26. Win+H (语音输入) ===
		if vkCode == 0x48 && hasWin {
			return 1
		}
		// === 27. Win+I (设置) - 已有，保持一致 ===
		// === 28. Win+J (建议) ===
		if vkCode == 0x4A && hasWin {
			return 1
		}
		// === 29. Win+K (连接设备) ===
		if vkCode == 0x4B && hasWin {
			return 1
		}
		// === 30. Win+O (屏幕方向) ===
		if vkCode == 0x4F && hasWin {
			return 1
		}
		// === 31. Win+. (表情符号) ===
		if vkCode == 0xBE && hasWin {
			return 1
		}
		// === 32. F11 (浏览器/应用全屏切换) ===
		if vkCode == _vkF11 {
			return 1
		}
		// === 33. Alt+F10 (最大化/还原) ===
		if vkCode == _vkF10 && hasAlt {
			return 1
		}
		// === 34. Win+L (锁屏) - 已有 ===
		// === 35. 通用 Win 组合拦截：任何未明确列出的 Win+键 都拦截 ===
		if hasWin {
			// 不拦截纯 Win 键按下（开始菜单已在第 2 条单独拦截）
			// 但拦截所有 Win+其他键的组合
			switch vkCode {
			case _vkLWin, _vkRWin:
				// 已处理：纯 Win 键已在第 2 条拦截
			case _vkTab, _vkD, _vkL, _vkE, _vkR, _vkI, _vkS, _vkEscape, _vkLeft, _vkRight, _vkUp, _vkDown, _vkSpace, 0x58, 0x4D, 0x50, 0x55, 0x41, 0x47, 0x48, 0x4A, 0x4B, 0x4F, 0xBE:
				// 已处理：上述 Win+键组合已在前面各条中拦截
			default:
				// 拦截其他所有 Win+键组合
				return 1
			}
		}
	}
	r, _, _ := _procCallNextHookEx.Call(_altF4HookHandle, uintptr(nCode), wParam, lParam)
	return r
})

// activateGlobalKeyboardBlock 激活全局键盘拦截（Kiosk 级）
func activateGlobalKeyboardBlock() {
	if _altF4HookHandle == 0 {
		installAltF4KeyboardHook()
	}
	_altF4BlockActive = true
	startKioskStyleDaemon()
	// ★ 禁用 Ctrl+Alt+Del 安全屏幕中的所有选项
	//   Ctrl+Alt+Del 是 Windows SAS，无法被用户级钩子拦截
	//   通过注册表策略让安全屏幕上的所有按钮失效，用户只能取消返回
	disableCtrlAltDelOptions()
}

// deactivateGlobalKeyboardBlock 关闭全局键盘拦截
func deactivateGlobalKeyboardBlock() {
	_altF4BlockActive = false
	stopKioskStyleDaemon()
	// 恢复 Ctrl+Alt+Del 安全屏幕选项
	restoreCtrlAltDelOptions()
}

// forwardCtrlAltDelToRDP 向远程桌面窗口发送 Ctrl+Alt+End
// 在 RDP 协议中，Ctrl+Alt+End 等价于远程的 Ctrl+Alt+Del
func forwardCtrlAltDelToRDP() {
	defer func() { _ = recover() }()
	// 查找 FreeRDP 窗口
	hwnd := findRDPWindow()
	if hwnd == 0 {
		return
	}
	// 激活窗口并发送 Ctrl+Alt+End 键序列
	_procSetForegroundWindow.Call(hwnd)
	_procBringWindowToTop.Call(hwnd)
	// 模拟按键序列：Ctrl 下 → Alt 下 → End 下 → End 上 → Alt 上 → Ctrl 上
	_procKeybdEvent.Call(_vkControl, 0, 0, 0)
	_procKeybdEvent.Call(_vkMenu, 0, 0, 0)
	_procKeybdEvent.Call(_vkEnd, 0, 0, 0)
	_procKeybdEvent.Call(_vkEnd, 0, _kbKeyEventFKeyUp, 0)
	_procKeybdEvent.Call(_vkMenu, 0, _kbKeyEventFKeyUp, 0)
	_procKeybdEvent.Call(_vkControl, 0, _kbKeyEventFKeyUp, 0)
}

// findRDPWindow 查找 FreeRDP/wfreerdp/sdl-freerdp 窗口句柄
func findRDPWindow() uintptr {
	classNames := []string{"FreeRDP", "wfreerdp", "sdl-freerdp"}
	for _, cn := range classNames {
		cn16, _ := syscall.UTF16PtrFromString(cn)
		if h, _, _ := _procFindWindowW.Call(uintptr(unsafe.Pointer(cn16)), 0); h != 0 {
			return h
		}
	}
	// 兜底：枚举窗口查找标题含 "Remote Desktop" 或 "云桌面" 的
	var bestHWND uintptr
	cb := syscall.NewCallback(func(hwnd, lparam uintptr) uintptr {
		title := getWindowText(hwnd)
		if len(title) > 0 && (contains(strings.ToLower(title), "remote desktop") || contains(title, "云桌面") || contains(title, "远程桌面")) {
			bestHWND = hwnd
			return 1
		}
		return 1
	})
	_procEnumWindowsSingleInstance.Call(cb, 0)
	return bestHWND
}

// installAltF4KeyboardHook 在程序启动时安装一次钩子。钩子本身一直保留，但由 _altF4BlockActive 控制是否实际拦截。
// （低级键盘钩子数量有限，安装一次比反复卸/挂更稳定，避免 race。）
func installAltF4KeyboardHook() {
	if _altF4HookHandle != 0 {
		return
	}
	// WH_KEYBOARD_LL 要求回调由 SetWindowsHookEx 在调用线程里注册，
	// 然后该线程必须有消息循环（GetMessage/PeekMessage）——所以专门起一条 goroutine 当钩子线程。
	go func() {
		defer func() { _ = recover() }()
		h, _, _ := _procSetWindowsHookExW.Call(
			uintptr(_whKeyboardLL),
			lowLevelKeyboardProc,
			0, // 低级钩子：hMod=0 即可（不是通过 dll 注入那种）
			0, // dwThreadId=0：挂到本桌面所有线程
		)
		if h == 0 {
			// 安装失败：忽略（退回到 OnBeforeClose + WS_SYSMENU 方案，至少不会崩）
			return
		}
		_altF4HookHandle = h
		// 钩子线程必须跑消息循环，否则 WH_KEYBOARD_LL 收不到事件
		var msg struct {
			hwnd    uintptr
			message uint32
			wParam  uintptr
			lParam  uintptr
			time    uint32
			pt_x    int32
			pt_y    int32
		}
		for {
			ret, _, _ := _procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			// GetMessage 返回 -1 出错、0 收到 WM_QUIT（此时我们应该退出循环）
			if ret == 0 || ret == ^uintptr(0) {
				break
			}
			// 低级钩子不需要 TranslateMessage/DispatchMessage，LL 在 GetMessage 内部就执行了
		}
		// 退出消息循环：卸载钩子（基本跑不到这里）
		if _altF4HookHandle != 0 {
			_, _, _ = _procUnhookWindowsHookEx.Call(_altF4HookHandle)
			_altF4HookHandle = 0
		}
	}()
	// 等一下让钩子线程完成 SetWindowsHookEx（避免 Lockdown 时还没装好）
	waitCycles := 0
	for waitCycles < 30 {
		if _altF4HookHandle != 0 {
			break
		}
		waitCycles++
		sleepMs(30)
	}
}

// 因为主程序 build 时 runtime/time 可能没导入，这里用 syscall.Sleep 兜底
func sleepMs(ms int) { modkernel32.NewProc("Sleep").Call(uintptr(ms)) }

// startAltF4Block 开启 ALT+F4 吞键（并启动 kiosk 守护 goroutine 反复去 WS_SYSMENU）
func startAltF4Block() {
	if _altF4HookHandle == 0 {
		installAltF4KeyboardHook()
	}
	_altF4BlockActive = true
	startKioskStyleDaemon()
	// ★ 禁用 Ctrl+Alt+Del 安全屏幕选项（SAS 无法被钩子拦截，通过注册表策略禁用）
	disableCtrlAltDelOptions()
}

// stopAltF4Block 关闭 ALT+F4 吞键（并停止守护）
func stopAltF4Block() {
	_altF4BlockActive = false
	stopKioskStyleDaemon()
	// 恢复 Ctrl+Alt+Del 安全屏幕选项
	restoreCtrlAltDelOptions()
}

// startKioskStyleDaemon 后台 goroutine：每隔 1 秒重新 apply 一次窗口去 WS_SYSMENU + 全屏。
// 防止 WebView2/Wails 在内部 recreate window / DPI change / theme change 后把 WS_SYSMENU 又加回来。
func startKioskStyleDaemon() {
	if _kioskDaemonOn {
		return
	}
	_kioskDaemonOn = true
	_kioskDaemonStopCh = make(chan struct{}, 1)
	go func() {
		defer func() {
			_ = recover()
			_kioskDaemonOn = false
		}()
		// 先立即 apply 一次
		_kioskLockdownOnce()
		ticker := newStopTicker(800)
		defer ticker.stop()
		for {
			select {
			case <-_kioskDaemonStopCh:
				return
			case <-ticker.ch():
				if !_altF4BlockActive {
					return
				}
				_kioskLockdownOnce()
			}
		}
	}()
}
func stopKioskStyleDaemon() {
	if !_kioskDaemonOn {
		return
	}
	defer func() { _ = recover() }()
	select {
	case _kioskDaemonStopCh <- struct{}{}:
	default:
	}
}

// platformPauseKioskDaemon 暂停 Kiosk 守护进程（启动远程桌面前调用，防止守护进程阻止主窗口最小化）
func platformPauseKioskDaemon() {
	stopKioskStyleDaemon()
}

// platformResumeKioskDaemon 恢复 Kiosk 守护进程（远程桌面退出后调用）
func platformResumeKioskDaemon(restoreKiosk bool) {
	if restoreKiosk {
		startKioskStyleDaemon()
	}
}

// ==========================================================
//  6. 系统级嵌入 Kiosk（替换 Winlogon Shell 为我们 exe / 控制面板卸载列表注册 / Winlogon 自动登录）
//     所有写 HKLM 的操作都需要管理员权限：如果当前进程不是管理员，
//     通过 ShellExecuteEx "runas" 让同一个 exe 带 --__kiosk_priv 参数重新启动
//     （会弹一次 UAC 让用户允许），子进程（管理员身份）执行真正注册表写入后退出。
//
// ==========================================================
const (
	_winlogonKey    = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon`
	_uninstallRoot  = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`
	_uninstallSub   = `CloudDesktopClient`
	_valShell       = "Shell"
	_valAutoLogon   = "AutoAdminLogon"
	_valDefaultUser = "DefaultUserName"
	_valDefaultPwd  = "DefaultPassword"
	_valDefaultDmn  = "DefaultDomainName"
)

// SystemKioskStatus 当前系统嵌入状态（返回前端显示开关勾选/当前值）
type SystemKioskStatus struct {
	// 控制面板
	UninstallRegistered bool   `json:"uninstallRegistered"`
	UninstallPublisher  string `json:"uninstallPublisher"`
	UninstallVersion    string `json:"uninstallVersion"`
	// Shell 替换
	ShellReplaced bool   `json:"shellReplaced"`
	CurrentShell  string `json:"currentShell"`
	OurExePath    string `json:"ourExePath"`
	// Winlogon 自动登录
	AutoLogonEnabled bool   `json:"autoLogonEnabled"`
	AutoLogonUser    string `json:"autoLogonUser"`
	AutoLogonDomain  string `json:"autoLogonDomain"`
	// 权限
	IsElevated bool `json:"isElevated"`
}

var modadvapi32 = _modAdvapi32 // 别名，让新代码读起来更直观

// ------- 辅助：注册表全局操作函数 -------

// readRegistryString 读取注册表字符串值（兼容 "HKLM"/"HKCU" 前缀），失败返回空字符串
func readRegistryString(hive, path, name string) string {
	switch strings.ToUpper(hive) {
	case "HKLM", "HKEY_LOCAL_MACHINE":
		if v, ok := regGetStringHKLM(path, name); ok {
			return v
		}
	case "HKCU", "HKEY_CURRENT_USER":
		if v, ok := regGetStringAny(_hkeyCurrentUser, path, name); ok {
			return v
		}
	}
	return ""
}

func regGetStringHKLM(path, name string) (string, bool) {
	return regGetStringAny(_hkeyLocalMachine, path, name)
}
func regGetStringAny(hive uintptr, path, name string) (string, bool) {
	var h uintptr
	p, _ := syscall.UTF16PtrFromString(path)
	// 32位进程访问 HKLM 时使用 KEY_WOW64_64KEY 访问真正的 64 位注册表视图
	// 避免注册表重定向到 WOW6432Node 导致读取失败
	samDesired := uintptr(_keyQueryValue)
	if hive == _hkeyLocalMachine {
		samDesired |= _keyWow64_64
	}
	r, _, _ := _procRegOpenKeyExW.Call(hive, uintptr(unsafe.Pointer(p)), 0, samDesired, uintptr(unsafe.Pointer(&h)))
	if r != 0 {
		return "", false
	}
	defer _procRegCloseKey.Call(h)
	buf := make([]uint16, 4096)
	blen := uint32(len(buf) * 2)
	var vtype uint32
	n, _ := syscall.UTF16PtrFromString(name)
	r2, _, _ := _procRegQueryValueExW.Call(h, uintptr(unsafe.Pointer(n)), 0, 0,
		uintptr(unsafe.Pointer(&vtype)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&blen)))
	if r2 != 0 {
		return "", false
	}
	if vtype != _regSz {
		return "", false
	}
	str := syscall.UTF16ToString(buf[:blen/2])
	return strings.TrimRight(str, "\x00"), true
}

func regSetDWORDHive(hive uintptr, path, name string, val uint32) error {
	var h uintptr
	p, _ := syscall.UTF16PtrFromString(path)
	r0, _, _ := _procRegCreateKeyExW.Call(hive, uintptr(unsafe.Pointer(p)), 0, 0, 0, uintptr(_keyAllAccess), 0,
		uintptr(unsafe.Pointer(&h)), 0)
	if r0 != 0 {
		return fmt.Errorf("RegCreateKeyExW %s err=%d", path, r0)
	}
	defer _procRegCloseKey.Call(h)
	name16, _ := syscall.UTF16PtrFromString(name)
	r1, _, _ := _procRegSetValueExW.Call(h, uintptr(unsafe.Pointer(name16)), 0, uintptr(_regDWORD),
		uintptr(unsafe.Pointer(&val)), uintptr(4))
	if r1 != 0 {
		return fmt.Errorf("RegSetValueExW %s\\%s DWORD err=%d", path, name, r1)
	}
	return nil
}

// isProcessElevated 当前进程是否以管理员身份运行（是否能写 HKLM）
func isProcessElevated() bool {
	defer func() { _ = recover() }()
	modadv := syscall.NewLazyDLL("advapi32.dll")
	pOpenTok := modadv.NewProc("OpenProcessToken")
	pGetInfo := modadv.NewProc("GetTokenInformation")
	pClose := modadv.NewProc("CloseHandle")
	var tok uintptr
	cur, _, _ := modkernel32.NewProc("GetCurrentProcess").Call()
	r0, _, _ := pOpenTok.Call(cur, 0x0008, uintptr(unsafe.Pointer(&tok))) // TOKEN_QUERY = 0x0008
	if r0 == 0 {
		return false
	}
	defer pClose.Call(tok)
	type tokElev struct{ TokenIsElevated uint32 }
	var te tokElev
	var retLen uint32
	const _tokenElevation = 18 // TokenElevation
	r1, _, _ := pGetInfo.Call(tok, _tokenElevation, uintptr(unsafe.Pointer(&te)), uintptr(unsafe.Sizeof(te)), uintptr(unsafe.Pointer(&retLen)))
	if r1 == 0 {
		return false
	}
	return te.TokenIsElevated != 0
}

func readSystemKioskStatus() *SystemKioskStatus {
	st := &SystemKioskStatus{IsElevated: isProcessElevated()}
	st.OurExePath = getExecutablePath()
	// 控制面板：检查子键是否存在
	publisher, okPub := regGetStringHKLM(uninstallSubKeyPath(), "Publisher")
	version, okVer := regGetStringHKLM(uninstallSubKeyPath(), "DisplayVersion")
	st.UninstallPublisher = publisher
	st.UninstallVersion = version
	st.UninstallRegistered = okPub || okVer
	// Shell
	if shell, ok := regGetStringHKLM(_winlogonKey, _valShell); ok {
		st.CurrentShell = shell
		exeLower := strings.ToLower(strings.TrimSpace(st.OurExePath))
		shellLower := strings.ToLower(strings.TrimSpace(shell))
		st.ShellReplaced = (exeLower != "" && shellLower != "" && (strings.EqualFold(shellLower, exeLower) ||
			strings.Contains(shellLower, "thin-client-desktop") || strings.Contains(shellLower, "cloud-desktop"))) ||
			(shellLower != "explorer.exe")
	}
	// 自动登录
	if al, ok := regGetStringHKLM(_winlogonKey, _valAutoLogon); ok && al == "1" {
		st.AutoLogonEnabled = true
	}
	if u, ok := regGetStringHKLM(_winlogonKey, _valDefaultUser); ok {
		st.AutoLogonUser = u
	}
	if d, ok := regGetStringHKLM(_winlogonKey, _valDefaultDmn); ok {
		st.AutoLogonDomain = d
	}
	return st
}

func deleteRegValHKLM(path, name string) {
	var h uintptr
	p, _ := syscall.UTF16PtrFromString(path)
	r, _, _ := _procRegCreateKeyExW.Call(_hkeyLocalMachine, uintptr(unsafe.Pointer(p)), 0, 0, 0, uintptr(_keyAllAccess), 0,
		uintptr(unsafe.Pointer(&h)), 0)
	if r != 0 {
		return
	}
	defer _procRegCloseKey.Call(h)
	n, _ := syscall.UTF16PtrFromString(name)
	_procRegDeleteValueW.Call(h, uintptr(unsafe.Pointer(n)))
}
func deleteRegKeyHKLM(path, sub string) {
	// RegDeleteKeyW (无子项递归) —— 我们卸载列表子键下没子项，足够
	var h uintptr
	p, _ := syscall.UTF16PtrFromString(path)
	r, _, _ := _procRegOpenKeyExW.Call(_hkeyLocalMachine, uintptr(unsafe.Pointer(p)), 0, _keyAllAccess, uintptr(unsafe.Pointer(&h)))
	if r != 0 {
		return
	}
	defer _procRegCloseKey.Call(h)
	s, _ := syscall.UTF16PtrFromString(sub)
	modadvapi32.NewProc("RegDeleteKeyW").Call(h, uintptr(unsafe.Pointer(s)))
}

// uninstallSubKeyPath 返回 HKLM\...\Uninstall\<我们的子键>
func uninstallSubKeyPath() string { return _uninstallRoot + `\` + _uninstallSub }

// regSetStringHKLM 写 HKLM 的 REG_SZ（供下面写控制面板/Winlogon 使用）
func regSetStringHKLM(path, name, value string) error {
	var h uintptr
	p, _ := syscall.UTF16PtrFromString(path)
	r0, _, _ := _procRegCreateKeyExW.Call(_hkeyLocalMachine, uintptr(unsafe.Pointer(p)), 0, 0, 0, uintptr(_keyAllAccess), 0,
		uintptr(unsafe.Pointer(&h)), 0)
	if r0 != 0 {
		return fmt.Errorf("RegCreateKeyExW %s err=%d", path, r0)
	}
	defer _procRegCloseKey.Call(h)
	name16, _ := syscall.UTF16PtrFromString(name)
	w := utf16.Encode([]rune(value + "\x00"))
	r1, _, _ := _procRegSetValueExW.Call(h, uintptr(unsafe.Pointer(name16)), 0, uintptr(_regSz),
		uintptr(unsafe.Pointer(&w[0])), uintptr(len(w)*2))
	if r1 != 0 {
		return fmt.Errorf("RegSetValueExW %s\\%s err=%d", path, name, r1)
	}
	return nil
}
func regSetDWORDHKLM(path, name string, val uint32) error {
	return regSetDWORDHive(_hkeyLocalMachine, path, name, val)
}

// writeUninstallEntry 写控制面板卸载列表条目
func writeUninstallEntry(publisher, version, displayName string) error {
	exe := getExecutablePath()
	icon := exe
	if exe == "" {
		exe = "thin-client-desktop.exe"
	}
	if displayName == "" {
		displayName = "云桌面客户端"
	}
	if publisher == "" {
		publisher = "山东飞力达软件有限公司"
	}
	if version == "" {
		version = "1.0.0"
	}
	_ = regSetStringHKLM(uninstallSubKeyPath(), "DisplayName", displayName)
	_ = regSetStringHKLM(uninstallSubKeyPath(), "DisplayVersion", version)
	_ = regSetStringHKLM(uninstallSubKeyPath(), "Publisher", publisher)
	_ = regSetStringHKLM(uninstallSubKeyPath(), "DisplayIcon", icon)
	_ = regSetStringHKLM(uninstallSubKeyPath(), "UninstallString", `"`+exe+`"`)
	_ = regSetStringHKLM(uninstallSubKeyPath(), "InstallLocation", filepath.Dir(exe))
	_ = regSetDWORDHKLM(uninstallSubKeyPath(), "NoModify", 1)
	_ = regSetDWORDHKLM(uninstallSubKeyPath(), "NoRepair", 1)
	_ = regSetStringHKLM(uninstallSubKeyPath(), "URLInfoAbout", "https://sdfeilida.com")
	_ = regSetStringHKLM(uninstallSubKeyPath(), "URLUpdateInfo", "https://sdfeilida.com")
	y, m, d := time.Now().Date()
	dateStr := fmt.Sprintf("%04d%02d%02d", y, int(m), d)
	_ = regSetStringHKLM(uninstallSubKeyPath(), "InstallDate", dateStr)
	return nil
}
func removeUninstallEntry() { deleteRegKeyHKLM(_uninstallRoot, _uninstallSub) }

func applyWinlogonShell(replace bool, ourExe string) error {
	if replace {
		if ourExe == "" {
			ourExe = getExecutablePath()
		}
		if ourExe == "" {
			return fmt.Errorf("无法定位我们的 exe 路径，不能替换系统 Shell")
		}
		return regSetStringHKLM(_winlogonKey, _valShell, ourExe)
	}
	return regSetStringHKLM(_winlogonKey, _valShell, "explorer.exe")
}

func applyAutoLogin(enabled bool, username, password, domain string) error {
	if enabled {
		if username == "" {
			return fmt.Errorf("启用自动登录时必须指定 Windows 用户名")
		}
		if err := regSetStringHKLM(_winlogonKey, _valAutoLogon, "1"); err != nil {
			return err
		}
		if err := regSetStringHKLM(_winlogonKey, _valDefaultUser, username); err != nil {
			return err
		}
		if password == "" {
			deleteRegValHKLM(_winlogonKey, _valDefaultPwd)
		} else if err := regSetStringHKLM(_winlogonKey, _valDefaultPwd, password); err != nil {
			return err
		}
		if domain == "" {
			deleteRegValHKLM(_winlogonKey, _valDefaultDmn)
		} else if err := regSetStringHKLM(_winlogonKey, _valDefaultDmn, domain); err != nil {
			return err
		}
		return nil
	}
	deleteRegValHKLM(_winlogonKey, _valAutoLogon)
	deleteRegValHKLM(_winlogonKey, _valDefaultUser)
	deleteRegValHKLM(_winlogonKey, _valDefaultPwd)
	deleteRegValHKLM(_winlogonKey, _valDefaultDmn)
	return nil
}

// runElevatedSelf 以管理员身份（runas）再启动一次我们 exe，带 --__kiosk_priv 参数 + 环境变量传配置
//
//	返回 true 表示已经触发（当前进程非管理员，已调 ShellExecute 拉子进程执行），
//	返回 false 表示本来就是管理员，直接执行即可
func runElevatedSelf(payload string) (handled bool, err error) {
	if isProcessElevated() {
		return false, nil
	}
	exe := getExecutablePath()
	if exe == "" {
		return false, fmt.Errorf("无法定位 exe 路径执行提权")
	}
	const _SEE_MASK_NOCLOSEPROCESS = 0x00000040
	const _SW_SHOW = 5
	type shei struct {
		cbSize       uint32
		fMask        uint32
		hwnd         uintptr
		lpVerb       *uint16
		lpFile       *uint16
		lpParameters *uint16
		lpDirectory  *uint16
		nShow        int32
		hInstApp     uintptr
		lpIDList     uintptr
		lpClass      uintptr
		hkeyClass    uintptr
		dwHotKey     uint32
		hIcon        uintptr
		hProcess     uintptr
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	// ★ Base64 编码 payload，避免 JSON 中的双引号在 Windows 命令行中被截断/错误转义
	encodedPayload := base64.StdEncoding.EncodeToString([]byte(payload))
	args, _ := syscall.UTF16PtrFromString(`--__kiosk_priv ` + encodedPayload)
	info := shei{
		fMask:        _SEE_MASK_NOCLOSEPROCESS,
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: args,
		nShow:        _SW_SHOW,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))
	proc := modshell32_Proc()
	r, _, e := proc.Call(uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return false, fmt.Errorf("提权申请失败（UAC）：%v", e)
	}
	// 等管理员子进程写完后退出（子进程退出时表示完成）
	if info.hProcess != 0 {
		const _INFINITE = 0xFFFFFFFF
		modkernel32.NewProc("WaitForSingleObject").Call(info.hProcess, uintptr(_INFINITE))
		var exitCode uint32
		modkernel32.NewProc("GetExitCodeProcess").Call(info.hProcess, uintptr(unsafe.Pointer(&exitCode)))
		modkernel32.NewProc("CloseHandle").Call(info.hProcess)
		if exitCode != 0 {
			return true, fmt.Errorf("管理员子进程返回错误码 %d", exitCode)
		}
	}
	return true, nil
}

func modshell32_Proc() *syscall.LazyProc {
	return syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteExW")
}

// ========== 以下两个"调度立即重启"和"取消调度重启"用于应用/解除嵌入后提示用户立即重启生效 ==========
func scheduleRebootNow() error {
	cmd := exec.Command("shutdown", "/r", "/t", "10", "/c", "系统级嵌入设置已应用，10 秒后自动重启计算机以生效…（管理员可运行 shutdown /a 取消）")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd.Start()
}

// privApplyPayload 解析 --__kiosk_priv 负载，并在管理员子进程中执行真正写 HKLM
type privApplyPayload struct {
	Op                 string `json:"op"` // "apply" / "revert"
	RegisterUninstall  bool   `json:"register_uninstall"`
	UninstallPublisher string `json:"uninstall_publisher"`
	UninstallVersion   string `json:"uninstall_version"`
	ReplaceShell       bool   `json:"replace_shell"`
	EnableAutoLogin    bool   `json:"enable_autologin"`
	AutoLoginUser      string `json:"autologin_user"`
	AutoLoginPwd       string `json:"autologin_pwd"`
	AutoLoginDomain    string `json:"autologin_domain"`
	ScheduleReboot     bool   `json:"schedule_reboot"`
}

func doPrivApply(p privApplyPayload) error {
	if !isProcessElevated() {
		return fmt.Errorf("doPrivApply 必须管理员权限")
	}
	switch p.Op {
	case "apply":
		if p.RegisterUninstall {
			if err := writeUninstallEntry(p.UninstallPublisher, p.UninstallVersion, "云桌面客户端"); err != nil {
				return err
			}
		}
		if err := applyWinlogonShell(p.ReplaceShell, ""); err != nil {
			return err
		}
		if err := applyAutoLogin(p.EnableAutoLogin, p.AutoLoginUser, p.AutoLoginPwd, p.AutoLoginDomain); err != nil {
			return err
		}
	case "revert":
		// 还原 Shell + 自动登录；控制面板卸载条目默认保留（用户如果要求删，前端单独带标志）
		if err := applyWinlogonShell(false, ""); err != nil {
			return err
		}
		if err := applyAutoLogin(false, "", "", ""); err != nil {
			return err
		}
		removeUninstallEntry()
	default:
		return fmt.Errorf("未知 op: %s", p.Op)
	}
	if p.ScheduleReboot {
		return scheduleRebootNow()
	}
	return nil
}

// TryRunSystemKioskPrivPayload 从 os.Args 解析 --__kiosk_priv 负载并执行（管理员子进程入口），返回 true 表示已处理应该退出
func TryRunSystemKioskPrivPayload() (handled bool) {
	defer func() { _ = recover() }()
	// 找 --__kiosk_priv 参数
	for i, a := range os.Args {
		if (a == "--__kiosk_priv" || a == "-__kiosk_priv") && i+1 < len(os.Args) {
			var p privApplyPayload
			// ★ Base64 解码（runElevatedSelf 用 Base64 编码避免了命令行双引号截断问题）
			raw := os.Args[i+1]
			decoded, err := base64.StdEncoding.DecodeString(raw)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Base64 解码提权负载失败：", err)
				os.Exit(2)
			}
			if err := json.Unmarshal(decoded, &p); err != nil {
				fmt.Fprintln(os.Stderr, "解析提权负载失败：", err)
				os.Exit(2)
			}
			if err := doPrivApply(p); err != nil {
				fmt.Fprintln(os.Stderr, "应用系统嵌入失败：", err)
				os.Exit(3)
			}
			os.Exit(0)
			return true
		}
	}
	return false
}

// ==========================================================
// 3.5) Kiosk 级：任务栏硬隐藏 + 窗口前置守护
//
//	只靠 WS_EX_TOPMOST 是压不住的，只要 Horizon/任意程序激活时唤醒 Explorer，
//	Shell_TrayWnd（任务栏）就会浮出，用户鼠标点"显示桌面"就能把主窗口最小化
//	→ 看到原系统退出。因此：
//	  A) 进入 Kiosk 时立刻 SW_HIDE 任务栏 + 第二任务栏
//	  B) kioskDaemon 每 800ms 重新隐藏一次（防止 Explorer 被唤醒后重新显示）
//	  C) 远程桌面退出后，立刻 bringWindowToFront + hideTaskbarWindows 再激活一次
//	  D) 退出 Kiosk 时 SW_SHOW 还原任务栏
//
// ==========================================================
const (
	_swHide          = 0
	_swShow          = 5
	_hwndTopMost     = ^uintptr(0) - 1          // HWND_TOPMOST = -1
	_hwndNoTopMost   = ^uintptr(0) - 2          // HWND_NOTOPMOST = -2
	_swpTopMostFlags = 0x0040 | 0x0001 | 0x0002 // SWP_SHOWWINDOW | SWP_NOSIZE | SWP_NOMOVE
)

// hideTaskbarWindows 强制隐藏任务栏（主任务栏 Shell_TrayWnd + 多显示器副任务栏 Shell_SecondaryTrayWnd）
func hideTaskbarWindows() {
	defer func() { _ = recover() }()
	names := []string{"Shell_TrayWnd", "Shell_SecondaryTrayWnd"}
	for _, n := range names {
		name16, _ := syscall.UTF16PtrFromString(n)
		hwnd, _, _ := _procFindWindowW.Call(uintptr(unsafe.Pointer(name16)), 0)
		if hwnd != 0 {
			_, _, _ = _procShowWindow.Call(hwnd, _swHide)
		}
	}
}

// showTaskbarWindows 还原隐藏的任务栏
func showTaskbarWindows() {
	defer func() { _ = recover() }()
	names := []string{"Shell_TrayWnd", "Shell_SecondaryTrayWnd"}
	for _, n := range names {
		name16, _ := syscall.UTF16PtrFromString(n)
		hwnd, _, _ := _procFindWindowW.Call(uintptr(unsafe.Pointer(name16)), 0)
		if hwnd != 0 {
			_, _, _ = _procShowWindow.Call(hwnd, _swShow)
		}
	}
}

// ensureKioskWindowOnTopAndFront Kiosk 守护兜底：
//
//	如果主窗口被最小化/失去前台/不是 TOPMOST，
//	立刻恢复 + 置 TOPMOST + 拉到前台，防止用户看到桌面
func ensureKioskWindowOnTopAndFront() {
	defer func() { _ = recover() }()
	hwnd := findOwnHWND()
	if hwnd == 0 {
		return
	}
	iconic, _, _ := _procIsIconic.Call(hwnd)
	if iconic != 0 {
		_, _, _ = _procShowWindow.Call(hwnd, _swzRestore)
	}
	// 强行 TOPMOST（HWND_TOPMOST, NOSIZE NOMOVE SHOWWINDOW）
	_, _, _ = _procSetWindowPos_.Call(hwnd, _hwndTopMost, 0, 0, 0, 0, 0x0001|0x0002|0x0040)
	// AllowSetForegroundWindow(ASFW_ANY) 先解锁限制，再置前台
	_, _, _ = _procAllowSetForegroundWindow_.Call(^uintptr(0))
	_, _, _ = _procSetForegroundWindow_.Call(hwnd)
	_, _, _ = _procBringWindowToTop.Call(hwnd)
	_, _, _ = _procShowWindow_.Call(hwnd, _swzShow)
}

// _kioskLockdownOnce 执行一次：去 WS_SYSMENU + 隐藏任务栏 + 置顶前置
func _kioskLockdownOnce() {
	defer func() { _ = recover() }()
	platformLockdownMainWindowKioskMode()
	// ★ Kiosk 关键：任务栏反复隐藏 + TOPMOST+前台，
	//    防止 Horizon 登录窗体/ Explorer 激活时任务栏浮出 → 点击显示桌面把主窗口最小化
	hideTaskbarWindows()
	ensureKioskWindowOnTopAndFront()
	// findOwnHWND 再额外 SetWindowPos 刷 frame (确保 nonclient area 重建)
	hwnd := findOwnHWND()
	if hwnd == 0 {
		return
	}
	const (
		swpNoZOrder     = 0x0004
		swpNoActivate   = 0x0010
		swpNoSize       = 0x0001
		swpNoMove       = 0x0002
		swpFrameChanged = 0x0020
	)
	_procSetWindowPos_.Call(hwnd, 0, 0, 0, 0, 0,
		swpNoZOrder|swpNoActivate|swpNoSize|swpNoMove|swpFrameChanged)
}

// stopTicker 是极简 ticker 封装，避免每次都 import time（保持 file 导入干净）
type stopTicker struct {
	ch_   chan struct{}
	stop_ chan struct{}
}

func newStopTicker(msInterval int) *stopTicker {
	t := &stopTicker{ch_: make(chan struct{}, 1), stop_: make(chan struct{}, 1)}
	go func() {
		defer func() { _ = recover() }()
		for {
			select {
			case <-t.stop_:
				return
			default:
			}
			sleepMs(msInterval)
			select {
			case t.ch_ <- struct{}{}:
			default:
			}
		}
	}()
	return t
}
func (t *stopTicker) ch() chan struct{} { return t.ch_ }
func (t *stopTicker) stop() {
	defer func() { _ = recover() }()
	select {
	case t.stop_ <- struct{}{}:
	default:
	}
}

// ==========================================================
// 5) 注册表策略：禁用 Ctrl+Alt+Del 安全屏幕中的所有选项
//    Ctrl+Alt+Del 是 Windows 安全注意序列（SAS），无法被用户级 WH_KEYBOARD_LL 钩子拦截。
//    通过注册表策略禁用安全屏幕上的所有功能选项，用户即使按下 Ctrl+Alt+Del
//    也只能看到空菜单或被禁用的按钮，只能点"取消"返回到应用。
//    同时写入 HKCU 和 HKLM（如果有管理员权限），确保策略生效。
// ==========================================================

// regDeleteValueHive 删除注册表值（指定 hive），忽略"值不存在"错误
func regDeleteValueHive(hive uintptr, path, name string) {
	var hKey uintptr
	utf16Path, _ := syscall.UTF16PtrFromString(path)
	ret, _, _ := _procRegCreateKeyExW.Call(
		hive,
		uintptr(unsafe.Pointer(utf16Path)),
		0, 0, 0, uintptr(_keyAllAccess), 0,
		uintptr(unsafe.Pointer(&hKey)), 0,
	)
	if ret != 0 {
		return
	}
	defer _procRegCloseKey.Call(hKey)
	utf16Name, _ := syscall.UTF16PtrFromString(name)
	_procRegDeleteValueW.Call(hKey, uintptr(unsafe.Pointer(utf16Name)))
}

// regSetPolicyDWORDBoth 同时写入 HKCU 和 HKLM 策略注册表 DWORD 值
// HKLM 写入可能因权限不足而失败（非管理员），HKCU 通常都会成功
func regSetPolicyDWORDBoth(path, name string, value uint32) {
	// HKCU（用户级，一定有权限）
	_ = regSetDWORDHive(_hkeyCurrentUser, path, name, value)
	// HKLM（需要管理员权限，失败则忽略）
	_ = regSetDWORDHive(_hkeyLocalMachine, path, name, value)
}

// regDeletePolicyValueBoth 同时删除 HKCU 和 HKLM 策略注册表值
func regDeletePolicyValueBoth(path, name string) {
	regDeleteValueHive(_hkeyCurrentUser, path, name)
	regDeleteValueHive(_hkeyLocalMachine, path, name)
}

// disableCtrlAltDelOptions 禁用 Ctrl+Alt+Del 安全屏幕中的所有选项
//   - DisableTaskMgr:          禁用"任务管理器"
//   - DisableLockWorkstation:  禁用"锁定"
//   - DisableChangePassword:   禁用"更改密码"
//   - HideFastUserSwitching:   禁用/隐藏"切换用户"
//   - NoLogoff:                禁用"注销"（在 Explorer 策略路径下）
func disableCtrlAltDelOptions() {
	defer func() { _ = recover() }()
	// System 策略路径下的选项（同时写 HKCU + HKLM）
	regSetPolicyDWORDBoth(_policySystemPath, "DisableTaskMgr", 1)
	regSetPolicyDWORDBoth(_policySystemPath, "DisableLockWorkstation", 1)
	regSetPolicyDWORDBoth(_policySystemPath, "DisableChangePassword", 1)
	regSetPolicyDWORDBoth(_policySystemPath, "HideFastUserSwitching", 1)
	// Explorer 策略路径下的注销选项
	regSetPolicyDWORDBoth(_policyExplorerPath, "NoLogoff", 1)
	// 额外：禁用快速用户切换功能（HKLM\...\Authentication\LogonUI）
	_ = regSetDWORDHive(_hkeyLocalMachine,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Authentication\LogonUI`,
		"FastUserSwitching", 0)
}

// restoreCtrlAltDelOptions 恢复 Ctrl+Alt+Del 安全屏幕选项（删除策略值）
func restoreCtrlAltDelOptions() {
	defer func() { _ = recover() }()
	regDeletePolicyValueBoth(_policySystemPath, "DisableTaskMgr")
	regDeletePolicyValueBoth(_policySystemPath, "DisableLockWorkstation")
	regDeletePolicyValueBoth(_policySystemPath, "DisableChangePassword")
	regDeletePolicyValueBoth(_policySystemPath, "HideFastUserSwitching")
	regDeletePolicyValueBoth(_policyExplorerPath, "NoLogoff")
	regDeleteValueHive(_hkeyLocalMachine,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Authentication\LogonUI`,
		"FastUserSwitching")
}
