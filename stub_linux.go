//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

// ---------------------------------------------------------------- 常量

const (
	appMenuLabel   = "编辑配置"
	menuLabel      = "发布到 LocalDrive"
	win11VerbTitle = "发布"

	// detachedEnvKey 标记「本进程由 detach 拉起」，用于避免无限自我重启。
	detachedEnvKey = "LOCALDRIVE_DETACHED"

	NIIF_INFO    = 1
	NIIF_WARNING = 2
	NIIF_ERROR   = 3

	MB_ICONINFORMATION = 0x40
	MB_ICONWARNING     = 0x30
	MB_ICONERROR       = 0x10
	MB_TOPMOST         = 0x40000
)

// ---------------------------------------------------------------- 剪贴板

func copyToClipboard(text string) error {
	if cmd := findClipboardCmd("copy"); cmd != "" {
		return copyViaCmd(cmd, text)
	}
	return fmt.Errorf("未找到剪贴板工具（请安装 xclip、xsel 或 wl-clipboard）")
}

func readClipboard() string {
	if cmd := findClipboardCmd("read"); cmd != "" {
		out, _ := exec.Command("sh", "-c", cmd).Output()
		return string(out)
	}
	return ""
}

func findClipboardCmd(mode string) string {
	var candidates []string
	if mode == "copy" {
		candidates = []string{
			"xclip -selection clipboard",
			"xsel --clipboard --input",
			"wl-copy",
		}
	} else {
		candidates = []string{
			"xclip -selection clipboard -o",
			"xsel --clipboard --output",
			"wl-paste",
		}
	}
	for _, c := range candidates {
		if _, err := exec.LookPath(strings.Fields(c)[0]); err == nil {
			return c
		}
	}
	return ""
}

func copyViaCmd(cmd string, text string) error {
	c := exec.Command("sh", "-c", cmd)
	c.Stdin = strings.NewReader(text)
	return c.Run()
}

// ---------------------------------------------------------------- 打开/通知

func openURL(url string) {
	_ = exec.Command("xdg-open", url).Start()
}

func openPath(path string) {
	_ = exec.Command("xdg-open", path).Start()
}

func msgBox(title, text string, flags uintptr) {
	fmt.Println("[" + title + "] " + text)
}

// ---------------------------------------------------------------- 单实例

// instanceLock 持有单实例锁的文件描述符。
// 必须用包级变量挂住，否则 os.File 一旦被 GC 回收就会关闭 fd，
// flock 随之释放，单实例保护形同虚设。
var instanceLock *os.File

func acquireSingleInstance() (uintptr, bool) {
	lockFile := filepath.Join(os.TempDir(), "localdrive.lock")
	f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0o666)
	if err != nil {
		return 0, false
	}
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		_ = f.Close()
		return 0, false
	}
	instanceLock = f
	return uintptr(f.Fd()), true
}

// releaseSingleInstance 主动释放单实例锁，让后续进程可以接管。
// 关闭文件描述符即释放 flock，无需显式 LOCK_UN。
func releaseSingleInstance() {
	if instanceLock != nil {
		_ = instanceLock.Close()
		instanceLock = nil
	}
}

func sendToTray(cmd, payload string) bool {
	return false
}

// ---------------------------------------------------------------- 进程管理

func spawnDetached() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	c := exec.Command(exe)
	// Setsid 让子进程成为新会话的首进程，彻底脱离控制终端。
	// 比依赖 nohup 更可靠，也不要求系统装了 coreutils。
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	// 标记为后台实例：子进程启动时不会再触发「超时转后台」的倒计时。
	env := make([]string, 0, len(os.Environ())+1)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, detachedEnvKey+"=") {
			env = append(env, e)
		}
	}
	c.Env = append(env, detachedEnvKey+"=1")

	c.Stdout = nil
	c.Stderr = nil
	c.Stdin = nil
	return c.Start()
}

// isTerminal 判断 stdin 是否真的连着终端。
// 只有从终端前台启动才需要「超时转后台」；由 systemd、nohup
// 或自身 detach 拉起的实例，stdin 都不是终端。
func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	if fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	// /dev/null 同样是字符设备，必须靠 fd 的实际指向再排除一次
	if link, err := os.Readlink("/proc/self/fd/0"); err == nil {
		return strings.HasPrefix(link, "/dev/pts/") ||
			strings.HasPrefix(link, "/dev/tty") ||
			strings.HasPrefix(link, "/dev/console")
	}
	return true
}

func attachConsole() bool {
	return true
}

// ---------------------------------------------------------------- 文件夹选择

func pickFolder(title, initial string) (string, bool) {
	if _, err := exec.LookPath("zenity"); err == nil {
		out, err := exec.Command("zenity", "--file-selection", "--directory", "--title="+title).Output()
		if err == nil {
			return strings.TrimSpace(string(out)), true
		}
	}
	if _, err := exec.LookPath("kdialog"); err == nil {
		out, err := exec.Command("kdialog", "--getexistingdirectory", initial, "--title", title).Output()
		if err == nil {
			return strings.TrimSpace(string(out)), true
		}
	}
	return "", false
}

// ---------------------------------------------------------------- 图标

func makeHIcon(size int) uintptr {
	return 0
}

func iconFilePath() string {
	return ""
}

// ---------------------------------------------------------------- COM 服务器

func isComServerInvocation(args []string) bool {
	return false
}

func runComServer() int {
	return 0
}

// ---------------------------------------------------------------- 右键菜单

func installShellMenu(exe string) error {
	return fmt.Errorf("Linux 不支持资源管理器右键菜单")
}

func uninstallShellMenu() error {
	return nil
}

func installAppMenu(exe, ico string) error {
	return fmt.Errorf("Linux 不支持")
}

func isShellMenuInstalled() bool {
	return false
}

func notifyShellChanged() {}

// ---------------------------------------------------------------- Win11 菜单

func installWin11Menu() error {
	return fmt.Errorf("Linux 不支持 Win11 一级菜单")
}

func uninstallWin11Menu() error {
	return nil
}

func ensureWin11PackageFiles() {}

// ---------------------------------------------------------------- 托盘

type trayApp struct {
	app *App
}

func initTray(app *App) *trayApp {
	return &trayApp{app: app}
}

func (t *trayApp) runLoop() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}

func (t *trayApp) balloon(title, body string, flags uint32) {
	fmt.Println("[通知] " + title + ": " + body)
}

// ---------------------------------------------------------------- 包级变量

var trayInstance *trayApp
