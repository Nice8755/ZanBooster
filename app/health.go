//go:build windows

package main

// health.go: ถอดจาก exe 4.11.15 · ตรวจเครื่องส่วนพรีเมียม (รีเฟรชเรตจอ ความร้อน Resizable BAR
// สายจอเสียบผิดช่อง ทางเข้าไบออสตามยี่ห้อเมนบอร์ด ค่าเกมสำหรับตี/ยิง) + ตั้งรีเฟรชเรต / รีสตาร์ทเข้าไบออส

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	pEnumDisplayDevices  = user32.NewProc("EnumDisplayDevicesW")
	pEnumDisplaySettings = user32.NewProc("EnumDisplaySettingsW")
	pChangeDisplayEx     = user32.NewProc("ChangeDisplaySettingsExW")
	pGetFirmwareType     = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetFirmwareType")
)

type displayDevice struct {
	Cb           uint32
	DeviceName   [32]uint16
	DeviceString [128]uint16
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

type devMode struct {
	DeviceName       [32]uint16
	SpecVersion      uint16
	DriverVersion    uint16
	Size             uint16
	DriverExtra      uint16
	Fields           uint32
	PosX             int32
	PosY             int32
	Orientation      uint32
	FixedOutput      uint32
	Color            int16
	Duplex           int16
	YResolution      int16
	TTOption         int16
	Collate          int16
	FormName         [32]uint16
	LogPixels        uint16
	BitsPerPel       uint32
	PelsWidth        uint32
	PelsHeight       uint32
	DisplayFlags     uint32
	DisplayFrequency uint32
	ICMMethod        uint32
	ICMIntent        uint32
	MediaType        uint32
	DitherType       uint32
	Reserved1        uint32
	Reserved2        uint32
	PanningWidth     uint32
	PanningHeight    uint32
}

type healthItem struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix"`
	Act    string `json:"act,omitempty"`
	ActTxt string `json:"actTxt,omitempty"`
}

type hzInfo struct {
	Dev, Name          string
	Cur, Max, W, H, Bpp uint32
}

var hzPrev []hzInfo

// monitorsHz: ทุกจอที่เปิดอยู่ รีเฟรชเรตปัจจุบัน และสูงสุดที่ทำได้ที่ความละเอียดเดิม
func monitorsHz() []hzInfo {
	var out []hzInfo
	for i := uint32(0); i < 16; i++ {
		dd := &displayDevice{Cb: uint32(unsafe.Sizeof(displayDevice{}))}
		if r, _, _ := pEnumDisplayDevices.Call(0, uintptr(i), uintptr(unsafe.Pointer(dd)), 0); r == 0 {
			break
		}
		if dd.StateFlags&1 == 0 { // ไม่ได้ต่อจอ
			continue
		}
		dm := &devMode{Size: 220}
		if r, _, _ := pEnumDisplaySettings.Call(uintptr(unsafe.Pointer(&dd.DeviceName[0])), uintptr(0xFFFFFFFF), uintptr(unsafe.Pointer(dm))); r == 0 {
			continue
		}
		h := hzInfo{Dev: syscall.UTF16ToString(dd.DeviceName[:]), Name: syscall.UTF16ToString(dd.DeviceString[:]),
			Cur: dm.DisplayFrequency, Max: dm.DisplayFrequency, W: dm.PelsWidth, H: dm.PelsHeight, Bpp: dm.BitsPerPel}
		mon := &displayDevice{Cb: uint32(unsafe.Sizeof(displayDevice{}))}
		if r, _, _ := pEnumDisplayDevices.Call(uintptr(unsafe.Pointer(&dd.DeviceName[0])), 0, uintptr(unsafe.Pointer(mon)), 0); r != 0 {
			if n := syscall.UTF16ToString(mon.DeviceString[:]); n != "" && !strings.EqualFold(n, "Generic PnP Monitor") {
				h.Name = n
			}
		}
		for j := uint32(0); j < 4000; j++ {
			m := &devMode{Size: 220}
			if r, _, _ := pEnumDisplaySettings.Call(uintptr(unsafe.Pointer(&dd.DeviceName[0])), uintptr(j), uintptr(unsafe.Pointer(m))); r == 0 {
				break
			}
			if m.PelsWidth == h.W && m.PelsHeight == h.H && m.BitsPerPel == h.Bpp && m.DisplayFrequency > h.Max && m.DisplayFrequency <= 1000 {
				h.Max = m.DisplayFrequency
			}
		}
		out = append(out, h)
	}
	return out
}

