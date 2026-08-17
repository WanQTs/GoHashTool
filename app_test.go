package main

// 绑定层纯逻辑的单元测试：不启动 Wails、不触碰 runtime，仅覆盖
// 算法解析、错误映射、汇总计数与任务淘汰等可脱离 GUI 的部分。

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"gohash/internal/checksum"
	"gohash/internal/hashcore"
)

func TestParseAlgos(t *testing.T) {
	if _, r := parseAlgos(nil); r.OK || r.Error.Code != "no_algo" {
		t.Errorf("empty algos: got %+v, want no_algo error", r)
	}
	if _, r := parseAlgos([]string{"md5", "blake3"}); r.OK || r.Error.Code != "bad_algo" {
		t.Errorf("bad algo: got %+v, want bad_algo error", r)
	}
	parsed, r := parseAlgos([]string{"SHA-256", "md5"})
	if !r.OK || len(parsed) != 2 || parsed[0] != "sha256" || parsed[1] != "md5" {
		t.Errorf("valid algos: got %v, %+v", parsed, r)
	}
}

func TestManifestErrorMapping(t *testing.T) {
	cases := []struct {
		code string
		line int
	}{
		{"mixed_length", 3},
		{"ext_algo_mismatch", 4},
		{"bad_line", 5},
		{"bad_hash_length", 7},
		{"empty_manifest", 0},
		{"unknown_code", 0},
	}
	for _, c := range cases {
		r := manifestError(&checksum.Error{Code: c.code, Line: c.line, Msg: "m"})
		if r.OK || r.Error == nil {
			t.Fatalf("%s: expect error result", c.code)
		}
		if r.Error.Code != c.code {
			t.Errorf("%s: code = %s", c.code, r.Error.Code)
		}
		if r.Error.Zh == "" || r.Error.En == "" {
			t.Errorf("%s: bilingual message must be non-empty", c.code)
		}
		if c.line > 0 && !strings.Contains(r.Error.Zh, strconv.Itoa(c.line)) {
			t.Errorf("%s: zh message should contain line %d: %s", c.code, c.line, r.Error.Zh)
		}
	}
	// 非 checksum.Error 一律映射为 parse_manifest
	r := manifestError(errors.New("boom"))
	if r.OK || r.Error.Code != "parse_manifest" || r.Error.Detail != "boom" {
		t.Errorf("plain error: got %+v", r.Error)
	}
}

func TestErrResult(t *testing.T) {
	r := errResult("x", "中", "en", errors.New("detail-info"))
	if r.OK || r.Error.Code != "x" || r.Error.Zh != "中" || r.Error.En != "en" || r.Error.Detail != "detail-info" {
		t.Errorf("got %+v", r.Error)
	}
	r = errResult("x", "中", "en", nil)
	if r.Error.Detail != "" {
		t.Errorf("nil err should produce empty detail, got %q", r.Error.Detail)
	}
}

func TestCountSummary(t *testing.T) {
	items := []Item{
		{Status: "ok", Verdict: "pass"},
		{Status: "ok", Verdict: "fail"},
		{Status: "not_found", Verdict: "missing"},
		{Status: "occupied"},
		{Status: "canceled"},
		{Status: "ok"},
	}
	sum := countSummary(items)
	if sum.OK != 3 || sum.Errors != 2 || sum.Pass != 1 || sum.Fail != 1 || sum.Missing != 1 {
		t.Errorf("sum = %+v, want ok=3 errors=2 pass=1 fail=1 missing=1", sum)
	}
}

// TestTaskEviction 已完成任务结果仅保留最近 maxFinishedTasks 个，运行中任务不参与淘汰。
func TestTaskEviction(t *testing.T) {
	a := NewApp()
	var ids []string
	for i := 0; i < 7; i++ {
		id, st := a.newTask([]string{"sha256"}, false, nil)
		ids = append(ids, id)
		a.mu.Lock()
		st.done = true // 模拟任务完成
		a.mu.Unlock()
	}
	runID, _ := a.newTask([]string{"sha256"}, false, nil) // 运行中任务

	a.mu.Lock()
	defer a.mu.Unlock()
	doneKept := 0
	for _, st := range a.tasks {
		if st.done {
			doneKept++
		}
	}
	if doneKept != maxFinishedTasks {
		t.Errorf("done tasks kept = %d, want %d", doneKept, maxFinishedTasks)
	}
	for i := 0; i < 3; i++ { // t1~t3 应已淘汰
		if _, ok := a.tasks[ids[i]]; ok {
			t.Errorf("task %s should be evicted", ids[i])
		}
	}
	if _, ok := a.tasks[ids[6]]; !ok {
		t.Error("newest done task should be kept")
	}
	if _, ok := a.tasks[runID]; !ok {
		t.Error("running task must never be evicted")
	}
}

