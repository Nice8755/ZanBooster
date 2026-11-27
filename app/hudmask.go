//go:build windows

package main

// hudmask.go: ถอดจาก exe 4.11.15 · หน้ากาก HUD (ให้ ZanGrade ไม่ย้อมสีแผนที่/ตัวเลข/แถบเลือด) + พรีเซ็ตกลางวัน/กลางคืน
// จับภาพจอ 6 ภาพระหว่างผู้ใช้หมุนกล้อง จุดที่ไม่ขยับแต่มีขอบคม = HUD · ทำงานเฉพาะตอนผู้ใช้กดปุ่มเอง (ไม่ยุ่งกับตัวเกม)
// หมายเหตุ: สูตรตัดสินจุด HUD ทำตามค่าคงที่ของตัวเก่า แต่ลำดับขั้นละเอียดบางส่วนเขียนใหม่ (ต้องลองในเกมจริง)

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	pSelectObject           = gdi32.NewProc("SelectObject")
	pBitBlt                 = gdi32.NewProc("BitBlt")
	pDeleteDC               = gdi32.NewProc("DeleteDC")
)

func zanHudPath() string { return filepath.Join(zanDataDir, "zan_hud.png") }

// blankMaskPNG: หน้ากากว่าง (ดำทั้งภาพ = ไม่กันส่วนไหน)
func blankMaskPNG() []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewGray(image.Rect(0, 0, 256, 144)))
	return b.Bytes()
}

func zanHudWrite(plugins string) {
	m := blankMaskPNG()
	if zanDataDir != "" {
		if b, err := os.ReadFile(zanHudPath()); err == nil {
			m = b
		}
	}
	p := filepath.Join(plugins, "reshade-shaders", "Textures", "ZanHudMask.png")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	zanWriteOne(p, m)
}

func zanHudRemove(plugins string) {
	p := filepath.Join(plugins, "reshade-shaders", "Textures", "ZanHudMask.png")
	if fileExists(p) {
		zanWriteOne(p, nil)
		os.Remove(p)
	}
}

// captureScreen: ภาพทั้งจอย่อเหลือ w×h เป็นค่าความสว่าง 0-1
func captureScreen(w, h int) ([]float64, bool) {
	sw, _, _ := pGetSystemMetrics.Call(0)
	sh, _, _ := pGetSystemMetrics.Call(1)
	if sw == 0 || sh == 0 {
		return nil, false
	}
	sdc, _, _ := pGetDC.Call(0)
	if sdc == 0 {
		return nil, false
	}
	defer pReleaseDC.Call(0, sdc)
	mdc, _, _ := pCreateCompatibleDC.Call(sdc)
	defer pDeleteDC.Call(mdc)
	bm, _, _ := pCreateCompatibleBitmap.Call(sdc, sw, sh)
	defer pDeleteObject.Call(bm)
	old, _, _ := pSelectObject.Call(mdc, bm)
	pBitBlt.Call(mdc, 0, 0, sw, sh, sdc, 0, 0, 0x00CC0020)
	pSelectObject.Call(mdc, old)
	W, H := int(sw), int(sh)
	buf := make([]byte, W*H*4)
	bmi := make([]byte, 44)
	*(*uint32)(unsafe.Pointer(&bmi[0])) = 40
	*(*int32)(unsafe.Pointer(&bmi[4])) = int32(W)
	*(*int32)(unsafe.Pointer(&bmi[8])) = -int32(H)
	*(*uint16)(unsafe.Pointer(&bmi[12])) = 1
	*(*uint16)(unsafe.Pointer(&bmi[14])) = 32
	if r, _, _ := pGetDIBits.Call(mdc, bm, 0, uintptr(H), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bmi[0])), 0); r == 0 {
		return nil, false
	}
	out := make([]float64, w*h)
	for y := 0; y < h; y++ {
		sy := y * H / h
		for x := 0; x < w; x++ {
			o := (sy*W + x*W/w) * 4
			out[y*w+x] = (0.0722*float64(buf[o]) + 0.7152*float64(buf[o+1]) + 0.2126*float64(buf[o+2])) / 255
		}
	}
	return out, true
}

