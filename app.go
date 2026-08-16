package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"gohash/internal/checksum"
	"gohash/internal/hashcore"
)

// AppError 结构化错误：错误码 + 中英双语信息，前端按当前语言展示，禁止 panic 与静默吞错。
type AppError struct {
	Code   string `json:"code"`
	Zh     string `json:"zh"`
	En     string `json:"en"`
	Detail string `json:"detail,omitempty"`
}

// Result 绑定方法统一返回结构（对话框选择、任务启动、导出等）。
type Result struct {
	OK         bool      `json:"ok"`
	Error      *AppError `json:"error,omitempty"`
	Paths      []string  `json:"paths,omitempty"`  // 多选路径
	Path       string    `json:"path,omitempty"`   // 单选路径
	TaskID     string    `json:"taskId,omitempty"` // 任务 ID
	Total      int       `json:"total"`            // 条目总数
	TotalBytes int64     `json:"totalBytes"`       // 总字节数
	Algo       string    `json:"algo,omitempty"`   // 批量校验识别出的算法
}

// Item 结果行（哈希计算与批量校验共用；校验场景带 Expected/Actual/Verdict）。
type Item struct {
	Path       string            `json:"path"`
	Name       string            `json:"name"`
	Size       int64             `json:"size"`
	Hashes     map[string]string `json:"hashes"`
	DurationMs int64             `json:"durationMs"`
	Status     string            `json:"status"` // ok / occupied / no_permission / not_found / error / canceled
	ErrCode    string            `json:"errCode,omitempty"`
	Expected   string            `json:"expected,omitempty"`
	Actual     string            `json:"actual,omitempty"`
	Verdict    string            `json:"verdict,omitempty"` // pass / fail / missing
}

// ProgressEvent 进度事件负载（hash:progress，200ms 节流）。
type ProgressEvent struct {
	TaskID      string  `json:"taskId"`
	Total       int     `json:"total"`
	Done        int     `json:"done"`
	BytesDone   int64   `json:"bytesDone"`
	BytesTotal  int64   `json:"totalBytes"`
	CurrentFile string  `json:"currentFile"`
	SpeedMBps   float64 `json:"speedMBps"`
	ElapsedMs   int64   `json:"elapsedMs"`
}

// ItemsEvent 结果行批量推送负载（hash:items，单次不超过 500 条）。
type ItemsEvent struct {
	TaskID string `json:"taskId"`
	Items  []Item `json:"items"`
}

// Summary 任务完成汇总（hash:done）。
type Summary struct {
	TaskID     string `json:"taskId"`
	Total      int    `json:"total"`
	OK         int    `json:"ok"`
	Errors     int    `json:"errors"`
	Pass       int    `json:"pass"`
	Fail       int    `json:"fail"`
	Missing    int    `json:"missing"`
	Canceled   bool   `json:"canceled"`
	ElapsedMs  int64  `json:"elapsedMs"`
	BytesDone  int64  `json:"bytesDone"`
	BytesTotal int64  `json:"totalBytes"`
	Fatal      string `json:"fatal,omitempty"` // 任务 goroutine panic 时的错误信息（兜底防护，正常为空）
}

// taskState 运行中/已完成任务的状态；items 保留供导出复用。
// done 标记任务已结束（newTask 据此淘汰最旧的已完成任务，防止内存无限增长）。
type taskState struct {
	cancel context.CancelFunc
	done   bool
	seq    int64 // 任务序号（单调递增），淘汰时按序找最旧
	mu     sync.Mutex
	items  []Item
	algos  []string
	verify bool
}

// App Wails 后端绑定结构。
type App struct {
	ctx   context.Context
	mu    sync.Mutex
	tasks map[string]*taskState
	seq   atomic.Int64
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{tasks: map[string]*taskState{}}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// ---------- 文件对话框 ----------

// PickFiles 多选文件对话框。
func (a *App) PickFiles() Result {
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select files",
	})
	if err != nil {
		return errResult("dialog", "打开文件对话框失败", "Failed to open file dialog", err)
	}
	return Result{OK: true, Paths: paths}
}

