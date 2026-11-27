package main

// สร้างไฟล์ .lnk ด้วย IShellLink ของ Windows โดยตรง (ไม่ต้องเรียก PowerShell)

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ole32              = windows.NewLazySystemDLL("ole32.dll")
	pCoInitializeEx    = ole32.NewProc("CoInitializeEx")
	pCoUninitialize    = ole32.NewProc("CoUninitialize")
	pCoCreateInstance  = ole32.NewProc("CoCreateInstance")
	clsidShellLink, _  = windows.GUIDFromString("{00021401-0000-0000-C000-000000000046}")
	iidIShellLinkW, _  = windows.GUIDFromString("{000214F9-0000-0000-C000-000000000046}")
	iidIPersistFile, _ = windows.GUIDFromString("{0000010b-0000-0000-C000-000000000046}")
)

type comObj struct{ vtbl *[32]uintptr }

func (o *comObj) call(i int, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(o.vtbl[i], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return r
}

func wstr(s string) uintptr {
	p, _ := windows.UTF16PtrFromString(s)
	return uintptr(unsafe.Pointer(p))
}

// writeShortcut สร้าง lnk ไปที่ target พร้อมตัวเลือก args
func writeShortcut(lnk, target, args, workDir string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const coinitApartment = 0x2
	if r, _, _ := pCoInitializeEx.Call(0, coinitApartment); r == 0 || r == 1 { // S_OK / S_FALSE
		defer pCoUninitialize.Call()
	}
	var sl *comObj
	if r, _, _ := pCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidShellLink)), 0, 1 /* CLSCTX_INPROC_SERVER */, uintptr(unsafe.Pointer(&iidIShellLinkW)), uintptr(unsafe.Pointer(&sl))); r != 0 || sl == nil {
		return fmt.Errorf("CoCreateInstance 0x%X", r)
	}
	defer sl.call(2) // Release
	// IShellLinkW: 20 SetPath, 11 SetArguments, 9 SetWorkingDirectory, 7 SetDescription
	if r := sl.call(20, wstr(target)); r != 0 {
		return fmt.Errorf("SetPath 0x%X", r)
	}
	sl.call(11, wstr(args))
	sl.call(9, wstr(workDir))
	sl.call(7, wstr("FiveM (Zan Booster)"))
	var pf *comObj
	if r := sl.call(0, uintptr(unsafe.Pointer(&iidIPersistFile)), uintptr(unsafe.Pointer(&pf))); r != 0 || pf == nil {
		return fmt.Errorf("IPersistFile 0x%X", r)
	}
	defer pf.call(2)
	if r := pf.call(6, wstr(lnk), 1); r != 0 { // IPersistFile::Save
		return fmt.Errorf("Save 0x%X", r)
	}
	return nil
}
