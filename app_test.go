package main

// 绑定层纯逻辑的单元测试：不启动 Wails、不触碰 runtime，仅覆盖
// 算法解析、错误映射、汇总计数与任务淘汰等可脱离 GUI 的部分。

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"
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
