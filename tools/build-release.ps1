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
$tmp = Join-Path $env:TEMP 'zb-release-ui.html'
[IO.File]::WriteAllText($tmp, $ui, $utf8)
"ui.html release size: {0:N0} bytes (slot 1,055,130)" -f $utf8.GetByteCount($ui)
$eng = [IO.File]::ReadAllText((Join-Path $root 'recovered\engine.ps1'), $utf8).Replace("`r`n", "`n")
$min = ($eng.Split("`n") | ForEach-Object { $_.Trim() } | Where-Object { $_ -notmatch '^#' }) -join "`n"
$ta = $null; $tb = $null; $pe = $null
[void][Management.Automation.Language.Parser]::ParseInput($eng, [ref]$ta, [ref]$pe); if ($pe.Count) { throw "engine: $($pe[0])" }
[void][Management.Automation.Language.Parser]::ParseInput($min, [ref]$tb, [ref]$pe); if ($pe.Count) { throw "engine min: $($pe[0])" }
$sig = { param($t) ($t | Where-Object { $_.Kind -notin 'NewLine', 'Comment' } | ForEach-Object { "$($_.Kind):$($_.Text)" }) -join "`n" }
if ((& $sig $ta) -cne (& $sig $tb)) { throw 'engine: stripping indentation changed the code' }
$engTmp = Join-Path $env:TEMP 'zb-release-engine.ps1'
[IO.File]::WriteAllText($engTmp, $min, $utf8)
$base = Join-Path $root 'beta\ZanBooster.exe'
if ((Get-FileHash $base).Hash -ne '34CBF139FB3D8811FD8AFD8A363B5E7DC86781067AC4F8DEB0FA8EEC8EB7F6F8') { throw 'beta\ZanBooster.exe on this branch must be the original v4.11.15 binary' }
$patch = Join-Path $env:TEMP 'patchexe.exe'
Push-Location $root
try {
    if (-not (Test-Path $patch)) { go build -mod=vendor -o $patch ./tools/patchexe; if ($LASTEXITCODE) { throw 'patchexe build failed' } }
    & $patch (Join-Path $root 'beta\ZanBooster.exe') $Out "ver=4.11.15:$Version" "ui=$tmp" "engine=$engTmp"
    if ($LASTEXITCODE) { throw 'patch failed' }
} finally { Pop-Location }
(Get-FileHash $Out).Hash.ToLower()
