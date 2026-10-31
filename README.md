# Zan Booster

โปรแกรมปรับเครื่องและเข้าเซิร์ฟ FiveM สำหรับ Windows 10/11

## ไฟล์ในโปรเจค

| ไฟล์ | หน้าที่ |
|---|---|
| `ui.html` | หน้าตาโปรแกรมทั้งหมด (HTML/CSS/JS) เปิดในเบราว์เซอร์ดูได้เลย จะใช้ข้อมูลจำลอง |
| `engine.ps1` | ส่วนที่ปรับเครื่องจริง (registry, power plan, กราฟิก, ตรวจเครื่อง, รายงาน) |
| `main.go` | ตัวโปรแกรม หน้าต่าง, CPU/แรม, วัดปิง, ล้างแคชแรม |
| `launcher.go` | ระบบเข้าเซิร์ฟ FiveM รอบเดียว |
| `update.go` | ระบบอัปเดต, หน้าเกี่ยวกับ **และค่าตั้งต้นที่ต้องแก้ (ดูด้านล่าง)** |
| `app.manifest`, `icon.ico`, `rsrc_windows_amd64.syso` | ไอคอนและการขอสิทธิ์ Admin (ฝังเข้า exe แล้ว) |
| `vendor/` | ไลบรารีที่ใช้ build ได้โดยไม่ต้องต่อเน็ต |
| `remote.json` | ไฟล์ที่วางไว้ใน GitHub repo ให้โปรแกรมดึงค่าใหม่ได้โดยไม่ต้องออกเวอร์ชัน |
| `build.bat` | ดับเบิลคลิกเพื่อสร้าง `ZanBooster.exe` |

## ค่าที่ต้องแก้ก่อนแจก (อยู่บนสุดของ `update.go`)

```go
const updateRepo = ""      // เช่น "zan123/ZanBooster"  ว่าง = ปิดระบบอัปเดต
const contactURL = ""      // เช่น ลิงก์ Discord  ว่าง = ไม่แสดงปุ่มติดต่อ
const authorName = "Zan"
```

เลขเวอร์ชันอยู่ใน `main.go`: `const appVersion = "1.6.0"`

## วิธี build

1. ติดตั้ง Go 1.24 ขึ้นไป จาก https://go.dev/dl/
2. ดับเบิลคลิก `build.bat`
3. ได้ไฟล์ `ZanBooster.exe` และค่า SHA256 แสดงในหน้าต่าง

แก้แค่ `ui.html` หรือ `engine.ps1` ก็ build ใหม่ได้เลย สองไฟล์นี้ฝังอยู่ใน exe

## ปล่อยเวอร์ชันใหม่

ดูขั้นตอนใน `RELEASE.md`

## ทดสอบก่อนแจก

ดูรายการใน `TESTING.md`

## ถ้า Windows Defender แจ้งว่าเป็นไวรัส

Zan Booster ไม่ได้ฝังโค้ดอันตราย ซอร์สโค้ดทั้งหมดอยู่ใน repo นี้ และไฟล์ทุกเวอร์ชันมีค่า SHA256 ให้ตรวจสอบในหน้า Release

ชื่ออย่าง `Trojan:Win32/Bearfoos.B!ml` ที่ลงท้ายด้วย `!ml` คือการเดาของระบบ machine learning ไม่ได้เจอโค้ดไวรัสจริง
โปรแกรมปรับเครื่องที่ต้องใช้สิทธิ์ admin แก้ registry และอัปเดตตัวเองมักโดนแบบนี้

- ใช้เวอร์ชันล่าสุดเสมอ (เวอร์ชันก่อน 1.7.8 มีโค้ดที่ Defender ไม่ชอบ ซึ่งเอาออกแล้ว)
- แจ้ง Microsoft ว่าตรวจผิดได้ที่ https://www.microsoft.com/en-us/wdsi/filesubmission
  เลือก Software developer → แนบ ZanBooster.exe → Incorrectly detected → ใส่ชื่อที่ Defender แจ้ง
