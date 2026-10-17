//go:build windows

package main

// Windows 11「一级右键菜单」的稀疏包（Sparse Package）安装 / 卸载。
//
// 背景：Win11 的新式右键菜单只接受「有包标识的应用」注册的 IExplorerCommand 命令。
// 本程序用一份稀疏包为自己取得包标识，包内不含二进制，通过
// uap10:AllowExternalContent 指向 exe 所在目录。
//
// 因为注册/注销走的是 Windows 部署 API（Add-AppxPackage / Remove-AppxPackage），
// 这里用隐藏窗口方式调用系统自带的 PowerShell 完成；不需要管理员权限。

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	// 与 AppxManifest.xml 中 Identity/@Name 一致
	win11PkgName = "LocalDrive.PublishShell"
	// 包版本（必须是四段式）
	win11PkgVersion = "2.4.0.0"
	// 菜单显示名，用户要求固定为「发布」
	win11VerbTitle = "发布"
)

// win11PkgDir 返回存放稀疏包清单与图标的目录（位于配置目录内，不污染共享根目录）。
func win11PkgDir() string { return filepath.Join(appDir(), "Win11Menu") }

func win11ManifestPath() string { return filepath.Join(win11PkgDir(), "AppxManifest.xml") }

// win11Manifest 生成稀疏包清单。
//
// exeName 为可执行文件名；实际路径由注册时的 -ExternalLocation 决定。
func win11Manifest(exeName string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<!-- 由 LocalDrive 自动生成，请勿手工修改 -->
<Package
  xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10"
  xmlns:uap="http://schemas.microsoft.com/appx/manifest/uap/windows10"
  xmlns:rescap="http://schemas.microsoft.com/appx/manifest/foundation/windows10/restrictedcapabilities"
  xmlns:desktop4="http://schemas.microsoft.com/appx/manifest/desktop/windows10/4"
  xmlns:desktop5="http://schemas.microsoft.com/appx/manifest/desktop/windows10/5"
  xmlns:uap10="http://schemas.microsoft.com/appx/manifest/uap/windows10/10"
  xmlns:com="http://schemas.microsoft.com/appx/manifest/com/windows10"
  IgnorableNamespaces="uap rescap desktop4 desktop5 uap10 com">

  <Identity
    Name="%s"
    ProcessorArchitecture="neutral"
    Publisher="CN=LocalDrive"
    Version="%s" />

  <Properties>
    <DisplayName>LocalDrive 发布</DisplayName>
    <PublisherDisplayName>LocalDrive</PublisherDisplayName>
    <Logo>Assets\Logo.png</Logo>
    <uap10:AllowExternalContent>true</uap10:AllowExternalContent>
  </Properties>

  <Dependencies>
    <TargetDeviceFamily Name="Windows.Desktop" MinVersion="10.0.22000.0" MaxVersionTested="10.0.26300.0" />
  </Dependencies>

  <Resources>
    <Resource Language="zh-CN" />
    <Resource Language="en-US" />
  </Resources>

  <Capabilities>
    <rescap:Capability Name="runFullTrust" />
    <rescap:Capability Name="unvirtualizedResources" />
  </Capabilities>

  <Applications>
    <Application Id="LocalDrivePublish"
                 Executable="%s"
                 uap10:TrustLevel="mediumIL"
                 uap10:RuntimeBehavior="win32App">
      <uap:VisualElements
        AppListEntry="none"
        DisplayName="LocalDrive 发布"
        Description="把文件发布到 LocalDrive 并复制 HTTP 链接"
        BackgroundColor="transparent"
        Square150x150Logo="Assets\Square150x150Logo.png"
        Square44x44Logo="Assets\Square44x44Logo.png" />
      <Extensions>
        <desktop4:Extension Category="windows.fileExplorerContextMenus">
          <desktop4:FileExplorerContextMenus>
            <desktop5:ItemType Type="*">
              <desktop5:Verb Id="LocalDrivePublish" Clsid="%s" />
            </desktop5:ItemType>
            <desktop5:ItemType Type="Directory">
              <desktop5:Verb Id="LocalDrivePublish" Clsid="%s" />
            </desktop5:ItemType>
          </desktop4:FileExplorerContextMenus>
        </desktop4:Extension>
        <com:Extension Category="windows.comServer">
          <com:ComServer>
            <com:ExeServer Executable="%s"
                           Arguments="-comserver"
                           DisplayName="LocalDrive 发布命令">
              <com:Class Id="%s" />
            </com:ExeServer>
          </com:ComServer>
        </com:Extension>
      </Extensions>
    </Application>
  </Applications>
</Package>
`, win11PkgName, win11PkgVersion, exeName,
		strings.Trim(comClsidString, "{}"), strings.Trim(comClsidString, "{}"),
		exeName, strings.Trim(comClsidString, "{}"))
}

// writeWin11Package 把清单与图标资源写入包目录。
func writeWin11Package() error {
	dir := win11PkgDir()
	assets := filepath.Join(dir, "Assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		return fmt.Errorf("创建包目录失败: %w", err)
	}

	exe := filepath.Base(exePath())
	if exe == "" {
		return errors.New("无法定位程序自身路径")
	}
	if err := os.WriteFile(win11ManifestPath(), []byte(win11Manifest(exe)), 0o644); err != nil {
		return fmt.Errorf("写入清单失败: %w", err)
	}

	glyph := renderGlyph()
	for name, size := range map[string]int{
		"Logo.png":              300,
		"Square150x150Logo.png": 150,
		"Square44x44Logo.png":   44,
	} {
		f, err := os.Create(filepath.Join(assets, name))
		if err != nil {
			return fmt.Errorf("创建 %s 失败: %w", name, err)
		}
		err = png.Encode(f, scaleCanvas(glyph, size))
		f.Close()
		if err != nil {
			return fmt.Errorf("写入 %s 失败: %w", name, err)
		}
	}
	return nil
}

// scaleCanvas 把图标画布缩放到 size×size（盒式滤波，预乘 alpha）。
func scaleCanvas(src *canvas, size int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	sx := float64(src.w) / float64(size)
	sy := float64(src.h) / float64(size)
	for y := 0; y < size; y++ {
		y0 := int(float64(y) * sy)
		y1 := int(float64(y+1) * sy)
		if y1 <= y0 {
			y1 = y0 + 1
		}
		if y1 > src.h {
			y1 = src.h
		}
		for x := 0; x < size; x++ {
			x0 := int(float64(x) * sx)
			x1 := int(float64(x+1) * sx)
			if x1 <= x0 {
				x1 = x0 + 1
			}
			if x1 > src.w {
				x1 = src.w
			}
			var sr, sg, sb, sa, n uint64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					p := src.px[yy*src.w+xx]
					a := uint64((p >> 24) & 0xff)
					sr += uint64((p>>16)&0xff) * a
					sg += uint64((p>>8)&0xff) * a
					sb += uint64(p&0xff) * a
					sa += a
					n++
				}
			}
			if n == 0 || sa == 0 {
				continue
			}
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8(sr / sa),
				G: uint8(sg / sa),
				B: uint8(sb / sa),
				A: uint8(sa / n),
			})
		}
	}
	return dst
}

// ---------------------------------------------------------------- PowerShell 调用

// psQuote 把字符串包成 PowerShell 单引号字面量。
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// runPowerShell 以隐藏窗口方式执行一段 PowerShell 脚本，返回合并输出。
func runPowerShell(script string) (string, error) {
	ps := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if !fileExists(ps) {
		ps = "powershell.exe"
	}
	const createNoWindow = 0x08000000
	full := "$ErrorActionPreference='Stop';" +
		"[Console]::OutputEncoding=[System.Text.Encoding]::UTF8;" + script
	cmd := exec.Command(ps, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", full)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// ---------------------------------------------------------------- 安装 / 卸载 / 查询

// installWin11Menu 生成并注册稀疏包（幂等，重复执行会重新注册到当前 exe 所在目录）。
//
// -ForceApplicationShutdown 是必需的：包一旦注册，shell 就会常驻一个
// `LocalDrive.exe -comserver` 进程，它占用着包资源，否则会报 0x80073D02。
func installWin11Menu() error {
	if err := writeWin11Package(); err != nil {
		return err
	}
	// 注销改名前注册的旧包，否则「显示更多选项」里会出现两条「发布」
	script := fmt.Sprintf(
		"Get-AppxPackage -Name %s -ErrorAction SilentlyContinue | Remove-AppxPackage -ErrorAction SilentlyContinue;"+
			"Add-AppxPackage -Register %s -ExternalLocation %s -ForceApplicationShutdown",
		psQuote(legacyBrand+".PublishShell"), psQuote(win11ManifestPath()), psQuote(exeDir()))
	out, err := runPowerShell(script)
	if err != nil {
		return fmt.Errorf("%v\n%s", err, out)
	}
	logf("[信息] 已注册 Win11 一级菜单稀疏包：%s", win11PkgName)
	return nil
}

// uninstallWin11Menu 注销稀疏包。
//
// 先结束常驻的 COM 服务器进程，否则包资源被占用会移除失败。
func uninstallWin11Menu() error {
	killScript := "@(Get-CimInstance Win32_Process -Filter \"Name='LocalDrive.exe'\")" +
		"| Where-Object { $_.CommandLine -like '*-comserver*' }" +
		"| ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue };" +
		"Start-Sleep -Milliseconds 500;"
	script := killScript + fmt.Sprintf(
		"Get-AppxPackage -Name %s | Remove-AppxPackage", psQuote(win11PkgName))
	out, err := runPowerShell(script)
	if err != nil {
		return fmt.Errorf("%v\n%s", err, out)
	}
	logf("[信息] 已注销 Win11 一级菜单稀疏包：%s", win11PkgName)
	return nil
}

// isWin11MenuInstalled 查询稀疏包是否已注册。
func isWin11MenuInstalled() bool {
	script := fmt.Sprintf(
		"@(Get-AppxPackage -Name %s).Count", psQuote(win11PkgName))
	out, err := runPowerShell(script)
	if err != nil {
		return false
	}
	return strings.HasPrefix(out, "1")
}

// ensureWin11PackageFiles 只保证包文件存在（首次运行时调用，便于「重新加载」）。
func ensureWin11PackageFiles() {
	if fileExists(win11ManifestPath()) {
		return
	}
	if err := writeWin11Package(); err != nil {
		logf("[警告] 生成 Win11 菜单包文件失败: %v", err)
	}
}
