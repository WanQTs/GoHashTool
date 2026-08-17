package hashcore

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 写临时文件并返回路径。
func writeTempFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return p
}

func hashSingle(t *testing.T, path string, size int64, algos []Algorithm) Result {
	t.Helper()
	var done atomic.Int64
	var results []Result
	var mu sync.Mutex
	HashFiles(context.Background(), []FileItem{{Path: path, Size: size}}, algos, nil,
		func(r Result) {
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}, &done)
	if len(results) != 1 {
		t.Fatalf("expect 1 result, got %d", len(results))
	}
	return results[0]
}

// TestKnownValues 已知值测试："abc" 的 SHA-256 与 MD5。
func TestKnownValues(t *testing.T) {
	dir := t.TempDir()
	p := writeTempFile(t, dir, "abc.bin", []byte("abc"))

	res := hashSingle(t, p, 3, []Algorithm{SHA256, MD5, SHA1, SHA512, CRC32})
	if res.Status != StatusOK {
		t.Fatalf("status = %s, err = %v", res.Status, res.Err)
	}
	const wantSHA256 = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	const wantMD5 = "900150983cd24fb0d6963f7d28e17f72"
	if got := res.Hashes[SHA256]; got != wantSHA256 {
		t.Errorf("sha256 = %s, want %s", got, wantSHA256)
	}
	if got := res.Hashes[MD5]; got != wantMD5 {
		t.Errorf("md5 = %s, want %s", got, wantMD5)
	}
	// 同时校验其余算法与标准库一次性计算结果一致（多算法一次扫描正确性）。
	if got := res.Hashes[SHA1]; got != fmt.Sprintf("%x", sha1.Sum([]byte("abc"))) {
		t.Errorf("sha1 mismatch: %s", got)
	}
	// CRC32("abc") 已知值。
	if got := res.Hashes[CRC32]; got != "352441c2" {
		t.Errorf("crc32 = %s, want 352441c2", got)
	}
}

// TestEmptyFile 空文件（0 字节）已知值：sha256("") 与 crc32("")=0。
func TestEmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := writeTempFile(t, dir, "empty.bin", nil)

	res := hashSingle(t, p, 0, []Algorithm{SHA256, CRC32, MD5})
	if res.Status != StatusOK {
		t.Fatalf("status = %s, err = %v", res.Status, res.Err)
	}
	const wantSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got := res.Hashes[SHA256]; got != wantSHA256 {
		t.Errorf("empty sha256 = %s, want %s", got, wantSHA256)
	}
	if got := res.Hashes[CRC32]; got != "00000000" {
		t.Errorf("empty crc32 = %s, want 00000000", got)
	}
}

// TestDirectoryReadError 目录被当文件计算：打开可能成功但读取必失败，
// 应标记为错误状态而非中断整批。
func TestDirectoryReadError(t *testing.T) {
	dir := t.TempDir()
	res := hashSingle(t, dir, 4096, []Algorithm{SHA256})
	if res.Status == StatusOK {
		t.Error("hashing a directory should not succeed")
	}
	if res.Status == StatusNotFound {
		t.Error("existing directory should not be reported as not_found")
	}
}

// TestTotalSize 缺失（Size=-1）与空文件（Size=0）不计入总字节数。
func TestTotalSize(t *testing.T) {
	items := []FileItem{{Size: 10}, {Size: -1}, {Size: 0}, {Size: 5}}
	if got := TotalSize(items); got != 15 {
		t.Errorf("TotalSize = %d, want 15", got)
	}
}

