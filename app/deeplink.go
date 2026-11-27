//go:build windows

package main

// deeplink.go: ถอดจาก exe 4.11.15 · ลิงก์ zanbooster://... (เช่น zanbooster://join/IP) จากเว็บ/ดิส
// เปิดโปรแกรมพร้อมคำสั่ง + รอคิวเซิร์ฟเต็มเบื้องหลัง (ตัวช่วยโหมดเล่นเกม) ว่างแล้วแจ้งเตือนและเข้าให้

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	pFindWindow          = user32.NewProc("FindWindowW")
	pSetForegroundWindow = user32.NewProc("SetForegroundWindow")
)

func linkFile() string { return filepath.Join(os.Getenv("ProgramData"), "ZanBooster", "deeplink.txt") }

// cleanLink: รับเฉพาะ zanbooster:... ที่สั้นและไม่มีตัวอักษรแปลก คืนส่วนหลัง zanbooster://
func cleanLink(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(strings.ToLower(s), "zanbooster:") {
		return ""
	}
	r := strings.TrimLeft(s[len("zanbooster:"):], "/")
	if len(r) > 300 || strings.ContainsAny(r, "\"<>\n") {
		return ""
	}
	return r
}

// handleLinkArg: เปิดจากลิงก์ → จดคำสั่งไว้ ถ้าหน้าโปรแกรมเปิดอยู่แล้วดึงขึ้นมาแล้วจบตัวนี้ (true = ให้ main จบ)
func handleLinkArg() bool {
	if len(os.Args) < 2 {
		return false
	}
	l := cleanLink(os.Args[1])
	if l == "" {
		return false
	}
	_ = os.MkdirAll(filepath.Dir(linkFile()), 0o755)
	_ = os.WriteFile(linkFile(), []byte(l), 0o644)
	if !mutexExists(uiMutexName) {
		return false
	}
	t, _ := windows.UTF16PtrFromString("Zan Booster")
	if h, _, _ := pFindWindow.Call(0, uintptr(unsafe.Pointer(t))); h != 0 {
		pShowWindow.Call(h, 9)
		pSetForegroundWindow.Call(h)
	}
	return true
}

// takeDeepLink: หน้าจอถามเป็นระยะ มีคำสั่งค้างไหม (อ่านแล้วลบ)
func (a *app) takeDeepLink() string {
	b, err := os.ReadFile(linkFile())
	if err != nil {
		return ""
	}
	_ = os.Remove(linkFile())
	return strings.TrimSpace(string(b))
}

// registerLinkScheme: ผูก zanbooster:// กับ exe นี้ (ของผู้ใช้คนนี้ ไม่ต้องสิทธิ์แอดมิน) ทำเฉพาะตอนที่อยู่ผิดที่
func registerLinkScheme() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := fmt.Sprintf(`"%s" "%%1"`, exe)
	const root = `Software\Classes\zanbooster`
	if k, err := registry.OpenKey(registry.CURRENT_USER, root+`\shell\open\command`, registry.QUERY_VALUE); err == nil {
		v, _, _ := k.GetStringValue("")
		k.Close()
		if v == cmd {
			return
		}
	}
	if k, _, err := registry.CreateKey(registry.CURRENT_USER, root, registry.SET_VALUE); err == nil {
		_ = k.SetStringValue("", "URL:Zan Booster")
		_ = k.SetStringValue("URL Protocol", "")
		k.Close()
	}
	if k, _, err := registry.CreateKey(registry.CURRENT_USER, root+`\DefaultIcon`, registry.SET_VALUE); err == nil {
		_ = k.SetStringValue("", `"`+exe+`",0`)
		k.Close()
	}
	if k, _, err := registry.CreateKey(registry.CURRENT_USER, root+`\shell\open\command`, registry.SET_VALUE); err == nil {
		_ = k.SetStringValue("", cmd)
		k.Close()
	}
}

type bgWatch struct {
	Connect string `json:"connect"`
	Base    string `json:"base"`
	Name    string `json:"name"`
	Since   int64  `json:"since"`
}

