package main

// วัดผล FPS ด้วย PresentMon (Intel, MIT license) อ่านเวลาเฟรมจริงจาก ETW
// ได้ค่า FPS เฉลี่ย, 1% low, สัดส่วนการ์ดจอทำงาน และ display latency

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed presentmon.bin
var presentMonExe []byte

type benchRun struct {
	At      int64   `json:"at"`
	Label   string  `json:"label"`
	Secs    int     `json:"secs"`
	Frames  int     `json:"frames"`
	Avg     float64 `json:"avg"`
	Low1    float64 `json:"low1"`
	Low01   float64 `json:"low01"`
	GpuPct  float64 `json:"gpuPct"`
	Latency float64 `json:"latency"`
	CpuPct  float64 `json:"cpuPct"`
	CoreMax float64 `json:"coreMax"`
	Game    string  `json:"game"`
	Base    bool    `json:"base"`
}

var benchMu sync.Mutex
var benchCmd *exec.Cmd
var benchCancel bool

func (a *app) benchPath() string { return filepath.Join(a.dataDir, "bench.json") }

func (a *app) benchLoad() []benchRun {
	var l []benchRun
	if b, err := os.ReadFile(a.benchPath()); err == nil {
		_ = json.Unmarshal(b, &l)
	}
	return l
}

func (a *app) benchSave(l []benchRun) {
	if len(l) > 30 {
		l = l[len(l)-30:]
	}
	b, _ := json.Marshal(l)
	_ = os.WriteFile(a.benchPath(), b, 0o644)
}

func (a *app) benchList() string {
	l := a.benchLoad()
	if l == nil {
		l = []benchRun{}
	}
	b, _ := json.Marshal(l)
	return string(b)
}

// benchEdit: op = "del" ลบ, "base" ตั้งเป็นค่าก่อนปรับ (ไว้เทียบ)
func (a *app) benchEdit(op string, at int64) string {
	l := a.benchLoad()
	out := l[:0]
	for _, r := range l {
		switch op {
		case "del":
			if r.At == at {
				continue
			}
		case "base":
			r.Base = r.At == at
		}
		out = append(out, r)
	}
	a.benchSave(out)
	return a.benchList()
}

func (a *app) presentMonPath() (string, error) {
	sum := sha256.Sum256(presentMonExe)
	p := filepath.Join(a.dataDir, "pm", "PresentMon-2.6.0-x64.exe")
	if b, err := os.ReadFile(p); err == nil {
		s2 := sha256.Sum256(b)
		if hex.EncodeToString(s2[:]) == hex.EncodeToString(sum[:]) {
			return p, nil
		}
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, presentMonExe, 0o755); err != nil {
		return "", err
	}
	return p, nil
}

func (a *app) benchEmit(v map[string]any) {
	b, _ := json.Marshal(v)
	a.emit("onBench", string(b))
}

func (a *app) benchStop() bool {
	benchMu.Lock()
	defer benchMu.Unlock()
	benchCancel = true
	if benchCmd != nil && benchCmd.Process != nil {
		_ = benchCmd.Process.Kill()
	}
	return true
}

// benchStart นับถอยหลัง delay วินาที (ให้สลับเข้าเกม) แล้ววัด secs วินาที
func (a *app) benchStart(secs, delay int, label string) bool {
	benchMu.Lock()
	if benchCmd != nil {
		benchMu.Unlock()
		return false
	}
	benchCancel = false
	benchMu.Unlock()
	if secs < 10 || secs > 600 {
		secs = 60
	}
	go a.benchRun(secs, delay, label)
	return true
}