func (a *app) zanHudCapture(dir string) reshadeState {
	if !fivemRunningAny() {
		return reshadeState{Msg: "เข้าเกมก่อน แล้วกดปุ่มนี้"}
	}
	c, _ := windows.UTF16PtrFromString("grcWindow")
	g, _, _ := pFindWindow.Call(uintptr(unsafe.Pointer(c)), 0)
	if g == 0 {
		return reshadeState{Msg: "หาหน้าต่างเกมไม่เจอ"}
	}
	pShowWindow.Call(g, 9)
	pSetForegroundWindow.Call(g)
	time.Sleep(1500 * time.Millisecond)
	const W, H = 512, 288
	const N = W * H
	var frames [][]float64
	for i := 0; i < 6; i++ {
		if f, ok := captureScreen(W, H); ok {
			frames = append(frames, f)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if a.hwnd != 0 {
		pShowWindow.Call(a.hwnd, 9)
		pSetForegroundWindow.Call(a.hwnd)
	}
	if len(frames) < 4 {
		return reshadeState{Msg: "จับภาพเกมไม่ได้"}
	}
	n := float64(len(frames))
	mean, vr := make([]float64, N), make([]float64, N)
	tot := 0.0
	for i := 0; i < N; i++ {
		s, s2 := 0.0, 0.0
		for _, f := range frames {
			s += f[i]
			s2 += f[i] * f[i]
		}
		mean[i] = s / n
		vr[i] = math.Max(s2/n-mean[i]*mean[i], 0)
		tot += mean[i]
	}
	if tot/N < 0.02 {
		return reshadeState{Msg: "ภาพเกมดำ (เกมเต็มจอแบบ Fullscreen) ตั้งเกมเป็น Windowed Borderless ก่อน แล้วลองใหม่"}
	}
	moving := 0
	for i := 0; i < N; i++ {
		if vr[i] > 0.03 {
			moving++
		}
	}
	if float64(moving)/N < 0.35 {
		return reshadeState{Msg: "กล้องไม่ได้ขยับ กดใหม่แล้วหมุนกล้องช้า ๆ ระหว่างนับ 3 วินาที"}
	}
	// จุดนิ่ง (แทบไม่เปลี่ยน) ที่มีขอบชัด = ส่วนของ HUD
	cand := make([]float64, N)
	for y := 1; y < H-1; y++ {
		for x := 1; x < W-1; x++ {
			i := y*W + x
			if vr[i] >= 0.012 {
				continue
			}
			e := 0.0
			for _, j := range []int{i - 1, i + 1, i - W, i + W} {
				e = math.Max(e, math.Abs(mean[i]-mean[j]))
			}
			if e > 0.06 {
				cand[i] = 1
			}
		}
	}
	// ตัดจุดเดี่ยว แล้วขยายให้คลุมทั้งตัวอักษร/ไอคอน
	morph := func(src []float64, r int, f func(a, b float64) float64, init float64) []float64 {
		dst := make([]float64, N)
		for y := 0; y < H; y++ {
			for x := 0; x < W; x++ {
				v := init
				for dy := -r; dy <= r; dy++ {
					for dx := -r; dx <= r; dx++ {
						xx, yy := x+dx, y+dy
						if xx < 0 || yy < 0 || xx >= W || yy >= H {
							continue
						}
						v = f(v, src[yy*W+xx])
					}
				}
				dst[y*W+x] = v
			}
		}
		return dst
	}
	m := morph(morph(cand, 2, math.Max, 0), 1, math.Min, 1)
	img := image.NewGray(image.Rect(0, 0, 256, 144))
	sum := 0.0
	for y := 0; y < 144; y++ {
		for x := 0; x < 256; x++ {
			v := 0.25 * (m[(2*y)*W+2*x] + m[(2*y)*W+2*x+1] + m[(2*y+1)*W+2*x] + m[(2*y+1)*W+2*x+1])
			dx, dy := float64(x)/256-0.5, float64(y)/144-0.5
			if math.Abs(dx) < 0.12 && math.Abs(dy) < 0.15 { // กลางจอ (เป้า) ไม่นับเป็น HUD
				v = 0
			}
			sum += v
			img.Pix[y*img.Stride+x] = uint8(v * 255)
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	if zanDataDir == "" {
		zanDataDir = a.dataDir
	}
	_ = os.WriteFile(zanHudPath(), b.Bytes(), 0o644)
	_, plugins, _ := reshadePaths(dir)
	zanHudWrite(plugins)
	st := a.reshadeStatus(dir)
	st.Msg = "hud:" + strconv.Itoa(int(sum/36864*100))
	if u8IniGet(filepath.Join(plugins, "ReShade.ini"), "INPUT", "KeyReload") == "136,0,0,0" {
		rsSendKey(136)
	}
	return st
}

func (a *app) zanHudClear(dir string) reshadeState {
	os.Remove(zanHudPath())
	_, plugins, _ := reshadePaths(dir)
	if fivemRunningAny() {
		zanHudWrite(plugins)
		if u8IniGet(filepath.Join(plugins, "ReShade.ini"), "INPUT", "KeyReload") == "136,0,0,0" {
			rsSendKey(136)
		}
	}
	st := a.reshadeStatus(dir)
	st.Msg = "hud:cleared"
	return st
}

// ----------------------------------------------------------- กลางวัน/กลางคืน --

func zanDNGet() (day, night string) {
	if zanDataDir == "" {
		return "", ""
	}
	b, err := os.ReadFile(filepath.Join(zanDataDir, "zan_dn.txt"))
	if err != nil {
		return "", ""
	}
	day, night, _ = strings.Cut(strings.TrimSpace(string(b)), ",")
	return day, night
}

// zanDNBody: พรีเซ็ตที่สลับเองตามเวลาในเกม (ZanGrade ฝั่งกลางวัน + ZanGradeN ฝั่งกลางคืน)
func zanDNBody() string {
	d, n := zanDNGet()
	dv, ok1 := zanVals[d]
	nv, ok2 := zanVals[n]
	if !ok1 || !ok2 {
		return ""
	}
	dv.DNRole, nv.DNRole = 1, 2
	ns := zgFx(nv)
	ns.Tech, ns.File = "ZanGradeN", "ZanGradeN.fx"
	return buildPreset([]fxSec{zgFx(dv), ns})
}

func zanDNWrite(plugins string) {
	if b := zanDNBody(); b != "" {
		zanWriteOne(filepath.Join(plugins, "Zan_DayNight.ini"), []byte(zanWatermark()+zanTransform(b)))
	}
}

func (a *app) zanDNSet(dir, v string) reshadeState {
	if !licGet().Premium {
		return reshadeState{Msg: "ฟีเจอร์พรีเมียม"}
	}
	_, plugins, _ := reshadePaths(dir)
	if !zanInstalled(plugins) {
		return reshadeState{Msg: zanNeedInstall}
	}
	d, n, _ := strings.Cut(v, ",")
	if _, ok := zanVals[d]; !ok {
		return reshadeState{Msg: "เลือกพรีเซ็ตกลางวันก่อน"}
	}
	if _, ok := zanVals[n]; !ok {
		return reshadeState{Msg: "เลือกพรีเซ็ตกลางคืนก่อน"}
	}
	if zanDataDir == "" {
		zanDataDir = a.dataDir
	}
	_ = os.WriteFile(filepath.Join(zanDataDir, "zan_dn.txt"), []byte(d+","+n), 0o644)
	f := filepath.Join(plugins, "Zan_DayNight.ini")
	if !fivemRunningAny() {
		reshadeSetPreset(plugins, f)
		st := a.reshadeStatus(dir)
		st.Msg = "set"
		return st
	}
	zanGradeWrite(plugins)
	zanDNWrite(plugins)
	st := a.reshadeStatus(dir)
	if zanShortcutsBound(plugins) && rsSendKey(0x8d) {
		st.Zan, st.Msg = "dn", "ingame:auto"
	} else {
		st.Msg = "ingame:Zan_DayNight.ini"
	}
	return st
}
