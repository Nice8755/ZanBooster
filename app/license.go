//go:build windows

package main

// license.go: ถอดจาก exe 4.11.15 (โค้ดเดิมหาย) ทุกค่าคงที่ ชื่อไฟล์ ชื่อในรีจิสทรี ข้อความ และลำดับขั้นตอน
// อ่านมาจากตัวเก่า เพื่อให้คีย์ / สิทธิ์ทดลองของลูกค้าเดิมใช้ต่อได้เหมือนเดิม ห้ามเปลี่ยนค่าโดยไม่จำเป็น
//
// ไฟล์ C:\ProgramData\ZanBooster\license.json  + สำเนาใน HKLM\SOFTWARE\ZanBooster (k = คีย์, t = "เวลาเริ่มทดลอง:ลายเซ็น", s = เวลาล่าสุดที่เห็น)
// เซิร์ฟเวอร์คีย์: remote.json "licApi" หรือ licAPIDefault · POST JSON {key, ref, trialHours, machine, v}

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows/registry"
)

const licAPIDefaultURL = "https://zan-license.zanhub.workers.dev"

var licAPIDefault = licAPIDefaultURL

// licLegacyVer: ตัวเก่าส่ง "4.11.15" ตายตัวในช่อง v และ User-Agent (เซิร์ฟเวอร์อาจดูค่านี้) ตัวใหม่ส่งเลขจริง
func licVer() string { return appVersion }

type licFile struct {
	Key      string `json:"key,omitempty"`
	Short    string `json:"sk,omitempty"`
	Trial    int64  `json:"t,omitempty"`
	TrialSig string `json:"ts,omitempty"`
	Seen     int64  `json:"s,omitempty"`
}

type licReply struct {
	OK   bool   `json:"ok"`
	Code string `json:"code"`
	Lic  string `json:"lic"`
	Lic2 string `json:"lic2"`
	Ref  string `json:"ref"`
}

type licState struct {
	Premium   bool   `json:"premium"`
	Mode      string `json:"mode"`
	Expires   int64  `json:"expires"`
	HoursLeft int    `json:"hoursLeft"`
	TrialUsed bool   `json:"trialUsed"`
	Machine   string `json:"machine"`
	Msg       string `json:"msg,omitempty"`
	RefMsg    string `json:"refMsg,omitempty"`
}

var (
	licGuid    string
	licMu      sync.Mutex
	licCache   licState
	licCacheAt time.Time
	licMigOnce sync.Once
)