// TestExportableSUMCount SUM 导出只统计「计算成功且含所选算法哈希」的行。
func TestExportableSUMCount(t *testing.T) {
	items := []Item{
		{Status: "ok", Hashes: map[string]string{"sha256": "a"}},
		{Status: "ok", Hashes: map[string]string{"md5": "b"}},          // 缺所选算法
		{Status: "occupied", Hashes: map[string]string{"sha256": "c"}}, // 失败行
		{Status: "ok"}, // 无哈希
	}
	if got := exportableSUMCount(items, "sha256"); got != 1 {
		t.Errorf("sha256 count = %d, want 1", got)
	}
	if got := exportableSUMCount(items, "md5"); got != 1 {
		t.Errorf("md5 count = %d, want 1", got)
	}
	if got := exportableSUMCount(nil, "sha256"); got != 0 {
		t.Errorf("nil items count = %d, want 0", got)
	}
}

// TestVerdictFor 批量校验结论映射：仅 not_found 计 missing；
// 占用/无权限/读取错误等「存在但不可读」必须计 error，不得混入缺失。
func TestVerdictFor(t *testing.T) {
	cases := []struct {
		name     string
		status   hashcore.Status
		expected string
		actual   string
		want     string
	}{
		{"pass", hashcore.StatusOK, "ABC", "abc", "pass"}, // 忽略大小写
		{"fail", hashcore.StatusOK, "abc", "def", "fail"},
		{"missing", hashcore.StatusNotFound, "abc", "", "missing"},
		{"occupied-is-error", hashcore.StatusOccupied, "abc", "", "error"},
		{"no-permission-is-error", hashcore.StatusNoPermission, "abc", "", "error"},
		{"read-error-is-error", hashcore.StatusError, "abc", "", "error"},
		{"canceled-is-error", hashcore.StatusCanceled, "abc", "", "error"},
	}
	for _, c := range cases {
		if got := verdictFor(c.status, c.expected, c.actual); got != c.want {
			t.Errorf("%s: verdictFor(%s) = %s, want %s", c.name, c.status, got, c.want)
		}
	}
}

// TestNewTaskRegistersCancel 取消句柄随任务入表同步登记：
// Start 返回即点取消不得报「任务不存在或已结束」。
func TestNewTaskRegistersCancel(t *testing.T) {
	a := NewApp()
	canceled := false
	_, st := a.newTask([]string{"sha256"}, false, func() { canceled = true })
	if st.cancel == nil {
		t.Fatal("cancel handle must be registered with the task")
	}
	st.cancel()
	if !canceled {
		t.Error("registered cancel must be callable")
	}
}

// TestExportSUMRejectsCRC32 CRC32 导出 SUM 后无法重新导入校验，必须拒绝。
func TestExportSUMRejectsCRC32(t *testing.T) {
	a := NewApp()
	id, st := a.newTask([]string{"crc32"}, false, nil)
	st.items = []Item{{
		Path: "f.bin", Status: "ok",
		Hashes: map[string]string{"crc32": "352441c2"},
	}}
	r := a.ExportSUM(id, filepath.Join(t.TempDir(), "out.crc32"), "crc32")
	if r.OK || r.Error == nil || r.Error.Code != "algo_not_exportable" {
		t.Errorf("crc32 export: got %+v, want algo_not_exportable error", r)
	}
}