// PickFolder 选择文件夹（递归计算）。
func (a *App) PickFolder() Result {
	p, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select folder",
	})
	if err != nil {
		return errResult("dialog", "打开目录对话框失败", "Failed to open directory dialog", err)
	}
	return Result{OK: true, Path: p}
}

// PickManifestFile 选择哈希清单文件。
func (a *App) PickManifestFile() Result {
	p, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select manifest",
		Filters: []runtime.FileFilter{
			{DisplayName: "Checksum files (*.sha256;*.sha1;*.sha512;*.md5;*.txt)", Pattern: "*.sha256;*.sha1;*.sha512;*.md5;*.txt;*.sum;*.sums"},
			{DisplayName: "All files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return errResult("dialog", "打开清单对话框失败", "Failed to open manifest dialog", err)
	}
	return Result{OK: true, Path: p}
}

// PickSavePath 导出保存对话框。
func (a *App) PickSavePath(defaultName, filterName, pattern string) Result {
	p, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: defaultName,
		Title:           "Export",
		Filters:         []runtime.FileFilter{{DisplayName: filterName, Pattern: pattern}},
	})
	if err != nil {
		return errResult("dialog", "打开保存对话框失败", "Failed to open save dialog", err)
	}
	return Result{OK: true, Path: p}
}

// CopyText 复制文本到剪贴板。
func (a *App) CopyText(text string) Result {
	if err := runtime.ClipboardSetText(a.ctx, text); err != nil {
		return errResult("clipboard", "复制失败", "Copy failed", err)
	}
	return Result{OK: true}
}

// ---------- 任务 ----------

// StartHashTask 启动哈希计算任务（异步），立即返回 taskId 与总量，
// 进度/结果通过 hash:progress / hash:items / hash:done 事件推送。
func (a *App) StartHashTask(paths []string, algos []string) Result {
	parsed, r := parseAlgos(algos)
	if !r.OK {
		return r
	}
	items, skippedDirs := hashcore.ExpandPathsDetailed(paths)
	// 不可读子目录不作为静默跳过：生成 no_permission 结果行前置展示，
	// 让用户明确知道这些子树未计入（行数计入总数与失败统计）。
	preItems := make([]Item, 0, len(skippedDirs))
	for _, d := range skippedDirs {
		preItems = append(preItems, Item{
			Path: d, Name: filepath.Base(d),
			Status:  string(hashcore.StatusNoPermission),
			ErrCode: string(hashcore.StatusNoPermission),
		})
	}
	if len(items) == 0 && len(preItems) == 0 {
		return errResult("no_files", "没有可计算的文件", "No files to hash", nil)
	}
	// ctx/cancel 必须在任务入表前同步创建并随 newTask 一起登记：
	// 否则 Start 返回到 runTask 写入 st.cancel 之间存在窗口，
	// 立刻点取消会得到「任务不存在或已结束」。
	ctx, cancel := context.WithCancel(a.ctx)
	taskID, st := a.newTask(algos, false, cancel)
	total := len(items) + len(preItems)
	go a.runTask(ctx, taskID, st, items, parsed, nil, preItems, total, hashcore.TotalSize(items))
	return Result{OK: true, TaskID: taskID, Total: total, TotalBytes: hashcore.TotalSize(items)}
}

