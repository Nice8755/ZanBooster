//go:build windows

package main

// reshade.go: ถอดจาก exe 4.11.15 · ติดตั้ง/ถอด/เปิด-ปิด ReShade ในโฟลเดอร์ plugins ของ FiveM (FiveM โหลด dxgi.dll เอง)
// ไม่แตะไฟล์เกม · dxgi.dll ของโปรแกรมอื่นที่มีอยู่เดิมเก็บสำรองเป็น dxgi.dll.zanbak แล้วคืนให้ตอนถอด

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const rsUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) ZanBooster/4.11.15"

var (
	reshadeLogRx = regexp.MustCompile(`ReShade5=ID:([0-9a-fA-F]{8})`)
	reshadeVerRx = regexp.MustCompile(`downloads/ReShade_Setup_(\d+\.\d+\.\d+)\.exe`)
	paidRx       = regexp.MustCompile(`(?i)rtgi|ssrt|martysmods|glamarye|regrade|prism`)
	rsLatestMu   sync.Mutex
	rsLatest     string
	rsLatestAt   time.Time
)

// reshadeCalcID: รหัสที่ FiveM ใช้ยืนยันว่ารับทราบปัญหา ReShade 5 (hash ชื่อเครื่องตัวเล็ก แบบเดียวกับ FiveM)
func reshadeCalcID() string {
	n := os.Getenv("COMPUTERNAME")
	if n == "" {
		n = "a"
	}
	var h uint32
	for i := 0; i < len(n); i++ {
		c := n[i]
		if c-'A' <= 25 {
			c += 32
		}
		h += uint32(int32(int8(c)))
		h += h << 10
		h ^= h >> 6
	}
	h += h << 3
	h ^= h >> 11
	h += h << 15
	return fmt.Sprintf("%08x", h)
}

func reshadeAckValue(id string) string {
	return "ID:" + id + " acknowledged that ReShade 5.x has a bug that will lead to game crashes"
}

// reshadeLogID: รหัสที่ FiveM เขียนไว้ในไฟล์ log ล่าสุด (แม่นกว่าคำนวณเอง)
func reshadeLogID(app string) (string, bool) {
	es, err := os.ReadDir(filepath.Join(app, "logs"))
	if err != nil {
		return "", false
	}
	type lf struct {
		p string
		t time.Time
	}
	var fs []lf
	for _, e := range es {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".log") {
			continue
		}
		if fi, err := e.Info(); err == nil {
			fs = append(fs, lf{filepath.Join(app, "logs", e.Name()), fi.ModTime()})
		}
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].t.After(fs[j].t) })
	for i := 0; i < len(fs) && i < 6; i++ {
		b, err := os.ReadFile(fs[i].p)
		if err != nil {
			continue
		}
		if m := reshadeLogRx.FindAllSubmatch(b, -1); len(m) > 0 {
			return strings.ToLower(string(m[len(m)-1][1])), true
		}
	}
	return "", false
}

// reshadePaths: โฟลเดอร์ FiveM.app, plugins และไฟล์ CitizenFX.ini
func reshadePaths(dir string) (app, plugins, cfx string) {
	app = filepath.Join(dir, "FiveM.app")
	return app, filepath.Join(app, "plugins"), filepath.Join(app, "CitizenFX.ini")
}

// reshadeFixAck: เขียนค่ารับทราบใน CitizenFX.ini ให้ FiveM ยอมโหลด ReShade (true = เพิ่งแก้)
func reshadeFixAck(dir string) (string, bool) {
	app, _, cfx := reshadePaths(dir)
	id, ok := reshadeLogID(app)
	if !ok {
		id = reshadeCalcID()
	}
	want := reshadeAckValue(id)
	if readIni(cfx, "Addons", "ReShade5", "") == want {
		return id, false
	}
	writeIni(cfx, "Addons", "ReShade5", want)
	return id, true
}

type reshadeState struct {
	OK        bool       `json:"ok"`
	Msg       string     `json:"m,omitempty"`
	Installed bool       `json:"installed"`
	Version   string     `json:"ver,omitempty"`
	Kind      string     `json:"kind,omitempty"`
	AckOK     bool       `json:"ackOk"`
	ID        string     `json:"id,omitempty"`
	Fixed     bool       `json:"fixed,omitempty"`
	Plugins   string     `json:"plugins,omitempty"`
	Disabled  bool       `json:"disabled,omitempty"`
	Latest    string     `json:"latest,omitempty"`
	Preset    string     `json:"preset,omitempty"`
	Zan       string     `json:"zan,omitempty"`
	Light     string     `json:"light,omitempty"`
	Eco       bool       `json:"eco,omitempty"`
	My        []myPreset `json:"my,omitempty"`
	DN        string     `json:"dn,omitempty"`
	Hud       bool       `json:"hud,omitempty"`
}

var (
	zanMyRx     = regexp.MustCompile(`(?i)^zan_my_(\d)\.ini$`)
	zanPresetRx = regexp.MustCompile(`(?i)^zan_preset_(\d+)\.ini$`)
	ackIDRx     = regexp.MustCompile(`^ID:([0-9a-fA-F]{8})`)
)

