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

	// window 先声明后赋值：SingleInstance 回调在闭包中引用它，
	// 而二实例回调只可能发生在 app.Run 之后（彼时窗口已创建）。
	var window *application.WebviewWindow
	app := application.New(application.Options{
		Name:        "gohash",
		Description: "文件哈希工具 File Hash Tool",
		Services: []application.Service{
			application.NewService(hashService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		// 单实例：重复启动时二实例把命令行参数转发给首实例后退出；
		// 首实例聚焦主窗口，若参数带着清单文件则转交前端路由（双击清单的场景）。
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "gohash-file-hash-tool",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				if window != nil {
					window.Restore()
					window.Focus()
				}
				if p := manifestArgFromArgs(data.Args); p != "" {
					hashService.notifyOpenWithFile(p)
				}
			},
		},
		// 声明可关联的清单扩展名：双击清单启动时收到 ApplicationOpenedWithFile。
		FileAssociations: assocExts,
	})
	hashService.attach(app)

	// 文件关联启动（首实例）：暂存清单路径，前端挂载后拉取（见 ConsumePendingOpenFile）。
	app.Event.OnApplicationEvent(events.Common.ApplicationOpenedWithFile, func(e *application.ApplicationEvent) {
		if p := e.Context().Filename(); p != "" {
			hashService.notifyOpenWithFile(p)
		}
	})

	// 注册清单文件关联到当前用户（HKCU 免管理员；已被占用的扩展名跳过不劫持），
	// 失败仅记日志，不影响启动。
	if exe, err := os.Executable(); err == nil {
		registerFileAssociations(exe, app.Logger)
	} else {
		app.Logger.Error("resolve executable path failed", "error", err)
	}

	// Mica 云母背景：需 BackgroundTypeTranslucent（Win11 22621+，低版本自动回退实色）；
	// 前端页面背景随之改为透明（见 style.css），面板以半透明色浮于云母之上。
	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
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