// StartVerifyTask 启动批量校验任务：解析清单 → 识别算法 → 以基准目录解析
// 相对路径 → 批量计算并逐一对比。baseDir 为空时取清单所在目录。
func (a *App) StartVerifyTask(manifestPath, baseDir string) Result {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return errResult("read_manifest", "读取清单文件失败", "Failed to read manifest file", err)
	}
	entries, err := checksum.ParseManifest(data)
	if err != nil {
		return manifestError(err)
	}
	algo, err := checksum.DetectAlgorithm(manifestPath, entries)
	if err != nil {
		return manifestError(err)
	}
	if baseDir == "" {
		baseDir = filepath.Dir(manifestPath)
	}

	// 解析条目路径并探测缺失文件；缺失项立即出结论，存在项交给引擎计算。
	// ResolveTargets 按解析后路径去重：同一路径重复出现仅保留首条，
	// 避免同一文件被重复计算、期望值互相覆盖。
	toHash, missing, expected := checksum.ResolveTargets(entries, baseDir)
	preItems := make([]Item, 0, len(missing))
	for _, e := range missing {
		preItems = append(preItems, Item{
			Path: e.Path, Name: filepath.Base(e.Path), Size: 0,
			Status: string(hashcore.StatusNotFound), Expected: e.Hash, Verdict: "missing",
		})
	}

	// 同 StartHashTask：取消句柄随任务入表同步登记，不留取消窗口。
	ctx, cancel := context.WithCancel(a.ctx)
	taskID, st := a.newTask([]string{string(algo)}, true, cancel)
	total := len(toHash) + len(missing)
	go a.runTask(ctx, taskID, st, toHash, []hashcore.Algorithm{algo}, expected, preItems, total, hashcore.TotalSize(toHash))
	return Result{OK: true, TaskID: taskID, Total: total, TotalBytes: hashcore.TotalSize(toHash), Algo: string(algo)}
}

// CancelTask 取消任务（1 秒内生效）。
func (a *App) CancelTask(taskID string) Result {
	a.mu.Lock()
	st, ok := a.tasks[taskID]
	a.mu.Unlock()
	if !ok || st.cancel == nil {
		return errResult("task_not_found", "任务不存在或已结束", "Task not found or already finished", nil)
	}
	st.cancel()
	return Result{OK: true}
}

// ExportCSV 导出任务结果为 CSV（带 UTF-8 BOM）。onlyFailed 时仅导出
// 校验不一致/缺失项（批量校验的一键导出不一致项）。
func (a *App) ExportCSV(taskID, path string, onlyFailed bool) Result {
	st, r := a.getTask(taskID)
	if !r.OK {
		return r
	}
	st.mu.Lock()
	src := make([]Item, len(st.items))
	copy(src, st.items)
	st.mu.Unlock()

	var items []checksum.ExportItem
	for _, it := range src {
		if onlyFailed && it.Verdict != "fail" && it.Verdict != "missing" && it.Verdict != "error" {
			continue
		}
		items = append(items, toExportItem(it))
	}
	if len(items) == 0 {
		return errResult("no_data", "没有可导出的数据", "No data to export", nil)
	}
	if err := writeExport(path, func(w io.Writer) error {
		return checksum.WriteCSV(w, items, st.algos, st.verify)
	}); err != nil {
		return errResult("export", "导出 CSV 失败", "Failed to export CSV", err)
	}
	return Result{OK: true, Path: path}
}

// ExportSUM 导出任务结果为标准 SUM 格式清单（可被批量校验重新导入，闭环）。
func (a *App) ExportSUM(taskID, path, algo string) Result {
	st, r := a.getTask(taskID)
	if !r.OK {
		return r
	}
	parsed, err := hashcore.ParseAlgorithm(algo)
	if err != nil {
		return errResult("bad_algo", "未知算法", "Unknown algorithm", err)
	}
	// CRC32 的 8 位摘要不在清单解析的识别范围内（仅 32/40/64/128），
	// 导出后会无法重新导入校验，破坏闭环——直接拒绝并说明原因。
	if parsed == hashcore.CRC32 {
		return errResult("algo_not_exportable",
			"CRC32 不支持导出 SUM 清单（导出后无法重新导入校验）",
			"CRC32 cannot be exported as a SUM manifest (re-import verification is not supported)", nil)
	}
	st.mu.Lock()
	src := make([]Item, len(st.items))
	copy(src, st.items)
	st.mu.Unlock()
	// WriteSUM 会跳过失败行与缺失该算法哈希的行；预先统计实际可写行数，
	// 一行都写不出时返回错误而不是留下一个空清单文件。
	if exportableSUMCount(src, string(parsed)) == 0 {
		return errResult("no_data", "所选算法没有可导出的哈希结果", "No hash results available for the selected algorithm", nil)
	}
	var items []checksum.ExportItem
	for _, it := range src {
		items = append(items, toExportItem(it))
	}
	if err := writeExport(path, func(w io.Writer) error {
		return checksum.WriteSUM(w, items, parsed)
	}); err != nil {
		return errResult("export", "导出 SUM 失败", "Failed to export SUM", err)
	}
	return Result{OK: true, Path: path}
}

