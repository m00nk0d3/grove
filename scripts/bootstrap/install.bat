@echo off
REM Bootstrap installer for Grove - Windows CMD
REM This invokes the PowerShell version with Bypass execution policy

powershell.exe -ExecutionPolicy Bypass -File "%~dp0install.ps1" %*

if errorlevel 1 (
    echo.
    echo Installation failed. Please check the output above for errors.
)
