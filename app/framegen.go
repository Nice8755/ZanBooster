//go:build windows

package main

// framegen.go: ถอดจาก exe 4.11.15 · เฟรมเจนของไดรเวอร์ NVIDIA (Smooth Motion, RTX 40/50)
// ตั้งในโปรไฟล์ของไดรเวอร์เฉพาะตัวเกม FiveM ("Zan Booster - FiveM") ไม่แตะไฟล์เกม ปิดแล้วคืนค่าเดิมของไดรเวอร์
// AMD ไม่มีทางตั้งจากภายนอก จึงบอกวิธีเปิด AFMF ใน AMD Software แทน

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

)

const (
	nvDRSCreateProfile        = 0xCC176068
	nvDRSFindProfileByName    = 0x7E4A9A0B
	nvDRSFindApplicationByNam = 0xEEE566B2
	nvDRSCreateApplication    = 0x4347A9DE
	nvDRSGetSettingX          = 0xEA99498D // GetSetting รุ่นใหม่ (ค่า ID เกิน 0xB0000000 ต้องใช้ตัวนี้)
	nvDRSRestoreDefaultX      = 0xD20D29DF
	nvDRSSetSettingX          = 0x8A2CF5F5

	nvSmoothMotionOn   = 0xB0D384C0 // 1 = เปิด
	nvSmoothMotionAPIs = 0xB0CC0875 // 7 = DX11 + DX12 + Vulkan

	nvProfileName = "Zan Booster - FiveM"
	nvExeNotFound = -166
)

// smoothMotionGpu: ชื่อการ์ด NVIDIA + ใช้ Smooth Motion ได้ไหม (RTX 40xx / 50xx)
func smoothMotionGpu() (string, bool) {
	name := ""
	for _, n := range gpuNames() {
		u := strings.ToUpper(n)
		if !strings.Contains(u, "NVIDIA") && !strings.Contains(u, "GEFORCE") {
			continue
		}
		name = n
		i := strings.Index(u, "RTX ")
		if i < 0 || len(u) <= i+5 {
			continue
		}
		if (u[i+4] == '4' || u[i+4] == '5') && u[i+5] == '0' {
			return n, true
		}
	}
	return name, false
}

// fivemGameExes: ชื่อไฟล์ตัวเกมที่ FiveM ใช้ (ตัวที่กำลังเปิด + ทุกตัวในโฟลเดอร์ subprocess)
func fivemGameExes(dir string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		l := strings.ToLower(n)
		if strings.HasSuffix(l, ".exe") && (strings.Contains(l, "gtaprocess") || strings.Contains(l, "gameprocess")) && !seen[l] {
			seen[l] = true
			out = append(out, n)
		}
	}
	if _, n := gtaProcess(); n != "" {
		add(n)
	}
	if dir != "" {
		if es, err := os.ReadDir(filepath.Join(dir, "FiveM.app", "data", "cache", "subprocess")); err == nil {
			for _, e := range es {
				add(e.Name())
			}
		}
	}
	return out
}

// nvWide: ข้อความ UTF-16 ขนาดตายตัว 2048 ตัว (แบบที่ NVAPI ใช้)
func nvWide(s string) []uint16 {
	b := make([]uint16, 2048)
	u, _ := syscall.UTF16FromString(s)
	if len(u) > 2048 {
		u = u[:2047]
	}
	copy(b, u)
	return b
}

func putWide(dst []byte, s string) {
	for i, c := range nvWide(s) {
		if 2*i+2 > len(dst) {
			break
		}
		binary.LittleEndian.PutUint16(dst[2*i:], c)
	}
}

// loadNVApp: NVAPI + ฟังก์ชันจัดการโปรไฟล์ของแต่ละเกม
func loadNVApp() (*nvAPI, error) {
	n, err := loadNV()
	if err != nil {
		return nil, err
	}
	for _, id := range []uint32{nvDRSCreateProfile, nvDRSFindProfileByName, nvDRSFindApplicationByNam, nvDRSCreateApplication} {
		p, _, _ := nvQueryInt.Call(uintptr(id))
		if p == 0 {
			return nil, fmt.Errorf("ไดรเวอร์ NVIDIA รุ่นนี้เก่าเกินไป อัปเดตไดรเวอร์ก่อน")
		}
		n.fns[id] = p
	}
	for _, id := range []uint32{nvDRSGetSettingX, nvDRSRestoreDefaultX, nvDRSSetSettingX} {
		if p, _, _ := nvQueryInt.Call(uintptr(id)); p != 0 {
			n.fns[id] = p
		}
	}
	return n, nil
}

