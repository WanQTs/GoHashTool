package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
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

	hashService := NewApp()

	app := application.New(application.Options{
		Name:        "gohash",
		Description: "文件哈希工具 File Hash Tool",
		Services: []application.Service{
			application.NewService(hashService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})
	hashService.attach(app)

	// Mica 云母背景：需 BackgroundTypeTranslucent（Win11 22621+，低版本自动回退实色）；
	// 前端页面背景随之改为透明（见 style.css），面板以半透明色浮于云母之上。
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:           "main",
		Title:          "文件哈希工具 File Hash Tool",
		Width:          1280,
		Height:         800,
		MinWidth:       960,
		MinHeight:      640,
		BackgroundType: application.BackgroundTypeTranslucent,
		EnableFileDrop: true,
		Windows: application.WindowsWindow{
			BackdropType: application.Mica,
			Theme:        application.SystemDefault,
		},
	})

	// 整窗文件拖拽：v3 在窗口级 EnableFileDrop + 前端 <body data-file-drop-target>，
	// 悬停高亮 class 为 file-drop-target-active（见 style.css）。
	// 前端事件拿不到绝对路径，这里在 Go 侧接收系统拖拽事件并把路径转发给前端路由。
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		if files := e.Context().DroppedFiles(); len(files) > 0 {
			app.Event.Emit("files-dropped", files)
		}
	})

	// 退出前取消所有运行中任务（引擎每 1 秒内响应取消）。
	app.OnShutdown(func() { hashService.cancelAll() })

	window.Show()
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
