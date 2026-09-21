package main

// CLI 命令行模式：语法对标 GNU md5sum/sha256sum（兼顾 certutil 的算法位习惯），
// 与 GUI 共用 hashcore/checksum 引擎，计算/校验结果与界面口径一致。
//
//	gohash                          无参数 → GUI（main.go 分流，不经此处）
//	gohash [选项] 文件/目录...       计算哈希（目录递归；通配符自行展开；默认 sha256）
//	gohash -c 清单 [清单...]         校验模式（md5sum -c 兼容）
//
// 输出契约：哈希/校验行走 stdout（单算法严格 md5sum 格式，可回导 -c）；
// 诊断与汇总警告走 stderr（GNU 措辞）；文案固定英文，脚本可稳定匹配。
// 退出码：0 全部成功 / 1 有文件失败或不一致 / 2 用法或清单错误。

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"gohash/internal/checksum"
	"gohash/internal/hashcore"
)

// cliOptions 一次 CLI 调用的解析结果。
type cliOptions struct {
	algos  []hashcore.Algorithm // 空 = 默认 sha256（哈希模式）
	check  bool                 // -c/--check：校验模式（布尔开关，清单走位置参数，与 GNU 一致）
	quiet  bool                 // -q/--quiet：校验时不打印 OK 行
	status bool                 // --status：校验时完全静默，仅退出码
	help   bool                 // -h/--help
	paths  []string             // 位置参数（哈希模式为文件/目录；校验模式为清单文件）
}

// tryCLI 判断本次调用是否走 CLI（在 application.New 之前分流，
// CLI 路径完全不触碰 Wails/单实例）。handled=true 时进程应以 code 退出。
func tryCLI(args []string) (handled bool, code int) {
	if len(args) == 0 {
		return false, 0 // 无参数 → GUI
	}
	opts, hasFlags, err := parseCLIArgs(args)
	if err != nil {
		// 用法错误一律按 CLI 报错（对标 GNU），不开 GUI。
		_, stderr := setupCLIOutput()
		fmt.Fprintf(stderr, "gohash: %v\n", err)
		fmt.Fprintln(stderr, "Try 'gohash --help' for more information.")
		return true, 2
	}
	if !routeCLI(opts, hasFlags) {
		return false, 0
	}
	return true, runCLI(opts)
}

// routeCLI 决定解析后的参数走 CLI 还是 GUI：
// 有任意选项 → CLI；无选项且恰好一个已存在的清单文件 → GUI
// （双击清单/「打开方式」的现状路径，二实例转发逻辑不变）；
// 无位置参数 → GUI；其余（普通文件/目录/多个参数）→ CLI 哈希模式。
func routeCLI(opts cliOptions, hasFlags bool) bool {
	if hasFlags {
		return true
	}
	if len(opts.paths) == 1 && isGUIArg(opts.paths[0]) {
		return false
	}
	return len(opts.paths) > 0
}

