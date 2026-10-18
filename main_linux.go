//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// 从终端前台启动后，若这段时间内没有按 Ctrl+C，就自动转入后台运行。
const (
	autoDetachSeconds = 20
	autoDetachDelay   = autoDetachSeconds * time.Second
)

// LocalDrive —— Linux 文件夹发布服务（无托盘，控制台运行）
//
// 运行模式：
//   - 无参数：前台运行，HTTP 文件服务；若从终端启动，20 秒后自动转入后台
//   - -foreground：强制留在前台，不自动转后台
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

	// 只有「从终端前台启动」才启用倒计时：
	//   - systemd / nohup 拉起的实例，stdin 不是终端
	//   - 自身 detach 拉起的实例带 LOCALDRIVE_DETACHED 标记
	//   - 显式 -foreground 的实例要求留在前台
	autoDetach := isTerminal() &&
		os.Getenv(detachedEnvKey) == "" &&
		!hasFlag(args, "-foreground") &&
		!hasFlag(args, "--foreground") &&
		!hasFlag(args, "-f")

	if autoDetach {
		fmt.Printf("%d 秒后自动转入后台运行，按 Ctrl+C 立即停止\n", autoDetachSeconds)
	} else {
		fmt.Println("按 Ctrl+C 停止")
	}

	if cfg.AutoOpen {
		openURL(app.LocalURL() + "/")
	}

	detached := false

	if autoDetach {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		select {
		case sig := <-sigCh:
			logf("[信息] 收到信号 %v，停止前台实例", sig)
		case <-time.After(autoDetachDelay):
			signal.Stop(sigCh)
			if err := handoffToBackground(app); err != nil {
				logf("[警告] 转入后台失败，继续前台运行：%v", err)
				fmt.Printf("转入后台失败：%v\n继续前台运行，按 Ctrl+C 停止\n", err)
				tray.runLoop()
			} else {
				detached = true
			}
		}
	} else {
		tray.runLoop()
	}

	close(stopCleaner)
	if detached {
		// 服务已交给后台实例，此时不能 clearState，
		// 否则会删掉新实例刚写回的 state.json
		logf("[信息] 已转入后台运行 | 状态文件 %s", statePath())
		fmt.Printf("已转入后台运行，日志：%s\n", logPath())
	} else {
		clearState()
		logf("[信息] 程序已退出")
	}
}

// handoffToBackground 把正在运行的服务交给一个脱离终端的新实例。
//
// 顺序是关键：必须先停服务、释放单实例锁，再拉起新进程。
// 否则新进程会因端口仍被占用而顺延到下一个端口，
// 或因 flock 仍被持有而直接判定「程序已在运行」退出。
func handoffToBackground(app *App) error {
	cfg := getConfig()

	app.Stop()
	releaseSingleInstance()

	if err := spawnDetached(); err != nil {
		// 拉起失败：把端口和锁抢回来，退回前台继续服务
		if _, ok := acquireSingleInstance(); !ok {
			logf("[错误] 重新获取单实例锁失败")
		}
		p, e := app.Start(cfg.Port)
		if e != nil {
			return fmt.Errorf("后台启动失败(%v)，前台恢复也失败(%v)", err, e)
		}
		logf("[信息] 已恢复前台运行，端口 %d", p)
		return err
	}

	// 等新实例写回 state.json 并应答请求，确认它真的接管了服务
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(150 * time.Millisecond)
		if st := loadState(); st.Port > 0 && st.PID != os.Getpid() && probeServer(st.Port) {
			return nil
		}
	}
	logf("[警告] 后台实例未在 10 秒内应答，请查看日志确认")
	return nil
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
