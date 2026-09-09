package tray

import (
	"bytes"
	"context"
	"embed"
	"encoding/binary"
	"image"
	"image/png"
	"sync"
	"sync/atomic"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed icon_source.png
var embeddedIconFS embed.FS

// Manager 系统托盘管理器（Windows 原生实现，不再依赖 getlantern/systray）。
// 设计目标：
//  - 用 Win32 Shell_NotifyIconW 自建隐藏消息窗口，精确区分左键/右键
//  - 左键单击 / 双击托盘图标 → 显示主窗口
//  - 右键托盘图标 → 弹出只有"退出程序"一个菜单项的弹出菜单
//  - 支持气泡通知、最小化到托盘（隐藏 Wails 主窗口）
type Manager struct {
	mu      sync.RWMutex
	wctx    context.Context // Wails runtime ctx，用于调用窗口操作/事件
	started bool
	enabled bool // 是否启用（Kiosk 模式下禁用托盘）
	quitFlag int32 // atomic：1 = 正在退出，避免重复退出
	quitHook func() // 退出前钩子（清理 Kiosk 标记、停止守护进程等）

	// 平台特定运行时数据（Windows: *windowsTrayData，在 tray_windows.go 中设置）
	platformData interface{}
}

// userRequestedQuit 全局原子标志：用户通过托盘菜单主动请求退出。
// main.go 的 OnBeforeClose 会检查此标志，为 true 时允许程序真正退出（不再阻止关闭）。
var userRequestedQuit int32 // atomic，0=正常运行，1=用户请求退出

// IsUserRequestedQuit 返回是否用户主动请求退出（供 main.go OnBeforeClose 检查）
func IsUserRequestedQuit() bool {
	return atomic.LoadInt32(&userRequestedQuit) == 1
}

// SetQuitHook 设置退出前钩子函数（用于清理 Kiosk 标记、停止守护进程等）
func (m *Manager) SetQuitHook(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.quitHook = fn
}

// New 创建一个托盘管理器（还未启动，需调用 Start）。
func New() *Manager {
	return &Manager{enabled: true}
}

// SetEnabled 控制是否启用托盘（Kiosk 模式下关闭）。
// 必须在 Start 之前或之后调用均可：如果已经启动且传 false，则停止托盘；
// 传 true 且尚未启动时会立即启动。
func (m *Manager) SetEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = enabled
	if !enabled && m.started {
		m.started = false
		m.mu.Unlock()
		m.stopPlatform()
		m.mu.Lock()
	}
}

// BindWailsContext 绑定 Wails runtime ctx（Wails OnStartup 时调用）。
func (m *Manager) BindWailsContext(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wctx = ctx
}

// Start 在独立 goroutine 中启动系统托盘（非阻塞）。
func (m *Manager) Start() {
	m.mu.RLock()
	if m.started || !m.enabled {
		m.mu.RUnlock()
		return
	}
	m.mu.RUnlock()

	m.mu.Lock()
	m.started = true
	m.mu.Unlock()

	// 委托给平台特定实现（tray_windows.go / tray_other.go）
	m.startPlatform()
}

// ---------- UI 动作（Wails 侧回调） ----------

// ShowWindow 从托盘还原显示主窗口（并前置）。
func (m *Manager) ShowWindow() {
	ctx := m.wailsCtx()
	if ctx == nil {
		return
	}
	wailsruntime.WindowShow(ctx)
	wailsruntime.WindowUnminimise(ctx)
	wailsruntime.WindowCenter(ctx)
}

// HideWindow 隐藏主窗口到托盘（不退出）。
func (m *Manager) HideWindow() {
	ctx := m.wailsCtx()
	if ctx == nil {
		return
	}
	wailsruntime.WindowHide(ctx)
}

// NavigateSettings 切换到设置页：先显示窗口，再给前端发事件。
// （当前托盘菜单没有"设置"项，但保留此方法供侧边栏"设置"按钮或快捷键调用。）
func (m *Manager) NavigateSettings() {
	m.ShowWindow()
	ctx := m.wailsCtx()
	if ctx != nil {
		wailsruntime.EventsEmit(ctx, "tray:navigate", "settings")
	}
}

// Quit 真正退出程序（托盘菜单"退出程序"触发，或侧边栏"退出"触发）。
// 原子 CAS 防止重复退出，先设置全局退出标志（让 OnBeforeClose 放行），
// 再调用退出钩子（清理 Kiosk 标记、停止守护进程），最后停止托盘 + 退出 Wails。
func (m *Manager) Quit() {
	if !atomic.CompareAndSwapInt32(&m.quitFlag, 0, 1) {
		return
	}
	// 1. 设置全局"用户主动退出"标志 → main.go OnBeforeClose 检查到此标志后 return false（允许退出）
	atomic.StoreInt32(&userRequestedQuit, 1)
	// 2. 调用退出前钩子（清理 Kiosk 标记文件、停止键盘钩子和守护进程）
	m.mu.RLock()
	hook := m.quitHook
	m.mu.RUnlock()
	if hook != nil {
		defer func() { _ = recover() }()
		hook()
	}
	// 3. 停止托盘（删除图标 + 退出消息循环）
	m.stopPlatform()
	// 4. 退出 Wails（触发 OnBeforeClose → 此时 IsUserRequestedQuit()=true → return false → 程序退出）
	ctx := m.wailsCtx()
	if ctx != nil {
		wailsruntime.Quit(ctx)
	}
}

