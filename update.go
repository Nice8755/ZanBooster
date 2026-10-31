//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ===== ตั้งค่าที่เดียว =====
// ชื่อ GitHub repo ที่ใช้ปล่อยเวอร์ชันใหม่ เช่น "zan123/ZanBooster"
// ปล่อยว่างไว้ = ปิดระบบอัปเดต (โปรแกรมยังทำงานปกติ)
const updateRepo = "Nice8755/ZanBooster"

// ช่องทางติดต่อที่แสดงในหน้าเกี่ยวกับ (ว่าง = ไม่แสดง)
const contactURL = ""
const authorName = "Zan"

var dlClient = &http.Client{Timeout: 5 * time.Minute}

type updateInfo struct {
	Available bool   `json:"available"`
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Notes     string `json:"notes"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Err       string `json:"err,omitempty"`
	Enabled   bool   `json:"enabled"`
}

func verParts(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	var r [3]int
	for i, p := range strings.SplitN(v, ".", 3) {
		r[i], _ = strconv.Atoi(strings.TrimFunc(p, func(c rune) bool { return c < '0' || c > '9' }))
	}
	return r
}

func newerThan(a, b string) bool {
	x, y := verParts(a), verParts(b)
	for i := 0; i < 3; i++ {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return false
}

func ghGet(url string) (*http.Response, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "ZanBooster/"+appVersion)
	req.Header.Set("Accept", "application/vnd.github+json")
	return dlClient.Do(req)
}

// checkUpdate ถาม GitHub Releases ว่ามีเวอร์ชันใหม่ไหม (เรียกแบบไม่บล็อก ผลส่งไป window.onUpdate)
func (a *app) checkUpdate() bool {
	go func() {
		ui := updateInfo{Current: appVersion, Enabled: updateRepo != ""}
		defer func() { b, _ := json.Marshal(ui); a.emit("onUpdate", string(b)) }()
		if updateRepo == "" {
			return
		}
		// ช่องทางหลัก: GitHub Releases (อัปเดตทันทีที่ปล่อย)
		resp, err := ghGet("https://api.github.com/repos/" + updateRepo + "/releases/latest")
		relOK := false
		if err == nil {
			defer resp.Body.Close()
			relOK = resp.StatusCode == 200
		}
		if relOK {
			var rel struct {
				Tag    string `json:"tag_name"`
				Body   string `json:"body"`
				Assets []struct {
					Name string `json:"name"`
					URL  string `json:"browser_download_url"`
					Size int64  `json:"size"`
				} `json:"assets"`
			}
			if json.NewDecoder(resp.Body).Decode(&rel) == nil && rel.Tag != "" {
				ui.Latest, ui.Notes = strings.TrimPrefix(rel.Tag, "v"), rel.Body
				for _, as := range rel.Assets {
					if strings.EqualFold(as.Name, "ZanBooster.exe") {
						ui.URL, ui.Size = as.URL, as.Size
					}
				}
				// ค่า SHA256 ใส่ไว้ในคำอธิบาย release บรรทัด "SHA256: xxxx"
				for _, line := range strings.Split(rel.Body, "\n") {
					if i := strings.Index(strings.ToUpper(line), "SHA256:"); i >= 0 {
						ui.SHA256 = strings.ToLower(strings.TrimSpace(line[i+7:]))
					}
				}
				ui.Available = ui.URL != "" && newerThan(ui.Latest, appVersion)
				if ui.URL != "" {
					return
				}
			}
		}
		// สำรอง: ไฟล์ latest.json ใน repo (ใส่เวลาต่อท้ายกันแคช)
		if r, err := ghGet("https://raw.githubusercontent.com/" + updateRepo + "/main/latest.json?t=" + strconv.FormatInt(time.Now().Unix(), 10)); err == nil {
			var lj struct {
				Version string `json:"version"`
				SHA256  string `json:"sha256"`
				Notes   string `json:"notes"`
				File    string `json:"file"`
			}
			ok := r.StatusCode == 200 && json.NewDecoder(r.Body).Decode(&lj) == nil && lj.Version != ""
			r.Body.Close()
			if ok {
				if lj.File == "" {
					lj.File = "release/ZanBooster.exe"
				}
				ui.Latest, ui.Notes, ui.SHA256 = strings.TrimPrefix(lj.Version, "v"), lj.Notes, strings.ToLower(lj.SHA256)
				ui.URL = "https://raw.githubusercontent.com/" + updateRepo + "/main/" + strings.TrimPrefix(lj.File, "/")
				ui.Available = newerThan(ui.Latest, appVersion)
				return
			}
		}
		ui.Err = "ต่อ GitHub ไม่ได้"
	}()
	return true
}

// applyUpdate ดาวน์โหลด exe ใหม่ ตรวจ SHA256 แล้วสลับไฟล์และเปิดใหม่
func (a *app) applyUpdate(url, sum string) string {
	okURL := strings.HasPrefix(url, "https://github.com/"+updateRepo+"/releases/") ||
		strings.HasPrefix(url, "https://raw.githubusercontent.com/"+updateRepo+"/")
	if updateRepo == "" || !okURL {
		return "ลิงก์อัปเดตไม่ถูกต้อง"
	}
	self, err := os.Executable()
	if err != nil {
		return err.Error()
	}
	resp, err := ghGet(url)
	if err != nil {
		return "ดาวน์โหลดไม่ได้: " + err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Sprintf("ดาวน์โหลดไม่ได้ (%d)", resp.StatusCode)
	}
	tmp := self + ".new"
	f, err := os.Create(tmp)
	if err != nil {
		return "เขียนไฟล์ไม่ได้: " + err.Error()
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, 200<<20))
	f.Close()
	if err != nil || n < 1<<20 {
		os.Remove(tmp)
		return "ไฟล์ที่ดาวน์โหลดไม่สมบูรณ์"
	}
	if sum == "" && strings.HasPrefix(url, "https://raw.githubusercontent.com/") {
		os.Remove(tmp)
		return "ไม่มีค่า SHA256 สำหรับตรวจไฟล์ ยกเลิกการอัปเดต"
	}
	if sum != "" && !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), sum) {
		os.Remove(tmp)
		return "ไฟล์ไม่ตรงกับ SHA256 ที่ประกาศไว้ ยกเลิกการอัปเดตเพื่อความปลอดภัย"
	}
	old := self + ".old"
	os.Remove(old)
	if err := os.Rename(self, old); err != nil {
		os.Remove(tmp)
		return "สลับไฟล์ไม่ได้: " + err.Error()
	}
	if err := os.Rename(tmp, self); err != nil {
		os.Rename(old, self)
		return "สลับไฟล์ไม่ได้: " + err.Error()
	}
	cmd := exec.Command(self)
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.Start()
	a.w.Terminate()
	return ""
}

// cleanupOldExe ลบไฟล์เก่าที่เหลือจากการอัปเดตครั้งก่อน
func cleanupOldExe() {
	if self, err := os.Executable(); err == nil {
		os.Remove(self + ".old")
		os.Remove(self + ".new")
	}
}

// ---- ค่าที่ดึงจาก GitHub ได้โดยไม่ต้องออกเวอร์ชันใหม่ (remote.json ใน repo) ----
type remoteCfg struct {
	FiveMDefaultBuild int    `json:"fivemDefaultBuild"`
	Notice            string `json:"notice"`
}

func (a *app) loadRemoteConfig() {
	cache := filepath.Join(a.dataDir, "remote.json")
	apply := func(b []byte) {
		var rc remoteCfg
		if json.Unmarshal(b, &rc) == nil {
			if rc.FiveMDefaultBuild >= 1604 {
				fivemDefaultBuild = rc.FiveMDefaultBuild
			}
			if rc.Notice != "" {
				a.emit("onNotice", jsonStr(rc.Notice))
			}
		}
	}
	if b, err := os.ReadFile(cache); err == nil {
		apply(b)
	}
	if updateRepo == "" {
		return
	}
	go func() {
		resp, err := ghGet("https://raw.githubusercontent.com/" + updateRepo + "/main/remote.json")
		if err != nil {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if json.Valid(b) {
			os.WriteFile(cache, b, 0o644)
			apply(b)
		}
	}()
}

// ---- ข้อมูลหน้าเกี่ยวกับ ----
type aboutInfo struct {
	Version string `json:"version"`
	Author  string `json:"author"`
	Contact string `json:"contact"`
	Repo    string `json:"repo"`
	SHA256  string `json:"sha256"`
	Path    string `json:"path"`
}

func (a *app) about() aboutInfo {
	ai := aboutInfo{Version: appVersion, Author: authorName, Contact: contactURL, Repo: updateRepo}
	if self, err := os.Executable(); err == nil {
		ai.Path = self
		if f, err := os.Open(self); err == nil {
			h := sha256.New()
			io.Copy(h, f)
			f.Close()
			ai.SHA256 = hex.EncodeToString(h.Sum(nil))
		}
	}
	return ai
}

// ---- ไฟล์ตั้งค่าเล็ก ๆ ของ UI ----
func (a *app) loadSettings() string {
	b, err := os.ReadFile(filepath.Join(a.dataDir, "settings.json"))
	if err != nil || !json.Valid(b) {
		return "{}"
	}
	return string(b)
}
func (a *app) saveSettings(s string) bool {
	if !json.Valid([]byte(s)) {
		return false
	}
	return os.WriteFile(filepath.Join(a.dataDir, "settings.json"), []byte(s), 0o644) == nil
}
