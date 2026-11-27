package main

// หาโฟลเดอร์ FiveM อัตโนมัติ (เร็ว ไม่ต้องรอ PowerShell)
// ลำดับ: ค่าที่เคยเจอ → ลิงก์ fivem:// ที่ FiveM ลงทะเบียนไว้ → เกมที่เปิดอยู่ → LocalAppData ทุก user
//        → รายการโปรแกรมที่ติดตั้ง → โฟลเดอร์ยอดนิยม (Desktop/Documents/Downloads) → สแกนทุกไดรฟ์ลึก 3 ชั้น

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var findMu sync.Mutex

func (a *app) configPath() string { return filepath.Join(a.dataDir, "config.json") }

func readConfig(path string) map[string]any {
	m := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		json.Unmarshal(bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}), &m)
	}
	return m
}

// fivemRoot รับ path อะไรก็ได้ (FiveM.exe / FiveM.app / โฟลเดอร์ติดตั้ง / คำสั่งใน registry) คืนโฟลเดอร์ที่มี FiveM.exe
var exeInCmd = regexp.MustCompile(`(?i)^\s*"?([^"]+?\.exe)`)

func fivemRoot(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if m := exeInCmd.FindStringSubmatch(p); m != nil {
		p = m[1]
	}
	p = os.ExpandEnv(strings.Trim(p, `"`))
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		p = filepath.Dir(p)
	}
	for i := 0; i < 4 && p != "" && p != filepath.Dir(p); i++ {
		if strings.EqualFold(filepath.Base(p), "FiveM.app") {
			p = filepath.Dir(p)
		}
		if fileExists(filepath.Join(p, "FiveM.exe")) && dirExists(filepath.Join(p, "FiveM.app")) {
			return p
		}
		p = filepath.Dir(p)
	}
	return ""
}

func fileExists(p string) bool { st, err := os.Stat(p); return err == nil && !st.IsDir() }
func dirExists(p string) bool  { st, err := os.Stat(p); return err == nil && st.IsDir() }

func regDefault(root registry.Key, path string) string {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, _ := k.GetStringValue("")
	return v
}

func runningFiveMPath() string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err := windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if !strings.HasPrefix(strings.ToLower(windows.UTF16ToString(pe.ExeFile[:])), "fivem") {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pe.ProcessID)
		if err != nil {
			continue
		}
		buf := make([]uint16, 1024)
		n := uint32(len(buf))
		err = windows.QueryFullProcessImageName(h, 0, &buf[0], &n)
		windows.CloseHandle(h)
		if err == nil {
			if r := fivemRoot(windows.UTF16ToString(buf[:n])); r != "" {
				return r
			}
		}
	}
	return ""
}

func userDirs() []string {
	var out []string
	sys := os.Getenv("SystemDrive")
	if sys == "" {
		sys = "C:"
	}
	es, _ := os.ReadDir(sys + `\Users`)
	for _, e := range es {
		if e.IsDir() {
			out = append(out, filepath.Join(sys+`\Users`, e.Name()))
		}
	}
	return out
}

var skipScan = map[string]bool{"windows": true, "$recycle.bin": true, "system volume information": true, "recovery": true, "programdata": true, "$windows.~bt": true, "$windows.~ws": true, "msocache": true, "perflogs": true}

