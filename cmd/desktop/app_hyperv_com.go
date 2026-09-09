//go:build windows
// +build windows

package main

import (
	"errors"
	"fmt"
	"net"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

var (
	kernel32                         = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentThreadId uintptr = kernel32.NewProc("GetCurrentThreadId").Addr()
)

// getCurrentThreadID 获取当前 OS 线程 ID
func getCurrentThreadID() uint64 {
	r1, _, _ := syscall.SyscallN(procGetCurrentThreadId)
	return uint64(r1)
}

// ================================================================
//  Hyper-V 电源控制 —— 纯 COM/WMI 实现（不依赖 PowerShell）
//
//  通过 go-ole 直接调用 SWbemLocator → ConnectServer → ExecQuery →
//  ExecMethod_("RequestStateChange")，完全不需要 powershell.exe /
//  cscript.exe / wmic.exe 等外部进程。
//
//  并发控制：使用容量为 4 的信号量（channel），允许最多 4 个并发
//  COM 查询。超过 4 个时，最多等待 5 秒，超时则立即返回错误，
//  避免前端 15s 超时被锁等待耗尽。
// ================================================================

// comSem 并发信号量：最多 4 个并发 COM 查询
// 4 个并发比串行快 4 倍，且不会导致 COM 初始化冲突（COINIT_MULTITHREADED 支持多线程）
var comSem = make(chan struct{}, 4)

// threadCOMMap 记录每个 OS 线程是否已初始化 COM
// key = OS thread ID（由 GetCurrentThreadId 获取）
// value = 是否已 CoInitializeEx
var (
	threadCOMMap sync.Map
)

// initCOMThreadSafe 线程安全的 COM 初始化
// 如果当前 OS 线程已初始化过 COM，直接返回；否则执行 CoInitializeEx
func initCOMThreadSafe() (needUninit bool) {
	tid := getCurrentThreadID()
	if _, loaded := threadCOMMap.Load(tid); loaded {
		return false // 本线程已初始化过，不需要 CoUninitialize
	}
	err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)
	if err == nil {
		threadCOMMap.Store(tid, true)
		return true
	}
	// S_FALSE = 已初始化；RPC_E_CHANGED_MODE = 已用 STA 初始化
	// 两种情况 COM 都可用，不需要 CoUninitialize
	threadCOMMap.Store(tid, true)
	return false
}

// releaseCOMThread 如果本线程初始化过 COM，则 CoUninitialize 并清除记录
func releaseCOMThread() {
	tid := getCurrentThreadID()
	if _, loaded := threadCOMMap.LoadAndDelete(tid); loaded {
		ole.CoUninitialize()
	}
}

// acquireCOMSem 获取 COM 并发信号量，最多等待 maxWait 秒
func acquireCOMSem(maxWait time.Duration) error {
	if maxWait <= 0 {
		select {
		case comSem <- struct{}{}:
			return nil
		default:
			return fmt.Errorf("COM 查询繁忙，请稍后重试")
		}
	}
	timer := time.NewTimer(maxWait)
	defer timer.Stop()
	select {
	case comSem <- struct{}{}:
		return nil
	case <-timer.C:
		return fmt.Errorf("COM 查询队列已满（等待 %v 超时），请稍后重试", maxWait)
	}
}

// releaseCOMSem 释放 COM 并发信号量
func releaseCOMSem() {
	<-comSem
}

// connectWMI 连接到 Hyper-V WMI 命名空间（本地或远程），返回 SWbemServices dispatch
// host="" / "."  → 本地连接；否则远程连接（可带凭据）
func connectWMI(host, username, password string) (*ole.IDispatch, func(), error) {
	namespace := `root\virtualization\v2`
	host = strings.TrimSpace(host)
	remote := host != "" && host != "."

	// 1. 创建 SWbemLocator
	locator, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return nil, nil, fmt.Errorf("创建 SWbemLocator 失败: %w", err)
	}
	locatorDisp, err := locator.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		locator.Release()
		return nil, nil, fmt.Errorf("查询 IDispatch 失败: %w", err)
	}

	// 2. ConnectServer
	var serviceResult *ole.VARIANT
	if remote {
		normalizedUser := normalizeHyperVUsername(host, username, true)
		if normalizedUser != "" {
			serviceResult, err = oleutil.CallMethod(locatorDisp, "ConnectServer",
				host, namespace, normalizedUser, password, "", "", 0, nil)
		} else {
			serviceResult, err = oleutil.CallMethod(locatorDisp, "ConnectServer",
				host, namespace, "", "", "", "", 0, nil)
		}
	} else {
		serviceResult, err = oleutil.CallMethod(locatorDisp, "ConnectServer",
			".", namespace, "", "", "", "", 0, nil)
	}
	locatorDisp.Release()
	locator.Release()
	if err != nil {
		return nil, nil, fmt.Errorf("连接 WMI 服务器失败: %w", err)
	}
	service := serviceResult.ToIDispatch()

	cleanup := func() {
		service.Release()
	}
	return service, cleanup, nil
}