// writeExport 原子导出：先写 `path + ".tmp"` 临时文件，写出并关闭成功后再
// rename 覆盖目标；任何一步失败都删除临时文件，避免留下半截导出文件。
func writeExport(path string, write func(io.Writer) error) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if err := write(f); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil { // Close 时才落盘的写错误不能吞
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}

// ---------- 内部实现 ----------

// maxFinishedTasks 已完成任务结果的保留上限（供导出复用）；
// 超出后淘汰最旧者，防止长跑会话中结果内存无限增长。运行中任务不参与淘汰。
const maxFinishedTasks = 4

func (a *App) newTask(algos []string, verify bool, cancel context.CancelFunc) (string, *taskState) {
	n := a.seq.Add(1)
	id := fmt.Sprintf("t%d", n)
	st := &taskState{algos: algos, verify: verify, seq: n, cancel: cancel}
	a.mu.Lock()
	a.tasks[id] = st
	a.evictDoneTasksLocked()
	a.mu.Unlock()
	return id, st
}

// evictDoneTasksLocked 仅保留最近 maxFinishedTasks 个已完成任务。调用方须持有 a.mu。
func (a *App) evictDoneTasksLocked() {
	type doneTask struct {
		id  string
		seq int64
	}
	var finished []doneTask
	for tid, s := range a.tasks {
		if s.done {
			finished = append(finished, doneTask{id: tid, seq: s.seq})
		}
	}
	if len(finished) <= maxFinishedTasks {
		return
	}
	sort.Slice(finished, func(i, j int) bool { return finished[i].seq < finished[j].seq })
	for _, d := range finished[:len(finished)-maxFinishedTasks] {
		delete(a.tasks, d.id)
	}
}

func (a *App) getTask(taskID string) (*taskState, Result) {
	a.mu.Lock()
	st, ok := a.tasks[taskID]
	a.mu.Unlock()
	if !ok {
		return nil, errResult("task_not_found", "任务结果已不存在", "Task result no longer exists", nil)
	}
	return st, Result{OK: true}
}

