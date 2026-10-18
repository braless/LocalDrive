package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	appName    = "LocalDrive"
	appVersion = "2.5.0"

	// 托盘窗口类名，供 WM_COPYDATA 进程间通信定位
	trayWindowClass = "LocalDrive.TrayWindow.v2"
	// 单实例互斥体名称
	mutexName = "LocalDrive.SingleInstance.v2"
)

// ---------------------------------------------------------------- 配置

// Config 对应程序目录下的 config.ini。
type Config struct {
	Port       int      // [server]  port
	AutoOpen   bool     // [server]  autoOpen
	Roots      []string // [paths]   roots       —— 网页展示的目录，可配置多个
	Root       string   // [paths]   root        —— 旧版单目录写法（仅用于兼容读取）
	PublishDir string   // [paths]   publishDir  —— 发布文件临时目录
	PublishTTL int      // [publish] ttlMinutes  —— 发布文件保留分钟数，0 = 不自动清理
	URLHost    string   // [network] urlHost
	PublicURL  string   // [network] publicURL   —— 对外访问地址前缀（域名 / 反向代理）
	ShowHidden bool     // [listing] showHidden
	PageSize   int      // [listing] pageSize
	MaxFiles   int      // [listing] maxFiles
	HideTypes  []string // [hide]    types       —— 按扩展名隐藏
	HideRules  []string // [hide]    rules       —— 按通配符隐藏

	// 站点外观
	SiteName   string // [site] name     —— 网页左上角名称
	PageTitle  string // [site] title    —— 浏览器标签名称，留空 = 同 name
	FooterText string // [site] footer   —— 底部介绍，留空 = 不显示
	Favicon    string // [site] favicon  —— 图标：文件路径 / 文本 / SVG / URL / data:
}

func defaultConfig() *Config {
	return &Config{
		Port:       8080,
		PageSize:   100,
		MaxFiles:   20000,
		PublishTTL: 60,
		SiteName:   "文件服务",
		FooterText: "由 LocalDrive 提供",
		HideTypes:  []string{"tmp", "temp", "log", "bak", "swp", "swo", "lock", "pyc"},
		HideRules: []string{
			"~$*", ".git", ".svn", "node_modules", "Thumbs.db", "desktop.ini",
			"config.ini", "config.ini.bak", "state.json",
		},
	}
}

// State 记录运行中实例的真实端口与地址，供「发布」子进程读取。
type State struct {
	PID  int    `json:"pid"`
	Port int    `json:"port"`
	Root string `json:"root"`
	Host string `json:"host"`
	URL  string `json:"url"`
	Time int64  `json:"time"`
}

var (
	cfgMu   sync.RWMutex
	curCfg  *Config
	pathMu  sync.Mutex
	appRoot string
)

// ---------------------------------------------------------------- 路径

// appDir 返回运行时数据目录（日志 / 状态 / 图标）。
// 配置文件不在这里，而是与 exe 放在一起，方便用户直接编辑。
func appDir() string {
	pathMu.Lock()
	defer pathMu.Unlock()
	if appRoot != "" {
		return appRoot
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.Getenv("APPDATA")
	}
	if base == "" {
		base = os.TempDir()
	}
	appRoot = filepath.Join(base, appName)
	_ = os.MkdirAll(appRoot, 0o755)
	return appRoot
}

var (
	cfgPathOnce sync.Once
	cfgPathVal  string
)

// configPath 返回配置文件路径：优先与程序放在同一目录。
// 如果程序目录不可写（例如装在 Program Files 下），自动退回 %LOCALAPPDATA%。
func configPath() string {
	cfgPathOnce.Do(func() {
		p := filepath.Join(exeDir(), "config.ini")
		if fileExists(p) || dirWritable(exeDir()) {
			cfgPathVal = p
			return
		}
		cfgPathVal = filepath.Join(appDir(), "config.ini")
		logf("[警告] 程序目录不可写，配置文件改用 %s", cfgPathVal)
	})
	return cfgPathVal
}

// legacyConfigPaths 返回旧版配置文件的位置（%LOCALAPPDATA%\LocalDrive）。
func legacyConfigPaths() []string {
	d := appDir()
	return []string{
		filepath.Join(d, "config.ini"),
		filepath.Join(d, "config.json"),
	}
}

