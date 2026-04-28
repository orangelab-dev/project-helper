@echo off
setlocal

cd /d "%~dp0"

set "APP="
if exist "project-helper.exe" set "APP=project-helper.exe"
if not defined APP if exist "bin\project-helper.exe" set "APP=bin\project-helper.exe"

if not defined APP (
  echo Cannot find project-helper.exe next to this script or in .\bin.
  echo Expected one of:
  echo   %CD%\project-helper.exe
  echo   %CD%\bin\project-helper.exe
  pause
  exit /b 1
)

echo Working directory: %CD%
echo Starting: %APP%
echo Open the configured address after startup. Default: http://localhost:8080

"%APP%"

echo.
echo project-helper has stopped.
pause
