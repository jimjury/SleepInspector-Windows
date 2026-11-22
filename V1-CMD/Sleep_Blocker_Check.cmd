@echo off
setlocal EnableExtensions
title Windows Sleep Blocker Check

rem Elevate only when required. No changes are made to power settings.
fltmc >nul 2>&1
if not errorlevel 1 goto check
set "SLEEP_BLOCKER_SCRIPT=%~f0"
powershell.exe -NoProfile -Command "try { Start-Process -FilePath $env:SLEEP_BLOCKER_SCRIPT -Verb RunAs -ErrorAction Stop } catch { Write-Host 'Administrator permission was not granted.'; exit 1 }"
if errorlevel 1 pause
exit /b

:check
cls
echo ================================================================
echo                  WINDOWS SLEEP BLOCKER CHECK
echo ================================================================
echo Current time: %date% %time%
echo.
echo Active power requests preventing automatic sleep/display-off:
echo ----------------------------------------------------------------
"%SystemRoot%\System32\powercfg.exe" /requests
echo.
echo ----------------------------------------------------------------
echo This is a snapshot of CURRENT requests, not a complete sleep audit.
echo No power settings were changed.
echo.
choice /C RQ /N /M "Press R to refresh or Q to quit: "
if errorlevel 2 exit /b
if errorlevel 1 goto check
