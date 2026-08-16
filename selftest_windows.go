//go:build windows

package main

import (
	"os"
	"syscall"
)

var (
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procAttachConsole = kernel32.NewProc("AttachConsole")
	procGetStdHandle  = kernel32.NewProc("GetStdHandle")
)

// attachParentConsole 附加到父进程控制台并重打开 stdout/stderr，
// 使 windowsgui 子系统构建的 exe 在 --selftest 下也能向终端打印结果。
// 没有父控制台（如双击运行）时静默跳过，退出码仍然有效。
func attachParentConsole() {
	const attachParentProcess = ^uintptr(0) // (uintptr)-1 = ATTACH_PARENT_PROCESS
	const invalidHandle = ^uintptr(0)
	if r, _, _ := procAttachConsole.Call(attachParentProcess); r == 0 {
		return
	}
	const stdOutputHandle = 0xFFFFFFF5 // (uint32)-11
	const stdErrorHandle = 0xFFFFFFF4  // (uint32)-12
	if h, _, _ := procGetStdHandle.Call(stdOutputHandle); h != 0 && h != invalidHandle {
		os.Stdout = os.NewFile(h, "stdout")
	}
	if h, _, _ := procGetStdHandle.Call(stdErrorHandle); h != 0 && h != invalidHandle {
		os.Stderr = os.NewFile(h, "stderr")
	}
}
