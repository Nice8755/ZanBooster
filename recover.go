package main

// กู้ FPS แบบเร็ว (ไม่ต้องออกเกม)
//   - คืนแรมจากโปรแกรมอื่นที่เปิดค้าง (EmptyWorkingSet ทุก process ยกเว้นเกมและส่วนสำคัญของ Windows)
//   - ล้าง Modified / Standby list
//   - ตั้ง Priority High, แกนตัวแรง และแผนพลังงานให้เกมใหม่ เผื่อมีอะไรมาเปลี่ยน
//   - เช็ก VRAM กับแรมของเกม ถ้าเต็มจากในเกมเอง บอกให้ใช้ "รีเกมแล้วเข้าเซิร์ฟเดิม" (กู้จากข้างนอกไม่ได้)
// ทำงานใน process เบื้องหลัง เรียกได้จากปุ่มในโปรแกรม (event) หรือปุ่มลัด Ctrl+Alt+F ระหว่างเล่น

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	pEmptyWorkingSet = kernel32.NewProc("K32EmptyWorkingSet")
	pGetProcMemInfo  = kernel32.NewProc("K32GetProcessMemoryInfo")
	pRegisterHotKey  = user32.NewProc("RegisterHotKey")
	pUnregisterHK    = user32.NewProc("UnregisterHotKey")
	pMsgWaitMulti    = user32.NewProc("MsgWaitForMultipleObjects")
	pMessageBeep     = user32.NewProc("MessageBeep")

	hotkeyState atomic.Value // "ok" / "busy" / "off"
	vramCapOnce sync.Once
	vramCapMB   float64
)

type recoverResult struct {
	At        int64   `json:"at"`
	Game      bool    `json:"game"`
	FreedMB   float64 `json:"freedMB"`
	Trimmed   int     `json:"trimmed"`
	StandbyMB float64 `json:"standbyMB"`
	FreeMB    float64 `json:"freeMB"`
	Prio      bool    `json:"prio"`
	Aff       bool    `json:"aff"`
	Power     bool    `json:"power"`
	VramPct   float64 `json:"vramPct"`
	GameMB    float64 `json:"gameMB"`
	Level     string  `json:"lv"`
	Advice    string  `json:"advice"`
}

// process ที่ไม่ควรไปยุ่ง (ตัดแรมแล้วอาจทำให้ภาพ/เสียงสะดุด หรือเป็นส่วนสำคัญของระบบ)
var noTrim = map[string]bool{
	"system": true, "registry": true, "memory compression": true, "smss.exe": true, "csrss.exe": true,
	"wininit.exe": true, "winlogon.exe": true, "services.exe": true, "lsass.exe": true, "dwm.exe": true,
	"audiodg.exe": true, "fontdrvhost.exe": true, "explorer.exe": true, "zanbooster.exe": true,
}

