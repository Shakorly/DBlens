@echo off
title DBLens Monitor
cd /d "%~dp0"
cls

echo.
echo  ==========================================
echo    DBLens Monitor - SQL Server Dashboard
echo  ==========================================
echo.

if not exist "%~dp0sqlmonitor.exe" (
    echo  ERROR: sqlmonitor.exe is missing from this folder.
    echo.
    echo  Please copy sqlmonitor.exe into this folder:
    echo  %~dp0
    echo.
    pause
    exit /b 1
)

:MENU
cls
echo.
echo  ==========================================
echo    DBLens Monitor
echo  ==========================================
echo.
echo    1.  Start DBLens
echo    2.  Add or Edit Servers
echo    3.  Stop DBLens
echo    4.  Update sqlmonitor.exe
echo    5.  View Log
echo    6.  Exit
echo.
set /p CHOICE=  Enter choice [1-6]: 

if "%CHOICE%"=="1" goto START
if "%CHOICE%"=="2" goto MANAGE
if "%CHOICE%"=="3" goto STOP
if "%CHOICE%"=="4" goto UPDATE
if "%CHOICE%"=="5" goto VIEWLOG
if "%CHOICE%"=="6" exit /b 0
cls
echo  Invalid choice, please try again.
goto MENU

:START
cls
echo.
echo  Starting DBLens...
echo.

:: Stop any existing instance
taskkill /F /IM sqlmonitor.exe >nul 2>&1
timeout /t 1 /nobreak >nul

:: Check config exists
if not exist "%~dp0config.json" (
    echo  ERROR: config.json not found.
    echo  Please run option 2 first to add your servers.
    echo.
    pause
    goto MENU
)

:: Check config has been set up (not still placeholder)
findstr /C:"ENTER-SERVER-NAME-HERE" "%~dp0config.json" >nul 2>&1
if %errorLevel% equ 0 (
    echo  *** CONFIG NOT SET UP YET ***
    echo.
    echo  Please choose option 2 first to add your servers.
    echo.
    pause
    goto MENU
)

:: Fix BOM in config.json before starting (in case Manage-Servers left one)
powershell -Command ^
  "$f='%~dp0config.json';" ^
  "$raw=[System.IO.File]::ReadAllText($f);" ^
  "$raw=$raw.TrimStart([char]0xFEFF);" ^
  "$nb=New-Object System.Text.UTF8Encoding $false;" ^
  "[System.IO.File]::WriteAllText($f,$raw,$nb)"

echo  Dashboard opening at: http://localhost:8080
echo.
echo  Keep this window open while monitoring.
echo  Close this window or press Ctrl+C to stop DBLens.
echo.
timeout /t 2 /nobreak >nul
start http://localhost:8080
"%~dp0sqlmonitor.exe" -config "%~dp0config.json"
echo.
echo  DBLens has stopped.
pause
goto MENU

:MANAGE
cls
powershell -ExecutionPolicy Bypass -File "%~dp0Manage-Servers.ps1"
cls
goto MENU

:STOP
cls
echo.
taskkill /F /IM sqlmonitor.exe >nul 2>&1
if %errorLevel% equ 0 (
    echo  DBLens stopped successfully.
) else (
    echo  DBLens was not running.
)
echo.
pause
goto MENU

:UPDATE
cls
echo.
echo  ==========================================
echo    Update sqlmonitor.exe
echo  ==========================================
echo.
echo  Steps:
echo.
echo    1. Download the new sqlmonitor.exe
echo    2. Copy it into this folder (replace the old one):
echo       %~dp0
echo    3. Press Enter here when ready.
echo.
pause

taskkill /F /IM sqlmonitor.exe >nul 2>&1
timeout /t 2 /nobreak >nul

echo.
set /p GO=  Start DBLens now with the new version? [Y/N]: 
if /i "%GO%"=="Y" goto START
goto MENU

:VIEWLOG
cls
echo.
if exist "%~dp0dblens.log" (
    echo  Last 50 lines of dblens.log:
    echo  ------------------------------------------
    powershell -Command "Get-Content '%~dp0dblens.log' -Tail 50"
) else (
    echo  No log file found yet. Start DBLens first.
)
echo.
pause
goto MENU
