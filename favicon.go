package main

import (
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------- 站点图标

// faviconAsset 是解析后的图标内容。
type faviconAsset struct {
	Data     []byte
	Mime     string
	Redirect string // 非空表示直接跳转到外部地址
}

var faviconMimeByExt = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
	".ico":  "image/x-icon",
	".svg":  "image/svg+xml; charset=utf-8",
}

// faviconURL 返回页面 <link rel="icon"> 应使用的地址。
// 带版本参数，配置一改浏览器就会重新拉取，不会沿用旧缓存。
func faviconURL(cfg *Config) string {
	v := strings.TrimSpace(cfg.Favicon)
	low := strings.ToLower(v)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return html.EscapeString(v)
	}
	return "/favicon.ico?v=" + hashString(v)
}

// hashString 返回短哈希，仅用于缓存失效。
func hashString(s string) string {
	return strconv.FormatUint(uint64(crc32.ChecksumIEEE([]byte(s))), 16)
}

// resolveFavicon 把 [site] favicon 的配置值解析成可输出的内容。
//
// 支持五种写法，按顺序判断：
//  1. data: URI
//  2. http(s):// 图片地址（重定向）
//  3. 内联 <svg> 片段
//  4. 图片文件路径（相对路径按程序目录解析）
//  5. 其余按 emoji / 短文本处理，生成纯色图标
func resolveFavicon(cfg *Config) faviconAsset {
	v := strings.TrimSpace(cfg.Favicon)
	if v == "" {
		return svgFavicon(faviconSVG)
	}
	low := strings.ToLower(v)
	switch {
	case strings.HasPrefix(low, "data:"):
		if a, ok := dataURIFavicon(v); ok {
			return a
		}
		return svgFavicon(faviconSVG)
	case strings.HasPrefix(low, "http://"), strings.HasPrefix(low, "https://"):
		return faviconAsset{Redirect: v}
	case strings.HasPrefix(low, "<svg"), strings.HasPrefix(low, "<?xml"):
		return svgFavicon(v)
	}

	// 文件路径：绝对路径直接读；相对路径先按程序目录解析
	p := v
	if !filepath.IsAbs(p) {
		cand := filepath.Join(exeDir(), filepath.FromSlash(p))
		if fileExists(cand) {
			p = cand
		}
	}
	if fileExists(p) {
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			mime := faviconMimeByExt[strings.ToLower(filepath.Ext(p))]
			if mime == "" {
				mime = http.DetectContentType(data)
			}
			return faviconAsset{Data: data, Mime: mime}
		}
	}
	return textFavicon(v)
}

func svgFavicon(s string) faviconAsset {
	return faviconAsset{Data: []byte(s), Mime: "image/svg+xml; charset=utf-8"}
}

// textFavicon 用 emoji / 短文本生成一个纯色方形图标。
func textFavicon(text string) faviconAsset {
	r := []rune(strings.TrimSpace(text))
	if len(r) > 4 {
		r = r[:4]
	}
	if len(r) == 0 {
		return svgFavicon(faviconSVG)
	}
	label := html.EscapeString(string(r))
	size := 34
	if len(r) > 1 {
		size = 22
	}
	svg := fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">`+
			`<rect width="64" height="64" rx="14" fill="#2f6feb"/>`+
			`<text x="32" y="32" font-size="%d" text-anchor="middle" dominant-baseline="central" `+
			`fill="#fff" font-family="Segoe UI Emoji,Apple Color Emoji,Noto Color Emoji,sans-serif">%s</text></svg>`,
		size, label)
	return svgFavicon(svg)
}

// dataURIFavicon 解析 data:image/...;base64,... 形式的图标。
func dataURIFavicon(v string) (faviconAsset, bool) {
	rest := v[len("data:"):]
	comma := strings.IndexByte(rest, ',')
	if comma < 0 {
		return faviconAsset{}, false
	}
	meta, payload := rest[:comma], rest[comma+1:]
	mime, isB64 := meta, false
	if i := strings.IndexByte(meta, ';'); i >= 0 {
		mime = meta[:i]
		isB64 = strings.Contains(meta[i:], "base64")
	}
	if mime == "" {
		mime = "image/png"
	}
	if isB64 {
		raw := strings.TrimSpace(payload)
		if unescaped, err := url.PathUnescape(raw); err == nil {
			raw = unescaped
		}
		d, err := base64.StdEncoding.DecodeString(raw)
		if err != nil || len(d) == 0 {
			return faviconAsset{}, false
		}
		return faviconAsset{Data: d, Mime: mime}, true
	}
	s, err := url.PathUnescape(payload)
	if err != nil {
		s = payload
	}
	if s == "" {
		return faviconAsset{}, false
	}
	return faviconAsset{Data: []byte(s), Mime: mime}, true
}

// serveFavicon 输出浏览器标签图标（/favicon.ico）。
func serveFavicon(w http.ResponseWriter, r *http.Request) {
	writeFavicon(w, r, getConfig())
}

// serveFaviconPreview 按「尚未保存的候选值」渲染图标，供设置页实时预览。
func serveFaviconPreview(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query().Get("v")
	if len(v) > 64*1024 {
		http.Error(w, "too large", http.StatusRequestEntityTooLarge)
		return
	}
	writeFavicon(w, r, &Config{Favicon: v})
}

func writeFavicon(w http.ResponseWriter, r *http.Request, cfg *Config) {
	fa := resolveFavicon(cfg)
	if fa.Redirect != "" {
		http.Redirect(w, r, fa.Redirect, http.StatusFound)
		return
	}
	if len(fa.Data) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", fa.Mime)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Length", strconv.Itoa(len(fa.Data)))
	_, _ = w.Write(fa.Data)
}
