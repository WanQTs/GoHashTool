// Package hashcore 提供流式文件哈希计算核心。
// 纯 Go 实现，不依赖 Wails，可独立测试与基准测量。
package hashcore

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
	"hash/crc32"
	"strings"
)

// Algorithm 哈希算法标识。
type Algorithm string

const (
	MD5    Algorithm = "md5"
	SHA1   Algorithm = "sha1"
	SHA256 Algorithm = "sha256"
	SHA512 Algorithm = "sha512"
	CRC32  Algorithm = "crc32"
)

// All 返回全部支持的算法（固定顺序，供界面 chip 展示）。
func All() []Algorithm {
	return []Algorithm{MD5, SHA1, SHA256, SHA512, CRC32}
}

// ParseAlgorithm 将用户输入（不区分大小写，允许连字符）解析为算法。
func ParseAlgorithm(s string) (Algorithm, error) {
	norm := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), "-", ""))
	switch norm {
	case "md5":
		return MD5, nil
	case "sha1":
		return SHA1, nil
	case "sha256":
		return SHA256, nil
	case "sha512":
		return SHA512, nil
	case "crc32":
		return CRC32, nil
	}
	return "", fmt.Errorf("unsupported algorithm: %q", s)
}

// New 创建对应算法的 hash.Hash。
// 性能说明：crypto/sha256、crypto/sha512 在 amd64 上由 Go 标准库自带的
// SHA-NI / AVX2 汇编路径加速，只要构建时不加 purego 等禁用汇编的 tag
// （本项目 wails3 build 不加任何此类 tag），即可达到硬件级吞吐。
func (a Algorithm) New() hash.Hash {
	switch a {
	case MD5:
		return md5.New()
	case SHA1:
		return sha1.New()
	case SHA256:
		return sha256.New()
	case SHA512:
		return sha512.New()
	case CRC32:
		return crc32.NewIEEE()
	}
	return sha256.New()
}

// Valid 报告算法标识是否受支持。
func (a Algorithm) Valid() bool {
	switch a {
	case MD5, SHA1, SHA256, SHA512, CRC32:
		return true
	}
	return false
}

// DetectByLength 按十六进制哈希字符串长度推断算法：
// 32=MD5、40=SHA-1、64=SHA-256、128=SHA-512。
func DetectByLength(n int) (Algorithm, error) {
	switch n {
	case 32:
		return MD5, nil
	case 40:
		return SHA1, nil
	case 64:
		return SHA256, nil
	case 128:
		return SHA512, nil
	}
	return "", fmt.Errorf("cannot detect algorithm by hash length %d", n)
}
