package main

// ล้างไฟล์เก่าในแคชเซิร์ฟ
// FiveM เก็บไฟล์ของทุกเซิร์ฟรวมกันใน data\server-cache (ตั้งชื่อตาม hash ไม่ได้แยกโฟลเดอร์ตามเซิร์ฟ)
// จึงลบเป็นรายไฟล์ตามเวลาที่ใช้ล่าสุดแทน ไฟล์ที่หายไป FiveM จะโหลดใหม่เองตอนเข้าเซิร์ฟ (ResourceCacheDeviceV2::ExistsOnDisk)
// ไม่แตะโฟลเดอร์ db (ดัชนีของแคช)

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type cacheInfo struct {
	ReqID    int     `json:"reqId"`
	OK       bool    `json:"ok"`
	Message  string  `json:"m,omitempty"`
	TotalMB  float64 `json:"totalMB"`
	Files    int     `json:"files"`
	OldMB    float64 `json:"oldMB"`
	OldFiles int     `json:"oldFiles"`
	LogsMB   float64 `json:"logsMB"`
	Days     int     `json:"days"`
	FreedMB  float64 `json:"freedMB,omitempty"`
	Deleted  int     `json:"deleted,omitempty"`
	Running  bool    `json:"running"`
}

// lastUsed = เวลาล่าสุดในบรรดา สร้าง/แก้ไข/เปิดอ่าน (บางเครื่องปิดการบันทึกเวลาเปิดอ่านไว้ จึงดูทั้ง 3 ค่า)
func lastUsed(info fs.FileInfo) time.Time {
	t := info.ModTime()
	if d, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		for _, ft := range []syscall.Filetime{d.CreationTime, d.LastAccessTime} {
			if x := time.Unix(0, ft.Nanoseconds()); x.After(t) {
				t = x
			}
		}
	}
	return t
}

func walkCache(fivemDir string, days int, del bool) cacheInfo {
	res := cacheInfo{OK: true, Days: days}
	cut := time.Now().AddDate(0, 0, -days)
	data := filepath.Join(fivemDir, "FiveM.app", "data")
	es, _ := os.ReadDir(data)
	for _, e := range es {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "server-cache") {
			continue
		}
		root := filepath.Join(data, e.Name())
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if d.Name() == "db" {
					return filepath.SkipDir
				}
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			mb := float64(info.Size()) / (1 << 20)
			res.TotalMB += mb
			res.Files++
			// ไฟล์ค้างจากการโหลดที่ไม่เสร็จ (unconfirmed) ลบได้เสมอ
			stale := strings.Contains(strings.ToLower(p), string(filepath.Separator)+"unconfirmed"+string(filepath.Separator))
			if stale || lastUsed(info).Before(cut) {
				if del {
					if os.Remove(p) == nil {
						res.FreedMB += mb
						res.Deleted++
					}
				} else {
					res.OldMB += mb
					res.OldFiles++
				}
			}
			return nil
		})
	}
	for _, d := range []string{"logs", "crashes"} {
		filepath.WalkDir(filepath.Join(fivemDir, "FiveM.app", d), func(p string, e fs.DirEntry, err error) error {
			if err == nil && !e.IsDir() {
				if info, err := e.Info(); err == nil {
					res.LogsMB += float64(info.Size()) / (1 << 20)
				}
			}
			return nil
		})
	}
	return res
}

func (a *app) cacheScanAsync(fivemDir string, days, reqID int) bool {
	go func() {
		r := walkCache(fivemDir, days, false)
		r.ReqID, r.Running = reqID, fivemRunningAny()
		b, _ := json.Marshal(r)
		a.emit("onCache", string(b))
	}()
	return true
}

func (a *app) cacheCleanAsync(fivemDir string, days, reqID int) bool {
	go func() {
		var r cacheInfo
		if fivemRunningAny() {
			r = cacheInfo{Message: "ปิด FiveM ก่อนล้างแคช", Running: true}
		} else {
			r = walkCache(fivemDir, days, true)
			after := walkCache(fivemDir, days, false)
			after.FreedMB, after.Deleted = r.FreedMB, r.Deleted
			r = after
		}
		r.ReqID = reqID
		b, _ := json.Marshal(r)
		a.emit("onCache", string(b))
	}()
	return true
}
