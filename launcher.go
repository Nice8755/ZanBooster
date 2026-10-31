//go:build windows

package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// build เริ่มต้นของ FiveM client (ตาม xbr::GetDefaultGTA5Build ใน source ของ FiveM)
// อัปเดตได้จาก remote.json บน GitHub โดยไม่ต้องออกเวอร์ชันใหม่
var fivemDefaultBuild = 3258

var (
	pWritePrivateProfile = kernel32.NewProc("WritePrivateProfileStringW")
)

type serverInfo struct {
	ReqID        int      `json:"reqId"`
	OK           bool     `json:"ok"`
	Err          string   `json:"err,omitempty"`
	Input        string   `json:"input"`
	Connect      string   `json:"connect"`
	Base         string   `json:"base"`
	Hostname     string   `json:"hostname"`
	Players      int      `json:"players"`
	MaxPlayers   int      `json:"maxPlayers"`
	Ping         float64  `json:"ping"`
	Build        int      `json:"build"`
	Pure         int      `json:"pure"`
	Pool         string   `json:"pool"`
	PoolCount    int      `json:"poolCount"`
	EffDefault   int      `json:"effDefault"`
	EnforceBuild bool     `json:"enforceBuild"`
	OneSync      string   `json:"onesync"`
	Resources    int      `json:"resources"`
	Locale       string   `json:"locale"`
	Tags         []string `json:"tags"`
	Icon         string   `json:"icon"`
	Country      string   `json:"country"`
}

var httpc = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func httpGet(u string) (*http.Response, error) {
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "CitizenFX/1")
	return httpc.Do(req)
}

var colorRx = regexp.MustCompile(`\^\d|~[a-zA-Z]~`)

// resolveBase แปลงสิ่งที่ผู้ใช้พิมพ์ (IP:port, cfx.re/join/xxxx, โค้ด) เป็น URL ของเซิร์ฟ แบบเดียวกับ NetLibrary::ResolveUrl
func resolveBase(in string, depth int) (string, string, error) {
	in = strings.TrimSpace(in)
	for _, p := range []string{"fivem://connect/", "connect "} {
		in = strings.TrimPrefix(in, p)
	}
	in = strings.TrimSuffix(in, "/")
	if in == "" {
		return "", "", fmt.Errorf("ใส่ IP หรือโค้ดเซิร์ฟก่อน")
	}
	if depth > 3 {
		return "", "", fmt.Errorf("หาเซิร์ฟไม่เจอ")
	}
	if strings.HasPrefix(in, "http://") || strings.HasPrefix(in, "https://") {
		return strings.TrimSuffix(in, "/") + "/", in, nil
	}
	if strings.Contains(in, ".cfx.re") && !strings.Contains(in, "cfx.re/join") {
		return "https://" + in + "/", in, nil
	}
	if !strings.ContainsAny(in, ".:") || strings.Contains(in, "cfx.re/join") {
		join := in
		if !strings.Contains(join, "cfx.re/join") {
			join = "cfx.re/join/" + in
		}
		join = strings.TrimPrefix(strings.TrimPrefix(join, "https://"), "http://")
		resp, err := httpGet("https://" + join)
		if err != nil {
			return "", "", fmt.Errorf("ต่อ cfx.re ไม่ได้: %v", err)
		}
		resp.Body.Close()
		u := resp.Header.Get("X-CitizenFX-Url")
		if u == "" || u == "https://private-placeholder.cfx.re/" {
			return "", "", fmt.Errorf("ไม่เจอเซิร์ฟจากโค้ดนี้ หรือเซิร์ฟปิดอยู่")
		}
		b, _, err := resolveBase(u, depth+1)
		return b, join, err
	}
	host := in
	if _, _, err := net.SplitHostPort(in); err != nil {
		host = net.JoinHostPort(in, "30120")
	}
	return "http://" + host + "/", in, nil
}

func atoi(s string) int { n, _ := strconv.Atoi(strings.TrimSpace(s)); return n }

