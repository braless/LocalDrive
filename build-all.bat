@echo off
chcp 65001 >nul
setlocal
cd /d "%~dp0"

echo ============================================================
echo   LocalDrive 三平台构建
echo ============================================================
echo.

where go >nul 2>nul
if errorlevel 1 (
    echo [错误] 未检测到 Go 工具链，请先安装 Go: https://go.dev/dl/
    pause
    exit /b 1
)

set CGO_ENABLED=0

echo [1/3] Windows amd64 ...
set GOOS=windows
set GOARCH=amd64
go build -trimpath -ldflags "-s -w -H windowsgui" -o LocalDrive.exe .
if errorlevel 1 goto fail

echo [2/3] Linux amd64 ...
set GOOS=linux
set GOARCH=amd64
go build -trimpath -ldflags "-s -w" -o LocalDrive-linux .
if errorlevel 1 goto fail

echo [3/3] Linux arm64 ...
set GOOS=linux
set GOARCH=arm64
go build -trimpath -ldflags "-s -w" -o LocalDrive-linux-arm64 .
if errorlevel 1 goto fail

echo.
echo 构建完成：
for %%A in (LocalDrive.exe LocalDrive-linux LocalDrive-linux-arm64) do echo   %%A  %%~zA 字节
echo.
pause
exit /b 0

:fail
echo.
echo [错误] 编译失败，请检查上方错误信息。
pause
exit /b 1
