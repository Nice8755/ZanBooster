# Checks that run on GitHub's Windows machines (and locally). Read-only: never applies a tweak.
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$utf8 = [Text.UTF8Encoding]::new($false)
$work = Join-Path ([IO.Path]::GetTempPath()) ('zbci-' + [guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($work)

# 1) engine parses on Windows PowerShell and its read-only actions answer
$eng = [IO.File]::ReadAllText((Join-Path $root 'recovered\engine.ps1'), $utf8)
$t = $null; $e = $null
[void][Management.Automation.Language.Parser]::ParseInput($eng, [ref]$t, [ref]$e)
if ($e.Count) { throw "engine.ps1: $($e[0])" }
$f = Join-Path $work 'engine.ps1'
[IO.File]::WriteAllText($f, $eng, [Text.UTF8Encoding]::new($true))
foreach ($a in 'Status', 'SysInfo', 'Health', 'FpsScan', 'GfxPlan', 'GfxStatus', 'Aff', 'Heat') {
    $o = & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $f -Action $a
    $l = [string]($o | Where-Object { $_ -match '"t":"(result|error)"' } | Select-Object -Last 1)
    if (-not $l -or $l -match '"t":"error"') { throw "engine action $a failed: $l" }
    "engine $a ok"
    if ($a -in 'Aff', 'Heat') { "  $($l.Substring(0, [math]::Min(400, $l.Length)))" }
}

# 1b) GitHub's throwaway machine only: CPU-core startup task is created, its script parses, and it is removed again
if ($env:GITHUB_ACTIONS) {
    $run = { param($i, $o) [string](& powershell.exe -NoProfile -ExecutionPolicy Bypass -File $f -Action Aff -Ids $i -Opt $o | Where-Object { $_ -match '"t":"(result|error)"' } | Select-Object -Last 1) }
    $l = & $run 'set' 'explorer|1'; $l = & $run 'boot' '1'
    if ($l -notmatch '"task":true') { throw "startup task not created: $l" }
    $bs = Join-Path $env:ProgramData 'ZanBooster\aff-boot.ps1'; $t2 = $null; $e2 = $null
    [void][Management.Automation.Language.Parser]::ParseInput([IO.File]::ReadAllText($bs), [ref]$t2, [ref]$e2); if ($e2.Count) { throw "aff-boot.ps1: $($e2[0])" }
    $l = & $run 'del' 'explorer'
    if ($l -notmatch '"task":false' -or (Test-Path $bs)) { throw "startup task not removed: $l" }
    'cpu-core startup task ok'
}

# 1c) YouTube search (needs internet; only reported, never fails the build)
if ($env:GITHUB_ACTIONS) {
    $l = [string](& powershell.exe -NoProfile -ExecutionPolicy Bypass -File $f -Action YtSearch -Opt 'ariana grande' | Where-Object { $_ -match '"t":"result"' } | Select-Object -Last 1)
    "  yt search: $($l.Substring(0, [math]::Min(300, $l.Length)))"
}

# 1d) tray helper (Tray action): hides a real window, puts an icon in the tray, quits when the window comes back
if ($env:GITHUB_ACTIONS) {
    $ast = [Management.Automation.Language.Parser]::ParseFile($f, [ref]$null, [ref]$null)
    $str = $ast.FindAll({ $args[0] -is [Management.Automation.Language.StringConstantExpressionAst] -and $args[0].Value -like 'Add-Type -AssemblyName System.Windows.Forms*' }, $true) | Select-Object -First 1
    if (-not $str) { throw 'tray script not found in engine' }
    Add-Type -Name V -Namespace CiTray -MemberDefinition '[DllImport("user32")]public static extern bool IsWindowVisible(IntPtr h);[DllImport("user32")]public static extern bool ShowWindow(IntPtr h,int c);'
    $np = Start-Process notepad.exe -PassThru; Start-Sleep 3; $np.Refresh(); $h = [int64]$np.MainWindowHandle
    if (-not $h) { '  tray: no test window on this runner, skipped' } else {
        $zw = ($ast.FindAll({ $args[0] -is [Management.Automation.Language.StringConstantExpressionAst] -and $args[0].Value -like '`[DllImport("user32")`]public static extern bool ShowWindow*' }, $true) | Select-Object -First 1).Value
        $s = $str.Value.Replace('ZBW', $zw).Replace('ZBH', "$h").Replace('ZBE', $np.Path.Replace("'", "''"))
        $tp = Start-Process powershell.exe -PassThru -WindowStyle Hidden -ArgumentList '-NoProfile', '-STA', '-EncodedCommand', ([Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($s)))
        $hid = $false; for ($i = 0; $i -lt 40 -and -not $hid; $i++) { Start-Sleep -Milliseconds 250; $hid = -not [CiTray.V]::IsWindowVisible([IntPtr]$h) }
        $alive = -not $tp.HasExited
        "  tray: window hidden=$hid, helper running=$alive"
        [void][CiTray.V]::ShowWindow([IntPtr]$h, 9); $gone = $tp.WaitForExit(6000)
        "  tray: window shown again, helper quit by itself=$gone"
        if (-not $gone) { try { $tp.Kill() } catch { } }
        try { $np.Kill() } catch { }
        if (-not ($hid -and $alive -and $gone)) { throw 'tray helper test failed' }
        # Dup: a second copy (same process name, started later) must bring the hidden first copy back
        $np = Start-Process notepad.exe -PassThru; Start-Sleep 3; $np.Refresh(); $h = [int64]$np.MainWindowHandle
        [void][CiTray.V]::ShowWindow([IntPtr]$h, 0); [IO.File]::WriteAllText("$env:TEMP\zbtray", "$($np.Id):$h")
        $fake = Join-Path $work 'notepad.exe'; Copy-Item "$env:WINDIR\System32\WindowsPowerShell\v1.0\powershell.exe" $fake
        $l = [string](& $fake -NoProfile -Command "& powershell.exe -NoProfile -ExecutionPolicy Bypass -File '$f' -Action Dup" | Where-Object { $_ -match '"t":"result"' } | Select-Object -Last 1)
        $back = [CiTray.V]::IsWindowVisible([IntPtr]$h); try { $np.Kill() } catch { }; Remove-Item "$env:TEMP\zbtray" -EA 0
        "  dup: $l, first copy shown again=$back"
        if ($l -notmatch '"found":true' -or -not $back) { throw 'dup test failed' }
    }
}

# 2) ui.html scripts have no syntax error, changelog is valid JSON
$ui = [IO.File]::ReadAllText((Join-Path $root 'recovered\ui.html'), $utf8)
$cl = [regex]::Match($ui, 'id="changelog">(.*?)</script>', 'Singleline').Groups[1].Value
$n = @($cl | ConvertFrom-Json).Count; if ($n -lt 1) { throw 'changelog empty' }
"changelog ok ($n)"
$node = Get-Command node -ErrorAction SilentlyContinue
if ($node) {
    $i = 0
    foreach ($m in [regex]::Matches($ui, '<script>(.*?)</script>', 'Singleline')) {
        $i++; $js = Join-Path $work "s$i.js"; [IO.File]::WriteAllText($js, $m.Groups[1].Value, $utf8)
        & node --check $js; if ($LASTEXITCODE) { throw "ui.html script $i has a syntax error" }
    }
    if ($i -lt 1) { throw 'no script found in ui.html' }
    "ui scripts ok ($i)"
} else { 'node not installed: ui syntax check skipped' }
[IO.Directory]::Delete($work, $true)
'ALL CHECKS OK'