// computeLaunch คำนวณค่าที่ FiveM จะใช้ตอนเชื่อมต่อ (ตาม NetLibrary.cpp) เพื่อให้เปิดเกมรอบเดียวไม่ต้องรีสตาร์ท
func computeLaunch(vars map[string]string, si *serverInfo) {
	pure := atoi(vars["sv_pureLevel"])
	poolRaw := strings.TrimSpace(vars["sv_poolSizesIncrease"])
	pool := ""
	if poolRaw != "" {
		var m map[string]uint32
		if json.Unmarshal([]byte(poolRaw), &m) == nil && len(m) > 0 {
			b, _ := json.Marshal(m)
			pool = string(b)
			si.PoolCount = len(m)
		}
	}
	val := strings.TrimSpace(vars["sv_enforceGameBuild"])
	serverDefault := fivemDefaultBuild
	hasSD := false
	if sd := atoi(vars["sv_defaultGameBuild"]); sd >= fivemDefaultBuild {
		serverDefault, hasSD = sd, true
	}
	legacy := !hasSD && vars["sv_replaceExeToSwitchBuilds"] != "false" && val != "1"
	if val != "" {
		b := atoi(val)
		if b == 3717 {
			b = 3751
		}
		si.Build, si.EnforceBuild = b, true
		if legacy {
			si.EffDefault = b
		} else {
			si.EffDefault = serverDefault
		}
		si.Pure, si.Pool = pure, pool
	} else {
		si.Build, si.EffDefault, si.Pure, si.Pool, si.PoolCount = serverDefault, serverDefault, 0, "", 0
	}
}

