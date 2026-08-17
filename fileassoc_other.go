//go:build !windows

package main

import "log/slog"

// registerFileAssociations 仅 Windows 有资源管理器扩展名关联注册；其他平台空实现。
func registerFileAssociations(string, *slog.Logger) {}
