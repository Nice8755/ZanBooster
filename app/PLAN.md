# exe ใหม่ (app/) — แผนงาน

เป้าหมาย: สร้าง ZanBooster.exe ใหม่จากโค้ด Go ทั้งหมด แทนการยัดไฟล์เข้า exe 4.11.15 เก่า
(ไม่มีขีดจำกัดพื้นที่ แก้ได้ทุกส่วน) โดยข้อมูลลูกค้าอยู่ครบ

## ข้อมูลลูกค้าต้องอยู่ครบ (ตัวใหม่อ่าน/เขียนที่เดิมทุกอย่าง)
- `C:\ProgramData\ZanBooster` : settings.json, servers.json, license.json, config/backup ของ engine
- `%LOCALAPPDATA%\ZanBooster\WebView2` : localStorage ของหน้าจอ (zbSetBak ฯลฯ)
- รีจิสทรี `HKLM\SOFTWARE\ZanBooster`
- รหัสเครื่อง/ตรวจคีย์: `licensecore.go` ถอดค่าคงที่จาก exe 4.11.15 แล้ว ห้ามแก้ค่า

## วิธีถอดโค้ดที่หายไป
- exe 4.11.15 ยังมีชื่อฟังก์ชัน ชื่อไฟล์ และเลขบรรทัดเดิม (`go tool nm`, `go tool objdump`)
- `tools/re/annotate.py` เติมข้อความที่โค้ดอ้างถึงลงในผล objdump:
  `go tool objdump -s 'main\.\(\*app\)\.licAPI' beta/ZanBooster.exe | python3 tools/re/annotate.py beta/ZanBooster.exe`
- เขียนใหม่ทีละไฟล์ตามชื่อไฟล์เดิม เทียบพฤติกรรมกับหน้าจอ (recovered/ui.html ตัวจำลองบอกรูปแบบข้อมูลที่ต้องส่งกลับ)

## เครื่องมือ
- `tools/re/annotate.py` เติมข้อความ · `tools/re/compact.py` ย่อให้อ่านง่าย · `tools/re/gotype.py` อ่าน struct (ชื่อช่อง + json tag) จาก `runtime.rodata+N`

## สร้าง / ทดสอบ
- `.github/workflows/newexe.yml` : push สาขา `claude/newexe*` → test + build → ดาวน์โหลดที่ Actions > Artifacts
- ไม่เกี่ยวกับระบบปล่อยอัปเดต (publish.yml ยังสร้างจาก exe เก่าเหมือนเดิม จนกว่าตัวใหม่ผ่านการทดสอบ)

## ก่อนปล่อยจริง (ต้องผ่านทุกข้อ)
1. รหัสเครื่องจากตัวใหม่ = ตัวเก่า บนคอมเจ้าของ (+ เครื่องอื่นถ้ามี)
2. คีย์เดิมยังเป็น Premium · ทดลองฟรี / ใส่คีย์ใหม่ใช้ได้
3. ค่าที่ตั้งไว้ ธีม เซิร์ฟ ยังอยู่หลังเปลี่ยนตัว
4. อัปเดตอัตโนมัติจากตัวเก่า → ตัวใหม่ได้ และตัวใหม่อัปเดตต่อได้
5. ปล่อยตามขั้น owner → premium → everyone