// TestExpandPathsDedup 重叠选择（文件夹 + 其中文件 + 同文件重复添加）只计算一次。
func TestExpandPathsDedup(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	p1 := writeTempFile(t, dir, "a.txt", []byte("a"))
	p2 := writeTempFile(t, sub, "b.txt", []byte("bb"))

	// 文件夹与其中文件重叠、同一文件重复出现。
	items := ExpandPaths([]string{dir, p1, p1})
	counts := map[string]int{}
	for _, it := range items {
		counts[it.Path]++
	}
	if len(items) != 2 {
		t.Errorf("expect 2 unique items, got %d: %+v", len(items), items)
	}
	for _, p := range []string{p1, p2} {
		if counts[p] != 1 {
			t.Errorf("%s appears %d times, want 1", p, counts[p])
		}
	}
}

// TestExpandPathsDetailed 正常树无跳过目录；与 ExpandPaths 结果一致。
func TestExpandPathsDetailed(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	p1 := writeTempFile(t, dir, "a.txt", []byte("a"))
	writeTempFile(t, sub, "b.txt", []byte("bb"))

	items, skipped := ExpandPathsDetailed([]string{dir, p1})
	if len(skipped) != 0 {
		t.Errorf("readable tree: skipped = %v, want empty", skipped)
	}
	if len(items) != 2 {
		t.Fatalf("expect 2 unique items, got %d: %+v", len(items), items)
	}
	plain := ExpandPaths([]string{dir, p1})
	if len(plain) != len(items) {
		t.Errorf("ExpandPaths = %d items, ExpandPathsDetailed = %d, want equal", len(plain), len(items))
	}
}

// TestLargeFileMatchesStdlib 大文件（走流水线路径）与标准库结果一致。
func TestLargeFileMatchesStdlib(t *testing.T) {
	dir := t.TempDir()
	// 70MB，刚好超过 64MB 流水线阈值。
	size := 70 << 20
	p := filepath.Join(dir, "large.bin")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	block := make([]byte, 1<<20)
	for i := range block {
		block[i] = byte(i * 31)
	}
	for written := 0; written < size; written += len(block) {
		if _, err := f.Write(block); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()

	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	wantSHA256 := fmt.Sprintf("%x", sha256.Sum256(data))
	wantMD5 := fmt.Sprintf("%x", md5.Sum(data))

	res := hashSingle(t, p, int64(size), []Algorithm{SHA256, MD5})
	if res.Status != StatusOK {
		t.Fatalf("status = %s, err = %v", res.Status, res.Err)
	}
	if res.Hashes[SHA256] != wantSHA256 {
		t.Error("pipelined sha256 mismatch")
	}
	if res.Hashes[MD5] != wantMD5 {
		t.Error("pipelined md5 mismatch")
	}
}

// TestExpandPaths 目录递归展开与缺失文件保留。
func TestExpandPaths(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	p1 := writeTempFile(t, dir, "a.txt", []byte("a"))
	p2 := writeTempFile(t, sub, "b.txt", []byte("bb"))

	items := ExpandPaths([]string{dir, filepath.Join(dir, "ghost.txt")})
	var found1, found2, ghost bool
	for _, it := range items {
		switch it.Path {
		case p1:
			found1 = it.Size == 1
		case p2:
			found2 = it.Size == 2
		case filepath.Join(dir, "ghost.txt"):
			ghost = it.Size == -1
		}
	}
	if !found1 || !found2 {
		t.Errorf("recursive expand failed: %+v", items)
	}
	if !ghost {
		t.Error("missing file should be kept with Size=-1")
	}
}

// TestMissingFileMarked 不存在/被删除的文件单独标记，不中断整批。
func TestMissingFileMarked(t *testing.T) {
	dir := t.TempDir()
	ok := writeTempFile(t, dir, "ok.txt", []byte("ok"))
	missing := filepath.Join(dir, "missing.txt")

	items := []FileItem{{Path: ok, Size: 2}, {Path: missing, Size: -1}}
	var results []Result
	var mu sync.Mutex // onItem 回调在不同 worker goroutine 中触发，需自行同步
	var done atomic.Int64
	HashFiles(context.Background(), items, []Algorithm{SHA256}, nil, func(r Result) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	}, &done)
	if len(results) != 2 {
		t.Fatalf("expect 2 results, got %d", len(results))
	}
	statuses := map[string]Status{}
	for _, r := range results {
		statuses[r.Path] = r.Status
	}
	if statuses[ok] != StatusOK {
		t.Errorf("ok file status = %s", statuses[ok])
	}
	if statuses[missing] != StatusNotFound {
		t.Errorf("missing file status = %s", statuses[missing])
	}
}