func statePath() string { return filepath.Join(appDir(), "state.json") }
func logPath() string   { return filepath.Join(appDir(), "LocalDrive.log") }

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// dirWritable 判断目录是否可写。
func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".localdrive-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// exePath 返回当前可执行文件的绝对路径。
func exePath() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return p
}

// exeDir 返回可执行文件所在目录。
func exeDir() string {
	p := exePath()
	if p == "" {
		if wd, err := os.Getwd(); err == nil {
			return wd
		}
		return "."
	}
	return filepath.Dir(p)
}

// ---------------------------------------------------------------- 读写

// loadConfig 读取 config.ini；不存在时从旧位置迁移，或生成一份带注释的默认配置。
func loadConfig() *Config {
	if data, err := os.ReadFile(configPath()); err == nil {
		cfg := defaultConfig()
		applyINI(cfg, parseINI(string(data)))
		cfg.normalize()
		return cfg
	}

	for _, old := range legacyConfigPaths() {
		if !fileExists(old) {
			continue
		}
		if strings.EqualFold(filepath.Ext(old), ".json") {
			if data, err := os.ReadFile(old); err == nil {
				cfg := defaultConfig()
				if json.Unmarshal(data, cfg) == nil {
					cfg.normalize()
					if saveConfig(cfg) == nil {
						_ = os.Rename(old, old+".bak")
						logf("[信息] 已把旧版 config.json 迁移为 %s", configPath())
						return cfg
					}
				}
			}
			continue
		}
		if data, err := os.ReadFile(old); err == nil {
			cfg := defaultConfig()
			applyINI(cfg, parseINI(string(data)))
			cfg.normalize()
			if saveConfig(cfg) == nil {
				_ = os.Rename(old, old+".bak")
				logf("[信息] 已把配置文件迁移到程序目录：%s", configPath())
				return cfg
			}
		}
	}

	cfg := defaultConfig()
	cfg.normalize()
	if err := saveConfig(cfg); err != nil {
		logf("[警告] 写入默认配置失败: %v", err)
	}
	return cfg
}

func (c *Config) normalize() {
	if c.Port < 1 || c.Port > 65535 {
		c.Port = 8080
	}
	if c.PageSize < 10 {
		c.PageSize = 100
	}
	if c.PageSize > 5000 {
		c.PageSize = 5000
	}
	if c.MaxFiles < 100 {
		c.MaxFiles = 20000
	}
	if c.PublishTTL < 0 {
		c.PublishTTL = 0
	}
	if c.PublishTTL > 60*24*30 {
		c.PublishTTL = 60 * 24 * 30
	}

	c.SiteName = strings.TrimSpace(c.SiteName)
	if c.SiteName == "" {
		c.SiteName = "文件服务"
	}
	c.PageTitle = strings.TrimSpace(c.PageTitle)
	if c.PageTitle == "" {
		c.PageTitle = c.SiteName
	}
	c.FooterText = strings.TrimSpace(c.FooterText)
	c.Favicon = strings.TrimSpace(c.Favicon)
	c.PublicURL = normalizePublicURL(c.PublicURL)
}

// normalizePublicURL 归一化「对外访问地址」：补协议、去掉尾部斜杠。
func normalizePublicURL(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	return strings.TrimRight(s, "/")
}

// externalBaseURL 返回对外分享地址前缀：优先 publicURL，否则 http://<IP>:<端口>。
func externalBaseURL(cfg *Config, port int) string {
	if u := normalizePublicURL(cfg.PublicURL); u != "" {
		return u
	}
	return fmt.Sprintf("http://%s:%d", pickURLHost(cfg.URLHost), port)
}

// saveConfig 把配置写回 config.ini（保留注释，便于用户手工编辑）。
func saveConfig(cfg *Config) error {
	return os.WriteFile(configPath(), []byte(renderINI(cfg)), 0o644)
}

func loadState() *State {
	st := &State{}
	if b, err := os.ReadFile(statePath()); err == nil {
		_ = json.Unmarshal(b, st)
	}
	return st
}

func saveState(st *State) {
	st.Time = time.Now().UnixMilli()
	if b, err := json.Marshal(st); err == nil {
		_ = os.WriteFile(statePath(), b, 0o644)
	}
}

func clearState() {
	_ = os.Remove(statePath())
}

// ---------------------------------------------------------------- 当前配置（并发安全）

func getConfig() *Config {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	if curCfg == nil {
		return defaultConfig()
	}
	return curCfg
}

