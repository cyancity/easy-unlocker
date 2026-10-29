package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

// version 由 release.sh 用 -ldflags -X main.version=<tag> 注入；本地构建是 dev。
var version = "dev"

func main() {
	// 安装器/支持场景问版本：打印即退，不进 GUI，也不触发单实例唤醒。
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println(version)
		return
	}

	// GUI 子系统进程没有控制台，错误全部进日志文件。
	logPath := filepath.Join(os.TempDir(), "easy-unlocker-desktop.log")
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		defer f.Close()
		log.SetOutput(f)
	}
	log.Printf("=== start %s ===", version)

	app := NewApp()
	err := wails.Run(&options.App{
		Title:     "easy-unlocker",
		Width:     1120,
		Height:    760,
		MinWidth:  820,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		// Mica 透出需要三层配合：窗口半透明 + webview 透明 + html 背景透明。
		// Win11 22621 以下自动退化为不透明窗口，CSS 里的兜底底色接管。
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		Windows: &windows.Options{
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			BackdropType:         windows.Mica,
			Theme:                windows.SystemDefault,
		},
		// 单实例：开始菜单/托盘连点只唤起已有实例，不出现第二个窗口和托盘图标。
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "cyancity-easy-unlocker-desktop",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) { app.showWindow() },
		},
		OnStartup: app.startup,
		// 常驻托盘：关窗只隐藏，真退出走托盘「退出」。
		OnBeforeClose: func(ctx context.Context) bool {
			if app.quitting.Load() {
				return false
			}
			app.hideWindow()
			return true
		},
		OnShutdown: func(ctx context.Context) { app.stopTray() },
		Bind:       []interface{}{app},
		LogLevel:   logger.DEBUG,
	})
	if err != nil {
		log.Fatal("wails.Run: ", err)
	}
	log.Println("=== clean exit ===")
}
