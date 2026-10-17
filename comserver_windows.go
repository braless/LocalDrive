//go:build windows

package main

// Windows 11 一级右键菜单支持。
//
// Win11 的「新式」右键菜单只接受由「有包标识的应用」（MSIX / 稀疏包）注册、
// 且实现 IExplorerCommand 接口的 COM 类。本文件用纯 Go（不依赖 cgo / C 编译器）
// 实现这个 COM 服务器：
//
//	LocalDrive.exe -comserver     ← 被 COM 以 -Embedding 方式拉起时进入本模式
//
// 它把 CLSID 注册为 CLSCTX_LOCAL_SERVER（进程外）类对象，供 shell 激活；
// shell 调用 GetTitle 拿到菜单名「发布」，调用 Invoke 时把选中项交给
// `LocalDrive.exe -publish` 完成发布。
//
// 所有 COM 接口都用「手搓 vtable」的方式实现：vtable 就是一组函数指针数组，
// 函数指针由 syscall.NewCallback 从普通 Go 函数生成。

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// ---------------------------------------------------------------- 常量

const (
	// 本 COM 类的 CLSID，必须与 AppxManifest.xml 中的 com:Class/@Id 完全一致。
	comClsidString = "{B7C4E1A9-3D82-4F5B-9E06-2A8C1D7F4B30}"

	// 菜单显示名（用户要求固定为「发布」）。
	comVerbTitle = "发布"

	coinitApartmentThreaded = 0x2 // COINIT_APARTMENTTHREADED
	clsidCtxLocalServer     = 0x4 // CLSCTX_LOCAL_SERVER
	regclsMultipleUse       = 0x1 // REGCLS_MULTIPLEUSE
	regclsSuspended         = 0x4 // REGCLS_SUSPENDED
	sigdnFileSysPath        = 0x80058000

	ecsEnabled  = 0 // ECS_ENABLED
	ecfDefault  = 0 // ECF_DEFAULT
	wmQuit      = 0x0012
	comIdleExit = 3 * time.Minute
)

// HRESULT 常量
const (
	hrSOK          = 0x00000000
	hrENotImpl     = 0x80004001
	hrENoInterface = 0x80004002
	hrEPointer     = 0x80004003
	hrEFail        = 0x80004005
	hrEOutOfMemory = 0x8007000E
)

// 接口 IID
var (
	iidIUnknown         = guidFromString("{00000000-0000-0000-C000-000000000046}")
	iidIClassFactory    = guidFromString("{00000001-0000-0000-C000-000000000046}")
	iidIExplorerCommand = guidFromString("{A08CE4D0-FA25-44AB-B57C-C7B1C323E0B9}")
	clsidLocalDrive     = guidFromString(comClsidString)
)

// ---------------------------------------------------------------- GUID

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// guidFromString 解析 "{XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX}" 形式的 GUID。
func guidFromString(s string) guid {
	var g guid
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	s = strings.ReplaceAll(s, "-", "")
	if len(s) != 32 {
		return g
	}
	hex := func(i int) uint64 {
		var v uint64
		for k := 0; k < 2; k++ {
			c := s[i+k]
			var d uint64
			switch {
			case c >= '0' && c <= '9':
				d = uint64(c - '0')
			case c >= 'a' && c <= 'f':
				d = uint64(c-'a') + 10
			case c >= 'A' && c <= 'F':
				d = uint64(c-'A') + 10
			}
			v = v<<4 | d
		}
		return v
	}
	g.Data1 = uint32(hex(0))<<24 | uint32(hex(2))<<16 | uint32(hex(4))<<8 | uint32(hex(6))
	g.Data2 = uint16(hex(8))<<8 | uint16(hex(10))
	g.Data3 = uint16(hex(12))<<8 | uint16(hex(14))
	for i := 0; i < 8; i++ {
		g.Data4[i] = byte(hex(16 + i*2))
	}
	return g
}

func (g guid) equal(o guid) bool { return g == o }

func (g guid) String() string {
	return fmt.Sprintf("{%08X-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X}",
		g.Data1, g.Data2, g.Data3,
		g.Data4[0], g.Data4[1], g.Data4[2], g.Data4[3],
		g.Data4[4], g.Data4[5], g.Data4[6], g.Data4[7])
}

