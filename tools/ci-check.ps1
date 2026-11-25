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

# 1d) tray helper + Dup on real windows (notepad stands in for the app window)
if ($env:GITHUB_ACTIONS) {
    $ast = [Management.Automation.Language.Parser]::ParseFile($f, [ref]$null, [ref]$null)
    $cs = { param($w) ($ast.FindAll({ $args[0] -is [Management.Automation.Language.StringConstantExpressionAst] -and $args[0].Value -like $w }, $true) | Select-Object -First 1).Value }
    $tray = & $cs 'Add-Type -AssemblyName System.Windows.Forms*'; $zw = & $cs '`[DllImport("user32")`]public static extern bool ShowWindowAsync*'
    if (-not $tray -or -not $zw) { throw 'tray script not found in engine' }
    Add-Type -Name W -Namespace Z -MemberDefinition $zw
    $vis = { param($h) [Z.W]::IsWindowVisible([IntPtr]$h) }; $ico = { param($h) [Z.W]::IsIconic([IntPtr]$h) }
    $np = Start-Process notepad.exe -PassThru; Start-Sleep 3; $np.Refresh(); $h = [int64]$np.MainWindowHandle
    if (-not $h) { '  tray: no test window on this runner, skipped' } else {
        $s = $tray.Replace('ZBW', $zw).Replace('ZBH', "$h").Replace('ZBE', $np.Path.Replace("'", "''"))
        $tp = Start-Process powershell.exe -PassThru -WindowStyle Hidden -ArgumentList '-NoProfile', '-STA', '-EncodedCommand', ([Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($s)))
        $min = $false; for ($i = 0; $i -lt 40 -and -not $min; $i++) { Start-Sleep -Milliseconds 250; $min = & $ico $h }
        Start-Sleep 1; $alive = -not $tp.HasExited; $stillVis = & $vis $h
        "  tray: minimized=$min, window not hidden=$stillVis, helper running=$alive"
        [void][Z.W]::ShowWindowAsync([IntPtr]$h, 9); $gone = $tp.WaitForExit(6000)
        "  tray: window restored, helper quit by itself=$gone"
        if (-not $gone) { try { $tp.Kill() } catch { } }
        try { $np.Kill() } catch { }
        if (-not ($min -and $stillVis -and $alive -and $gone)) { throw 'tray helper test failed' }
        $fake = Join-Path $work 'notepad.exe'; Copy-Item "$env:WINDIR\System32\WindowsPowerShell\v1.0\powershell.exe" $fake
        $dup = { $l = [string](& $fake -NoProfile -Command "& powershell.exe -NoProfile -ExecutionPolicy Bypass -File '$f' -Action Dup" | Where-Object { $_ -match '"t":"result"' } | Select-Object -Last 1); $l }
        # minimized copy -> brought back
        $np = Start-Process notepad.exe -PassThru; Start-Sleep 6; $np.Refresh(); $h = [int64]$np.MainWindowHandle; [void][Z.W]::ShowWindowAsync([IntPtr]$h, 6); Start-Sleep 1
        $l = & $dup; Start-Sleep 1; $back = -not (& $ico $h); try { $np.Kill() } catch { }
        "  dup (minimized copy): $l, restored=$back"; if ($l -notmatch '"found":true' -or -not $back) { throw 'dup minimized test failed' }
        # copy hidden by an older version -> closed so the new one can open
        $np = Start-Process notepad.exe -PassThru; Start-Sleep 6; $np.Refresh(); $h = [int64]$np.MainWindowHandle; [void][Z.W]::ShowWindowAsync([IntPtr]$h, 0); Start-Sleep 1
        [IO.File]::WriteAllText("$env:TEMP\zbtray", "$($np.Id):$h")
        $l = & $dup; $closed = $np.WaitForExit(3000); Remove-Item "$env:TEMP\zbtray" -EA 0
        "  dup (old hidden copy): $l, closed=$closed"; if ($l -notmatch '"found":false' -or -not $closed) { throw 'dup hidden test failed' }
        # copy open on screen -> left alone
        $np = Start-Process notepad.exe -PassThru; Start-Sleep 6
        $l = & $dup; $kept = -not $np.HasExited; try { $np.Kill() } catch { }
        "  dup (copy on screen): $l, left running=$kept"; if ($l -notmatch '"found":false' -or -not $kept) { throw 'dup on-screen test failed' }
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
