//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

// ---------------------------------------------------------------- DLL

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")

	pRegisterClassExW     = user32.NewProc("RegisterClassExW")
	pCreateWindowExW      = user32.NewProc("CreateWindowExW")
	pDefWindowProcW       = user32.NewProc("DefWindowProcW")
	pDestroyWindow        = user32.NewProc("DestroyWindow")
	pGetMessageW          = user32.NewProc("GetMessageW")
	pTranslateMessage     = user32.NewProc("TranslateMessage")
	pDispatchMessageW     = user32.NewProc("DispatchMessageW")
	pPostQuitMessage      = user32.NewProc("PostQuitMessage")
	pPostMessageW         = user32.NewProc("PostMessageW")
	pSendMessageW         = user32.NewProc("SendMessageW")
	pFindWindowW          = user32.NewProc("FindWindowW")
	pSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	pGetCursorPos         = user32.NewProc("GetCursorPos")
	pCreatePopupMenu      = user32.NewProc("CreatePopupMenu")
	pAppendMenuW          = user32.NewProc("AppendMenuW")
	pDestroyMenu          = user32.NewProc("DestroyMenu")
	pTrackPopupMenu       = user32.NewProc("TrackPopupMenu")
	pLoadCursorW          = user32.NewProc("LoadCursorW")
	pMessageBoxW          = user32.NewProc("MessageBoxW")
	pRegisterWindowMsg    = user32.NewProc("RegisterWindowMessageW")
	pCreateIconFromResEx  = user32.NewProc("CreateIconFromResourceEx")
	pDestroyIcon          = user32.NewProc("DestroyIcon")
	pSetProcessDPIAware   = user32.NewProc("SetProcessDPIAware")
	pOpenClipboard        = user32.NewProc("OpenClipboard")
	pEmptyClipboard       = user32.NewProc("EmptyClipboard")
	pCloseClipboard       = user32.NewProc("CloseClipboard")
	pSetClipboardData     = user32.NewProc("SetClipboardData")
	pIsClipboardFormatAvl = user32.NewProc("IsClipboardFormatAvailable")
	pGetClipboardData     = user32.NewProc("GetClipboardData")

	pShellNotifyIconW   = shell32.NewProc("Shell_NotifyIconW")
	pSHBrowseForFolderW = shell32.NewProc("SHBrowseForFolderW")
	pSHGetPathFromIDL   = shell32.NewProc("SHGetPathFromIDListW")
	pShellExecuteW      = shell32.NewProc("ShellExecuteW")

	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	pGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	pGlobalLock       = kernel32.NewProc("GlobalLock")
	pGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	pGlobalFree       = kernel32.NewProc("GlobalFree")
	pCreateMutexW     = kernel32.NewProc("CreateMutexW")
	pGetLastError     = kernel32.NewProc("GetLastError")
	pCloseHandle      = kernel32.NewProc("CloseHandle")
	pAttachConsole    = kernel32.NewProc("AttachConsole")
	pGetConsoleWindow = kernel32.NewProc("GetConsoleWindow")
	pSetConsoleOutCP  = kernel32.NewProc("SetConsoleOutputCP")
	pGetConsoleMode   = kernel32.NewProc("GetConsoleMode")
	pSetConsoleMode   = kernel32.NewProc("SetConsoleMode")
	pSetConsoleTitle  = kernel32.NewProc("SetConsoleTitleW")

	pCoInitializeEx = ole32.NewProc("CoInitializeEx")
	pCoTaskMemFree  = ole32.NewProc("CoTaskMemFree")
)

// ---------------------------------------------------------------- 常量

