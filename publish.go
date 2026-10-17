package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// runPublish 处理来自资源管理器右键菜单的「发布」请求。
//
// 流程：确保服务在运行 → （必要时）复制文件到发布目录 → 生成 http 地址 → 写入剪贴板 → 气泡通知。
// 若文件已经位于发布目录内，则跳过复制（只刷新时间戳，延长保留期），直接生成链接。
func runPublish(paths []string) {
	defer logPanic("runPublish")

	if len(paths) == 0 {
		return
	}
	logf("[信息] 收到发布请求，共 %d 项：%v", len(paths), paths)

	cfg := getConfig()
	pubDir := canon(cfg.ResolvePublishDir())

	if err := os.MkdirAll(pubDir, 0o755); err != nil {
		fail("无法创建发布目录", err.Error())
		return
	}

	port, err := ensureServerRunning()
	if err != nil {
		fail("服务未运行", "无法启动 LocalDrive 服务：\n"+err.Error())
		return
	}

	base := externalBaseURL(cfg, port)

	var urls []string
	seen := map[string]bool{}
	copied, linked := 0, 0

	for _, src := range paths {
		abs := canon(src)
		fi, err := os.Stat(abs)
		if err != nil {
			logf("[警告] 跳过无法访问的路径：%s", abs)
			continue
		}

		// 文件已在发布目录内：不再复制，只刷新时间戳以延长保留期
		if within(pubDir, abs) {
			now := time.Now()
			_ = os.Chtimes(abs, now, now)
			u, err := urlForTarget(base, cfg, abs, fi.IsDir())
			if err != nil {
				logf("[错误] %v", err)
				continue
			}
			if !seen[u] {
				seen[u] = true
				urls = append(urls, u)
			}
			linked++
			logf("[信息] 已在发布目录内，跳过复制：%s", abs)
			continue
		}

		var dst string
		if fi.IsDir() {
			dst, err = publishDirectory(abs, pubDir)
		} else {
			dst, err = publishFile(abs, pubDir)
		}
		if err != nil {
			logf("[错误] 发布失败 %s: %v", abs, err)
			continue
		}
		u, err := urlForTarget(base, cfg, dst, fi.IsDir())
		if err != nil {
			logf("[错误] %v", err)
			continue
		}
		if seen[u] {
			continue
		}
		seen[u] = true
		urls = append(urls, u)
		copied++
		logf("[信息] 已复制并发布 %s -> %s", abs, u)
	}

	if len(urls) == 0 {
		fail("发布失败", "没有成功发布任何文件，请查看运行日志。")
		return
	}

	text := strings.Join(urls, "\r\n")
	if err := copyToClipboard(text); err != nil {
		fail("已发布但复制失败", err.Error()+"\n\n地址：\n"+text)
		return
	}

	title := "已发布并复制链接"
	switch {
	case copied == 0:
		title = "链接已复制（文件已在发布目录内）"
	case linked == 0:
		title = fmt.Sprintf("已复制 %d 个文件并复制链接", copied)
	default:
		title = fmt.Sprintf("已复制 %d 个、%d 个已在发布目录内", copied, linked)
	}

	body := text
	if len(urls) > 1 {
		body = fmt.Sprintf("共 %d 个链接已复制到剪贴板\n%s\n...", len(urls), urls[0])
	}
	if cfg.PublishTTL > 0 {
		body += fmt.Sprintf("\n（%s 后自动清理）", humanMinutes(cfg.PublishTTL))
	}
	logf("[信息] %s：%s", title, text)
	notify("notify", title, body)
}

// humanMinutes 把分钟数写成「1 小时」「30 分钟」这类可读文本。
func humanMinutes(m int) string {
	if m >= 60 && m%60 == 0 {
		return fmt.Sprintf("%d 小时", m/60)
	}
	if m > 60 {
		return fmt.Sprintf("%d 小时 %d 分钟", m/60, m%60)
	}
	return fmt.Sprintf("%d 分钟", m)
}

// urlForTarget 为某个真实路径生成 HTTP 访问地址。
//
// 依次在「实际参与列举的目录」（配置的展示目录 + 必要时附加的发布目录）里
// 找到包含它的那一个，再按多目录规则补上 @N 前缀。
func urlForTarget(base string, cfg *Config, target string, isDir bool) (string, error) {
	target = absClean(target)
	roots := cfg.EffectiveRoots()
	for i, r := range roots {
		if !within(r, target) {
			continue
		}
		rel, err := filepath.Rel(r, target)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			rel = ""
		}
		return linkFor(base, rootPrefix(i, len(roots))+rel, isDir)
	}
	return "", fmt.Errorf("目标不在任何展示目录或发布目录内：%s", target)
}

