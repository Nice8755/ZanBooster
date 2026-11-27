//go:build windows

package main

// crosshair.go: ถอดจาก exe 4.11.15 · เป้าเล็งซ้อนกลางจอเกม (พรีเมียม) เป็นหน้าต่างโปร่งใสคลิกทะลุ ไม่ยุ่งกับตัวเกม
// แสดงเฉพาะตอนหน้าต่างเกมอยู่ด้านหน้า (หรือ "แสดงตลอด") · ตั้งค่าในหน้าจอ settings.xh

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pGetClientRect       = user32.NewProc("GetClientRect")
	pClientToScreen      = user32.NewProc("ClientToScreen")
	pUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	pCreateDIBSection    = gdi32.NewProc("CreateDIBSection")
)

type xhCfg struct {
	On      bool   `json:"on"`
	Style   string `json:"style"`
	Color   string `json:"color"`
	Size    int    `json:"size"`
	Thick   int    `json:"thick"`
	Gap     int    `json:"gap"`
	Alpha   int    `json:"alpha"`
	Outline bool   `json:"outline"`
	Always  bool   `json:"always"`
}

func clampI(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (c xhCfg) norm() xhCfg {
	switch c.Style {
	case "t", "dot", "cross", "circle", "crossdot":
	default:
		c.Style = "cross"
	}
	c.Size = clampI(c.Size, 2, 60)
	c.Thick = clampI(c.Thick, 1, 10)
	c.Gap = clampI(c.Gap, 0, 30)
	if c.Alpha == 0 {
		c.Alpha = 100
	}
	c.Alpha = clampI(c.Alpha, 20, 100)
	return c
}

// xhConfig: ค่าเป้าเล็ง (ไม่ใช่พรีเมียม = ปิด)
func (a *app) xhConfig() xhCfg {
	c := &xhCfg{}
	if !licGet().Premium {
		return xhCfg{}
	}
	var s struct {
		XH *xhCfg `json:"xh"`
	}
	s.XH = c
	if json.Unmarshal([]byte(a.loadSettings()), &s) != nil || s.XH == nil {
		return xhCfg{}
	}
	return s.XH.norm()
}

func xhColor(s string) (r, g, b float64) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return 0, 255, 102
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 255, 102
	}
	return float64(v >> 16 & 255), float64(v >> 8 & 255), float64(v & 255)
}

// xhInside: จุด (x,y) ห่างจากกลาง อยู่ในเส้นของเป้าแบบนี้ไหม (t = ความหนา, ext = ขยายขอบสำหรับเส้นขอบดำ)
func xhInside(style string, x, y float64, size, t, gap, ext float64) bool {
	h := t/2 + ext
	ax, ay := math.Abs(x), math.Abs(y)
	arm := func() bool {
		if ay <= h && ax >= gap-ext && ax <= gap+size+ext {
			return true
		}
		return ax <= h && ay >= gap-ext && ay <= gap+size+ext
	}
	dot := func() bool { r := math.Max(1, t) + ext; return x*x+y*y <= r*r }
	switch style {
	case "t":
		if ay <= h && ax >= gap-ext && ax <= gap+size+ext {
			return true
		}
		return ax <= h && y >= gap-ext && y <= gap+size+ext
	case "dot":
		return dot()
	case "circle":
		return math.Abs(math.Hypot(x, y)-(gap+size/2)) <= h
	case "crossdot":
		return arm() || dot()
	}
	return arm()
}

// xhRender: ภาพเป้าแบบ BGRA premultiplied (สุ่มตัวอย่าง 4×4 ต่อพิกเซลให้ขอบเนียน)
func xhRender(c xhCfg) ([]byte, int) {
	c = c.norm()
	n := 2*(c.Size+c.Gap+c.Thick+4) + 1
	if c.Style == "dot" {
		n = 2*(c.Thick+4) + 1
	}
	r, g, b := xhColor(c.Color)
	al := float64(c.Alpha) / 100
	out := make([]byte, n*n*4)
	half := float64(n) / 2
	for py := 0; py < n; py++ {
		for px := 0; px < n; px++ {
			cov, ol := 0.0, 0.0
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					x := float64(px) + (float64(sx)+0.5)*0.25 - half
					y := float64(py) + (float64(sy)+0.5)*0.25 - half
					if xhInside(c.Style, x, y, float64(c.Size), float64(c.Thick), float64(c.Gap), 0) {
						cov++
					} else if c.Outline && xhInside(c.Style, x, y, float64(c.Size), float64(c.Thick), float64(c.Gap), 1) {
						ol++
					}
				}
			}
			cov, ol = cov/16, ol/16
			a := (cov + ol) * al
			o := (py*n + px) * 4
			out[o] = byte(math.Min(255, b*cov*al))
			out[o+1] = byte(math.Min(255, g*cov*al))
			out[o+2] = byte(math.Min(255, r*cov*al))
			out[o+3] = byte(math.Min(255, a*255))
		}
	}
	return out, n
}