// isGUIArg 报告该参数是否「打开方式」场景（已存在的清单文件）。
func isGUIArg(path string) bool {
	if !isOpenWithManifest(path) {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func runCLI(opts cliOptions) int {
	stdout, stderr := setupCLIOutput()
	switch {
	case opts.help:
		printHelp(stdout)
		return 0
	case opts.check:
		return runCLICheck(opts, stdout, stderr)
	default:
		return runCLIHash(opts, stdout, stderr)
	}
}

// parseCLIArgs 解析命令行参数（不含程序名）。手写解析而非 stdlib flag，
// 以支持 GNU 习惯：长短选项、-a=v / --algorithm=v、选项与位置参数混排、-- 终止。
// hasFlags 表示是否出现过任何选项，供 GUI/CLI 分流。
func parseCLIArgs(args []string) (opts cliOptions, hasFlags bool, err error) {
	seenAlgo := map[hashcore.Algorithm]bool{}
	addAlgos := func(v string) error {
		for _, s := range strings.Split(v, ",") {
			a, perr := hashcore.ParseAlgorithm(s)
			if perr != nil {
				return fmt.Errorf("unknown algorithm: %q", strings.TrimSpace(s))
			}
			if !seenAlgo[a] { // 去重保序：重复指定同一算法不产生重复行
				seenAlgo[a] = true
				opts.algos = append(opts.algos, a)
			}
		}
		return nil
	}

	noMoreOpts := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if noMoreOpts || arg == "" || arg[0] != '-' || arg == "-" {
			opts.paths = append(opts.paths, arg)
			continue
		}
		if arg == "--" {
			noMoreOpts = true
			continue
		}
		hasFlags = true
		var name, inlineVal string
		var hasInline bool
		if strings.HasPrefix(arg, "--") {
			body := arg[2:]
			if k := strings.IndexByte(body, '='); k >= 0 {
				name, inlineVal, hasInline = body[:k], body[k+1:], true
			} else {
				name = body
			}
		} else {
			body := arg[1:]
			name = body[:1]
			if len(body) > 1 { // 不支持短选项合写/连写值（-amd5），只接受 -a=md5
				if body[1] == '=' {
					inlineVal, hasInline = body[2:], true
				} else {
					return opts, hasFlags, fmt.Errorf("invalid option -- '%s'", body)
				}
			}
		}
		needVal := func() (string, error) {
			if hasInline {
				return inlineVal, nil
			}
			if i+1 < len(args) {
				i++
				return args[i], nil
			}
			return "", fmt.Errorf("option requires an argument -- '%s'", name)
		}
		switch name {
		case "a", "algorithm":
			v, verr := needVal()
			if verr != nil {
				return opts, hasFlags, verr
			}
			if aerr := addAlgos(v); aerr != nil {
				return opts, hasFlags, aerr
			}
		case "c", "check":
			opts.check = true
		case "q", "quiet":
			opts.quiet = true
		case "status":
			opts.status = true
		case "h", "help":
			opts.help = true
		default:
			if len(name) > 1 {
				return opts, hasFlags, fmt.Errorf("unrecognized option '--%s'", name)
			}
			return opts, hasFlags, fmt.Errorf("invalid option -- '%s'", name)
		}
	}
	// 校验模式的算法由清单识别决定，显式 -a 只会造成歧义，直接拒绝。
	if opts.check && len(opts.algos) > 0 {
		return opts, hasFlags, fmt.Errorf("option --algorithm cannot be used with --check")
	}
	return opts, hasFlags, nil
}

// expandGlobs 展开参数中的通配符（Windows shell 不做展开，对标 GNU 工具
// Windows 移植版的内部展开）；无匹配时保留原样，由计算层报告 not_found。
func expandGlobs(paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if !strings.ContainsAny(p, "*?[") {
			out = append(out, p)
			continue
		}
		m, err := filepath.Glob(p)
		if err != nil { // ErrBadPattern
			return nil, err
		}
		if len(m) == 0 {
			out = append(out, p)
			continue
		}
		out = append(out, m...)
	}
	return out, nil
}

