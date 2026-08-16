package hashcore

import (
	"context"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// 性能取舍（对应需求逐条说明）：
//
//  1. 流式读取 + 自适应缓冲：任何文件都不整体读入内存。
//     小文件（<64MB）用 1MB 缓冲、大文件用 16MB 缓冲，缓冲全部来自
//     sync.Pool 复用，长跑过程无大对象分配，RSS 占用与文件体积无关。
//
//  2. IO 与计算重叠：大文件走双缓冲流水线，prefetch goroutine 预读下一分块，
//     主 goroutine 同时计算当前分块哈希，NVMe 下磁盘带宽被打满而非交替等待。
//
// 3. 多算法一次扫描：io.MultiWriter 组合多个 hash.Hash，文件只读一遍。
//
//  4. 多文件并行：小文件级别并发，worker 数 = min(NumCPU, 8)。
//     依据：小文件（如 1KB）单文件 IO 往返开销占比极高，并发把 CPU 喂满
//     收益最大；大文件瓶颈在磁盘带宽与 SHA 计算本身，过度并发反而引发
//     磁头/队列竞争，因此大文件在独立通道上逐个流水线处理。
const (
	smallFileThreshold = 64 << 20 // 64MB：大小文件分界
	smallBufSize       = 1 << 20  // 1MB：小文件缓冲
	largeBufSize       = 16 << 20 // 16MB：大文件缓冲
	maxWorkers         = 8
)

// 缓冲池存 *[]byte 而非 []byte，避免 sync.Pool 接口装箱产生额外分配。
var (
	smallBufPool = sync.Pool{New: func() any { b := make([]byte, smallBufSize); return &b }}
	largeBufPool = sync.Pool{New: func() any { b := make([]byte, largeBufSize); return &b }}
)

// Status 单文件计算状态。
type Status string

const (
	StatusOK           Status = "ok"
	StatusCanceled     Status = "canceled"
	StatusNotFound     Status = "not_found"
	StatusNoPermission Status = "no_permission"
	StatusOccupied     Status = "occupied"
	StatusError        Status = "error"
)

// FileItem 待计算文件。Size < 0 表示文件已不存在（用于在结果中标记缺失）。
type FileItem struct {
	Path string
	Size int64
}

// Result 单文件计算结果。
type Result struct {
	Path     string
	Size     int64
	Hashes   map[Algorithm]string
	Duration time.Duration
	Status   Status
	Err      error
}

// ExpandPaths 将文件/目录混合列表展开为文件清单；目录递归遍历，
// 跳过非常规文件（符号链接、设备等）。已不存在的文件以 Size=-1 保留，
// 交由计算层标记为缺失，保证「不中断整批任务」。
// 按规范化路径去重：同一文件经重叠的文件夹/文件选择只会出现一次。
// 被跳过的不可读目录见 ExpandPathsDetailed。
func ExpandPaths(paths []string) []FileItem {
	items, _ := ExpandPathsDetailed(paths)
	return items
}

// ExpandPathsDetailed 是 ExpandPaths 的完整版：额外返回遍历中因不可读而
// 被跳过的目录，供调用方生成可见的错误行（禁止静默吞错：子树未计入必须让用户知道）。
func ExpandPathsDetailed(paths []string) (items []FileItem, skippedDirs []string) {
	seen := make(map[string]struct{})
	add := func(p string, size int64) {
		key := CanonicalKey(p)
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		items = append(items, FileItem{Path: p, Size: size})
	}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			add(p, -1)
			continue
		}
		if !info.IsDir() {
			if info.Mode().IsRegular() {
				add(p, info.Size())
			}
			continue
		}
		// 目录递归：遍历出错时目录读不动才跳过其子树；文件级错误
		// （如遍历过程中被删除）只跳过该文件——SkipDir 作用于文件会
		// 连带跳过同目录的其余文件，造成静默漏算。
		_ = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				action := walkErrAction(d, err)
				if action == filepath.SkipDir { // 不可读子树：记录下来交由上层展示
					skippedDirs = append(skippedDirs, path)
				}
				return action
			}
			if d.IsDir() {
				return nil
			}
			fi, err := d.Info()
			if err != nil || !fi.Mode().IsRegular() {
				return nil
			}
			add(path, fi.Size())
			return nil
		})
	}
	return items, skippedDirs
}

