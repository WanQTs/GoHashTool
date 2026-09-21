//go:build windows

package main

// CLI 模式的控制台输出：windowsgui 子系统默认没有控制台。
// 1) 标准句柄已是文件/管道（重定向、Git Bash MinTTY 的管道模拟）→ 直接写 UTF-8 字节，
//    无需附加控制台，也规避了伪终端环境下附加控制台可能截断输出的问题；
// 2) 否则附加父控制台（cmd / PowerShell 交互式运行），控制台句柄经 WriteConsoleW
//    以 UTF-16 写入——中文路径在 GBK 代码页控制台下也不乱码，且不改动父控制台代码页；
// 3) 都没有（无父控制台的环境）→ 静默丢弃输出，退出码仍然有效（同 --selftest 契约）。

import (
	"io"
	"os"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	procGetFileType    = kernel32.NewProc("GetFileType")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
	procWriteConsoleW  = kernel32.NewProc("WriteConsoleW")
)

const (
	cliStdOutputHandle = 0xFFFFFFF5 // (uint32)-11
	cliStdErrorHandle  = 0xFFFFFFF4 // (uint32)-12
	cliInvalidHandle   = ^uintptr(0)

	fileTypeDisk = 1 // FILE_TYPE_DISK
	fileTypeChar = 2 // FILE_TYPE_CHAR（控制台即属此类）
	fileTypePipe = 3 // FILE_TYPE_PIPE
)

// setupCLIOutput 返回 CLI 的 stdout/stderr writer（见文件头注释的判定顺序）。
func setupCLIOutput() (stdout, stderr io.Writer) {
	return cliWriter(cliStdOutputHandle), cliWriter(cliStdErrorHandle)
}

func cliWriter(stdHandle uint32) io.Writer {
	// 已继承的句柄（重定向到文件/管道时无需附加控制台即可直接写）。
	if w := cliWriterFromHandle(getStdHandle(stdHandle)); w != nil {
		return w
	}
	// 无可用继承句柄：附加父控制台后重取。
	const attachParentProcess = ^uintptr(0) // (uintptr)-1 = ATTACH_PARENT_PROCESS
	if r, _, _ := procAttachConsole.Call(attachParentProcess); r == 0 {
		return io.Discard
	}
	if w := cliWriterFromHandle(getStdHandle(stdHandle)); w != nil {
		return w
	}
	return io.Discard
}

func getStdHandle(stdHandle uint32) uintptr {
	h, _, _ := procGetStdHandle.Call(uintptr(stdHandle))
	if h == 0 || h == cliInvalidHandle {
		return 0
	}
	return h
}

// cliWriterFromHandle 按句柄类型选择写入方式；句柄不可用返回 nil。
func cliWriterFromHandle(h uintptr) io.Writer {
	if h == 0 {
		return nil
	}
	ft, _, _ := procGetFileType.Call(h)
	switch ft {
	case fileTypeDisk, fileTypePipe:
		return os.NewFile(h, "gohash-cli")
	case fileTypeChar:
		if isConsoleHandle(h) {
			return consoleWriter{h: syscall.Handle(h)}
		}
		return os.NewFile(h, "gohash-cli") // 其他字符设备兜底按字节写
	}
	return nil
}

func isConsoleHandle(h uintptr) bool {
	var mode uint32
	r, _, _ := procGetConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode)))
	return r != 0
}

// consoleWriter 以 UTF-16 写入真实控制台（WriteConsoleW）。
type consoleWriter struct {
	h syscall.Handle
}

func (w consoleWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	u16 := utf16.Encode([]rune(string(p)))
	var written uint32
	r, _, err := procWriteConsoleW.Call(
		uintptr(w.h),
		uintptr(unsafe.Pointer(&u16[0])),
		uintptr(len(u16)),
		uintptr(unsafe.Pointer(&written)),
		0,
	)
	if r == 0 {
		return 0, err
	}
	return len(p), nil
}
