package main

import (
	"encoding/binary"
	"os"
)

// 托盘图标完全由代码绘制，不依赖外部 .ico 文件，保证单文件交付。
// 渲染流程：在 128x128 画布上以 4 倍超采样绘制，再降采样到目标尺寸以获得平滑边缘。

const glyphSize = 128

type canvas struct {
	w, h int
	px   []uint32 // 0xAARRGGBB
}

func newCanvas(w, h int) *canvas {
	return &canvas{w: w, h: h, px: make([]uint32, w*h)}
}

func (c *canvas) set(x, y int, col uint32) {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	c.px[y*c.w+x] = col
}

func (c *canvas) fillRect(x, y, w, h int, col uint32) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			c.set(xx, yy, col)
		}
	}
}

// fillRoundRect 绘制圆角矩形。
func (c *canvas) fillRoundRect(x, y, w, h, r int, col uint32) {
	if r <= 0 {
		c.fillRect(x, y, w, h, col)
		return
	}
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			cx, cy := xx, yy
			if xx < r {
				cx = r
			} else if xx > w-1-r {
				cx = w - 1 - r
			}
			if yy < r {
				cy = r
			} else if yy > h-1-r {
				cy = h - 1 - r
			}
			dx, dy := xx-cx, yy-cy
			if dx*dx+dy*dy > r*r {
				continue
			}
			c.set(x+xx, y+yy, col)
		}
	}
}

// fillTriangle 用扫描线填充三角形。
func (c *canvas) fillTriangle(x1, y1, x2, y2, x3, y3 int, col uint32) {
	minY := y1
	if y2 < minY {
		minY = y2
	}
	if y3 < minY {
		minY = y3
	}
	maxY := y1
	if y2 > maxY {
		maxY = y2
	}
	if y3 > maxY {
		maxY = y3
	}
	for y := minY; y <= maxY; y++ {
		var xs [3]int
		n := 0
		add := func(ax, ay, bx, by int) {
			if ay == by {
				return
			}
			if (y >= ay && y <= by) || (y >= by && y <= ay) {
				t := float64(y-ay) / float64(by-ay)
				xs[n] = ax + int(float64(bx-ax)*t+0.5)
				n++
			}
		}
		add(x1, y1, x2, y2)
		add(x2, y2, x3, y3)
		add(x3, y3, x1, y1)
		if n < 2 {
			continue
		}
		lo, hi := xs[0], xs[0]
		for i := 1; i < n; i++ {
			if xs[i] < lo {
				lo = xs[i]
			}
			if xs[i] > hi {
				hi = xs[i]
			}
		}
		for x := lo; x <= hi; x++ {
			c.set(x, y, col)
		}
	}
}

// downsample 按整数倍降采样，使用预乘 alpha 平均，避免边缘发黑。
func (c *canvas) downsample(scale int) *canvas {
	if scale <= 1 {
		return c
	}
	ow, oh := c.w/scale, c.h/scale
	out := newCanvas(ow, oh)
	n := uint32(scale * scale)
	for y := 0; y < oh; y++ {
		for x := 0; x < ow; x++ {
			var r, g, b, a uint32
			for dy := 0; dy < scale; dy++ {
				row := (y*scale + dy) * c.w
				for dx := 0; dx < scale; dx++ {
					p := c.px[row+x*scale+dx]
					pa := (p >> 24) & 0xff
					r += ((p >> 16) & 0xff) * pa / 255
					g += ((p >> 8) & 0xff) * pa / 255
					b += (p & 0xff) * pa / 255
					a += pa
				}
			}
			aa := a / n
			if aa == 0 {
				continue
			}
			rr := clamp8(r / n * 255 / aa)
			gg := clamp8(g / n * 255 / aa)
			bb := clamp8(b / n * 255 / aa)
			out.px[y*ow+x] = aa<<24 | rr<<16 | gg<<8 | bb
		}
	}
	return out
}

