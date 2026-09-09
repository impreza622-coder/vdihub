//go:build !windows

package main

// listUSBDevicesOS 非 Windows 平台的 stub
func listUSBDevicesOS() ([]USBDevice, error) {
	return []USBDevice{}, nil
}
