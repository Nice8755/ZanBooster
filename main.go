//go:build windows

package main

import (
	"bufio"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

//go:embed engine.ps1
var engineScript []byte

//go:embed ui.html
var uiHTML string

//go:embed zan.pow
var zanPow []byte

const appVersion = "1.7.6"

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	dwmapi   = windows.NewLazySystemDLL("dwmapi.dll")
	iphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")

	pGetWindowLongPtr   = user32.NewProc("GetWindowLongPtrW")
	pSetWindowLongPtr   = user32.NewProc("SetWindowLongPtrW")
	pSetWindowPos       = user32.NewProc("SetWindowPos")
	pShowWindow         = user32.NewProc("ShowWindow")
	pIsIconic           = user32.NewProc("IsIconic")
	pReleaseCapture     = user32.NewProc("ReleaseCapture")
	pPostMessage        = user32.NewProc("PostMessageW")
	pMessageBox         = user32.NewProc("MessageBoxW")
	pGetDpiForSystem    = user32.NewProc("GetDpiForSystem")
	pGetSystemMetrics   = user32.NewProc("GetSystemMetrics")
	pGetSystemTimes     = kernel32.NewProc("GetSystemTimes")
	pGlobalMemoryStatus = kernel32.NewProc("GlobalMemoryStatusEx")
	pDwmSetAttr         = dwmapi.NewProc("DwmSetWindowAttribute")
	pIcmpCreateFile     = iphlpapi.NewProc("IcmpCreateFile")
	pIcmpCloseHandle    = iphlpapi.NewProc("IcmpCloseHandle")
	pIcmpSendEcho       = iphlpapi.NewProc("IcmpSendEcho")
	pShellExecute       = shell32.NewProc("ShellExecuteW")
	ntdll               = windows.NewLazySystemDLL("ntdll.dll")
	pNtSetSysInfo       = ntdll.NewProc("NtSetSystemInformation")
)

type app struct {
	w          webview2.WebView
	hwnd       uintptr
	enginePath string
	dataDir    string

	busyMu sync.Mutex
	busy   bool

	pingMu   sync.Mutex
	pingStop chan struct{}

	cpuMu                         sync.Mutex
	lastIdle, lastKernel, lastUser uint64
}

// ------------------------------------------------------------------ main --