## สถานะ
- [x] โครง: โค้ด v2.6 + licensecore + หน้าจอ/engine ล่าสุด สร้างผ่าน (ยังไม่มีฟังก์ชันด้านล่าง)
- [x] license.go: ระบบคีย์ ทดลองฟรี ตรวจซ้ำ 30 นาที ชวนเพื่อน (ถอดครบ 18 ฟังก์ชัน ข้อความ/ชื่อรีจิสทรี/โครงสร้าง JSON ตรงตัวเก่า) · isShortKey
- [x] update.go: ตัวเก่าเช็ก latest.json (main) แบบเดียวกับ v2.6 + ระบบ Beta เก่า (beta.json, /beta/check) ซึ่งเลิกใช้แล้ว → ใช้ของ v2.6 ได้ (ตัวเก่าอัปเดตมาตัวใหม่ได้ เพราะแค่โหลดไฟล์ตาม latest.json + ตรวจ SHA256) · หน้าจอเลือกช่อง owner/premium เอง (ringPick ใน ui.html)
- [x] health.go (รีเฟรชเรต ความร้อน ReBAR สายจอ ไบออส) · combat.go (ค่าเกมตี/ยิง) · shot.go (บันทึก/คัดลอกการ์ดผล)
- [x] sysbench.go (สูตรเดิม) · reRam (recover_ram.go) · deeplink.go (zanbooster:// + รอคิวเบื้องหลัง + แจ้งเตือน) · winState
- [x] เทียบไฟล์ v2.6 กับ 4.11.15: launcher (พรีเมียม, entLow/texLow, autoClose) · gamemode (autoRam, เตือน FPS ตก, bgWatchLoop) · recover (noTrim) · feedback (ค้นดิส /v1/dcfind) · update (เหลือแต่ Beta ที่เลิกแล้ว)
- [ ] ต้องใส่ secret REPORT_WEBHOOK (ห้องรายงานปัญหา) ตอนสร้าง: -X main.reportWebhook=... (ตัวเก่าฝังลิงก์ไว้ในไฟล์ ห้ามใส่ในที่เก็บ)
- [x] ถอดครบทุกฟังก์ชันของ 4.11.15 แล้ว (เทียบรายชื่อฟังก์ชันทั้งหมดใน exe เก่า เหลือแต่ Beta เก่าที่เลิกใช้: betaFromServer, isBetaMachine, ghGetRaw)
  - framegen.go (Smooth Motion NVIDIA) · upscale.go (Zan Upscale, NVIDIA App, DLDSR)
  - reshade.go + zangrade.go + zanextra.go + hudmask.go + zansharp.go (ReShade ทั้งชุด, พรีเซ็ต Zan p1-p12, พรีเซ็ตของฉัน, กลางวัน/กลางคืน, หน้ากาก HUD, ZanSharp)
  - crosshair.go (เป้าเล็งซ้อนจอ) · hotkeyLoop เปลี่ยนเป็นแบบ 4.11.15 = ไม่มีปุ่มลัด Ctrl+Alt+F แล้ว
  - ข้อมูลพรีเซ็ต p1-p12 + ตารางหมวด ดึงจาก exe ด้วย tools/re/rsdata.py → app/reshade_data.go
  - ZanGrade.fx / แม่แบบ ZanSharp ดึงจาก exe เก็บแบบบีบอัด+สลับไบต์ (app/assets/zg.bin, zs.bin) ไม่วางข้อความตรง ๆ
- [ ] ส่วนที่เขียนใหม่ตามพฤติกรรม ไม่ได้ตรงทีละคำสั่ง (ต้องลองในเกมจริง): ขั้นตัดสินจุด HUD ใน zanHudCapture, รูปทรงเป้าใน xhRender/xhInside, ลำดับบางขั้นใน u8IniSet
- [ ] ทดสอบบนเครื่องเจ้าของ: รหัสเครื่อง/คีย์/ค่าที่ตั้ง, ติดตั้ง ReShade, ใช้พรีเซ็ต z1-z5 (engine RsCustom), Smooth Motion, Upscale, เป้าเล็ง
- [ ] ไฟล์ที่มีอยู่แล้วจาก v2.6 ต้องเทียบกับ 4.11.15 ว่าเปลี่ยนอะไร (main, launcher, gamemode, gpudriver, findfivem, update, bench, backup, recover ...)
- [x] ไฟล์เชดเดอร์ที่ฝังใน exe เก่า ดึงออกมาแล้ว (ZanQV/ZanNVE/ZanGradeN สร้างจาก ZanGrade.fx แบบเดียวกับตัวเก่า)
