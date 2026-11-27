# ลองเปิด exe ตัวใหม่บนเครื่อง CI: ต้องเปิดได้ มีหน้าต่าง "Zan Booster" มีตัวแสดงผล WebView2 และไม่ปิดตัวเองภายใน 25 วินาที
# เก็บภาพหน้าจอไว้ที่ -Shot (ถ้าใส่) ให้ดูว่าหน้าจอขึ้นจริง ไม่ค้างหน้าโลโก้
#   powershell -File tools/newexe-smoke.ps1 -Exe C:\path\ZanBooster.exe [-Shot C:\path\shot.png]
# สคริปต์นี้ใช้บนเครื่อง CI เท่านั้น ไม่ได้อยู่ในโปรแกรม
param([Parameter(Mandatory)][string]$Exe, [string]$Shot = '')
$ErrorActionPreference = 'Stop'
Add-Type @'
using System; using System.Runtime.InteropServices; using System.Text;
public static class ZbW {
  public delegate bool EnumProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc f, IntPtr l);
  [DllImport("user32.dll")] public static extern int GetWindowThreadProcessId(IntPtr h, out int pid);
  [DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern int GetWindowText(IntPtr h, StringBuilder s, int n);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  public static string Find(int pid) {
    string found = null;
    EnumWindows((h, l) => { int p; GetWindowThreadProcessId(h, out p); if (p == pid && IsWindowVisible(h)) { var s = new StringBuilder(256); GetWindowText(h, s, 256); if (s.Length > 0) found = s.ToString(); } return true; }, IntPtr.Zero);
    return found;
  }
}
'@
$p = Start-Process -FilePath $Exe -PassThru
"started pid $($p.Id)"
$title = $null; $wv = $false
for ($i = 0; $i -lt 25; $i++) {
  Start-Sleep 1
  if ($p.HasExited) { throw "the new exe closed by itself after $i s (exit code $($p.ExitCode))" }
  if (-not $title) { $title = [ZbW]::Find($p.Id) }
  if (-not $wv) { $wv = [bool](Get-CimInstance Win32_Process -Filter "Name='msedgewebview2.exe'" | Where-Object { $_.ParentProcessId -eq $p.Id }) }
}
if (-not $title) { throw 'no visible window after 25 s' }
"window: $title"
if (-not $wv) { throw 'WebView2 did not start (screen would be blank)' }
'WebView2 started'
if ($Shot) {
  Add-Type -AssemblyName System.Windows.Forms, System.Drawing
  $b = [Windows.Forms.Screen]::PrimaryScreen.Bounds
  $bmp = New-Object Drawing.Bitmap $b.Width, $b.Height
  $g = [Drawing.Graphics]::FromImage($bmp); $g.CopyFromScreen($b.Location, [Drawing.Point]::Empty, $b.Size)
  $bmp.Save($Shot, [Drawing.Imaging.ImageFormat]::Png); $g.Dispose(); $bmp.Dispose()
  "screenshot: $Shot"
}
Get-CimInstance Win32_Process | Where-Object { $_.ParentProcessId -eq $p.Id } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
'smoke test passed: opens, shows its window, stays running'