// setX/getX/delX: ใช้ฟังก์ชันรุ่นใหม่ถ้าไดรเวอร์มี ไม่งั้นใช้รุ่นเดิม
func (s *nvSession) setX(id, v uint32) error {
	if s.n.fns[nvDRSSetSettingX] == 0 {
		return s.set(id, v)
	}
	buf := make([]byte, nvSettingSize)
	binary.LittleEndian.PutUint32(buf[0:], nvSettingVer)
	binary.LittleEndian.PutUint32(buf[nvOffID:], id)
	binary.LittleEndian.PutUint32(buf[nvOffType:], 0)
	binary.LittleEndian.PutUint32(buf[nvOffCurrent:], v)
	r := s.n.call(nvDRSSetSettingX, s.h, s.prof, uintptr(unsafe.Pointer(&buf[0])), 0, 0)
	runtime.KeepAlive(buf)
	if r != 0 {
		return fmt.Errorf("SetSetting %d", r)
	}
	return nil
}

func (s *nvSession) getX(id uint32) (uint32, bool) {
	if s.n.fns[nvDRSGetSettingX] == 0 {
		v, _, err := s.get(id)
		return v, err == nil
	}
	buf := make([]byte, nvSettingSize)
	binary.LittleEndian.PutUint32(buf[0:], nvSettingVer)
	var extra uint32
	r := s.n.call(nvDRSGetSettingX, s.h, s.prof, uintptr(id), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&extra)))
	runtime.KeepAlive(buf)
	if r != 0 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(buf[nvOffCurrent:]), true
}

func (s *nvSession) delX(id uint32) {
	if s.n.fns[nvDRSRestoreDefaultX] == 0 {
		s.restoreDefault(id)
		return
	}
	s.n.call(nvDRSRestoreDefaultX, s.h, s.prof, uintptr(id))
}

// appProfile: โปรไฟล์ของไดรเวอร์ที่ผูกกับ exe นี้ · create=true สร้างโปรไฟล์ "Zan Booster - FiveM" แล้วผูกให้ถ้ายังไม่มี
func (s *nvSession) appProfile(exe string, create bool) (uintptr, error) {
	const appSize = 0x500C // NVDRS_APPLICATION_V4
	app := make([]byte, appSize)
	binary.LittleEndian.PutUint32(app[0:], appSize|4<<16)
	name := nvWide(exe)
	var prof uintptr
	r := s.n.call(nvDRSFindApplicationByNam, s.h, uintptr(unsafe.Pointer(&name[0])), uintptr(unsafe.Pointer(&prof)), uintptr(unsafe.Pointer(&app[0])))
	runtime.KeepAlive(app)
	if r == 0 && prof != 0 {
		return prof, nil
	}
	if !create {
		return 0, nil
	}
	if r != nvExeNotFound && r != 0 {
		return 0, fmt.Errorf("FindApplicationByName %d", r)
	}
	pn := nvWide(nvProfileName)
	prof = 0
	if r := s.n.call(nvDRSFindProfileByName, s.h, uintptr(unsafe.Pointer(&pn[0])), uintptr(unsafe.Pointer(&prof))); r != 0 || prof == 0 {
		const profSize = 0x1014 // NVDRS_PROFILE_V1
		p := make([]byte, profSize)
		binary.LittleEndian.PutUint32(p[0:], profSize|1<<16)
		putWide(p[4:], nvProfileName)
		prof = 0
		r := s.n.call(nvDRSCreateProfile, s.h, uintptr(unsafe.Pointer(&p[0])), uintptr(unsafe.Pointer(&prof)))
		runtime.KeepAlive(p)
		if r != 0 || prof == 0 {
			return 0, fmt.Errorf("CreateProfile %d", r)
		}
	}
	a := make([]byte, appSize)
	binary.LittleEndian.PutUint32(a[0:], appSize|4<<16)
	putWide(a[8:], exe)
	putWide(a[0x1008:], "FiveM")
	r = s.n.call(nvDRSCreateApplication, s.h, prof, uintptr(unsafe.Pointer(&a[0])))
	runtime.KeepAlive(a)
	if r != 0 {
		return 0, fmt.Errorf("CreateApplication %d", r)
	}
	return prof, nil
}

type fgInfo struct {
	Vendor    string   `json:"vendor"`
	Gpu       string   `json:"gpu"`
	Supported bool     `json:"supported"`
	On        bool     `json:"on"`
	Exes      []string `json:"exes"`
	Message   string   `json:"m,omitempty"`
}

