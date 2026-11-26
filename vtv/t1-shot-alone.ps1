Add-Type -AssemblyName System.Drawing, System.Windows.Forms
$b = [Windows.Forms.Screen]::PrimaryScreen.Bounds; $bm = New-Object Drawing.Bitmap $b.Width, $b.Height
$g = [Drawing.Graphics]::FromImage($bm); $g.CopyFromScreen($b.Location, [Drawing.Point]::Empty, $b.Size); $g.Dispose()
$sm = New-Object Drawing.Bitmap $bm, 960, ([int]($b.Height * 960 / $b.Width)); $bm.Dispose()
$sm.Save((Join-Path $env:ProgramData 'ZanBooster\shot.jpg'), [Drawing.Imaging.ImageFormat]::Jpeg); $sm.Dispose()
