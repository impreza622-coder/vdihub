//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"unicode/utf16"
	"syscall"
)

// listUSBDevicesOS 枚举本地 USB 设备（Windows 实现）
// 使用 PowerShell + WMI 查询所有含 VID/PID 的 USB 设备
func listUSBDevicesOS() ([]USBDevice, error) {
	// ★ 用绝对路径调用 PowerShell，避免精简版 Windows / PATH 被截断时找不到 powershell
	psExe := findPowerShellExe()
	if psExe == "" {
		// PowerShell 不存在，直接回退到 WMIC
		return listUSBDevicesWMIC()
	}
	// PowerShell 查询：获取所有 PNPDeviceID 含 VID_&PID_ 的设备
	// 强制 UTF-8 输出，避免中文设备名乱码
	cmd := exec.Command(psExe, "-NoProfile", "-NonInteractive", "-Command",
		`[Console]::OutputEncoding = [Text.Encoding]::UTF8; Get-WmiObject Win32_PnPEntity | Where-Object { $_.PNPDeviceID -match 'VID_[0-9A-Fa-f]{4}&PID_[0-9A-Fa-f]{4}' } | Select-Object Name, PNPDeviceID, ConfigManagerErrorCode | ConvertTo-Json -Compress`)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	output, err := cmd.Output()
	if err != nil {
		return listUSBDevicesWMIC()
	}
	// PowerShell 5.1 在 Windows 上默认输出 UTF-16 LE（带 BOM），需要转换为 UTF-8
	// Windows PowerShell (5.1) 的 ConvertTo-Json 输出是 UTF-16 LE 编码
	text := decodePowerShellOutput(output)
	text = strings.TrimSpace(text)
	output = []byte(text)
	if len(output) == 0 {
		return []USBDevice{}, nil
	}

	// WMI 返回单个对象时不是数组，需要包装
	if output[0] != '[' {
		output = []byte("[" + string(output) + "]")
	}

	var raw []struct {
		Name                  string `json:"Name"`
		PNPDeviceID           string `json:"PNPDeviceID"`
		ConfigManagerErrorCode int    `json:"ConfigManagerErrorCode"`
	}
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, fmt.Errorf("解析USB设备列表失败: %w", err)
	}

	vidPidRe := regexp.MustCompile(`VID_([0-9A-Fa-f]{4})&PID_([0-9A-Fa-f]{4})`)
	seen := make(map[string]bool) // 去重（同一 VID:PID 只显示一次）
	var devices []USBDevice

	for _, item := range raw {
		m := vidPidRe.FindStringSubmatch(item.PNPDeviceID)
		if m == nil {
			continue
		}
		vid := strings.ToUpper(m[1])
		pid := strings.ToUpper(m[2])
		key := vid + ":" + pid
		if seen[key] {
			continue
		}
		seen[key] = true

		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = fmt.Sprintf("USB设备 %s:%s", vid, pid)
		}

		devices = append(devices, USBDevice{
			Name:      name,
			VID:       vid,
			PID:       pid,
			DeviceID:  item.PNPDeviceID,
			Available: item.ConfigManagerErrorCode == 0, // 0 = 设备正常工作
		})
	}

	// 按名称排序
	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Name < devices[j].Name
	})

	return devices, nil
}

// listUSBDevicesWMIC 使用 wmic 命令枚举 USB 设备（旧系统兼容）
func listUSBDevicesWMIC() ([]USBDevice, error) {
	// wmic 默认输出系统编码（中文系统为 GBK），需用 chcp 65001 切换到 UTF-8
	cmd := exec.Command("cmd", "/c", "chcp", "65001", ">", "nul", "&&", "wmic", "path", "Win32_PnPEntity", "where",
		"PNPDeviceID like 'USB%'", "get", "Name,PNPDeviceID", "/format:csv")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("无法枚举USB设备: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	vidPidRe := regexp.MustCompile(`VID_([0-9A-Fa-f]{4})&PID_([0-9A-Fa-f]{4})`)
	seen := make(map[string]bool)
	var devices []USBDevice

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Node,Name,PNPDeviceID") {
			continue
		}
		m := vidPidRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		vid := strings.ToUpper(m[1])
		pid := strings.ToUpper(m[2])
		key := vid + ":" + pid
		if seen[key] {
			continue
		}
		seen[key] = true

		// CSV 格式：Node,Name,PNPDeviceID
		parts := strings.SplitN(line, ",", 3)
		name := "USB设备"
		if len(parts) >= 2 {
			name = strings.TrimSpace(parts[1])
		}
		if name == "" {
			name = fmt.Sprintf("USB设备 %s:%s", vid, pid)
		}

		devices = append(devices, USBDevice{
			Name:      name,
			VID:       vid,
			PID:       pid,
			DeviceID:  line,
			Available: true,
		})
	}

	return devices, nil
}

// decodePowerShellOutput 解码 PowerShell 输出
// Windows PowerShell 5.1 的 ConvertTo-Json 输出为 UTF-16 LE（带 BOM）
// PowerShell Core 7+ 输出为 UTF-8
// 此函数自动检测编码并转换为 UTF-8 字符串
func decodePowerShellOutput(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	// 检测 UTF-16 LE BOM
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		data = data[2:]
		if len(data)%2 != 0 {
			data = data[:len(data)-1]
		}
		u16s := make([]uint16, len(data)/2)
		for i := range u16s {
			u16s[i] = uint16(data[i*2]) | uint16(data[i*2+1])<<8
		}
		return string(utf16.Decode(u16s))
	}
	// 检测 UTF-16 BE BOM
	if len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF {
		data = data[2:]
		if len(data)%2 != 0 {
			data = data[:len(data)-1]
		}
		u16s := make([]uint16, len(data)/2)
		for i := range u16s {
			u16s[i] = uint16(data[i*2])<<8 | uint16(data[i*2+1])
		}
		return string(utf16.Decode(u16s))
	}
	// UTF-8 BOM
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}
	return string(data)
}