func clamp8(v uint32) uint32 {
	if v > 255 {
		return 255
	}
	return v
}

// renderGlyph 绘制图标主体：蓝色圆角方块 + 白色「上传/发布」箭头。
func renderGlyph() *canvas {
	const (
		bg     = 0xFF2F6FEB
		white  = 0xFFFFFFFF
		size   = glyphSize
		radius = 28 // 约 7/32
	)
	c := newCanvas(size, size)
	c.fillRoundRect(0, 0, size, size, radius, bg)

	// 箭头（向上）
	c.fillTriangle(64, 30, 36, 62, 92, 62, white)
	c.fillRect(52, 60, 24, 28, white)
	// 底部托板
	c.fillRoundRect(32, 96, 64, 11, 5, white)

	return c
}

// iconBits 生成用于 CreateIconFromResourceEx 的图标资源数据
// （BITMAPINFOHEADER + XOR 位图 + AND 掩码，不含 ICO 文件头）。
func iconBits(size int) []byte {
	scale := glyphSize / size
	if scale < 1 {
		scale = 1
	}
	img := renderGlyph().downsample(scale)

	hdr := make([]byte, 40)
	binary.LittleEndian.PutUint32(hdr[0:], 40)             // biSize
	binary.LittleEndian.PutUint32(hdr[4:], uint32(size))   // biWidth
	binary.LittleEndian.PutUint32(hdr[8:], uint32(size*2)) // biHeight = XOR + AND
	binary.LittleEndian.PutUint16(hdr[12:], 1)             // biPlanes
	binary.LittleEndian.PutUint16(hdr[14:], 32)            // biBitCount
	binary.LittleEndian.PutUint32(hdr[16:], 0)             // BI_RGB

	xor := make([]byte, size*size*4)
	for y := 0; y < size; y++ {
		src := (size - 1 - y) * size // 位图为自下而上存储
		for x := 0; x < size; x++ {
			p := img.px[src+x]
			o := (y*size + x) * 4
			xor[o+0] = byte(p)       // B
			xor[o+1] = byte(p >> 8)  // G
			xor[o+2] = byte(p >> 16) // R
			xor[o+3] = byte(p >> 24) // A
		}
	}

	// AND 掩码全部置 0（不透明），透明度由 alpha 通道决定
	andRow := ((size + 31) / 32) * 4
	and := make([]byte, andRow*size)

	out := make([]byte, 0, len(hdr)+len(xor)+len(and))
	out = append(out, hdr...)
	out = append(out, xor...)
	out = append(out, and...)
	return out
}

// writeICO 生成多尺寸 .ico 文件（供设置程序图标等用途）。
func writeICO(path string, sizes []int) error {
	type img struct {
		size int
		data []byte
	}
	imgs := make([]img, 0, len(sizes))
	for _, s := range sizes {
		imgs = append(imgs, img{size: s, data: iconBits(s)})
	}

	buf := make([]byte, 0, 1024)
	head := make([]byte, 6)
	binary.LittleEndian.PutUint16(head[2:], 1) // type = icon
	binary.LittleEndian.PutUint16(head[4:], uint16(len(imgs)))
	buf = append(buf, head...)

	offset := 6 + 16*len(imgs)
	for _, im := range imgs {
		e := make([]byte, 16)
		if im.size >= 256 {
			e[0], e[1] = 0, 0
		} else {
			e[0], e[1] = byte(im.size), byte(im.size)
		}
		binary.LittleEndian.PutUint16(e[4:], 1)  // planes
		binary.LittleEndian.PutUint16(e[6:], 32) // bit count
		binary.LittleEndian.PutUint32(e[8:], uint32(len(im.data)))
		binary.LittleEndian.PutUint32(e[12:], uint32(offset))
		buf = append(buf, e...)
		offset += len(im.data)
	}
	for _, im := range imgs {
		buf = append(buf, im.data...)
	}
	return os.WriteFile(path, buf, 0o644)
}
