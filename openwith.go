package main

import (
	"os"
	"strings"
)

// 「打开方式」接入：双击清单文件启动应用时，Windows 以文件路径为参数拉起进程，
// 由 ApplicationOpenedWithFile 事件拿到路径（首实例）；应用已运行时再次双击，
// 则由单实例的 OnSecondInstanceLaunch 回调转交（二实例随即退出）。
// 两条路径统一经 notifyOpenWithFile 处理：广播 open-with-file 事件并暂存路径——
// 启动场景前端尚未订阅事件，靠挂载后经 ConsumePendingOpenFile 拉取兜底；
// 前端拿到路径后复用整窗拖拽的路由规则（单个清单 → 批量校验页自动开始）。

// assocExts 注册到资源管理器的清单扩展名（.txt 不注册，避免劫持通用文本文件）。
var assocExts = []string{".sha256", ".sha1", ".sha512", ".md5", ".sum", ".sums"}

// openWithExts 识别为清单的扩展名（含 .txt：覆盖用户显式「打开方式」选择的场景，
// 与前端 api 层 MANIFEST_EXTS 保持一致）。
var openWithExts = append([]string{".txt"}, assocExts...)

// isOpenWithManifest 按扩展名判断是否清单文件。
func isOpenWithManifest(p string) bool {
	lower := strings.ToLower(p)
	for _, ext := range openWithExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// manifestArgFromArgs 从命令行参数（args[0] 为可执行文件自身路径）挑出第一个
// 实际存在的清单文件；没有则返回空字符串。
func manifestArgFromArgs(args []string) string {
	for _, a := range args[1:] {
		if !isOpenWithManifest(a) {
			continue
		}
		if st, err := os.Stat(a); err == nil && !st.IsDir() {
			return a
		}
	}
	return ""
}

// notifyOpenWithFile 统一入口：暂存路径（供前端启动拉取）并广播事件（供运行中消费）。
func (a *App) notifyOpenWithFile(path string) {
	a.openMu.Lock()
	a.pendingOpen = path
	a.openMu.Unlock()
	a.app.Event.Emit("open-with-file", path)
}

// ConsumePendingOpenFile 前端挂载后拉取并清空暂存的清单路径（无暂存时 Path 为空）。
func (a *App) ConsumePendingOpenFile() Result {
	a.openMu.Lock()
	p := a.pendingOpen
	a.pendingOpen = ""
	a.openMu.Unlock()
	return Result{OK: true, Path: p}
}

// ---------- 文件关联（设置里的显式开关；仅 Windows，HKCU 免管理员） ----------

// assocUnsupported 非 Windows 平台的结构化错误。
func assocUnsupported() Result {
	return errResult("unsupported_platform", "当前平台不支持文件关联注册", "File association is not supported on this platform", nil)
}

// GetFileAssocStatus 当前关联到本应用的扩展名数量（前端开关的勾选依据）。
func (a *App) GetFileAssocStatus() Result {
	if !fileAssocSupported() {
		return assocUnsupported()
	}
	return Result{OK: true, Count: fileAssocCount()}
}

// RegisterFileAssociations 显式注册全部清单扩展名关联（被其他程序占用的跳过），
// Count 为实际写入数量。
func (a *App) RegisterFileAssociations() Result {
	if !fileAssocSupported() {
		return assocUnsupported()
	}
	exe, err := os.Executable()
	if err != nil {
		return errResult("assoc", "解析程序路径失败", "Failed to resolve executable path", err)
	}
	return Result{OK: true, Count: registerFileAssoc(exe, a.app.Logger)}
}

// UnregisterFileAssociations 解除关联：只删除本应用自有 ProgID 条目，
// Count 为实际删除的扩展名数量。
func (a *App) UnregisterFileAssociations() Result {
	if !fileAssocSupported() {
		return assocUnsupported()
	}
	return Result{OK: true, Count: unregisterFileAssoc(a.app.Logger)}
}
