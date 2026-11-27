//go:build windows

package main

// recover_ram.go: ถอดจาก exe 4.11.15 (recover.go: trimOthersN, purgeStandbyGentle, reRamN, reRamAsync)
// เคลียร์แรมระหว่างเล่น: คืนแรมเฉพาะโปรแกรมที่กินเกิน 120 MB เรียงจากมากไปน้อย ไม่แตะเกม/หน้าต่างที่ใช้อยู่/ผู้ใช้อื่น

import (
	"os"
	"sort"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	pGetWindowThreadPid  = user32.NewProc("GetWindowThreadProcessId")
)

// trimOthersN: n = จำนวนสูงสุด (0 = ไม่จำกัด) คืนจำนวนที่คืนแรมได้
func trimOthersN(n int) int {
	enablePrivilege("SeDebugPrivilege")
	enablePrivilege("SeIncreaseQuotaPrivilege")
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snap)
	self := uint32(os.Getpid())
	var mySess uint32
	_ = windows.ProcessIdToSessionId(self, &mySess)
	var fgPid uint32
	if fg, _, _ := pGetForegroundWindow.Call(); fg != 0 {
		pGetWindowThreadPid.Call(fg, uintptr(unsafe.Pointer(&fgPid)))
	}
	type cand struct {
		pid uint32
		ws  uint64
	}
	var list []cand
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err := windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		name := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
		if pe.ProcessID <= 4 || pe.ProcessID == self || pe.ProcessID == fgPid || noTrim[name] || strings.HasPrefix(name, "fivem") || strings.HasPrefix(name, "gta") {
			continue
		}
		var s uint32
		if windows.ProcessIdToSessionId(pe.ProcessID, &s) != nil || s != mySess {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_SET_QUOTA, false, pe.ProcessID)
		if err != nil {
			continue
		}
		var pmc [10]uint64 // PROCESS_MEMORY_COUNTERS (80 ไบต์) [2] = WorkingSetSize
		pmc[0] = 80
		if r, _, _ := pGetProcMemInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&pmc[0])), 80); r != 0 && pmc[2] > 120<<20 {
			list = append(list, cand{pe.ProcessID, pmc[2]})
		}
		windows.CloseHandle(h)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ws > list[j].ws })
	done := 0
	for _, c := range list {
		if n > 0 && done >= n {
			break
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_SET_QUOTA, false, c.pid)
		if err != nil {
			continue
		}
		if r, _, _ := pEmptyWorkingSet.Call(uintptr(h)); r != 0 {
			done++
		}
		windows.CloseHandle(h)
		time.Sleep(40 * time.Millisecond) // ทีละตัว ไม่ให้ดิสก์ทำงานหนักพร้อมกัน
	}
	return done
}

// purgeStandbyGentle: ล้างเฉพาะแคชแรมสำคัญน้อย (low priority standby) ไม่ทำให้เกมโหลดของใหม่
func purgeStandbyGentle() {
	enablePrivilege("SeProfileSingleProcessPrivilege")
	c := uint32(5)
	pNtSetSysInfo.Call(80, uintptr(unsafe.Pointer(&c)), 4)
}

func (a *app) reRamN(n int) map[string]any {
	before := freeMB()
	t := trimOthersN(n)
	purgeStandbyGentle()
	after := freeMB()
	freed := 0.0
	if before >= 0 && after > before {
		freed = after - before
	}
	pid, _ := gtaProcess()
	return map[string]any{"ok": true, "freedMB": freed, "freeMB": after, "trimmed": t, "game": pid != 0}
}

func (a *app) reRamAsync(reqID int) bool {
	go func() { a.emitGpu(reqID, a.reRamN(0)) }()
	return true
}