// scanDrives หา FiveM.exe + FiveM.app ในทุกไดรฟ์ ลึกไม่เกิน 3 ชั้น เช่น D:\FiveM, D:\Games\FiveM, E:\Games\Rockstar\FiveM
func scanDrives() string {
	var found atomic.Value
	found.Store("")
	done := func() bool { return found.Load().(string) != "" }
	var wg sync.WaitGroup
	for c := 'C'; c <= 'Z'; c++ {
		root := string(c) + `:\`
		if t := windows.GetDriveType(windows.StringToUTF16Ptr(root)); t != windows.DRIVE_FIXED {
			continue
		}
		wg.Add(1)
		go func(root string) {
			defer wg.Done()
			var walk func(dir string, depth int) bool
			walk = func(dir string, depth int) bool {
				if done() {
					return true
				}
				if depth > 0 && fileExists(filepath.Join(dir, "FiveM.exe")) && dirExists(filepath.Join(dir, "FiveM.app")) {
					found.CompareAndSwap("", dir)
					return true
				}
				if depth >= 3 {
					return false
				}
				es, err := os.ReadDir(dir)
				if err != nil {
					return false
				}
				for _, e := range es {
					if !e.IsDir() || e.Type()&os.ModeSymlink != 0 {
						continue
					}
					n := strings.ToLower(e.Name())
					if depth == 0 && (skipScan[n] || n == "users") {
						continue
					}
					if strings.HasPrefix(n, ".") || n == "node_modules" || n == "windowsapps" {
						continue
					}
					if walk(filepath.Join(dir, e.Name()), depth+1) {
						return true
					}
				}
				return false
			}
			walk(root, 0)
		}(root)
	}
	wg.Wait()
	return found.Load().(string)
}

// findFiveM คืนโฟลเดอร์ที่มี FiveM.exe หรือ "" และจำไว้ใน config.json ให้ engine ใช้ด้วย
func (a *app) findFiveM() string {
	findMu.Lock()
	defer findMu.Unlock()
	cfg := readConfig(a.configPath())
	try := func(p string) string { return fivemRoot(p) }
	cands := []func() string{
		func() string { s, _ := cfg["FiveMPath"].(string); return try(s) },
		func() string {
			for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE, registry.CLASSES_ROOT} {
				for _, k := range []string{`Software\Classes\FiveM.ProtocolHandler\shell\open\command`, `Software\Classes\fivem\shell\open\command`, `FiveM.ProtocolHandler\shell\open\command`, `fivem\shell\open\command`} {
					if r := try(regDefault(root, k)); r != "" {
						return r
					}
				}
			}
			return ""
		},
		runningFiveMPath,
		func() string { return try(filepath.Join(os.Getenv("LOCALAPPDATA"), "FiveM")) },
		func() string {
			for _, u := range userDirs() {
				for _, sub := range []string{`AppData\Local\FiveM`, `Desktop\FiveM`, `Documents\FiveM`, `Downloads\FiveM`, `FiveM`} {
					if r := try(filepath.Join(u, sub)); r != "" {
						return r
					}
				}
			}
			return ""
		},
		func() string {
			for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
				for _, base := range []string{`Software\Microsoft\Windows\CurrentVersion\Uninstall`, `Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`} {
					k, err := registry.OpenKey(root, base, registry.ENUMERATE_SUB_KEYS)
					if err != nil {
						continue
					}
					names, _ := k.ReadSubKeyNames(-1)
					k.Close()
					for _, n := range names {
						sk, err := registry.OpenKey(root, base+`\`+n, registry.QUERY_VALUE)
						if err != nil {
							continue
						}
						dn, _, _ := sk.GetStringValue("DisplayName")
						if strings.Contains(strings.ToLower(dn+n), "fivem") {
							for _, v := range []string{"InstallLocation", "DisplayIcon", "UninstallString"} {
								if s, _, err := sk.GetStringValue(v); err == nil {
									if r := try(s); r != "" {
										sk.Close()
										return r
									}
								}
							}
						}
						sk.Close()
					}
				}
			}
			return ""
		},
		scanDrives,
	}
	for _, f := range cands {
		if r := f(); r != "" {
			app := filepath.Join(r, "FiveM.app")
			if s, _ := cfg["FiveMPath"].(string); !strings.EqualFold(s, app) {
				cfg["FiveMPath"] = app
				if b, err := json.MarshalIndent(cfg, "", "  "); err == nil {
					os.WriteFile(a.configPath(), append([]byte{0xEF, 0xBB, 0xBF}, b...), 0o644)
				}
			}
			return r
		}
	}
	return ""
}

func (a *app) findFiveMAsync(reqID int) bool {
	go func() {
		b, _ := json.Marshal(map[string]any{"reqId": reqID, "path": a.findFiveM()})
		a.emit("onFindFiveM", string(b))
	}()
	return true
}