const (
	WM_NULL          = 0x0000
	WM_DESTROY       = 0x0002
	WM_CLOSE         = 0x0010
	WM_COMMAND       = 0x0111
	WM_COPYDATA      = 0x004A
	WM_LBUTTONUP     = 0x0202
	WM_LBUTTONDBLCLK = 0x0203
	WM_RBUTTONUP     = 0x0205
	WM_APP           = 0x8000

	// 自定义托盘回调消息
	WM_TRAYICON = WM_APP + 1

	NIM_ADD        = 0x00000000
	NIM_MODIFY     = 0x00000001
	NIM_DELETE     = 0x00000002
	NIF_MESSAGE    = 0x00000001
	NIF_ICON       = 0x00000002
	NIF_TIP        = 0x00000004
	NIF_INFO       = 0x00000010
	NIIF_INFO      = 0x00000001
	NIIF_WARNING   = 0x00000002
	NIIF_ERROR     = 0x00000003
	NIIF_NOSOUND   = 0x00000010
	NIIF_RESPECTQT = 0x00000080

	MF_STRING    = 0x00000000
	MF_POPUP     = 0x00000010
	MF_SEPARATOR = 0x00000800
	MF_CHECKED   = 0x00000008
	MF_GRAYED    = 0x00000001
	MF_DISABLED  = 0x00000002
	MF_DEFAULT   = 0x00001000

	TPM_LEFTALIGN   = 0x0000
	TPM_RIGHTBUTTON = 0x0002
	TPM_RETURNCMD   = 0x0100

	IDC_ARROW = 32512

	MB_OK              = 0x00000000
	MB_ICONINFORMATION = 0x00000040
	MB_ICONWARNING     = 0x00000030
	MB_ICONERROR       = 0x00000010
	MB_SETFOREGROUND   = 0x00010000
	MB_TOPMOST         = 0x00040000

	CF_UNICODETEXT = 13
	GMEM_MOVEABLE  = 0x0002

	SW_SHOWNORMAL = 1

	BIF_RETURNONLYFSDIRS = 0x00000001
	BIF_NEWDIALOGSTYLE   = 0x00000040
	BIF_EDITBOX          = 0x00000010
	BIF_USENEWUI         = BIF_NEWDIALOGSTYLE | BIF_EDITBOX

	BFFM_INITIALIZED  = 1
	BFFM_SETSELECTION = 0x0467

	ERROR_ALREADY_EXISTS = 183
)

// ---------------------------------------------------------------- 结构体

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type msgT struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

type point struct{ X, Y int32 }

type notifyIconDataW struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

type copyDataStruct struct {
	DwData uintptr
	CbData uint32
	LpData unsafe.Pointer
}

type browseInfoW struct {
	HwndOwner      uintptr
	PidlRoot       uintptr
	PszDisplayName *uint16
	LpszTitle      *uint16
	UlFlags        uint32
	Lpfn           uintptr
	LParam         uintptr
	IImage         int32
}

// ---------------------------------------------------------------- 基础工具

// ptrFromUintptr 把 Win32 API / 窗口消息返回的地址值转换为 Go 指针。
//
// 这些地址由操作系统提供（窗口消息参数、GlobalLock 返回的全局内存），
// 不归 Go 堆管理，其生命周期由调用方保证，因此不存在 GC 移动或悬垂指针问题。
// 此处不直接写 unsafe.Pointer(p)，是为了明确表达「地址来自系统」这一前提。
func ptrFromUintptr(p uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&p))
}

func utf16Ptr(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		// 字符串含 NUL 时退化为空串，避免返回空指针
		empty, _ := syscall.UTF16PtrFromString("")
		return empty
	}
	return p
}

func copyUTF16(dst []uint16, s string) {
	src := syscall.StringToUTF16(s)
	n := len(src)
	if n > len(dst) {
		n = len(dst)
	}
	copy(dst[:n], src[:n])
}

func moduleHandle() uintptr {
	h, _, _ := pGetModuleHandleW.Call(0)
	return h
}

// ---------------------------------------------------------------- 图标

// makeHIcon 生成指定尺寸的 HICON，调用方负责 DestroyIcon。
func makeHIcon(size int) uintptr {
	bits := iconBits(size)
	h, _, _ := pCreateIconFromResEx.Call(
		uintptr(unsafe.Pointer(&bits[0])),
		uintptr(len(bits)),
		1, // fIcon = TRUE
		0x00030000,
		uintptr(size),
		uintptr(size),
		0, // LR_DEFAULTCOLOR
	)
	return h
}