func setConfig(cfg *Config) {
	cfgMu.Lock()
	curCfg = cfg
	cfgMu.Unlock()
}

// ---------------------------------------------------------------- INI 解析 / 生成

// parseINI 解析极简 INI：支持 [section]、key = value、; 与 # 注释、引号包裹的值。
// 同一个 key 出现多次时，值按行累加（便于把「一个目录一行」写进配置文件）。
func parseINI(text string) map[string]map[string]string {
	out := map[string]map[string]string{}
	section := ""
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if i := strings.Index(line, "]"); i > 0 {
				section = strings.ToLower(strings.TrimSpace(line[1:i]))
				if _, ok := out[section]; !ok {
					out[section] = map[string]string{}
				}
			}
			continue
		}
		i := strings.Index(line, "=")
		if i < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:i]))
		val := strings.TrimSpace(line[i+1:])
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') ||
			(val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		if _, ok := out[section]; !ok {
			out[section] = map[string]string{}
		}
		if prev, ok := out[section][key]; ok && prev != val {
			out[section][key] = prev + "\n" + val
		} else {
			out[section][key] = val
		}
	}
	return out
}

func iniGet(ini map[string]map[string]string, section, key string) (string, bool) {
	s, ok := ini[strings.ToLower(section)]
	if !ok {
		return "", false
	}
	v, ok := s[strings.ToLower(key)]
	return v, ok
}

// firstLine 取多行值的第一行。
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func iniString(ini map[string]map[string]string, section, key string, def string) string {
	if v, ok := iniGet(ini, section, key); ok {
		return firstLine(v)
	}
	return def
}

func iniBool(ini map[string]map[string]string, section, key string, def bool) bool {
	v, ok := iniGet(ini, section, key)
	if !ok {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(firstLine(v))) {
	case "1", "true", "yes", "on", "y":
		return true
	case "0", "false", "no", "off", "n":
		return false
	}
	return def
}

func iniInt(ini map[string]map[string]string, section, key string, def int) int {
	v, ok := iniGet(ini, section, key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(firstLine(v)))
	if err != nil {
		return def
	}
	return n
}

// iniList 读取「名称列表」：按逗号或换行分隔（扩展名 / 通配符用）。
func iniList(ini map[string]map[string]string, section, key string, def []string) []string {
	v, ok := iniGet(ini, section, key)
	if !ok {
		return def
	}
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return splitList(v)
}

// iniPathList 读取「路径列表」：按 | 或换行分隔（Windows 路径里不会出现 |）。
func iniPathList(ini map[string]map[string]string, section, key string, def []string) []string {
	v, ok := iniGet(ini, section, key)
	if !ok {
		return def
	}
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return splitPathList(v)
}

func applyINI(cfg *Config, ini map[string]map[string]string) {
	cfg.Port = iniInt(ini, "server", "port", cfg.Port)
	cfg.AutoOpen = iniBool(ini, "server", "autoOpen", cfg.AutoOpen)

	cfg.Root = strings.TrimSpace(iniString(ini, "paths", "root", cfg.Root))
	cfg.Roots = iniPathList(ini, "paths", "roots", nil)
	if len(cfg.Roots) == 0 && cfg.Root != "" {
		cfg.Roots = []string{cfg.Root}
	}
	if len(cfg.Roots) > 0 {
		cfg.Root = ""
	}
	cfg.PublishDir = strings.TrimSpace(iniString(ini, "paths", "publishDir", cfg.PublishDir))

	cfg.PublishTTL = iniInt(ini, "publish", "ttlMinutes", cfg.PublishTTL)

	cfg.URLHost = strings.TrimSpace(iniString(ini, "network", "urlHost", cfg.URLHost))
	cfg.PublicURL = strings.TrimSpace(iniString(ini, "network", "publicURL", cfg.PublicURL))

	cfg.SiteName = iniString(ini, "site", "name", cfg.SiteName)
	cfg.PageTitle = iniString(ini, "site", "title", cfg.PageTitle)
	cfg.FooterText = iniString(ini, "site", "footer", cfg.FooterText)
	cfg.Favicon = iniString(ini, "site", "favicon", cfg.Favicon)

	cfg.ShowHidden = iniBool(ini, "listing", "showHidden", cfg.ShowHidden)
	cfg.PageSize = iniInt(ini, "listing", "pageSize", cfg.PageSize)
	cfg.MaxFiles = iniInt(ini, "listing", "maxFiles", cfg.MaxFiles)

	cfg.HideTypes = iniList(ini, "hide", "types", cfg.HideTypes)
	cfg.HideRules = iniList(ini, "hide", "rules", cfg.HideRules)
}

