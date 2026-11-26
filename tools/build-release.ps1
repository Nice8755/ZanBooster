# Build a release exe from recovered/ui.html + recovered/engine.ps1 on top of the v4.11.15 binary.
#   powershell -File tools/build-release.ps1 -Version 4.11.18 -Out C:\path\ZanBooster.exe
# The browser mock (only used when previewing ui.html in a normal browser) is stripped from the
# release copy to free space in the fixed-size slot inside the exe.
param([Parameter(Mandatory)][string]$Version, [Parameter(Mandatory)][string]$Out)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$utf8 = [Text.UTF8Encoding]::new($false)
$ui = [IO.File]::ReadAllText((Join-Path $root 'recovered\ui.html'), $utf8).Replace("`r`n", "`n")
$a = $ui.IndexOf("if (!NATIVE) {`n"); $b = $ui.IndexOf("`n}`n`n/* ================= data")
if ($a -lt 0 -or $b -lt $a) { throw 'mock block markers not found' }
$ui = $ui.Substring(0, $a) + "if (!NATIVE) {" + $ui.Substring($b + 1)
# strip indentation and comment-only lines (release copy only)
$ui = ($ui.Split("`n") | ForEach-Object { $_.TrimStart(' ') } | Where-Object { $_ -ne '' -and $_ -notmatch '^//' -and $_ -notmatch '^/\*[^*]*\*/$' }) -join "`n"
# drop Thai comments at the end of script lines after ; { } ) , when the code part has no open quote (release copy only, node checks it below)
$ui = ($ui.Split("`n") | ForEach-Object {
    $m = [regex]::Match($_, '^(.*[;{}),])\s+// [^\n]*[\u0E00-\u0E7F][^\n]*$')
    if ($m.Success -and -not ((@($m.Groups[1].Value.ToCharArray() | Where-Object { $_ -in [char]39, [char]34, [char]96 } | Group-Object | Where-Object { $_.Count % 2 }).Count))) { $m.Groups[1].Value } else { $_ }
}) -join "`n"
# compact the stylesheet: comments, line breaks, spaces around braces (release copy only)
$s1 = $ui.IndexOf('<style>') + 7; $s2 = $ui.IndexOf('</style>', $s1); if ($s1 -lt 7 -or $s2 -lt $s1) { throw 'style block not found' }
$css = [regex]::Replace($ui.Substring($s1, $s2 - $s1), '(?s)/\*.*?\*/', '')
$css = [regex]::Replace([regex]::Replace([regex]::Replace($css, ';?\s*\}\s*', '}'), '\s*\{\s*', '{'), ';\s+', ';')
$ui = $ui.Substring(0, $s1) + $css + $ui.Substring($s2)
# graphics preview images: base85 instead of base64 (same bytes, about 24 KB smaller), decoded by Z8() at startup
$abc = -join ([char[]](33..125) | Where-Object { '"\<&''`$%'.IndexOf($_) -lt 0 })
if ($abc.Length -ne 85) { throw "alphabet $($abc.Length)" }
$p1 = $ui.IndexOf('const ZP_PREV = {'); $p2 = $ui.IndexOf("`n", $p1); if ($p1 -lt 0) { throw 'ZP_PREV not found' }
$sb = [Text.StringBuilder]::new(); $at = $p1
foreach ($m in [regex]::Matches($ui.Substring($p1, $p2 - $p1), '"data:image/jpeg;base64,([A-Za-z0-9+/=]+)"')) {
    $raw = [Convert]::FromBase64String($m.Groups[1].Value); $len = $raw.Length
    [void]$sb.Append($ui, $at, $p1 + $m.Index - $at).Append('Z8("'); $at = $p1 + $m.Index + $m.Length
    $buf = [byte[]]::new([int][math]::Ceiling($len / 4) * 4); [Array]::Copy($raw, $buf, $len)
    for ($i = 0; $i -lt $buf.Length; $i += 4) {
        [long]$v = [long]$buf[$i] * 16777216 + [long]$buf[$i + 1] * 65536 + [long]$buf[$i + 2] * 256 + $buf[$i + 3]
        $c = [char[]]::new(5); for ($j = 4; $j -ge 0; $j--) { $c[$j] = $abc[[int]($v % 85)]; $v = [long][math]::Floor([decimal]$v / 85) }
        [void]$sb.Append($c)
    }
    [void]$sb.Append("`",$len)")
}
$dec = 'const Z8=(s,n)=>{const A="' + $abc + '";let o="";for(let i=0;i<s.length;i+=5){let v=0;for(let j=0;j<5;j++)v=v*85+A.indexOf(s[i+j]);o+=String.fromCharCode(v>>>24,v>>>16&255,v>>>8&255,v&255)}return"data:image/jpeg;base64,"+btoa(o.slice(0,n))};' + "`n"
$ui = $ui.Substring(0, $p1) + $dec + $sb.ToString() + $ui.Substring($at)
# drop the oldest changelog entries from the release copy until it fits the slot
$cut = 0
while ($utf8.GetByteCount($ui) -gt 1054600) { $end = $ui.IndexOf(']</script>', $ui.IndexOf('id="changelog"')); $st = $ui.LastIndexOf(', {"v": "', $end); if ($st -lt 0) { throw 'cannot fit' }; $ui = $ui.Remove($st, $end - $st); $cut++ }
"changelog entries dropped: $cut"
if (Get-Command node -EA 0) {
    $k = 0
    foreach ($sm in [regex]::Matches($ui, '<script>(.*?)</script>', 'Singleline')) { $k++; $js = Join-Path $env:TEMP "zb-release-s$k.js"; [IO.File]::WriteAllText($js, $sm.Groups[1].Value, $utf8); & node --check $js; if ($LASTEXITCODE) { throw "release ui script $k has a syntax error" } }
    if ($k -lt 1) { throw 'no script found in release ui' }
    "release ui scripts ok ($k)"
}
$tmp = Join-Path $env:TEMP 'zb-release-ui.html'
[IO.File]::WriteAllText($tmp, $ui, $utf8)
"ui.html release size: {0:N0} bytes (slot 1,055,130)" -f $utf8.GetByteCount($ui)
$eng = [IO.File]::ReadAllText((Join-Path $root 'recovered\engine.ps1'), $utf8).Replace("`r`n", "`n")
$min = ($eng.Split("`n") | ForEach-Object { $_.Trim() } | Where-Object { $_ -notmatch '^#' }) -join "`n"
# drop comments at the end of code lines too (the token check below proves the code is unchanged)
$ct = $null; $ce = $null; [void][Management.Automation.Language.Parser]::ParseInput($min, [ref]$ct, [ref]$ce)
foreach ($k in @($ct | Where-Object { $_.Kind -eq 'Comment' -and $_.Text -notlike '<#*' } | Sort-Object { $_.Extent.StartOffset } -Descending)) { $min = $min.Remove($k.Extent.StartOffset, $k.Extent.EndOffset - $k.Extent.StartOffset) }
$min = ($min.Split("`n") | ForEach-Object { $_.TrimEnd() }) -join "`n"
# squeeze spaces next to { } @{ ; = | , (release copy only, same token check below proves the code is unchanged)
# never next to a token that starts with '-' (parameters, -eq, -and ...) so nothing can glue onto them
$tk = $null; $pe = $null; [void][Management.Automation.Language.Parser]::ParseInput($min, [ref]$tk, [ref]$pe)
$sq = [Text.StringBuilder]::new($min)
for ($i = $tk.Count - 2; $i -ge 1; $i--) {
    $t = $tk[$i]; if ("$($t.Kind)" -notin 'LCurly', 'RCurly', 'AtCurly', 'Semi', 'Equals', 'Pipe', 'Comma') { continue }
    $e = $t.Extent.EndOffset; $n = 0; while ($e + $n -lt $sq.Length -and $sq[$e + $n] -eq ' ') { $n++ }
    if ($n -and -not $tk[$i + 1].Text.StartsWith('-')) { [void]$sq.Remove($e, $n) }
    $s = $t.Extent.StartOffset; $n = 0; while ($s - $n -gt 0 -and $sq[$s - $n - 1] -eq ' ') { $n++ }
    $pv = $tk[$i - 1]; $glue = "$($t.Kind)" -in 'LCurly', 'AtCurly' -and "$($pv.Kind)" -notin 'RParen', 'Equals', 'Semi', 'Pipe', 'Comma', 'LCurly', 'RCurly', 'AtCurly' -and -not ($pv.TokenFlags -band [Management.Automation.Language.TokenFlags]::Keyword)
    if ($n -and -not $glue -and -not $pv.Text.StartsWith('-')) { [void]$sq.Remove($s - $n, $n) }
}
$min = $sq.ToString()
# long command names -> built-in Windows PowerShell 5.1 aliases (only tokens used as a command; the token check below maps them back)
$als = @{ 'ForEach-Object' = '%'; 'Where-Object' = '?'; 'Select-Object' = 'select'; 'Measure-Object' = 'measure'; 'Get-ChildItem' = 'gci'; 'Get-Content' = 'gc'; 'Get-Process' = 'gps'; 'Get-ItemProperty' = 'gp'; 'Set-ItemProperty' = 'sp'; 'Remove-Item' = 'ri'; 'Sort-Object' = 'sort'; 'New-Item' = 'ni'; 'Copy-Item' = 'cpi'; 'Start-Sleep' = 'sleep'; 'Get-CimInstance' = 'gcim'; 'Get-Item' = 'gi'; 'Remove-ItemProperty' = 'rp'; 'Start-Process' = 'saps'; 'Get-Service' = 'gsv'; 'Group-Object' = 'group'; 'Add-Content' = 'ac'; 'Invoke-RestMethod' = 'irm'; 'Invoke-WebRequest' = 'iwr' }
$tk = $null; $pe = $null; [void][Management.Automation.Language.Parser]::ParseInput($min, [ref]$tk, [ref]$pe)
$sq = [Text.StringBuilder]::new($min)
foreach ($t in @($tk | Where-Object { ($_.TokenFlags -band [Management.Automation.Language.TokenFlags]::CommandName) -and $als.ContainsKey($_.Text) } | Sort-Object { $_.Extent.StartOffset } -Descending)) {
    [void]$sq.Remove($t.Extent.StartOffset, $t.Extent.EndOffset - $t.Extent.StartOffset).Insert($t.Extent.StartOffset, $als[$t.Text])
}
$min = $sq.ToString(); $back = @{}; foreach ($k in $als.Keys) { $back[$als[$k]] = $k }
# the engine's own function names -> short names (definitions + calls only, never inside strings; the token check maps them back)
$tk = $null; $pe = $null; $fa = [Management.Automation.Language.Parser]::ParseInput($min, [ref]$tk, [ref]$pe)
$fmap = @{}; $fi = 0; foreach ($fd in $fa.FindAll({ $args[0] -is [Management.Automation.Language.FunctionDefinitionAst] }, $true)) { if (-not $fmap.ContainsKey($fd.Name) -and $fd.Name.Length -gt 3) { $fmap[$fd.Name] = 'z' + $fi; $fi++ } }
$sq = [Text.StringBuilder]::new($min)
for ($i = $tk.Count - 1; $i -ge 0; $i--) {
    $t = $tk[$i]; if (-not $fmap.ContainsKey($t.Text)) { continue }
    $isDef = $i -gt 0 -and "$($tk[$i - 1].Kind)" -eq 'Function'; $isCall = [bool]($t.TokenFlags -band [Management.Automation.Language.TokenFlags]::CommandName)
    if ($isDef -or $isCall) { [void]$sq.Remove($t.Extent.StartOffset, $t.Extent.EndOffset - $t.Extent.StartOffset).Insert($t.Extent.StartOffset, $fmap[$t.Text]) }
}
$min = $sq.ToString(); $fback = @{}; foreach ($k in $fmap.Keys) { $fback[$fmap[$k]] = $k }
"engine functions renamed: $fi"
"engine release size: {0:N0} bytes (slot 117,970)" -f $utf8.GetByteCount($min)
$ta = $null; $tb = $null; $pe = $null
[void][Management.Automation.Language.Parser]::ParseInput($eng, [ref]$ta, [ref]$pe); if ($pe.Count) { throw "engine: $($pe[0])" }
[void][Management.Automation.Language.Parser]::ParseInput($min, [ref]$tb, [ref]$pe); if ($pe.Count) { throw "engine min: $($pe[0])" }
$sig = { param($t) ($t | Where-Object { $_.Kind -notin 'NewLine', 'Comment' } | ForEach-Object { $x = $_.Text; if ("$($_.Kind)" -notmatch 'String' -and $fback.ContainsKey($x)) { $x = $fback[$x] }; if ("$($_.Kind)" -notmatch 'String' -and $fmap.ContainsKey($x)) { "Fn:$($x.ToLower())" } elseif (($_.TokenFlags -band [Management.Automation.Language.TokenFlags]::CommandName) -and $back.ContainsKey($x)) { "Cmd:$($back[$x])" } elseif ($_.TokenFlags -band [Management.Automation.Language.TokenFlags]::CommandName -and $als.ContainsKey($x)) { "Cmd:$x" } else { "$($_.Kind):$x" } }) -join "`n" }
$sa = (& $sig $ta) -split "`n"; $sb = (& $sig $tb) -split "`n"
if (($sa -join "`n") -ne ($sb -join "`n")) { for ($q = 0; $q -lt [math]::Max($sa.Count, $sb.Count); $q++) { if ($sa[$q] -ne $sb[$q]) { "first difference at token $q :"; "  source : $($sa[[math]::Max(0,$q-2)..($q+2)] -join ' | ')"; "  release: $($sb[[math]::Max(0,$q-2)..($q+2)] -join ' | ')"; break } }; throw 'engine: stripping indentation changed the code' }
$engTmp = Join-Path $env:TEMP 'zb-release-engine.ps1'
[IO.File]::WriteAllText($engTmp, $min, $utf8)
# the squeezed copy must still answer the read-only actions on Windows PowerShell
if (Get-Command powershell.exe -EA 0) {
    $engRun = Join-Path $env:TEMP 'zb-release-engine-run.ps1'; [IO.File]::WriteAllText($engRun, $min, [Text.UTF8Encoding]::new($true))
    foreach ($a in 'Status', 'SysInfo', 'Health', 'FpsScan', 'GfxPlan', 'GfxStatus', 'Aff', 'Heat', 'Dup') {
        $l = [string](& powershell.exe -NoProfile -ExecutionPolicy Bypass -File $engRun -Action $a | Where-Object { $_ -match '"t":"(result|error)"' } | Select-Object -Last 1)
        if (-not $l -or $l -match '"t":"error"') { throw "release engine action $a failed: $l" }
    }
    'release engine actions ok'
}
$base = Join-Path $root 'beta\ZanBooster.exe'
if ((Get-FileHash $base).Hash -ne '34CBF139FB3D8811FD8AFD8A363B5E7DC86781067AC4F8DEB0FA8EEC8EB7F6F8') { throw 'beta\ZanBooster.exe on this branch must be the original v4.11.15 binary' }
$fxTmp = Join-Path $env:TEMP 'zb-release-shader.fx'
& (Join-Path $PSScriptRoot 'shader-patch.ps1') -Exe $base -Out $fxTmp
$patch = Join-Path $env:TEMP 'patchexe-2.exe'
Push-Location $root
try {
    if (1) { go build -mod=vendor -o $patch ./tools/patchexe; if ($LASTEXITCODE) { throw 'patchexe build failed' } }
    & $patch (Join-Path $root 'beta\ZanBooster.exe') $Out "ver=4.11.15:$Version" "ui=$tmp" "engine=$engTmp" "shader=$fxTmp"
    if ($LASTEXITCODE) { throw 'patch failed' }
} finally { Pop-Location }
(Get-FileHash $Out).Hash.ToLower()
