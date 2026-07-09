package main

import (
	"context"
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "QuickText",
		Width:     1200,
		Height:    800,
		MinWidth:  600,
		MinHeight: 400,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 24, G: 24, B: 26, A: 1},
		OnStartup: func(ctx context.Context) {
			app.startup(ctx)
			if len(os.Args) > 1 {
				filePath := os.Args[1]
				if _, err := os.Stat(filePath); err == nil {
					app.pendingFile = filePath
				}
			}
		},
		OnBeforeClose: app.BeforeClose,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
