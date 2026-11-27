package main

// อุณหภูมิการ์ดจอ: อ่านจาก WDDM (D3DKMT ADAPTERPERFDATA) แบบเดียวกับ Task Manager
// ใช้ได้ทั้ง NVIDIA / AMD / Intel บน Windows 10 2004 ขึ้นไป ถ้าไม่ได้ค่า ลอง NVAPI ต่อ

import (
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	gdi32k               = windows.NewLazySystemDLL("gdi32.dll")
	pD3DKMTEnumAdapters2 = gdi32k.NewProc("D3DKMTEnumAdapters2")
	pD3DKMTQueryAdapter  = gdi32k.NewProc("D3DKMTQueryAdapterInfo")
	pD3DKMTCloseAdapter  = gdi32k.NewProc("D3DKMTCloseAdapter")
	nvEnumPhysicalGPUs   = uint32(0xE5AC921F)
	nvGetThermalSettings = uint32(0xE3640A56)
	tempMu               sync.Mutex
	tempNV               *nvAPI
	tempNVTried          bool
)

type d3dAdapterInfo struct {
	h       uint32
	luidLo  uint32
	luidHi  int32
	sources uint32
	precise int32
}

type d3dPerfData struct {
	physIdx  uint32
	_        uint32
	memFreq  uint64
	maxMem   uint64
	maxMemOC uint64
	memBW    uint64
	pcieBW   uint64
	fanRPM   uint32
	power    uint32
	temp     uint32 // deci-Celsius
	override uint8
	_        [3]byte
}

type d3dQuery struct {
	h    uint32
	typ  uint32
	data uintptr
	size uint32
	_    uint32
}

func d3dGpuTemp() float64 {
	if pD3DKMTEnumAdapters2.Find() != nil || pD3DKMTQueryAdapter.Find() != nil {
		return 0
	}
	var e struct {
		n uint32
		_ uint32
		p uintptr
	}
	if r, _, _ := pD3DKMTEnumAdapters2.Call(uintptr(unsafe.Pointer(&e))); r != 0 || e.n == 0 {
		return 0
	}
	ads := make([]d3dAdapterInfo, e.n)
	e.p = uintptr(unsafe.Pointer(&ads[0]))
	if r, _, _ := pD3DKMTEnumAdapters2.Call(uintptr(unsafe.Pointer(&e))); r != 0 {
		return 0
	}
	best := 0.0
	for i := 0; i < int(e.n); i++ {
		var pd d3dPerfData
		q := d3dQuery{h: ads[i].h, typ: 62, data: uintptr(unsafe.Pointer(&pd)), size: uint32(unsafe.Sizeof(pd))} // KMTQAITYPE_ADAPTERPERFDATA
		if r, _, _ := pD3DKMTQueryAdapter.Call(uintptr(unsafe.Pointer(&q))); r == 0 {
			if t := float64(pd.temp) / 10; t > 5 && t < 130 && t > best {
				best = t
			}
		}
		h := ads[i].h
		pD3DKMTCloseAdapter.Call(uintptr(unsafe.Pointer(&h)))
	}
	return best
}

func nvGpuTemp() float64 {
	if !tempNVTried {
		tempNVTried = true
		if err := nvapiDLL.Load(); err == nil {
			n := &nvAPI{fns: map[uint32]uintptr{}}
			ok := true
			for _, id := range []uint32{nvInitialize, nvEnumPhysicalGPUs, nvGetThermalSettings} {
				p, _, _ := nvQueryInt.Call(uintptr(id))
				if p == 0 {
					ok = false
					break
				}
				n.fns[id] = p
			}
			if ok && n.call(nvInitialize) == 0 {
				tempNV = n
			}
		}
	}
	if tempNV == nil {
		return 0
	}
	var gpus [64]uintptr
	var cnt uint32
	if tempNV.call(nvEnumPhysicalGPUs, uintptr(unsafe.Pointer(&gpus[0])), uintptr(unsafe.Pointer(&cnt))) != 0 || cnt == 0 {
		return 0
	}
	// NV_GPU_THERMAL_SETTINGS_V2: version, count, sensor[3]{controller, min, max, current, target}
	var ts [17]int32
	ts[0] = 68 | 2<<16
	if r, _, _ := syscall.SyscallN(tempNV.fns[nvGetThermalSettings], gpus[0], 15, uintptr(unsafe.Pointer(&ts[0]))); int32(r) != 0 || ts[1] == 0 {
		return 0
	}
	if t := float64(ts[2+3]); t > 5 && t < 130 {
		return t
	}
	return 0
}

func gpuTempC() float64 {
	tempMu.Lock()
	defer tempMu.Unlock()
	if t := d3dGpuTemp(); t > 0 {
		return t
	}
	return nvGpuTemp()
}