// runTask 引擎驱动：进度 200ms 节流上报，结果行批量推送（单次 ≤500 条）。
// ctx 由启动方同步创建（取消句柄随任务入表，见 StartHashTask）。
// 收尾统一在 defer 完成（recover 兜底 → 停 ticker → 冲刷结果行 → 发 hash:done），
// panic 时也会发出带 fatal 的完成事件，绝不让进程崩溃或前端卡在运行态。
func (a *App) runTask(ctx context.Context, taskID string, st *taskState, items []hashcore.FileItem,
	algos []hashcore.Algorithm, expected map[string]string, preItems []Item,
	grandTotal int, totalBytes int64) {

	start := time.Now()
	var bytesDone atomic.Int64
	doneCount := atomic.Int64{}
	doneCount.Store(int64(len(preItems)))
	var current atomic.Value
	current.Store("")

	var pmu sync.Mutex
	pending := append([]Item(nil), preItems...)
	if len(preItems) > 0 {
		st.mu.Lock()
		st.items = append(st.items, preItems...)
		st.mu.Unlock()
	}

	// flush 全程持锁（置换 + 发射一体）：onItem 与 ticker 可能并发触发 flush，
	// 若置换后就放锁，两个 flush 的 EventsEmit 先后无序，结果行会乱序到达。
	// EventsEmit 只是内存队列推送，持锁时间可忽略。
	flush := func() {
		pmu.Lock()
		defer pmu.Unlock()
		batch := pending
		pending = nil
		for len(batch) > 0 { // 单次推送不超过 500 条
			n := len(batch)
			if n > 500 {
				n = 500
			}
			runtime.EventsEmit(a.ctx, "hash:items", ItemsEvent{TaskID: taskID, Items: batch[:n]})
			batch = batch[n:]
		}
	}

	// 进度上报节流：每 200ms 一次，避免事件洪峰卡界面。
	stopTicker := make(chan struct{})
	tickerDone := make(chan struct{}) // 收尾时先等 ticker goroutine 退出再 flush，保证 hash:done 之后不再有进度/结果事件
	go func() {
		defer close(tickerDone)
		t := time.NewTicker(200 * time.Millisecond)
		defer t.Stop()
		lastBytes, lastTime := int64(0), start
		for {
			select {
			case <-stopTicker:
				return
			case now := <-t.C:
				bd := bytesDone.Load()
				dt := now.Sub(lastTime).Seconds()
				speed := 0.0
				if dt > 0 {
					speed = float64(bd-lastBytes) / dt / 1e6
				}
				lastBytes, lastTime = bd, now
				cur, _ := current.Load().(string)
				runtime.EventsEmit(a.ctx, "hash:progress", ProgressEvent{
					TaskID: taskID, Total: grandTotal, Done: int(doneCount.Load()),
					BytesDone: bd, BytesTotal: totalBytes, CurrentFile: cur,
					SpeedMBps: speed, ElapsedMs: time.Since(start).Milliseconds(),
				})
				flush()
			}
		}
	}()

	// 最先注册、最后执行：清除取消句柄并标记任务完成（供 newTask 淘汰）。
	defer func() {
		a.mu.Lock()
		st.cancel = nil // 任务结束后保留 items 供导出，仅清除取消句柄
		st.done = true
		a.mu.Unlock()
	}()

	// 后注册、先执行：panic 兜底 + 收尾上报。
	defer func() {
		var fatal string
		if r := recover(); r != nil {
			fatal = fmt.Sprintf("%v", r)
			runtime.LogError(a.ctx, fmt.Sprintf("task %s panic: %v", taskID, r))
		}
		close(stopTicker)
		<-tickerDone // 等 ticker goroutine 完全退出，其未发出的 flush 由下面统一完成
		flush()
		st.mu.Lock()
		all := make([]Item, len(st.items))
		copy(all, st.items)
		st.mu.Unlock()
		sum := countSummary(all)
		sum.TaskID = taskID
		sum.Total = grandTotal
		sum.Canceled = ctx.Err() != nil
		sum.ElapsedMs = time.Since(start).Milliseconds()
		sum.BytesDone = bytesDone.Load()
		sum.BytesTotal = totalBytes
		sum.Fatal = fatal
		runtime.EventsEmit(a.ctx, "hash:done", sum)
	}()

	onStart := func(path string) { current.Store(path) }
	onItem := func(r hashcore.Result) {
		item := resultToItem(r)
		if expected != nil { // 批量校验：与清单期望值对比（忽略大小写）
			item.Expected = expected[r.Path]
			item.Verdict = verdictFor(r.Status, item.Expected, r.Hashes[algos[0]])
			if r.Status == hashcore.StatusOK {
				item.Actual = r.Hashes[algos[0]]
			}
		}
		st.mu.Lock()
		st.items = append(st.items, item)
		st.mu.Unlock()
		pmu.Lock()
		pending = append(pending, item)
		n := len(pending)
		pmu.Unlock()
		doneCount.Add(1)
		if n >= 500 {
			flush()
		}
	}

	hashcore.HashFiles(ctx, items, algos, onStart, onItem, &bytesDone)
}

// verdictFor 批量校验的单行结论（纯函数，供单元测试）：
// 计算成功 → 与期望值忽略大小写对比（pass/fail）；
// 文件不存在 → missing；占用/无权限/读取错误等 → error（存在但不可读，
// 与「缺失」语义不同，统计与导出一并区分）。
func verdictFor(st hashcore.Status, expected, actual string) string {
	if st == hashcore.StatusOK {
		if checksum.EqualHash(expected, actual) {
			return "pass"
		}
		return "fail"
	}
	if st == hashcore.StatusNotFound {
		return "missing"
	}
	return "error"
}