// Notify 气泡通知（当前仅设置 tooltip，未来可用 Shell_NotifyIcon 气球扩展）。
func (m *Manager) Notify(title, msg string) {
	// 简单实现：暂无独立气泡 API，保留接口供未来扩展
	_ = title
	_ = msg
}

// ---------- helpers ----------

func (m *Manager) wailsCtx() context.Context {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.wctx
}

// ============================================================
//  appIconBytes()：从嵌入的 icon_source.png（品牌真实图标）解码 PNG，
//  缩放到 32x32 后打包成合法 ICO，供 Win32 CreateIconFromResource 使用。
//  不再用内存手绘"紫色云"占位，真正显示用户的品牌蓝云。
// ============================================================

func appIconBytes() []byte {
	const target = 32
	// 1) 从嵌入 FS 读取原始 PNG 字节
	srcData, err := embeddedIconFS.ReadFile("icon_source.png")
	if err != nil {
		return fallbackIconBytes() // 极端情况：PNG 嵌入丢失 → 退化到手绘占位云
	}
	// 2) image/png 解码 → NRGBA
	srcImg, err := png.Decode(bytes.NewReader(srcData))
	if err != nil {
		return fallbackIconBytes()
	}
	// 3) 最近邻缩放到 target × target（简单、够用，托盘 32x32 肉眼看不出差别）
	scaled := nearestNeighbor(srcImg, target, target)
	// 4) 把 NRGBA(RGBA) 转成 ICO 需要的 BGRA 32bit 逐行自底向上像素
	w, h := target, target
	bgra := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		srcY := h - 1 - y // ICO/DIB 自底向上
		for x := 0; x < w; x++ {
			r, g, b, a := scaled.At(x, srcY).RGBA()
			i := (y*w + x) * 4
			bgra[i+0] = uint8(b >> 8)
			bgra[i+1] = uint8(g >> 8)
			bgra[i+2] = uint8(r >> 8)
			bgra[i+3] = uint8(a >> 8)
		}
	}
	return buildICO32x32BGRA(bgra, w, h)
}

// fallbackIconBytes：嵌入 PNG 读取失败时的保底（32x32 紫色云占位）。
func fallbackIconBytes() []byte {
	const w, h = 32, 32
	pixels := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			r := 6
			in := inRoundRect(x, y, w, h, r)
			if in {
				t := float32(y) / float32(h-1)
				R := uint8(0x81 - float32(0x81-0x63)*t)
				G := uint8(0x8c - float32(0x8c-0x66)*t)
				B := uint8(0xf8 - float32(0xf8-0xf1)*t)
				dist := distToBorder(x, y, w, h, r)
				if dist < 2 {
					R = scaleByte(R, 0.88)
					G = scaleByte(G, 0.88)
					B = scaleByte(B, 0.88)
				}
				pixels[i+0] = B
				pixels[i+1] = G
				pixels[i+2] = R
				pixels[i+3] = 0xFF
			} else {
				pixels[i+3] = 0x00
			}
		}
	}
	drawCloud(pixels, w, h, 16, 18, 7)
	return buildICO32x32BGRA(pixels, w, h)
}

// nearestNeighbor 最邻近缩放（托盘 32x32 足够，不引入第三方图像库）
func nearestNeighbor(src image.Image, nw, nh int) image.NRGBA {
	sw := src.Bounds().Dx()
	sh := src.Bounds().Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy := y * sh / nh
		for x := 0; x < nw; x++ {
			sx := x * sw / nw
			r, g, b, a := src.At(sx+src.Bounds().Min.X, sy+src.Bounds().Min.Y).RGBA()
			off := dst.PixOffset(x, y)
			dst.Pix[off+0] = uint8(r >> 8)
			dst.Pix[off+1] = uint8(g >> 8)
			dst.Pix[off+2] = uint8(b >> 8)
			dst.Pix[off+3] = uint8(a >> 8)
		}
	}
	return *dst
}

// ---- 小几何工具 ----

func inRoundRect(x, y, w, h, r int) bool {
	if x < 0 || y < 0 || x >= w || y >= h {
		return false
	}
	// 如果在四角
	left := x < r
	right := x >= w-r
	top := y < r
	bottom := y >= h-r
	if !(left || right || top || bottom) {
		return true
	}
	// 选择圆心
	cx, cy := r, r
	if right {
		cx = w - 1 - r
	}
	if bottom {
		cy = h - 1 - r
	}
	dx := x - cx
	dy := y - cy
	return dx*dx+dy*dy <= r*r
}

