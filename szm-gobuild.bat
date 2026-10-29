@echo off
setlocal EnableExtensions
cd /d "%~dp0"
title SnipeZilla Server Manager - Build

rem Go must be installed (https://go.dev/dl/)
where go >nul 2>&1
if errorlevel 1 (
    echo.
    echo  Go is not installed, or not in the PATH: https://go.dev/dl/
    echo.
    pause
    exit /b 1
)

:menu
cls
call :readversion
echo.
echo   SnipeZilla Server Manager %VERSION%
echo   Folder: %~dp0
echo  ==============================
echo   Build
echo  ==============================
echo   [1] Windows  (szm.exe)
echo   [2] Linux    (szm)
echo   [3] Both
echo   [0] Exit
echo.
echo   The version is "var version" in cmd\szm\main.go.
echo.
choice /c 1230 /n /m "  Select an option: "
if errorlevel 4 goto :end
if errorlevel 3 goto :both
if errorlevel 2 goto :linux
if errorlevel 1 goto :windows
goto :menu

:windows
call :buildwindows
goto :done

:linux
call :buildlinux
goto :done

:both
call :buildwindows
if errorlevel 1 goto :done
call :buildlinux
goto :done

:done
echo.
pause
goto :menu

:end
echo.
echo  Bye!
endlocal
exit /b 0


rem ---------------------------------------------------------------------------------------------------------------
rem szm.exe: its Properties (version, icon) first, then the program. Built as szm-new.exe, then put in place: when
rem szm.exe is running (the manager, its service), Windows does not let it be replaced, and szm-new.exe stays.
:buildwindows
echo.
echo  Windows: Properties (version %VERSION%, icon)...
setlocal
set "GOOS="
set "GOARCH="
go generate ./cmd/szm
if errorlevel 1 (
    echo.
    echo  FAILED: go generate. The Properties of szm.exe could not be written.
    endlocal & exit /b 1
)
echo  Windows: building...
set "CGO_ENABLED=0"
set "GOOS=windows"
set "GOARCH=amd64"
go build -trimpath -ldflags "-s -w" -o szm-new.exe ./cmd/szm
if errorlevel 1 (
    echo.
    echo  FAILED: the Windows build. See the errors above.
    endlocal & exit /b 1
)
endlocal
move /y szm-new.exe szm.exe >nul 2>&1
if errorlevel 1 (
    echo.
    echo  Built, but szm.exe is in use (the manager is running^): the new one is szm-new.exe.
    echo  Stop the manager, rename szm-new.exe to szm.exe, and start it again.
    call :size szm-new.exe
    exit /b 0
)
echo.
echo  Done: szm.exe
call :size szm.exe
exit /b 0

rem ---------------------------------------------------------------------------------------------------------------
:buildlinux
echo.
echo  Linux: building...
setlocal
set "CGO_ENABLED=0"
set "GOOS=linux"
set "GOARCH=amd64"
go build -trimpath -ldflags "-s -w" -o szm ./cmd/szm
if errorlevel 1 (
    echo.
    echo  FAILED: the Linux build. See the errors above.
    endlocal & exit /b 1
)
endlocal
echo.
echo  Done: szm (copy it to the Linux machine; chmod +x szm there^)
call :size szm
exit /b 0

rem ---------------------------------------------------------------------------------------------------------------
rem VERSION: from cmd\szm\main.go, the line   var version = "2.0.0"
:readversion
set "VERSION=?"
for /f "tokens=2 delims==" %%V in ('findstr /b /c:"var version" cmd\szm\main.go') do set "VERSION=%%~V"
set "VERSION=%VERSION: =%"
set "VERSION=%VERSION:"=%"
exit /b 0

rem the size of a file, in KB, and when it was made
:size
for %%F in ("%~1") do (
    set /a KB=%%~zF/1024
    call echo   %%KB%% KB, %%~tF
)
exit /b 0
