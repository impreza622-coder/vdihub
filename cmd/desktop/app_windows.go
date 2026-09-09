package main

import (
	"syscall"
	"unsafe"
)

var (
	_user32                        = syscall.NewLazyDLL("user32.dll")
	_procEnumWindows              = _user32.NewProc("EnumWindows")
	_procGetWindowThreadProcessId = _user32.NewProc("GetWindowThreadProcessId")
	_procGetWindowLongW           = _user32.NewProc("GetWindowLongW")
	_procSetWindowLongW           = _user32.NewProc("SetWindowLongW")
	_procSetWindowPos             = _user32.NewProc("SetWindowPos")
	_procReleaseCapture           = _user32.NewProc("ReleaseCapture")
	_procSendMessageW             = _user32.NewProc("SendMessageW")
	_procGetWindowRect            = _user32.NewProc("GetWindowRect")
	_procGetSystemMetrics         = _user32.NewProc("GetSystemMetrics")
	_procMoveWindow               = _user32.NewProc("MoveWindow")
)

const (
	_wmNclbuttondown = 0x00A1
	_htCaption       = 2
	_smCxScreen      = 0
	_smCyScreen      = 1
)

type _rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

const (
	// GWLP_* 常量通过 uint32(负数) 后转 uintptr，避免 -16 常量直接溢出 uintptr
	_wsCaption    = 0x00C00000
	_wsThickFrame = 0x00040000
	_wsSysMenu    = 0x00080000
	_wsMinimizeBox = 0x00020000
	_wsMaximizeBox = 0x00010000
	_wsExAppWindow = 0x00040000
	_wsExWindowEdge = 0x00000100
	_wsExClientEdge = 0x00000200

	_swpNoSize        = 0x0001
	_swpNoMove        = 0x0002
	_swpNoZOrder     = 0x0004
	_swpNoActivate   = 0x0010
	_swpShowWindow   = 0x0040
	_swpFrameChanged = 0x0020
)

// findOwnHWND 返回当前进程自己的主窗口句柄
func findOwnHWND() uintptr {
	pid := uintptr(syscall.Getpid())
	found := uintptr(0)
	cb := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var outPid uint32
		_procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&outPid)))
		if uintptr(outPid) == pid {
			found = hwnd
			return 0
		}
		return 1
	})
	_procEnumWindows.Call(cb, 0)
	return found
}

// platformApplyKioskFrameless 去掉标题栏/边框 → 真正的无边框全屏
func platformApplyKioskFrameless() {
	hwnd := findOwnHWND()
	if hwnd == 0 {
		return
	}
	// GWL_STYLE = -16 = 0xFFFFFFF0 (as uint32); GWL_EXSTYLE = -20 = 0xFFFFFFEC
	gwlStyle := uintptr(^uint32(15))  // 0xFFFFFFF0
	gwlExStyle := uintptr(^uint32(19)) // 0xFFFFFFEC
	style, _, _ := _procGetWindowLongW.Call(hwnd, gwlStyle)
	newStyle := style &^ (_wsCaption | _wsThickFrame | _wsSysMenu | _wsMinimizeBox | _wsMaximizeBox)
	_procSetWindowLongW.Call(hwnd, gwlStyle, newStyle)
	exStyle, _, _ := _procGetWindowLongW.Call(hwnd, gwlExStyle)
	newEx := exStyle &^ (_wsExAppWindow | _wsExWindowEdge | _wsExClientEdge)
	_procSetWindowLongW.Call(hwnd, gwlExStyle, newEx)
	_procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0,
		_swpNoSize|_swpNoMove|_swpNoZOrder|_swpNoActivate|_swpShowWindow|_swpFrameChanged)
}

// platformStartWindowDrag 无边框窗口拖拽移动：ReleaseCapture + WM_NCLBUTTONDOWN(HTCAPTION)
func platformStartWindowDrag() {
	hwnd := findOwnHWND()
	if hwnd == 0 {
		return
	}
	_procReleaseCapture.Call()
	_procSendMessageW.Call(hwnd, uintptr(_wmNclbuttondown), uintptr(_htCaption), 0)
}

// platformWindowGetRect 返回 x,y,w,h
func platformWindowGetRect() (x, y, w, h int, ok bool) {
	hwnd := findOwnHWND()
	if hwnd == 0 {
		return 0, 0, 0, 0, false
	}
	var r _rect
	ret, _, _ := _procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	if ret == 0 {
		return 0, 0, 0, 0, false
	}
	return int(r.Left), int(r.Top), int(r.Right - r.Left), int(r.Bottom - r.Top), true
}

// platformWindowSetPos 移动窗口（保持原尺寸）
func platformWindowSetPos(x, y int) {
	hwnd := findOwnHWND()
	if hwnd == 0 {
		return
	}
	_, _, w, h, ok := platformWindowGetRect()
	if !ok {
		return
	}
	_procMoveWindow.Call(hwnd, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 1)
}

// platformCenterWindowOnScreen 启动后窗口居中（避免"卡在左上角"）
func platformCenterWindowOnScreen() {
	hwnd := findOwnHWND()
	if hwnd == 0 {
		return
	}
	_, _, w, h, ok := platformWindowGetRect()
	if !ok {
		w, h = 1200, 800
	}
	cx, _, _ := _procGetSystemMetrics.Call(uintptr(_smCxScreen))
	cy, _, _ := _procGetSystemMetrics.Call(uintptr(_smCyScreen))
	if cx == 0 || cy == 0 {
		return
	}
	newX := int(int(cx)-w) / 2
	newY := int(int(cy)-h) / 2
	if newX < 0 {
		newX = 0
	}
	if newY < 0 {
		newY = 0
	}
	_procMoveWindow.Call(hwnd, uintptr(newX), uintptr(newY), uintptr(w), uintptr(h), 1)
}

// platformRestoreFramed 恢复标题栏+边框
func platformRestoreFramed() {
	hwnd := findOwnHWND()
	if hwnd == 0 {
		return
	}
	gwlStyle := uintptr(^uint32(15))
	gwlExStyle := uintptr(^uint32(19))
	style, _, _ := _procGetWindowLongW.Call(hwnd, gwlStyle)
	newStyle := style | _wsCaption | _wsThickFrame | _wsSysMenu | _wsMinimizeBox | _wsMaximizeBox
	_procSetWindowLongW.Call(hwnd, gwlStyle, newStyle)
	exStyle, _, _ := _procGetWindowLongW.Call(hwnd, gwlExStyle)
	newEx := exStyle | _wsExAppWindow | _wsExWindowEdge | _wsExClientEdge
	_procSetWindowLongW.Call(hwnd, gwlExStyle, newEx)
	_procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0,
		_swpNoSize|_swpNoMove|_swpNoZOrder|_swpNoActivate|_swpShowWindow|_swpFrameChanged)
}