// TestWriteExportConcurrent 同路径并发导出使用唯一临时文件（os.CreateTemp），
// 互不干扰；最终内容是某一次的完整写入，且目录下无残留临时文件。
func TestWriteExportConcurrent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.csv")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := writeExport(target, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "row-%d\n", i)
				return err
			}); err != nil {
				t.Errorf("writeExport: %v", err)
			}
		}(i)
	}
	wg.Wait()
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "row-") || !strings.HasSuffix(string(data), "\n") {
		t.Errorf("content must be one complete write, got %q", data)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "out.csv" {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestIsOpenWithManifest(t *testing.T) {
	yes := []string{"a.sha256", "B.MD5", "x/y/z.sum", "note.txt", "f.sHa1"}
	for _, p := range yes {
		if !isOpenWithManifest(p) {
			t.Errorf("%s should be recognised as manifest", p)
		}
	}
	no := []string{"a.csv", "b.exe", "sha256", "c.sha256.bak", ""}
	for _, p := range no {
		if isOpenWithManifest(p) {
			t.Errorf("%s should not be recognised as manifest", p)
		}
	}
	// .txt 仅识别不注册（避免劫持通用文本文件关联）
	for _, ext := range assocExts {
		if ext == ".txt" {
			t.Fatal(".txt must not be registered as a file association")
		}
	}
}

func TestManifestArgFromArgs(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "release.sha256")
	if err := os.WriteFile(manifest, []byte("deadbeef  app.exe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(dir, "data.csv")
	if err := os.WriteFile(plain, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// args[0] 是可执行文件自身路径，不参与挑选
	if got := manifestArgFromArgs([]string{"gohash.exe", manifest}); got != manifest {
		t.Errorf("got %q, want %q", got, manifest)
	}
	// 第一个实际存在的清单胜出；非清单与不存在路径跳过
	missing := filepath.Join(dir, "gone.md5")
	if got := manifestArgFromArgs([]string{"gohash.exe", plain, missing, manifest}); got != manifest {
		t.Errorf("got %q, want %q", got, manifest)
	}
	if got := manifestArgFromArgs([]string{"gohash.exe"}); got != "" {
		t.Errorf("no args: got %q, want empty", got)
	}
	if got := manifestArgFromArgs([]string{"gohash.exe", plain, missing}); got != "" {
		t.Errorf("no manifest: got %q, want empty", got)
	}
}

func TestConsumePendingOpenFile(t *testing.T) {
	a := NewApp()
	if r := a.ConsumePendingOpenFile(); !r.OK || r.Path != "" {
		t.Errorf("empty pending: got %+v", r)
	}
	a.openMu.Lock()
	a.pendingOpen = `C:\sums\release.sha256`
	a.openMu.Unlock()
	r := a.ConsumePendingOpenFile()
	if !r.OK || r.Path != `C:\sums\release.sha256` {
		t.Errorf("got %+v", r)
	}
	// 拉取后清空，再拉为空
	if r := a.ConsumePendingOpenFile(); r.Path != "" {
		t.Errorf("should be cleared after consume, got %q", r.Path)
	}
}

func TestSetAlwaysOnTopWithoutApp(t *testing.T) {
	// 无 Wails 应用实例（单测环境）时结构化报错，不得 panic
	a := NewApp()
	if r := a.SetAlwaysOnTop(true); r.OK || r.Error == nil || r.Error.Code != "no_window" {
		t.Errorf("got %+v, want no_window error", r)
	}
}

func TestDecodeRowContext(t *testing.T) {
	// 与前端 encodeURIComponent(JSON.stringify(...)) 等价的手工编码样例：
	// JSON 文本 {"path":"C:\\a b\\f.txt","hashes":{"md5":"abc"}}
	enc := `%7B%22path%22%3A%22C%3A%5C%5Ca%20b%5C%5Cf.txt%22%2C%22hashes%22%3A%7B%22md5%22%3A%22abc%22%7D%7D`
	p, err := decodeRowContext(enc)
	if err != nil {
		t.Fatal(err)
	}
	if p.Path != `C:\a b\f.txt` {
		t.Errorf("path = %q", p.Path)
	}
	if p.Hashes["md5"] != "abc" {
		t.Errorf("md5 = %q", p.Hashes["md5"])
	}

	// 中文路径（UTF-8 百分号编码）
	encZh := `%7B%22path%22%3A%22%E4%B8%AD%E6%96%87.txt%22%2C%22hashes%22%3A%7B%7D%7D`
	pz, err := decodeRowContext(encZh)
	if err != nil {
		t.Fatal(err)
	}
	if pz.Path != "中文.txt" {
		t.Errorf("zh path = %q", pz.Path)
	}

	// 非法百分号转义与非法 JSON 都必须报错（不得静默吞错）
	if _, err := decodeRowContext("%zz"); err == nil {
		t.Error("bad escape should fail")
	}
	if _, err := decodeRowContext("%7Bbad"); err == nil {
		t.Error("bad JSON should fail")
	}
}

func TestFileAssocPlan(t *testing.T) {
	const progid = "GoHashTool.sha256"
	want := assocCommandFor(`D:\Tools\gohash.exe`)
	stale := assocCommandFor(`C:\Old\gohash.exe`)

	// 注册判定：未关联 → 写；他人占用 → 跳过；我们的但路径陈旧 → 自愈写；已是期望 → 跳过
	if !planAssocWrite("", progid, "", want) {
		t.Error("unassociated ext should be written")
	}
	if planAssocWrite("OtherApp.sha256", progid, "", want) {
		t.Error("ext owned by another app must not be hijacked")
	}
	if !planAssocWrite(progid, progid, stale, want) {
		t.Error("stale command of our own ProgID should be healed")
	}
	if planAssocWrite(progid, progid, want, want) {
		t.Error("already-current association should be skipped")
	}

	// 自愈判定：只动「明确是我们的且陈旧」；未关联（含用户手动解除后）一律不写
	if !planAssocHeal(progid, progid, stale, want) {
		t.Error("stale own association should be healed")
	}
	if planAssocHeal("", progid, "", want) {
		t.Error("unassociated ext must not be auto-registered by heal")
	}
	if planAssocHeal("OtherApp.sha256", progid, stale, want) {
		t.Error("foreign-owned ext must not be healed")
	}

	// 解除判定：只删自己的 ProgID
	if !planAssocRemove(progid, progid) {
		t.Error("our own association should be removable")
	}
	if planAssocRemove("OtherApp.sha256", progid) || planAssocRemove("", progid) {
		t.Error("foreign or absent association must not be removed")
	}

	if got := progIDFor(".md5"); got != "GoHashTool.md5" {
		t.Errorf("progIDFor = %q", got)
	}
	if got := assocCommandFor(`C:\a b\gohash.exe`); got != `"C:\a b\gohash.exe" "%1"` {
		t.Errorf("assocCommandFor = %q", got)
	}
}
