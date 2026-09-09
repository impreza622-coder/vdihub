//go:build !windows

package protocol

import "runtime"

// getSysProcAttr returns nil for non-Windows platforms.
// The type signature mirrors Windows variant but with generic pointer.
// However, since non-Windows builds do not use syscall.SysProcAttr
// and the protocol manager unix code simply doesn't set SysProcAttr,
// we provide a placeholder that is only compiled here.
// Note: we never reference syscall.SysProcAttr type here because
// it does not exist on non-Windows builds (Darwin/Linux structs are different).
func getSysProcAttr() *struct{} { return nil }

func fetchHorizonBrokerCert(host string, port int) (interface{}, error) { return nil, runtime.GOOS }
func ensureHostsEntry(ip, hostname string) (bool, error)                { return false, nil }
func installCertToRoot(der interface{}, label string) error             { return nil }
func runHorizonSendKeys(processID int, waitSeconds int)                 {}
func removeSysMenuForPid(pid uint32) bool                               { return false }
func collectHorizonPIDs(rootPID uint32) map[uint32]bool                 { return map[uint32]bool{rootPID: true} }
func collectDescendantPIDs(rootPID uint32) map[uint32]bool              { return map[uint32]bool{rootPID: true} }
func applyHorizonShadeHide()                                            {}
func applyHorizonIgnoreCert()                                           {}
func forceForegroundWindow(hwnd uintptr)                                {}
func hasVisibleDesktopWindow(rootPID uint32) bool                       { return true } // 非 Windows 无法检查，假定成功
func activateSessionWindow(rootPID uint32) bool                         { return true } // 非 Windows 无需激活
func restoreIconicSessionWindows(rootPID uint32) bool                   { return true } // 非 Windows 无需激活
func checkDotNet8DesktopRuntime() (bool, string)                        { return true, "non-Windows" }
func getBundledDotNetRuntimePath() string                               { return "" } // 非 Windows 无自带运行时
func killProcessTree(rootPID uint32)                                    {}