func (a *app) benchRun(secs, delay int, label string) {
	fail := func(m string) { a.benchEmit(map[string]any{"t": "fail", "m": m}) }
	pm, err := a.presentMonPath()
	if err != nil {
		fail("แตกไฟล์ตัววัดไม่ได้: " + err.Error())
		return
	}
	for i := delay; i > 0; i-- {
		benchMu.Lock()
		c := benchCancel
		benchMu.Unlock()
		if c {
			a.benchEmit(map[string]any{"t": "cancel"})
			return
		}
		a.benchEmit(map[string]any{"t": "count", "n": i})
		time.Sleep(time.Second)
	}
	pid, game := gtaProcess()
	if pid == 0 {
		fail("ยังไม่ได้เปิดเกม FiveM เข้าเซิร์ฟก่อนแล้วค่อยกดวัด")
		return
	}
	cmd := exec.Command(pm, "--process_id", strconv.Itoa(int(pid)), "--output_stdout", "--no_console_stats",
		"--timed", strconv.Itoa(secs), "--terminate_after_timed", "--terminate_on_proc_exit",
		"--stop_existing_session", "--session_name", "ZanBoosterBench", "--no_track_input")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.StdoutPipe()
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		fail("เริ่มตัววัดไม่ได้: " + err.Error())
		return
	}
	benchMu.Lock()
	benchCmd = cmd
	benchMu.Unlock()
	defer func() { benchMu.Lock(); benchCmd = nil; benchMu.Unlock() }()
	pMessageBeep.Call(0)

	// ใช้งานการ์ดจอ/ซีพียูจาก Performance Counter (สำรองกรณี PresentMon ไม่ให้ค่า GPU)
	stopPdh := make(chan struct{})
	var pg, pc, pm2 []float64
	pdhDone := make(chan struct{})
	go func() {
		defer close(pdhDone)
		q := openPdh()
		if q == nil {
			return
		}
		defer q.close()
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-stopPdh:
				return
			case <-t.C:
				if g, c, m, ok := q.sample(); ok {
					pg, pc, pm2 = append(pg, g), append(pc, c), append(pm2, m)
				}
			}
		}
	}()

	var ft, gb, lat []float64
	col := map[string]int{}
	start := time.Now()
	last := start
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	num := func(f []string, name string) (float64, bool) {
		i, ok := col[name]
		if !ok || i >= len(f) {
			return 0, false
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(f[i]), 64)
		if err != nil || math.IsNaN(v) {
			return 0, false
		}
		return v, true
	}
	winStart := 0
	for sc.Scan() {
		f := strings.Split(sc.Text(), ",")
		if len(col) == 0 {
			if len(f) > 3 && f[0] == "Application" {
				for i, n := range f {
					col[strings.TrimSpace(n)] = i
				}
			}
			continue
		}
		v, ok := num(f, "FrameTime")
		if !ok {
			v, ok = num(f, "MsBetweenPresents")
		}
		if !ok || v <= 0 || v > 2000 {
			continue
		}
		ft = append(ft, v)
		if g, ok := num(f, "GPUBusy"); ok {
			gb = append(gb, math.Min(g, v))
		}
		if d, ok := num(f, "DisplayLatency"); ok && d > 0 && d < 1000 {
			lat = append(lat, d)
		}
		if time.Since(last) >= time.Second {
			last = time.Now()
			s := 0.0
			for _, x := range ft[winStart:] {
				s += x
			}
			live := 0.0
			if n := len(ft) - winStart; n > 0 && s > 0 {
				live = 1000 * float64(n) / s
			}
			winStart = len(ft)
			a.benchEmit(map[string]any{"t": "tick", "el": int(time.Since(start).Seconds()), "secs": secs, "fps": math.Round(live), "frames": len(ft)})
		}
	}
	_ = cmd.Wait()
	close(stopPdh)
	<-pdhDone
	benchMu.Lock()
	c := benchCancel
	benchMu.Unlock()
	if c {
		a.benchEmit(map[string]any{"t": "cancel"})
		return
	}
	pMessageBeep.Call(0)
	if len(ft) < 100 {
		m := "เก็บเฟรมได้น้อยเกินไป (เกมถูกพับจอหรือปิดไประหว่างวัด)"
		if len(col) == 0 {
			m = "ตัววัดไม่ส่งข้อมูลกลับมา " + strings.TrimSpace(errb.String())
		}
		fail(m)
		return
	}
	el := time.Since(start).Seconds()
	r := summarize(ft, gb, lat)
	avg := func(v []float64) float64 {
		if len(v) == 0 {
			return 0
		}
		t := 0.0
		for _, x := range v {
			t += x
		}
		return math.Round(t / float64(len(v)))
	}
	if r.GpuPct == 0 {
		r.GpuPct = avg(pg)
	}
	r.CpuPct, r.CoreMax = avg(pc), avg(pm2)
	r.At, r.Label, r.Game, r.Secs = time.Now().Unix(), label, game, int(math.Round(el))
	l := a.benchLoad()
	hasBase := false
	for _, x := range l {
		hasBase = hasBase || x.Base
	}
	r.Base = !hasBase
	l = append(l, r)
	a.benchSave(l)
	b, _ := json.Marshal(r)
	a.benchEmit(map[string]any{"t": "done", "r": json.RawMessage(b)})
}

func summarize(ft, gb, lat []float64) benchRun {
	sum := 0.0
	for _, v := range ft {
		sum += v
	}
	s := append([]float64(nil), ft...)
	sort.Sort(sort.Reverse(sort.Float64Slice(s)))
	// 1% low = FPS เฉลี่ยของเฟรมที่ช้าที่สุด 1%
	worst := func(p float64) float64 {
		n := int(math.Max(1, math.Floor(float64(len(s))*p)))
		t := 0.0
		for _, v := range s[:n] {
			t += v
		}
		return 1000 / (t / float64(n))
	}
	r := benchRun{Frames: len(ft), Avg: 1000 * float64(len(ft)) / sum, Low1: worst(0.01), Low01: worst(0.001)}
	if len(gb) > len(ft)/2 {
		g := 0.0
		for _, v := range gb {
			g += v
		}
		r.GpuPct = math.Min(100, 100*g/sum*float64(len(ft))/float64(len(gb)))
	}
	if len(lat) > 0 {
		t := 0.0
		for _, v := range lat {
			t += v
		}
		r.Latency = t / float64(len(lat))
	}
	round := func(v float64) float64 { return math.Round(v*10) / 10 }
	r.Avg, r.Low1, r.Low01, r.GpuPct, r.Latency = round(r.Avg), round(r.Low1), round(r.Low01), math.Round(r.GpuPct), round(r.Latency)
	return r
}

