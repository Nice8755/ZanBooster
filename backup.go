package main

// สำรอง / คืนการตั้งค่าเกม เป็นไฟล์ zip ไฟล์เดียว (ย้ายเครื่อง / ลง Windows ใหม่ได้)
//   appdata/CitizenFX/...      กราฟิก FiveM (gta5_settings.xml), ปุ่มคีย์ลัดและค่าในเกม (fivem.cfg)
//   fivemapp/CitizenFX.ini     ค่าของ FiveM (ตอนคืนจะคงที่อยู่ของ GTA V ของเครื่องนี้ไว้)
//   rockstar/Profiles/...      การตั้งค่าปุ่มของ GTA V
//   zan/servers.json, settings.json  รายการเซิร์ฟและการตั้งค่าของ Zan Booster

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

type backupFile struct {
	Name string  `json:"name"`
	KB   float64 `json:"kb"`
}

type backupEntry struct {
	File  string  `json:"file"`
	Time  int64   `json:"time"`
	KB    float64 `json:"kb"`
	Items int     `json:"items"`
	Auto  bool    `json:"auto"`
}

type backupInfo struct {
	OK      bool          `json:"ok"`
	Message string        `json:"m,omitempty"`
	Dir     string        `json:"dir"`
	Items   []backupFile  `json:"items"`
	List    []backupEntry `json:"list"`
	Saved   string        `json:"saved,omitempty"`
	Count   int           `json:"count,omitempty"`
}

func documentsDir() string {
	if p, err := windows.KnownFolderPath(windows.FOLDERID_Documents, 0); err == nil {
		return p
	}
	return filepath.Join(os.Getenv("USERPROFILE"), "Documents")
}

func (a *app) backupDir() string { return filepath.Join(documentsDir(), "ZanBooster Backups") }

// backupRoots: ชื่อในไฟล์ zip -> โฟลเดอร์จริงในเครื่องนี้
func (a *app) backupRoots(fivemDir string) map[string]string {
	r := map[string]string{
		"appdata/CitizenFX": filepath.Join(os.Getenv("APPDATA"), "CitizenFX"),
		"rockstar/Profiles": filepath.Join(documentsDir(), "Rockstar Games", "GTA V", "Profiles"),
		"zan":               a.dataDir,
	}
	if fivemDir != "" {
		r["fivemapp"] = filepath.Join(fivemDir, "FiveM.app")
	}
	return r
}

// ไฟล์ที่เก็บในแต่ละที่ (ไม่เอาแคช/log ที่ใหญ่และสร้างใหม่ได้)
func wantFile(root, rel string) bool {
	l := strings.ToLower(filepath.ToSlash(rel))
	switch root {
	case "appdata/CitizenFX":
		return !strings.Contains(l, "/") && (strings.HasSuffix(l, ".xml") || strings.HasSuffix(l, ".cfg") || strings.HasSuffix(l, ".ini") || strings.HasSuffix(l, ".json"))
	case "fivemapp":
		return l == "citizenfx.ini"
	case "zan":
		return l == "servers.json" || l == "settings.json"
	case "rockstar/Profiles":
		return strings.HasSuffix(l, "pc_settings.bin") || strings.HasSuffix(l, "cfg.dat") || strings.HasSuffix(l, "control_user.xml")
	}
	return false
}

func (a *app) collectBackup(fivemDir string) (map[string]string, []backupFile) {
	files := map[string]string{} // zip path -> real path
	var items []backupFile
	roots := a.backupRoots(fivemDir)
	keys := make([]string, 0, len(roots))
	for k := range roots {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		base := roots[k]
		filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(base, p)
			if !wantFile(k, rel) {
				return nil
			}
			info, err := d.Info()
			if err != nil || info.Size() > 8<<20 {
				return nil
			}
			zp := k + "/" + filepath.ToSlash(rel)
			files[zp] = p
			items = append(items, backupFile{zp, float64(info.Size()) / 1024})
			return nil
		})
	}
	return files, items
}

