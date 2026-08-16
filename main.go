package main

import (
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// --selftest：无 GUI 核心功能自检（供冒烟脚本验证「算得对」），以退出码报告结果。
	for _, arg := range os.Args[1:] {
		if arg == "--selftest" {
			os.Exit(runSelfTest())
		}
	}

	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "文件哈希工具 File Hash Tool",
		Width:     1280,
		Height:    800,
		MinWidth:  960,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
		// 整窗文件拖拽：前端 runtime.OnFileDrop 接收绝对路径；
		// 悬停高亮由 Wails 给 --wails-drop-target: drop 元素自动加
		// wails-drop-target-active class 实现（见 style.css 的 ::after 虚线框）。
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
