//go:build !windows

package main

// platformApplyKioskFrameless 非 Windows 平台空实现
func platformApplyKioskFrameless() {}

// platformRestoreFramed 非 Windows 平台空实现
func platformRestoreFramed() {}

// platformStartWindowDrag 非 Windows 平台空实现
func platformStartWindowDrag() {}

// platformWindowGetRect 非 Windows 平台空实现
func platformWindowGetRect() (int, int, int, int, bool) { return 0, 0, 0, 0, false }

// platformWindowSetPos 非 Windows 平台空实现
func platformWindowSetPos(int, int) {}

// platformCenterWindowOnScreen 非 Windows 平台空实现
func platformCenterWindowOnScreen() {}

// platformPauseKioskDaemon 非 Windows 平台空实现
func platformPauseKioskDaemon() {}

// platformResumeKioskDaemon 非 Windows 平台空实现
func platformResumeKioskDaemon(restoreKiosk bool) {}

// activateGlobalKeyboardBlock 非 Windows 平台空实现
func activateGlobalKeyboardBlock() {}

// deactivateGlobalKeyboardBlock 非 Windows 平台空实现
func deactivateGlobalKeyboardBlock() {}
