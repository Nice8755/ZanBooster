//go:build windows

package main

// session.go (4.20.07): สรุปหลังเล่นจบ แจ้งเตือนมุมจอหลังปิดเกม + เก็บประวัติรอบเล่นไว้ให้หน้าโปรแกรม
// ตัวเลขทุกตัวมาจากสิ่งที่ตัวช่วยทำหรือวัดได้จริงระหว่างเล่น ไม่มีตัวเลข FPS เดา (FPS ดูจากการวัดในหน้าวัดผล)

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type gameSession struct {
	At      int64   `json:"at"`  // เวลาเริ่ม (unix)
	Min     int     `json:"min"` // นาทีที่เล่น
	RamN    int     `json:"ramN"`
	RamMB   float64 `json:"ramMB"`
	Purges  int     `json:"purges"`
	BgN     int     `json:"bgN"`
	GPU     float64 `json:"gpu"`
	CPUMax  float64 `json:"cpuMax"`
	Verdict string  `json:"vid,omitempty"`
}

func (a *app) sessionsPath() string { return filepath.Join(a.dataDir, "sessions.json") }

// gameSessions: ประวัติรอบเล่น (หน้าโปรแกรมใช้ในสรุปสัปดาห์)
func (a *app) gameSessions() string {
	b, err := os.ReadFile(a.sessionsPath())
	if err != nil {
		return "[]"
	}
	return string(b)
}

func (a *app) sessionSummary(st gmStatus, bgPeak int) {
	min := int(time.Since(time.Unix(st.Since, 0)).Minutes())
	if min < 10 { // เข้าแป๊บเดียว (เช่นรีเกมสลับ build) ไม่ต้องสรุป
		return
	}
	s := gameSession{At: st.Since, Min: min, RamN: st.ArN, RamMB: st.ArMB, Purges: st.Purges, BgN: bgPeak, GPU: st.GPU, CPUMax: st.CPUMax, Verdict: st.VerdictID}
	var all []gameSession
	if b, err := os.ReadFile(a.sessionsPath()); err == nil {
		_ = json.Unmarshal(b, &all)
	}
	all = append(all, s)
	if len(all) > 120 {
		all = all[len(all)-120:]
	}
	if b, err := json.Marshal(all); err == nil {
		_ = os.WriteFile(a.sessionsPath(), b, 0o644)
	}
	if c := a.gmConfig(); c.Sum != nil && !*c.Sum {
		return
	}
	dur := fmt.Sprintf("%d นาที", min)
	if min >= 60 {
		dur = fmt.Sprintf("%d ชม. %d นาที", min/60, min%60)
	}
	var parts []string
	if st.ArN > 0 {
		parts = append(parts, fmt.Sprintf("ล้างแรมให้ %d ครั้ง (คืน %.1f GB)", st.ArN, st.ArMB/1024))
	} else if st.Purges > 0 {
		parts = append(parts, fmt.Sprintf("ล้างแรมให้ %d ครั้ง", st.Purges))
	}
	if bgPeak > 0 {
		parts = append(parts, fmt.Sprintf("ให้เกมได้ซีพียูก่อน %d โปรแกรม", bgPeak))
	}
	if st.GPU > 0 {
		parts = append(parts, fmt.Sprintf("การ์ดจอทำงานเฉลี่ย %.0f%%", st.GPU))
	}
	body := strings.Join(parts, " · ")
	switch st.VerdictID {
	case "gpu":
		body += "\nการ์ดจอทำงานเต็ม ลองกดวัดผลแล้วล็อกเพดาน FPS จะลดดีเลย์"
	case "cpu":
		body += "\nติดที่ซีพียู/สคริปต์เซิร์ฟ ลดกราฟิกช่วยได้น้อย"
	}
	toastNotify("Zan Booster · เล่นไป "+dur, strings.TrimSpace(body))
}
