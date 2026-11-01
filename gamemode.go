package main

// โหมดเล่นเกม: เปิดเองตอนเกมเปิด ปิดเองตอนเกมปิด
//   - ให้เกมใช้แต่แกนตัวแรง (เฉพาะซีพียูที่มีแกน 2 แบบ เช่น Intel รุ่น 12 ขึ้นไป)
//   - จังหวะนาฬิกา 0.5 ms (เฉพาะ Windows 11 ขึ้นไป: Windows 10 ตั้งแต่ 2004 ไม่ยอมให้โปรแกรมอื่นปรับแทนเกม)
//   - ล้างแรม Standby อัตโนมัติเมื่อแรมว่างเหลือน้อย
//   - สลับไปใช้แผนพลังงาน ZAN_Project_Optimizer ระหว่างเล่น แล้วคืนแผนเดิม
//   - ดูว่าซีพียูหรือการ์ดจอเป็นตัวถ่วง

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	powrprof          = windows.NewLazySystemDLL("powrprof.dll")
	pPowerGetActive   = powrprof.NewProc("PowerGetActiveScheme")
	pPowerSetActive   = powrprof.NewProc("PowerSetActiveScheme")
	pGetLPIEx         = kernel32.NewProc("GetLogicalProcessorInformationEx")
	pSetAffinity      = kernel32.NewProc("SetProcessAffinityMask")
	pGetAffinity      = kernel32.NewProc("GetProcessAffinityMask")
	pNtSetTimerRes    = ntdll.NewProc("NtSetTimerResolution")
	pdh               = windows.NewLazySystemDLL("pdh.dll")
	pPdhOpenQuery     = pdh.NewProc("PdhOpenQueryW")
	pPdhAddEnglish    = pdh.NewProc("PdhAddEnglishCounterW")
	pPdhCollect       = pdh.NewProc("PdhCollectQueryData")
	pPdhGetArray      = pdh.NewProc("PdhGetFormattedCounterArrayW")
	pPdhCloseQuery    = pdh.NewProc("PdhCloseQuery")
	zanSchemeGUID, _  = windows.GUIDFromString("{5f7b1c3e-9a4d-4e2b-8c61-0f9e2d7a4b11}")
	gameModeStateLock sync.Mutex
)

type gmSettings struct {
	On     bool  `json:"on"`
	Aff    bool  `json:"aff"`
	Timer  bool  `json:"timer"`
	Ram    bool  `json:"ram"`
	Power  bool  `json:"power"`
	Hotkey *bool `json:"hotkey,omitempty"`
}

type gmStatus struct {
	Active    bool           `json:"active"`
	Game      string         `json:"game,omitempty"`
	Since     int64          `json:"since,omitempty"`
	Aff       string         `json:"aff"`
	AffOK     bool           `json:"affOk"`
	Timer     string         `json:"timer"`
	TimerOK   bool           `json:"timerOk"`
	Ram       string         `json:"ram"`
	RamOK     bool           `json:"ramOk"`
	Purges    int            `json:"purges"`
	FreedMB   float64        `json:"freedMB"`
	Power     string         `json:"power"`
	PowerOK   bool           `json:"powerOk"`
	GPU       float64        `json:"gpu"`
	CPU       float64        `json:"cpu"`
	CPUMax    float64        `json:"cpuMax"`
	Verdict   string         `json:"verdict"`
	VerdictID string         `json:"vid"`
	Samples   int            `json:"samples"`
	Recover   *recoverResult `json:"rec,omitempty"`
	Hotkey    string         `json:"hk,omitempty"`
}

func (a *app) gmConfig() gmSettings {
	var cfg struct {
		GM *gmSettings `json:"gm"`
	}
	json.Unmarshal([]byte(a.loadSettings()), &cfg)
	if cfg.GM == nil {
		return gmSettings{On: true, Aff: true, Timer: true, Ram: true, Power: true}
	}
	return *cfg.GM
}

// ---- แกนซีพียู ----

