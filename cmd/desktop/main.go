package main

import (
	"context"
	"embed"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"vdihub/internal/tray"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed VERSION
var versionRaw []byte

// bootstrapSystemPATH 启动时扩展进程 PATH，确保系统关键工具（PowerShell/cmd/shutdown）都能找到
// 解决某些精简版 Windows PATH 被截断时 "exec: powershell.exe not found in %PATH%" 错误
func bootstrapSystemPATH() {
	windir := os.Getenv("SystemRoot")
	if windir == "" {
		windir = `C:\Windows`
	}
	extra := []string{
		filepath.Join(windir, "System32"),
		filepath.Join(windir, "System32", "WindowsPowerShell", "v1.0"),
		filepath.Join(windir, "SysWOW64"),
		filepath.Join(windir, "SysWOW64", "WindowsPowerShell", "v1.0"),
	}
	oldPATH := os.Getenv("PATH")
	buf := oldPATH
	for _, p := range extra {
		// 避免重复追加
		if !containsPathEntry(oldPATH, p) {
			buf += `;` + p
		}
	}
	_ = os.Setenv("PATH", buf)

	// 另外尝试读取安装程序写的 _runtime_paths.txt，有就把目录也加进去
	if exe, err := os.Executable(); err == nil {
		hint := filepath.Join(filepath.Dir(exe), "_runtime_paths.txt")
		if data, err := os.ReadFile(hint); err == nil {
			_ = os.Setenv("THIN_CLIENT_RUNTIME_HINTS", string(data))
		}
	}
}

// containsPathEntry 判断 PATH 字符串中是否包含指定目录（简单包含即可，path/filepath 匹配大小写不敏感）
func containsPathEntry(pathList, entry string) bool {
	if pathList == "" || entry == "" {
		return false
	}
	// 大小写不敏感匹配
	upList := strings.ToUpper(pathList)
	upEntry := strings.ToUpper(entry)
	return strings.Contains(upList, upEntry)
}

func main() {
	// -1. PATH 环境变量兜底（必须在任何 exec.Command 之前执行）
	bootstrapSystemPATH()

	// 0. 解析启动参数（--kiosk-autostart 等）—— 让 GetStartHint 在 StartUp 之前就可用
	setGlobalStartHintFromArgs()

	// --__kiosk_priv：管理员子进程入口（解析负载并写注册表/替换 Shell/自动登录/控制面板）
	//   写操作只在真正以管理员身份启动的这个"短命子进程"中完成，
	//   正常客户端进程不继承管理员权限，避免 WebView2 表现异常
	if TryRunSystemKioskPrivPayload() {
		return
	}

	// 0.5 尽早安装 WH_KEYBOARD_LL 低级键盘钩子（主程序/Kiosk 的 ALT+F4 屏蔽在系统层就生效，避免 race）
	//     钩子默认不拦截（_altF4BlockActive=false），直到 ApplyKioskMode/LockdownKioskClose 打开开关
	installAltF4KeyboardHook()

	// 1. 单实例：如果老实例已存在，就激活老窗口后直接退出（不会开两个进程）
	mut, existing := ensureSingleInstance()
	defer releaseSingleInstance(mut)
	if existing {
		// 激活已完成（ensureSingleInstance 里弹到前台），本进程退出
		return
	}

	app := NewApp()
	authService := NewAuthService(app)
	vmService := NewVMService(authService)
	protocolService := NewProtocolService(app)

	err := wails.Run(&options.App{
		Title:     "瘦客户端云桌面系统",
		Width:     1100,
		Height:    790,
		MinWidth:  1100,
		MinHeight: 790,
		// 普通模式：点击关闭按钮 → 隐藏窗口到托盘（不退出进程）
		// Kiosk 模式下我们在 OnBeforeClose 里再次拦截，保证不退出
		HideWindowOnClose: true,
		Frameless:         true, // 默认无边框（标题栏由前端绘制）
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.StartUp,
		OnShutdown: app.Shutdown,
		// OnBeforeClose：窗口真正销毁之前的最后一道拦截（第三重保险）
		//  第一重：WH_KEYBOARD_LL 吞 ALT+F4（系统层）
		//  第二重：平台锁定 WS_SYSMENU + kiosk daemon 反复 apply
		//  第三重：这里 → 两种情况都 return true 阻止销毁
		//  ★ 例外：用户通过托盘菜单"退出程序"主动退出时，tray.Quit() 会设置
		//    tray.IsUserRequestedQuit()=true，此时放行（return false）让程序真正退出
		OnBeforeClose: func(ctx context.Context) (prevent bool) {
			// 用户主动请求退出（托盘菜单"退出程序"）→ 允许退出，不再拦截
			if tray.IsUserRequestedQuit() {
				return false
			}
			if app != nil && app.isKioskFlagOnDisk() {
				// Kiosk 模式：不但阻止关闭，还立刻把窗口拉回全屏显示（防止 Alt+F4 让它最小化/隐藏到桌面）
				defer func() { _ = recover() }()
				// 再次 apply 锁定（防止 Wails 即将销毁之前把 WS_SYSMENU 又加回来）
				platformLockdownMainWindowKioskMode()
				// 重新激活 + 全屏
				runtime.WindowShow(ctx)
				runtime.WindowFullscreen(ctx)
				return true
			}
			_ = app.WindowClose()
			// 普通 → 隐藏到托盘，进程继续跑
			return true
		},
		Bind: []interface{}{
			app,
			authService,
			vmService,
			protocolService,
		},
	})
	// 保持 mut 引用不被 GC 提前释放（如果 defer 没到）
	_ = unsafe.Pointer(mut)

	if err != nil {
		println("Error:", err.Error())
	}
}
