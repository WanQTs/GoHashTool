// Package checksum 提供哈希清单（md5sum/sha256sum 标准格式）的解析、
// 算法自动识别与对比逻辑，纯 Go 实现，不依赖 Wails。
package checksum

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gohash/internal/hashcore"
)

// Entry 清单条目。
type Entry struct {
	Hash string `json:"hash"` // 原始十六进制字符串（比较时忽略大小写）
	Path string `json:"path"` // 清单中的文件名（可能是相对路径）
	Line int    `json:"line"` // 在清单文件中的行号（1 起）
}

// Error 结构化解析错误，Code 供前端做双语映射，Line 指出问题行号。
type Error struct {
	Code string // bad_line / bad_hash_length / mixed_length / empty_manifest
	Line int
	Msg  string
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s (line %d): %s", e.Code, e.Line, e.Msg)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Msg)
}

// ParseManifest 解析 md5sum/sha256sum 标准输出格式，兼容：
//   - "*文件名" 二进制标记与两个空格的文本模式
//   - 文件名含换行/反斜杠时 GNU 工具生成的行首 "\" 转义行
//   - 文件名含空格
//   - UTF-8 BOM、Windows/Unix 换行符混用
//   - 以 "#" 开头的注释行（部分项目发布的清单含说明文字，直接跳过；
//     合法条目行首必为十六进制哈希，不可能与注释行混淆）
func ParseManifest(data []byte) ([]Entry, error) {
	// UTF-8 BOM
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	var entries []Entry
	for i, raw := range strings.Split(text, "\n") {
		lineNo := i + 1
		line := strings.TrimRight(raw, " \t")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// 行首 "\" 转义标记：文件名中含特殊字符，需反转义。
		escaped := strings.HasPrefix(line, "\\")
		if escaped {
			line = line[1:]
		}
		// 格式：<hex-hash><空白>[*]<文件名>；文件名可含空格，按第一个空白切分。
		idx := strings.IndexAny(line, " \t")
		if idx <= 0 {
			return nil, &Error{Code: "bad_line", Line: lineNo, Msg: "missing hash/filename separator"}
		}
		hashStr := line[:idx]
		name := strings.TrimLeft(line[idx:], " \t")
		if strings.HasPrefix(name, "*") { // 二进制模式标记
			name = name[1:]
		}
		if name == "" {
			return nil, &Error{Code: "bad_line", Line: lineNo, Msg: "empty filename"}
		}
		if !isHex(hashStr) {
			return nil, &Error{Code: "bad_line", Line: lineNo, Msg: "hash is not hex: " + hashStr}
		}
		if _, err := hashcore.DetectByLength(len(hashStr)); err != nil {
			return nil, &Error{Code: "bad_hash_length", Line: lineNo,
				Msg: fmt.Sprintf("unsupported hash length %d", len(hashStr))}
		}
		if escaped {
			name = unescapeName(name)
		}
		entries = append(entries, Entry{Hash: hashStr, Path: name, Line: lineNo})
	}
	if len(entries) == 0 {
		return nil, &Error{Code: "empty_manifest", Msg: "no valid entries found"}
	}
	return entries, nil
}

// DetectAlgorithm 识别清单算法：已知扩展名（.md5/.sha1/.sha256/.sha512）与
// 条目哈希长度交叉校验——长度明显属于另一种算法时报 ext_algo_mismatch 并给出行号
// （多半是文件被改错后缀，显式报错比算完全部不一致更友好）；扩展名缺失或
// 不识别时按哈希长度推断，长度混杂时报 mixed_length。
func DetectAlgorithm(manifestName string, entries []Entry) (hashcore.Algorithm, error) {
	if extAlgo := algoByExt(manifestName); extAlgo != "" {
		wantLen := hexLen(extAlgo)
		for _, e := range entries {
			if len(e.Hash) != wantLen {
				return "", &Error{Code: "ext_algo_mismatch", Line: e.Line,
					Msg: fmt.Sprintf("hash length %d does not match %s (%d) implied by extension",
						len(e.Hash), extAlgo, wantLen)}
			}
		}
		return extAlgo, nil
	}
	first := len(entries[0].Hash)
	for _, e := range entries[1:] {
		if len(e.Hash) != first {
			return "", &Error{Code: "mixed_length", Line: e.Line,
				Msg: fmt.Sprintf("hash length %d differs from line %d (%d)", len(e.Hash), entries[0].Line, first)}
		}
	}
	return hashcore.DetectByLength(first)
}

// algoByExt 按清单文件名扩展名推断算法，不识别时返回空串。
func algoByExt(manifestName string) hashcore.Algorithm {
	switch strings.ToLower(filepath.Ext(manifestName)) {
	case ".md5":
		return hashcore.MD5
	case ".sha1":
		return hashcore.SHA1
	case ".sha256":
		return hashcore.SHA256
	case ".sha512":
		return hashcore.SHA512
	}
	return ""
}

// hexLen 返回算法十六进制摘要的字符数。
func hexLen(a hashcore.Algorithm) int {
	switch a {
	case hashcore.MD5:
		return 32
	case hashcore.SHA1:
		return 40
	case hashcore.SHA256:
		return 64
	case hashcore.SHA512:
		return 128
	}
	return 0
}

// ResolveTargets 将清单条目解析为待计算目标：相对路径按 baseDir 解析为绝对路径，
// 已删除或指向目录的条目归入 missing（Path 已改写为解析后的完整路径）。
// 同一路径重复出现时仅保留首条（经 CanonicalKey 规范化，Windows 下大小写折叠），
// 避免同一文件被重复计算、期望值互相覆盖。
func ResolveTargets(entries []Entry, baseDir string) (toHash []hashcore.FileItem, missing []Entry, expected map[string]string) {
	expected = make(map[string]string, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		p := e.Path
		if !filepath.IsAbs(p) {
			p = filepath.Join(baseDir, filepath.FromSlash(p))
		}
		if _, dup := seen[hashcore.CanonicalKey(p)]; dup {
			continue
		}
		seen[hashcore.CanonicalKey(p)] = struct{}{}
		info, statErr := os.Stat(p)
		if statErr != nil || info.IsDir() {
			missing = append(missing, Entry{Hash: e.Hash, Path: p, Line: e.Line})
			continue
		}
		expected[p] = e.Hash
		toHash = append(toHash, hashcore.FileItem{Path: p, Size: info.Size()})
	}
	return toHash, missing, expected
}

// EqualHash 哈希比较忽略大小写。
func EqualHash(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// IsManifestName 按扩展名判断是否为可导入的清单文件。
func IsManifestName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md5", ".sha1", ".sha256", ".sha512", ".txt", ".sum", ".sums":
		return true
	}
	return false
}

func isHex(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// unescapeName 反转义 GNU 风格文件名：\\ → \，\n → 换行。
func unescapeName(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case '\\':
				b.WriteByte('\\')
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// escapeName 与 unescapeName 互逆，用于导出。
func escapeName(s string) (string, bool) {
	need := strings.ContainsAny(s, "\\\n")
	if !need {
		return s, false
	}
	r := strings.NewReplacer("\\", "\\\\", "\n", "\\n")
	return r.Replace(s), true
}