// renderINI 生成带完整注释的配置文件内容。
func renderINI(c *Config) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	w("; ============================================================\r\n")
	w(";  %s v%s  配置文件\r\n", appName, appVersion)
	w(";  本文件与 %s.exe 放在同一目录\r\n", appName)
	w(";  改完在托盘图标右键菜单里点「重新加载配置」即可生效\r\n")
	w(";  也可以在资源管理器里右键程序图标 →「编辑配置」直接打开\r\n")
	w("; ============================================================\r\n\r\n")

	w("[site]\r\n")
	w("; 网页左上角显示的名称\r\n")
	w("name = %s\r\n\r\n", c.SiteName)
	w("; 浏览器标签页标题；留空 = 与上面的 name 相同\r\n")
	w("title = %s\r\n\r\n", titleForINI(c))
	w("; 页面底部的介绍文字；留空 = 不显示该行\r\n")
	w("footer = %s\r\n\r\n", c.FooterText)
	w("; 浏览器标签图标（favicon）。可填：\r\n")
	w(";   1) 图片文件路径 —— 相对路径按程序目录解析，支持 png / ico / jpg / gif / webp / svg\r\n")
	w(";   2) emoji 或 1-4 个字符 —— 自动生成纯色图标\r\n")
	w(";   3) 内联 <svg>...</svg> 片段\r\n")
	w(";   4) http(s):// 图片地址\r\n")
	w(";   5) data:image/...;base64,...\r\n")
	w("; 留空 = 使用内置图标\r\n")
	w("favicon = %s\r\n\r\n", c.Favicon)

	w("[server]\r\n")
	w("; HTTP 监听端口；被占用时会自动向后顺延寻找可用端口\r\n")
	w("port = %d\r\n\r\n", c.Port)
	w("; 启动后是否自动打开浏览器\r\n")
	w("autoOpen = %v\r\n\r\n", c.AutoOpen)

	w("[paths]\r\n")
	w("; 网页上要展示的目录，可以写多个，用 | 分隔；也可以一行写一个\r\n")
	w(";   只配一个目录 → 网页直接展示该目录下的文件\r\n")
	w(";   配多个目录   → 网页先展示目录列表，再进入各自的下一级\r\n")
	w("; 留空 = 不展示任何目录（只保留下面「发布」的临时目录）\r\n")
	w("; 例： roots = D:\\共享资料 | E:\\照片\r\n")
	w("roots = %s\r\n\r\n", strings.Join(c.Roots, " | "))
	w("; 「发布」文件的临时存放目录，留空 = <程序目录>\\published\r\n")
	w("; 里面的文件是临时的，会按 [publish] ttlMinutes 自动清理\r\n")
	w("publishDir = %s\r\n\r\n", c.PublishDir)

	w("[publish]\r\n")
	w("; 发布文件的保留时长（分钟）。到期后自动删除；填 0 表示不自动清理\r\n")
	w("ttlMinutes = %d\r\n\r\n", c.PublishTTL)

	w("[network]\r\n")
	w("; 生成分享链接时使用的本机 IP，留空 = 自动选择（优先 192.168.x）\r\n")
	w("urlHost = %s\r\n\r\n", c.URLHost)
	w("; 对外访问地址前缀，用于生成分享链接。反向代理 / 域名访问时填这里\r\n")
	w("; 例：https://files.example.com  或  http://nas.local:8080\r\n")
	w("; 留空 = http://<上面的 IP>:<端口>\r\n")
	w("publicURL = %s\r\n\r\n", c.PublicURL)

	w("[listing]\r\n")
	w("; 是否列举以 . 开头的隐藏文件\r\n")
	w("showHidden = %v\r\n\r\n", c.ShowHidden)
	w("; 网页列表每页显示条数\r\n")
	w("pageSize = %d\r\n\r\n", c.PageSize)
	w("; 全量列举接口的文件数量上限\r\n")
	w("maxFiles = %d\r\n\r\n", c.MaxFiles)

	w("[hide]\r\n")
	w("; 按扩展名隐藏文件（不区分大小写，逗号分隔，不用写点号）\r\n")
	w("types = %s\r\n\r\n", strings.Join(c.HideTypes, ", "))
	w("; 按文件名 / 文件夹名通配符隐藏（支持 * 和 ?，逗号分隔）\r\n")
	w("; 例：~$*  *.log  .git  node_modules  Thumbs.db\r\n")
	w("rules = %s\r\n", strings.Join(c.HideRules, ", "))

	return b.String()
}

