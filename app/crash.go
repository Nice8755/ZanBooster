//go:build windows

package main

// crash.go (4.20.07): เกมเด้ง → อ่านไฟล์ที่ FiveM เขียนไว้ (crashes\*.dmp + logs\CitizenFX_log_*.log) แล้วบอกสาเหตุที่พบบ่อย
// อ่านอย่างเดียว ไม่แก้ไฟล์เกม · เดาสาเหตุจากรหัสข้อผิดพลาดในบันทึกเท่านั้น ไม่เจอ = บอกตรง ๆ ว่าไม่รู้

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type crashInfo struct {
	At   int64  `json:"at"`             // เวลาเด้ง (unix) 0 = ไม่เคยเด้ง
	Code string `json:"code,omitempty"` // รหัสที่เจอในบันทึก
	Kind string `json:"kind"`           // mem / gpu / cache / script / unknown
}

var crashRules = []struct {
	kind string
	rx   *regexp.Regexp
}{
	{"mem", regexp.MustCompile(`(?i)ERR_MEM_[A-Z_]+|out of memory|outofmemory|pool full|ERR_MEM`)},
	{"gpu", regexp.MustCompile(`(?i)ERR_GFX_[A-Z_0-9]+|DXGI_ERROR_DEVICE_(REMOVED|HUNG|RESET)|device (removed|lost)|nvwgf2umx|nvlddmkm|atidxx64|amdxx64`)},
	{"cache", regexp.MustCompile(`(?i)ERR_STR_[A-Z_]+|ERR_FIL_[A-Z_]+|corrupt|failed to (load|read) .*\.(rpf|ytd|ydr|yft)`)},
	{"script", regexp.MustCompile(`(?i)citizen-scripting[-a-z]*\.dll|SCRIPT ERROR|resource [^ ]+ crashed`)},
}

func newestFile(dir, pat string) (string, time.Time) {
	var best string
	var bt time.Time
	m, _ := filepath.Glob(filepath.Join(dir, pat))
	for _, f := range m {
		if fi, err := os.Stat(f); err == nil && fi.ModTime().After(bt) {
			best, bt = f, fi.ModTime()
		}
	}
	return best, bt
}

func tailText(p string, n int64) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.Size() > n {
		f.Seek(-n, io.SeekEnd)
	}
	b, _ := io.ReadAll(f)
	return string(b)
}

// crashCheck: เด้งครั้งล่าสุดของ FiveM ในโฟลเดอร์นี้ (JSON)
func (a *app) crashCheck(dir string) string {
	out := crashInfo{Kind: "unknown"}
	if dir != "" {
		appDir := filepath.Join(dir, "FiveM.app")
		_, at := newestFile(filepath.Join(appDir, "crashes"), "*.dmp")
		if !at.IsZero() {
			out.At = at.Unix()
			// บันทึกที่เขียนช่วงเดียวกับตอนเด้ง
			if lg, lt := newestFile(filepath.Join(appDir, "logs"), "CitizenFX_log_*.log"); lg != "" && lt.After(at.Add(-6*time.Hour)) {
				txt := tailText(lg, 256<<10)
				if i := strings.LastIndex(strings.ToLower(txt), "crash"); i > 0 {
					txt = txt[max(0, i-20000):]
				}
				for _, r := range crashRules {
					if m := r.rx.FindString(txt); m != "" {
						out.Kind, out.Code = r.kind, strings.TrimSpace(m)
						break
					}
				}
			}
		}
	}
	b, _ := json.Marshal(out)
	return string(b)
}
