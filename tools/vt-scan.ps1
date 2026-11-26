# Upload files to VirusTotal (key in $env:VT), wait for the results, print who flags each file.
# usage: ./tools/vt-scan.ps1 -Files a.ps1,b.exe     (free key: 4 uploads a minute, so it paces itself)
param([string[]]$Files)
$ErrorActionPreference = 'Continue'
if (-not $env:VT) { 'VT_API_KEY secret is not set, no scan'; return }
$ids = [ordered]@{}
foreach ($f in $Files) {
    if (-not (Test-Path $f)) { "missing: $f"; continue }
    $r = curl.exe -s -H "x-apikey: $env:VT" -F "file=@$f" https://www.virustotal.com/api/v3/files | ConvertFrom-Json
    if ($r.data.id) { $ids[(Split-Path $f -Leaf)] = $r.data.id } else { "upload failed for ${f}: $($r | ConvertTo-Json -Compress)" }
    Start-Sleep 16
}
foreach ($n in $ids.Keys) {
    $a = $null
    for ($i = 0; $i -lt 45; $i++) { $a = curl.exe -s -H "x-apikey: $env:VT" "https://www.virustotal.com/api/v3/analyses/$($ids[$n])" | ConvertFrom-Json; if ($a.data.attributes.status -eq 'completed') { break }; Start-Sleep 20 }
    $st = $a.data.attributes.stats
    $fl = @($a.data.attributes.results.PSObject.Properties | Where-Object { $_.Value.category -in 'malicious', 'suspicious' } | ForEach-Object { "$($_.Name)=$($_.Value.result)" })
    "VT $n : malicious=$($st.malicious) suspicious=$($st.suspicious) undetected=$($st.undetected) | $($fl -join ' ; ')"
}
