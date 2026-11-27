//go:build windows

package main

// zanextra.go: ถอดจาก exe 4.11.15 · พรีเซ็ตของฉัน 3 ช่อง (บันทึก/ใช้/ลบ/แชร์เป็นโค้ด ZAN1:) · โหมดประหยัด · ปุ่มเปิดปิดเอฟเฟกต์ในเกม

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func zanMyBodyPath(i int) string {
	return filepath.Join(zanDataDir, "zan_my", strconv.Itoa(i)+".ini")
}

func zanMyNamePath(i int) string {
	return filepath.Join(zanDataDir, "zan_my", strconv.Itoa(i)+".name")
}

func zanMyFile(plugins string, i int) string {
	return filepath.Join(plugins, "Zan_My_"+strconv.Itoa(i)+".ini")
}

// zanMySlot: "m2" -> 2 (ช่อง 1-3 · ไม่ถูกต้อง = 0)
func zanMySlot(s string) int {
	n := atoi(strings.TrimPrefix(s, "m"))
	if n < 1 || n > 3 {
		return 0
	}
	return n
}

// zanTransform: ใส่สีไฟถนนที่เลือก + โหมดประหยัด (ปิดเอฟเฟกต์ที่กินแรง ลดแสงฟุ้งครึ่งหนึ่ง)
func zanTransform(body string) string {
	body = zanApplyLight(body)
	if !zanEco {
		return body
	}
	ls := strings.Split(body, "\r\n")
	for i, l := range ls {
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		switch k {
		case "AO", "Aurora", "LightGlow", "SunShafts":
			ls[i] = k + "=" + f6(0)
		case "Bloom":
			ls[i] = k + "=" + f6(atof(v)*0.5)
		}
	}
	return strings.Join(ls, "\r\n")
}

func atof(s string) float64 { f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64); return f }

// stripComments: ตัดบรรทัด # (หัวไฟล์ลายน้ำ) ออก
func stripComments(s string) string {
	var keep []string
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "#") {
			continue
		}
		keep = append(keep, l)
	}
	return stripZanSharpBody(strings.TrimSpace(strings.Join(keep, "\r\n"))) + "\r\n"
}

func zanWatermark() string { return zanHeader() }

func zanMyWrite(plugins string) {
	for i := 1; i <= 3; i++ {
		if b, err := os.ReadFile(zanMyBodyPath(i)); err == nil {
			zanWriteOne(zanMyFile(plugins, i), []byte(zanWatermark()+zanTransform(string(b))))
		}
	}
}

// zanMyCapture: ค่าที่ปรับในเกมของพรีเซ็ตของฉัน เก็บกลับเข้าช่อง
func zanMyCapture(plugins string) {
	for i := 1; i <= 3; i++ {
		b, err := os.ReadFile(zanMyFile(plugins, i))
		if err != nil || !fileExists(zanMyBodyPath(i)) {
			continue
		}
		_ = os.WriteFile(zanMyBodyPath(i), []byte(stripComments(string(b))), 0o644)
	}
}

func zanMyRemove(plugins string) {
	for i := 1; i <= 3; i++ {
		f := zanMyFile(plugins, i)
		zanWriteOne(f, nil)
		os.Remove(f)
	}
}

type myPreset struct {
	Slot string `json:"slot"`
	Name string `json:"name"`
}

func zanMyList() []myPreset {
	var out []myPreset
	for i := 1; i <= 3; i++ {
		if !fileExists(zanMyBodyPath(i)) {
			continue
		}
		n := ""
		if b, err := os.ReadFile(zanMyNamePath(i)); err == nil {
			n = strings.TrimSpace(string(b))
		}
		if n == "" {
			n = "พรีเซ็ตของฉัน " + strconv.Itoa(i)
		}
		out = append(out, myPreset{"m" + strconv.Itoa(i), n})
	}
	return out
}

