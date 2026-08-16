package main

// --selftest：无 GUI 的核心功能自检，供冒烟脚本验证 exe 不止「能开窗」而且「算得对」。
// 逐项打印 PASS/FAIL，任一失败以非零退出码结束。

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"gohash/internal/checksum"
	"gohash/internal/hashcore"
)

// runSelfTest 执行全部自检项，返回进程退出码（0 = 全部通过）。
func runSelfTest() int {
	attachParentConsole() // windowsgui 子系统默认无控制台，附加到父进程控制台以便打印

	dir, err := os.MkdirTemp("", "gohash-selftest")
	if err != nil {
		fmt.Printf("FAIL 创建临时目录: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()

	checks := []struct {
		name string
		fn   func(dir string) error
	}{
		{"known-values(abc)", selfTestKnownValues},
		{"empty-file", selfTestEmptyFile},
		{"sum-roundtrip", selfTestSUMRoundTrip},
		{"manifest-compat", selfTestManifestCompat},
	}
	failed := 0
	for _, c := range checks {
		if err := c.fn(dir); err != nil {
			failed++
			fmt.Printf("FAIL %s: %v\n", c.name, err)
		} else {
			fmt.Printf("PASS %s\n", c.name)
		}
	}
	if failed > 0 {
		fmt.Printf("SELFTEST FAILED (%d/%d)\n", failed, len(checks))
		return 1
	}
	fmt.Printf("SELFTEST OK (%d checks)\n", len(checks))
	return 0
}

// selfTestHashFile 用引擎计算单文件哈希（小文件流式路径）。
func selfTestHashFile(path string, size int64, algos ...hashcore.Algorithm) (hashcore.Result, error) {
	var done atomic.Int64
	var results []hashcore.Result
	hashcore.HashFiles(context.Background(), []hashcore.FileItem{{Path: path, Size: size}}, algos, nil,
		func(r hashcore.Result) { results = append(results, r) }, &done)
	if len(results) != 1 {
		return hashcore.Result{}, fmt.Errorf("expect 1 result, got %d", len(results))
	}
	if results[0].Status != hashcore.StatusOK {
		return hashcore.Result{}, fmt.Errorf("status = %s, err = %v", results[0].Status, results[0].Err)
	}
	return results[0], nil
}

// selfTestKnownValues "abc" 的五种算法已知值。
func selfTestKnownValues(dir string) error {
	p := filepath.Join(dir, "abc.bin")
	if err := os.WriteFile(p, []byte("abc"), 0o644); err != nil {
		return err
	}
	res, err := selfTestHashFile(p, 3, hashcore.All()...)
	if err != nil {
		return err
	}
	want := map[hashcore.Algorithm]string{
		hashcore.MD5:    "900150983cd24fb0d6963f7d28e17f72",
		hashcore.SHA1:   "a9993e364706816aba3e25717850c26c9cd0d89d",
		hashcore.SHA256: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		hashcore.SHA512: "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f",
		hashcore.CRC32:  "352441c2",
	}
	for algo, w := range want {
		if got := res.Hashes[algo]; got != w {
			return fmt.Errorf("%s = %s, want %s", algo, got, w)
		}
	}
	return nil
}

// selfTestEmptyFile 空文件的 SHA-256 已知值。
func selfTestEmptyFile(dir string) error {
	p := filepath.Join(dir, "empty.bin")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		return err
	}
	res, err := selfTestHashFile(p, 0, hashcore.SHA256)
	if err != nil {
		return err
	}
	const want = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if res.Hashes[hashcore.SHA256] != want {
		return fmt.Errorf("empty sha256 = %s, want %s", res.Hashes[hashcore.SHA256], want)
	}
	return nil
}

// selfTestSUMRoundTrip 计算 → 导出 SUM → 重新解析 → 识别算法 → 重新计算 → 全部一致。
func selfTestSUMRoundTrip(dir string) error {
	files := map[string]string{
		"alpha.bin":            "content-alpha",
		"sub/my file name.bin": "content-with-space",
	}
	var items []hashcore.FileItem
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return err
		}
		info, _ := os.Stat(p)
		items = append(items, hashcore.FileItem{Path: p, Size: info.Size()})
	}

	var export []checksum.ExportItem
	for _, it := range items {
		res, err := selfTestHashFile(it.Path, it.Size, hashcore.SHA256)
		if err != nil {
			return err
		}
		export = append(export, checksum.ExportItem{
			Path:   it.Path,
			Size:   it.Size,
			Hashes: map[string]string{string(hashcore.SHA256): res.Hashes[hashcore.SHA256]},
			Status: "ok",
		})
	}

	var buf bytes.Buffer
	if err := checksum.WriteSUM(&buf, export, hashcore.SHA256); err != nil {
		return err
	}
	entries, err := checksum.ParseManifest(buf.Bytes())
	if err != nil {
		return err
	}
	algo, err := checksum.DetectAlgorithm("out.sha256", entries)
	if err != nil {
		return err
	}
	if algo != hashcore.SHA256 {
		return fmt.Errorf("algo = %s, want sha256", algo)
	}
	for _, e := range entries {
		res, err := selfTestHashFile(e.Path, 0, algo)
		if err != nil {
			return err
		}
		if !checksum.EqualHash(e.Hash, res.Hashes[algo]) {
			return fmt.Errorf("roundtrip mismatch for %s", e.Path)
		}
	}
	return nil
}

// selfTestManifestCompat BOM + CRLF/LF 混用 + GNU 转义行 + 坏行行号。
func selfTestManifestCompat(dir string) error {
	const h = "900150983cd24fb0d6963f7d28e17f72"
	data := []byte{0xEF, 0xBB, 0xBF}
	data = append(data, []byte(h+"  a.txt\r\n")...)
	data = append(data, []byte("\\"+h+"  b\\\\c.txt\n")...)
	entries, err := checksum.ParseManifest(data)
	if err != nil {
		return err
	}
	if len(entries) != 2 || entries[0].Path != "a.txt" || entries[1].Path != `b\c.txt` {
		return fmt.Errorf("entries = %+v", entries)
	}

	_, err = checksum.ParseManifest([]byte(h + "  a.txt\nnot-a-hash-line\n"))
	var ce *checksum.Error
	if !errors.As(err, &ce) || ce.Code != "bad_line" || ce.Line != 2 {
		return fmt.Errorf("bad line error = %v, want bad_line at line 2", err)
	}
	return nil
}