// queryVMObject 查询 Msvm_ComputerSystem，返回 VM dispatch
func queryVMObject(service *ole.IDispatch, vmName string) (*ole.IDispatch, func(), error) {
	wql := fmt.Sprintf("SELECT * FROM Msvm_ComputerSystem WHERE ElementName='%s'", escapeWQLFilter(vmName))
	queryResult, err := oleutil.CallMethod(service, "ExecQuery", wql)
	if err != nil {
		return nil, nil, fmt.Errorf("WMI 查询失败: %w", err)
	}
	items := queryResult.ToIDispatch()

	// 检查 Count
	countVal, err := oleutil.GetProperty(items, "Count")
	if err != nil {
		items.Release()
		return nil, nil, fmt.Errorf("获取查询结果数量失败: %w", err)
	}
	count := int(countVal.Val)
	countVal.Clear()
	if count == 0 {
		items.Release()
		return nil, nil, fmt.Errorf("Hyper-V 虚拟机 '%s' 不存在", vmName)
	}

	// 获取第一个 VM 对象
	itemVal, err := oleutil.CallMethod(items, "ItemIndex", 0)
	items.Release()
	if err != nil {
		return nil, nil, fmt.Errorf("获取 VM 对象失败: %w", err)
	}
	vm := itemVal.ToIDispatch()

	cleanup := func() {
		vm.Release()
	}
	return vm, cleanup, nil
}

// hyperVControlViaCOM 通过 COM/WMI 直接控制 Hyper-V 虚拟机电源状态
// requestedState: 2=开机, 3=硬关, 4=软关, 10=硬重启, 11=软重启, 32770=暂停, 32771=恢复
func hyperVControlViaCOM(host, vmName, username, password string, requestedState int) error {
	// 1. 获取并发信号量（最多等 5 秒，避免阻塞前端超时）
	if err := acquireCOMSem(5 * time.Second); err != nil {
		return err
	}
	defer releaseCOMSem()

	// 2. 锁定 OS 线程 + 线程安全 COM 初始化
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	needUninit := initCOMThreadSafe()
	if needUninit {
		defer releaseCOMThread()
	}

	// 3. 连接 WMI
	service, svcCleanup, err := connectWMI(host, username, password)
	if err != nil {
		return err
	}
	defer svcCleanup()

	// 4. 查询 VM
	vm, vmCleanup, err := queryVMObject(service, vmName)
	if err != nil {
		return err
	}
	defer vmCleanup()

	// 5. 调用 RequestStateChange 方法
	methodsVal, err := oleutil.GetProperty(vm, "Methods_")
	if err != nil {
		return fmt.Errorf("获取 Methods_ 失败: %w", err)
	}
	methods := methodsVal.ToIDispatch()
	defer methods.Release()

	methodVal, err := oleutil.CallMethod(methods, "Item", "RequestStateChange")
	if err != nil {
		return fmt.Errorf("获取 RequestStateChange 方法定义失败: %w", err)
	}
	method := methodVal.ToIDispatch()
	defer method.Release()

	inParamsVal, err := oleutil.GetProperty(method, "InParameters")
	if err != nil {
		return fmt.Errorf("获取 InParameters 失败: %w", err)
	}
	inParams := inParamsVal.ToIDispatch()
	defer inParams.Release()

	spawnedVal, err := oleutil.CallMethod(inParams, "SpawnInstance_")
	if err != nil {
		return fmt.Errorf("创建参数实例失败: %w", err)
	}
	spawned := spawnedVal.ToIDispatch()
	defer spawned.Release()

	_, err = oleutil.PutProperty(spawned, "RequestedState", requestedState)
	if err != nil {
		return fmt.Errorf("设置 RequestedState 参数失败: %w", err)
	}

	outParamsVal, err := oleutil.CallMethod(vm, "ExecMethod_", "RequestStateChange", spawned)
	if err != nil {
		return fmt.Errorf("执行 RequestStateChange 失败: %w", err)
	}
	outParams := outParamsVal.ToIDispatch()
	defer outParams.Release()

	retVal, err := oleutil.GetProperty(outParams, "ReturnValue")
	if err != nil {
		return fmt.Errorf("获取 ReturnValue 失败: %w", err)
	}
	returnValue := int(retVal.Val)
	retVal.Clear()

	if returnValue != 0 && returnValue != 4096 {
		return fmt.Errorf("Hyper-V 操作失败，返回码=%d", returnValue)
	}
	return nil
}

