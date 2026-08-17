package main

// 文件关联（清单扩展名 → 双击用本工具打开）的纯逻辑部分，与平台注册表 IO 分离，
// 供单元测试直接覆盖。平台实现见 fileassoc_windows.go / fileassoc_other.go。
//
// 设计原则（方案 B：显式开关，默认关）：
//   - 启动时绝不自动注册；仅「自愈」——用户显式注册过、且 exe 路径已变化时更新打开命令；
//   - 注册/解除只触碰本应用自有 ProgID（GoHashTool<ext>），被其他程序占用的扩展名跳过不劫持；
//   - 解除同样只删自己的 ProgID，他人关联一概不动。

// progIDFor 扩展名对应的本应用 ProgID（如 .sha256 → GoHashTool.sha256）。
func progIDFor(ext string) string { return "GoHashTool" + ext }

// assocCommandFor ProgID 登记的打开命令（资源管理器双击时执行）。
func assocCommandFor(exePath string) string { return `"` + exePath + `" "%1"` }

// planAssocWrite 单个扩展名的注册动作判定：
// owner 为该扩展名当前的关联目标（空 = 未关联），cmd 为我们 ProgID 当前登记的打开命令。
// 返回 true 表示应（重）写入：未关联 → 注册；关联到我们但命令陈旧 → 自愈。
// 返回 false：被其他程序占用（不劫持），或已是期望命令。
func planAssocWrite(owner, progid, cmd, wantCmd string) bool {
	if owner == "" {
		return true
	}
	if owner != progid {
		return false
	}
	return cmd != wantCmd
}

// planAssocHeal 启动自愈判定：仅处理「明确是我们注册的（owner 就是我们的 ProgID）
// 且路径已陈旧」的条目。未关联（从未注册或用户已手动解除）一律不写。
func planAssocHeal(owner, progid, cmd, wantCmd string) bool {
	return owner == progid && cmd != wantCmd
}

// planAssocRemove 解除判定：仅当扩展名关联到我们的 ProgID 时才删除；
// 他人占用或未关联时不动。
func planAssocRemove(owner, progid string) bool {
	return owner == progid
}