// walkErrAction 决定 WalkDir 遇到错误时如何继续（提取为独立函数以便单测）：
// 目录不可读返回 SkipDir 跳过该子树；文件级错误返回 nil 仅跳过该文件。
func walkErrAction(d fs.DirEntry, err error) error {
	if err == nil {
		return nil
	}
	if d != nil && d.IsDir() {
		return filepath.SkipDir
	}
	return nil
}

// CanonicalKey 路径去重键：Clean 规范化；Windows 文件系统大小写不敏感，做大小写折叠。
// 供 ExpandPaths 与 checksum.ResolveTargets 共用，保证两处去重口径一致。
func CanonicalKey(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}

// TotalSize 汇总清单字节数（缺失文件不计）。
func TotalSize(items []FileItem) int64 {
	var total int64
	for _, it := range items {
		if it.Size > 0 {
			total += it.Size
		}
	}
	return total
}

// HashFiles 对清单内全部文件计算哈希，结果通过 onItem 逐个回调
// （回调在不同 worker goroutine 中触发，调用方需自行保证同步）。
// onStart 在开始处理某文件前回调（可为 nil），供上层展示「当前文件」。
// bytesDone 为累计已处理字节数的原子计数器，供上层做节流进度上报。
// ctx 取消后正在读的分块会尽快退出（最坏情况为一个分块的读取时长），
// 未开始的文件直接跳过。
func HashFiles(ctx context.Context, items []FileItem, algos []Algorithm, onStart func(string), onItem func(Result), bytesDone *atomic.Int64) {
	var small, large []FileItem
	for _, it := range items {
		if it.Size >= smallFileThreshold {
			large = append(large, it)
		} else {
			small = append(small, it)
		}
	}

	var wg sync.WaitGroup

	// 大文件通道：单 worker + 双缓冲流水线，瓶颈是磁盘带宽，串行即可打满。
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, it := range large {
			if ctx.Err() != nil {
				return
			}
			if onStart != nil {
				onStart(it.Path)
			}
			onItem(hashFilePipelined(ctx, it, algos, bytesDone))
		}
	}()

	// 小文件通道：worker pool 并发吃满 CPU。
	workers := runtime.NumCPU()
	if workers > maxWorkers {
		workers = maxWorkers
	}
	if workers < 1 {
		workers = 1
	}
	// 带缓冲的派发通道：喂入 goroutine 可提前跑一段，
	// 避免十万级小文件场景下每次派发都等待 worker 交接。
	smallCh := make(chan FileItem, workers*4)
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for it := range smallCh {
				if ctx.Err() != nil {
					continue // 排空通道后直接退出
				}
				if onStart != nil {
					onStart(it.Path)
				}
				onItem(hashFileStreaming(ctx, it, algos, bytesDone))
			}
		}()
	}

feed:
	for _, it := range small {
		select {
		case smallCh <- it:
		case <-ctx.Done():
			break feed
		}
	}
	close(smallCh)
	wg.Wait()
}

// multiHasher 用 io.MultiWriter 组合多个 hash.Hash，实现多算法一次扫描。
func multiHasher(algos []Algorithm) (io.Writer, map[Algorithm]hash.Hash) {
	hs := make(map[Algorithm]hash.Hash, len(algos))
	ws := make([]io.Writer, 0, len(algos))
	for _, a := range algos {
		h := a.New()
		hs[a] = h
		ws = append(ws, h)
	}
	return io.MultiWriter(ws...), hs
}

func sumHashes(hs map[Algorithm]hash.Hash) map[Algorithm]string {
	out := make(map[Algorithm]string, len(hs))
	for a, h := range hs {
		out[a] = hex.EncodeToString(h.Sum(nil))
	}
	return out
}