func (a *app) backupList() []backupEntry {
	es, _ := os.ReadDir(a.backupDir())
	var out []backupEntry
	for _, e := range es {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".zip") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		n := 0
		if zr, err := zip.OpenReader(filepath.Join(a.backupDir(), e.Name())); err == nil {
			n = len(zr.File)
			zr.Close()
		}
		out = append(out, backupEntry{File: e.Name(), Time: info.ModTime().Unix(), KB: float64(info.Size()) / 1024, Items: n, Auto: strings.Contains(e.Name(), "ก่อนคืนค่า") || strings.Contains(e.Name(), "before-restore")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time > out[j].Time })
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

func (a *app) backupInfo(fivemDir string) backupInfo {
	_, items := a.collectBackup(fivemDir)
	return backupInfo{OK: true, Dir: a.backupDir(), Items: items, List: a.backupList()}
}

func (a *app) writeBackup(fivemDir, name string) (string, int, error) {
	files, _ := a.collectBackup(fivemDir)
	if len(files) == 0 {
		return "", 0, fmt.Errorf("ไม่เจอไฟล์ตั้งค่าเกมให้สำรอง")
	}
	os.MkdirAll(a.backupDir(), 0o755)
	out := filepath.Join(a.backupDir(), name)
	f, err := os.Create(out)
	if err != nil {
		return "", 0, err
	}
	zw := zip.NewWriter(f)
	n := 0
	for zp, real := range files {
		b, err := os.ReadFile(real)
		if err != nil {
			continue
		}
		w, err := zw.Create(zp)
		if err == nil {
			w.Write(b)
			n++
		}
	}
	meta, _ := json.Marshal(map[string]any{"app": "Zan Booster", "version": appVersion, "time": time.Now().Unix(), "computer": os.Getenv("COMPUTERNAME")})
	if w, err := zw.Create("zanbooster-backup.json"); err == nil {
		w.Write(meta)
	}
	zw.Close()
	f.Close()
	return out, n, nil
}

func (a *app) backupCreate(fivemDir string) backupInfo {
	out, n, err := a.writeBackup(fivemDir, "ZanBooster_"+time.Now().Format("2006-01-02_1504")+".zip")
	if err != nil {
		return backupInfo{Message: err.Error()}
	}
	r := a.backupInfo(fivemDir)
	r.Saved, r.Count = out, n
	return r
}

// keepIVPath คืน CitizenFX.ini จาก backup แต่คงที่อยู่ GTA V ของเครื่องนี้ไว้ (เครื่องอื่นอาจลง GTA คนละที่)
func keepIVPath(restored []byte, current string) []byte {
	cur, err := os.ReadFile(current)
	if err != nil {
		return restored
	}
	var iv string
	for _, l := range strings.Split(string(cur), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "IVPath=") {
			iv = strings.TrimRight(l, "\r")
		}
	}
	if iv == "" {
		return restored
	}
	lines := strings.Split(string(restored), "\n")
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "IVPath=") {
			lines[i] = iv + "\r"
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

func (a *app) backupRestore(fivemDir, file string) backupInfo {
	if strings.ContainsAny(file, `\/`) {
		return backupInfo{Message: "ชื่อไฟล์ไม่ถูกต้อง"}
	}
	if fivemRunningAny() {
		return backupInfo{Message: "ปิด FiveM ก่อนคืนค่า"}
	}
	zr, err := zip.OpenReader(filepath.Join(a.backupDir(), file))
	if err != nil {
		return backupInfo{Message: "เปิดไฟล์สำรองไม่ได้: " + err.Error()}
	}
	defer zr.Close()
	// สำรองของเดิมไว้ก่อน เผื่อคืนผิดไฟล์
	a.writeBackup(fivemDir, "ZanBooster_before-restore_"+time.Now().Format("2006-01-02_150405")+".zip")
	roots := a.backupRoots(fivemDir)
	n := 0
	for _, zf := range zr.File {
		var root, rel string
		for k := range roots {
			if strings.HasPrefix(zf.Name, k+"/") && len(k) > len(root) {
				root, rel = k, strings.TrimPrefix(zf.Name, k+"/")
			}
		}
		if root == "" || rel == "" || strings.Contains(rel, "..") || !wantFile(root, filepath.FromSlash(rel)) {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			continue
		}
		var buf bytes.Buffer
		io.Copy(&buf, io.LimitReader(rc, 16<<20))
		rc.Close()
		dst := filepath.Join(roots[root], filepath.FromSlash(rel))
		data := buf.Bytes()
		if root == "fivemapp" && strings.EqualFold(rel, "CitizenFX.ini") {
			data = keepIVPath(data, dst)
		}
		os.MkdirAll(filepath.Dir(dst), 0o755)
		if os.WriteFile(dst, data, 0o644) == nil {
			n++
		}
	}
	r := a.backupInfo(fivemDir)
	r.Count = n
	if n == 0 {
		r.OK, r.Message = false, "ไฟล์นี้ไม่มีค่าที่คืนได้"
	}
	return r
}

func (a *app) openBackupDir() bool {
	os.MkdirAll(a.backupDir(), 0o755)
	return explorerOpen(a.backupDir()) == nil
}

func (a *app) backupAsync(op, fivemDir, file string, reqID int) bool {
	go func() {
		var r backupInfo
		switch op {
		case "create":
			r = a.backupCreate(fivemDir)
		case "restore":
			r = a.backupRestore(fivemDir, file)
		default:
			r = a.backupInfo(fivemDir)
		}
		b, _ := json.Marshal(map[string]any{"reqId": reqID, "r": r})
		a.emit("onBackup", string(b))
	}()
	return true
}