// runCLIHash 哈希模式：计算并输出每个文件的哈希行，按输入顺序（GNU 行为）。
// 单算法输出严格 md5sum 格式（复用 checksum.WriteSUM，含 GNU 行首转义），
// 可被 gohash -c 与 GNU sha256sum 直接复用；多算法每行带算法名前缀（人类可读，
// 不保证可回导）。任一文件失败 → 退出码 1。
func runCLIHash(opts cliOptions, stdout, stderr io.Writer) int {
	algos := opts.algos
	if len(algos) == 0 {
		algos = []hashcore.Algorithm{hashcore.SHA256}
	}
	paths, err := expandGlobs(opts.paths)
	if err != nil {
		fmt.Fprintf(stderr, "gohash: %v\n", err)
		return 2
	}
	if len(paths) == 0 {
		fmt.Fprintln(stderr, "gohash: no input files")
		fmt.Fprintln(stderr, "Try 'gohash --help' for more information.")
		return 2
	}

	// CLI 生命周期即进程生命周期（Ctrl+C 直接终止进程），无需取消链路；
	// ExpandPathsDetailedContext 仅在取消时返回错误，此处忽略。
	items, skippedDirs, _ := hashcore.ExpandPathsDetailedContext(context.Background(), paths, nil)
	failed := false
	for _, d := range skippedDirs {
		fmt.Fprintf(stderr, "gohash: %s: cannot read directory\n", d)
		failed = true
	}
	if len(items) == 0 {
		if failed {
			return 1
		}
		fmt.Fprintln(stderr, "gohash: no input files")
		fmt.Fprintln(stderr, "Try 'gohash --help' for more information.")
		return 2
	}

	// onItem 在多 worker goroutine 并发触发，收集结果必须持锁。
	var mu sync.Mutex
	results := make(map[string]hashcore.Result, len(items))
	var bytesDone atomic.Int64
	hashcore.HashFiles(context.Background(), items, algos, nil, func(r hashcore.Result) {
		mu.Lock()
		results[r.Path] = r
		mu.Unlock()
	}, &bytesDone)

	var sumItems []checksum.ExportItem
	single := len(algos) == 1
	for _, it := range items {
		r := results[it.Path]
		if r.Status != hashcore.StatusOK {
			fmt.Fprintf(stderr, "gohash: %s: %s\n", it.Path, statusMessage(r.Status, r.Err))
			failed = true
			continue
		}
		if single {
			sumItems = append(sumItems, checksum.ExportItem{
				Path:   it.Path,
				Hashes: map[string]string{string(algos[0]): r.Hashes[algos[0]]},
				Status: string(hashcore.StatusOK),
			})
			continue
		}
		for _, a := range algos {
			fmt.Fprintf(stdout, "%-6s %s  %s\n", strings.ToUpper(string(a)), r.Hashes[a], it.Path)
		}
	}
	if single {
		_ = checksum.WriteSUM(stdout, sumItems, algos[0])
	}
	if failed {
		return 1
	}
	return 0
}

// runCLICheck 校验模式（md5sum -c 兼容）：位置参数即清单文件，可多个，
// 逐个校验；返回码取最差（0 全部一致 / 1 有不一致或不可读 / 2 清单读取或解析失败）。
func runCLICheck(opts cliOptions, stdout, stderr io.Writer) int {
	if len(opts.paths) == 0 { // GNU 此时读 stdin，本工具不读（GUI 程序无 stdin 惯例）
		fmt.Fprintln(stderr, "gohash: no input files")
		fmt.Fprintln(stderr, "Try 'gohash --help' for more information.")
		return 2
	}
	worst := 0
	for _, m := range opts.paths {
		if code := checkOneManifest(m, opts, stdout, stderr); code > worst {
			worst = code
		}
	}
	return worst
}

