package main

// บันทึกการเล่นยาว: ตัวเบื้องหลังเก็บค่าทุก 30 วินาทีระหว่างเกมเปิด แล้ววิเคราะห์ว่าทำไมเล่นนาน ๆ แล้ว FPS ตก
// ไม่แตะตัวเกม อ่านจากตัวนับของ Windows (PDH), ข้อมูลหน่วยความจำของ process และ nvidia-smi (ถ้ามี)

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var pGetProcMemInfo = kernel32.NewProc("K32GetProcessMemoryInfo")

type ssample struct {
	T        float64 `json:"t"`  // นาทีตั้งแต่เริ่ม
	GPU      float64 `json:"g"`  // % การ์ดจอ
	CPUMax   float64 `json:"cm"` // % แกนที่หนักสุด
	CPU      float64 `json:"c"`  // % ซีพียูรวม
	Perf     float64 `json:"p"`  // % ความเร็วซีพียูเทียบความเร็วปกติ
	GameMB   float64 `json:"m"`  // แรมที่เกมใช้
	VramMB   float64 `json:"v"`  // VRAM ที่เกมใช้
	VramAll  float64 `json:"va"` // VRAM ที่ใช้ทั้งเครื่อง
	FreeMB   float64 `json:"f"`  // แรมว่างของเครื่อง
	Temp     float64 `json:"tp,omitempty"`
	Clock    float64 `json:"ck,omitempty"`
	ClockMax float64 `json:"cx,omitempty"`
}

type session struct {
	Start   int64     `json:"start"`
	Game    string    `json:"game"`
	VramCap float64   `json:"vramCap"`
	Samples []ssample `json:"samples"`
}

type recorder struct {
	dir   string
	file  string
	s     session
	t0    time.Time
	ticks int
	smi   string

	perf, vg, va float64 // ค่าล่าสุดจาก PDH
}