// zanMyOp: save:mN:ชื่อ · apply:mN · del:mN · export:mN · import:mN:ZAN1:...
func (a *app) zanMyOp(dir, arg string) reshadeState {
	if !licGet().Premium {
		return reshadeState{Msg: "พรีเซ็ตของฉันเป็นฟีเจอร์พรีเมียม"}
	}
	_, plugins, _ := reshadePaths(dir)
	if !zanInstalled(plugins) {
		return reshadeState{Msg: zanNeedInstall}
	}
	parts := strings.SplitN(arg, ":", 3)
	op := parts[0]
	slot := 0
	if len(parts) > 1 {
		slot = zanMySlot(parts[1])
	}
	if slot == 0 {
		return reshadeState{Msg: "ช่องพรีเซ็ตไม่ถูกต้อง"}
	}
	if zanDataDir == "" {
		zanDataDir = a.dataDir
	}
	_ = os.MkdirAll(filepath.Join(zanDataDir, "zan_my"), 0o755)
	switch op {
	case "save":
		cur := u8IniGet(filepath.Join(plugins, "ReShade.ini"), "GENERAL", "PresetPath")
		var body string
		if b, err := os.ReadFile(cur); err == nil && fivemRunningAny() {
			body = stripComments(string(b))
		} else {
			for k, p := range zanPresets {
				if strings.EqualFold(filepath.Base(cur), p.File) {
					body = stripComments(string(zanUserBody(k, p.Body)))
				}
			}
		}
		if body == "" || !strings.Contains(body, "Techniques=") {
			return reshadeState{Msg: "ยังไม่ได้เลือกพรีเซ็ต เลือก Zan Preset แล้วปรับในเกมก่อนกดบันทึก"}
		}
		_ = os.WriteFile(zanMyBodyPath(slot), []byte(body), 0o644)
		name := ""
		if len(parts) > 2 {
			name = strings.TrimSpace(parts[2])
			if r := []rune(name); len(r) > 40 {
				name = string(r[:40])
			}
		}
		_ = os.WriteFile(zanMyNamePath(slot), []byte(name), 0o644)
		if fivemRunningAny() {
			zanMyWrite(plugins)
		}
		st := a.reshadeStatus(dir)
		st.Msg = "my:saved"
		return st
	case "apply":
		if !fileExists(zanMyBodyPath(slot)) {
			return reshadeState{Msg: "ช่องนี้ยังว่าง"}
		}
		f := zanMyFile(plugins, slot)
		if !fivemRunningAny() {
			reshadeSetPreset(plugins, f)
			st := a.reshadeStatus(dir)
			st.Msg = "set"
			return st
		}
		zanMyWrite(plugins)
		st := a.reshadeStatus(dir)
		if zanShortcutsBound(plugins) && rsSendKey(uintptr(0x89+slot)) {
			st.Msg = "ingame:auto"
		} else {
			st.Msg = "ingame:" + filepath.Base(f)
		}
		return st
	case "del":
		os.Remove(zanMyBodyPath(slot))
		os.Remove(zanMyNamePath(slot))
		if f := zanMyFile(plugins, slot); fileExists(f) {
			zanWriteOne(f, nil)
			os.Remove(f)
		}
		st := a.reshadeStatus(dir)
		st.Msg = "my:deleted"
		return st
	case "export":
		b, err := os.ReadFile(zanMyBodyPath(slot))
		if err != nil {
			return reshadeState{Msg: "ช่องนี้ยังว่าง"}
		}
		name := ""
		if n, err := os.ReadFile(zanMyNamePath(slot)); err == nil {
			name = strings.TrimSpace(string(n))
		}
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		zw.Write([]byte(name + "\n" + string(b)))
		zw.Close()
		st := a.reshadeStatus(dir)
		st.Msg = "code:ZAN1:" + base64.StdEncoding.EncodeToString(buf.Bytes())
		return st
	case "import":
		code := ""
		if len(parts) > 2 {
			code = strings.TrimSpace(parts[2])
		}
		code = strings.TrimPrefix(code, "ZAN1:")
		raw, err := base64.StdEncoding.DecodeString(code)
		if err != nil {
			return reshadeState{Msg: "โค้ดพรีเซ็ตไม่ถูกต้อง"}
		}
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return reshadeState{Msg: "โค้ดพรีเซ็ตไม่ถูกต้อง"}
		}
		data, _ := io.ReadAll(io.LimitReader(zr, 1<<20))
		name, body, _ := strings.Cut(string(data), "\n")
		if !strings.Contains(body, "Techniques=") {
			return reshadeState{Msg: "โค้ดพรีเซ็ตไม่ถูกต้อง"}
		}
		_ = os.WriteFile(zanMyBodyPath(slot), []byte(stripComments(body)), 0o644)
		_ = os.WriteFile(zanMyNamePath(slot), []byte(strings.TrimSpace(name)), 0o644)
		if fivemRunningAny() {
			zanMyWrite(plugins)
		}
		st := a.reshadeStatus(dir)
		st.Msg = "my:imported"
		return st
	}
	return reshadeState{Msg: "ไม่รู้จักคำสั่ง"}
}

// zanFxToggle: เปิด/ปิดเอฟเฟกต์ทั้งหมดในเกม (ปุ่ม KeyEffects ของ ReShade)
func (a *app) zanFxToggle(dir string) reshadeState {
	_, plugins, _ := reshadePaths(dir)
	st := a.reshadeStatus(dir)
	if !fivemRunningAny() {
		st.Msg = "fx:nogame"
		return st
	}
	if u8IniGet(filepath.Join(plugins, "ReShade.ini"), "INPUT", "KeyEffects") == "137,0,0,0" && rsSendKey(137) {
		st.Msg = "fx:auto"
	} else {
		st.Msg = "fx:restart"
	}
	return st
}

func (a *app) zanSetEco(dir string, on bool) reshadeState {
	zanEco = on
	if zanDataDir == "" {
		zanDataDir = a.dataDir
	}
	v := "0"
	if on {
		v = "1"
	}
	_ = os.WriteFile(filepath.Join(zanDataDir, "zan_eco.txt"), []byte(v), 0o644)
	_, plugins, _ := reshadePaths(dir)
	st := a.reshadeStatus(dir)
	st.Msg = "eco:set"
	if fivemRunningAny() && zanInstalled(plugins) {
		zanWriteFiles(plugins)
		zanMyWrite(plugins)
		if u8IniGet(filepath.Join(plugins, "ReShade.ini"), "INPUT", "KeyReload") == "136,0,0,0" && rsSendKey(136) {
			st.Msg = "eco:auto"
		} else {
			st.Msg = "eco:reload"
		}
	}
	return st
}