// watchList: เซิร์ฟที่ตั้งรอคิวไว้ (ในค่าที่ตั้งของหน้าจอ settings.watch) ไม่เกิน 3 ชั่วโมง
func (a *app) watchList() []bgWatch {
	var s struct {
		Watch []bgWatch `json:"watch"`
	}
	_ = json.Unmarshal([]byte(a.loadSettings()), &s)
	now := time.Now().Unix()
	var out []bgWatch
	for _, w := range s.Watch {
		if w.Base != "" && w.Connect != "" && now-w.Since < 3*3600 {
			out = append(out, w)
		}
	}
	return out
}

func (a *app) watchPending() []bgWatch {
	if !licGet().Premium {
		return nil
	}
	return a.watchList()
}

func (a *app) watchRemove(connect string) {
	m := map[string]any{}
	if json.Unmarshal([]byte(a.loadSettings()), &m) != nil {
		return
	}
	arr, _ := m["watch"].([]any)
	var keep []any
	for _, x := range arr {
		if o, ok := x.(map[string]any); ok && o["connect"] == connect {
			continue
		}
		keep = append(keep, x)
	}
	m["watch"] = keep
	b, _ := json.Marshal(m)
	a.saveSettings(string(b))
}

// slotFree: เซิร์ฟมีที่ว่างไหม (dynamic.json ของเซิร์ฟ FiveM)
func slotFree(base string) (bool, int, int) {
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	resp, err := httpGet(base + "dynamic.json")
	if err != nil {
		return false, 0, 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false, 0, 0
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var d struct {
		Clients int `json:"clients"`
		Max     any `json:"sv_maxclients"`
	}
	if json.Unmarshal(b, &d) != nil {
		return false, 0, 0
	}
	max := atoi(fmt.Sprint(d.Max))
	return max > 0 && d.Clients < max, d.Clients, max
}

// bgWatchLoop: ทำงานในตัวช่วยโหมดเล่นเกมตอนหน้าโปรแกรมปิดอยู่ (เปิดอยู่ = หน้าจอทำเอง)
func (a *app) bgWatchLoop() {
	for {
		time.Sleep(4 * time.Second)
		if mutexExists(uiMutexName) || !licGet().Premium {
			continue
		}
		for _, w := range a.watchList() {
			free, c, m := slotFree(w.Base)
			if !free {
				continue
			}
			a.watchRemove(w.Connect)
			toastNotify("เซิร์ฟมีที่ว่างแล้ว", fmt.Sprintf("%s (%d/%d) Zan Booster กำลังเข้าให้", w.Name, c, m))
			if exe, err := os.Executable(); err == nil {
				if exec.Command(exe, "zanbooster://join/"+url.PathEscape(w.Connect)+"?auto=1").Start() == nil {
					break
				}
			}
		}
	}
}

// toastNotify: แจ้งเตือนมุมจอของ Windows (ใช้ตัวแจ้งเตือนของ PowerShell ที่มีในทุกเครื่อง)
func toastNotify(title, body string) {
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "'", "''", `"`, "&quot;")
	ps := "[Windows.UI.Notifications.ToastNotificationManager,Windows.UI.Notifications,ContentType=WindowsRuntime]|Out-Null;" +
		"[Windows.Data.Xml.Dom.XmlDocument,Windows.Data.Xml.Dom.XmlDocument,ContentType=WindowsRuntime]|Out-Null;" +
		"$x=New-Object Windows.Data.Xml.Dom.XmlDocument;$x.LoadXml('<toast scenario=\"reminder\"><visual><binding template=\"ToastGeneric\"><text>" +
		esc.Replace(title) + "</text><text>" + esc.Replace(body) +
		"</text></binding></visual><audio src=\"ms-winsoundevent:Notification.Reminder\"/></toast>');" +
		"[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\\WindowsPowerShell\\v1.0\\powershell.exe').Show([Windows.UI.Notifications.ToastNotification]::new($x))"
	c := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", ps)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	_ = c.Start()
}
