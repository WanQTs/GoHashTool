//go:build !windows

package main

// 非 Windows 平台没有控制台子系统之分，CLI 直接向标准输出打印。

import (
	"io"
	"os"
)

func setupCLIOutput() (stdout, stderr io.Writer) {
	return os.Stdout, os.Stderr
}
