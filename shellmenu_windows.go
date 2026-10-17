//go:build windows

package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"
)

// 资源管理器右键菜单通过 HKCU 注册表注册（无需管理员权限）。
// 直接调用 advapi32 的注册表 API，不依赖外部 reg.exe：
// 既避免被杀软/安全策略拦截，也不会弹出多余的控制台窗口。

var (
	advapi32 = syscall.NewLazyDLL("advapi32.dll")

	pRegCreateKeyExW = advapi32.NewProc("RegCreateKeyExW")
	pRegSetValueExW  = advapi32.NewProc("RegSetValueExW")
	pRegDeleteTreeW  = advapi32.NewProc("RegDeleteTreeW")
	pRegOpenKeyExW   = advapi32.NewProc("RegOpenKeyExW")
	pRegQueryValueEx = advapi32.NewProc("RegQueryValueExW")
	pRegCloseKey     = advapi32.NewProc("RegCloseKey")

	pSHChangeNotify = shell32.NewProc("SHChangeNotify")
)

// 预定义注册表根键（64 位下需要符号扩展）
var (
	hkeyCurrentUser = uintptr(0x80000001) | ^uintptr(0xFFFFFFFF)
)

const (
	regSZ      = 1
	regDWORD   = 4
	keyWrite   = 0x20006 // KEY_WRITE
	keyRead    = 0x20019 // KEY_READ
	regNoError = 0
)

const (
	menuKeyName = "LocalDrivePublish"
	menuLabel   = "发布到 LocalDrive"

	// 右键 LocalDrive.exe 本身时出现的菜单项
	appMenuKeyName = "LocalDriveConfig"
	appMenuLabel   = "编辑配置 (LocalDrive)"
)