// TestCancel 取消后任务尽快停止。
func TestCancel(t *testing.T) {
	dir := t.TempDir()
	// 构造足够多大文件，保证取消时仍有未完成任务。
	var items []FileItem
	block := make([]byte, 1<<20)
	for i := 0; i < 4; i++ {
		p := filepath.Join(dir, fmt.Sprintf("big%d.bin", i))
		f, _ := os.Create(p)
		for w := 0; w < 200; w++ { // 每个 200MB
			f.Write(block)
		}
		f.Close()
		items = append(items, FileItem{Path: p, Size: 200 << 20})
	}

	ctx, cancel := context.WithCancel(context.Background())
	var done atomic.Int64
	finished := make(chan struct{})
	go func() {
		HashFiles(ctx, items, []Algorithm{SHA256}, nil, func(Result) {}, &done)
		close(finished)
	}()
	time.Sleep(150 * time.Millisecond)
	cancelStart := time.Now()
	cancel()
	select {
	case <-finished:
		if elapsed := time.Since(cancelStart); elapsed > time.Second {
			t.Errorf("cancel took %v, want < 1s", elapsed)
		}
	case <-time.After(3 * time.Second):
		t.Error("cancel did not take effect within 3s")
	}
}

// TestCancelStreaming 流式路径（<64MB 文件走 worker pool）取消同样 1 秒内生效。
func TestCancelStreaming(t *testing.T) {
	dir := t.TempDir()
	// 16 个 63MB 文件（刚好低于 64MB 流水线阈值），保证取消时仍有在途任务。
	var items []FileItem
	block := make([]byte, 1<<20)
	for i := 0; i < 16; i++ {
		p := filepath.Join(dir, fmt.Sprintf("mid%d.bin", i))
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		for w := 0; w < 63; w++ {
			if _, err := f.Write(block); err != nil {
				t.Fatal(err)
			}
		}
		f.Close()
		items = append(items, FileItem{Path: p, Size: 63 << 20})
	}

	ctx, cancel := context.WithCancel(context.Background())
	var done atomic.Int64
	finished := make(chan struct{})
	go func() {
		HashFiles(ctx, items, []Algorithm{SHA256}, nil, func(Result) {}, &done)
		close(finished)
	}()
	time.Sleep(100 * time.Millisecond)
	cancelStart := time.Now()
	cancel()
	select {
	case <-finished:
		if elapsed := time.Since(cancelStart); elapsed > time.Second {
			t.Errorf("streaming cancel took %v, want < 1s", elapsed)
		}
	case <-time.After(3 * time.Second):
		t.Error("streaming cancel did not take effect within 3s")
	}
}