// perfCoreMask คืน mask ของ logical processor ที่อยู่บนแกนตัวแรง (EfficiencyClass สูงสุด)
// ถ้าซีพียูมีแกนแบบเดียว คืน 0
func perfCoreMask() (mask uintptr, pCores, threads int) {
	var n uint32
	pGetLPIEx.Call(0, 0, uintptr(unsafe.Pointer(&n))) // RelationProcessorCore = 0
	if n == 0 {
		return 0, 0, 0
	}
	buf := make([]byte, n)
	if r, _, _ := pGetLPIEx.Call(0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))); r == 0 {
		return 0, 0, 0
	}
	type core struct {
		eff  byte
		mask uintptr
	}
	var cores []core
	classes := map[byte]bool{}
	for off := uint32(0); off+42 <= n; {
		size := *(*uint32)(unsafe.Pointer(&buf[off+4]))
		if size == 0 {
			break
		}
		eff := buf[off+9]
		group := *(*uint16)(unsafe.Pointer(&buf[off+40]))
		if group == 0 {
			cores = append(cores, core{eff, *(*uintptr)(unsafe.Pointer(&buf[off+32]))})
			classes[eff] = true
		}
		off += size
	}
	if len(classes) < 2 {
		return 0, 0, 0
	}
	var top byte
	for c := range classes {
		if c > top {
			top = c
		}
	}
	for _, c := range cores {
		if c.eff == top {
			mask |= c.mask
			pCores++
		}
	}
	for m := mask; m != 0; m &= m - 1 {
		threads++
	}
	return
}

func setAffinity(pid uint32, mask uintptr) error {
	h, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION|windows.PROCESS_QUERY_INFORMATION, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	var pm, sm uintptr
	pGetAffinity.Call(uintptr(h), uintptr(unsafe.Pointer(&pm)), uintptr(unsafe.Pointer(&sm)))
	if pm == mask&sm {
		return nil
	}
	if r, _, e := pSetAffinity.Call(uintptr(h), mask&sm); r == 0 {
		return e
	}
	return nil
}

// ---- จังหวะนาฬิกา ----

func winBuild() uint32 { return windows.RtlGetVersion().BuildNumber }

