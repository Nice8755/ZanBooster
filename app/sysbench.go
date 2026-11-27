//go:build windows

package main

// sysbench.go: ถอดจาก exe 4.11.15 · วัดเครื่องโดยไม่ต้องเข้าเกม (ก่อน/หลังบูสต์)
// สูตรและค่าคงที่ตรงกับตัวเก่าทุกตัว เพื่อให้ผลที่วัดไว้ก่อนเปลี่ยนตัวโปรแกรมยังเทียบกันได้

import (
	"math"
	"math/bits"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pQPF      = windows.NewLazySystemDLL("kernel32.dll").NewProc("QueryPerformanceFrequency")
	pQPC      = windows.NewLazySystemDLL("kernel32.dll").NewProc("QueryPerformanceCounter")
	pSleep    = windows.NewLazySystemDLL("kernel32.dll").NewProc("Sleep")
	qpcFreq   float64
	benchSink uint64
)

func qpcUs() float64 {
	if qpcFreq == 0 {
		var f int64
		pQPF.Call(uintptr(unsafe.Pointer(&f)))
		qpcFreq = float64(f) / 1e6
	}
	var c int64
	pQPC.Call(uintptr(unsafe.Pointer(&c)))
	return float64(c) / qpcFreq
}

// benchWork: งานคำนวณ 1 ชุด (เลขสุ่ม xorshift + นับบิต + ทศนิยม)
func benchWork(seed uint64) uint64 {
	x := seed | 1
	f := 1.0001
	for i := 0; i < 20000; i++ {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		x += uint64(bits.OnesCount64(x))
		f = f*1.0000001 + float64(x&1023)*1e-9
		if f > 4 {
			f = 1.0001
		}
	}
	return x + uint64(f)
}

// cpuScore: จำนวนชุดงานที่ทำได้ต่อวินาที / 10 (threads เธรดพร้อมกัน นาน d)
func cpuScore(threads int, d time.Duration) float64 {
	var total int64
	var wg sync.WaitGroup
	end := time.Now().Add(d)
	for t := 0; t < threads; t++ {
		wg.Add(1)
		go func(seed uint64) {
			defer wg.Done()
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			var n int64
			x := seed + 7
			for time.Now().Before(end) {
				x = benchWork(x)
				n++
			}
			atomic.AddUint64(&benchSink, x)
			atomic.AddInt64(&total, n)
		}(uint64(t))
	}
	wg.Wait()
	return float64(total) / d.Seconds() / 10
}

// timerTest: Sleep(1) เกินไปกี่ ms (ความแม่นของนาฬิกา Windows) + เครื่องสะดุดเกิน 50 µs บ่อยแค่ไหน
func timerTest() map[string]any {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var over []float64
	for i := 0; i < 300; i++ {
		t0 := qpcUs()
		pSleep.Call(1)
		over = append(over, (qpcUs()-t0)/1000-1)
	}
	sort.Float64s(over)
	sum := 0.0
	for _, v := range over {
		sum += v
	}
	avg := sum / float64(len(over))
	t0 := qpcUs()
	prev, stalls, worst := t0, 0, 0.0
	for {
		now := qpcUs()
		if now-t0 > 1.5e6 {
			break
		}
		if d := now - prev; d > 50 {
			stalls++
			if d > worst {
				worst = d
			}
		}
		prev = now
	}
	return map[string]any{
		"sleepAvg": math.Round(avg*1000) / 1000,
		"sleepP99": math.Round(over[len(over)*99/100]*1000) / 1000,
		"stalls":   math.Round(float64(stalls)/1.5*10) / 10,
		"worstUs":  math.Round(worst),
	}
}

// ramTest: เวลาเข้าถึงแรมแบบสุ่ม (ns) + ความเร็วคัดลอก (GB/s)
func ramTest() map[string]any {
	const n = 1 << 23
	next := make([]uint32, n)
	perm := make([]uint32, n)
	for i := range perm {
		perm[i] = uint32(i)
	}
	x := uint64(0x9e3779b97f4a7c15)
	for i := n - 1; i > 0; i-- {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		j := x % uint64(i+1)
		perm[i], perm[j] = perm[j], perm[i]
	}
	for i := 0; i < n; i++ {
		next[perm[i]] = perm[(i+1)&(n-1)]
	}
	runtime.LockOSThread()
	t := time.Now()
	p := uint32(0)
	for i := 0; i < 4000000; i++ {
		p = next[p]
	}
	lat := float64(time.Since(t)) / 4e6
	runtime.UnlockOSThread()
	atomic.AddUint64(&benchSink, uint64(p))
	next, perm = nil, nil
	src, dst := make([]byte, 64<<20), make([]byte, 64<<20)
	for i := 0; i < len(src); i += 4096 {
		src[i], dst[i] = byte(i), 0
	}
	copy(dst, src)
	t = time.Now()
	k := 0
	for time.Since(t) < 1200*time.Millisecond {
		copy(dst, src)
		k++
	}
	gbs := float64(k<<27) / time.Since(t).Seconds() / 1e9
	src, dst = nil, nil
	runtime.GC()
	return map[string]any{"latNs": math.Round(lat*10) / 10, "gbs": math.Round(gbs*10) / 10}
}

func (a *app) sysBenchAsync(part string, reqID int) bool {
	go func() {
		var r any
		switch part {
		case "cpu1":
			r = map[string]any{"score": math.Round(cpuScore(1, 3*time.Second))}
		case "cpuN":
			r = map[string]any{"score": math.Round(cpuScore(runtime.NumCPU(), 3*time.Second)), "threads": runtime.NumCPU()}
		case "timer":
			r = timerTest()
		case "ram":
			r = ramTest()
		default:
			r = map[string]any{"err": "unknown"}
		}
		a.emitGpu(reqID, r)
	}()
	return true
}
