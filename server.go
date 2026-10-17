package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// ---------------------------------------------------------------- App

// App 持有 HTTP 服务与当前生效的配置，支持运行时切换端口。
type App struct {
	mu      sync.Mutex
	httpSrv *http.Server
	ln      net.Listener
	port    int
	host    string
}

func newApp(cfg *Config) *App {
	return &App{port: cfg.Port}
}

// Port 返回当前实际监听端口。
func (a *App) Port() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.port
}

// BaseURL 返回用于对外分享的地址前缀。
func (a *App) BaseURL() string {
	a.mu.Lock()
	port, host := a.port, a.host
	a.mu.Unlock()
	cfg := getConfig()
	if u := normalizePublicURL(cfg.PublicURL); u != "" {
		return u
	}
	if host == "" {
		host = pickURLHost(cfg.URLHost)
	}
	return fmt.Sprintf("http://%s:%d", host, port)
}

// LocalURL 返回本机管理地址。
func (a *App) LocalURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", a.Port())
}

// Start 在指定端口启动服务；端口被占用时自动顺延。
func (a *App) Start(preferred int) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.startLocked(preferred)
}

func (a *App) startLocked(preferred int) (int, error) {
	ln, port, err := listenOn(preferred)
	if err != nil {
		return 0, err
	}
	srv := &http.Server{
		Handler:           a.router(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	old, oldLn := a.httpSrv, a.ln
	a.httpSrv, a.ln, a.port = srv, ln, port

	go func() {
		defer logPanic("http.Serve")
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logf("[错误] HTTP 服务异常退出: %v", err)
		}
	}()
	if old != nil {
		_ = old.Close()
	}
	if oldLn != nil {
		_ = oldLn.Close()
	}
	logf("[信息] HTTP 服务已启动，端口 %d，展示目录 %v", port, getConfig().EffectiveRoots())
	a.host = pickURLHost(getConfig().URLHost)
	return port, nil
}

// Restart 切换到新端口并重启服务。
func (a *App) Restart(port int) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.startLocked(port)
}

// Stop 关闭服务。
func (a *App) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = a.httpSrv.Shutdown(ctx)
		cancel()
	}
}

// ApplyConfig 保存配置并应用（端口变化时重启服务）。
func (a *App) ApplyConfig(next *Config) (port int, restarted bool, err error) {
	prev := getConfig()
	if err := saveConfig(next); err != nil {
		return a.Port(), false, fmt.Errorf("保存配置失败: %w", err)
	}
	setConfig(next)
	a.host = pickURLHost(next.URLHost)

	if next.Port != prev.Port {
		p, e := a.Restart(next.Port)
		if e != nil {
			return a.Port(), false, fmt.Errorf("切换端口失败: %w", e)
		}
		port, restarted = p, true
	} else {
		port = a.Port()
	}
	a.writeState()
	return port, restarted, nil
}

// writeState 把运行状态写入 state.json，供「发布」子进程读取真实端口。
func (a *App) writeState() {
	cfg := getConfig()
	saveState(&State{
		PID:  os.Getpid(),
		Port: a.Port(),
		Root: cfg.ResolveRoot(),
		Host: a.host,
		URL:  a.BaseURL(),
	})
}

// ---------------------------------------------------------------- 监听

func listenOn(preferred int) (net.Listener, int, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", preferred))
	if err == nil {
		return ln, ln.Addr().(*net.TCPAddr).Port, nil
	}
	if preferred == 0 {
		return nil, 0, err
	}
	for p := preferred + 1; p <= preferred+20 && p <= 65535; p++ {
		if ln, e := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", p)); e == nil {
			logf("[警告] 端口 %d 已被占用，自动改用 %d", preferred, p)
			return ln, p, nil
		}
	}
	return nil, 0, err
}

// ---------------------------------------------------------------- 路由