// countSummary 汇总任务结果计数（纯函数，供单元测试）。
func countSummary(items []Item) Summary {
	var sum Summary
	for _, it := range items {
		switch it.Verdict {
		case "pass":
			sum.Pass++
		case "fail":
			sum.Fail++
		case "missing":
			sum.Missing++
		}
		if it.Status == string(hashcore.StatusOK) {
			sum.OK++
		} else if it.Status != string(hashcore.StatusCanceled) {
			sum.Errors++
		}
	}
	return sum
}

func resultToItem(r hashcore.Result) Item {
	it := Item{
		Path:       r.Path,
		Name:       filepath.Base(r.Path),
		Size:       r.Size,
		DurationMs: r.Duration.Milliseconds(),
		Status:     string(r.Status),
	}
	if r.Status == hashcore.StatusOK {
		it.Hashes = map[string]string{}
		for algo, h := range r.Hashes {
			it.Hashes[string(algo)] = h
		}
	} else {
		it.ErrCode = string(r.Status)
	}
	return it
}

func toExportItem(it Item) checksum.ExportItem {
	return checksum.ExportItem{
		Path: it.Path, Size: it.Size, Hashes: it.Hashes,
		DurationMs: it.DurationMs, Status: it.Status,
		Expected: it.Expected, Actual: it.Actual, Verdict: it.Verdict,
	}
}

// exportableSUMCount 统计 SUM 导出实际会写出的行数（计算成功且含该算法哈希）。
func exportableSUMCount(items []Item, algo string) int {
	n := 0
	for _, it := range items {
		if it.Status == string(hashcore.StatusOK) && it.Hashes[algo] != "" {
			n++
		}
	}
	return n
}

func parseAlgos(algos []string) ([]hashcore.Algorithm, Result) {
	if len(algos) == 0 {
		return nil, errResult("no_algo", "请至少选择一种算法", "Select at least one algorithm", nil)
	}
	out := make([]hashcore.Algorithm, 0, len(algos))
	for _, s := range algos {
		a, err := hashcore.ParseAlgorithm(s)
		if err != nil {
			return nil, errResult("bad_algo", "未知算法: "+s, "Unknown algorithm: "+s, err)
		}
		out = append(out, a)
	}
	return out, Result{OK: true}
}

// manifestError 将清单解析/识别错误映射为双语结构化错误（含行号）。
func manifestError(err error) Result {
	var ce *checksum.Error
	if errors.As(err, &ce) {
		var zh, en string
		switch ce.Code {
		case "mixed_length":
			zh = fmt.Sprintf("清单中哈希长度不一致（第 %d 行与首行不同），同一清单算法必须一致", ce.Line)
			en = fmt.Sprintf("Inconsistent hash lengths (line %d differs from the first line); one manifest must use a single algorithm", ce.Line)
		case "ext_algo_mismatch":
			zh = fmt.Sprintf("清单第 %d 行哈希长度与文件扩展名所示算法不符（可能改错了后缀名）", ce.Line)
			en = fmt.Sprintf("Hash length at line %d does not match the algorithm implied by the file extension (misnamed file?)", ce.Line)
		case "bad_line":
			zh = fmt.Sprintf("清单第 %d 行格式错误，无法解析", ce.Line)
			en = fmt.Sprintf("Malformed manifest line %d", ce.Line)
		case "bad_hash_length":
			zh = fmt.Sprintf("清单第 %d 行哈希长度不受支持（仅支持 MD5/SHA-1/SHA-256/SHA-512）", ce.Line)
			en = fmt.Sprintf("Unsupported hash length at line %d (MD5/SHA-1/SHA-256/SHA-512 only)", ce.Line)
		case "empty_manifest":
			zh = "清单文件为空或没有有效条目"
			en = "Manifest is empty or has no valid entries"
		default:
			zh = "清单解析失败"
			en = "Failed to parse manifest"
		}
		return Result{OK: false, Error: &AppError{Code: ce.Code, Zh: zh, En: en, Detail: ce.Msg}}
	}
	return errResult("parse_manifest", "清单解析失败", "Failed to parse manifest", err)
}

func errResult(code, zh, en string, err error) Result {
	ae := &AppError{Code: code, Zh: zh, En: en}
	if err != nil {
		ae.Detail = err.Error()
	}
	return Result{OK: false, Error: ae}
}
