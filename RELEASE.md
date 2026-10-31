# ตั้งระบบอัปเดตครั้งแรก (ทำครั้งเดียว)

1. สมัคร GitHub ที่ https://github.com
2. สร้าง repo ใหม่ชื่อ `ZanBooster` ตั้งเป็น **Public** (ระบบอัปเดตอ่าน repo แบบ private ไม่ได้)
3. อัปโหลดไฟล์ `remote.json` เข้า repo (branch `main`)
4. เปิด `update.go` แก้บรรทัด `const updateRepo = ""` เป็น `const updateRepo = "ชื่อGitHubของคุณ/ZanBooster"`
5. build ใหม่ด้วย `build.bat` แล้วปล่อยเวอร์ชันนี้ตามขั้นตอนด้านล่าง

ตั้งแต่เวอร์ชันนี้เป็นต้นไป คนที่ใช้โปรแกรมจะเห็นปุ่ม "มีเวอร์ชันใหม่" บนแถบด้านบน ทุกครั้งที่คุณปล่อยเวอร์ชันใหม่

# ปล่อยเวอร์ชันใหม่ (ทำทุกครั้ง)

1. แก้เลขใน `main.go` เช่น `const appVersion = "1.6.1"` (ต้องมากกว่าเดิม)
2. ดับเบิลคลิก `build.bat` จด SHA256 ที่แสดง
3. ใน GitHub ไปที่ repo > **Releases** > **Draft a new release**
   - Tag: `v1.6.1` (ตรงกับเลขใน main.go)
   - Title: อะไรก็ได้ เช่น `Zan Booster 1.6.1`
   - คำอธิบาย: เขียนว่าเปลี่ยนอะไรบ้าง แล้ว**ต้องมีบรรทัดนี้** (ใช้ค่าจาก build.bat):
     ```
     SHA256: (ค่าที่ build.bat แสดง)
     ```
   - แนบไฟล์ `ZanBooster.exe` (ชื่อไฟล์ต้องเป็นแบบนี้เป๊ะ)
4. กด **Publish release**

โปรแกรมจะดาวน์โหลดไฟล์ ตรวจ SHA256 ถ้าไม่ตรงจะยกเลิกการอัปเดต แล้วสลับไฟล์และเปิดใหม่เอง

# เปลี่ยนค่าโดยไม่ต้องปล่อยเวอร์ชันใหม่ (remote.json)

แก้ไฟล์ `remote.json` ใน repo บน GitHub ได้เลย โปรแกรมจะดึงตอนเปิด

- `fivemDefaultBuild` : build เริ่มต้นของ FiveM (ตอนนี้ 3258) ถ้า FiveM อัปเดตแล้วระบบเข้าเซิร์ฟเริ่มรีเกม ให้แก้เลขนี้
  ดูค่าจริงได้จาก `GetDefaultGTA5Build()` ในไฟล์ https://github.com/citizenfx/fivem/blob/master/code/client/shared/CrossBuildRuntime.h
- `notice` : ข้อความที่จะเด้งให้ทุกคนเห็นตอนเปิดโปรแกรม (ว่างไว้ = ไม่แสดง)
