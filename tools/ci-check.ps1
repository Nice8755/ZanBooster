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
