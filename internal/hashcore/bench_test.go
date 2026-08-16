package hashcore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// TestMain 在全部基准结束后清理夹具目录（1GB 大文件 + 1 万个小文件）。
func TestMain(m *testing.M) {
	code := m.Run()
	if benchDir != "" {
		_ = os.RemoveAll(benchDir)
	}
	os.Exit(code)
}

// 基准测试夹具：1GB 大文件与 1 万个 1KB 小文件，跨 benchmark 复用（只建一次）。
var (
	benchOnce sync.Once
	benchDir  string
	bigFile   string
	smallSet  []FileItem
)

func setupBenchFixtures(b *testing.B) {
	benchOnce.Do(func() {
		var err error
		benchDir, err = os.MkdirTemp("", "hashcore-bench")
		if err != nil {
			panic(err)
		}
		// 1GB 大文件：16MB 缓冲重复写 64 次。
		bigFile = filepath.Join(benchDir, "big-1gb.bin")
		f, err := os.Create(bigFile)
		if err != nil {
			panic(err)
		}
		block := make([]byte, 16<<20)
		for i := range block {
			block[i] = byte(i)
		}
		for w := 0; w < 64; w++ {
			if _, err := f.Write(block); err != nil {
				panic(err)
			}
		}
		f.Close()

		// 1 万个 1KB 小文件。
		smallDir := filepath.Join(benchDir, "small")
		if err := os.MkdirAll(smallDir, 0o755); err != nil {
			panic(err)
		}
		data := make([]byte, 1024)
		for i := 0; i < 10000; i++ {
			p := filepath.Join(smallDir, fmt.Sprintf("f%05d.bin", i))
			if err := os.WriteFile(p, data, 0o644); err != nil {
				panic(err)
			}
			smallSet = append(smallSet, FileItem{Path: p, Size: 1024})
		}
	})
}

func runBench(b *testing.B, items []FileItem, algos []Algorithm) {
	b.Helper()
	b.ResetTimer()
	var done atomic.Int64
	for i := 0; i < b.N; i++ {
		done.Store(0)
		HashFiles(context.Background(), items, algos, nil, func(Result) {}, &done)
	}
	b.StopTimer()
	var total int64
	for _, it := range items {
		total += it.Size
	}
	b.ReportMetric(float64(total)/1e6*float64(b.N), "MB_total")
}

// BenchmarkSHA256LargeFile 对应性能基准：SHA-256 单文件 1GB ≤ 3 秒（NVMe）。
func BenchmarkSHA256LargeFile(b *testing.B) {
	setupBenchFixtures(b)
	runBench(b, []FileItem{{Path: bigFile, Size: 1 << 30}}, []Algorithm{SHA256})
}

// BenchmarkSHA256MD5LargeFile 对应性能基准：SHA-256 + MD5 双算法同扫 1GB ≤ 3.5 秒。
func BenchmarkSHA256MD5LargeFile(b *testing.B) {
	setupBenchFixtures(b)
	runBench(b, []FileItem{{Path: bigFile, Size: 1 << 30}}, []Algorithm{SHA256, MD5})
}

// BenchmarkManySmallFiles 对应性能基准：1 万个 1KB 小文件批量计算 ≤ 10 秒。
func BenchmarkManySmallFiles(b *testing.B) {
	setupBenchFixtures(b)
	runBench(b, smallSet, []Algorithm{SHA256})
}
