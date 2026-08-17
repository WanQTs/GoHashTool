//go:build windows

package main

import (
	"log/slog"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

// registerFileAssociations 把清单扩展名关联注册到当前用户
// （HKCU\Software\Classes，免管理员，可在系统设置「默认应用」中随时解除）。
// 已被其他程序占用的扩展名跳过不劫持；单个扩展名失败只记日志，不影响启动。
func registerFileAssociations(exePath string, logger *slog.Logger) {
	changed := false
	for _, ext := range assocExts {
		if fileExtAssociated(ext) {
			continue
		}
		if err := writeFileAssociation(ext, exePath); err != nil {
			logger.Error("register file association failed", "ext", ext, "error", err)
			continue
		}
		changed = true
	}
	if changed {
		notifyShellAssocChanged()
	}
}

// fileExtAssociated 检查扩展名是否已有程序关联。
// 读 HKCR 合并视图（含系统级 HKLM 注册），写入则只写 HKCU。
func fileExtAssociated(ext string) bool {
	k, err := registry.OpenKey(registry.CLASSES_ROOT, ext, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue("")
	return err == nil && v != ""
}

// writeFileAssociation 写入单个扩展名的 ProgID 关联：
// .ext → ProgID → 友好名 / 图标 / 打开命令（"exe" "%1"）。
func writeFileAssociation(ext, exePath string) error {
	progid := "GoHashTool" + ext // 如 GoHashTool.sha256
	if err := regSetString(`Software\Classes\`+ext, "", progid); err != nil {
		return err
	}
	if err := regSetString(`Software\Classes\`+progid, "", "GoHashTool Checksum Manifest"); err != nil {
		return err
	}
	if err := regSetString(`Software\Classes\`+progid+`\DefaultIcon`, "", `"`+exePath+`",0`); err != nil {
		return err
	}
	return regSetString(`Software\Classes\`+progid+`\shell\open\command`, "", `"`+exePath+`" "%1"`)
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
