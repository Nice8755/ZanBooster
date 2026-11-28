//go:build windows

package main

// bgprio.go (4.20.06): ตอนเล่น FiveM ลดความสำคัญของโปรแกรมเบื้องหลังที่กินซีพียู (เบราว์เซอร์ Steam OneDrive ฯลฯ)
// เป็น "ต่ำกว่าปกติ" ให้เกมได้ซีพียูก่อน ออกเกมแล้วคืนค่าเดิมทุกตัว · จดไว้ในไฟล์ ถ้าดับกลางคันจะคืนให้ตอนตัวช่วยเปิดรอบหน้า
// ไม่แตะ Discord / โปรแกรมสตรีม / เสียง (กันเสียงคุยหรือไลฟ์สะดุด) และไม่แตะตัวเกมหรือกันโกง

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var bgApps = map[string]bool{
	"chrome.exe": true, "msedge.exe": true, "firefox.exe": true, "opera.exe": true, "brave.exe": true, "vivaldi.exe": true,
	"steam.exe": true, "steamwebhelper.exe": true, "epicgameslauncher.exe": true, "epicwebhelper.exe": true, "eadesktop.exe": true,
	"onedrive.exe": true, "dropbox.exe": true, "googledrivefs.exe": true,
	"ms-teams.exe": true, "teams.exe": true, "whatsapp.exe": true, "telegram.exe": true, "line.exe": true,
	"adobe desktop service.exe": true, "creative cloud.exe": true, "searchapp.exe": true, "searchhost.exe": true,
}

type bgEntry struct {
	Name string `json:"n"`
	Prio uint32 `json:"p"`
}

type bgPrio struct {
	path string
	m    map[uint32]bgEntry
	last time.Time
}

func newBgPrio(dataDir string) *bgPrio {
	b := &bgPrio{path: filepath.Join(dataDir, "bg_prio.json"), m: map[uint32]bgEntry{}}
	if raw, err := os.ReadFile(b.path); err == nil { // ค้างจากรอบก่อน (ดับกลางคัน): คืนให้ก่อน
		_ = json.Unmarshal(raw, &b.m)
		b.restore()
	}
	return b
}

func (b *bgPrio) save() {
	if len(b.m) == 0 {
		os.Remove(b.path)
		return
	}
	raw, _ := json.Marshal(b.m)
	_ = os.WriteFile(b.path, raw, 0o644)
}

// procNames: pid -> ชื่อไฟล์ (ตัวเล็ก) ของทุก process
func procNames() map[uint32]string {
	out := map[uint32]string{}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return out
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		out[e.ProcessID] = strings.ToLower(windows.UTF16ToString(e.ExeFile[:]))
	}
	return out
}

// lower: เรียกทุกรอบตอนเกมเปิด (ทำจริงทุก 15 วินาที จับโปรแกรมที่เพิ่งเปิดด้วย) คืนจำนวนที่ลดอยู่
func (b *bgPrio) lower() int {
	if time.Since(b.last) < 15*time.Second {
		return len(b.m)
	}
	b.last = time.Now()
	changed := false
	for pid, name := range procNames() {
		if !bgApps[name] {
			continue
		}
		if _, done := b.m[pid]; done {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			continue
		}
		if pc, err := windows.GetPriorityClass(h); err == nil && (pc == windows.NORMAL_PRIORITY_CLASS || pc == windows.ABOVE_NORMAL_PRIORITY_CLASS) {
			if windows.SetPriorityClass(h, windows.BELOW_NORMAL_PRIORITY_CLASS) == nil {
				b.m[pid] = bgEntry{name, pc}
				changed = true
			}
		}
		windows.CloseHandle(h)
	}
	if changed {
		b.save()
	}
	return len(b.m)
}

// restore: คืนค่าเดิมเฉพาะตัวที่ยังเป็นค่าที่เราตั้ง และชื่อยังตรง (กันเลข pid ถูกใช้ซ้ำ)
func (b *bgPrio) restore() {
	if len(b.m) == 0 {
		return
	}
	names := procNames()
	for pid, e := range b.m {
		if names[pid] != e.Name {
			continue
		}
		if h, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid); err == nil {
			if pc, err := windows.GetPriorityClass(h); err == nil && pc == windows.BELOW_NORMAL_PRIORITY_CLASS {
				windows.SetPriorityClass(h, e.Prio)
			}
			windows.CloseHandle(h)
		}
	}
	b.m = map[uint32]bgEntry{}
	b.last = time.Time{}
	b.save()
}
