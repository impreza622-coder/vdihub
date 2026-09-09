//go:build windows

package tray

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ============================================================
//  Win32 常量（窗口消息、Shell_NotifyIcon 标志、菜单标志）
// ============================================================
const (
	wmUser          = 0x0400
	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205
	wmCommand       = 0x0111
	wmDestroy       = 0x0002
	wmClose         = 0x0010

	nimAdd    = 0x00000000
	nimDelete = 0x00000002
	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	mfString    = 0x00000000
	mfSeparator = 0x00000800

	tpmLeftAlign   = 0x0000
	tpmBottomAlign = 0x0020
	tpmReturnCmd   = 0x0100

	// 托盘回调消息 ID（WM_USER + 1）
	trayCallbackMsg = wmUser + 1
	// 右键菜单"退出程序"命令 ID
	menuExitCmdID = uint32(1001)

	// 创建消息专用窗口的 hWndParent
	hwndMessage = ^uintptr(2) // HWND_MESSAGE = (HWND)-3

	// CreateIconFromResource 的版本参数
	iconResVer = 0x00030000
)

// ============================================================
//  Win32 结构体（精确匹配 C ABI 布局，Go 编译器会自动处理对齐 padding）
// ============================================================

// WNDCLASSEXW（80 字节 @ x64）
type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     windows.Handle
	HIcon         windows.Handle
	HCursor       windows.Handle
	HbrBackground windows.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       windows.Handle
}