// shellMenuSubKeys 返回需要注册的子键路径（相对 HKCU）。
func shellMenuSubKeys() []string {
	return []string{
		`Software\Classes\*\shell\` + menuKeyName,
		`Software\Classes\Directory\shell\` + menuKeyName,
	}
}

// appMenuSubKey 返回「右键 LocalDrive.exe 本身」时出现的菜单项键。
//
// Software\Classes\Applications\<exe 文件名>\shell\<动词> 是 Windows 为
// 「针对某个具体程序」注册右键菜单的标准位置 —— 只对这个 exe 生效，
// 不影响其它程序，也不需要管理员权限。
func appMenuSubKey(exe string) string {
	return `Software\Classes\Applications\` + filepath.Base(exe) + `\shell\` + appMenuKeyName
}

// ---------------------------------------------------------------- 旧版本迁移

// legacyBrand 是改名前使用的产品名。
//
// 拆开拼接而不是写成完整字面量，是为了让当前代码里不再出现旧名称，
// 同时仍能定位旧版本留下的注册表项 —— 这只是一段一次性清理逻辑。
const legacyBrand = "Temp" + "Http"

// removeLegacyMenus 清理旧版本注册的右键菜单项。
//
// 产品改名后旧键不会被新版本覆盖；不清理的话，资源管理器右键菜单里会
// 同时出现新旧两条「发布」。
func removeLegacyMenus() {
	subs := []string{
		`Software\Classes\*\shell\` + legacyBrand + `Publish`,
		`Software\Classes\Directory\shell\` + legacyBrand + `Publish`,
		`Software\Classes\Applications\` + legacyBrand + `.exe\shell\` + legacyBrand + `Config`,
	}
	removed := 0
	for _, sub := range subs {
		if !regKeyExists(sub) {
			continue
		}
		if regDeleteTree(sub) == nil {
			removed++
		}
	}
	if removed > 0 {
		logf("[信息] 已清理旧版本遗留的 %d 个右键菜单项", removed)
	}
}

// ---------------------------------------------------------------- 底层封装

func regCreateKey(subKey string) (uintptr, error) {
	var hKey uintptr
	r, _, _ := pRegCreateKeyExW.Call(
		hkeyCurrentUser,
		uintptr(unsafe.Pointer(utf16Ptr(subKey))),
		0, 0, 0,
		keyWrite, 0,
		uintptr(unsafe.Pointer(&hKey)),
		0,
	)
	if r != regNoError {
		return 0, fmt.Errorf("创建注册表项失败（错误码 %d）: %s", r, subKey)
	}
	return hKey, nil
}

func regSetString(hKey uintptr, name, value string) error {
	data := syscall.StringToUTF16(value) // 已含结尾的 0
	var namePtr uintptr
	if name != "" {
		namePtr = uintptr(unsafe.Pointer(utf16Ptr(name)))
	}
	r, _, _ := pRegSetValueExW.Call(
		hKey, namePtr, 0, regSZ,
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)*2),
	)
	if r != regNoError {
		return fmt.Errorf("写入注册表值失败（错误码 %d）: %s", r, name)
	}
	return nil
}

func regSetDWORD(hKey uintptr, name string, value uint32) error {
	v := value
	r, _, _ := pRegSetValueExW.Call(
		hKey, uintptr(unsafe.Pointer(utf16Ptr(name))), 0, regDWORD,
		uintptr(unsafe.Pointer(&v)), 4,
	)
	if r != regNoError {
		return fmt.Errorf("写入注册表 DWORD 失败（错误码 %d）: %s", r, name)
	}
	return nil
}

// regKeyExists 判断子键是否存在且可读。
func regKeyExists(subKey string) bool {
	var hKey uintptr
	r, _, _ := pRegOpenKeyExW.Call(
		hkeyCurrentUser,
		uintptr(unsafe.Pointer(utf16Ptr(subKey))),
		0, keyRead,
		uintptr(unsafe.Pointer(&hKey)),
	)
	if r != regNoError || hKey == 0 {
		return false
	}
	pRegCloseKey.Call(hKey)
	return true
}

// regDeleteTree 递归删除子键；键不存在视为成功。
func regDeleteTree(subKey string) error {
	const errorFileNotFound = 2
	r, _, _ := pRegDeleteTreeW.Call(
		hkeyCurrentUser,
		uintptr(unsafe.Pointer(utf16Ptr(subKey))),
	)
	if r == regNoError || r == errorFileNotFound {
		return nil
	}
	return fmt.Errorf("删除注册表项失败（错误码 %d）: %s", r, subKey)
}

// ---------------------------------------------------------------- 对外接口

// iconFilePath 返回程序图标的 .ico 路径（不存在时生成）。
func iconFilePath() string {
	p := filepath.Join(appDir(), appName+".ico")
	if err := writeICO(p, []int{16, 32, 48}); err != nil {
		logf("[警告] 生成图标文件失败: %v", err)
	}
	return p
}

// installShellMenu 注册资源管理器右键菜单。
func installShellMenu(exe string) error {
	if exe == "" {
		return errors.New("无法定位程序自身路径")
	}
	// 先清掉旧版本留下的菜单项，避免出现两条同名菜单
	removeLegacyMenus()

	ico := iconFilePath()
	command := `"` + exe + `" -publish "%1"`

	var firstErr error
	for _, sub := range shellMenuSubKeys() {
		hKey, err := regCreateKey(sub)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		steps := []error{
			regSetString(hKey, "", menuLabel),
			regSetString(hKey, "Icon", ico+",0"),
			// Player 模式：多选时一次性把所有选中项交给同一条命令
			regSetString(hKey, "MultiSelectModel", "Player"),
			regSetDWORD(hKey, "MultiSelectLimit", 100),
		}
		pRegCloseKey.Call(hKey)

		cmdKey, err := regCreateKey(sub + `\command`)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		steps = append(steps, regSetString(cmdKey, "", command))
		pRegCloseKey.Call(cmdKey)

		for _, e := range steps {
			if e != nil && firstErr == nil {
				firstErr = e
			}
		}
	}
	if firstErr != nil {
		return firstErr
	}

	// 「右键 exe 本身 → 编辑配置」
	if err := installAppMenu(exe, ico); err != nil && firstErr == nil {
		firstErr = err
	}

	notifyShellChanged()
	logf("[信息] 已安装资源管理器右键菜单：%s", command)
	return firstErr
}

// installAppMenu 注册「右键 LocalDrive.exe 本身 → 编辑配置」。
func installAppMenu(exe, ico string) error {
	sub := appMenuSubKey(exe)
	hKey, err := regCreateKey(sub)
	if err != nil {
		return err
	}
	steps := []error{
		regSetString(hKey, "", appMenuLabel),
		regSetString(hKey, "Icon", ico+",0"),
	}
	pRegCloseKey.Call(hKey)

	cmdKey, err := regCreateKey(sub + `\command`)
	if err != nil {
		return err
	}
	steps = append(steps, regSetString(cmdKey, "", `"`+exe+`" -edit-config`))
	pRegCloseKey.Call(cmdKey)

	for _, e := range steps {
		if e != nil {
			return e
		}
	}
	logf("[信息] 已注册「%s」（在 %s 上点右键即可看到）", appMenuLabel, filepath.Base(exe))
	return nil
}

// uninstallShellMenu 移除资源管理器右键菜单。
func uninstallShellMenu() error {
	var firstErr error
	for _, sub := range shellMenuSubKeys() {
		if err := regDeleteTree(sub); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if exe := exePath(); exe != "" {
		if err := regDeleteTree(appMenuSubKey(exe)); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	notifyShellChanged()

	if firstErr != nil {
		logf("[错误] 移除右键菜单失败: %v", firstErr)
		return firstErr
	}
	logf("[信息] 已移除资源管理器右键菜单")
	return nil
}

// isShellMenuInstalled 检查右键菜单是否已注册。
func isShellMenuInstalled() bool {
	for _, sub := range shellMenuSubKeys() {
		if !regKeyExists(sub) {
			return false
		}
	}
	return true
}

// notifyShellChanged 通知资源管理器刷新右键菜单缓存。
func notifyShellChanged() {
	const (
		shcneAssocChanged = 0x08000000
		shcnfIDList       = 0x0000
	)
	pSHChangeNotify.Call(shcneAssocChanged, shcnfIDList, 0, 0)
}