func gpuVramCapMB() float64 {
	const cls = `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, cls, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return 0
	}
	defer k.Close()
	names, _ := k.ReadSubKeyNames(-1)
	var best float64
	for _, n := range names {
		sk, err := registry.OpenKey(k, n, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		if v, _, err := sk.GetIntegerValue("HardwareInformation.qwMemorySize"); err == nil {
			best = max(best, float64(v)/(1<<20))
		} else if b, _, err := sk.GetBinaryValue("HardwareInformation.qwMemorySize"); err == nil && len(b) >= 8 {
			best = max(best, float64(*(*uint64)(unsafe.Pointer(&b[0])))/(1<<20))
		} else if b, _, err := sk.GetBinaryValue("HardwareInformation.MemorySize"); err == nil && len(b) >= 4 {
			best = max(best, float64(*(*uint32)(unsafe.Pointer(&b[0])))/(1<<20))
		}
		sk.Close()
	}
	return best
}

func findNvidiaSmi() string {
	for _, p := range []string{
		filepath.Join(os.Getenv("SystemRoot"), "System32", "nvidia-smi.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "NVIDIA Corporation", "NVSMI", "nvidia-smi.exe"),
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func nvidiaStats(smi string) (temp, clock, clockMax float64) {
	if smi == "" {
		return
	}
	c := exec.Command(smi, "--query-gpu=temperature.gpu,clocks.gr,clocks.max.gr", "--format=csv,noheader,nounits")
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := c.Output()
	if err != nil {
		return
	}
	f := strings.Split(strings.Split(strings.TrimSpace(string(out)), "\n")[0], ",")
	if len(f) >= 3 {
		temp, _ = strconv.ParseFloat(strings.TrimSpace(f[0]), 64)
		clock, _ = strconv.ParseFloat(strings.TrimSpace(f[1]), 64)
		clockMax, _ = strconv.ParseFloat(strings.TrimSpace(f[2]), 64)
	}
	return
}

func processPrivateMB(pid uint32) float64 {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(h)
	var pmc [10]uintptr // PROCESS_MEMORY_COUNTERS_EX (cb + PageFaultCount รวมเป็นช่องแรก)
	if r, _, _ := pGetProcMemInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&pmc[0])), 80); r == 0 {
		return 0
	}
	return float64(pmc[9]) / (1 << 20) // PrivateUsage
}

func newRecorder(dataDir, game string) *recorder {
	dir := filepath.Join(dataDir, "sessions")
	os.MkdirAll(dir, 0o755)
	// เก็บไว้ 10 รอบล่าสุด
	if es, err := os.ReadDir(dir); err == nil && len(es) >= 10 {
		sort.Slice(es, func(i, j int) bool { return es[i].Name() < es[j].Name() })
		for _, e := range es[:len(es)-9] {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	now := time.Now()
	return &recorder{
		dir: dir, t0: now, smi: findNvidiaSmi(),
		file: filepath.Join(dir, now.Format("20060102_150405")+".json"),
		s:    session{Start: now.Unix(), Game: game, VramCap: gpuVramCapMB()},
	}
}

// tick เรียกทุก 3 วินาที เก็บจริงทุก 30 วินาที
func (r *recorder) tick(pid uint32, gpu, cpuMax, cpu float64) {
	r.ticks++
	if r.ticks%10 != 0 {
		return
	}
	s := ssample{T: time.Since(r.t0).Minutes(), GPU: gpu, CPUMax: cpuMax, CPU: cpu, GameMB: processPrivateMB(pid), FreeMB: freeMB()}
	s.Perf, s.VramMB, s.VramAll = r.perf, r.vg, r.va
	s.Temp, s.Clock, s.ClockMax = nvidiaStats(r.smi)
	r.s.Samples = append(r.s.Samples, s)
	if b, err := json.Marshal(r.s); err == nil {
		tmp := r.file + ".tmp"
		if os.WriteFile(tmp, b, 0o644) == nil {
			os.Rename(tmp, r.file)
		}
	}
}

// ---- อ่านให้หน้าโปรแกรม ----

type sessionItem struct {
	File    string  `json:"file"`
	Start   int64   `json:"start"`
	Minutes float64 `json:"min"`
	Game    string  `json:"game"`
}

type finding struct {
	Level  string `json:"lv"` // bad / warn / info / ok
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Fix    string `json:"fix"`
}

type sessionReport struct {
	OK       bool      `json:"ok"`
	Message  string    `json:"m,omitempty"`
	Session  session   `json:"s"`
	Findings []finding `json:"findings"`
	HasTemp  bool      `json:"hasTemp"`
}

func (a *app) sessionList() []sessionItem {
	dir := filepath.Join(a.dataDir, "sessions")
	es, _ := os.ReadDir(dir)
	out := []sessionItem{}
	for i := len(es) - 1; i >= 0; i-- {
		n := es[i].Name()
		if !strings.HasSuffix(n, ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			continue
		}
		var s session
		if json.Unmarshal(b, &s) != nil || len(s.Samples) == 0 {
			continue
		}
		out = append(out, sessionItem{File: n, Start: s.Start, Minutes: s.Samples[len(s.Samples)-1].T, Game: s.Game})
	}
	return out
}

func (a *app) sessionLoad(file string) sessionReport {
	if strings.ContainsAny(file, `\/`) || !strings.HasSuffix(file, ".json") {
		return sessionReport{Message: "ชื่อไฟล์ไม่ถูกต้อง"}
	}
	b, err := os.ReadFile(filepath.Join(a.dataDir, "sessions", file))
	if err != nil {
		return sessionReport{Message: "เปิดไฟล์ไม่ได้"}
	}
	var s session
	if json.Unmarshal(b, &s) != nil {
		return sessionReport{Message: "ไฟล์เสีย"}
	}
	r := sessionReport{OK: true, Session: s, Findings: analyzeSession(s)}
	for _, x := range s.Samples {
		if x.Temp > 0 {
			r.HasTemp = true
			break
		}
	}
	return r
}

func avgOf(ss []ssample, f func(ssample) float64) float64 {
	var t, n float64
	for _, s := range ss {
		if v := f(s); v > 0 {
			t += v
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return t / n
}

func analyzeSession(s session) []finding {
	ss := s.Samples
	if len(ss) == 0 || ss[len(ss)-1].T < 20 {
		return []finding{{Level: "info", Title: "ข้อมูลยังน้อยไป", Detail: "ต้องเล่นต่อเนื่องอย่างน้อย 20 นาที ถึงจะเทียบช่วงต้นกับช่วงท้ายได้", Fix: "เปิดเกมเล่นตามปกติจนถึงช่วงที่ FPS ตก แล้วกลับมาดูหน้านี้"}}
	}
	end := ss[len(ss)-1].T
	var base, last []ssample
	for _, x := range ss {
		if x.T >= 5 && x.T <= 15 {
			base = append(base, x)
		}
		if x.T >= end-10 {
			last = append(last, x)
		}
	}
	if len(base) == 0 {
		base = ss[:min(len(ss), 10)]
	}
	b := func(f func(ssample) float64) float64 { return avgOf(base, f) }
	l := func(f func(ssample) float64) float64 { return avgOf(last, f) }
	gb := func(mb float64) string { return fmt.Sprintf("%.1f GB", mb/1024) }
	var out []finding

	// 1) VRAM เต็ม
	vAllL, vGameB, vGameL := l(func(x ssample) float64 { return x.VramAll }), b(func(x ssample) float64 { return x.VramMB }), l(func(x ssample) float64 { return x.VramMB })
	if s.VramCap > 0 && vAllL > 0 {
		pct := vAllL / s.VramCap
		switch {
		case pct >= 0.92:
			out = append(out, finding{"bad", "หน่วยความจำการ์ดจอ (VRAM) เต็ม",
				fmt.Sprintf("ช่วงท้ายใช้ %s จาก %s (%.0f%%) พอเต็ม เกมต้องดึงภาพจากแรมเครื่องแทน FPS เลยตกและกระตุก", gb(vAllL), gb(s.VramCap), pct*100),
				"ลด Texture Quality ลง 1 ขั้น และเลื่อน Extended Texture Budget ไปซ้ายสุด ช่วยมากที่สุด หรือใช้กราฟิกแบบ \"สมดุล\" ใน Zan Booster"})
		case pct >= 0.85 && vGameL-vGameB > 1024:
			out = append(out, finding{"warn", "VRAM ใกล้เต็ม และเพิ่มขึ้นเรื่อย ๆ",
				fmt.Sprintf("เกมใช้ VRAM เพิ่มจาก %s เป็น %s (ทั้งเครื่อง %.0f%%)", gb(vGameB), gb(vGameL), pct*100),
				"ลด Texture Quality ลง 1 ขั้น ถ้ายังตกอยู่"})
		}
	}
	// 2) แรมของเกมโตขึ้นเรื่อย ๆ
	mB, mL := b(func(x ssample) float64 { return x.GameMB }), l(func(x ssample) float64 { return x.GameMB })
	if mB > 0 && mL-mB >= 2048 && mL >= mB*1.35 {
		out = append(out, finding{"bad", "แรมที่เกมใช้โตขึ้นเรื่อย ๆ",
			fmt.Sprintf("จาก %s ช่วงต้น เป็น %s ช่วงท้าย อาการนี้มักมาจากสคริปต์ของเซิร์ฟที่คืนหน่วยความจำไม่หมด", gb(mB), gb(mL)),
			"ออกแล้วเข้าเซิร์ฟใหม่ทุก 1–1.5 ชม. (กด \"เข้า\" ใน Zan Booster ตอนเกมเปิดอยู่ ไม่ต้องรีเกม) ถ้ายังไม่หาย ให้ปิดเกมแล้วเปิดใหม่"})
	}
	// 3) ซีพียูลดความเร็ว
	pB, pL := b(func(x ssample) float64 { return x.Perf }), l(func(x ssample) float64 { return x.Perf })
	cB, cL := b(func(x ssample) float64 { return x.CPU }), l(func(x ssample) float64 { return x.CPU })
	if pB > 0 && pL > 0 && pL < pB*0.85 && cL >= cB*0.8 {
		out = append(out, finding{"bad", "ซีพียูลดความเร็วตัวเองลง",
			fmt.Sprintf("ความเร็วช่วงท้ายเหลือ %.0f%% ของช่วงต้น ทั้งที่งานยังหนักเท่าเดิม มักเป็นเพราะร้อนเกิน", pL/pB*100),
			"ทำความสะอาดพัดลมและซิงค์ซีพียู เปลี่ยนซิลิโคน และเช็กว่าในเคสมีลมเข้าออก"})
	}
	// 4) การ์ดจอร้อน / ลดความเร็ว
	tL := l(func(x ssample) float64 { return x.Temp })
	kB, kL := b(func(x ssample) float64 { return x.Clock }), l(func(x ssample) float64 { return x.Clock })
	gB, gL := b(func(x ssample) float64 { return x.GPU }), l(func(x ssample) float64 { return x.GPU })
	if tL >= 83 {
		out = append(out, finding{"bad", fmt.Sprintf("การ์ดจอร้อน %.0f°C", tL),
			"ช่วงท้ายการ์ดจอร้อนจนเริ่มลดความเร็วตัวเอง",
			"ทำความสะอาดพัดลมการ์ดจอ ปรับให้พัดลมหมุนแรงขึ้น หรือล็อก FPS ไม่ให้การ์ดจอทำงาน 100% ตลอด"})
	} else if kB > 0 && kL > 0 && kL < kB*0.85 && gL >= 85 {
		out = append(out, finding{"warn", "การ์ดจอลดความเร็ว",
			fmt.Sprintf("ความเร็วการ์ดจอลดจาก %.0f เป็น %.0f MHz ทั้งที่ยังทำงานหนัก", kB, kL),
			"เช็กความร้อนและพัดลมการ์ดจอ"})
	}
	// 5) แรมเครื่องใกล้หมด
	minFree := 1e9
	for _, x := range last {
		if x.FreeMB > 0 && x.FreeMB < minFree {
			minFree = x.FreeMB
		}
	}
	if minFree < 700 {
		out = append(out, finding{"warn", "แรมว่างของเครื่องเหลือน้อย",
			fmt.Sprintf("ช่วงท้ายแรมว่างเหลือต่ำสุด %.0f MB", minFree),
			"ปิด Chrome, Discord หรือโปรแกรมอื่นที่เปิดค้างระหว่างเล่น และเปิด \"ล้างแรมอัตโนมัติ\" ในโหมดเล่นเกม"})
	}
	// 6) ภาระย้ายไปซีพียู
	cmB, cmL := b(func(x ssample) float64 { return x.CPUMax }), l(func(x ssample) float64 { return x.CPUMax })
	if cmL-cmB >= 15 && gL < gB-15 {
		out = append(out, finding{"info", "ซีพียูหนักขึ้นช่วงท้าย",
			fmt.Sprintf("แกนที่หนักสุดขึ้นจาก %.0f%% เป็น %.0f%% ส่วนการ์ดจอทำงานลดลง แปลว่าเกมรอซีพียู", cmB, cmL),
			"ช่วงคนหรือรถรอบตัวเยอะ ให้ลดระยะมองเห็น จำนวนคนและรถในเกม"})
	}
	if len(out) == 0 {
		out = append(out, finding{"ok", "ฝั่งเครื่องปกติดี",
			"VRAM, แรม, ความร้อน และความเร็วซีพียูไม่ได้เปลี่ยนแบบผิดปกติ",
			"ถ้า FPS ยังตก น่าจะมาจากฝั่งเซิร์ฟ เช่น คนเยอะขึ้นในพื้นที่ หรือสคริปต์เซิร์ฟ ลองเช็กด้วย F8 พิมพ์ resmon 1"})
	}
	return out
}
