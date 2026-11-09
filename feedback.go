package main

// ส่งรายงานปัญหาเข้า Discord (#รายงานปัญหา ใน ZAN Hub) ผ่าน webhook
// ส่งแค่ข้อความที่ผู้ใช้พิมพ์ + zip ของ log/สเปกเครื่อง (ไม่มีรหัสผ่านหรือข้อมูลบัญชี)
// ลิงก์ webhook ไม่อยู่ในซอร์สโค้ด: ใส่ตอน build ด้วย -ldflags "-X main.reportWebhook=..."

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var reportWebhook string

var reportMu sync.Mutex

func trimRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return string(r)
}

func (a *app) canSendReport() bool { return reportWebhook != "" }

// sendReport อัปโหลด zip รายงาน (ที่ engine สร้างไว้ใน TEMP) พร้อมข้อความ แล้วลบไฟล์ทิ้ง  คืนข้อความ error ("" = สำเร็จ)
func (a *app) sendReport(zipPath, desc, contact string) string {
	reportMu.Lock()
	defer reportMu.Unlock()
	if reportWebhook == "" {
		return "เวอร์ชันนี้ส่งรายงานไม่ได้"
	}
	// รับเฉพาะไฟล์รายงานที่ engine สร้างใน TEMP
	tmp, _ := filepath.Abs(os.TempDir())
	zp, _ := filepath.Abs(zipPath)
	if !strings.HasPrefix(strings.ToLower(zp), strings.ToLower(tmp)) || !strings.HasPrefix(filepath.Base(zp), "ZanBooster-report-") || filepath.Ext(zp) != ".zip" {
		return "ไฟล์รายงานไม่ถูกต้อง"
	}
	defer os.Remove(zp)
	// กันส่งถี่: 1 ครั้งต่อ 3 นาที
	stamp := filepath.Join(a.dataDir, "report_last.txt")
	if b, err := os.ReadFile(stamp); err == nil {
		var t int64
		fmt.Sscan(string(b), &t)
		if wait := 180 - (time.Now().Unix() - t); wait > 0 && wait <= 180 {
			return fmt.Sprintf("เพิ่งส่งไป รออีก %d วินาทีแล้วค่อยส่งใหม่", wait)
		}
	}
	data, err := os.ReadFile(zp)
	if err != nil {
		return "อ่านไฟล์รายงานไม่ได้"
	}
	if len(data) > 9*1024*1024 {
		return "ไฟล์รายงานใหญ่เกินไป"
	}
	desc = trimRunes(desc, 1500)
	if desc == "" {
		desc = "(ไม่ได้พิมพ์รายละเอียด)"
	}
	contact = trimRunes(contact, 80)
	if contact == "" {
		contact = "-"
	}
	payload := map[string]any{
		"username":         "Zan Booster",
		"allowed_mentions": map[string]any{"parse": []string{}},
		"embeds": []map[string]any{{
			"title":       "รายงานปัญหา · Zan Booster v" + appVersion,
			"description": desc,
			"color":       0x00E5FF,
			"fields": []map[string]any{
				{"name": "ติดต่อกลับ", "value": contact, "inline": true},
				{"name": "Windows build", "value": fmt.Sprint(winBuild()), "inline": true},
			},
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		}},
	}
	pj, _ := json.Marshal(payload)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("payload_json", string(pj))
	fw, _ := mw.CreateFormFile("files[0]", filepath.Base(zp))
	fw.Write(data)
	mw.Close()
	req, _ := http.NewRequest("POST", reportWebhook, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("User-Agent", "ZanBooster/"+appVersion)
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return "ส่งไม่ได้ เช็กอินเทอร์เน็ตแล้วลองใหม่"
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Sprintf("ส่งไม่สำเร็จ (HTTP %d)", resp.StatusCode)
	}
	os.WriteFile(stamp, []byte(fmt.Sprint(time.Now().Unix())), 0o644)
	return ""
}
