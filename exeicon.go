package main

// ดึงไอคอนจากไฟล์ .exe ในเครื่อง (เช่น FiveM.exe) แปลงเป็น PNG ให้หน้าโปรแกรมแสดง
// ไม่ได้ฝังโลโก้ของใครไว้ในโปรแกรม อ่านจากไฟล์ที่ผู้ใช้ลงไว้เองตอนเปิดโปรแกรม

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pPrivateExtractIcons = user32.NewProc("PrivateExtractIconsW")
	pGetIconInfo         = user32.NewProc("GetIconInfo")
	pDestroyIcon         = user32.NewProc("DestroyIcon")
	pGetDC               = user32.NewProc("GetDC")
	pReleaseDC           = user32.NewProc("ReleaseDC")
	pGetDIBits           = gdi32.NewProc("GetDIBits")
	pDeleteObject        = gdi32.NewProc("DeleteObject")
)

type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  uintptr
	hbmColor uintptr
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

func dibPixels(hdc, hbm uintptr, size int) []byte {
	bi := bitmapInfoHeader{Size: 40, Width: int32(size), Height: -int32(size), Planes: 1, BitCount: 32}
	buf := make([]byte, size*size*4)
	// BITMAPINFO = header + สีตาราง 1 ช่อง (ไม่ใช้ตอน 32bpp) เผื่อพื้นที่ไว้
	var bmi [44]byte
	*(*bitmapInfoHeader)(unsafe.Pointer(&bmi[0])) = bi
	if r, _, _ := pGetDIBits.Call(hdc, hbm, 0, uintptr(size), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bmi[0])), 0); r == 0 {
		return nil
	}
	return buf
}

// exeIconPNG คืน data URL ของไอคอนขนาด size×size หรือ "" ถ้าดึงไม่ได้
func exeIconPNG(path string, size int) string {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	var hicon uintptr
	var id uint32
	if n, _, _ := pPrivateExtractIcons.Call(uintptr(unsafe.Pointer(p)), 0, uintptr(size), uintptr(size), uintptr(unsafe.Pointer(&hicon)), uintptr(unsafe.Pointer(&id)), 1, 0); n == 0 || n == 0xFFFFFFFF || hicon == 0 {
		return ""
	}
	defer pDestroyIcon.Call(hicon)
	var ii iconInfo
	if r, _, _ := pGetIconInfo.Call(hicon, uintptr(unsafe.Pointer(&ii))); r == 0 {
		return ""
	}
	defer pDeleteObject.Call(ii.hbmMask)
	if ii.hbmColor == 0 {
		return ""
	}
	defer pDeleteObject.Call(ii.hbmColor)
	hdc, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, hdc)
	px := dibPixels(hdc, ii.hbmColor, size)
	if px == nil {
		return ""
	}
	hasAlpha := false
	for i := 3; i < len(px); i += 4 {
		if px[i] != 0 {
			hasAlpha = true
			break
		}
	}
	var mask []byte
	if !hasAlpha {
		mask = dibPixels(hdc, ii.hbmMask, size)
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for i := 0; i < size*size; i++ {
		b, g, r, a := px[i*4], px[i*4+1], px[i*4+2], px[i*4+3]
		if !hasAlpha {
			a = 255
			if mask != nil && mask[i*4] != 0 { // ส่วนที่ mask ขาว = โปร่งใส
				a = 0
			}
		}
		img.SetNRGBA(i%size, i/size, color.NRGBA{r, g, b, a})
	}
	var out bytes.Buffer
	if png.Encode(&out, img) != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(out.Bytes())
}

// fivemIcon ไอคอนของ FiveM.exe ในโฟลเดอร์ที่เจอ
func (a *app) fivemIcon(fivemDir string) string {
	exe := filepath.Join(fivemDir, "FiveM.exe")
	if _, err := os.Stat(exe); err != nil {
		return ""
	}
	return exeIconPNG(exe, 128)
}
