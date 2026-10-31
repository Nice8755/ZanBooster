# ตั้งระบบอัปเดตครั้งแรก (ทำครั้งเดียว)

1. สมัคร GitHub ที่ https://github.com
2. สร้าง repo ใหม่ชื่อ `ZanBooster` ตั้งเป็น **Public** (ระบบอัปเดตอ่าน repo แบบ private ไม่ได้)
3. อัปโหลดไฟล์ `remote.json` เข้า repo (branch `main`)
4. เปิด `update.go` แก้บรรทัด `const updateRepo = ""` เป็น `const updateRepo = "ชื่อGitHubของคุณ/ZanBooster"`
5. build ใหม่ด้วย `build.bat` แล้วปล่อยเวอร์ชันนี้ตามขั้นตอนด้านล่าง

ตั้งแต่เวอร์ชันนี้เป็นต้นไป คนที่ใช้โปรแกรมจะเห็นปุ่ม "มีเวอร์ชันใหม่" บนแถบด้านบน ทุกครั้งที่คุณปล่อยเวอร์ชันใหม่

# ปล่อยเวอร์ชันใหม่ (ทำทุกครั้ง)

ตั้งแต่ v1.7.0 โปรแกรมเช็กอัปเดตจากไฟล์ `latest.json` ใน repo ก่อน (ไม่ต้องสร้าง Release ก็ได้)

1. แก้เลขใน `main.go` เช่น `const appVersion = "1.7.1"`
2. ดับเบิลคลิก `build.bat` จด SHA256
3. เอา `ZanBooster.exe` ไปทับไฟล์ `release/ZanBooster.exe` ใน repo
4. แก้ `latest.json`:
   ```json
   { "version": "1.7.1", "sha256": "ค่าจาก build.bat", "file": "release/ZanBooster.exe", "notes": "เปลี่ยนอะไรบ้าง" }
   ```
5. commit / push (หรืออัปโหลดผ่านหน้าเว็บ GitHub)

ภายในไม่กี่นาที ทุกคนที่เปิดโปรแกรมจะเห็นปุ่ม "มีเวอร์ชันใหม่"

GitHub Actions (`.github/workflows/release.yml`) จะสร้างหน้า Release ตามเลขเวอร์ชันให้เองอัตโนมัติ พร้อมแนบ exe และบรรทัด SHA256
ถ้า SHA256 ใน latest.json ไม่ตรงกับไฟล์ exe ระบบจะไม่สร้าง Release (ดูผลได้ที่แท็บ Actions)

ถ้าอยากให้มีหน้า Release ให้คนโหลดด้วย ก็สร้าง Release แบบเดิมได้ (แนบ exe ตัวเดียวกัน และใส่บรรทัด `SHA256:`)

# เปลี่ยนค่าโดยไม่ต้องปล่อยเวอร์ชันใหม่ (remote.json)

แก้ไฟล์ `remote.json` ใน repo บน GitHub ได้เลย โปรแกรมจะดึงตอนเปิด

- `fivemDefaultBuild` : build เริ่มต้นของ FiveM (ตอนนี้ 3258) ถ้า FiveM อัปเดตแล้วระบบเข้าเซิร์ฟเริ่มรีเกม ให้แก้เลขนี้
  ดูค่าจริงได้จาก `GetDefaultGTA5Build()` ในไฟล์ https://github.com/citizenfx/fivem/blob/master/code/client/shared/CrossBuildRuntime.h
- `notice` : ข้อความที่จะเด้งให้ทุกคนเห็นตอนเปิดโปรแกรม (ว่างไว้ = ไม่แสดง)
