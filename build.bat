@echo off
chcp 65001 >nul
setlocal
cd /d "%~dp0"

echo ============================================================
echo   LocalDrive 构建脚本  (Windows 托盘版)
echo ============================================================
echo.

where go >nul 2>nul
if errorlevel 1 (
    echo [错误] 未检测到 Go 工具链，请先安装 Go: https://go.dev/dl/
    pause
    exit /b 1
)

echo [1/3] 正在编译 LocalDrive.exe ...
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
REM -H windowsgui 让程序以 GUI 子系统运行，双击时不会弹出 cmd 黑窗口
go build -trimpath -ldflags "-s -w -H windowsgui" -o LocalDrive.exe .

if errorlevel 1 (
    echo.
    echo [错误] 编译失败，请检查上方错误信息。
    pause
    exit /b 1
)

echo [2/3] 编译完成 -^> %~dp0LocalDrive.exe
for %%A in (LocalDrive.exe) do echo       文件大小: %%~zA 字节

echo [3/3] 同步资源管理器右键菜单命令路径 ...
REM 程序启动时会自动注册，这里仅提示
echo       程序首次运行会自动安装「发布到 LocalDrive」右键菜单。

echo.
echo 提示：双击 LocalDrive.exe 后程序会常驻任务栏托盘（无控制台窗口），
echo       右键托盘图标可设置端口、目录与本机 IP。
echo.
pause