func main() {
	a := &app{}
	a.dataDir = filepath.Join(os.Getenv("ProgramData"), "ZanBooster")
	_ = os.MkdirAll(a.dataDir, 0o755)
	a.enginePath = filepath.Join(a.dataDir, "engine.ps1")
	bom := []byte{0xEF, 0xBB, 0xBF}
	if err := os.WriteFile(a.enginePath, append(bom, engineScript...), 0o644); err != nil {
		msgBox("Zan Booster", "เขียนไฟล์ engine ไม่ได้: "+err.Error(), 0x10)
		return
	}

	_ = os.WriteFile(filepath.Join(a.dataDir, "ZAN_Project_Optimizer.pow"), zanPow, 0o644)

	dpi := uintptr(96)
	if pGetDpiForSystem.Find() == nil {
		if d, _, _ := pGetDpiForSystem.Call(); d > 0 {
			dpi = d
		}
	}
	scale := func(v int) int { return v * int(dpi) / 96 }
	sw, _, _ := pGetSystemMetrics.Call(0)
	sh, _, _ := pGetSystemMetrics.Call(1)
	width, height := scale(1240), scale(800)
	if maxW := int(sw) * 94 / 100; width > maxW {
		width = maxW
	}
	if maxH := int(sh) * 90 / 100; height > maxH {
		height = maxH
	}

	// เครื่องที่ไม่มี WebView2 (เช่น Windows LTSC / Windows ตัดแต่ง) ติดตั้งให้อัตโนมัติ
	if !hasWebView2() {
		installWebView2()
	}

	// ลดทรัพยากรของ WebView2: process เดียวสำหรับหน้าจอ ปิดฟีเจอร์เบื้องหลังที่ไม่ใช้
	os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "--renderer-process-limit=1 --disable-extensions --disable-background-networking --disable-component-update --disable-sync --disable-features=msSmartScreenProtection,msWebOOUI,msPdfOOUI,SpareRendererForSitePerProcess,AutofillServerCommunication --js-flags=--max-old-space-size=64")
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		DataPath:  filepath.Join(os.Getenv("LOCALAPPDATA"), "ZanBooster", "WebView2"),
		WindowOptions: webview2.WindowOptions{
			Title:  "Zan Booster",
			Width:  uint(width),
			Height: uint(height),
			IconId: 2,
			Center: true,
		},
	})
	if w == nil {
		r := msgBox("Zan Booster",
			"เครื่องนี้ยังไม่มี Microsoft Edge WebView2 Runtime\n(ปกติ Windows 10/11 มีอยู่แล้ว)\n\nกด OK เพื่อเปิดหน้าดาวน์โหลด ติดตั้งแล้วเปิดโปรแกรมใหม่อีกครั้ง", 0x31)
		if r == 1 {
			openURL("https://go.microsoft.com/fwlink/p/?LinkId=2124703")
		}
		return
	}
	defer w.Destroy()
	a.w = w
	a.hwnd = uintptr(w.Window())
	a.makeFrameless(width, height)
	a.cpuUsage() // prime counters

	must(w.Bind("engine", a.engine))
	must(w.Bind("stats", a.stats))
	must(w.Bind("pingStart", a.pingStart))
	must(w.Bind("pingStop", a.pingStopFn))
	must(w.Bind("winMin", a.winMin))
	must(w.Bind("winClose", a.winClose))
	must(w.Bind("winDrag", a.winDrag))
	must(w.Bind("openUrl", openURL))
	must(w.Bind("openDataDir", func() { openURL(a.dataDir) }))
	must(w.Bind("appVersion", func() string { return appVersion }))
	must(w.Bind("purgeMemory", a.purgeMemory))
	must(w.Bind("serverLookup", a.serverLookup))
	must(w.Bind("serverStatus", a.serverStatusCheck))
	must(w.Bind("checkBuild", a.checkBuild))
	must(w.Bind("isMinimized", func() bool { r, _, _ := pIsIconic.Call(a.hwnd); return r != 0 }))
	must(w.Bind("checkUpdate", a.checkUpdate))
	must(w.Bind("applyUpdate", a.applyUpdate))
	must(w.Bind("about", a.about))
	must(w.Bind("loadSettings", a.loadSettings))
	must(w.Bind("saveSettings", a.saveSettings))
	must(w.Bind("openPath", openURL))
	must(w.Bind("launchServer", a.launchServerAsync))
	must(w.Bind("loadRecent", a.loadRecent))
	must(w.Bind("saveRecent", a.saveRecent))

	cleanupOldExe()
	a.loadRemoteConfig()
	w.SetHtml(uiHTML)
	w.Run()
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// ---------------------------------------------------------------- window --

func (a *app) makeFrameless(width, height int) {
	const (
		gwlStyle       = ^uintptr(15) // -16
		wsPopup        = 0x80000000
		wsVisible      = 0x10000000
		wsClipChildren = 0x02000000
		wsSysMenu      = 0x00080000
		wsMinimizeBox  = 0x00020000
		swpFlags       = 0x0002 | 0x0004 | 0x0020 | 0x0001 // NOMOVE|NOZORDER|FRAMECHANGED|NOSIZE
	)
	pSetWindowLongPtr.Call(a.hwnd, gwlStyle, wsPopup|wsVisible|wsClipChildren|wsSysMenu|wsMinimizeBox)
	pSetWindowPos.Call(a.hwnd, 0, 0, 0, 0, 0, swpFlags)
	// บังคับ WM_SIZE ให้ WebView ขยายเต็มพื้นที่ใหม่หลังเอากรอบออก
	const swpResize = 0x0002 | 0x0004 | 0x0010 // NOMOVE|NOZORDER|NOACTIVATE
	pSetWindowPos.Call(a.hwnd, 0, 0, 0, uintptr(width-1), uintptr(height-1), swpResize)
	pSetWindowPos.Call(a.hwnd, 0, 0, 0, uintptr(width), uintptr(height), swpResize)

	if pDwmSetAttr.Find() == nil {
		// Windows 11: มุมโค้ง + เงา, dark mode
		pref := uint32(2) // DWMWCP_ROUND
		pDwmSetAttr.Call(a.hwnd, 33, uintptr(unsafe.Pointer(&pref)), 4)
		dark := uint32(1)
		pDwmSetAttr.Call(a.hwnd, 20, uintptr(unsafe.Pointer(&dark)), 4)
	}
}

func (a *app) winMin()   { pShowWindow.Call(a.hwnd, 6) } // SW_MINIMIZE
func (a *app) winClose() { a.pingStopFn(); a.w.Terminate() }
func (a *app) winDrag() {
	pReleaseCapture.Call()
	pPostMessage.Call(a.hwnd, 0x00A1, 2, 0) // WM_NCLBUTTONDOWN, HTCAPTION
}

