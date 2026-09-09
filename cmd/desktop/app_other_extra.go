//go:build !windows

package main

import "unsafe"

func ensureSingleInstance() (unsafe.Pointer, bool) { return nil, false }
func releaseSingleInstance(_ unsafe.Pointer) {}
func setAutostart(enabled bool, kioskMode bool) error { return nil }
func autostartStatus() (bool, bool) { return false, false }
func platformLockdownMainWindowKioskMode() {}
func platformRestoreMainWindow() {}
func installAltF4KeyboardHook()  {}
func startAltF4Block()           {}
func stopAltF4Block()            {}
func isProcessRunning(nameLower string) bool { return false }
func bringWindowToFront() {}
