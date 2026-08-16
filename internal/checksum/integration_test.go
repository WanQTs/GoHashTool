package checksum_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"gohash/internal/checksum"
	"gohash/internal/hashcore"
)

// hashPaths 用引擎计算一组文件的哈希，返回 path -> hashes。
func hashPaths(t *testing.T, items []hashcore.FileItem, algo hashcore.Algorithm) map[string]map[string]string {
	t.Helper()
	out := map[string]map[string]string{}
	var mu sync.Mutex
	var done atomic.Int64
	hashcore.HashFiles(context.Background(), items, []hashcore.Algorithm{algo}, nil, func(r hashcore.Result) {
		mu.Lock()
		if r.Status == hashcore.StatusOK {
			m := map[string]string{}
			for a, h := range r.Hashes {
				m[string(a)] = h
			}
			out[r.Path] = m
		}
		mu.Unlock()
	}, &done)
	return out
}

// TestSUMRoundTrip 闭环集成测试：
// 计算 → 导出 SUM → 重新导入解析 → 算法识别 → 重新计算 → 全部通过。
func TestSUMRoundTrip(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"alpha.bin":        "content-alpha",
		"my file name.bin": "content-with-space",
		"sub/beta.bin":     "content-beta",
	}
	var items []hashcore.FileItem
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		info, _ := os.Stat(p)
		items = append(items, hashcore.FileItem{Path: p, Size: info.Size()})
	}

	// 1. 计算哈希并组装导出行。
	hashed := hashPaths(t, items, hashcore.SHA256)
	var export []checksum.ExportItem
	for _, it := range items {
		export = append(export, checksum.ExportItem{
			Path:   it.Path,
			Size:   it.Size,
			Hashes: hashed[it.Path],
			Status: "ok",
		})
	}

	// 2. 导出 SUM 文件。
	sumPath := filepath.Join(dir, "out.sha256")
	var buf bytes.Buffer
	if err := checksum.WriteSUM(&buf, export, hashcore.SHA256); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sumPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	// 3. 重新导入：解析 + 算法识别（按扩展名）。
	data, err := os.ReadFile(sumPath)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := checksum.ParseManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	algo, err := checksum.DetectAlgorithm(sumPath, entries)
	if err != nil {
		t.Fatal(err)
	}
	if algo != hashcore.SHA256 {
		t.Fatalf("algo = %s", algo)
	}

	// 4. 解析路径（绝对路径直接用）并重新计算对比。
	var reItems []hashcore.FileItem
	for _, e := range entries {
		p := e.Path
		if !filepath.IsAbs(p) {
			p = filepath.Join(filepath.Dir(sumPath), p)
		}
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("exported path should exist: %s", p)
		}
		reItems = append(reItems, hashcore.FileItem{Path: p, Size: info.Size()})
	}
	reHashed := hashPaths(t, reItems, algo)

	if len(reHashed) != len(entries) {
		t.Fatalf("rehashed %d files, want %d", len(reHashed), len(entries))
	}
	for _, e := range entries {
		p := e.Path
		if !filepath.IsAbs(p) {
			p = filepath.Join(filepath.Dir(sumPath), p)
		}
		actual := reHashed[p][string(algo)]
		if !checksum.EqualHash(e.Hash, actual) {
			t.Errorf("round trip mismatch for %s: expected %s got %s", p, e.Hash, actual)
		}
	}
}

// TestSUMRoundTripRelative 相对路径条目相对基准目录解析的闭环。
func TestSUMRoundTripRelative(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	hashed := hashPaths(t, []hashcore.FileItem{{Path: filepath.Join(dir, "a.txt"), Size: 3}}, hashcore.MD5)

	var buf bytes.Buffer
	err := checksum.WriteSUM(&buf, []checksum.ExportItem{{
		Path:   "a.txt", // 相对路径
		Size:   3,
		Hashes: hashed[filepath.Join(dir, "a.txt")],
		Status: "ok",
	}}, hashcore.MD5)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := checksum.ParseManifest(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	algo, err := checksum.DetectAlgorithm("noext", entries) // 无扩展名 → 按长度推断
	if err != nil || algo != hashcore.MD5 {
		t.Fatalf("algo = %v, %v", algo, err)
	}
	resolved := filepath.Join(dir, entries[0].Path)
	reHashed := hashPaths(t, []hashcore.FileItem{{Path: resolved, Size: 3}}, algo)
	if !checksum.EqualHash(entries[0].Hash, reHashed[resolved][string(algo)]) {
		t.Error("relative round trip mismatch")
	}
}

// TestWriteCSVBOM CSV 导出带 UTF-8 BOM 且行数正确。
func TestWriteCSVBOM(t *testing.T) {
	var buf bytes.Buffer
	items := []checksum.ExportItem{{
		Path:   `C:\data\a.txt`,
		Size:   3,
		Hashes: map[string]string{"sha256": strings.Repeat("ab", 32)},
		Status: "ok",
	}}
	if err := checksum.WriteCSV(&buf, items, []string{"sha256"}, false); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	if len(b) < 3 || b[0] != 0xEF || b[1] != 0xBB || b[2] != 0xBF {
		t.Error("missing UTF-8 BOM")
	}
	lines := strings.Split(strings.TrimSpace(string(b[3:])), "\n")
	if len(lines) != 2 { // header + 1 行
		t.Errorf("csv lines = %d", len(lines))
	}
	if !strings.Contains(lines[0], "sha256") || !strings.Contains(lines[1], "a.txt") {
		t.Errorf("csv content unexpected: %q", string(b))
	}
}