// hashFileStreaming 小文件顺序流式计算（1MB 池化缓冲）。
func hashFileStreaming(ctx context.Context, it FileItem, algos []Algorithm, bytesDone *atomic.Int64) Result {
	start := time.Now()
	res := Result{Path: it.Path, Size: it.Size, Status: StatusOK}
	if it.Size < 0 {
		res.Status = StatusNotFound
		return res
	}
	f, err := os.Open(it.Path)
	if err != nil {
		res.Status = classifyError(err)
		res.Err = err
		return res
	}
	defer f.Close()

	w, hs := multiHasher(algos)
	bp := smallBufPool.Get().(*[]byte)
	defer smallBufPool.Put(bp)
	buf := *bp

	for {
		if err := ctx.Err(); err != nil {
			res.Status = StatusCanceled
			res.Err = err
			return res
		}
		n, rerr := f.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n]) // hash.Hash 的 Write 永不返回错误
			bytesDone.Add(int64(n))
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			res.Status = classifyError(rerr)
			res.Err = rerr
			return res
		}
	}
	res.Hashes = sumHashes(hs)
	res.Duration = time.Since(start)
	return res
}

// hashFilePipelined 大文件双缓冲流水线：prefetch goroutine 预读下一分块，
// 当前 goroutine 同时计算上一分块哈希，IO 与计算重叠。
func hashFilePipelined(ctx context.Context, it FileItem, algos []Algorithm, bytesDone *atomic.Int64) Result {
	start := time.Now()
	res := Result{Path: it.Path, Size: it.Size, Status: StatusOK}
	if it.Size < 0 {
		res.Status = StatusNotFound
		return res
	}
	f, err := os.Open(it.Path)
	if err != nil {
		res.Status = classifyError(err)
		res.Err = err
		return res
	}
	defer f.Close()

	w, hs := multiHasher(algos)

	type chunk struct {
		bp  *[]byte
		n   int
		err error
	}
	// 容量 1：预读完成的分块等待被消费的同时，读 goroutine 可再读一块，
	// 配合双缓冲池实现「读下一块」与「算这一块」并行。
	ch := make(chan chunk, 1)
	readCtx, stopReader := context.WithCancel(ctx)
	defer stopReader() // 主循环提前退出时解除读 goroutine 的发送阻塞

	go func() {
		defer close(ch)
		for {
			bp := largeBufPool.Get().(*[]byte)
			n, err := f.Read(*bp)
			if n <= 0 {
				largeBufPool.Put(bp)
				if err != nil && err != io.EOF {
					select {
					case ch <- chunk{err: err}:
					case <-readCtx.Done():
					}
				}
				return
			}
			c := chunk{bp: bp, n: n}
			if err != nil && err != io.EOF {
				c.err = err
			}
			select {
			case ch <- c:
			case <-readCtx.Done():
				largeBufPool.Put(bp)
				return
			}
			if err != nil { // 数据已送达（含 EOF 尾块），读取结束
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			res.Status = StatusCanceled
			res.Err = ctx.Err()
			return res
		case c, ok := <-ch:
			if !ok {
				res.Hashes = sumHashes(hs)
				res.Duration = time.Since(start)
				return res
			}
			if c.err != nil {
				// 读 goroutine 的纯错误分块不带缓冲（bp 为 nil）：
				// nil 指针回池会让后续 Get 到它的计算直接 nil 解引用 panic。
				if c.bp != nil {
					largeBufPool.Put(c.bp)
				}
				res.Status = classifyError(c.err)
				res.Err = c.err
				return res
			}
			_, _ = w.Write((*c.bp)[:c.n])
			bytesDone.Add(int64(c.n))
			largeBufPool.Put(c.bp)
		}
	}
}

// classifyError 将 IO 错误归类为状态码；文件被占用/无权限/已删除
// 单独标记，不中断整批任务。
func classifyError(err error) Status {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return StatusNotFound
	case errors.Is(err, fs.ErrPermission):
		return StatusNoPermission
	case errors.Is(err, context.Canceled):
		return StatusCanceled
	}
	// Windows 特有：ERROR_SHARING_VIOLATION(32)=文件被占用，
	// ERROR_LOCK_VIOLATION(33)=区域被锁定。用数值避免平台专属常量。
	var errno syscall.Errno
	if errors.As(err, &errno) {
		if errno == 32 || errno == 33 {
			return StatusOccupied
		}
	}
	return StatusError
}