func timerSupported() bool {
	if winBuild() < 22000 {
		return false
	}
	// Windows 11: ต้องเปิดค่านี้ก่อน การปรับจังหวะนาฬิกาจากโปรแกรมนี้ถึงจะมีผลกับเกม (มีผลหลังรีสตาร์ทครั้งแรก)
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\kernel`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err == nil {
		if v, _, e := k.GetIntegerValue("GlobalTimerResolutionRequests"); e != nil || v != 1 {
			k.SetDWordValue("GlobalTimerResolutionRequests", 1)
		}
		k.Close()
	}
	return true
}

func setTimerRes(on bool) float64 {
	var cur uint32
	set := uintptr(0)
	if on {
		set = 1
	}
	pNtSetTimerRes.Call(5000, set, uintptr(unsafe.Pointer(&cur))) // 5000 x 100ns = 0.5 ms
	return float64(cur) / 10000
}

// ---- แรม ----

func freeMB() float64 {
	var info [22]uintptr
	var ret uint32
	if r, _, _ := pNtQuerySysInfo.Call(80, uintptr(unsafe.Pointer(&info[0])), unsafe.Sizeof(info), uintptr(unsafe.Pointer(&ret))); r != 0 {
		return -1
	}
	return float64(info[0]+info[1]) * 4096 / (1 << 20) // Zero + Free pages
}

func purgeStandby() bool {
	if enablePrivilege("SeProfileSingleProcessPrivilege") != nil {
		return false
	}
	c := uint32(4)
	r, _, _ := pNtSetSysInfo.Call(80, uintptr(unsafe.Pointer(&c)), 4)
	return r == 0
}

// ---- แผนพลังงาน ----

func activeScheme() (windows.GUID, bool) {
	var p *windows.GUID
	if r, _, _ := pPowerGetActive.Call(0, uintptr(unsafe.Pointer(&p))); r != 0 || p == nil {
		return windows.GUID{}, false
	}
	g := *p
	windows.LocalFree(windows.Handle(unsafe.Pointer(p)))
	return g, true
}

func setScheme(g windows.GUID) bool {
	r, _, _ := pPowerSetActive.Call(0, uintptr(unsafe.Pointer(&g)))
	return r == 0
}

var (
	pPowerWriteAC    = powrprof.NewProc("PowerWriteACValueIndex")
	pPowerWriteDC    = powrprof.NewProc("PowerWriteDCValueIndex")
	guidVideoSub, _  = windows.GUIDFromString("{7516b95f-f776-4464-8c53-06167f40cc99}")
	guidVideoIdle, _ = windows.GUIDFromString("{3c0bc021-c8a8-4e07-a973-6b14cbcb2b7e}")
)

// displayNever ตั้งปิดหน้าจอ = Never ในแผน ZAN (ทั้งเสียบปลั๊กและแบต)
func displayNever() {
	g := zanSchemeGUID
	pPowerWriteAC.Call(0, uintptr(unsafe.Pointer(&g)), uintptr(unsafe.Pointer(&guidVideoSub)), uintptr(unsafe.Pointer(&guidVideoIdle)), 0)
	pPowerWriteDC.Call(0, uintptr(unsafe.Pointer(&g)), uintptr(unsafe.Pointer(&guidVideoSub)), uintptr(unsafe.Pointer(&guidVideoIdle)), 0)
}

func (a *app) ensureZanScheme() bool {
	if setScheme(zanSchemeGUID) {
		displayNever()
		setScheme(zanSchemeGUID) // ให้ค่าที่เพิ่งเขียนมีผลทันที
		return true
	}
	pow := filepath.Join(a.dataDir, "ZAN_Project_Optimizer.pow")
	c := exec.Command("powercfg", "/import", pow, strings.Trim(zanSchemeGUID.String(), "{}"))
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	c.Run()
	if !setScheme(zanSchemeGUID) {
		return false
	}
	displayNever()
	return setScheme(zanSchemeGUID)
}

// ---- ตัวถ่วง (PDH) ----

type pdhQuery struct {
	q, gpu, cpu, perf, gmem, amem uintptr
}

func openPdh() *pdhQuery {
	p := &pdhQuery{}
	if r, _, _ := pPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&p.q))); r != 0 {
		return nil
	}
	add := func(path string, out *uintptr) {
		s, _ := windows.UTF16PtrFromString(path)
		pPdhAddEnglish.Call(p.q, uintptr(unsafe.Pointer(s)), 0, uintptr(unsafe.Pointer(out)))
	}
	add(`\GPU Engine(*engtype_3D)\Utilization Percentage`, &p.gpu)
	add(`\Processor(*)\% Processor Time`, &p.cpu)
	add(`\Processor Information(_Total)\% Processor Performance`, &p.perf)
	add(`\GPU Process Memory(*)\Dedicated Usage`, &p.gmem)
	add(`\GPU Adapter Memory(*)\Dedicated Usage`, &p.amem)
	pPdhCollect.Call(p.q)
	return p
}

func (p *pdhQuery) close() { pPdhCloseQuery.Call(p.q) }

func pdhArray(counter uintptr) map[string]float64 {
	if counter == 0 {
		return nil
	}
	const fmtDouble, fmtNoCap = 0x200, 0x8000
	var size, count uint32
	r, _, _ := pPdhGetArray.Call(counter, fmtDouble|fmtNoCap, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if uint32(r) != 0x800007D2 || size == 0 { // PDH_MORE_DATA
		return nil
	}
	buf := make([]byte, size)
	if r, _, _ = pPdhGetArray.Call(counter, fmtDouble|fmtNoCap, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0]))); r != 0 {
		return nil
	}
	out := map[string]float64{}
	for i := 0; i < int(count) && (i+1)*24 <= len(buf); i++ {
		off := i * 24
		name := windows.UTF16PtrToString(*(**uint16)(unsafe.Pointer(&buf[off])))
		if st := *(*uint32)(unsafe.Pointer(&buf[off+8])); st > 1 {
			continue
		}
		out[name] += *(*float64)(unsafe.Pointer(&buf[off+16]))
	}
	return out
}

// sample คืน % การ์ดจอ (รวมงาน 3D ทั้งหมด), % ซีพียูเฉลี่ย, % แกนที่หนักที่สุด
func (p *pdhQuery) sample() (gpu, cpu, cpuMax float64, ok bool) {
	if r, _, _ := pPdhCollect.Call(p.q); r != 0 {
		return
	}
	g := pdhArray(p.gpu)
	for _, v := range g {
		gpu += v
	}
	if gpu > 100 {
		gpu = 100
	}
	c := pdhArray(p.cpu)
	if len(c) == 0 {
		return
	}
	for k, v := range c {
		if k == "_Total" {
			cpu = v
		} else if v > cpuMax {
			cpuMax = v
		}
	}
	if cpuMax > 100 {
		cpuMax = 100
	}
	return gpu, cpu, cpuMax, true
}

// extra อ่านค่าจากรอบ collect ล่าสุด: % ความเร็วซีพียู, VRAM ของเกม (MB), VRAM ทั้งเครื่อง (MB)
func (p *pdhQuery) extra(pid uint32) (perf, vramGame, vramAll float64) {
	for _, v := range pdhArray(p.perf) {
		perf = v
	}
	pre := fmt.Sprintf("pid_%d_", pid)
	for k, v := range pdhArray(p.gmem) {
		if strings.HasPrefix(k, pre) {
			vramGame += v
		}
	}
	for _, v := range pdhArray(p.amem) {
		vramAll += v
	}
	return perf, vramGame / (1 << 20), vramAll / (1 << 20)
}

func verdict(gpu, cpuMax float64) (string, string) {
	switch {
	case gpu >= 90:
		return "gpu", fmt.Sprintf("การ์ดจอทำงานเต็มที่ (%.0f%%) อยากได้เฟรมเพิ่มให้ลดกราฟิก เช่น เงา, MSAA, Post FX หรือลดความละเอียดจอ", gpu)
	case cpuMax >= 85 && gpu < 75:
		return "cpu", fmt.Sprintf("ซีพียูไม่ทัน (แกนที่หนักสุด %.0f%%, การ์ดจอแค่ %.0f%%) ให้ลดระยะมองเห็น, จำนวนคนและรถในเกม ลดกราฟิกไม่ค่อยช่วย", cpuMax, gpu)
	case gpu < 60 && cpuMax < 70:
		return "cap", "เครื่องยังเหลือแรง ถ้าเฟรมยังต่ำ น่าจะติดล็อกเฟรม/VSync หรือสคริปต์ของเซิร์ฟเอง"
	default:
		return "ok", "สมดุลดี ซีพียูกับการ์ดจอทำงานพอ ๆ กัน"
	}
}

// ---- ตัวคุมหลัก ----

// gameWatcher ทำงานใน process เบื้องหลัง (--gamemode) ไม่มีหน้าต่าง เขียนสถานะลงไฟล์ให้หน้าโปรแกรมอ่าน
func (a *app) gameWatcher(publish func(string), quit func() bool, idleExit func(idle time.Duration) bool, wait func() bool) {
	var (
		st            gmStatus
		pid           uint32
		prevScheme    windows.GUID
		havePrev      bool
		timerOn       bool
		lastPurge     time.Time
		pq            *pdhQuery
		pqAge         int
		gpuS, cpuS    []float64
		lastEmit      string
		lastRec       *recoverResult
		gameSince     time.Time
		seenMarked    bool
		pMask, pc, th = perfCoreMask()
	)
	stop := func() {
		if timerOn {
			setTimerRes(false)
			timerOn = false
		}
		if havePrev {
			if cur, ok := activeScheme(); ok && cur == zanSchemeGUID {
				setScheme(prevScheme)
			}
			havePrev = false
		}
		if pq != nil {
			pq.close()
			pq = nil
		}
		pid, gpuS, cpuS = 0, nil, nil
		st = gmStatus{}
	}
	lastGame := time.Now()
	for {
		doRecover := wait() // รอ 3 วินาที หรือจนมีคนกดกู้ FPS (ปุ่มในโปรแกรม / ปุ่มลัด)
		if quit() {
			stop()
			publish(`{"active":false}`)
			return
		}
		a.boostPriority()
		gameModeStateLock.Lock()
		cfg := a.gmConfig()
		gp, name := gtaProcess()
		if gp != 0 {
			lastGame = time.Now()
		} else if idleExit(time.Since(lastGame)) {
			stop()
			gameModeStateLock.Unlock()
			publish(`{"active":false}`)
			return
		}
		if doRecover {
			r := a.softRecover(gp, pMask, cfg, pq)
			lastRec = &r
		}
		if gp == 0 || !cfg.On {
			if pid != 0 {
				stop()
			}
		} else {
			if gp == pid && !seenMarked && time.Since(gameSince) > 30*time.Second {
				// เกมเปิดมาได้แล้ว = ไฟล์ของ build นี้ครบ จดไว้ให้หน้าเซิร์ฟขึ้น ✓
				seenMarked = true
				var bs []int
				if b, _, ok := runningLaunchArgs(); ok {
					bs = append(bs, b)
				}
				if m := regexp.MustCompile(`(?i)_b(\d+)_`).FindStringSubmatch(name); m != nil {
					bs = append(bs, atoi(m[1]))
				}
				a.markBuildSeen(bs...)
			}
			if gp != pid { // เกมเพิ่งเปิด หรือ FiveM รีเกมเพื่อสลับ build
				if pid == 0 {
					st = gmStatus{Active: true, Since: time.Now().Unix()}
					if cfg.Power {
						if cur, ok := activeScheme(); ok && cur != zanSchemeGUID {
							prevScheme, havePrev = cur, true
						}
						if a.ensureZanScheme() {
							st.Power, st.PowerOK = "ใช้แผน ZAN_Project_Optimizer ระหว่างเล่น", true
						} else {
							st.Power, havePrev = "สลับแผนพลังงานไม่ได้", false
						}
					} else {
						st.Power = "ปิดอยู่"
					}
					if cfg.Timer && timerSupported() {
						cur := setTimerRes(true)
						timerOn = true
						st.Timer, st.TimerOK = fmt.Sprintf("ตั้งเป็น %.1f ms ระหว่างเล่น", cur), true
					} else if cfg.Timer {
						st.Timer = "Windows 10 ไม่ยอมให้โปรแกรมอื่นปรับให้เกม (ต้อง Windows 11)"
					} else {
						st.Timer = "ปิดอยู่"
					}
					pq, pqAge = openPdh(), 0
				}
				pid = gp
				gameSince, seenMarked = time.Now(), false
				st.Game = name
				if cfg.Aff && pMask != 0 {
					if err := setAffinity(gp, pMask); err == nil {
						st.Aff, st.AffOK = fmt.Sprintf("เกมใช้แกนตัวแรง %d แกน (%d เธรด)", pc, th), true
					} else {
						st.Aff = "ตั้งแกนไม่ได้: " + err.Error()
					}
				} else if cfg.Aff {
					st.Aff = "ซีพียูนี้มีแกนแบบเดียว ไม่ต้องตั้ง"
				} else {
					st.Aff = "ปิดอยู่"
				}
			} else if cfg.Aff && pMask != 0 && st.AffOK {
				setAffinity(gp, pMask) // เผื่อมีอะไรมาเปลี่ยนกลับ
			}
			// แรม
			if cfg.Ram {
				st.RamOK = true
				f, sb := freeMB(), standbyMB()
				if f >= 0 && f < 1024 && sb > 1024 && time.Since(lastPurge) > 60*time.Second {
					lastPurge = time.Now()
					if purgeStandby() {
						st.Purges++
						if after := standbyMB(); after >= 0 && sb > after {
							st.FreedMB += sb - after
						}
					}
				}
				if st.Purges > 0 {
					st.Ram = fmt.Sprintf("ล้างไปแล้ว %d ครั้ง คืนแรม %.0f MB", st.Purges, st.FreedMB)
				} else {
					st.Ram = fmt.Sprintf("คอยดูอยู่ แรมว่าง %.0f MB", f)
				}
			} else {
				st.Ram, st.RamOK = "ปิดอยู่", false
			}
			// ตัวถ่วง: เฉลี่ย 30 วินาทีล่าสุด
			if pq != nil {
				if g, c, cm, ok := pq.sample(); ok {
					gpuS, cpuS = append(gpuS, g), append(cpuS, cm)
					if len(gpuS) > 10 {
						gpuS, cpuS = gpuS[1:], cpuS[1:]
					}
					st.CPU = c
				}
				if pqAge++; pqAge >= 20 { // เปิดใหม่เป็นระยะ ให้เห็นงานการ์ดจอของตัวเกมที่เพิ่งเริ่ม
					pq.close()
					pq, pqAge = openPdh(), 0
				}
			}
			st.Samples = len(gpuS)
			if len(gpuS) > 0 {
				st.GPU, st.CPUMax = median(gpuS), median(cpuS)
			}
			if len(gpuS) >= 4 {
				st.VerdictID, st.Verdict = verdict(st.GPU, st.CPUMax)
			} else {
				st.VerdictID, st.Verdict = "wait", "กำลังเก็บข้อมูล..."
			}
		}
		st.Recover = lastRec
		st.Hotkey, _ = hotkeyState.Load().(string)
		b, _ := json.Marshal(st)
		gameModeStateLock.Unlock()
		if s := string(b); s != lastEmit {
			lastEmit = s
			publish(s)
		}
	}
}

func median(v []float64) float64 {
	c := append([]float64(nil), v...)
	sort.Float64s(c)
	return c[len(c)/2]
}

// gameModeInfo บอก UI ว่าเครื่องนี้รองรับอะไรบ้าง
func (a *app) gameModeInfo() map[string]any {
	_, pc, th := perfCoreMask()
	return map[string]any{"pCores": pc, "pThreads": th, "timer": winBuild() >= 22000, "build": winBuild()}
}

// ---- process เบื้องหลัง ----

const (
	gmMutexName   = `Local\ZanBoosterGM`
	gmQuitName    = `Local\ZanBoosterGMQuit`
	uiMutexName   = `Local\ZanBoosterUI`
	gmRecoverName = `Local\ZanBoosterRecover`
)

func mutexExists(name string) bool {
	p, _ := windows.UTF16PtrFromString(name)
	h, err := windows.OpenMutex(windows.SYNCHRONIZE, false, p)
	if err != nil {
		return false
	}
	windows.CloseHandle(h)
	return true
}

func (a *app) statusPath() string { return filepath.Join(a.dataDir, "gm_status.json") }

// runGameModeHelper = ZanBooster.exe --gamemode
// อยู่ต่อได้แม้ปิดหน้าโปรแกรม ปิดตัวเองเมื่อไม่มีเกมเปิดนาน 15 นาทีและหน้าโปรแกรมไม่ได้เปิด
func runGameModeHelper() {
	a := &app{dataDir: filepath.Join(os.Getenv("ProgramData"), "ZanBooster")}
	mn, _ := windows.UTF16PtrFromString(gmMutexName)
	m, err := windows.CreateMutex(nil, false, mn)
	if err != nil { // มีตัวทำงานอยู่แล้ว
		return
	}
	defer windows.CloseHandle(m)
	qn, _ := windows.UTF16PtrFromString(gmQuitName)
	q, _ := windows.CreateEvent(nil, 1, 0, qn)
	windows.ResetEvent(q)
	defer windows.CloseHandle(q)
	publish := func(s string) {
		// แนบเวอร์ชันไว้ ให้หน้าโปรแกรมรู้ว่าต้องเปลี่ยนตัวเบื้องหลังหลังอัปเดตไหม
		var m map[string]any
		if json.Unmarshal([]byte(s), &m) != nil {
			return
		}
		m["ver"], m["pid"] = appVersion, os.Getpid()
		b, _ := json.Marshal(m)
		tmp := a.statusPath() + ".tmp"
		if os.WriteFile(tmp, b, 0o644) == nil {
			os.Rename(tmp, a.statusPath())
		}
	}
	rn, _ := windows.UTF16PtrFromString(gmRecoverName)
	rv, _ := windows.CreateEvent(nil, 0, 0, rn) // auto-reset
	defer windows.CloseHandle(rv)
	go a.hotkeyLoop(rv)
	publish(`{"active":false}`)
	a.gameWatcher(publish,
		func() bool { r, _ := windows.WaitForSingleObject(q, 0); return r == windows.WAIT_OBJECT_0 },
		func(idle time.Duration) bool { return idle > 15*time.Minute && !mutexExists(uiMutexName) },
		func() bool { r, _ := windows.WaitForSingleObject(rv, 3000); return r == windows.WAIT_OBJECT_0 })
}

// ensureGameModeHelper เปิดตัวเบื้องหลังถ้ายังไม่มี / เปลี่ยนตัวใหม่ถ้าเป็นเวอร์ชันเก่า
func (a *app) ensureGameModeHelper() {
	if mutexExists(gmMutexName) {
		var st struct {
			Ver string `json:"ver"`
		}
		if b, err := os.ReadFile(a.statusPath()); err == nil {
			json.Unmarshal(b, &st)
		}
		if st.Ver == appVersion {
			return
		}
		qn, _ := windows.UTF16PtrFromString(gmQuitName)
		if q, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, qn); err == nil {
			windows.SetEvent(q)
			windows.CloseHandle(q)
		}
		for i := 0; i < 40 && mutexExists(gmMutexName); i++ {
			time.Sleep(250 * time.Millisecond)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	c := exec.Command(exe, "--gamemode")
	if c.Start() == nil {
		c.Process.Release()
	}
}

// gameModeRelay อ่านสถานะจากตัวเบื้องหลังแล้วส่งให้หน้าโปรแกรม และเปิดตัวเบื้องหลังใหม่ถ้าหายไป
func (a *app) gameModeRelay() {
	last := ""
	for i := 0; ; i++ {
		if i%10 == 5 && !mutexExists(gmMutexName) {
			a.ensureGameModeHelper()
		}
		if b, err := os.ReadFile(a.statusPath()); err == nil {
			if s := string(b); s != last && json.Valid(b) {
				last = s
				a.emit("onGameMode", s)
			}
		}
		time.Sleep(2 * time.Second)
	}
}
