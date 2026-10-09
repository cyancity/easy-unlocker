package main

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// GUI 子系统进程没有控制台，错误全部进日志文件。
	logPath := filepath.Join(os.TempDir(), "easy-unlocker-desktop.log")
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		defer f.Close()
		log.SetOutput(f)
	}
	log.Println("=== start ===")

	app := NewApp()
	err := wails.Run(&options.App{
		Title:     "easy-unlocker",
		Width:     1080,
		Height:    740,
		MinWidth:  760,
		MinHeight: 560,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 13, G: 15, B: 20, A: 255},
		OnStartup:        app.startup,
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
