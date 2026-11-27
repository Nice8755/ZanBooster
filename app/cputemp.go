package main

// อุณหภูมิ CPU: Windows ไม่มี API อ่านเซนเซอร์ CPU ตรงๆ ถ้าไม่ลงไดรเวอร์เพิ่ม (ซึ่งแอนตี้ไวรัส/กันโกงมักบล็อก)
// เลยอ่านจาก Thermal Zone ของ ACPI ผ่าน PDH แทน โน้ตบุ๊กส่วนใหญ่มีค่าจริง พีซีตั้งโต๊ะหลายรุ่นไม่มีหรือเป็นค่าหลอก
// ค่าที่ไม่น่าเชื่อ (ต่ำกว่า 30°C หรือเกิน 115°C) จะไม่แสดง (คืน 0)

import (
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var cpuTemp struct {
	mu       sync.Mutex
	tried    bool
	broken   bool
	q        uintptr
	hi, kelv uintptr
}

func cpuTempC() (c float64) {
	cpuTemp.mu.Lock()
	defer cpuTemp.mu.Unlock()
	if cpuTemp.broken {
		return 0
	}
	// เซนเซอร์เป็นของแถม: ถ้าพังกลางทางให้เลิกอ่าน ไม่ให้โปรแกรมปิดตัว
	defer func() {
		if recover() != nil {
			cpuTemp.broken = true
			c = 0
		}
	}()
	if !cpuTemp.tried {
		cpuTemp.tried = true
		if pPdhOpenQuery.Find() != nil || pPdhAddEnglish.Find() != nil || pPdhCollect.Find() != nil || pPdhGetArray.Find() != nil {
			cpuTemp.broken = true
			return 0
		}
		if r, _, _ := pPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&cpuTemp.q))); r != 0 {
			cpuTemp.broken = true
			return 0
		}
		add := func(path string, out *uintptr) {
			p, _ := windows.UTF16PtrFromString(path)
			if r, _, _ := pPdhAddEnglish.Call(cpuTemp.q, uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(out))); r != 0 {
				*out = 0
			}
		}
		add(`\Thermal Zone Information(*)\High Precision Temperature`, &cpuTemp.hi) // 1 = 0.1 K
		add(`\Thermal Zone Information(*)\Temperature`, &cpuTemp.kelv)              // K
		if cpuTemp.hi == 0 && cpuTemp.kelv == 0 {
			cpuTemp.broken = true // เครื่องนี้ไม่มี thermal zone
			return 0
		}
	}
	if r, _, _ := pPdhCollect.Call(cpuTemp.q); r != 0 {
		return 0
	}
	if cpuTemp.hi != 0 {
		c = pickZoneTemp(pdhArray(cpuTemp.hi), 10)
	}
	if c == 0 && cpuTemp.kelv != 0 {
		c = pickZoneTemp(pdhArray(cpuTemp.kelv), 1)
	}
	return c
}

// pickZoneTemp เลือก thermal zone ที่น่าจะเป็น CPU: ชื่อมี CPU ก่อน ไม่งั้นเอาค่าสูงสุดที่ดูสมเหตุสมผล
func pickZoneTemp(zones map[string]float64, div float64) float64 {
	best, cpuNamed := 0.0, false
	for name, raw := range zones {
		c := raw/div - 273.15
		if c < 30 || c > 115 { // ค่าหลอก/ไม่มีเซนเซอร์จริง (ACPI หลายเครื่องตอบ 27.8°C ตลอด)
			continue
		}
		isCPU := strings.Contains(strings.ToUpper(name), "CPU")
		switch {
		case isCPU && (!cpuNamed || c > best):
			best, cpuNamed = c, true
		case !isCPU && !cpuNamed && c > best:
			best = c
		}
	}
	return best
}