// hyperVStatusViaCOM 通过 COM/WMI 查询 Hyper-V 虚拟机当前状态
// 返回 EnabledState 原始值：2=运行中, 3=已关闭, 32770=已暂停, 32773=已保存, 32774=正在启动, ...
func hyperVStatusViaCOM(host, vmName, username, password string) (int, error) {
	// 1. 获取并发信号量（最多等 5 秒，避免阻塞前端超时）
	if err := acquireCOMSem(5 * time.Second); err != nil {
		return 0, err
	}
	defer releaseCOMSem()

	// 2. 锁定 OS 线程 + 线程安全 COM 初始化
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	needUninit := initCOMThreadSafe()
	if needUninit {
		defer releaseCOMThread()
	}

	// 3. 连接 WMI（带重试：同一主机并发 DCOM ConnectServer 凭据协商竞争时，
	//    会偶发 0x80070005「拒绝访问」，稍候重试即可成功）
	var service *ole.IDispatch
	var svcCleanup func()
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		service, svcCleanup, err = connectWMI(host, username, password)
		if err == nil {
			break
		}
		if !isWMIAccessDenied(err) || attempt == 2 {
			return 0, err
		}
		time.Sleep(300 * time.Millisecond)
	}
	defer svcCleanup()

	// 4. 查询 VM
	vm, vmCleanup, err := queryVMObject(service, vmName)
	if err != nil {
		return 0, err
	}
	defer vmCleanup()

	// 5. 读取 EnabledState 属性
	stateVal, err := oleutil.GetProperty(vm, "EnabledState")
	if err != nil {
		return 0, fmt.Errorf("获取 EnabledState 失败: %w", err)
	}
	enabledState := int(stateVal.Val)
	stateVal.Clear()

	return enabledState, nil
}

// isWMIAccessDenied 判断错误是否为 DCOM 0x80070005 拒绝访问（并发凭据协商的瞬时错误）
func isWMIAccessDenied(err error) bool {
	var oe *ole.OleError
	if errors.As(err, &oe) {
		return oe.Code() == 0x80070005
	}
	msg := err.Error()
	return strings.Contains(msg, "0x80070005") || strings.Contains(msg, "拒绝访问")
}

// mapEnabledStateToString 将 Hyper-V EnabledState 原始值映射为前端状态字符串
func mapEnabledStateToString(state int) string {
	switch state {
	case 2, 32770, 32774, 32773, 32776, 32777, 32771:
		return "running"
	case 3:
		return "stopped"
	case 32769, 32768:
		return "paused"
	default:
		return ""
	}
}

// ================================================================
//  RDP 端口检测 —— 用于没有电源控制后端的 RDP/FreeRDP 虚拟机
//
//  当虚拟机没有配置 power_backend（none）时，通过 TCP 连接
//  RDP 端口（默认 3389）判断虚拟机是否在线：
//    - 连接成功（有响应）→ running
//    - 连接拒绝/超时      → stopped
//  这种方式不需要任何 API 权限，纯 TCP 检测。
// ================================================================

// checkRDPPort 通过 TCP 检测 RDP 端口是否可达
// host: 虚拟机 IP 地址；port: 默认 3389；timeout: 超时时间
func checkRDPPort(host string, port int, timeout time.Duration) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("RDP 检测需要虚拟机 IP 地址")
	}
	if port <= 0 {
		port = 3389
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		// 连接失败 = 虚拟机离线
		return "stopped", nil
	}
	conn.Close()
	return "running", nil
}

// checkAnyPort 通过多端口探测判断虚拟机是否在线
// 依次探测：3389(RDP) → 443(HTTPS/Horizon Blast) → 5900(VNC/SPICE)
// 任意一个端口能建立 TCP 连接就算 running，全部不通才算 stopped
func checkAnyPort(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("TCP 检测需要虚拟机 IP 地址")
	}

	ports := []int{3389, 443, 5900}
	singleTimeout := 2 * time.Second
	for _, p := range ports {
		addr := fmt.Sprintf("%s:%d", host, p)
		conn, err := net.DialTimeout("tcp", addr, singleTimeout)
		if err == nil {
			conn.Close()
			return "running", nil
		}
	}
	// 全部不通 = 虚拟机离线
	return "stopped", nil
}