// ---------------------------------------------------------------- 剪贴板

// copyToClipboard 把文本写入系统剪贴板。
func copyToClipboard(text string) error {
	const attempts = 10
	var ok uintptr
	for i := 0; i < attempts; i++ {
		ok, _, _ = pOpenClipboard.Call(0)
		if ok != 0 {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if ok == 0 {
		return errors.New("无法打开剪贴板（可能被其他程序占用）")
	}
	defer pCloseClipboard.Call()

	pEmptyClipboard.Call()

	data := syscall.StringToUTF16(text)
	size := uintptr(len(data) * 2)
	hMem, _, _ := pGlobalAlloc.Call(GMEM_MOVEABLE, size)
	if hMem == 0 {
		return errors.New("内存分配失败")
	}
	ptr, _, _ := pGlobalLock.Call(hMem)
	if ptr == 0 {
		pGlobalFree.Call(hMem)
		return errors.New("内存锁定失败")
	}
	// 直接拷贝到全局内存
	dst := unsafe.Slice((*uint16)(ptrFromUintptr(ptr)), len(data))
	copy(dst, data)
	pGlobalUnlock.Call(hMem)

	if r, _, _ := pSetClipboardData.Call(CF_UNICODETEXT, hMem); r == 0 {
		pGlobalFree.Call(hMem)
		return errors.New("写入剪贴板失败")
	}
	// 所有权已转移给系统，不再 GlobalFree
	return nil
}

// readClipboard 读取剪贴板文本（用于发布时合并多条链接）。
func readClipboard() string {
	ok, _, _ := pOpenClipboard.Call(0)
	if ok == 0 {
		return ""
	}
	defer pCloseClipboard.Call()

	h, _, _ := pGetClipboardData.Call(CF_UNICODETEXT)
	if h == 0 {
		return ""
	}
	ptr, _, _ := pGlobalLock.Call(h)
	if ptr == 0 {
		return ""
	}
	defer pGlobalUnlock.Call(h)

	// 以 0 结尾的 UTF-16 串
	base := ptrFromUintptr(ptr)
	var buf []uint16
	for i := 0; i < 1<<20; i++ {
		v := *(*uint16)(unsafe.Add(base, uintptr(i*2)))
		if v == 0 {
			break
		}
		buf = append(buf, v)
	}
	return syscall.UTF16ToString(buf)
}

// ---------------------------------------------------------------- 打开外部程序

// openURL 用系统默认浏览器打开网址。
func openURL(url string) {
	verb := utf16Ptr("open")
	target := utf16Ptr(url)
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(target)), 0, 0, SW_SHOWNORMAL)
}

// openPath 用资源管理器打开文件夹或文件。
func openPath(path string) {
	verb := utf16Ptr("open")
	target := utf16Ptr(path)
	r, _, _ := pShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(target)), 0, 0, SW_SHOWNORMAL)
	if r <= 32 {
		// 回退：直接调用 explorer
		_ = exec.Command("explorer.exe", path).Start()
	}
}

// msgBox 弹出系统消息框（仅在托盘不可用等兜底场景使用）。
func msgBox(title, text string, flags uintptr) {
	t := utf16Ptr(title)
	m := utf16Ptr(text)
	pMessageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags|MB_SETFOREGROUND)
}

// ---------------------------------------------------------------- 文件夹选择

var (
	// 保持在包级，确保回调期间指针有效
	browseInitPath *uint16
)

func browseCallback(hwnd uintptr, uMsg uint32, lParam, lpData uintptr) uintptr {
	if uMsg == BFFM_INITIALIZED && browseInitPath != nil {
		pSendMessageW.Call(hwnd, BFFM_SETSELECTION, 1, uintptr(unsafe.Pointer(browseInitPath)))
	}
	return 0
}

