//go:build windows

package main

// combat.go: ถอดจาก exe 4.11.15 · ค่าเกมสำหรับตี/ยิง: ปิดภาพเบลอตอนหันกล้อง เบลอฉากหลัง และ VSync
// ใน gta5_settings.xml (แก้เฉพาะ 3 ค่านี้ เก็บไฟล์เดิมไว้ที่ ProgramData\ZanBooster\gta5_settings.original.xml)

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var combatKeys = []struct {
	Key, Good, Label string
	Bad              func(v string) bool
}{
	{"MotionBlurStrength", "0.000000", "เบลอตอนหันกล้อง", func(v string) bool { f, _ := strconv.ParseFloat(v, 64); return f > 0 }},
	{"DoF", "false", "เบลอฉากหลัง (DoF)", func(v string) bool { return strings.EqualFold(v, "true") }},
	{"VSync", "0", "VSync", func(v string) bool { return v != "" && v != "0" }},
}

func gfxFile() string { return filepath.Join(os.Getenv("APPDATA"), "CitizenFX", "gta5_settings.xml") }

func combatRe(key string) *regexp.Regexp {
	return regexp.MustCompile(`(<` + key + `\s+value=")([^"]*)(")`)
}

// combatBad: ชื่อค่าที่ยังเปิดอยู่ · ok=false ถ้ายังไม่มีไฟล์ตั้งค่าของเกม
func combatBad() (bad []string, ok bool) {
	b, err := os.ReadFile(gfxFile())
	if err != nil {
		return nil, false
	}
	for _, k := range combatKeys {
		if m := combatRe(k.Key).FindSubmatch(b); len(m) > 2 && k.Bad(string(m[2])) {
			bad = append(bad, k.Label)
		}
	}
	return bad, true
}

func (a *app) combatItem() (healthItem, bool) {
	bad, ok := combatBad()
	if !ok {
		return healthItem{}, false
	}
	it := healthItem{ID: "combat", Title: "ค่าเกมสำหรับตี / ยิง"}
	if len(bad) == 0 {
		it.Status, it.Detail = "ok", "ไม่มีภาพเบลอ ปิด VSync แล้ว"
		return it, true
	}
	it.Status, it.Detail = "warn", "ยังเปิดอยู่: "+strings.Join(bad, " · ")
	it.Fix = "ภาพเบลอทำให้มองคนไม่ชัด VSync ทำให้ตีช้าลง (ปุ่มนี้ไม่แตะค่ากราฟิกอื่น) · ตั้งในเกมเพิ่ม: Mouse Input Method = Raw Input"
	it.Act, it.ActTxt = "combat", "แก้ให้"
	return it, true
}

func (a *app) combatFix() string {
	if fivemRunningAny() {
		return "ปิด FiveM ก่อน เกมจะเขียนทับไฟล์ตอนปิด"
	}
	f := gfxFile()
	b, err := os.ReadFile(f)
	if err != nil {
		return "ยังไม่มีไฟล์ตั้งค่าของเกม เข้า FiveM สักครั้งก่อน"
	}
	orig := filepath.Join(a.dataDir, "gta5_settings.original.xml")
	if _, err := os.Stat(orig); err != nil {
		_ = os.WriteFile(orig, b, 0o644)
	}
	for _, k := range combatKeys {
		b = combatRe(k.Key).ReplaceAll(b, []byte("${1}"+k.Good+"${3}"))
	}
	if err := os.WriteFile(f, b, 0o644); err != nil {
		return "เขียนไฟล์ไม่ได้: " + err.Error()
	}
	return ""
}
