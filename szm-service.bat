@echo off
rem SnipeZilla Server Manager - Windows service
rem Double-click for the menu, or: szm-service.bat install | uninstall | start | stop | restart | status
setlocal EnableExtensions
set "SVC=SnipeZillaServerManager"
cd /d "%~dp0"

rem --- Administrator rights: ask for them, then run again -------------------------------------------
net session >nul 2>&1
if errorlevel 1 (
    echo Asking for administrator rights...
    if "%~1"=="" (
        powershell -NoProfile -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
    ) else (
        powershell -NoProfile -Command "Start-Process -FilePath '%~f0' -ArgumentList '%~1' -Verb RunAs"
    )
    exit /b
)
cd /d "%~dp0"

if not exist "%~dp0szm.exe" (
    echo szm.exe was not found in %~dp0
    echo Put this file next to szm.exe.
    pause
    exit /b 1
)

if /i "%~1"=="install"   call :install   & goto :done
if /i "%~1"=="uninstall" call :uninstall & goto :done
if /i "%~1"=="start"     call :start     & goto :done
if /i "%~1"=="stop"      call :stop      & goto :done
if /i "%~1"=="restart"   call :restart   & goto :done
if /i "%~1"=="status"    call :status    & goto :done

rem --- Menu -----------------------------------------------------------------------------------------
:menu
cls
echo.
echo   SnipeZilla Server Manager - Windows service
echo   Folder: %~dp0
call :status
echo.
echo   1  Install the service and start it
echo   2  Uninstall the service
echo   3  Start
echo   4  Stop
echo   5  Restart  - after replacing szm.exe
echo   6  Open the web interface
echo   0  Exit
echo.
choice /c 1234560 /n /m "  Choose: "
set "PICK=%errorlevel%"
echo.
if "%PICK%"=="1" call :install
if "%PICK%"=="2" call :uninstall
if "%PICK%"=="3" call :start
if "%PICK%"=="4" call :stop
if "%PICK%"=="5" call :restart
if "%PICK%"=="6" start "" "http://localhost:8888/"
if "%PICK%"=="7" exit /b 0
echo.
pause
goto :menu

rem --- Actions --------------------------------------------------------------------------------------
:status
set "STATE=NOT INSTALLED"
sc query "%SVC%" >nul 2>&1
if not errorlevel 1 (
    for /f "tokens=3 delims=: " %%s in ('sc query "%SVC%" ^| findstr /c:"STATE"') do set "STATE=%%s"
)
echo   Service: %STATE%
exit /b 0

:install
sc query "%SVC%" >nul 2>&1
if not errorlevel 1 (
    echo The service is already installed. To move it to this folder: uninstall it, then install it again.
    exit /b 0
)
echo Installing the service. It will run szm.exe from %~dp0
"%~dp0szm.exe" install
if errorlevel 1 exit /b 1
echo.
echo Remember to delete the old Task Scheduler task of SnipeZilla Srcds Manager 1.x, if there is one.
exit /b 0

:uninstall
sc query "%SVC%" >nul 2>&1
if errorlevel 1 (
    echo The service is not installed.
    exit /b 0
)
echo The game servers keep running, but nothing watches or restarts them any more.
echo Your settings, users and logs stay in this folder.
choice /c YN /m "Uninstall the service"
if errorlevel 2 exit /b 0
"%~dp0szm.exe" uninstall
exit /b %errorlevel%

:start
"%~dp0szm.exe" start
if errorlevel 1 exit /b 1
echo Started. Web interface: http://localhost:8888/
exit /b 0

:stop
"%~dp0szm.exe" stop
if errorlevel 1 exit /b 1
call :waitstopped
echo Stopped. The game servers keep running.
exit /b 0

:restart
"%~dp0szm.exe" stop
call :waitstopped
"%~dp0szm.exe" start
if errorlevel 1 exit /b 1
echo Restarted. The game servers kept running.
exit /b 0

:waitstopped
for /l %%i in (1,1,30) do (
    sc query "%SVC%" | findstr /c:"STOPPED" >nul && exit /b 0
    timeout /t 1 /nobreak >nul
)
echo The service is taking long to stop.
exit /b 0

:done
if "%~1" neq "" if /i "%~2" neq "nopause" pause
exit /b 0