// ---------------------------------------------------------------- 追加的 Win32 过程

var (
	pCoRegisterClassObject = ole32.NewProc("CoRegisterClassObject")
	pCoResumeClassObjects  = ole32.NewProc("CoResumeClassObjects")
	pCoRevokeClassObject   = ole32.NewProc("CoRevokeClassObject")
	pCoUninitialize        = ole32.NewProc("CoUninitialize")

	shlwapi             = syscall.NewLazyDLL("shlwapi.dll")
	pSHStrDupW          = shlwapi.NewProc("SHStrDupW")
	pGetCurrentThreadId = kernel32.NewProc("GetCurrentThreadId")
	pPostThreadMessageW = user32.NewProc("PostThreadMessageW")
)

// ---------------------------------------------------------------- 内存小工具

func hres(v uint32) uintptr { return uintptr(int64(int32(v))) }

func readPtr(p uintptr) uintptr {
	if p == 0 {
		return 0
	}
	return *(*uintptr)(ptrFromUintptr(p))
}

func writePtr(p, v uintptr) {
	if p == 0 {
		return
	}
	*(*uintptr)(ptrFromUintptr(p)) = v
}

func writeU32(p uintptr, v uint32) {
	if p == 0 {
		return
	}
	*(*uint32)(ptrFromUintptr(p)) = v
}

// vcall 通过 vtable 索引调用一个 COM 方法。
func vcall(vtbl uintptr, idx int, args ...uintptr) uintptr {
	if vtbl == 0 {
		return hres(hrEFail)
	}
	fn := readPtr(vtbl + uintptr(idx)*unsafe.Sizeof(uintptr(0)))
	if fn == 0 {
		return hres(hrENotImpl)
	}
	r, _, _ := syscall.SyscallN(fn, args...)
	return r
}

// utf16PtrToString 读取以 NUL 结尾的 UTF-16 字符串。
func utf16PtrToString(p uintptr) string {
	if p == 0 {
		return ""
	}
	buf := make([]uint16, 0, 64)
	for i := 0; i < 32768; i++ {
		c := *(*uint16)(ptrFromUintptr(p + uintptr(i)*2))
		if c == 0 {
			break
		}
		buf = append(buf, c)
	}
	return syscall.UTF16ToString(buf)
}

// strDupW 用 COM 任务分配器复制一个宽字符串（shell 会负责释放）。
func strDupW(s string) (uintptr, error) {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return 0, err
	}
	var out uintptr
	hr, _, _ := pSHStrDupW.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&out)))
	if int32(uint32(hr)) < 0 {
		return 0, fmt.Errorf("SHStrDupW 失败: 0x%08X", uint32(hr))
	}
	return out, nil
}

// ---------------------------------------------------------------- 活动对象登记
//
// 交给 shell 的对象必须防止被 Go 垃圾回收器提前回收，
// 所以用一个全局表持有 Go 指针，Release 到 0 时移除。

var (
	comMu     sync.Mutex
	comLive   = map[uintptr]any{}
	comActive int
	comLast   time.Time
)

func comKeep(p uintptr, v any) {
	comMu.Lock()
	comLive[p] = v
	comActive++
	comLast = time.Now()
	comMu.Unlock()
}

func comDrop(p uintptr) {
	comMu.Lock()
	if _, ok := comLive[p]; ok {
		delete(comLive, p)
		comActive--
	}
	comLast = time.Now()
	comMu.Unlock()
}

func comTouch() {
	comMu.Lock()
	comLast = time.Now()
	comMu.Unlock()
}

// ---------------------------------------------------------------- IUnknown / IClassFactory vtable

var (
	explCmdVtbl [11]uintptr // IUnknown(3) + IExplorerCommand(8)
	factoryVtbl [5]uintptr  // IUnknown(3) + IClassFactory(2)

	comInitOnce sync.Once
)