// setHz: flags 2 = ลองก่อน (CDS_TEST), 1 = ตั้งจริงและจำ (CDS_UPDATEREGISTRY) · คืนรหัสผล 0 = สำเร็จ
func setHz(h hzInfo, hz uint32, flags uintptr) int {
	dm := &devMode{Size: 220, PelsWidth: h.W, PelsHeight: h.H, DisplayFrequency: hz, Fields: 0x80000 | 0x100000 | 0x400000}
	p, _ := windows.UTF16PtrFromString(h.Dev)
	r, _, _ := pChangeDisplayEx.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(dm)), 0, flags, 0)
	return int(int32(r))
}

func (a *app) fixHz() string {
	hzPrev = nil
	var msg []string
	for _, m := range monitorsHz() {
		if m.Max <= m.Cur {
			continue
		}
		if c := setHz(m, m.Max, 2); c != 0 {
			msg = append(msg, fmt.Sprintf("%s ไดรเวอร์ไม่รับ %d Hz", m.Name, m.Max))
			continue
		}
		if c := setHz(m, m.Max, 1); c == 0 {
			hzPrev = append(hzPrev, m)
			msg = append(msg, fmt.Sprintf("%s %d → %d Hz", m.Name, m.Cur, m.Max))
		} else {
			msg = append(msg, fmt.Sprintf("%s ตั้งไม่สำเร็จ (รหัส %d)", m.Name, c))
		}
	}
	if len(msg) == 0 {
		return "ทุกจอใช้รีเฟรชเรตสูงสุดอยู่แล้ว"
	}
	return strings.Join(msg, " · ")
}

// revertHz: ภาพผิดปกติ / ไม่ได้กดยืนยันใน 15 วิ → กลับค่าเดิม
func (a *app) revertHz() {
	for _, p := range hzPrev {
		setHz(p, p.Cur, 1)
	}
	hzPrev = nil
}

func (a *app) rebootBios() string {
	c := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "shutdown.exe"), "/r", "/fw", "/t", "0")
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := c.CombinedOutput()
	if err != nil {
		return "เข้าไบออสจาก Windows ไม่ได้ (เครื่องอาจเป็นไบออสแบบเก่า) ให้รีสตาร์ทแล้วกด Del หรือ F2 รัว ๆ ตอนขึ้นโลโก้: " + strings.TrimSpace(string(out))
	}
	return ""
}