func msgBox(title, text string, flags uintptr) uintptr {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	r, _, _ := pMessageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags)
	return r
}

func openURL(u string) {
	verb, _ := windows.UTF16PtrFromString("open")
	p, _ := windows.UTF16PtrFromString(u)
	pShellExecute.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(p)), 0, 0, 1)
}

func (a *app) emit(fn string, payload string) {
	a.w.Dispatch(func() { a.w.Eval("window." + fn + " && window." + fn + "(" + payload + ")") })
}

// ---------------------------------------------------------------- engine --

// engine เริ่มงานเบื้องหลัง ผลลัพธ์ส่งกลับทาง window.onEngine(evt)
func (a *app) engine(action, ids, opt string) bool {
	a.busyMu.Lock()
	if a.busy {
		a.busyMu.Unlock()
		return false
	}
	a.busy = true
	a.busyMu.Unlock()

	go func() {
		defer func() {
			a.busyMu.Lock()
			a.busy = false
			a.busyMu.Unlock()
			a.emit("onEngine", `{"t":"done","action":`+jsonStr(action)+`}`)
		}()
		args := []string{"-NoProfile", "-NonInteractive", "-STA", "-ExecutionPolicy", "Bypass", "-File", a.enginePath, "-Action", action}
		if ids != "" {
			args = append(args, "-Ids", ids)
		}
		if opt != "" {
			args = append(args, "-Opt", opt)
		}
		cmd := exec.Command("powershell.exe", args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		out, err := cmd.StdoutPipe()
		if err != nil {
			a.emitErr(action, err)
			return
		}
		if err := cmd.Start(); err != nil {
			a.emitErr(action, err)
			return
		}
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\uFEFF"))
			if line != "" && json.Valid([]byte(line)) {
				a.emit("onEngine", line)
			}
		}
		_ = cmd.Wait()
	}()
	return true
}

func (a *app) emitErr(action string, err error) {
	a.emit("onEngine", fmt.Sprintf(`{"t":"error","action":%s,"m":%s}`, jsonStr(action), jsonStr(err.Error())))
}

func jsonStr(s string) string { b, _ := json.Marshal(s); return string(b) }

// ----------------------------------------------------------------- stats --

type stats struct {
	CPU      float64 `json:"cpu"`
	RAM      float64 `json:"ram"`
	RAMUsed  float64 `json:"ramUsed"`
	RAMTotal float64 `json:"ramTotal"`
	FiveM    bool    `json:"fivem"`
}

func ft(f windows.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }

func (a *app) cpuUsage() float64 {
	var idle, kern, user windows.Filetime
	r, _, _ := pGetSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kern)), uintptr(unsafe.Pointer(&user)))
	if r == 0 {
		return 0
	}
	a.cpuMu.Lock()
	defer a.cpuMu.Unlock()
	i, k, u := ft(idle), ft(kern), ft(user)
	di, dk, du := i-a.lastIdle, k-a.lastKernel, u-a.lastUser
	a.lastIdle, a.lastKernel, a.lastUser = i, k, u
	total := dk + du
	if total == 0 || di > total {
		return 0
	}
	return float64(total-di) * 100 / float64(total)
}

type memStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func fivemRunning() bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if windows.Process32First(snap, &pe) != nil {
		return false
	}
	for {
		name := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
		if strings.HasPrefix(name, "fivem") && strings.Contains(name, "gtaprocess") {
			return true
		}
		if windows.Process32Next(snap, &pe) != nil {
			return false
		}
	}
}

func (a *app) stats() stats {
	s := stats{CPU: a.cpuUsage(), FiveM: fivemRunning()}
	var m memStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	if r, _, _ := pGlobalMemoryStatus.Call(uintptr(unsafe.Pointer(&m))); r != 0 {
		s.RAMTotal = float64(m.TotalPhys) / (1 << 30)
		s.RAMUsed = float64(m.TotalPhys-m.AvailPhys) / (1 << 30)
		s.RAM = float64(m.MemoryLoad)
	}
	return s
}

// ------------------------------------------------------------------ ping --

