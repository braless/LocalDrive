//go:build windows

package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// LocalDrive —— Windows 文件夹发布服务（托盘常驻，无控制台窗口）
//
// 运行模式：
//   - 无参数：后台常驻，托盘图标 + HTTP 文件服务
//   - -publish <路径...>：由资源管理器右键菜单调用，发布文件并复制链接到剪贴板
//   - -edit-config：打开 config.ini（资源管理器里右键 LocalDrive.exe 即可调用）
//   - -comserver（或 shell 传入的 -Embedding）：作为进程外 COM 服务器运行，
//     为 Windows 11 一级右键菜单提供「发布」命令
func main() {
	initLog()
	defer closeLog()
	defer logPanic("main")

	args := os.Args[1:]

	// 配置对所有运行模式都生效（含「发布」子进程与 COM 服务器），所以最先加载
	firstRun := !fileExists(configPath())
	cfg := loadConfig()
	setConfig(cfg)

	// Windows 11 一级右键菜单：shell 通过 COM 激活本进程（命令行带 -Embedding）
	if isComServerInvocation(args) || hasFlag(args, "-comserver") {
		os.Exit(runComServer())
	}

	// 「发布」模式（路径可能以 - 开头，因此手工扫描参数）
	if paths, ok := extractPublishPaths(args); ok {
		runPublish(paths)
		return
	}

	// 打开配置文件（不参与单实例判断，可随时调用）
	if hasFlag(args, "-edit-config") || hasFlag(args, "--config") {
		if err := saveConfig(cfg); err != nil {
			logf("[警告] 写入配置文件失败: %v", err)
		}
		logf("[信息] 打开配置文件：%s", configPath())
		openPath(configPath())
		return
	}

	// 调试模式：附加到父进程控制台，输出实时日志
	if hasFlag(args, "-console") {
		if attachConsole() {
			logToStd = true
			logf("[信息] 已附加到控制台，日志将同时输出到终端")
		}
	}

	switch {
	case hasFlag(args, "-install-menu"):
		if err := installShellMenu(exePath()); err != nil {
			logf("[错误] 安装右键菜单失败: %v", err)
			msgBox(appName, "安装失败：\n"+err.Error(), MB_ICONERROR)
			return
		}
		msgBox(appName, "资源管理器右键菜单已安装。\n\n在任意文件或文件夹上点右键，即可看到「"+menuLabel+"」。",
			MB_ICONINFORMATION)
		return
	case hasFlag(args, "-uninstall-menu"):
		if err := uninstallShellMenu(); err != nil {
			msgBox(appName, "移除失败：\n"+err.Error(), MB_ICONERROR)
			return
		}
		msgBox(appName, "资源管理器右键菜单已移除。", MB_ICONINFORMATION)
		return
	case hasFlag(args, "-install-win11"):
		if err := installWin11Menu(); err != nil {
			logf("[错误] 安装 Win11 一级菜单失败: %v", err)
			msgBox(appName, "安装 Win11 一级菜单失败：\n"+err.Error(), MB_ICONERROR)
			return
		}
		msgBox(appName, "Windows 11 一级右键菜单已安装。\n\n在任意文件或文件夹上点右键，一级菜单中即可看到「"+
			win11VerbTitle+"」，无需再点「显示更多选项」。", MB_ICONINFORMATION)
		return
	case hasFlag(args, "-uninstall-win11"):
		if err := uninstallWin11Menu(); err != nil {
			msgBox(appName, "移除 Win11 一级菜单失败：\n"+err.Error(), MB_ICONERROR)
			return
		}
		msgBox(appName, "Windows 11 一级右键菜单已移除。", MB_ICONINFORMATION)
		return
	case hasFlag(args, "-icon"):
		fmt.Println(iconFilePath())
		return
	}

	// 单实例：已有实例则唤起并退出
	if _, first := acquireSingleInstance(); !first {
		logf("[信息] 检测到已有实例在运行，转为唤起操作")
		if sendToTray("show", "") {
			return
		}
		msgBox(appName, "程序已在后台运行。\n\n请查看任务栏右下角的托盘图标（可能需要展开「隐藏的图标」）。",
			MB_ICONINFORMATION)
		return
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if firstRun {
		if err := installShellMenu(exePath()); err != nil {
			logf("[警告] 首次运行自动安装右键菜单失败: %v", err)
		} else {
			logf("[信息] 首次运行已自动安装资源管理器右键菜单")
		}
		// Windows 11 一级菜单（稀疏包）。失败不影响其它功能，可在托盘菜单里重试。
		if err := installWin11Menu(); err != nil {
			logf("[警告] 首次运行自动安装 Win11 一级菜单失败: %v", err)
		} else {
			logf("[信息] 首次运行已自动安装 Win11 一级菜单「%s」", win11VerbTitle)
		}
		logf("[信息] 已生成配置文件：%s", configPath())
	} else {
		// 非首次运行也确保包文件存在，便于用户随时在托盘菜单里启用
		ensureWin11PackageFiles()
	}

	// 发布目录必须存在，否则无法作为临时存放点
	if err := ensurePublishDir(cfg); err != nil {
		logf("[警告] 创建发布目录失败: %v", err)
	}

	app := newApp(cfg)

	// 先建托盘窗口再启动服务，确保「发布」子进程能立刻找到通知窗口
	tray := initTray(app)

	port, err := app.Start(cfg.Port)
	if err != nil {
		logf("[错误] 启动 HTTP 服务失败: %v", err)
		msgBox(appName, "启动服务失败：\n"+err.Error(), MB_ICONERROR)
		return
	}
	app.writeState()

	// 发布目录到期自动清理
	stopCleaner := make(chan struct{})
	startPublishCleaner(stopCleaner)

	logf("[信息] %s v%s 已启动 | 端口 %d | 展示目录 %v | 发布目录 %s（保留 %d 分钟）",
		appName, appVersion, port, cfg.EffectiveRoots(), cfg.ResolvePublishDir(), cfg.PublishTTL)

	tray.balloon(appName+" 已在后台运行",
		fmt.Sprintf("%s/\n端口 %d\n右键托盘图标可修改设置", app.BaseURL(), port), NIIF_INFO)

	if cfg.AutoOpen {
		openURL(app.LocalURL() + "/")
	}

	tray.runLoop()

	close(stopCleaner)
	clearState()
	logf("[信息] 程序已退出")
}

// ---------------------------------------------------------------- 参数解析

// extractPublishPaths 识别 -publish 模式并返回待发布的路径列表。
func extractPublishPaths(args []string) ([]string, bool) {
	for i, a := range args {
		if strings.EqualFold(a, "-publish") || strings.EqualFold(a, "--publish") {
			paths := make([]string, 0, len(args)-i-1)
			for _, p := range args[i+1:] {
				if p = strings.TrimSpace(p); p != "" {
					paths = append(paths, p)
				}
			}
			return paths, true
		}
	}
	return nil, false
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if strings.EqualFold(a, flag) {
			return true
		}
	}
	return false
}