func TestParseAlgorithm(t *testing.T) {
	for in, want := range map[string]Algorithm{
		"md5": MD5, "SHA-256": SHA256, "SHA512": SHA512, "sha-1": SHA1, "CRC32": CRC32,
	} {
		got, err := ParseAlgorithm(in)
		if err != nil || got != want {
			t.Errorf("ParseAlgorithm(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseAlgorithm("blake3"); err == nil {
		t.Error("expect error for unsupported algorithm")
	}
}

func TestDetectByLength(t *testing.T) {
	for n, want := range map[int]Algorithm{32: MD5, 40: SHA1, 64: SHA256, 128: SHA512} {
		got, err := DetectByLength(n)
		if err != nil || got != want {
			t.Errorf("DetectByLength(%d) = %v, %v; want %v", n, got, err, want)
		}
	}
	if _, err := DetectByLength(7); err == nil {
		t.Error("expect error for unknown length")
	}
}

// TestWalkErrAction 目录遍历出错时的继续策略：目录不可读跳过其子树，
// 文件级错误只跳过该文件——SkipDir 作用于文件会连带跳过同目录其余文件（静默漏算）。
func TestWalkErrAction(t *testing.T) {
	if got := walkErrAction(nil, nil); got != nil {
		t.Errorf("nil err: got %v, want nil", got)
	}
	if got := walkErrAction(fakeDirEntry{dir: true}, errors.New("denied")); got != filepath.SkipDir {
		t.Errorf("dir err: got %v, want SkipDir", got)
	}
	if got := walkErrAction(fakeDirEntry{dir: false}, errors.New("gone")); got != nil {
		t.Errorf("file err: got %v, want nil (skip only this file)", got)
	}
	// 根路径 Lstat 失败时 d 为 nil，同样只跳过自身。
	if got := walkErrAction(nil, errors.New("root gone")); got != nil {
		t.Errorf("nil entry err: got %v, want nil", got)
	}
}

// fakeDirEntry 最小 fs.DirEntry 实现，供 walkErrAction 单测。
type fakeDirEntry struct{ dir bool }

func (e fakeDirEntry) Name() string { return "x" }
func (e fakeDirEntry) IsDir() bool  { return e.dir }
func (e fakeDirEntry) Type() fs.FileMode {
	if e.dir {
		return fs.ModeDir
	}
	return 0
}
func (e fakeDirEntry) Info() (fs.FileInfo, error) { return nil, nil }

// TestCanonicalKey Windows 下大小写折叠，其余平台原样（仅 Clean 规范化）。
func TestCanonicalKey(t *testing.T) {
	got := CanonicalKey(`C:\Mix\\Case\File.TXT`)
	if runtime.GOOS == "windows" {
		if got != strings.ToLower(filepath.Clean(`C:\Mix\\Case\File.TXT`)) {
			t.Errorf("windows key = %q, want case-folded clean path", got)
		}
	} else if got != filepath.Clean(`C:\Mix\\Case\File.TXT`) {
		t.Errorf("non-windows key = %q, want clean path without folding", got)
	}
}

// TestExpandPathsDetailedContextCancel 目录展开可被取消：扫描到一半取消 ctx，
// 遍历必须尽快终止并返回 context.Canceled（部分结果不得当作完整清单使用）。
func TestExpandPathsDetailedContextCancel(t *testing.T) {
	dir := t.TempDir()
	// 造足够多的文件（200 目录 × 50 文件），保证取消时遍历仍在进行。
	for i := 0; i < 200; i++ {
		sub := filepath.Join(dir, fmt.Sprintf("d%03d", i))
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 50; j++ {
			writeTempFile(t, sub, fmt.Sprintf("f%02d.txt", j), []byte("x"))
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	found := 0
	_, _, err := ExpandPathsDetailedContext(ctx, []string{dir}, func(string, int) {
		found++
		if found == 100 { // 扫到 100 个时取消（总量 10000）
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if found >= 10000 {
		t.Errorf("scan should stop shortly after cancel, found = %d (total 10000)", found)
	}
}

// TestExpandPathsDetailedContextScanCallback onScan 按纳入顺序递增报数，
// 末次计数等于最终文件数（供上层做「已发现 N 个文件」的扫描进度）。
func TestExpandPathsDetailedContextScanCallback(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		writeTempFile(t, dir, fmt.Sprintf("f%d.txt", i), []byte("x"))
	}
	var counts []int
	items, _, err := ExpandPathsDetailedContext(context.Background(), []string{dir},
		func(_ string, found int) { counts = append(counts, found) })
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 5 || len(counts) != 5 {
		t.Fatalf("items = %d, counts = %v; want 5 items and 5 callbacks", len(items), counts)
	}
	for i, c := range counts {
		if c != i+1 {
			t.Fatalf("counts = %v, want 1..5", counts)
		}
	}
}
