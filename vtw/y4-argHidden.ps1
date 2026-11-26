param(
    [string]$Action = 'Status',
    [string]$Ids = '',
    [string]$Opt = ''
)
# Zan Booster - engine (เรียกจากตัวโปรแกรม .exe)
# ส่งผลลัพธ์ออก stdout เป็น JSON ทีละบรรทัด: {"t":"log"|"step"|"result"|"error", ...}

[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$DataDir    = Join-Path $env:ProgramData 'ZanBooster'
$BackupFile = Join-Path $DataDir 'backup.json'
$LogFile    = Join-Path $DataDir 'log.txt'
$SchemeGuid = '5f7b1c3e-9a4d-4e2b-8c61-0f9e2d7a4b11'
$UltimateGuid = 'e9a42b02-d5df-448d-aa00-03f14749eb61'
$HighPerfGuid = '8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c'
$BalancedGuid = '381b4222-f694-41f0-9685-ff5bb260df2e'
$ConfigFile = Join-Path $env:ProgramData 'ZanBooster\config.json'
$HKMM       = 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Multimedia\SystemProfile'
$VIDEO_SUB = '7516b95f-f776-4464-8c53-06167f40cc99'; $VIDEO_IDLE = '3c0bc021-c8a8-4e07-a973-6b14cbcb2b7e'
$USB_SUB    = '2a737441-1930-4402-8d77-b2bebba308a3'
$USB_SET    = '48e6b7a6-50f5-4782-a5d4-53bb8f07e226'
$KHV = 'HKLM:\SYSTEM\CurrentControlSet\Control\DeviceGuard\Scenarios\HypervisorEnforcedCodeIntegrity'; $KGC = 'HKCU:\System\GameConfigStore'; $KGB = 'HKCU:\Software\Microsoft\GameBar'; $KUP = 'HKCU:\Software\Microsoft\DirectX\UserGpuPreferences'; $KMS = 'HKCU:\Control Panel\Mouse'; $KBA = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\BackgroundAccessApplications'; $KDV = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\GameDVR'
New-Item -ItemType Directory -Path $DataDir -Force | Out-Null

# ------------------------------------------------------------- output ----
$Script:Capture = $null
function Emit($obj) { if ($null -ne $Script:Capture) { $Script:Capture.Add($obj); return }; [Console]::Out.WriteLine(($obj | ConvertTo-Json -Compress -Depth 6)); [Console]::Out.Flush() }
function Write-Log([string]$m) {
    if ($Script:DryRun) { return }
    Emit @{ t = 'log'; m = $m }
    try { Add-Content -Path $LogFile -Value ("[{0}] {1}" -f (Get-Date -Format 'yyyy-MM-dd HH:mm:ss'), $m) -Encoding UTF8 } catch { }
}


# ------------------------------------------------------- find FiveM ----
function Get-Config {
    if (Test-Path $ConfigFile) { try { return Get-Content $ConfigFile -Raw -Encoding UTF8 | ConvertFrom-Json } catch { } }
    [pscustomobject]@{}
}
function Save-Config($cfg) { $cfg | ConvertTo-Json | Set-Content $ConfigFile -Encoding UTF8 }

# รับ path อะไรก็ได้ (โฟลเดอร์ติดตั้ง / FiveM.exe / FiveM.app / data) แล้วคืน path ของ FiveM.app ถ้าถูกต้อง
function Resolve-FiveMApp([string]$p) {
    if (-not $p) { return $null }
    $p = $p.Trim().Trim('"')
    if ($p -match '^(.*?\.exe)') { $p = $Matches[1] }
    try { $p = [Environment]::ExpandEnvironmentVariables($p) } catch { }
    if (Test-Path -LiteralPath $p -PathType Leaf) { $p = Split-Path $p -Parent }
    for ($i = 0; $i -lt 4 -and $p; $i++) {
        if ((Split-Path $p -Leaf) -ieq 'FiveM.app' -and (Test-Path -LiteralPath $p -PathType Container)) { return $p }
        $c = Join-Path $p 'FiveM.app'
        if (Test-Path -LiteralPath $c -PathType Container) { return $c }
        $p = Split-Path $p -Parent
    }
    $null
}

function Find-FiveM {
    $cands = [Collections.Generic.List[string]]::new()
    $cfg = Get-Config
    if ($cfg.FiveMPath) { $cands.Add([string]$cfg.FiveMPath) }
    # 1) เกมที่กำลังเปิดอยู่
    try { Get-Process -EA 0 | Where-Object { $_.Name -like 'FiveM*' } | ForEach-Object { if ($_.Path) { $cands.Add($_.Path) } } } catch { }
    # 2) LocalAppData ของผู้ใช้ปัจจุบัน
    $cands.Add((Join-Path $env:LOCALAPPDATA 'FiveM'))
    # 3) รายการโปรแกรมที่ติดตั้ง (ทุก user)
    $uroots = @('HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall', 'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall')
    try { Get-ChildItem 'Registry::HKEY_USERS' -EA 0 | ForEach-Object { $uroots += "Registry::$($_.Name)\Software\Microsoft\Windows\CurrentVersion\Uninstall" } } catch { }
    foreach ($r in $uroots) {
        if (-not (Test-Path $r)) { continue }
        Get-ChildItem $r -EA 0 | ForEach-Object {
            $pr = Get-ItemProperty $_.PSPath -EA 0
            if ($pr.DisplayName -match 'FiveM' -or $_.PSChildName -match 'FiveM') {
                foreach ($v in $pr.InstallLocation, $pr.DisplayIcon, $pr.UninstallString) { if ($v) { $cands.Add([string]$v) } }
            }
        }
    }
    # 4) Shortcut บน Desktop / Start Menu
    try {
        $sh = New-Object -ComObject WScript.Shell
        $lnkDirs = @([Environment]::GetFolderPath('Desktop'), [Environment]::GetFolderPath('CommonDesktopDirectory'),
                     [Environment]::GetFolderPath('StartMenu'), [Environment]::GetFolderPath('CommonStartMenu'))
        Get-ChildItem 'C:\Users' -Directory -EA 0 | ForEach-Object {
            $lnkDirs += (Join-Path $_.FullName 'Desktop'); $lnkDirs += (Join-Path $_.FullName 'AppData\Roaming\Microsoft\Windows\Start Menu')
        }
        foreach ($d in $lnkDirs) {
            if (-not $d -or -not (Test-Path $d)) { continue }
            Get-ChildItem $d -Filter '*FiveM*.lnk' -Recurse -Depth 3 -EA 0 | ForEach-Object {
                $t = $sh.CreateShortcut($_.FullName).TargetPath; if ($t) { $cands.Add($t) }
            }
        }
    } catch { }
    # 5) LocalAppData ของทุก user (กรณีเปิดโปรแกรมด้วยบัญชี admin อื่น)
    Get-ChildItem 'C:\Users' -Directory -EA 0 | ForEach-Object { $cands.Add((Join-Path $_.FullName 'AppData\Local\FiveM')) }
    foreach ($c in $cands) { $r = Resolve-FiveMApp $c; if ($r) { return $r } }
    # 6) สแกนไดรฟ์ (ลึก 2 ชั้น) เช่น D:\FiveM, D:\Games\FiveM
    foreach ($drv in [IO.DriveInfo]::GetDrives()) {
        if ($drv.DriveType -ne 'Fixed' -or -not $drv.IsReady) { continue }
        $root = $drv.RootDirectory.FullName
        $lvl1 = @(Get-ChildItem $root -Directory -Force -EA 0 | Where-Object { $_.Name -notmatch '^(Windows|\$Recycle\.Bin|System Volume Information|ProgramData|Recovery)$' })
        foreach ($d in $lvl1) {
            $r = Resolve-FiveMApp $d.FullName; if ($r) { return $r }
            foreach ($d2 in @(Get-ChildItem $d.FullName -Directory -EA 0)) {
                if (Test-Path -LiteralPath (Join-Path $d2.FullName 'FiveM.app')) { return (Join-Path $d2.FullName 'FiveM.app') }
            }
        }
    }
    $null
}

function Get-FiveMData {
    if ($Script:FiveMAppCache) { return (Join-Path $Script:FiveMAppCache 'data') }
    $app = Find-FiveM
    if ($app) {
        $Script:FiveMAppCache = $app
        $cfg = Get-Config
        if ($cfg.FiveMPath -ne $app) { $cfg | Add-Member -NotePropertyName FiveMPath -NotePropertyValue $app -Force; Save-Config $cfg }
        return (Join-Path $app 'data')
    }
    $null
}

# ------------------------------------------------------------- backup ----
function New-EmptyBackup { @{ Registry = @{}; CreatedKeys = @(); PrevScheme = $null; Powercfg = @{}; Adapters = @{}; Defender = @(); Applied = @() } }
$Script:AppliedKnown = $false
$Script:DryRun = $false
$Script:DryKeys = [Collections.Generic.List[string]]::new()
function Load-Backup {
    $b = New-EmptyBackup
    if (Test-Path $BackupFile) {
        try {
            $j = Get-Content $BackupFile -Raw -Encoding UTF8 | ConvertFrom-Json
            if ($j.Registry)    { foreach ($p in $j.Registry.PSObject.Properties) { $b.Registry[$p.Name] = $p.Value } }
            if ($j.CreatedKeys) { $b.CreatedKeys = @($j.CreatedKeys) }
            if ($j.PrevScheme)  { $b.PrevScheme = [string]$j.PrevScheme }
            if ($j.Powercfg)    { foreach ($p in $j.Powercfg.PSObject.Properties) { $b.Powercfg[$p.Name] = $p.Value } }
            if ($j.Adapters)    { foreach ($p in $j.Adapters.PSObject.Properties) { $b.Adapters[$p.Name] = $p.Value } }
            if ($j.Defender)    { $b.Defender = @($j.Defender) }
            if ($j.PSObject.Properties.Name -contains 'Applied') { $b.Applied = @($j.Applied | Where-Object { $_ }); $Script:AppliedKnown = $true }
        } catch { }
    }
    $b
}
function Save-Backup { $Script:Backup | ConvertTo-Json -Depth 6 | Set-Content -Path $BackupFile -Encoding UTF8 }
function Test-HasBackup {
    $b = $Script:Backup
    [bool](($b.Registry.Count + $b.CreatedKeys.Count + $b.Powercfg.Count + $b.Adapters.Count + $b.Defender.Count) -gt 0 -or $b.PrevScheme -or (Test-Path (Join-Path $DataDir 'gta5_settings.original.xml')) -or (Test-Path (Join-Path $DataDir 'nui_gpu.original.txt')))
}
$Script:Backup = Load-Backup

# ------------------------------------------------------------ helpers ----
function Get-Reg([string]$Path, [string]$Name) {
    try {
        if (-not (Test-Path -LiteralPath $Path)) { return $null }
        (Get-Item -LiteralPath $Path).GetValue($Name, $null, 'DoNotExpandEnvironmentNames')
    } catch { $null }
}

function Set-RegValue([string]$Path, [string]$Name, $Value, [string]$Type = 'DWord') {
    if ($Script:DryRun) { $Script:DryKeys.Add("$Path|$Name"); return }
    $id = "$Path|$Name"
    if (-not $Script:Backup.Registry.ContainsKey($id)) {
        $existed = $false; $old = $null; $oldType = $null
        if (Test-Path -LiteralPath $Path) {
            $key = Get-Item -LiteralPath $Path
            if ($key.GetValueNames() -contains $Name) {
                $existed = $true
                $old     = $key.GetValue($Name, $null, 'DoNotExpandEnvironmentNames')
                $oldType = $key.GetValueKind($Name).ToString()
            }
        }
        $Script:Backup.Registry[$id] = [ordered]@{ Path = $Path; Name = $Name; Existed = $existed; Value = $old; Type = $oldType }
    }
    if (-not (Test-Path -LiteralPath $Path)) {
        $top = $Path; $parent = Split-Path $top -Parent
        while ($parent -and -not (Test-Path -LiteralPath $parent)) { $top = $parent; $parent = Split-Path $top -Parent }
        if ($Script:Backup.CreatedKeys -notcontains $top) { $Script:Backup.CreatedKeys += $top }
        New-Item -Path $Path -Force | Out-Null
    }
    New-ItemProperty -LiteralPath $Path -Name $Name -Value $Value -PropertyType $Type -Force | Out-Null
}

function Remove-RegValue([string]$Path, [string]$Name) {
    # ลบค่า (สำรองค่าเดิมไว้ก่อน เพื่อคืนค่าได้)
    if ($Script:DryRun) { $Script:DryKeys.Add("$Path|$Name"); return }
    if (-not (Test-Path -LiteralPath $Path)) { return }
    $key = Get-Item -LiteralPath $Path
    if ($key.GetValueNames() -notcontains $Name) { return }
    $id = "$Path|$Name"
    if (-not $Script:Backup.Registry.ContainsKey($id)) {
        $Script:Backup.Registry[$id] = [ordered]@{ Path = $Path; Name = $Name; Existed = $true
            Value = $key.GetValue($Name, $null, 'DoNotExpandEnvironmentNames'); Type = $key.GetValueKind($Name).ToString() }
    }
    Remove-ItemProperty -LiteralPath $Path -Name $Name
}

$DXPrefs  = $KUP
$GCS      = $KGC
$GameExeRx = '(?i)\\(GTA5|GTA5_Enhanced|PlayGTAV|FiveM[^\\]*)\.exe$'
function Get-OsBuild { try { [int](Get-CimInstance Win32_OperatingSystem).BuildNumber } catch { 0 } }

function Get-ActiveScheme {
    $o = (powercfg /getactivescheme) -join ' '
    if ($o -match '([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})') { return $Matches[1].ToLower() }
    $null
}
function Get-PowercfgIndex([string]$Sub, [string]$Setting) {
    $o = (powercfg /query SCHEME_CURRENT $Sub $Setting) -join "`n"
    $hex = @([regex]::Matches($o, '0x[0-9a-fA-F]{8}') | ForEach-Object { $_.Value })
    if ($hex.Count -ge 2) { return @([Convert]::ToInt32($hex[-2], 16), [Convert]::ToInt32($hex[-1], 16)) }
    $null
}
function Set-PowercfgIndex([string]$Sub, [string]$Setting, [int]$Value) {
    if ($Script:DryRun) { $Script:DryKeys.Add("PC|$Sub|$Setting|$Value"); return }
    $scheme = Get-ActiveScheme
    $id = "$scheme|$Sub|$Setting"
    if ($scheme -ne $SchemeGuid -and -not $Script:Backup.Powercfg.ContainsKey($id)) {
        $cur = Get-PowercfgIndex $Sub $Setting
        if ($cur) { $Script:Backup.Powercfg[$id] = @($cur[0], $cur[1]) }
    }
    powercfg /setacvalueindex SCHEME_CURRENT $Sub $Setting $Value | Out-Null
    powercfg /setdcvalueindex SCHEME_CURRENT $Sub $Setting $Value | Out-Null
    powercfg /setactive SCHEME_CURRENT | Out-Null
}
function Get-UpAdapters { @(Get-NetAdapter -Physical -EA 0 | Where-Object { $_.Status -eq 'Up' }) }
function Get-FolderSize([string]$p) {
    if (-not (Test-Path $p)) { return 0 }
    $s = (Get-ChildItem $p -Recurse -Force -File -EA 0 | Measure-Object Length -Sum).Sum
    if ($s) { [int64]$s } else { 0 }
}

# ------------------------------------------------------------- tweaks ----
# Apply = ปรับ, Check = ตรวจว่าตอนนี้ปรับอยู่หรือไม่
$Tweaks = [ordered]@{
    PowerPlan = @{
        Apply = {
            $prev = Get-ActiveScheme
            if (-not $Script:Backup.PrevScheme -and $prev -ne $SchemeGuid) { $Script:Backup.PrevScheme = $prev }
            if (((powercfg /list) -join ' ') -match $SchemeGuid) {
                if ((Get-ActiveScheme) -eq $SchemeGuid) { powercfg /setactive $BalancedGuid | Out-Null }
                powercfg /delete $SchemeGuid | Out-Null
            }
            $pow = Join-Path $DataDir 'ZAN_Project_Optimizer.pow'
            if (Test-Path $pow) { powercfg /import $pow $SchemeGuid | Out-Null }
            if (((powercfg /list) -join ' ') -notmatch $SchemeGuid) {
                # สำรอง: นำเข้าไม่ได้ ใช้ Ultimate Performance แทน
                Write-Log 'นำเข้า ZAN_Project_Optimizer ไม่ได้ ใช้ Ultimate Performance แทน'
                powercfg /duplicatescheme $UltimateGuid $SchemeGuid | Out-Null
                if ($LASTEXITCODE -ne 0) { powercfg /duplicatescheme $HighPerfGuid $SchemeGuid | Out-Null }
                if ($LASTEXITCODE -ne 0) { throw 'เครื่องนี้สร้าง Power Plan เพิ่มไม่ได้ (พบบ่อยในโน้ตบุ๊กบางรุ่น)' }
                powercfg /changename $SchemeGuid 'Zan Booster' 'Created by Zan Booster' | Out-Null
            }
            # ปิดหน้าจอ (พักจอ) = Never ทั้งตอนเสียบปลั๊กและใช้แบต
            powercfg /setacvalueindex $SchemeGuid $VIDEO_SUB $VIDEO_IDLE 0 | Out-Null
            powercfg /setdcvalueindex $SchemeGuid $VIDEO_SUB $VIDEO_IDLE 0 | Out-Null
            if (@($Script:Backup.Applied) -contains 'UsbSuspend') { powercfg /setacvalueindex $SchemeGuid $USB_SUB $USB_SET 0 | Out-Null; powercfg /setdcvalueindex $SchemeGuid $USB_SUB $USB_SET 0 | Out-Null }
            # เสียบปลั๊ก: ไม่พักแกน CPU (core parking) และเร่งความถี่ CPU แบบไว (แก้กระตุกตอนเกมเรียกแกนที่หลับอยู่) · อยู่ใน Power Plan ของ Zan เท่านั้น ยกเลิกแล้วหายไปพร้อมแผน
            powercfg /setacvalueindex $SchemeGuid 54533251-82be-4824-96c1-47b60b740d00 0cc5b647-c1df-4637-891a-dec35c318583 100 2>$null | Out-Null
            powercfg /setacvalueindex $SchemeGuid 54533251-82be-4824-96c1-47b60b740d00 be337238-0d82-4146-a960-4f3749d470c7 2 2>$null | Out-Null
            powercfg /setactive $SchemeGuid | Out-Null
            if ((Get-ActiveScheme) -ne $SchemeGuid) { throw 'เปิดใช้ Power Plan ไม่ได้' }
        }
        Check = {
            if ((Get-ActiveScheme) -ne $SchemeGuid) { return $false }
            $v = Get-PowercfgIndex $VIDEO_SUB $VIDEO_IDLE
            if ($v -and $v[0] -ne 0) { return $false }
            # ต้องเป็น ZAN_Project_Optimizer (ถ้า import ไม่ได้ ยอมรับตัวสำรองชื่อ Zan Booster)
            $line = (powercfg /list) | Where-Object { $_ -match $SchemeGuid } | Select-Object -First 1
            $hasPow = Test-Path (Join-Path $DataDir 'ZAN_Project_Optimizer.pow')
            ($line -match 'ZAN_Project_Optimizer') -or (-not $hasPow)
        }
    }
    GameMode = @{
        Apply = {
            Set-RegValue $KGB 'AutoGameModeEnabled' 1
            Set-RegValue $KGB 'AllowAutoGameMode' 1
        }
        Check = { (Get-Reg $KGB 'AutoGameModeEnabled') -eq 1 }
    }
    GameDVR = @{
        Apply = {
            Set-RegValue $KGC 'GameDVR_Enabled' 0
            Set-RegValue $KDV 'AppCaptureEnabled' 0
            Set-RegValue 'HKLM:\SOFTWARE\Policies\Microsoft\Windows\GameDVR' 'AllowGameDVR' 0
        }
        Check = { (Get-Reg $KGC 'GameDVR_Enabled') -eq 0 }
    }
    MMCSS = @{
        Apply = {
            Set-RegValue $HKMM 'SystemResponsiveness' 10
            $g = "$HKMM\Tasks\Games"
            Set-RegValue $g 'GPU Priority' 8
            Set-RegValue $g 'Priority' 6
            Set-RegValue $g 'Scheduling Category' 'High' 'String'
            Set-RegValue $g 'SFIO Priority' 'High' 'String'
        }
        Check = { ((Get-Reg $HKMM 'SystemResponsiveness') -eq 10) -and ((Get-Reg "$HKMM\Tasks\Games" 'Priority') -eq 6) }
    }
    PowerThrottle = @{
        Apply = { Set-RegValue 'HKLM:\SYSTEM\CurrentControlSet\Control\Power\PowerThrottling' 'PowerThrottlingOff' 1 }
        Check = { (Get-Reg 'HKLM:\SYSTEM\CurrentControlSet\Control\Power\PowerThrottling' 'PowerThrottlingOff') -eq 1 }
    }
    HAGS = @{
        Apply = { Set-RegValue 'HKLM:\SYSTEM\CurrentControlSet\Control\GraphicsDrivers' 'HwSchMode' 2 }
        Check = { (Get-Reg 'HKLM:\SYSTEM\CurrentControlSet\Control\GraphicsDrivers' 'HwSchMode') -eq 2 }
    }
    VisualFX = @{
        Apply = {
            Set-RegValue 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\VisualEffects' 'VisualFXSetting' 2
            Set-RegValue 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize' 'EnableTransparency' 0
            Set-RegValue 'HKCU:\Control Panel\Desktop\WindowMetrics' 'MinAnimate' '0' 'String'
        }
        Check = { (Get-Reg 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\VisualEffects' 'VisualFXSetting') -eq 2 }
    }
    BgApps = @{
        Apply = { Set-RegValue $KBA 'GlobalUserDisabled' 1 }
        Check = { (Get-Reg $KBA 'GlobalUserDisabled') -eq 1 }
    }
    PrioSep = @{
        Apply = { Set-RegValue 'HKLM:\SYSTEM\CurrentControlSet\Control\PriorityControl' 'Win32PrioritySeparation' 38 }
        Check = { (Get-Reg 'HKLM:\SYSTEM\CurrentControlSet\Control\PriorityControl' 'Win32PrioritySeparation') -eq 38 }
    }
    MouseAccel = @{
        Apply = {
            Set-RegValue $KMS 'MouseSpeed' '0' 'String'
            Set-RegValue $KMS 'MouseThreshold1' '0' 'String'
            Set-RegValue $KMS 'MouseThreshold2' '0' 'String'
        }
        Check = { [string](Get-Reg $KMS 'MouseSpeed') -eq '0' }
    }
    UsbSuspend = @{
        Apply = { Set-PowercfgIndex $USB_SUB $USB_SET 0 }
        Check = { $v = Get-PowercfgIndex $USB_SUB $USB_SET; $v -and $v[0] -eq 0 }
    }
    FSO = @{
        Apply = {
            Set-RegValue $KGC 'GameDVR_FSEBehaviorMode' 2
            Set-RegValue $KGC 'GameDVR_FSEBehavior' 2
            Set-RegValue $KGC 'GameDVR_HonorUserFSEBehaviorMode' 1
            Set-RegValue $KGC 'GameDVR_DXGIHonorFSEWindowsCompatible' 1
        }
        Check = { (Get-Reg $GCS 'GameDVR_HonorUserFSEBehaviorMode') -eq 1 }
    }
    FastAltTab = @{
        Apply = {
            # 1) เปิด Fullscreen Optimizations ทั้งระบบ (ให้ Windows ใช้ Flip Model แทน Exclusive mode switch)
            Set-RegValue $GCS 'GameDVR_FSEBehavior' 0
            Set-RegValue $GCS 'GameDVR_HonorUserFSEBehaviorMode' 0
            Set-RegValue $GCS 'GameDVR_DXGIHonorFSEWindowsCompatible' 0
            # 2) ลบ "Disable fullscreen optimizations" ที่เคยติ๊กไว้ใน Properties ของ GTA5 / FiveM
            $n = 0
            foreach ($root in 'HKCU:', 'HKLM:') {
                $lp = "$root\Software\Microsoft\Windows NT\CurrentVersion\AppCompatFlags\Layers"
                if (-not (Test-Path $lp)) { continue }
                $k = Get-Item $lp
                foreach ($name in $k.GetValueNames()) {
                    if ($name -notmatch $GameExeRx) { continue }
                    $val = [string]$k.GetValue($name)
                    if ($val -notmatch '(?i)DISABLEDXMAXIMIZEDWINDOWEDMODE') { continue }
                    $rest = (($val -split '\s+') | Where-Object { $_ -and $_ -ne '~' -and $_ -notmatch '(?i)^DISABLEDXMAXIMIZEDWINDOWEDMODE$' }) -join ' '
                    if ($rest) { Set-RegValue $lp $name ("~ " + $rest) 'String' } else { Remove-RegValue $lp $name }
                    $n++
                }
            }
            if ($n) { Write-Log "ปลดล็อก Fullscreen Optimizations ของเกม $n ไฟล์" }
            # 3) Optimizations for windowed games (Windows 11): อัปเกรด DX11 ให้ใช้ Flip Model -> latency เท่า Exclusive
            $cur = [string](Get-Reg $DXPrefs 'DirectXUserGlobalSettings')
            $parts = @($cur -split ';' | Where-Object { $_ -and $_ -notmatch '^SwapEffectUpgradeEnable=' })
            $parts += 'SwapEffectUpgradeEnable=1'
            Set-RegValue $DXPrefs 'DirectXUserGlobalSettings' (($parts -join ';') + ';') 'String'
            if ((Get-OsBuild) -lt 22000) { Write-Log 'Windows 10 ใช้ได้เกือบทั้งหมด ยกเว้น Windowed Optimizations ที่มีเฉพาะ Windows 11' }
        }
        Check = {
            ((Get-Reg $GCS 'GameDVR_HonorUserFSEBehaviorMode') -ne 1) -and
            ((Get-Reg $GCS 'GameDVR_DXGIHonorFSEWindowsCompatible') -ne 1) -and
            ([string](Get-Reg $DXPrefs 'DirectXUserGlobalSettings') -match 'SwapEffectUpgradeEnable=1')
        }
    }
    NetThrottle = @{
        Apply = { Set-RegValue $HKMM 'NetworkThrottlingIndex' -1 }   # -1 = 0xFFFFFFFF
        Check = { (Get-Reg $HKMM 'NetworkThrottlingIndex') -eq -1 }
    }
    Nagle = @{
        Apply = {
            $ads = Get-UpAdapters
            if ($ads.Count -eq 0) { throw 'ไม่พบการ์ดเครือข่ายที่เชื่อมต่ออยู่' }
            foreach ($a in $ads) {
                $p = "HKLM:\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\$($a.InterfaceGuid)"
                if (Test-Path $p) {
                    Set-RegValue $p 'TcpAckFrequency' 1
                    Set-RegValue $p 'TCPNoDelay' 1
                    Set-RegValue $p 'TcpDelAckTicks' 0
                }
            }
        }
        Check = {
            $l = @(Get-UpAdapters); if (-not $l.Count) { $l = @(Get-NetAdapter -Physical -EA 0) }
            @($l | Where-Object { (Get-Reg "HKLM:\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\$($_.InterfaceGuid)" 'TCPNoDelay') -eq 1 }).Count -gt 0
        }
    }
    Adapter = @{
        Apply = {
            $changed = @()
            foreach ($a in Get-UpAdapters) {
                foreach ($kw in '*InterruptModeration', '*EEE', 'EEE', 'AdvancedEEE', 'EnableGreenEthernet', 'GigaLite', 'PowerSavingMode', '*FlowControl') {
                    $prop = Get-NetAdapterAdvancedProperty -Name $a.Name -RegistryKeyword $kw -EA 0
                    if (-not $prop) { continue }
                    $id = "$($a.Name)|$kw"
                    if (-not $Script:Backup.Adapters.ContainsKey($id)) { $Script:Backup.Adapters[$id] = [string]($prop.RegistryValue | Select-Object -First 1) }
                    Set-NetAdapterAdvancedProperty -Name $a.Name -RegistryKeyword $kw -RegistryValue 0 -NoRestart -ErrorAction Stop
                    if ($changed -notcontains $a.Name) { $changed += $a.Name }
                }
            }
            Save-Backup
            if ($changed.Count -eq 0) { throw 'การ์ดแลนเครื่องนี้ไม่มีตัวเลือกนี้' }
            Write-Log 'รีสตาร์ทการ์ดแลน (เน็ตหลุดไม่กี่วินาที)...'
            foreach ($n in $changed) { Restart-NetAdapter -Name $n -Confirm:$false }
            for ($i = 0; $i -lt 40 -and @(Get-NetAdapter -Name $changed -EA 0 | Where-Object { $_.Status -eq 'Up' }).Count -lt $changed.Count; $i++) { Start-Sleep -Milliseconds 500 }
        }
        Check = { $Script:Backup.Adapters.Count -gt 0 }
    }
    HVCI = @{
        # Memory Integrity (Core Isolation) บางเครื่องกิน FPS 5-15% ต้องรีสตาร์ทถึงมีผล
        Apply = { Set-RegValue $KHV 'Enabled' 0 }
        Check = { (Get-Reg $KHV 'Enabled') -ne 1 }
    }
    TimerRes = @{
        Apply = { Set-RegValue 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager\kernel' 'GlobalTimerResolutionRequests' 1 }
        Check = { [Environment]::OSVersion.Version.Build -lt 22000 -or (Get-Reg 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager\kernel' 'GlobalTimerResolutionRequests') -eq 1 }
    }
    NoP2PUpdate = @{
        Apply = { Set-RegValue 'HKLM:\SOFTWARE\Policies\Microsoft\Windows\DeliveryOptimization' 'DODownloadMode' 0 }
        Check = { (Get-Reg 'HKLM:\SOFTWARE\Policies\Microsoft\Windows\DeliveryOptimization' 'DODownloadMode') -eq 0 }
    }
    DefenderFiveM = @{
        # ทำแค่ "เพิ่มข้อยกเว้น" ไม่เปิด Defender ถ้า Defender ปิดอยู่แล้วจะไม่ทำอะไรเลย
        Apply = {
            if (-not (Test-DefenderOn)) { Write-Log 'Defender ปิดอยู่แล้ว ไม่ต้องตั้ง (ไม่ได้เปิด Defender กลับมา)'; return }
            $fd = Get-FiveMData
            if (-not $fd) { throw 'ยังไม่เจอโฟลเดอร์ FiveM' }
            $p = Split-Path $fd -Parent
            if ($Script:DryRun) { $Script:DryKeys.Add("MP|$p"); return }
            if (@((Get-MpPreference).ExclusionPath) -contains $p) { return }
            Add-MpPreference -ExclusionPath $p -ErrorAction Stop
            if ($Script:Backup.Defender -notcontains $p) { $Script:Backup.Defender = @($Script:Backup.Defender) + $p }
            Write-Log "Defender ข้ามโฟลเดอร์ $p"
        }
        Check = {
            if (-not (Test-DefenderOn)) { return $true } # Defender ปิดอยู่ = ไม่มีการสแกนอยู่แล้ว
            $fd = Get-FiveMData
            [bool]($fd -and (@((Get-MpPreference -EA 0).ExclusionPath) -contains (Split-Path $fd -Parent)))
        }
    }
    GpuMsi = @{
        Apply = {
            $n = 0
            foreach ($d in @(Get-PnpDevice -Class Display -PresentOnly -EA 0 | Where-Object { $_.InstanceId -like 'PCI\*' })) {
                $k = "HKLM:\SYSTEM\CurrentControlSet\Enum\$($d.InstanceId)\Device Parameters\Interrupt Management\MessageSignaledInterruptProperties"
                try { Set-RegValue $k 'MSISupported' 1; $n++ } catch { Write-Log "ตั้ง MSI ให้ $($d.FriendlyName) ไม่ได้" }
            }
            if ($n -eq 0) { throw 'ไม่พบการ์ดจอที่ตั้งได้' }
            Write-Log "ตั้ง MSI mode ให้การ์ดจอ $n ตัว (มีผลหลังรีสตาร์ท)"
        }
        Check = {
            $ds = @(Get-PnpDevice -Class Display -PresentOnly -EA 0 | Where-Object { $_.InstanceId -like 'PCI\*' })
            $ds.Count -gt 0 -and @($ds | Where-Object { (Get-Reg "HKLM:\SYSTEM\CurrentControlSet\Enum\$($_.InstanceId)\Device Parameters\Interrupt Management\MessageSignaledInterruptProperties" 'MSISupported') -ne 1 }).Count -eq 0
        }
    }
    GpuPref = @{
        Apply = {
            $key = $KUP
            $n = 0
            foreach ($exe in Get-FiveMExes) { Set-RegValue $key $exe 'GpuPreference=2;' 'String'; $n++ }
            Write-Log "ตั้งการ์ดจอแยกให้ $n ไฟล์"
        }
        Check = {
            $key = $KUP
            $exes = @(Get-FiveMExes)
            $exes.Count -gt 0 -and @($exes | Where-Object { (Get-Reg $key $_) -ne 'GpuPreference=2;' }).Count -eq 0
        }
    }
    FiveMPrio = @{
        # ให้ Windows เปิดตัวเกม FiveM ที่ priority High ตั้งแต่เริ่ม: ตั้งตามชื่อไฟล์ทุก build ที่ใช้กันไว้ก่อน (ล้างแคชหรือยังไม่เคยเข้า build นั้นก็ได้ผล) + ไฟล์ที่เจอในเครื่อง
        Apply = { foreach ($n in Get-FiveMPrioNames) { Set-RegValue "$KIF\$n\PerfOptions" 'CpuPriorityClass' 3 } }
        Check = { -not @(Get-FiveMPrioNames | Where-Object { (Get-Reg "$KIF\$_\PerfOptions" 'CpuPriorityClass') -ne 3 }).Count }
    }
    SearchIdx = @{
        Apply = {
            Set-RegValue 'HKLM:\SYSTEM\CurrentControlSet\Services\WSearch' 'Start' 3
            if (-not $Script:DryRun) { Stop-Service WSearch -Force -EA 0 }
        }
        Check = { $s = Get-Reg 'HKLM:\SYSTEM\CurrentControlSet\Services\WSearch' 'Start'; $null -eq $s -or $s -ge 3 }
    }
    Services = @{
        Apply = {
            # Start: 3 = Manual (เปิดเองเมื่อจำเป็น), 4 = Disabled
            $list = [ordered]@{ DiagTrack = 4; dmwappushservice = 4; SysMain = 4; MapsBroker = 4; RetailDemo = 4; WerSvc = 3;
                                XblAuthManager = 3; XblGameSave = 3; XboxNetApiSvc = 3 }
            $n = 0
            foreach ($name in $list.Keys) {
                $key = "HKLM:\SYSTEM\CurrentControlSet\Services\$name"
                if (-not (Test-Path $key)) { continue }
                Set-RegValue $key 'Start' $list[$name]
                if ($list[$name] -eq 4 -and -not $Script:DryRun) { try { Stop-Service -Name $name -Force -EA 0 } catch { } }
                $n++
            }
            Write-Log "ปรับ service $n ตัว"
        }
        Check = { (Get-Reg 'HKLM:\SYSTEM\CurrentControlSet\Services\DiagTrack' 'Start') -eq 4 -and (Get-Reg 'HKLM:\SYSTEM\CurrentControlSet\Services\SysMain' 'Start') -eq 4 }
    }
    # ปิด Multi-Plane Overlay: ต้นเหตุภาพกระตุก/กะพริบในเกมแบบหน้าต่าง (มีผลหลังรีสตาร์ท)
    MPO = @{
        Apply = { Set-RegValue 'HKLM:\SOFTWARE\Microsoft\Windows\Dwm' 'OverlayTestMode' 5 }
        Check = { (Get-Reg 'HKLM:\SOFTWARE\Microsoft\Windows\Dwm' 'OverlayTestMode') -eq 5 }
    }
    # ปิด Xbox Game Bar ทั้งหมด (ไม่ใช่แค่อัดคลิป) ไม่ให้แฝงตัวในเกม
    GameBarOff = @{
        Apply = {
            Set-RegValue $KGB 'UseNexusForGameBarEnabled' 0
            Set-RegValue $KGB 'ShowStartupPanel' 0
            Set-RegValue $KDV 'AppCaptureEnabled' 0
            Set-RegValue $KGC 'GameDVR_Enabled' 0
            Set-RegValue 'HKLM:\SOFTWARE\Microsoft\PolicyManager\default\ApplicationManagement\AllowGameDVR' 'value' 0
        }
        Check = { (Get-Reg $KGB 'UseNexusForGameBarEnabled') -eq 0 }
    }
}

# ------------------------------------------------------------ actions ----
function Test-DefenderOn {
    if (-not (Get-Command Get-MpComputerStatus -EA 0)) { return $false }
    $svc = Get-Service WinDefend -EA 0
    if (-not $svc -or $svc.Status -ne 'Running') { return $false }
    try { return [bool](Get-MpComputerStatus -ErrorAction Stop).RealTimeProtectionEnabled } catch { return $false }
}
function Get-TweakKeys([string]$id) {
    $Script:DryKeys.Clear(); $Script:DryRun = $true
    try { & $Tweaks[$id].Apply } catch { } finally { $Script:DryRun = $false }
    @($Script:DryKeys)
}
function Test-TweakOwned([string]$id) {
    if ($id -eq 'PowerPlan') { return [bool]$Script:Backup.PrevScheme }
    if ($id -eq 'Adapter') { return $Script:Backup.Adapters.Count -gt 0 }
    if ($id -eq 'DefenderFiveM') { return @($Script:Backup.Defender).Count -gt 0 }
    foreach ($k in Get-TweakKeys $id) { if ($Script:Backup.Registry.ContainsKey($k)) { return $true } }
    $false
}
function Restore-RegEntry($e) {
    try {
        if ($e.Existed) {
            if (-not (Test-Path -LiteralPath $e.Path)) { New-Item -Path $e.Path -Force | Out-Null }
            $val = $e.Value
            if ($e.Type -eq 'Binary')      { $val = [byte[]]@($e.Value) }
            if ($e.Type -eq 'MultiString') { $val = [string[]]@($e.Value) }
            New-ItemProperty -LiteralPath $e.Path -Name $e.Name -Value $val -PropertyType $e.Type -Force | Out-Null
        } else {
            Remove-ItemProperty -LiteralPath $e.Path -Name $e.Name -EA 0
        }
    } catch { Write-Log "คืนค่า $($e.Path)\$($e.Name) ไม่ได้: $($_.Exception.Message)" }
}
function Revert-Tweak([string]$id) {
    $b = $Script:Backup
    if ($id -eq 'PowerPlan') {
        $prev = $b.PrevScheme; if (-not $prev) { $prev = $BalancedGuid }
        powercfg /setactive $prev | Out-Null
        if ($LASTEXITCODE -ne 0) { powercfg /setactive $BalancedGuid | Out-Null }
        if (((powercfg /list) -join ' ') -match $SchemeGuid) { powercfg /delete $SchemeGuid | Out-Null }
        $b.PrevScheme = $null
        return
    }
    if ($id -eq 'DefenderFiveM') {
        if (@($b.Defender).Count -eq 0) { throw 'ไม่มีค่าเดิมเก็บไว้ (โฟลเดอร์นี้ถูกยกเว้นไว้ก่อนแล้ว)' }
        foreach ($p in @($b.Defender)) { Remove-MpPreference -ExclusionPath $p -EA 0 }
        $b.Defender = @()
        return
    }
    if ($id -eq 'Adapter') {
        if ($b.Adapters.Count -eq 0) { throw 'ไม่มีค่าเดิมเก็บไว้' }
        $restart = @()
        foreach ($kv in @($b.Adapters.GetEnumerator())) {
            $parts = $kv.Key -split '\|'
            try { Set-NetAdapterAdvancedProperty -Name $parts[0] -RegistryKeyword $parts[1] -RegistryValue $kv.Value -NoRestart -ErrorAction Stop; if ($restart -notcontains $parts[0]) { $restart += $parts[0] } } catch { }
        }
        $b.Adapters = @{}
        foreach ($n in $restart) { Restart-NetAdapter -Name $n -Confirm:$false -EA 0 }
        return
    }
    $n = 0
    $keep = @{}
    foreach ($o in @($b.Applied)) { if ($o -ne $id -and $Tweaks.Contains($o) -and $o -notin 'PowerPlan', 'Adapter', 'DefenderFiveM') { foreach ($k2 in Get-TweakKeys $o) { $keep[$k2] = 1 } } }
    foreach ($k in Get-TweakKeys $id) {
        if ($k.StartsWith('PC|')) {
            $p = $k -split '\|'
            $own = $false
            foreach ($bk in @($b.Powercfg.Keys | Where-Object { $_ -like "*|$($p[1])|$($p[2])" })) {
                $g = ($bk -split '\|')[0]; $v = @($b.Powercfg[$bk])
                powercfg /setacvalueindex $g $p[1] $p[2] $v[0] | Out-Null
                powercfg /setdcvalueindex $g $p[1] $p[2] $v[1] | Out-Null
                if ($g -eq (Get-ActiveScheme)) { $own = $true }
                $b.Powercfg.Remove($bk)
            }
            if (-not $own) {
                powercfg /setacvalueindex SCHEME_CURRENT $p[1] $p[2] 1 | Out-Null
                powercfg /setdcvalueindex SCHEME_CURRENT $p[1] $p[2] 1 | Out-Null
            }
            powercfg /setactive SCHEME_CURRENT | Out-Null
            $n++
            continue
        }
        if ($keep.ContainsKey($k)) { $n++; continue }
        if ($b.Registry.ContainsKey($k)) { Restore-RegEntry $b.Registry[$k]; $b.Registry.Remove($k); $n++ }
    }
    if ($n -eq 0) { throw 'ไม่มีค่าเดิมเก็บไว้ ค่านี้ Windows หรือโปรแกรมอื่นตั้งไว้เอง' }
}
function Do-Revert {
    $list = @($Ids -split ',' | Where-Object { $_ -and $Tweaks.Contains($_) })
    $ok = 0; $fail = 0; $i = 0
    foreach ($id in $list) {
        $i++
        Emit @{ t = 'step'; id = $id; state = 'run'; i = $i; n = $list.Count; revert = $true }
        try {
            Revert-Tweak $id
            $Script:Backup.Applied = @($Script:Backup.Applied | Where-Object { $_ -ne $id })
            Save-Backup
            Emit @{ t = 'step'; id = $id; state = 'ok'; i = $i; n = $list.Count; revert = $true }
            $ok++
        } catch {
            Save-Backup
            Emit @{ t = 'step'; id = $id; state = 'fail'; i = $i; n = $list.Count; m = $_.Exception.Message; revert = $true }
            Write-Log "[คืนค่า $id] $($_.Exception.Message)"
            $fail++
        }
    }
    Emit @{ t = 'result'; action = 'Revert'; ok = $ok; fail = $fail }
}

function Do-Status {
    $st = @{}
    foreach ($k in $Tweaks.Keys) {
        try { $st[$k] = [bool](& $Tweaks[$k].Check) } catch { $st[$k] = $false }
    }
    if (-not $Script:AppliedKnown) {
        $Script:Backup.Applied = @($Tweaks.Keys | Where-Object { $st[$_] -and (Test-TweakOwned $_) })
        $Script:AppliedKnown = $true
        if (Test-HasBackup) { Save-Backup }
    }
    $zanos = $false
    try { $zanos = (Test-Path -LiteralPath (Join-Path $env:windir 'ZanOSModules')) -or (Test-Path -LiteralPath 'HKLM:\SOFTWARE\ZanOS') } catch { }
    $boot = 0; try { $boot = [DateTimeOffset]::new((Get-CimInstance Win32_OperatingSystem).LastBootUpTime).ToUnixTimeSeconds() } catch { }
    Emit @{ t = 'result'; action = 'Status'; boot = $boot; status = $st; hasBackup = (Test-HasBackup); applied = @($Script:Backup.Applied); zanos = [bool]$zanos }
}

function Do-SysInfo {
    $info = @{}
    try { $info.cpu = (Get-CimInstance Win32_Processor | Select-Object -First 1).Name.Trim() } catch { $info.cpu = '-' }
    try { $info.cores = [int](Get-CimInstance Win32_Processor | Measure-Object NumberOfLogicalProcessors -Sum).Sum } catch { }
    try { $info.gpu = ((Get-CimInstance Win32_VideoController | Where-Object { $_.Name -notmatch 'Basic|Remote|Virtual|Parsec|Meta' }).Name) -join ' + ' } catch { $info.gpu = '-' }
    try { $info.ram = [math]::Round((Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory / 1GB) } catch { }
    try { $os = Get-CimInstance Win32_OperatingSystem; $info.os = $os.Caption; $info.build = $os.BuildNumber; $info.ubr = (Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion' -EA 0).UBR } catch { }
    try {
        $ad = Get-UpAdapters | Select-Object -First 1
        if ($ad) { $info.net = $ad.InterfaceDescription; $info.link = $ad.LinkSpeed }
    } catch { }
    $fd = Get-FiveMData
    $info.fivem = [bool]$fd
    if ($fd) { $info.fivemPath = Split-Path (Split-Path $fd -Parent) -Parent; $info.cache = [math]::Round((Get-FolderSize (Join-Path $fd 'cache')) / 1MB) }
    Emit @{ t = 'result'; action = 'SysInfo'; info = $info }
}

function Do-Apply {
    $list = @($Ids -split ',' | Where-Object { $_ -and $Tweaks.Contains($_) })
    if ($Opt -match 'restorepoint') {
        Write-Log 'กำลังสร้างจุดคืนค่า Windows...'
        try {
            Enable-ComputerRestore -Drive "$env:SystemDrive\" -EA 0
            $before = @(Get-ComputerRestorePoint -EA 0).Count
            Checkpoint-Computer -Description 'Before Zan Booster' -RestorePointType MODIFY_SETTINGS -WarningAction SilentlyContinue
            $after = @(Get-ComputerRestorePoint -EA 0).Count
            if ($after -gt $before) { Write-Log 'สร้างจุดคืนค่าแล้ว' }
            else { Write-Log 'วันนี้สร้างจุดคืนค่าไปแล้ว Windows ให้สร้างได้วันละครั้ง ใช้ backup ของโปรแกรมแทน' }
        } catch { Write-Log 'สร้างจุดคืนค่าไม่ได้ ใช้ backup ของโปรแกรมแทน' }
    }
    $ok = 0; $fail = 0; $i = 0
    foreach ($id in $list) {
        $i++
        Emit @{ t = 'step'; id = $id; state = 'run'; i = $i; n = $list.Count }
        try {
            & $Tweaks[$id].Apply
            if ($Script:Backup.Applied -notcontains $id) { $Script:Backup.Applied = @($Script:Backup.Applied) + $id }
            Save-Backup
            Emit @{ t = 'step'; id = $id; state = 'ok'; i = $i; n = $list.Count }
            $ok++
        } catch {
            Save-Backup
            Emit @{ t = 'step'; id = $id; state = 'fail'; i = $i; n = $list.Count; m = $_.Exception.Message }
            Write-Log "[$id] $($_.Exception.Message)"
            $fail++
        }
    }
    Emit @{ t = 'result'; action = 'Apply'; ok = $ok; fail = $fail }
}

# ------------------------------------------------------- graphics ----
$GfxFile   = Join-Path $env:APPDATA 'CitizenFX\gta5_settings.xml'
$GfxBackup = Join-Path $DataDir 'gta5_settings.original.xml'

# ----- ตรวจสเปกเพื่อคำนวณกราฟิก -----
function Get-GpuInfo {
    $best = $null
    $cls = 'HKLM:\SYSTEM\ControlSet001\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}'
    Get-ChildItem $cls -EA 0 | Where-Object { $_.PSChildName -match '^\d{4}$' } | ForEach-Object {
        $pr = Get-ItemProperty $_.PSPath -EA 0
        $name = [string]$pr.DriverDesc
        $mem = $pr.'HardwareInformation.qwMemorySize'
        if (-not $mem) { $raw = $pr.'HardwareInformation.MemorySize'; if ($raw -is [byte[]]) { $mem = [BitConverter]::ToUInt32($raw, 0) } else { $mem = $raw } }
        if ($name -and $name -notmatch 'Basic|Remote|Virtual|Parsec|Meta') {
            $gb = [math]::Round([double]$mem / 1GB, 1)
            if (-not $best -or $gb -gt $best.vram) { $best = [pscustomobject]@{ name = $name; vram = $gb } }
        }
    }
    if (-not $best) {
        $vc = Get-CimInstance Win32_VideoController | Where-Object { $_.Name -notmatch 'Basic|Remote|Virtual' } | Select-Object -First 1
        $best = [pscustomobject]@{ name = [string]$vc.Name; vram = [math]::Round([double]$vc.AdapterRAM / 1GB, 1) }
    }
    $best
}

function Get-GpuTier([string]$n, [double]$vram) {
    $n = $n.ToUpper()
    if ($n -match 'RTX\s*(4070\s*TI|4080|4090|5070\s*TI|5080|5090|3080|3090)|RX\s*(7900|6800|6900|6950|9070)') { return 5 }
    if ($n -match 'RTX\s*(4060\s*TI|4070|5060\s*TI|5070|3070|3060\s*TI|2080)|RX\s*(6700|6750|7700|7800)|ARC\s*B580') { return 4 }
    if ($n -match 'RTX\s*(4060|5060|3060|2060|2070)|GTX\s*(1080|1070\s*TI)|RX\s*(6600|6650|7600|5700)|ARC\s*A7[57]0') { return 3 }
    if ($n -match 'RTX\s*(3050|4050|2050)|GTX\s*(1660|1650|1070|1060|980)|RX\s*(580|590|5500|5600|6500|6400|VEGA\s*(56|64))|ARC\s*A3[58]0') { return 2 }
    if ($n -match 'GTX|RX\s|RADEON|INTEL|UHD|IRIS|VEGA|MX\d') { return 1 }
    if ($vram -ge 12) { return 4 } elseif ($vram -ge 8) { return 3 } elseif ($vram -ge 4) { return 2 } else { return 1 }
}

function Get-CpuTier {
    $cpu = Get-CimInstance Win32_Processor | Select-Object -First 1
    $n = ([string]$cpu.Name).ToUpper(); $th = [int]$cpu.NumberOfLogicalProcessors
    $t = 2
    if ($n -match 'I9|RYZEN 9') { $t = 4 } elseif ($n -match 'I7|RYZEN 7') { $t = 4 } elseif ($n -match 'I5|RYZEN 5') { $t = 3 } elseif ($n -match 'I3|RYZEN 3') { $t = 2 }
    elseif ($n -match 'CELERON|PENTIUM|ATHLON|N\d{3,4}') { $t = 1 }
    # รุ่นใหม่ (Intel 12th+ / Ryzen 5000+ / Core Ultra) แรงขึ้นอีกขั้น
    $modern = $false
    if ($n -match 'I\d-(\d{2})\d{3}' -and [int]$Matches[1] -ge 12) { $modern = $true }
    if ($n -match 'RYZEN \d (\d)\d{3}' -and [int]$Matches[1] -ge 5) { $modern = $true }
    if ($n -match 'CORE\(TM\) ULTRA|CORE ULTRA') { $modern = $true; $t = [math]::Max($t, 3) }
    if ($modern) { $t++ }
    if ($th -le 4) { $t = [math]::Min($t, 2) }
    [math]::Max(1, [math]::Min(5, $t))
}

function Get-GfxPlan {
    $gi = Get-GpuInfo
    $GT = Get-GpuTier $gi.name $gi.vram
    $CT = Get-CpuTier
    $ram = [math]::Round((Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory / 1GB)
    if ($ram -lt 12) { $CT = [math]::Min($CT, 2) }
    $VR = [double]$gi.vram

    # ลื่นสุด: ต่ำสุดทุกอย่าง
    $perf = [ordered]@{
        # ลื่นสุด = ปิดทุกอย่างที่ปิดได้ (เงา 0 = ปิด), ที่ปิดไม่ได้ตั้งต่ำสุด
        TextureQuality = 0; ShaderQuality = 0; ShadowQuality = 0; Shadow_SoftShadows = 0; UltraShadows_Enabled = 'false'; Shadow_LongShadows = 'false'
        Shadow_ParticleShadows = 'false'; Shadow_Distance = 1.0; ReflectionQuality = 0; ReflectionMSAA = 0; Reflection_MipBlur = 'false'
        MSAA = 0; TXAA_Enabled = 'false'; FXAA_Enabled = 'false'; AnisotropicFiltering = 0; SSAO = 0; Tessellation = 0
        GrassQuality = 0; WaterQuality = 0; ParticleQuality = 0; PostFX = 0; DoF = 'false'; MotionBlurStrength = 0.0
        Lighting_FogVolumes = 'false'; Shader_SSA = 'false'; LodScale = 0.0; MaxLodScale = 0.0; PedLodBias = 0.0; VehicleLodBias = 0.0
        CityDensity = 0.0; PedVarietyMultiplier = 0.0; VehicleVarietyMultiplier = 0.0; HdStreamingInFlight = 'false'; VSync = 0
    }

    function Build([double]$lg, [double]$lc, [bool]$top) {
        $r = { param($max) [int][math]::Round($lg * $max) }
        $tex = $(if ($top) { if ($VR -ge 7) { 2 } elseif ($VR -ge 3.5) { 1 } else { 0 } } else { if ($VR -ge 10) { 2 } elseif ($VR -ge 5.5) { 1 } else { 0 } })
        $grass = & $r 3; if ($GT -lt 5) { $grass = [math]::Min($grass, 2) }
        $msaa = 0; $rmsaa = 0
        if ($top -and $GT -ge 5) { $msaa = 2; $rmsaa = 2 }
        [ordered]@{
            TextureQuality = $tex; ShaderQuality = (& $r 2); ShadowQuality = [math]::Max(1, (& $r 3)); Shadow_SoftShadows = (& $r 3)
            UltraShadows_Enabled = $(if ($top -and $GT -ge 5) { 'true' } else { 'false' })
            Shadow_LongShadows = $(if ($top -and $GT -ge 4) { 'true' } else { 'false' })
            Shadow_ParticleShadows = $(if ($lg -ge 0.5) { 'true' } else { 'false' })
            ReflectionQuality = (& $r 3); ReflectionMSAA = $rmsaa; MSAA = $msaa; TXAA_Enabled = 'false'; FXAA_Enabled = 'true'
            AnisotropicFiltering = $(if ($GT -ge 2) { 16 } else { 4 }); SSAO = (& $r 2); Tessellation = (& $r 3)
            GrassQuality = $grass; WaterQuality = (& $r 2); ParticleQuality = (& $r 2); PostFX = (& $r 3)
            DoF = 'false'; MotionBlurStrength = 0.0; Lighting_FogVolumes = $(if ($lg -ge 0.5) { 'true' } else { 'false' })
            LodScale = [math]::Round($lc, 2); MaxLodScale = $(if ($top) { [math]::Round([math]::Max(0.0, $lc - 0.6), 2) } else { 0.0 })
            PedLodBias = [math]::Round($lc * 0.2, 2); VehicleLodBias = [math]::Round($lc * 0.2, 2)
            CityDensity = [math]::Round($lc, 2); PedVarietyMultiplier = [math]::Round($lc, 2); VehicleVarietyMultiplier = [math]::Round($lc, 2)
            VSync = 0
        }
    }
    $balLg = @(0.2, 0.35, 0.5, 0.65, 0.8)[$GT - 1]; $balLc = @(0.4, 0.55, 0.7, 0.8, 0.9)[$CT - 1]
    $qLg   = @(0.4, 0.6, 0.8, 0.95, 1.0)[$GT - 1];  $qLc   = @(0.6, 0.75, 0.9, 1.0, 1.0)[$CT - 1]
    [pscustomobject]@{
        gpu = $gi.name; vram = $VR; cpu = ((Get-CimInstance Win32_Processor | Select-Object -First 1).Name).Trim(); ram = $ram
        gpuTier = $GT; cpuTier = $CT
        presets = @{ performance = $perf; balanced = (Build $balLg $balLc $false); quality = (Build $qLg $qLc $true) }
    }
}

function Do-GfxPlan {
    $p = Get-GfxPlan
    Emit @{ t = 'result'; action = 'GfxPlan'; gpu = $p.gpu; vram = $p.vram; cpu = $p.cpu; ram = $p.ram; gpuTier = $p.gpuTier; cpuTier = $p.cpuTier; presets = $p.presets }
}

function Do-GfxStatus {
    Emit @{ t = 'result'; action = 'GfxStatus'; found = (Test-Path $GfxFile); hasBackup = (Test-Path $GfxBackup); path = $GfxFile }
}

# ให้หน้า UI ในเกม (NUI) วาดในโปรเซสเดียวกับเกม เป็นตัวเลือกของ FiveM เอง เก็บค่าเดิมไว้คืนได้
$NuiCfg = Join-Path $env:APPDATA 'CitizenFX\fivem.cfg'
$NuiBackup = Join-Path $DataDir 'nui_gpu.original.txt'
function Set-NuiGpu([bool]$on) {
    if (Get-Process -EA 0 | Where-Object { $_.Name -like 'FiveM*' }) { throw 'ปิด FiveM ก่อน เกมจะเขียนทับไฟล์ตอนปิด' }
    if (-not (Test-Path -LiteralPath $NuiCfg)) { throw 'ยังไม่มีไฟล์ตั้งค่า FiveM เข้าเกมสักครั้งก่อน' }
    if (-not $on -and -not (Test-Path -LiteralPath $NuiBackup)) { return }
    $utf = New-Object System.Text.UTF8Encoding($false)
    $t = [IO.File]::ReadAllText($NuiCfg, $utf); $nl = $(if ($t.Contains("`r`n")) { "`r`n" } else { "`n" })
    $rx = [regex]'(?m)^[ \t]*seta?[ \t]+"?nui_useInProcessGpu"?[ \t]+[^\r\n]*(\r?\n)?'
    if ($on) {
        if (-not (Test-Path -LiteralPath $NuiBackup)) { [IO.File]::WriteAllText($NuiBackup, $rx.Match($t).Value.Trim(), $utf) }
        $line = 'seta "nui_useInProcessGpu" "true"'
    } else { $line = [IO.File]::ReadAllText($NuiBackup, $utf).Trim() }
    $t = $rx.Replace($t, '')
    if ($line) { if ($t -and -not $t.EndsWith("`n")) { $t += $nl }; $t += $line + $nl }
    $tmp = "$NuiCfg.zan"; [IO.File]::WriteAllText($tmp, $t, $utf); Move-Item -LiteralPath $tmp -Destination $NuiCfg -Force
    if (-not $on) { Remove-Item -LiteralPath $NuiBackup -Force }
}
function Do-NuiGpu {
    try { Set-NuiGpu ($Opt -ne 'off'); Write-Log "NUI in-process GPU: $Opt"; Emit @{ t = 'result'; action = 'NuiGpu'; ok = $true } }
    catch { Emit @{ t = 'result'; action = 'NuiGpu'; ok = $false; m = $_.Exception.Message } }
}

# ปิด overlay ของ Discord ทุกตัว + Steam: เปลี่ยนชื่อไฟล์โมดูล / ตั้งค่า Steam คืนได้
$OvBak = Join-Path $DataDir 'overlay.json'
function Get-OvDisc {
    foreach ($v in 'Discord', 'DiscordPTB', 'DiscordCanary', 'DiscordDevelopment') {
        $d = Join-Path $env:LOCALAPPDATA $v
        if (-not (Test-Path -LiteralPath $d)) { continue }
        $f = @(Get-ChildItem "$d\app-*\modules\discord_*overlay*\*\*.node*" -EA 0 | ForEach-Object { $_.FullName })
        [pscustomobject]@{ n = $v; d = $d; on = @($f -like '*.node'); off = @($f -like '*.node.zanoff') }
    }
}
function Get-OvSteam { [string](Get-Reg 'HKCU:\Software\Valve\Steam' 'SteamPath') -replace '/', '\' }
function Get-OvVdf { $s = Get-OvSteam; if ($s) { @(Get-ChildItem "$s\userdata\*\config\localconfig.vdf" -EA 0 | ForEach-Object { $_.FullName }) } }
function Get-OvSys([string]$x) { [regex]::Match($x, '(?m)^\t"system"\r?\n\t\{\r?\n[\s\S]*?^\t\}') }
function Get-OvVal([string]$f) { $m = [regex]::Match((Get-OvSys ([IO.File]::ReadAllText($f))).Value, '(?m)^\t\t"EnableGameOverlay"\t\t"(\d)"'); if ($m.Success) { $m.Groups[1].Value } else { '' } }
function Set-OvVal([string]$f, [string]$v) {
    $x = [IO.File]::ReadAllText($f); $nl = $(if ($x.Contains("`r`n")) { "`r`n" } else { "`n" }); $m = Get-OvSys $x
    if (-not $m.Success) { throw 'อ่านไฟล์ตั้งค่า Steam ไม่ได้' }
    $b = [regex]::Replace($m.Value, '(?m)^\t\t"EnableGameOverlay"[^\n]*\n', '')
    if ($v -ne '') { $b = $b.Insert($b.IndexOf("{$nl") + 1 + $nl.Length, "`t`t`"EnableGameOverlay`"`t`t`"$v`"$nl") }
    $x = $x.Remove($m.Index, $m.Length).Insert($m.Index, $b)
    [IO.File]::WriteAllText("$f.zan", $x, (New-Object System.Text.UTF8Encoding($false))); Move-Item -LiteralPath "$f.zan" -Destination $f -Force
}
function Get-WebCache([string]$d) { 'Cache', 'Code Cache', 'GPUCache' | ForEach-Object { "$d\$_" } }
function Clear-WebCache([string]$d, [string[]]$n) {
    for ($i = 0; $i -lt 20 -and (Get-Process -Name $n -EA 0); $i++) { Start-Sleep -Milliseconds 300 }
    if (Get-Process -Name $n -EA 0) { return }
    Get-WebCache $d | Where-Object { Test-Path -LiteralPath $_ } | ForEach-Object { [void](Remove-DirContents $_) }
}
function Do-OvOff {
    $un = $Opt -eq 'undo'; $ok = @(); $er = @()
    try {
        if (Get-Process -EA 0 | Where-Object { $_.Name -like 'FiveM*' }) { throw 'ปิด FiveM ก่อน แล้วกดอีกครั้ง' }
        foreach ($v in @(Get-OvDisc)) {
            $fs = @($(if ($un) { $v.off } else { $v.on })); if (-not $fs.Count) { continue }
            $ps = @(Get-Process -Name $v.n -EA 0 | Where-Object { $_.Path -like "$($v.d)\*" }); $exe = $(if ($ps.Count) { $ps[0].Path } else { '' })
            try {
                $ps | ForEach-Object { try { $_.Kill(); [void]$_.WaitForExit(5000) } catch { } }
                foreach ($f in $fs) {
                    $to = $(if ($un) { $f -replace '\.zanoff$', '' } else { "$f.zanoff" })
                    for ($i = 0; ; $i++) {
                        try {
                            if ($un -and (Test-Path -LiteralPath $to)) { Remove-Item -LiteralPath $f -Force } else { Move-Item -LiteralPath $f -Destination $to -Force }
                            break
                        } catch { if ($i -ge 15) { throw }; Start-Sleep -Milliseconds 300 }
                    }
                }
                $ok += $v.n
                if ($exe) { Clear-WebCache "$env:APPDATA\$($v.n.ToLower())" $v.n }
            } catch { $er += "$($v.n): $($_.Exception.Message)" }
            finally { if ($exe) { Start-Process explorer.exe "`"$exe`"" } }
        }
        $bk = @{}; if (Test-Path $OvBak) { (Get-Content $OvBak -Raw -Encoding UTF8 | ConvertFrom-Json).PSObject.Properties | ForEach-Object { $bk[$_.Name] = [string]$_.Value } }
        $vf = @(Get-OvVdf | Where-Object { if ($un) { $bk.ContainsKey($_) } else { (Get-OvVal $_) -ne '0' } })
        if ($vf.Count) {
            $exe = ''
            try {
                if ([int](Get-Reg 'HKCU:\Software\Valve\Steam' 'RunningAppID')) { throw 'มีเกม Steam เปิดอยู่ ปิดเกมก่อน' }
                if (Get-Process steam -EA 0) {
                    $x = (Get-OvSteam) + '\steam.exe'; Start-Process $x '-shutdown'; $exe = $x
                    for ($i = 0; $i -lt 60 -and (Get-Process steam -EA 0); $i++) { Start-Sleep -Milliseconds 500 }
                    if (Get-Process steam -EA 0) { throw 'Steam ยังไม่ปิด ลองกดอีกครั้ง' }
                }
                if ($exe) { Clear-WebCache "$env:LOCALAPPDATA\Steam\htmlcache\Default" 'steam', 'steamwebhelper' }
                foreach ($f in $vf) {
                    if ($un) { Set-OvVal $f $bk[$f]; $bk.Remove($f) }
                    else { if (-not $bk.ContainsKey($f)) { $bk[$f] = Get-OvVal $f; $bk | ConvertTo-Json | Set-Content $OvBak -Encoding UTF8 }; Set-OvVal $f '0' }
                }
                if ($un) { if ($bk.Count) { $bk | ConvertTo-Json | Set-Content $OvBak -Encoding UTF8 } else { Remove-Item $OvBak -Force } }
                $ok += 'Steam'
            } catch { $er += "Steam: $($_.Exception.Message)" }
            finally { if ($exe -and -not (Get-Process steam -EA 0)) { Start-Process explorer.exe "`"$exe`"" } }
        }
    } catch { $er += $_.Exception.Message }
    if ($ok.Count) { Write-Log "overlay $Opt : $($ok -join ', ')" }
    Emit @{ t = 'result'; action = 'OvOff'; ok = ($ok.Count -gt 0); n = ($ok -join ', '); er = ($er -join ' · ') }
}
# จูนเร็ว: ลดเฉพาะชุดที่ตัน ลดอย่างเดียวไม่เพิ่ม เก็บไฟล์ก่อนจูนไว้คืน
$TuneBackup = Join-Path $DataDir 'gta5_settings.pretune.xml'
$StepBackup = Join-Path $DataDir 'gta5_settings.prestep.xml'
function Do-GfxTune {
    try {
        if ($Opt -eq 'keep') { Remove-Item $TuneBackup, $StepBackup -Force -EA 0; Emit @{ t = 'result'; action = 'GfxTune'; ok = $true; n = 0 }; return }
        if (Get-Process -EA 0 | Where-Object { $_.Name -like 'FiveM*' }) { throw 'ปิด FiveM ก่อน เกมจะเขียนทับไฟล์ตอนปิด' }
        if ($Opt -eq 'stepback') {
            if (-not (Test-Path $StepBackup)) { throw 'ไม่มีไฟล์ก่อนขั้นนี้เก็บไว้' }
            Copy-Item $StepBackup $GfxFile -Force; Remove-Item $StepBackup -Force
            Emit @{ t = 'result'; action = 'GfxTune'; ok = $true; n = 0 }; return
        }
        if ($Opt -eq 'undo') {
            if (-not (Test-Path $TuneBackup)) { throw 'ไม่มีไฟล์ก่อนจูนเก็บไว้' }
            Copy-Item $TuneBackup $GfxFile -Force; Remove-Item $TuneBackup, $StepBackup -Force -EA 0
            Emit @{ t = 'result'; action = 'GfxTune'; ok = $true; n = 0 }; return
        }
        if (-not (Test-Path $GfxFile)) { throw 'ยังไม่มีไฟล์ตั้งค่ากราฟิก เข้าเกม FiveM สักครั้งก่อน' }
        $g = @{ ShadowQuality = 1; Shadow_SoftShadows = 0; Shadow_Distance = 1; UltraShadows_Enabled = 'false'; Shadow_LongShadows = 'false'; Shadow_ParticleShadows = 'false'; ReflectionQuality = 0; ReflectionMSAA = 0; MSAA = 0; MSAAFragments = 0; TXAA_Enabled = 'false'; SSAO = 0; Tessellation = 0; GrassQuality = 0; WaterQuality = 0; ParticleQuality = 0; PostFX = 0; Lighting_FogVolumes = 'false'; DoF = 'false'; MotionBlurStrength = 0; VSync = 0 }
        $c = @{ LodScale = 0; MaxLodScale = 0; PedLodBias = 0; VehicleLodBias = 0; CityDensity = 0; PedVarietyMultiplier = 0; VehicleVarietyMultiplier = 0; GrassQuality = 0; ShadowQuality = 1; Shadow_LongShadows = 'false'; UltraShadows_Enabled = 'false'; HdStreamingInFlight = 'false'; VSync = 0 }
        $set = @{}; if ($Opt -in 'gpu', 'both') { foreach ($k in $g.Keys) { $set[$k] = $g[$k] } }; if ($Opt -in 'cpu', 'both') { foreach ($k in $c.Keys) { $set[$k] = $c[$k] } }
        $step = $Opt -match '^(gpu|cpu)([123])$'
        if ($step) {
            $k1 = $Matches[1]; $lv = [int]$Matches[2]; $src = $(if ($k1 -eq 'gpu') { $g } else { $c })
            $t1 = 'ReflectionMSAA TXAA_Enabled Lighting_FogVolumes DoF MotionBlurStrength Shadow_ParticleShadows Shadow_LongShadows UltraShadows_Enabled VSync MaxLodScale VehicleLodBias PedLodBias HdStreamingInFlight' -split ' '
            $t2 = 'MSAA MSAAFragments Tessellation SSAO Shadow_SoftShadows WaterQuality ParticleQuality LodScale CityDensity PedVarietyMultiplier VehicleVarietyMultiplier' -split ' '
            foreach ($k in $src.Keys) { if ($lv -eq 3 -or $t1 -contains $k -or ($lv -eq 2 -and $t2 -contains $k)) { $set[$k] = $(if ($lv -eq 2 -and $k1 -eq 'cpu' -and $t2 -contains $k) { 0.5 } else { $src[$k] }) } }
        }
        if (-not $set.Count) { throw 'ไม่รู้จักแบบจูนนี้' }
        [xml]$x = Get-Content $GfxFile -Raw -Encoding UTF8
        $ci = [Globalization.CultureInfo]::InvariantCulture; $n = 0
        foreach ($k in $set.Keys) {
            foreach ($node in @($x.SelectNodes("//$k"))) {
                $a = $node.Attributes['value']; if (-not $a) { continue }
                if ($set[$k] -is [string]) { if ($a.Value -ne 'false') { $a.Value = 'false'; $n++ } }
                else { $v = 0.0; if ([double]::TryParse($a.Value, [Globalization.NumberStyles]::Float, $ci, [ref]$v) -and $v -gt $set[$k]) { $a.Value = $(if ($a.Value.Contains('.')) { ([double]$set[$k]).ToString('0.000000', $ci) } else { [string]$set[$k] }); $n++ } }
            }
        }
        if ($n) {
            if (-not (Test-Path $GfxBackup)) { Copy-Item $GfxFile $GfxBackup -Force }
            if (-not (Test-Path $TuneBackup)) { Copy-Item $GfxFile $TuneBackup -Force }
            if ($step) { Copy-Item $GfxFile $StepBackup -Force }
            $cs = $x.SelectSingleNode('//configSource'); if ($cs) { $cs.InnerText = 'SMC_USER' }
            $s = New-Object System.Xml.XmlWriterSettings; $s.Indent = $true; $s.Encoding = New-Object System.Text.UTF8Encoding($false)
            $w = [System.Xml.XmlWriter]::Create($GfxFile, $s); $x.Save($w); $w.Close()
            Write-Log "จูนกราฟิก ($Opt) $n ค่า"
        }
        Emit @{ t = 'result'; action = 'GfxTune'; ok = $true; n = $n }
    } catch { Emit @{ t = 'result'; action = 'GfxTune'; ok = $false; m = $_.Exception.Message } }
}

function Do-GfxApply {
    $preset = $Opt
    $GfxPresets = (Get-GfxPlan).presets
    # ปรับแต่งเอง: รับเฉพาะชื่อค่าที่มีในชุดลื่นสุด และค่าเป็นตัวเลขหรือ true/false
    if ($preset -eq 'custom') {
        $c = [ordered]@{}; $pf = $GfxPresets.performance
        foreach ($kv in $Ids -split ';') { $k, $v = $kv -split '=', 2; if ($pf.Contains($k) -and $v -match '^(\d+(\.\d+)?|true|false)$') { $c[$k] = $(if ($pf[$k] -is [double]) { [double]::Parse($v, [Globalization.CultureInfo]::InvariantCulture) } elseif ($v -match '^\d+$') { [int]$v } else { $v }) } }
        if ($c.Count) { $GfxPresets.custom = $c }
    }
    if (-not $GfxPresets.ContainsKey($preset)) { Emit @{ t = 'result'; action = 'GfxApply'; ok = $false; m = 'ไม่รู้จัก preset นี้' }; return }
    if (Get-Process -EA 0 | Where-Object { $_.Name -like 'FiveM*' }) {
        Emit @{ t = 'result'; action = 'GfxApply'; ok = $false; m = 'ปิด FiveM ก่อน เกมจะเขียนทับไฟล์ตอนปิด' }; return
    }
    if (-not (Test-Path $GfxFile)) {
        Emit @{ t = 'result'; action = 'GfxApply'; ok = $false; m = 'ยังไม่มีไฟล์ตั้งค่ากราฟิก เข้าเกม FiveM สักครั้งก่อนแล้วลองใหม่' }; return
    }
    if (-not (Test-Path $GfxBackup)) { Copy-Item $GfxFile $GfxBackup -Force; Write-Log 'เก็บไฟล์กราฟิกเดิมไว้แล้ว' }
    [xml]$x = Get-Content $GfxFile -Raw -Encoding UTF8
    $n = 0
    foreach ($k in $GfxPresets[$preset].Keys) {
        $nodes = @($x.SelectNodes("//$k"))
        foreach ($node in $nodes) {
            $attr = $node.Attributes['value']
            if (-not $attr) { continue }
            $v = $GfxPresets[$preset][$k]
            if ($v -is [double] -or $v -is [single] -or ($v -is [decimal])) { $attr.Value = $v.ToString('0.000000', [Globalization.CultureInfo]::InvariantCulture) }
            else { $attr.Value = [string]$v }
            $n++
        }
    }
    $cs = $x.SelectSingleNode('//configSource'); if ($cs) { $cs.InnerText = 'SMC_USER' }
    $settings = New-Object System.Xml.XmlWriterSettings
    $settings.Indent = $true; $settings.Encoding = New-Object System.Text.UTF8Encoding($false)
    $w = [System.Xml.XmlWriter]::Create($GfxFile, $settings); $x.Save($w); $w.Close()
    Emit @{ t = 'result'; action = 'GfxApply'; ok = $true; n = $n }
}

function Do-GfxRestore {
    if (-not (Test-Path $GfxBackup)) { Emit @{ t = 'result'; action = 'GfxRestore'; ok = $false; m = 'ยังไม่เคยเปลี่ยนกราฟิกผ่านโปรแกรม' }; return }
    if (Get-Process -EA 0 | Where-Object { $_.Name -like 'FiveM*' }) {
        Emit @{ t = 'result'; action = 'GfxRestore'; ok = $false; m = 'ปิด FiveM ก่อน' }; return
    }
    Copy-Item $GfxBackup $GfxFile -Force
    Remove-Item $GfxBackup -Force
    Emit @{ t = 'result'; action = 'GfxRestore'; ok = $true }
}

# --------------------------------------------------------- health ----
function Get-DriveMedia([string]$path) {
    try {
        $letter = ([IO.Path]::GetPathRoot($path)).Substring(0, 1)
        $part = Get-Partition -DriveLetter $letter -ErrorAction Stop
        $disk = Get-Disk -Number $part.DiskNumber -ErrorAction Stop
        $pd = Get-PhysicalDisk -EA 0 | Where-Object { [string]$_.DeviceId -eq [string]$disk.Number } | Select-Object -First 1
        $media = [string]$pd.MediaType; $bus = [string]$pd.BusType
        if ($bus -eq 'NVMe') { return 'NVMe SSD' }
        if ($media -eq 'SSD') { return 'SSD' }
        if ($media -eq 'HDD') { return 'HDD' }
        if ($disk.FriendlyName -match 'SSD|NVMe') { return 'SSD' }
        return 'ไม่ทราบ'
    } catch { 'ไม่ทราบ' }
}

# ไฟล์ exe ของ FiveM ที่ควรใช้การ์ดจอแยก (FiveM.exe + ตัวเกมทุก build ใน cache\subprocess + GTA5.exe)
$KIF = 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options'
function Get-FiveMPrioNames {
    $n = [Collections.Generic.List[string]]::new(); $n.Add('FiveM_GTAProcess.exe'); $n.Add('FiveM_GameProcess.exe')
    foreach ($b in 1604, 2060, 2189, 2372, 2545, 2612, 2699, 2802, 2944, 3095, 3258, 3323, 3407, 3570) { $n.Add("FiveM_b$($b)_GTAProcess.exe") }
    foreach ($x in Get-FiveMExes) { $l = Split-Path $x -Leaf; if ($l -match '(?i)(GTAProcess|GameProcess)\.exe$' -and -not $n.Contains($l)) { $n.Add($l) } }
    $n
}
function Get-FiveMExes {
    $out = [Collections.Generic.List[string]]::new()
    $fd = Get-FiveMData
    if ($fd) {
        $root = Split-Path (Split-Path $fd -Parent) -Parent
        $fe = Join-Path $root 'FiveM.exe'; if (Test-Path -LiteralPath $fe) { $out.Add($fe) }
        $sub = Join-Path $fd 'cache\subprocess'
        if (Test-Path -LiteralPath $sub) { Get-ChildItem -LiteralPath $sub -Filter '*.exe' -EA 0 | Where-Object { $_.Name -match 'GTAProcess|GameProcess' } | ForEach-Object { $out.Add($_.FullName) } }
        $ini = Join-Path (Split-Path $fd -Parent) 'CitizenFX.ini'
        if (Test-Path -LiteralPath $ini) {
            $m = Select-String -LiteralPath $ini -Pattern '^IVPath=(.+)$' -EA 0 | Select-Object -First 1
            if ($m) { $g = Join-Path $m.Matches[0].Groups[1].Value.Trim() 'GTA5.exe'; if (Test-Path -LiteralPath $g) { $out.Add($g) } }
        }
    }
    $out
}

# ความเร็วแรมจากรหัสรุ่น: นับเฉพาะความเร็วที่มีขายจริง และแรมคละรุ่นใช้ตัวที่ช้าที่สุด
function Get-KitSpeed($mods) {
    $o = @(foreach ($m in $mods) {
        $v = 0
        foreach ($x in [regex]::Matches([string]$m.PartNumber, '(?<!\d)(2400|2666|2800|2933|3000|3200|3333|3466|3600|3733|3866|4000|4133|4266|4400|4600|4800|5200|5600|6000|6200|6400|6600|6800|7000|7200|7600|8000|8200|8400)(?!\d)')) { $v = [math]::Max($v, [int]$x.Value) }
        $v
    })
    [int]($o | Measure-Object -Minimum).Minimum
}
# เสียง: บริการเสียงของ Windows และอุปกรณ์ที่ใช้งานได้
# เอฟเฟกต์ของคนอื่นที่คอมไพล์ไม่ผ่านใน FiveM: เปลี่ยนชื่อเป็น .off ให้ ReShade ข้าม (ไม่ลบ เปลี่ยนชื่อกลับได้)
function Do-RsFix {
    $n = 0; $fd = Get-FiveMData
    if ($fd) { foreach ($x in 'METEOR\MartysMods_LONGEXPOSURE.fx', 'UniqFX\UniqFX_ConditionalUIMask.fx') { $q = Join-Path (Split-Path $fd) "plugins\reshade-shaders\Shaders\$x"; if (Test-Path -LiteralPath $q) { try { Move-Item -LiteralPath $q "$q.off" -Force -ErrorAction Stop; $n++ } catch { } } } }
    Emit @{ t = 'result'; action = 'RsFix'; ok = $true; n = $n }
}
# ระดับพรีเซ็ต Zan: ใส่ ZAN_LEVEL ใน ReShade.ini (ปิดเกมก่อน ReShade เขียนทับตอนปิด)
# ปรับภาพเอง: เขียนพรีเซ็ต Zan_Custom.ini (ค่าของ ZanGrade) แล้วชี้ ReShade.ini ไปที่ไฟล์นี้ เก็บพรีเซ็ตเดิมไว้คืน
# Ids set (Opt = Name=ค่า;Name=a/b/c) หรือ off (กลับไปพรีเซ็ตเดิม) · ต้องปิดเกมก่อน
function Do-RsCustom {
    $m = ''; $fd = Get-FiveMData; $pl = if ($fd) { Join-Path (Split-Path $fd) 'plugins' }; $ini = if ($pl) { Join-Path $pl 'ReShade.ini' }
    if (-not $ini -or -not (Test-Path $ini)) { $m = 'noreshade' }
    elseif (Get-Process FiveM* -EA 0) { $m = 'running' }
    elseif (-not (Get-ChildItem $pl -Recurse -Filter ZanGrade.fx -EA 0 | Select-Object -First 1)) { $m = 'nozan' }
    else {
        $c = Get-Config; $x = [IO.File]::ReadAllText($ini); $f = Join-Path $pl 'Zan_Custom.ini'
        $cur = [regex]::Match($x, '(?m)^PresetPath=(.*)$').Groups[1].Value.Trim()
        if ($Ids -eq 'set') {
            $l = foreach ($kv in $Opt -split ';') { $k, $v = $kv -split '=', 2; if ($k -match '^[A-Za-z]+$' -and $v -match '^-?[\d.]+(/-?[\d.]+){0,2}$') { "$k=$($v.Replace('/', ','))" } }
            [IO.File]::WriteAllText($f, "Techniques=ZanGrade@ZanGrade.fx`r`nTechniqueSorting=ZanGrade@ZanGrade.fx`r`n`r`n[ZanGrade.fx]`r`n" + ($l -join "`r`n") + "`r`n")
            if ($cur -notlike '*Zan_Custom.ini') { $c | Add-Member rsPrev $cur -Force; Save-Config $c }
            $to = $f
        } else { $to = [string]$c.rsPrev }
        if ($to -or $Ids -eq 'set' -or $cur -like '*Zan_Custom.ini') {
            $x = $(if ($x -match '(?m)^PresetPath=') { [regex]::Replace($x, '(?m)^PresetPath=.*$', "PresetPath=$($to.Replace('$', '$$'))") } else { $x -replace '(?m)^\[GENERAL\]\s*$', "[GENERAL]`r`nPresetPath=$to" })
            [IO.File]::WriteAllText($ini, $x); Write-Log "ReShade preset -> $to"
        }
        $m = 'ok'
    }
    Emit @{ t = 'result'; action = 'RsCustom'; ok = ($m -eq 'ok'); m = $m }
}

function Do-RsLevel {
    $ok = $false; $fd = Get-FiveMData; $p = if ($fd) { Join-Path (Split-Path $fd) 'plugins\ReShade.ini' }
    if ($p -and (Test-Path $p) -and -not (Get-Process FiveM*)) {
        $k = 'PreprocessorDefinitions='
        $x = [IO.File]::ReadAllText($p) -replace 'ZAN_LEVEL=\d,?' -replace "(?m)^$k", "${k}ZAN_LEVEL=$Opt," -replace '(ZAN_LEVEL=\d),(?=\r?\n)', '$1'
        [IO.File]::WriteAllText($p, $x); $ok = $x -match 'ZAN_LEVEL'
    }
    Emit @{ t = 'result'; action = 'RsLevel'; ok = $ok }
}
function Get-Audio {
    $sv = @(foreach ($n in 'AudioEndpointBuilder', 'Audiosrv') { $s = Get-Service $n -EA 0; if (-not $s -or $s.Status -ne 'Running' -or (Get-Reg "HKLM:\SYSTEM\CurrentControlSet\Services\$n" 'Start') -ne 2) { $n } })
    $out = @(Get-PnpDevice -Class AudioEndpoint -PresentOnly -EA 0 | Where-Object { $_.Status -eq 'OK' -and $_.InstanceId -like '*{0.0.0.00000000}*' } | ForEach-Object { $_.FriendlyName })
    @{ sv = $sv; out = $out }
}
function Do-AudioFix {
    $m = @(); $old = @{}; $er = ''
    try {
        $a = Get-Audio
        foreach ($n in 'AudioEndpointBuilder', 'Audiosrv') { $k = "HKLM:\SYSTEM\CurrentControlSet\Services\$n"; $old[$n] = Get-Reg $k 'Start'; if ($old[$n] -ne 2) { & sc.exe config $n start= auto | Out-Null } }
        if ($a.sv.Count) { Save-FpsBackup 'audio' $old; $m += 'เปิดบริการเสียง' }
        if (-not $a.out.Count) { foreach ($d in @(Get-PnpDevice -Class MEDIA -PresentOnly -EA 0 | Where-Object { "$($_.Problem)" -match 'DISABLED$|^22$' })) { Enable-PnpDevice -InstanceId $d.InstanceId -Confirm:$false; $m += "เปิด $($d.FriendlyName)" } }
        Stop-Service Audiosrv -Force -EA 0
        Restart-Service AudioEndpointBuilder -Force
    } catch { $er = $_.Exception.Message } finally { Start-Service Audiosrv -EA 0 }
    Start-Sleep 3; $a = Get-Audio; Write-Log "เสียง: $($m -join ', ') $er"
    Start-Process explorer.exe 'ms-settings:sound'
    Emit @{ t = 'result'; action = 'AudioFix'; ok = (-not $er -and -not $a.sv.Count); m = $er; out = ($a.out -join ', ') }
}
function Do-Health {
    $r = [Collections.Generic.List[object]]::new()
    function Add($id, $title, $status, $detail, $fix) { $r.Add([ordered]@{ id = $id; title = $title; status = $status; detail = $detail; fix = $fix }) }

    # RAM speed / XMP
    try {
        $mods = @(Get-CimInstance Win32_PhysicalMemory)
        $cfg = ($mods | Measure-Object ConfiguredClockSpeed -Maximum).Maximum
        $rated = ($mods | Measure-Object Speed -Minimum).Minimum
        $type = ($mods | Select-Object -First 1).SMBIOSMemoryType
        $ddr = switch ($type) { 26 { 'DDR4' } 34 { 'DDR5' } 24 { 'DDR3' } default { 'DDR' } }
        $low = ($ddr -eq 'DDR4' -and $cfg -le 2666) -or ($ddr -eq 'DDR5' -and $cfg -le 4800) -or ($rated -gt $cfg)
        $kit = Get-KitSpeed $mods
        if ($kit -ge $cfg + 200) { Add 'xmp' 'ความเร็วแรม' 'warn' "รุ่นแรมน่าจะเป็น $kit MHz แต่วิ่งอยู่ $cfg MHz" 'เข้าไบออสแล้วเปิด XMP / EXPO / DOCP' }
        elseif ($kit -and $cfg -ge $kit - 50) { Add 'xmp' 'ความเร็วแรม' 'ok' "แรม $ddr วิ่งที่ $cfg MHz" '' }
        elseif ($low) { Add 'xmp' 'ความเร็วแรม' 'warn' "แรม $ddr วิ่งอยู่ที่ $cfg MHz อาจยังไม่ได้เปิด XMP / EXPO" 'ถ้าแรมที่ซื้อมาเป็น 3200 MHz ขึ้นไป (DDR4) หรือ 5600 ขึ้นไป (DDR5) ให้เปิด XMP / EXPO ในไบออส ดูทางไปและปุ่มรีสตาร์ทเข้าไบออสในหัวข้อไบออสด้านล่าง ข้อนี้ช่วย FPS ใน FiveM ได้มากที่สุด' }
        else { Add 'xmp' 'ความเร็วแรม' 'ok' "แรม $ddr วิ่งที่ $cfg MHz" '' }

        # channels
        $sizes = @($mods | ForEach-Object { [math]::Round($_.Capacity / 1GB) })
        $total = ($sizes | Measure-Object -Sum).Sum
        $txt = ($sizes | ForEach-Object { "$_" }) -join ' + '
        if ($mods.Count -eq 1) { Add 'chan' 'แรมแบบ Dual Channel' 'warn' "ใส่แรมแผงเดียว ($txt GB) ทำงานแบบ Single Channel" 'ใส่แรมเพิ่มอีกแผงขนาดเท่ากัน FPS จะขึ้นชัดเจนในเกมที่ใช้ CPU หนักอย่าง FiveM' }
        elseif (($sizes | Select-Object -Unique).Count -gt 1) { Add 'chan' 'แรมแบบ Dual Channel' 'info' "แรมคนละขนาด ($txt GB) ส่วนที่เกินจะทำงานแบบ Single Channel" 'ถ้าจะอัปเกรด ให้ใช้แรมขนาดเท่ากันทุกแผง' }
        else { Add 'chan' 'แรมแบบ Dual Channel' 'ok' "$($mods.Count) แผง ($txt GB)" '' }
        if ($total -lt 16) { Add 'ramsize' 'ขนาดแรม' 'warn' "แรมรวม $total GB" 'FiveM กับเซิร์ฟที่มี mod เยอะควรมี 16 GB ขึ้นไป' }
    } catch { }

    # การ์ดจอวิ่งกี่เลน PCIe (ตอนนี้/สูงสุด) ข้อความภาษาไทยอยู่ใน ui.html
    try { foreach ($g in @(Get-CimInstance Win32_VideoController | Where-Object { $_.Name -match 'NVIDIA|AMD|Radeon|Arc' })) { $w = foreach ($i in 10, 12) { [int](Get-PnpDeviceProperty -InstanceId $g.PNPDeviceID -KeyName "{3ab22e31-8264-4b4e-9af5-a8d2d8e33e62} $i").Data }; if ($w[0]) { Add 'pcie' $g.Name $(if ($w[0] -lt $w[1]) { 'warn' } else { 'ok' }) "$($w[0])/$($w[1])" '' } } } catch { }

    # รีเฟรชเรตจอ ตรวจในตัวโปรแกรม (ทุกจอ + ปุ่มแก้)
    try {
        # driver age
        $gpu = Get-CimInstance Win32_VideoController | Where-Object { $_.Name -match 'NVIDIA|AMD|Radeon|Intel\(R\) Arc' } | Select-Object -First 1
        if ($gpu -and $gpu.DriverDate) {
            $days = [int]((Get-Date) - $gpu.DriverDate).TotalDays
            $d = $gpu.DriverDate.ToString('dd/MM/yyyy')
            if ($days -gt 180) { Add 'driver' 'ไดรเวอร์การ์ดจอ' 'warn' "ไดรเวอร์ลงวันที่ $d (ประมาณ $([math]::Round($days / 30)) เดือนก่อน)" 'โหลดไดรเวอร์ล่าสุดจาก NVIDIA App / AMD Adrenalin' }
            else { Add 'driver' 'ไดรเวอร์การ์ดจอ' 'ok' "ลงวันที่ $d" '' }
        }
    } catch { }

    # storage
    $fd = Get-FiveMData
    if ($fd) {
        $m = Get-DriveMedia $fd
        $st = $(if ($m -eq 'HDD') { 'warn' } elseif ($m -eq 'ไม่ทราบ') { 'info' } else { 'ok' })
        Add 'fivemdisk' 'ไดรฟ์ที่ลง FiveM' $st "ไดรฟ์ $(([IO.Path]::GetPathRoot($fd)).TrimEnd('\')) เป็น $m" $(if ($m -eq 'HDD') { 'ย้าย FiveM ไปลงใน SSD จะโหลดเร็วขึ้นและกระตุกน้อยลงมาก' } else { '' })
        $ini = Join-Path (Split-Path $fd -Parent) 'CitizenFX.ini'
        if (Test-Path $ini) {
            $line = Select-String -Path $ini -Pattern '^IVPath=(.+)$' | Select-Object -First 1
            if ($line) {
                $gta = $line.Matches[0].Groups[1].Value.Trim()
                $m2 = Get-DriveMedia $gta
                $st2 = $(if ($m2 -eq 'HDD') { 'warn' } elseif ($m2 -eq 'ไม่ทราบ') { 'info' } else { 'ok' })
                Add 'gtadisk' 'ไดรฟ์ที่ลง GTA V' $st2 "$gta ($m2)" $(if ($m2 -eq 'HDD') { 'GTA V อยู่บน HDD ทำให้แมพโหลดช้าและกระตุกตอนขับรถเร็ว ควรย้ายไป SSD' } else { '' })
            }
        }
    }

    # free space on system drive
    try {
        $c = Get-CimInstance Win32_LogicalDisk -Filter "DeviceID='$env:SystemDrive'"
        $pct = [math]::Round($c.FreeSpace / $c.Size * 100); $gb = [math]::Round($c.FreeSpace / 1GB)
        if ($pct -lt 10) { Add 'space' "พื้นที่ว่างไดรฟ์ $env:SystemDrive" 'warn' "เหลือ $gb GB ($pct%)" 'ลบไฟล์ที่ไม่ใช้ หรือกดล้างไฟล์ขยะในหน้าเครื่องมือ พื้นที่ใกล้เต็มทำให้ Windows ช้า' }
        else { Add 'space' "พื้นที่ว่างไดรฟ์ $env:SystemDrive" 'ok' "เหลือ $gb GB ($pct%)" '' }
    } catch { }

    # battery
    try {
        $bat = Get-CimInstance Win32_Battery -EA 0 | Select-Object -First 1
        if ($bat) {
            if ($bat.BatteryStatus -eq 1) { Add 'power' 'แหล่งไฟ' 'warn' 'กำลังใช้แบตเตอรี่' 'เสียบสายชาร์จตอนเล่น โน้ตบุ๊กจะลดความเร็วการ์ดจอกับ CPU ตอนใช้แบต' }
            else { Add 'power' 'แหล่งไฟ' 'ok' 'เสียบสายชาร์จอยู่' '' }
        }
    } catch { }

    # Memory Integrity (Core Isolation)
    try {
        $dg = Get-CimInstance -Namespace root\Microsoft\Windows\DeviceGuard -ClassName Win32_DeviceGuard -ErrorAction Stop
        $hvci = @($dg.SecurityServicesRunning) -contains 2
        $locked = (Get-Reg $KHV 'Locked') -eq 1
        if ($hvci) { Add 'hvci' 'Memory Integrity (Core Isolation)' 'warn' 'เปิดอยู่ บางเครื่องเสีย FPS 5-15%' $(if ($locked) { 'ถูกล็อกไว้จาก UEFI/องค์กร ต้องปิดใน Windows Security > Device security > Core isolation' } else { 'ปิดได้ในหน้าปรับแต่ง "ปิด Memory Integrity" แล้วรีสตาร์ท (ความปลอดภัยของเครื่องจะลดลงเล็กน้อย)' }) }
        else { Add 'hvci' 'Memory Integrity (Core Isolation)' 'ok' 'ปิดอยู่ ไม่กิน FPS' '' }
    } catch { }

    # overlay ที่ทำให้เฟรมสะดุด
    try {
        $names = @(Get-Process -EA 0 | ForEach-Object { $_.ProcessName.ToLower() })
        $ov = [Collections.Generic.List[string]]::new(); $tips = [Collections.Generic.List[string]]::new()
        if ($names -contains 'nvidia overlay' -or $names -contains 'nvidia share') { $ov.Add('NVIDIA Overlay'); $tips.Add('NVIDIA app/GeForce Experience > ปิด In-game overlay') }
        $od = @(Get-OvDisc); $au = @($od | Where-Object { $_.on.Count } | ForEach-Object { $_.n }); $vf = @(Get-OvVdf)
        if (@($vf | Where-Object { (Get-OvVal $_) -ne '0' }).Count) { $au += 'Steam' }
        $au | ForEach-Object { $ov.Add($_) }
        if ($names -contains 'radeonsoftware' -or $names -contains 'amdrsserv') { $ov.Add('AMD Adrenalin'); $tips.Add('Adrenalin > Preferences > ปิด In-Game Overlay') }
        if ((Get-Reg $KDV 'AppCaptureEnabled') -ne 0) { $ov.Add('Xbox Game Bar'); $tips.Add('หน้าปรับแต่ง > ปิดการอัดคลิปของ Xbox Game Bar') }
        if ($ov.Count) { Add 'overlay' 'Overlay ในเกม' 'info' ($ov -join ', ') (($tips -join ' · ') + ' (ปิดแค่ overlay ที่ขึ้นทับในเกม ตัวโปรแกรมยังเปิดคุยและแชร์จอได้ตามปกติ)'); if ($au.Count) { $r[$r.Count - 1].act = 'ovoff'; $r[$r.Count - 1].actTxt = 'ปิดให้เลย' } }
        $oz = @($od | Where-Object { $_.off.Count -and -not $_.on.Count } | ForEach-Object { $_.n }); if ((Test-Path $OvBak) -and $au -notcontains 'Steam') { $oz += 'Steam' }
        if (-not $ov.Count -and -not $oz.Count) { Add 'overlay' 'Overlay' 'ok' 'ไม่เจอ overlay ที่เปิดอยู่' '' }
        if ($oz.Count) { Add 'ovundo' 'Overlay ปิดไว้แล้ว' 'ok' ($oz -join ', ') ''; $r[$r.Count - 1].act = 'ovundo'; $r[$r.Count - 1].actTxt = 'เปิดกลับ' }
    } catch { }

    # power plan
    $act = Get-ActiveScheme
    if ($act -eq $SchemeGuid -or $act -eq $UltimateGuid -or $act -eq $HighPerfGuid) { Add 'plan' 'Power Plan' 'ok' $(if ($act -eq $SchemeGuid) { 'ใช้ ZAN Project Optimizer อยู่' } else { 'ใช้โหมดประสิทธิภาพสูงอยู่' }) '' }
    else { Add 'plan' 'Power Plan' 'warn' 'ยังใช้ Power Plan แบบประหยัด/สมดุล' 'เปิด "Power Plan: ZAN Project Optimizer" ในหน้าปรับแต่ง หรือกดบูสต์เครื่อง' }

    try {
        $a = Get-Audio
        if ($a.sv.Count) { Add 'audio' 'เสียง' 'warn' "บริการเสียงไม่ทำงาน: $($a.sv -join ', ')" 'กดซ่อมเสียง' }
        elseif (-not $a.out.Count) { Add 'audio' 'เสียง' 'warn' 'ไม่เจอลำโพงหรือหูฟังที่ใช้งานได้' 'เช็กสาย แล้วกดซ่อมเสียง' }
        else { Add 'audio' 'เสียง' 'ok' ($a.out -join ', ') '' }
        $r[$r.Count - 1].act = 'audio'; $r[$r.Count - 1].actTxt = 'ซ่อมเสียง'
    } catch { }
    Emit @{ t = 'result'; action = 'Health'; items = $r.ToArray() }
}

# ------------------------------------------------------- pre-game ----
$KnownApps = [ordered]@{
    chrome = 'Google Chrome'; msedge = 'Microsoft Edge'; firefox = 'Firefox'; opera = 'Opera'; brave = 'Brave'
    steamwebhelper = 'Steam (หน้าต่างเว็บ)'; EpicGamesLauncher = 'Epic Games Launcher'
    OneDrive = 'OneDrive'; Teams = 'Microsoft Teams'; 'ms-teams' = 'Microsoft Teams'; Spotify = 'Spotify'
    'Adobe Desktop Service' = 'Adobe Creative Cloud'; 'Creative Cloud' = 'Adobe Creative Cloud'; 'Battle.net' = 'Battle.net'
    EADesktop = 'EA App'; 'GalaxyClient' = 'GOG Galaxy'; Origin = 'Origin'; 'LeagueClient' = 'League of Legends Client'
    'RiotClientServices' = 'Riot Client'; uTorrent = 'uTorrent'; qbittorrent = 'qBittorrent'; IDMan = 'Internet Download Manager'
    'WallpaperEngine' = 'Wallpaper Engine'; wallpaper32 = 'Wallpaper Engine'; wallpaper64 = 'Wallpaper Engine'
}

# ======================================================== สแกนหาตัวดึง FPS ====
# อ่านอย่างเดียว ไม่แก้อะไร · แต่ละข้อ: status = ok | warn | heavy | unknown (ตรวจไม่ได้)
$FpsBackupFile = Join-Path $DataDir 'fpsfix.json'
function Get-FpsBackup { if (Test-Path $FpsBackupFile) { try { return (Get-Content $FpsBackupFile -Raw -Encoding UTF8 | ConvertFrom-Json) } catch { } }; return $null }
function Save-FpsBackup($id, $val) {
    $b = @{}; $o = Get-FpsBackup
    if ($o) { $o.PSObject.Properties | ForEach-Object { $b[$_.Name] = $_.Value } }
    if (-not $b.ContainsKey($id)) { $b[$id] = $val } # เก็บค่าแรกสุดไว้ (กดแก้ซ้ำไม่ทับค่าเดิม)
    $b | ConvertTo-Json -Depth 6 | Set-Content $FpsBackupFile -Encoding UTF8
}
function Remove-FpsBackup($id) {
    $o = Get-FpsBackup; if (-not $o) { return }
    $b = @{}; $o.PSObject.Properties | Where-Object { $_.Name -ne $id } | ForEach-Object { $b[$_.Name] = $_.Value }
    $b | ConvertTo-Json -Depth 6 | Set-Content $FpsBackupFile -Encoding UTF8
}

$Script:FpsProtect = @('idle','system','registry','smss','csrss','wininit','winlogon','services','lsass','lsaiso','svchost','dwm','explorer','fontdrvhost','memory compression','sihost','ctfmon','runtimebroker','startmenuexperiencehost','shellexperiencehost','searchhost','searchapp','textinputhost','audiodg','msmpeng','nissrv','securityhealthservice','securityhealthsystray','spoolsv','conhost','dllhost','taskhostw','wudfhost','msedgewebview2','powershell','pwsh','zanbooster','fivem','gta5','gtavlauncher','nvcontainer','nvdisplay.container','atiesrxx','atieclxx','amdfendrsr','wmiprvse','smartscreen','applicationframehost','systemsettings','lockapp','userinit','vmmem','vmcompute')
function Test-FpsProtected([string]$n) { $l = $n.ToLower(); if ($Script:FpsProtect -contains $l) { return $true }; return ($l -like 'fivem*' -or $l -like 'gta5*' -or $l -like 'zanbooster*') }

function Do-FpsScan {
    $tHDD = 'เกมอยู่บน SSD หรือ HDD'
    $tMAX = 'ความเร็ว CPU สูงสุด'
    $tCPU = 'โปรแกรมเบื้องหลังแย่ง CPU'
    $r = [Collections.Generic.List[object]]::new()
    $bk = Get-FpsBackup
    function Add($cat, $id, $title, $status, $detail, $fix, $act = '', $actTxt = '', $guide = '', $restart = $false, $extra = $null) {
        $it = [ordered]@{ cat = $cat; id = $id; title = $title; status = $status; detail = $detail; fix = $fix; act = $act; actTxt = $actTxt; guide = $guide; restart = $restart; undo = [bool]($bk -and $bk.PSObject.Properties[$id]) }
        if ($extra) { $it.extra = $extra }
        $r.Add($it)
    }
    function Unknown($cat, $id, $title) { Add $cat $id $title 'unknown' 'ตรวจไม่ได้' '' }
    $lostGB = 0.0
    try { # แต่ละหมวดแยก try: เครื่องไหนอ่านค่าบางข้อไม่ได้ ข้อที่เหลือยังขึ้นครบ (ไม่ขึ้น 'สแกนไม่สำเร็จ')

    # ---------- แรม ----------
    $mods = $null; $os = $null; $vcs = @()
    try { $mods = @(Get-CimInstance Win32_PhysicalMemory -ErrorAction Stop) } catch { }
    try { $os = Get-CimInstance Win32_OperatingSystem -ErrorAction Stop } catch { }
    try { $vcs = @(Get-CimInstance Win32_VideoController -ErrorAction Stop) } catch { }
    $igpuRx = 'AMD Radeon\(TM\) Graphics|^AMD Radeon Graphics|Radeon\(TM\) Vega|Radeon Vega|Intel\(R\) (UHD|Iris|HD) Graphics|Intel\(R\) Graphics|Radeon\(TM\) [0-9]{3}M'
    $dgpuRx = 'NVIDIA|Radeon RX|Radeon\(TM\) RX|Radeon Pro|Intel\(R\) Arc'
    $igpus = @($vcs | Where-Object { $_.Name -match $igpuRx -and $_.Name -notmatch $dgpuRx })
    $dgpus = @($vcs | Where-Object { $_.Name -match $dgpuRx })
    $bcd = $null
    try {
        $out = & bcdedit.exe /enum '{current}' 2>$null
        if ($LASTEXITCODE -eq 0 -and $out) { $bcd = @{}; foreach ($ln in $out) { if ($ln -match '^\s*(truncatememory|removememory|numproc)\s+(\S+)') { $bcd[$Matches[1]] = $Matches[2] } } }
    } catch { }

    if ($mods -and $os) {
        $inst = ($mods | Measure-Object Capacity -Sum).Sum / 1GB
        $vis = [double]$os.TotalVisibleMemorySize / 1MB
        $res = [math]::Round($inst - $vis, 1)
        if ($res -gt 0.5) {
            $lostGB = $res
            $cause = @()
            if ($bcd -and $bcd.Count) { $cause += 'ถูกจำกัดไว้ใน msconfig' }
            if ($igpus.Count -and $dgpus.Count) { $cause += 'การ์ดจอออนบอร์ดจองไว้' } elseif ($igpus.Count) { $cause += 'การ์ดจอออนบอร์ดใช้เป็นแรมการ์ดจอ' }
            $why = $(if ($cause.Count) { ' (' + ($cause -join ', ') + ')' } else { '' })
            Add 'ram' 'reserved' 'แรมหายไป' $(if ($res -ge 1.5) { 'heavy' } else { 'warn' }) ("ติดตั้ง $([math]::Round($inst)) GB แต่ Windows ใช้ได้แค่ $([math]::Round($vis,1)) GB หายไป $res GB" + $why) $(if ($cause.Count) { 'ดูรายการด้านล่างว่าแก้ยังไง' } elseif ($dgpus.Count) { 'เมนบอร์ดจองไว้ ลองดูวิธีปิดการ์ดจอออนบอร์ดในไบออส' } else { 'ส่วนใหญ่มาจากการ์ดจอออนบอร์ด ดูวิธีลดในไบออส' }) '' '' 'igpu'
        } else { Add 'ram' 'reserved' 'แรมหายไป' 'ok' "ใช้ได้เต็ม $([math]::Round($vis,1)) จาก $([math]::Round($inst)) GB" '' }
    } else { Unknown 'ram' 'reserved' 'แรมหายไป' }

    if ($vcs.Count) {
        if ($igpus.Count -and $dgpus.Count) {
            $ig = $igpus[0].Name.Trim()
            $st = $(if ($igpus[0].Status -eq 'OK' -or $igpus[0].ConfigManagerErrorCode -eq 0) { $(if ($lostGB -ge 1) { 'heavy' } else { 'warn' }) } else { 'ok' })
            if ($st -eq 'ok') { Add 'ram' 'igpu' 'การ์ดจอออนบอร์ด' 'ok' "$ig ปิดอยู่" '' }
            else { Add 'ram' 'igpu' 'การ์ดจอออนบอร์ดเปิดซ้อนกับการ์ดจอแยก' $st $(if ($lostGB -gt 0.5) { "$ig จองแรมไว้ราว $lostGB GB" } else { "$ig ทำงานอยู่ด้วย ทั้งที่มี $($dgpus[0].Name.Trim())" }) 'ปิดการ์ดจอออนบอร์ด หรือลดแรมที่มันจอง (UMA Frame Buffer) ในไบออส' '' '' 'igpu' }
        } elseif ($igpus.Count) { Add 'ram' 'igpu' 'การ์ดจอออนบอร์ด' 'ok' 'เครื่องนี้ใช้การ์ดจอออนบอร์ดอย่างเดียว ต้องแบ่งแรมให้การ์ดจอเป็นปกติ' '' }
        else { Add 'ram' 'igpu' 'การ์ดจอออนบอร์ด' 'ok' 'ไม่มี หรือปิดอยู่' '' }
    } else { Unknown 'ram' 'igpu' 'การ์ดจอออนบอร์ด' }

    if ($bcd -ne $null) {
        if ($bcd.Count) {
            $d = ($bcd.Keys | ForEach-Object { $(switch ($_) { 'truncatememory' { 'จำกัดแรม' } 'removememory' { 'ตัดแรมออก' } 'numproc' { "จำกัดคอร์ CPU ไว้ $($bcd[$_]) คอร์" } }) }) -join ' · '
            Add 'ram' 'bcd' 'ตั้งจำกัดแรม/CPU ไว้ใน msconfig' 'heavy' $d 'กดแก้ แล้วรีสตาร์ทเครื่อง' 'bcd' 'แก้ให้เลย' '' $true
        } else { Add 'ram' 'bcd' 'จำกัดแรม/CPU ใน msconfig' 'ok' 'ไม่ได้จำกัดไว้' '' }
    } else { Unknown 'ram' 'bcd' 'จำกัดแรม/CPU ใน msconfig' }

    if ($mods) {
        $cfg = ($mods | Measure-Object ConfiguredClockSpeed -Maximum).Maximum
        $rated = ($mods | Measure-Object Speed -Minimum).Minimum
        $ddr = switch (($mods | Select-Object -First 1).SMBIOSMemoryType) { 26 { 'DDR4' } 34 { 'DDR5' } 24 { 'DDR3' } default { 'DDR' } }
        $pnSpeed = Get-KitSpeed $mods
        $sure = ($rated -gt $cfg + 100) -or ($pnSpeed -gt $cfg + 100)
        $lowStd = ($ddr -eq 'DDR4' -and $cfg -le 2666) -or ($ddr -eq 'DDR5' -and $cfg -le 4800)
        if (-not $cfg) { Unknown 'ram' 'xmp' 'ความเร็วแรม (XMP/EXPO)' }
        elseif ($sure) { Add 'ram' 'xmp' 'แรมวิ่งไม่เต็มสเปก' 'heavy' "แรม $ddr วิ่ง $cfg MHz แต่รองรับ $([math]::Max($rated,$pnSpeed)) MHz" 'เปิด XMP / EXPO ในไบออส ข้อนี้ช่วย FPS ใน FiveM ได้มากที่สุด' '' '' 'xmp' }
        elseif ($lowStd) { Add 'ram' 'xmp' 'แรมวิ่งความเร็วพื้นฐาน' 'warn' "แรม $ddr วิ่ง $cfg MHz อาจยังไม่ได้เปิด XMP / EXPO" 'ถ้าแรมที่ซื้อมาเร็วกว่านี้ (ดูที่สติกเกอร์บนแรม) ให้เปิด XMP / EXPO ในไบออส' '' '' 'xmp' }
        else { Add 'ram' 'xmp' 'ความเร็วแรม' 'ok' "$ddr $cfg MHz" '' }

        $chan = @($mods | ForEach-Object { $s = "$($_.BankLabel) $($_.DeviceLocator)"; if ($s -match 'Channel\s*([A-D])|CHANNEL\s*([A-D])|DIMM_?([A-D])\d|\b([A-D])[12]\b') { ($Matches[1..4] | Where-Object { $_ } | Select-Object -First 1).ToUpper() } else { '?' } })
        $laptop = $false; try { $laptop = [bool](Get-CimInstance Win32_Battery -EA 0) } catch { }
        $sizes = @($mods | ForEach-Object { [math]::Round($_.Capacity / 1GB) }); $txt = ($sizes -join ' + ')
        if ($mods.Count -eq 1) { Add 'ram' 'chan' 'แรมแผงเดียว (Single channel)' $(if ($laptop) { 'warn' } else { 'heavy' }) "ใส่แรมแผงเดียว $txt GB" 'ใช้ 2 แผงขนาดเท่ากัน เสียบช่อง A2 กับ B2 (ช่องที่ 2 กับ 4 นับจาก CPU) FPS ใน FiveM ขึ้นชัด' '' '' 'chan' }
        elseif (($chan | Where-Object { $_ -ne '?' } | Select-Object -Unique).Count -eq 1 -and ($chan -notcontains '?')) { Add 'ram' 'chan' 'เสียบแรมผิดช่อง' 'heavy' "แรม $($mods.Count) แผงอยู่ช่อง $($chan[0]) เหมือนกัน ทำงานแบบ Single channel" 'ย้ายแรมไปเสียบช่อง A2 กับ B2 (ดูตัวหนังสือบนเมนบอร์ดข้างช่องแรม)' '' '' 'chan' }
        elseif (($sizes | Select-Object -Unique).Count -gt 1) { Add 'ram' 'chan' 'แรมคนละขนาด' 'warn' "$txt GB ส่วนที่เกินทำงานแบบ Single channel" 'ถ้าจะอัปเกรด ใช้แรมขนาดเท่ากันทุกแผง' }
        else { Add 'ram' 'chan' 'แรม Dual channel' 'ok' "$($mods.Count) แผง ($txt GB)" '' }
    } else { Unknown 'ram' 'xmp' 'ความเร็วแรม (XMP/EXPO)'; Unknown 'ram' 'chan' 'แรม Dual channel' }

    if ($os) {
        $tot = [double]$os.TotalVisibleMemorySize; $free = [double]$os.FreePhysicalMemory
        $pct = [math]::Round((1 - $free / $tot) * 100)
        if ($pct -ge 80) {
            $top = @(Get-Process -EA 0 | Where-Object { -not (Test-FpsProtected $_.ProcessName) -and $_.Id -ne $PID } | Group-Object ProcessName | ForEach-Object {
                $mb = [math]::Round((($_.Group | Measure-Object WorkingSet64 -Sum).Sum) / 1MB)
                $p1 = $_.Group | Select-Object -First 1; $path = ''; try { $path = $p1.Path } catch { }
                [pscustomobject]@{ name = $_.Name; mb = $mb; path = "$path" } } | Sort-Object mb -Descending | Select-Object -First 5)
            Add 'ram' 'ramfull' 'แรมใกล้เต็ม' $(if ($pct -ge 90) { 'heavy' } else { 'warn' }) ("ใช้ไป $pct% · ที่กินเยอะ: " + (($top | ForEach-Object { "$($_.name) $($_.mb) MB" }) -join ', ')) 'ปิดโปรแกรมที่ไม่ได้ใช้ก่อนเข้าเกม' 'kill' 'เลือกปิด' '' $false @{ procs = $top }
        } else { Add 'ram' 'ramfull' 'แรมที่ใช้อยู่' 'ok' "ใช้ไป $pct%" '' }
        $vt = [double]$os.TotalVirtualMemorySize; $vf = [double]$os.FreeVirtualMemory
        $cs = $null; try { $cs = Get-CimInstance Win32_ComputerSystem -ErrorAction Stop } catch { }
        $pf = @(); try { $pf = @(Get-CimInstance Win32_PageFileUsage -ErrorAction Stop) } catch { }
        if ($cs -and -not $cs.AutomaticManagedPagefile -and $pf.Count -eq 0) { Add 'ram' 'pagefile' 'ปิด Pagefile ไว้' 'heavy' 'ไม่มี Pagefile เลย FiveM แครชหรือกระตุกตอนแรมเต็ม' 'เปิดให้ Windows จัดการขนาดเอง แล้วรีสตาร์ท' 'pagefile' 'แก้ให้เลย' '' $true }
        elseif ($vt -gt 0) {
            $cp = [math]::Round((1 - $vf / $vt) * 100)
            if ($cp -ge 85) { Add 'ram' 'pagefile' 'หน่วยความจำรวมใกล้เต็ม (Commit)' $(if ($cp -ge 95) { 'heavy' } else { 'warn' }) "ใช้ไป $cp% ของแรม+Pagefile" $(if ($cs -and -not $cs.AutomaticManagedPagefile) { 'Pagefile ถูกตั้งขนาดเองไว้ กดแก้ให้ Windows จัดการเอง แล้วรีสตาร์ท' } else { 'ปิดโปรแกรมที่ไม่ใช้ หรือเพิ่มแรม' }) $(if ($cs -and -not $cs.AutomaticManagedPagefile) { 'pagefile' } else { '' }) 'แก้ให้เลย' '' ([bool]($cs -and -not $cs.AutomaticManagedPagefile)) }
            else { Add 'ram' 'pagefile' 'Pagefile / Commit' 'ok' "ใช้ไป $cp%" '' }
        } else { Unknown 'ram' 'pagefile' 'Pagefile / Commit' }
    } else { Unknown 'ram' 'ramfull' 'แรมที่ใช้อยู่'; Unknown 'ram' 'pagefile' 'Pagefile / Commit' }

    } catch { Add 'ram' 'scanerr' 'ตรวจแรมบางข้อไม่ได้' 'unknown' "$($_.Exception.Message)" '' }

    # ---------- การ์ดจอ ----------
    try {
    $gpu = $dgpus | Select-Object -First 1
    if ($gpu -and $gpu.DriverDate) {
        $days = [int]((Get-Date) - $gpu.DriverDate).TotalDays; $d = $gpu.DriverDate.ToString('dd/MM/yyyy')
        $url = $(if ($gpu.Name -match 'NVIDIA') { 'https://www.nvidia.com/Download/index.aspx' } elseif ($gpu.Name -match 'Radeon|AMD') { 'https://www.amd.com/en/support/download/drivers.html' } else { 'https://www.intel.com/content/www/us/en/download-center/home.html' })
        if ($days -gt 180) { Add 'gpu' 'driver' 'ไดรเวอร์การ์ดจอเก่า' 'warn' "ลงวันที่ $d (ราว $([math]::Round($days / 30)) เดือนก่อน)" 'โหลดตัวล่าสุดจากเว็บผู้ผลิต' ('url:' + $url) 'ไปเว็บโหลดไดรเวอร์' }
        else { Add 'gpu' 'driver' 'ไดรเวอร์การ์ดจอ' 'ok' "ลงวันที่ $d" '' }
    } elseif ($dgpus.Count) { Unknown 'gpu' 'driver' 'ไดรเวอร์การ์ดจอ' }

    } catch { Unknown 'gpu' 'driver' 'ไดรเวอร์การ์ดจอ' }

    # ---------- CPU / ระบบ ----------
    try {
        $n = [Environment]::ProcessorCount
        $s1 = @{}; Get-Process -EA 0 | ForEach-Object { try { $s1[$_.Id] = $_.TotalProcessorTime.TotalMilliseconds } catch { } }
        Start-Sleep -Milliseconds 1500
        $hogs = @(Get-Process -EA 0 | ForEach-Object {
            $t0 = $s1[$_.Id]; if ($t0 -eq $null) { return }; $t1 = 0; try { $t1 = $_.TotalProcessorTime.TotalMilliseconds } catch { return }
            $pc = ($t1 - $t0) / 1500 / $n * 100
            if ($pc -ge 4 -and $_.ProcessName -notmatch '^(Idle|System|FiveM|GTA5|ZanBooster|powershell|msedgewebview2)') { [pscustomobject]@{ name = $_.ProcessName; pct = [math]::Round($pc) } } } | Sort-Object pct -Descending | Select-Object -First 5)
        $known = @{ 'tiworker' = 'Windows Update กำลังติดตั้ง'; 'trustedinstaller' = 'Windows Update กำลังติดตั้ง'; 'msmpeng' = 'Defender กำลังสแกน'; 'mpdefendercoreservice' = 'Defender กำลังสแกน'; 'searchindexer' = 'Windows กำลังทำดัชนีค้นหาไฟล์'; 'googleupdate' = 'ตัวอัปเดต Google'; 'microsoftedgeupdate' = 'ตัวอัปเดต Edge'; 'onedrive' = 'OneDrive กำลังซิงก์'; 'compattelrunner' = 'Windows เก็บข้อมูลการใช้งาน' }
        if ($hogs.Count) {
            $txt = ($hogs | ForEach-Object { $k = $known[$_.name.ToLower()]; "$(if ($k) { $k } else { $_.name }) $($_.pct)%" }) -join ', '
            $sum = ($hogs | Measure-Object pct -Sum).Sum
            Add 'sys' 'cpuhog' $tCPU $(if ($sum -ge 25) { 'heavy' } else { 'warn' }) $txt 'ถ้าเป็น Windows Update / Defender รอให้เสร็จก่อนเล่น ถ้าเป็นโปรแกรมอื่นปิดก่อนเข้าเกม'
        } else { Add 'sys' 'cpuhog' $tCPU 'ok' 'ไม่มีอะไรแย่ง CPU ตอนนี้' '' }
    } catch { Unknown 'sys' 'cpuhog' $tCPU }

    try {
        $names = @(Get-Process -EA 0 | ForEach-Object { $_.ProcessName.ToLower() })
        $ov = @(); $tips = @()
        $au = @(Get-OvDisc | Where-Object { $_.on.Count } | ForEach-Object { $_.n })
        if (@(Get-OvVdf | Where-Object { (Get-OvVal $_) -ne '0' }).Count) { $au += 'Steam' }
        $ov += $au
        if ($names -contains 'nvidia overlay' -or $names -contains 'nvidia share') { $ov += 'NVIDIA'; $tips += 'NVIDIA App > Settings > ปิด NVIDIA Overlay' }
        if ($names -contains 'radeonsoftware' -or $names -contains 'amdrsserv') { $ov += 'AMD'; $tips += 'Adrenalin > Settings > Preferences > ปิด In-Game Overlay' }
        if ($names -contains 'rtss') { $ov += 'RivaTuner (RTSS)'; $tips += 'ปิด RTSS ถ้าไม่ได้ใช้ล็อกเฟรม' }
        $gb = (Get-Reg $KDV 'AppCaptureEnabled') -ne 0
        if ($gb) { $ov += 'Xbox Game Bar' }
        if ($ov.Count) { Add 'sys' 'overlay' 'Overlay ที่ทับในเกม' 'warn' ($ov -join ', ') (($tips -join ' · ') + $(if ($gb) { ' · Game Bar กดแก้ให้ได้' } else { '' }) + ' (ปิดแค่ overlay ตัวโปรแกรมยังใช้คุย/แชร์จอได้)') $(if ($au.Count) { 'ovoff' } elseif ($gb) { 'gamebar' } else { '' }) $(if ($au.Count) { 'ปิดให้เลย' } else { 'ปิด Game Bar' }) }
        else { Add 'sys' 'overlay' 'Overlay ที่ทับในเกม' 'ok' 'ไม่เจอ' '' }
    } catch { Unknown 'sys' 'overlay' 'Overlay ที่ทับในเกม' }

    try {
        $dg = Get-CimInstance -Namespace root\Microsoft\Windows\DeviceGuard -ClassName Win32_DeviceGuard -ErrorAction Stop
        $hvci = @($dg.SecurityServicesRunning) -contains 2
        $locked = (Get-Reg $KHV 'Locked') -eq 1
        if ($hvci) { Add 'sys' 'vbs' 'Memory Integrity (VBS) เปิดอยู่' 'warn' 'บางเครื่องเสีย FPS 5-15%' $(if ($locked) { 'ถูกล็อกจาก UEFI ต้องปิดเองใน Windows Security > Device security > Core isolation' } else { 'ปิดแล้วต้องรีสตาร์ท ความปลอดภัยจากไดรเวอร์อันตรายจะลดลง' }) $(if ($locked) { '' } else { 'vbs' }) 'ปิดให้' '' $true }
        else { Add 'sys' 'vbs' 'Memory Integrity (VBS)' 'ok' 'ปิดอยู่' '' }
    } catch { Unknown 'sys' 'vbs' 'Memory Integrity (VBS)' }

    try {
        $act = Get-ActiveScheme
        $hasZan = [bool](& powercfg.exe /list 2>$null | Select-String $SchemeGuid)
        if ($act -eq $SchemeGuid) { Add 'sys' 'plan' 'แผนพลังงาน' 'ok' 'ใช้ ZAN Project Optimizer อยู่' '' }
        elseif ($hasZan) { Add 'sys' 'plan' 'แผนพลังงานไม่ใช่ ZAN' 'warn' 'ยังไม่ได้ใช้ ZAN Project Optimizer' 'กดสลับ มีผลทันที' 'plan' 'สลับให้เลย' }
        elseif ($act -eq $UltimateGuid -or $act -eq $HighPerfGuid) { Add 'sys' 'plan' 'แผนพลังงาน' 'ok' 'ใช้โหมดประสิทธิภาพสูงอยู่' '' }
        else { Add 'sys' 'plan' 'แผนพลังงานแบบประหยัด/สมดุล' 'warn' 'ยังไม่มีแผน ZAN Project Optimizer' 'กดบูสต์เครื่องในหน้าบูสต์ จะสร้างและเปิดแผนให้' }
    } catch { Unknown 'sys' 'plan' 'แผนพลังงาน' }

    try {
        $q = & powercfg.exe /query SCHEME_CURRENT SUB_PROCESSOR PROCTHROTTLEMAX 2>$null
        $hex = @([regex]::Matches(($q -join "`n"), '0x([0-9a-fA-F]{8})') | ForEach-Object { [Convert]::ToInt32($_.Groups[1].Value, 16) })
        if ($hex.Count -ge 2) {
            $ac = $hex[$hex.Count - 2]
            if ($ac -lt 100) { Add 'sys' 'cpumax' 'จำกัดความเร็ว CPU ไว้' 'heavy' "แผนพลังงานจำกัด CPU ไว้ที่ $ac%" 'กดแก้ให้ CPU วิ่งได้ 100%' 'cpumax' 'แก้ให้เลย' }
            else { Add 'sys' 'cpumax' $tMAX 'ok' 'ไม่ได้จำกัด (100%)' 'ถ้า FPS ตกกลางเกมทั้งที่ข้อนี้ผ่าน อาจเป็นเพราะร้อนเกิน ดูข้อความร้อนด้านล่าง' }
        } else { Unknown 'sys' 'cpumax' $tMAX }
    } catch { Unknown 'sys' 'cpumax' $tMAX }

    try {
        $st = @(Get-CimInstance Win32_StartupCommand -ErrorAction Stop | Where-Object { $_.Command -notmatch 'SecurityHealth|ZanBooster' })
        $nm = @($st | ForEach-Object { $_.Name } | Select-Object -Unique)
        if ($nm.Count -gt 8) { Add 'sys' 'startup' 'โปรแกรมเปิดตอนสตาร์ทเครื่องเยอะ' 'warn' ("$($nm.Count) โปรแกรม: " + (($nm | Select-Object -First 6) -join ', ') + $(if ($nm.Count -gt 6) { ' ...' } else { '' })) 'ปิดตัวที่ไม่ต้องใช้ใน Task Manager > Startup apps' 'taskmgr' 'เปิด Startup apps' }
        else { Add 'sys' 'startup' 'โปรแกรมเปิดตอนสตาร์ทเครื่อง' 'ok' "$($nm.Count) โปรแกรม" '' }
    } catch { Unknown 'sys' 'startup' 'โปรแกรมเปิดตอนสตาร์ทเครื่อง' }

    # ---------- ดิสก์ ----------
    try {
    $fd = $null; try { $fd = Get-FiveMData } catch { }
    if ($fd) {
        $paths = @(@{ n = 'FiveM'; p = $fd })
        $ini = Join-Path (Split-Path $fd -Parent) 'CitizenFX.ini'
        if (Test-Path $ini) { $line = Select-String -Path $ini -Pattern '^IVPath=(.+)$' | Select-Object -First 1; if ($line) { $paths += @{ n = 'GTA V'; p = $line.Matches[0].Groups[1].Value.Trim() } } }
        $hdd = @(); $unk = $false
        foreach ($x in $paths) { $m = Get-DriveMedia $x.p; if ($m -eq 'HDD') { $hdd += "$($x.n) ($([IO.Path]::GetPathRoot($x.p).TrimEnd('\')))" } elseif ($m -eq 'ไม่ทราบ') { $unk = $true } }
        if ($hdd.Count) { Add 'disk' 'hdd' 'เกมอยู่บนฮาร์ดดิสก์ (HDD)' 'heavy' ($hdd -join ', ') 'ย้ายไปลง SSD แมพโหลดเร็วขึ้น กระตุกตอนขับรถเร็วน้อยลงมาก' }
        elseif ($unk) { Unknown 'disk' 'hdd' $tHDD }
        else { Add 'disk' 'hdd' 'เกมอยู่บน SSD' 'ok' (($paths | ForEach-Object { $_.n }) -join ' + ') '' }
        $low = @()
        foreach ($root in @($paths | ForEach-Object { [IO.Path]::GetPathRoot($_.p) } | Select-Object -Unique)) {
            try { $di = New-Object IO.DriveInfo $root; $pct = [math]::Round($di.AvailableFreeSpace / $di.TotalSize * 100); if ($pct -lt 10) { $low += "$($root.TrimEnd('\')) เหลือ $([math]::Round($di.AvailableFreeSpace / 1GB)) GB ($pct%)" } } catch { }
        }
        if ($low.Count) { Add 'disk' 'space' 'ไดรฟ์ที่ลงเกมพื้นที่ใกล้เต็ม' 'warn' ($low -join ', ') 'ลบไฟล์ที่ไม่ใช้ หรือกดล้างไฟล์ขยะในหน้าเครื่องมือ ให้เหลือเกิน 10%' }
        else { Add 'disk' 'space' 'พื้นที่ไดรฟ์ที่ลงเกม' 'ok' 'เหลือเกิน 10%' '' }
    } else { Unknown 'disk' 'hdd' $tHDD; Unknown 'disk' 'space' 'พื้นที่ไดรฟ์ที่ลงเกม' }

    } catch { Unknown 'disk' 'hdd' $tHDD }

    Emit @{ t = 'result'; action = 'FpsScan'; items = $r.ToArray(); lostGB = $lostGB }
}

# แก้ตามที่ผู้ใช้กด (เก็บค่าเดิมก่อนทุกครั้ง) $Ids = รหัสข้อ, $Opt = ค่าเสริม
function Do-FpsFix {
    $id = $Ids; $restart = $false; $m = ''
    switch ($id) {
        'bcd' {
            $out = & bcdedit.exe /enum '{current}' 2>$null; $old = @{}
            foreach ($ln in $out) { if ($ln -match '^\s*(truncatememory|removememory|numproc)\s+(\S+)') { $old[$Matches[1]] = $Matches[2] } }
            Save-FpsBackup 'bcd' $old
            foreach ($k in $old.Keys) { & bcdedit.exe /deletevalue '{current}' $k | Out-Null }
            $m = 'ลบค่าจำกัดแรม/CPU แล้ว'; $restart = $true
        }
        'pagefile' {
            $cs = Get-CimInstance Win32_ComputerSystem
            Save-FpsBackup 'pagefile' @{ auto = [bool]$cs.AutomaticManagedPagefile; files = @((Get-ItemProperty 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager\Memory Management' -Name PagingFiles -EA 0).PagingFiles) }
            Set-CimInstance -InputObject $cs -Property @{ AutomaticManagedPagefile = $true } | Out-Null
            $m = 'ให้ Windows จัดการ Pagefile เองแล้ว'; $restart = $true
        }
        'vbs' {
            $p = $KHV
            Save-FpsBackup 'vbs' @{ enabled = (Get-Reg $p 'Enabled') }
            Set-RegValue $p 'Enabled' 0
            $m = 'ปิด Memory Integrity แล้ว'; $restart = $true
        }
        'plan' {
            Save-FpsBackup 'plan' @{ guid = (Get-ActiveScheme) }
            & powercfg.exe /setactive $SchemeGuid | Out-Null
            $m = 'สลับเป็น ZAN Project Optimizer แล้ว'
        }
        'cpumax' {
            $q = & powercfg.exe /query SCHEME_CURRENT SUB_PROCESSOR PROCTHROTTLEMAX 2>$null
            $hex = @([regex]::Matches(($q -join "`n"), '0x([0-9a-fA-F]{8})') | ForEach-Object { [Convert]::ToInt32($_.Groups[1].Value, 16) })
            Save-FpsBackup 'cpumax' @{ guid = (Get-ActiveScheme); ac = $hex[$hex.Count - 2]; dc = $hex[$hex.Count - 1] }
            & powercfg.exe /setacvalueindex SCHEME_CURRENT SUB_PROCESSOR PROCTHROTTLEMAX 100 | Out-Null
            & powercfg.exe /setactive SCHEME_CURRENT | Out-Null
            $m = 'ให้ CPU วิ่งได้ 100% แล้ว (ตอนเสียบปลั๊ก)'
        }
        'gamebar' {
            $p = $KDV; $p2 = $KGC
            Save-FpsBackup 'gamebar' @{ cap = (Get-Reg $p 'AppCaptureEnabled'); dvr = (Get-Reg $p2 'GameDVR_Enabled') }
            Set-RegValue $p 'AppCaptureEnabled' 0; Set-RegValue $p2 'GameDVR_Enabled' 0
            $m = 'ปิดการอัด/overlay ของ Xbox Game Bar แล้ว'
        }
        'kill' {
            $names = @($Opt -split ',' | Where-Object { $_ }); $done = @(); $paths = @()
            foreach ($n in $names) {
                if (Test-FpsProtected $n) { continue }
                $ps = @(Get-Process -Name $n -EA 0); if (-not $ps.Count) { continue }
                $pth = ''; try { $pth = $ps[0].Path } catch { }
                foreach ($p in $ps) { try { [void]$p.CloseMainWindow() } catch { } }
                Start-Sleep -Milliseconds 1200
                Get-Process -Name $n -EA 0 | ForEach-Object { try { $_.Kill() } catch { } }
                $done += $n; if ($pth) { $paths += $pth }
            }
            $o = Get-FpsBackup; $prev = @(); if ($o -and $o.kill) { $prev = @($o.kill) }
            Remove-FpsBackup 'kill'; Save-FpsBackup 'kill' @(@($prev) + @($paths) | Select-Object -Unique)
            $m = $(if ($done.Count) { 'ปิดแล้ว: ' + ($done -join ', ') } else { 'ไม่มีโปรแกรมให้ปิด' })
        }
        default { Emit @{ t = 'result'; action = 'FpsFix'; ok = $false; m = 'ไม่รู้จักรายการนี้' }; return }
    }
    Write-Log "สแกนตัวดึง FPS: แก้ $id ($m)"
    Emit @{ t = 'result'; action = 'FpsFix'; ok = $true; m = $m; restart = $restart }
}

function Do-FpsUndo {
    $id = $Ids; $o = Get-FpsBackup
    if (-not $o -or -not $o.PSObject.Properties[$id]) { Emit @{ t = 'result'; action = 'FpsUndo'; ok = $false; m = 'ไม่มีค่าเดิมให้ย้อน' }; return }
    $v = $o.$id; $restart = $false
    switch ($id) {
        'bcd' { foreach ($p in $v.PSObject.Properties) { & bcdedit.exe /set '{current}' $p.Name $p.Value | Out-Null }; $restart = $true }
        'pagefile' {
            $cs = Get-CimInstance Win32_ComputerSystem
            Set-CimInstance -InputObject $cs -Property @{ AutomaticManagedPagefile = [bool]$v.auto } | Out-Null
            if (-not $v.auto) { Set-ItemProperty 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager\Memory Management' -Name PagingFiles -Value @($v.files) -Type MultiString }
            $restart = $true
        }
        'vbs' { $p = $KHV; if ($v.enabled -ne $null) { Set-RegValue $p 'Enabled' ([int]$v.enabled) } else { Remove-ItemProperty $p -Name Enabled -EA 0 }; $restart = $true }
        'plan' { if ($v.guid) { & powercfg.exe /setactive $v.guid | Out-Null } }
        'cpumax' { & powercfg.exe /setacvalueindex $v.guid SUB_PROCESSOR PROCTHROTTLEMAX ([int]$v.ac) | Out-Null; & powercfg.exe /setactive SCHEME_CURRENT | Out-Null }
        'gamebar' {
            $p = $KDV; $p2 = $KGC
            if ($v.cap -ne $null) { Set-RegValue $p 'AppCaptureEnabled' ([int]$v.cap) } else { Remove-ItemProperty $p -Name AppCaptureEnabled -EA 0 }
            if ($v.dvr -ne $null) { Set-RegValue $p2 'GameDVR_Enabled' ([int]$v.dvr) } else { Remove-ItemProperty $p2 -Name GameDVR_Enabled -EA 0 }
        }
        'kill' { foreach ($pth in @($v)) { if ($pth -and (Test-Path $pth)) { try { Start-Process -FilePath $pth } catch { } } } }
    }
    Remove-FpsBackup $id
    Write-Log "สแกนตัวดึง FPS: ย้อนกลับ $id"
    Emit @{ t = 'result'; action = 'FpsUndo'; ok = $true; restart = $restart }
}

function Do-ListApps {
    $procs = Get-Process -EA 0
    $out = @()
    foreach ($k in $KnownApps.Keys) {
        if (-not $KnownApps[$k]) { continue }
        $ps = @($procs | Where-Object { $_.ProcessName -ieq $k })
        if ($ps.Count -eq 0) { continue }
        $mb = [math]::Round((($ps | Measure-Object WorkingSet64 -Sum).Sum) / 1MB)
        $out += [ordered]@{ id = $k; name = $KnownApps[$k]; mb = $mb; count = $ps.Count }
    }
    Emit @{ t = 'result'; action = 'ListApps'; apps = @($out) }
}

function Do-CloseApps {
    $n = 0; $mb = 0
    foreach ($id in @($Ids -split ',' | Where-Object { $_ })) {
        $ps = @(Get-Process -Name $id -EA 0)
        if ($ps.Count -eq 0) { continue }
        $mb += [math]::Round((($ps | Measure-Object WorkingSet64 -Sum).Sum) / 1MB)
        foreach ($p in $ps) { try { [void]$p.CloseMainWindow() } catch { } }
        Start-Sleep -Milliseconds 1500
        Get-Process -Name $id -EA 0 | ForEach-Object { try { $_.Kill() } catch { } }
        Write-Log "ปิด $($KnownApps[$id])"
        $n++
    }
    ipconfig /flushdns | Out-Null
    Emit @{ t = 'result'; action = 'CloseApps'; ok = $true; n = $n; mb = $mb }
}

function Do-Report {
    # ทุกขั้นแยก try/catch: ขั้นไหนพัง จดลง errors.txt แล้วไปต่อ (รายงานยังส่งได้เสมอ)
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    $tmp = Join-Path $env:TEMP "ZanBooster-report-$stamp"
    New-Item -ItemType Directory -Path $tmp -Force | Out-Null
    $errs = [Collections.Generic.List[string]]::new()
    $send = ($Ids -eq 'send') # ส่งเข้า Discord: เก็บเฉพาะที่จำเป็น ไฟล์เล็ก
    try {
        $Script:Capture = [Collections.Generic.List[object]]::new()
        foreach ($fn in 'Do-SysInfo', 'Do-Status', 'Do-Health', 'Do-GfxPlan', 'Do-GfxStatus') { try { & $fn } catch { $errs.Add("$fn : $($_.Exception.Message)") } }
        $data = $Script:Capture.ToArray(); $Script:Capture = $null
        $osName = ''; try { $os = Get-CimInstance Win32_OperatingSystem; $osName = "$($os.Caption) $($os.Version)" } catch { }
        $report = [ordered]@{ created = (Get-Date).ToString('s'); app = "$Opt"; os = $osName; results = $data }
        try { $report | ConvertTo-Json -Depth 8 | Set-Content (Join-Path $tmp 'report.json') -Encoding UTF8 }
        catch { $errs.Add("report.json : $($_.Exception.Message)"); try { ($data | ForEach-Object { $_ | Out-String }) -join "`r`n" | Set-Content (Join-Path $tmp 'report.txt') -Encoding UTF8 } catch { } }
    } catch { $errs.Add("collect : $($_.Exception.Message)"); $Script:Capture = $null }
    $files = @('log.txt', 'backup.json', 'config.json'); if (-not $send) { $files += @('settings.json', 'remote.json') }
    foreach ($f in $files) { try { $src = Join-Path $DataDir $f; if (Test-Path $src) { Copy-Item -LiteralPath $src -Destination $tmp } } catch { $errs.Add("copy $f : $($_.Exception.Message)") } }
    try {
        $fd = Get-FiveMData
        if ($fd) {
            $app = Split-Path $fd -Parent
            $ini = Join-Path $app 'CitizenFX.ini'; if (Test-Path $ini) { Copy-Item -LiteralPath $ini -Destination $tmp }
            $logs = Join-Path $app 'logs'
            if (Test-Path $logs) {
                foreach ($lf in @(Get-ChildItem -LiteralPath $logs -File | Sort-Object LastWriteTime -Descending | Select-Object -First 2)) {
                    try {
                        $dst = Join-Path $tmp $lf.Name
                        if ($send -and $lf.Length -gt 409600) {
                            $fs = New-Object System.IO.FileStream($lf.FullName, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read, [System.IO.FileShare]::ReadWrite)
                            $out = $null
                            try {
                                $fs.Position = [long]($lf.Length - 409600)
                                $out = New-Object System.IO.FileStream($dst, [System.IO.FileMode]::Create, [System.IO.FileAccess]::Write)
                                $fs.CopyTo($out)
                            } finally { if ($out) { $out.Dispose() }; $fs.Dispose() }
                        } else { Copy-Item -LiteralPath $lf.FullName -Destination $tmp }
                    } catch { $errs.Add("fivem log $($lf.Name) : $($_.Exception.Message)") }
                }
            }
        }
    } catch { $errs.Add("fivem : $($_.Exception.Message)") }
    try { if (Test-Path $GfxFile) { Copy-Item -LiteralPath $GfxFile -Destination $tmp } } catch { $errs.Add("gfx : $($_.Exception.Message)") }
    if ($errs.Count) { try { $errs -join "`r`n" | Set-Content (Join-Path $tmp 'errors.txt') -Encoding UTF8 } catch { } }
    $zipDir = [Environment]::GetFolderPath('Desktop'); if ($send) { $zipDir = $env:TEMP }
    $zip = Join-Path $zipDir "ZanBooster-report-$stamp.zip"
    try {
        Add-Type -AssemblyName System.IO.Compression.FileSystem -EA 0
        if (Test-Path $zip) { Remove-Item $zip -Force }
        [System.IO.Compression.ZipFile]::CreateFromDirectory($tmp, $zip)
    } catch {
        try { Compress-Archive -Path (Join-Path $tmp '*') -DestinationPath $zip -Force } catch { Remove-Item $tmp -Recurse -Force -EA 0; throw "สร้างไฟล์รายงานไม่ได้: $($_.Exception.Message)" }
    }
    Remove-Item $tmp -Recurse -Force -EA 0
    if (-not $send) { Start-Process explorer.exe "/select,`"$zip`"" }
    Emit @{ t = 'result'; action = 'Report'; ok = $true; path = $zip }
}

function Do-Restore {
    $c = Get-Config; if ($c.affBoot) { $c | Add-Member affBoot $false -Force; Save-Config $c; Sync-AffBoot $false }
    if (-not (Test-HasBackup)) { Emit @{ t = 'result'; action = 'Restore'; ok = $false; m = 'ยังไม่มีค่าที่ต้องคืน' }; return }
    $b = $Script:Backup
    foreach ($e in @($b.Registry.Values)) {
        try {
            if ($e.Existed) {
                if (-not (Test-Path -LiteralPath $e.Path)) { New-Item -Path $e.Path -Force | Out-Null }
                $val = $e.Value
                if ($e.Type -eq 'Binary')      { $val = [byte[]]@($e.Value) }
                if ($e.Type -eq 'MultiString') { $val = [string[]]@($e.Value) }
                New-ItemProperty -LiteralPath $e.Path -Name $e.Name -Value $val -PropertyType $e.Type -Force | Out-Null
            } else {
                Remove-ItemProperty -LiteralPath $e.Path -Name $e.Name -EA 0
            }
        } catch { Write-Log "คืนค่า $($e.Path)\$($e.Name) ไม่ได้: $($_.Exception.Message)" }
    }
    Write-Log "คืนค่า Registry $($b.Registry.Count) รายการ"
    foreach ($k in $b.CreatedKeys) { if (Test-Path -LiteralPath $k) { Remove-Item -LiteralPath $k -Recurse -Force -EA 0 } }

    foreach ($kv in @($b.Powercfg.GetEnumerator())) {
        $parts = $kv.Key -split '\|'
        powercfg /setacvalueindex $parts[0] $parts[1] $parts[2] $kv.Value[0] | Out-Null
        powercfg /setdcvalueindex $parts[0] $parts[1] $parts[2] $kv.Value[1] | Out-Null
    }
    if ($b.PrevScheme) {
        powercfg /setactive $b.PrevScheme | Out-Null
        if ($LASTEXITCODE -ne 0) { powercfg /setactive $BalancedGuid | Out-Null }
        Write-Log 'คืน Power Plan เดิมแล้ว'
    }
    if (((powercfg /list) -join ' ') -match $SchemeGuid) {
        if ((Get-ActiveScheme) -eq $SchemeGuid) { powercfg /setactive $BalancedGuid | Out-Null }
        powercfg /delete $SchemeGuid | Out-Null
    }
    $restart = @()
    foreach ($kv in @($b.Adapters.GetEnumerator())) {
        $parts = $kv.Key -split '\|'
        try {
            Set-NetAdapterAdvancedProperty -Name $parts[0] -RegistryKeyword $parts[1] -RegistryValue $kv.Value -NoRestart -ErrorAction Stop
            if ($restart -notcontains $parts[0]) { $restart += $parts[0] }
        } catch { Write-Log "คืนค่าการ์ดแลน $($parts[0]) ไม่ได้" }
    }
    foreach ($n in $restart) { Restart-NetAdapter -Name $n -Confirm:$false -EA 0 }
    foreach ($p in @($b.Defender)) { Remove-MpPreference -ExclusionPath $p -EA 0; Write-Log "Defender กลับมาสแกน $p" }

    if ((Test-Path $GfxBackup) -and -not (Get-Process -EA 0 | Where-Object { $_.Name -like 'FiveM*' })) {
        Copy-Item $GfxBackup $GfxFile -Force; Remove-Item $GfxBackup -Force; Write-Log 'คืนไฟล์กราฟิก FiveM เดิมแล้ว'
    }
    try { Set-NuiGpu $false } catch { Write-Log $_.Exception.Message }
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    if (Test-Path $BackupFile) { Move-Item $BackupFile "$BackupFile.restored-$stamp" -Force }
    $Script:Backup = New-EmptyBackup
    Emit @{ t = 'result'; action = 'Restore'; ok = $true }
}

function Do-CleanCache {
    if (Get-Process -EA 0 | Where-Object { $_.Name -like 'FiveM*' }) {
        Emit @{ t = 'result'; action = 'CleanCache'; ok = $false; m = 'กรุณาปิด FiveM ก่อนล้างแคช' }; return
    }
    $FiveMData = Get-FiveMData
    if (-not $FiveMData) { Emit @{ t = 'result'; action = 'CleanCache'; ok = $false; notfound = $true; m = 'หาโฟลเดอร์ FiveM ไม่เจอ' }; return }
    Write-Log "FiveM: $(Split-Path (Split-Path $FiveMData -Parent) -Parent)"
    if (-not (Test-Path $FiveMData)) { Emit @{ t = 'result'; action = 'CleanCache'; ok = $true; mb = 0 }; return }
    $total = [int64]0
    foreach ($d in 'server-cache', 'server-cache-priv') {
        $p = Join-Path $FiveMData $d
        if (Test-Path $p) { $total += Get-FolderSize $p; Remove-Item $p -Recurse -Force -EA 0; Write-Log "ลบ $d" }
    }
    $cache = Join-Path $FiveMData 'cache'
    if (Test-Path $cache) {
        Get-ChildItem $cache -Force -EA 0 | Where-Object { $_.Name -ne 'game' } | ForEach-Object {
            $total += $(if ($_.PSIsContainer) { Get-FolderSize $_.FullName } else { $_.Length })
            Remove-Item $_.FullName -Recurse -Force -EA 0
        }
        Write-Log 'ลบ cache (เก็บ cache\game ไว้)'
    }
    foreach ($d in 'crashes', 'logs') {
        $p = Join-Path (Split-Path $FiveMData -Parent) $d
        if (Test-Path $p) { $total += Get-FolderSize $p; Get-ChildItem $p -Force | Remove-Item -Recurse -Force -EA 0 }
    }
    Emit @{ t = 'result'; action = 'CleanCache'; ok = $true; mb = [math]::Round($total / 1MB) }
}

function Do-PickFiveM {
    Add-Type -AssemblyName System.Windows.Forms
    [System.Windows.Forms.Application]::EnableVisualStyles()
    $owner = New-Object System.Windows.Forms.Form -Property @{ TopMost = $true; ShowInTaskbar = $false; WindowState = 'Minimized' }
    $dlg = New-Object System.Windows.Forms.FolderBrowserDialog
    $dlg.Description = 'เลือกโฟลเดอร์ที่ติดตั้ง FiveM (โฟลเดอร์ที่มี FiveM.exe และ FiveM.app)'
    $dlg.ShowNewFolderButton = $false
    $res = $dlg.ShowDialog($owner); $owner.Dispose()
    if ($res -ne 'OK') { Emit @{ t = 'result'; action = 'PickFiveM'; ok = $false; cancel = $true }; return }
    $app = Resolve-FiveMApp $dlg.SelectedPath
    if (-not $app) { Emit @{ t = 'result'; action = 'PickFiveM'; ok = $false; m = 'โฟลเดอร์นี้ไม่ใช่ FiveM ลองเลือกโฟลเดอร์ที่มีไฟล์ FiveM.exe' }; return }
    $cfg = Get-Config
    $cfg | Add-Member -NotePropertyName FiveMPath -NotePropertyValue $app -Force
    Save-Config $cfg
    Emit @{ t = 'result'; action = 'PickFiveM'; ok = $true; path = (Split-Path $app -Parent) }
}

function Do-CleanTemp {
    $total = [int64]0
    foreach ($p in $env:TEMP, (Join-Path $env:SystemRoot 'Temp')) {
        $total += Remove-DirContents $p
    }
    ipconfig /flushdns | Out-Null
    Emit @{ t = 'result'; action = 'CleanTemp'; ok = $true; mb = [math]::Round($total / 1MB) }
}

# ---------------------------------------------------------------- junk ----
# แอปที่เปิดอยู่ห้ามล้าง ลบได้แค่ครึ่งเดียวแล้วแคชพัง เปิดแอปครั้งหน้าจะค้าง
$Script:JunkSkip = @()
function Test-JunkFree([string]$n) {
    if (Get-Process -Name $n -EA 0) { if ($Script:JunkSkip -notcontains $n) { $Script:JunkSkip += $n }; return $false }
    $true
}
function Get-JunkTargets([string]$id) {
    $L = $env:LOCALAPPDATA; $R = $env:APPDATA; $W = $env:SystemRoot
    $dirs = @(); $files = @()
    switch ($id) {
        'temp'    { $dirs = @($env:TEMP, "$W\Temp") }
        'wu'      { $dirs = @("$W\SoftwareDistribution\Download") }
        'do'      { $dirs = @("$W\ServiceProfiles\NetworkService\AppData\Local\Microsoft\Windows\DeliveryOptimization\Cache") }
        'dumps'   {
            $dirs = @("$W\Minidump", "$L\CrashDumps", "$env:ProgramData\Microsoft\Windows\WER\ReportArchive", "$env:ProgramData\Microsoft\Windows\WER\ReportQueue", "$L\Microsoft\Windows\WER\ReportArchive", "$L\Microsoft\Windows\WER\ReportQueue")
            $files = @("$W\MEMORY.DMP")
        }
        'thumbs'  { $files = @(Get-ChildItem "$L\Microsoft\Windows\Explorer" -Filter '*cache_*.db' -Force -EA 0 | ForEach-Object FullName) }
        'browser' {
            foreach ($root in "$L\Google\Chrome\User Data", "$L\Microsoft\Edge\User Data", "$L\BraveSoftware\Brave-Browser\User Data", "$R\Opera Software\Opera Stable", "$L\Opera Software\Opera Stable", "$R\Opera Software\Opera GX Stable", "$L\Opera Software\Opera GX Stable") {
                if (-not (Test-Path -LiteralPath $root)) { continue }
                if (-not (Test-JunkFree $(switch -Wildcard ($root) { '*Chrome*' { 'chrome' } '*Edge*' { 'msedge' } '*Brave*' { 'brave' } default { 'opera' } }))) { continue }
                $profiles = @($root) + @(Get-ChildItem -LiteralPath $root -Directory -Force -EA 0 | Where-Object { $_.Name -eq 'Default' -or $_.Name -like 'Profile *' } | ForEach-Object FullName)
                foreach ($pr in $profiles) { foreach ($c in 'Cache', 'Code Cache', 'GPUCache', 'Service Worker\CacheStorage') { $dirs += (Join-Path $pr $c) } }
                $dirs += (Join-Path $root 'ShaderCache'); $dirs += (Join-Path $root 'GrShaderCache')
            }
            if (Test-JunkFree 'firefox') { $dirs += @(Get-ChildItem "$L\Mozilla\Firefox\Profiles" -Directory -Force -EA 0 | ForEach-Object { Join-Path $_.FullName 'cache2' }) }
        }
        'apps'    {
            foreach ($d in 'discord', 'discordptb', 'discordcanary') { if (Test-JunkFree $d) { $dirs += Get-WebCache "$R\$d" } }
            if ((Test-JunkFree 'steam') -and (Test-JunkFree 'steamwebhelper')) { $dirs += Get-WebCache "$L\Steam\htmlcache\Default"; $dirs += Get-WebCache "$L\Steam\htmlcache" }
            if (Test-JunkFree 'spotify') { $dirs += "$L\Spotify\Data", "$R\Spotify\Data" }
            if (Test-JunkFree 'teams') { $dirs += "$L\Microsoft\Teams\Cache", "$R\Microsoft\Teams\Cache" }
        }
        'shader'  { $dirs = @("$L\NVIDIA\DXCache", "$L\NVIDIA\GLCache", "$L\AMD\DxCache", "$L\AMD\DxcCache", "$L\AMD\VkCache", "$L\D3DSCache") }
        'recycle' { }
    }
    @{ Dirs = @($dirs | Where-Object { $_ -and (Test-Path -LiteralPath $_) }); Files = @($files | Where-Object { $_ -and (Test-Path -LiteralPath $_) }) }
}
$JunkIds = @('temp', 'wu', 'do', 'dumps', 'thumbs', 'browser', 'apps', 'shader', 'recycle')

function Get-RecycleSize {
    $t = [int64]0
    foreach ($d in [System.IO.DriveInfo]::GetDrives()) {
        if ($d.DriveType -ne 'Fixed' -or -not $d.IsReady) { continue }
        $t += Get-FolderSize (Join-Path $d.RootDirectory.FullName '$Recycle.Bin')
    }
    $t
}

function Do-JunkScan {
    $out = @{}
    foreach ($id in $JunkIds) {
        if ($id -eq 'recycle') { $out[$id] = Get-RecycleSize; continue }
        $tg = Get-JunkTargets $id
        $n = [int64]0
        foreach ($d in $tg.Dirs) { $n += Get-FolderSize $d }
        foreach ($f in $tg.Files) { try { $n += (Get-Item -LiteralPath $f -Force).Length } catch { } }
        $out[$id] = $n
        Emit @{ t = 'junk'; id = $id; bytes = $n }
    }
    Emit @{ t = 'result'; action = 'JunkScan'; ok = $true; cats = $out }
}

function Remove-DirContents([string]$d) {
    $freed = [int64]0
    foreach ($it in @(Get-ChildItem -LiteralPath $d -Force -EA 0)) {
        try {
            if ($it.PSIsContainer) {
                if ($it.Attributes -band [IO.FileAttributes]::ReparsePoint) { continue } # ลิงก์ไปที่อื่น ห้ามลบข้างใน
                $freed += Remove-DirContents $it.FullName
                $it.Delete()
            } else {
                $sz = $it.Length
                if ($it.IsReadOnly) { $it.IsReadOnly = $false }
                $it.Delete(); $freed += $sz
            }
        } catch { } # ไฟล์ที่เปิดใช้อยู่ข้ามไป
    }
    $freed
}
function Do-JunkClean {
    $list = @($Ids -split ',' | Where-Object { $JunkIds -contains $_ })
    $per = @{}; $total = [int64]0
    foreach ($id in $list) {
        Emit @{ t = 'junk'; id = $id; state = 'run' }
        $freed = [int64]0
        try {
            switch ($id) {
                'recycle' { $b = Get-RecycleSize; Clear-RecycleBin -Force -EA 0; $freed = [math]::Max(0, $b - (Get-RecycleSize)) }
                'wu' {
                    $tg = Get-JunkTargets $id
                    $svc = @('wuauserv', 'bits') | Where-Object { (Get-Service $_ -EA 0).Status -eq 'Running' }
                    foreach ($x in $svc) { Stop-Service $x -Force -EA 0 }
                    foreach ($d in $tg.Dirs) { $freed += Remove-DirContents $d }
                    foreach ($x in $svc) { Start-Service $x -EA 0 }
                }
                'do' {
                    $tg = Get-JunkTargets $id
                    $b = [int64]0; foreach ($d in $tg.Dirs) { $b += Get-FolderSize $d }
                    try { Delete-DeliveryOptimizationCache -Force -ErrorAction Stop } catch { foreach ($d in $tg.Dirs) { Remove-DirContents $d | Out-Null } }
                    $a = [int64]0; foreach ($d in $tg.Dirs) { $a += Get-FolderSize $d }
                    $freed = [math]::Max(0, $b - $a)
                }
                default {
                    $tg = Get-JunkTargets $id
                    foreach ($d in $tg.Dirs) { $freed += Remove-DirContents $d }
                    foreach ($f in $tg.Files) { try { $sz = (Get-Item -LiteralPath $f -Force).Length; Remove-Item -LiteralPath $f -Force -ErrorAction Stop; $freed += $sz } catch { } }
                }
            }
            if ($id -eq 'temp') { ipconfig /flushdns | Out-Null }
        } catch { Write-Log "ล้าง $id ไม่ครบ: $($_.Exception.Message)" }
        $per[$id] = $freed; $total += $freed
        Emit @{ t = 'junk'; id = $id; state = 'ok'; freed = $freed }
    }
    Write-Log ("ล้างขยะทั้งเครื่อง ได้พื้นที่คืน {0:N0} MB" -f ($total / 1MB))
    Emit @{ t = 'result'; action = 'JunkClean'; ok = $true; mb = [math]::Round($total / 1MB); per = $per; skip = ($Script:JunkSkip -join ', ') }
}

# ถามแอป Discord ที่เปิดอยู่ว่าเป็นบัญชีไหน (ช่อง RPC ปกติของ Discord) แล้วขอยศ Test ให้บัญชีนั้น
function Get-DcUser([string]$app) {
    foreach ($i in 0..9) {
        $p = New-Object IO.Pipes.NamedPipeClientStream('.', "discord-ipc-$i", [IO.Pipes.PipeDirection]::InOut, [IO.Pipes.PipeOptions]::Asynchronous)
        try {
            $p.Connect(200)
            $b = [Text.Encoding]::UTF8.GetBytes("{`"v`":1,`"client_id`":`"$app`"}")
            $o = [BitConverter]::GetBytes([int]0) + [BitConverter]::GetBytes([int]$b.Length) + $b
            $p.Write($o, 0, $o.Length); $p.Flush()
            $rd = { param($n) $z = New-Object byte[] $n; $g = 0; while ($g -lt $n) { $k = $p.ReadAsync($z, $g, $n - $g); if (-not $k.Wait(3000) -or $k.Result -le 0) { throw 'pipe' }; $g += $k.Result }; , $z }
            $h = & $rd 8; $n = [BitConverter]::ToInt32($h, 4)
            if ([BitConverter]::ToInt32($h, 0) -eq 1 -and $n -gt 0 -and $n -lt 65536) {
                $j = [Text.Encoding]::UTF8.GetString((& $rd $n)) | ConvertFrom-Json
                if ($j.evt -eq 'READY' -and $j.data.user.id) { return $j.data.user }
            }
        } catch { } finally { $p.Dispose() }
    }
}
function Do-TestRole {
    $code = ''; $name = ''
    try {
        [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor 3072
        $api = 'https://zan-license.zanhub.workers.dev/v1/test'
        $u = Get-DcUser ([string](Invoke-RestMethod $api -TimeoutSec 8).app)
        if (-not $u) { $code = 'nodiscord' }
        else {
            $name = [string]$u.username
            $q = Invoke-RestMethod $api -Method Post -ContentType 'application/json' -TimeoutSec 15 -Body (@{ m = $Ids; u = [string]$u.id; e = [int64]$Opt } | ConvertTo-Json -Compress)
            $code = $(if ($q.ok) { 'ok' } else { [string]$q.code })
        }
    } catch { $code = 'net' }
    Emit @{ t = 'result'; action = 'TestRole'; ok = ($code -eq 'ok'); code = $code; name = $name }
}

# เลือกแกน CPU ให้แต่ละโปรแกรม: Ids '' = ดูอย่างเดียว, set (Opt ชื่อ|mask), del (Opt ชื่อ คืนค่าเดิม), apply (ใช้กฎทั้งหมดอีกครั้ง)
# กฎเก็บใน config.json เป็น "mask|ค่าเดิม" ค่าเดิมคือ affinity ตอนตั้งกฎครั้งแรก
# ตั้งแกนตอนเปิดเครื่อง (ไม่ต้องเปิด Zan Booster): งานใน Task Scheduler รันเป็น SYSTEM ตอนล็อกอิน ไม่มีหน้าต่าง
# ลองซ้ำทุก 5 วินาทีราว 2 นาที เพราะ audiodg เริ่มตอนมีเสียงแรก ไม่ใช่ทันทีที่เข้าเครื่อง · ปิดเมื่อติ๊กออก/ไม่มีกฎ/คืนค่าทั้งหมด
$AffTask = 'ZanBooster CPU cores'
function Sync-AffBoot([bool]$on) {
    $f = Join-Path $DataDir 'aff-boot.ps1'
    try {
        if ($on) {
            [IO.File]::WriteAllText($f, "`$c = Get-Content '$ConfigFile' -Raw | ConvertFrom-Json`nfor (`$i = 0; `$i -lt 24; `$i++) { foreach (`$r in `$c.aff.PSObject.Properties) { `$m = [int64](`$r.Value -split '\|')[0]; foreach (`$p in @(Get-Process -Name `$r.Name -EA 0)) { try { if ([int64]`$p.ProcessorAffinity -ne `$m) { `$p.ProcessorAffinity = [IntPtr]`$m } } catch { } } }; Start-Sleep 5 }")
            Register-ScheduledTask -TaskName $AffTask -Action (New-ScheduledTaskAction -Execute 'powershell.exe' -Argument "-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$f`"") -Trigger (New-ScheduledTaskTrigger -AtLogOn) -Principal (New-ScheduledTaskPrincipal -UserId 'SYSTEM' -RunLevel Highest) -Settings (New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit (New-TimeSpan -Minutes 5)) -Force | Out-Null
        } else { Unregister-ScheduledTask -TaskName $AffTask -Confirm:$false -EA 0; Remove-Item $f -Force -EA 0 }
    } catch { Write-Log "CPU affinity boot: $_" }
}

function Do-Aff {
    $c = Get-Config; $r = @{}; if ($c.aff) { $c.aff.PSObject.Properties | ForEach-Object { $r[$_.Name] = [string]$_.Value } }
    $n = [Environment]::ProcessorCount; $all = $(if ($n -ge 64) { -1 } else { ([int64]1 -shl $n) - 1 })
    $script:hit = 0
    $set = { param($k, $m) foreach ($p in @(Get-Process -Name $k -EA 0)) { try { $p.ProcessorAffinity = [IntPtr][int64]$m; $script:hit++ } catch { } } }
    if ($Ids -eq 'set') {
        $k, $m = $Opt -split '\|'; $o = "$all"; $old = $r[$k]
        if ($r[$k]) { $o = ($r[$k] -split '\|')[1] } else { try { $o = [string][int64](Get-Process -Name $k -EA 0 | Select-Object -First 1).ProcessorAffinity } catch { } }
        $r[$k] = "$m|$o"; & $set $k $m
        if (-not $script:hit -and (Get-Process -Name $k -EA 0)) { $r.Remove($k); if ($old) { $r[$k] = $old }; $fail = 1 }
    } elseif ($Ids -eq 'del' -and $r[$Opt]) { & $set $Opt ($r[$Opt] -split '\|')[1]; $r.Remove($Opt) }
    elseif ($Ids -eq 'apply') { foreach ($k in @($r.Keys)) { & $set $k ($r[$k] -split '\|')[0] } }
    elseif ($Ids -eq 'boot') { $c | Add-Member affBoot ($Opt -eq '1') -Force }
    if ($Ids -in 'set', 'del', 'boot') { $c | Add-Member aff ([pscustomobject]$r) -Force; Save-Config $c; Write-Log "CPU affinity $Ids $Opt"; Sync-AffBoot ([bool]$c.affBoot -and $r.Count) }
    $cores = [Collections.Generic.List[object]]::new()
    try {
        if (-not ('ZC' -as [type])) { Add-Type 'using System;using System.Runtime.InteropServices;public class ZC{[DllImport("kernel32")]public static extern bool GetLogicalProcessorInformationEx(int r,IntPtr b,ref int n);}' }
        $X = [Runtime.InteropServices.Marshal]; $z = 0; [void][ZC]::GetLogicalProcessorInformationEx(0, [IntPtr]::Zero, [ref]$z); $b = $X::AllocHGlobal($z)
        if ([ZC]::GetLogicalProcessorInformationEx(0, $b, [ref]$z)) { for ($o = 0; $o -lt $z; $o += $X::ReadInt32($b, $o + 4)) { if ($X::ReadInt16($b, $o + 40) -eq 0) { $cores.Add(@{ e = $X::ReadByte($b, $o + 9); m = [string]$X::ReadInt64($b, $o + 32) }) } } }
        $X::FreeHGlobal($b)
    } catch { $ce = "$_" }
    $seen = @{}; $list = [Collections.Generic.List[object]]::new()
    foreach ($p in @(Get-Process -EA 0)) {
        $k = $p.ProcessName
        if ($seen[$k] -or ($p.MainWindowHandle -eq 0 -and -not $r[$k] -and $k -ne 'audiodg') -or $p.Id -eq $PID) { continue }
        try { $list.Add(@{ n = $k; d = [string]$p.Description; a = [string][int64]$p.ProcessorAffinity }); $seen[$k] = 1 } catch { if ($k -eq 'audiodg') { $list.Add(@{ n = $k; a = '' }); $seen[$k] = 1 } }
    }
    Emit @{ t = 'result'; action = 'Aff'; ok = $true; hit = $script:hit; fail = $fail; ce = $ce; boot = [bool]$c.affBoot; task = $(if ($Ids -ne 'apply') { [bool](Get-ScheduledTask -TaskName $AffTask -EA 0) }); all = "$all"; cores = $cores.ToArray(); apps = $list.ToArray(); rules = $r }
}

# ระหว่างเล่น: CPU ถูกจำกัดความเร็วกี่ % (100 = ไม่โดน) และอุณหภูมิ/ลดความเร็วเพราะร้อนของการ์ด NVIDIA
function Do-Heat {
    $h = @{ t = 'result'; action = 'Heat'; ok = $true }
    try { $h.cpu = [int](Get-CimInstance Win32_PerfFormattedData_Counters_ProcessorInformation -Filter "Name='_Total'").PercentPerformanceLimit } catch { }
    try { $h.gpu = [string](nvidia-smi --query-gpu=temperature.gpu,clocks_throttle_reasons.hw_thermal_slowdown,clocks_throttle_reasons.sw_thermal_slowdown --format=csv,noheader,nounits | Select-Object -First 1) } catch { }
    Emit $h
}

# ค้นหาเพลงใน YouTube จากชื่อ (ไม่ต้องใช้กุญแจ API): อ่านหน้าผลค้นหาแล้วดึงรหัสคลิป ชื่อ ช่อง ความยาว สูงสุด 12 รายการ
function Do-YtSearch {
    $o = [Collections.Generic.List[object]]::new()
    try {
        [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor 3072
        $h = (Invoke-WebRequest "https://www.youtube.com/results?search_query=$([uri]::EscapeDataString($Opt))&sp=EgIQAQ%3D%3D" -UseBasicParsing -TimeoutSec 15 -UserAgent 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36' -Headers @{ 'Accept-Language' = 'th,en;q=0.8' }).Content
        $seen = @{}; $lr = [regex]'"lengthText":\{.{0,400}?"simpleText":"([\d:.]+)"'
        foreach ($m in [regex]::Matches($h, '"videoRenderer":\{"videoId":"([\w-]{11})"(.{0,3000}?)"title":\{"runs":\[\{"text":"((?:[^"\\]|\\.)*)"(.{0,3000}?)"(?:longBylineText|ownerText)":\{"runs":\[\{"text":"((?:[^"\\]|\\.)*)"')) {
            $v = $m.Groups[1].Value; if ($seen[$v]) { continue }; $seen[$v] = 1
            $d = ''; try { $d = $lr.Match($h, $m.Index).Groups[1].Value } catch { }
            $o.Add(@{ v = $v; t = [regex]::Unescape($m.Groups[3].Value); c = [regex]::Unescape($m.Groups[5].Value); d = $d })
            if ($o.Count -ge 12) { break }
        }
    } catch { $e = "$_" }
    Emit @{ t = 'result'; action = 'YtSearch'; ok = ($o.Count -gt 0); items = $o.ToArray(); m = $e }
}

# เข้าแล้วปิดโปรแกรม: ดูว่า FiveM เข้าเซิร์ฟสำเร็จหรือยัง (บรรทัด connectOK ใน log หลังผ่านหน้ายืนยันของเซิร์ฟ) อ่านอย่างเดียว ไม่แตะเกม
# Opt = ตำแหน่งในไฟล์ที่อ่านไปแล้ว คืน n = ขนาดไฟล์ตอนนี้, f = ชื่อไฟล์ log ล่าสุด, hit = เจอ connectOK หลังตำแหน่งนั้น
$ZW = '[DllImport("user32")]public static extern bool ShowWindowAsync(IntPtr h,int c);[DllImport("user32")]public static extern bool IsIconic(IntPtr h);[DllImport("user32")]public static extern bool SetForegroundWindow(IntPtr h);[DllImport("user32")]public static extern bool IsWindow(IntPtr h);[DllImport("user32")]public static extern bool IsWindowVisible(IntPtr h);[DllImport("user32")]public static extern bool PostMessage(IntPtr h,int m,IntPtr w,IntPtr l);[ComImport,Guid("56FDF342-FD6D-11d0-958A-006097C9A090"),InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]public interface ITL{void HrInit();void AddTab(IntPtr h);void DeleteTab(IntPtr h);}[ComImport,Guid("56FDF344-FD6D-11d0-958A-006097C9A090")]public class TL{}public static void Tab(IntPtr h,bool a){var t=(ITL)new TL();t.HrInit();if(a)t.AddTab(h);else t.DeleteTab(h);}'
# เปิดโปรแกรมซ้ำ: ถ้ามีตัวเดิมซ่อนที่ถาดหรือพับอยู่ (ไม่แตะตัวที่เปิดโชว์อยู่ เช่นตัวเก่าที่กำลังปิดตอนอัปเดต) ดึงตัวเดิมขึ้นมา หน้าจอจะปิดตัวใหม่เอง
function Do-Dup {
    $f = $false
    try {
        $me = Get-Process -Id (Get-CimInstance Win32_Process -Filter "ProcessId=$PID").ParentProcessId
        $t = "$env:TEMP\zbtray"; $tp = @(0, 0); $tw = [datetime]0; if (Test-Path $t) { $tp = [IO.File]::ReadAllText($t) -split ':'; $tw = (Get-Item $t).LastWriteTime }
        $o = Get-Process $me.ProcessName -EA 0 | Where-Object { $_.Id -ne $me.Id -and $_.StartTime -lt $me.StartTime.AddSeconds(-5) -and ($_.MainWindowHandle -ne 0 -or ("$($_.Id)" -eq $tp[0] -and $tw -gt $_.StartTime)) } | Select-Object -First 1
        if ($o) { Start-Sleep 1; $o.Refresh() }
        if ($o -and -not $o.HasExited) {
            $h = [int64]$o.MainWindowHandle; if (-not $h -and "$($o.Id)" -eq $tp[0]) { $h = [int64]$tp[1] }
            Add-Type -Name W -Namespace Z -MemberDefinition $ZW
            if (-not [Z.W]::IsWindow([IntPtr]$h)) { }
            elseif (-not [Z.W]::IsWindowVisible([IntPtr]$h)) { Stop-Process -Id $o.Id -Force -EA 0 }
            elseif ($o.Responding -and [Z.W]::IsIconic([IntPtr]$h)) { [Z.W]::Tab([IntPtr]$h, $true); [void][Z.W]::ShowWindowAsync([IntPtr]$h, 9); [void][Z.W]::SetForegroundWindow([IntPtr]$h); $f = $true }
        }
    } catch { }
    Emit @{ t = 'result'; action = 'Dup'; ok = $true; found = $f }
}

# ปุ่มปิดตอนเพลงเล่น: ซ่อนหน้าต่างไว้ที่ถาดข้างนาฬิกา (ตัวช่วยแยกกระบวนการ คลิกไอคอนเพื่อเปิด คลิกขวาเพื่อปิดโปรแกรม)
function Do-Tray {
    $ok = $false
    try {
        $pp = (Get-CimInstance Win32_Process -Filter "ProcessId=$PID").ParentProcessId; $p = Get-Process -Id $pp; $h = [int64]$p.MainWindowHandle
        if ($h) {
            $s = '$mx=New-Object Threading.Mutex($false,''Local\ZanBoosterTray'');if(-not $mx.WaitOne(0)){exit};Add-Type -AssemblyName System.Windows.Forms,System.Drawing;Add-Type -Name W -Namespace Z -MemberDefinition ''ZBW'';$h=[IntPtr]ZBH;$x=0;$n=New-Object Windows.Forms.NotifyIcon;$n.Icon=[Drawing.Icon]::ExtractAssociatedIcon(''ZBE'');$n.Text=''Zan Booster'';$n.Visible=1;$q={$n.Visible=0;[Windows.Forms.Application]::Exit()};$o={[Z.W]::Tab($h,1);[void][Z.W]::ShowWindowAsync($h,9);[void][Z.W]::SetForegroundWindow($h);&$q};$n.add_MouseClick({if($_.Button -eq ''Left''){&$o}});$m=New-Object Windows.Forms.ContextMenuStrip;[void]$m.Items.Add(''เปิด Zan Booster'',$null,{&$o});[void]$m.Items.Add(''ปิดโปรแกรม'',$null,{$global:x=1;[void][Z.W]::PostMessage($h,16,[IntPtr]::Zero,[IntPtr]::Zero);&$q});$n.ContextMenuStrip=$m;[void][Z.W]::ShowWindowAsync($h,6);Start-Sleep -m 400;[Z.W]::Tab($h,0);$t=New-Object Windows.Forms.Timer;$t.Interval=2000;$t.add_Tick({if(-not [Z.W]::IsWindow($h) -or -not [Z.W]::IsIconic($h)){[Z.W]::Tab($h,1);&$q}});$t.Start();try{[Windows.Forms.Application]::Run()}finally{$n.Dispose();if(-not $x -and [Z.W]::IsWindow($h)){[Z.W]::Tab($h,1)}}'.Replace('ZBW', $ZW).Replace('ZBH', "$h").Replace('ZBE', $p.Path.Replace("'", "''"))
            Start-Helper 'zb-tray.ps1' $s
            Add-Type -Name W -Namespace Z -MemberDefinition $ZW
            for ($i = 0; $i -lt 40 -and -not $ok; $i++) { Start-Sleep -Milliseconds 250; $ok = [Z.W]::IsIconic([IntPtr]$h) }
            if ($ok) { [IO.File]::WriteAllText("$env:TEMP\zbtray", "$pp`:$h") }
        }
    } catch { }
    Emit @{ t = 'result'; action = 'Tray'; ok = $ok }
}

function Do-Rpc {
    $ok = $false
    try {
        if ($Opt -eq 'off') { try { [Threading.EventWaitHandle]::OpenExisting('Local\ZanBoosterRpcStop').Set() | Out-Null } catch { }; $ok = $true }
        else {
            $pp = (Get-CimInstance Win32_Process -Filter "ProcessId=$PID").ParentProcessId
            $s = '$m=New-Object Threading.Mutex($false,''Local\ZanBoosterRpc'');if(-not $m.WaitOne(0)){exit};$q=New-Object Threading.EventWaitHandle($false,''ManualReset'',''Local\ZanBoosterRpcStop'');$c=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String(''ZBC'')) -split "`n";$pp=ZBP;function G($s,$n){$d=New-Object byte[] $n;$r=0;while($r -lt $n){$k=$s.Read($d,$r,$n-$r);if($k -le 0){throw ''x''};$r+=$k};,$d}function F($s,$o,$t){$b=[Text.Encoding]::UTF8.GetBytes($t);$s.Write([BitConverter]::GetBytes([int]$o),0,4);$s.Write([BitConverter]::GetBytes($b.Length),0,4);$s.Write($b,0,$b.Length);$s.Flush();$h=G $s 8;$null=G $s ([BitConverter]::ToInt32($h,4))}while((Get-Process -Id $pp -EA 0) -and -not $q.WaitOne(0)){$s=$null;for($i=0;$i -lt 10 -and -not $s;$i++){try{$p=New-Object IO.Pipes.NamedPipeClientStream(''.'',"discord-ipc-$i",''InOut'');$p.Connect(250);$s=$p}catch{}}if($s){try{F $s 0 $c[0];while((Get-Process -Id $pp -EA 0) -and -not $q.WaitOne(0)){F $s 1 $c[1].Replace(''ZPID'',"$PID");if($q.WaitOne(60000)){break}}}catch{};try{$s.Dispose()}catch{}}if($q.WaitOne(20000)){break}}'.Replace('ZBP', "$pp").Replace('ZBC', $Opt)
            Start-Helper 'zb-rpc.ps1' $s; $ok = $true
        }
    } catch { }
    Emit @{ t = 'result'; action = 'Rpc'; ok = $ok }
}

# ตัวช่วยเบื้องหลัง: เขียนเป็นไฟล์สคริปต์ธรรมดาใน ProgramData\ZanBooster แล้วเปิดแบบปกติ (ไม่ใช้คำสั่งเข้ารหัส แอนติไวรัสจะไม่เข้าใจผิด)
function Start-Helper([string]$n, [string]$s) {
    $f = Join-Path $DataDir $n; [IO.File]::WriteAllText($f, $s, [Text.UTF8Encoding]::new($true))
    Start-Process powershell.exe -ArgumentList '-NoProfile', '-STA', '-WindowStyle', 'Hidden', '-File', "`"$f`""
}
function Do-JoinWait {
    $n = -1; $hit = $false; $nm = ''
    try {
        $lg = Get-ChildItem (Join-Path (Split-Path (Get-FiveMData)) 'logs') -Filter 'CitizenFX_log*' -EA 0 | Sort-Object LastWriteTime | Select-Object -Last 1
        if ($lg) {
            $fs = [IO.File]::Open($lg.FullName, 'Open', 'Read', 'ReadWrite'); $n = $fs.Length; $nm = $lg.Name
            if ($Opt -match '^\d+$' -and [int64]$Opt -lt $n) { $fs.Position = [int64]$Opt; $hit = [IO.StreamReader]::new($fs).ReadToEnd() -match '(?i)connectOK' }
            $fs.Close()
        }
    } catch { }
    Emit @{ t = 'result'; action = 'JoinWait'; ok = ($n -ge 0); n = $n; f = $nm; hit = $hit }
}

function Do-BoostNow {
    $procs = @(Get-Process -EA 0 | Where-Object { $_.Name -like 'FiveM*GTAProcess' })
    if ($procs.Count -eq 0) { Emit @{ t = 'result'; action = 'BoostNow'; ok = $false; m = 'ยังไม่ได้เปิดเกม FiveM' }; return }
    foreach ($p in $procs) { try { $p.PriorityClass = 'High'; Write-Log "$($p.Name) -> High" } catch { } }
    Emit @{ t = 'result'; action = 'BoostNow'; ok = $true }
}

try {
    switch ($Action) {
        'Status'     { Do-Status }
        'SysInfo'    { Do-SysInfo }
        'Apply'      { Do-Apply }
        'Restore'    { Do-Restore }
        'Revert'     { Do-Revert }
        'CleanCache' { Do-CleanCache }
        'CleanTemp'  { Do-CleanTemp }
        'JunkScan'   { Do-JunkScan }
        'JunkClean'  { Do-JunkClean }
        'UpdPrep'    {
        try {
            $pp = (Get-CimInstance Win32_Process -Filter "ProcessId=$PID").ParentProcessId
            $x = [string](Get-CimInstance Win32_Process -Filter "ProcessId=$pp").ExecutablePath
            if ($x -like '*.exe') {
                Get-ChildItem -LiteralPath (Split-Path $x) -Filter "$(Split-Path $x -Leaf).old-*" -File | ForEach-Object { try { $_.Delete() } catch { } }
                if (Test-Path -LiteralPath "$x.old") { try { [IO.File]::Delete("$x.old") } catch { [IO.File]::Move("$x.old", "$x.old-$PID") } }
            }
        } catch { }
        Emit @{ t = 'result'; action = 'UpdPrep'; ok = $true }
    }
    'NuiGpu'     { Do-NuiGpu }
    'GfxTune'    { Do-GfxTune }
    'OvOff'      { Do-OvOff }
    'RsFix'      { Do-RsFix }
    'RsLevel'    { Do-RsLevel }
    'RsCustom'   { Do-RsCustom }
    'AudioFix'   { Do-AudioFix }
    'TestRole'   { Do-TestRole }
    'BoostNow'   { Do-BoostNow }
    'Aff'        { Do-Aff }
    'Heat'       { Do-Heat }
    'YtSearch'   { Do-YtSearch }
    'JoinWait'   { Do-JoinWait }
    'Tray'       { Do-Tray }
    'Dup'        { Do-Dup }
    'Rpc'        { Do-Rpc }
    'Reboot'     { shutdown.exe /r /t 5 /c 'Zan Booster'; Emit @{ t = 'result'; action = 'Reboot'; ok = ($LASTEXITCODE -eq 0) } }
    'NuiPrio'    { $n = 0; foreach ($x in @(Get-Process FiveM_ChromeBrowser -EA 0)) { try { $x.PriorityClass = $(if ($Opt -eq 'low') { 'BelowNormal' } else { 'Normal' }); $n++ } catch { } }; Emit @{ t = 'result'; action = 'NuiPrio'; ok = ($n -gt 0); n = $n } }
        'PickFiveM'  { Do-PickFiveM }
        'Health'     { Do-Health }
        'GfxStatus'  { Do-GfxStatus }
        'GfxPlan'    { Do-GfxPlan }
        'Report'     { Do-Report }
        'GfxApply'   { Do-GfxApply }
        'GfxRestore' { Do-GfxRestore }
        'ListApps'   { Do-ListApps }
        'CloseApps'  { Do-CloseApps }
        'FpsScan'    { Do-FpsScan }
        'FpsFix'     { Do-FpsFix }
        'FpsUndo'    { Do-FpsUndo }
        default      { Emit @{ t = 'error'; m = "unknown action $Action" } }
    }
} catch {
    Emit @{ t = 'error'; action = $Action; m = $_.Exception.Message }
}
