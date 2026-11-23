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
$tmp = Join-Path $env:TEMP 'zb-release-ui.html'
[IO.File]::WriteAllText($tmp, $ui, $utf8)
"ui.html release size: {0:N0} bytes (slot 1,055,130)" -f $utf8.GetByteCount($ui)
$base = Join-Path $root 'beta\ZanBooster.exe'
if ((Get-FileHash $base).Hash -ne '34CBF139FB3D8811FD8AFD8A363B5E7DC86781067AC4F8DEB0FA8EEC8EB7F6F8') { throw 'beta\ZanBooster.exe on this branch must be the original v4.11.15 binary' }
$patch = Join-Path $env:TEMP 'patchexe.exe'
Push-Location $root
try {
    if (-not (Test-Path $patch)) { go build -mod=vendor -o $patch ./tools/patchexe; if ($LASTEXITCODE) { throw 'patchexe build failed' } }
    & $patch (Join-Path $root 'beta\ZanBooster.exe') $Out "ver=4.11.15:$Version" "ui=$tmp" "engine=$(Join-Path $root 'recovered\engine.ps1')"
    if ($LASTEXITCODE) { throw 'patch failed' }
} finally { Pop-Location }
(Get-FileHash $Out).Hash.ToLower()