func (a *app) serverLookup(input string, reqID int) bool {
	go func() {
		si := serverInfo{ReqID: reqID, Input: input}
		defer func() {
			b, _ := json.Marshal(si)
			a.emit("onServerInfo", string(b))
		}()
		base, conn, err := resolveBase(input, 0)
		if err != nil {
			si.Err = err.Error()
			return
		}
		si.Base, si.Connect = base, conn

		var info struct {
			Vars      map[string]string `json:"vars"`
			Resources []string          `json:"resources"`
			Icon      string            `json:"icon"`
		}
		resp, err := httpGet(base + "info.json")
		if err != nil {
			si.Err = "เซิร์ฟไม่ตอบ (อาจปิดอยู่ หรือ IP/พอร์ตผิด)"
			return
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if json.Unmarshal(body, &info) != nil || info.Vars == nil {
			si.Err = "อ่านข้อมูลเซิร์ฟไม่ได้"
			return
		}
		computeLaunch(info.Vars, &si)
		si.Resources = len(info.Resources)
		if info.Icon != "" && len(info.Icon) < 300000 {
			si.Icon = "data:image/png;base64," + info.Icon
		}
		si.Country = countryFromLocale(info.Vars["locale"])
		si.OneSync = info.Vars["onesync_enabled"]
		si.Locale = info.Vars["locale"]
		if t := info.Vars["tags"]; t != "" {
			for _, x := range strings.Split(t, ",") {
				if x = strings.TrimSpace(x); x != "" && len(si.Tags) < 6 {
					si.Tags = append(si.Tags, x)
				}
			}
		}
		si.Hostname = strings.TrimSpace(colorRx.ReplaceAllString(info.Vars["sv_projectName"], ""))

		if resp, err := httpGet(base + "dynamic.json"); err == nil {
			var dyn struct {
				Hostname string `json:"hostname"`
				Clients  int    `json:"clients"`
				Max      any    `json:"sv_maxclients"`
			}
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			if json.Unmarshal(b, &dyn) == nil {
				si.Players = dyn.Clients
				si.MaxPlayers = atoi(fmt.Sprint(dyn.Max))
				if h := strings.TrimSpace(colorRx.ReplaceAllString(dyn.Hostname, "")); h != "" && si.Hostname == "" {
					si.Hostname = h
				}
			}
		}
		if si.Hostname == "" {
			si.Hostname = conn
		}
		if u, err := url.Parse(base); err == nil {
			hostport := u.Host
			if u.Port() == "" {
				if u.Scheme == "https" {
					hostport = net.JoinHostPort(u.Hostname(), "443")
				} else {
					hostport = net.JoinHostPort(u.Hostname(), "80")
				}
			}
			t0 := time.Now()
			if c, err := net.DialTimeout("tcp", hostport, 3*time.Second); err == nil {
				si.Ping = float64(time.Since(t0).Microseconds()) / 1000
				c.Close()
			}
		}
		if si.Country == "" {
			si.Country = geoCountry(base)
		}
		si.OK = true
	}()
	return true
}

// countryFromLocale "th-TH" -> "TH" (ไม่นับค่าเริ่มต้น root-AQ)
func countryFromLocale(loc string) string {
	loc = strings.TrimSpace(loc)
	if i := strings.LastIndexAny(loc, "-_"); i >= 0 && len(loc)-i == 3 {
		cc := strings.ToUpper(loc[i+1:])
		if cc != "AQ" && cc >= "AA" && cc <= "ZZ" {
			return cc
		}
	}
	return ""
}

// geoCountry หาประเทศจาก IP ของเซิร์ฟ (ใช้เมื่อเซิร์ฟไม่ได้ตั้ง locale)
func geoCountry(base string) string {
	u, err := url.Parse(base)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	if strings.HasSuffix(host, ".cfx.re") {
		return "" // ซ่อน IP ผ่าน proxy ของ cfx หาประเทศจริงไม่ได้
	}
	ip := net.ParseIP(host)
	if ip == nil {
		if ips, err := net.LookupIP(host); err == nil && len(ips) > 0 {
			ip = ips[0]
		}
	}
	if ip == nil {
		return ""
	}
	c := &http.Client{Timeout: 4 * time.Second}
	resp, err := c.Get("http://ip-api.com/json/" + ip.String() + "?fields=countryCode")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var r struct {
		CountryCode string `json:"countryCode"`
	}
	json.NewDecoder(resp.Body).Decode(&r)
	return strings.ToUpper(r.CountryCode)
}

type serverStatus struct {
	ReqID   int     `json:"reqId"`
	OK      bool    `json:"ok"`
	Players int     `json:"players"`
	Max     int     `json:"maxPlayers"`
	Ping    float64 `json:"ping"`
}

// serverStatusCheck ดึงแค่จำนวนคนและปิง (เบา ใช้รีเฟรชบ่อย ๆ)
func (a *app) serverStatusCheck(base string, reqID int) bool {
	go func() {
		st := serverStatus{ReqID: reqID}
		defer func() { b, _ := json.Marshal(st); a.emit("onServerStatus", string(b)) }()
		if !strings.HasPrefix(base, "http") {
			return
		}
		if !strings.HasSuffix(base, "/") {
			base += "/"
		}
		t0 := time.Now()
		resp, err := httpGet(base + "dynamic.json")
		if err != nil {
			return
		}
		st.Ping = float64(time.Since(t0).Microseconds()) / 1000
		var dyn struct {
			Clients int `json:"clients"`
			Max     any `json:"sv_maxclients"`
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if json.Unmarshal(b, &dyn) != nil {
			return
		}
		st.Players, st.Max, st.OK = dyn.Clients, atoi(fmt.Sprint(dyn.Max)), true
		// ปิงแบบ TCP แม่นกว่าเวลาตอบ HTTP
		if u, err := url.Parse(base); err == nil {
			hp := u.Host
			if u.Port() == "" {
				if u.Scheme == "https" {
					hp = net.JoinHostPort(u.Hostname(), "443")
				} else {
					hp = net.JoinHostPort(u.Hostname(), "80")
				}
			}
			t1 := time.Now()
			if c, err := net.DialTimeout("tcp", hp, 3*time.Second); err == nil {
				st.Ping = float64(time.Since(t1).Microseconds()) / 1000
				c.Close()
			}
		}
	}()
	return true
}

type launchResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"m,omitempty"`
	Cmd     string `json:"cmd,omitempty"`
	Mode    string `json:"mode,omitempty"`
}

func writeIni(path, section, key, val string) {
	s, _ := windows.UTF16PtrFromString(section)
	k, _ := windows.UTF16PtrFromString(key)
	v, _ := windows.UTF16PtrFromString(val)
	p, _ := windows.UTF16PtrFromString(path)
	pWritePrivateProfile.Call(uintptr(unsafe.Pointer(s)), uintptr(unsafe.Pointer(k)), uintptr(unsafe.Pointer(v)), uintptr(unsafe.Pointer(p)))
}

func killFiveM() int {
	n := 0
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err := windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		name := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
		if strings.HasPrefix(name, "fivem") {
			if h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pe.ProcessID); err == nil {
				windows.TerminateProcess(h, 0)
				windows.CloseHandle(h)
				n++
			}
		}
	}
	return n
}

func fivemRunningAny() bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err := windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if strings.HasPrefix(strings.ToLower(windows.UTF16ToString(pe.ExeFile[:])), "fivem") {
			return true
		}
	}
	return false
}

// ---- ตรวจสถานะเกม ----

// gtaProcess คืน PID ของตัวเกม (FiveM_b3258_GTAProcess.exe / FiveM_GTAProcess.exe) ถ้ามี
func gtaProcess() (uint32, string) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, ""
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err := windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		name := windows.UTF16ToString(pe.ExeFile[:])
		l := strings.ToLower(name)
		if strings.HasPrefix(l, "fivem") && strings.Contains(l, "gtaprocess") {
			return pe.ProcessID, name
		}
	}
	return 0, ""
}

// menuReady = FiveM โหลดหน้าเมนูเสร็จแล้ว
// ตัวเกมจะ SetEvent("CitizenFX_PreUIExit") ตอนหน้าเมนูขึ้นครบ (UiDone ใน RenderHooks.cpp)
func menuReady() bool {
	for _, n := range []string{"CitizenFX_PreUIExit", "CitizenFX_PreUIExitCL2"} {
		p, _ := windows.UTF16PtrFromString(n)
		h, err := windows.OpenEvent(windows.SYNCHRONIZE, false, p)
		if err != nil {
			continue
		}
		r, _ := windows.WaitForSingleObject(h, 0)
		windows.CloseHandle(h)
		if r == windows.WAIT_OBJECT_0 {
			return true
		}
	}
	return false
}

var (
	enumPid     uint32
	enumVisible bool
	enumCb      = windows.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		var pid uint32
		windows.GetWindowThreadProcessId(h, &pid)
		if pid == enumPid && windows.IsWindowVisible(h) {
			var cls [64]uint16
			windows.GetClassName(h, &cls[0], 64)
			if windows.UTF16ToString(cls[:]) == "grcWindow" {
				enumVisible = true
				return 0
			}
		}
		return 1
	})
)

// gameWindowVisible = หน้าต่างเกมโชว์แล้ว (FiveM ซ่อนไว้จนหน้าเมนูโหลดเสร็จ)
func gameWindowVisible(pid uint32) bool {
	enumPid, enumVisible = pid, false
	windows.EnumWindows(enumCb, nil)
	return enumVisible
}

// logSince อ่าน log ล่าสุดของ FiveM ที่เขียนหลังเวลา t (ไฟล์เปิดอยู่ก็อ่านได้)
func logSince(fivemDir string, t time.Time) string {
	dir := filepath.Join(fivemDir, "FiveM.app", "logs")
	es, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var out strings.Builder
	for _, e := range es {
		if !strings.HasPrefix(e.Name(), "CitizenFX_log_") {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().After(t) {
			if f, err := os.Open(filepath.Join(dir, e.Name())); err == nil {
				if info.Size() > 4<<20 {
					f.Seek(info.Size()-(4<<20), 0)
				}
				b, _ := io.ReadAll(f)
				f.Close()
				out.Write(b)
			}
		}
	}
	return out.String()
}

func explorerOpen(target string) error {
	return exec.Command(filepath.Join(os.Getenv("SystemRoot"), "explorer.exe"), target).Start()
}

// launchServer เปิดเกมแบบ 2 จังหวะ ให้เข้าเซิร์ฟได้ในการเปิดเกมรอบเดียว
//
//  1. ปิด FiveM เดิม แล้วตั้ง CitizenFX.ini ให้ build / pure / pool ตรงกับเซิร์ฟ (ค่าเดียวกับที่ FiveM เขียนเองตอนสลับ build)
//  2. เปิด FiveM เปล่า ๆ ผ่าน Explorer (ยังไม่ส่ง IP)
//  3. รอจนหน้าเมนูโหลดเสร็จจริง + รอเพิ่มอีกนิด
//  4. ค่อยส่ง fivem://connect/IP เข้าไปในเกมที่เปิดอยู่ (ไม่เปิดเกมใหม่) แล้วเช็ก log ว่ากำลังเชื่อมต่อจริง
//
// force = ปิดเกมแล้วเปิดใหม่เสมอ (ใช้กับ "รีเกมแล้วเข้าเซิร์ฟเดิม")
func (a *app) launchServer(fivemDir, connect string, build, pure, effDefault int, pool string, force bool, step func(string)) launchResult {
	exe := filepath.Join(fivemDir, "FiveM.exe")
	if _, err := os.Stat(exe); err != nil {
		return launchResult{Message: "ไม่เจอ FiveM.exe ใน " + fivemDir}
	}
	link := "fivem://connect/" + connect

	// เกมเปิดอยู่ที่หน้าเมนูแล้ว: ส่ง IP เข้าไปเลย ไม่ต้องปิดเกม
	// (ถ้า build/pure/pool ไม่ตรง FiveM จะสลับเองแล้วต่อเซิร์ฟให้อัตโนมัติ)
	if pid, _ := gtaProcess(); !force && pid != 0 && (menuReady() || gameWindowVisible(pid)) {
		step("เกมเปิดอยู่ ส่ง IP เข้าไปเลย")
		if r, done := sendConnect(fivemDir, link, time.Now().Add(-2*time.Second), 1, step); done {
			r.Mode = "ไม่รีเกม"
			return r
		}
		// เกมไม่รับลิงก์ (มักเพราะอยู่ในเซิร์ฟอื่นอยู่) ต้องปิดแล้วเปิดใหม่
	}

	// FiveM กำลังเปิดอยู่แต่ยังไม่ถึงหน้าเมนู: รอมันโหลดต่อ ไม่ต้องปิดทิ้ง
	reuse := false
	if pid, _ := gtaProcess(); !force && fivemRunningAny() && !(pid != 0 && menuReady()) {
		reuse = true
	}
	mode := "ปกติ"
	start := time.Now().Add(-2 * time.Second)
	if reuse {
		mode = "ไม่รีเกม"
	} else {
		step("ปิดเกมเดิม")
		if killFiveM() > 0 {
			for i := 0; i < 20 && fivemRunningAny(); i++ {
				time.Sleep(250 * time.Millisecond)
			}
			time.Sleep(1500 * time.Millisecond)
		}

		ini := filepath.Join(fivemDir, "FiveM.app", "CitizenFX.ini")
		if _, err := os.Stat(ini); err == nil && build > 0 {
			writeIni(ini, "Game", "PoolSizesIncrease", pool)
			writeIni(ini, "Game", "DefaultBuild", strconv.Itoa(effDefault))
			writeIni(ini, "Game", "PureLevel", strconv.Itoa(pure))
			writeIni(ini, "Game", "SavedBuildNumber", strconv.Itoa(build))
			mode = fmt.Sprintf("b%d", build)
		}

		start = time.Now().Add(-2 * time.Second)
		step("กำลังเปิด FiveM")
		if err := explorerOpen(exe); err != nil {
			return launchResult{Message: "เปิด FiveM ไม่ได้: " + err.Error()}
		}
	}

	// รอหน้าเมนู: สูงสุด 6 นาที (ครั้งแรกของ build ใหม่ FiveM ต้องโหลดไฟล์ก่อน)
	step("รอเกมโหลดหน้าเมนู")
	deadline := time.Now().Add(6 * time.Minute)
	var pid uint32
	readySince := time.Time{}
	sawAny := false
	for {
		if time.Now().After(deadline) {
			return launchResult{Message: "รอหน้าเมนู FiveM นานเกินไป ถ้าเกมขึ้นแล้วให้กดเข้าอีกครั้ง", Cmd: link, Mode: mode}
		}
		if fivemRunningAny() {
			sawAny = true
		} else if sawAny && time.Since(start) > 20*time.Second {
			// FiveM ปิดตัวไปเอง (อาจปิดหน้าต่างอัปเดต หรือเกมเด้ง)
			time.Sleep(3 * time.Second)
			if !fivemRunningAny() {
				return launchResult{Message: "FiveM ปิดตัวไประหว่างโหลด ลองเปิด FiveM เองหนึ่งครั้งให้ขึ้นหน้าเมนู แล้วกดเข้าใหม่", Cmd: link, Mode: mode}
			}
		} else if !sawAny && time.Since(start) > 40*time.Second {
			return launchResult{Message: "FiveM ไม่เปิดขึ้นมา ลองเปิด FiveM เองหนึ่งครั้ง แล้วกดเข้าใหม่", Cmd: link, Mode: mode}
		}
		p, _ := gtaProcess()
		ready := p != 0 && (menuReady() || gameWindowVisible(p) || strings.Contains(logSince(fivemDir, start), "Settings loaded!"))
		if p != pid {
			pid, readySince = p, time.Time{} // ตัวเกมเปลี่ยน (FiveM รีเกมเพื่อสลับ build) เริ่มนับใหม่
		}
		if ready {
			if readySince.IsZero() {
				readySince = time.Now()
				step("หน้าเมนูขึ้นแล้ว รอให้นิ่ง")
			} else if time.Since(readySince) >= 4*time.Second {
				break
			}
		} else {
			readySince = time.Time{}
		}
		time.Sleep(700 * time.Millisecond)
	}

	if r, done := sendConnect(fivemDir, link, start, 3, step); done {
		r.Mode = mode
		return r
	}
	return launchResult{Message: "ส่ง IP แล้วแต่เกมยังไม่เชื่อมต่อ ลองกดเข้าในเกมเอง หรือกดเข้าอีกครั้ง", Cmd: link, Mode: mode}
}

// sendConnect ส่ง fivem://connect เข้าเกมที่เปิดอยู่ (FiveM.exe ตัวใหม่จะส่งต่อทาง IPC แล้วปิดตัวเอง ไม่เปิดเกมซ้อน)
// แล้วอ่าน log ว่าเกมเริ่มเชื่อมต่อจริง  done=false แปลว่าเกมไม่ตอบเลย
func sendConnect(fivemDir, link string, since time.Time, tries int, step func(string)) (launchResult, bool) {
	for try := 1; try <= tries; try++ {
		sent := time.Now()
		step(fmt.Sprintf("ส่ง IP เข้าเกม (ครั้งที่ %d)", try))
		before := len(logSince(fivemDir, since))
		explorerOpen(link)
		for i := 0; i < 24; i++ { // ~12 วินาที
			time.Sleep(500 * time.Millisecond)
			lg := logSince(fivemDir, since)
			if len(lg) > before {
				tail := lg[before:]
				if k := strings.LastIndex(tail, "OnConnectionError"); k >= 0 {
					msg := tail[k:]
					if n := strings.IndexByte(msg, '\n'); n > 0 {
						msg = msg[:n]
					}
					msg = strings.TrimSpace(strings.TrimPrefix(msg, "OnConnectionError:"))
					return launchResult{Message: "เซิร์ฟตอบกลับ: " + msg, Cmd: link}, true
				}
				for _, k := range []string{"OnConnectionProgress", "Connecting to server", "Switching from build", "already connecting"} {
					if strings.Contains(tail, k) {
						return launchResult{OK: true, Cmd: link}, true
					}
				}
			}
			if pid, _ := gtaProcess(); pid == 0 && time.Since(sent) > 5*time.Second {
				return launchResult{Message: "เกมปิดตัวหลังส่ง IP ดู log ใน FiveM.app\\logs", Cmd: link}, true
			}
		}
		if logSince(fivemDir, since) == "" {
			return launchResult{OK: true, Cmd: link}, true // อ่าน log ไม่ได้ ถือว่าส่งแล้ว
		}
	}
	return launchResult{}, false
}

// ---- รายการเซิร์ฟที่เคยเข้า ----
func (a *app) recentPath() string { return filepath.Join(a.dataDir, "servers.json") }
func (a *app) loadRecent() string {
	b, err := os.ReadFile(a.recentPath())
	if err != nil || !json.Valid(b) {
		return "[]"
	}
	return string(b)
}
func (a *app) saveRecent(s string) bool {
	if !json.Valid([]byte(s)) {
		return false
	}
	return os.WriteFile(a.recentPath(), []byte(s), 0o644) == nil
}

// ---- ตรวจว่า build ที่เซิร์ฟต้องการ มีไฟล์ในเครื่องแล้วหรือยัง ----
// ข้อมูลไฟล์ของแต่ละ build มาจาก g_entriesToLoadPerBuild ใน code/client/launcher/GameCache.cpp ของ FiveM
//
//go:embed builds.json
var buildsJSON []byte

type buildFile struct {
	F string `json:"f"` // ชื่อไฟล์ใน data\game-storage
	N int64  `json:"n"` // ขนาดเต็ม (byte)
}

type buildCheck struct {
	Known     bool    `json:"known"`
	Cached    bool    `json:"cached"`
	MissingMB float64 `json:"missingMB"`
}

var buildTable map[string][]buildFile

func (a *app) checkBuild(fivemDir string, build int) buildCheck {
	if buildTable == nil {
		json.Unmarshal(buildsJSON, &buildTable)
	}
	files, ok := buildTable[strconv.Itoa(build)]
	if !ok || fivemDir == "" {
		return buildCheck{}
	}
	dir := filepath.Join(fivemDir, "FiveM.app", "data", "game-storage")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return buildCheck{Known: true, MissingMB: totalMB(files)}
	}
	// รายการไฟล์ในเครื่อง: ชื่อจริง + ขนาด
	type fe struct {
		name string
		size int64
	}
	var have []fe
	for _, e := range entries {
		if info, err := e.Info(); err == nil && !e.IsDir() {
			have = append(have, fe{e.Name(), info.Size()})
		}
	}
	res := buildCheck{Known: true, Cached: true}
	for _, f := range files {
		found := false
		prefix := f.F[:strings.LastIndex(f.F, "_")+1] // เช่น "update+update.rpf_"
		for _, h := range have {
			// ตรงชื่อเป๊ะ (hash เดียวกัน) หรือไฟล์ชนิดเดียวกันที่ขนาดเท่ากัน (FiveM บางเวอร์ชันใช้ hash ต่างกัน)
			if h.name == f.F || (strings.HasPrefix(h.name, prefix) && h.size == f.N) {
				found = h.size > 0
				break
			}
		}
		if !found {
			res.Cached = false
			res.MissingMB += float64(f.N) / (1 << 20)
		}
	}
	return res
}

func totalMB(files []buildFile) float64 {
	var t int64
	for _, f := range files {
		t += f.N
	}
	return float64(t) / (1 << 20)
}

// launchServerAsync เรียก launchServer ในเบื้องหลัง (ไม่ให้หน้าต่างค้าง) ผลส่งไป window.onLaunch
func (a *app) launchServerAsync(fivemDir, connect string, build, pure, effDefault int, pool string, force bool, reqID int) bool {
	go func() {
		r := a.launchServer(fivemDir, connect, build, pure, effDefault, pool, force, func(t string) {
			b, _ := json.Marshal(map[string]any{"reqId": reqID, "t": t})
			a.emit("onLaunchStep", string(b))
		})
		b, _ := json.Marshal(struct {
			launchResult
			ReqID int `json:"reqId"`
		}{r, reqID})
		a.emit("onLaunch", string(b))
	}()
	return true
}

// ---- ดัน Priority ของ FiveM เป็น High อัตโนมัติขณะเปิดโปรแกรม (ไม่แก้ registry) ----
func (a *app) boostPriority() {
	var cfg struct {
		AutoPrio *bool `json:"autoPrio"`
	}
	json.Unmarshal([]byte(a.loadSettings()), &cfg)
	if cfg.AutoPrio != nil && !*cfg.AutoPrio {
		return
	}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err := windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		name := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
		if strings.HasPrefix(name, "fivem") && strings.Contains(name, "gtaprocess") {
			if h, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pe.ProcessID); err == nil {
				if pc, _ := windows.GetPriorityClass(h); pc != windows.HIGH_PRIORITY_CLASS {
					windows.SetPriorityClass(h, windows.HIGH_PRIORITY_CLASS)
				}
				windows.CloseHandle(h)
			}
		}
	}
}