// checkOneManifest 校验单个清单：相对路径按 GNU 语义相对当前工作目录解析；
// 逐行输出 <name>: OK / FAILED / FAILED open or read（name 按清单所列原样回显），
// 汇总警告走 stderr；--quiet 抑制 OK 行，--status 抑制全部行与警告。
func checkOneManifest(path string, opts cliOptions, stdout, stderr io.Writer) int {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "gohash: %s: %s\n", path, openErrorText(err))
		return 2
	}
	entries, err := checksum.ParseManifest(data)
	if err != nil {
		fmt.Fprintf(stderr, "gohash: %s: %s\n", path, err)
		return 2
	}
	algo, err := checksum.DetectAlgorithm(path, entries)
	if err != nil {
		fmt.Fprintf(stderr, "gohash: %s: %s\n", path, err)
		return 2
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "gohash: %v\n", err)
		return 2
	}

	toHash, missing, _ := checksum.ResolveTargets(entries, cwd)
	missingSet := make(map[string]bool, len(missing))
	for _, e := range missing {
		missingSet[hashcore.CanonicalKey(e.Path)] = true
	}
	var mu sync.Mutex
	results := make(map[string]hashcore.Result, len(toHash))
	var bytesDone atomic.Int64
	hashcore.HashFiles(context.Background(), toHash, []hashcore.Algorithm{algo}, nil, func(r hashcore.Result) {
		mu.Lock()
		results[r.Path] = r
		mu.Unlock()
	}, &bytesDone)

	var mismatch, unreadable int
	for _, e := range entries {
		p := e.Path
		if !filepath.IsAbs(p) {
			p = filepath.Join(cwd, filepath.FromSlash(p))
		}
		if missingSet[hashcore.CanonicalKey(p)] {
			unreadable++
			if !opts.status {
				fmt.Fprintf(stderr, "gohash: %s: No such file or directory\n", e.Path)
				fmt.Fprintf(stdout, "%s: FAILED open or read\n", e.Path)
			}
			continue
		}
		r, ok := results[p]
		if !ok {
			// 不可达：ResolveTargets 已将缺失项分流，其余必在结果集中。防御性计数。
			unreadable++
			continue
		}
		switch verdictFor(r.Status, e.Hash, r.Hashes[algo]) {
		case "pass":
			if !opts.quiet && !opts.status {
				fmt.Fprintf(stdout, "%s: OK\n", e.Path)
			}
		case "fail":
			mismatch++
			if !opts.status {
				fmt.Fprintf(stdout, "%s: FAILED\n", e.Path)
			}
		default: // missing/error（占用/无权限等「存在但读不了」）
			unreadable++
			if !opts.status {
				fmt.Fprintf(stderr, "gohash: %s: %s\n", e.Path, statusMessage(r.Status, r.Err))
				fmt.Fprintf(stdout, "%s: FAILED open or read\n", e.Path)
			}
		}
	}
	if !opts.status {
		if mismatch > 0 {
			fmt.Fprintf(stderr, "gohash: WARNING: %d computed checksum%s did NOT match\n", mismatch, pluralS(mismatch))
		}
		if unreadable > 0 {
			fmt.Fprintf(stderr, "gohash: WARNING: %d listed file%s could not be read\n", unreadable, pluralS(unreadable))
		}
	}
	if mismatch+unreadable > 0 {
		return 1
	}
	return 0
}

// statusMessage 把计算状态映射为 GNU 风格的 errno 措辞（stderr 诊断用）。
func statusMessage(st hashcore.Status, err error) string {
	switch st {
	case hashcore.StatusNotFound:
		return "No such file or directory"
	case hashcore.StatusNoPermission:
		return "Permission denied"
	case hashcore.StatusOccupied:
		return "Device or resource busy"
	}
	if err != nil {
		return err.Error()
	}
	return "I/O error"
}

// openErrorText 提取打开文件失败的简短措辞（GNU 风格 errno 文本）。
func openErrorText(err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "No such file or directory"
	case errors.Is(err, fs.ErrPermission):
		return "Permission denied"
	}
	return err.Error()
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// printHelp 中英双语简版用法。
func printHelp(w io.Writer) {
	fmt.Fprint(w, `GoHashTool — 文件哈希工具 File Hash Tool

用法 Usage:
  gohash                          启动图形界面 Launch the GUI
  gohash [选项] 文件/目录...      计算哈希 Hash files (directories recurse)
  gohash -c 清单...               按清单校验 Verify against manifest(s) (md5sum -c compatible)

选项 Options:
  -a, --algorithm ALGO   md5 | sha1 | sha256 | sha512 | crc32
                         可重复或逗号分隔；默认 sha256
                         repeatable / comma-separated (default: sha256)
  -c, --check            校验模式（清单走位置参数，可多个）；相对路径相对当前目录解析
                         check mode; manifest(s) as arguments; relative paths
                         resolve against the current directory
  -q, --quiet            校验时不打印 OK 行 don't print OK lines (with -c)
      --status           校验时不输出任何内容，仅以退出码报告
                         print nothing, report via exit code only (with -c)
  -h, --help             显示本帮助 show this help
      --selftest         无界面自检 self-test without GUI

退出码 Exit codes: 0 = 全部成功 all OK, 1 = 有文件失败/不一致 failures, 2 = 用法或清单错误 usage/manifest error

示例 Examples:
  gohash setup.iso
  gohash -a md5 -a sha256 big.iso
  gohash -a sha1,md5 src\ *.dll
  gohash -c list.sha256
  gohash -c --status list.md5 || echo FAILED
`)
}
