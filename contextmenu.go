package main

import (
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// 结果行原生右键菜单。前端在表格行上声明 CSS 变量（见 result-table.vue）：
//
//	--custom-contextmenu: result-row
//	--custom-contextmenu-data: encodeURIComponent(JSON.stringify({path, hashes}))
//
// Wails runtime 拦截 contextmenu 事件后按名字弹出此处注册的原生菜单；
// 菜单动作（复制哈希/复制路径/资源管理器显示）在 Go 侧闭环完成——
// 复制成功经 context:copied 事件、失败经 context:error 事件通知前端 toast
// （原生菜单没有返回值通道，反馈只能走事件）。
const rowContextMenuName = "result-row"

// ContextMenuLabels 菜单文案（前端按当前语言即时取值传入，切换语言后重建菜单）。
type ContextMenuLabels struct {
	CopyHash string `json:"copyHash"`
	CopyPath string `json:"copyPath"`
	Reveal   string `json:"reveal"`
}

// rowContextPayload 是 --custom-contextmenu-data 解码后的行数据。
type rowContextPayload struct {
	Path   string            `json:"path"`
	Hashes map[string]string `json:"hashes"`
}

// decodeRowContext 解码前端编码（encodeURIComponent(JSON)）的行数据。
// 注意前端用 encodeURIComponent（空格为 %20），对应 PathUnescape 而非 QueryUnescape。
func decodeRowContext(s string) (rowContextPayload, error) {
	var p rowContextPayload
	raw, err := url.PathUnescape(s)
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return p, err
	}
	return p, nil
}

// rowAlgoLabels 右键子菜单的算法展示名（与前端 ALGO_LABELS 一致）。
var rowAlgoLabels = map[string]string{
	"md5": "MD5", "sha1": "SHA-1", "sha256": "SHA-256", "sha512": "SHA-512", "crc32": "CRC32",
}

// rowMenu 当前已注册的结果行菜单（重建时销毁旧菜单释放原生资源）。
var rowMenu struct {
	sync.Mutex
	menu *application.ContextMenu
}

// SetupResultContextMenu （重）建结果行右键菜单：应用启动与每次切换语言时由前端调用。
func (a *App) SetupResultContextMenu(labels ContextMenuLabels) Result {
	if a.app == nil {
		return errResult("no_app", "应用未就绪", "Application not ready", nil)
	}
	if labels.CopyHash == "" || labels.CopyPath == "" || labels.Reveal == "" {
		return errResult("bad_labels", "菜单文案不能为空", "Menu labels must not be empty", nil)
	}
	menu := application.NewContextMenu(rowContextMenuName)
	sub := menu.AddSubmenu(labels.CopyHash)
	for _, algo := range []string{"md5", "sha1", "sha256", "sha512", "crc32"} {
		algo := algo
		sub.Add(rowAlgoLabels[algo]).OnClick(func(ctx *application.Context) {
			a.copyRowHash(ctx, algo)
		})
	}
	menu.Add(labels.CopyPath).OnClick(a.copyRowPath)
	menu.AddSeparator()
	menu.Add(labels.Reveal).OnClick(a.revealRow)
	menu.Update()

	a.app.ContextMenu.Add(rowContextMenuName, menu) // 同名注册直接覆盖
	rowMenu.Lock()
	old := rowMenu.menu
	rowMenu.menu = menu
	rowMenu.Unlock()
	if old != nil {
		old.Destroy() // 映射已被新菜单覆盖后，销毁旧实例释放原生资源
	}
	return Result{OK: true}
}

// copyRowHash 复制该行指定算法的哈希值；该行没有此算法结果（失败行等）时动作无意义，直接返回。
func (a *App) copyRowHash(ctx *application.Context, algo string) {
	p, err := decodeRowContext(ctx.ContextMenuData())
	if err != nil {
		a.emitContextError("ctx_decode", "读取行数据失败", "Failed to read row data", err)
		return
	}
	if h := p.Hashes[algo]; h != "" {
		a.copyAndNotify(h)
	}
}

func (a *App) copyRowPath(ctx *application.Context) {
	p, err := decodeRowContext(ctx.ContextMenuData())
	if err != nil {
		a.emitContextError("ctx_decode", "读取行数据失败", "Failed to read row data", err)
		return
	}
	if p.Path != "" {
		a.copyAndNotify(p.Path)
	}
}

func (a *App) copyAndNotify(text string) {
	if a.app.Clipboard.SetText(text) {
		a.app.Event.Emit("context:copied")
	} else {
		a.emitContextError("clipboard", "复制失败", "Copy failed", nil)
	}
}

// revealRow 在资源管理器中定位该文件；文件已不存在（缺失行）时退而打开父目录。
func (a *App) revealRow(ctx *application.Context) {
	p, err := decodeRowContext(ctx.ContextMenuData())
	if err != nil {
		a.emitContextError("ctx_decode", "读取行数据失败", "Failed to read row data", err)
		return
	}
	if p.Path == "" {
		return
	}
	if err := revealInExplorer(p.Path); err != nil {
		a.emitContextError("reveal", "打开资源管理器失败", "Failed to open Explorer", err)
	}
}

// revealInExplorer explorer /select,"path" 定位文件；文件不存在时打开父目录。
// explorer 成功也可能返回非零退出码，故只 Start 不 Wait。
func revealInExplorer(path string) error {
	if _, err := os.Stat(path); err != nil {
		return exec.Command("explorer", filepath.Dir(path)).Start()
	}
	return exec.Command("explorer", `/select,"`+path+`"`).Start()
}

// emitContextError 菜单动作失败经 context:error 事件通知前端 toast（双语结构化错误契约）。
func (a *App) emitContextError(code, zh, en string, err error) {
	ae := &AppError{Code: code, Zh: zh, En: en}
	if err != nil {
		ae.Detail = err.Error()
	}
	a.app.Event.Emit("context:error", ae)
}
