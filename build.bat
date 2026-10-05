@echo off
chcp 65001 >nul
setlocal
cd /d "%~dp0"

echo.
echo === MrW Overlay for POE 2 - build ===
echo.

rem --- Go ---
where go >nul 2>&1
if errorlevel 1 (
  echo [ERROR] Go not found. Install it from https://go.dev/dl/  ^(1.25 or later^)
  goto :end
)
for /f "tokens=3" %%v in ('go version') do echo Go       : %%v

rem --- Node / npm ---
where npm >nul 2>&1
if errorlevel 1 (
  echo [ERROR] npm not found. Install Node.js from https://nodejs.org/  ^(20 or later^)
  goto :end
)
for /f "tokens=*" %%v in ('npm --version') do echo npm      : %%v

rem --- Wails CLI ---
for /f "tokens=*" %%g in ('go env GOPATH') do set "GOBIN=%%g\bin"
set "WAILS=%GOBIN%\wails3.exe"
if not exist "%WAILS%" (
  echo Wails CLI not found, installing... ^(this can take a few minutes^)
  go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.23
  if errorlevel 1 (
    echo [ERROR] Could not install the Wails CLI.
    goto :end
  )
)
echo Wails    : %WAILS%
echo.

rem Windows Smart App Control blocks unsigned test executables under %%TEMP%%,
rem so the temporary build folder is kept inside the project.
if not exist ".gotmp" mkdir ".gotmp"
set "GOTMPDIR=%CD%\.gotmp"

echo Building... ^(the first build takes a few minutes^)
echo Note: lines such as "uname" / "tail" not found are harmless.
echo.
"%WAILS%" build
if errorlevel 1 (
  echo.
  echo [ERROR] Build failed.
  goto :end
)

echo.
if exist "bin\poe2filter.exe" (
  echo === Done ===
  echo App: %CD%\bin\poe2filter.exe
  echo.
  echo Note: while Windows Smart App Control is on, unsigned executables
  echo you build yourself cannot run. If it is blocked: Windows Security -^>
  echo App ^& browser control -^> Smart App Control -^> Off.
) else (
  echo [ERROR] bin\poe2filter.exe was not created.
)

:end
echo.
pause
endlocal
