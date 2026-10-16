@echo off
setlocal
cd /d %~dp0

rem --- pure-Go build: no C++ toolchain needed ---
rem     build.bat          -> eurokey.exe (GUI, single binary)
rem     build.bat test     -> run engine + shell tests
rem     build.bat console  -> eurokey.exe with console (debug)
rem
rem app.ico / app.manifest / rsrc_amd64.syso are prebuilt. Regenerate with:
rem     rsrc -manifest app.manifest -ico app.ico -o rsrc_amd64.syso
rem (arm64 needs its own rsrc_arm64.syso: rsrc -arch arm64 ...)

if "%1"=="test" (
    go test ./... -v
    exit /b %errorlevel%
)
if "%1"=="console" (
    go build -ldflags "-s -w" -o eurokey.exe .
    exit /b %errorlevel%
)
go build -ldflags "-H windowsgui -s -w" -o eurokey.exe .
exit /b %errorlevel%