func initComVtables() {
	comInitOnce.Do(func() {
		explCmdVtbl[0] = syscall.NewCallback(ecQueryInterface)
		explCmdVtbl[1] = syscall.NewCallback(ecAddRef)
		explCmdVtbl[2] = syscall.NewCallback(ecRelease)
		explCmdVtbl[3] = syscall.NewCallback(ecGetTitle)
		explCmdVtbl[4] = syscall.NewCallback(ecGetIcon)
		explCmdVtbl[5] = syscall.NewCallback(ecGetToolTip)
		explCmdVtbl[6] = syscall.NewCallback(ecGetCanonicalName)
		explCmdVtbl[7] = syscall.NewCallback(ecGetState)
		explCmdVtbl[8] = syscall.NewCallback(ecInvoke)
		explCmdVtbl[9] = syscall.NewCallback(ecGetFlags)
		explCmdVtbl[10] = syscall.NewCallback(ecEnumSubCommands)

		factoryVtbl[0] = syscall.NewCallback(cfQueryInterface)
		factoryVtbl[1] = syscall.NewCallback(cfAddRef)
		factoryVtbl[2] = syscall.NewCallback(cfRelease)
		factoryVtbl[3] = syscall.NewCallback(cfCreateInstance)
		factoryVtbl[4] = syscall.NewCallback(cfLockServer)
	})
}

// comObject 是所有手搓 COM 对象的公共头部：首字段必须是指向 vtable 的指针。
type comObject struct {
	lpVtbl uintptr
	refs   int32
}

// ---------------------------------------------------------------- 命令对象

// IExplorerCommand 实现

func ecQueryInterface(this, riid, ppv uintptr) uintptr {
	if ppv == 0 {
		return hres(hrEPointer)
	}
	writePtr(ppv, 0)
	if riid == 0 {
		return hres(hrEPointer)
	}
	want := *(*guid)(ptrFromUintptr(riid))
	switch {
	case want.equal(iidIUnknown), want.equal(iidIExplorerCommand):
		writePtr(ppv, this)
		ecAddRef(this)
		return hres(hrSOK)
	}
	return hres(hrENoInterface)
}

func ecAddRef(this uintptr) uintptr {
	comTouch()
	if this == 0 {
		return 0
	}
	o := (*comObject)(ptrFromUintptr(this))
	return uintptr(atomicAdd(&o.refs, 1))
}

func ecRelease(this uintptr) uintptr {
	comTouch()
	if this == 0 {
		return 0
	}
	o := (*comObject)(ptrFromUintptr(this))
	n := atomicAdd(&o.refs, -1)
	if n <= 0 {
		comDrop(this)
	}
	return uintptr(n)
}

func ecGetTitle(this, arr, ppszName uintptr) uintptr {
	comTouch()
	if ppszName == 0 {
		return hres(hrEPointer)
	}
	p, err := strDupW(comVerbTitle)
	if err != nil {
		logf("[错误] COM GetTitle 失败: %v", err)
		return hres(hrEOutOfMemory)
	}
	writePtr(ppszName, p)
	return hres(hrSOK)
}

func ecGetIcon(this, arr, ppszIcon uintptr) uintptr {
	comTouch()
	if ppszIcon == 0 {
		return hres(hrEPointer)
	}
	ico := iconFilePath()
	if !fileExists(ico) {
		return hres(hrENotImpl)
	}
	p, err := strDupW(ico + ",0")
	if err != nil {
		return hres(hrEOutOfMemory)
	}
	writePtr(ppszIcon, p)
	return hres(hrSOK)
}

func ecGetToolTip(this, arr, ppszInfotip uintptr) uintptr {
	comTouch()
	if ppszInfotip != 0 {
		writePtr(ppszInfotip, 0)
	}
	return hres(hrENotImpl)
}

func ecGetCanonicalName(this, pguid uintptr) uintptr {
	comTouch()
	if pguid == 0 {
		return hres(hrEPointer)
	}
	for i := uintptr(0); i < 16; i++ {
		*(*byte)(ptrFromUintptr(pguid + i)) = 0
	}
	return hres(hrSOK)
}

func ecGetState(this, arr, fOkToBeSlow, pCmdState uintptr) uintptr {
	comTouch()
	if pCmdState == 0 {
		return hres(hrEPointer)
	}
	writeU32(pCmdState, ecsEnabled)
	return hres(hrSOK)
}

