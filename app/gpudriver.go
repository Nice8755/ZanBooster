package main

// ตั้งค่าไดรเวอร์การ์ดจอแบบรวม (ทุกเกม) ให้ได้ FPS มากที่สุด พร้อมเก็บค่าเดิมไว้คืนได้
//   NVIDIA: เขียนโปรไฟล์ Global ผ่าน NVAPI DRS (ที่เดียวกับ NVIDIA Control Panel > Manage 3D settings > Global)
//           ID ของฟังก์ชันและค่าจาก nvapi_interface.h / NvApiDriverSettings.h ของ NVIDIA
//   AMD:    เขียนค่าของ Adrenalin ใน registry (UMD ของการ์ดจอ) ค่าเดียวกับที่หน้า Graphics > Advanced ใช้

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// ---------------------------------------------------------------- NVIDIA --

var (
	nvapiDLL   = windows.NewLazySystemDLL("nvapi64.dll")
	nvQueryInt = nvapiDLL.NewProc("nvapi_QueryInterface")
)

const (
	nvInitialize            = 0x0150e828
	nvDRSCreateSession      = 0x0694d52e
	nvDRSDestroySession     = 0xdad9cff8
	nvDRSLoadSettings       = 0x375dbd6b
	nvDRSSaveSettings       = 0xfcbc7e14
	nvDRSGetCurrentGlobal   = 0x617bff9f
	nvDRSSetSetting         = 0x577dd202
	nvDRSGetSetting         = 0x73bf8338
	nvDRSRestoreDefault     = 0x53f0381e
	nvSettingSize           = 12320 // sizeof(NVDRS_SETTING_V1) (#pragma pack 4)
	nvSettingVer            = nvSettingSize | 1<<16
	nvOffID, nvOffType      = 4100, 4104
	nvOffLocation           = 4108
	nvOffCurrent            = 8220
	nvStatusSettingNotFound = -160
)

type gpuSetting struct {
	ID    uint32 `json:"id"`
	Name  string `json:"name"`
	Value uint32 `json:"value"`
	Label string `json:"label"`
}

// ค่าที่ตั้ง (Global ใช้กับทุกเกม)
var nvSettings = []gpuSetting{
	{0x1057EB71, "Power management mode", 1, "Prefer maximum performance"},
	{0x00CE2691, "Texture filtering - Quality", 0x14, "High performance"},
	{0x00E73211, "Texture filtering - Anisotropic sample optimization", 1, "On"},
	{0x00A879CF, "Vertical sync", 0x08416747, "Off"},
	{0x00198FFF, "Shader Cache", 1, "On"},
	{0x00AC8497, "Shader Cache size", 0xFFFFFFFF, "Unlimited"},
	{0x20C1221E, "Threaded optimization", 1, "On"},
	{0x0064B541, "Preferred refresh rate", 1, "Highest available"},
	{0x1074C972, "FXAA", 0, "Off"},
	{0x1075D972, "Ansel", 0, "Off"},
	{0x00667329, "Ambient Occlusion (บังคับจากไดรเวอร์)", 0, "Off"},
	{0x0098C1AC, "MFAA", 0, "Off"},
	{0x10D48A85, "Transparency anti-aliasing", 0, "Off"},
}

// ลด input lag: เฟรมรอคิวน้อยลง (Low Latency Mode = On) ใช้ได้กับเกม DX11 อย่าง FiveM
var nvLowLat = []gpuSetting{
	{0x007BA09E, "Max pre-rendered frames", 1, "1"},
	{0x0005F543, "Low Latency Mode", 1, "On"},
}

const nvFrameLimitID = 0x10835002 // Max Frame Rate (0 = ปิด)

type nvAPI struct{ fns map[uint32]uintptr }

func loadNV() (*nvAPI, error) {
	if err := nvapiDLL.Load(); err != nil {
		return nil, fmt.Errorf("ไม่มีไดรเวอร์ NVIDIA")
	}
	n := &nvAPI{fns: map[uint32]uintptr{}}
	for _, id := range []uint32{nvInitialize, nvDRSCreateSession, nvDRSDestroySession, nvDRSLoadSettings, nvDRSSaveSettings, nvDRSGetCurrentGlobal, nvDRSSetSetting, nvDRSGetSetting, nvDRSRestoreDefault} {
		p, _, _ := nvQueryInt.Call(uintptr(id))
		if p == 0 {
			return nil, fmt.Errorf("NVAPI ไม่มีฟังก์ชัน 0x%08X", id)
		}
		n.fns[id] = p
	}
	if r := n.call(nvInitialize); r != 0 {
		return nil, fmt.Errorf("NvAPI_Initialize %d", r)
	}
	return n, nil
}

