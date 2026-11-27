//go:build windows

package main

// upscale.go: ถอดจาก exe 4.11.15 · Zan Upscale = ตั้ง SamplingMode ของเกม (ความละเอียดเรนเดอร์) + ZanSharp ลับภาพ
// แก้เฉพาะค่า SamplingMode ใน gta5_settings.xml (เก็บไฟล์เดิมไว้ก่อน) ตอนเกมเปิดอยู่จะรอให้ปิดเกมก่อนแล้วค่อยเขียน
// openNvcp / dldsrInfo: ปุ่มเปิด NVIDIA App และเช็กว่าการ์ดเป็น RTX (ใช้ DLDSR ได้)

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type upMode struct {
	Sampling int
	Sharp    float64
}

var upModes = map[string]upMode{"off": {0, 0}, "perf": {3, 0}, "quality": {6, 0}, "ultra": {8, 0}}

var (
	samplingRx = regexp.MustCompile(`(<SamplingMode\s+value=")(\d+)(")`)
	blurTechRx = regexp.MustCompile(`(?i)blur|gaussian|fxaa|dof|bokeh|softfocus`)
	gfxWRx     = regexp.MustCompile(`<ScreenWidth\s+value="(\d+)"`)
	gfxHRx     = regexp.MustCompile(`<ScreenHeight\s+value="(\d+)"`)
	gpuT4      = regexp.MustCompile(`(?i)rtx\s*(4080|4090|5080|5090)|rx\s*7900\s*xtx?|rx\s*9070\s*xt`)
	gpuT3      = regexp.MustCompile(`(?i)rtx\s*(3070|3080|3090|4060\s*ti|4070|5060\s*ti|5070)|rx\s*(6800|6900|6950|7800|7900|9070|9060\s*xt)|arc\s*b580`)
	gpuT2      = regexp.MustCompile(`(?i)rtx\s*(2060|2070|2080|3050|3060|4050|4060|5050|5060)|gtx\s*(1070|1080)|rx\s*(5700|6600|6650|6700|6750|7600|7700|9060)|arc\s*a7`)
	gpuAnyRx   = regexp.MustCompile(`(?i)rtx|gtx|radeon rx|arc`)
)

type upMemo struct {
	Preset    string  `json:"preset"`
	AddedCAS  bool    `json:"addedCas"`
	PrevSharp string  `json:"prevSharp"`
	Sharp     float64 `json:"sharp"`
	Pending   string  `json:"pending,omitempty"`
	Blurs     string  `json:"blurs,omitempty"`
}

func (a *app) upMemoPath() string { return filepath.Join(a.dataDir, "upscale.json") }