func ecInvoke(this, arr, pbc uintptr) uintptr {
	comTouch()
	paths := shellItemPaths(arr)
	if len(paths) == 0 {
		logf("[警告] COM Invoke 未取到任何选中项")
		return hres(hrSOK)
	}
	logf("[信息] COM Invoke 收到 %d 个选中项", len(paths))
	if err := spawnPublish(paths); err != nil {
		logf("[错误] COM Invoke 启动发布失败: %v", err)
	}
	return hres(hrSOK)
}

func ecGetFlags(this, pFlags uintptr) uintptr {
	comTouch()
	if pFlags == 0 {
		return hres(hrEPointer)
	}
	writeU32(pFlags, ecfDefault)
	return hres(hrSOK)
}

func ecEnumSubCommands(this, ppEnum uintptr) uintptr {
	comTouch()
	if ppEnum != 0 {
		writePtr(ppEnum, 0)
	}
	return hres(hrENotImpl)
}

// ---------------------------------------------------------------- 类工厂

func cfQueryInterface(this, riid, ppv uintptr) uintptr {
	if ppv == 0 || riid == 0 {
		return hres(hrEPointer)
	}
	writePtr(ppv, 0)
	want := *(*guid)(ptrFromUintptr(riid))
	switch {
	case want.equal(iidIUnknown), want.equal(iidIClassFactory):
		writePtr(ppv, this)
		cfAddRef(this)
		return hres(hrSOK)
	}
	return hres(hrENoInterface)
}

func cfAddRef(this uintptr) uintptr {
	if this == 0 {
		return 0
	}
	o := (*comObject)(ptrFromUintptr(this))
	return uintptr(atomicAdd(&o.refs, 1))
}

func cfRelease(this uintptr) uintptr {
	if this == 0 {
		return 0
	}
	o := (*comObject)(ptrFromUintptr(this))
	return uintptr(atomicAdd(&o.refs, -1))
}

func cfCreateInstance(this, outer, riid, ppv uintptr) uintptr {
	if ppv == 0 {
		return hres(hrEPointer)
	}
	writePtr(ppv, 0)
	if outer != 0 {
		return hres(0x80040110) // CLASS_E_NOAGGREGATION
	}
	initComVtables()
	o := &comObject{lpVtbl: uintptr(unsafe.Pointer(&explCmdVtbl[0])), refs: 1}
	objPtr := uintptr(unsafe.Pointer(o))
	comKeep(objPtr, o)
	return ecQueryInterface(objPtr, riid, ppv)
}

func cfLockServer(this, fLock uintptr) uintptr { return hres(hrSOK) }

// atomicAdd 对引用计数做原子加减。
func atomicAdd(p *int32, delta int32) int32 {
	return atomic.AddInt32(p, delta)
}

// ---------------------------------------------------------------- IShellItemArray 取值

func shellItemPaths(arr uintptr) []string {
	if arr == 0 {
		return nil
	}
	vtbl := readPtr(arr)
	if vtbl == 0 {
		return nil
	}
	var count uint32
	if hr := vcall(vtbl, 7, arr, uintptr(unsafe.Pointer(&count))); int32(uint32(hr)) < 0 {
		return nil
	}
	if count > 4096 {
		count = 4096
	}
	out := make([]string, 0, count)
	for i := uint32(0); i < count; i++ {
		var item uintptr
		if hr := vcall(vtbl, 8, arr, uintptr(i), uintptr(unsafe.Pointer(&item))); int32(uint32(hr)) < 0 || item == 0 {
			continue
		}
		ivtbl := readPtr(item)
		var pwstr uintptr
		hr := vcall(ivtbl, 5, item, uintptr(sigdnFileSysPath), uintptr(unsafe.Pointer(&pwstr)))
		if int32(uint32(hr)) >= 0 && pwstr != 0 {
			if s := utf16String(pwstr); s != "" {
				out = append(out, s)
			}
			pCoTaskMemFree.Call(pwstr)
		}
		vcall(ivtbl, 2, item) // Release
	}
	return out
}

func utf16String(p uintptr) string {
	s := utf16PtrToString(p)
	return strings.TrimRight(s, "\x00")
}