func (a *App) router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			serveIndex(w, r)
		case "/settings":
			serveSettings(w, r)
		case "/favicon.ico":
			serveFavicon(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/api/info", a.serveInfo)
	mux.HandleFunc("/api/favicon", serveFaviconPreview)
	mux.HandleFunc("/api/list", a.serveList)
	mux.HandleFunc("/api/all", a.serveAll)
	mux.HandleFunc("/api/config", a.serveConfig)
	mux.HandleFunc("/raw/", func(w http.ResponseWriter, r *http.Request) { a.serveFile(w, r, false) })
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) { a.serveFile(w, r, true) })
	return mux
}

// ---------------------------------------------------------------- 数据结构

type entry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"`
	Ext     string `json:"ext"`
	Real    string `json:"realPath,omitempty"`
	Missing bool   `json:"missing,omitempty"`
}

// rootInfo 描述一个展示目录，供前端渲染面包屑与目录卡片。
type rootInfo struct {
	Index   int    `json:"index"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Missing bool   `json:"missing"`
}

// ---------------------------------------------------------------- 虚拟根

// rootPrefix 返回多目录模式下第 i 个目录的路径前缀（单目录时为空）。
func rootPrefix(i, n int) string {
	if n > 1 {
		return "@" + strconv.Itoa(i) + "/"
	}
	return ""
}

// splitRoot 把「可能带虚拟根前缀」的相对路径拆成真实根目录 + 剩余相对路径。
//
// 多目录模式下，路径第一段形如 @0 / @1 表示第 N 个展示目录；
// 没有前缀时按第 0 个处理，这样历史链接（不带前缀）依然可用。
func splitRoot(roots []string, rel string) (root string, sub string, ok bool) {
	if len(roots) == 0 {
		return "", "", false
	}
	rel = strings.Trim(strings.ReplaceAll(rel, "\\", "/"), "/")
	if len(roots) == 1 {
		return roots[0], rel, true
	}
	if rel == "" {
		return "", "", false
	}
	seg, rest, _ := strings.Cut(rel, "/")
	if strings.HasPrefix(seg, "@") {
		n, err := strconv.Atoi(seg[1:])
		if err != nil || n < 0 || n >= len(roots) {
			return "", "", false
		}
		return roots[n], rest, true
	}
	return roots[0], rel, true
}

// rootInfos 为每个展示目录生成显示信息（重名时自动加序号）。
func rootInfos(roots []string) []rootInfo {
	out := make([]rootInfo, 0, len(roots))
	used := map[string]int{}
	for i, r := range roots {
		name := filepath.Base(r)
		if name == "" || name == "." || name == string(filepath.Separator) {
			name = r
		}
		key := strings.ToLower(name)
		if n, ok := used[key]; ok {
			used[key] = n + 1
			name = fmt.Sprintf("%s (%d)", name, n+1)
		} else {
			used[key] = 1
		}
		missing := true
		if fi, err := os.Stat(r); err == nil && fi.IsDir() {
			missing = false
		}
		out = append(out, rootInfo{Index: i, Name: name, Path: r, Missing: missing})
	}
	return out
}

// parentOf 返回相对路径的上一级。
func parentOf(rel string) string {
	rel = strings.Trim(rel, "/")
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[:i]
	}
	return ""
}

// ---------------------------------------------------------------- 接口

func (a *App) serveInfo(w http.ResponseWriter, r *http.Request) {
	cfg := getConfig()
	host, _ := os.Hostname()
	roots := cfg.EffectiveRoots()
	writeJSON(w, http.StatusOK, map[string]any{
		"app":        appName,
		"version":    appVersion,
		"siteName":   cfg.SiteName,
		"pageTitle":  cfg.PageTitle,
		"footer":     cfg.FooterText,
		"faviconURL": faviconURL(cfg),
		"root":       cfg.ResolveRoot(),
		"roots":      rootInfos(roots),
		"publishDir": cfg.ResolvePublishDir(),
		"publishTTL": cfg.PublishTTL,
		"configPath": configPath(),
		"port":       a.Port(),
		"baseURL":    a.BaseURL(),
		"publicURL":  cfg.PublicURL,
		"host":       cfg.URLHost,
		"urlHost":    pickURLHost(cfg.URLHost),
		"machine":    host,
		"ips":        localIPs(),
		"hidden":     cfg.ShowHidden,
		"pageSize":   cfg.PageSize,
		"max":        cfg.MaxFiles,
		"hideTypes":  cfg.HideTypes,
		"hideRules":  cfg.HideRules,
	})
}

func (a *App) serveList(w http.ResponseWriter, r *http.Request) {
	cfg := getConfig()
	roots := cfg.EffectiveRoots()
	rel := strings.Trim(r.URL.Query().Get("p"), "/")
	infos := rootInfos(roots)

	if len(roots) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"mode": "dir", "path": "", "parent": "", "entries": []entry{},
			"roots": infos, "rootIndex": -1, "rootName": "", "rootReal": "",
			"dirs": 0, "files": 0, "size": 0,
			"notice": "还没有配置要展示的目录。请打开「设置」添加目录，或编辑 config.ini 的 [paths] roots。",
		})
		return
	}

	// 配置了多个目录：顶层先列出各个目录本身
	if len(roots) > 1 && rel == "" {
		items := make([]entry, 0, len(roots))
		for _, ri := range infos {
			items = append(items, entry{
				Name: ri.Name, Path: "@" + strconv.Itoa(ri.Index),
				IsDir: true, Real: ri.Path, Missing: ri.Missing,
			})
		}
		sort.SliceStable(items, func(i, j int) bool { return naturalLess(items[i].Name, items[j].Name) })
		writeJSON(w, http.StatusOK, map[string]any{
			"mode": "roots", "path": "", "parent": "", "entries": items,
			"roots": infos, "rootIndex": -1, "rootName": "", "rootReal": "",
			"dirs": len(items), "files": 0, "size": 0,
		})
		return
	}

	root, sub, ok := splitRoot(roots, rel)
	if !ok {
		writeErr(w, http.StatusBadRequest, "非法路径")
		return
	}
	a.writeListing(w, cfg, roots, infos, root, sub, rel)
}

// writeListing 列举某个真实目录下的内容。
func (a *App) writeListing(w http.ResponseWriter, cfg *Config, roots []string, infos []rootInfo, root, sub, rel string) {
	idx, name := -1, ""
	for i, rr := range roots {
		if rr == root {
			idx = i
			break
		}
	}
	if idx >= 0 {
		name = infos[idx].Name
	}

	full, ok := resolve(root, sub)
	if !ok {
		writeErr(w, http.StatusBadRequest, "非法路径")
		return
	}
	fi, err := os.Stat(full)
	if err != nil {
		writeErr(w, http.StatusNotFound, "路径不存在："+full)
		return
	}
	if !fi.IsDir() {
		writeErr(w, http.StatusBadRequest, "不是目录")
		return
	}
	des, err := os.ReadDir(full)
	if err != nil {
		writeErr(w, http.StatusForbidden, "无法读取目录: "+err.Error())
		return
	}

	prefix := rootPrefix(idx, len(roots))
	if sub != "" {
		prefix += strings.Trim(sub, "/") + "/"
	}

	items := make([]entry, 0, len(des))
	for _, de := range des {
		nm := de.Name()
		if !cfg.ShowHidden && strings.HasPrefix(nm, ".") {
			continue
		}
		if shouldHide(nm, cfg) {
			continue
		}
		info, err := de.Info()
		if err != nil {
			continue
		}
		isDir := info.IsDir()
		if info.Mode()&fs.ModeSymlink != 0 {
			if t, err := os.Stat(filepath.Join(full, nm)); err == nil {
				isDir = t.IsDir()
				info = t
			}
		}
		items = append(items, entry{
			Name: nm, Path: prefix + nm, IsDir: isDir,
			Size: info.Size(), ModTime: info.ModTime().UnixMilli(), Ext: extOf(nm),
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return naturalLess(items[i].Name, items[j].Name)
	})

	var files int
	var total int64
	for _, it := range items {
		if !it.IsDir {
			files++
			total += it.Size
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode": "dir", "path": rel, "parent": parentOf(rel),
		"roots": infos, "rootIndex": idx, "rootName": name, "rootReal": root,
		"entries": items, "dirs": len(items) - files, "files": files,
		"size": total, "truncated": false,
	})
}

func (a *App) serveAll(w http.ResponseWriter, r *http.Request) {
	cfg := getConfig()
	roots := cfg.EffectiveRoots()
	items := make([]entry, 0, 256)
	truncated := false

	for i, root := range roots {
		if truncated {
			break
		}
		prefix := rootPrefix(i, len(roots))
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || p == root {
				return nil
			}
			rel, e := filepath.Rel(root, p)
			if e != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if !cfg.ShowHidden && hasHiddenSegment(rel) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if shouldHide(d.Name(), cfg) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if len(items) >= cfg.MaxFiles {
				truncated = true
				return fs.SkipAll
			}
			info, e := d.Info()
			if e != nil {
				return nil
			}
			full := prefix + rel
			items = append(items, entry{
				Name: full, Path: full, Size: info.Size(),
				ModTime: info.ModTime().UnixMilli(), Ext: extOf(rel),
			})
			return nil
		})
	}

	sort.SliceStable(items, func(i, j int) bool { return naturalLess(items[i].Path, items[j].Path) })
	var total int64
	for _, it := range items {
		total += it.Size
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode": "all", "path": "", "parent": "", "entries": items,
		"roots": rootInfos(roots), "dirs": 0, "files": len(items),
		"size": total, "truncated": truncated,
	})
}

func (a *App) serveFile(w http.ResponseWriter, r *http.Request, asAttachment bool) {
	cfg := getConfig()
	roots := cfg.EffectiveRoots()
	prefix := "/raw/"
	if asAttachment {
		prefix = "/dl/"
	}
	rel, err := url.PathUnescape(strings.TrimPrefix(r.URL.EscapedPath(), prefix))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "路径编码错误")
		return
	}
	root, sub, ok := splitRoot(roots, rel)
	if !ok {
		writeErr(w, http.StatusForbidden, "禁止访问该路径")
		return
	}
	full, ok := resolve(root, sub)
	if !ok {
		writeErr(w, http.StatusForbidden, "禁止访问该路径")
		return
	}
	f, err := os.Open(full)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeErr(w, http.StatusNotFound, "文件不存在")
		} else {
			writeErr(w, http.StatusForbidden, "无法打开文件")
		}
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取文件信息失败")
		return
	}
	if fi.IsDir() {
		http.Redirect(w, r, "/?p="+url.QueryEscape(rel), http.StatusFound)
		return
	}
	name := fi.Name()
	if asAttachment {
		if v := mime.FormatMediaType("attachment", map[string]string{"filename": name}); v != "" {
			w.Header().Set("Content-Disposition", v)
		}
	}
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, name, fi.ModTime(), f)
}

// ---------------------------------------------------------------- 配置接口

type configPayload struct {
	Port       int    `json:"port"`
	Roots      string `json:"roots"` // 一行一个目录
	PublishDir string `json:"publishDir"`
	PublishTTL int    `json:"publishTTL"`
	URLHost    string `json:"urlHost"`
	PublicURL  string `json:"publicURL"`
	ShowHidden bool   `json:"showHidden"`
	PageSize   int    `json:"pageSize"`
	MaxFiles   int    `json:"maxFiles"`
	AutoOpen   bool   `json:"autoOpen"`
	HideTypes  string `json:"hideTypes"` // 逗号分隔
	HideRules  string `json:"hideRules"` // 逗号分隔

	SiteName   string `json:"siteName"`
	PageTitle  string `json:"pageTitle"`
	FooterText string `json:"footerText"`
	Favicon    string `json:"favicon"`
}

func (a *App) serveConfig(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r) {
		writeErr(w, http.StatusForbidden, "仅允许本机修改设置")
		return
	}
	cfg := getConfig()

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, configPayload{
			Port: cfg.Port, Roots: strings.Join(cfg.ResolveRoots(), "\n"),
			PublishDir: cfg.PublishDir, PublishTTL: cfg.PublishTTL,
			URLHost: cfg.URLHost, PublicURL: cfg.PublicURL, ShowHidden: cfg.ShowHidden,
			PageSize: cfg.PageSize, MaxFiles: cfg.MaxFiles, AutoOpen: cfg.AutoOpen,
			HideTypes: strings.Join(cfg.HideTypes, ", "),
			HideRules: strings.Join(cfg.HideRules, ", "),

			SiteName: cfg.SiteName, PageTitle: titleForINI(cfg),
			FooterText: cfg.FooterText, Favicon: cfg.Favicon,
		})

	case http.MethodPost:
		var p configPayload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeErr(w, http.StatusBadRequest, "参数格式错误")
			return
		}
		roots := splitPathList(p.Roots)
		for i, r := range roots {
			roots[i] = absClean(r)
		}
		next := &Config{
			Port: p.Port, Roots: roots, PublishDir: strings.TrimSpace(p.PublishDir),
			PublishTTL: p.PublishTTL, URLHost: strings.TrimSpace(p.URLHost),
			PublicURL:  strings.TrimSpace(p.PublicURL),
			ShowHidden: p.ShowHidden, PageSize: p.PageSize, MaxFiles: p.MaxFiles,
			AutoOpen:  p.AutoOpen,
			HideTypes: splitList(p.HideTypes),
			HideRules: splitList(p.HideRules),

			SiteName:   strings.TrimSpace(p.SiteName),
			PageTitle:  strings.TrimSpace(p.PageTitle),
			FooterText: strings.TrimSpace(p.FooterText),
			Favicon:    strings.TrimSpace(p.Favicon),
		}
		if next.Port < 1 || next.Port > 65535 {
			writeErr(w, http.StatusBadRequest, "端口必须在 1 - 65535 之间")
			return
		}
		next.normalize()

		var bad []string
		for _, r := range next.Roots {
			if fi, err := os.Stat(r); err != nil || !fi.IsDir() {
				bad = append(bad, r)
			}
		}
		if len(bad) > 0 {
			writeErr(w, http.StatusBadRequest, "目录不存在或不是文件夹：\n"+strings.Join(bad, "\n"))
			return
		}

		port, restarted, err := a.ApplyConfig(next)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		logf("[信息] 配置已更新：端口=%d 目录=%v 发布目录=%s 保留=%d 分钟 对外IP=%s 每页=%d",
			port, next.EffectiveRoots(), next.ResolvePublishDir(), next.PublishTTL,
			pickURLHost(next.URLHost), next.PageSize)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "port": port, "restarted": restarted, "baseURL": a.BaseURL(),
		})

	default:
		writeErr(w, http.StatusMethodNotAllowed, "不支持的方法")
	}
}

// ---------------------------------------------------------------- 路径安全

func resolve(root, rel string) (string, bool) {
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.TrimPrefix(rel, "/")
	full := filepath.Join(root, filepath.FromSlash(rel))
	if !within(root, full) {
		return "", false
	}
	if real, err := filepath.EvalSymlinks(full); err == nil {
		if !within(root, real) {
			return "", false
		}
		full = real
	}
	return full, true
}

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	if rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ---------------------------------------------------------------- 发布目录清理

// startPublishCleaner 启动后台清理：定期删除发布目录里超过保留时长的文件。
func startPublishCleaner(stop <-chan struct{}) {
	go func() {
		defer logPanic("publishCleaner")
		select {
		case <-time.After(30 * time.Second):
		case <-stop:
			return
		}
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			n, err := cleanPublishDir()
			if err != nil {
				logf("[警告] 清理发布目录失败: %v", err)
			} else if n > 0 {
				logf("[信息] 已自动清理 %d 个过期的发布文件", n)
			}
			select {
			case <-ticker.C:
			case <-stop:
				return
			}
		}
	}()
}

// cleanPublishDir 删除发布目录中超过保留时长的文件，并顺手清掉空目录。
func cleanPublishDir() (int, error) {
	cfg := getConfig()
	ttl := cfg.PublishTTL
	if ttl <= 0 {
		return 0, nil
	}
	dir := cfg.ResolvePublishDir()
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return 0, nil
	}
	// 安全护栏：发布目录若包含程序自身，绝不清理
	if within(dir, exePath()) {
		return 0, errors.New("发布目录包含程序自身，已跳过清理（请改为独立子目录）")
	}

	deadline := time.Now().Add(-time.Duration(ttl) * time.Minute)
	removed := 0
	var dirs []string
	walkErr := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != dir {
				dirs = append(dirs, p)
			}
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return nil
		}
		if info.ModTime().Before(deadline) && os.Remove(p) == nil {
			removed++
		}
		return nil
	})
	// 自底向上删除空目录（非空时 os.Remove 会失败，正好跳过）
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = os.Remove(dirs[i])
	}
	return removed, walkErr
}

// ---------------------------------------------------------------- 工具

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func extOf(name string) string {
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
}

func hasHiddenSegment(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if strings.HasPrefix(seg, ".") && seg != "." && seg != ".." {
			return true
		}
	}
	return false
}

// shouldHide 判断某个文件 / 文件夹名是否命中 config.ini 中 [hide] 段的规则。
//   - types：按扩展名匹配（不区分大小写，配置里可写可不写点号）
//   - rules：按文件名通配符匹配（* 与 ?，不区分大小写）
func shouldHide(name string, cfg *Config) bool {
	if ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), "."); ext != "" {
		for _, t := range cfg.HideTypes {
			if strings.EqualFold(strings.TrimPrefix(t, "."), ext) {
				return true
			}
		}
	}
	lower := strings.ToLower(name)
	for _, rule := range cfg.HideRules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		if ok, err := filepath.Match(strings.ToLower(rule), lower); err == nil && ok {
			return true
		}
	}
	return false
}

// ReloadConfig 重新读取 config.ini 并应用（供托盘菜单调用）。
func (a *App) ReloadConfig() (port int, restarted bool, err error) {
	next := loadConfig()
	prev := getConfig()
	if next.Port != prev.Port {
		if p, e := a.Restart(next.Port); e != nil {
			return a.Port(), false, e
		} else {
			port, restarted = p, true
		}
	} else {
		port = a.Port()
	}
	setConfig(next)
	a.mu.Lock()
	a.host = pickURLHost(next.URLHost)
	a.mu.Unlock()
	a.writeState()
	return port, restarted, nil
}

// naturalLess 自然排序：file2 排在 file10 之前。
func naturalLess(a, b string) bool {
	ra, rb := []rune(a), []rune(b)
	i, j := 0, 0
	for i < len(ra) && j < len(rb) {
		ca, cb := ra[i], rb[j]
		if unicode.IsDigit(ca) && unicode.IsDigit(cb) {
			si, sj := i, j
			for i < len(ra) && unicode.IsDigit(ra[i]) {
				i++
			}
			for j < len(rb) && unicode.IsDigit(rb[j]) {
				j++
			}
			na := strings.TrimLeft(string(ra[si:i]), "0")
			nb := strings.TrimLeft(string(rb[sj:j]), "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		la, lb := unicode.ToLower(ca), unicode.ToLower(cb)
		if la != lb {
			return la < lb
		}
		i++
		j++
	}
	return len(ra)-i < len(rb)-j
}

// localIPs 返回本机所有非回环 IPv4 地址。
func localIPs() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.IsLoopback() {
				continue
			}
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				out = append(out, ip4.String())
			}
		}
	}
	return out
}
