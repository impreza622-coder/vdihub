package main

// tools/genico：从 PNG 生成多尺寸 Windows .ico（Wails 打包 + EXE 图标用）
// 不依赖第三方图像库，只用 Go 标准库 image/png + 最邻近缩放。
//
// 用法（从项目根）：
//   go run ./tools/genico \
//       -src d:\AIxiangmu\vdihub\cmd\desktop\build\appicon.png \
//       -out d:\AIxiangmu\vdihub\cmd\desktop\build\windows\icon.ico

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
)

var sizes = []int{16, 24, 32, 48, 64, 128, 256}

// frame 表示 ICO 内一帧（一个尺寸的 BGRA 像素块）
type frame struct {
	size   int
	pixels []byte // BGRA w*h*4，自底向上
}

func main() {
	src := flag.String("src", "", "源 PNG 路径")
	out := flag.String("out", "", "输出 ICO 路径")
	flag.Parse()
	if *src == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "用法: genico -src <png> -out <ico>")
		os.Exit(2)
	}
	if err := run(*src, *out); err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
	fmt.Println("OK →", *out)
}

func run(src, out string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	// 每个尺寸生成 PNG(BGRA)+AND mask，然后拼装 ICO
	frames := make([]frame, 0, len(sizes))
	for _, s := range sizes {
		scaled := nearestNeighbor(img, s, s)
		bgra := make([]byte, s*s*4)
		for y := 0; y < s; y++ {
			srcY := s - 1 - y
			for x := 0; x < s; x++ {
				r, g, b, a := scaled.At(x, srcY).RGBA()
				i := (y*s + x) * 4
				bgra[i+0] = uint8(b >> 8)
				bgra[i+1] = uint8(g >> 8)
				bgra[i+2] = uint8(r >> 8)
				bgra[i+3] = uint8(a >> 8)
			}
		}
		frames = append(frames, frame{size: s, pixels: bgra})
	}
	// 拼装 ICO
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	return writeICO(f, frames)
}

// writeICO 写一个多尺寸 .ico 文件。
// 参考：https://en.wikipedia.org/wiki/ICO_(file_format)
func writeICO(w io.Writer, frames []frame) error {
	// 1) ICONDIR (6 字节)
	var tmp [4]byte
	_, err := w.Write([]byte{
		0x00, 0x00, // reserved
		0x01, 0x00, // type=1(ICO)
		byte(len(frames) & 0xFF), byte((len(frames) >> 8) & 0xFF),
	})
	if err != nil {
		return err
	}
	// 2) 先算每个 ICONDIRENTRY 里的 imageOffset
	dirSize := 6 + 16*len(frames)
	offsets := make([]uint32, len(frames))
	cur := uint32(dirSize)
	for i, fr := range frames {
		s := fr.size
		stride := (s + 31) / 32 * 4
		maskSize := stride * s
		dataSize := 40 + uint32(len(fr.pixels)) + uint32(maskSize)
		offsets[i] = cur
		cur += dataSize
	}
	// 3) ICONDIRENTRY 每帧 16 字节
	for i, fr := range frames {
		s := fr.size
		w_ := byte(s)
		h_ := byte(s)
		if s >= 256 {
			w_ = 0
			h_ = 0
		}
		stride := (s + 31) / 32 * 4
		maskSize := stride * s
		dataSize := 40 + uint32(len(fr.pixels)) + uint32(maskSize)
		_, err := w.Write([]byte{
			w_, h_,        // width/height (0 = >=256)
			0x00,          // color count
			0x00,          // reserved
			0x01, 0x00,    // color planes
			0x20, 0x00,    // bpp 32
		})
		if err != nil {
			return err
		}
		binary.LittleEndian.PutUint32(tmp[:], dataSize)
		if _, err := w.Write(tmp[:]); err != nil {
			return err
		}
		binary.LittleEndian.PutUint32(tmp[:], offsets[i])
		if _, err := w.Write(tmp[:]); err != nil {
			return err
		}
	}
	// 4) 每帧 BITMAPINFOHEADER + BGRA + AND mask
	for _, fr := range frames {
		s := fr.size
		stride := (s + 31) / 32 * 4
		mask := make([]byte, stride*s) // 全 0 → 依赖 alpha 通道即可
		bih := make([]byte, 40)
		binary.LittleEndian.PutUint32(bih[0:4], 40)                  // biSize
		binary.LittleEndian.PutUint32(bih[4:8], uint32(s))           // biWidth
		binary.LittleEndian.PutUint32(bih[8:12], uint32(s*2))        // biHeight (XOR + AND)
		binary.LittleEndian.PutUint16(bih[12:14], 1)                 // biPlanes
		binary.LittleEndian.PutUint16(bih[14:16], 32)                // biBitCount
		binary.LittleEndian.PutUint32(bih[20:24], uint32(len(fr.pixels)+len(mask))) // biSizeImage
		if _, err := w.Write(bih); err != nil {
			return err
		}
		if _, err := w.Write(fr.pixels); err != nil {
			return err
		}
		if _, err := w.Write(mask); err != nil {
			return err
		}
	}
	return nil
}

func nearestNeighbor(src image.Image, nw, nh int) image.NRGBA {
	sw := src.Bounds().Dx()
	sh := src.Bounds().Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	minX := src.Bounds().Min.X
	minY := src.Bounds().Min.Y
	for y := 0; y < nh; y++ {
		sy := y * sh / nh
		for x := 0; x < nw; x++ {
			sx := x * sw / nw
			r, g, b, a := src.At(sx+minX, sy+minY).RGBA()
			off := dst.PixOffset(x, y)
			dst.Pix[off+0] = uint8(r >> 8)
			dst.Pix[off+1] = uint8(g >> 8)
			dst.Pix[off+2] = uint8(b >> 8)
			dst.Pix[off+3] = uint8(a >> 8)
		}
	}
	return *dst
}