func boardInfo() (vendor, product string) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\BIOS`, registry.QUERY_VALUE)
	if err != nil {
		return "", ""
	}
	defer k.Close()
	v, _, _ := k.GetStringValue("BaseBoardManufacturer")
	p, _, _ := k.GetStringValue("BaseBoardProduct")
	return strings.TrimSpace(v), strings.TrimSpace(p)
}

var bar1Re = regexp.MustCompile(`(?s)BAR1 Memory Usage\s*\n\s*Total\s*:\s*(\d+)\s*MiB`)

// rebarNvidia: 1 = เปิด (BAR1 ใหญ่กว่า 256 MiB), 0 = ปิด, -1 = ตรวจไม่ได้ (ไม่ใช่ NVIDIA)
func rebarNvidia() int {
	smi := filepath.Join(os.Getenv("SystemRoot"), "System32", "nvidia-smi.exe")
	if _, err := os.Stat(smi); err != nil {
		return -1
	}
	c := exec.Command(smi, "-q", "-d", "MEMORY")
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := c.Output()
	if err != nil {
		return -1
	}
	m := bar1Re.FindSubmatch(out)
	if len(m) < 2 {
		return -1
	}
	if n, _ := strconv.Atoi(string(m[1])); n > 256 {
		return 1
	}
	return 0
}

// primaryAdapter: การ์ดจอที่จอหลักต่ออยู่จริง
func primaryAdapter() string {
	for i := uint32(0); i < 16; i++ {
		dd := &displayDevice{Cb: uint32(unsafe.Sizeof(displayDevice{}))}
		if r, _, _ := pEnumDisplayDevices.Call(0, uintptr(i), uintptr(unsafe.Pointer(dd)), 0); r == 0 {
			break
		}
		if dd.StateFlags&1 != 0 && dd.StateFlags&4 != 0 {
			return syscall.UTF16ToString(dd.DeviceString[:])
		}
	}
	return ""
}

// gpuNames: ชื่อการ์ดจอทุกตัวที่ติดตั้ง (รีจิสทรีคลาสการ์ดจอ)
func gpuNames() []string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil
	}
	defer k.Close()
	subs, _ := k.ReadSubKeyNames(-1)
	var out []string
	for _, s := range subs {
		sk, err := registry.OpenKey(k, s, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		if d, _, err := sk.GetStringValue("DriverDesc"); err == nil && d != "" {
			out = append(out, d)
		}
		sk.Close()
	}
	return out
}

var biosGuides = []struct{ match, xmp, rebar string }{
	{"asus", "Ai Tweaker > Ai Overclock Tuner > เลือก XMP I (Intel) หรือ EXPO / D.O.C.P (AMD)", "Advanced > PCI Subsystem Settings > เปิด Above 4G Decoding และ Re-Size BAR Support"},
	{"micro-star", "หน้าแรก (EZ Mode) กดสวิตช์ A-XMP หรือ EXPO มุมซ้ายบน หรือ OC > A-XMP", "Settings > Advanced > PCI Subsystem Settings > เปิด Above 4G memory และ Re-Size BAR Support"},
	{"msi", "หน้าแรก (EZ Mode) กดสวิตช์ A-XMP หรือ EXPO มุมซ้ายบน หรือ OC > A-XMP", "Settings > Advanced > PCI Subsystem Settings > เปิด Above 4G memory และ Re-Size BAR Support"},
	{"gigabyte", "Tweaker > Extreme Memory Profile (X.M.P.) หรือ EXPO > เลือก Profile1", "Settings > IO Ports > เปิด Above 4G Decoding และ Re-Size BAR Support"},
	{"asrock", "OC Tweaker > DRAM Configuration > Load XMP Setting หรือ EXPO > เลือก Profile 1", "Advanced > Chipset Configuration > เปิด Above 4G Decoding และ C.A.M. Clear / Re-Size BAR"},
}

func (a *app) healthExtra() string {
	var items []healthItem
	for _, m := range monitorsHz() {
		it := healthItem{ID: "hz", Title: "รีเฟรชเรตจอ · " + m.Name}
		if m.Max > m.Cur+4 {
			it.Status, it.Detail = "warn", fmt.Sprintf("ตั้งไว้ %d Hz แต่จอรองรับได้ถึง %d Hz", m.Cur, m.Max)
			it.Fix = "กดปุ่ม โปรแกรมตั้งเป็นค่าสูงสุดให้ ภาพจะลื่นขึ้นทันที ถ้าจอดับหรือภาพผิดปกติ ไม่ต้องกดอะไร 15 วินาทีจะกลับค่าเดิมเอง"
			it.ActTxt, it.Act = fmt.Sprintf("ตั้งเป็น %d Hz", m.Max), "hz"
		} else {
			it.Status, it.Detail = "ok", fmt.Sprintf("%d Hz (สูงสุดของจอแล้ว)", m.Cur)
		}
		items = append(items, it)
	}
	// ความร้อน
	cpu, gpu := cpuTempC(), gpuTempC()
	if cpu > 0 || gpu > 0 {
		var p []string
		if cpu > 0 {
			p = append(p, fmt.Sprintf("CPU %.0f°C", cpu))
		}
		if gpu > 0 {
			p = append(p, fmt.Sprintf("การ์ดจอ %.0f°C", gpu))
		}
		it := healthItem{ID: "temp", Title: "ความร้อน", Detail: strings.Join(p, " · ")}
		switch {
		case cpu >= 90 || gpu >= 85:
			it.Status, it.Fix = "warn", "ร้อนจนเครื่องลดความเร็วตัวเอง FPS จะตกกลางเกม ทำความสะอาดฝุ่น เช็กพัดลม เปลี่ยนซิลิโคน CPU หรือวางโน้ตบุ๊กบนพัดลมรอง"
		case cpu >= 80 || gpu >= 78:
			it.Status, it.Fix = "info", "ค่อนข้างร้อน ถ้าวัดตอนไม่ได้เล่นเกมแปลว่าระบายความร้อนไม่ดี ลองเช็กตอนเล่นเกมอีกครั้ง"
		default:
			it.Status, it.Detail = "ok", it.Detail+" (เช็กตอนเล่นเกมจะเห็นค่าจริง)"
		}
		items = append(items, it)
	}
	// Resizable BAR
	switch rebarNvidia() {
	case 1:
		items = append(items, healthItem{ID: "rebar", Title: "Resizable BAR", Status: "ok", Detail: "เปิดอยู่"})
	case 0:
		items = append(items, healthItem{ID: "rebar", Title: "Resizable BAR", Status: "info", Detail: "ปิดอยู่ (ได้เพิ่มไม่กี่ % และเฉพาะบางเกม)",
			Fix: "เปิดในไบออส ต้องเป็นการ์ดจอ RTX 30 ขึ้นไป และ Windows ติดตั้งแบบ UEFI ดูทางไปในหัวข้อไบออสด้านล่าง"})
	default:
		items = append(items, healthItem{ID: "rebar", Title: "Resizable BAR / SAM", Status: "unknown", Detail: "ตรวจไม่ได้ (ตรวจได้เฉพาะการ์ด NVIDIA)"})
	}
	// จอหลักเสียบที่การ์ดจอออนบอร์ดทั้งที่มีการ์ดจอแยก
	if pa := primaryAdapter(); pa != "" {
		dg := regexp.MustCompile(`(?i)nvidia|radeon rx|radeon\(tm\) rx|intel\(r\) arc`)
		ig := regexp.MustCompile(`(?i)amd radeon\(tm\) graphics|^amd radeon graphics|vega|intel\(r\) (uhd|iris|hd) graphics|intel\(r\) graphics`)
		has := false
		for _, n := range gpuNames() {
			if dg.MatchString(n) {
				has = true
				break
			}
		}
		if has && ig.MatchString(pa) {
			items = append(items, healthItem{ID: "igpuout", Title: "เสียบสายจอที่เมนบอร์ด", Status: "heavy",
				Detail: "จอหลักต่อกับ " + pa + " (การ์ดจอออนบอร์ด) ไม่ได้ต่อการ์ดจอแยก",
				Fix:    "ย้ายสายจอไปเสียบช่องของการ์ดจอแยก (ช่องแนวนอนด้านล่างหลังเคส) ไม่ใช่ช่องข้าง USB ด้านบน FPS จะกลับมาเยอะมาก"})
		} else if has {
			items = append(items, healthItem{ID: "igpuout", Title: "สายจอ", Status: "ok", Detail: "จอหลักต่อกับ " + pa})
		}
	}
	// ไบออส: ทางไปเปิด XMP / Resizable BAR ตามยี่ห้อเมนบอร์ด
	if v, p := boardInfo(); v != "" {
		lv := strings.ToLower(v)
		xmp := "หาเมนู XMP, D.O.C.P หรือ EXPO (มักอยู่ในหน้า OC / Tweaker / Memory) แล้วเลือก Profile 1"
		rb := "หาเมนู Above 4G Decoding และ Re-Size BAR (มักอยู่ใน Advanced > PCI) แล้วเปิดทั้งคู่"
		for _, g := range biosGuides {
			if strings.Contains(lv, g.match) {
				xmp, rb = g.xmp, g.rebar
				break
			}
		}
		items = append(items, healthItem{ID: "bios", Title: "ไบออส · " + strings.TrimSpace(v+" "+p), Status: "info",
			Detail: "เปิด XMP: " + xmp + " · เปิด Resizable BAR: " + rb,
			Fix:    "เสร็จแล้วกด Save & Exit (มักเป็นปุ่ม F10) ถ้าเปิด XMP แล้วเครื่องเปิดไม่ติด เครื่องจะกลับค่าเดิมเองหลังรีสตาร์ท 2-3 ครั้ง",
			Act:    "bios", ActTxt: "รีสตาร์ทเข้าไบออส"})
	}
	if it, ok := a.combatItem(); ok {
		items = append(items, it)
	}
	b, _ := json.Marshal(items)
	return string(b)
}

func (a *app) biosInfo() string {
	v, p := boardInfo()
	uefi := false
	if pGetFirmwareType.Find() == nil {
		var t uint32
		if r, _, _ := pGetFirmwareType.Call(uintptr(unsafe.Pointer(&t))); r != 0 {
			uefi = t == 2
		}
	}
	b, _ := json.Marshal(map[string]any{"vendor": v, "product": p, "uefi": uefi})
	return string(b)
}