// ---------------------------------------------------------------- 启动发布子进程

// spawnPublish 以分离方式启动 `LocalDrive.exe -publish <路径...>`。
//
// shell 在 UI 线程上调用 Invoke，必须尽快返回，所以发布动作交给子进程完成。
func spawnPublish(paths []string) error {
	exe := exePath()
	if exe == "" {
		return errors.New("无法定位程序自身路径")
	}
	const (
		detachedProcess        = 0x00000008
		createNoWindow         = 0x08000000
		createBreakawayFromJob = 0x01000000
	)
	base := uint32(detachedProcess | createNoWindow)
	args := append([]string{"-publish"}, paths...)

	run := func(flags uint32) error {
		cmd := exec.Command(exe, args...)
		cmd.Dir = exeDir()
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags}
		return cmd.Start()
	}
	if err := run(base | createBreakawayFromJob); err == nil {
		return nil
	}
	return run(base)
}

// ---------------------------------------------------------------- 服务器主体

// runComServer 以进程外 COM 服务器身份运行，供 shell 激活 IExplorerCommand。
// 返回进程退出码。
func runComServer() int {
	runtime.LockOSThread()
	initComVtables()

	hr, _, _ := pCoInitializeEx.Call(0, coinitApartmentThreaded)
	if int32(uint32(hr)) < 0 {
		logf("[错误] CoInitializeEx 失败: 0x%08X", uint32(hr))
		return 1
	}
	defer pCoUninitialize.Call()

	factory := &comObject{lpVtbl: uintptr(unsafe.Pointer(&factoryVtbl[0])), refs: 1}
	factoryPtr := uintptr(unsafe.Pointer(factory))
	comKeep(factoryPtr, factory)

	var cookie uint32
	r, _, err := pCoRegisterClassObject.Call(
		uintptr(unsafe.Pointer(&clsidLocalDrive)),
		factoryPtr,
		clsidCtxLocalServer,
		regclsMultipleUse|regclsSuspended,
		uintptr(unsafe.Pointer(&cookie)),
	)
	if int32(uint32(r)) < 0 {
		logf("[错误] CoRegisterClassObject 失败: 0x%08X (%v)", uint32(r), err)
		return 2
	}
	if r, _, _ := pCoResumeClassObjects.Call(); int32(uint32(r)) < 0 {
		logf("[错误] CoResumeClassObjects 失败: 0x%08X", uint32(r))
		return 3
	}
	logf("[信息] COM 服务器已就绪，CLSID=%s，菜单名「%s」", comClsidString, comVerbTitle)

	// 空闲退出：shell 用完对象后如果长时间没有新调用，就自行结束，避免常驻。
	stop := make(chan struct{})
	tid, _, _ := pGetCurrentThreadId.Call()
	go func() {
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				comMu.Lock()
				idle := time.Since(comLast)
				active := comActive
				comMu.Unlock()
				if active <= 0 && idle > comIdleExit {
					logf("[信息] COM 服务器空闲 %v，退出", idle.Truncate(time.Second))
					pPostThreadMessageW.Call(tid, wmQuit, 0, 0)
					return
				}
			}
		}
	}()

	runComMessageLoop()
	close(stop)

	pCoRevokeClassObject.Call(uintptr(cookie))
	comMu.Lock()
	delete(comLive, factoryPtr)
	comActive--
	comMu.Unlock()
	logf("[信息] COM 服务器已退出")
	return 0
}

// runComMessageLoop 运行标准消息泵（STA 需要它来投递跨套间调用）。
func runComMessageLoop() {
	// MSG 在 x64 上为 48~56 字节，这里留足余量。
	var msg [64]byte
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
		switch int32(r) {
		case 0: // WM_QUIT
			return
		case -1: // 错误
			return
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg[0])))
	}
}

// isComServerInvocation 判断当前进程是否由 COM 以服务器身份拉起。
//
// COM 会用 -Embedding（可能还带 -Automation / -Embedding 组合）启动本地服务器。
func isComServerInvocation(args []string) bool {
	for _, a := range args {
		if strings.EqualFold(a, "-Embedding") || strings.EqualFold(a, "/Embedding") {
			return true
		}
	}
	return false
}