// pickFolder 弹出系统文件夹选择对话框。
func pickFolder(title, initial string) (string, bool) {
	pCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED

	display := make([]uint16, 260)
	bi := browseInfoW{
		HwndOwner:      0,
		PszDisplayName: &display[0],
		LpszTitle:      utf16Ptr(title),
		UlFlags:        BIF_RETURNONLYFSDIRS | BIF_USENEWUI,
	}
	if initial != "" {
		browseInitPath = utf16Ptr(initial)
		bi.Lpfn = syscall.NewCallback(browseCallback)
	}

	pidl, _, _ := pSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return "", false
	}
	defer pCoTaskMemFree.Call(pidl)

	buf := make([]uint16, 1024)
	r, _, _ := pSHGetPathFromIDL.Call(pidl, uintptr(unsafe.Pointer(&buf[0])))
	if r == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

// ---------------------------------------------------------------- 进程与单实例

// acquireSingleInstance 尝试获取单实例锁；返回 false 表示已有实例在运行。
func acquireSingleInstance() (uintptr, bool) {
	name := utf16Ptr(mutexName)
	h, _, err := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return 0, true
	}
	if errno, ok := err.(syscall.Errno); ok && errno == ERROR_ALREADY_EXISTS {
		return h, false
	}
	// 部分情况下错误码通过 GetLastError 返回
	if le, _, _ := pGetLastError.Call(); le == ERROR_ALREADY_EXISTS {
		return h, false
	}
	return h, true
}

// findTrayWindow 查找已运行的托盘窗口句柄。
func findTrayWindow() uintptr {
	cls := utf16Ptr(trayWindowClass)
	h, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(cls)), 0)
	return h
}

// sendToTray 通过 WM_COPYDATA 向托盘进程发送一条指令。
func sendToTray(cmd, payload string) bool {
	hwnd := findTrayWindow()
	if hwnd == 0 {
		return false
	}
	text := syscall.StringToUTF16(cmd + "\x1f" + payload)
	cds := copyDataStruct{
		DwData: 0,
		CbData: uint32(len(text) * 2),
		LpData: unsafe.Pointer(&text[0]),
	}
	r, _, _ := pSendMessageW.Call(hwnd, WM_COPYDATA, 0, uintptr(unsafe.Pointer(&cds)))
	runtime.KeepAlive(text)
	return r != 0
}

// spawnDetached 以无窗口方式启动一个新的后台实例。
//
// 优先使用 CREATE_BREAKAWAY_FROM_JOB 让子进程脱离调用方的 Job 对象，
// 避免父进程（资源管理器 / 终端）退出时把常驻服务一并结束；
// 若当前 Job 不允许脱离，则回退为普通分离进程。
func spawnDetached() error {
	exe := exePath()
	if exe == "" {
		return errors.New("无法定位程序自身路径")
	}
	const (
		detachedProcess        = 0x00000008 // DETACHED_PROCESS
		createNoWindow         = 0x08000000 // CREATE_NO_WINDOW
		createBreakawayFromJob = 0x01000000 // CREATE_BREAKAWAY_FROM_JOB
	)
	base := uint32(detachedProcess | createNoWindow)

	run := func(flags uint32) error {
		cmd := exec.Command(exe)
		cmd.Dir = exeDir()
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags}
		return cmd.Start()
	}

	if err := run(base | createBreakawayFromJob); err == nil {
		logf("[信息] 已脱离父进程 Job 对象启动后台实例")
		return nil
	}
	return run(base)
}

// ---------------------------------------------------------------- 控制台（调试）

// attachConsole 在从终端启动时附加到父进程控制台，便于用 -console 查看实时日志。
func attachConsole() bool {
	const attachParentProcess = ^uintptr(1) // (DWORD)-1
	if r, _, _ := pAttachConsole.Call(attachParentProcess); r == 0 {
		return false
	}
	pSetConsoleOutCP.Call(65001)
	// GUI 子系统的进程启动时标准句柄无效，需要重新打开控制台设备
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout = f
		os.Stderr = f
	}
	if f, err := os.OpenFile("CONIN$", os.O_RDONLY, 0); err == nil {
		os.Stdin = f
	}
	pSetConsoleTitle.Call(uintptr(unsafe.Pointer(utf16Ptr(appName + " - 文件夹发布服务"))))
	return true
}
