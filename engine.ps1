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
New-Item -ItemType Directory -Path $DataDir -Force | Out-Null

# ------------------------------------------------------------- output ----
$Script:Capture = $null
function Emit($obj) { if ($Script:Capture -ne $null) { $Script:Capture.Add($obj); return }; [Console]::Out.WriteLine(($obj | ConvertTo-Json -Compress -Depth 6)); [Console]::Out.Flush() }
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
    $cands = New-Object System.Collections.Generic.List[string]
    $cfg = Get-Config
    if ($cfg.FiveMPath) { $cands.Add([string]$cfg.FiveMPath) }
    # 1) เกมที่กำลังเปิดอยู่
    try { Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.Name -like 'FiveM*' } | ForEach-Object { if ($_.Path) { $cands.Add($_.Path) } } } catch { }
    # 2) LocalAppData ของผู้ใช้ปัจจุบัน
    $cands.Add((Join-Path $env:LOCALAPPDATA 'FiveM'))
    # 3) รายการโปรแกรมที่ติดตั้ง (ทุก user)
    $uroots = @('HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall', 'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall')
    try { Get-ChildItem 'Registry::HKEY_USERS' -ErrorAction SilentlyContinue | ForEach-Object { $uroots += "Registry::$($_.Name)\Software\Microsoft\Windows\CurrentVersion\Uninstall" } } catch { }
    foreach ($r in $uroots) {
        if (-not (Test-Path $r)) { continue }
        Get-ChildItem $r -ErrorAction SilentlyContinue | ForEach-Object {
            $pr = Get-ItemProperty $_.PSPath -ErrorAction SilentlyContinue
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
        Get-ChildItem 'C:\Users' -Directory -ErrorAction SilentlyContinue | ForEach-Object {
            $lnkDirs += (Join-Path $_.FullName 'Desktop'); $lnkDirs += (Join-Path $_.FullName 'AppData\Roaming\Microsoft\Windows\Start Menu')
        }
        foreach ($d in $lnkDirs) {
            if (-not $d -or -not (Test-Path $d)) { continue }
            Get-ChildItem $d -Filter '*FiveM*.lnk' -Recurse -Depth 3 -ErrorAction SilentlyContinue | ForEach-Object {
                $t = $sh.CreateShortcut($_.FullName).TargetPath; if ($t) { $cands.Add($t) }
            }
        }
    } catch { }
    # 5) LocalAppData ของทุก user (กรณีเปิดโปรแกรมด้วยบัญชี admin อื่น)
    Get-ChildItem 'C:\Users' -Directory -ErrorAction SilentlyContinue | ForEach-Object { $cands.Add((Join-Path $_.FullName 'AppData\Local\FiveM')) }
    foreach ($c in $cands) { $r = Resolve-FiveMApp $c; if ($r) { return $r } }
    # 6) สแกนไดรฟ์ (ลึก 2 ชั้น) เช่น D:\FiveM, D:\Games\FiveM
    foreach ($drv in [IO.DriveInfo]::GetDrives()) {
        if ($drv.DriveType -ne 'Fixed' -or -not $drv.IsReady) { continue }
        $root = $drv.RootDirectory.FullName
        $lvl1 = @(Get-ChildItem $root -Directory -Force -ErrorAction SilentlyContinue | Where-Object { $_.Name -notmatch '^(Windows|\$Recycle\.Bin|System Volume Information|ProgramData|Recovery)$' })
        foreach ($d in $lvl1) {
            $r = Resolve-FiveMApp $d.FullName; if ($r) { return $r }
            foreach ($d2 in @(Get-ChildItem $d.FullName -Directory -ErrorAction SilentlyContinue)) {
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
$Script:DryKeys = New-Object System.Collections.Generic.List[string]
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
    [bool](($b.Registry.Count + $b.CreatedKeys.Count + $b.Powercfg.Count + $b.Adapters.Count + $b.Defender.Count) -gt 0 -or $b.PrevScheme -or (Test-Path (Join-Path $DataDir 'gta5_settings.original.xml')))
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

$DXPrefs  = 'HKCU:\Software\Microsoft\DirectX\UserGpuPreferences'
$GCS      = 'HKCU:\System\GameConfigStore'
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
function Get-UpAdapters { @(Get-NetAdapter -Physical -ErrorAction SilentlyContinue | Where-Object { $_.Status -eq 'Up' }) }
function Get-FolderSize([string]$p) {
    if (-not (Test-Path $p)) { return 0 }
    $s = (Get-ChildItem $p -Recurse -Force -File -ErrorAction SilentlyContinue | Measure-Object Length -Sum).Sum
    if ($s) { [int64]$s } else { 0 }
}

# ------------------------------------------------------------- tweaks ----
# Apply = ปรับ, Check = ตรวจว่าตอนนี้ปรับอยู่หรือไม่
$Tweaks = [ordered]@{
    PowerPlan = @{
        Apply = {
            $prev = Get-ActiveScheme
            if (-not $Script:Backup.PrevScheme -and $prev -ne $SchemeGuid) { $Script:Backup.PrevScheme = $prev }
            # ลบ plan เดิมของโปรแกรม (เวอร์ชันเก่าใช้ Ultimate Performance) แล้วนำเข้า ZAN_Project_Optimizer ใหม่ทุกครั้ง
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
            Set-RegValue 'HKCU:\Software\Microsoft\GameBar' 'AutoGameModeEnabled' 1
            Set-RegValue 'HKCU:\Software\Microsoft\GameBar' 'AllowAutoGameMode' 1
        }
        Check = { (Get-Reg 'HKCU:\Software\Microsoft\GameBar' 'AutoGameModeEnabled') -eq 1 }
    }
    GameDVR = @{
        Apply = {
            Set-RegValue 'HKCU:\System\GameConfigStore' 'GameDVR_Enabled' 0
            Set-RegValue 'HKCU:\Software\Microsoft\Windows\CurrentVersion\GameDVR' 'AppCaptureEnabled' 0
            Set-RegValue 'HKLM:\SOFTWARE\Policies\Microsoft\Windows\GameDVR' 'AllowGameDVR' 0
        }
        Check = { (Get-Reg 'HKCU:\System\GameConfigStore' 'GameDVR_Enabled') -eq 0 }
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
        Apply = { Set-RegValue 'HKCU:\Software\Microsoft\Windows\CurrentVersion\BackgroundAccessApplications' 'GlobalUserDisabled' 1 }
        Check = { (Get-Reg 'HKCU:\Software\Microsoft\Windows\CurrentVersion\BackgroundAccessApplications' 'GlobalUserDisabled') -eq 1 }
    }
    PrioSep = @{
        Apply = { Set-RegValue 'HKLM:\SYSTEM\CurrentControlSet\Control\PriorityControl' 'Win32PrioritySeparation' 38 }
        Check = { (Get-Reg 'HKLM:\SYSTEM\CurrentControlSet\Control\PriorityControl' 'Win32PrioritySeparation') -eq 38 }
    }
    MouseAccel = @{
        Apply = {
            Set-RegValue 'HKCU:\Control Panel\Mouse' 'MouseSpeed' '0' 'String'
            Set-RegValue 'HKCU:\Control Panel\Mouse' 'MouseThreshold1' '0' 'String'
            Set-RegValue 'HKCU:\Control Panel\Mouse' 'MouseThreshold2' '0' 'String'
        }
        Check = { [string](Get-Reg 'HKCU:\Control Panel\Mouse' 'MouseSpeed') -eq '0' }
    }
    UsbSuspend = @{
        Apply = { Set-PowercfgIndex $USB_SUB $USB_SET 0 }
        Check = { $v = Get-PowercfgIndex $USB_SUB $USB_SET; $v -and $v[0] -eq 0 }
    }
    FSO = @{
        Apply = {
            Set-RegValue 'HKCU:\System\GameConfigStore' 'GameDVR_FSEBehaviorMode' 2
            Set-RegValue 'HKCU:\System\GameConfigStore' 'GameDVR_FSEBehavior' 2
            Set-RegValue 'HKCU:\System\GameConfigStore' 'GameDVR_HonorUserFSEBehaviorMode' 1
            Set-RegValue 'HKCU:\System\GameConfigStore' 'GameDVR_DXGIHonorFSEWindowsCompatible' 1
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
            $a = Get-UpAdapters | Select-Object -First 1
            $a -and ((Get-Reg "HKLM:\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\$($a.InterfaceGuid)" 'TCPNoDelay') -eq 1)
        }
    }
    Adapter = @{
        Apply = {
            $changed = @()
            foreach ($a in Get-UpAdapters) {
                foreach ($kw in '*InterruptModeration', '*EEE') {
                    $prop = Get-NetAdapterAdvancedProperty -Name $a.Name -RegistryKeyword $kw -ErrorAction SilentlyContinue
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
        }
        Check = { $Script:Backup.Adapters.Count -gt 0 }
    }
    HVCI = @{
        # Memory Integrity (Core Isolation) บางเครื่องกิน FPS 5-15% ต้องรีสตาร์ทถึงมีผล
        Apply = { Set-RegValue 'HKLM:\SYSTEM\CurrentControlSet\Control\DeviceGuard\Scenarios\HypervisorEnforcedCodeIntegrity' 'Enabled' 0 }
        Check = { (Get-Reg 'HKLM:\SYSTEM\CurrentControlSet\Control\DeviceGuard\Scenarios\HypervisorEnforcedCodeIntegrity' 'Enabled') -ne 1 }
    }
    GpuMsi = @{
        # ให้การ์ดจอใช้ MSI interrupt แทน line-based: ลด DPC latency เฟรมสม่ำเสมอขึ้น (การ์ดรุ่นใหม่ส่วนใหญ่เปิดอยู่แล้ว) ต้องรีสตาร์ท
        Apply = {
            $n = 0
            foreach ($d in @(Get-PnpDevice -Class Display -PresentOnly -ErrorAction SilentlyContinue | Where-Object { $_.InstanceId -like 'PCI\*' })) {
                $k = "HKLM:\SYSTEM\CurrentControlSet\Enum\$($d.InstanceId)\Device Parameters\Interrupt Management\MessageSignaledInterruptProperties"
                try { Set-RegValue $k 'MSISupported' 1; $n++ } catch { Write-Log "ตั้ง MSI ให้ $($d.FriendlyName) ไม่ได้" }
            }
            if ($n -eq 0) { throw 'ไม่พบการ์ดจอที่ตั้งได้' }
            Write-Log "ตั้ง MSI mode ให้การ์ดจอ $n ตัว (มีผลหลังรีสตาร์ท)"
        }
        Check = {
            $ds = @(Get-PnpDevice -Class Display -PresentOnly -ErrorAction SilentlyContinue | Where-Object { $_.InstanceId -like 'PCI\*' })
            $ds.Count -gt 0 -and @($ds | Where-Object { (Get-Reg "HKLM:\SYSTEM\CurrentControlSet\Enum\$($_.InstanceId)\Device Parameters\Interrupt Management\MessageSignaledInterruptProperties" 'MSISupported') -ne 1 }).Count -eq 0
        }
    }
    GpuPref = @{
        # ให้ FiveM / GTA ใช้การ์ดจอแยกเสมอ (โน้ตบุ๊กที่มีการ์ดจอ 2 ตัว) เครื่องตั้งโต๊ะไม่มีผลอะไร
        Apply = {
            $key = 'HKCU:\Software\Microsoft\DirectX\UserGpuPreferences'
            $n = 0
            foreach ($exe in Get-FiveMExes) { Set-RegValue $key $exe 'GpuPreference=2;' 'String'; $n++ }
            Write-Log "ตั้งการ์ดจอแยกให้ $n ไฟล์"
        }
        Check = {
            $key = 'HKCU:\Software\Microsoft\DirectX\UserGpuPreferences'
            $exes = @(Get-FiveMExes)
            $exes.Count -gt 0 -and @($exes | Where-Object { (Get-Reg $key $_) -ne 'GpuPreference=2;' }).Count -eq 0
        }
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
                if ($list[$name] -eq 4 -and -not $Script:DryRun) { try { Stop-Service -Name $name -Force -ErrorAction SilentlyContinue } catch { } }
                $n++
            }
            Write-Log "ปรับ service $n ตัว"
        }
        Check = { (Get-Reg 'HKLM:\SYSTEM\CurrentControlSet\Services\DiagTrack' 'Start') -eq 4 -and (Get-Reg 'HKLM:\SYSTEM\CurrentControlSet\Services\SysMain' 'Start') -eq 4 }
    }
}

# ------------------------------------------------------------ actions ----
function Get-TweakKeys([string]$id) {
    $Script:DryKeys.Clear(); $Script:DryRun = $true
    try { & $Tweaks[$id].Apply } catch { } finally { $Script:DryRun = $false }
    @($Script:DryKeys)
}
function Test-TweakOwned([string]$id) {
    if ($id -eq 'PowerPlan') { return [bool]$Script:Backup.PrevScheme }
    if ($id -eq 'Adapter') { return $Script:Backup.Adapters.Count -gt 0 }
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
            Remove-ItemProperty -LiteralPath $e.Path -Name $e.Name -ErrorAction SilentlyContinue
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
    if ($id -eq 'Adapter') {
        if ($b.Adapters.Count -eq 0) { throw 'ไม่มีค่าเดิมเก็บไว้' }
        $restart = @()
        foreach ($kv in @($b.Adapters.GetEnumerator())) {
            $parts = $kv.Key -split '\|'
            try { Set-NetAdapterAdvancedProperty -Name $parts[0] -RegistryKeyword $parts[1] -RegistryValue $kv.Value -NoRestart -ErrorAction Stop; if ($restart -notcontains $parts[0]) { $restart += $parts[0] } } catch { }
        }
        $b.Adapters = @{}
        foreach ($n in $restart) { Restart-NetAdapter -Name $n -Confirm:$false -ErrorAction SilentlyContinue }
        return
    }
    $n = 0
    foreach ($k in Get-TweakKeys $id) {
        if ($k.StartsWith('PC|')) {
            $p = $k -split '\|'
            $scheme = Get-ActiveScheme
            $bk = "$scheme|$($p[1])|$($p[2])"
            $v = $(if ($b.Powercfg.ContainsKey($bk)) { @($b.Powercfg[$bk]) } else { @(1, 1) })
            powercfg /setacvalueindex SCHEME_CURRENT $p[1] $p[2] $v[0] | Out-Null
            powercfg /setdcvalueindex SCHEME_CURRENT $p[1] $p[2] $v[1] | Out-Null
            powercfg /setactive SCHEME_CURRENT | Out-Null
            $b.Powercfg.Remove($bk); $n++
            continue
        }
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
        # backup จากเวอร์ชันเก่ายังไม่มีรายการที่ตั้งไว้: เดาจากค่าที่โปรแกรมเคยแก้จริง
        $Script:Backup.Applied = @($Tweaks.Keys | Where-Object { $st[$_] -and (Test-TweakOwned $_) })
        $Script:AppliedKnown = $true
        if (Test-HasBackup) { Save-Backup }
    }
    Emit @{ t = 'result'; action = 'Status'; status = $st; hasBackup = (Test-HasBackup); applied = @($Script:Backup.Applied) }
}

function Do-SysInfo {
    $info = @{}
    try { $info.cpu = (Get-CimInstance Win32_Processor | Select-Object -First 1).Name.Trim() } catch { $info.cpu = '-' }
    try { $info.cores = [int](Get-CimInstance Win32_Processor | Measure-Object NumberOfLogicalProcessors -Sum).Sum } catch { }
    try { $info.gpu = ((Get-CimInstance Win32_VideoController | Where-Object { $_.Name -notmatch 'Basic|Remote|Virtual|Parsec|Meta' }).Name) -join ' + ' } catch { $info.gpu = '-' }
    try { $info.ram = [math]::Round((Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory / 1GB) } catch { }
    try { $os = Get-CimInstance Win32_OperatingSystem; $info.os = $os.Caption; $info.build = $os.BuildNumber } catch { }
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
            Enable-ComputerRestore -Drive "$env:SystemDrive\" -ErrorAction SilentlyContinue
            $before = @(Get-ComputerRestorePoint -ErrorAction SilentlyContinue).Count
            Checkpoint-Computer -Description 'Before Zan Booster' -RestorePointType MODIFY_SETTINGS -WarningAction SilentlyContinue
            $after = @(Get-ComputerRestorePoint -ErrorAction SilentlyContinue).Count
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
    Get-ChildItem $cls -ErrorAction SilentlyContinue | Where-Object { $_.PSChildName -match '^\d{4}$' } | ForEach-Object {
        $pr = Get-ItemProperty $_.PSPath -ErrorAction SilentlyContinue
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
            GrassQuality = $grass; WaterQuality = (& $r 2); ParticleQuality = (& $r 3); PostFX = (& $r 3)
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

function Do-GfxApply {
    $preset = $Opt
    $GfxPresets = (Get-GfxPlan).presets
    if (-not $GfxPresets.ContainsKey($preset)) { Emit @{ t = 'result'; action = 'GfxApply'; ok = $false; m = 'ไม่รู้จัก preset นี้' }; return }
    if (Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.Name -like 'FiveM*' }) {
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
    if (Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.Name -like 'FiveM*' }) {
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
        $pd = Get-PhysicalDisk -ErrorAction SilentlyContinue | Where-Object { [string]$_.DeviceId -eq [string]$disk.Number } | Select-Object -First 1
        $media = [string]$pd.MediaType; $bus = [string]$pd.BusType
        if ($bus -eq 'NVMe') { return 'NVMe SSD' }
        if ($media -eq 'SSD') { return 'SSD' }
        if ($media -eq 'HDD') { return 'HDD' }
        if ($disk.FriendlyName -match 'SSD|NVMe') { return 'SSD' }
        return 'ไม่ทราบ'
    } catch { 'ไม่ทราบ' }
}

# ไฟล์ exe ของ FiveM ที่ควรใช้การ์ดจอแยก (FiveM.exe + ตัวเกมทุก build ใน cache\subprocess + GTA5.exe)
function Get-FiveMExes {
    $out = New-Object System.Collections.Generic.List[string]
    $fd = Get-FiveMData
    if ($fd) {
        $root = Split-Path (Split-Path $fd -Parent) -Parent
        $fe = Join-Path $root 'FiveM.exe'; if (Test-Path -LiteralPath $fe) { $out.Add($fe) }
        $sub = Join-Path $fd 'cache\subprocess'
        if (Test-Path -LiteralPath $sub) { Get-ChildItem -LiteralPath $sub -Filter '*.exe' -ErrorAction SilentlyContinue | Where-Object { $_.Name -match 'GTAProcess|GameProcess' } | ForEach-Object { $out.Add($_.FullName) } }
        $ini = Join-Path (Split-Path $fd -Parent) 'CitizenFX.ini'
        if (Test-Path -LiteralPath $ini) {
            $m = Select-String -LiteralPath $ini -Pattern '^IVPath=(.+)$' -ErrorAction SilentlyContinue | Select-Object -First 1
            if ($m) { $g = Join-Path $m.Matches[0].Groups[1].Value.Trim() 'GTA5.exe'; if (Test-Path -LiteralPath $g) { $out.Add($g) } }
        }
    }
    $out
}

function Do-Health {
    $r = New-Object System.Collections.Generic.List[object]
    function Add($id, $title, $status, $detail, $fix) { $r.Add([ordered]@{ id = $id; title = $title; status = $status; detail = $detail; fix = $fix }) }

    # RAM speed / XMP
    try {
        $mods = @(Get-CimInstance Win32_PhysicalMemory)
        $cfg = ($mods | Measure-Object ConfiguredClockSpeed -Maximum).Maximum
        $rated = ($mods | Measure-Object Speed -Maximum).Maximum
        $type = ($mods | Select-Object -First 1).SMBIOSMemoryType
        $ddr = switch ($type) { 26 { 'DDR4' } 34 { 'DDR5' } 24 { 'DDR3' } default { 'DDR' } }
        $low = ($ddr -eq 'DDR4' -and $cfg -le 2666) -or ($ddr -eq 'DDR5' -and $cfg -le 4800) -or ($rated -gt $cfg)
        if ($low) { Add 'xmp' 'ความเร็วแรม' 'warn' "แรม $ddr วิ่งอยู่ที่ $cfg MHz อาจยังไม่ได้เปิด XMP / EXPO" 'เข้า BIOS ตอนเปิดเครื่อง (กด Del หรือ F2) หาเมนู XMP, D.O.C.P หรือ EXPO แล้วเปิด จากนั้นกด Save & Exit ส่วนนี้ช่วย FPS ใน FiveM ได้เยอะ' }
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

    # refresh rate
    try {
        $vc = Get-CimInstance Win32_VideoController | Where-Object { $_.CurrentRefreshRate -gt 0 } | Select-Object -First 1
        $cur = [int]$vc.CurrentRefreshRate; $max = [int]$vc.MaxRefreshRate
        if ($cur -le 60 -and $max -gt 75) { Add 'hz' 'รีเฟรชเรตจอ' 'warn' "จอตั้งไว้ $cur Hz แต่รองรับได้ถึง $max Hz" 'คลิกขวาที่เดสก์ท็อป > Display settings > Advanced display > เลือก Refresh rate สูงสุด' }
        elseif ($cur -le 60) { Add 'hz' 'รีเฟรชเรตจอ' 'info' "จอตั้งไว้ $cur Hz" 'ถ้าจอรองรับ 144Hz ขึ้นไป ให้ไปตั้งใน Display settings > Advanced display' }
        else { Add 'hz' 'รีเฟรชเรตจอ' 'ok' "$cur Hz" '' }

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
        $bat = Get-CimInstance Win32_Battery -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($bat) {
            if ($bat.BatteryStatus -eq 1) { Add 'power' 'แหล่งไฟ' 'warn' 'กำลังใช้แบตเตอรี่' 'เสียบสายชาร์จตอนเล่น โน้ตบุ๊กจะลดความเร็วการ์ดจอกับ CPU ตอนใช้แบต' }
            else { Add 'power' 'แหล่งไฟ' 'ok' 'เสียบสายชาร์จอยู่' '' }
        }
    } catch { }

    # Memory Integrity (Core Isolation)
    try {
        $dg = Get-CimInstance -Namespace root\Microsoft\Windows\DeviceGuard -ClassName Win32_DeviceGuard -ErrorAction Stop
        $hvci = @($dg.SecurityServicesRunning) -contains 2
        $locked = (Get-Reg 'HKLM:\SYSTEM\CurrentControlSet\Control\DeviceGuard\Scenarios\HypervisorEnforcedCodeIntegrity' 'Locked') -eq 1
        if ($hvci) { Add 'hvci' 'Memory Integrity (Core Isolation)' 'warn' 'เปิดอยู่ บางเครื่องเสีย FPS 5-15%' $(if ($locked) { 'ถูกล็อกไว้จาก UEFI/องค์กร ต้องปิดใน Windows Security > Device security > Core isolation' } else { 'ปิดได้ในหน้าปรับแต่ง "ปิด Memory Integrity" แล้วรีสตาร์ท (ความปลอดภัยของเครื่องจะลดลงเล็กน้อย)' }) }
        else { Add 'hvci' 'Memory Integrity (Core Isolation)' 'ok' 'ปิดอยู่ ไม่กิน FPS' '' }
    } catch { }

    # overlay ที่ทำให้เฟรมสะดุด
    try {
        $names = @(Get-Process -ErrorAction SilentlyContinue | ForEach-Object { $_.ProcessName.ToLower() })
        $ov = New-Object System.Collections.Generic.List[string]; $tips = New-Object System.Collections.Generic.List[string]
        if ($names -contains 'nvidia overlay' -or $names -contains 'nvidia share') { $ov.Add('NVIDIA Overlay'); $tips.Add('NVIDIA app/GeForce Experience > ปิด In-game overlay') }
        if ($names -contains 'discord' -or $names -contains 'discordptb' -or $names -contains 'discordcanary') { $ov.Add('Discord'); $tips.Add('Discord > User Settings > Game Overlay > ปิด Enable in-game overlay') }
        if ($names -contains 'steam') { $ov.Add('Steam'); $tips.Add('Steam > Settings > In Game > ปิด Steam Overlay') }
        if ($names -contains 'radeonsoftware' -or $names -contains 'amdrsserv') { $ov.Add('AMD Adrenalin'); $tips.Add('Adrenalin > Preferences > ปิด In-Game Overlay') }
        if ((Get-Reg 'HKCU:\Software\Microsoft\Windows\CurrentVersion\GameDVR' 'AppCaptureEnabled') -ne 0) { $ov.Add('Xbox Game Bar'); $tips.Add('หน้าปรับแต่ง > ปิดการอัดคลิปของ Xbox Game Bar') }
        if ($ov.Count) { Add 'overlay' 'Overlay ที่อาจเปิดอยู่' 'info' ($ov -join ', ') (($tips -join ' · ') + ' (ถ้าไม่ได้ใช้ ปิดไว้เฟรมนิ่งกว่า)') }
        else { Add 'overlay' 'Overlay' 'ok' 'ไม่เจอ overlay ที่เปิดอยู่' '' }
    } catch { }

    # power plan
    $act = Get-ActiveScheme
    if ($act -eq $SchemeGuid -or $act -eq $UltimateGuid -or $act -eq $HighPerfGuid) { Add 'plan' 'Power Plan' 'ok' $(if ($act -eq $SchemeGuid) { 'ใช้ ZAN Project Optimizer อยู่' } else { 'ใช้โหมดประสิทธิภาพสูงอยู่' }) '' }
    else { Add 'plan' 'Power Plan' 'warn' 'ยังใช้ Power Plan แบบประหยัด/สมดุล' 'เปิด "Power Plan: ZAN Project Optimizer" ในหน้าปรับแต่ง หรือกดบูสต์เครื่อง' }

    Emit @{ t = 'result'; action = 'Health'; items = @($r) }
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

function Do-ListApps {
    $procs = Get-Process -ErrorAction SilentlyContinue
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
        $ps = @(Get-Process -Name $id -ErrorAction SilentlyContinue)
        if ($ps.Count -eq 0) { continue }
        $mb += [math]::Round((($ps | Measure-Object WorkingSet64 -Sum).Sum) / 1MB)
        foreach ($p in $ps) { try { [void]$p.CloseMainWindow() } catch { } }
        Start-Sleep -Milliseconds 1500
        Get-Process -Name $id -ErrorAction SilentlyContinue | ForEach-Object { try { $_.Kill() } catch { } }
        Write-Log "ปิด $($KnownApps[$id])"
        $n++
    }
    ipconfig /flushdns | Out-Null
    Emit @{ t = 'result'; action = 'CloseApps'; ok = $true; n = $n; mb = $mb }
}

function Do-Report {
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    $tmp = Join-Path $env:TEMP "ZanBooster-report-$stamp"
    New-Item -ItemType Directory -Path $tmp -Force | Out-Null
    $Script:Capture = New-Object System.Collections.Generic.List[object]
    foreach ($fn in 'Do-SysInfo', 'Do-Status', 'Do-Health', 'Do-GfxPlan', 'Do-GfxStatus') { try { & $fn } catch { $Script:Capture.Add(@{ action = $fn; error = $_.Exception.Message }) } }
    $data = $Script:Capture; $Script:Capture = $null
    $os = Get-CimInstance Win32_OperatingSystem
    $report = [ordered]@{ created = (Get-Date).ToString('s'); app = $Opt; os = "$($os.Caption) $($os.Version)"; results = @($data) }
    $report | ConvertTo-Json -Depth 8 | Set-Content (Join-Path $tmp 'report.json') -Encoding UTF8
    foreach ($f in 'log.txt', 'backup.json', 'config.json', 'settings.json', 'remote.json') {
        $src = Join-Path $DataDir $f; if (Test-Path $src) { Copy-Item $src $tmp }
    }
    $fd = Get-FiveMData
    if ($fd) {
        $app = Split-Path $fd -Parent
        $ini = Join-Path $app 'CitizenFX.ini'; if (Test-Path $ini) { Copy-Item $ini $tmp }
        $logs = Join-Path $app 'logs'
        if (Test-Path $logs) { Get-ChildItem $logs -File | Sort-Object LastWriteTime -Descending | Select-Object -First 2 | Copy-Item -Destination $tmp }
    }
    if (Test-Path $GfxFile) { Copy-Item $GfxFile $tmp }
    $zip = Join-Path ([Environment]::GetFolderPath('Desktop')) "ZanBooster-report-$stamp.zip"
    Compress-Archive -Path (Join-Path $tmp '*') -DestinationPath $zip -Force
    Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
    Start-Process explorer.exe "/select,`"$zip`""
    Emit @{ t = 'result'; action = 'Report'; ok = $true; path = $zip }
}

function Do-Restore {
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
                Remove-ItemProperty -LiteralPath $e.Path -Name $e.Name -ErrorAction SilentlyContinue
            }
        } catch { Write-Log "คืนค่า $($e.Path)\$($e.Name) ไม่ได้: $($_.Exception.Message)" }
    }
    Write-Log "คืนค่า Registry $($b.Registry.Count) รายการ"
    foreach ($k in $b.CreatedKeys) { if (Test-Path -LiteralPath $k) { Remove-Item -LiteralPath $k -Recurse -Force -ErrorAction SilentlyContinue } }

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
    foreach ($n in $restart) { Restart-NetAdapter -Name $n -Confirm:$false -ErrorAction SilentlyContinue }

    if ((Test-Path $GfxBackup) -and -not (Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.Name -like 'FiveM*' })) {
        Copy-Item $GfxBackup $GfxFile -Force; Remove-Item $GfxBackup -Force; Write-Log 'คืนไฟล์กราฟิก FiveM เดิมแล้ว'
    }
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    if (Test-Path $BackupFile) { Move-Item $BackupFile "$BackupFile.restored-$stamp" -Force }
    $Script:Backup = New-EmptyBackup
    Emit @{ t = 'result'; action = 'Restore'; ok = $true }
}

function Do-CleanCache {
    if (Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.Name -like 'FiveM*' }) {
        Emit @{ t = 'result'; action = 'CleanCache'; ok = $false; m = 'กรุณาปิด FiveM ก่อนล้างแคช' }; return
    }
    $FiveMData = Get-FiveMData
    if (-not $FiveMData) { Emit @{ t = 'result'; action = 'CleanCache'; ok = $false; notfound = $true; m = 'หาโฟลเดอร์ FiveM ไม่เจอ' }; return }
    Write-Log "FiveM: $(Split-Path (Split-Path $FiveMData -Parent) -Parent)"
    if (-not (Test-Path $FiveMData)) { Emit @{ t = 'result'; action = 'CleanCache'; ok = $true; mb = 0 }; return }
    $total = [int64]0
    foreach ($d in 'server-cache', 'server-cache-priv') {
        $p = Join-Path $FiveMData $d
        if (Test-Path $p) { $total += Get-FolderSize $p; Remove-Item $p -Recurse -Force -ErrorAction SilentlyContinue; Write-Log "ลบ $d" }
    }
    $cache = Join-Path $FiveMData 'cache'
    if (Test-Path $cache) {
        Get-ChildItem $cache -Force -ErrorAction SilentlyContinue | Where-Object { $_.Name -ne 'game' } | ForEach-Object {
            $total += $(if ($_.PSIsContainer) { Get-FolderSize $_.FullName } else { $_.Length })
            Remove-Item $_.FullName -Recurse -Force -ErrorAction SilentlyContinue
        }
        Write-Log 'ลบ cache (เก็บ cache\game ไว้)'
    }
    foreach ($d in 'crashes', 'logs') {
        $p = Join-Path (Split-Path $FiveMData -Parent) $d
        if (Test-Path $p) { $total += Get-FolderSize $p; Get-ChildItem $p -Force | Remove-Item -Recurse -Force -ErrorAction SilentlyContinue }
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
        Get-ChildItem $p -Force -ErrorAction SilentlyContinue | ForEach-Object {
            $size = $(if ($_.PSIsContainer) { Get-FolderSize $_.FullName } else { $_.Length })
            try { Remove-Item $_.FullName -Recurse -Force -ErrorAction Stop; $total += $size } catch { }
        }
    }
    ipconfig /flushdns | Out-Null
    Emit @{ t = 'result'; action = 'CleanTemp'; ok = $true; mb = [math]::Round($total / 1MB) }
}

# ---------------------------------------------------------------- junk ----
# ล้างขยะทั้งเครื่อง: แต่ละหมวดคืนรายการ "โฟลเดอร์ที่ลบไฟล์ข้างใน" กับ "ไฟล์เดี่ยว"
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
        'thumbs'  { $files = @(Get-ChildItem "$L\Microsoft\Windows\Explorer" -Filter '*cache_*.db' -Force -ErrorAction SilentlyContinue | ForEach-Object FullName) }
        'browser' {
            foreach ($root in "$L\Google\Chrome\User Data", "$L\Microsoft\Edge\User Data", "$L\BraveSoftware\Brave-Browser\User Data", "$R\Opera Software\Opera Stable", "$L\Opera Software\Opera Stable", "$R\Opera Software\Opera GX Stable", "$L\Opera Software\Opera GX Stable") {
                if (-not (Test-Path -LiteralPath $root)) { continue }
                $profiles = @($root) + @(Get-ChildItem -LiteralPath $root -Directory -Force -ErrorAction SilentlyContinue | Where-Object { $_.Name -eq 'Default' -or $_.Name -like 'Profile *' } | ForEach-Object FullName)
                foreach ($pr in $profiles) { foreach ($c in 'Cache', 'Code Cache', 'GPUCache', 'Service Worker\CacheStorage') { $dirs += (Join-Path $pr $c) } }
                $dirs += (Join-Path $root 'ShaderCache'); $dirs += (Join-Path $root 'GrShaderCache')
            }
            $dirs += @(Get-ChildItem "$L\Mozilla\Firefox\Profiles" -Directory -Force -ErrorAction SilentlyContinue | ForEach-Object { Join-Path $_.FullName 'cache2' })
        }
        'apps'    {
            foreach ($d in 'discord', 'discordptb', 'discordcanary') { foreach ($c in 'Cache', 'Code Cache', 'GPUCache') { $dirs += "$R\$d\$c" } }
            $dirs += @("$L\Steam\htmlcache", "$L\Spotify\Data", "$R\Spotify\Data", "$L\Microsoft\Teams\Cache", "$R\Microsoft\Teams\Cache")
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
    foreach ($it in @(Get-ChildItem -LiteralPath $d -Force -ErrorAction SilentlyContinue)) {
        if ($it.PSIsContainer) {
            $sz = Get-FolderSize $it.FullName
            try { Remove-Item -LiteralPath $it.FullName -Recurse -Force -ErrorAction Stop; $freed += $sz }
            catch { $freed += [math]::Max(0, $sz - (Get-FolderSize $it.FullName)) } # ไฟล์ที่เปิดใช้อยู่ข้ามไป
        } else {
            try { $sz = $it.Length; Remove-Item -LiteralPath $it.FullName -Force -ErrorAction Stop; $freed += $sz } catch { }
        }
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
                'recycle' { $b = Get-RecycleSize; Clear-RecycleBin -Force -ErrorAction SilentlyContinue; $freed = [math]::Max(0, $b - (Get-RecycleSize)) }
                'wu' {
                    $tg = Get-JunkTargets $id
                    $svc = @('wuauserv', 'bits') | Where-Object { (Get-Service $_ -ErrorAction SilentlyContinue).Status -eq 'Running' }
                    foreach ($x in $svc) { Stop-Service $x -Force -ErrorAction SilentlyContinue }
                    foreach ($d in $tg.Dirs) { $freed += Remove-DirContents $d }
                    foreach ($x in $svc) { Start-Service $x -ErrorAction SilentlyContinue }
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
    Emit @{ t = 'result'; action = 'JunkClean'; ok = $true; mb = [math]::Round($total / 1MB); per = $per }
}

function Do-BoostNow {
    $procs = @(Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.Name -like 'FiveM*GTAProcess' })
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
        'BoostNow'   { Do-BoostNow }
        'PickFiveM'  { Do-PickFiveM }
        'Health'     { Do-Health }
        'GfxStatus'  { Do-GfxStatus }
        'GfxPlan'    { Do-GfxPlan }
        'Report'     { Do-Report }
        'GfxApply'   { Do-GfxApply }
        'GfxRestore' { Do-GfxRestore }
        'ListApps'   { Do-ListApps }
        'CloseApps'  { Do-CloseApps }
        default      { Emit @{ t = 'error'; m = "unknown action $Action" } }
    }
} catch {
    Emit @{ t = 'error'; action = $Action; m = $_.Exception.Message }
}
