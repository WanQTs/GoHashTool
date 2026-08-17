//go:build windows

package main

import (
	"log/slog"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

// 文件关联的 Windows 注册表实现：全部读写限定在 HKCU\Software\Classes
// （免管理员、每用户生效，可在系统设置中解除）。读取走 HKCR 合并视图
// （能同时看到系统级 HKLM 注册，避免误判占用），写入/删除只走 HKCU。
//
// 注册/解除由前端设置开关显式触发（默认关）；启动时仅 healFileAssoc
// 自愈本应用自有但路径陈旧的条目，未注册的机器零写入。

func fileAssocSupported() bool { return true }

// fileAssocCount 当前关联到本应用 ProgID 的扩展名数（设置开关的勾选状态依据）。
func fileAssocCount() int {
	n := 0
	for _, ext := range assocExts {
		if readExtOwner(ext) == progIDFor(ext) {
			n++
		}
	}
	return n
}

// registerFileAssoc 注册（含自愈）全部清单扩展名，返回实际写入的数量。
// 被其他程序占用的扩展名跳过并记日志；单个失败不影响其余。
func registerFileAssoc(exePath string, logger *slog.Logger) int {
	want := assocCommandFor(exePath)
	n := 0
	for _, ext := range assocExts {
		progid := progIDFor(ext)
		if !planAssocWrite(readExtOwner(ext), progid, readAssocCommand(progid), want) {
			continue
		}
		if err := writeFileAssociation(ext, progid, exePath); err != nil {
			logger.Error("register file association failed", "ext", ext, "error", err)
			continue
		}
		n++
	}
	if n > 0 {
		notifyShellAssocChanged()
	}
	return n
}

// unregisterFileAssoc 解除关联：只删除关联到本应用 ProgID 的扩展名键与 ProgID 子树，
// 返回删除的扩展名数量。他人占用或未关联的条目一概不动。
func unregisterFileAssoc(logger *slog.Logger) int {
	n := 0
	for _, ext := range assocExts {
		progid := progIDFor(ext)
		if !planAssocRemove(readExtOwner(ext), progid) {
			continue
		}
		if err := deleteFileAssociation(ext, progid); err != nil {
			logger.Error("unregister file association failed", "ext", ext, "error", err)
			continue
		}
		n++
	}
	if n > 0 {
		notifyShellAssocChanged()
	}
	return n
}

// healFileAssoc 启动自愈：仅当扩展名明确关联到本应用（用户此前显式注册过）
// 而登记的打开命令已不是当前 exe 路径时，更新为当前路径；其余情况零写入。
func healFileAssoc(exePath string, logger *slog.Logger) {
	want := assocCommandFor(exePath)
	n := 0
	for _, ext := range assocExts {
		progid := progIDFor(ext)
		if !planAssocHeal(readExtOwner(ext), progid, readAssocCommand(progid), want) {
			continue
		}
		if err := writeFileAssociation(ext, progid, exePath); err != nil {
			logger.Error("heal file association failed", "ext", ext, "error", err)
			continue
		}
		n++
	}
	if n > 0 {
		notifyShellAssocChanged()
	}
}

// readExtOwner 读扩展名当前的关联目标（HKCR 合并视图；无关联或异常返回空）。
func readExtOwner(ext string) string {
	k, err := registry.OpenKey(registry.CLASSES_ROOT, ext, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue("")
	if err != nil {
		return ""
	}
	return v
}

// readAssocCommand 读 ProgID 登记的打开命令（HKCR 合并视图；无则返回空）。
func readAssocCommand(progid string) string {
	k, err := registry.OpenKey(registry.CLASSES_ROOT, progid+`\shell\open\command`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue("")
	if err != nil {
		return ""
	}
	return v
}

// writeFileAssociation 写入单个扩展名的 ProgID 关联：
// .ext → ProgID → 友好名 / 图标 / 打开命令。已存在则覆盖（注册与自愈共用）。
func writeFileAssociation(ext, progid, exePath string) error {
	if err := regSetString(`Software\Classes\`+ext, "", progid); err != nil {
		return err
	}
	if err := regSetString(`Software\Classes\`+progid, "", "GoHashTool Checksum Manifest"); err != nil {
		return err
	}
	if err := regSetString(`Software\Classes\`+progid+`\DefaultIcon`, "", `"`+exePath+`",0`); err != nil {
		return err
	}
	return regSetString(`Software\Classes\`+progid+`\shell\open\command`, "", assocCommandFor(exePath))
}

// deleteFileAssociation 删除扩展名键与 ProgID 子树（registry.DeleteKey 不递归，
// 须按子键先后的顺序删除）。调用方须已通过 planAssocRemove 确认归属。
func deleteFileAssociation(ext, progid string) error {
	base := `Software\Classes\` + progid
	for _, sub := range []string{
		base + `\shell\open\command`,
		base + `\shell\open`,
		base + `\shell`,
		base + `\DefaultIcon`,
		base,
		`Software\Classes\` + ext,
	} {
		if err := registry.DeleteKey(registry.CURRENT_USER, sub); err != nil {
			return err
		}
	}
	return nil
}

func regSetString(path, name, value string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.CREATE_SUB_KEY|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(name, value)
}

// notifyShellAssocChanged 通知外壳关联已变更，否则要重启资源管理器才生效。
// 失败无害（下次重启资源管理器自然生效），故忽略返回值。
func notifyShellAssocChanged() {
	const SHCNE_ASSOCCHANGED = 0x08000000
	const SHCNF_IDLIST = 0x0000
	proc := syscall.NewLazyDLL("shell32.dll").NewProc("SHChangeNotify")
	_, _, _ = proc.Call(SHCNE_ASSOCCHANGED, SHCNF_IDLIST, 0, 0)
}
