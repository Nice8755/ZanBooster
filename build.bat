@echo off
cd /d "%~dp0"
where go >nul 2>nul
if errorlevel 1 (
  echo [!] Go is not installed. Download: https://go.dev/dl/  ^(version 1.24 or newer^)
  pause
  exit /b 1
)
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
echo Building ZanBooster.exe ...
go build -mod=vendor -trimpath -ldflags "-H windowsgui -s -w" -o ZanBooster.exe .
if errorlevel 1 (
  echo [!] Build failed
  pause
  exit /b 1
)
echo.
echo Done: ZanBooster.exe
echo SHA256 ^(put this line in the GitHub release description: SHA256: xxxx^)
certutil -hashfile ZanBooster.exe SHA256
pause
