@echo off
setlocal
set "APP=%~dp0SleepInspector.ps1"
if not exist "%APP%" (
    echo SleepInspector.ps1 was not found next to this launcher.
    echo Please extract all files into the same folder.
    pause
    exit /b 1
)
start "" "%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -STA -ExecutionPolicy Bypass -WindowStyle Hidden -File "%APP%"
if errorlevel 1 (
    echo Failed to start Windows PowerShell.
    pause
    exit /b 1
)
exit /b 0