func gpuVramCapMB() float64 {
	vramCapOnce.Do(func() {
		const cls = `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, cls, registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			return
		}
		defer k.Close()
		names, _ := k.ReadSubKeyNames(-1)
		for _, n := range names {
			sk, err := registry.OpenKey(k, n, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			if v, _, err := sk.GetIntegerValue("HardwareInformation.qwMemorySize"); err == nil {
				vramCapMB = max(vramCapMB, float64(v)/(1<<20))
			} else if b, _, err := sk.GetBinaryValue("HardwareInformation.qwMemorySize"); err == nil && len(b) >= 8 {
				vramCapMB = max(vramCapMB, float64(*(*uint64)(unsafe.Pointer(&b[0])))/(1<<20))
			} else if b, _, err := sk.GetBinaryValue("HardwareInformation.MemorySize"); err == nil && len(b) >= 4 {
				vramCapMB = max(vramCapMB, float64(*(*uint32)(unsafe.Pointer(&b[0])))/(1<<20))
			}
			sk.Close()
		}
	})
	return vramCapMB
}

func processPrivateMB(pid uint32) float64 {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(h)
	var pmc [10]uintptr // PROCESS_MEMORY_COUNTERS_EX: ช่องแรก = cb + PageFaultCount, ช่องสุดท้าย = PrivateUsage
	*(*uint32)(unsafe.Pointer(&pmc[0])) = 80
	if r, _, _ := pGetProcMemInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&pmc[0])), 80); r == 0 {
		return 0
	}
	return float64(pmc[9]) / (1 << 20)
}

// trimOthers คืนแรมจากทุก process ยกเว้นเกม คืนจำนวนที่ตัดได้
func trimOthers() int {
	enablePrivilege("SeDebugPrivilege")
	enablePrivilege("SeIncreaseQuotaPrivilege")
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snap)
	self := uint32(os.Getpid())
	n := 0
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err := windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		name := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
		if pe.ProcessID <= 4 || pe.ProcessID == self || noTrim[name] || strings.HasPrefix(name, "fivem") || strings.HasPrefix(name, "gta") {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_SET_QUOTA, false, pe.ProcessID)
		if err != nil {
			continue
		}
		if r, _, _ := pEmptyWorkingSet.Call(uintptr(h)); r != 0 {
			n++
		}
		windows.CloseHandle(h)
	}
	return n
}

func (a *app) softRecover(gp uint32, pMask uintptr, cfg gmSettings, pq *pdhQuery) recoverResult {
	r := recoverResult{At: time.Now().Unix(), Game: gp != 0}
	before := freeMB()
	r.Trimmed = trimOthers()
	r.StandbyMB = standbyMB()
	if enablePrivilege("SeProfileSingleProcessPrivilege") == nil {
		for _, cmd := range []uint32{4} { // 4 = purge standby list (ไม่ flush ลงดิสก์ เครื่องจะไม่หน่วง)
			c := cmd
			pNtSetSysInfo.Call(80, uintptr(unsafe.Pointer(&c)), 4)
		}
	}
	r.FreeMB = freeMB()
	if before >= 0 && r.FreeMB > before {
		r.FreedMB = r.FreeMB - before
	}
	if gp != 0 {
		if h, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, gp); err == nil {
			r.Prio = windows.SetPriorityClass(h, windows.HIGH_PRIORITY_CLASS) == nil
			windows.CloseHandle(h)
		}
		if cfg.On && cfg.Aff && pMask != 0 {
			r.Aff = setAffinity(gp, pMask) == nil
		}
		if cfg.On && cfg.Power {
			r.Power = a.ensureZanScheme()
		}
		r.GameMB = processPrivateMB(gp)
		if pq != nil {
			pq.sample()
			if _, _, all := pq.extra(gp); all > 0 && gpuVramCapMB() > 0 {
				r.VramPct = all / gpuVramCapMB() * 100
			}
		}
	}
	switch {
	case !r.Game:
		r.Level, r.Advice = "info", "ตอนนี้ไม่ได้เปิดเกม คืนแรมให้แล้ว"
	case r.VramPct >= 92:
		r.Level, r.Advice = "warn", fmt.Sprintf("VRAM การ์ดจอเต็ม (%.0f%%) ส่วนนี้กู้จากข้างนอกเกมไม่ได้ ให้กด \"รีเกมแล้วเข้าเซิร์ฟเดิม\" และลด Texture Quality ลง 1 ขั้นจะไม่กลับมาเต็มอีก", r.VramPct)
	case r.GameMB >= 10240:
		r.Level, r.Advice = "warn", fmt.Sprintf("เกมใช้แรมไป %.1f GB แล้ว (สคริปต์เซิร์ฟสะสม) ถ้ายังไม่ลื่นให้กด \"รีเกมแล้วเข้าเซิร์ฟเดิม\"", r.GameMB/1024)
	default:
		r.Level, r.Advice = "ok", "เคลียร์แล้ว ถ้าเฟรมยังไม่กลับมา ให้กด \"รีเกมแล้วเข้าเซิร์ฟเดิม\""
	}
	return r
}

func (a *app) hotkeyEnabled() bool {
	c := a.gmConfig()
	return c.Hotkey == nil || *c.Hotkey
}

// hotkeyLoop ปุ่มลัด Ctrl+Alt+F ใช้ได้ระหว่างอยู่ในเกม (RegisterHotKey ไม่ยุ่งกับตัวเกม)
func (a *app) hotkeyLoop(ev windows.Handle) {
	runtime.LockOSThread()
	const (
		id         = 0x5A42
		modAlt     = 0x1
		modControl = 0x2
		modNoRep   = 0x4000
		wmHotkey   = 0x0312
		qsAllInput = 0x04FF
	)
	registered := false
	hotkeyState.Store("off")
	var msg [6]uint64 // MSG = 48 bytes
	for {
		want := a.hotkeyEnabled()
		if want && !registered {
			if r, _, _ := pRegisterHotKey.Call(0, id, modControl|modAlt|modNoRep, 'F'); r != 0 {
				registered = true
				hotkeyState.Store("ok")
			} else {
				hotkeyState.Store("busy") // มีโปรแกรมอื่นใช้ปุ่มนี้อยู่
			}
		} else if !want && registered {
			pUnregisterHK.Call(0, id)
			registered = false
			hotkeyState.Store("off")
		} else if !want {
			hotkeyState.Store("off")
		}
		pMsgWaitMulti.Call(0, 0, 0, 2000, qsAllInput)
		for {
			r, _, _ := pPeekMessage.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0, 1)
			if r == 0 {
				break
			}
			if uint32(msg[1]) == wmHotkey {
				pMessageBeep.Call(0)
				windows.SetEvent(ev)
			}
		}
	}
}

// triggerRecover ปุ่ม "กู้ FPS" ในหน้าโปรแกรม: สั่งตัวเบื้องหลังให้ทำ
func (a *app) triggerRecover() bool {
	if !mutexExists(gmMutexName) {
		a.ensureGameModeHelper()
		for i := 0; i < 20 && !mutexExists(gmMutexName); i++ {
			time.Sleep(150 * time.Millisecond)
		}
	}
	p, _ := windows.UTF16PtrFromString(gmRecoverName)
	h, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, p)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	return windows.SetEvent(h) == nil
}
