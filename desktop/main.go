package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
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
		Bind:             []interface{}{app},
	})
	if err != nil {
		log.Fatal(err)
	}
}
