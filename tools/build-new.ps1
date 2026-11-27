# สร้าง exe ตัวใหม่ (โค้ด Go ใน app/) พร้อมปล่อย: หน้าจอ + engine ล่าสุด, เลขเวอร์ชันใน exe, ลิงก์รายงานปัญหาจาก secret
#   powershell -File tools/build-new.ps1 -Version 4.20.01 -Out C:\path\ZanBooster.exe
# ต้องมี Go 1.24 · ลิงก์ webhook มาจาก $env:REPORT_WEBHOOK เท่านั้น (ห้ามใส่ในที่เก็บ)
param([Parameter(Mandatory)][string]$Version, [Parameter(Mandatory)][string]$Out)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$app = Join-Path $root 'app'
$utf8 = [Text.UTF8Encoding]::new($false)

# หน้าจอ: ตัดตัวจำลองของเบราว์เซอร์ออก (ใช้เฉพาะตอนเปิด ui.html ในเบราว์เซอร์) แบบเดียวกับตัวสร้างเดิม
$ui = [IO.File]::ReadAllText((Join-Path $root 'recovered\ui.html'), $utf8).Replace("`r`n", "`n")
$a = $ui.IndexOf("if (!NATIVE) {`n"); $b = $ui.IndexOf("`n}`n`n/* ================= data")
if ($a -lt 0 -or $b -lt $a) { throw 'mock block markers not found' }
$ui = $ui.Substring(0, $a) + "if (!NATIVE) {" + $ui.Substring($b + 1)
[IO.File]::WriteAllText((Join-Path $app 'ui.html'), $ui, $utf8)
Copy-Item (Join-Path $root 'recovered\engine.ps1') (Join-Path $app 'engine.ps1') -Force
"ui.html: $($utf8.GetByteCount($ui)) bytes (no size limit in the new exe)"

$env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
$hook = "$env:REPORT_WEBHOOK".Trim()
if (-not $hook) { 'REPORT_WEBHOOK secret is not set: the report button will say it cannot send' }
$ld = "-H windowsgui -s -w -X main.appVersion=$Version"
if ($hook) { $ld += " -X main.reportWebhook=$hook" }

# เลขเวอร์ชันในคุณสมบัติไฟล์ (Windows > Properties): สร้าง resource ใหม่จากตัวเดิม (ไอคอน + manifest แอดมินเหมือนเดิม)
$syso = Join-Path $app 'rsrc_windows_amd64.syso'
$keep = "$syso.orig"; Copy-Item $syso $keep -Force
$wr = Join-Path $env:TEMP "zb-winres-$PID"
try {
  go install github.com/tc-hib/go-winres@v0.3.3
  if ($LASTEXITCODE) { throw 'go-winres install failed' }
  $gw = Join-Path (go env GOPATH) 'bin\go-winres.exe'
  $tmpExe = Join-Path $wr 'plain.exe'; New-Item -ItemType Directory $wr -Force | Out-Null
  go build -mod=vendor -trimpath -ldflags "-H windowsgui" -o $tmpExe $app
  if ($LASTEXITCODE) { throw 'build failed' }
  Push-Location $wr
  & $gw extract --dir winres $tmpExe
  if ($LASTEXITCODE) { throw 'extract failed' }
  & $gw make --in winres\winres.json --out rsrc --arch amd64 --file-version "$Version.0" --product-version "$Version.0"
  if ($LASTEXITCODE) { throw 'make failed' }
  Pop-Location
  Copy-Item (Join-Path $wr 'rsrc_windows_amd64.syso') $syso -Force
  "version resource: $Version.0"
} catch {
  Pop-Location -ErrorAction SilentlyContinue
  throw "version resource: $_"
}

try {
  go build -mod=vendor -trimpath -ldflags $ld -o $Out $app
  if ($LASTEXITCODE) { throw 'build failed' }
} finally {
  Copy-Item $keep $syso -Force; Remove-Item $keep -Force
  Remove-Item $wr -Recurse -Force -ErrorAction SilentlyContinue
}
$v = (Get-Item $Out).VersionInfo.FileVersion
if ($v -ne "$Version.0") { throw "exe version is $v" }
"built $Out ($((Get-Item $Out).Length) bytes, version $v)"
