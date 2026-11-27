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
- [ ] ฟังก์ชันที่ต้องถอดจาก 4.11.15 (ชื่อไฟล์เดิม · จำนวน · ชื่อ):
- `reshade.go` (48): reshadeAsync, reshadeCheck, reshadeImport, reshadeInstall, reshadeRemove, reshadeStatus, reshadeToggle, reshadeZan, zanFlush, zanMemoGet, zanMemoSet, zanReset, zanSetLight, zanWatch, buildPreset, dlBytes, fxDest, fxInstallOne, fxPackages, iniVals, isReshadeDll, map.init.3, reshadeCalcID, reshadeEffects, reshadeFixAck, reshadeLatest, reshadeLogID, reshadePaths, reshadeSetPreset, rsSendKey, stripZanSharp, stripZanSharpBody, u8IniGet, u8IniSet, u8IniSetTop, zanApplyLight, zanBody, zanCapture, zanCleanup, zanExitCleanup, zanIdx, zanInstalled, zanSetup, zanShortcutData, zanShortcutsBound, zanUserBody, zanWriteFiles, zanWriteOne
- `license.go` (18): licAPI, licActivate, licBonus, licCall, licInfo, licMigrate, licPost, licRecheck, licStartTrial, refInfo, remoteRaw, licCompute, licDataDir, licGet, licLoad, licSave, machineGuid, trialSig
- `zanextra.go` (14): zanFxToggle, zanMyOp, zanSetEco, atof, stripComments, zanMyBodyPath, zanMyCapture, zanMyList, zanMyNamePath, zanMyRemove, zanMySlot, zanMyWrite, zanTransform, zanWatermark
- `framegen.go` (13): frameGenInfo, frameGenInfoAsync, frameGenSet, frameGenSetAsync, (*nvSession).appProfile, (*nvSession).delX, (*nvSession).getX, (*nvSession).setX, fivemGameExes, gpuNames, loadNVApp, nvWide, smoothMotionGpu
- `upscale.go` (12): dldsrInfo, upscaleAsync, upscaleFlush, upscaleInfo, upscaleSet, map.init.4, openNvcp, presetUndoCAS, readSampling, upRecommend, upSpec, writeSampling
- `deeplink.go` (10): bgWatchLoop, takeDeepLink, watchList, watchPending, watchRemove, cleanLink, handleLinkArg, registerLinkScheme, slotFree, toastNotify
- `hudmask.go` (10): zanDNSet, zanHudCapture, zanHudClear, blankMaskPNG, captureScreen, zanDNBody, zanDNGet, zanDNWrite, zanHudRemove, zanHudWrite
- `health.go` (9): biosInfo, fixHz, healthExtra, rebootBios, boardInfo, monitorsHz, primaryAdapter, rebarNvidia, setHz
- `zangrade.go` (7): map.init.5, zan3, zanGradeRemove, zanGradeWrite, zanStyleFx, zgFx, zgStyle
- `crosshair.go` (6): xhConfig, (*xhWin).build, (*xhWin).destroy, (*xhWin).freeBitmap, (*xhWin).show, (*xhWin).tick
- `sysbench.go` (5): sysBenchAsync, cpuScore, qpcUs, ramTest, timerTest
- `combat.go` (4): combatFix, combatItem, combatBad, gfxFile
- `crosshairdraw.go` (4): xhCfg.norm, xhColor, xhInside, xhRender
- `licensecore.go` (4): isShortKey, machineBytes, machineCode, parseLicKey
- `recover.go` (4): reRamAsync, reRamN, purgeStandbyGentle, trimOthersN
- `update.go` (3): betaFromServer, isBetaMachine, ghGetRaw
- `gamemode.go` (2): autoRamOn, totalRAMMB
- `shot.go` (2): saveShot, showShot
- `feedback.go` (1): cleanDiscordID
- `launcher.go` (1): autoCloseOn
- `zansharp.go` (1): writeZanSharp
- [ ] ไฟล์ที่มีอยู่แล้วจาก v2.6 ต้องเทียบกับ 4.11.15 ว่าเปลี่ยนอะไร (main, launcher, gamemode, gpudriver, findfivem, update, bench, backup, recover ...)
- [ ] ไฟล์เชดเดอร์/พรีเซ็ตที่ฝังใน exe เก่า (ZanGrade.fx, ZanQV, ZanNVE, ZanSharp ...) ดึงออกมาเป็นไฟล์