func (a *app) frameGenInfo(dir string) fgInfo {
	r := fgInfo{Vendor: gpuVendor(), Exes: fivemGameExes(dir)}
	switch r.Vendor {
	case "nvidia":
		r.Gpu, r.Supported = smoothMotionGpu()
		if !r.Supported {
			r.Message = "Smooth Motion ใช้ได้กับ GeForce RTX 40 และ RTX 50 เท่านั้น"
			return r
		}
		n, err := loadNVApp()
		if err != nil {
			r.Message = err.Error()
			return r
		}
		s, err := n.open()
		if err != nil {
			r.Message = err.Error()
			return r
		}
		defer s.close()
		for _, e := range r.Exes {
			p, err := s.appProfile(e, false)
			if err != nil || p == 0 {
				continue
			}
			s.prof = p
			if v, ok := s.getX(nvSmoothMotionOn); ok && v == 1 {
				r.On = true
				break
			}
		}
	case "amd":
		for _, n := range gpuNames() {
			if strings.Contains(strings.ToLower(n), "radeon") {
				r.Gpu = n
				break
			}
		}
		r.Message = "การ์ดจอ AMD เปิดเฟรมเจน (AFMF) ได้ใน AMD Software: กด Alt+R > Gaming > Graphics > เปิด AMD Fluid Motion Frames"
	default:
		r.Message = "ไม่พบการ์ดจอ NVIDIA หรือ AMD"
	}
	return r
}

func (a *app) frameGenSet(dir string, on bool) gpuResult {
	if gpuVendor() != "nvidia" {
		return gpuResult{Message: "ปุ่มนี้ใช้กับการ์ดจอ NVIDIA RTX 40 / 50 เท่านั้น"}
	}
	if _, ok := smoothMotionGpu(); !ok {
		return gpuResult{Message: "Smooth Motion ใช้ได้กับ GeForce RTX 40 และ RTX 50 เท่านั้น"}
	}
	exes := fivemGameExes(dir)
	if len(exes) == 0 {
		return gpuResult{Message: "ยังไม่เจอไฟล์ตัวเกมของ FiveM เข้าเซิร์ฟสัก 1 ครั้งก่อนแล้วลองใหม่"}
	}
	n, err := loadNVApp()
	if err != nil {
		return gpuResult{Message: err.Error()}
	}
	s, err := n.open()
	if err != nil {
		return gpuResult{Message: err.Error()}
	}
	defer s.close()
	var r gpuResult
	for _, e := range exes {
		p, err := s.appProfile(e, on)
		if err != nil {
			r.Failed = append(r.Failed, e+": "+err.Error())
			continue
		}
		if p == 0 {
			continue
		}
		s.prof = p
		if on {
			e1 := s.setX(nvSmoothMotionOn, 1)
			e2 := s.setX(nvSmoothMotionAPIs, 7)
			if e1 != nil || e2 != nil {
				r.Failed = append(r.Failed, fmt.Sprintf("%s: enable=%v apis=%v", e, e1, e2))
				continue
			}
		} else {
			_ = s.setX(nvSmoothMotionOn, 0)
			s.delX(nvSmoothMotionAPIs)
		}
		r.Done = append(r.Done, e)
	}
	if err := s.save(); err != nil {
		return gpuResult{Message: "บันทึกค่าไดรเวอร์ไม่สำเร็จ: " + err.Error(), Failed: r.Failed}
	}
	r.OK = len(r.Failed) == 0
	if len(r.Failed) > 0 && len(r.Done) == 0 {
		r.Message = "ไดรเวอร์ NVIDIA ไม่รับค่า Smooth Motion ลองอัปเดตไดรเวอร์เป็นรุ่นล่าสุดแล้วกดใหม่ (" + strings.Join(r.Failed, " | ") + ")"
	}
	return r
}

func (a *app) frameGenInfoAsync(dir string, reqID int) bool {
	go func() {
		runtime.LockOSThread()
		a.emitGpu(reqID, a.frameGenInfo(dir))
	}()
	return true
}

func (a *app) frameGenSetAsync(dir string, on bool, reqID int) bool {
	if on && !licGet().Premium {
		a.emitGpu(reqID, gpuResult{Message: "ฟีเจอร์นี้เป็นของพรีเมียม"})
		return true
	}
	go func() {
		runtime.LockOSThread()
		a.emitGpu(reqID, a.frameGenSet(dir, on))
	}()
	return true
}
