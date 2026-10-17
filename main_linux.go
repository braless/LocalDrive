//go:build !windows

package main

import (
	"fmt"
	"os"
	"strings"
)

// LocalDrive —— Linux 文件夹发布服务（无托盘，控制台运行）
//
// 运行模式：
//   - 无参数：前台运行，HTTP 文件服务
//   - -publish <路径...>：发布文件并复制链接到剪贴板
//   - -edit-config：打开 config.ini
func main() {
	initLog()
	defer closeLog()
	defer logPanic("main")

	args := os.Args[1:]

	// 配置对所有运行模式都生效，所以最先加载
	firstRun := !fileExists(configPath())
	cfg := loadConfig()
	setConfig(cfg)

	// 「发布」模式
	if paths, ok := extractPublishPaths(args); ok {
		runPublish(paths)
		return
	}

	// 打开配置文件
	if hasFlag(args, "-edit-config") || hasFlag(args, "--config") {
		if err := saveConfig(cfg); err != nil {
			logf("[警告] 写入配置文件失败: %v", err)
		}
		logf("[信息] 打开配置文件：%s", configPath())
		openPath(configPath())
		return
	}

	// 单实例：已有实例则唤起并退出
	if _, first := acquireSingleInstance(); !first {
		logf("[信息] 检测到已有实例在运行")
		msgBox(appName, "程序已在运行。", MB_ICONINFORMATION)
		return
	}

	if firstRun {
		logf("[信息] 已生成配置文件：%s", configPath())
	}

	// 发布目录必须存在
	if err := ensurePublishDir(cfg); err != nil {
		logf("[警告] 创建发布目录失败: %v", err)
	}

	app := newApp(cfg)
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

	fmt.Printf("LocalDrive 已启动: %s/ (端口 %d)\n", app.BaseURL(), port)
	fmt.Println("按 Ctrl+C 停止")

	if cfg.AutoOpen {
		openURL(app.LocalURL() + "/")
	}

	tray.runLoop()

	close(stopCleaner)
	clearState()
	logf("[信息] 程序已退出")
}

// ---------------------------------------------------------------- 参数解析（Linux 版）

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