// MSG（44 字节 @ x64）
type msg struct {
	Hwnd    windows.HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

// POINT（8 字节）
type point struct {
	X, Y int32
}

// NOTIFYICONDATAW（V3，952 字节 @ x64，不含 GUID/BalloonIcon 扩展字段）
type notifyIconData struct {
	CbSize           uint32
	HWnd             windows.HWND
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            windows.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
}

// ============================================================
//  Lazy DLL 加载（user32.dll / shell32.dll）
//  golang.org/x/sys/windows 不导出窗口管理 API，所以自己声明。
// ============================================================
var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procDefWindowProc    = user32.NewProc("DefWindowProcW")
	procGetMessage       = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessage  = user32.NewProc("DispatchMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procPostMessage      = user32.NewProc("PostMessageW")
	procSendMessage      = user32.NewProc("SendMessageW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	procAppendMenu       = user32.NewProc("AppendMenuW")
	procTrackPopupMenu   = user32.NewProc("TrackPopupMenu")
	procDestroyMenu      = user32.NewProc("DestroyMenu")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procSetForegroundWnd = user32.NewProc("SetForegroundWindow")
	procLoadIcon         = user32.NewProc("LoadIconW")
	procLoadCursor       = user32.NewProc("LoadCursorW")
	procCreateIconFromRes = user32.NewProc("CreateIconFromResource")

	procShellNotifyIcon  = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")
)

// ============================================================
//  Windows 托盘运行时数据
// ============================================================

// windowsTrayData 保存托盘消息窗口句柄、图标句柄和 NOTIFYICONDATA。
type windowsTrayData struct {
	hwnd  windows.HWND
	hicon windows.Handle
	nid   notifyIconData
}

// 全局变量：WindowProc 必须是全局函数（windows.NewCallback 要求），
// 所以通过全局变量让 wndProc 访问 Manager。
var (
	gMgr  *Manager
	gData *windowsTrayData
)

// ============================================================
//  WindowProc（全局函数，由 windows.NewCallback 转成 C 回调指针）
// ============================================================

// trayWndProc 托盘消息窗口的 WindowProc。
// 签名必须用 uintptr（不用 windows.HWND），确保 syscall.NewCallback 兼容。
func trayWndProc(hwnd uintptr, msg uint32, wParam uintptr, lParam uintptr) uintptr {
	switch msg {
	case trayCallbackMsg:
		// 托盘图标鼠标消息（lParam 是具体鼠标消息）
		switch lParam {
		case wmLButtonUp, wmLButtonDblClk:
			// 左键单击 / 双击 → 显示主窗口
			if gMgr != nil {
				go gMgr.ShowWindow()
			}
			return 0
		case wmRButtonUp:
			// 右键 → 弹出只有"退出程序"的菜单
			showContextMenu(windows.HWND(hwnd))
			return 0
		}

	case wmCommand:
		// 菜单项点击：低字 wParam = 命令 ID
		cmdID := uint32(wParam & 0xFFFF)
		if cmdID == menuExitCmdID {
			if gMgr != nil {
				go gMgr.Quit()
			}
			return 0
		}

	case wmDestroy:
		// 窗口销毁：先删除托盘图标，再退出消息循环
		removeTrayIcon()
		postQuitMessage(0)
		return 0
	}

	// 默认处理（含 WM_CLOSE → DefWindowProc 自动调 DestroyWindow）
	return defWindowProc(windows.HWND(hwnd), msg, wParam, lParam)
}

// ============================================================
//  右键菜单：只有"退出程序"一项
// ============================================================

func showContextMenu(hwnd windows.HWND) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	labelPtr, _ := windows.UTF16PtrFromString("退出程序")
	procAppendMenu.Call(menu, mfString, uintptr(menuExitCmdID), uintptr(unsafe.Pointer(labelPtr)))

	// 获取当前鼠标位置（菜单弹出位置）
	var pt point
	ret, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	if ret == 0 {
		pt = point{X: 0, Y: 0}
	}

	// Windows quirk：弹出菜单前必须 SetForegroundWindow，
	// 否则菜单无法在点击外部时自动关闭。
	procSetForegroundWnd.Call(uintptr(hwnd))

	// TrackPopupMenu 返回选中的命令 ID（因为用了 TPM_RETURNCMD）
	cmd, _, _ := procTrackPopupMenu.Call(
		menu,
		tpmReturnCmd|tpmLeftAlign|tpmBottomAlign,
		uintptr(pt.X),
		uintptr(pt.Y),
		0,
		uintptr(hwnd),
		0,
	)

	if cmd != 0 {
		// 用户选了菜单项 → 发 WM_COMMAND 给自己的窗口
		sendMessage(hwnd, wmCommand, cmd, 0)
	}
}

// ============================================================
//  托盘图标增删
// ============================================================

func addTrayIcon(data *windowsTrayData, tooltip string) {
	// 填充 NOTIFYICONDATA
	data.nid = notifyIconData{
		CbSize:           uint32(unsafe.Sizeof(notifyIconData{})),
		HWnd:             data.hwnd,
		UID:              1,
		UFlags:           nifMessage | nifIcon | nifTip,
		UCallbackMessage: trayCallbackMsg,
		HIcon:            data.hicon,
	}
	// 填充 szTip（tooltip，最多 128 个 wchar）
	tipUTF16, _ := windows.UTF16FromString(tooltip)
	copy(data.nid.SzTip[:], tipUTF16)

	procShellNotifyIcon.Call(uintptr(nimAdd), uintptr(unsafe.Pointer(&data.nid)))
}

func removeTrayIcon() {
	if gData == nil {
		return
	}
	gData.nid.UFlags = 0
	procShellNotifyIcon.Call(uintptr(nimDelete), uintptr(unsafe.Pointer(&gData.nid)))
}

// ============================================================
//  HICON 加载：从 ICO 字节流创建（用 CreateIconFromResource）
//  ICO 文件格式：ICONDIR(6B) + ICONDIRENTRY(16B) + BITMAPINFOHEADER + 像素数据
//  CreateIconFromResource 接收的是 BITMAPINFOHEADER 起始的指针。
// ============================================================

func loadHICONFromICO(ico []byte) windows.Handle {
	if len(ico) < 22 {
		return 0
	}
	// 跳过 ICONDIR(6) + ICONDIRENTRY(16) = 22 字节
	presbits := &ico[22]
	dwResSize := uintptr(len(ico) - 22)
	hIcon, _, _ := procCreateIconFromRes.Call(
		uintptr(unsafe.Pointer(presbits)),
		dwResSize,
		1, // TRUE = icon (not cursor)
		iconResVer,
	)
	return windows.Handle(hIcon)
}

// ============================================================
//  Win32 API 薄封装
// ============================================================

func defWindowProc(hwnd windows.HWND, msg uint32, wParam uintptr, lParam uintptr) uintptr {
	ret, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

func postQuitMessage(exitCode int32) {
	procPostQuitMessage.Call(uintptr(exitCode))
}

func postMessage(hwnd windows.HWND, msg uint32, wParam uintptr, lParam uintptr) bool {
	ret, _, _ := procPostMessage.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret != 0
}

func sendMessage(hwnd windows.HWND, msg uint32, wParam uintptr, lParam uintptr) uintptr {
	ret, _, _ := procSendMessage.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

// ============================================================
//  Manager 平台特定实现
// ============================================================

// startPlatform 在独立 goroutine 中创建托盘消息窗口 + 图标 + 消息循环。
// 阻塞直到 PostQuitMessage 被调用（Quit/WM_DESTROY 触发）。
func (m *Manager) startPlatform() {
	gMgr = m

	go func() {
		// 消息循环必须在锁定的 OS 线程上运行
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		// 1. 注册窗口类（仅一次）
		className, _ := windows.UTF16PtrFromString("ShouKeHuduanTrayWnd")
		hInst, _, _ := procGetModuleHandle.Call(0) // NULL = 当前进程模块句柄

		// 默认图标（CreateIconFromResource 失败时的兜底）
		hDefIcon, _, _ := procLoadIcon.Call(uintptr(hInst), 32512) // IDI_APPLICATION

		wc := wndClassEx{
			CbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
			LpfnWndProc:   windows.NewCallback(trayWndProc),
			HInstance:     windows.Handle(hInst),
			LpszClassName: className,
			HIcon:         windows.Handle(hDefIcon),
			HIconSm:       windows.Handle(hDefIcon),
		}
		procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

		// 2. 创建消息专用隐藏窗口（hWndParent = HWND_MESSAGE）
		hwnd, _, _ := procCreateWindowEx.Call(
			0,                          // dwExStyle
			uintptr(unsafe.Pointer(className)), // lpClassName
			0,                          // lpWindowName
			0,                          // dwStyle（不显示）
			0, 0, 0, 0,                // x, y, width, height
			hwndMessage,                // hWndParent = HWND_MESSAGE
			0,                          // hMenu
			uintptr(hInst),             // hInstance
			0,                          // lpParam
		)
		if hwnd == 0 {
			return
		}

		// 3. 从品牌 ICO 字节流加载 HICON
		hicon := loadHICONFromICO(appIconBytes())
		if hicon == 0 {
			hicon = windows.Handle(hDefIcon) // 兜底用系统默认图标
		}

		// 4. 创建托盘数据 + 添加图标
		data := &windowsTrayData{
			hwnd:  windows.HWND(hwnd),
			hicon: hicon,
		}
		gData = data
		m.mu.Lock()
		m.platformData = data
		m.mu.Unlock()

		addTrayIcon(data, "瘦客户端云桌面 · 单击显示主窗口")

		// 5. 消息循环（阻塞直到 PostQuitMessage）
		var m2 msg
		for {
			ret, _, _ := procGetMessage.Call(
				uintptr(unsafe.Pointer(&m2)),
				0, // hWnd = NULL（取所有窗口消息）
				0, 0,
			)
			// ret = -1 出错；ret = 0 收到 WM_QUIT 退出
			if int32(ret) <= 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m2)))
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&m2)))
		}

		// 6. 清理
		procDestroyWindow.Call(hwnd)
	}()
}

// stopPlatform 停止托盘（Kiosk 模式禁用托盘时调用，或 Quit 退出时调用）。
func (m *Manager) stopPlatform() {
	m.mu.RLock()
	data, _ := m.platformData.(*windowsTrayData)
	m.mu.RUnlock()
	if data == nil || data.hwnd == 0 {
		return
	}
	// 发 WM_CLOSE → DefWindowProc → DestroyWindow → WM_DESTROY → PostQuitMessage
	postMessage(data.hwnd, wmClose, 0, 0)
}
