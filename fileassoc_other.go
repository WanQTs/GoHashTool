//go:build !windows

package main

import "log/slog"

// 文件关联注册表读写仅 Windows 平台有；其他平台均为空实现（不支持），
// 绑定方法经 fileAssocSupported 判断后返回结构化错误。

func fileAssocSupported() bool { return false }

func fileAssocCount() int { return 0 }

func registerFileAssoc(string, *slog.Logger) int { return 0 }

func unregisterFileAssoc(*slog.Logger) int { return 0 }

func healFileAssoc(string, *slog.Logger) {}
