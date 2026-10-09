@echo off
rem ---------------------------------------------------------------------------
rem  AutoTrigger offline prerequisite installer (7-Zip + OCR engine + config.json)
rem
rem  Thin launcher for install-windows.ps1. It exists so that:
rem    1) a plain double-click works without touching the machine execution policy
rem    2) the window stays open long enough to read the result / exit code
rem
rem  Usage:  install-windows.bat [any install-windows.ps1 parameters]
rem  Example: install-windows.bat -Check -Json
rem
rem  Recommended: right-click -> "Run as administrator", so that 7-Zip can be
rem  installed to C:\Program Files\7-Zip. Without elevation the script still
rem  works: it falls back to <delivery>\third_party\7zip\7-Zip.
rem ---------------------------------------------------------------------------
setlocal

set "PS=%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe"
if not exist "%PS%" set "PS=powershell.exe"

set "SCRIPT=%~dp0install-windows.ps1"
if not exist "%SCRIPT%" (
  echo [ERROR] install-windows.ps1 was not found next to this launcher.
  echo         Expected: %SCRIPT%
  pause
  exit /b 1
)

echo ====================================================================
echo  AutoTrigger - installing offline prerequisites (7-Zip / OCR / config)
echo ====================================================================
echo.

"%PS%" -NoProfile -ExecutionPolicy Bypass -File "%SCRIPT%" %*
set "RC=%ERRORLEVEL%"

echo.
if "%RC%"=="0" (
  echo [OK]   Prerequisites are ready. You can start AutoTrigger.exe now.
) else if "%RC%"=="4" (
  echo [FAIL] 7-Zip is still missing. Re-run as Administrator, then check the log above.
) else if "%RC%"=="5" (
  echo [FAIL] The OCR engine is not ready. Check third_party\ocr\*.7z, then re-run as Administrator.
) else (
  echo [FAIL] Installer exited with code %RC%. See the log above.
)
echo.
pause
exit /b %RC%
