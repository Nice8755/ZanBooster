# Tiny static file server for previewing ui.html in a browser (mock mode).
param([int]$Port = 8765, [string]$Root = (Join-Path $PSScriptRoot '..\recovered'))
$Root = (Resolve-Path $Root).Path
$l = [Net.HttpListener]::new(); $l.Prefixes.Add("http://localhost:$Port/"); $l.Start()
Write-Host "serving $Root on http://localhost:$Port/"
$types = @{ '.html' = 'text/html; charset=utf-8'; '.js' = 'text/javascript'; '.css' = 'text/css'; '.png' = 'image/png'; '.svg' = 'image/svg+xml' }
while ($l.IsListening) {
    $c = $l.GetContext()
    $p = [Uri]::UnescapeDataString($c.Request.Url.AbsolutePath.TrimStart('/')); if (-not $p) { $p = 'ui.html' }
    $f = Join-Path $Root $p
    if ((Test-Path -LiteralPath $f -PathType Leaf) -and ([IO.Path]::GetFullPath($f)).StartsWith($Root)) {
        $b = [IO.File]::ReadAllBytes($f); $ext = [IO.Path]::GetExtension($f)
        $c.Response.ContentType = $(if ($types[$ext]) { $types[$ext] } else { 'application/octet-stream' })
        $c.Response.OutputStream.Write($b, 0, $b.Length)
    } else { $c.Response.StatusCode = 404 }
    $c.Response.Close()
}