// titleForINI 输出配置用的标题：与站点名称相同时写空，表示「跟随 name」。
func titleForINI(c *Config) string {
	if c.PageTitle == c.SiteName {
		return ""
	}
	return c.PageTitle
}

// splitList 拆分「名称列表」，按逗号 / 竖线 / 换行。
func splitList(s string) []string {
	return splitBy(s, func(r rune) bool {
		return r == ',' || r == '|' || r == '\n' || r == '\r'
	})
}

// splitPathList 拆分「路径列表」，按竖线 / 换行（路径里可能出现逗号，故不按逗号拆）。
func splitPathList(s string) []string {
	return splitBy(s, func(r rune) bool {
		return r == '|' || r == '\n' || r == '\r'
	})
}

func splitBy(s string, sep func(rune) bool) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, sep) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---------------------------------------------------------------- 派生路径

// absClean 转绝对路径并解析符号链接 / 目录联接。
func absClean(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	abs = filepath.Clean(abs)
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	return abs
}

// ResolveRoots 返回配置中所有根目录的绝对路径（去空、去重）。
func (c *Config) ResolveRoots() []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		abs := absClean(p)
		key := strings.ToLower(abs)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, abs)
	}
	for _, r := range c.Roots {
		add(r)
	}
	if len(out) == 0 && strings.TrimSpace(c.Root) != "" {
		add(c.Root)
	}
	return out
}

// ResolveRoot 返回第一个根目录（没有配置时退回程序目录），仅用于展示。
func (c *Config) ResolveRoot() string {
	if roots := c.ResolveRoots(); len(roots) > 0 {
		return roots[0]
	}
	return absClean(exeDir())
}

// EffectiveRoots 返回实际参与列举的目录：
// 配置的根目录 + （不在任何根目录内、且已经有文件的）发布临时目录。
func (c *Config) EffectiveRoots() []string {
	roots := c.ResolveRoots()
	pub := c.ResolvePublishDir()
	for _, r := range roots {
		if within(r, pub) {
			return roots
		}
	}
	if !dirHasEntries(pub) {
		return roots
	}
	out := make([]string, 0, len(roots)+1)
	out = append(out, roots...)
	out = append(out, pub)
	return out
}

// dirHasEntries 判断目录是否存在且至少含有一个条目。
func dirHasEntries(dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return false
	}
	defer f.Close()
	names, err := f.Readdirnames(1)
	return err == nil && len(names) > 0
}

// ResolvePublishDir 返回「发布」文件的临时存放目录（绝对路径）。
//
// 安全护栏：如果配置指向了包含程序自身的目录（例如被误填成程序目录或盘符根目录），
// 自动回退到 <程序目录>\published —— 否则 1 小时自动清理会把程序文件删掉。
func (c *Config) ResolvePublishDir() string {
	fallback := filepath.Join(exeDir(), "published")
	dir := ""
	if strings.TrimSpace(c.PublishDir) != "" {
		dir = absClean(c.PublishDir)
	}
	if dir == "" {
		return fallback
	}
	if within(dir, exePath()) || strings.EqualFold(dir, exeDir()) {
		return fallback
	}
	return dir
}

// ---------------------------------------------------------------- 本机 IP

// pickURLHost 返回生成对外链接时应使用的 IP。
func pickURLHost(prefer string) string {
	ips := localIPs()
	if prefer != "" {
		for _, ip := range ips {
			if ip == prefer {
				return ip
			}
		}
	}
	return bestIP(ips)
}

// bestIP 从本机 IPv4 中挑选最适合局域网分享的一个。
func bestIP(ips []string) string {
	if len(ips) == 0 {
		return "127.0.0.1"
	}
	score := func(ip string) int {
		switch {
		case strings.HasPrefix(ip, "192.168."):
			return 40
		case strings.HasPrefix(ip, "10."):
			return 30
		case strings.HasPrefix(ip, "172."):
			return 20
		default:
			return 10
		}
	}
	best := ips[0]
	for _, ip := range ips[1:] {
		if score(ip) > score(best) {
			best = ip
		}
	}
	return best
}