func machineGuid() string {
	if licGuid != "" {
		return licGuid
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err == nil {
		defer k.Close()
		if v, _, err := k.GetStringValue("MachineGuid"); err == nil && v != "" {
			licGuid = v
			return v
		}
	}
	h, _ := os.Hostname()
	licGuid = "host:" + h
	return licGuid
}

func licDataDir() string { return filepath.Join(os.Getenv("ProgramData"), "ZanBooster") }

func trialSig(t int64) string {
	s := sha256.Sum256([]byte(fmt.Sprintf("zb-trial|%d|%s", t, strings.ToLower(machineGuid()))))
	return hex.EncodeToString(s[:8])
}

func licLoad() licFile {
	f := new(licFile)
	if b, err := os.ReadFile(filepath.Join(licDataDir(), "license.json")); err == nil {
		_ = json.Unmarshal(b, f)
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\ZanBooster`, registry.QUERY_VALUE)
	if err != nil {
		return *f
	}
	defer k.Close()
	// สิทธิ์ทดลอง: เก็บในรีจิสทรีด้วย ลบ license.json แล้วขอทดลองใหม่ไม่ได้
	if v, _, err := k.GetStringValue("t"); err == nil {
		if p := strings.SplitN(v, ":", 2); len(p) == 2 {
			// ใช้ค่าจากรีจิสทรีถ้าไฟล์ไม่มี / ไฟล์มีค่าที่ใหม่กว่า / ลายเซ็นในไฟล์ไม่ตรง (ลายเซ็นตรวจอีกทีตอนคำนวณสถานะ)
			if t, err := strconv.ParseInt(p[0], 10, 64); err == nil && (f.Trial == 0 || t < f.Trial || trialSig(f.Trial) != f.TrialSig) {
				f.Trial, f.TrialSig = t, p[1]
			}
		}
	}
	// เวลาล่าสุดที่เห็น: กันย้อนนาฬิกาเครื่องเพื่อยืดวันหมดอายุ
	if v, _, err := k.GetStringValue("s"); err == nil {
		if s, err := strconv.ParseInt(v, 10, 64); err == nil && f.Seen < s {
			f.Seen = s
		}
	}
	if f.Key == "" {
		if v, _, err := k.GetStringValue("k"); err == nil {
			f.Key = v
		}
	}
	return *f
}

func licSave(f licFile) {
	_ = os.MkdirAll(licDataDir(), 0o755)
	b, _ := json.Marshal(f)
	_ = os.WriteFile(filepath.Join(licDataDir(), "license.json"), b, 0o644)
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, `SOFTWARE\ZanBooster`, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	if f.Trial != 0 {
		_ = k.SetStringValue("t", fmt.Sprintf("%d:%s", f.Trial, f.TrialSig))
	}
	_ = k.SetStringValue("s", strconv.FormatInt(f.Seen, 10))
	if f.Key != "" {
		_ = k.SetStringValue("k", f.Key)
	} else {
		_ = k.DeleteValue("k")
	}
}

func licCompute() licState {
	f := licLoad()
	now := time.Now().Unix()
	if f.Seen > now {
		now = f.Seen
	} else if now-f.Seen > 600 {
		f.Seen = now
		licSave(f)
	}
	st := licState{Mode: "free", Machine: machineCode(machineGuid()), TrialUsed: f.Trial != 0}
	if f.Key != "" {
		if _, exp, err := parseLicKey(f.Key, machineBytes(machineGuid())); err == nil {
			if exp.IsZero() {
				st.Premium, st.Mode = true, "lifetime"
				return st
			}
			if left := exp.Unix() - now; left > 0 {
				st.Premium, st.Mode, st.Expires, st.HoursLeft = true, "timed", exp.Unix(), int((left+3599)/3600)
				return st
			}
			st.Msg = "คีย์หมดอายุแล้ว"
		}
	}
	if f.Trial != 0 && trialSig(f.Trial) == f.TrialSig {
		end := f.Trial + 3*24*3600
		if end-now > 0 && now >= f.Trial-600 {
			st.Premium, st.Mode, st.Expires, st.HoursLeft = true, "trial", end, int((end-now+3599)/3600)
			return st
		}
	}
	return st
}

// licGet: คำนวณใหม่ไม่เกินทุก 20 วินาที
func licGet() licState {
	licMu.Lock()
	defer licMu.Unlock()
	if time.Since(licCacheAt) > 20*time.Second {
		licCache = licCompute()
		licCacheAt = time.Now()
	}
	return licCache
}

// licMigrate: คีย์สั้น (ZAN-XXXX...) ที่ใส่ไว้แต่ยังไม่มีคีย์ยาว → ขอคีย์ยาวจากเซิร์ฟเวอร์ครั้งเดียว
func (a *app) licMigrate() {
	f := licLoad()
	if f.Key == "" || !isShortKey(f.Key) {
		return
	}
	r, err := a.licCall("/v1/activate", f.Key)
	if err != nil || !r.OK || r.Lic == "" {
		return
	}
	if _, _, err := parseLicKey(r.Lic, machineBytes(machineGuid())); err != nil {
		return
	}
	f.Short, f.Key = f.Key, r.Lic
	licSave(f)
}

func (a *app) licInfo() licState {
	licMigOnce.Do(func() {
		done := make(chan struct{})
		go func() { a.licMigrate(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	})
	licMu.Lock()
	licCacheAt = time.Time{}
	licMu.Unlock()
	return licGet()
}

func (a *app) licAPI() string {
	var c struct {
		LicAPI string `json:"licApi"`
	}
	_ = json.Unmarshal([]byte(a.remoteRaw()), &c)
	if s := strings.TrimRight(strings.TrimSpace(c.LicAPI), "/"); strings.HasPrefix(s, "https://") {
		return s
	}
	return strings.TrimRight(licAPIDefault, "/")
}

func (a *app) licCall(path, key string) (licReply, error) {
	r := new(licReply)
	err := a.licPost(path, map[string]string{"key": key}, r)
	if r.Lic2 != "" {
		r.Lic = r.Lic2
	}
	return *r, err
}

func (a *app) licPost(path string, body map[string]string, out any) error {
	api := a.licAPI()
	if api == "" {
		return fmt.Errorf("no server")
	}
	body["machine"] = machineCode(machineGuid())
	body["v"] = licVer()
	b, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", api+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ZanBooster/"+licVer())
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

var refCodeMsg = map[string]string{
	"ok":      "ใช้โค้ดเพื่อนแล้ว ได้เพิ่ม 3 วันทั้งคุณและเพื่อน",
	"invalid": "โค้ดเพื่อนไม่ถูกต้อง (คีย์ใช้ได้ตามปกติ)",
	"self":    "ใช้โค้ดของตัวเองไม่ได้ (คีย์ใช้ได้ตามปกติ)",
	"used":    "เครื่องนี้เคยใช้โค้ดเพื่อนไปแล้ว (คีย์ใช้ได้ตามปกติ)",
}

var licCodeMsg = map[string]string{
	"invalid": "คีย์ไม่ถูกต้อง ตรวจว่าพิมพ์ครบและถูกทุกตัว",
	"used":    "คีย์นี้ถูกใช้กับเครื่องอื่นแล้ว (1 คีย์ใช้ได้ 1 เครื่อง) ถ้าเปลี่ยนเครื่อง ติดต่อผู้ขายใน Discord",
	"revoked": "คีย์นี้ถูกยกเลิกแล้ว ติดต่อผู้ขายใน Discord",
	"expired": "คีย์นี้หมดอายุแล้ว",
	"machine": "อ่านรหัสเครื่องไม่ได้ ลองปิดแล้วเปิดโปรแกรมใหม่",
}

// refInfo: ข้อมูลระบบชวนเพื่อนของเครื่องนี้ (โค้ดของฉัน, จำนวนคนที่ชวน ...) ส่งต่อให้หน้าจอตามที่เซิร์ฟเวอร์ตอบ
func (a *app) refInfo() string {
	out := new(map[string]any)
	if err := a.licPost("/v1/ref", map[string]string{}, out); err != nil || *out == nil {
		return `{"ok":false}`
	}
	b, _ := json.Marshal(*out)
	go a.licBonus()
	return string(b)
}

// licBonus: รับวันโบนัส (จากการชวนเพื่อน) ถ้าเซิร์ฟเวอร์ให้คีย์ที่หมดช้ากว่าเดิม
func (a *app) licBonus() {
	f := licLoad()
	mb := machineBytes(machineGuid())
	var cur int64
	if f.Key != "" {
		_, exp, err := parseLicKey(f.Key, mb)
		if err == nil && exp.IsZero() {
			return // ถาวรอยู่แล้ว
		}
		if err == nil {
			cur = exp.Unix()
		} else {
			cur = time.Time{}.Unix()
		}
	}
	if f.Short != "" && cur > time.Now().Unix() {
		return
	}
	r := new(licReply)
	err := a.licPost("/v1/bonus", map[string]string{}, r)
	if r.Lic2 != "" {
		r.Lic = r.Lic2
	}
	if err != nil || !r.OK || r.Lic == "" {
		return
	}
	_, exp, err := parseLicKey(r.Lic, mb)
	if err != nil || (!exp.IsZero() && cur >= exp.Unix()) {
		return
	}
	f.Key = r.Lic
	licSave(f)
	a.licInfo()
}

func normKey(k string) string { return strings.ToUpper(strings.Join(strings.Fields(k), "")) }

// licActivate: ใส่คีย์ (คีย์สั้นถามเซิร์ฟเวอร์ / คีย์ยาวตรวจในเครื่อง) ref = โค้ดเพื่อน
func (a *app) licActivate(key, ref string) licState {
	fail := func(m string) licState { st := a.licInfo(); st.Msg = m; return st }
	long, short, refMsg := key, "", ""
	if isShortKey(key) {
		r := new(licReply)
		body := map[string]string{"key": key}
		if ref = strings.TrimSpace(ref); ref != "" {
			body["ref"] = ref
		}
		if st := licCompute(); st.Mode == "trial" && st.HoursLeft > 0 {
			body["trialHours"] = strconv.Itoa(st.HoursLeft)
		}
		if err := a.licPost("/v1/activate", body, r); err != nil {
			return fail("ต่อเซิร์ฟเวอร์ตรวจคีย์ไม่ได้ ตรวจอินเทอร์เน็ตแล้วลองใหม่")
		}
		if r.Lic2 != "" {
			r.Lic = r.Lic2
		}
		refMsg = refCodeMsg[r.Ref]
		if !r.OK || r.Lic == "" {
			if m, ok := licCodeMsg[r.Code]; ok {
				return fail(m)
			}
			return fail("ใช้คีย์นี้ไม่ได้ (" + r.Code + ")")
		}
		short, long = normKey(key), r.Lic
	}
	_, exp, err := parseLicKey(long, machineBytes(machineGuid()))
	if err != nil {
		return fail(err.Error())
	}
	f := licLoad()
	now := time.Now().Unix()
	if f.Seen > now {
		now = f.Seen
	}
	if !exp.IsZero() && now >= exp.Unix() {
		return fail("คีย์นี้หมดอายุแล้ว")
	}
	f.Key, f.Short = normKey(long), short
	licSave(f)
	st := a.licInfo()
	st.RefMsg = refMsg
	return st
}

// licRecheck: ทำงานเบื้องหลังตลอด ทุก 30 นาที ตรวจคีย์สั้นกับเซิร์ฟเวอร์ (ต่ออายุ / ยกเลิก)
func (a *app) licRecheck() {
	time.Sleep(5 * time.Second)
	for {
		a.licMigrate()
		a.licBonus()
		if f := licLoad(); f.Key != "" && f.Short != "" {
			if r, err := a.licCall("/v1/check", f.Short); err == nil {
				if r.OK && r.Lic != "" && r.Lic != f.Key {
					if _, _, err := parseLicKey(r.Lic, machineBytes(machineGuid())); err == nil {
						f.Key = r.Lic
						licSave(f)
						a.licInfo()
					}
				} else if !r.OK {
					switch r.Code {
					case "revoked", "invalid", "used", "unused":
						f.Key, f.Short = "", ""
						licSave(f)
					}
				}
			}
		}
		time.Sleep(30 * time.Minute)
	}
}

func (a *app) licStartTrial() licState {
	f := licLoad()
	if f.Trial != 0 {
		st := a.licInfo()
		st.Msg = "เครื่องนี้ใช้สิทธิ์ทดลองฟรีไปแล้ว"
		return st
	}
	now := time.Now().Unix()
	if f.Seen > now {
		now = f.Seen
	}
	f.Trial, f.TrialSig, f.Seen = now, trialSig(now), now
	licSave(f)
	return a.licInfo()
}

// remoteRaw: remote.json ที่โปรแกรมโหลดมาเก็บไว้ (ตั้งค่าจากเน็ต) ไม่มี / เสีย = "{}"
func (a *app) remoteRaw() string {
	b, err := os.ReadFile(filepath.Join(a.dataDir, "remote.json"))
	if err != nil || !json.Valid(b) {
		return "{}"
	}
	return string(b)
}