func (a *app) reshadeStatus(dir string) reshadeState {
	st := reshadeState{OK: true}
	if dir == "" || !dirExists(filepath.Join(dir, "FiveM.app")) {
		return reshadeState{Msg: "ยังไม่เจอโฟลเดอร์ FiveM"}
	}
	_, plugins, cfx := reshadePaths(dir)
	st.Plugins = plugins
	dll := filepath.Join(plugins, "dxgi.dll")
	if isReshadeDll(filepath.Join(plugins, "dxgi.dll.off")) && !isReshadeDll(dll) {
		st.Installed, st.Disabled = true, true
		if b, err := os.ReadFile(filepath.Join(plugins, "ReShade.zanver")); err == nil {
			st.Version = strings.TrimSpace(string(b))
		}
	}
	if isReshadeDll(dll) {
		st.Installed = true
		if b, err := os.ReadFile(filepath.Join(plugins, "ReShade.zanver")); err == nil {
			st.Version = strings.TrimSpace(string(b))
		}
	}
	if b, err := os.ReadFile(filepath.Join(plugins, "ReShade.zankind")); err == nil {
		st.Kind = strings.TrimSpace(string(b))
	}
	if !fivemRunningAny() {
		if id, fixed := reshadeFixAck(dir); fixed {
			st.Fixed = true
			st.ID = id
		}
	}
	if st.Installed {
		st.Latest = reshadeLatest()
		if !fivemRunningAny() {
			a.zanFlush(dir)
		}
		if p := u8IniGet(filepath.Join(plugins, "ReShade.ini"), "GENERAL", "PresetPath"); p != "" {
			st.Preset = p
			st.Light, st.Eco = zanLight, zanEco
			if zanDataDir != "" {
				st.My = zanMyList()
				if d, n := zanDNGet(); d != "" {
					st.DN = d + "," + n
				}
				st.Hud = fileExists(filepath.Join(zanDataDir, "zan_hud.png"))
			}
			base := filepath.Base(strings.ReplaceAll(p, "/", `\`))
			if strings.EqualFold(base, "Zan_DayNight.ini") {
				st.Zan = "dn"
			} else if m := zanMyRx.FindStringSubmatch(base); m != nil {
				st.Zan = "m" + m[1]
			} else if m := zanPresetRx.FindStringSubmatch(base); m != nil {
				st.Zan = "p" + fmt.Sprint(atoi(m[1]))
			}
		}
	}
	v := readIni(cfx, "Addons", "ReShade5", "")
	st.AckOK = v != "" && strings.Contains(v, " acknowledged that ReShade 5.x has a bug that will lead to game crashes")
	if m := ackIDRx.FindStringSubmatch(v); m != nil && st.ID == "" {
		st.ID = m[1]
	}
	return st
}

// dlBytes: ดาวน์โหลดทั้งไฟล์ (ไม่เกิน max ไบต์) หมดเวลา 3 นาที
func dlBytes(u string, max int64) ([]byte, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", rsUA)
	resp, err := (&http.Client{Timeout: 180 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, max))
}

func (a *app) reshadeInstall(dir string, step func(string)) reshadeState {
	fail := func(m string) reshadeState { return reshadeState{Msg: m} }
	if dir == "" || !dirExists(filepath.Join(dir, "FiveM.app")) {
		return fail("ยังไม่เจอโฟลเดอร์ FiveM ตั้งในหน้าเครื่องมือก่อน")
	}
	if fivemRunningAny() {
		return fail("ปิด FiveM ก่อน แล้วกดใหม่")
	}
	_, plugins, _ := reshadePaths(dir)
	step("หาเวอร์ชันล่าสุดของ ReShade...")
	page, err := dlBytes("https://reshade.me/", 4<<20)
	if err != nil {
		return fail("ต่อเว็บ reshade.me ไม่ได้: " + err.Error())
	}
	m := reshadeVerRx.FindSubmatch(page)
	if m == nil {
		return fail("หาไฟล์ดาวน์โหลดของ ReShade ไม่เจอ (เว็บอาจเปลี่ยน)")
	}
	ver := string(m[1])
	step("ดาวน์โหลด ReShade " + ver + " (Addon)...")
	kind := "Addon"
	setup, err := dlBytes("https://reshade.me/downloads/ReShade_Setup_"+ver+"_Addon.exe", 200<<20)
	if err != nil || len(setup) < 1<<20 {
		step("โหลดตัว Addon ไม่ได้ ใช้ ReShade " + ver + " ตัวปกติแทน...")
		kind = ""
		setup, err = dlBytes("https://reshade.me/downloads/ReShade_Setup_"+ver+".exe", 200<<20)
		if err != nil {
			return fail("ดาวน์โหลด ReShade ไม่สำเร็จ: " + err.Error())
		}
	}
	// ตัวติดตั้งของ ReShade คือ exe ที่มีไฟล์ zip ต่อท้าย
	zr, err := zip.NewReader(bytes.NewReader(setup), int64(len(setup)))
	if err != nil {
		return fail("เปิดไฟล์ติดตั้ง ReShade ไม่ได้: " + err.Error())
	}
	var dll []byte
	for _, f := range zr.File {
		if strings.EqualFold(filepath.Base(f.Name), "ReShade64.dll") {
			if rc, err := f.Open(); err == nil {
				dll, _ = io.ReadAll(rc)
				rc.Close()
			}
			break
		}
	}
	if len(dll) < 100000 {
		return fail("ไม่เจอ ReShade64.dll ในไฟล์ติดตั้ง")
	}
	step("ติดตั้งลงโฟลเดอร์ plugins ของ FiveM...")
	if err := os.MkdirAll(plugins, 0o755); err != nil {
		return fail("สร้างโฟลเดอร์ plugins ไม่ได้: " + err.Error())
	}
	dst := filepath.Join(plugins, "dxgi.dll")
	if isReshadeDll(dst + ".off") {
		os.Remove(dst + ".off")
	}
	if old, err := os.ReadFile(dst); err == nil && !bytes.Contains(old, []byte("ReShade")) {
		_ = os.WriteFile(dst+".zanbak", old, 0o644)
	}
	if err := os.WriteFile(dst, dll, 0o644); err != nil {
		return fail("เขียน dxgi.dll ไม่ได้: " + err.Error())
	}
	n, err := reshadeEffects(plugins, step)
	if err != nil && n == 0 {
		return fail("ดาวน์โหลดชุดเอฟเฟกต์ไม่สำเร็จ ตรวจอินเทอร์เน็ตแล้วลองใหม่")
	}
	ini := filepath.Join(plugins, "ReShade.ini")
	if _, err := os.Stat(ini); err != nil {
		sh := filepath.Join(plugins, "reshade-shaders")
		s := fmt.Sprintf("[GENERAL]\r\nEffectSearchPaths=%s\\**\r\nTextureSearchPaths=%s\\**\r\nPresetPath=%s\r\n\r\n[INPUT]\r\nKeyOverlay=36,0,0,0\r\n\r\n[OVERLAY]\r\nTutorialProgress=4\r\n",
			filepath.Join(sh, "Shaders"), filepath.Join(sh, "Textures"), filepath.Join(plugins, "ReShadePreset.ini"))
		_ = os.WriteFile(ini, []byte(s), 0o644)
	}
	_ = os.WriteFile(filepath.Join(plugins, "ReShade.zanver"), []byte(ver), 0o644)
	step("ตั้งค่า CitizenFX.ini ให้ FiveM ยอมโหลด ReShade...")
	id, _ := reshadeFixAck(dir)
	_ = os.WriteFile(filepath.Join(plugins, "ReShade.zankind"), []byte(kind), 0o644)
	msg := fmt.Sprintf("ติดตั้ง ReShade %s %s แล้ว · เอฟเฟกต์ %d ไฟล์ · ID %s", ver, kind, n, id)
	step(msg)
	st := a.reshadeStatus(dir)
	st.Msg = msg
	return st
}

func (a *app) reshadeRemove(dir string) reshadeState {
	if fivemRunningAny() {
		return reshadeState{Msg: "ปิด FiveM ก่อน แล้วกดใหม่"}
	}
	_, plugins, cfx := reshadePaths(dir)
	dst := filepath.Join(plugins, "dxgi.dll")
	if b, err := os.ReadFile(dst); err == nil && bytes.Contains(b, []byte("ReShade")) {
		if err := os.Remove(dst); err != nil {
			return reshadeState{Msg: "ลบ dxgi.dll ไม่ได้: " + err.Error()}
		}
	}
	if isReshadeDll(dst + ".off") {
		os.Remove(dst + ".off")
	}
	if b, err := os.ReadFile(dst + ".zanbak"); err == nil {
		if !fileExists(dst) {
			_ = os.WriteFile(dst, b, 0o644)
		}
		os.Remove(dst + ".zanbak")
	}
	os.Remove(filepath.Join(plugins, "ReShade.zanver"))
	os.Remove(filepath.Join(plugins, "ReShade.zankind"))
	writeIni(cfx, "Addons", "ReShade5", "")
	return a.reshadeStatus(dir)
}

// ------------------------------------------------------------ ชุดเอฟเฟกต์ --

type fxPkg struct {
	Name, URL, Install, TexInstall string
	Deny                           map[string]bool
}

// fxPackages: รายการชุดเอฟเฟกต์ทางการของ ReShade (EffectPackages.ini) โหลดไม่ได้ใช้ชุดมาตรฐาน
func fxPackages() []fxPkg {
	std := []fxPkg{{"Standard effects", "https://github.com/crosire/reshade-shaders/archive/slim.zip", `.\reshade-shaders\Shaders`, `.\reshade-shaders\Textures`, nil}}
	b, err := dlBytes("https://raw.githubusercontent.com/crosire/reshade-shaders/list/EffectPackages.ini", 1<<20)
	if err != nil {
		return std
	}
	var out []fxPkg
	var cur *fxPkg
	flush := func() {
		if cur != nil && cur.URL != "" && cur.Install != "" {
			out = append(out, *cur)
		}
	}
	for _, ln := range strings.Split(strings.ReplaceAll(string(b), "\r", ""), "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "[") {
			flush()
			cur = &fxPkg{Deny: map[string]bool{}}
			continue
		}
		kv := strings.SplitN(ln, "=", 2)
		if cur == nil || len(kv) != 2 {
			continue
		}
		k, v := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
		switch k {
		case "PackageName":
			cur.Name = v
		case "DownloadUrl":
			cur.URL = v
		case "InstallPath":
			cur.Install = v
		case "TextureInstallPath":
			cur.TexInstall = v
		case "DenyEffectFiles":
			for _, d := range strings.Split(v, ",") {
				cur.Deny[strings.ToLower(strings.TrimSpace(d))] = true
			}
		}
	}
	flush()
	if len(out) == 0 {
		return std
	}
	return out
}

// fxDest: ที่ลงไฟล์ของชุดนี้ (ต้องอยู่ใต้ reshade-shaders เท่านั้น)
func fxDest(plugins, p, def string) string {
	p = strings.ReplaceAll(p, "/", `\`)
	p = strings.TrimPrefix(p, `.\`)
	p = strings.TrimPrefix(p, `\`)
	if p == "" || strings.Contains(p, "..") || !strings.HasPrefix(strings.ToLower(p), "reshade-shaders") {
		p = def
	}
	return filepath.Join(plugins, p)
}

func fxInstallOne(p fxPkg, plugins string) (int, error) {
	b, err := dlBytes(p.URL, 300<<20)
	if err != nil {
		return 0, err
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return 0, err
	}
	hasSh := false
	for _, f := range zr.File {
		if strings.Contains(strings.ToLower(f.Name), "/shaders/") {
			hasSh = true
			break
		}
	}
	shDst := fxDest(plugins, p.Install, `reshade-shaders\Shaders`)
	txDst := fxDest(plugins, p.TexInstall, `reshade-shaders\Textures`)
	n := 0
	for _, f := range zr.File {
		if f.Mode().IsDir() || strings.Contains(f.Name, "..") {
			continue
		}
		l := strings.ToLower(f.Name)
		var dst string
		if i := strings.Index(l, "/shaders/"); i >= 0 {
			rel := f.Name[i+9:]
			if p.Deny[strings.ToLower(filepath.Base(rel))] {
				continue
			}
			dst = filepath.Join(shDst, filepath.FromSlash(rel))
		} else if i := strings.Index(l, "/textures/"); i >= 0 {
			dst = filepath.Join(txDst, filepath.FromSlash(f.Name[i+10:]))
		} else if !hasSh && (strings.HasSuffix(l, ".fx") || strings.HasSuffix(l, ".fxh")) {
			if p.Deny[strings.ToLower(filepath.Base(l))] {
				continue
			}
			rel := f.Name
			if j := strings.LastIndex(rel, "/"); j >= 0 {
				rel = rel[j+1:]
			}
			dst = filepath.Join(shDst, rel)
		} else {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		if os.WriteFile(dst, data, 0o644) == nil && strings.HasSuffix(l, ".fx") {
			n++
		}
	}
	return n, nil
}

// reshadeEffects: ลงชุดเอฟเฟกต์ทุกชุดพร้อมกันทีละ 4 ชุด คืนจำนวนไฟล์ .fx ที่ลงได้
func reshadeEffects(plugins string, step func(string)) (int, error) {
	step("ดูรายการชุดเอฟเฟกต์ทั้งหมด...")
	pkgs := fxPackages()
	var mu sync.Mutex
	total, failed, done := 0, 0, 0
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, p := range pkgs {
		wg.Add(1)
		sem <- struct{}{}
		go func(p fxPkg) {
			defer wg.Done()
			n, err := fxInstallOne(p, plugins)
			<-sem
			mu.Lock()
			done++
			if err != nil {
				failed++
			}
			total += n
			step(fmt.Sprintf("ลงชุดเอฟเฟกต์ %d/%d · %s", done, len(pkgs), p.Name))
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	step(fmt.Sprintf("ลงเอฟเฟกต์แล้ว %d ไฟล์ (โหลดไม่สำเร็จ %d ชุด)", total, failed))
	if total == 0 {
		return 0, fmt.Errorf("no effects")
	}
	return total, nil
}

// reshadeLatest: เวอร์ชันล่าสุดบนเว็บ ReShade (จำไว้ 6 ชั่วโมง)
func reshadeLatest() string {
	rsLatestMu.Lock()
	defer rsLatestMu.Unlock()
	if rsLatest != "" && time.Since(rsLatestAt) < 6*time.Hour {
		return rsLatest
	}
	if b, err := dlBytes("https://reshade.me/", 4<<20); err == nil {
		if m := reshadeVerRx.FindSubmatch(b); m != nil {
			rsLatest, rsLatestAt = string(m[1]), time.Now()
		}
	}
	return rsLatest
}

func isReshadeDll(p string) bool {
	b, err := os.ReadFile(p)
	return err == nil && bytes.Contains(b, []byte("ReShade"))
}

// reshadeToggle: เปิด/ปิดชั่วคราวโดยเปลี่ยนชื่อ dxgi.dll <-> dxgi.dll.off
func (a *app) reshadeToggle(dir string, on bool) reshadeState {
	if fivemRunningAny() {
		return reshadeState{Msg: "ปิด FiveM ก่อน แล้วกดใหม่"}
	}
	_, plugins, _ := reshadePaths(dir)
	dll, off := filepath.Join(plugins, "dxgi.dll"), filepath.Join(plugins, "dxgi.dll.off")
	if on {
		if isReshadeDll(off) {
			if fileExists(dll) && !isReshadeDll(dll) {
				return reshadeState{Msg: "มี dxgi.dll ของโปรแกรมอื่นอยู่ในโฟลเดอร์ plugins"}
			}
			os.Remove(dll)
			os.Rename(off, dll)
		}
	} else if isReshadeDll(dll) {
		os.Remove(off)
		os.Rename(dll, off)
	}
	return a.reshadeStatus(dir)
}

func reshadeSetPreset(plugins, preset string) {
	u8IniSet(filepath.Join(plugins, "ReShade.ini"), "GENERAL", "PresetPath", preset)
}

// reshadeImport: เลือกไฟล์พรีเซ็ตของคนอื่น (.ini) มาใช้ คัดลอกเข้า plugins แล้วตั้งเป็นพรีเซ็ตปัจจุบัน
func (a *app) reshadeImport(dir string) reshadeState {
	if fivemRunningAny() {
		return reshadeState{Msg: "ปิด FiveM ก่อน แล้วกดใหม่ (เกมจะเขียนทับค่าตอนปิด)"}
	}
	ps := "Add-Type -AssemblyName System.Windows.Forms; $d=New-Object System.Windows.Forms.OpenFileDialog; $d.Title='เลือกไฟล์พรีเซ็ต ReShade (.ini)'; $d.Filter='ReShade preset (*.ini;*.txt)|*.ini;*.txt|All files (*.*)|*.*'; if($d.ShowDialog() -eq 'OK'){[Console]::OutputEncoding=[Text.Encoding]::UTF8; $d.FileName}"
	c := exec.Command("powershell", "-NoProfile", "-STA", "-Command", ps)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, _ := c.Output()
	src := strings.TrimSpace(string(out))
	if src == "" {
		return reshadeState{Msg: "cancel"}
	}
	b, err := os.ReadFile(src)
	if err != nil || !bytes.Contains(b, []byte("Techniques")) {
		return reshadeState{Msg: "ไฟล์นี้ไม่ใช่พรีเซ็ต ReShade (ไม่มีบรรทัด Techniques)"}
	}
	_, plugins, _ := reshadePaths(dir)
	name := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src)) + ".ini"
	dst := filepath.Join(plugins, name)
	if !strings.EqualFold(dst, src) {
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return reshadeState{Msg: err.Error()}
		}
	}
	reshadeSetPreset(plugins, dst)
	st := a.reshadeStatus(dir)
	st.Msg = "imported:" + name
	return st
}

type presetCheck struct {
	OK      bool     `json:"ok"`
	Msg     string   `json:"m,omitempty"`
	Preset  string   `json:"preset"`
	Total   int      `json:"total"`
	Missing []string `json:"missing"`
	Paid    []string `json:"paid"`
}

// reshadeCheck: พรีเซ็ตที่ใช้อยู่ต้องการเอฟเฟกต์ไฟล์ไหนบ้างที่ยังไม่มีในเครื่อง (และตัวไหนเป็นของเสียเงิน)
func (a *app) reshadeCheck(dir string) presetCheck {
	_, plugins, _ := reshadePaths(dir)
	p := u8IniGet(filepath.Join(plugins, "ReShade.ini"), "GENERAL", "PresetPath")
	if p == "" {
		return presetCheck{Msg: "ยังไม่ได้เลือกพรีเซ็ต (เลือกในเมนู ReShade หรือกดนำเข้าพรีเซ็ต)"}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(plugins, p)
	}
	r := presetCheck{Preset: filepath.Base(p)}
	b, err := os.ReadFile(p)
	if err != nil {
		r.Msg = err.Error()
		return r
	}
	have := map[string]bool{}
	_ = filepath.WalkDir(filepath.Join(plugins, "reshade-shaders"), func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".fx") {
			have[strings.ToLower(d.Name())] = true
		}
		return nil
	})
	seen := map[string]bool{}
	for _, ln := range strings.Split(strings.ReplaceAll(string(b), "\r", ""), "\n") {
		kv := strings.SplitN(ln, "=", 2)
		if len(kv) != 2 || strings.TrimSpace(kv[0]) != "Techniques" {
			continue
		}
		for _, t := range strings.Split(kv[1], ",") {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			r.Total++
			at := strings.LastIndex(t, "@")
			if at < 0 {
				continue
			}
			f := strings.ToLower(t[at+1:])
			if have[f] || seen[f] {
				continue
			}
			seen[f] = true
			r.Missing = append(r.Missing, t[at+1:])
			if paidRx.MatchString(f) {
				r.Paid = append(r.Paid, t[at+1:])
			}
		}
		break
	}
	r.OK = len(r.Missing) == 0
	return r
}

// ------------------------------------------------------- พรีเซ็ต Zan p1-p12 --
// ไฟล์พรีเซ็ต/เอฟเฟกต์ของ Zan มีอยู่ในเครื่องเฉพาะตอนเกมเปิด (zanWatch เขียนให้) ปิดเกมแล้วลบทิ้ง กันคัดลอกไปแจก
// ค่าที่ผู้ใช้ปรับในเกมเก็บไว้ที่ ProgramData\ZanBooster\zan_user\<key>.ini แล้วใส่กลับรอบหน้า

type zanPreset struct{ File, Body string }

var (
	zanPresets = func() map[string]zanPreset {
		m := map[string]zanPreset{}
		for k, v := range zanVals {
			m[k] = zanPreset{fmt.Sprintf("Zan_Preset_%02d.ini", zanIdx(k)), zan3(v)}
		}
		return m
	}()
	zanDir, zanDataDir string
	zanDirMu           sync.Mutex
	rsMu               sync.Mutex
	zanWatchO          sync.Once
	zanLight           string
	zanEco             bool
)

type zanMemo struct {
	Key     string `json:"key"`
	UserPre string `json:"userPre,omitempty"`
	Backup  string `json:"backup,omitempty"`
}

func (a *app) zanMemoGet() zanMemo {
	var m zanMemo
	if b, err := os.ReadFile(filepath.Join(a.dataDir, "zan_preset.json")); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	if m.Key == "p13" {
		m.Key = "p11"
	}
	return m
}

func (a *app) zanMemoSet(m zanMemo) {
	b, _ := json.Marshal(m)
	_ = os.WriteFile(filepath.Join(a.dataDir, "zan_preset.json"), b, 0o644)
}

func zanIdx(k string) int { return atoi(strings.TrimPrefix(k, "p")) }

// zanShortcutData: ปุ่มลัดในเกมของ ReShade ให้สลับพรีเซ็ตได้ทันที (F13-F24 + ปุ่มที่ไม่มีบนคีย์บอร์ด)
func zanShortcutData(plugins string) (keys, paths string) {
	var ks, ps []string
	vk := func(v int) { ks = append(ks, strconv.Itoa(v), "0", "0", "0") }
	for i := 1; i <= len(zanPresets); i++ {
		if i <= 12 {
			vk(i + 0x7b)
		} else {
			vk(i + 0x81)
		}
		ps = append(ps, filepath.Join(plugins, zanPresets["p"+strconv.Itoa(i)].File))
	}
	for i := 1; i <= 3; i++ {
		vk(0x89 + i)
		ps = append(ps, filepath.Join(plugins, "Zan_My_"+strconv.Itoa(i)+".ini"))
	}
	vk(0x8d)
	ps = append(ps, filepath.Join(plugins, "Zan_DayNight.ini"))
	return strings.Join(ks, ","), strings.Join(ps, ",")
}

func zanShortcutsBound(plugins string) bool {
	k, p := zanShortcutData(plugins)
	ini := filepath.Join(plugins, "ReShade.ini")
	return u8IniGet(ini, "GENERAL", "PresetShortcutKeys") == k && strings.EqualFold(u8IniGet(ini, "GENERAL", "PresetShortcutPaths"), p)
}

// zanApplyLight: สีไฟถนนที่ผู้ใช้เลือก แทนค่าในพรีเซ็ต
func zanApplyLight(body string) string {
	if zanLight == "" {
		return body
	}
	ls := strings.Split(body, "\r\n")
	for i, l := range ls {
		switch {
		case strings.HasPrefix(l, "LightColor="):
			ls[i] = "LightColor=" + zanLight
		case l == "LightTint="+f6(0):
			ls[i] = "LightTint=" + f6(0.55)
		case l == "LightGlow="+f6(0):
			ls[i] = "LightGlow=" + f6(0.15)
		case strings.HasPrefix(l, "LightWarmColor="), strings.HasPrefix(l, "LightWhiteColor="):
			ls[i] = l[:strings.Index(l, "=")+1] + zanLight
		case strings.HasPrefix(l, "LightTwoTone="):
			if v, err := strconv.ParseFloat(strings.TrimPrefix(l, "LightTwoTone="), 64); err != nil || v < 0.6 {
				ls[i] = "LightTwoTone=" + f6(0.6)
			}
		}
	}
	return strings.Join(ls, "\r\n")
}

func zanHeader() string {
	return "# Zan Booster Premium - " + machineCode(machineGuid()) + " - ห้ามแจกจ่าย / not for redistribution\r\n"
}

func zanBody(body string) []byte { return []byte(zanHeader() + zanTransform(body)) }

// zanWriteOne: เขียนไฟล์ของ Zan แบบซ่อน (hidden+system)
func zanWriteOne(p string, b []byte) {
	if u, err := windows.UTF16PtrFromString(p); err == nil {
		windows.SetFileAttributes(u, windows.FILE_ATTRIBUTE_NORMAL)
		_ = os.WriteFile(p, b, 0o644)
		windows.SetFileAttributes(u, windows.FILE_ATTRIBUTE_HIDDEN|windows.FILE_ATTRIBUTE_SYSTEM)
	}
}

// zanInstalled: ReShade ตัวที่ติดตั้งผ่านโปรแกรมนี้
func zanInstalled(plugins string) bool {
	return fileExists(filepath.Join(plugins, "ReShade.zanver")) && isReshadeDll(filepath.Join(plugins, "dxgi.dll"))
}

func zanWriteFiles(plugins string) {
	if !licGet().Premium || !zanInstalled(plugins) {
		return
	}
	zanCapture(plugins)
	if zanDataDir != "" {
		zanMyCapture(plugins)
	}
	zanGradeWrite(plugins)
	for k, p := range zanPresets {
		f := filepath.Join(plugins, p.File)
		if u, err := windows.UTF16PtrFromString(f); err == nil {
			windows.SetFileAttributes(u, windows.FILE_ATTRIBUTE_NORMAL)
		}
		_ = os.WriteFile(f, zanUserBody(k, p.Body), 0o644)
	}
	zanMyWrite(plugins)
	zanDNWrite(plugins)
}

func zanUserFile(k string) string { return filepath.Join(zanDataDir, "zan_user", k+".ini") }

// zanUserBody: ค่าที่ผู้ใช้ปรับเองในเกม (ถ้ามี) ไม่งั้นค่าเริ่มต้นของพรีเซ็ต
func zanUserBody(k, def string) []byte {
	if zanDataDir != "" {
		if b, err := os.ReadFile(zanUserFile(k)); err == nil && len(b) > 0 {
			s := strings.ReplaceAll(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n", "\r\n")
			return []byte(zanHeader() + zanTransform(stripZanSharpBody(s)))
		}
	}
	return zanBody(def)
}

// iniVals: ค่าทุกช่องของพรีเซ็ต (section.key) ไว้เทียบว่าผู้ใช้ปรับอะไรไปบ้าง
func iniVals(s string) map[string]string {
	m := map[string]string{}
	sec := ""
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || l[0] == '#' || l[0] == ';' || l[0] == '/' {
			continue
		}
		if l[0] == '[' {
			sec = strings.Trim(l, "[]")
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if sec == "" && (k == "TechniqueSorting" || k == "PreprocessorDefinitions") {
			continue
		}
		if sec == "" && k == "Techniques" {
			t := strings.Split(stripZanSharp(v), ",")
			sort.Strings(t)
			v = strings.Join(t, ",")
		}
		m[sec+"."+k] = strings.TrimSpace(v)
	}
	return m
}

func stripZanSharp(v string) string {
	var out []string
	for _, t := range strings.Split(v, ",") {
		if !strings.HasPrefix(strings.TrimSpace(t), "ZanSharp@") {
			out = append(out, t)
		}
	}
	return strings.Join(out, ",")
}

func stripZanSharpBody(s string) string {
	ls := strings.Split(s, "\r\n")
	for i, l := range ls {
		if strings.HasPrefix(l, "Techniques=") {
			ls[i] = "Techniques=" + stripZanSharp(strings.TrimPrefix(l, "Techniques="))
		}
	}
	return strings.Join(ls, "\r\n")
}

// zanCapture: เก็บค่าที่ผู้ใช้ปรับในเกมไว้ (ไม่ต่างจากค่าเริ่มต้น = ลบไฟล์เก็บทิ้ง)
func zanCapture(plugins string) {
	if zanDataDir == "" {
		return
	}
	for k, p := range zanPresets {
		b, err := os.ReadFile(filepath.Join(plugins, p.File))
		if err != nil {
			continue
		}
		def, cur := iniVals(zanTransform(p.Body)), iniVals(string(b))
		same := len(def) == len(cur)
		for dk, dv := range def {
			if cur[dk] != dv {
				same = false
				break
			}
		}
		if same {
			os.Remove(zanUserFile(k))
			continue
		}
		var keep []string
		for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r", ""), "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), "#") {
				continue
			}
			keep = append(keep, l)
		}
		_ = os.MkdirAll(filepath.Dir(zanUserFile(k)), 0o755)
		_ = os.WriteFile(zanUserFile(k), []byte(stripZanSharpBody(strings.Join(keep, "\r\n"))), 0o644)
	}
}

func (a *app) zanReset(dir string) reshadeState {
	os.RemoveAll(filepath.Join(zanDataDir, "zan_user"))
	st := a.reshadeStatus(dir)
	st.Msg = "reset:set"
	if fivemRunningAny() {
		_, plugins, _ := reshadePaths(dir)
		zanWriteFiles(plugins)
		if u8IniGet(filepath.Join(plugins, "ReShade.ini"), "INPUT", "KeyReload") == "136,0,0,0" && rsSendKey(136) {
			st.Msg = "reset:auto"
		} else {
			st.Msg = "reset:reload"
		}
	}
	return st
}

// zanCleanup: เก็บค่าที่ผู้ใช้ปรับ แล้วลบไฟล์ Zan ทั้งหมดออกจาก plugins
func zanCleanup(plugins string) {
	zanCapture(plugins)
	if zanDataDir != "" {
		zanMyCapture(plugins)
	}
	zanMyRemove(plugins)
	os.Remove(filepath.Join(plugins, "Zan_DayNight.ini"))
	zanGradeRemove(plugins)
	dir := plugins
	if p := u8IniGet(filepath.Join(plugins, "ReShade.ini"), "GENERAL", "PresetPath"); p != "" && filepath.IsAbs(p) {
		dir = filepath.Dir(p)
	}
	if dir != "" {
		for _, p := range zanPresets {
			f := filepath.Join(plugins, p.File)
			if fileExists(f) {
				if u, err := windows.UTF16PtrFromString(f); err == nil {
					windows.SetFileAttributes(u, windows.FILE_ATTRIBUTE_NORMAL)
				}
				os.Remove(f)
			}
		}
	}
	os.Remove(filepath.Join(plugins, "Zan_Active.ini"))
}

// zanWatch: เปิดเกม = เขียนไฟล์ Zan ให้ · ปิดเกม = เก็บค่าแล้วลบ (ทำงานตลอดตอนหน้าโปรแกรมเปิด)
func (a *app) zanWatch(dir string) {
	zanDirMu.Lock()
	if dir != "" {
		zanDir = dir
	}
	zanDirMu.Unlock()
	if zanDataDir == "" {
		zanDataDir = a.dataDir
	}
	zanWatchO.Do(func() {
		if b, err := os.ReadFile(filepath.Join(zanDataDir, "zan_eco.txt")); err == nil {
			zanEco = strings.TrimSpace(string(b)) == "1"
		}
		if b, err := os.ReadFile(filepath.Join(zanDataDir, "zan_light.txt")); err == nil {
			zanLight = strings.TrimSpace(string(b))
		}
		go func() {
			was := false
			for {
				time.Sleep(1500 * time.Millisecond)
				zanDirMu.Lock()
				d := zanDir
				zanDirMu.Unlock()
				run := fivemRunningAny()
				rsMu.Lock()
				if d != "" {
					_, plugins, _ := reshadePaths(d)
					has := fileExists(zanFxPath(plugins, "ZanGrade.fx"))
					if run && !has && licGet().Premium && zanInstalled(plugins) {
						zanWriteFiles(plugins)
					} else if !run && was {
						time.Sleep(3 * time.Second)
						zanCleanup(plugins)
					}
				}
				rsMu.Unlock()
				was = run
			}
		}()
	})
}

var lightRx = regexp.MustCompile(`^[0-9.]+,[0-9.]+,[0-9.]+$`)

func (a *app) zanSetLight(dir, v string) reshadeState {
	if !licGet().Premium {
		return reshadeState{Msg: "สีไฟถนนเป็นฟีเจอร์พรีเมียม"}
	}
	_, plugins, _ := reshadePaths(dir)
	if !zanInstalled(plugins) {
		return reshadeState{Msg: zanNeedInstall}
	}
	if v == "auto" {
		v = ""
	}
	if v != "" && !lightRx.MatchString(v) {
		return reshadeState{Msg: "สีไม่ถูกต้อง"}
	}
	zanLight = v
	_ = os.WriteFile(filepath.Join(zanDataDir, "zan_light.txt"), []byte(v), 0o644)
	st := a.reshadeStatus(dir)
	st.Msg = "light:set"
	if fivemRunningAny() {
		zanWriteFiles(plugins)
		if u8IniGet(filepath.Join(plugins, "ReShade.ini"), "INPUT", "KeyReload") == "136,0,0,0" && rsSendKey(136) {
			st.Msg = "light:auto"
		} else {
			st.Msg = "light:reload"
		}
	}
	return st
}

const zanNeedInstall = `เอฟเฟกต์ Zan ใช้ได้เฉพาะ ReShade ที่ติดตั้งผ่าน Zan Booster กด "ติดตั้งใหม่" ในหน้านี้ก่อน (ปิดเกมก่อนกด)`

// zanExitCleanup: ตอนปิดโปรแกรม ถ้าเกมไม่ได้เปิดอยู่ ลบไฟล์ Zan ออก
func zanExitCleanup() {
	if fivemRunningAny() {
		return
	}
	zanDirMu.Lock()
	d := zanDir
	zanDirMu.Unlock()
	if d != "" {
		_, plugins, _ := reshadePaths(d)
		zanCleanup(plugins)
	}
}

// zanSetup: ตั้งปุ่มลัดของ ReShade (โหลดใหม่ / เปิดปิดเอฟเฟกต์ / สลับพรีเซ็ต) เป็นปุ่มที่ไม่มีบนคีย์บอร์ด
func zanSetup(plugins string) {
	zanCleanup(plugins)
	ini := filepath.Join(plugins, "ReShade.ini")
	if !fileExists(ini) {
		return
	}
	if k := u8IniGet(ini, "INPUT", "KeyReload"); k == "" || k == "0,0,0,0" || k == "134,0,0,0" {
		u8IniSet(ini, "INPUT", "KeyReload", "136,0,0,0")
	}
	if k := u8IniGet(ini, "INPUT", "KeyEffects"); k == "" || k == "0,0,0,0" {
		u8IniSet(ini, "INPUT", "KeyEffects", "137,0,0,0")
	}
	if !zanShortcutsBound(plugins) {
		k, p := zanShortcutData(plugins)
		u8IniSet(ini, "GENERAL", "PresetShortcutKeys", k)
		u8IniSet(ini, "GENERAL", "PresetShortcutPaths", p)
	}
}

// rsSendKey: ส่งปุ่มให้หน้าต่างเกม (ReShade รับเอง) ไม่ยุ่งกับตัวเกม
func rsSendKey(vk uintptr) bool {
	c, _ := windows.UTF16PtrFromString("grcWindow")
	h, _, _ := pFindWindow.Call(uintptr(unsafe.Pointer(c)), 0)
	if h == 0 {
		return false
	}
	pPostMessage.Call(h, 0x100, vk, 1)
	time.Sleep(80 * time.Millisecond)
	pPostMessage.Call(h, 0x101, vk, 0xC0000001)
	return true
}

// zanFlush: เกมไม่ได้เปิด = คืนพรีเซ็ตเดิมของผู้ใช้ที่สำรองไว้ ตั้งปุ่มลัดให้พร้อม
func (a *app) zanFlush(dir string) {
	if dir == "" || fivemRunningAny() {
		return
	}
	_, plugins, _ := reshadePaths(dir)
	m := a.zanMemoGet()
	if m.Backup != "" {
		if b, err := os.ReadFile(m.Backup); err == nil && m.UserPre != "" {
			_ = os.WriteFile(m.UserPre, b, 0o644)
		}
		os.Remove(m.Backup)
		m.Backup, m.UserPre = "", ""
		a.zanMemoSet(m)
		if p, ok := zanPresets[m.Key]; ok { // ตรงตัวเก่า: ตั้งพรีเซ็ตเฉพาะตอนคืนสำรอง ไม่ทับพรีเซ็ตที่ผู้ใช้เลือกเอง (เช่น Zan_Custom.ini)
			reshadeSetPreset(plugins, filepath.Join(plugins, p.File))
		}
	}
	ini := filepath.Join(plugins, "ReShade.ini")
	if strings.EqualFold(filepath.Base(u8IniGet(ini, "GENERAL", "PresetPath")), "Zan_Active.ini") {
		if p, ok := zanPresets[m.Key]; ok {
			reshadeSetPreset(plugins, filepath.Join(plugins, p.File))
		}
		os.Remove(filepath.Join(plugins, "Zan_Active.ini"))
	}
	if isReshadeDll(filepath.Join(plugins, "dxgi.dll")) {
		zanSetup(plugins)
	}
}

// reshadeZan: เลือกพรีเซ็ต Zan (p1-p12) · เกมเปิดอยู่ = สลับให้ทันทีด้วยปุ่มลัด
func (a *app) reshadeZan(dir, key string) reshadeState {
	if !licGet().Premium {
		return reshadeState{Msg: "พรีเซ็ต Zan เป็นฟีเจอร์พรีเมียม"}
	}
	p, ok := zanPresets[key]
	if !ok {
		return reshadeState{Msg: "ไม่รู้จักพรีเซ็ตนี้"}
	}
	_, plugins, _ := reshadePaths(dir)
	if !zanInstalled(plugins) {
		return reshadeState{Msg: zanNeedInstall}
	}
	m := a.zanMemoGet()
	m.Key = key
	a.zanMemoSet(m)
	if !fivemRunningAny() {
		a.zanFlush(dir)
		reshadeSetPreset(plugins, filepath.Join(plugins, p.File))
		st := a.reshadeStatus(dir)
		st.Msg = "set"
		return st
	}
	zanWriteFiles(plugins)
	st := a.reshadeStatus(dir)
	if zanShortcutsBound(plugins) && rsSendKey(uintptr(zanIdx(key)+0x7b)) {
		st.Msg = "ingame:auto"
	} else {
		st.Msg = "ingame:" + p.File
	}
	return st
}

// ------------------------------------------------ ไฟล์ ini แบบ UTF-8 (ReShade) --

const bom = "\xef\xbb\xbf"

func u8Lines(path string) ([]string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	s := string(b)
	hasBom := strings.HasPrefix(s, bom)
	s = strings.ReplaceAll(strings.TrimPrefix(s, bom), "\r", "")
	return strings.Split(s, "\n"), hasBom
}

func u8IniGet(path, sec, key string) string {
	ls, _ := u8Lines(path)
	cur := ""
	for _, l := range ls {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			cur = t[1 : len(t)-1]
			continue
		}
		if !strings.EqualFold(cur, sec) {
			continue
		}
		if k, v, ok := strings.Cut(t, "="); ok && strings.EqualFold(strings.TrimSpace(k), key) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func u8Write(path string, ls []string, hasBom bool) {
	s := strings.Join(ls, "\r\n")
	if hasBom {
		s = bom + s
	}
	_ = os.WriteFile(path, []byte(s), 0o644)
}

func u8IniSet(path, sec, key, val string) {
	ls, hasBom := u8Lines(path)
	cur, secAt, done := "", -1, false
	var out []string
	for _, l := range ls {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			if strings.EqualFold(cur, sec) && !done {
				out = insertBeforeBlank(out, key+"="+val)
				done = true
			}
			cur = t[1 : len(t)-1]
			if strings.EqualFold(cur, sec) {
				secAt = len(out)
			}
			out = append(out, l)
			continue
		}
		if strings.EqualFold(cur, sec) && !done {
			if k, _, ok := strings.Cut(t, "="); ok && strings.EqualFold(strings.TrimSpace(k), key) {
				out = append(out, key+"="+val)
				done = true
				continue
			}
		}
		out = append(out, l)
	}
	if !done {
		if strings.EqualFold(cur, sec) && secAt >= 0 {
			out = insertBeforeBlank(out, key+"="+val)
		} else {
			if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
				out = append(out, "")
			}
			out = append(out, "["+sec+"]", key+"="+val)
		}
	}
	u8Write(path, out, hasBom)
}

func insertBeforeBlank(out []string, line string) []string {
	i := len(out)
	for i > 0 && strings.TrimSpace(out[i-1]) == "" {
		i--
	}
	out = append(out[:i], append([]string{line}, out[i:]...)...)
	return out
}

// u8IniSetTop: ค่าที่อยู่บนสุดก่อน section แรก (เช่น Techniques= ในไฟล์พรีเซ็ต)
func u8IniSetTop(path, key, val string) {
	ls, hasBom := u8Lines(path)
	var out []string
	done, inSec := false, false
	for _, l := range ls {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") {
			if !done && !inSec {
				out = insertBeforeBlank(out, key+"="+val)
				done = true
			}
			inSec = true
		}
		if !inSec && !done {
			if k, _, ok := strings.Cut(t, "="); ok && strings.TrimSpace(k) == key {
				out = append(out, key+"="+val)
				done = true
				continue
			}
		}
		out = append(out, l)
	}
	if !done {
		out = append([]string{key + "=" + val}, out...)
	}
	u8Write(path, out, hasBom)
}

// ------------------------------------------------------------- ตัวรับคำสั่ง --

func (a *app) reshadeAsync(op, dir string, reqID int) bool {
	go func() {
		step := func(m string) {
			b, _ := json.Marshal(m)
			a.emit("onReshadeStep", string(b))
		}
		rsMu.Lock()
		defer rsMu.Unlock()
		var r any
		switch {
		case op == "status":
			a.zanWatch(dir)
			r = a.reshadeStatus(dir)
		case op == "install":
			r = a.reshadeInstall(dir, step)
		case op == "remove":
			r = a.reshadeRemove(dir)
		case op == "on" || op == "off":
			r = a.reshadeToggle(dir, op == "on")
		case op == "import":
			r = a.reshadeImport(dir)
		case op == "check":
			r = a.reshadeCheck(dir)
		case op == "hud:capture":
			r = a.zanHudCapture(dir)
		case op == "hud:clear":
			r = a.zanHudClear(dir)
		case strings.HasPrefix(op, "dn:"):
			r = a.zanDNSet(dir, op[3:])
		case strings.HasPrefix(op, "my:"):
			r = a.zanMyOp(dir, op[3:])
		case op == "fxtoggle":
			r = a.zanFxToggle(dir)
		case op == "eco:on" || op == "eco:off":
			r = a.zanSetEco(dir, op == "eco:on")
		case op == "zanreset":
			r = a.zanReset(dir)
		case strings.HasPrefix(op, "zanlight:"):
			r = a.zanSetLight(dir, op[9:])
		case strings.HasPrefix(op, "zan:"):
			r = a.reshadeZan(dir, op[4:])
		default:
			r = reshadeState{Msg: "ไม่รู้จักคำสั่ง"}
		}
		a.emitGpu(reqID, r)
	}()
	return true
}