func (a *app) upMemoGet() upMemo {
	var m upMemo
	if b, err := os.ReadFile(a.upMemoPath()); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func (a *app) upMemoSet(m upMemo) {
	b, _ := json.Marshal(m)
	_ = os.WriteFile(a.upMemoPath(), b, 0o644)
}

// readSampling: ค่า SamplingMode ปัจจุบัน (ไม่มีบรรทัดนี้ = 0) · false = ยังไม่มีไฟล์ตั้งค่าเกม
func readSampling() (int, bool) {
	b, err := os.ReadFile(gfxFile())
	if err != nil {
		return 0, false
	}
	if m := samplingRx.FindSubmatch(b); m != nil {
		return atoi(string(m[2])), true
	}
	return 0, true
}

func writeSampling(dataDir string, n int) string {
	f := gfxFile()
	b, err := os.ReadFile(f)
	if err != nil {
		return "ยังไม่มีไฟล์ตั้งค่าของเกม เข้า FiveM สักครั้งก่อน"
	}
	orig := filepath.Join(dataDir, "gta5_settings.original.xml")
	if _, err := os.Stat(orig); err != nil {
		_ = os.WriteFile(orig, b, 0o644)
	}
	if samplingRx.Match(b) {
		b = samplingRx.ReplaceAll(b, []byte("${1}"+strconv.Itoa(n)+"${3}"))
	} else {
		s := string(b)
		i := strings.Index(s, "</graphics>")
		if i < 0 {
			return "ไฟล์ตั้งค่าของเกมไม่มีส่วนกราฟิก"
		}
		b = []byte(s[:i] + `  <SamplingMode value="` + strconv.Itoa(n) + "\" />\r\n  " + s[i:])
	}
	if err := os.WriteFile(f, b, 0o644); err != nil {
		return "เขียนไฟล์ไม่ได้: " + err.Error()
	}
	return ""
}

// presetUndoCAS: ตัวเก่ารุ่นแรกใส่ CAS ลงพรีเซ็ตของผู้ใช้ · ตอนนี้ย้ายไปใช้ ZanSharp แล้ว จึงคืนพรีเซ็ตให้เหมือนเดิม
func presetUndoCAS(m upMemo) {
	if m.Preset == "" || !fileExists(m.Preset) {
		return
	}
	if m.AddedCAS {
		drop := func(key string) {
			var keep []string
			for _, t := range strings.Split(u8IniGet(m.Preset, "", key), ",") {
				if t = strings.TrimSpace(t); t != "" && t != "ContrastAdaptiveSharpen@CAS.fx" {
					keep = append(keep, t)
				}
			}
			u8IniSetTop(m.Preset, key, strings.Join(keep, ","))
		}
		drop("Techniques")
		drop("TechniqueSorting")
	} else if m.PrevSharp != "" {
		u8IniSet(m.Preset, "CAS.fx", "Sharpening", m.PrevSharp)
	}
	if m.Blurs != "" {
		cur := strings.TrimSpace(u8IniGet(m.Preset, "", "Techniques"))
		if cur != "" {
			cur += ","
		}
		u8IniSetTop(m.Preset, "Techniques", cur+m.Blurs)
	}
}

type upState struct {
	OK       bool   `json:"ok"`
	Msg      string `json:"m,omitempty"`
	Mode     string `json:"mode"`
	Sampling int    `json:"sampling"`
	Reshade  bool   `json:"reshade"`
	Sharp    bool   `json:"sharp"`
	Blurs    string `json:"blurs,omitempty"`
	W        int    `json:"w,omitempty"`
	H        int    `json:"h,omitempty"`
	GPU      string `json:"gpu,omitempty"`
	Rec      string `json:"rec,omitempty"`
}

// upRecommend: โหมดที่แนะนำตามรุ่นการ์ดจอและความละเอียดจอ
func upRecommend(gpu string, w, h int) string {
	t := 1
	switch {
	case gpuT4.MatchString(gpu):
		t = 4
	case gpuT3.MatchString(gpu):
		t = 3
	case gpuT2.MatchString(gpu):
		t = 2
	}
	if px := w * h; px > 5100000 {
		t -= 2
	} else if px > 2300000 {
		t--
	}
	if t < 1 {
		t = 1
	}
	return map[int]string{4: "ultra", 3: "quality", 2: "off"}[t] + map[bool]string{true: "perf"}[t == 1]
}

func upSpec() (w, h int, gpu, rec string) {
	if b, err := os.ReadFile(gfxFile()); err == nil {
		if m := gfxWRx.FindSubmatch(b); m != nil {
			w = atoi(string(m[1]))
		}
		if m := gfxHRx.FindSubmatch(b); m != nil {
			h = atoi(string(m[1]))
		}
	}
	for _, n := range gpuNames() {
		if gpu == "" || gpuAnyRx.MatchString(n) {
			gpu = n
			if gpuAnyRx.MatchString(n) {
				break
			}
		}
	}
	rw, rh := w, h
	if rw == 0 || rh == 0 {
		rw, rh = 1920, 1080
	}
	return w, h, gpu, upRecommend(gpu, rw, rh)
}

func (a *app) upscaleInfo(dir string) upState {
	s, ok := readSampling()
	if !ok {
		return upState{Msg: "ยังไม่มีไฟล์ตั้งค่าของเกม เข้า FiveM สักครั้งก่อน"}
	}
	st := upState{OK: true, Mode: "custom", Sampling: s}
	for k, m := range upModes {
		if m.Sampling == s {
			st.Mode = k
			break
		}
	}
	st.W, st.H, st.GPU, st.Rec = upSpec()
	_, plugins, _ := reshadePaths(dir)
	st.Reshade = isReshadeDll(filepath.Join(plugins, "dxgi.dll"))
	if b, err := os.ReadFile(zanFxPath(plugins, "ZanSharp.fx")); st.Reshade && err == nil {
		st.Sharp = strings.Contains(string(b), "enabled = true")
	}
	if m := a.upMemoGet(); m.Preset != "" {
		presetUndoCAS(m)
		m.Preset, m.AddedCAS, m.PrevSharp, m.Blurs = "", false, "", ""
		a.upMemoSet(m)
	}
	return st
}

func (a *app) upscaleSet(dir, mode string) upState {
	if !licGet().Premium {
		return upState{Msg: "Zan Upscale เป็นฟีเจอร์พรีเมียม"}
	}
	m, ok := upModes[mode]
	if !ok {
		return upState{Msg: "ไม่รู้จักโหมดนี้"}
	}
	run := fivemRunningAny()
	if !run {
		if e := writeSampling(a.dataDir, m.Sampling); e != "" {
			return upState{Msg: e}
		}
	}
	memo := a.upMemoGet()
	if memo.Preset != "" {
		presetUndoCAS(memo)
		memo = upMemo{Pending: memo.Pending}
	}
	_, plugins, _ := reshadePaths(dir)
	if isReshadeDll(filepath.Join(plugins, "dxgi.dll")) {
		if writeZanSharp(plugins, mode) && run {
			if u8IniGet(filepath.Join(plugins, "ReShade.ini"), "INPUT", "KeyReload") == "136,0,0,0" {
				rsSendKey(0x88)
			}
		}
	}
	memo.Pending = ""
	if run {
		memo.Pending = mode
	}
	a.upMemoSet(memo)
	st := a.upscaleInfo(dir)
	if run {
		st.Msg = "ingame"
	}
	return st
}

// upscaleFlush: เกมปิดแล้ว เขียนค่าที่รอไว้ให้
func (a *app) upscaleFlush(dir string) upState {
	memo := a.upMemoGet()
	if memo.Pending == "" || fivemRunningAny() {
		return a.upscaleInfo(dir)
	}
	m, ok := upModes[memo.Pending]
	memo.Pending = ""
	a.upMemoSet(memo)
	if ok {
		if e := writeSampling(a.dataDir, m.Sampling); e != "" {
			return upState{Msg: e}
		}
	}
	st := a.upscaleInfo(dir)
	st.Msg = "flushed"
	return st
}

func (a *app) upscaleAsync(op, dir string, reqID int) bool {
	go func() {
		switch op {
		case "info":
			a.emitGpu(reqID, a.upscaleInfo(dir))
		case "flush":
			a.emitGpu(reqID, a.upscaleFlush(dir))
		default:
			a.emitGpu(reqID, a.upscaleSet(dir, op))
		}
	}()
	return true
}

// openNvcp: เปิด NVIDIA App (หรือ NVIDIA Control Panel ตัวเก่า) · คืนข้อความถ้าไม่เจอ
func (a *app) openNvcp() string {
	pf := os.Getenv("ProgramFiles")
	for _, p := range []string{
		filepath.Join(pf, "NVIDIA Corporation", "NVIDIA App", "CEF", "NVIDIA App.exe"),
		filepath.Join(pf, "NVIDIA Corporation", "Control Panel Client", "nvcplui.exe"),
	} {
		if fileExists(p) && exec.Command(p).Start() == nil {
			return ""
		}
	}
	return "ไม่พบ NVIDIA App ในเครื่องนี้"
}

// dldsrInfo: การ์ด RTX ใช้ DLDSR ได้ไหม
func (a *app) dldsrInfo() string {
	for _, n := range gpuNames() {
		if strings.Contains(strings.ToUpper(n), "RTX") {
			b, _ := json.Marshal(map[string]any{"ok": true, "gpu": n})
			return string(b)
		}
	}
	g := ""
	if ns := gpuNames(); len(ns) > 0 {
		g = ns[0]
	}
	b, _ := json.Marshal(map[string]any{"ok": false, "gpu": g})
	return string(b)
}
