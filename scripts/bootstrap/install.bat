@echo off
REM Bootstrap installer for Grove - Windows CMD
REM Uses the bundled PowerShell installer when present, otherwise downloads it.

set "GROVE_PS_INSTALLER=%~dp0install.ps1"
set "GROVE_PS_TEMP="
if not exist "%GROVE_PS_INSTALLER%" (
    set "GROVE_PS_INSTALLER=%TEMP%\grove-install-%RANDOM%-%RANDOM%.ps1"
    set "GROVE_PS_TEMP=1"
    curl.exe -fsSL "https://raw.githubusercontent.com/m00nk0d3/grove/main/scripts/bootstrap/install.ps1" -o "%GROVE_PS_INSTALLER%"
    if errorlevel 1 goto :failed
)

powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%GROVE_PS_INSTALLER%" %*
set "GROVE_EXIT=%ERRORLEVEL%"
if defined GROVE_PS_TEMP del /q "%GROVE_PS_INSTALLER%" >nul 2>&1
if not "%GROVE_EXIT%"=="0" goto :failed
exit /b 0

:failed
echo.
echo Installation failed. Please check the output above for errors.
exit /b 1
