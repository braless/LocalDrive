package main

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// 由于程序以 GUI 子系统运行（无控制台窗口），所有运行信息写入日志文件，
// 便于出问题时排查：%LOCALAPPDATA%\LocalDrive\LocalDrive.log

var (
	logMu     sync.Mutex
	logFile   *os.File
	logToStd  bool
	logLimit  int64 = 2 << 20 // 超过 2MB 自动轮转
	logInited bool
)

func initLog() {
	logMu.Lock()
	defer logMu.Unlock()
	if logInited {
		return
	}
	logInited = true
	f, err := os.OpenFile(logPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	if fi, err := f.Stat(); err == nil && fi.Size() > logLimit {
		_ = f.Close()
		_ = os.Rename(logPath(), logPath()+".old")
		f, err = os.OpenFile(logPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return
		}
	}
	logFile = f
}

// logf 写入一行带时间戳的日志。
func logf(format string, a ...any) {
	line := time.Now().Format("2006-01-02 15:04:05.000") + "  " + fmt.Sprintf(format, a...)
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		_, _ = logFile.WriteString(line + "\r\n")
	}
	if logToStd {
		fmt.Println(line)
	}
}

// logPanic 捕获异常并记录，避免 GUI 程序静默退出。
func logPanic(where string) {
	if r := recover(); r != nil {
		logf("[PANIC] %s: %v", where, r)
	}
}

func closeLog() {
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
}
