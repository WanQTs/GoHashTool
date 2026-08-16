//go:build !windows

package main

// 非 Windows 平台没有控制台子系统之分，--selftest 直接打印即可。
func attachParentConsole() {}
