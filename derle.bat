@echo off
chcp 65001 >nul
setlocal
cd /d "%~dp0"

echo.
echo === PoE2 Filtre - derleme ===
echo.

rem --- Go ---
where go >nul 2>&1
if errorlevel 1 (
  echo [HATA] Go bulunamadi. Kur: https://go.dev/dl/  ^(1.25 veya uzeri^)
  goto :bitir
)
for /f "tokens=3" %%v in ('go version') do echo Go       : %%v

rem --- Node / npm ---
where npm >nul 2>&1
if errorlevel 1 (
  echo [HATA] npm bulunamadi. Node.js kur: https://nodejs.org/  ^(20 veya uzeri^)
  goto :bitir
)
for /f "tokens=*" %%v in ('npm --version') do echo npm      : %%v

rem --- Wails CLI ---
for /f "tokens=*" %%g in ('go env GOPATH') do set "GOBIN=%%g\bin"
set "WAILS=%GOBIN%\wails3.exe"
if not exist "%WAILS%" (
  echo Wails CLI yok, kuruluyor... ^(birkac dakika surebilir^)
  go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.23
  if errorlevel 1 (
    echo [HATA] Wails CLI kurulamadi.
    goto :bitir
  )
)
echo Wails    : %WAILS%
echo.

rem Windows Smart App Control imzasiz test exe'lerini %%TEMP%% icinde engelliyor;
rem gecici derleme klasorunu proje icine aliyoruz.
if not exist ".gotmp" mkdir ".gotmp"
set "GOTMPDIR=%CD%\.gotmp"

echo Derleniyor... ^(ilk derleme birkac dakika surer^)
echo Not: "uname" / "tail" bulunamadi gibi satirlar cikabilir, zararsizdir.
echo.
"%WAILS%" build
if errorlevel 1 (
  echo.
  echo [HATA] Derleme basarisiz.
  goto :bitir
)

echo.
if exist "bin\poe2filter.exe" (
  echo === Tamamlandi ===
  echo Uygulama: %CD%\bin\poe2filter.exe
  echo.
  echo Not: Windows "Akilli Uygulama Denetimi" acikken kendi derledigin
  echo imzasiz exe'ler calistirilamaz. Engellenirse Windows Guvenligi -^>
  echo Uygulama ve tarayici denetimi -^> Akilli Uygulama Denetimi -^> Kapali.
) else (
  echo [HATA] bin\poe2filter.exe olusmadi.
)

:bitir
echo.
pause
endlocal