func (n *nvAPI) call(id uint32, args ...uintptr) int32 {
	r, _, _ := syscall.SyscallN(n.fns[id], args...)
	return int32(r)
}

type nvSession struct {
	n       *nvAPI
	h, prof uintptr
}

func (n *nvAPI) open() (*nvSession, error) {
	s := &nvSession{n: n}
	if r := n.call(nvDRSCreateSession, uintptr(unsafe.Pointer(&s.h))); r != 0 {
		return nil, fmt.Errorf("DRS_CreateSession %d", r)
	}
	if r := n.call(nvDRSLoadSettings, s.h); r != 0 {
		n.call(nvDRSDestroySession, s.h)
		return nil, fmt.Errorf("DRS_LoadSettings %d", r)
	}
	if r := n.call(nvDRSGetCurrentGlobal, s.h, uintptr(unsafe.Pointer(&s.prof))); r != 0 {
		n.call(nvDRSDestroySession, s.h)
		return nil, fmt.Errorf("DRS_GetCurrentGlobalProfile %d", r)
	}
	return s, nil
}

func (s *nvSession) close() { s.n.call(nvDRSDestroySession, s.h) }

// get คืนค่า DWORD ปัจจุบัน และ set=false ถ้าโปรไฟล์ไม่ได้ตั้งค่านี้ไว้ (ใช้ค่าเริ่มต้นของไดรเวอร์)
func (s *nvSession) get(id uint32) (val uint32, set bool, err error) {
	buf := make([]byte, nvSettingSize)
	binary.LittleEndian.PutUint32(buf[0:], nvSettingVer)
	r := s.n.call(nvDRSGetSetting, s.h, s.prof, uintptr(id), uintptr(unsafe.Pointer(&buf[0])))
	runtime.KeepAlive(buf)
	if r == nvStatusSettingNotFound {
		return 0, false, nil
	}
	if r != 0 {
		return 0, false, fmt.Errorf("GetSetting %d", r)
	}
	// settingLocation: 0 = ตั้งในโปรไฟล์นี้เอง, อื่น ๆ = สืบทอดมา
	return binary.LittleEndian.Uint32(buf[nvOffCurrent:]), binary.LittleEndian.Uint32(buf[nvOffLocation:]) == 0, nil
}

func (s *nvSession) set(id, v uint32) error {
	buf := make([]byte, nvSettingSize)
	binary.LittleEndian.PutUint32(buf[0:], nvSettingVer)
	binary.LittleEndian.PutUint32(buf[nvOffID:], id)
	binary.LittleEndian.PutUint32(buf[nvOffType:], 0) // NVDRS_DWORD_TYPE
	binary.LittleEndian.PutUint32(buf[nvOffCurrent:], v)
	r := s.n.call(nvDRSSetSetting, s.h, s.prof, uintptr(unsafe.Pointer(&buf[0])))
	runtime.KeepAlive(buf)
	if r != 0 {
		return fmt.Errorf("SetSetting %d", r)
	}
	return nil
}

func (s *nvSession) restoreDefault(id uint32) {
	s.n.call(nvDRSRestoreDefault, s.h, s.prof, uintptr(id))
}

func (s *nvSession) save() error {
	if r := s.n.call(nvDRSSaveSettings, s.h); r != 0 {
		return fmt.Errorf("DRS_SaveSettings %d", r)
	}
	return nil
}

// ------------------------------------------------------------------- AMD --

const displayClass = `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`

type amdValue struct {
	Sub   string // "" = คีย์ของการ์ดจอ, "UMD" = ค่า 3D ของ Adrenalin
	Name  string
	Str   string // ค่าแบบข้อความ เก็บเป็น REG_BINARY UTF-16 (แบบที่ Adrenalin ใช้)
	DWord *uint32
	Label string
}

func u32(v uint32) *uint32 { return &v }

var amdSettings = []amdValue{
	{"UMD", "TFQ", "2", nil, "Texture Filtering Quality: Performance"},
	{"UMD", "ShaderCache", "2", nil, "Shader Cache: Always on"},
	{"UMD", "Tessellation_OPTION", "2", nil, "Tessellation Mode: Override application settings"},
	{"UMD", "Tessellation", "1", nil, "Maximum Tessellation Level: Off (x1)"},
	{"UMD", "VSyncControl", "0", nil, "Wait for Vertical Refresh: Always off"},
	{"", "EnableUlps", "", u32(0), "ULPS (ประหยัดไฟการ์ดจอตัวรอง): Off"},
	{"UMD", "TurboSync", "0", nil, "Enhanced Sync: Off"},
	{"UMD", "MLF", "0", nil, "Morphological Anti-Aliasing: Off"},
	{"UMD", "SurfaceFormatReplacements", "1", nil, "Surface Format Optimization: On"},
	{"", "KMD_ChillEnabled", "", u32(0), "Radeon Chill: Off"},
	{"", "KMD_RadeonBoostEnabled", "", u32(0), "Radeon Boost: Off"},
}