func distToBorder(x, y, w, h, r int) float64 {
	// 返回点到圆角矩形边缘的最近距离（正数=内部，负数=外部）
	d := min(x, y, (w-1)-x, (h-1)-y)
	return float64(d)
}

func drawCloud(pixels []byte, w, h, cx, cy, r int) {
	// 一朵云：大圆 (cx,cy,r) + 左小圆 (cx-r, cy, r-2) + 右小圆 (cx+r, cy, r-2) + 底部带
	dirs := [][3]int{
		{cx - r, cy + 1, r - 2},
		{cx + r - 1, cy + 1, r - 3},
		{cx, cy - 1, r},
	}
	for y := cy - r; y <= cy+r+4; y++ {
		for x := cx - 2*r; x <= cx+2*r; x++ {
			if x < 0 || y < 0 || x >= w || y >= h {
				continue
			}
			inside := false
			for _, d := range dirs {
				dx := x - d[0]
				dy := y - d[1]
				rr := d[2]
				if dx*dx+dy*dy <= rr*rr {
					inside = true
					break
				}
			}
			// 底部腰带：cy 到 cy+r，横向 cx-2r+3 ~ cx+2r-5
			if !inside {
				if y >= cy-1 && y <= cy+r+2 && x >= cx-2*r+4 && x <= cx+2*r-6 {
					inside = true
				}
			}
			if !inside {
				continue
			}
			i := (y*w + x) * 4
			// 只有背景不透明部分我们才涂色（避免伸出圆角方块）
			if pixels[i+3] == 0 {
				continue
			}
			pixels[i+0] = 0xFF
			pixels[i+1] = 0xFE
			pixels[i+2] = 0xFF
			pixels[i+3] = 0xF8
		}
	}
}

func scaleByte(v uint8, ratio float32) uint8 {
	n := float32(v) * ratio
	if n < 0 {
		return 0
	}
	if n > 255 {
		return 255
	}
	return uint8(n)
}

// buildICO32x32BGRA 生成单帧 32x32 的合法 .ico 文件字节。
func buildICO32x32BGRA(bgra []byte, w, h int) []byte {
	// BGRA 像素在 ICO/DIB 中是 自底向上 的（行翻转）
	flipped := make([]byte, len(bgra))
	for y := 0; y < h; y++ {
		srcY := h - 1 - y
		copy(flipped[y*w*4:], bgra[srcY*w*4:(srcY+1)*w*4])
	}
	// AND mask：按 1 位/像素，行 stride = (w+31)/32*4 字节
	stride := (w + 31) / 32 * 4
	mask := make([]byte, stride*h) // 全 0 = 不透明（依赖 alpha 通道）

	bihSize := uint32(40)
	dataSize := uint32(bihSize) + uint32(len(flipped)) + uint32(len(mask))

	buf := make([]byte, 0, 6+16+int(dataSize))
	// ICONDIR
	buf = append(buf,
		0x00, 0x00, // reserved
		0x01, 0x00, // type = 1 (ICO)
		0x01, 0x00, // count = 1
	)
	// ICONDIRENTRY
	entryOffset := uint32(22) // 6 + 16
	buf = append(buf,
		byte(w), byte(h), // width/height (0 = >=256)
		0x00,          // color count
		0x00,          // reserved
		0x01, 0x00,    // color planes
		0x20, 0x00,    // bits per pixel 32
	)
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], dataSize)
	buf = append(buf, tmp[:]...) // bytesInRes
	binary.LittleEndian.PutUint32(tmp[:], entryOffset)
	buf = append(buf, tmp[:]...) // imageOffset

	// BITMAPINFOHEADER
	binary.LittleEndian.PutUint32(tmp[:], bihSize)
	buf = append(buf, tmp[:]...)
	binary.LittleEndian.PutUint32(tmp[:], uint32(w))
	buf = append(buf, tmp[:]...)
	binary.LittleEndian.PutUint32(tmp[:], uint32(h*2)) // XOR + AND mask = 2*height
	buf = append(buf, tmp[:]...)
	buf = append(buf, 0x01, 0x00) // planes
	buf = append(buf, 0x20, 0x00) // bitCount 32
	buf = append(buf, 0x00, 0x00, 0x00, 0x00) // compression 0 (BI_RGB)
	binary.LittleEndian.PutUint32(tmp[:], uint32(len(flipped)+len(mask)))
	buf = append(buf, tmp[:]...) // sizeImage
	// 剩下的 bih 字段 填充 0
	buf = append(buf,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	)
	buf = append(buf, flipped...)
	buf = append(buf, mask...)
	return buf
}
