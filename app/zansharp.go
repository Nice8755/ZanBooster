//go:build windows

package main

// zansharp.go: ถอดจาก exe 4.11.15 · ZanSharp.fx เอฟเฟกต์ลับภาพของ Zan Upscale (สร้างจากแม่แบบ ตั้งค่าจากโปรแกรม)

import (
	"fmt"
	"os"
	"path/filepath"
)

var zanSharpLevels = map[string]struct{ Sharp, Clarity float64 }{
	"perf":    {1.0, 0.7},
	"quality": {1.0, 0.7},
	"ultra":   {1.0, 0.7},
}

// writeZanSharp: เขียน ZanSharp.fx ตามโหมด (ไม่รู้จักโหมด = ปิดเอฟเฟกต์) · true = ไฟล์เปลี่ยน
func writeZanSharp(plugins, mode string) bool {
	if !dirExists(filepath.Join(plugins, "reshade-shaders")) || !zanInstalled(plugins) {
		return false
	}
	lv, on := zanSharpLevels[mode]
	p := zanFxPath(plugins, "ZanSharp.fx")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	zgLoad()
	flag, en := "0", "false"
	if on {
		flag, en = "1", "true"
	}
	s := fmt.Sprintf(zsTmpl, flag, f6(lv.Sharp), f6(lv.Clarity), en)
	if b, err := os.ReadFile(p); err == nil && string(b) == s {
		return false
	}
	_ = os.WriteFile(p, []byte(s), 0o644)
	return true
}
