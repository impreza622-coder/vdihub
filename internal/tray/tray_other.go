//go:build !windows

package tray

// startPlatform 非 Windows 平台：托盘不实现（瘦客户端仅面向 Windows）。
func (m *Manager) startPlatform() {}

// stopPlatform 非 Windows 平台：空操作。
func (m *Manager) stopPlatform() {}