func (a *app) pingStart(target string) bool {
	a.pingStopFn()
	target = strings.TrimSpace(target)
	for _, p := range []string{"fivem://connect/", "https://", "http://", "fivem://"} {
		target = strings.TrimPrefix(target, p)
	}
	target = strings.TrimSuffix(target, "/")
	if target == "" {
		return false
	}
	host, port := target, ""
	if h, p, err := net.SplitHostPort(target); err == nil {
		host, port = h, p
	}
	stop := make(chan struct{})
	a.pingMu.Lock()
	a.pingStop = stop
	a.pingMu.Unlock()

	go func() {
		mode := "ICMP"
		if port != "" {
			mode = "TCP"
		}
		var ip4 net.IP
		if port == "" {
			if addr, err := net.ResolveIPAddr("ip4", host); err == nil {
				ip4 = addr.IP.To4()
			}
			if ip4 == nil {
				a.emit("onPing", `{"ok":false,"err":"resolve","mode":"ICMP"}`)
				return
			}
		}
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			var ms float64
			ok := false
			if port != "" {
				t0 := time.Now()
				c, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
				if err == nil {
					ms = float64(time.Since(t0).Microseconds()) / 1000
					ok = true
					c.Close()
				}
			} else {
				ms, ok = icmpPing(ip4, 2000)
			}
			a.emit("onPing", fmt.Sprintf(`{"ok":%t,"ms":%.1f,"mode":"%s"}`, ok, ms, mode))
			select {
			case <-stop:
				return
			case <-tick.C:
			}
		}
	}()
	return true
}

func (a *app) pingStopFn() {
	a.pingMu.Lock()
	defer a.pingMu.Unlock()
	if a.pingStop != nil {
		close(a.pingStop)
		a.pingStop = nil
	}
}

func icmpPing(ip net.IP, timeoutMs uint32) (float64, bool) {
	h, _, _ := pIcmpCreateFile.Call()
	if h == 0 || h == ^uintptr(0) {
		return 0, false
	}
	defer pIcmpCloseHandle.Call(h)
	data := []byte("ZanBoosterPing")
	reply := make([]byte, 256)
	addr := binary.LittleEndian.Uint32(ip)
	n, _, _ := pIcmpSendEcho.Call(h, uintptr(addr), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0,
		uintptr(unsafe.Pointer(&reply[0])), uintptr(len(reply)), uintptr(timeoutMs))
	if n == 0 {
		return 0, false
	}
	status := binary.LittleEndian.Uint32(reply[4:8])
	rtt := binary.LittleEndian.Uint32(reply[8:12])
	if status != 0 {
		return 0, false
	}
	return float64(rtt), true
}

// --------------------------------------------------------- memory clean --

func enablePrivilege(name string) error {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &tok); err != nil {
		return err
	}
	defer tok.Close()
	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr(name), &luid); err != nil {
		return err
	}
	tp := windows.Tokenprivileges{PrivilegeCount: 1}
	tp.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
	return windows.AdjustTokenPrivileges(tok, false, &tp, 0, nil, nil)
}

type memResult struct {
	OK        bool    `json:"ok"`
	StandbyMB float64 `json:"standbyMB"`
	FreedMB   float64 `json:"freedMB"`
	Message   string  `json:"m,omitempty"`
}

var pNtQuerySysInfo = ntdll.NewProc("NtQuerySystemInformation")

// standbyMB อ่านขนาด Standby List (แรมที่ Windows เก็บไว้เป็นแคช)
func standbyMB() float64 {
	var info [22]uintptr // SYSTEM_MEMORY_LIST_INFORMATION
	var ret uint32
	r, _, _ := pNtQuerySysInfo.Call(80, uintptr(unsafe.Pointer(&info[0])), unsafe.Sizeof(info), uintptr(unsafe.Pointer(&ret)))
	if r != 0 {
		return -1
	}
	var pages uintptr
	for i := 5; i < 13; i++ { // PageCountByPriority[8]
		pages += info[i]
	}
	return float64(pages) * 4096 / (1 << 20)
}

// purgeMemory ล้าง Standby List ช่วยลดอาการกระตุกในเครื่องที่แรมไม่เยอะ
func (a *app) purgeMemory() memResult {
	if err := enablePrivilege("SeProfileSingleProcessPrivilege"); err != nil {
		return memResult{Message: err.Error()}
	}
	before := standbyMB()
	const systemMemoryListInformation = 80
	for _, cmd := range []uint32{3, 4} { // 3 = flush modified list, 4 = purge standby list
		c := cmd
		r, _, _ := pNtSetSysInfo.Call(systemMemoryListInformation, uintptr(unsafe.Pointer(&c)), 4)
		if r != 0 && cmd == 4 {
			return memResult{Message: fmt.Sprintf("NTSTATUS 0x%X", r)}
		}
	}
	after := standbyMB()
	res := memResult{OK: true, StandbyMB: before}
	if before >= 0 && after >= 0 && before > after {
		res.FreedMB = before - after
	}
	return res
}