// linkFor 把相对路径转成访问地址。
func linkFor(base, rel string, isDir bool) (string, error) {
	rel = strings.Trim(rel, "/")
	if isDir {
		if rel == "" {
			return base + "/", nil
		}
		return base + "/?p=" + url.QueryEscape(rel), nil
	}
	if rel == "" {
		return "", errors.New("无法生成文件链接：路径为空")
	}
	return base + "/raw/" + escapeRel(rel), nil
}

// canon 归一化路径：转绝对路径并解析符号链接 / 目录联接。
// 这样「文件是否已在发布目录内」的判断不受 junction、大小写、相对路径写法的影响。
func canon(p string) string { return absClean(p) }

// ---------------------------------------------------------------- 服务保活

// ensureServerRunning 确保后台服务在运行，返回真实监听端口。
func ensureServerRunning() (int, error) {
	if st := loadState(); st.Port > 0 && probeServer(st.Port) {
		return st.Port, nil
	}
	logf("[信息] 服务未运行，正在后台启动 ...")
	if err := spawnDetached(); err != nil {
		return 0, err
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(150 * time.Millisecond)
		if st := loadState(); st.Port > 0 && probeServer(st.Port) {
			// 再等一小会，确保托盘窗口已就绪，便于弹出通知
			time.Sleep(200 * time.Millisecond)
			return st.Port, nil
		}
	}
	return 0, errors.New("启动超时，请手动双击运行程序后再试")
}

// probeServer 探测指定端口上是否是我们自己的服务。
func probeServer(port int) bool {
	c := &http.Client{Timeout: 700 * time.Millisecond}
	resp, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/api/info", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var m map[string]any
	if json.NewDecoder(resp.Body).Decode(&m) != nil {
		return false
	}
	return m["app"] == appName
}

// ---------------------------------------------------------------- 复制

func ensurePublishDir(cfg *Config) error {
	return os.MkdirAll(cfg.ResolvePublishDir(), 0o755)
}

// publishFile 把单个文件复制到发布目录，返回目标绝对路径。
// 调用方需先确认源文件不在发布目录内。
func publishFile(src, pubDir string) (string, error) {
	fi, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(pubDir, filepath.Base(src))
	if ex, err := os.Stat(dst); err == nil && ex.Size() != fi.Size() {
		// 同名但内容不同，避免覆盖，另起一个名字
		dst = freeName(pubDir, filepath.Base(src))
	}
	if err := copyFile(src, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// publishDirectory 把整个文件夹复制到发布目录，返回目标绝对路径。
func publishDirectory(src, pubDir string) (string, error) {
	dst := filepath.Join(pubDir, filepath.Base(src))
	if _, err := os.Stat(dst); err == nil {
		dst = freeName(pubDir, filepath.Base(src))
	}
	if err := copyTree(src, dst); err != nil {
		return "", err
	}
	return dst, nil
}

func freeName(dir, name string) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; i < 1000; i++ {
		cand := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%d%s", stem, time.Now().UnixNano(), ext))
}

// copyFile 复制文件，并把目标文件的修改时间设为「现在」。
// 修改时间就是保留期的计时起点，所以不能沿用源文件的时间。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	now := time.Now()
	_ = os.Chtimes(dst, now, now)
	return nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, e := filepath.Rel(src, p)
		if e != nil {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(p, target)
	})
}

// escapeRel 对相对路径逐段做 URL 编码，保留 / 分隔符。
func escapeRel(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// ---------------------------------------------------------------- 通知

// notify 通过托盘进程弹气泡；托盘不可用时退化为消息框。
func notify(cmd, title, body string) {
	for i := 0; i < 8; i++ {
		if sendToTray(cmd, title+"\x1f"+body) {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	flags := uintptr(MB_ICONINFORMATION | MB_TOPMOST)
	if cmd == "error" {
		flags = MB_ICONERROR | MB_TOPMOST
	}
	msgBox(appName, title+"\n\n"+body, flags)
}

func fail(title, body string) {
	logf("[错误] %s：%s", title, body)
	notify("error", title, body)
}
