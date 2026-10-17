//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// ---------------------------------------------------------------- 菜单命令 ID

const (
	cmdOpenPage      = 1001
	cmdCopyURL       = 1002
	cmdOpenSettings  = 1003
	cmdPortBase      = 1100
	cmdPortCustom    = 1199
	cmdOpenPublish   = 1202
	cmdChangePublish = 1204
	cmdAddRoot       = 1205
	cmdCleanPublish  = 1206
	cmdRootOpenBase  = 1210 // 1210..1219 打开第 N 个展示目录
	cmdRootDelBase   = 1230 // 1230..1239 移除第 N 个展示目录
	cmdIPAuto        = 1300
	cmdIPBase        = 1310
	cmdToggleShell   = 1401
	cmdOpenConfigDir = 1402
	cmdOpenLog       = 1403
	cmdAbout         = 1404
	cmdEditConfig    = 1405
	cmdReloadConfig  = 1406
	cmdToggleWin11   = 1407
	cmdQuit          = 1999
)

// maxRootMenuItems 限制菜单里最多列出多少个展示目录。
const maxRootMenuItems = 10

// maxIPMenuItems 限制「本机 IP」子菜单最多列出多少个地址。
const maxIPMenuItems = 10

// shortenPath 把过长路径缩成「盘符\...\末两级」，避免菜单项被撑爆。
func shortenPath(p string, max int) string {
	r := []rune(p)
	if len(r) <= max {
		return p
	}
	vol := filepath.VolumeName(p)
	rest := strings.TrimPrefix(p, vol)
	parts := strings.Split(strings.Trim(rest, `\`), `\`)
	if len(parts) <= 2 {
		return p
	}
	return vol + `\...\` + strings.Join(parts[len(parts)-2:], `\`)
}

var portPresets = []int{8080, 8081, 8888, 9000, 18080}

type trayApp struct {
	hwnd           uintptr
	hIcon          uintptr
	app            *App
	nid            notifyIconDataW
	taskbarCreated uint32
	lastIPs        []string
}

var trayInstance *trayApp

// ---------------------------------------------------------------- 启动

// initTray 创建托盘窗口并挂载图标。必须在锁定 OS 线程后调用。
func initTray(app *App) *trayApp {
	t := &trayApp{app: app}
	trayInstance = t

	hInst := moduleHandle()
	cls := utf16Ptr(trayWindowClass)
	wc := wndClassExW{
		CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		LpfnWndProc:   syscall.NewCallback(trayWndProc),
		HInstance:     hInst,
		LpszClassName: cls,
	}
	if r, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		logf("[错误] 注册窗口类失败: %v", err)
		return t
	}

	title := utf16Ptr(appName)
	hwnd, _, err := pCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(cls)),
		uintptr(unsafe.Pointer(title)),
		0, 0, 0, 0, 0, // 无样式、零尺寸、不显示
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		logf("[错误] 创建托盘窗口失败: %v", err)
		return t
	}
	t.hwnd = hwnd

	// 资源管理器重启后需要重新添加图标
	if name := utf16Ptr("TaskbarCreated"); name != nil {
		if m, _, _ := pRegisterWindowMsg.Call(uintptr(unsafe.Pointer(name))); m != 0 {
			t.taskbarCreated = uint32(m)
		}
	}

	t.hIcon = makeHIcon(32)
	t.addIcon()
	logf("[信息] 托盘图标已就绪 hwnd=0x%x", hwnd)
	return t
}

// runLoop 运行托盘消息循环（阻塞，直到退出）。
func (t *trayApp) runLoop() {
	defer logPanic("runLoop")

	var msg msgT
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	t.delIcon()
	if t.hIcon != 0 {
		pDestroyIcon.Call(t.hIcon)
	}
	logf("[信息] 托盘消息循环已结束")
}

// ---------------------------------------------------------------- 窗口过程

func trayWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	t := trayInstance
	if t != nil {
		switch msg {
		case WM_TRAYICON:
			switch lParam {
			case WM_RBUTTONUP:
				t.showMenu()
				return 0
			case WM_LBUTTONDBLCLK:
				openURL(t.app.LocalURL() + "/")
				return 0
			}
		case WM_COPYDATA:
			t.handleCopyData(lParam)
			return 1
		case WM_DESTROY:
			pPostQuitMessage.Call(0)
			return 0
		}
		if t.taskbarCreated != 0 && uint32(msg) == t.taskbarCreated {
			// 资源管理器重启，重新挂载图标
			logf("[信息] 检测到任务栏重建，重新添加托盘图标")
			t.addIcon()
			return 0
		}
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

// ---------------------------------------------------------------- 托盘图标

func (t *trayApp) addIcon() {
	t.nid = notifyIconDataW{
		CbSize:           uint32(unsafe.Sizeof(notifyIconDataW{})),
		HWnd:             t.hwnd,
		UID:              1,
		UFlags:           NIF_MESSAGE | NIF_ICON | NIF_TIP,
		UCallbackMessage: WM_TRAYICON,
		HIcon:            t.hIcon,
	}
	copyUTF16(t.nid.SzTip[:], appName+" - 文件夹发布服务（右键查看菜单）")
	if r, _, err := pShellNotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(&t.nid))); r == 0 {
		logf("[错误] 添加托盘图标失败: %v", err)
	}
}

func (t *trayApp) delIcon() {
	nid := t.nid
	nid.UFlags = 0
	pShellNotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&nid)))
}

// balloon 弹出气泡通知（Win10+ 会显示为系统通知）。
func (t *trayApp) balloon(title, body string, flags uint32) {
	nid := t.nid
	nid.UFlags = NIF_INFO
	nid.DwInfoFlags = flags
	copyUTF16(nid.SzInfoTitle[:], title)
	copyUTF16(nid.SzInfo[:], body)
	if r, _, _ := pShellNotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&nid))); r == 0 {
		logf("[警告] 气泡通知发送失败（可能已被系统通知设置屏蔽）")
	}
}

// ---------------------------------------------------------------- 右键菜单

func menuAdd(hMenu uintptr, flags uintptr, id uint32, text string) {
	pAppendMenuW.Call(hMenu, flags, uintptr(id), uintptr(unsafe.Pointer(utf16Ptr(text))))
}

func menuSep(hMenu uintptr) {
	pAppendMenuW.Call(hMenu, MF_SEPARATOR, 0, 0)
}

func menuAddSub(hMenu, sub uintptr, text string) {
	pAppendMenuW.Call(hMenu, MF_POPUP|MF_STRING, sub, uintptr(unsafe.Pointer(utf16Ptr(text))))
}

func newSubMenu() uintptr {
	h, _, _ := pCreatePopupMenu.Call()
	return h
}

func (t *trayApp) showMenu() {
	defer logPanic("showMenu")
	cfg := getConfig()

	hMenu := newSubMenu()
	if hMenu == 0 {
		return
	}
	defer pDestroyMenu.Call(hMenu)

	menuAdd(hMenu, MF_STRING|MF_GRAYED, 0,
		fmt.Sprintf("%s v%s  ·  端口 %d", appName, appVersion, t.app.Port()))
	menuSep(hMenu)

	menuAdd(hMenu, MF_STRING, cmdOpenPage, "打开文件列表")
	menuAdd(hMenu, MF_STRING, cmdCopyURL, "复制访问地址")
	menuAdd(hMenu, MF_STRING, cmdOpenSettings, "打开设置页 ...")
	menuSep(hMenu)

	// ---- 指定端口
	sub := newSubMenu()
	cur := t.app.Port()
	for i, p := range portPresets {
		flags := uintptr(MF_STRING)
		if p == cur {
			flags |= MF_CHECKED
		}
		menuAdd(sub, flags, uint32(cmdPortBase+i), strconv.Itoa(p))
	}
	menuSep(sub)
	menuAdd(sub, MF_STRING, cmdPortCustom, "自定义端口 ...")
	menuAddSub(hMenu, sub, "指定端口")

	// ---- 目录配置
	roots := cfg.ResolveRoots()
	sub2 := newSubMenu()
	if len(roots) == 0 {
		menuAdd(sub2, MF_STRING|MF_GRAYED, 0, "（尚未配置展示目录）")
	}
	for i, r := range roots {
		if i >= maxRootMenuItems {
			break
		}
		menuAdd(sub2, MF_STRING, uint32(cmdRootOpenBase+i), "打开  "+shortenPath(r, 48))
	}
	if len(roots) > 0 {
		menuSep(sub2)
	}
	menuAdd(sub2, MF_STRING, cmdAddRoot, "添加目录 ...")
	if len(roots) > 0 {
		subDel := newSubMenu()
		for i, r := range roots {
			if i >= maxRootMenuItems {
				break
			}
			menuAdd(subDel, MF_STRING, uint32(cmdRootDelBase+i), "移除  "+shortenPath(r, 48))
		}
		menuAddSub(sub2, subDel, "移除目录")
	}
	menuSep(sub2)
	menuAdd(sub2, MF_STRING, cmdOpenPublish, "打开发布目录（临时文件）")
	menuAdd(sub2, MF_STRING, cmdChangePublish, "更改发布目录 ...")
	if cfg.PublishTTL > 0 {
		menuAdd(sub2, MF_STRING, cmdCleanPublish,
			fmt.Sprintf("立即清理发布目录（%s 自动清理）", humanMinutes(cfg.PublishTTL)))
	} else {
		menuAdd(sub2, MF_STRING, cmdCleanPublish, "立即清理发布目录（自动清理已关闭）")
	}
	menuAddSub(hMenu, sub2, "目录配置")

	// ---- 本机 IP
	ips := localIPs()
	t.lastIPs = ips
	sub3 := newSubMenu()
	flagsAuto := uintptr(MF_STRING)
	if cfg.URLHost == "" {
		flagsAuto |= MF_CHECKED
	}
	menuAdd(sub3, flagsAuto, cmdIPAuto, "自动选择（推荐）")
	if len(ips) > 0 {
		menuSep(sub3)
		for i, ip := range ips {
			if i >= maxIPMenuItems {
				break
			}
			flags := uintptr(MF_STRING)
			if cfg.URLHost == ip {
				flags |= MF_CHECKED
			}
			menuAdd(sub3, flags, uint32(cmdIPBase+i), ip)
		}
	}
	menuAddSub(hMenu, sub3, "本机 IP")

	menuSep(hMenu)
	if isShellMenuInstalled() {
		menuAdd(hMenu, MF_STRING, cmdToggleShell, "移除传统右键菜单项")
	} else {
		menuAdd(hMenu, MF_STRING, cmdToggleShell, "安装传统右键菜单项")
	}
	if isWin11MenuInstalled() {
		menuAdd(hMenu, MF_STRING, cmdToggleWin11, "移除 Win11 一级菜单「发布」")
	} else {
		menuAdd(hMenu, MF_STRING, cmdToggleWin11, "安装 Win11 一级菜单「发布」")
	}
	menuAdd(hMenu, MF_STRING, cmdEditConfig, "编辑配置文件 config.ini")
	menuAdd(hMenu, MF_STRING, cmdReloadConfig, "重新加载配置")
	menuSep(hMenu)
	menuAdd(hMenu, MF_STRING, cmdOpenConfigDir, "打开配置目录")
	menuAdd(hMenu, MF_STRING, cmdOpenLog, "查看运行日志")
	menuAdd(hMenu, MF_STRING, cmdAbout, "关于")
	menuSep(hMenu)
	menuAdd(hMenu, MF_STRING, cmdQuit, "退出")

	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(t.hwnd)
	sel, _, _ := pTrackPopupMenu.Call(
		hMenu,
		TPM_RIGHTBUTTON|TPM_RETURNCMD,
		uintptr(pt.X), uintptr(pt.Y), 0, t.hwnd, 0,
	)
	pPostMessageW.Call(t.hwnd, WM_NULL, 0, 0)

	if sel != 0 {
		t.onCommand(int(sel))
	}
}

// ---------------------------------------------------------------- 菜单动作

func (t *trayApp) onCommand(id int) {
	defer logPanic("onCommand")
	cfg := getConfig()

	switch {
	case id == cmdOpenPage:
		openURL(t.app.LocalURL() + "/")
	case id == cmdCopyURL:
		if err := copyToClipboard(t.app.BaseURL() + "/"); err != nil {
			msgBox(appName, err.Error(), MB_ICONERROR)
		} else {
			t.balloon("已复制访问地址", t.app.BaseURL()+"/", NIIF_INFO)
		}
	case id == cmdOpenSettings:
		openURL(t.app.LocalURL() + "/settings")
	case id == cmdPortCustom:
		openURL(t.app.LocalURL() + "/settings")
	case id >= cmdPortBase && id < cmdPortBase+len(portPresets):
		t.changePort(portPresets[id-cmdPortBase])
	case id == cmdIPAuto:
		t.changeIP("")
	case id >= cmdIPBase && id < cmdIPBase+maxIPMenuItems:
		if i := id - cmdIPBase; i >= 0 && i < len(t.lastIPs) {
			t.changeIP(t.lastIPs[i])
		}
	case id >= cmdRootOpenBase && id < cmdRootOpenBase+maxRootMenuItems:
		if roots := cfg.ResolveRoots(); id-cmdRootOpenBase < len(roots) {
			openPath(roots[id-cmdRootOpenBase])
		}
	case id >= cmdRootDelBase && id < cmdRootDelBase+maxRootMenuItems:
		roots := cfg.ResolveRoots()
		if i := id - cmdRootDelBase; i < len(roots) {
			removed := roots[i]
			next := make([]string, 0, len(roots)-1)
			next = append(next, roots[:i]...)
			next = append(next, roots[i+1:]...)
			t.applyConfig(func(c *Config) { c.Roots = next; c.Root = "" })
			t.balloon("已移除展示目录", removed, NIIF_INFO)
		}
	case id == cmdAddRoot:
		if p, ok := pickFolder("选择要在网页上展示的目录", exeDir()); ok {
			p = absClean(p)
			dup := false
			for _, r := range cfg.ResolveRoots() {
				if strings.EqualFold(r, p) {
					dup = true
				}
			}
			if dup {
				t.balloon("该目录已在列表中", p, NIIF_WARNING)
				break
			}
			t.applyConfig(func(c *Config) {
				c.Roots = append(append([]string{}, c.Roots...), p)
				c.Root = ""
			})
			t.balloon("已添加展示目录", p, NIIF_INFO)
		}
	case id == cmdOpenPublish:
		ensurePublishDir(cfg)
		openPath(cfg.ResolvePublishDir())
	case id == cmdChangePublish:
		if p, ok := pickFolder("选择「发布」文件的临时存放目录", cfg.ResolvePublishDir()); ok {
			t.applyConfig(func(c *Config) { c.PublishDir = p })
			t.balloon("发布目录已更新", cfg.ResolvePublishDir(), NIIF_INFO)
		}
	case id == cmdCleanPublish:
		n, err := cleanPublishDir()
		if err != nil {
			msgBox(appName, "清理失败：\n"+err.Error(), MB_ICONERROR)
			break
		}
		t.balloon("已清理发布目录", fmt.Sprintf("删除了 %d 个过期文件\n%s", n, cfg.ResolvePublishDir()), NIIF_INFO)
	case id == cmdToggleShell:
		if isShellMenuInstalled() {
			if err := uninstallShellMenu(); err != nil {
				msgBox(appName, "移除失败：\n"+err.Error(), MB_ICONERROR)
			} else {
				t.balloon("已移除右键菜单", "资源管理器中的「发布到 LocalDrive」已移除", NIIF_INFO)
			}
		} else {
			if err := installShellMenu(exePath()); err != nil {
				msgBox(appName, "安装失败：\n"+err.Error(), MB_ICONERROR)
			} else {
				t.balloon("已安装右键菜单", "在文件或文件夹上点右键，选择「发布到 LocalDrive」", NIIF_INFO)
			}
		}
	case id == cmdToggleWin11:
		if isWin11MenuInstalled() {
			if err := uninstallWin11Menu(); err != nil {
				msgBox(appName, "移除 Win11 一级菜单失败：\n"+err.Error(), MB_ICONERROR)
			} else {
				t.balloon("已移除 Win11 一级菜单", "「发布」已从 Win11 新式右键菜单中移除", NIIF_INFO)
			}
		} else {
			if err := installWin11Menu(); err != nil {
				msgBox(appName, "安装 Win11 一级菜单失败：\n"+err.Error(), MB_ICONERROR)
			} else {
				t.balloon("已安装 Win11 一级菜单", "现在在文件或文件夹上点右键，一级菜单里就有「发布」", NIIF_INFO)
			}
		}
	case id == cmdOpenConfigDir:
		openPath(appDir())
	case id == cmdEditConfig:
		openPath(configPath())
	case id == cmdReloadConfig:
		port, restarted, err := t.app.ReloadConfig()
		if err != nil {
			msgBox(appName, "重新加载配置失败：\n"+err.Error(), MB_ICONERROR)
			return
		}
		if restarted {
			t.balloon("配置已重新加载", fmt.Sprintf("端口已切换为 %d\n%s/", port, t.app.BaseURL()), NIIF_INFO)
		} else {
			t.balloon("配置已重新加载", fmt.Sprintf("端口 %d\n%s/", port, t.app.BaseURL()), NIIF_INFO)
		}
	case id == cmdOpenLog:
		openPath(logPath())
	case id == cmdAbout:
		roots := cfg.ResolveRoots()
		list := "（未配置）"
		if len(roots) > 0 {
			list = strings.Join(roots, "\n")
		}
		msgBox(appName+" v"+appVersion,
			"文件夹发布服务\n\n"+
				"展示目录（"+strconv.Itoa(len(roots))+" 个）：\n"+list+"\n\n"+
				"发布目录："+cfg.ResolvePublishDir()+"\n"+
				"保留时长："+humanMinutes(cfg.PublishTTL)+"（0 = 不清理）\n"+
				"监听端口："+strconv.Itoa(t.app.Port())+"\n"+
				"访问地址："+t.app.BaseURL()+"/\n\n"+
				"配置文件："+configPath()+"\n"+
				"运行日志："+logPath(),
			MB_ICONINFORMATION|MB_TOPMOST)
	case id == cmdQuit:
		t.quit()
	}
}

func (t *trayApp) changePort(p int) {
	cfg := getConfig()
	next := *cfg
	next.Port = p
	port, restarted, err := t.app.ApplyConfig(&next)
	if err != nil {
		msgBox(appName, err.Error(), MB_ICONERROR)
		return
	}
	if restarted {
		t.balloon("端口已切换", fmt.Sprintf("新地址：%s/\n端口 %d", t.app.BaseURL(), port), NIIF_INFO)
	} else {
		t.balloon("端口未变化", fmt.Sprintf("仍在 %d 端口：%s/", port, t.app.BaseURL()), NIIF_INFO)
	}
}

func (t *trayApp) changeIP(ip string) {
	cfg := getConfig()
	next := *cfg
	next.URLHost = ip
	if _, _, err := t.app.ApplyConfig(&next); err != nil {
		msgBox(appName, err.Error(), MB_ICONERROR)
		return
	}
	label := ip
	if label == "" {
		label = "自动选择"
	}
	t.balloon("对外 IP 已更新", label+"\n"+t.app.BaseURL()+"/", NIIF_INFO)
}

func (t *trayApp) applyConfig(mut func(*Config)) {
	cfg := getConfig()
	next := *cfg
	mut(&next)
	if _, _, err := t.app.ApplyConfig(&next); err != nil {
		msgBox(appName, err.Error(), MB_ICONERROR)
	}
}

func (t *trayApp) quit() {
	logf("[信息] 收到退出指令，正在停止服务")
	t.delIcon()
	t.app.Stop()
	clearState()
	pDestroyWindow.Call(t.hwnd)
}

// ---------------------------------------------------------------- 进程间消息

// handleCopyData 接收「发布」子进程发来的通知。
// 数据格式：命令 + 0x1f + 载荷
func (t *trayApp) handleCopyData(lParam uintptr) {
	defer logPanic("handleCopyData")
	cds := (*copyDataStruct)(ptrFromUintptr(lParam))
	if cds.CbData == 0 || cds.LpData == nil {
		return
	}
	n := int(cds.CbData) / 2
	raw := unsafe.Slice((*uint16)(cds.LpData), n)
	s := syscall.UTF16ToString(raw)

	cmd, payload, _ := strings.Cut(s, "\x1f")
	switch cmd {
	case "notify":
		title, body, _ := strings.Cut(payload, "\x1f")
		t.balloon(title, body, NIIF_INFO)
	case "error":
		title, body, _ := strings.Cut(payload, "\x1f")
		t.balloon(title, body, NIIF_ERROR)
	case "show":
		openURL(t.app.LocalURL() + "/")
	case "quit":
		t.quit()
	}
}