var amdLowLat = []amdValue{
	{"UMD", "FlipQueueSize", "1", nil, "Flip Queue Size: 1"},
	{"", "KMD_DeLagEnabled", "", u32(1), "Radeon Anti-Lag: On"},
}

func amdKeys() []string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, displayClass, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil
	}
	defer k.Close()
	names, _ := k.ReadSubKeyNames(-1)
	var out []string
	for _, n := range names {
		sk, err := registry.OpenKey(k, n, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		prov, _, _ := sk.GetStringValue("ProviderName")
		desc, _, _ := sk.GetStringValue("DriverDesc")
		sk.Close()
		if strings.Contains(strings.ToLower(prov+" "+desc), "advanced micro devices") || strings.Contains(strings.ToLower(desc), "radeon") {
			out = append(out, displayClass+`\`+n)
		}
	}
	return out
}

func utf16Bytes(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, len(u)*2)
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[i*2:], c)
	}
	return b
}

// ---------------------------------------------------------------- shared --

type gpuBackupItem struct {
	Vendor  string `json:"vendor"`
	ID      uint32 `json:"id,omitempty"`
	Key     string `json:"key,omitempty"`
	Name    string `json:"name,omitempty"`
	Existed bool   `json:"existed"`
	U32     uint32 `json:"u32,omitempty"`
	Type    uint32 `json:"type,omitempty"`
	Data    []byte `json:"data,omitempty"`
}

type gpuInfo struct {
	Vendor   string       `json:"vendor"` // nvidia / amd / none
	Applied  bool         `json:"applied"`
	Backup   bool         `json:"backup"`
	Cap      uint32       `json:"cap"`
	LowLat   bool         `json:"lowLat"`
	LatSet   []gpuSetting `json:"latSettings"`
	Refresh  int          `json:"refresh"`
	Settings []gpuSetting `json:"settings"`
	Message  string       `json:"m,omitempty"`
}

type gpuResult struct {
	OK      bool     `json:"ok"`
	Message string   `json:"m,omitempty"`
	Done    []string `json:"done"`
	Failed  []string `json:"failed"`
}

func (a *app) gpuBackupPath() string { return filepath.Join(a.dataDir, "gpu_backup.json") }

func (a *app) loadGpuBackup() []gpuBackupItem {
	var b []gpuBackupItem
	if d, err := os.ReadFile(a.gpuBackupPath()); err == nil {
		json.Unmarshal(d, &b)
	}
	return b
}

func hasBackup(b []gpuBackupItem, match func(gpuBackupItem) bool) bool {
	for _, x := range b {
		if match(x) {
			return true
		}
	}
	return false
}

func displayRefresh() int {
	var dm [220]byte
	binary.LittleEndian.PutUint16(dm[68:], 220) // dmSize
	r, _, _ := user32.NewProc("EnumDisplaySettingsW").Call(0, uintptr(0xFFFFFFFF), uintptr(unsafe.Pointer(&dm[0])))
	if r == 0 {
		return 0
	}
	return int(binary.LittleEndian.Uint32(dm[184:]))
}

func gpuVendor() string {
	if nvapiDLL.Load() == nil {
		return "nvidia"
	}
	if len(amdKeys()) > 0 {
		return "amd"
	}
	return "none"
}

func (a *app) gpuDriverInfo() gpuInfo {
	info := gpuInfo{Vendor: gpuVendor(), Refresh: displayRefresh(), Backup: len(a.loadGpuBackup()) > 0}
	switch info.Vendor {
	case "nvidia":
		info.Settings = nvSettings
		n, err := loadNV()
		if err != nil {
			info.Message = err.Error()
			return info
		}
		s, err := n.open()
		if err != nil {
			info.Message = err.Error()
			return info
		}
		defer s.close()
		v1, _, _ := s.get(0x1057EB71)
		v2, _, _ := s.get(0x00CE2691)
		info.Applied = v1 == 1 && v2 == 0x14
		info.Cap, _, _ = s.get(nvFrameLimitID)
		pr, _, _ := s.get(0x007BA09E)
		info.LowLat = pr == 1
		info.LatSet = nvLowLat
	case "amd":
		for _, x := range amdSettings {
			info.Settings = append(info.Settings, gpuSetting{Name: x.Name, Label: x.Label})
		}
		for _, x := range amdLowLat {
			info.LatSet = append(info.LatSet, gpuSetting{Name: x.Name, Label: x.Label})
		}
		for _, k := range amdKeys() {
			if uk, err := registry.OpenKey(registry.LOCAL_MACHINE, k+`\UMD`, registry.QUERY_VALUE); err == nil {
				b, _, _ := uk.GetBinaryValue("TFQ")
				fq, _, _ := uk.GetBinaryValue("FlipQueueSize")
				uk.Close()
				info.Applied = string(b) == string(utf16Bytes("2"))
				info.LowLat = string(fq) == string(utf16Bytes("1"))
			}
			if dk, err := registry.OpenKey(registry.LOCAL_MACHINE, k, registry.QUERY_VALUE); err == nil {
				if on, _, err := dk.GetIntegerValue("KMD_FRTEnabled"); err == nil && on == 1 {
					v, _, _ := dk.GetIntegerValue("KMD_MaxFrameRateRequested")
					info.Cap = uint32(v)
				}
				dk.Close()
			}
			break
		}
	}
	return info
}

// gpuDriverApply ตั้งค่าทั้งหมด cap = ล็อก FPS (0 = ไม่ล็อก)
func (a *app) gpuDriverApply(capFps int, lowLat bool) gpuResult {
	backup := a.loadGpuBackup()
	res := gpuResult{}
	switch gpuVendor() {
	case "nvidia":
		n, err := loadNV()
		if err != nil {
			return gpuResult{Message: err.Error()}
		}
		s, err := n.open()
		if err != nil {
			return gpuResult{Message: err.Error()}
		}
		defer s.close()
		all := append([]gpuSetting{}, nvSettings...)
		all = append(all, gpuSetting{nvFrameLimitID, "Max Frame Rate", uint32(max(capFps, 0)), ""})
		if lowLat {
			all = append(all, nvLowLat...)
		} else {
			// ปิดลด input lag: คืนค่าเดิมของสองข้อนี้ถ้าเคยตั้งไว้
			for _, x := range nvLowLat {
				for _, b := range backup {
					if b.Vendor == "nvidia" && b.ID == x.ID {
						if b.Existed {
							s.set(x.ID, b.U32)
						} else {
							s.restoreDefault(x.ID)
						}
					}
				}
			}
		}
		for _, x := range all {
			if !hasBackup(backup, func(b gpuBackupItem) bool { return b.Vendor == "nvidia" && b.ID == x.ID }) {
				v, set, err := s.get(x.ID)
				if err == nil {
					backup = append(backup, gpuBackupItem{Vendor: "nvidia", ID: x.ID, Existed: set, U32: v})
				}
			}
			if err := s.set(x.ID, x.Value); err != nil {
				res.Failed = append(res.Failed, x.Name)
			} else {
				res.Done = append(res.Done, x.Name)
			}
		}
		if err := s.save(); err != nil {
			return gpuResult{Message: err.Error(), Failed: res.Failed}
		}
	case "amd":
		keys := amdKeys()
		for _, k := range keys {
			vals := append([]amdValue{}, amdSettings...)
			if lowLat {
				vals = append(vals, amdLowLat...)
			}
			for _, x := range append(vals,
				amdValue{"", "KMD_FRTEnabled", "", u32(boolU32(capFps > 0)), ""},
				amdValue{"", "KMD_MaxFrameRateRequested", "", u32(uint32(max(capFps, 0))), ""}) {
				path := k
				if x.Sub != "" {
					path += `\` + x.Sub
				}
				kk, _, err := registry.CreateKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE|registry.SET_VALUE)
				if err != nil {
					res.Failed = append(res.Failed, x.Name)
					continue
				}
				if !hasBackup(backup, func(b gpuBackupItem) bool { return b.Vendor == "amd" && b.Key == path && b.Name == x.Name }) {
					item := gpuBackupItem{Vendor: "amd", Key: path, Name: x.Name}
					buf := make([]byte, 4096)
					if n, typ, err := kk.GetValue(x.Name, buf); err == nil {
						item.Existed, item.Type, item.Data = true, typ, buf[:n]
					}
					backup = append(backup, item)
				}
				if x.DWord != nil {
					err = kk.SetDWordValue(x.Name, *x.DWord)
				} else {
					err = kk.SetBinaryValue(x.Name, utf16Bytes(x.Str))
				}
				kk.Close()
				if err != nil {
					res.Failed = append(res.Failed, x.Name)
				} else if x.Label != "" {
					res.Done = append(res.Done, x.Label)
				}
			}
		}
		if len(keys) == 0 {
			return gpuResult{Message: "ไม่เจอไดรเวอร์ AMD"}
		}
		if !lowLat { // ปิดลด input lag: คืนค่าเดิมเฉพาะส่วนนี้
			keep := backup[:0]
			for _, b := range backup {
				isLat := false
				for _, x := range amdLowLat {
					isLat = isLat || (b.Vendor == "amd" && b.Name == x.Name)
				}
				if !isLat {
					keep = append(keep, b)
					continue
				}
				if k, err := registry.OpenKey(registry.LOCAL_MACHINE, b.Key, registry.SET_VALUE); err == nil {
					if b.Existed {
						setRawValue(k, b.Name, b.Type, b.Data)
					} else {
						k.DeleteValue(b.Name)
					}
					k.Close()
				}
			}
			backup = keep
		}
	default:
		return gpuResult{Message: "ไม่เจอการ์ดจอ NVIDIA หรือ AMD"}
	}
	if b, err := json.MarshalIndent(backup, "", " "); err == nil {
		os.WriteFile(a.gpuBackupPath(), b, 0o644)
	}
	res.OK = len(res.Done) > 0
	return res
}

func boolU32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// gpuDriverRestore คืนค่าที่เก็บไว้ทั้งหมด
func (a *app) gpuDriverRestore() gpuResult {
	backup := a.loadGpuBackup()
	if len(backup) == 0 {
		return gpuResult{Message: "ยังไม่มีค่าที่ต้องคืน"}
	}
	res := gpuResult{}
	var s *nvSession
	for _, b := range backup {
		if b.Vendor == "nvidia" && s == nil {
			if n, err := loadNV(); err == nil {
				s, _ = n.open()
			}
		}
	}
	for _, b := range backup {
		switch b.Vendor {
		case "nvidia":
			if s == nil {
				res.Failed = append(res.Failed, fmt.Sprintf("0x%08X", b.ID))
				continue
			}
			if b.Existed {
				if err := s.set(b.ID, b.U32); err != nil {
					res.Failed = append(res.Failed, fmt.Sprintf("0x%08X", b.ID))
					continue
				}
			} else {
				s.restoreDefault(b.ID)
			}
			res.Done = append(res.Done, fmt.Sprintf("0x%08X", b.ID))
		case "amd":
			k, err := registry.OpenKey(registry.LOCAL_MACHINE, b.Key, registry.SET_VALUE)
			if err != nil {
				res.Failed = append(res.Failed, b.Name)
				continue
			}
			if b.Existed {
				err = setRawValue(k, b.Name, b.Type, b.Data)
			} else {
				err = k.DeleteValue(b.Name)
				if err == registry.ErrNotExist {
					err = nil
				}
			}
			k.Close()
			if err != nil {
				res.Failed = append(res.Failed, b.Name)
			} else {
				res.Done = append(res.Done, b.Name)
			}
		}
	}
	if s != nil {
		if err := s.save(); err != nil {
			res.Message = err.Error()
		}
		s.close()
	}
	if len(res.Failed) == 0 {
		os.Remove(a.gpuBackupPath())
	}
	res.OK = len(res.Failed) == 0
	return res
}

func setRawValue(k registry.Key, name string, typ uint32, data []byte) error {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	var d *byte
	if len(data) > 0 {
		d = &data[0]
	}
	r, _, _ := windows.NewLazySystemDLL("advapi32.dll").NewProc("RegSetValueExW").Call(uintptr(k), uintptr(unsafe.Pointer(p)), 0, uintptr(typ), uintptr(unsafe.Pointer(d)), uintptr(len(data)))
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

// async wrappers (งานไดรเวอร์ใช้เวลาเป็นวินาที ไม่ให้หน้าต่างค้าง)
func (a *app) gpuInfoAsync(reqID int) bool {
	go func() { runtime.LockOSThread(); a.emitGpu(reqID, a.gpuDriverInfo()) }()
	return true
}
func (a *app) gpuApplyAsync(capFps int, lowLat bool, reqID int) bool {
	go func() { runtime.LockOSThread(); a.emitGpu(reqID, a.gpuDriverApply(capFps, lowLat)) }()
	return true
}
func (a *app) gpuRestoreAsync(reqID int) bool {
	go func() { runtime.LockOSThread(); a.emitGpu(reqID, a.gpuDriverRestore()) }()
	return true
}
func (a *app) emitGpu(reqID int, v any) {
	b, _ := json.Marshal(map[string]any{"reqId": reqID, "r": v})
	a.emit("onGpu", string(b))
}
