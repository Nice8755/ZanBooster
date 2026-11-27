//go:build windows

package main

// shot.go: ถอดจาก exe 4.11.15 · บันทึกการ์ดผล FPS เป็นไฟล์รูป (รูปภาพ\Zan Booster\FPS-วันที่.png) และคัดลอกเข้าคลิปบอร์ด

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

func (a *app) saveShot(dataURL string, cp bool) string {
	res := func(ok bool, path, msg string) string {
		b, _ := json.Marshal(map[string]any{"ok": ok, "path": path, "msg": msg})
		return string(b)
	}
	s := dataURL
	if i := strings.Index(s, ","); i >= 0 && strings.HasPrefix(s, "data:") {
		s = s[i+1:]
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) < 100 {
		return res(false, "", "รูปเสีย")
	}
	dir, err := windows.KnownFolderPath(windows.FOLDERID_Pictures, 0)
	if err != nil || dir == "" {
		h, _ := os.UserHomeDir()
		dir = filepath.Join(h, "Pictures")
	}
	dir = filepath.Join(dir, "Zan Booster")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "FPS-"+time.Now().Format("20060102-150405")+".png")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return res(false, "", err.Error())
	}
	if !cp {
		return res(true, path, "")
	}
	ps := "Add-Type -AssemblyName System.Windows.Forms,System.Drawing; $i=[System.Drawing.Image]::FromFile('" +
		strings.ReplaceAll(path, "'", "''") + "'); [System.Windows.Forms.Clipboard]::SetImage($i); $i.Dispose()"
	c := exec.Command("powershell", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-Command", ps)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := c.Run(); err != nil {
		return res(true, path, "คัดลอกไม่สำเร็จ แต่บันทึกรูปไว้แล้ว")
	}
	return res(true, path, "")
}

func showShot(path string) {
	if path == "" {
		return
	}
	_ = exec.Command("explorer", "/select,", path).Start()
}
