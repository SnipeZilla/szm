@echo off
setlocal EnableExtensions
cd /d "%~dp0"
title SnipeZilla Server Manager - SHA256SUMS.txt

rem SHA256SUMS.txt: the SHA-256 of szm.exe and szm, for the download page. Run it after the last build, on the
rem files you upload. One line per file, "<hash>  <name>" with Linux line ends: there, sha256sum -c SHA256SUMS.txt
rem checks them; on Windows, certutil -hashfile szm.exe SHA256 gives the same number.

call :readversion
echo.
echo   SnipeZilla Server Manager %VERSION%
echo.

powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$files = @('szm.exe', 'szm') | Where-Object { Test-Path -LiteralPath $_ -PathType Leaf };" ^
  "if (-not $files) { Write-Host '  No szm.exe or szm in this folder: build them first (szm-gobuild.bat).'; exit 1 };" ^
  "$lines = foreach ($f in $files) { (Get-FileHash -LiteralPath $f -Algorithm SHA256).Hash.ToLower() + '  ' + $f };" ^
  "[IO.File]::WriteAllText((Join-Path (Get-Location) 'SHA256SUMS.txt'), (($lines -join [char]10) + [char]10));" ^
  "$lines | ForEach-Object { Write-Host ('  ' + $_) }"
if errorlevel 1 goto :end

echo.
echo   Saved: SHA256SUMS.txt (upload it next to the files, or paste the numbers under the download links)

:end
echo.
pause
endlocal
exit /b 0

rem VERSION: from cmd\szm\main.go, the line   var version = "2.0.0"
:readversion
set "VERSION=?"
for /f "tokens=2 delims==" %%V in ('findstr /b /c:"var version" cmd\szm\main.go') do set "VERSION=%%~V"
set "VERSION=%VERSION: =%"
set "VERSION=%VERSION:"=%"
exit /b 0