type xhWin struct {
	hwnd, dc, bmp, old uintptr
	key                string
	n                  int
	shown              bool
	x, y               int32
}

func (w *xhWin) freeBitmap() {
	if w.dc != 0 {
		pSelectObject.Call(w.dc, w.old)
		pDeleteObject.Call(w.bmp)
		pDeleteDC.Call(w.dc)
		w.dc, w.bmp, w.old = 0, 0, 0
	}
}

func (w *xhWin) destroy() {
	w.freeBitmap()
	if w.hwnd != 0 {
		pDestroyWindow.Call(w.hwnd)
	}
	*w = xhWin{}
}

func (w *xhWin) build(c xhCfg) bool {
	if w.hwnd == 0 {
		cls, _ := windows.UTF16PtrFromString("Static")
		h, _, _ := pCreateWindowEx.Call(0x080800A8, uintptr(unsafe.Pointer(cls)), 0, 0x80000000, 0, 0, 8, 8, 0, 0, 0, 0)
		if h == 0 {
			return false
		}
		w.hwnd = h
	}
	w.freeBitmap()
	px, n := xhRender(c)
	bi := make([]byte, 40)
	*(*uint32)(unsafe.Pointer(&bi[0])) = 40
	*(*int32)(unsafe.Pointer(&bi[4])) = int32(n)
	*(*int32)(unsafe.Pointer(&bi[8])) = -int32(n)
	*(*uint32)(unsafe.Pointer(&bi[12])) = 0x200001 // planes 1, 32 บิต
	dc, _, _ := pCreateCompatibleDC.Call(0)
	var bits unsafe.Pointer
	bm, _, _ := pCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bi[0])), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bm == 0 || bits == nil {
		pDeleteDC.Call(dc)
		return false
	}
	copy(unsafe.Slice((*byte)(bits), len(px)), px)
	old, _, _ := pSelectObject.Call(dc, bm)
	w.dc, w.bmp, w.old, w.n = dc, bm, old, n
	return true
}

func (w *xhWin) show(c xhCfg, cx, cy int32) {
	key := fmt.Sprintf("%+v", c)
	if key != w.key || w.hwnd == 0 {
		if !w.build(c) {
			return
		}
		w.key = key
		w.shown = false
	}
	x, y := cx-int32(w.n/2), cy-int32(w.n/2)
	if !w.shown || x != w.x || y != w.y {
		pt := [2]int32{x, y}
		sz := [2]int32{int32(w.n), int32(w.n)}
		src := [2]int32{}
		blend := uint32(0x01FF0000) // AC_SRC_OVER, alpha 255, AC_SRC_ALPHA
		pUpdateLayeredWindow.Call(w.hwnd, 0, uintptr(unsafe.Pointer(&pt)), uintptr(unsafe.Pointer(&sz)), w.dc,
			uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), 2)
		pSetWindowPos.Call(w.hwnd, ^uintptr(0), uintptr(x), uintptr(y), 0, 0, 0x0001|0x0010|0x0040) // TOPMOST, NOSIZE|NOACTIVATE|SHOWWINDOW
		w.x, w.y, w.shown = x, y, true
	}
}

func (w *xhWin) hide() {
	if w.hwnd != 0 && w.shown {
		pShowWindow.Call(w.hwnd, 0)
		w.shown = false
	}
}

// tick: เรียกทุกรอบ · เปิดอยู่ + เกมอยู่ด้านหน้า = แสดงกลางหน้าต่างเกม
func (w *xhWin) tick(c xhCfg, pid uint32, visible bool) {
	if !c.On || pid == 0 {
		if w.hwnd != 0 {
			w.destroy()
		}
		return
	}
	if !visible {
		w.hide()
		return
	}
	fg, _, _ := pGetForegroundWindow.Call()
	var fp uint32
	pGetWindowThreadPid.Call(fg, uintptr(unsafe.Pointer(&fp)))
	var cx, cy int32
	if fp == pid {
		var rc [4]int32
		pGetClientRect.Call(fg, uintptr(unsafe.Pointer(&rc)))
		pt := [2]int32{rc[2] / 2, rc[3] / 2}
		pClientToScreen.Call(fg, uintptr(unsafe.Pointer(&pt)))
		if rc[2] < 200 || rc[3] < 200 {
			w.hide()
			return
		}
		cx, cy = pt[0], pt[1]
	} else if c.Always {
		sw, _, _ := pGetSystemMetrics.Call(0)
		sh, _, _ := pGetSystemMetrics.Call(1)
		cx, cy = int32(sw/2), int32(sh/2)
	} else {
		w.hide()
		return
	}
	w.show(c, cx, cy)
}
